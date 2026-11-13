// Package reqctx carries who made a request and its correlation id through a
// context, from the HTTP layer down to logs, the outbox and the audit log.
package reqctx

import "context"

type key int

const (
	requestIDKey key = iota
	actorKey
	apiKeyKey
)

// APIKey is the key a request came with: it reaches one collection, read only
// unless CanWrite, and acts as the member who made it (the actor).
type APIKey struct {
	ID           string
	Name         string
	CollectionID string
	CanWrite     bool
}

func WithAPIKey(ctx context.Context, k APIKey) context.Context {
	return context.WithValue(ctx, apiKeyKey, k)
}

// Key is the request's API key; ok is false for a signed in person.
func Key(ctx context.Context) (APIKey, bool) {
	k, ok := ctx.Value(apiKeyKey).(APIKey)
	return k, ok
}

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
