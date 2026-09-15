package main

import (
	"testing"

	"github.com/jeramiahgcoffey/ai-meter/internal/config"
)

func TestSettingsSnapshotSortsProvidersAlphabetically(t *testing.T) {
	resolution := config.Resolution{Config: config.Config{Providers: []config.Provider{
		{ID: "codex-local-alt", Label: "Codex alt"},
		{ID: "openai", Label: "OpenAI API"},
		{ID: "claude-local", Label: "Claude"},
		{ID: "claude-local-alt", Label: "Claude alt"},
		{ID: "codex-local", Label: "Codex"},
	}}}

	got := settingsSnapshot(resolution)
	want := []string{"Claude", "Claude alt", "Codex", "Codex alt", "OpenAI API"}
	if len(got.Providers) != len(want) {
		t.Fatalf("providers = %+v", got.Providers)
	}
	for i, label := range want {
		if got.Providers[i].Label != label {
			t.Fatalf("provider %d = %q, want %q", i, got.Providers[i].Label, label)
		}
	}
}

func TestBuildVersionPrefersInjectedReleaseVersion(t *testing.T) {
	previous := version
	version = "v0.2.0"
	t.Cleanup(func() { version = previous })
	if got := buildVersion(); got != "0.2.0" {
		t.Fatalf("buildVersion() = %q", got)
	}
}
