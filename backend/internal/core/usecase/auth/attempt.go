package auth

import (
	"context"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/clientip"
)

// reserveAttempt checks the limits and stores the attempt as failed before any
// password is tested, under a lock per client and account: parallel requests see
// each other, so none passes a limit because the others are still hashing.
func (s *Service) reserveAttempt(ctx context.Context, email string, ip, ua *string, now time.Time) (string, error) {
	var id, limited string
	var wait time.Duration
	err := s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		attempts := uow.Attempts()
		// Always this order: two logins never wait on each other's lock.
		if ip != nil && *ip != "" {
			first, _ := clientip.Range(*ip)
			if err := uow.Lock(ctx, "login:ip:"+first); err != nil {
				return err
			}
		}
		if err := uow.Lock(ctx, "login:email:"+email); err != nil {
			return err
		}

		var err error
		if limited, wait, err = s.overLimit(ctx, attempts, email, ip, now); err != nil {
			return err
		}
		kind := domain.FailureBadPassword
		if limited != "" {
			kind = limited
		}
		a := &domain.LoginAttempt{CreatedAt: now, Email: email, IPAddress: ip, UserAgent: ua, FailureKind: &kind}
		if err := attempts.Create(ctx, a); err != nil {
			return err
		}
		id = a.ID
		return nil
	})
	if err != nil {
		return "", err
	}
	if limited != "" {
		return "", apperror.New(apperror.CodeTooManyRequests, "too many failed attempts").
			WithDetails(apperror.DetailRetryAfter, max(1, int(wait.Seconds())))
	}
	return id, nil
}

// overLimit names the limit this login is over, if any, and how long until
// its oldest failure leaves the window and the limit opens again.
func (s *Service) overLimit(ctx context.Context, attempts output.LoginAttemptRepository, email string, ip *string, now time.Time) (string, time.Duration, error) {
	n, err := attempts.CountRecentFailures(ctx, email, ip, now.Add(-s.Cfg.LoginAttemptEmailWindow), now.Add(-s.Cfg.LoginAttemptIPWindow))
	left := func(oldest *time.Time, window time.Duration) time.Duration {
		if oldest == nil {
			return window
		}
		return oldest.Add(window).Sub(now)
	}
	switch {
	case err != nil:
		return "", 0, err
	case n.Email >= s.Cfg.LoginAttemptEmailMaxFailures:
		return domain.FailureRateLimitedEmail, left(n.EmailOldest, s.Cfg.LoginAttemptEmailWindow), nil
	case n.Pair >= s.Cfg.LoginAttemptPairMaxFailures:
		return domain.FailureRateLimitedEmail, left(n.PairOldest, s.Cfg.LoginAttemptEmailWindow), nil
	case n.IP >= s.Cfg.LoginAttemptIPMaxFailures:
		return domain.FailureRateLimitedIP, left(n.IPOldest, s.Cfg.LoginAttemptIPWindow), nil
	}
	return "", 0, nil
}
