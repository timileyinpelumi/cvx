package ai

import (
	"fmt"
	"os"
)

const (
	groqBaseURL   = "https://api.groq.com/openai/v1"
	openAIBaseURL = "https://api.openai.com/v1"

	groqDefaultModel      = "openai/gpt-oss-120b"
	openAIDefaultModel    = "gpt-4.1"
	anthropicDefaultModel = "claude-opus-5"
)

// NewFromEnv builds the LLM implementation selected by CVX_LLM_PROVIDER
// (groq | openai | anthropic; default groq when unset/empty), with an
// optional CVX_LLM_MODEL override. It returns the constructed LLM, a
// "provider/model" string for logging, or an error naming the missing env
// var / unknown provider.
func NewFromEnv() (LLM, string, error) {
	provider := os.Getenv("CVX_LLM_PROVIDER")
	if provider == "" {
		provider = "groq"
	}
	modelOverride := os.Getenv("CVX_LLM_MODEL")

	switch provider {
	case "groq":
		key := os.Getenv("GROQ_API_KEY")
		if key == "" {
			return nil, "", fmt.Errorf("missing GROQ_API_KEY for provider groq")
		}
		model := groqDefaultModel
		if modelOverride != "" {
			model = modelOverride
		}
		return newOpenAICompat(groqBaseURL, key, model, false), "groq/" + model, nil

	case "openai":
		key := os.Getenv("OPENAI_API_KEY")
		if key == "" {
			return nil, "", fmt.Errorf("missing OPENAI_API_KEY for provider openai")
		}
		model := openAIDefaultModel
		if modelOverride != "" {
			model = modelOverride
		}
		return newOpenAICompat(openAIBaseURL, key, model, true), "openai/" + model, nil

	case "anthropic":
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return nil, "", fmt.Errorf("missing ANTHROPIC_API_KEY for provider anthropic")
		}
		model := anthropicDefaultModel
		if modelOverride != "" {
			model = modelOverride
		}
		return NewAnthropicWithModel(model), "anthropic/" + model, nil

	default:
		return nil, "", fmt.Errorf("unknown CVX_LLM_PROVIDER %q (want groq, openai, or anthropic)", provider)
	}
}
