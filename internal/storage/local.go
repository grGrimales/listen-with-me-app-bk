package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalStorage saves files to the local filesystem and returns an HTTP URL.
// Set UPLOAD_DIR and SERVER_BASE_URL env vars to configure paths.
type LocalStorage struct {
	dir     string // absolute or relative path to the upload directory
	baseURL string // public base URL of the server, e.g. http://localhost:8082
}

func NewLocalStorage(dir, baseURL string) (*LocalStorage, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating upload dir %q: %w", dir, err)
	}
	return &LocalStorage{dir: dir, baseURL: baseURL}, nil
}

func (s *LocalStorage) Upload(_ context.Context, filename string, r io.Reader, _ string) (string, error) {
	dst := filepath.Join(s.dir, filename)
	f, err := os.Create(dst)
	if err != nil {
		return "", fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return "", fmt.Errorf("writing file: %w", err)
	}
	return fmt.Sprintf("%s/uploads/audio/%s", s.baseURL, filename), nil
}
