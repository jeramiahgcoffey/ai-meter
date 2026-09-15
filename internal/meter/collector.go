package meter

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

type Collector struct {
	providers []Provider
	cache     *Cache
	now       func() time.Time
}

func NewCollector(providers []Provider, cache *Cache) *Collector {
	return &Collector{providers: providers, cache: cache, now: time.Now}
}

func (c *Collector) Collect(ctx context.Context, period Period) Dashboard {
	results := make(chan Snapshot, len(c.providers))
	var wg sync.WaitGroup
	for _, provider := range c.providers {
		wg.Add(1)
		go func(p Provider) {
			defer wg.Done()
			results <- p.Fetch(ctx, period)
		}(provider)
	}
	wg.Wait()
	close(results)

	dashboard := Dashboard{GeneratedAt: c.now(), Period: period}
	for result := range results {
		if result.Status == Failed && c.cache != nil {
			if stale, err := c.cache.Load(result.ID); err == nil {
				stale.Status = Cached
				stale.Issues = append(stale.Issues, result.Issues...)
				result = stale
			}
		} else if c.cache != nil {
			_ = c.cache.Store(result)
		}
		dashboard.Providers = append(dashboard.Providers, result)
	}
	sortProviders(dashboard.Providers)
	return dashboard
}

func (c *Collector) Offline(period Period) Dashboard {
	dashboard := Dashboard{GeneratedAt: c.now(), Period: period}
	if c.cache == nil {
		return dashboard
	}
	for _, provider := range c.providers {
		if result, err := c.cache.Load(provider.ID()); err == nil {
			result.Status = Cached
			dashboard.Providers = append(dashboard.Providers, result)
		}
	}
	sortProviders(dashboard.Providers)
	return dashboard
}

func sortProviders(providers []Snapshot) {
	sort.SliceStable(providers, func(i, j int) bool {
		leftLabel := strings.ToLower(providers[i].Label)
		rightLabel := strings.ToLower(providers[j].Label)
		if leftLabel != rightLabel {
			return leftLabel < rightLabel
		}
		return providers[i].ID < providers[j].ID
	})
}
