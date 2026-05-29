package google

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"listen-with-me/backend/internal/tts"
)

const apiBase = "https://texttospeech.googleapis.com/v1/text:synthesize"

type Provider struct {
	apiKey string
	client *http.Client
}

func New(apiKey string) *Provider {
	return &Provider{apiKey: apiKey, client: &http.Client{}}
}

func (p *Provider) GenerateAudio(ctx context.Context, text, voiceID, _ string) (*tts.AudioResult, error) {
	// Extract language code from voice name: "en-US-Neural2-C" → "en-US"
	parts := strings.SplitN(voiceID, "-", 3)
	langCode := "en-US"
	if len(parts) >= 2 {
		langCode = parts[0] + "-" + parts[1]
	}

	body, _ := json.Marshal(map[string]any{
		"input": map[string]string{"text": text},
		"voice": map[string]string{
			"languageCode": langCode,
			"name":         voiceID,
		},
		"audioConfig": map[string]string{"audioEncoding": "MP3"},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"?key="+p.apiKey, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		AudioContent string `json:"audioContent"`
		Error        *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, fmt.Errorf("google tts: %s", result.Error.Message)
	}

	audio, err := base64.StdEncoding.DecodeString(result.AudioContent)
	if err != nil {
		return nil, fmt.Errorf("google tts: failed to decode audio: %w", err)
	}

	return &tts.AudioResult{Data: audio, ContentType: "audio/mpeg"}, nil
}

func (p *Provider) ListModels(_ context.Context) ([]tts.Model, error) {
	return []tts.Model{
		{ID: "neural2", Name: "Neural2"},
		{ID: "wavenet", Name: "WaveNet"},
		{ID: "standard", Name: "Standard"},
	}, nil
}
