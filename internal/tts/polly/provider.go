package polly

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/polly"
	"github.com/aws/aws-sdk-go-v2/service/polly/types"
)

// Provider wraps Amazon Polly. Uses default AWS credential chain
// (env vars: AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY / AWS_REGION).
type Provider struct {
	client *polly.Client
}

// New builds a Polly client. Returns nil client if AWS config cannot be loaded
// so the app can still start without AWS creds — the handler will surface the
// error only when the user tries to generate audio.
func New(ctx context.Context) (*Provider, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("polly: load aws config: %w", err)
	}
	return &Provider{client: polly.NewFromConfig(cfg)}, nil
}

// VoiceFor picks the best Neural voice per (language, gender).
// Curated for language learning: clear, textbook-standard accents.
// See: https://docs.aws.amazon.com/polly/latest/dg/voicelist.html
func VoiceFor(language, gender string) string {
	switch language {
	case "pt":
		if gender == "male" {
			return "Thiago" // pt-BR Neural (male)
		}
		return "Camila" // pt-BR Neural (female)
	case "en":
		fallthrough
	default:
		if gender == "male" {
			return "Matthew" // en-US Neural (male)
		}
		return "Joanna" // en-US Neural (female)
	}
}

// Synthesize turns text into MP3 audio using a Neural voice picked from (language, gender).
// gender must be "female" or "male" — anything else is treated as "female".
func (p *Provider) Synthesize(ctx context.Context, text, language, gender string) ([]byte, string, error) {
	if p == nil || p.client == nil {
		return nil, "", fmt.Errorf("polly provider not initialized (missing AWS credentials?)")
	}
	voice := VoiceFor(language, gender)
	out, err := p.client.SynthesizeSpeech(ctx, &polly.SynthesizeSpeechInput{
		Text:         aws.String(text),
		OutputFormat: types.OutputFormatMp3,
		VoiceId:      types.VoiceId(voice),
		Engine:       types.EngineNeural,
	})
	if err != nil {
		return nil, "", fmt.Errorf("polly synthesize: %w", err)
	}
	defer out.AudioStream.Close()
	data, err := io.ReadAll(out.AudioStream)
	if err != nil {
		return nil, "", fmt.Errorf("polly read stream: %w", err)
	}
	return data, "audio/mpeg", nil
}
