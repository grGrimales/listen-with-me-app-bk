package tts

import "context"

// Provider abstracts a text-to-speech backend.
// Swap ElevenLabs for any other provider by implementing this interface.
type Provider interface {
	GenerateAudio(ctx context.Context, text, voiceID, modelID string) (*AudioResult, error)
	ListModels(ctx context.Context) ([]Model, error)
}

type AudioResult struct {
	Data        []byte
	ContentType string
}

type Model struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
