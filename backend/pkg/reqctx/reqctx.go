// Package reqctx carries who made a request and its correlation id through a
// context, from the HTTP layer down to logs, the outbox and the audit log.
package reqctx

import "context"

type key int

const (
	requestIDKey key = iota
	actorKey
)

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID is empty outside an HTTP request.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

func WithActor(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, actorKey, userID)
}

// Actor is the authenticated user, empty for anonymous requests and the operator CLI.
func Actor(ctx context.Context) string {
	id, _ := ctx.Value(actorKey).(string)
	return id
}
