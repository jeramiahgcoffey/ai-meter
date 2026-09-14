package meter

import (
	"context"
	"sort"
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
	sort.SliceStable(dashboard.Providers, func(i, j int) bool {
		return dashboard.Providers[i].Label < dashboard.Providers[j].Label
	})
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
	sort.SliceStable(dashboard.Providers, func(i, j int) bool {
		return dashboard.Providers[i].Label < dashboard.Providers[j].Label
	})
	return dashboard
}
