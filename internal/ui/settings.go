package ui

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

type SettingsSnapshot struct {
	ConfigPath string
	Files      []string
	Providers  []SettingProvider
}

type SettingProvider struct {
	Label  string
	Kind   string
	Source string
}

type ProviderDraft struct {
	Kind             string
	Label            string
	ID               string
	CredentialSource string
	CredentialRef    string
	MonthlyBudgetUSD float64
}

type SettingsController struct {
	Load func() SettingsSnapshot
	Save func(ProviderDraft) error
}

type Option func(*Model)

func WithSettings(controller SettingsController) Option {
	return func(model *Model) {
		model.settingsController = controller
	}
}

type screen uint8

const (
	dashboardScreen screen = iota
	settingsScreen
	setupScreen
)

type wizardStep uint8

const (
	kindStep wizardStep = iota
	labelStep
	idStep
	credentialSourceStep
	credentialRefStep
	budgetStep
	reviewStep
)

type setupWizard struct {
	step             wizardStep
	kind             string
	label            string
	id               string
	credentialSource string
	credentialRef    string
	budget           string
	err              string
}

func newSetupWizard() setupWizard {
	return setupWizard{kind: "openai", credentialSource: "env"}
}

func (m Model) updateSettings(message tea.KeyMsg) (Model, tea.Cmd) {
	switch message.String() {
	case "esc", "s":
		m.screen = dashboardScreen
	case "q":
		return m, tea.Quit
	case "a", "enter":
		m.wizard = newSetupWizard()
		m.screen = setupScreen
	case "r":
		m.loadSettings()
	}
	return m, nil
}

func (m Model) updateSetup(message tea.KeyMsg) (Model, tea.Cmd) {
	wizard := &m.wizard
	wizard.err = ""
	switch message.String() {
	case "esc":
		if wizard.step == kindStep {
			m.screen = settingsScreen
		} else {
			wizard.step--
		}
		return m, nil
	case "left", "right", "up", "down", "tab":
		if wizard.step == kindStep {
			wizard.kind = alternate(wizard.kind, "openai", "anthropic")
		}
		if wizard.step == credentialSourceStep {
			wizard.credentialSource = alternate(wizard.credentialSource, "env", "file")
		}
		return m, nil
	case "enter":
		return m.advanceSetup()
	case "backspace":
		wizard.setCurrentInput(removeLastRune(wizard.currentInput()))
		return m, nil
	}
	if message.Type == tea.KeyRunes && wizard.textStep() {
		wizard.setCurrentInput(wizard.currentInput() + string(message.Runes))
	}
	return m, nil
}

func (m Model) advanceSetup() (Model, tea.Cmd) {
	wizard := &m.wizard
	switch wizard.step {
	case kindStep:
		if wizard.kind == "anthropic" {
			wizard.label, wizard.id, wizard.credentialRef = "Anthropic API", "anthropic", "ANTHROPIC_ADMIN_KEY"
		} else {
			wizard.label, wizard.id, wizard.credentialRef = "OpenAI API", "openai", "OPENAI_ADMIN_KEY"
		}
	case labelStep:
		if strings.TrimSpace(wizard.label) == "" {
			wizard.err = "Label is required"
			return m, nil
		}
	case idStep:
		if strings.TrimSpace(wizard.id) == "" {
			wizard.err = "Account ID is required"
			return m, nil
		}
	case credentialSourceStep:
		if wizard.credentialSource == "file" {
			wizard.credentialRef = ""
		} else if wizard.credentialRef == "" {
			if wizard.kind == "anthropic" {
				wizard.credentialRef = "ANTHROPIC_ADMIN_KEY"
			} else {
				wizard.credentialRef = "OPENAI_ADMIN_KEY"
			}
		}
	case credentialRefStep:
		if strings.TrimSpace(wizard.credentialRef) == "" {
			wizard.err = "Credential reference is required"
			return m, nil
		}
	case budgetStep:
		if _, err := wizard.budgetValue(); err != nil {
			wizard.err = err.Error()
			return m, nil
		}
	case reviewStep:
		if m.settingsController.Save == nil {
			wizard.err = "Settings cannot be saved in this mode"
			return m, nil
		}
		budget, err := wizard.budgetValue()
		if err != nil {
			wizard.err = err.Error()
			return m, nil
		}
		draft := ProviderDraft{
			Kind: wizard.kind, Label: strings.TrimSpace(wizard.label), ID: strings.TrimSpace(wizard.id),
			CredentialSource: wizard.credentialSource, CredentialRef: strings.TrimSpace(wizard.credentialRef),
			MonthlyBudgetUSD: budget,
		}
		if err := m.settingsController.Save(draft); err != nil {
			wizard.err = err.Error()
			return m, nil
		}
		m.loadSettings()
		m.notice = "Saved " + draft.Label
		m.screen = settingsScreen
		if m.load != nil {
			m.loading = true
			return m, func() tea.Msg { return loadedMsg(m.load()) }
		}
		return m, nil
	}
	wizard.step++
	return m, nil
}

func (m *Model) loadSettings() {
	if m.settingsController.Load != nil {
		m.settings = m.settingsController.Load()
	}
}

func (m Model) settingsView(width, height int) string {
	lines := []string{
		title.Render("Settings"),
		subtle.Render(wrap("Local Codex and Claude profiles are detected automatically.", width)),
		"",
		sectionTitle.Render("Config"),
		settingLine("Write to", m.settings.ConfigPath, width),
	}
	if len(m.settings.Files) == 0 {
		lines = append(lines, settingLine("Loaded", "none", width))
	} else {
		for index, file := range m.settings.Files {
			label := ""
			if index == 0 {
				label = "Loaded"
			}
			lines = append(lines, settingLine(label, file, width))
		}
	}
	lines = append(lines, "", sectionTitle.Render("Providers"))
	if len(m.settings.Providers) == 0 {
		lines = append(lines, subtle.Render("No providers detected."))
	} else {
		labelWidth := min(22, max(12, width/4))
		kindWidth := min(16, max(11, width/5))
		lines = append(lines, subtle.Render(fmt.Sprintf("%-*s %-*s %s", labelWidth, "Provider", kindWidth, "Type", "Source")))
		for _, provider := range m.settings.Providers {
			sourceWidth := max(8, width-labelWidth-kindWidth-2)
			lines = append(lines, fmt.Sprintf("%-*s %-*s %s",
				labelWidth, truncate(provider.Label, labelWidth),
				kindWidth, truncate(provider.Kind, kindWidth),
				truncate(provider.Source, sourceWidth),
			))
		}
	}
	lines = append(lines, "", accent.Render("a")+"  Add API account")
	if m.notice != "" {
		lines = append(lines, "", success.Render(m.notice))
	}
	return fitHeight(strings.Join(lines, "\n"), height)
}

func (m Model) setupView(width, height int) string {
	wizard := m.wizard
	lines := []string{
		title.Render("Add API account") + subtle.Render(fmt.Sprintf("  %d of 7", int(wizard.step)+1)),
		subtle.Render(wrap("This stores a credential reference, never the key.", width)),
		"",
	}
	switch wizard.step {
	case kindStep:
		lines = append(lines, sectionTitle.Render("Provider"), choice(wizard.kind, "openai", "anthropic"))
	case labelStep:
		lines = append(lines, sectionTitle.Render("Display name"), inputLine(wizard.label, width))
	case idStep:
		lines = append(lines, sectionTitle.Render("Account ID"), inputLine(wizard.id, width), subtle.Render(wrap("Use a short, unique name such as work or personal.", width)))
	case credentialSourceStep:
		lines = append(lines, sectionTitle.Render("Credential source"), choice(wizard.credentialSource, "env", "file"))
	case credentialRefStep:
		label := "Environment variable"
		if wizard.credentialSource == "file" {
			label = "Mode 0600 key file"
		}
		lines = append(lines, sectionTitle.Render(label), inputLine(wizard.credentialRef, width))
	case budgetStep:
		lines = append(lines, sectionTitle.Render("Monthly budget in USD"), inputLine(wizard.budget, width), subtle.Render(wrap("Leave blank to omit the budget.", width)))
	case reviewStep:
		budget := "none"
		if wizard.budget != "" {
			budget = "$" + wizard.budget
		}
		lines = append(lines,
			sectionTitle.Render("Review"),
			settingLine("Provider", wizard.kind, width),
			settingLine("Name", wizard.label, width),
			settingLine("Account ID", wizard.id, width),
			settingLine("Credential", wizard.credentialSource+": "+wizard.credentialRef, width),
			settingLine("Budget", budget, width),
			"",
			accent.Render("enter")+"  Save account",
		)
	}
	if wizard.err != "" {
		lines = append(lines, "", failure.Render(wizard.err))
	}
	return fitHeight(strings.Join(lines, "\n"), height)
}

func (wizard setupWizard) textStep() bool {
	return wizard.step == labelStep || wizard.step == idStep || wizard.step == credentialRefStep || wizard.step == budgetStep
}

func (wizard setupWizard) currentInput() string {
	switch wizard.step {
	case labelStep:
		return wizard.label
	case idStep:
		return wizard.id
	case credentialRefStep:
		return wizard.credentialRef
	case budgetStep:
		return wizard.budget
	default:
		return ""
	}
}

func (wizard *setupWizard) setCurrentInput(value string) {
	switch wizard.step {
	case labelStep:
		wizard.label = value
	case idStep:
		wizard.id = value
	case credentialRefStep:
		wizard.credentialRef = value
	case budgetStep:
		wizard.budget = value
	}
}

func (wizard setupWizard) budgetValue() (float64, error) {
	if strings.TrimSpace(wizard.budget) == "" {
		return 0, nil
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(wizard.budget), 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("Budget must be a positive number")
	}
	return value, nil
}

func alternate(current, first, second string) string {
	if current == first {
		return second
	}
	return first
}

func removeLastRune(value string) string {
	if value == "" {
		return value
	}
	_, size := utf8.DecodeLastRuneInString(value)
	return value[:len(value)-size]
}

func inputLine(value string, width int) string {
	return accent.Render("› ") + truncate(value, max(1, width-3)) + accent.Render("█")
}

func choice(current string, values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if value == current {
			parts = append(parts, accent.Bold(true).Render("● "+value))
		} else {
			parts = append(parts, subtle.Render("○ "+value))
		}
	}
	return strings.Join(parts, "    ")
}

func settingLine(label, value string, width int) string {
	labelWidth := min(14, max(8, width/5))
	return fmt.Sprintf("%-*s %s", labelWidth, label, truncate(value, max(1, width-labelWidth-1)))
}
