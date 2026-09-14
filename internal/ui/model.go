package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jeramiahgcoffey/ai-meter/internal/meter"
)

var (
	ink      = lipgloss.Color("#DCE7F2")
	muted    = lipgloss.Color("#71879B")
	blue     = lipgloss.Color("#5DADE2")
	sea      = lipgloss.Color("#52D3B0")
	warn     = lipgloss.Color("#F4B860")
	danger   = lipgloss.Color("#EE6C77")
	panel    = lipgloss.Color("#243849")
	title    = lipgloss.NewStyle().Foreground(ink).Bold(true)
	subtle   = lipgloss.NewStyle().Foreground(muted)
	selected = lipgloss.NewStyle().Foreground(blue).Bold(true)
	divider  = lipgloss.NewStyle().Foreground(panel)
)

type loader func() meter.Dashboard
type loadedMsg meter.Dashboard

type Model struct {
	dashboard  meter.Dashboard
	load       loader
	selected   int
	width      int
	height     int
	loading    bool
	detailOnly bool
}

func New(dashboard meter.Dashboard, reload loader) Model {
	return Model{dashboard: dashboard, load: reload}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected+1 < len(m.dashboard.Providers) {
				m.selected++
			}
		case "r":
			if m.load != nil && !m.loading {
				m.loading = true
				return m, func() tea.Msg { return loadedMsg(m.load()) }
			}
		case "enter":
			if len(m.dashboard.Providers) > 0 && m.width < 94 {
				m.detailOnly = !m.detailOnly
			}
		case "esc":
			m.detailOnly = false
		}
	case loadedMsg:
		m.dashboard = meter.Dashboard(msg)
		m.loading = false
		if m.selected >= len(m.dashboard.Providers) {
			m.selected = max(0, len(m.dashboard.Providers)-1)
		}
	}
	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading ai-meter..."
	}
	header := m.header()
	contentHeight := max(1, m.height-4)
	if m.detailOnly {
		body := fitHeight(m.detail(m.width, contentHeight), contentHeight)
		return header + "\n" + divider.Render(strings.Repeat("─", max(1, m.width))) + "\n" + body + "\n" + m.footer()
	}
	if len(m.dashboard.Providers) == 0 {
		body := "No usage sources found.\n\nSign in with Codex or Claude Code, set an admin API key,\nor create ~/.config/ai-meter/config.json.\n\nRun ai-meter --demo to preview the dashboard."
		if len(m.dashboard.Issues) > 0 {
			body += "\n\n" + strings.Join(m.dashboard.Issues, "\n")
		}
		return header + "\n\n" + lipgloss.NewStyle().Foreground(muted).Width(m.width).Align(lipgloss.Center).Render(body) + "\n\n" + m.footer()
	}
	content := fitHeight(m.providerList(m.width, contentHeight), contentHeight)
	if m.width >= 94 {
		listWidth := min(72, max(48, m.width/2))
		detailWidth := m.width - listWidth - 3
		content = lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(listWidth).Render(fitHeight(m.providerList(listWidth, contentHeight), contentHeight)),
			divider.Render("│ "),
			lipgloss.NewStyle().Width(detailWidth).Render(fitHeight(m.detail(detailWidth, contentHeight), contentHeight)),
		)
	}
	return header + "\n" + divider.Render(strings.Repeat("─", max(1, m.width))) + "\n" + content + "\n" + m.footer()
}

func (m Model) header() string {
	name := title.Render("ai meter")
	period := subtle.Render(fmt.Sprintf("%s to %s", m.dashboard.Period.Start.Format("Jan 2"), m.dashboard.Period.End.Format("Jan 2")))
	status := subtle.Render(m.dashboard.GeneratedAt.Format("updated 15:04"))
	if m.loading {
		status = lipgloss.NewStyle().Foreground(warn).Render("refreshing...")
	}
	gap := max(1, m.width-lipgloss.Width(name)-lipgloss.Width(period)-lipgloss.Width(status)-4)
	return " " + name + "  " + period + strings.Repeat(" ", gap) + status
}

func (m Model) providerList(width, height int) string {
	var rows []string
	now := time.Now()
	visible := max(1, height/2)
	start := 0
	if m.selected >= visible {
		start = m.selected - visible + 1
	}
	end := min(len(m.dashboard.Providers), start+visible)
	for i := start; i < end; i++ {
		provider := m.dashboard.Providers[i]
		cursor := "  "
		labelStyle := lipgloss.NewStyle().Foreground(ink)
		if i == m.selected {
			cursor = "› "
			labelStyle = selected
		}
		status := statusStyle(provider.Status).Render(string(provider.Status))
		labelWidth := max(4, width-lipgloss.Width(status)-4)
		renderedLabel := labelStyle.Render(truncate(provider.Label, labelWidth))
		gap := max(1, width-2-lipgloss.Width(renderedLabel)-lipgloss.Width(status))
		rows = append(rows, cursor+renderedLabel+strings.Repeat(" ", gap)+status)
		summary := compactProviderSummaryAt(provider, now)
		rows = append(rows, "  "+subtle.Render(truncate(summary, max(1, width-2))))
	}
	return strings.Join(rows, "\n")
}

func (m Model) detail(width, height int) string {
	if len(m.dashboard.Providers) == 0 {
		return ""
	}
	p := m.dashboard.Providers[m.selected]
	lines := []string{
		title.Render(truncate(p.Label, max(10, width-18))) + "  " + statusStyle(p.Status).Render(string(p.Status)) + subtle.Render("  "+age(p.ObservedAt)),
		fmt.Sprintf("%s in   %s out   %s", formatValue(p.InputTokens), formatValue(p.OutputTokens), formatValue(p.Requests)),
	}
	if p.Spend.State == meter.Known || p.Budget.State == meter.Known {
		line := fmt.Sprintf("Spend %s   Budget %s", formatValue(p.Spend), formatValue(p.Budget))
		if p.Spend.State == meter.Known && p.Budget.State == meter.Known && p.Budget.Value > 0 {
			line += fmt.Sprintf("   %.0f%%", p.Spend.Value/p.Budget.Value*100)
		}
		lines = append(lines, line)
	}
	if len(p.UsageWindows) > 0 {
		lines = append(lines, "", title.Render("Usage limits"))
		now := time.Now()
		for _, window := range p.UsageWindows {
			lines = append(lines, usageWindowLineAt(window, width, now))
		}
	}
	if len(p.Details) > 0 {
		lines = append(lines, "", title.Render("Details"))
		for _, metric := range p.Details {
			lines = append(lines, detailMetricLine(metric.Label, metric.Value, width))
		}
	}
	if len(p.Models) > 0 {
		modelWidth := max(12, width-30)
		lines = append(lines, "", title.Render("Models"), subtle.Render(fmt.Sprintf("%-*s %8s %8s", modelWidth, "Model", "Input", "Output")))
		limit := min(len(p.Models), max(1, height-len(lines)-2))
		for _, model := range p.Models[:limit] {
			lines = append(lines, fmt.Sprintf("%-*s %8s %8s", modelWidth, truncate(model.ID, modelWidth), compactOrDash(model.InputTokens), compactOrDash(model.OutputTokens)))
		}
		if limit < len(p.Models) {
			lines = append(lines, subtle.Render(fmt.Sprintf("+%d models in --json", len(p.Models)-limit)))
		}
	}
	if len(p.Issues) > 0 {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(danger).Render("Issue")+"  "+subtle.Render(wrap(p.Issues[0], max(20, width-8))))
	}
	lines = append(lines, "", subtle.Render(truncate(p.Source, max(1, width))))
	if len(lines) > height {
		lines = append(lines[:max(1, height-1)], subtle.Render("… more in --json"))
	}
	return strings.Join(lines, "\n")
}

func (m Model) footer() string {
	hint := "↑/↓ select   enter details   r refresh   q quit"
	if m.detailOnly {
		hint = "esc back   r refresh   q quit"
	}
	return divider.Render(strings.Repeat("─", max(1, m.width))) + "\n " + subtle.Render(hint)
}

func compactOrDash(value meter.Value) string {
	if value.State != meter.Known {
		return "-"
	}
	return compact(value.Value)
}

func compactProviderSummary(provider meter.Snapshot) string {
	return compactProviderSummaryAt(provider, time.Now())
}

func compactProviderSummaryAt(provider meter.Snapshot, now time.Time) string {
	if len(provider.UsageWindows) > 0 {
		var parts []string
		seenResets := make(map[int64]bool)
		limit := min(3, len(provider.UsageWindows))
		for _, window := range provider.UsageWindows[:limit] {
			name := usageWindowName(window)
			if provider.Provider == "codex" {
				name = window.Label
			}
			part := fmt.Sprintf("%s %.0f%% left", name, window.AvailablePercent)
			resetKey := window.ResetsAt.Unix()
			if remaining := resetRemaining(window.ResetsAt, now); remaining != "" && !seenResets[resetKey] {
				part += " ↻ " + remaining
			}
			if !window.ResetsAt.IsZero() {
				seenResets[resetKey] = true
			}
			parts = append(parts, part)
		}
		if limit < len(provider.UsageWindows) {
			parts = append(parts, fmt.Sprintf("+%d", len(provider.UsageWindows)-limit))
		}
		return strings.Join(parts, "  ")
	}
	parts := []string{formatTokens(provider.InputTokens, provider.OutputTokens)}
	if provider.Requests.State == meter.Known {
		parts = append(parts, compact(provider.Requests.Value)+" req")
	}
	if provider.Spend.State == meter.Known {
		parts = append(parts, formatValue(provider.Spend))
	}
	return strings.Join(parts, "  ")
}

func usageWindowLine(window meter.UsageWindow, width int) string {
	return usageWindowLineAt(window, width, time.Now())
}

func usageWindowLineAt(window meter.UsageWindow, width int, now time.Time) string {
	color := capacityColor(window.AvailablePercent)
	nameWidth := min(12, max(2, width/4))
	name := truncate(usageWindowName(window), nameWidth)
	percent := fmt.Sprintf("%.0f%% left", window.AvailablePercent)
	reset := ""
	if remaining := resetRemaining(window.ResetsAt, now); remaining != "" {
		reset = "reset in " + remaining
	}

	fixedWidth := lipgloss.Width(name) + 2 + lipgloss.Width(percent)
	if reset != "" {
		fixedWidth += 2 + lipgloss.Width(reset)
	}
	barWidth := min(14, width-fixedWidth-2)
	if barWidth < 4 {
		barWidth = 0
	}

	parts := []string{lipgloss.NewStyle().Foreground(color).Render(name)}
	if barWidth > 0 {
		parts = append(parts, progressBar(window.AvailablePercent, barWidth))
	}
	parts = append(parts, lipgloss.NewStyle().Foreground(color).Render(percent))
	if reset != "" && lipgloss.Width(strings.Join(parts, "  "))+2+lipgloss.Width(reset) <= width {
		parts = append(parts, subtle.Render(reset))
	}
	return strings.Join(parts, "  ")
}

func progressBar(availablePercent float64, width int) string {
	if width <= 0 {
		return ""
	}
	availablePercent = min(100, max(0, availablePercent))
	filled := int(math.Round(availablePercent / 100 * float64(width)))
	return lipgloss.NewStyle().Foreground(capacityColor(availablePercent)).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(panel).Render(strings.Repeat("░", width-filled))
}

func capacityColor(availablePercent float64) lipgloss.Color {
	if availablePercent <= 0 {
		return danger
	}
	if availablePercent <= 20 {
		return warn
	}
	return sea
}

func resetRemaining(resetsAt, now time.Time) string {
	if resetsAt.IsZero() {
		return ""
	}
	remaining := resetsAt.Sub(now)
	if remaining <= 0 {
		return "now"
	}
	remaining = remaining.Truncate(time.Minute)
	days := int(remaining / (24 * time.Hour))
	remaining -= time.Duration(days) * 24 * time.Hour
	hours := int(remaining / time.Hour)
	remaining -= time.Duration(hours) * time.Hour
	minutes := int(remaining / time.Minute)
	if days > 0 {
		if hours == 0 {
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		if minutes == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return "<1m"
}

func usageWindowName(window meter.UsageWindow) string {
	if window.Scope == "" || window.Scope == "codex" || window.Scope == "Codex" || window.Scope == "Claude" {
		return window.Label
	}
	return window.Scope + " " + window.Label
}

func fitHeight(value string, height int) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(value, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func detailMetricLine(label string, value meter.Value, width int) string {
	labelWidth := min(18, max(8, width-18))
	return fmt.Sprintf("%-*s %s", labelWidth, truncate(label, labelWidth), formatValue(value))
}

func formatValue(value meter.Value) string {
	if value.State != meter.Known {
		if value.State == meter.Unsupported {
			return subtle.Render("not provided")
		}
		return lipgloss.NewStyle().Foreground(warn).Render("unavailable")
	}
	switch value.Unit {
	case "USD":
		return fmt.Sprintf("$%.2f", value.Value)
	case "tokens":
		return compact(value.Value) + " tok"
	case "requests":
		return compact(value.Value) + " req"
	case "%":
		return fmt.Sprintf("%.0f%%", value.Value)
	default:
		return fmt.Sprintf("%.2f %s", value.Value, value.Unit)
	}
}

func formatTokens(input, output meter.Value) string {
	if input.State != meter.Known || output.State != meter.Known {
		return "token data unavailable"
	}
	return compact(input.Value) + " in  " + compact(output.Value) + " out"
}

func compact(value float64) string {
	if value >= 1_000_000 {
		return fmt.Sprintf("%.1fM", value/1_000_000)
	}
	if value >= 1_000 {
		return fmt.Sprintf("%.1fk", value/1_000)
	}
	return fmt.Sprintf("%.0f", value)
}

func statusStyle(status meter.Status) lipgloss.Style {
	color := sea
	if status == meter.Partial || status == meter.Cached {
		color = warn
	}
	if status == meter.Failed {
		color = danger
	}
	return lipgloss.NewStyle().Foreground(color)
}

func age(value time.Time) string {
	d := time.Since(value).Round(time.Minute)
	if d < time.Minute {
		return "just now"
	}
	return d.String() + " ago"
}

func truncate(value string, width int) string {
	if len(value) <= width {
		return value
	}
	if width < 2 {
		return value[:width]
	}
	return value[:width-1] + "…"
}

func wrap(value string, width int) string {
	if width <= 0 {
		return value
	}
	words := strings.Fields(value)
	var lines []string
	line := ""
	for _, word := range words {
		if len(line)+len(word)+1 > width && line != "" {
			lines = append(lines, line)
			line = word
		} else if line == "" {
			line = word
		} else {
			line += " " + word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
