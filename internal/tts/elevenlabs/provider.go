package elevenlabs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
	"unicode"

	"listen-with-me/backend/internal/tts"
)

const baseURL = "https://api.elevenlabs.io/v1"

type Provider struct {
	apiKey string
	client *http.Client
}

func New(apiKey string) *Provider {
	return &Provider{
		apiKey: apiKey,
		client: &http.Client{Timeout: 120 * time.Second},
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

// AlignedWord is a single word with its ms start/end in the generated audio.
type AlignedWord struct {
	Word    string `json:"word"`
	StartMs int    `json:"start_ms"`
	EndMs   int    `json:"end_ms"`
}

// AudioWithAlignment holds MP3 bytes + word-level timings for a single generation.
type AudioWithAlignment struct {
	Data        []byte
	ContentType string
	Words       []AlignedWord
}

type withTimestampsResponse struct {
	AudioBase64 string `json:"audio_base64"`
	Alignment   struct {
		Characters               []string  `json:"characters"`
		CharacterStartTimesSec   []float64 `json:"character_start_times_seconds"`
		CharacterEndTimesSec     []float64 `json:"character_end_times_seconds"`
	} `json:"alignment"`
}

// GenerateAudioWithTimestamps requests audio + character-level alignment and groups
// characters into words (splitting on whitespace).
func (p *Provider) GenerateAudioWithTimestamps(ctx context.Context, text, voiceID, modelID string) (*AudioWithAlignment, error) {
	payload := map[string]interface{}{
		"text":     text,
		"model_id": modelID,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/text-to-speech/%s/with-timestamps", baseURL, voiceID),
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", p.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("elevenlabs error %d: %s", resp.StatusCode, string(body))
	}

	var parsed withTimestampsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	audioBytes, err := base64.StdEncoding.DecodeString(parsed.AudioBase64)
	if err != nil {
		return nil, fmt.Errorf("decode audio: %w", err)
	}

	words := groupCharsIntoWords(
		parsed.Alignment.Characters,
		parsed.Alignment.CharacterStartTimesSec,
		parsed.Alignment.CharacterEndTimesSec,
	)

	return &AudioWithAlignment{
		Data:        audioBytes,
		ContentType: "audio/mpeg",
		Words:       words,
	}, nil
}

// groupCharsIntoWords walks characters and produces one AlignedWord per whitespace-delimited token.
// Punctuation stays glued to the adjacent word so the reconstructed sequence matches the source text.
func groupCharsIntoWords(chars []string, startsSec, endsSec []float64) []AlignedWord {
	n := len(chars)
	if n == 0 || len(startsSec) != n || len(endsSec) != n {
		return nil
	}

	var words []AlignedWord
	var cur AlignedWord
	inWord := false

	flush := func() {
		if inWord && cur.Word != "" {
			words = append(words, cur)
		}
		cur = AlignedWord{}
		inWord = false
	}

	for i := 0; i < n; i++ {
		ch := chars[i]
		isSpace := ch == "" || (len(ch) == 1 && unicode.IsSpace(rune(ch[0])))
		if isSpace {
			flush()
			continue
		}
		if !inWord {
			cur.StartMs = int(startsSec[i] * 1000)
			inWord = true
		}
		cur.Word += ch
		cur.EndMs = int(endsSec[i] * 1000)
	}
	flush()
	return words
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
