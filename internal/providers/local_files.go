package providers

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jeramiahgcoffey/ai-meter/internal/meter"
)

func effectiveUsageWindows(windows []meter.UsageWindow) []meter.UsageWindow {
	byDuration := make(map[int64]meter.UsageWindow)
	for _, window := range windows {
		current, exists := byDuration[window.WindowMinutes]
		if !exists || window.UsedPercent > current.UsedPercent {
			byDuration[window.WindowMinutes] = window
		}
	}
	minutes := make([]int64, 0, len(byDuration))
	for duration := range byDuration {
		minutes = append(minutes, duration)
	}
	sort.Slice(minutes, func(i, j int) bool { return minutes[i] < minutes[j] })
	result := make([]meter.UsageWindow, 0, len(minutes))
	for _, duration := range minutes {
		result = append(result, byDuration[duration])
	}
	return result
}

const maxLocalLogLine = 64 * 1024 * 1024

type localFile struct {
	path    string
	size    int64
	modTime int64
}

func localJSONLFiles(ctx context.Context, root, directory string, periodStart time.Time) ([]localFile, error) {
	base := filepath.Join(root, directory)
	var files []localFile
	err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(entry.Name()) != ".jsonl" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().Before(periodStart) {
			return nil
		}
		files = append(files, localFile{path: path, size: info.Size(), modTime: info.ModTime().UnixNano()})
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	return files, err
}

func scanLocalJSONL(ctx context.Context, path string, wanted [][]byte, consume func([]byte) error) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	malformed := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxLocalLogLine)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return malformed, err
		}
		line := scanner.Bytes()
		match := false
		for _, marker := range wanted {
			if bytes.Contains(line, marker) {
				match = true
				break
			}
		}
		if !match {
			continue
		}
		if err := consume(line); err != nil {
			malformed++
		}
	}
	if err := scanner.Err(); err != nil {
		return malformed, fmt.Errorf("read %s: %w", path, err)
	}
	return malformed, nil
}

func localEventTime(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return parsed, err == nil
}

func withinPeriod(value time.Time, period meterPeriod) bool {
	return !value.Before(period.start) && !value.After(period.end)
}

// meterPeriod keeps local scanning helpers independent of provider wire types.
type meterPeriod struct {
	start time.Time
	end   time.Time
}
