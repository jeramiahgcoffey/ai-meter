package providers

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jeramiahgcoffey/ai-meter/internal/meter"
)

func TestZaiLocalAddsPlanQuotaWindows(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "projects", "one")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	data := `{"timestamp":"2026-09-02T12:00:00Z","type":"assistant","uuid":"u1","message":{"id":"m1","model":"glm-5.3-flash","usage":{"input_tokens":100,"output_tokens":25}}}` + "\n"
	if err := os.WriteFile(filepath.Join(sessionDir, "one.jsonl"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "zai-key" {
			t.Errorf("z.ai monitor request must carry the raw key, got %q", r.Header.Get("Authorization"))
		}
		if r.URL.String() != "https://api.z.ai/api/monitor/usage/quota/limit" {
			t.Errorf("unexpected quota URL %q", r.URL.String())
		}
		return response(`{"code":200,"msg":"Operation successful","success":true,"data":{"level":"lite","limits":[` +
			`{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":2000,"currentValue":43,"remaining":1956,"percentage":2,"nextResetTime":1789763623973},` +
			`{"type":"CREDIT_LIMIT","unit":6,"number":1,"usage":10000,"currentValue":43,"remaining":9956,"percentage":1,"nextResetTime":1790349542982}]}}`), nil
	})
	now := time.Date(2026, 9, 14, 8, 30, 0, 0, time.UTC)
	p := &ZaiLocal{ClaudeLocal: ClaudeLocal{InstanceID: "claude-local-zai", Label: "Claude zai", Root: root}, Key: "zai-key", Client: client, Now: func() time.Time { return now }}
	got := p.Fetch(context.Background(), testPeriod())
	if got.Status != meter.Fresh {
		t.Fatalf("status = %s, issues = %v", got.Status, got.Issues)
	}
	if got.InputTokens.Value != 100 || got.Requests.Value != 1 {
		t.Fatalf("local token scan broken: %+v", got)
	}
	if len(got.UsageWindows) != 2 {
		t.Fatalf("windows = %+v", got.UsageWindows)
	}
	fiveHour, weekly := got.UsageWindows[0], got.UsageWindows[1]
	if fiveHour.Label != "5h" || fiveHour.Scope != "GLM" || fiveHour.WindowMinutes != 300 || fiveHour.UsedPercent != 2 || fiveHour.AvailablePercent != 98 {
		t.Fatalf("5h window = %+v", fiveHour)
	}
	if !fiveHour.ResetsAt.Equal(time.UnixMilli(1789763623973)) || !fiveHour.ObservedAt.Equal(now) {
		t.Fatalf("5h reset/observed = %v/%v", fiveHour.ResetsAt, fiveHour.ObservedAt)
	}
	if weekly.Label != "7d" || weekly.WindowMinutes != 10080 || weekly.AvailablePercent != 99 {
		t.Fatalf("weekly window = %+v", weekly)
	}
	for _, metric := range got.Details {
		if metric.Label == "Subscription limit" && metric.Value.State == meter.Unsupported {
			t.Fatalf("unsupported placeholder should be replaced by z.ai windows: %+v", got.Details)
		}
	}
	if !strings.Contains(got.Source, "z.ai GLM plan quota") {
		t.Fatalf("source = %q", got.Source)
	}
}

func TestZaiLocalWithoutKeyDegradesButKeepsTokens(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "projects", "one")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	data := `{"timestamp":"2026-09-02T12:00:00Z","type":"assistant","uuid":"u1","message":{"id":"m1","model":"glm-5.3","usage":{"input_tokens":40,"output_tokens":5}}}` + "\n"
	if err := os.WriteFile(filepath.Join(sessionDir, "one.jsonl"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	client := roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("quota request must not fire without a key")
		return notFound(), nil
	})
	p := &ZaiLocal{ClaudeLocal: ClaudeLocal{InstanceID: "claude-local-zai", Label: "Claude zai", Root: root}, Client: client}
	got := p.Fetch(context.Background(), testPeriod())
	if got.Status != meter.Partial || got.InputTokens.Value != 40 {
		t.Fatalf("snapshot = %+v", got)
	}
	if len(got.UsageWindows) != 0 || len(got.Issues) != 1 || !strings.Contains(got.Issues[0], "ZAI_API_KEY") {
		t.Fatalf("missing-key handling = %+v / %v", got.UsageWindows, got.Issues)
	}
}

func TestZaiLocalQuotaFailureKeepsLocalTokens(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	client := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("boom")), Header: make(http.Header)}, nil
	})
	p := &ZaiLocal{ClaudeLocal: ClaudeLocal{InstanceID: "claude-local-zai", Label: "Claude zai", Root: root}, Key: "zai-key", Client: client}
	got := p.Fetch(context.Background(), testPeriod())
	if got.Status != meter.Partial || !containsIssue(got.Issues, "HTTP 500") {
		t.Fatalf("quota failure handling = %+v / %v", got.Status, got.Issues)
	}
}

func TestParseZaiQuotaMapsAndSkipsLimits(t *testing.T) {
	now := time.Date(2026, 9, 14, 8, 30, 0, 0, time.UTC)
	payload := zaiQuotaResponse{}
	payload.Success = true
	payload.Data.Level = "lite"
	payload.Data.Limits = []zaiLimit{
		{Type: "CREDIT_LIMIT", Unit: unitPtr(3), Number: numberPtr(5), Percentage: pctPtr(2), NextResetTime: millisPtr(1789763623973)},
		{Type: "TOKENS_LIMIT", Unit: unitPtr(6), Number: numberPtr(1), Percentage: pctPtr(104), NextResetTime: millisPtr(1790349542982)},
		{Type: "TIME_LIMIT", Unit: unitPtr(2), Number: numberPtr(1), Percentage: pctPtr(50), NextResetTime: millisPtr(1790349542982)},
		{Type: "CREDIT_LIMIT", Unit: unitPtr(4), Number: numberPtr(9), Percentage: pctPtr(7), NextResetTime: millisPtr(1790349542982)},
		{Type: "CREDIT_LIMIT", Unit: unitPtr(3), Number: numberPtr(5), NextResetTime: millisPtr(1790349542982)},
	}
	windows, notes, err := parseZaiQuota(payload, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(windows) != 2 {
		t.Fatalf("windows = %+v", windows)
	}
	if windows[0].Label != "5h" || windows[0].UsedPercent != 2 || !windows[0].ResetsAt.Equal(time.UnixMilli(1789763623973)) {
		t.Fatalf("5h window = %+v", windows[0])
	}
	if windows[1].Label != "7d" || windows[1].UsedPercent != 104 || windows[1].AvailablePercent != 0 {
		t.Fatalf("weekly window must clamp available at zero: %+v", windows[1])
	}
	if len(notes) != 3 {
		t.Fatalf("notes = %v", notes)
	}
	if !strings.Contains(notes[0], "TIME_LIMIT") || !strings.Contains(notes[1], "unit 4") || !strings.Contains(notes[2], "without a utilization percentage") {
		t.Fatalf("notes = %v", notes)
	}

	payload.Success = false
	payload.Code = 401
	payload.Msg = "Unauthorized"
	if _, _, err := parseZaiQuota(payload, now); err == nil || !strings.Contains(err.Error(), "401 Unauthorized") {
		t.Fatalf("failed payload error = %v", err)
	}
}

func unitPtr(v float64) *float64 { return &v }

func numberPtr(v float64) *float64 { return &v }

func pctPtr(v float64) *float64 { return &v }

func millisPtr(v int64) *int64 { return &v }

func containsIssue(issues []string, needle string) bool {
	for _, issue := range issues {
		if strings.Contains(issue, needle) {
			return true
		}
	}
	return false
}
