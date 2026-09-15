package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jeramiahgcoffey/ai-meter/internal/meter"
)

type ClaudeLocal struct {
	InstanceID string
	Label      string
	Root       string

	mu    sync.Mutex
	files map[string]claudeFileCache
}

type claudeFileCache struct {
	size      int64
	modTime   int64
	records   []claudeUsageRecord
	malformed int
}

type claudeUsageRecord struct {
	ID        string
	Timestamp time.Time
	Model     string
	Input     float64
	Output    float64
	CacheRead float64
	CacheMake float64
}

type claudeEvent struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	UUID      string `json:"uuid"`
	Message   struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage *struct {
			Input     float64 `json:"input_tokens"`
			Output    float64 `json:"output_tokens"`
			CacheRead float64 `json:"cache_read_input_tokens"`
			CacheMake float64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type claudeUsageCache struct {
	Cached struct {
		FetchedAtMS int64 `json:"fetchedAtMs"`
		Utilization struct {
			FiveHour *claudeUsageWindow  `json:"five_hour"`
			SevenDay *claudeUsageWindow  `json:"seven_day"`
			Limits   []claudeScopedLimit `json:"limits"`
		} `json:"utilization"`
	} `json:"cachedUsageUtilization"`
}

type claudeUsageWindow struct {
	Utilization *float64   `json:"utilization"`
	ResetsAt    *time.Time `json:"resets_at"`
}

type claudeScopedLimit struct {
	Kind     string     `json:"kind"`
	Percent  *float64   `json:"percent"`
	ResetsAt *time.Time `json:"resets_at"`
	Scope    *struct {
		Model *struct {
			ID          *string `json:"id"`
			DisplayName string  `json:"display_name"`
		} `json:"model"`
	} `json:"scope"`
}

func (p *ClaudeLocal) ID() string { return p.InstanceID }

func (p *ClaudeLocal) Fetch(ctx context.Context, period meter.Period) meter.Snapshot {
	snapshot := localSnapshot(p.InstanceID, "claude", p.Label, period, "Local Claude session metadata observed on this machine")
	usageWindows, usageErr := readClaudeUsageWindows(p.Root)
	snapshot.UsageWindows = usageWindows
	if len(usageWindows) == 0 {
		snapshot.Details = []meter.Metric{{Label: "Subscription limit", Value: meter.UnsupportedValue("Claude has not cached subscription limits for this profile")}}
	} else {
		snapshot.Source = "Claude subscription limit cache plus local session metadata observed on this machine"
	}
	if usageErr != nil {
		snapshot.Issues = append(snapshot.Issues, usageErr.Error())
		snapshot.Status = meter.Partial
	}
	files, err := localJSONLFiles(ctx, p.Root, "projects", localScanStart(period))
	if err != nil {
		return failedLocalSnapshot(snapshot, err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.files == nil {
		p.files = make(map[string]claudeFileCache)
	}
	active := make(map[string]bool, len(files))
	for _, file := range files {
		active[file.path] = true
		cached, ok := p.files[file.path]
		if ok && cached.size == file.size && cached.modTime == file.modTime {
			continue
		}
		parsed, parseErr := parseClaudeFile(ctx, file.path)
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
	cacheMake := make(map[string]float64)
	seen := make(map[string]bool)
	malformed := 0
	for _, cached := range p.files {
		malformed += cached.malformed
		for _, record := range cached.records {
			if record.ID != "" && seen[record.ID] {
				continue
			}
			if record.ID != "" {
				seen[record.ID] = true
			}
			noteLocalActivity(&snapshot, record.Timestamp, period.End)
			if !withinPeriod(record.Timestamp, meterPeriod{start: period.Start, end: period.End}) {
				continue
			}
			model := record.Model
			if model == "" {
				model = "unknown model"
			}
			row := localModel(byModel, model)
			row.InputTokens = addKnown(row.InputTokens, record.Input+record.CacheRead+record.CacheMake, "tokens")
			row.OutputTokens = addKnown(row.OutputTokens, record.Output, "tokens")
			row.Requests = addKnown(row.Requests, 1, "requests")
			cacheRead[model] += record.CacheRead
			cacheMake[model] += record.CacheMake
		}
	}
	finishLocalUsage(&snapshot, byModel)
	for id, row := range byModel {
		row.Details = []meter.Metric{
			{Label: "Cache read input", Value: meter.KnownValue(cacheRead[id], "tokens")},
			{Label: "Cache creation input", Value: meter.KnownValue(cacheMake[id], "tokens")},
		}
	}
	snapshot.Models = sortedModels(byModel)
	snapshot.Details = append(snapshot.Details,
		meter.Metric{Label: "Cache read input", Value: meter.KnownValue(sumValues(cacheRead), "tokens")},
		meter.Metric{Label: "Cache creation input", Value: meter.KnownValue(sumValues(cacheMake), "tokens")},
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

func readClaudeUsageWindows(root string) ([]meter.UsageWindow, error) {
	path := filepath.Join(root, ".claude.json")
	if filepath.Base(root) == ".claude" {
		path = filepath.Join(filepath.Dir(root), ".claude.json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read Claude usage cache: %w", err)
	}
	var cache claudeUsageCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("parse Claude usage cache: %w", err)
	}
	observedAt := time.UnixMilli(cache.Cached.FetchedAtMS)
	if cache.Cached.FetchedAtMS == 0 {
		observedAt = time.Time{}
	}
	var windows []meter.UsageWindow
	for _, item := range []struct {
		label   string
		minutes int64
		window  *claudeUsageWindow
	}{
		{label: "5h", minutes: 300, window: cache.Cached.Utilization.FiveHour},
		{label: "7d", minutes: 10080, window: cache.Cached.Utilization.SevenDay},
	} {
		if item.window == nil || item.window.Utilization == nil {
			continue
		}
		used := *item.window.Utilization
		window := meter.UsageWindow{
			Label: item.label, Scope: "Claude", WindowMinutes: item.minutes,
			UsedPercent: used, AvailablePercent: max(0, 100-used), ObservedAt: observedAt,
		}
		if item.window.ResetsAt != nil {
			window.ResetsAt = *item.window.ResetsAt
		}
		windows = append(windows, window)
	}
	for _, limit := range cache.Cached.Utilization.Limits {
		if limit.Kind != "weekly_scoped" || limit.Percent == nil || limit.Scope == nil || limit.Scope.Model == nil {
			continue
		}
		scope := limit.Scope.Model.DisplayName
		id := ""
		if limit.Scope.Model.ID != nil {
			id = *limit.Scope.Model.ID
		}
		if !strings.Contains(strings.ToLower(scope+" "+id), "fable") {
			continue
		}
		if scope == "" {
			scope = id
		}
		used := *limit.Percent
		window := meter.UsageWindow{
			Label: "7d", Scope: scope, LimitID: "claude:model:" + id, WindowMinutes: 10080,
			UsedPercent: used, AvailablePercent: max(0, 100-used), ObservedAt: observedAt,
		}
		if limit.ResetsAt != nil {
			window.ResetsAt = *limit.ResetsAt
		}
		windows = append(windows, window)
	}
	return windows, nil
}

func parseClaudeFile(ctx context.Context, path string) (claudeFileCache, error) {
	var result claudeFileCache
	malformed, err := scanLocalJSONL(ctx, path, [][]byte{[]byte(`"type":"assistant"`), []byte(`"type": "assistant"`)}, func(line []byte) error {
		var event claudeEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return err
		}
		if event.Type != "assistant" || event.Message.Usage == nil {
			return nil
		}
		timestamp, ok := localEventTime(event.Timestamp)
		if !ok {
			return fmt.Errorf("missing timestamp")
		}
		usage := event.Message.Usage
		if usage.Input == 0 && usage.Output == 0 && usage.CacheRead == 0 && usage.CacheMake == 0 {
			return nil
		}
		id := event.Message.ID
		if id == "" {
			id = event.UUID
		}
		result.records = append(result.records, claudeUsageRecord{
			ID: id, Timestamp: timestamp, Model: event.Message.Model,
			Input: usage.Input, Output: usage.Output,
			CacheRead: usage.CacheRead, CacheMake: usage.CacheMake,
		})
		return nil
	})
	result.malformed = malformed
	return result, err
}
