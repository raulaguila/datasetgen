package mailer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/raulaguila/dataset/internal/adapter/outbound/emailpayload"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/pkg/aescrypt"
)

var testKey = []byte(strings.Repeat("k", 32))

type fakeOutbox struct {
	msgs     []domain.OutboxMessage
	sent     []string
	failed   map[string]bool // id -> terminal
	lastErr  string
	markErr  error         // set when ctx is already cancelled
	markLeft time.Duration // time left on the MarkSent context
}

func (f *fakeOutbox) Enqueue(context.Context, string, []byte) error { return nil }
func (f *fakeOutbox) ClaimPending(context.Context, int, time.Time, time.Duration) ([]domain.OutboxMessage, error) {
	return f.msgs, nil
}

func (f *fakeOutbox) MarkSent(ctx context.Context, id, _ string, _ time.Time) error {
	if ctx.Err() != nil {
		f.markErr = ctx.Err()
		return ctx.Err()
	}
	if deadline, ok := ctx.Deadline(); ok {
		f.markLeft = time.Until(deadline)
	}
	f.sent = append(f.sent, id)
	return nil
}

func (f *fakeOutbox) MarkFailed(_ context.Context, id, _ string, _ int, _ time.Time, _ *time.Time, lastErr string, terminal bool) error {
	f.lastErr = lastErr
	if f.failed == nil {
		f.failed = map[string]bool{}
	}
	f.failed[id] = terminal
	return nil
}

type fakeMail struct {
	bodies []string
	err    error
	onSend func()
}

func (f *fakeMail) Enabled() bool { return true }
func (f *fakeMail) Send(_, _, body string) error {
	f.bodies = append(f.bodies, body)
	if f.onSend != nil {
		f.onSend()
	}
	return f.err
}

func message(t *testing.T, id, sealed string, attempts int) domain.OutboxMessage {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"email": "a@acme.io", "token": sealed})
	if err != nil {
		t.Fatal(err)
	}
	return domain.OutboxMessage{ID: id, Purpose: domain.OutboxPurposeActivation, Payload: payload, Attempts: attempts}
}

func sealed(t *testing.T, plain string) string {
	t.Helper()
	out, err := aescrypt.Encrypt(testKey, plain)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTickOpensTokenAndMarksSent(t *testing.T) {
	box := &fakeOutbox{msgs: []domain.OutboxMessage{message(t, "1", sealed(t, "plain-token"), 0)}}
	mail := &fakeMail{}
	w := &Worker{Outbox: box, Mail: mail, Codec: emailpayload.New(testKey), Clock: fixedClock{}}
	if err := w.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(box.sent) != 1 || !strings.Contains(mail.bodies[0], "plain-token") {
		t.Fatalf("sent %v bodies %v", box.sent, mail.bodies)
	}
}

func TestBodyLinksToTheWebApp(t *testing.T) {
	box := &fakeOutbox{msgs: []domain.OutboxMessage{message(t, "1", sealed(t, "plain-token"), 0)}}
	mail := &fakeMail{}
	w := &Worker{Outbox: box, Mail: mail, Codec: emailpayload.New(testKey), Clock: fixedClock{}, AppURL: "https://id.acme.io/"}
	if err := w.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mail.bodies[0], "https://id.acme.io/activate#token=plain-token\n") || !strings.Contains(mail.bodies[0], "Olá,") {
		t.Fatalf("body %q", mail.bodies[0])
	}
}

func TestBodyFollowsTheUsersLanguage(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{"email": "a@acme.io", "token": sealed(t, "plain-token"), "lang": "en-US"})
	box := &fakeOutbox{msgs: []domain.OutboxMessage{{ID: "1", Purpose: domain.OutboxPurposePasswordReset, Payload: payload}}}
	mail := &fakeMail{}
	w := &Worker{Outbox: box, Mail: mail, Codec: emailpayload.New(testKey), Clock: fixedClock{}}
	if err := w.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mail.bodies[0], "Use this token to choose a new password:") {
		t.Fatalf("body %q", mail.bodies[0])
	}
}

func TestUnreadableTokenFailsWithoutSending(t *testing.T) {
	box := &fakeOutbox{msgs: []domain.OutboxMessage{message(t, "1", "plain-legacy-token", 0)}}
	mail := &fakeMail{}
	w := &Worker{Outbox: box, Mail: mail, Codec: emailpayload.New(testKey), Clock: fixedClock{}}
	if err := w.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(mail.bodies) != 0 || !box.failed["1"] {
		t.Fatalf("bodies %v failed %v", mail.bodies, box.failed)
	}
}

func TestRetriesThenGivesUp(t *testing.T) {
	box := &fakeOutbox{msgs: []domain.OutboxMessage{
		message(t, "retry", sealed(t, "x"), 0),
		message(t, "last", sealed(t, "x"), maxAttempts-1),
	}}
	w := &Worker{Outbox: box, Mail: &fakeMail{err: errors.New("boom")}, Codec: emailpayload.New(testKey), Clock: fixedClock{}}
	if err := w.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if box.failed["retry"] || !box.failed["last"] {
		t.Fatalf("terminal flags %v", box.failed)
	}
}

func TestLongErrorStaysValidForPostgres(t *testing.T) {
	box := &fakeOutbox{msgs: []domain.OutboxMessage{message(t, "m", sealed(t, "x"), 0)}}
	long := strings.Repeat("a", maxErrorBytes-1) + "é"
	w := &Worker{Outbox: box, Mail: &fakeMail{err: errors.New(long)}, Codec: emailpayload.New(testKey), Clock: fixedClock{}}
	if err := w.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(box.lastErr) || len(box.lastErr) > maxErrorBytes {
		t.Fatalf("%d bytes, valid=%v", len(box.lastErr), utf8.ValidString(box.lastErr))
	}
}

func TestShutdownMidSendStillRecordsResultAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	box := &fakeOutbox{msgs: []domain.OutboxMessage{message(t, "1", sealed(t, "x"), 0), message(t, "2", sealed(t, "x"), 0)}}
	mail := &fakeMail{onSend: cancel}
	w := &Worker{Outbox: box, Mail: mail, Codec: emailpayload.New(testKey), Clock: fixedClock{}}
	if err := w.tick(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if box.markErr != nil || len(box.sent) != 1 || len(mail.bodies) != 1 {
		t.Fatalf("markErr %v sent %v sends %d", box.markErr, box.sent, len(mail.bodies))
	}
}

func TestSlowSendDoesNotEatTheMarkTimeout(t *testing.T) {
	box := &fakeOutbox{msgs: []domain.OutboxMessage{message(t, "1", sealed(t, "x"), 0)}}
	mail := &fakeMail{onSend: func() { time.Sleep(300 * time.Millisecond) }}
	w := &Worker{Outbox: box, Mail: mail, Codec: emailpayload.New(testKey), Clock: fixedClock{}}
	if err := w.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if box.markLeft < markTimeout-200*time.Millisecond {
		t.Fatalf("mark had only %v left after the send", box.markLeft)
	}
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Unix(1000, 0) }
