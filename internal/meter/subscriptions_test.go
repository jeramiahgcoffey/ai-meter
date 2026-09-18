package meter

import (
	"testing"
	"time"
)

func TestZaiPlanLimitsAppearInSummaries(t *testing.T) {
	snapshot := Snapshot{Provider: "claude", Label: "GLM", UsageWindows: []UsageWindow{
		{Label: "5h", Scope: "GLM", WindowMinutes: 300, AvailablePercent: 94},
		{Label: "7d", Scope: "GLM", WindowMinutes: 10080, AvailablePercent: 99},
		{Label: "7d", Scope: "Mystery", LimitID: "third-party", WindowMinutes: 10080, AvailablePercent: 10},
	}}

	applicable := ApplicableSubscriptionLimits(snapshot)
	if len(applicable) != 2 || applicable[0].Label != "5h" || applicable[0].Scope != "GLM" || applicable[1].Label != "7d" {
		t.Fatalf("GLM applicable limits = %+v", applicable)
	}
	summary := SubscriptionSummaryLimits(snapshot)
	if len(summary) != 1 || summary[0].Label != "7d" || summary[0].Scope != "GLM" {
		t.Fatalf("GLM summary limits = %+v", summary)
	}
}

func TestActiveSubscriptionsSelectApplicableLimits(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-2 * time.Hour)
	stale := now.Add(-6 * time.Hour)
	dashboard := Dashboard{Providers: []Snapshot{
		{
			ID: "claude-local", Provider: "claude", Label: "Claude", LastActivityAt: &recent,
			UsageWindows: []UsageWindow{
				{Label: "5h", Scope: "Claude", WindowMinutes: 300, AvailablePercent: 72},
				{Label: "7d", Scope: "Claude", WindowMinutes: 10080, AvailablePercent: 54},
				{Label: "7d", Scope: "Fable", LimitID: "claude:model:fable", WindowMinutes: 10080, AvailablePercent: 18},
			},
		},
		{
			ID: "codex-local", Provider: "codex", Label: "Codex", LastActivityAt: &recent,
			UsageWindows: []UsageWindow{
				{Label: "5h", Scope: "GPT-5.3-Codex-Spark", LimitID: "codex_spark", WindowMinutes: 300, AvailablePercent: 10},
				{Label: "7d", Scope: "GPT-5.3-Codex-Spark", LimitID: "codex_spark", WindowMinutes: 10080, AvailablePercent: 8},
				{Label: "7d", Scope: "codex", LimitID: "codex", WindowMinutes: 10080, AvailablePercent: 64},
			},
		},
		{ID: "claude-old", Provider: "claude", Label: "Old Claude", LastActivityAt: &stale},
		{ID: "openai", Provider: "openai", Label: "OpenAI", LastActivityAt: &recent},
	}}

	accounts := ActiveSubscriptions(dashboard, now, 5*time.Hour)
	if len(accounts) != 2 {
		t.Fatalf("active subscriptions = %+v", accounts)
	}
	if got := accounts[0].Limits; len(got) != 2 || got[0].Label != "5h" || got[1].Scope != "Fable" {
		t.Fatalf("Claude applicable limits = %+v", got)
	}
	if got := accounts[1].Limits; len(got) != 1 || got[0].LimitID != "codex" {
		t.Fatalf("Codex applicable limits = %+v", got)
	}
}

func TestActiveSubscriptionsIncludesBoundaryAndMissingLimits(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	boundary := now.Add(-5 * time.Hour)
	future := now.Add(time.Minute)
	dashboard := Dashboard{Providers: []Snapshot{
		{ID: "boundary", Provider: "claude", LastActivityAt: &boundary},
		{ID: "future", Provider: "codex", LastActivityAt: &future},
		{ID: "unknown", Provider: "claude"},
	}}

	accounts := ActiveSubscriptions(dashboard, now, 5*time.Hour)
	if len(accounts) != 1 || accounts[0].Snapshot.ID != "boundary" || len(accounts[0].Limits) != 0 {
		t.Fatalf("boundary activity = %+v", accounts)
	}
}

func TestSubscriptionSummaryLimitsUseSevenDayWindows(t *testing.T) {
	claude := Snapshot{Provider: "claude", UsageWindows: []UsageWindow{
		{Label: "5h", Scope: "Claude", WindowMinutes: 300},
		{Label: "7d", Scope: "Claude", WindowMinutes: 10080, AvailablePercent: 54},
		{Label: "7d", Scope: "Fable", LimitID: "claude:model:fable", WindowMinutes: 10080, AvailablePercent: 18},
	}}
	limits := SubscriptionSummaryLimits(claude)
	if len(limits) != 2 || limits[0].Scope != "Claude" || limits[1].Scope != "Fable" {
		t.Fatalf("Claude selector limits = %+v", limits)
	}

	codex := Snapshot{Provider: "codex", UsageWindows: []UsageWindow{
		{Label: "5h", Scope: "GPT-5.3-Codex-Spark", LimitID: "codex_spark", WindowMinutes: 300},
		{Label: "7d", Scope: "GPT-5.3-Codex-Spark", LimitID: "codex_spark", WindowMinutes: 10080},
		{Label: "7d", Scope: "codex", LimitID: "codex", WindowMinutes: 10080, AvailablePercent: 64},
	}}
	limits = SubscriptionSummaryLimits(codex)
	if len(limits) != 1 || limits[0].LimitID != "codex" {
		t.Fatalf("Codex selector limits = %+v", limits)
	}
}
