// Package trash serves deleted records and collections: list, restore, delete for good.
package trash

import (
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
)

type Handler struct {
	svc input.ToolsService
}

func New(svc input.ToolsService) *Handler {
	return &Handler{svc: svc}
}

// Routes mounts the deleted collections on /v1/collections, by id: a deleted collection
// gave its address up. Mount it before the :owner/:slug group, which would take "trash"
// for a username.
func (h *Handler) Routes(r fiber.Router) {
	r.Get("/trash", h.TrashedCollections)
	r.Post("/trash/:id/restore", h.RestoreCollection)
	r.Delete("/trash/:id", h.PurgeCollection)
}

// CollectionRoutes mounts a collection's deleted records on /v1/collections/:owner/:slug.
func (h *Handler) CollectionRoutes(at fiber.Router) {
	at.Get("/trash", h.TrashedRecords)
	at.Post("/trash/restore", h.RestoreRecords)
	at.Post("/trash/purge", h.PurgeRecords)
}

func fail(c fiber.Ctx, err error) error { return mapper.WriteError(c, err) }

func badBody(c fiber.Ctx) error {
	return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
}

type idsDTO struct {
	IDs []string `json:"ids"`
} // @name TrashIDs

type restoreDTO struct {
	IDs    []string                   `json:"ids"`
	Values map[string]json.RawMessage `json:"values,omitempty" swaggertype:"object"`
} // @name TrashRestoreInput

type trashedRecordDTO struct {
	ID            string          `json:"id"`
	Data          json.RawMessage `json:"data" swaggertype:"object"`
	DeletedAt     time.Time       `json:"deleted_at"`
	DeletedByName string          `json:"deleted_by_name,omitempty"`
} // @name TrashedRecord

// @Summary The collection's deleted records, newest first (up to 200); editors
// @Tags Trash
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Security BearerAuth
// @Success 200 {object} object{items=[]trashedRecordDTO}
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID trashListRecords
// @Router /v1/collections/{owner}/{slug}/trash [get]
func (h *Handler) TrashedRecords(c fiber.Ctx) error {
	items, err := h.svc.TrashedRecords(c.Context(), middleware.CollectionID(c))
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
} // @name TrashCount

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
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID trashRestoreRecords
// @Router /v1/collections/{owner}/{slug}/trash/restore [post]
func (h *Handler) RestoreRecords(c fiber.Ctx) error {
	var body restoreDTO
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	n, err := h.svc.RestoreRecords(c.Context(), middleware.CollectionID(c), body.IDs, body.Values)
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
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID trashPurgeRecords
// @Router /v1/collections/{owner}/{slug}/trash/purge [post]
func (h *Handler) PurgeRecords(c fiber.Ctx) error {
	var body idsDTO
	if len(c.Body()) > 0 {
		if err := c.Bind().Body(&body); err != nil {
			return badBody(c)
		}
	}
	n, err := h.svc.PurgeRecords(c.Context(), middleware.CollectionID(c), body.IDs)
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
} // @name TrashedCollection

// @Summary Deleted collections the caller owns
// @Tags Trash
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{items=[]trashedCollectionDTO}
// @Failure 401 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID trashListCollections
// @Router /v1/collections/trash [get]
func (h *Handler) TrashedCollections(c fiber.Ctx) error {
	items, err := h.svc.TrashedCollections(c.Context())
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
// @Failure 401 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID trashRestoreCollection
// @Router /v1/collections/trash/{id}/restore [post]
func (h *Handler) RestoreCollection(c fiber.Ctx) error {
	if err := h.svc.RestoreCollection(c.Context(), c.Params("id")); err != nil {
		return fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary Delete a deleted collection for good, records included; owners
// @Tags Trash
// @Param id path string true "Collection ID"
// @Security BearerAuth
// @Success 204
// @Failure 401 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID trashPurgeCollection
// @Router /v1/collections/trash/{id} [delete]
func (h *Handler) PurgeCollection(c fiber.Ctx) error {
	if err := h.svc.PurgeCollection(c.Context(), c.Params("id")); err != nil {
		return fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
