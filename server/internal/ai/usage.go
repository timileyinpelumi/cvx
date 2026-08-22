package ai

import "context"

// Usage is what one model call consumed. Providers report it in the
// response and the client used to throw it away, which made the per-compose
// cost of a five-call pipeline unknowable.
type Usage struct {
	Provider     string
	Model        string
	PromptTokens int
	OutputTokens int
	TotalTokens  int
}

// Cost estimates what a call cost in US dollars, using the published per
// million token rates below. An estimate on the panel beats no number at
// all; the rates are a constant to update, not a billing integration.
func (u Usage) Cost() float64 {
	rate, ok := modelRates[u.Model]
	if !ok {
		return 0
	}
	return (float64(u.PromptTokens)*rate.in + float64(u.OutputTokens)*rate.out) / 1_000_000
}

type rate struct{ in, out float64 }

// Published rates, US dollars per million tokens. Unknown models cost 0
// rather than a guess, and the panel shows the call count regardless.
var modelRates = map[string]rate{
	"openai/gpt-oss-120b":       {in: 0.15, out: 0.60},
	"openai/gpt-oss-20b":        {in: 0.075, out: 0.30},
	"llama-3.3-70b-versatile":   {in: 0.59, out: 0.79},
	"gpt-4o-mini":               {in: 0.15, out: 0.60},
	"gpt-4o":                    {in: 2.50, out: 10.00},
	"claude-sonnet-4-5":         {in: 3.00, out: 15.00},
	"claude-haiku-4-5-20251001": {in: 1.00, out: 5.00},
}

// UsageSink receives the usage of every model call. Set once at startup;
// nil means nobody is listening and the client does no extra work.
type UsageSink func(ctx context.Context, u Usage, ms int64, err error)

// usageSink is a package variable rather than a field on each client
// because every provider constructor would otherwise have to thread it, and
// there is exactly one sink per process.
var usageSink UsageSink

// SetUsageSink installs the recorder. Safe to leave unset in tests.
func SetUsageSink(sink UsageSink) { usageSink = sink }

func reportUsage(ctx context.Context, u Usage, ms int64, err error) {
	if usageSink != nil {
		usageSink(ctx, u, ms, err)
	}
}
