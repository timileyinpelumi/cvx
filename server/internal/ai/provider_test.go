package ai

import (
	"strings"
	"testing"
)

func clearLLMEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"CVX_LLM_PROVIDER", "CVX_LLM_MODEL", "GROQ_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY"} {
		t.Setenv(k, "")
	}
}

func TestNewFromEnvDefaultGroq(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("GROQ_API_KEY", "gk")

	llm, desc, err := NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv: %v", err)
	}
	if llm == nil {
		t.Fatal("want non-nil LLM")
	}
	if desc != "groq/openai/gpt-oss-120b" {
		t.Fatalf("want groq/openai/gpt-oss-120b, got %q", desc)
	}
}

func TestNewFromEnvMissingKeyNamesVar(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("CVX_LLM_PROVIDER", "groq")

	if _, _, err := NewFromEnv(); err == nil || !strings.Contains(err.Error(), "GROQ_API_KEY") {
		t.Fatalf("want error naming GROQ_API_KEY, got %v", err)
	}
}

func TestNewFromEnvUnknownProvider(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("CVX_LLM_PROVIDER", "bogus")

	_, _, err := NewFromEnv()
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	want := `unknown CVX_LLM_PROVIDER "bogus" (want groq, openai, or anthropic)`
	if err.Error() != want {
		t.Fatalf("want %q, got %q", want, err.Error())
	}
}

func TestNewFromEnvModelOverride(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("CVX_LLM_PROVIDER", "groq")
	t.Setenv("GROQ_API_KEY", "gk")
	t.Setenv("CVX_LLM_MODEL", "custom-model")

	_, desc, err := NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv: %v", err)
	}
	if desc != "groq/custom-model" {
		t.Fatalf("want groq/custom-model, got %q", desc)
	}
}

func TestNewFromEnvOpenAI(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("CVX_LLM_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "ok")

	llm, desc, err := NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv: %v", err)
	}
	if llm == nil {
		t.Fatal("want non-nil LLM")
	}
	if desc != "openai/gpt-4.1" {
		t.Fatalf("want openai/gpt-4.1, got %q", desc)
	}
}

func TestNewFromEnvOpenAIMissingKey(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("CVX_LLM_PROVIDER", "openai")

	if _, _, err := NewFromEnv(); err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Fatalf("want error naming OPENAI_API_KEY, got %v", err)
	}
}

func TestNewFromEnvAnthropic(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("CVX_LLM_PROVIDER", "anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "ak")

	llm, desc, err := NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv: %v", err)
	}
	if llm == nil {
		t.Fatal("want non-nil LLM")
	}
	if desc != "anthropic/claude-opus-5" {
		t.Fatalf("want anthropic/claude-opus-5, got %q", desc)
	}
}

func TestNewFromEnvAnthropicMissingKey(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("CVX_LLM_PROVIDER", "anthropic")

	if _, _, err := NewFromEnv(); err == nil || !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Fatalf("want error naming ANTHROPIC_API_KEY, got %v", err)
	}
}
