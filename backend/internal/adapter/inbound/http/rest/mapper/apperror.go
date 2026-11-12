package mapper

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/pkg/apperror"
)

// LocalCause holds the error behind a 5xx or degraded answer, for the request
// log. Clients never see it.
const LocalCause = "http.error_cause"

type errorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Field   string         `json:"field,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

// WriteError maps apperror (or generic error) to an HTTP JSON response.
func WriteError(c fiber.Ctx, err error) error {
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		appErr = apperror.New(apperror.CodeInternal, "internal error").WithCause(err)
	}

	status := statusFor(appErr.Code)
	if seconds, ok := appErr.Details[apperror.DetailRetryAfter].(int); ok && seconds > 0 {
		c.Set(fiber.HeaderRetryAfter, strconv.Itoa(seconds))
	}
	if status >= fiber.StatusInternalServerError {
		c.Locals(LocalCause, err)
	}
	return c.Status(status).JSON(errorBody{
		Code:    string(appErr.Code),
		Message: appErr.Message,
		Field:   appErr.Field,
		Details: appErr.Details,
	})
}

// ErrorHandler gives errors raised by Fiber itself, such as an unknown route or
// an oversized body, the same JSON shape as every other error.
func ErrorHandler(c fiber.Ctx, err error) error {
	var fiberErr *fiber.Error
	if !errors.As(err, &fiberErr) {
		return WriteError(c, err)
	}
	code := apperror.CodeInvalidInput
	switch {
	case fiberErr.Code == fiber.StatusNotFound:
		code = apperror.CodeNotFound
	case fiberErr.Code >= fiber.StatusInternalServerError:
		code = apperror.CodeInternal
	}
	return c.Status(fiberErr.Code).JSON(errorBody{Code: string(code), Message: fiberErr.Message})
}

func statusFor(code apperror.Code) int {
	switch code {
	case apperror.CodeUnauthorized, apperror.CodeInvalidCredentials, apperror.CodeTokenExpired:
		return fiber.StatusUnauthorized
	case apperror.CodeForbidden:
		return fiber.StatusForbidden
	case apperror.CodeNotFound:
		return fiber.StatusNotFound
	case apperror.CodeAlreadyExists, apperror.CodeConflict, apperror.CodeResourceInUse:
		return fiber.StatusConflict
	case apperror.CodeInvalidInput, apperror.CodeValidationFailed:
		return fiber.StatusBadRequest
	case apperror.CodeTooManyRequests:
		return fiber.StatusTooManyRequests
	case apperror.CodeTimedOut:
		return fiber.StatusGatewayTimeout
	default:
		return fiber.StatusInternalServerError
	}
}
