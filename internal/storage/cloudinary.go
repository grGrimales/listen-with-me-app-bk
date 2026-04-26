package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
)

type CloudinaryStorage struct {
	cl *cloudinary.Cloudinary
}

func NewCloudinaryStorage(cloudinaryURL string) (*CloudinaryStorage, error) {
	cld, err := cloudinary.NewFromURL(cloudinaryURL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cloudinary: %w", err)
	}
	return &CloudinaryStorage{cl: cld}, nil
}

func (s *CloudinaryStorage) Upload(ctx context.Context, filename string, r io.Reader, _ string) (string, error) {
	// Use filename as PublicID but remove extension as Cloudinary adds it back
	publicID := strings.TrimSuffix(filename, filepathExt(filename))
	
	resp, err := s.cl.Upload.Upload(ctx, r, uploader.UploadParams{
		PublicID:     publicID,
		ResourceType: "auto", // Automatically detect if it's image or audio (video)
	})
	if err != nil {
		return "", fmt.Errorf("cloudinary upload error: %w", err)
	}

	return resp.SecureURL, nil
}

func (s *CloudinaryStorage) Delete(ctx context.Context, fileURL string) error {
	if fileURL == "" {
		return nil
	}

	// Extract public_id from Cloudinary URL
	publicID := extractPublicID(fileURL)
	if publicID == "" {
		return nil
	}

	resourceType := "image"
	if strings.Contains(fileURL, "/video/") || strings.Contains(fileURL, "/audio/") {
		resourceType = "video"
	}

	_, err := s.cl.Upload.Destroy(ctx, uploader.DestroyParams{
		PublicID:     publicID,
		ResourceType: resourceType,
	})
	return err
}

// Helpers

func filepathExt(path string) string {
	for i := len(path) - 1; i >= 0 && path[i] != '/'; i-- {
		if path[i] == '.' {
			return path[i:]
		}
	}
	return ""
}

func extractPublicID(url string) string {
	// Simplified extraction logic for standard Cloudinary URLs
	// Find /upload/ section
	parts := strings.Split(url, "/upload/")
	if len(parts) < 2 {
		return ""
	}
	
	// Remove version (v12345/) if present
	afterUpload := parts[1]
	if strings.HasPrefix(afterUpload, "v") {
		firstSlash := strings.Index(afterUpload, "/")
		if firstSlash != -1 {
			afterUpload = afterUpload[firstSlash+1:]
		}
	}
	
	// Remove extension
	lastDot := strings.LastIndex(afterUpload, ".")
	if lastDot != -1 {
		return afterUpload[:lastDot]
	}
	return afterUpload
}
