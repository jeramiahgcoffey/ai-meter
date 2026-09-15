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
	ink          = lipgloss.AdaptiveColor{Light: "#18212B", Dark: "#E6EDF3"}
	muted        = lipgloss.AdaptiveColor{Light: "#52606D", Dark: "#8396A8"}
	blue         = lipgloss.AdaptiveColor{Light: "#087F8C", Dark: "#55D6E2"}
	sea          = lipgloss.AdaptiveColor{Light: "#08785B", Dark: "#78E0B2"}
	warn         = lipgloss.AdaptiveColor{Light: "#B54708", Dark: "#FF9F43"}
	danger       = lipgloss.AdaptiveColor{Light: "#B42332", Dark: "#FF5F6D"}
	panel        = lipgloss.AdaptiveColor{Light: "#D5DEE7", Dark: "#2B3A48"}
	title        = lipgloss.NewStyle().Foreground(ink).Bold(true)
	subtle       = lipgloss.NewStyle().Foreground(muted)
	selected     = lipgloss.NewStyle().Foreground(blue).Bold(true)
	accent       = lipgloss.NewStyle().Foreground(blue)
	success      = lipgloss.NewStyle().Foreground(sea)
	failure      = lipgloss.NewStyle().Foreground(danger)
	sectionTitle = lipgloss.NewStyle().Foreground(blue).Bold(true)
	divider      = lipgloss.NewStyle().Foreground(panel)
)

type loader func() meter.Dashboard
type loadedMsg meter.Dashboard

type Model struct {
	dashboard          meter.Dashboard
	load               loader
	selected           int
	width              int
	height             int
	loading            bool
	detailOnly         bool
	screen             screen
	settings           SettingsSnapshot
	settingsController SettingsController
	wizard             setupWizard
	notice             string
	returnScreen       screen
}

func New(dashboard meter.Dashboard, reload loader, options ...Option) Model {
	model := Model{dashboard: dashboard, load: reload}
	for _, option := range options {
		option(&model)
	}
	return model
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.screen == settingsScreen {
			return m.updateSettings(msg)
		}
		if m.screen == setupScreen {
			return m.updateSetup(msg)
		}
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "s":
			if m.settingsController.Load != nil {
				m.loadSettings()
				m.notice = ""
				m.returnScreen = m.screen
				m.screen = settingsScreen
			}
		case "d":
			if m.screen == dashScreen {
				m.screen = dashboardScreen
			} else {
				m.detailOnly = false
				m.screen = dashScreen
			}
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
			if m.screen == dashboardScreen && len(m.dashboard.Providers) > 0 && m.width < 94 {
				m.detailOnly = !m.detailOnly
			}
		case "esc":
			if m.screen == dashScreen {
				m.screen = dashboardScreen
			} else {
				m.detailOnly = false
			}
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
	if m.screen == settingsScreen {
		body := m.settingsView(m.width, contentHeight)
		return header + "\n" + divider.Render(strings.Repeat("─", max(1, m.width))) + "\n" + body + "\n" + m.footer()
	}
	if m.screen == setupScreen {
		body := m.setupView(m.width, contentHeight)
		return header + "\n" + divider.Render(strings.Repeat("─", max(1, m.width))) + "\n" + body + "\n" + m.footer()
	}
	if m.screen == dashScreen {
		body := fitHeight(m.dashView(m.width, contentHeight, time.Now()), contentHeight)
		return header + "\n" + divider.Render(strings.Repeat("─", max(1, m.width))) + "\n" + body + "\n" + m.footer()
	}
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
	page := "usage"
	if m.screen == settingsScreen {
		page = "settings"
	}
	if m.screen == setupScreen {
		page = "add account"
	}
	if m.screen == dashScreen {
		page = "dash"
	}
	left := " " + name + "  " + accent.Bold(true).Render(page)
	statusText := m.dashboard.GeneratedAt.Format("updated 15:04")
	statusStyle := subtle
	if m.loading {
		statusText = "refreshing..."
		statusStyle = lipgloss.NewStyle().Foreground(warn)
	}
	if m.screen == dashboardScreen {
		period := subtle.Render(fmt.Sprintf("%s to %s", m.dashboard.Period.Start.Format("Jan 2"), m.dashboard.Period.End.Format("Jan 2")))
		candidate := left + "  " + period
		if lipgloss.Width(candidate)+1+lipgloss.Width(statusText) <= m.width {
			left = candidate
		}
	}
	statusText = truncate(statusText, max(1, m.width-lipgloss.Width(left)-1))
	status := statusStyle.Render(statusText)
	gap := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(status))
	return left + strings.Repeat(" ", gap) + status
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
			cursor = "▸ "
			labelStyle = selected
		}
		status := statusStyle(provider.Status).Render("● " + string(provider.Status))
		labelWidth := max(4, width-lipgloss.Width(status)-4)
		renderedLabel := labelStyle.Render(truncate(provider.Label, labelWidth))
		gap := max(1, width-2-lipgloss.Width(renderedLabel)-lipgloss.Width(status))
		rows = append(rows, cursor+renderedLabel+strings.Repeat(" ", gap)+status)
		summary := renderedProviderSummaryAt(provider, max(1, width-2), now)
		rows = append(rows, "  "+summary)
	}
	return strings.Join(rows, "\n")
}

func (m Model) detail(width, height int) string {
	if len(m.dashboard.Providers) == 0 {
		return ""
	}
	p := m.dashboard.Providers[m.selected]
	lines := []string{
		providerHeader(p, width),
		usageStats(p, width),
	}
	if p.Spend.State == meter.Known || p.Budget.State == meter.Known {
		stats := []labeledStat{{"Spend", formatValue(p.Spend)}, {"Budget", formatValue(p.Budget)}}
		if p.Spend.State == meter.Known && p.Budget.State == meter.Known && p.Budget.Value > 0 {
			stats = append(stats, labeledStat{"Used", fmt.Sprintf("%.0f%%", p.Spend.Value/p.Budget.Value*100)})
		}
		lines = append(lines, renderStats(stats, width))
	}
	if len(p.UsageWindows) > 0 {
		lines = append(lines, "", title.Render("Usage limits"))
		now := time.Now()
		for _, window := range detailUsageWindows(p) {
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
	compactHint := "↑/↓ select   enter details   r refresh   q quit"
	if m.settingsController.Load != nil {
		hint = "↑/↓ select   enter details   r refresh   s settings   q quit"
		compactHint = "↑/↓ select   r refresh   s settings   q quit"
	}
	if m.detailOnly {
		hint = "esc back   r refresh   s settings   q quit"
		compactHint = hint
	}
	if m.screen == dashScreen {
		hint = "d all accounts   r refresh   q quit"
		compactHint = hint
		if m.settingsController.Load != nil {
			hint = "d all accounts   r refresh   s settings   q quit"
			compactHint = "d accounts   r refresh   s settings   q quit"
		}
	}
	if m.screen == settingsScreen {
		hint = "a add API account   r reload   esc back   q quit"
		compactHint = "a add   r reload   esc back   q quit"
	}
	if m.screen == setupScreen {
		hint = "enter next   ←/→ choose   esc back   ctrl+c quit"
		compactHint = "enter next   esc back   ^c quit"
		if m.wizard.step == reviewStep {
			hint = "enter save   esc back   ctrl+c quit"
			compactHint = "enter save   esc back   ^c quit"
		}
	}
	if lipgloss.Width(hint)+1 > m.width {
		hint = compactHint
	}
	hint = truncate(hint, max(1, m.width-1))
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
	windows := providerSummaryWindows(provider)
	if len(windows) > 0 {
		var parts []string
		seenResets := make(map[int64]bool)
		limit := min(3, len(windows))
		for _, window := range windows[:limit] {
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
		if limit < len(windows) {
			parts = append(parts, fmt.Sprintf("+%d", len(windows)-limit))
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
	renderedName := lipgloss.NewStyle().Foreground(color).Render(fmt.Sprintf("%-*s", nameWidth, name))
	percent := fmt.Sprintf("%.0f%% left", window.AvailablePercent)
	reset := ""
	if remaining := resetRemaining(window.ResetsAt, now); remaining != "" {
		reset = "reset in " + remaining
	}

	fixedWidth := nameWidth + 2 + lipgloss.Width(percent)
	if reset != "" {
		fixedWidth += 2 + lipgloss.Width(reset)
	}
	barWidth := min(14, width-fixedWidth-2)
	if barWidth < 4 {
		barWidth = 0
	}

	parts := []string{renderedName}
	if barWidth > 0 {
		parts = append(parts, progressBar(window.AvailablePercent, barWidth))
	}
	parts = append(parts, lipgloss.NewStyle().Foreground(color).Render(percent))
	if reset != "" && lipgloss.Width(strings.Join(parts, "  "))+2+lipgloss.Width(reset) <= width {
		parts = append(parts, subtle.Render(reset))
	}
	return strings.Join(parts, "  ")
}

type labeledStat struct {
	label string
	value string
}

func providerHeader(provider meter.Snapshot, width int) string {
	meta := statusStyle(provider.Status).Render("● "+string(provider.Status)) + subtle.Render("  "+age(provider.ObservedAt))
	nameWidth := max(4, width-lipgloss.Width(meta)-2)
	name := title.Render(truncate(provider.Label, nameWidth))
	gap := max(1, width-lipgloss.Width(name)-lipgloss.Width(meta))
	return name + strings.Repeat(" ", gap) + meta
}

func usageStats(provider meter.Snapshot, width int) string {
	stats := []labeledStat{
		{"Input", compactOrDash(provider.InputTokens)},
		{"Output", compactOrDash(provider.OutputTokens)},
		{"Requests", compactOrDash(provider.Requests)},
	}
	return renderStats(stats, width)
}

func renderStats(stats []labeledStat, width int) string {
	parts := make([]string, 0, len(stats))
	for _, stat := range stats {
		parts = append(parts, subtle.Render(stat.label)+" "+title.Render(stat.value))
	}
	line := strings.Join(parts, "   ")
	if lipgloss.Width(line) <= width {
		return line
	}
	compactParts := make([]string, 0, len(stats))
	for _, stat := range stats {
		label := stat.label
		if len(label) > 3 {
			label = label[:3]
		}
		compactParts = append(compactParts, subtle.Render(label)+" "+title.Render(stat.value))
	}
	return strings.Join(compactParts, "  ")
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

func capacityColor(availablePercent float64) lipgloss.AdaptiveColor {
	switch capacityTierFor(availablePercent) {
	case capacityCritical:
		return danger
	case capacityWarning:
		return warn
	default:
		return sea
	}
}

type capacityTier uint8

const (
	capacityHealthy capacityTier = iota
	capacityWarning
	capacityCritical
)

func capacityTierFor(availablePercent float64) capacityTier {
	if availablePercent <= 10 {
		return capacityCritical
	}
	if availablePercent <= 25 {
		return capacityWarning
	}
	return capacityHealthy
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
	return ageAt(value, time.Now())
}

func ageAt(value, now time.Time) string {
	d := now.Sub(value).Round(time.Minute)
	if d < time.Minute {
		return "just now"
	}
	days := int(d / (24 * time.Hour))
	d -= time.Duration(days) * 24 * time.Hour
	hours := int(d / time.Hour)
	d -= time.Duration(hours) * time.Hour
	minutes := int(d / time.Minute)
	if days > 0 {
		if hours > 0 {
			return fmt.Sprintf("%dd %dh ago", days, hours)
		}
		return fmt.Sprintf("%dd ago", days)
	}
	if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("%dh %dm ago", hours, minutes)
		}
		return fmt.Sprintf("%dh ago", hours)
	}
	return fmt.Sprintf("%dm ago", minutes)
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	var result strings.Builder
	for _, character := range value {
		candidate := result.String() + string(character) + "…"
		if lipgloss.Width(candidate) > width {
			break
		}
		result.WriteRune(character)
	}
	return result.String() + "…"
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
