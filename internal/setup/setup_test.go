package setup

import (
	"bytes"
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
