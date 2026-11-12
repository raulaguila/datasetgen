package output

import "context"

// FileStore keeps uploaded files by key, such as "avatars/<user id>".
// shortcut: whole files in memory; stream with io.Reader once files exceed a few MB.
type FileStore interface {
	Put(ctx context.Context, key string, data []byte) error
	// Get returns a CodeNotFound apperror when the key holds nothing.
	Get(ctx context.Context, key string) ([]byte, error)
	// Delete succeeds when the key already holds nothing.
	Delete(ctx context.Context, key string) error
}
