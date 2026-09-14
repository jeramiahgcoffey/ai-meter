package providers

import (
	"context"
	"time"

	"github.com/jeramiahgcoffey/ai-meter/internal/meter"
)

type Demo struct {
	InstanceID string
	Name       string
	Kind       string
	Spend      float64
	Budget     float64
	Input      float64
	Output     float64
	Requests   float64
	Status     meter.Status
	Limited    bool
	Models     []meter.ModelUsage
}

func (p Demo) ID() string { return p.InstanceID }
func (p Demo) Fetch(_ context.Context, period meter.Period) meter.Snapshot {
	snapshot := meter.Snapshot{
		ID: p.InstanceID, Provider: p.Kind, Label: p.Name, Period: period,
		ObservedAt: time.Now(), Status: p.Status,
		Spend: meter.KnownValue(p.Spend, "USD"), Budget: meter.KnownValue(p.Budget, "USD"),
		InputTokens: meter.KnownValue(p.Input, "tokens"), OutputTokens: meter.KnownValue(p.Output, "tokens"),
		Requests: meter.KnownValue(p.Requests, "requests"), Source: "Demo data",
		Models: p.Models,
	}
	if p.Limited {
		snapshot.Spend = meter.UnsupportedValue("Personal AI-credit data needs the GitHub adapter")
		snapshot.Budget = meter.UnsupportedValue("No budget loaded")
		snapshot.InputTokens = meter.UnsupportedValue("Not exposed by this demo account")
		snapshot.OutputTokens = meter.UnsupportedValue("Not exposed by this demo account")
		snapshot.Requests = meter.UnsupportedValue("Not exposed by this demo account")
		snapshot.Issues = []string{"This row demonstrates a provider with incomplete machine-readable data"}
	}
	return snapshot
}

func DemoProviders() []meter.Provider {
	return []meter.Provider{
		Demo{InstanceID: "anthropic-demo", Name: "Claude API", Kind: "anthropic", Spend: 38.44, Budget: 75, Input: 3_840_100, Output: 492_800, Requests: 1274, Status: meter.Fresh, Models: []meter.ModelUsage{
			{ID: "claude-fable-5-1", InputTokens: meter.KnownValue(2_610_000, "tokens"), OutputTokens: meter.KnownValue(341_000, "tokens"), Requests: meter.UnsupportedValue("not reported"), Spend: meter.UnsupportedValue("not attributed")},
			{ID: "claude-opus-5", InputTokens: meter.KnownValue(1_230_100, "tokens"), OutputTokens: meter.KnownValue(151_800, "tokens"), Requests: meter.UnsupportedValue("not reported"), Spend: meter.UnsupportedValue("not attributed")},
		}},
		Demo{InstanceID: "openai-demo", Name: "OpenAI API", Kind: "openai", Spend: 91.72, Budget: 120, Input: 8_910_400, Output: 1_240_300, Requests: 3128, Status: meter.Fresh, Models: []meter.ModelUsage{
			{ID: "gpt-5.6", InputTokens: meter.KnownValue(6_400_000, "tokens"), OutputTokens: meter.KnownValue(940_300, "tokens"), Requests: meter.KnownValue(2204, "requests"), Spend: meter.UnsupportedValue("not attributed")},
			{ID: "gpt-5.5-mini", InputTokens: meter.KnownValue(2_510_400, "tokens"), OutputTokens: meter.KnownValue(300_000, "tokens"), Requests: meter.KnownValue(924, "requests"), Spend: meter.UnsupportedValue("not attributed")},
		}},
		Demo{InstanceID: "copilot-demo", Name: "GitHub Copilot", Kind: "github", Status: meter.Partial, Limited: true},
	}
}
