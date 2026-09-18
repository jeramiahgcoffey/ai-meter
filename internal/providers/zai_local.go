package providers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jeramiahgcoffey/ai-meter/internal/meter"
)

// ZaiLocal reports a Claude Code home that runs against z.ai's
// Anthropic-compatible GLM endpoint. Token totals come from the shared
// Claude session scan; plan capacity comes from z.ai's monitor API, so
// this provider needs the z.ai key. The key is sent only to the monitor
// endpoint and never appears in the snapshot.
type ZaiLocal struct {
	ClaudeLocal
	Key     string
	BaseURL string
	Client  httpClient
	Now     func() time.Time
}

type zaiQuotaResponse struct {
	Success bool   `json:"success"`
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Data    struct {
		Level  string     `json:"level"`
		Limits []zaiLimit `json:"limits"`
	} `json:"data"`
}

// zaiLimit mirrors one quota bucket. Credit plans report CREDIT_LIMIT
// with the bucket size in usage and the spent share in currentValue;
// token-metered plans report TOKENS_LIMIT. Both carry the window
// identity in unit and number plus percentage and nextResetTime.
type zaiLimit struct {
	Type          string   `json:"type"`
	Unit          *float64 `json:"unit"`
	Number        *float64 `json:"number"`
	Percentage    *float64 `json:"percentage"`
	NextResetTime *int64   `json:"nextResetTime"`
}

func (p *ZaiLocal) Fetch(ctx context.Context, period meter.Period) meter.Snapshot {
	snapshot := p.ClaudeLocal.Fetch(ctx, period)
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	windows, notes, err := p.quotaWindows(ctx, client)
	if err != nil {
		snapshot.Issues = append(snapshot.Issues, err.Error())
		snapshot.Status = meter.Partial
		return snapshot
	}
	snapshot.UsageWindows = append(snapshot.UsageWindows, windows...)
	snapshot.Details = dropUnsupportedLimitDetail(snapshot.Details)
	snapshot.Source = "z.ai GLM plan quota plus local session metadata observed on this machine"
	snapshot.Issues = append(snapshot.Issues, notes...)
	return snapshot
}

func (p *ZaiLocal) quotaWindows(ctx context.Context, client httpClient) ([]meter.UsageWindow, []string, error) {
	if p.Key == "" {
		return nil, nil, fmt.Errorf("no z.ai key found; set ZAI_API_KEY or store one in the macOS Keychain under \"zai-api-key\"")
	}
	base := p.BaseURL
	if base == "" {
		base = "https://api.z.ai"
	}
	var payload zaiQuotaResponse
	headers := map[string]string{
		"Authorization": p.Key,
		"Accept":        "application/json",
	}
	if err := getJSON(ctx, client, base+"/api/monitor/usage/quota/limit", headers, &payload); err != nil {
		return nil, nil, fmt.Errorf("read z.ai plan quota: %w", err)
	}
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	return parseZaiQuota(payload, now)
}

func parseZaiQuota(payload zaiQuotaResponse, now time.Time) ([]meter.UsageWindow, []string, error) {
	if !payload.Success {
		return nil, nil, fmt.Errorf("z.ai quota request failed: code %d %s", payload.Code, payload.Msg)
	}
	var windows []meter.UsageWindow
	var notes []string
	for _, limit := range payload.Data.Limits {
		if limit.Type != "CREDIT_LIMIT" && limit.Type != "TOKENS_LIMIT" {
			notes = append(notes, fmt.Sprintf("Skipped unknown z.ai limit type %q.", limit.Type))
			continue
		}
		label, minutes, known := zaiLimitWindow(limit)
		if !known {
			notes = append(notes, fmt.Sprintf("Skipped z.ai %s limit with unknown window (unit %s, number %s).", limit.Type, zaiLimitNumber(limit.Unit), zaiLimitNumber(limit.Number)))
			continue
		}
		if limit.Percentage == nil {
			notes = append(notes, fmt.Sprintf("Skipped z.ai %s %s limit without a utilization percentage.", limit.Type, label))
			continue
		}
		used := *limit.Percentage
		window := meter.UsageWindow{
			Label: label, Scope: "GLM", WindowMinutes: minutes,
			UsedPercent: used, AvailablePercent: max(0, 100-used), ObservedAt: now,
		}
		if limit.NextResetTime != nil {
			window.ResetsAt = time.UnixMilli(*limit.NextResetTime)
		}
		windows = append(windows, window)
	}
	return windows, notes, nil
}

// zaiLimitWindow names the bucket: unit 3 with number 5 is the 5-hour
// cycle and unit 6 with number 1 is the weekly cycle.
func zaiLimitWindow(limit zaiLimit) (string, int64, bool) {
	if limit.Unit == nil || limit.Number == nil {
		return "", 0, false
	}
	switch {
	case *limit.Unit == 3 && *limit.Number == 5:
		return "5h", 300, true
	case *limit.Unit == 6 && *limit.Number == 1:
		return "7d", 10080, true
	default:
		return "", 0, false
	}
}

func zaiLimitNumber(value *float64) string {
	if value == nil {
		return "?"
	}
	return fmt.Sprintf("%v", *value)
}

// dropUnsupportedLimitDetail removes the placeholder the Claude scan adds
// when a home has no cached subscription limits; the z.ai windows replace it.
func dropUnsupportedLimitDetail(details []meter.Metric) []meter.Metric {
	kept := details[:0]
	for _, metric := range details {
		if metric.Label == "Subscription limit" && metric.Value.State == meter.Unsupported {
			continue
		}
		kept = append(kept, metric)
	}
	return kept
}
