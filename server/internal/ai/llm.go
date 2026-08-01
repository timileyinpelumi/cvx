package ai

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// LLM is the seam tests fake. Real impl wraps anthropic-sdk-go.
type LLM interface {
	// GenerateJSON sends one user turn (blocks) + optional system prompt,
	// constrained to schema, and returns the raw JSON text.
	GenerateJSON(ctx context.Context, system string, blocks []ContentBlock, schema map[string]any) ([]byte, error)
}

// ContentBlock is a single user-turn content block; exactly one of Text or PDF is set.
type ContentBlock struct {
	Text string
	PDF  []byte
}

type anthropicLLM struct {
	client anthropic.Client
}

// NewAnthropic constructs a real LLM backed by the Anthropic API.
// The API key is read from ANTHROPIC_API_KEY via the SDK's default client options.
func NewAnthropic() LLM {
	return anthropicLLM{client: anthropic.NewClient()}
}

func (a anthropicLLM) GenerateJSON(ctx context.Context, system string, blocks []ContentBlock, schema map[string]any) ([]byte, error) {
	var content []anthropic.ContentBlockParamUnion
	for _, b := range blocks {
		if b.PDF != nil {
			content = append(content, anthropic.NewDocumentBlock(anthropic.Base64PDFSourceParam{
				Data: base64.StdEncoding.EncodeToString(b.PDF)}))
		} else {
			content = append(content, anthropic.NewTextBlock(b.Text))
		}
	}
	params := anthropic.MessageNewParams{
		Model:     "claude-opus-5",
		MaxTokens: 16000,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(content...)},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffortMedium,
			Format: anthropic.JSONOutputFormatParam{Schema: schema},
		},
	}
	if system != "" {
		params.System = []anthropic.TextBlockParam{{Text: system}}
	}
	resp, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return nil, err
	}
	for _, block := range resp.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			return []byte(tb.Text), nil
		}
	}
	return nil, fmt.Errorf("no text block in response (stop_reason=%s)", resp.StopReason)
}
