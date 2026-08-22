package ai

import (
	"bytes"
	"context"
	"cvx/internal/pdftext"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// maxOutputTokens bounds a single completion, matching the budget already
// used for the Anthropic path (see llm.go's MaxTokens: 16000). Without an
// explicit cap, Groq falls back to a small default that truncates a
// multi-field JSON response mid-object — the response then fails schema
// validation with json_validate_failed instead of a clear "too short" error.
const maxOutputTokens = 16000

// The output-token-budget field is named differently across providers:
// maxTokensFieldLegacy ("max_tokens") is accepted by both Groq and OpenAI
// but is deprecated on both in favor of maxTokensFieldModern
// ("max_completion_tokens") — and on OpenAI, reasoning models (o1/o3/...)
// reject max_tokens outright, so any provider whose model lineup might
// include one of those must use the modern field.
const (
	maxTokensFieldLegacy = "max_tokens"
	maxTokensFieldModern = "max_completion_tokens"
)

// openAICompat is an LLM backed by any OpenAI-chat-completions-compatible
// endpoint. It serves both OpenAI and Groq — they differ only in baseURL,
// key, model, whether the endpoint accepts a PDF file part natively, and
// which JSON field name carries the output-token budget.
type openAICompat struct {
	baseURL        string
	apiKey         string
	model          string
	pdfNative      bool
	maxTokensField string
	hc             *http.Client
}

func newOpenAICompat(baseURL, apiKey, model string, pdfNative bool, maxTokensField string) *openAICompat {
	return &openAICompat{
		baseURL:        baseURL,
		apiKey:         apiKey,
		model:          model,
		pdfNative:      pdfNative,
		maxTokensField: maxTokensField,
		hc:             &http.Client{Timeout: 120 * time.Second},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type chatRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	ResponseFormat responseFormat `json:"response_format"`
	// Exactly one of these is set, per c.maxTokensField.
	MaxTokens           int `json:"max_tokens,omitempty"`
	MaxCompletionTokens int `json:"max_completion_tokens,omitempty"`
}

type responseFormat struct {
	Type       string     `json:"type"`
	JSONSchema jsonSchema `json:"json_schema"`
}

type jsonSchema struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *openAICompat) GenerateJSON(ctx context.Context, system string, blocks []ContentBlock, schema map[string]any) ([]byte, error) {
	var parts []map[string]any
	for _, b := range blocks {
		if b.PDF != nil {
			if c.pdfNative {
				parts = append(parts, map[string]any{
					"type": "file",
					"file": map[string]any{
						"filename":  "resume.pdf",
						"file_data": "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(b.PDF),
					},
				})
			} else {
				text, err := pdftext.ExtractText(b.PDF)
				if err != nil {
					return nil, fmt.Errorf("openaicompat: %w", err)
				}
				parts = append(parts, map[string]any{
					"type": "text",
					"text": "RESUME TEXT (extracted from PDF):\n" + text,
				})
			}
		} else {
			parts = append(parts, map[string]any{
				"type": "text",
				"text": b.Text,
			})
		}
	}

	var messages []chatMessage
	if system != "" {
		messages = append(messages, chatMessage{Role: "system", Content: system})
	}
	messages = append(messages, chatMessage{Role: "user", Content: parts})

	reqBody := chatRequest{
		Model:    c.model,
		Messages: messages,
		ResponseFormat: responseFormat{
			Type: "json_schema",
			JSONSchema: jsonSchema{
				Name:   "result",
				Strict: true,
				Schema: schema,
			},
		},
	}
	if c.maxTokensField == maxTokensFieldModern {
		reqBody.MaxCompletionTokens = maxOutputTokens
	} else {
		reqBody.MaxTokens = maxOutputTokens
	}

	buf, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("openaicompat: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := string(respBytes)
		if len(body) > 2000 {
			body = body[:2000]
		}
		return nil, fmt.Errorf("openaicompat: status %d: %s", resp.StatusCode, body)
	}

	var cr chatResponse
	if err := json.Unmarshal(respBytes, &cr); err != nil {
		return nil, fmt.Errorf("openaicompat: unmarshal response: %w", err)
	}

	if len(cr.Choices) == 0 || cr.Choices[0].Message.Content == "" {
		if cr.Error != nil && cr.Error.Message != "" {
			return nil, fmt.Errorf("openaicompat: empty response: %s", cr.Error.Message)
		}
		return nil, fmt.Errorf("openaicompat: empty response (no choices)")
	}

	return []byte(cr.Choices[0].Message.Content), nil
}
