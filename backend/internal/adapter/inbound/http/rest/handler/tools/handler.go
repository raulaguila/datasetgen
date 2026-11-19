// Package tools serves a collection's API keys, webhooks, trash, field
// history, saved views and files.
package tools

import (
	"encoding/json"
	"mime"
	"net/url"
	"slices"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
)

type Handler struct {
	integrations input.IntegrationService
	tools        input.ToolsService
	records      input.RecordService
}

func New(integrations input.IntegrationService, tools input.ToolsService, records input.RecordService) *Handler {
	return &Handler{integrations: integrations, tools: tools, records: records}
}

// TrashRoutes mounts the deleted collections on /v1/collections, by id: a deleted
// collection gave its address up.
func (h *Handler) TrashRoutes(r fiber.Router) {
	r.Get("/trash", h.TrashedCollections)
	r.Post("/trash/:id/restore", h.RestoreCollection)
	r.Delete("/trash/:id", h.PurgeCollection)
}

// CollectionRoutes mounts a collection's tools on /v1/collections/:owner/:slug.
func (h *Handler) CollectionRoutes(at fiber.Router) {
	at.Get("/keys", h.Keys)
	at.Delete("/keys/:keyId", h.DeleteKey)
	at.Get("/shares", h.Shares)
	at.Post("/shares", h.CreateShare)
	at.Delete("/shares/:shareId", h.DeleteShare)
	at.Get("/webhooks", h.Webhooks)
	at.Post("/webhooks", h.CreateWebhook)
	at.Patch("/webhooks/:hookId", h.UpdateWebhook)
	at.Delete("/webhooks/:hookId", h.DeleteWebhook)
	at.Post("/webhooks/:hookId/test", h.TestWebhook)
	at.Get("/webhooks/:hookId/deliveries", h.Deliveries)
	at.Post("/webhooks/:hookId/deliveries/:deliveryId/redeliver", h.Redeliver)
	at.Get("/automations", h.Automations)
	at.Post("/automations", h.CreateAutomation)
	at.Put("/automations/:autoId", h.UpdateAutomation)
	at.Delete("/automations/:autoId", h.DeleteAutomation)
	at.Post("/automations/:autoId/run", h.RunAutomation)
	at.Get("/mcp-tools", h.MCPTools)
	at.Post("/mcp-tools", h.CreateMCPTool)
	at.Put("/mcp-tools/:toolId", h.UpdateMCPTool)
	at.Delete("/mcp-tools/:toolId", h.DeleteMCPTool)
	at.Post("/mcp-tools/test", h.TestMCPTool)
	at.Get("/mcp-calls", h.MCPCalls)
	at.Get("/trash", h.TrashedRecords)
	at.Post("/trash/restore", h.RestoreRecords)
	at.Post("/trash/purge", h.PurgeRecords)
	at.Get("/schema-versions", h.SchemaVersions)
	at.Get("/dashboard", h.Dashboard)
	at.Put("/dashboard", h.SaveDashboard)
	at.Get("/views", h.Views)
	at.Post("/views", h.CreateView)
	at.Delete("/views/:viewId", h.DeleteView)
	at.Post("/files", h.Upload)
}

// MeRoutes mounts the caller's own API keys on /v1/me/keys.
func (h *Handler) MeRoutes(r fiber.Router) {
	r.Get("/", h.MyKeys)
	r.Post("/", h.CreateMyKey)
	r.Put("/:keyId", h.UpdateMyKey)
	r.Delete("/:keyId", h.DeleteMyKey)
	r.Get("/:keyId/calls", h.KeyMCPCalls)
}

// RecordRoutes mounts on /v1/records before its /:id routes.
func (h *Handler) RecordRoutes(r fiber.Router) {
	r.Post("/bulk-update", h.BulkUpdate)
}

// FileRoutes mounts on /v1/files.
func (h *Handler) FileRoutes(r fiber.Router) {
	r.Get("/:id", h.Download)
}

func fail(c fiber.Ctx, err error) error { return mapper.WriteError(c, err) }

func badBody(c fiber.Ctx) error {
	return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
}

type scopeDTO struct {
	CollectionID   string `json:"collection_id"`
	CollectionName string `json:"collection_name,omitempty"`
	CanWrite       bool   `json:"can_write"`
}

type keyDTO struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Prefix      string     `json:"prefix"`
	CanWrite    bool       `json:"can_write"`
	Scopes      []scopeDTO `json:"scopes,omitempty"`
	CreatorName string     `json:"creator_name,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

func toKeyDTO(k domain.APIKey) keyDTO {
	var scopes []scopeDTO
	for _, sc := range k.Scopes {
		scopes = append(scopes, scopeDTO(sc))
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
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Router /v1/collections/{owner}/{slug}/keys [get]
func (h *Handler) Keys(c fiber.Ctx) error {
	keys, err := h.integrations.Keys(c.Context(), middleware.CollectionID(c))
	if err != nil {
		return fail(c, err)
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
// @Failure 404 {object} mapper.errorBody
// @Router /v1/collections/{owner}/{slug}/keys/{keyId} [delete]
func (h *Handler) DeleteKey(c fiber.Ctx) error {
	if err := h.integrations.DeleteKey(c.Context(), middleware.CollectionID(c), c.Params("keyId")); err != nil {
		return fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type keyBodyDTO struct {
	Name   string     `json:"name" example:"Assistente"`
	Scopes []scopeDTO `json:"scopes"`
	// ExpiresInDays (1-365) ends the key that many days after it is made; 0 or absent never ends. Ignored on update.
	ExpiresInDays int `json:"expires_in_days,omitempty" example:"90"`
	// RenewDays, on update only, gives the key a new end that many days from now (0: no end); absent keeps it.
	RenewDays *int `json:"renew_days,omitempty" example:"90"`
}

func (b keyBodyDTO) scopes() []domain.KeyScope {
	out := make([]domain.KeyScope, 0, len(b.Scopes))
	for _, sc := range b.Scopes {
		out = append(out, domain.KeyScope{CollectionID: sc.CollectionID, CanWrite: sc.CanWrite})
	}
	return out
}

type newKeyDTO struct {
	keyDTO
	// Secret is shown once: send it as "Authorization: Bearer <secret>" or "X-API-Key: <secret>".
	Secret string `json:"secret"`
}

// @Summary The signed in member's API keys
// @Tags API keys
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{items=[]keyDTO}
// @Router /v1/me/keys [get]
func (h *Handler) MyKeys(c fiber.Ctx) error {
	keys, err := h.integrations.MyKeys(c.Context())
	if err != nil {
		return fail(c, err)
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
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Router /v1/me/keys [post]
func (h *Handler) CreateMyKey(c fiber.Ctx) error {
	var body keyBodyDTO
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	k, err := h.integrations.CreateMyKey(c.Context(), body.Name, body.scopes(), body.ExpiresInDays)
	if err != nil {
		return fail(c, err)
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
// @Failure 404 {object} mapper.errorBody
// @Router /v1/me/keys/{keyId} [put]
func (h *Handler) UpdateMyKey(c fiber.Ctx) error {
	var body keyBodyDTO
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	k, err := h.integrations.UpdateMyKey(c.Context(), c.Params("keyId"), body.Name, body.scopes(), body.RenewDays)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(toKeyDTO(*k))
}

// @Summary Delete an API key
// @Tags API keys
// @Param keyId path string true "Key ID"
// @Security BearerAuth
// @Success 204
// @Failure 404 {object} mapper.errorBody
// @Router /v1/me/keys/{keyId} [delete]
func (h *Handler) DeleteMyKey(c fiber.Ctx) error {
	if err := h.integrations.DeleteMyKey(c.Context(), c.Params("keyId")); err != nil {
		return fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type shareDTO struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind" example:"form"`
	Token      string     `json:"token"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

func toShareDTO(s domain.Share) shareDTO {
	return shareDTO{ID: s.ID, Kind: s.Kind, Token: s.Token, CreatedAt: s.CreatedAt, LastUsedAt: s.LastUsedAt, ExpiresAt: s.ExpiresAt}
}

// @Summary A collection's share links; owners
// @Tags Share links
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Security BearerAuth
// @Success 200 {object} object{items=[]shareDTO}
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Router /v1/collections/{owner}/{slug}/shares [get]
func (h *Handler) Shares(c fiber.Ctx) error {
	shares, err := h.integrations.Shares(c.Context(), middleware.CollectionID(c))
	if err != nil {
		return fail(c, err)
	}
	out := make([]shareDTO, 0, len(shares))
	for _, s := range shares {
		out = append(out, toShareDTO(s))
	}
	return c.JSON(fiber.Map{"items": out})
}

// @Summary Make a share link: a public form (kind form) or a read-only list (kind view); owners
// @Tags Share links
// @Accept json
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param body body object{kind=string,expires_in_days=int} true "Link; expires_in_days 1 to 365, or 0 for no end"
// @Security BearerAuth
// @Success 201 {object} shareDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Router /v1/collections/{owner}/{slug}/shares [post]
func (h *Handler) CreateShare(c fiber.Ctx) error {
	var body struct {
		Kind          string `json:"kind"`
		ExpiresInDays int    `json:"expires_in_days"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	s, err := h.integrations.CreateShare(c.Context(), middleware.CollectionID(c), body.Kind, body.ExpiresInDays)
	if err != nil {
		return fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(toShareDTO(*s))
}

// @Summary Delete a share link; owners
// @Tags Share links
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param shareId path string true "Link ID"
// @Security BearerAuth
// @Success 204
// @Failure 404 {object} mapper.errorBody
// @Router /v1/collections/{owner}/{slug}/shares/{shareId} [delete]
func (h *Handler) DeleteShare(c fiber.Ctx) error {
	if err := h.integrations.DeleteShare(c.Context(), middleware.CollectionID(c), c.Params("shareId")); err != nil {
		return fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type webhookDTO struct {
	ID         string     `json:"id"`
	URL        string     `json:"url"`
	Events     []string   `json:"events"`
	CreatedAt  time.Time  `json:"created_at"`
	LastStatus *int       `json:"last_status,omitempty"`
	LastError  string     `json:"last_error,omitempty"`
	LastAt     *time.Time `json:"last_at,omitempty"`
	// Failures counts failed calls in a row; at 10 the hook pauses (paused_at) until resumed.
	Failures int        `json:"failures"`
	PausedAt *time.Time `json:"paused_at,omitempty"`
}

func toHookDTO(w domain.Webhook) webhookDTO {
	return webhookDTO{ID: w.ID, URL: w.URL, Events: w.Events, CreatedAt: w.CreatedAt, LastStatus: w.LastStatus, LastError: w.LastError, LastAt: w.LastAt, Failures: w.Failures, PausedAt: w.PausedAt}
}

// @Summary A collection's webhooks; owners
// @Tags Webhooks
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Security BearerAuth
// @Success 200 {object} object{items=[]webhookDTO}
// @Router /v1/collections/{owner}/{slug}/webhooks [get]
func (h *Handler) Webhooks(c fiber.Ctx) error {
	hooks, err := h.integrations.Webhooks(c.Context(), middleware.CollectionID(c))
	if err != nil {
		return fail(c, err)
	}
	out := make([]webhookDTO, 0, len(hooks))
	for _, w := range hooks {
		out = append(out, toHookDTO(w))
	}
	return c.JSON(fiber.Map{"items": out})
}

type createHookDTO struct {
	URL    string   `json:"url" example:"https://example.com/hooks/dataset"`
	Events []string `json:"events" example:"record.created"`
}

type newHookDTO struct {
	webhookDTO
	// Secret signs every call (X-Dataset-Signature: t=<unix>,v1=<HMAC-SHA256 of "<t>.<body>">); shown once.
	Secret string `json:"secret"`
}

// @Summary Add a webhook; owners
// @Description Events: record.created, record.updated, record.deleted, records.imported. Calls are POSTs with a JSON body, retried with backoff on failure.
// @Tags Webhooks
// @Accept json
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param body body createHookDTO true "Webhook"
// @Security BearerAuth
// @Success 201 {object} newHookDTO
// @Failure 400 {object} mapper.errorBody
// @Router /v1/collections/{owner}/{slug}/webhooks [post]
func (h *Handler) CreateWebhook(c fiber.Ctx) error {
	var body createHookDTO
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	w, err := h.integrations.CreateWebhook(c.Context(), middleware.CollectionID(c), body.URL, body.Events)
	if err != nil {
		return fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(newHookDTO{webhookDTO: toHookDTO(w.Webhook), Secret: w.Secret})
}

type updateHookDTO struct {
	URL    string   `json:"url" example:"https://example.com/hooks/dataset"`
	Events []string `json:"events" example:"record.created"`
	// Enabled false pauses the hook; true resumes it and clears its failures.
	Enabled bool `json:"enabled" example:"true"`
}

// @Summary Change, pause or resume a webhook; owners
// @Description Keeps the signing secret. A hook pauses by itself after 10 failed calls in a row; send enabled true to resume it.
// @Tags Webhooks
// @Accept json
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param hookId path string true "Webhook ID"
// @Param body body updateHookDTO true "Webhook"
// @Security BearerAuth
// @Success 200 {object} webhookDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Router /v1/collections/{owner}/{slug}/webhooks/{hookId} [patch]
func (h *Handler) UpdateWebhook(c fiber.Ctx) error {
	var body updateHookDTO
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	w, err := h.integrations.UpdateWebhook(c.Context(), middleware.CollectionID(c), c.Params("hookId"), body.URL, body.Events, body.Enabled)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(toHookDTO(*w))
}

// @Summary Delete a webhook; owners
// @Tags Webhooks
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param hookId path string true "Webhook ID"
// @Security BearerAuth
// @Success 204
// @Router /v1/collections/{owner}/{slug}/webhooks/{hookId} [delete]
func (h *Handler) DeleteWebhook(c fiber.Ctx) error {
	if err := h.integrations.DeleteWebhook(c.Context(), middleware.CollectionID(c), c.Params("hookId")); err != nil {
		return fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary Send a ping to a webhook now; owners
// @Tags Webhooks
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param hookId path string true "Webhook ID"
// @Security BearerAuth
// @Success 200 {object} webhookDTO "with the answer"
// @Router /v1/collections/{owner}/{slug}/webhooks/{hookId}/test [post]
func (h *Handler) TestWebhook(c fiber.Ctx) error {
	w, err := h.integrations.TestWebhook(c.Context(), middleware.CollectionID(c), c.Params("hookId"))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(toHookDTO(*w))
}

type idsDTO struct {
	IDs []string `json:"ids"`
}

type restoreDTO struct {
	IDs    []string                   `json:"ids"`
	Values map[string]json.RawMessage `json:"values,omitempty"`
}

type trashedRecordDTO struct {
	ID            string          `json:"id"`
	Data          json.RawMessage `json:"data" swaggertype:"object"`
	DeletedAt     time.Time       `json:"deleted_at"`
	DeletedByName string          `json:"deleted_by_name,omitempty"`
}

// @Summary The collection's deleted records, newest first (up to 200); editors
// @Tags Trash
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Security BearerAuth
// @Success 200 {object} object{items=[]trashedRecordDTO}
// @Router /v1/collections/{owner}/{slug}/trash [get]
func (h *Handler) TrashedRecords(c fiber.Ctx) error {
	items, err := h.tools.TrashedRecords(c.Context(), middleware.CollectionID(c))
	if err != nil {
		return fail(c, err)
	}
	out := make([]trashedRecordDTO, 0, len(items))
	for _, r := range items {
		out = append(out, trashedRecordDTO{ID: r.ID, Data: r.Data, DeletedAt: r.DeletedAt, DeletedByName: r.DeletedByName})
	}
	return c.JSON(fiber.Map{"items": out, "retention_days": int(domain.TrashRetention.Hours() / 24)})
}

type countDTO struct {
	Count int64 `json:"count"`
}

// @Summary Restore deleted records; editors
// @Description All or none; refused (409) when a unique value they hold was taken since.
// @Tags Trash
// @Accept json
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param body body restoreDTO true "Record ids (1 to 100); values, for one record, give its unique fields new values"
// @Security BearerAuth
// @Success 200 {object} countDTO
// @Router /v1/collections/{owner}/{slug}/trash/restore [post]
func (h *Handler) RestoreRecords(c fiber.Ctx) error {
	var body restoreDTO
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	n, err := h.tools.RestoreRecords(c.Context(), middleware.CollectionID(c), body.IDs, body.Values)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(countDTO{Count: int64(n)})
}

// @Summary Delete trashed records for good; owners
// @Description Without ids, empties the collection's whole trash.
// @Tags Trash
// @Accept json
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param body body idsDTO false "Record ids (up to 100)"
// @Security BearerAuth
// @Success 200 {object} countDTO
// @Router /v1/collections/{owner}/{slug}/trash/purge [post]
func (h *Handler) PurgeRecords(c fiber.Ctx) error {
	var body idsDTO
	if len(c.Body()) > 0 {
		if err := c.Bind().Body(&body); err != nil {
			return badBody(c)
		}
	}
	n, err := h.tools.PurgeRecords(c.Context(), middleware.CollectionID(c), body.IDs)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(countDTO{Count: n})
}

type trashedCollectionDTO struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Icon        string    `json:"icon"`
	RecordCount int64     `json:"record_count"`
	DeletedAt   time.Time `json:"deleted_at"`
}

// @Summary Deleted collections the caller owns
// @Tags Trash
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{items=[]trashedCollectionDTO}
// @Router /v1/collections/trash [get]
func (h *Handler) TrashedCollections(c fiber.Ctx) error {
	items, err := h.tools.TrashedCollections(c.Context())
	if err != nil {
		return fail(c, err)
	}
	out := make([]trashedCollectionDTO, 0, len(items))
	for _, t := range items {
		out = append(out, trashedCollectionDTO(t))
	}
	return c.JSON(fiber.Map{"items": out, "retention_days": int(domain.TrashRetention.Hours() / 24)})
}

// @Summary Restore a deleted collection; owners
// @Tags Trash
// @Param id path string true "Collection ID"
// @Security BearerAuth
// @Success 204
// @Router /v1/collections/trash/{id}/restore [post]
func (h *Handler) RestoreCollection(c fiber.Ctx) error {
	if err := h.tools.RestoreCollection(c.Context(), c.Params("id")); err != nil {
		return fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary Delete a deleted collection for good, records included; owners
// @Tags Trash
// @Param id path string true "Collection ID"
// @Security BearerAuth
// @Success 204
// @Router /v1/collections/trash/{id} [delete]
func (h *Handler) PurgeCollection(c fiber.Ctx) error {
	if err := h.tools.PurgeCollection(c.Context(), c.Params("id")); err != nil {
		return fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type schemaVersionDTO struct {
	Version     int             `json:"version"`
	Schema      json.RawMessage `json:"schema" swaggertype:"object"`
	SavedAt     time.Time       `json:"saved_at"`
	SavedByName string          `json:"saved_by_name,omitempty"`
}

// @Summary Earlier fields of the collection, newest first (up to 50); owners
// @Tags Collections
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Security BearerAuth
// @Success 200 {object} object{items=[]schemaVersionDTO}
// @Router /v1/collections/{owner}/{slug}/schema-versions [get]
func (h *Handler) SchemaVersions(c fiber.Ctx) error {
	items, err := h.tools.SchemaVersions(c.Context(), middleware.CollectionID(c))
	if err != nil {
		return fail(c, err)
	}
	out := make([]schemaVersionDTO, 0, len(items))
	for _, v := range items {
		out = append(out, schemaVersionDTO(v))
	}
	return c.JSON(fiber.Map{"items": out})
}

type viewDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Query     string    `json:"query"`
	Shared    bool      `json:"shared"`
	Mine      bool      `json:"mine"`
	CreatedAt time.Time `json:"created_at"`
}

func toViewDTO(v domain.SavedView) viewDTO {
	return viewDTO{ID: v.ID, Name: v.Name, Query: v.Query, Shared: v.Shared, Mine: v.Mine, CreatedAt: v.CreatedAt}
}

// @Summary The collection's dashboard widgets; members
// @Tags Collections
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Security BearerAuth
// @Success 200 {object} object{widgets=[]object}
// @Router /v1/collections/{owner}/{slug}/dashboard [get]
func (h *Handler) Dashboard(c fiber.Ctx) error {
	widgets, err := h.tools.Dashboard(c.Context(), middleware.CollectionID(c))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(fiber.Map{"widgets": widgets})
}

type dashboardDTO struct {
	Widgets json.RawMessage `json:"widgets" swaggertype:"array,object"`
}

// @Summary Replace the collection's dashboard widgets (kinds count, sum, avg, min, max, bars, list); owners
// @Tags Collections
// @Accept json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param body body dashboardDTO true "Widgets"
// @Security BearerAuth
// @Success 204
// @Router /v1/collections/{owner}/{slug}/dashboard [put]
func (h *Handler) SaveDashboard(c fiber.Ctx) error {
	var body dashboardDTO
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	if err := h.tools.SaveDashboard(c.Context(), middleware.CollectionID(c), body.Widgets); err != nil {
		return fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary The caller's saved views of the record list and the shared ones; viewers
// @Tags Collections
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Security BearerAuth
// @Success 200 {object} object{items=[]viewDTO}
// @Router /v1/collections/{owner}/{slug}/views [get]
func (h *Handler) Views(c fiber.Ctx) error {
	items, err := h.tools.Views(c.Context(), middleware.CollectionID(c))
	if err != nil {
		return fail(c, err)
	}
	out := make([]viewDTO, 0, len(items))
	for _, v := range items {
		out = append(out, toViewDTO(v))
	}
	return c.JSON(fiber.Map{"items": out})
}

type createViewDTO struct {
	Name   string `json:"name"`
	Query  string `json:"query" example:"sort=data.nome&order=asc"`
	Shared bool   `json:"shared"`
}

// @Summary Save a view of the record list; shared views need an editor
// @Tags Collections
// @Accept json
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param body body createViewDTO true "View"
// @Security BearerAuth
// @Success 201 {object} viewDTO
// @Router /v1/collections/{owner}/{slug}/views [post]
func (h *Handler) CreateView(c fiber.Ctx) error {
	var body createViewDTO
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	v, err := h.tools.CreateView(c.Context(), middleware.CollectionID(c), body.Name, body.Query, body.Shared)
	if err != nil {
		return fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(toViewDTO(*v))
}

// @Summary Delete one of the caller's saved views
// @Tags Collections
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param viewId path string true "View ID"
// @Security BearerAuth
// @Success 204
// @Router /v1/collections/{owner}/{slug}/views/{viewId} [delete]
func (h *Handler) DeleteView(c fiber.Ctx) error {
	if err := h.tools.DeleteView(c.Context(), middleware.CollectionID(c), c.Params("viewId")); err != nil {
		return fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type fileDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Size int    `json:"size"`
	Type string `json:"type"`
}

// @Summary Upload a file to attach to the collection's records; editors
// @Description The body is the file itself (up to 8 MB); name goes in the query. Put the answer in a file field's value.
// @Tags Collections
// @Accept application/octet-stream
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param name query string true "File name"
// @Security BearerAuth
// @Success 201 {object} fileDTO
// @Router /v1/collections/{owner}/{slug}/files [post]
func (h *Handler) Upload(c fiber.Ctx) error {
	contentType, _, _ := mime.ParseMediaType(c.Get(fiber.HeaderContentType))
	f, err := h.tools.UploadFile(c.Context(), middleware.CollectionID(c), c.Query("name"), contentType, c.Body())
	if err != nil {
		return fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fileDTO{ID: f.ID, Name: f.Name, Size: f.Size, Type: f.ContentType})
}

// Images open in the browser; anything else downloads, so a stored page never runs here.
var inline = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}

// @Summary Download an attached file; viewers of its collection (API keys too)
// @Tags Collections
// @Produce octet-stream
// @Param id path string true "File ID"
// @Security BearerAuth
// @Success 200 {file} binary
// @Failure 404 {object} mapper.errorBody
// @Router /v1/files/{id} [get]
func (h *Handler) Download(c fiber.Ctx) error {
	f, data, err := h.tools.File(c.Context(), c.Params("id"))
	if err != nil {
		return fail(c, err)
	}
	disposition := "attachment"
	contentType := f.ContentType
	if slices.Contains(inline, contentType) {
		disposition = "inline"
	} else {
		contentType = "application/octet-stream"
	}
	c.Set(fiber.HeaderContentType, contentType)
	c.Set(fiber.HeaderContentDisposition, disposition+"; filename*=UTF-8''"+url.PathEscape(f.Name))
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Content-Security-Policy", "sandbox")
	c.Set(fiber.HeaderCacheControl, "private, max-age=3600")
	return c.Send(data)
}

type bulkDTO struct {
	IDs []string                   `json:"ids"`
	Set map[string]json.RawMessage `json:"set" swaggertype:"object"`
}

// @Summary Set the same values in several records of one collection; editors
// @Description null removes a field. Every record must still pass the collection's fields, or none changes.
// @Tags Records
// @Accept json
// @Produce json
// @Param body body bulkDTO true "Ids (1 to 100) and values"
// @Security BearerAuth
// @Success 200 {object} countDTO
// @Failure 422 {object} mapper.errorBody
// @Router /v1/records/bulk-update [post]
func (h *Handler) BulkUpdate(c fiber.Ctx) error {
	var body bulkDTO
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	n, err := h.records.BulkUpdate(c.Context(), input.BulkUpdateCommand{IDs: body.IDs, Set: body.Set})
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(countDTO{Count: int64(n)})
}
