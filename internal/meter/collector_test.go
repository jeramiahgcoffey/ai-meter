package meter

import (
	"context"
	"testing"
	"time"
)

type fakeProvider struct{ snapshot Snapshot }

func (p fakeProvider) ID() string                             { return p.snapshot.ID }
func (p fakeProvider) Fetch(context.Context, Period) Snapshot { return p.snapshot }

func TestCollectorKeepsPartialResultsAndUsesStaleCache(t *testing.T) {
	dir := t.TempDir()
	cache := &Cache{Dir: dir}
	old := Snapshot{ID: "bad", Label: "Cached", Status: Fresh, ObservedAt: time.Now().Add(-time.Hour)}
	if err := cache.Store(old); err != nil {
		t.Fatal(err)
	}
	collector := NewCollector([]Provider{
		fakeProvider{Snapshot{ID: "good", Label: "Good", Status: Fresh}},
		fakeProvider{Snapshot{ID: "bad", Label: "Cached", Status: Failed, Issues: []string{"timeout"}}},
	}, cache)
	got := collector.Collect(context.Background(), Period{})
	if len(got.Providers) != 2 {
		t.Fatalf("got %d providers", len(got.Providers))
	}
	if got.Providers[0].Status != Cached || len(got.Providers[0].Issues) != 1 {
		t.Fatalf("stale fallback missing: %+v", got.Providers[0])
	}
}

func TestCollectorSortsBaseProfilesBeforeAPIsAndAlternates(t *testing.T) {
	collector := NewCollector([]Provider{
		fakeProvider{Snapshot{ID: "codex-local-alt", Label: "Codex alt", Status: Fresh}},
		fakeProvider{Snapshot{ID: "openai", Label: "OpenAI API", Status: Fresh}},
		fakeProvider{Snapshot{ID: "claude-local", Label: "Claude", Status: Fresh}},
		fakeProvider{Snapshot{ID: "claude-local-alt", Label: "Claude alt", Status: Fresh}},
		fakeProvider{Snapshot{ID: "codex-local", Label: "Codex", Status: Fresh}},
	}, nil)

	got := collector.Collect(context.Background(), Period{})
	want := []string{"Claude", "Codex", "OpenAI API", "Claude alt", "Codex alt"}
	if len(got.Providers) != len(want) {
		t.Fatalf("got %d providers, want %d", len(got.Providers), len(want))
	}
	for i, label := range want {
		if got.Providers[i].Label != label {
			t.Fatalf("provider %d = %q, want %q", i, got.Providers[i].Label, label)
		}
	}
}
