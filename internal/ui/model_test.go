package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jeramiahgcoffey/ai-meter/internal/meter"
)

func TestWideViewShowsBudgetRailAndDetails(t *testing.T) {
	now := time.Now()
	dashboard := meter.Dashboard{
		GeneratedAt: now,
		Period:      meter.Period{Start: now.AddDate(0, 0, -7), End: now},
		Providers: []meter.Snapshot{{
			ID: "openai", Label: "OpenAI", Status: meter.Fresh, ObservedAt: now,
			Spend: meter.KnownValue(40, "USD"), Budget: meter.KnownValue(100, "USD"),
			InputTokens: meter.KnownValue(1000, "tokens"), OutputTokens: meter.KnownValue(200, "tokens"),
			Requests: meter.KnownValue(4, "requests"), Source: "fixture",
			Models: []meter.ModelUsage{{ID: "claude-fable-5-1", InputTokens: meter.KnownValue(800, "tokens"), OutputTokens: meter.KnownValue(120, "tokens"), Requests: meter.UnsupportedValue("not reported")}},
		}},
	}
	model := New(dashboard, nil)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
	view := updated.(Model).View()
	for _, want := range []string{"ai meter", "OpenAI", "40%", "Spend", "Input", "claude-fable-5-1", "r refresh"} {
		if !strings.Contains(view, want) {
			t.Errorf("view does not contain %q", want)
		}
	}
}

func TestNarrowViewOpensModelDetails(t *testing.T) {
	now := time.Now()
	dashboard := meter.Dashboard{GeneratedAt: now, Period: meter.Period{Start: now, End: now}, Providers: []meter.Snapshot{{
		ID: "anthropic", Label: "Claude", Status: meter.Fresh, ObservedAt: now,
		Spend: meter.KnownValue(1, "USD"), Budget: meter.KnownValue(5, "USD"),
		InputTokens: meter.KnownValue(20, "tokens"), OutputTokens: meter.KnownValue(10, "tokens"), Requests: meter.UnsupportedValue("not reported"),
		Models: []meter.ModelUsage{{ID: "claude-fable-5-1", InputTokens: meter.KnownValue(20, "tokens"), OutputTokens: meter.KnownValue(10, "tokens")}},
	}}}
	model := New(dashboard, nil)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	updated, _ = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(updated.(Model).View(), "claude-fable-5-1") {
		t.Fatal("narrow detail did not show model usage")
	}
}

func TestSelectionDoesNotChangeRenderedHeight(t *testing.T) {
	now := time.Now()
	busy := meter.Snapshot{
		ID: "busy", Label: "Busy provider", Status: meter.Fresh, ObservedAt: now,
		Spend: meter.UnsupportedValue("subscription"), Budget: meter.UnsupportedValue("subscription"),
		InputTokens: meter.KnownValue(1000, "tokens"), OutputTokens: meter.KnownValue(200, "tokens"), Requests: meter.KnownValue(4, "requests"),
		Details: []meter.Metric{
			{Label: "Subscription limit with a long provider label", Value: meter.UnsupportedValue("a long note that must not wrap the terminal")},
			{Label: "Cache write", Value: meter.KnownValue(50, "tokens")},
		},
		UsageWindows: []meter.UsageWindow{
			{Label: "5h", Scope: "GPT-5.3-Codex-Spark", UsedPercent: 22, AvailablePercent: 78, ResetsAt: now.Add(3 * time.Hour)},
			{Label: "7d", Scope: "codex", UsedPercent: 55, AvailablePercent: 45, ResetsAt: now.Add(4 * 24 * time.Hour)},
		},
	}
	for i := 0; i < 8; i++ {
		busy.Models = append(busy.Models, meter.ModelUsage{
			ID: fmt.Sprintf("model-%d", i), InputTokens: meter.KnownValue(float64(100-i), "tokens"), OutputTokens: meter.KnownValue(10, "tokens"),
			Details: []meter.Metric{{Label: "Nested cache", Value: meter.KnownValue(20, "tokens")}},
		})
	}
	quiet := meter.Snapshot{
		ID: "quiet", Label: "Quiet provider", Status: meter.Fresh, ObservedAt: now,
		Spend: meter.KnownValue(1, "USD"), Budget: meter.KnownValue(10, "USD"),
		InputTokens: meter.KnownValue(10, "tokens"), OutputTokens: meter.KnownValue(2, "tokens"), Requests: meter.KnownValue(1, "requests"),
	}
	dashboard := meter.Dashboard{GeneratedAt: now, Period: meter.Period{Start: now, End: now}, Providers: []meter.Snapshot{busy, quiet, busy, quiet}}
	model := New(dashboard, nil)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	model = updated.(Model)
	want := strings.Count(model.View(), "\n")
	assertViewFits(t, model.View(), 110)
	if want+1 > 34 {
		t.Fatalf("initial view is %d lines in a 34-line terminal", want+1)
	}
	for i := 0; i < 3; i++ {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(Model)
		got := strings.Count(model.View(), "\n")
		if got != want {
			t.Fatalf("selection %d changed rendered height from %d to %d lines", i+1, want+1, got+1)
		}
		assertViewFits(t, model.View(), 110)
	}
}

func TestQuotaDisplayUsesOneRemainingPercentage(t *testing.T) {
	now := time.Now()
	provider := meter.Snapshot{
		ID: "codex", Label: "Codex", Status: meter.Fresh, ObservedAt: now,
		InputTokens: meter.KnownValue(10, "tokens"), OutputTokens: meter.KnownValue(2, "tokens"), Requests: meter.KnownValue(1, "requests"),
		UsageWindows: []meter.UsageWindow{
			{Label: "5h", UsedPercent: 22, AvailablePercent: 78},
			{Label: "7d", UsedPercent: 55, AvailablePercent: 45},
			{Label: "7d", Scope: "Fable", UsedPercent: 54, AvailablePercent: 46},
		},
	}
	dashboard := meter.Dashboard{GeneratedAt: now, Period: meter.Period{Start: now, End: now}, Providers: []meter.Snapshot{provider}}
	model := New(dashboard, nil)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 110, Height: 24})
	view := updated.(Model).View()
	for _, want := range []string{"5h 78% left", "7d 45% left", "Fable 7d 46% left"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view does not contain %q:\n%s", want, view)
		}
	}
	for _, redundant := range []string{"used/avail", "% used", "% avail"} {
		if strings.Contains(view, redundant) {
			t.Fatalf("view contains redundant quota label %q:\n%s", redundant, view)
		}
	}
}

func TestCodexPickerSummaryOmitsInternalLimitScope(t *testing.T) {
	provider := meter.Snapshot{Provider: "codex", UsageWindows: []meter.UsageWindow{
		{Label: "5h", Scope: "GPT-5.3-Codex-Spark", AvailablePercent: 78},
		{Label: "7d", Scope: "codex", AvailablePercent: 45},
	}}
	summary := compactProviderSummary(provider)
	if summary != "5h 78% left  7d 45% left" {
		t.Fatalf("Codex picker summary = %q", summary)
	}
}

func assertViewFits(t *testing.T, view string, width int) {
	t.Helper()
	for number, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("line %d is %d columns in a %d-column terminal: %q", number+1, got, width, line)
		}
	}
}
