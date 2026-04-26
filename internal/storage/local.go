package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	
	// Ensure parent directory exists (e.g., uploads/images or uploads/audio)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return "", fmt.Errorf("creating parent directory: %w", err)
	}

	f, err := os.Create(dst)
	if err != nil {
		return "", fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return "", fmt.Errorf("writing file: %w", err)
	}
	return fmt.Sprintf("%s/uploads/%s", s.baseURL, filename), nil
}

func (s *LocalStorage) Delete(_ context.Context, fileURL string) error {
	if fileURL == "" {
		return nil
	}

	// Extract filename from URL (e.g., http://localhost:8082/uploads/audio/myfile.mp3 -> myfile.mp3)
	parts := strings.Split(fileURL, "/")
	if len(parts) == 0 {
		return nil
	}
	filename := parts[len(parts)-1]

	filePath := filepath.Join(s.dir, filename)

	// Check if file exists before trying to delete
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil
	}

	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("deleting file %q: %w", filePath, err)
	}

	return nil
}
