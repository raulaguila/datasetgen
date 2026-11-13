// Package integration runs a collection's API keys and webhooks.
package integration

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/aescrypt"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
	"github.com/raulaguila/dataset/pkg/tokenhash"
	"github.com/raulaguila/dataset/pkg/validate"
)

const (
	maxName  = 80
	maxURL   = 2000
	maxHooks = 10
	maxKeys  = 20
)

// Service keeps keys and hooks behind the owner role; the key secret is stored
// only as a hash, the hook secret sealed with SealKey (it signs every call).
type Service struct {
	Store   output.UnitOfWork
	Tx      output.TxManager
	Send    output.WebhookSender
	SealKey []byte
}

var _ input.IntegrationService = (*Service)(nil)

func (s *Service) owner(ctx context.Context, uow output.UnitOfWork, collectionID string) error {
	_, err := policy.CollectionRole(ctx, uow.Collections(), collectionID, domain.RoleOwner)
	return err
}

func (s *Service) Keys(ctx context.Context, collectionID string) ([]domain.APIKey, error) {
	if err := s.owner(ctx, s.Store, collectionID); err != nil {
		return nil, err
	}
	return s.Store.Integrations().Keys(ctx, collectionID)
}

func (s *Service) CreateKey(ctx context.Context, collectionID, name string, canWrite bool) (*input.NewKey, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > maxName {
		return nil, apperror.New(apperror.CodeInvalidInput, "name must be 1-80 characters").WithField("name")
	}
	random, err := tokenhash.RandomURLSafe()
	if err != nil {
		return nil, err
	}
	secret := domain.APIKeyPrefix + random
	out := &input.NewKey{Secret: secret, Key: domain.APIKey{CollectionID: collectionID, Name: name, Prefix: secret[:12], CanWrite: canWrite, CreatedBy: reqctx.Actor(ctx)}}
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := s.owner(ctx, uow, collectionID); err != nil {
			return err
		}
		if _, byKey := reqctx.Key(ctx); byKey {
			return apperror.New(apperror.CodeForbidden, "an API key cannot make keys")
		}
		existing, err := uow.Integrations().Keys(ctx, collectionID)
		if err != nil {
			return err
		}
		if len(existing) >= maxKeys {
			return apperror.New(apperror.CodeInvalidInput, "a collection holds up to 20 keys; delete one first")
		}
		if err := uow.Integrations().CreateKey(ctx, &out.Key, tokenhash.SHA256Bytes(secret)); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "collection.key_create", collectionID, map[string]any{"collection_id": collectionID, "key": name, "write": canWrite})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) DeleteKey(ctx context.Context, collectionID, id string) error {
	if err := validate.ID(id); err != nil {
		return err
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := s.owner(ctx, uow, collectionID); err != nil {
			return err
		}
		if err := uow.Integrations().DeleteKey(ctx, collectionID, id); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "collection.key_delete", collectionID, map[string]any{"collection_id": collectionID})
	})
}

func (s *Service) AuthenticateKey(ctx context.Context, secret string) (*domain.KeyGrant, error) {
	bad := apperror.New(apperror.CodeUnauthorized, "invalid API key")
	if !strings.HasPrefix(secret, domain.APIKeyPrefix) || len(secret) > 100 {
		return nil, bad
	}
	g, err := s.Store.Integrations().Grant(ctx, tokenhash.SHA256Bytes(secret))
	if err != nil {
		if apperror.IsCode(err, apperror.CodeNotFound) {
			return nil, bad
		}
		return nil, err
	}
	return g, nil
}

func (s *Service) Webhooks(ctx context.Context, collectionID string) ([]domain.Webhook, error) {
	if err := s.owner(ctx, s.Store, collectionID); err != nil {
		return nil, err
	}
	return s.Store.Integrations().Webhooks(ctx, collectionID)
}

func checkURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || len(raw) > maxURL || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return "", apperror.New(apperror.CodeInvalidInput, "url must be an http or https address").WithField("url")
	}
	return raw, nil
}

func (s *Service) CreateWebhook(ctx context.Context, collectionID, rawURL string, events []string) (*input.NewWebhook, error) {
	u, err := checkURL(rawURL)
	if err != nil {
		return nil, err
	}
	events = slices.Compact(slices.Sorted(slices.Values(events)))
	if len(events) == 0 {
		return nil, apperror.New(apperror.CodeInvalidInput, "pick at least one event").WithField("events")
	}
	for _, e := range events {
		if !slices.Contains(domain.WebhookEvents, e) {
			return nil, apperror.New(apperror.CodeInvalidInput, "unknown event "+e).WithField("events")
		}
	}
	random, err := tokenhash.RandomURLSafe()
	if err != nil {
		return nil, err
	}
	secret := "whsec_" + random
	sealed, err := aescrypt.Encrypt(s.SealKey, secret)
	if err != nil {
		return nil, err
	}
	out := &input.NewWebhook{Secret: secret, Webhook: domain.Webhook{CollectionID: collectionID, URL: u, Secret: sealed, Events: events}}
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := s.owner(ctx, uow, collectionID); err != nil {
			return err
		}
		existing, err := uow.Integrations().Webhooks(ctx, collectionID)
		if err != nil {
			return err
		}
		if len(existing) >= maxHooks {
			return apperror.New(apperror.CodeInvalidInput, "a collection holds up to 10 webhooks; delete one first")
		}
		if err := uow.Integrations().CreateWebhook(ctx, &out.Webhook, reqctx.Actor(ctx)); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "collection.webhook_create", collectionID, map[string]any{"collection_id": collectionID, "url": u})
	})
	if err != nil {
		return nil, err
	}
	out.Webhook.Secret = ""
	return out, nil
}

func (s *Service) DeleteWebhook(ctx context.Context, collectionID, id string) error {
	if err := validate.ID(id); err != nil {
		return err
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := s.owner(ctx, uow, collectionID); err != nil {
			return err
		}
		if err := uow.Integrations().DeleteWebhook(ctx, collectionID, id); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "collection.webhook_delete", collectionID, map[string]any{"collection_id": collectionID})
	})
}

func (s *Service) TestWebhook(ctx context.Context, collectionID, id string) (*domain.Webhook, error) {
	if err := validate.ID(id); err != nil {
		return nil, err
	}
	if err := s.owner(ctx, s.Store, collectionID); err != nil {
		return nil, err
	}
	h, err := s.Store.Integrations().Webhook(ctx, id)
	if err != nil || h.CollectionID != collectionID {
		return nil, apperror.New(apperror.CodeNotFound, "webhook not found")
	}
	_ = s.deliver(ctx, h, map[string]any{"event": "ping", "collection_id": collectionID, "occurred_at": time.Now().UTC().Format(time.RFC3339)})
	hooks, err := s.Store.Integrations().Webhooks(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	for _, w := range hooks {
		if w.ID == id {
			return &w, nil
		}
	}
	return nil, apperror.New(apperror.CodeNotFound, "webhook not found")
}

// Sign is the X-Dataset-Signature value: "t=<unix>,v1=<hex HMAC-SHA256 of "<unix>.<body>">".
// The time inside the signed text lets a receiver refuse old replays.
func Sign(secret string, at time.Time, body []byte) string {
	ts := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

// deliver sends one call and records its answer on the hook.
func (s *Service) deliver(ctx context.Context, h *domain.Webhook, payload map[string]any) error {
	secret, err := aescrypt.Decrypt(s.SealKey, h.Secret)
	if err != nil {
		return policy.PermanentJobError(err)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return policy.PermanentJobError(err)
	}
	status, sendErr := s.Send.Send(ctx, h.URL, body, map[string]string{"X-Dataset-Signature": Sign(secret, time.Now(), body), "X-Dataset-Event": strings.TrimSpace(asString(payload["event"]))})
	errText := ""
	if sendErr != nil {
		errText = sendErr.Error()
		if len(errText) > 300 {
			errText = errText[:300]
		}
	}
	if err := s.Store.Integrations().WebhookResult(ctx, h.ID, status, errText); err != nil {
		return err
	}
	return sendErr
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

// Jobs delivers queued webhook calls; a failure retries with the job backoff.
func (s *Service) Jobs() map[string]policy.JobHandler {
	return map[string]policy.JobHandler{
		policy.JobWebhook: func(ctx context.Context, payload []byte) error {
			var job policy.WebhookJob
			if err := json.Unmarshal(payload, &job); err != nil {
				return policy.PermanentJobError(err)
			}
			h, err := s.Store.Integrations().Webhook(ctx, job.HookID)
			if apperror.IsCode(err, apperror.CodeNotFound) {
				return nil // deleted since: nothing to send
			}
			if err != nil {
				return err
			}
			return s.deliver(ctx, h, job.Body)
		},
	}
}
