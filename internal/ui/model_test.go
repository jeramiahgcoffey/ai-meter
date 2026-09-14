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

func TestDashboardFitsFortyColumnTerminal(t *testing.T) {
	now := time.Now()
	dashboard := meter.Dashboard{GeneratedAt: now, Period: meter.Period{Start: now, End: now}, Providers: []meter.Snapshot{{
		ID: "claude-local", Label: "Claude with a long profile name", Status: meter.Fresh, ObservedAt: now,
		UsageWindows: []meter.UsageWindow{{Label: "7d", AvailablePercent: 55, ResetsAt: now.Add(48 * time.Hour)}},
	}}}
	model := New(dashboard, nil)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	assertViewFits(t, updated.(Model).View(), 40)
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

func TestPickerSummaryShowsTimeUntilReset(t *testing.T) {
	now := time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)
	provider := meter.Snapshot{Provider: "codex", UsageWindows: []meter.UsageWindow{
		{Label: "5h", AvailablePercent: 78, ResetsAt: now.Add(2*time.Hour + 14*time.Minute)},
		{Label: "7d", AvailablePercent: 45, ResetsAt: now.Add(2*24*time.Hour + 7*time.Hour + 30*time.Minute)},
	}}

	summary := compactProviderSummaryAt(provider, now)
	if summary != "5h 78% left ↻ 2h 14m  7d 45% left ↻ 2d 7h" {
		t.Fatalf("picker summary = %q", summary)
	}
}

func TestPickerSummaryDoesNotRepeatSharedReset(t *testing.T) {
	now := time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)
	reset := now.Add(2*24*time.Hour + 7*time.Hour)
	provider := meter.Snapshot{Provider: "claude", UsageWindows: []meter.UsageWindow{
		{Label: "7d", Scope: "Claude", AvailablePercent: 45, ResetsAt: reset},
		{Label: "7d", Scope: "Fable", AvailablePercent: 30, ResetsAt: reset},
	}}

	summary := compactProviderSummaryAt(provider, now)
	if summary != "7d 45% left ↻ 2d 7h  Fable 7d 30% left" {
		t.Fatalf("picker summary = %q", summary)
	}
}

func TestProgressBarRepresentsRemainingCapacity(t *testing.T) {
	bar := progressBar(50, 10)
	if strings.Count(bar, "█") != 5 || strings.Count(bar, "░") != 5 {
		t.Fatalf("50%% progress bar = %q", bar)
	}
	if got := lipgloss.Width(bar); got != 10 {
		t.Fatalf("progress bar width = %d, want 10", got)
	}
}

func TestUsageWindowLineFitsAndShowsResetCountdown(t *testing.T) {
	now := time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)
	window := meter.UsageWindow{
		Label: "7d", Scope: "Fable", AvailablePercent: 46,
		ResetsAt: now.Add(3*24*time.Hour + 8*time.Hour),
	}
	line := usageWindowLineAt(window, 56, now)
	for _, want := range []string{"Fable 7d", "46% left", "reset in 3d 8h", "█", "░"} {
		if !strings.Contains(line, want) {
			t.Fatalf("usage window line does not contain %q: %q", want, line)
		}
	}
	if got := lipgloss.Width(line); got > 56 {
		t.Fatalf("usage window line is %d columns: %q", got, line)
	}
}

func TestResetRemainingOmitsEmptyUnits(t *testing.T) {
	now := time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		duration time.Duration
		want     string
	}{
		{duration: 2 * time.Hour, want: "2h"},
		{duration: 2 * 24 * time.Hour, want: "2d"},
		{duration: 2*24*time.Hour + 7*time.Hour, want: "2d 7h"},
	} {
		if got := resetRemaining(now.Add(test.duration), now); got != test.want {
			t.Errorf("resetRemaining(%s) = %q, want %q", test.duration, got, test.want)
		}
	}
}

func TestTruncateDoesNotSplitUnicodeCharacters(t *testing.T) {
	got := truncate("5h 91% left ↻ 3h", 14)
	if !strings.Contains(got, "↻") || strings.Contains(got, "�") {
		t.Fatalf("truncate split Unicode character: %q", got)
	}
	if width := lipgloss.Width(got); width != 14 {
		t.Fatalf("truncated width = %d, want 14: %q", width, got)
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
