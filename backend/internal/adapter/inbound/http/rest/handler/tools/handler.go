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

// CollectionRoutes mounts on /v1/collections before its /:id routes.
func (h *Handler) CollectionRoutes(r fiber.Router) {
	r.Get("/trash", h.TrashedCollections)
	r.Post("/trash/:id/restore", h.RestoreCollection)
	r.Delete("/trash/:id", h.PurgeCollection)
	r.Get("/:id/keys", h.Keys)
	r.Post("/:id/keys", h.CreateKey)
	r.Delete("/:id/keys/:keyId", h.DeleteKey)
	r.Get("/:id/webhooks", h.Webhooks)
	r.Post("/:id/webhooks", h.CreateWebhook)
	r.Delete("/:id/webhooks/:hookId", h.DeleteWebhook)
	r.Post("/:id/webhooks/:hookId/test", h.TestWebhook)
	r.Get("/:id/trash", h.TrashedRecords)
	r.Post("/:id/trash/restore", h.RestoreRecords)
	r.Post("/:id/trash/purge", h.PurgeRecords)
	r.Get("/:id/schema-versions", h.SchemaVersions)
	r.Get("/:id/views", h.Views)
	r.Post("/:id/views", h.CreateView)
	r.Delete("/:id/views/:viewId", h.DeleteView)
	r.Post("/:id/files", h.Upload)
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

type keyDTO struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Prefix      string     `json:"prefix"`
	CanWrite    bool       `json:"can_write"`
	CreatorName string     `json:"creator_name,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

func toKeyDTO(k domain.APIKey) keyDTO {
	return keyDTO{ID: k.ID, Name: k.Name, Prefix: k.Prefix, CanWrite: k.CanWrite, CreatorName: k.CreatorName, CreatedAt: k.CreatedAt, LastUsedAt: k.LastUsedAt}
}

// @Summary A collection's API keys; owners
// @Tags Integrations
// @Produce json
// @Param id path string true "Collection ID"
// @Security BearerAuth
// @Success 200 {object} object{items=[]keyDTO}
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Router /v1/collections/{id}/keys [get]
func (h *Handler) Keys(c fiber.Ctx) error {
	keys, err := h.integrations.Keys(c.Context(), c.Params("id"))
	if err != nil {
		return fail(c, err)
	}
	out := make([]keyDTO, 0, len(keys))
	for _, k := range keys {
		out = append(out, toKeyDTO(k))
	}
	return c.JSON(fiber.Map{"items": out})
}

type createKeyDTO struct {
	Name     string `json:"name" example:"Relatórios"`
	CanWrite bool   `json:"can_write"`
}

type newKeyDTO struct {
	keyDTO
	// Secret is shown once: send it as "Authorization: Bearer <secret>" or "X-API-Key: <secret>".
	Secret string `json:"secret"`
}

// @Summary Make an API key for the collection's records; owners
// @Description The key acts as its maker, as a viewer (read) or an editor (can_write), on this collection's records only.
// @Tags Integrations
// @Accept json
// @Produce json
// @Param id path string true "Collection ID"
// @Param body body createKeyDTO true "Key"
// @Security BearerAuth
// @Success 201 {object} newKeyDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Router /v1/collections/{id}/keys [post]
func (h *Handler) CreateKey(c fiber.Ctx) error {
	var body createKeyDTO
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	k, err := h.integrations.CreateKey(c.Context(), c.Params("id"), body.Name, body.CanWrite)
	if err != nil {
		return fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(newKeyDTO{keyDTO: toKeyDTO(k.Key), Secret: k.Secret})
}

// @Summary Delete an API key; owners
// @Tags Integrations
// @Param id path string true "Collection ID"
// @Param keyId path string true "Key ID"
// @Security BearerAuth
// @Success 204
// @Failure 404 {object} mapper.errorBody
// @Router /v1/collections/{id}/keys/{keyId} [delete]
func (h *Handler) DeleteKey(c fiber.Ctx) error {
	if err := h.integrations.DeleteKey(c.Context(), c.Params("id"), c.Params("keyId")); err != nil {
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
}

func toHookDTO(w domain.Webhook) webhookDTO {
	return webhookDTO{ID: w.ID, URL: w.URL, Events: w.Events, CreatedAt: w.CreatedAt, LastStatus: w.LastStatus, LastError: w.LastError, LastAt: w.LastAt}
}

// @Summary A collection's webhooks; owners
// @Tags Integrations
// @Produce json
// @Param id path string true "Collection ID"
// @Security BearerAuth
// @Success 200 {object} object{items=[]webhookDTO}
// @Router /v1/collections/{id}/webhooks [get]
func (h *Handler) Webhooks(c fiber.Ctx) error {
	hooks, err := h.integrations.Webhooks(c.Context(), c.Params("id"))
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
// @Tags Integrations
// @Accept json
// @Produce json
// @Param id path string true "Collection ID"
// @Param body body createHookDTO true "Webhook"
// @Security BearerAuth
// @Success 201 {object} newHookDTO
// @Failure 400 {object} mapper.errorBody
// @Router /v1/collections/{id}/webhooks [post]
func (h *Handler) CreateWebhook(c fiber.Ctx) error {
	var body createHookDTO
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	w, err := h.integrations.CreateWebhook(c.Context(), c.Params("id"), body.URL, body.Events)
	if err != nil {
		return fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(newHookDTO{webhookDTO: toHookDTO(w.Webhook), Secret: w.Secret})
}

// @Summary Delete a webhook; owners
// @Tags Integrations
// @Param id path string true "Collection ID"
// @Param hookId path string true "Webhook ID"
// @Security BearerAuth
// @Success 204
// @Router /v1/collections/{id}/webhooks/{hookId} [delete]
func (h *Handler) DeleteWebhook(c fiber.Ctx) error {
	if err := h.integrations.DeleteWebhook(c.Context(), c.Params("id"), c.Params("hookId")); err != nil {
		return fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary Send a ping to a webhook now; owners
// @Tags Integrations
// @Produce json
// @Param id path string true "Collection ID"
// @Param hookId path string true "Webhook ID"
// @Security BearerAuth
// @Success 200 {object} webhookDTO "with the answer"
// @Router /v1/collections/{id}/webhooks/{hookId}/test [post]
func (h *Handler) TestWebhook(c fiber.Ctx) error {
	w, err := h.integrations.TestWebhook(c.Context(), c.Params("id"), c.Params("hookId"))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(toHookDTO(*w))
}

type idsDTO struct {
	IDs []string `json:"ids"`
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
// @Param id path string true "Collection ID"
// @Security BearerAuth
// @Success 200 {object} object{items=[]trashedRecordDTO}
// @Router /v1/collections/{id}/trash [get]
func (h *Handler) TrashedRecords(c fiber.Ctx) error {
	items, err := h.tools.TrashedRecords(c.Context(), c.Params("id"))
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
// @Param id path string true "Collection ID"
// @Param body body idsDTO true "Record ids (1 to 100)"
// @Security BearerAuth
// @Success 200 {object} countDTO
// @Router /v1/collections/{id}/trash/restore [post]
func (h *Handler) RestoreRecords(c fiber.Ctx) error {
	var body idsDTO
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	n, err := h.tools.RestoreRecords(c.Context(), c.Params("id"), body.IDs)
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
// @Param id path string true "Collection ID"
// @Param body body idsDTO false "Record ids (up to 100)"
// @Security BearerAuth
// @Success 200 {object} countDTO
// @Router /v1/collections/{id}/trash/purge [post]
func (h *Handler) PurgeRecords(c fiber.Ctx) error {
	var body idsDTO
	if len(c.Body()) > 0 {
		if err := c.Bind().Body(&body); err != nil {
			return badBody(c)
		}
	}
	n, err := h.tools.PurgeRecords(c.Context(), c.Params("id"), body.IDs)
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
// @Param id path string true "Collection ID"
// @Security BearerAuth
// @Success 200 {object} object{items=[]schemaVersionDTO}
// @Router /v1/collections/{id}/schema-versions [get]
func (h *Handler) SchemaVersions(c fiber.Ctx) error {
	items, err := h.tools.SchemaVersions(c.Context(), c.Params("id"))
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

// @Summary The caller's saved views of the record list and the shared ones; viewers
// @Tags Collections
// @Produce json
// @Param id path string true "Collection ID"
// @Security BearerAuth
// @Success 200 {object} object{items=[]viewDTO}
// @Router /v1/collections/{id}/views [get]
func (h *Handler) Views(c fiber.Ctx) error {
	items, err := h.tools.Views(c.Context(), c.Params("id"))
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
// @Param id path string true "Collection ID"
// @Param body body createViewDTO true "View"
// @Security BearerAuth
// @Success 201 {object} viewDTO
// @Router /v1/collections/{id}/views [post]
func (h *Handler) CreateView(c fiber.Ctx) error {
	var body createViewDTO
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	v, err := h.tools.CreateView(c.Context(), c.Params("id"), body.Name, body.Query, body.Shared)
	if err != nil {
		return fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(toViewDTO(*v))
}

// @Summary Delete one of the caller's saved views
// @Tags Collections
// @Param id path string true "Collection ID"
// @Param viewId path string true "View ID"
// @Security BearerAuth
// @Success 204
// @Router /v1/collections/{id}/views/{viewId} [delete]
func (h *Handler) DeleteView(c fiber.Ctx) error {
	if err := h.tools.DeleteView(c.Context(), c.Params("id"), c.Params("viewId")); err != nil {
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
// @Description The body is the file itself (up to 1 MB); name goes in the query. Put the answer in a file field's value.
// @Tags Collections
// @Accept application/octet-stream
// @Produce json
// @Param id path string true "Collection ID"
// @Param name query string true "File name"
// @Security BearerAuth
// @Success 201 {object} fileDTO
// @Router /v1/collections/{id}/files [post]
func (h *Handler) Upload(c fiber.Ctx) error {
	contentType, _, _ := mime.ParseMediaType(c.Get(fiber.HeaderContentType))
	f, err := h.tools.UploadFile(c.Context(), c.Params("id"), c.Query("name"), contentType, c.Body())
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
