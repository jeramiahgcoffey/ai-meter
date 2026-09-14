package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/jeramiahgcoffey/ai-meter/internal/meter"
)

type CodexLocal struct {
	InstanceID string
	Label      string
	Root       string

	rateLimitsReader func(context.Context, string) ([]meter.UsageWindow, error)

	mu    sync.Mutex
	files map[string]codexFileCache
}

type codexFileCache struct {
	size      int64
	modTime   int64
	records   []codexUsageRecord
	quotas    []codexQuotaRecord
	malformed int
}

type codexUsageRecord struct {
	ID        string
	Timestamp time.Time
	Model     string
	Usage     codexTokenUsage
}

type codexQuotaRecord struct {
	Timestamp time.Time
	Limits    codexRateLimits
}

type codexTokenUsage struct {
	InputTokens           float64 `json:"input_tokens"`
	CachedInputTokens     float64 `json:"cached_input_tokens"`
	CacheWriteInputTokens float64 `json:"cache_write_input_tokens"`
	OutputTokens          float64 `json:"output_tokens"`
	ReasoningOutputTokens float64 `json:"reasoning_output_tokens"`
	TotalTokens           float64 `json:"total_tokens"`
}

type codexRateWindow struct {
	UsedPercent   float64 `json:"used_percent"`
	WindowMinutes int64   `json:"window_minutes"`
	ResetsAt      int64   `json:"resets_at"`
}

type codexRateLimits struct {
	Primary   *codexRateWindow `json:"primary"`
	Secondary *codexRateWindow `json:"secondary"`
	PlanType  string           `json:"plan_type"`
}

type codexEvent struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Payload   struct {
		Type       string          `json:"type"`
		Model      string          `json:"model"`
		ResponseID string          `json:"response_id"`
		Usage      codexTokenUsage `json:"usage"`
		Info       struct {
			Last  codexTokenUsage `json:"last_token_usage"`
			Total codexTokenUsage `json:"total_token_usage"`
		} `json:"info"`
		RateLimits *codexRateLimits `json:"rate_limits"`
	} `json:"payload"`
}

func (p *CodexLocal) ID() string { return p.InstanceID }

func (p *CodexLocal) Fetch(ctx context.Context, period meter.Period) meter.Snapshot {
	snapshot := localSnapshot(p.InstanceID, "codex", p.Label, period, "Local Codex session metadata observed on this machine")
	var liveWindows []meter.UsageWindow
	var liveErr error
	if _, err := os.Stat(filepath.Join(p.Root, "auth.json")); err == nil {
		reader := p.rateLimitsReader
		if reader == nil {
			reader = readCodexRateLimits
		}
		liveCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		liveWindows, liveErr = reader(liveCtx, p.Root)
		cancel()
	}
	files, err := localJSONLFiles(ctx, p.Root, "sessions", period.Start)
	if err != nil {
		return failedLocalSnapshot(snapshot, err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.files == nil {
		p.files = make(map[string]codexFileCache)
	}
	active := make(map[string]bool, len(files))
	for _, file := range files {
		active[file.path] = true
		cached, ok := p.files[file.path]
		if ok && cached.size == file.size && cached.modTime == file.modTime {
			continue
		}
		parsed, parseErr := parseCodexFile(ctx, file.path)
		if parseErr != nil {
			if ctx.Err() != nil {
				return failedLocalSnapshot(snapshot, ctx.Err())
			}
			snapshot.Status = meter.Partial
			snapshot.Issues = append(snapshot.Issues, parseErr.Error())
			continue
		}
		parsed.size, parsed.modTime = file.size, file.modTime
		p.files[file.path] = parsed
	}
	for path := range p.files {
		if !active[path] {
			delete(p.files, path)
		}
	}

	byModel := make(map[string]*meter.ModelUsage)
	cacheRead := make(map[string]float64)
	cacheWrite := make(map[string]float64)
	unique := make(map[string]codexUsageRecord)
	var anonymous []codexUsageRecord
	latestWindows := make(map[int64]codexWindowObservation)
	malformed := 0
	for _, cached := range p.files {
		malformed += cached.malformed
		for _, record := range cached.records {
			if !withinPeriod(record.Timestamp, meterPeriod{start: period.Start, end: period.End}) {
				continue
			}
			if record.ID == "" {
				anonymous = append(anonymous, record)
				continue
			}
			existing, exists := unique[record.ID]
			if !exists || existing.Model == "unknown model" && record.Model != "unknown model" {
				unique[record.ID] = record
			}
		}
		for _, quota := range cached.quotas {
			if quota.Timestamp.After(period.End) {
				continue
			}
			keepLatestCodexWindow(latestWindows, quota.Timestamp, quota.Limits.Primary)
			keepLatestCodexWindow(latestWindows, quota.Timestamp, quota.Limits.Secondary)
		}
	}
	for _, record := range unique {
		addCodexUsage(byModel, cacheRead, cacheWrite, record.Model, record.Usage)
	}
	for _, record := range anonymous {
		addCodexUsage(byModel, cacheRead, cacheWrite, record.Model, record.Usage)
	}

	finishLocalUsage(&snapshot, byModel)
	for id, row := range byModel {
		row.Details = []meter.Metric{
			{Label: "Cached input", Value: meter.KnownValue(cacheRead[id], "tokens")},
			{Label: "Cache write input", Value: meter.KnownValue(cacheWrite[id], "tokens")},
		}
	}
	snapshot.Models = sortedModels(byModel)
	snapshot.UsageWindows = liveWindows
	if len(snapshot.UsageWindows) == 0 {
		snapshot.UsageWindows = codexUsageWindows(latestWindows)
	} else {
		snapshot.Source = "Codex OAuth subscription limits plus local session metadata observed on this machine"
	}
	if liveErr != nil {
		snapshot.Issues = append(snapshot.Issues, "Live subscription limits unavailable; showing session-log fallback: "+liveErr.Error())
		if snapshot.Status == meter.Fresh {
			snapshot.Status = meter.Partial
		}
	}
	snapshot.Details = append(snapshot.Details,
		meter.Metric{Label: "Cached input", Value: meter.KnownValue(sumValues(cacheRead), "tokens")},
		meter.Metric{Label: "Cache write input", Value: meter.KnownValue(sumValues(cacheWrite), "tokens")},
	)
	if malformed > 0 {
		snapshot.Issues = append(snapshot.Issues, fmt.Sprintf("Skipped %d malformed usage metadata records.", malformed))
		if snapshot.Status == meter.Fresh {
			snapshot.Status = meter.Partial
		}
	}
	markEmptyLocal(&snapshot)
	return snapshot
}

func parseCodexFile(ctx context.Context, path string) (codexFileCache, error) {
	var result codexFileCache
	model := "unknown model"
	var fallback []codexUsageRecord
	var previous *codexTokenUsage
	malformed, err := scanLocalJSONL(ctx, path, [][]byte{[]byte(`"token_usage_record"`), []byte(`"turn_context"`), []byte(`"token_count"`)}, func(line []byte) error {
		var event codexEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return err
		}
		timestamp, ok := localEventTime(event.Timestamp)
		if !ok {
			return fmt.Errorf("missing timestamp")
		}
		switch {
		case event.Type == "turn_context":
			if event.Payload.Model != "" {
				model = event.Payload.Model
			}
		case event.Type == "token_usage_record":
			result.records = append(result.records, codexUsageRecord{ID: event.Payload.ResponseID, Timestamp: timestamp, Model: model, Usage: event.Payload.Usage})
		case event.Type == "event_msg" && event.Payload.Type == "token_count":
			if event.Payload.RateLimits != nil {
				result.quotas = append(result.quotas, codexQuotaRecord{Timestamp: timestamp, Limits: *event.Payload.RateLimits})
			}
			current := event.Payload.Info.Total
			delta := current
			if previous != nil {
				delta = positiveCodexDelta(current, *previous)
			}
			if codexUsagePresent(delta) {
				fallback = append(fallback, codexUsageRecord{Timestamp: timestamp, Model: model, Usage: delta})
			}
			previous = &current
		}
		return nil
	})
	result.malformed = malformed
	if len(result.records) == 0 {
		result.records = fallback
	}
	return result, err
}

func codexUsagePresent(usage codexTokenUsage) bool {
	return usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.TotalTokens != 0
}

func positiveCodexDelta(current, previous codexTokenUsage) codexTokenUsage {
	return codexTokenUsage{
		InputTokens:           positiveDelta(current.InputTokens, previous.InputTokens),
		CachedInputTokens:     positiveDelta(current.CachedInputTokens, previous.CachedInputTokens),
		CacheWriteInputTokens: positiveDelta(current.CacheWriteInputTokens, previous.CacheWriteInputTokens),
		OutputTokens:          positiveDelta(current.OutputTokens, previous.OutputTokens),
		ReasoningOutputTokens: positiveDelta(current.ReasoningOutputTokens, previous.ReasoningOutputTokens),
		TotalTokens:           positiveDelta(current.TotalTokens, previous.TotalTokens),
	}
}

func positiveDelta(current, previous float64) float64 {
	if current < previous {
		return current
	}
	return current - previous
}

func addCodexUsage(byModel map[string]*meter.ModelUsage, cacheRead, cacheWrite map[string]float64, model string, usage codexTokenUsage) {
	if model == "" {
		model = "unknown model"
	}
	row := localModel(byModel, model)
	row.InputTokens = addKnown(row.InputTokens, usage.InputTokens, "tokens")
	row.OutputTokens = addKnown(row.OutputTokens, usage.OutputTokens, "tokens")
	row.Requests = addKnown(row.Requests, 1, "requests")
	cacheRead[model] += usage.CachedInputTokens
	cacheWrite[model] += usage.CacheWriteInputTokens
}

type codexWindowObservation struct {
	Timestamp time.Time
	Window    codexRateWindow
}

func keepLatestCodexWindow(windows map[int64]codexWindowObservation, timestamp time.Time, window *codexRateWindow) {
	if window == nil || window.WindowMinutes <= 0 {
		return
	}
	current, exists := windows[window.WindowMinutes]
	if !exists || timestamp.After(current.Timestamp) {
		windows[window.WindowMinutes] = codexWindowObservation{Timestamp: timestamp, Window: *window}
	}
}

func codexUsageWindows(windows map[int64]codexWindowObservation) []meter.UsageWindow {
	minutes := make([]int64, 0, len(windows))
	for value := range windows {
		minutes = append(minutes, value)
	}
	sort.Slice(minutes, func(i, j int) bool { return minutes[i] < minutes[j] })
	result := make([]meter.UsageWindow, 0, len(minutes))
	for _, value := range minutes {
		observation := windows[value]
		if observation.Window.ResetsAt > 0 && time.Unix(observation.Window.ResetsAt, 0).Before(time.Now()) {
			continue
		}
		available := 100 - observation.Window.UsedPercent
		if available < 0 {
			available = 0
		}
		result = append(result, meter.UsageWindow{
			Label: durationLabel(value), WindowMinutes: value,
			UsedPercent: observation.Window.UsedPercent, AvailablePercent: available,
			ResetsAt: time.Unix(observation.Window.ResetsAt, 0), ObservedAt: observation.Timestamp,
		})
	}
	return result
}

func durationLabel(minutes int64) string {
	if minutes <= 0 {
		return "unknown"
	}
	if minutes%1440 == 0 {
		return fmt.Sprintf("%dd", minutes/1440)
	}
	if minutes%60 == 0 {
		return fmt.Sprintf("%dh", minutes/60)
	}
	return fmt.Sprintf("%dm", minutes)
}

func localModel(byModel map[string]*meter.ModelUsage, model string) *meter.ModelUsage {
	row := byModel[model]
	if row == nil {
		row = &meter.ModelUsage{ID: model, Spend: meter.UnsupportedValue("local subscription usage has no model cost attribution")}
		byModel[model] = row
	}
	return row
}

func localSnapshot(id, provider, label string, period meter.Period, source string) meter.Snapshot {
	return meter.Snapshot{
		ID: id, Provider: provider, Label: label, Period: period, ObservedAt: time.Now(), Status: meter.Fresh,
		Spend:       meter.UnsupportedValue("local subscription usage does not expose invoice spend"),
		Budget:      meter.UnsupportedValue("subscription limits are shown separately when the client records them"),
		InputTokens: meter.KnownValue(0, "tokens"), OutputTokens: meter.KnownValue(0, "tokens"),
		Requests: meter.KnownValue(0, "requests"), Source: source,
	}
}

func finishLocalUsage(snapshot *meter.Snapshot, byModel map[string]*meter.ModelUsage) {
	for _, row := range byModel {
		snapshot.InputTokens.Value += knownValue(row.InputTokens)
		snapshot.OutputTokens.Value += knownValue(row.OutputTokens)
		snapshot.Requests.Value += knownValue(row.Requests)
	}
	snapshot.Models = sortedModels(byModel)
}

func knownValue(value meter.Value) float64 {
	if value.State != meter.Known {
		return 0
	}
	return value.Value
}

func failedLocalSnapshot(snapshot meter.Snapshot, err error) meter.Snapshot {
	snapshot.Status = meter.Failed
	snapshot.Issues = []string{err.Error()}
	return snapshot
}

func markEmptyLocal(snapshot *meter.Snapshot) {
	if len(snapshot.Models) != 0 || snapshot.Status == meter.Failed {
		return
	}
	snapshot.Status = meter.Partial
	snapshot.Issues = append(snapshot.Issues, "No usage metadata found in local session logs for this period.")
}
