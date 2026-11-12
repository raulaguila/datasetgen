package shared

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/raulaguila/dataset/pkg/apperror"
)

// WrapError translates infrastructure-level errors (gorm, pgerror) into
// domain-level apperror types. If the error is not a recognized infrastructure
// error, it is wrapped as an internal database error.
func WrapError(err error, resource string) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[*apperror.Error](err); ok {
		return err
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return apperror.New(apperror.CodeTimedOut, resource+" query timed out").WithCause(err)
	case errors.Is(err, gorm.ErrRecordNotFound):
		return apperror.New(apperror.CodeNotFound, resource+" not found").WithDetails("resource", resource)
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pgErr.Code {
		case "23505": // unique_violation
			return apperror.New(apperror.CodeAlreadyExists, resource+" already exists").WithDetails("resource", resource)
		case "23503": // foreign_key_violation
			return apperror.New(apperror.CodeResourceInUse, resource+" is in use").WithDetails("resource", resource)
		case "23514": // check_violation
			return apperror.New(apperror.CodeValidationFailed, "constraint violation")
		case "42703": // undefined_column
			return apperror.New(apperror.CodeInvalidInput, "undefined column or parameter name").WithField("column")
		}
	}
	return apperror.New(apperror.CodeDatabaseError, resource+" database error").WithCause(err)
}
