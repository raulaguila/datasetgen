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
	"github.com/raulaguila/dataset/pkg/pointer"
	"github.com/raulaguila/dataset/pkg/reqctx"
	"github.com/raulaguila/dataset/pkg/tokenhash"
	"github.com/raulaguila/dataset/pkg/validate"
)

const (
	maxName    = 80
	maxURL     = 2000
	maxHooks   = 10
	maxKeys    = 20 // per member
	maxKeyDays = 365
	// maxScopes caps the collections one key reaches.
	maxScopes = 50
)

// Service keeps keys and hooks behind the owner role; the key secret is stored
// only as a hash, the hook secret sealed with SealKey (it signs every call).
type Service struct {
	Store   output.UnitOfWork
	Tx      output.TxManager
	Send    output.WebhookSender
	SealKey []byte
	// Records runs automations' conditions and changes, as the automation's maker.
	Records input.RecordService
	// Mail seals an automation's email for the outbox the mailer sends from.
	Mail output.EmailTokenCodec
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

// DeleteKey takes the collection out of a key: its owner decides who reads it,
// not what else the key reaches.
func (s *Service) DeleteKey(ctx context.Context, collectionID, id string) error {
	if err := validate.ID(id); err != nil {
		return err
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := s.owner(ctx, uow, collectionID); err != nil {
			return err
		}
		k, err := uow.Integrations().DropScope(ctx, collectionID, id)
		if err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "collection.key_delete", collectionID, map[string]any{"collection_id": collectionID, "key": k.Name, "prefix": k.Prefix})
	})
}

func (s *Service) MyKeys(ctx context.Context) ([]domain.APIKey, error) {
	return s.Store.Integrations().UserKeys(ctx, reqctx.Actor(ctx))
}

// keyInput checks a key's name and collections; the roles are checked in the transaction.
func keyInput(ctx context.Context, name string, scopes []domain.KeyScope) (string, []domain.KeyScope, error) {
	if _, byKey := reqctx.Key(ctx); byKey {
		return "", nil, apperror.New(apperror.CodeForbidden, "an API key cannot manage keys")
	}
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > maxName {
		return "", nil, apperror.New(apperror.CodeInvalidInput, "name must be 1-80 characters").WithField("name")
	}
	if len(scopes) < 1 || len(scopes) > maxScopes {
		return "", nil, apperror.New(apperror.CodeInvalidInput, "pick 1 to 50 collections").WithField("scopes")
	}
	out := make([]domain.KeyScope, 0, len(scopes))
	seen := map[string]bool{}
	for _, sc := range scopes {
		if err := validate.ID(sc.CollectionID); err != nil {
			return "", nil, apperror.New(apperror.CodeInvalidInput, "invalid collection").WithField("scopes")
		}
		if seen[sc.CollectionID] {
			return "", nil, apperror.New(apperror.CodeInvalidInput, "a collection appears twice").WithField("scopes")
		}
		seen[sc.CollectionID] = true
		out = append(out, domain.KeyScope{CollectionID: sc.CollectionID, CanWrite: sc.CanWrite})
	}
	return name, out, nil
}

// allowed checks the caller may hand each collection to a key: viewer to read, editor to write.
func allowed(ctx context.Context, uow output.UnitOfWork, scopes []domain.KeyScope) error {
	for _, sc := range scopes {
		need := domain.RoleViewer
		if sc.CanWrite {
			need = domain.RoleEditor
		}
		if _, err := policy.CollectionRole(ctx, uow.Collections(), sc.CollectionID, need); err != nil {
			return err
		}
	}
	return nil
}

func keyAudit(ctx context.Context, uow output.UnitOfWork, action, name, prefix string, sc domain.KeyScope) error {
	return policy.Audit(ctx, uow, action, sc.CollectionID, map[string]any{"collection_id": sc.CollectionID, "key": name, "prefix": prefix, "write": sc.CanWrite})
}

// expiryOf is the end of a key that lasts days from now; 0 means no end.
func expiryOf(days int, field string) (*time.Time, error) {
	if days < 0 || days > maxKeyDays {
		return nil, apperror.New(apperror.CodeInvalidInput, "a key lasts 1 to 365 days, or 0 for no end").WithField(field)
	}
	if days == 0 {
		return nil, nil
	}
	return pointer.Of(time.Now().Add(time.Duration(days) * 24 * time.Hour)), nil
}

func (s *Service) CreateMyKey(ctx context.Context, name string, scopes []domain.KeyScope, expiresInDays int) (*input.NewKey, error) {
	name, scopes, err := keyInput(ctx, name, scopes)
	if err != nil {
		return nil, err
	}
	expires, err := expiryOf(expiresInDays, "expires_in_days")
	if err != nil {
		return nil, err
	}
	random, err := tokenhash.RandomURLSafe()
	if err != nil {
		return nil, err
	}
	secret := domain.APIKeyPrefix + random
	actor := reqctx.Actor(ctx)
	out := &input.NewKey{Secret: secret, Key: domain.APIKey{Name: name, Prefix: secret[:12], Scopes: scopes, CreatedBy: actor, ExpiresAt: expires}}
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := allowed(ctx, uow, scopes); err != nil {
			return err
		}
		existing, err := uow.Integrations().UserKeys(ctx, actor)
		if err != nil {
			return err
		}
		if len(existing) >= maxKeys {
			return apperror.New(apperror.CodeInvalidInput, "you hold up to 20 keys; delete one first")
		}
		if err := uow.Integrations().CreateKey(ctx, &out.Key, tokenhash.SHA256Bytes(secret)); err != nil {
			return err
		}
		for _, sc := range scopes {
			if err := keyAudit(ctx, uow, "collection.key_create", name, out.Key.Prefix, sc); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateMyKey renames a key and sets its collections; each collection it gains,
// loses or changes access to logs the change. renewDays, when set, gives the key
// a new end counted from now (0: no end), which also brings an expired key back.
func (s *Service) UpdateMyKey(ctx context.Context, id, name string, scopes []domain.KeyScope, renewDays *int) (*domain.APIKey, error) {
	if err := validate.ID(id); err != nil {
		return nil, err
	}
	name, scopes, err := keyInput(ctx, name, scopes)
	if err != nil {
		return nil, err
	}
	var expires *time.Time
	if renewDays != nil {
		if expires, err = expiryOf(*renewDays, "renew_days"); err != nil {
			return nil, err
		}
	}
	actor := reqctx.Actor(ctx)
	var out *domain.APIKey
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		old, err := uow.Integrations().UserKey(ctx, id, actor)
		if err != nil {
			return err
		}
		before := map[string]bool{}
		for _, sc := range old.Scopes {
			before[sc.CollectionID] = sc.CanWrite
		}
		// Only what changes needs the role: keeping a collection the member can no longer reach is harmless.
		var changed []domain.KeyScope
		for _, sc := range scopes {
			if w, ok := before[sc.CollectionID]; !ok || w != sc.CanWrite {
				changed = append(changed, sc)
			}
		}
		if err := allowed(ctx, uow, changed); err != nil {
			return err
		}
		if err := uow.Integrations().UpdateKey(ctx, id, name, scopes); err != nil {
			return err
		}
		if renewDays != nil {
			if err := uow.Integrations().SetKeyExpiry(ctx, id, expires); err != nil {
				return err
			}
		}
		after := map[string]bool{}
		for _, sc := range changed {
			action := "collection.key_create"
			if _, ok := before[sc.CollectionID]; ok {
				action = "collection.key_update"
			}
			if err := keyAudit(ctx, uow, action, name, old.Prefix, sc); err != nil {
				return err
			}
		}
		for _, sc := range scopes {
			after[sc.CollectionID] = true
		}
		for _, sc := range old.Scopes {
			if !after[sc.CollectionID] {
				if err := keyAudit(ctx, uow, "collection.key_delete", old.Name, old.Prefix, sc); err != nil {
					return err
				}
			}
		}
		out, err = uow.Integrations().UserKey(ctx, id, actor)
		return err
	})
	return out, err
}

func (s *Service) DeleteMyKey(ctx context.Context, id string) error {
	if err := validate.ID(id); err != nil {
		return err
	}
	if _, byKey := reqctx.Key(ctx); byKey {
		return apperror.New(apperror.CodeForbidden, "an API key cannot manage keys")
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		k, err := uow.Integrations().UserKey(ctx, id, reqctx.Actor(ctx))
		if err != nil {
			return err
		}
		if err := uow.Integrations().DeleteKey(ctx, id); err != nil {
			return err
		}
		for _, sc := range k.Scopes {
			if err := keyAudit(ctx, uow, "collection.key_delete", k.Name, k.Prefix, sc); err != nil {
				return err
			}
		}
		return nil
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
	if g.Share != "" {
		return nil, bad
	}
	return g, nil
}

const maxShares = 10

func (s *Service) Shares(ctx context.Context, collectionID string) ([]domain.Share, error) {
	if err := s.owner(ctx, s.Store, collectionID); err != nil {
		return nil, err
	}
	shares, err := s.Store.Integrations().Shares(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	for i := range shares {
		if shares[i].Token, err = aescrypt.Decrypt(s.SealKey, shares[i].Token); err != nil {
			return nil, err
		}
	}
	return shares, nil
}

func (s *Service) CreateShare(ctx context.Context, collectionID, kind string) (*domain.Share, error) {
	if kind != domain.ShareForm && kind != domain.ShareView {
		return nil, apperror.New(apperror.CodeInvalidInput, "kind must be form or view").WithField("kind")
	}
	random, err := tokenhash.RandomURLSafe()
	if err != nil {
		return nil, err
	}
	token := domain.SharePrefix + random
	sealed, err := aescrypt.Encrypt(s.SealKey, token)
	if err != nil {
		return nil, err
	}
	var out *domain.Share
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := s.owner(ctx, uow, collectionID); err != nil {
			return err
		}
		if _, byKey := reqctx.Key(ctx); byKey {
			return apperror.New(apperror.CodeForbidden, "an API key cannot make links")
		}
		existing, err := uow.Integrations().Shares(ctx, collectionID)
		if err != nil {
			return err
		}
		if len(existing) >= maxShares {
			return apperror.New(apperror.CodeInvalidInput, "a collection holds up to 10 links; delete one first")
		}
		if out, err = uow.Integrations().CreateShare(ctx, collectionID, kind, sealed, tokenhash.SHA256Bytes(token), reqctx.Actor(ctx)); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "collection.share_create", collectionID, map[string]any{"collection_id": collectionID, "kind": kind})
	})
	if err != nil {
		return nil, err
	}
	out.Token = token
	return out, nil
}

func (s *Service) DeleteShare(ctx context.Context, collectionID, id string) error {
	if err := validate.ID(id); err != nil {
		return err
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := s.owner(ctx, uow, collectionID); err != nil {
			return err
		}
		kind, err := uow.Integrations().DeleteShare(ctx, collectionID, id)
		if err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "collection.share_delete", collectionID, map[string]any{"collection_id": collectionID, "kind": kind})
	})
}

func (s *Service) AuthenticateShare(ctx context.Context, token string) (*domain.KeyGrant, error) {
	bad := apperror.New(apperror.CodeNotFound, "link not found")
	if !strings.HasPrefix(token, domain.SharePrefix) || len(token) > 100 {
		return nil, bad
	}
	g, err := s.Store.Integrations().Grant(ctx, tokenhash.SHA256Bytes(token))
	if err != nil {
		if apperror.IsCode(err, apperror.CodeNotFound) {
			return nil, bad
		}
		return nil, err
	}
	if g.Share == "" {
		return nil, bad
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
		u, err := uow.Integrations().DeleteWebhook(ctx, collectionID, id)
		if err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "collection.webhook_delete", collectionID, map[string]any{"collection_id": collectionID, "url": u})
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
	d := &domain.WebhookDelivery{WebhookID: h.ID, Event: asString(payload["event"]), Body: body, Error: errText}
	if status > 0 {
		d.Status = &status
	}
	if err := s.Store.Integrations().LogDelivery(ctx, d); err != nil {
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
		policy.JobAutomation: s.runJob,
	}
}
