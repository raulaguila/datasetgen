// Package apikey serves the API keys: each member's own under /v1/me/keys, and the keys
// that reach a collection, for its owners.
package apikey

import (
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
)

type Handler struct {
	svc input.IntegrationService
}

func New(svc input.IntegrationService) *Handler {
	return &Handler{svc: svc}
}

// MeRoutes mounts the caller's own keys on /v1/me/keys.
func (h *Handler) MeRoutes(r fiber.Router) {
	r.Get("/", h.MyKeys)
	r.Post("/", h.CreateMyKey)
	r.Put("/:keyId", h.UpdateMyKey)
	r.Delete("/:keyId", h.DeleteMyKey)
}

// CollectionRoutes mounts the keys that reach a collection on /v1/collections/:owner/:slug.
func (h *Handler) CollectionRoutes(at fiber.Router) {
	at.Get("/keys", h.Keys)
	at.Delete("/keys/:keyId", h.DeleteKey)
}

// ScopeDTO is one collection a key reaches.
type ScopeDTO struct {
	CollectionID   string `json:"collection_id"`
	CollectionName string `json:"collection_name,omitempty"`
	CanWrite       bool   `json:"can_write"`
} // @name KeyScope

type keyDTO struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Prefix      string     `json:"prefix"`
	CanWrite    bool       `json:"can_write"`
	Scopes      []ScopeDTO `json:"scopes,omitempty"`
	CreatorName string     `json:"creator_name,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
} // @name APIKey

func toKeyDTO(k domain.APIKey) keyDTO {
	var scopes []ScopeDTO
	for _, sc := range k.Scopes {
		scopes = append(scopes, ScopeDTO(sc))
	}
	return keyDTO{ID: k.ID, Name: k.Name, Prefix: k.Prefix, CanWrite: k.CanWrite, Scopes: scopes, CreatorName: k.CreatorName, CreatedAt: k.CreatedAt, LastUsedAt: k.LastUsedAt, ExpiresAt: k.ExpiresAt}
}

func keyList(keys []domain.APIKey) fiber.Map {
	out := make([]keyDTO, 0, len(keys))
	for _, k := range keys {
		out = append(out, toKeyDTO(k))
	}
	return fiber.Map{"items": out}
}

// @Summary The API keys that reach a collection; owners
// @Description can_write is each key's access to this collection.
// @Tags API keys
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Security BearerAuth
// @Success 200 {object} object{items=[]keyDTO}
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID apikeyCollectionList
// @Router /v1/collections/{owner}/{slug}/keys [get]
func (h *Handler) Keys(c fiber.Ctx) error {
	keys, err := h.svc.Keys(c.Context(), middleware.CollectionID(c))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(keyList(keys))
}

// @Summary Take the collection out of an API key; owners
// @Description The key keeps its other collections.
// @Tags API keys
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param keyId path string true "Key ID"
// @Security BearerAuth
// @Success 204
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID apikeyCollectionDelete
// @Router /v1/collections/{owner}/{slug}/keys/{keyId} [delete]
func (h *Handler) DeleteKey(c fiber.Ctx) error {
	if err := h.svc.DeleteKey(c.Context(), middleware.CollectionID(c), c.Params("keyId")); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type keyBodyDTO struct {
	Name   string     `json:"name" example:"Assistente"`
	Scopes []ScopeDTO `json:"scopes"`
	// ExpiresInDays (1-365) ends the key that many days after it is made; 0 or absent never ends. Ignored on update.
	ExpiresInDays int `json:"expires_in_days,omitempty" example:"90"`
	// RenewDays, on update only, gives the key a new end that many days from now (0: no end); absent keeps it.
	RenewDays *int `json:"renew_days,omitempty" example:"90"`
} // @name APIKeyInput

func (b keyBodyDTO) scopes() []domain.KeyScope { return KeyScopes(b.Scopes) }

// KeyScopes turns the collections a key may reach into the domain's; OAuth approvals share it.
func KeyScopes(list []ScopeDTO) []domain.KeyScope {
	out := make([]domain.KeyScope, 0, len(list))
	for _, sc := range list {
		out = append(out, domain.KeyScope{CollectionID: sc.CollectionID, CanWrite: sc.CanWrite})
	}
	return out
}

type newKeyDTO struct {
	keyDTO
	// Secret is shown once: send it as "Authorization: Bearer <secret>" or "X-API-Key: <secret>".
	Secret string `json:"secret"`
} // @name NewAPIKey

// @Summary The signed in member's API keys
// @Tags API keys
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{items=[]keyDTO}
// @Failure 401 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID apikeyList
// @Router /v1/me/keys [get]
func (h *Handler) MyKeys(c fiber.Ctx) error {
	keys, err := h.svc.MyKeys(c.Context())
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(keyList(keys))
}

// @Summary Make an API key
// @Description The key acts as its maker on the collections in scopes: read (viewer) or, with can_write, write (editor; needs the editor role).
// @Tags API keys
// @Accept json
// @Produce json
// @Param body body keyBodyDTO true "Key"
// @Security BearerAuth
// @Success 201 {object} newKeyDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID apikeyCreate
// @Router /v1/me/keys [post]
func (h *Handler) CreateMyKey(c fiber.Ctx) error {
	var body keyBodyDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.BadBody(c)
	}
	k, err := h.svc.CreateMyKey(c.Context(), body.Name, body.scopes(), body.ExpiresInDays)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(newKeyDTO{keyDTO: toKeyDTO(k.Key), Secret: k.Secret})
}

// @Summary Rename an API key and set its collections
// @Tags API keys
// @Accept json
// @Produce json
// @Param keyId path string true "Key ID"
// @Param body body keyBodyDTO true "Key"
// @Security BearerAuth
// @Success 200 {object} keyDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID apikeyUpdate
// @Router /v1/me/keys/{keyId} [put]
func (h *Handler) UpdateMyKey(c fiber.Ctx) error {
	var body keyBodyDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.BadBody(c)
	}
	k, err := h.svc.UpdateMyKey(c.Context(), c.Params("keyId"), body.Name, body.scopes(), body.RenewDays)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(toKeyDTO(*k))
}

// @Summary Delete an API key
// @Tags API keys
// @Param keyId path string true "Key ID"
// @Security BearerAuth
// @Success 204
// @Failure 401 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID apikeyDelete
// @Router /v1/me/keys/{keyId} [delete]
func (h *Handler) DeleteMyKey(c fiber.Ctx) error {
	if err := h.svc.DeleteMyKey(c.Context(), c.Params("keyId")); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
