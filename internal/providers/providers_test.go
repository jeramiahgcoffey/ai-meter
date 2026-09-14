package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jeramiahgcoffey/ai-meter/internal/meter"
)

func TestOpenAIFetchCombinesCostAndUsage(t *testing.T) {
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing auth header")
		}
		switch r.URL.Path {
		case "/v1/organization/costs":
			if len(r.URL.Query()["group_by[]"]) != 0 {
				t.Error("costs should not be grouped by model")
			}
			if r.URL.Query().Get("page") == "cost-2" {
				return response(`{"has_more":false,"data":[{"results":[{"amount":{"value":2.34,"currency":"usd"}}]}]}`), nil
			}
			return response(`{"has_more":true,"next_page":"cost-2","data":[{"results":[{"amount":{"value":10,"currency":"usd"}}]}]}`), nil
		case "/v1/organization/usage/completions":
			if len(r.URL.Query()["group_by[]"]) != 1 || r.URL.Query()["group_by[]"][0] != "model" {
				t.Error("usage is not grouped by model")
			}
			if r.URL.Query().Get("page") == "usage-2" {
				return response(`{"has_more":false,"data":[{"results":[{"model":"gpt-5.6","input_tokens":500,"output_tokens":50,"num_model_requests":3}]}]}`), nil
			}
			return response(`{"has_more":true,"next_page":"usage-2","data":[{"results":[{"model":"gpt-5.6","input_tokens":1000,"output_tokens":250,"num_model_requests":7}]}]}`), nil
		default:
			return notFound(), nil
		}
	})
	p := &OpenAI{InstanceID: "openai", Label: "OpenAI", Key: "secret", BaseURL: "https://example.test", Client: client}
	got := p.Fetch(context.Background(), testPeriod())
	if got.Status != meter.Fresh || got.Spend.Value != 12.34 || got.InputTokens.Value != 1500 || got.Requests.Value != 10 {
		t.Fatalf("unexpected snapshot: %+v", got)
	}
	if len(got.Models) != 1 || got.Models[0].ID != "gpt-5.6" || got.Models[0].InputTokens.Value != 1500 {
		t.Fatalf("unexpected models: %+v", got.Models)
	}
}

func TestAnthropicFetchIncludesCacheTokens(t *testing.T) {
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("X-Api-Key") != "secret" {
			t.Error("missing auth header")
		}
		switch r.URL.Path {
		case "/v1/organizations/cost_report":
			if len(r.URL.Query()["group_by[]"]) != 0 {
				t.Error("costs should not be grouped by model")
			}
			if r.URL.Query().Get("page") == "cost-2" {
				return response(`{"has_more":false,"data":[{"results":[{"amount":"1.25","currency":"USD"}]}]}`), nil
			}
			return response(`{"has_more":true,"next_page":"cost-2","data":[{"results":[{"amount":"4.50","currency":"USD"}]}]}`), nil
		case "/v1/organizations/usage_report/messages":
			if len(r.URL.Query()["group_by[]"]) != 1 || r.URL.Query()["group_by[]"][0] != "model" {
				t.Error("usage is not grouped by model")
			}
			if r.URL.Query().Get("page") == "usage-2" {
				return response(`{"has_more":false,"data":[{"results":[{"model":"claude-fable-5-1","uncached_input_tokens":50,"cache_read_input_tokens":0,"cache_creation":{"ephemeral_1h_input_tokens":0,"ephemeral_5m_input_tokens":0},"output_tokens":10}]}]}`), nil
			}
			return response(`{"has_more":true,"next_page":"usage-2","data":[{"results":[{"model":"claude-fable-5-1","uncached_input_tokens":100,"cache_read_input_tokens":20,"cache_creation":{"ephemeral_1h_input_tokens":3,"ephemeral_5m_input_tokens":2},"output_tokens":40}]}]}`), nil
		default:
			return notFound(), nil
		}
	})
	p := &Anthropic{InstanceID: "anthropic", Label: "Anthropic", Key: "secret", BaseURL: "https://example.test", Client: client}
	got := p.Fetch(context.Background(), testPeriod())
	if got.Status != meter.Fresh || got.Spend.Value != 5.75 || got.InputTokens.Value != 175 || got.OutputTokens.Value != 50 {
		t.Fatalf("unexpected snapshot: %+v", got)
	}
	if len(got.Models) != 1 || got.Models[0].ID != "claude-fable-5-1" || got.Models[0].InputTokens.Value != 175 {
		t.Fatalf("unexpected Fable usage: %+v", got.Models)
	}
	if len(got.Models[0].Details) != 3 || got.Models[0].Details[1].Value.Value != 20 {
		t.Fatalf("missing cache details: %+v", got.Models[0].Details)
	}
}

func TestOpenAIFetchFollowsUsagePages(t *testing.T) {
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v1/organization/costs" {
			return response(`{"data":[]}`), nil
		}
		if r.URL.Query().Get("page") == "p2" {
			return response(`{"has_more":false,"data":[{"results":[{"model":"gpt-b","input_tokens":2,"output_tokens":1,"num_model_requests":1}]}]}`), nil
		}
		return response(`{"has_more":true,"next_page":"p2","data":[{"results":[{"model":"gpt-a","input_tokens":3,"output_tokens":1,"num_model_requests":1}]}]}`), nil
	})
	p := &OpenAI{InstanceID: "openai", Label: "OpenAI", Key: "secret", BaseURL: "https://example.test", Client: client}
	got := p.Fetch(context.Background(), testPeriod())
	if got.InputTokens.Value != 5 || got.OutputTokens.Value != 2 || len(got.Models) != 2 {
		t.Fatalf("pagination was not aggregated: %+v", got)
	}
}

func TestClaudeLocalReadsTypedMetadataAndDeduplicatesResponses(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "projects", "one")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	data := `{"timestamp":"2026-09-02T12:00:00Z","type":"assistant","uuid":"u1","message":{"id":"m1","model":"claude-fable-5-1","content":"PRIVATE MARKER","usage":{"input_tokens":100,"output_tokens":25,"cache_read_input_tokens":10,"cache_creation_input_tokens":5,"iterations":[{"input_tokens":9000}]}}}` + "\n"
	if err := os.WriteFile(filepath.Join(sessionDir, "one.jsonl"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "copy.jsonl"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := &ClaudeLocal{InstanceID: "claude-local", Label: "Claude local", Root: root}
	got := provider.Fetch(context.Background(), testPeriod())
	if got.Status != meter.Fresh || got.InputTokens.Value != 115 || got.OutputTokens.Value != 25 || got.Requests.Value != 1 {
		t.Fatalf("unexpected local usage: %+v", got)
	}
	if len(got.Models) != 1 || got.Models[0].ID != "claude-fable-5-1" || len(got.Models[0].Details) != 2 {
		t.Fatalf("unexpected model details: %+v", got.Models)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("PRIVATE MARKER")) {
		t.Fatal("message content escaped the typed local parser")
	}
	if len(got.Details) != 3 || got.Details[0].Value.State != meter.Unsupported || got.Details[1].Value.Value != 10 {
		t.Fatalf("missing cached Claude quota should remain explicit: %+v", got.Details)
	}
}

func TestClaudeLocalReadsCachedSubscriptionWindows(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	data := `{"cachedUsageUtilization":{"fetchedAtMs":1789411393744,"utilization":{"five_hour":{"utilization":12,"resets_at":"2026-09-14T22:00:00Z"},"seven_day":{"utilization":43,"resets_at":"2026-09-17T03:59:59Z"},"limits":[{"kind":"weekly_scoped","group":"weekly","percent":54,"resets_at":"2026-09-17T03:59:59Z","scope":{"model":{"id":"claude-fable-5-1","display_name":"Fable"}}}]}}}`
	if err := os.WriteFile(filepath.Join(root, ".claude.json"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	got := (&ClaudeLocal{InstanceID: "claude-local", Label: "Claude", Root: root}).Fetch(context.Background(), testPeriod())
	if len(got.UsageWindows) != 3 {
		t.Fatalf("Claude windows = %+v", got.UsageWindows)
	}
	if got.UsageWindows[0].Label != "5h" || got.UsageWindows[0].AvailablePercent != 88 {
		t.Fatalf("Claude 5-hour window = %+v", got.UsageWindows[0])
	}
	if got.UsageWindows[1].Label != "7d" || got.UsageWindows[1].AvailablePercent != 57 {
		t.Fatalf("Claude 7-day window = %+v", got.UsageWindows[1])
	}
	if got.UsageWindows[2].Scope != "Fable" || got.UsageWindows[2].AvailablePercent != 46 {
		t.Fatalf("Claude Fable window = %+v", got.UsageWindows[2])
	}
	if len(got.Details) != 2 {
		t.Fatalf("supported quota left an unsupported detail: %+v", got.Details)
	}
}

func TestCodexLocalUsesResponseUsageAndActualQuotaShape(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions", "2026", "09", "02")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	data := `{"timestamp":"2026-09-02T12:00:00Z","type":"turn_context","payload":{"model":"gpt-5.6-sol"}}` + "\n" +
		`{"timestamp":"2026-09-02T12:00:01Z","type":"token_usage_record","payload":{"response_id":"r1","usage":{"input_tokens":100,"cached_input_tokens":20,"output_tokens":25,"total_tokens":125}}}` + "\n" +
		`{"timestamp":"2026-09-02T12:00:02Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"output_tokens":25},"total_token_usage":{"input_tokens":100,"output_tokens":25}},"rate_limits":{"primary":{"used_percent":67,"window_minutes":300,"resets_at":1790011300},"secondary":{"used_percent":12,"window_minutes":10080,"resets_at":1790011300}}}}` + "\n" +
		`{"timestamp":"2026-09-02T12:00:02.500Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"output_tokens":25}},"rate_limits":{"primary":{"used_percent":13,"window_minutes":10080,"resets_at":1790011300},"secondary":null}}}` + "\n" +
		`{"timestamp":"2026-09-02T12:00:03Z","type":"response_item","payload":{"type":"message","content":"PRIVATE MARKER token_usage_record input_tokens"}}` + "\n"
	if err := os.WriteFile(filepath.Join(sessionDir, "one.jsonl"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := &CodexLocal{InstanceID: "codex-local", Label: "Codex local", Root: root}
	got := provider.Fetch(context.Background(), testPeriod())
	if got.Status != meter.Fresh || got.InputTokens.Value != 100 || got.OutputTokens.Value != 25 || got.Requests.Value != 1 {
		t.Fatalf("unexpected Codex usage: %+v", got)
	}
	if len(got.Models) != 1 || got.Models[0].ID != "gpt-5.6-sol" || got.Models[0].Details[0].Value.Value != 20 {
		t.Fatalf("unexpected Codex model usage: %+v", got.Models)
	}
	if len(got.UsageWindows) != 2 || got.UsageWindows[0].Label != "5h" || got.UsageWindows[0].UsedPercent != 67 || got.UsageWindows[0].AvailablePercent != 33 {
		t.Fatalf("unexpected 5-hour window: %+v", got.UsageWindows)
	}
	if got.UsageWindows[1].Label != "7d" || got.UsageWindows[1].UsedPercent != 13 || got.UsageWindows[1].AvailablePercent != 87 {
		t.Fatalf("newer partial quota erased or failed to update a window: %+v", got.UsageWindows)
	}
}

func TestCodexLocalHonorsCancelledContext(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := (&CodexLocal{InstanceID: "codex-local", Label: "Codex", Root: root}).Fetch(ctx, testPeriod())
	if got.Status != meter.Failed || len(got.Issues) == 0 {
		t.Fatalf("cancelled fetch = %+v", got)
	}
}

func TestCodexLiveUsageWindowsPreserveLimitScopes(t *testing.T) {
	fiveHours := int64(300)
	sevenDays := int64(10080)
	reset := int64(1_790_011_300)
	spark := "GPT-5.3-Codex-Spark"
	response := codexRateLimitsResponse{RateLimitsByLimitID: map[string]codexLiveSnapshot{
		"codex": {
			LimitID: "codex",
			Primary: &codexLiveWindow{UsedPercent: 13, WindowDurationMin: &sevenDays, ResetsAt: &reset},
		},
		"codex_bengalfox": {
			LimitID: "codex_bengalfox", LimitName: &spark,
			Primary:   &codexLiveWindow{UsedPercent: 22, WindowDurationMin: &fiveHours, ResetsAt: &reset},
			Secondary: &codexLiveWindow{UsedPercent: 7, WindowDurationMin: &sevenDays, ResetsAt: &reset},
		},
	}}

	windows := codexLiveUsageWindows(response, time.Unix(reset-60, 0))
	if len(windows) != 2 {
		t.Fatalf("windows = %+v", windows)
	}
	if windows[0].Label != "5h" || windows[0].Scope != spark || windows[0].AvailablePercent != 78 {
		t.Fatalf("5-hour model limit = %+v", windows[0])
	}
	if windows[1].LimitID != "codex" || windows[1].UsedPercent != 13 || windows[1].AvailablePercent != 87 {
		t.Fatalf("general 7-day limit = %+v", windows[1])
	}
}

func testPeriod() meter.Period {
	return meter.Period{Start: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(request *http.Request) (*http.Response, error) { return f(request) }

func response(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header)}
}

func notFound() *http.Response {
	return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(bytes.NewBufferString("not found")), Header: make(http.Header)}
}
