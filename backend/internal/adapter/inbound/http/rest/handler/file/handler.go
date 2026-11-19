// Package file serves the files attached to records: upload into a collection, download by id.
package file

import (
	"mime"
	"net/url"
	"slices"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/port/input"
)

type Handler struct {
	svc input.ToolsService
}

func New(svc input.ToolsService) *Handler {
	return &Handler{svc: svc}
}

// CollectionRoutes mounts the upload on /v1/collections/:owner/:slug.
func (h *Handler) CollectionRoutes(at fiber.Router) {
	at.Post("/files", h.Upload)
}

// Routes mounts the download on /v1/files; API keys reach it too.
func (h *Handler) Routes(r fiber.Router) {
	r.Get("/:id", h.Download)
}

func fail(c fiber.Ctx, err error) error { return mapper.WriteError(c, err) }

type fileDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Size int    `json:"size"`
	Type string `json:"type"`
} // @name File

// @Summary Upload a file to attach to the collection's records; editors
// @Description The body is the file itself (up to 8 MB); name goes in the query. Put the answer in a file field's value.
// @Tags Files
// @Accept application/octet-stream
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param name query string true "File name"
// @Security BearerAuth
// @Success 201 {object} fileDTO
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID fileUpload
// @Router /v1/collections/{owner}/{slug}/files [post]
func (h *Handler) Upload(c fiber.Ctx) error {
	contentType, _, _ := mime.ParseMediaType(c.Get(fiber.HeaderContentType))
	f, err := h.svc.UploadFile(c.Context(), middleware.CollectionID(c), c.Query("name"), contentType, c.Body())
	if err != nil {
		return fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fileDTO{ID: f.ID, Name: f.Name, Size: f.Size, Type: f.ContentType})
}

// Images open in the browser; anything else downloads, so a stored page never runs here.
var inline = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}

// @Summary Download an attached file; viewers of its collection (API keys too)
// @Tags Files
// @Produce octet-stream
// @Param id path string true "File ID"
// @Security BearerAuth
// @Success 200 {file} binary
// @Security ApiKeyAuth
// @Failure 401 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID fileDownload
// @Router /v1/files/{id} [get]
func (h *Handler) Download(c fiber.Ctx) error {
	f, data, err := h.svc.File(c.Context(), c.Params("id"))
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
