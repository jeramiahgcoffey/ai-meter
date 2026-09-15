package providers

import (
	"context"
	"time"

	"github.com/jeramiahgcoffey/ai-meter/internal/meter"
)

type Demo struct {
	InstanceID   string
	Name         string
	Kind         string
	Spend        float64
	Budget       float64
	Input        float64
	Output       float64
	Requests     float64
	Status       meter.Status
	Limited      bool
	Subscription bool
	ActiveAgo    time.Duration
	UsageWindows []meter.UsageWindow
	Models       []meter.ModelUsage
}

func (p Demo) ID() string { return p.InstanceID }
func (p Demo) Fetch(_ context.Context, period meter.Period) meter.Snapshot {
	snapshot := meter.Snapshot{
		ID: p.InstanceID, Provider: p.Kind, Label: p.Name, Period: period,
		ObservedAt: time.Now(), Status: p.Status,
		Spend: meter.KnownValue(p.Spend, "USD"), Budget: meter.KnownValue(p.Budget, "USD"),
		InputTokens: meter.KnownValue(p.Input, "tokens"), OutputTokens: meter.KnownValue(p.Output, "tokens"),
		Requests: meter.KnownValue(p.Requests, "requests"), Source: "Demo data",
		Models: p.Models, UsageWindows: p.UsageWindows,
	}
	if p.Subscription {
		lastActivity := time.Now().Add(-p.ActiveAgo)
		snapshot.LastActivityAt = &lastActivity
		snapshot.Spend = meter.UnsupportedValue("local subscription usage does not expose invoice spend")
		snapshot.Budget = meter.UnsupportedValue("subscription limits are shown separately")
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
	now := time.Now()
	return []meter.Provider{
		Demo{InstanceID: "claude-local", Name: "Claude", Kind: "claude", Subscription: true, ActiveAgo: 34 * time.Minute, Input: 2_840_100, Output: 392_800, Requests: 874, Status: meter.Fresh, UsageWindows: []meter.UsageWindow{
			{Label: "5h", Scope: "Claude", WindowMinutes: 300, AvailablePercent: 68, UsedPercent: 32, ResetsAt: now.Add(2*time.Hour + 41*time.Minute), ObservedAt: now},
			{Label: "7d", Scope: "Claude", WindowMinutes: 10080, AvailablePercent: 44, UsedPercent: 56, ResetsAt: now.Add(3*24*time.Hour + 6*time.Hour), ObservedAt: now},
			{Label: "7d", Scope: "Fable", LimitID: "claude:model:fable", WindowMinutes: 10080, AvailablePercent: 18, UsedPercent: 82, ResetsAt: now.Add(3*24*time.Hour + 6*time.Hour), ObservedAt: now},
		}},
		Demo{InstanceID: "codex-local", Name: "Codex", Kind: "codex", Subscription: true, ActiveAgo: 71 * time.Minute, Input: 5_120_400, Output: 740_300, Requests: 1328, Status: meter.Fresh, UsageWindows: []meter.UsageWindow{
			{Label: "5h", Scope: "GPT-5.3-Codex-Spark", LimitID: "codex_spark", WindowMinutes: 300, AvailablePercent: 72, UsedPercent: 28, ResetsAt: now.Add(3*time.Hour + 9*time.Minute), ObservedAt: now},
			{Label: "7d", Scope: "codex", LimitID: "codex", WindowMinutes: 10080, AvailablePercent: 8, UsedPercent: 92, ResetsAt: now.Add(19*time.Hour + 22*time.Minute), ObservedAt: now},
		}},
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
