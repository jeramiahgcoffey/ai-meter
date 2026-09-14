package setup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeramiahgcoffey/ai-meter/internal/config"
)

func TestRunWritesAdditionalConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "work.json")
	input := strings.NewReader("anthropic\nClaude Fable\nclaude-work\nenv\nANTHROPIC_WORK_ADMIN_KEY\n90\n")
	var output bytes.Buffer
	if err := Run(input, &output, path); err != nil {
		t.Fatal(err)
	}
	got, err := config.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Providers) != 1 || got.Providers[0].ID != "claude-work" || got.Providers[0].MonthlyBudgetUSD != 90 {
		t.Fatalf("unexpected config: %+v", got)
	}
	if !strings.Contains(output.String(), "never the key itself") {
		t.Fatalf("missing credential guidance: %s", output.String())
	}
}

func TestSaveReplacesAnAccountByID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	first := Account{Kind: "openai", Label: "OpenAI", ID: "work", CredentialSource: "env", CredentialRef: "OPENAI_ADMIN_KEY", MonthlyBudgetUSD: 20}
	if err := Save(path, first); err != nil {
		t.Fatal(err)
	}
	updated := first
	updated.Label = "OpenAI work"
	updated.MonthlyBudgetUSD = 50
	if err := Save(path, updated); err != nil {
		t.Fatal(err)
	}
	got, err := config.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Providers) != 1 || got.Providers[0].Label != "OpenAI work" || got.Providers[0].MonthlyBudgetUSD != 50 {
		t.Fatalf("saved providers = %+v", got.Providers)
	}
}

func TestSaveRejectsInvalidAccountBeforeWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	err := Save(path, Account{Kind: "openai", Label: "OpenAI", ID: "work", CredentialSource: "secret", CredentialRef: "value"})
	if err == nil || !strings.Contains(err.Error(), "credential source") {
		t.Fatalf("Save error = %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("invalid account wrote config file: %v", statErr)
	}
}
