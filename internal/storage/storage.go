package storage

import (
	"context"
	"io"
)

// AudioStorage abstracts where audio files are stored.
// Replace LocalStorage with S3Storage (or any other provider) without touching handlers.
type AudioStorage interface {
	Upload(ctx context.Context, filename string, r io.Reader, contentType string) (url string, err error)
}
