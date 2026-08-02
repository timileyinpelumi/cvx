// Command cvxeval runs the cvx quality eval harness (server/eval) against a
// set of fixed JD fixtures and prints a scored report.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"cvx/eval"
	"cvx/internal/ai"
)

// resultsDir is where Report.Save writes JSON, relative to the working
// directory cvxeval is run from (server/, same convention as cmd/cvx's
// "data" dir) — independent of -fixtures so results always land in one
// place regardless of which fixtures directory was used.
const resultsDir = "eval/results"

const (
	defaultJudgeProvider = "groq"
	// defaultJudgeModel must support Groq's Structured Outputs (strict
	// json_schema response_format) — ai.LLM.GenerateJSON always requests
	// strict:true. As of the 2026-08-02 baseline run, Groq's own docs
	// (https://console.groq.com/docs/structured-outputs) list ONLY
	// openai/gpt-oss-20b and openai/gpt-oss-120b under "Models with Strict
	// Mode" — llama-3.3-70b-versatile is not on that list and rejected the
	// request outright. gpt-oss-20b is the only other model on it, so it's
	// the judge default despite sharing a "gpt-oss" lineage with the
	// generator's own default (gpt-oss-120b): it is still a materially
	// different, independently-weighted checkpoint, not the same model
	// grading itself. Revisit if Groq adds a genuinely different-family
	// model to that support list.
	defaultJudgeModel = "openai/gpt-oss-20b"
)

// loadDotEnv sets environment variables from KEY=VALUE lines in path,
// skipping blank lines and #-comments. It never overrides a variable that is
// already set, so real deployment env always wins over the .env file.
// Duplicated from cmd/cvx/main.go: main packages can't be imported, and
// there's no shared internal package to hang this on.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if _, set := os.LookupEnv(key); set {
			continue
		}
		os.Setenv(key, strings.TrimSpace(value))
	}
}

// splitCSV splits a comma-separated flag value into trimmed, non-empty
// parts, so an empty -ids flag produces nil (all fixtures) rather than a
// slice containing one empty string.
func splitCSV(v string) []string {
	if v == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func main() {
	label := flag.String("label", "", "name for this eval run (required)")
	idsFlag := flag.String("ids", "", "comma-separated fixture ids to run (default: all)")
	cover := flag.Bool("cover", false, "also generate and judge cover letters")
	fixturesDir := flag.String("fixtures", "eval/fixtures", "path to the fixtures directory")
	judgeProviderFlag := flag.String("judge-provider", "", "override judge provider (default: env CVX_EVAL_JUDGE_PROVIDER, or groq)")
	judgeModelFlag := flag.String("judge-model", "", "override judge model (default: env CVX_EVAL_JUDGE_MODEL, or openai/gpt-oss-20b)")
	flag.Parse()

	if *label == "" {
		fmt.Fprintln(os.Stderr, "cvxeval: -label is required")
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	loadDotEnv(".env")

	ids := splitCSV(*idsFlag)

	gen, genDesc, err := ai.NewFromEnv()
	if err != nil {
		slog.Error("generator llm", "err", err)
		os.Exit(1)
	}
	slog.Info("generator", "llm", genDesc)

	judgeProvider := *judgeProviderFlag
	if judgeProvider == "" {
		judgeProvider = os.Getenv("CVX_EVAL_JUDGE_PROVIDER")
	}
	if judgeProvider == "" {
		judgeProvider = defaultJudgeProvider
	}
	judgeModel := *judgeModelFlag
	if judgeModel == "" {
		judgeModel = os.Getenv("CVX_EVAL_JUDGE_MODEL")
	}
	if judgeModel == "" {
		judgeModel = defaultJudgeModel
	}
	judge, err := ai.NewWithProviderModel(judgeProvider, judgeModel)
	if err != nil {
		slog.Error("judge llm", "err", err)
		os.Exit(1)
	}
	slog.Info("judge", "provider", judgeProvider, "model", judgeModel)

	report, err := eval.Run(context.Background(), gen, judge, *fixturesDir, ids, *cover)
	if err != nil {
		slog.Error("eval run", "err", err)
		os.Exit(1)
	}
	report.Label = *label

	if err := report.Render(os.Stdout); err != nil {
		slog.Error("render report", "err", err)
		os.Exit(1)
	}

	path, err := report.Save(resultsDir)
	if err != nil {
		slog.Error("save report", "err", err)
		os.Exit(1)
	}
	fmt.Println(path)
}
