package auth

import (
	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/validate"
)

type ssoInfoDTO struct {
	Enabled bool   `json:"enabled"`
	Name    string `json:"name,omitempty"`
} // @name SSOInfo

type ssoStartDTO struct {
	URL  string `json:"url"`
	Flow string `json:"flow"` // keep it (an httpOnly cookie) and send it back to the callback
} // @name SSOStart

type ssoCallbackDTO struct {
	Code  string `json:"code"`
	State string `json:"state"`
	Flow  string `json:"flow"`
} // @name SSOCallbackInput

// @Summary Whether single sign-on is on, and its button label
// @Tags Auth
// @Produce json
// @Success 200 {object} ssoInfoDTO
// @Failure 429 {object} mapper.errorBody
// @ID authSSOInfo
// @Router /v1/auth/sso [get]
func (h *Handler) SSOInfo(c fiber.Ctx) error {
	name := h.svc.SSOName()
	return c.JSON(ssoInfoDTO{Enabled: name != "", Name: name})
}

// @Summary Start single sign-on
// @Description Answers the provider address to send the browser to, and a sealed flow for the callback.
// @Tags Auth
// @Produce json
// @Success 200 {object} ssoStartDTO
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID authSSOStart
// @Router /v1/auth/sso/start [post]
func (h *Handler) SSOStart(c fiber.Ctx) error {
	url, flow, err := h.svc.SSOStart(c.Context())
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(ssoStartDTO{URL: url, Flow: flow})
}

// @Summary Finish single sign-on
// @Description Logs in the existing, active account whose email the provider verified; it never creates accounts.
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body ssoCallbackDTO true "Request"
// @Success 200 {object} tokenPairDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID authSSOCallback
// @Router /v1/auth/sso/callback [post]
func (h *Handler) SSOCallback(c fiber.Ctx) error {
	var body ssoCallbackDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	ip := c.IP()
	ua := validate.Text(string(c.Request().Header.UserAgent()), maxUserAgentBytes)
	pair, err := h.svc.SSOLogin(c.Context(), input.SSOCommand{Code: body.Code, State: body.State, Flow: body.Flow, IPAddress: &ip, UserAgent: &ua})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return h.sendTokens(c, pair)
}
