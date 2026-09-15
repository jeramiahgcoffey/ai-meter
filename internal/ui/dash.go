package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/jeramiahgcoffey/ai-meter/internal/meter"
)

func (m Model) dashView(width, height int, now time.Time) string {
	accounts := meter.ActiveSubscriptions(m.dashboard, now, meter.RecentActivityWindow)
	if len(accounts) == 0 {
		return lipgloss.NewStyle().Foreground(muted).Width(width).Align(lipgloss.Center).Render(
			"No local subscription activity in the last 5h.\n\nPress d to view every account.",
		)
	}

	heading := sectionTitle.Render("Active capacity")
	subheading := subtle.Render("  used on this machine · last 5h")
	if lipgloss.Width(heading)+lipgloss.Width(subheading) <= width {
		heading += subheading
	}
	lines := []string{heading, ""}
	for index, account := range accounts {
		activity := "active " + ageAt(*account.Snapshot.LastActivityAt, now)
		status := statusStyle(account.Snapshot.Status).Render("●")
		nameWidth := max(4, width-lipgloss.Width(activity)-5)
		name := title.Render(truncate(account.Snapshot.Label, nameWidth))
		gap := max(1, width-lipgloss.Width(name)-lipgloss.Width(activity)-3)
		lines = append(lines, status+" "+name+strings.Repeat(" ", gap)+subtle.Render(activity))
		if len(account.Limits) == 0 {
			lines = append(lines, "  "+subtle.Render("No applicable limit reported"))
		} else {
			for _, window := range account.Limits {
				lines = append(lines, "  "+dashLimitLine(window, max(1, width-2), now))
			}
		}
		if index+1 < len(accounts) {
			lines = append(lines, "")
		}
	}
	if len(lines) > height {
		lines = lines[:max(1, height)]
		lines[len(lines)-1] = subtle.Render("… more active limits")
	}
	return strings.Join(lines, "\n")
}

func dashLimitLine(window meter.UsageWindow, width int, now time.Time) string {
	name := usageWindowName(window)
	nameWidth := min(10, max(4, width/6))
	percent := fmt.Sprintf("%3.0f%% left", window.AvailablePercent)
	coloredPercent := lipgloss.NewStyle().Foreground(capacityColor(window.AvailablePercent)).Bold(true).Render(percent)
	reset := resetRemaining(window.ResetsAt, now)
	if width < 34 {
		plain := name + " " + percent
		if lipgloss.Width(plain) > width {
			return truncate(plain, width)
		}
		return subtle.Render(name) + " " + coloredPercent
	}

	renderedName := subtle.Render(fmt.Sprintf("%-*s", nameWidth, truncate(name, nameWidth)))
	fixed := nameWidth + 2 + lipgloss.Width(percent)
	resetText := ""
	if reset != "" {
		resetText = "↻ " + reset
		fixed += 2 + lipgloss.Width(resetText)
	}
	barWidth := min(16, width-fixed-2)
	parts := []string{renderedName}
	if barWidth >= 4 {
		parts = append(parts, progressBar(window.AvailablePercent, barWidth))
	}
	parts = append(parts, coloredPercent)
	line := strings.Join(parts, "  ")
	if resetText != "" && lipgloss.Width(line)+2+lipgloss.Width(resetText) <= width {
		line += "  " + subtle.Render(resetText)
	}
	return line
}

func providerSummaryWindows(provider meter.Snapshot) []meter.UsageWindow {
	weekly := meter.SubscriptionSummaryLimits(provider)
	if len(weekly) > 0 || provider.Provider == "claude" || provider.Provider == "codex" {
		return weekly
	}
	return nil
}

func overviewUsageWindows(windows []meter.UsageWindow) []meter.UsageWindow {
	byDuration := make(map[string]meter.UsageWindow)
	for _, window := range windows {
		key := fmt.Sprintf("%d", window.WindowMinutes)
		if window.WindowMinutes == 0 {
			key = window.Label
		}
		current, exists := byDuration[key]
		if !exists || window.UsedPercent > current.UsedPercent {
			byDuration[key] = window
		}
	}
	durations := make([]string, 0, len(byDuration))
	for duration := range byDuration {
		durations = append(durations, duration)
	}
	sort.Slice(durations, func(i, j int) bool {
		return byDuration[durations[i]].WindowMinutes < byDuration[durations[j]].WindowMinutes ||
			byDuration[durations[i]].WindowMinutes == byDuration[durations[j]].WindowMinutes && byDuration[durations[i]].Label < byDuration[durations[j]].Label
	})
	result := make([]meter.UsageWindow, 0, len(durations))
	for _, duration := range durations {
		result = append(result, byDuration[duration])
	}
	return result
}

func detailUsageWindows(provider meter.Snapshot) []meter.UsageWindow {
	if provider.Provider == "codex" {
		return overviewUsageWindows(provider.UsageWindows)
	}
	return provider.UsageWindows
}

func renderedProviderSummaryAt(provider meter.Snapshot, width int, now time.Time) string {
	windows := providerSummaryWindows(provider)
	if len(windows) == 0 {
		return subtle.Render(truncate(compactProviderSummaryAt(provider, now), width))
	}
	type summaryCell struct {
		name      string
		percent   string
		reset     string
		available float64
	}
	windowCells := make([]summaryCell, 0, len(windows))
	seenResets := make(map[int64]bool)
	for _, window := range windows {
		name := usageWindowName(window)
		if provider.Provider == "codex" {
			name = window.Label
		}
		cell := summaryCell{name: name, percent: fmt.Sprintf("%3.0f%% left", window.AvailablePercent), available: window.AvailablePercent}
		resetKey := window.ResetsAt.Unix()
		if reset := resetRemaining(window.ResetsAt, now); reset != "" && !seenResets[resetKey] {
			cell.reset = "↻ " + reset
		}
		if !window.ResetsAt.IsZero() {
			seenResets[resetKey] = true
		}
		windowCells = append(windowCells, cell)
	}

	const alignedNameWidth = 9
	nameWidth := alignedNameWidth
	baseWidth := max(0, len(windowCells)-1) * 3
	for _, cell := range windowCells {
		baseWidth += nameWidth + 1 + lipgloss.Width(cell.percent)
	}
	if baseWidth > width {
		nameWidth = 0
	}

	cells := make([]string, 0, len(windowCells))
	resets := make([]string, 0, len(windowCells))
	used := 0
	for _, cell := range windowCells {
		name := cell.name
		if nameWidth > 0 {
			name = fmt.Sprintf("%-*s", nameWidth, truncate(name, nameWidth))
		}
		base := subtle.Render(name) + " " + lipgloss.NewStyle().Foreground(capacityColor(cell.available)).Bold(true).Render(cell.percent)
		separatorWidth := 0
		if len(cells) > 0 {
			separatorWidth = 3
		}
		if used+separatorWidth+lipgloss.Width(base) > width {
			break
		}
		cells = append(cells, base)
		resets = append(resets, cell.reset)
		used += separatorWidth + lipgloss.Width(base)
	}
	if len(cells) == 0 {
		window := windows[0]
		plain := fmt.Sprintf("%s %.0f%% left", usageWindowName(window), window.AvailablePercent)
		return lipgloss.NewStyle().Foreground(capacityColor(window.AvailablePercent)).Bold(true).Render(truncate(plain, width))
	}
	for index, reset := range resets {
		if reset == "" {
			continue
		}
		addition := " " + subtle.Render(reset)
		if used+lipgloss.Width(addition) > width {
			continue
		}
		cells[index] += addition
		used += lipgloss.Width(addition)
	}
	return strings.Join(cells, divider.Render(" │ "))
}
