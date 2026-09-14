package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveMergesEnvironmentDefaultAndExplicitFiles(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("OPENAI_ADMIN_KEY", "secret")
	defaultDir := filepath.Join(home, ".config", "ai-meter")
	if err := os.MkdirAll(defaultDir, 0o700); err != nil {
		t.Fatal(err)
	}
	defaultPath := filepath.Join(defaultDir, "config.json")
	explicit := filepath.Join(work, "extra.json")
	writeTestConfig(t, defaultPath, Config{Providers: []Provider{{ID: "openai", Kind: "openai", Label: "Default label", CredentialEnv: "OPENAI_ADMIN_KEY"}}})
	writeTestConfig(t, explicit, Config{Providers: []Provider{{ID: "openai", Kind: "openai", Label: "Work label", CredentialEnv: "OPENAI_ADMIN_KEY", MonthlyBudgetUSD: 50}}})

	oldConfig := os.Getenv("XDG_CONFIG_HOME")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	defer os.Setenv("XDG_CONFIG_HOME", oldConfig)
	got, err := Resolve(ResolveOptions{Paths: []string{explicit}, HomeDir: home, WorkDir: work, LookupEnv: os.LookupEnv})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Config.Providers) != 1 || got.Config.Providers[0].Label != "Work label" || got.Config.Providers[0].MonthlyBudgetUSD != 50 {
		t.Fatalf("unexpected merge: %+v", got.Config.Providers)
	}
}

func TestResolveAllowsPartialOverlayFromAdditionalFile(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("OPENAI_ADMIN_KEY", "secret")
	overlay := filepath.Join(work, "budget.json")
	writeRawConfig(t, overlay, `{"providers":[{"id":"openai","monthly_budget_usd":42}]}`)

	got, err := Resolve(ResolveOptions{Paths: []string{overlay}, HomeDir: home, WorkDir: work, LookupEnv: os.LookupEnv})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Config.Providers) != 1 {
		t.Fatalf("providers = %+v", got.Config.Providers)
	}
	provider := got.Config.Providers[0]
	if provider.Kind != "openai" || provider.CredentialEnv != "OPENAI_ADMIN_KEY" || provider.MonthlyBudgetUSD != 42 {
		t.Fatalf("overlay did not preserve discovered provider fields: %+v", provider)
	}
}

func TestResolveReportsConsumerProviderConfigsWithoutUsingThem(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex", "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(ResolveOptions{HomeDir: home, WorkDir: t.TempDir(), LookupEnv: func(string) (string, bool) { return "", false }})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Config.Providers) != 1 || got.Config.Providers[0].Kind != "codex-local" {
		t.Fatalf("unexpected discovery: %+v", got)
	}
	if len(got.Detections) != 1 || !got.Detections[0].Usable {
		t.Fatalf("unexpected detection detail: %+v", got.Detections)
	}
}

func TestResolveDiscoversAlternateLocalHomesWithStableIDs(t *testing.T) {
	home := t.TempDir()
	for _, path := range []string{
		filepath.Join(home, ".codex-alt", "sessions"),
		filepath.Join(home, ".claude-alt", "projects"),
	} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, ".codex-alt", "auth.json"), []byte(`{"tokens":"PRIVATE"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude-alt", "settings.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(ResolveOptions{HomeDir: home, WorkDir: t.TempDir(), LookupEnv: func(string) (string, bool) { return "", false }})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Config.Providers) != 2 {
		t.Fatalf("providers = %+v", got.Config.Providers)
	}
	if got.Config.Providers[0].ID != "codex-local-alt" || got.Config.Providers[1].ID != "claude-local-alt" {
		t.Fatalf("unstable alternate IDs: %+v", got.Config.Providers)
	}
	if got.Config.Providers[0].LocalRoot == "" || got.Config.Providers[1].LocalRoot == "" {
		t.Fatalf("local roots missing: %+v", got.Config.Providers)
	}
}

func writeTestConfig(t *testing.T, path string, cfg Config) {
	t.Helper()
	if err := Write(path, cfg); err != nil {
		t.Fatal(err)
	}
}

func writeRawConfig(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}
