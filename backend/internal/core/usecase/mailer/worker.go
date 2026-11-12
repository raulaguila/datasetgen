package mailer

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/validate"
)

const (
	defaultBatch   = 10
	defaultPoll    = 2 * time.Second
	maxAttempts    = 14 // about 75 minutes of backoff, to outlast an SMTP outage
	maxBackoffSecs = 15 * 60
	maxErrorBytes  = 500
	// claimLease must outlast a full batch of sends (10 x the 30s SMTP deadline).
	claimLease  = 10 * time.Minute
	markTimeout = 5 * time.Second
)

// MailSender sends a rendered email.
type MailSender interface {
	Enabled() bool
	Send(to, subject, body string) error
}

// Worker claims outbox rows and sends email.
type Worker struct {
	Heartbeat func() // optional liveness signal after a successful queue poll or send
	Outbox    output.OutboxRepository
	Mail      MailSender
	Log       output.Logger
	Clock     output.Clock
	Codec     output.EmailTokenCodec
	AppURL    string // web app address for links; empty sends the bare token
	Batch     int
	Poll      time.Duration
}

func (w *Worker) Run(ctx context.Context) error {
	if w.Batch <= 0 {
		w.Batch = defaultBatch
	}
	if w.Poll <= 0 {
		w.Poll = defaultPoll
	}

	ticker := time.NewTicker(w.Poll)
	defer ticker.Stop()

	output.Log(w.Log).Info("Mailer worker started", slog.Int("batch", w.Batch), slog.Duration("poll", w.Poll))
	for {
		if err := w.tick(ctx); err != nil {
			output.Log(w.Log).Warn("Mailer tick error", slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
			output.Log(w.Log).Info("Mailer worker stopped")
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *Worker) tick(ctx context.Context) error {
	now := w.Clock.Now()
	msgs, err := w.Outbox.ClaimPending(ctx, w.Batch, now, claimLease)
	if err != nil {
		return err
	}
	if w.Heartbeat != nil {
		w.Heartbeat()
	}
	for i := range msgs {
		if ctx.Err() != nil { // shutting down: unsent rows return when the lease ends
			return ctx.Err()
		}
		w.process(ctx, &msgs[i])
		if w.Heartbeat != nil {
			w.Heartbeat()
		}
	}
	return nil
}

func (w *Worker) process(ctx context.Context, msg *domain.OutboxMessage) {
	to, subject, body, err := render(msg, w.Codec, w.AppURL)
	if err != nil {
		w.fail(ctx, msg, err.Error(), true)
		return
	}
	if !w.Mail.Enabled() {
		w.fail(ctx, msg, "smtp_disabled", true)
		return
	}
	if err := w.Mail.Send(to, subject, body); err != nil {
		attempts := msg.Attempts + 1
		terminal := attempts >= maxAttempts || err.Error() == "smtp_misconfigured"
		w.fail(ctx, msg, validate.Text(err.Error(), maxErrorBytes), terminal)
		return
	}
	now := w.Clock.Now()
	mark, cancel := markContext(ctx)
	defer cancel()
	if err := w.Outbox.MarkSent(mark, msg.ID, msg.ClaimID, now); err != nil {
		output.Log(w.Log).Warn("MarkSent failed", slog.String("outbox_id", msg.ID), slog.String("error", err.Error()))
		return
	}
	output.Log(w.Log).Info("Email sent", slog.String("outbox_id", msg.ID), slog.String("purpose", msg.Purpose), slog.String("request_id", msg.RequestID))
}

func (w *Worker) fail(ctx context.Context, msg *domain.OutboxMessage, lastErr string, terminal bool) {
	now := w.Clock.Now()
	attempts := msg.Attempts + 1
	backoff := time.Duration(math.Min(math.Pow(2, float64(attempts)), float64(maxBackoffSecs))) * time.Second
	available := now.Add(backoff)
	var processed *time.Time
	if terminal {
		processed = &now
	}
	mark, cancel := markContext(ctx)
	defer cancel()
	if err := w.Outbox.MarkFailed(mark, msg.ID, msg.ClaimID, attempts, available, processed, lastErr, terminal); err != nil {
		output.Log(w.Log).Warn("MarkFailed failed", slog.String("outbox_id", msg.ID), slog.String("error", err.Error()))
		return
	}
	output.Log(w.Log).Warn("Email send failed",
		slog.String("outbox_id", msg.ID),
		slog.String("purpose", msg.Purpose),
		slog.String("request_id", msg.RequestID),
		slog.Int("attempts", attempts),
		slog.Bool("terminal", terminal),
		slog.String("error", lastErr),
	)
}

// markContext starts after the send: a result must be recorded even when the
// send was slow or shutdown cancelled ctx meanwhile.
func markContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), markTimeout)
}

// emailText is one email in one language. Link and Bare end the sentence that
// introduces the link, or the token alone when no web app address is set.
type emailText struct {
	Subject, Hello, Link, OrToken, Bare string
}

var emailTexts = map[string]map[string]emailText{
	domain.OutboxPurposeActivation: {
		"pt": {"Ative sua conta", "Olá,", "Abra este link para definir sua senha e ativar sua conta:", "Ou informe este código nessa página:", "Use este código para definir sua senha e ativar sua conta:"},
		"en": {"Activate your account", "Hello,", "Open this link to set your password and activate your account:", "Or enter this token on that page:", "Use this token to set your password and activate your account:"},
	},
	domain.OutboxPurposePasswordReset: {
		"pt": {"Redefinição de senha", "Olá,", "Abra este link para escolher uma nova senha:", "Ou informe este código nessa página:", "Use este código para escolher uma nova senha:"},
		"en": {"Password reset", "Hello,", "Open this link to choose a new password:", "Or enter this token on that page:", "Use this token to choose a new password:"},
	},
}

var emailPages = map[string]string{
	domain.OutboxPurposeActivation:    "/activate",
	domain.OutboxPurposePasswordReset: "/reset-password",
}

func render(msg *domain.OutboxMessage, codec output.EmailTokenCodec, appURL string) (to, subject, body string, err error) {
	p, err := codec.Decode(msg.Payload)
	if err != nil {
		return "", "", "", err
	}
	texts, ok := emailTexts[msg.Purpose]
	if !ok {
		return "", "", "", fmt.Errorf("unknown_purpose")
	}
	lang := "pt" // domain.DefaultLanguage; rows queued before Lang existed have none
	if p.Lang != "" && !strings.HasPrefix(strings.ToLower(p.Lang), "pt") {
		lang = "en"
	}
	t := texts[lang]
	body = t.Hello + "\n\n"
	if appURL != "" {
		// In the fragment the token never reaches a server log or a Referer header.
		body += t.Link + "\n\n" + strings.TrimRight(appURL, "/") + emailPages[msg.Purpose] + "#token=" + url.QueryEscape(p.Token) + "\n\n" + t.OrToken + "\n\n"
	} else {
		body += t.Bare + "\n\n"
	}
	return p.Email, t.Subject, body + p.Token + "\n", nil
}
