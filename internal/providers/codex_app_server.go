package providers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/jeramiahgcoffey/ai-meter/internal/meter"
)

type codexAppServerResponse struct {
	ID     json.RawMessage          `json:"id"`
	Result *codexRateLimitsResponse `json:"result,omitempty"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type codexRateLimitsResponse struct {
	RateLimits          codexLiveSnapshot            `json:"rateLimits"`
	RateLimitsByLimitID map[string]codexLiveSnapshot `json:"rateLimitsByLimitId"`
}

type codexLiveSnapshot struct {
	LimitID   string           `json:"limitId"`
	LimitName *string          `json:"limitName"`
	Primary   *codexLiveWindow `json:"primary"`
	Secondary *codexLiveWindow `json:"secondary"`
}

type codexLiveWindow struct {
	UsedPercent       float64 `json:"usedPercent"`
	WindowDurationMin *int64  `json:"windowDurationMins"`
	ResetsAt          *int64  `json:"resetsAt"`
}

func readCodexRateLimits(ctx context.Context, root string) ([]meter.UsageWindow, error) {
	command := exec.CommandContext(ctx, "codex", "app-server", "--stdio")
	command.Env = codexEnvironment(root)
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open Codex app-server input: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open Codex app-server output: %w", err)
	}
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start Codex app-server: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = command.Wait()
	}()

	encoder := json.NewEncoder(stdin)
	if err := encoder.Encode(map[string]any{
		"id": 1, "method": "initialize",
		"params": map[string]any{"clientInfo": map[string]string{"name": "ai-meter", "version": "0.1.0"}},
	}); err != nil {
		return nil, fmt.Errorf("initialize Codex app-server: %w", err)
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	if _, err := readCodexResponse(scanner, "1"); err != nil {
		return nil, fmt.Errorf("initialize Codex app-server: %w", err)
	}
	if err := encoder.Encode(map[string]any{"id": 2, "method": "account/rateLimits/read"}); err != nil {
		return nil, fmt.Errorf("request Codex rate limits: %w", err)
	}
	response, err := readCodexResponse(scanner, "2")
	if err != nil {
		return nil, fmt.Errorf("read Codex rate limits: %w", err)
	}
	if response.Result == nil {
		return nil, fmt.Errorf("Codex app-server returned no rate limits")
	}
	return codexLiveUsageWindows(*response.Result, time.Now()), nil
}

func readCodexResponse(scanner *bufio.Scanner, id string) (codexAppServerResponse, error) {
	for scanner.Scan() {
		var response codexAppServerResponse
		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
			continue
		}
		if string(response.ID) != id {
			continue
		}
		if response.Error != nil {
			return response, fmt.Errorf("%s (code %d)", response.Error.Message, response.Error.Code)
		}
		return response, nil
	}
	if err := scanner.Err(); err != nil {
		return codexAppServerResponse{}, err
	}
	return codexAppServerResponse{}, io.EOF
}

func codexEnvironment(root string) []string {
	environment := make([]string, 0, len(os.Environ())+1)
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "CODEX_HOME=") {
			environment = append(environment, item)
		}
	}
	return append(environment, "CODEX_HOME="+root)
}

func codexLiveUsageWindows(response codexRateLimitsResponse, observedAt time.Time) []meter.UsageWindow {
	buckets := response.RateLimitsByLimitID
	if len(buckets) == 0 {
		id := response.RateLimits.LimitID
		if id == "" {
			id = "codex"
		}
		buckets = map[string]codexLiveSnapshot{id: response.RateLimits}
	}
	ids := make([]string, 0, len(buckets))
	for id := range buckets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var result []meter.UsageWindow
	for _, id := range ids {
		bucket := buckets[id]
		scope := id
		if bucket.LimitName != nil && *bucket.LimitName != "" {
			scope = *bucket.LimitName
		}
		for _, window := range []*codexLiveWindow{bucket.Primary, bucket.Secondary} {
			if window == nil || window.WindowDurationMin == nil || *window.WindowDurationMin <= 0 {
				continue
			}
			available := max(0, 100-window.UsedPercent)
			row := meter.UsageWindow{
				Label: durationLabel(*window.WindowDurationMin), Scope: scope, LimitID: id,
				WindowMinutes: *window.WindowDurationMin, UsedPercent: window.UsedPercent,
				AvailablePercent: available, ObservedAt: observedAt,
			}
			if window.ResetsAt != nil {
				row.ResetsAt = time.Unix(*window.ResetsAt, 0)
			}
			result = append(result, row)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].WindowMinutes != result[j].WindowMinutes {
			return result[i].WindowMinutes < result[j].WindowMinutes
		}
		if (result[i].LimitID == "codex") != (result[j].LimitID == "codex") {
			return result[i].LimitID == "codex"
		}
		return result[i].Scope < result[j].Scope
	})
	return result
}
