package auth

import (
	"context"
	"log/slog"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/tokenhash"
)

// Config holds auth policy from environment (no config package import).
type Config struct {
	SessionMaxLifetime           time.Duration // a session ends this long after login, however often it refreshes
	RegistrationTokenTTL         time.Duration
	PasswordResetTokenTTL        time.Duration
	MaxFailedLoginAttempts       int
	AccountLockDuration          time.Duration
	AllowManualTokenDelivery     bool
	LoginAttemptEmailWindow      time.Duration
	LoginAttemptEmailMaxFailures int // all clients together; high, or anyone could lock an account out
	LoginAttemptPairMaxFailures  int // one client against one account
	LoginAttemptIPWindow         time.Duration
	LoginAttemptIPMaxFailures    int
	EmailTokenCooldown           time.Duration // minimum gap between emails of one purpose to one user
	EmailTokenHourlyMax          int           // 0 means no cap
	PasswordResetMinDuration     time.Duration // floor of a reset request, so its timing reveals no account
	AllowRegistration            bool          // off: accounts come only from invitations
}

// Service implements identity auth flows.
type Service struct {
	Tx     output.TxManager
	UOW    output.UnitOfWork
	Tokens output.TokenIssuer
	Clock  output.Clock
	Cfg    Config
	Codec  output.EmailTokenCodec
	Log    output.Logger // optional: a nil logger drops the message
}

var _ input.AuthService = (*Service)(nil)

func New(
	tx output.TxManager,
	uow output.UnitOfWork,
	tokens output.TokenIssuer,
	clock output.Clock,
	cfg Config,
	codec output.EmailTokenCodec,
) *Service {
	return &Service{Tx: tx, UOW: uow, Tokens: tokens, Clock: clock, Cfg: cfg, Codec: codec}
}

// warn reports a side step that failed without failing the request.
func (s *Service) warn(msg string, err error) {
	if err != nil {
		output.Log(s.Log).Warn(msg, slog.String("error", err.Error()))
	}
}

// emailThrottled limits emails per account: the per-IP limit does not stop many
// addresses flooding one mailbox. The hourly cap also protects the link already
// sent, since each new token kills the one before it.
func (s *Service) emailThrottled(ctx context.Context, uow output.UnitOfWork, userID, purpose string) (bool, error) {
	if err := uow.Lock(ctx, "email:"+userID+":"+purpose); err != nil {
		return false, err
	}
	now := s.Clock.Now()
	recent, err := uow.Tokens().IssuedSince(ctx, userID, purpose, now.Add(-s.Cfg.EmailTokenCooldown))
	if recent || err != nil || s.Cfg.EmailTokenHourlyMax <= 0 {
		return recent, err
	}
	n, err := uow.Tokens().CountIssuedSince(ctx, userID, purpose, now.Add(-time.Hour))
	return n >= s.Cfg.EmailTokenHourlyMax, err
}

// replaceToken stores a new token for the user and kills the live one, so only
// the newest link works. Only its hash is kept.
func (s *Service) replaceToken(ctx context.Context, uow output.UnitOfWork, userID, purpose, plain string, ttl time.Duration) (time.Time, error) {
	if err := uow.Tokens().InvalidateActive(ctx, userID, purpose); err != nil {
		return time.Time{}, err
	}
	now := s.Clock.Now()
	tok := &domain.EmailToken{
		UserID:    userID,
		Purpose:   purpose,
		TokenHash: tokenhash.SHA256Bytes(plain),
		ExpiresAt: now.Add(ttl),
		CreatedAt: now,
	}
	return tok.ExpiresAt, uow.Tokens().Create(ctx, tok)
}

// issueEmailToken replaces the user's live token and queues the email. The
// outbox only ever holds the token sealed; the mailer opens it to send.
func (s *Service) issueEmailToken(ctx context.Context, uow output.UnitOfWork, u *domain.User, purpose, plain string, ttl time.Duration) error {
	if _, err := s.replaceToken(ctx, uow, u.ID, purpose, plain, ttl); err != nil {
		return err
	}
	lang := domain.DefaultLanguage // a new account has saved no preference yet
	if prefs, err := uow.Preferences().Find(ctx, u.ID); err == nil {
		lang = prefs.Language
	} else if !apperror.IsNotFound(err) {
		return err
	}
	payload, err := s.Codec.Encode(output.EmailToken{Email: u.Email, Token: plain, Lang: lang})
	if err != nil {
		return apperror.New(apperror.CodeInternal, "email token encoding failed").WithCause(err)
	}

	return uow.Outbox().Enqueue(ctx, purpose, payload)
}
