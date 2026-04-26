package storage

import (
	"context"
	"io"
)

// FileStorage abstracts where files (audio, images, etc.) are stored.
// Replace LocalStorage with S3Storage (or any other provider) without touching handlers.
type FileStorage interface {
	Upload(ctx context.Context, filename string, r io.Reader, contentType string) (url string, err error)
	// Delete removes a file by its public URL. A no-op (nil error) if url is empty.
	Delete(ctx context.Context, url string) error
}
