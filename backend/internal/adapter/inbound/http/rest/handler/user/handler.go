package user

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
)

// Inviter creates accounts on behalf of an administrator.
type Inviter interface {
	Invite(ctx context.Context, cmd input.RegisterCommand) (*input.RegisterResult, error)
	Reinvite(ctx context.Context, userID string) (*input.RegisterResult, error)
	ActivationLink(ctx context.Context, userID string) (*input.RegisterResult, error)
}

type Handler struct {
	svc     input.UserService
	inviter Inviter
}

func New(svc input.UserService, inviter Inviter) *Handler {
	return &Handler{svc: svc, inviter: inviter}
}

// Routes expects a router already behind Bearer.
func (h *Handler) Routes(r fiber.Router, authz input.Authorizer) {
	read := middleware.Require(authz, domain.ResourceUsers, domain.ActionRead)
	update := middleware.Require(authz, domain.ResourceUsers, domain.ActionUpdate)
	r.Get("/", read, h.List)
	r.Get("/counts", read, h.Counts)
	r.Post("/", middleware.Require(authz, domain.ResourceUsers, domain.ActionCreate), h.Invite)
	r.Post("/:id/invite", update, h.Reinvite)
	// The link signs in as the account, roles included; only who may grant roles may hold it.
	r.Post("/:id/activation-link", update, middleware.Require(authz, domain.ResourceRoles, domain.ActionUpdate), h.ActivationLink)
	r.Get("/:id", read, h.Get)
	r.Post("/disable", update, h.Disable)
	r.Post("/enable", update, h.Enable)
	r.Post("/unlock", update, h.Unlock)
	r.Delete("/:id/sessions", update, h.RevokeSessions)
}

type userDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Status    string    `json:"status" example:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type roleDTO struct {
	ID   string `json:"id"`
	Name string `json:"name" example:"support"`
}

type detailsDTO struct {
	userDTO
	Roles  []roleDTO  `json:"roles"`
	Access *accessDTO `json:"access,omitempty"`
}

type accessDTO struct {
	LastLoginAt    *time.Time `json:"last_login_at"`
	FailedLogins   int        `json:"failed_logins"`   // toward the account lock
	LockedUntil    *time.Time `json:"locked_until"`    // past or null: not locked
	RecentFailures int        `json:"recent_failures"` // inside the login email window
	ActiveSessions int        `json:"active_sessions" example:"2"`
	// ActivationExpiresAt is when the live activation link stops working; null when there is none.
	ActivationExpiresAt *time.Time `json:"activation_expires_at"`
}

func toDTO(u domain.User) userDTO {
	return userDTO{ID: u.ID, Name: u.Name, Email: u.Email, Status: u.Status, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt}
}

// @Summary List users, without those holding a protected role
// @Tags Users
// @Produce json
// @Param page query int false "Page" default(1)
// @Param limit query int false "Items per page, up to 100" default(20)
// @Param search query string false "Part of the name or email"
// @Param status query string false "Status" Enums(pending, active, locked, disabled)
// @Param role_id query string false "Only holders of this role"
// @Param sort query string false "Sort field" Enums(name, email, status, created_at) default(name)
// @Param order query string false "Sort direction" Enums(asc, desc) default(asc)
// @Security BearerAuth
// @Success 200 {object} object{items=[]userDTO,pagination=mapper.PaginationDTO}
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/users [get]
func (h *Handler) List(c fiber.Ctx) error {
	filter := domain.UserFilter{
		Page:   domain.NewPage(fiber.Query[int](c, "page"), fiber.Query[int](c, "limit")),
		Search: c.Query("search"),
		Status: c.Query("status"),
		RoleID: c.Query("role_id"),
		Sort:   c.Query("sort"),
		Order:  c.Query("order"),
	}
	users, total, err := h.svc.List(c.Context(), filter)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]userDTO, 0, len(users))
	for _, u := range users {
		out = append(out, toDTO(u))
	}
	return c.JSON(fiber.Map{"items": out, "pagination": mapper.Pagination(filter.Page, total)})
}

type countsDTO struct {
	Total   int64 `json:"total"`
	Pending int64 `json:"pending"`
	Locked  int64 `json:"locked"`
}

// @Summary Count the listed users, in total and waiting on an action
// @Tags Users
// @Produce json
// @Security BearerAuth
// @Success 200 {object} countsDTO
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/users/counts [get]
func (h *Handler) Counts(c fiber.Ctx) error {
	n, err := h.svc.Counts(c.Context())
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(countsDTO{Total: n.Total, Pending: n.Pending, Locked: n.Locked})
}

// @Summary Get a user with their roles
// @Tags Users
// @Produce json
// @Param id path string true "User ID"
// @Security BearerAuth
// @Success 200 {object} detailsDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/users/{id} [get]
func (h *Handler) Get(c fiber.Ctx) error {
	user, err := h.svc.Get(c.Context(), c.Params("id"))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := detailsDTO{userDTO: toDTO(user.User), Roles: make([]roleDTO, 0, len(user.Roles))}
	for _, r := range user.Roles {
		out.Roles = append(out.Roles, roleDTO{ID: r.ID, Name: r.Name})
	}
	if a := user.Access; a != nil {
		out.Access = &accessDTO{a.LastLoginAt, a.FailedLogins, a.LockedUntil, a.RecentFailures, a.ActiveSessions, a.ActivationExpiresAt}
	}
	return c.JSON(out)
}

type idsDTO struct {
	IDs []string `json:"ids"`
}

func (h *Handler) bindIDs(c fiber.Ctx) ([]string, error) {
	var body idsDTO
	if err := c.Bind().Body(&body); err != nil {
		return nil, apperror.New(apperror.CodeInvalidInput, "invalid body")
	}
	return body.IDs, nil
}

// @Summary Disable users and end their sessions; all or none
// @Tags Users
// @Accept json
// @Produce json
// @Param body body idsDTO true "User IDs, 1 to 100; not your own"
// @Security BearerAuth
// @Success 204
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/users/disable [post]
func (h *Handler) Disable(c fiber.Ctx) error {
	ids, err := h.bindIDs(c)
	if err == nil {
		err = h.svc.DisableMany(c.Context(), middleware.UserID(c), ids)
	}
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary Enable users; one who never set a password goes back to pending. All or none
// @Tags Users
// @Accept json
// @Produce json
// @Param body body idsDTO true "User IDs, 1 to 100"
// @Security BearerAuth
// @Success 204
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/users/enable [post]
func (h *Handler) Enable(c fiber.Ctx) error {
	ids, err := h.bindIDs(c)
	if err == nil {
		err = h.svc.EnableMany(c.Context(), ids)
	}
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary Let users sign in again: lift the account lock and forget recent failed logins. All or none
// @Tags Users
// @Accept json
// @Produce json
// @Param body body idsDTO true "User IDs, 1 to 100"
// @Security BearerAuth
// @Success 204
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/users/unlock [post]
func (h *Handler) Unlock(c fiber.Ctx) error {
	ids, err := h.bindIDs(c)
	if err == nil {
		err = h.svc.UnlockMany(c.Context(), ids)
	}
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type inviteDTO struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// @Summary Create an account for someone and send its activation email
// @Tags Users
// @Accept json
// @Produce json
// @Param body body inviteDTO true "Request"
// @Security BearerAuth
// @Success 201 {object} object{user_id=string,status=string,activation_token=string} "activation_token only with manual delivery"
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/users [post]
func (h *Handler) Invite(c fiber.Ctx) error {
	var body inviteDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	res, err := h.inviter.Invite(c.Context(), input.RegisterCommand{Name: body.Name, Email: body.Email})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(inviteAnswer(res))
}

func inviteAnswer(res *input.RegisterResult) fiber.Map {
	out := fiber.Map{"user_id": res.UserID, "status": res.Status}
	if res.ManualDelivery {
		out["activation_token"] = res.ActivationToken
	}
	return out
}

// @Summary Send a pending account a new activation email
// @Tags Users
// @Produce json
// @Param id path string true "User ID"
// @Security BearerAuth
// @Success 200 {object} object{user_id=string,status=string,activation_token=string} "activation_token only with manual delivery"
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/users/{id}/invite [post]
func (h *Handler) Reinvite(c fiber.Ctx) error {
	res, err := h.inviter.Reinvite(c.Context(), c.Params("id"))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(inviteAnswer(res))
}

// @Summary Make a new activation link for a pending user, to hand over by hand
// @Description The old link stops working and no email is sent. The token comes back
// @Description whatever the delivery mode. Needs users:update and roles:update: the account may hold roles already.
// @Tags Users
// @Produce json
// @Param id path string true "User ID"
// @Security BearerAuth
// @Success 200 {object} object{user_id=string,status=string,activation_token=string,expires_at=string}
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/users/{id}/activation-link [post]
func (h *Handler) ActivationLink(c fiber.Ctx) error {
	res, err := h.inviter.ActivationLink(c.Context(), c.Params("id"))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(fiber.Map{"user_id": res.UserID, "status": res.Status, "activation_token": res.ActivationToken, "expires_at": res.ExpiresAt})
}

// @Summary End every session of another user
// @Tags Users
// @Produce json
// @Param id path string true "User ID"
// @Security BearerAuth
// @Success 204
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/users/{id}/sessions [delete]
func (h *Handler) RevokeSessions(c fiber.Ctx) error {
	if err := h.svc.RevokeSessions(c.Context(), middleware.UserID(c), c.Params("id")); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
