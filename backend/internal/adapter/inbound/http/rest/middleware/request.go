package middleware

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"time"

	"github.com/raulaguila/dataset/pkg/telemetry"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/clientip"
	"github.com/raulaguila/dataset/pkg/logger"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

var requestIDFormat = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// RequestID gives every request a correlation id, returned in X-Request-ID and
// carried by the context to logs, the outbox and the audit log. An incoming id
// is kept only from a trusted proxy: any client could reuse another's id.
func RequestID() fiber.Handler {
	return func(c fiber.Ctx) error {
		id := c.Get(fiber.HeaderXRequestID)
		if !c.IsProxyTrusted() || !requestIDFormat.MatchString(id) {
			id = uuid.NewString()
		}
		c.Set(fiber.HeaderXRequestID, id)
		c.SetContext(reqctx.WithRequestID(c.Context(), id))
		return c.Next()
	}
}

// RequestLog always records server errors and degraded responses. all also
// enables ordinary request logs. Only paths are logged, never query strings.
func RequestLog(log *logger.Logger, all bool) fiber.Handler {
	zero := log.Zerolog()
	return func(c fiber.Ctx) error {
		start := time.Now()
		defer func() { telemetry.ObserveRequest(time.Since(start), c.Response().StatusCode()) }()

		failure := c.Next()
		if failure != nil {
			if err := c.App().ErrorHandler(c, failure); err != nil {
				return err
			}
		}

		status := c.Response().StatusCode()
		cause, _ := c.Locals(mapper.LocalCause).(error)
		degraded := cause != nil
		if !all && !degraded && status < fiber.StatusInternalServerError {
			return nil
		}
		if cause == nil {
			cause = failure
		}

		level, message := zerolog.InfoLevel, "HTTP Request"
		switch {
		case status >= fiber.StatusInternalServerError:
			level, message = zerolog.ErrorLevel, "Server error"
		case degraded:
			level, message = zerolog.WarnLevel, "Request degraded"
		case status >= fiber.StatusBadRequest:
			level, message = zerolog.WarnLevel, "Client error"
		}
		event := zero.WithLevel(level).
			Int("status", status).
			Str("method", c.Method()).
			Str("ip", c.IP()).
			Str("latency", time.Since(start).String()). // "12.3ms": the unit travels with the value
			Str("path", c.Path()).
			Str("request_id", reqctx.RequestID(c.Context())).
			Err(cause)
		if appErr, ok := errors.AsType[*apperror.Error](cause); ok && appErr.Cause != nil {
			event.Str("cause", appErr.Cause.Error())
		}
		event.Msg(message)
		return nil
	}
}

// Timeout bounds the work of a request, so a slow database cannot pile up
// requests without end.
func Timeout(d time.Duration) fiber.Handler {
	return func(c fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), d)
		defer cancel()
		c.SetContext(ctx)
		return c.Next()
	}
}

// Hits counts one request for key in the current minute and returns the total
// of that minute, across every instance.
type Hits func(ctx context.Context, key string) (int64, error)

// RateLimit allows maxHits requests per client per minute. name keeps the
// limiters of one client apart. A counter failure fails the request: the
// database it needs is down anyway.
func RateLimit(name string, maxHits int, hits Hits) fiber.Handler {
	return func(c fiber.Ctx) error {
		first, _ := clientip.Range(c.IP())
		n, err := hits(c.Context(), name+":"+first)
		if err != nil {
			return err
		}
		if n > int64(maxHits) {
			c.Set(fiber.HeaderRetryAfter, strconv.Itoa(60-time.Now().Second()))
			return mapper.WriteError(c, apperror.New(apperror.CodeTooManyRequests, "too many requests"))
		}
		return c.Next()
	}
}
