package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jeramiahgcoffey/ai-meter/internal/meter"
)

func TestSettingsPageShowsSafeProviderSources(t *testing.T) {
	now := time.Now()
	model := New(meter.Dashboard{GeneratedAt: now, Period: meter.Period{Start: now, End: now}}, nil,
		WithSettings(SettingsController{Load: func() SettingsSnapshot {
			return SettingsSnapshot{
				ConfigPath: "/tmp/config.json",
				Files:      []string{"/tmp/config.json"},
				Providers: []SettingProvider{
					{Label: "Claude", Kind: "claude", Source: "/home/test/.claude"},
					{Label: "OpenAI API", Kind: "openai", Source: "$OPENAI_ADMIN_KEY"},
				},
			}
		}}))
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	updated, _ = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	view := updated.(Model).View()
	for _, want := range []string{"Settings", "/tmp/config.json", "Claude", "$OPENAI_ADMIN_KEY", "Add API account"} {
		if !strings.Contains(view, want) {
			t.Fatalf("settings view does not contain %q:\n%s", want, view)
		}
	}
	assertViewFits(t, view, 90)
}

func TestSetupWizardSavesDefaultOpenAIAccount(t *testing.T) {
	now := time.Now()
	var saved ProviderDraft
	model := New(meter.Dashboard{GeneratedAt: now, Period: meter.Period{Start: now, End: now}}, nil,
		WithSettings(SettingsController{
			Load: func() SettingsSnapshot { return SettingsSnapshot{ConfigPath: "/tmp/config.json"} },
			Save: func(draft ProviderDraft) error {
				saved = draft
				return nil
			},
		}))
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	model = updated.(Model)
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'s'}},
		{Type: tea.KeyRunes, Runes: []rune{'a'}},
		{Type: tea.KeyEnter},
		{Type: tea.KeyEnter},
		{Type: tea.KeyEnter},
		{Type: tea.KeyEnter},
		{Type: tea.KeyEnter},
		{Type: tea.KeyEnter},
		{Type: tea.KeyEnter},
	} {
		updated, _ = model.Update(key)
		model = updated.(Model)
	}
	if saved.Kind != "openai" || saved.Label != "OpenAI API" || saved.ID != "openai" || saved.CredentialRef != "OPENAI_ADMIN_KEY" {
		t.Fatalf("saved draft = %+v", saved)
	}
	if model.screen != settingsScreen || model.notice != "Saved OpenAI API" {
		t.Fatalf("model did not return to settings after save: screen=%d notice=%q", model.screen, model.notice)
	}
}

func TestSetupWizardTreatsQAsTextInput(t *testing.T) {
	wizard := newSetupWizard()
	wizard.step = labelStep
	model := Model{screen: setupScreen, wizard: wizard}
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if command != nil {
		t.Fatal("q quit while editing a wizard field")
	}
	if updated.(Model).wizard.label != "q" {
		t.Fatalf("label = %q", updated.(Model).wizard.label)
	}
}

func TestSettingsAndWizardFitNarrowTerminal(t *testing.T) {
	now := time.Now()
	model := New(meter.Dashboard{GeneratedAt: now, Period: meter.Period{Start: now, End: now}}, nil,
		WithSettings(SettingsController{Load: func() SettingsSnapshot {
			return SettingsSnapshot{
				ConfigPath: "/a/very/long/path/to/config.json",
				Providers: []SettingProvider{{
					Label:  "A provider label that is too long",
					Kind:   "anthropic",
					Source: "$A_VERY_LONG_ENVIRONMENT_VARIABLE",
				}},
			}
		}}))
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	model = updated.(Model)
	assertViewFits(t, model.View(), 40)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = updated.(Model)
	model.wizard.step = credentialRefStep
	model.wizard.credentialRef = strings.Repeat("LONG_SECRET_FILE_NAME", 4)
	assertViewFits(t, model.View(), 40)
}
