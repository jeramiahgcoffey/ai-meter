package meter

import (
	"context"
	"time"
)

type State string

const (
	Known       State = "known"
	Unsupported State = "unsupported"
	Unavailable State = "unavailable"
)

type Value struct {
	State State   `json:"state"`
	Value float64 `json:"value,omitempty"`
	Unit  string  `json:"unit,omitempty"`
	Note  string  `json:"note,omitempty"`
}

func KnownValue(value float64, unit string) Value {
	return Value{State: Known, Value: value, Unit: unit}
}

func UnsupportedValue(note string) Value {
	return Value{State: Unsupported, Note: note}
}

func UnavailableValue(note string) Value {
	return Value{State: Unavailable, Note: note}
}

type Period struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type Status string

const (
	Fresh   Status = "fresh"
	Cached  Status = "cached"
	Partial Status = "partial"
	Failed  Status = "failed"
)

type Snapshot struct {
	ID           string        `json:"id"`
	Provider     string        `json:"provider"`
	Label        string        `json:"label"`
	Period       Period        `json:"period"`
	ObservedAt   time.Time     `json:"observed_at"`
	Status       Status        `json:"status"`
	Spend        Value         `json:"spend"`
	Budget       Value         `json:"budget"`
	InputTokens  Value         `json:"input_tokens"`
	OutputTokens Value         `json:"output_tokens"`
	Requests     Value         `json:"requests"`
	UsageWindows []UsageWindow `json:"usage_windows,omitempty"`
	Models       []ModelUsage  `json:"models,omitempty"`
	Details      []Metric      `json:"details,omitempty"`
	Source       string        `json:"source"`
	Issues       []string      `json:"issues,omitempty"`
}

type UsageWindow struct {
	Label            string    `json:"label"`
	Scope            string    `json:"scope,omitempty"`
	LimitID          string    `json:"limit_id,omitempty"`
	WindowMinutes    int64     `json:"window_minutes"`
	UsedPercent      float64   `json:"used_percent"`
	AvailablePercent float64   `json:"available_percent"`
	ResetsAt         time.Time `json:"resets_at,omitempty"`
	ObservedAt       time.Time `json:"observed_at"`
}

type ModelUsage struct {
	ID           string   `json:"id"`
	InputTokens  Value    `json:"input_tokens"`
	OutputTokens Value    `json:"output_tokens"`
	Requests     Value    `json:"requests"`
	Spend        Value    `json:"spend"`
	Details      []Metric `json:"details,omitempty"`
}

type Metric struct {
	Label string `json:"label"`
	Value Value  `json:"value"`
	Note  string `json:"note,omitempty"`
}

type Dashboard struct {
	GeneratedAt time.Time  `json:"generated_at"`
	Period      Period     `json:"period"`
	Providers   []Snapshot `json:"providers"`
	Issues      []string   `json:"issues,omitempty"`
}

type Provider interface {
	ID() string
	Fetch(ctx context.Context, period Period) Snapshot
}
