// Package files stores uploaded files on local disk or in an S3-compatible bucket.
package files

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
)

// Keys are written by the application, never by clients; the check stops a bug
// from reaching outside the storage root.
var keyFormat = regexp.MustCompile(`^[a-z0-9_-]+(/[a-zA-Z0-9_.-]+)*$`)

func checkKey(key string) error {
	if !keyFormat.MatchString(key) || strings.Contains(key, "..") || len(key) > 512 {
		return fmt.Errorf("invalid file key %q", key)
	}
	return nil
}

func notFound() error { return apperror.New(apperror.CodeNotFound, "file not found") }

// Local keeps files under one directory. Only one backend instance may use it.
type Local struct {
	Dir string
}

var _ output.FileStore = (*Local)(nil)

func (l *Local) path(key string) (string, error) {
	if err := checkKey(key); err != nil {
		return "", err
	}
	return filepath.Join(l.Dir, filepath.FromSlash(key)), nil
}

// Put writes to a temporary file and renames it, so a reader never sees half a file.
func (l *Local) Put(_ context.Context, key string, data []byte) error {
	path, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".upload-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // a no-op once renamed
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (l *Local) Get(_ context.Context, key string) ([]byte, error) {
	path, err := l.path(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path) //nolint:gosec // path comes from checkKey, which keeps it under Dir
	if errors.Is(err, fs.ErrNotExist) {
		return nil, notFound()
	}
	return data, err
}

func (l *Local) Delete(_ context.Context, key string) error {
	path, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
