package auth

import (
	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/pkg/apperror"
)

type mfaStatusDTO struct {
	Enabled      bool `json:"enabled"`
	RecoveryLeft int  `json:"recovery_left"`
} // @name MFAStatus

type mfaSetupDTO struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"`
} // @name MFASetup

type mfaCodeDTO struct {
	Code string `json:"code"`
} // @name MFACodeInput

type mfaRecoveryDTO struct {
	RecoveryCodes []string `json:"recovery_codes"`
} // @name MFARecoveryCodes

type mfaDisableDTO struct {
	Password string `json:"password"`
} // @name MFADisableInput

// @Summary Whether two-step login is on
// @Tags Profile
// @Produce json
// @Security BearerAuth
// @Success 200 {object} mfaStatusDTO
// @Failure 401 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID authMFAStatus
// @Router /v1/me/mfa [get]
func (h *Handler) MFAStatus(c fiber.Ctx) error {
	st, err := h.svc.MFAStatus(c.Context())
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(mfaStatusDTO{Enabled: st.Enabled, RecoveryLeft: st.RecoveryLeft})
}

// @Summary Start two-step login: a new secret for an authenticator app
// @Description Nothing changes at login until /v1/me/mfa/enable confirms a code.
// @Tags Profile
// @Produce json
// @Security BearerAuth
// @Success 200 {object} mfaSetupDTO
// @Failure 401 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID authSetupMFA
// @Router /v1/me/mfa/setup [post]
func (h *Handler) SetupMFA(c fiber.Ctx) error {
	secret, uri, err := h.svc.SetupMFA(c.Context())
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(mfaSetupDTO{Secret: secret, URI: uri})
}

// @Summary Turn two-step login on with a code from the app
// @Description Answers the recovery codes, shown this once.
// @Tags Profile
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body mfaCodeDTO true "Request"
// @Success 200 {object} mfaRecoveryDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID authEnableMFA
// @Router /v1/me/mfa/enable [post]
func (h *Handler) EnableMFA(c fiber.Ctx) error {
	var body mfaCodeDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	codes, err := h.svc.EnableMFA(c.Context(), body.Code)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(mfaRecoveryDTO{RecoveryCodes: codes})
}

// @Summary Turn two-step login off
// @Tags Profile
// @Accept json
// @Security BearerAuth
// @Param body body mfaDisableDTO true "Request"
// @Success 204
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID authDisableMFA
// @Router /v1/me/mfa/disable [post]
func (h *Handler) DisableMFA(c fiber.Ctx) error {
	var body mfaDisableDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	if err := h.svc.DisableMFA(c.Context(), body.Password); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
