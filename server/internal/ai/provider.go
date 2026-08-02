package ai

import (
	"fmt"
	"os"
)

const (
	groqBaseURL        = "https://api.groq.com/openai/v1"
	openAIBaseURL      = "https://api.openai.com/v1"
	agentRouterBaseURL = "https://agentrouter.org/v1"

	groqDefaultModel        = "openai/gpt-oss-120b"
	openAIDefaultModel      = "gpt-4.1"
	anthropicDefaultModel   = "claude-opus-5"
	agentRouterDefaultModel = "claude-opus-5"
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
		return newOpenAICompat(groqBaseURL, key, model, false, maxTokensFieldLegacy), "groq/" + model, nil

	case "openai":
		key := os.Getenv("OPENAI_API_KEY")
		if key == "" {
			return nil, "", fmt.Errorf("missing OPENAI_API_KEY for provider openai")
		}
		model := openAIDefaultModel
		if modelOverride != "" {
			model = modelOverride
		}
		return newOpenAICompat(openAIBaseURL, key, model, true, maxTokensFieldModern), "openai/" + model, nil

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

	case "agentrouter":
		key := os.Getenv("AGENTROUTER_API_KEY")
		if key == "" {
			return nil, "", fmt.Errorf("missing AGENTROUTER_API_KEY for provider agentrouter")
		}
		model := agentRouterDefaultModel
		if modelOverride != "" {
			model = modelOverride
		}
		// Claude-family backends behind OpenAI-compatible routers take the
		// classic max_tokens field, and PDFs go in as extracted text.
		return newOpenAICompat(agentRouterBaseURL, key, model, false, maxTokensFieldLegacy), "agentrouter/" + model, nil

	default:
		return nil, "", fmt.Errorf("unknown CVX_LLM_PROVIDER %q (want groq, openai, anthropic, or agentrouter)", provider)
	}
}

// NewWithProviderModel builds the LLM implementation for an explicit
// provider/model pair, independent of CVX_LLM_PROVIDER/CVX_LLM_MODEL. Used by
// cvxeval to construct a judge LLM that can be a different provider/model
// than the generator under test, while still reading each provider's API key
// from its usual env var.
func NewWithProviderModel(provider, model string) (LLM, error) {
	switch provider {
	case "groq":
		key := os.Getenv("GROQ_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("missing GROQ_API_KEY for provider groq")
		}
		return newOpenAICompat(groqBaseURL, key, model, false, maxTokensFieldLegacy), nil

	case "openai":
		key := os.Getenv("OPENAI_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("missing OPENAI_API_KEY for provider openai")
		}
		return newOpenAICompat(openAIBaseURL, key, model, true, maxTokensFieldModern), nil

	case "anthropic":
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("missing ANTHROPIC_API_KEY for provider anthropic")
		}
		return NewAnthropicWithModel(model), nil

	case "agentrouter":
		key := os.Getenv("AGENTROUTER_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("missing AGENTROUTER_API_KEY for provider agentrouter")
		}
		return newOpenAICompat(agentRouterBaseURL, key, model, false, maxTokensFieldLegacy), nil

	default:
		return nil, fmt.Errorf("unknown provider %q (want groq, openai, anthropic, or agentrouter)", provider)
	}
}
