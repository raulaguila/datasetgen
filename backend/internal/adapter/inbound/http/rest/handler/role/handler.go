package role

import (
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
)

type Handler struct {
	svc input.RoleService
}

func New(svc input.RoleService) *Handler {
	return &Handler{svc: svc}
}

// Routes expects a router already behind Bearer.
func (h *Handler) Routes(r fiber.Router, authz input.Authorizer) {
	read := middleware.Require(authz, domain.ResourceRoles, domain.ActionRead)
	update := middleware.Require(authz, domain.ResourceRoles, domain.ActionUpdate)
	remove := middleware.Require(authz, domain.ResourceRoles, domain.ActionDelete)
	r.Get("/", read, h.List)
	r.Get("/:id", read, h.Get)
	r.Post("/", middleware.Require(authz, domain.ResourceRoles, domain.ActionCreate), h.Create)
	r.Post("/delete", remove, h.Delete)
	r.Patch("/:id", update, h.Update)
	r.Put("/:id/permissions", update, h.SetPermissions)
	r.Get("/:id/members", read, h.Members)
	r.Put("/:id/members/:user_id", update, h.AddMember)
	r.Delete("/:id/members/:user_id", update, h.RemoveMember)
}

// UserRoutes mounts the role routes that live under /users, behind Bearer.
func (h *Handler) UserRoutes(r fiber.Router, authz input.Authorizer) {
	r.Put("/:id/roles", middleware.Require(authz, domain.ResourceRoles, domain.ActionUpdate), h.SetUserRoles)
}

type roleDTO struct {
	ID          string              `json:"id"`
	Name        string              `json:"name" example:"support"`
	Enabled     bool                `json:"enabled"`
	IsDefault   bool                `json:"is_default"`
	Permissions map[string][]string `json:"permissions"`
	CreatedAt   time.Time           `json:"created_at"`
	MemberCount int64               `json:"member_count"`
}

type createDTO struct {
	Name string `json:"name" example:"support"`
}

type updateDTO struct {
	Name    *string `json:"name" example:"support"`
	Enabled *bool   `json:"enabled"`
	// Marks a role every new account gets; several may be default.
	IsDefault *bool `json:"is_default"`
}

type idsDTO struct {
	IDs []string `json:"ids"`
}

type permissionsDTO struct {
	Permissions map[string][]string `json:"permissions"`
}

type userRolesDTO struct {
	RoleIDs []string `json:"role_ids"`
}

type memberDTO struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Status string `json:"status" example:"active"`
}

func toDTO(r domain.Role) roleDTO {
	return roleDTO{
		ID: r.ID, Name: r.Name, Enabled: r.Enabled, IsDefault: r.IsDefault,
		Permissions: mapper.Permissions(r.Permissions), CreatedAt: r.CreatedAt, MemberCount: r.MemberCount,
	}
}

// @Summary List the roles the API can change, with their permissions
// @Tags Roles
// @Produce json
// @Param page query int false "Page" default(1)
// @Param limit query int false "Items per page, up to 100" default(20)
// @Param search query string false "Part of the name"
// @Param sort query string false "Sort field" Enums(name, created_at) default(name)
// @Param order query string false "Sort direction" Enums(asc, desc) default(asc)
// @Security BearerAuth
// @Success 200 {object} object{items=[]roleDTO,pagination=mapper.PaginationDTO}
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/roles [get]
func (h *Handler) List(c fiber.Ctx) error {
	filter := domain.RoleFilter{
		Page:   domain.NewPage(fiber.Query[int](c, "page"), fiber.Query[int](c, "limit")),
		Search: c.Query("search"),
		Sort:   c.Query("sort"),
		Order:  c.Query("order"),
	}
	roles, total, err := h.svc.List(c.Context(), filter)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]roleDTO, 0, len(roles))
	for _, r := range roles {
		out = append(out, toDTO(r))
	}
	return c.JSON(fiber.Map{"items": out, "pagination": mapper.Pagination(filter.Page, total)})
}

// @Summary Get a role the API can change
// @Tags Roles
// @Produce json
// @Param id path string true "Role ID"
// @Security BearerAuth
// @Success 200 {object} roleDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/roles/{id} [get]
func (h *Handler) Get(c fiber.Ctx) error {
	role, err := h.svc.Get(c.Context(), c.Params("id"))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(toDTO(*role))
}

// @Summary Delete roles without members; all or none
// @Tags Roles
// @Accept json
// @Produce json
// @Param body body idsDTO true "Role IDs, 1 to 100"
// @Security BearerAuth
// @Success 204
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/roles/delete [post]
func (h *Handler) Delete(c fiber.Ctx) error {
	var body idsDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	if err := h.svc.DeleteMany(c.Context(), body.IDs); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary Create a role
// @Tags Roles
// @Produce json
// @Accept json
// @Param body body createDTO true "Request"
// @Security BearerAuth
// @Success 201 {object} roleDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/roles [post]
func (h *Handler) Create(c fiber.Ctx) error {
	var body createDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	role, err := h.svc.Create(c.Context(), body.Name)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(toDTO(*role))
}

// @Summary Update the name, the enabled flag or the default mark of a role
// @Tags Roles
// @Produce json
// @Accept json
// @Param id path string true "Role ID"
// @Param body body updateDTO true "Request"
// @Security BearerAuth
// @Success 200 {object} roleDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/roles/{id} [patch]
func (h *Handler) Update(c fiber.Ctx) error {
	var body updateDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	role, err := h.svc.Update(c.Context(), c.Params("id"), domain.RolePatch{Name: body.Name, Enabled: body.Enabled, IsDefault: body.IsDefault})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(toDTO(*role))
}

// @Summary Replace every permission of a role at once; a resource left out loses its actions
// @Tags Roles
// @Produce json
// @Accept json
// @Param id path string true "Role ID"
// @Param body body permissionsDTO true "Request"
// @Security BearerAuth
// @Success 200 {object} roleDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/roles/{id}/permissions [put]
func (h *Handler) SetPermissions(c fiber.Ctx) error {
	var body permissionsDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	if body.Permissions == nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "permissions required").WithField("permissions"))
	}
	permissions := make(map[string]domain.Action, len(body.Permissions))
	for resource, names := range body.Permissions {
		actions, ok := domain.ParseActions(names)
		if !ok {
			return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "actions must be create, read, update or delete").WithField("permissions"))
		}
		permissions[resource] = actions
	}
	role, err := h.svc.SetPermissions(c.Context(), c.Params("id"), permissions)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(toDTO(*role))
}

// @Summary List the members of a role
// @Tags Roles
// @Produce json
// @Param id path string true "Role ID"
// @Param page query int false "Page" default(1)
// @Param limit query int false "Items per page, up to 100" default(20)
// @Param search query string false "Part of the name or email"
// @Security BearerAuth
// @Success 200 {object} object{items=[]memberDTO,pagination=mapper.PaginationDTO}
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/roles/{id}/members [get]
func (h *Handler) Members(c fiber.Ctx) error {
	page := domain.NewPage(fiber.Query[int](c, "page"), fiber.Query[int](c, "limit"))
	users, total, err := h.svc.Members(c.Context(), c.Params("id"), domain.UserFilter{Page: page, Search: c.Query("search")})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]memberDTO, 0, len(users))
	for _, u := range users {
		out = append(out, memberDTO{ID: u.ID, Name: u.Name, Email: u.Email, Status: u.Status})
	}
	return c.JSON(fiber.Map{"items": out, "pagination": mapper.Pagination(page, total)})
}

// @Summary Add a user to a role
// @Tags Roles
// @Produce json
// @Param id path string true "Role ID"
// @Param user_id path string true "User ID"
// @Security BearerAuth
// @Success 204
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/roles/{id}/members/{user_id} [put]
func (h *Handler) AddMember(c fiber.Ctx) error {
	if err := h.svc.AddMember(c.Context(), c.Params("id"), c.Params("user_id")); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary Remove a user from a role
// @Tags Roles
// @Produce json
// @Param id path string true "Role ID"
// @Param user_id path string true "User ID"
// @Security BearerAuth
// @Success 204
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/roles/{id}/members/{user_id} [delete]
func (h *Handler) RemoveMember(c fiber.Ctx) error {
	if err := h.svc.RemoveMember(c.Context(), c.Params("id"), c.Params("user_id")); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary Replace every role of a user
// @Description Roles not listed are removed. Protected roles and users cannot be managed here.
// @Tags Users
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Param body body userRolesDTO true "Role IDs"
// @Security BearerAuth
// @Success 204
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/users/{id}/roles [put]
func (h *Handler) SetUserRoles(c fiber.Ctx) error {
	var body userRolesDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	if body.RoleIDs == nil { // a missing field must not strip every role
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "role_ids is required").WithField("role_ids"))
	}
	if err := h.svc.SetUserRoles(c.Context(), c.Params("id"), body.RoleIDs); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
