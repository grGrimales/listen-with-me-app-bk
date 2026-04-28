package elevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"listen-with-me/backend/internal/tts"
)

const baseURL = "https://api.elevenlabs.io/v1"

type Provider struct {
	apiKey string
	client *http.Client
}

func New(apiKey string) *Provider {
	return &Provider{
		apiKey:  apiKey,
		client:  &http.Client{},
	}
}

func (p *Provider) GenerateAudio(ctx context.Context, text, voiceID, modelID string) (*tts.AudioResult, error) {
	payload := map[string]interface{}{
		"text":     text,
		"model_id": modelID,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/text-to-speech/%s", baseURL, voiceID),
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("elevenlabs error %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read audio response: %w", err)
	}

	return &tts.AudioResult{
		Data:        data,
		ContentType: "audio/mpeg",
	}, nil
}

// hardcoded list — ElevenLabs models change rarely; update here when needed
var availableModels = []tts.Model{
	{ID: "eleven_v3", Name: "Eleven v3"},
	{ID: "eleven_v3_turbo", Name: "Eleven v3 Turbo"},
	{ID: "eleven_multilingual_v2", Name: "Multilingual v2"},
	{ID: "eleven_turbo_v2_5", Name: "Turbo v2.5"},
	{ID: "eleven_turbo_v2", Name: "Turbo v2"},
	{ID: "eleven_flash_v2_5", Name: "Flash v2.5"},
	{ID: "eleven_monolingual_v1", Name: "Monolingual v1 (English only)"},
}

func (p *Provider) ListModels(_ context.Context) ([]tts.Model, error) {
	return availableModels, nil
}
