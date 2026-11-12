package auth

import (
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/validate"
)

const maxUserAgentBytes = 512

type Handler struct {
	svc     input.AuthService
	account input.AccountService
}

func New(svc input.AuthService, account input.AccountService) *Handler {
	return &Handler{svc: svc, account: account}
}

// MeRoutes mounts the profile of the current user; r is already behind Bearer.
func (h *Handler) MeRoutes(r fiber.Router, passwordLimit fiber.Handler) {
	r.Get("/", h.Me)
	r.Patch("/", h.Rename)
	r.Post("/password", passwordLimit, h.ChangePassword)
	r.Get("/avatar", h.Avatar)
	r.Put("/avatar", h.SetAvatar)
	r.Delete("/avatar", h.RemoveAvatar)
}

// Routes mounts the auth routes; each limiter runs on the routes that need it.
func (h *Handler) Routes(r fiber.Router, bearer, authLimit, passwordLimit fiber.Handler) {
	r.Post("/register", authLimit, h.Register)
	r.Post("/activate", authLimit, h.Activate)
	r.Post("/login", authLimit, h.Login)
	r.Post("/refresh", h.Refresh)
	r.Post("/logout", bearer, h.Logout)
	r.Post("/password-reset/request", passwordLimit, h.RequestPasswordReset)
	r.Post("/password-reset/confirm", passwordLimit, h.ConfirmPasswordReset)
}

type registerDTO struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type activateDTO struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type loginDTO struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshDTO struct {
	RefreshToken string `json:"refresh_token"`
}

type renameDTO struct {
	Name string `json:"name"`
}

type changePasswordDTO struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type resetRequestDTO struct {
	Email string `json:"email"`
}

type resetConfirmDTO struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// userDTO is the same object in login, refresh and /me.
type userDTO struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Email       string                `json:"email"`
	Status      string                `json:"status"`
	CreatedAt   time.Time             `json:"created_at"`
	IsSuperuser bool                  `json:"is_superuser"`
	Permissions map[string][]string   `json:"permissions"`
	Preferences mapper.PreferencesDTO `json:"preferences"`
	Roles       []string              `json:"roles"`
	// AvatarAt is when the picture was last set; absent when there is none. Use it
	// as a cache buster on /v1/me/avatar.
	AvatarAt *time.Time `json:"avatar_at,omitempty"`
}

type tokenPairDTO struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresIn    int64    `json:"expires_in"`
	TokenType    string   `json:"token_type"`
	User         *userDTO `json:"user,omitempty"` // missing only if the profile failed to load
}

type meDTO struct {
	User      userDTO `json:"user"`
	SessionID string  `json:"session_id"`
}

func toUserDTO(p *domain.Profile) userDTO {
	return userDTO{
		ID:          p.User.ID,
		Name:        p.User.Name,
		Email:       p.User.Email,
		Status:      p.User.Status,
		CreatedAt:   p.User.CreatedAt,
		IsSuperuser: p.IsSuperuser,
		Permissions: mapper.Permissions(p.Permissions),
		Preferences: mapper.Preferences(p.Preferences),
		Roles:       p.Roles,
		AvatarAt:    p.AvatarAt,
	}
}

// sendTokens answers with the tokens even if the profile fails: a refresh has
// already spent the old token, so dropping the new one would end the session.
func (h *Handler) sendTokens(c fiber.Ctx, pair *domain.TokenPair) error {
	out := tokenPairDTO{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresIn:    pair.ExpiresIn,
		TokenType:    pair.TokenType,
	}
	profile, err := h.account.Profile(c.Context(), pair.UserID)
	if err != nil {
		c.Locals(mapper.LocalCause, err)
		return c.JSON(out)
	}
	user := toUserDTO(profile)
	out.User = &user
	return c.JSON(out)
}

// @Summary Create an account and send its activation email
// @Tags Registration
// @Produce json
// @Accept json
// @Param body body registerDTO true "Request"
// @Success 201 {object} object{user_id=string,status=string,activation_token=string} "activation_token only with manual delivery"
// @Failure 400 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/auth/register [post]
func (h *Handler) Register(c fiber.Ctx) error {
	var body registerDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	res, err := h.svc.Register(c.Context(), input.RegisterCommand{Name: body.Name, Email: body.Email})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := fiber.Map{
		"user_id": res.UserID,
		"status":  res.Status,
	}
	if res.ManualDelivery {
		out["activation_token"] = res.ActivationToken
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// @Summary Activate an account and set its password
// @Tags Registration
// @Produce json
// @Accept json
// @Param body body activateDTO true "Request"
// @Success 204
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/auth/activate [post]
func (h *Handler) Activate(c fiber.Ctx) error {
	var body activateDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	if err := h.svc.Activate(c.Context(), input.ActivateCommand{Token: body.Token, Password: body.Password}); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary Log in
// @Tags Auth
// @Produce json
// @Accept json
// @Param body body loginDTO true "Request"
// @Success 200 {object} tokenPairDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/auth/login [post]
func (h *Handler) Login(c fiber.Ctx) error {
	var body loginDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	ip := c.IP()
	ua := validate.Text(string(c.Request().Header.UserAgent()), maxUserAgentBytes)
	pair, err := h.svc.Login(c.Context(), input.LoginCommand{
		Email:     body.Email,
		Password:  body.Password,
		IPAddress: &ip,
		UserAgent: &ua,
	})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return h.sendTokens(c, pair)
}

// @Summary Rotate the refresh token
// @Tags Auth
// @Produce json
// @Accept json
// @Param body body refreshDTO true "Request"
// @Success 200 {object} tokenPairDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/auth/refresh [post]
func (h *Handler) Refresh(c fiber.Ctx) error {
	var body refreshDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	pair, err := h.svc.Refresh(c.Context(), input.RefreshCommand{RefreshToken: body.RefreshToken})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return h.sendTokens(c, pair)
}

// @Summary End the current session
// @Tags Auth
// @Produce json
// @Security BearerAuth
// @Success 204
// @Failure 401 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/auth/logout [post]
func (h *Handler) Logout(c fiber.Ctx) error {
	if err := h.svc.Logout(c.Context(), middleware.SessionID(c)); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary Current user, with live permissions and preferences
// @Tags Profile
// @Produce json
// @Security BearerAuth
// @Success 200 {object} meDTO
// @Failure 401 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/me [get]
func (h *Handler) Me(c fiber.Ctx) error {
	profile, err := h.account.Profile(c.Context(), middleware.UserID(c))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(meDTO{User: toUserDTO(profile), SessionID: middleware.SessionID(c)})
}

// @Summary Change the name of the current user
// @Tags Profile
// @Produce json
// @Accept json
// @Security BearerAuth
// @Param body body renameDTO true "Request"
// @Success 200 {object} meDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/me [patch]
func (h *Handler) Rename(c fiber.Ctx) error {
	var body renameDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	profile, err := h.account.Rename(c.Context(), middleware.UserID(c), body.Name)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(meDTO{User: toUserDTO(profile), SessionID: middleware.SessionID(c)})
}

// @Summary Get the profile picture
// @Tags Profile
// @Produce image/png,image/jpeg,image/webp
// @Security BearerAuth
// @Success 200 {file} binary
// @Failure 401 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/me/avatar [get]
func (h *Handler) Avatar(c fiber.Ctx) error {
	image, contentType, err := h.account.Avatar(c.Context(), middleware.UserID(c))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	c.Set(fiber.HeaderContentType, contentType)
	c.Set("X-Content-Type-Options", "nosniff")
	// The URL carries avatar_at, so a new picture is a new URL; private keeps it out of shared caches.
	c.Set(fiber.HeaderCacheControl, "private, max-age=86400")
	return c.Send(image)
}

// @Summary Set the profile picture
// @Description The body is the image itself: PNG, JPEG or WebP, at most 100 KB. The type is read from the bytes.
// @Tags Profile
// @Accept image/png,image/jpeg,image/webp
// @Security BearerAuth
// @Param body body string true "Image bytes"
// @Success 204
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/me/avatar [put]
func (h *Handler) SetAvatar(c fiber.Ctx) error {
	if err := h.account.SetAvatar(c.Context(), middleware.UserID(c), c.Body()); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary Remove the profile picture
// @Tags Profile
// @Security BearerAuth
// @Success 204
// @Failure 401 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/me/avatar [delete]
func (h *Handler) RemoveAvatar(c fiber.Ctx) error {
	if err := h.account.RemoveAvatar(c.Context(), middleware.UserID(c)); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary Change the password and end the other sessions
// @Tags Profile
// @Accept json
// @Security BearerAuth
// @Param body body changePasswordDTO true "Request"
// @Success 204
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/me/password [post]
func (h *Handler) ChangePassword(c fiber.Ctx) error {
	var body changePasswordDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	err := h.svc.ChangePassword(c.Context(), input.ChangePasswordCommand{
		UserID:          middleware.UserID(c),
		SessionID:       middleware.SessionID(c),
		CurrentPassword: body.CurrentPassword,
		NewPassword:     body.NewPassword,
	})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary Send a password reset email
// @Tags Password reset
// @Produce json
// @Accept json
// @Param body body resetRequestDTO true "Request"
// @Success 200 {object} object{ok=bool,reset_token=string} "reset_token only with manual delivery"
// @Failure 400 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/auth/password-reset/request [post]
func (h *Handler) RequestPasswordReset(c fiber.Ctx) error {
	var body resetRequestDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	res, err := h.svc.RequestPasswordReset(c.Context(), input.RequestPasswordResetCommand{Email: body.Email})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := fiber.Map{"ok": true}
	if res.ManualDelivery {
		out["reset_token"] = res.ResetToken
	}
	return c.JSON(out)
}

// @Summary Set a new password with a reset token
// @Tags Password reset
// @Produce json
// @Accept json
// @Param body body resetConfirmDTO true "Request"
// @Success 204
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/auth/password-reset/confirm [post]
func (h *Handler) ConfirmPasswordReset(c fiber.Ctx) error {
	var body resetConfirmDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	if err := h.svc.ConfirmPasswordReset(c.Context(), input.ConfirmPasswordResetCommand{
		Token: body.Token, Password: body.Password,
	}); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
