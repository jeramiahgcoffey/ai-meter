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
