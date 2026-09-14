package providers

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/jeramiahgcoffey/ai-meter/internal/meter"
)

type Anthropic struct {
	InstanceID string
	Label      string
	Key        string
	BaseURL    string
	BudgetUSD  float64
	Client     httpClient
}

func (p *Anthropic) ID() string { return p.InstanceID }

func (p *Anthropic) Fetch(ctx context.Context, period meter.Period) meter.Snapshot {
	base := p.BaseURL
	if base == "" {
		base = "https://api.anthropic.com"
	}
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	snapshot := meter.Snapshot{
		ID: p.InstanceID, Provider: "anthropic", Label: p.Label, Period: period,
		ObservedAt: time.Now(), Status: meter.Fresh,
		Spend: meter.UnavailableValue("cost endpoint did not return data"), Budget: budgetValue(p.BudgetUSD),
		InputTokens:  meter.UnavailableValue("usage endpoint did not return data"),
		OutputTokens: meter.UnavailableValue("usage endpoint did not return data"),
		Requests:     meter.UnsupportedValue("Anthropic usage report has no request count"),
		Source:       "Anthropic organization usage and cost reports",
	}
	if p.Key == "" {
		snapshot.Status = meter.Failed
		snapshot.Issues = []string{"Reporting credential is not available. Run ai-meter config doctor or ai-meter setup."}
		return snapshot
	}
	headers := map[string]string{"X-Api-Key": p.Key, "Anthropic-Version": "2023-06-01", "Content-Type": "application/json"}
	var wg sync.WaitGroup
	var costErr, usageErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		var responses []anthropicCostResponse
		responses, costErr = fetchAnthropicCosts(ctx, client, base, headers, period)
		if costErr == nil {
			var total float64
			for _, response := range responses {
				for _, bucket := range response.Data {
					for _, result := range bucket.Results {
						value, err := strconv.ParseFloat(result.Amount, 64)
						if err != nil {
							costErr = err
							return
						}
						total += value
					}
				}
			}
			snapshot.Spend = meter.KnownValue(total, "USD")
			snapshot.Spend.Note = "Provider-reported organization cost"
		}
	}()
	go func() {
		defer wg.Done()
		var responses []anthropicUsageResponse
		responses, usageErr = fetchAnthropicUsage(ctx, client, base, headers, period)
		if usageErr == nil {
			var input, output float64
			byModel := map[string]*meter.ModelUsage{}
			cacheReadByModel := map[string]float64{}
			cacheCreateByModel := map[string]float64{}
			uncachedByModel := map[string]float64{}
			for _, response := range responses {
				for _, bucket := range response.Data {
					for _, result := range bucket.Results {
						cacheCreate := result.CacheCreation.Ephemeral1h + result.CacheCreation.Ephemeral5m
						modelInput := result.UncachedInputTokens + result.CacheReadInputTokens + cacheCreate
						input += modelInput
						output += result.OutputTokens
						row := byModel[result.Model]
						if row == nil {
							row = &meter.ModelUsage{ID: result.Model, Requests: meter.UnsupportedValue("Anthropic usage reports do not include request counts"), Spend: meter.UnsupportedValue("Cost report model attribution is not guaranteed")}
							byModel[result.Model] = row
						}
						row.InputTokens = addKnown(row.InputTokens, modelInput, "tokens")
						row.OutputTokens = addKnown(row.OutputTokens, result.OutputTokens, "tokens")
						uncachedByModel[result.Model] += result.UncachedInputTokens
						cacheReadByModel[result.Model] += result.CacheReadInputTokens
						cacheCreateByModel[result.Model] += cacheCreate
					}
				}
			}
			snapshot.InputTokens = meter.KnownValue(input, "tokens")
			snapshot.InputTokens.Note = "Includes uncached, cache-read, and cache-creation input tokens"
			snapshot.OutputTokens = meter.KnownValue(output, "tokens")
			for id, row := range byModel {
				row.Details = []meter.Metric{
					{Label: "Uncached input", Value: meter.KnownValue(uncachedByModel[id], "tokens")},
					{Label: "Cache read input", Value: meter.KnownValue(cacheReadByModel[id], "tokens")},
					{Label: "Cache creation input", Value: meter.KnownValue(cacheCreateByModel[id], "tokens")},
				}
			}
			snapshot.Details = []meter.Metric{
				{Label: "Uncached input", Value: meter.KnownValue(sumValues(uncachedByModel), "tokens")},
				{Label: "Cache read input", Value: meter.KnownValue(sumValues(cacheReadByModel), "tokens")},
				{Label: "Cache creation input", Value: meter.KnownValue(sumValues(cacheCreateByModel), "tokens")},
			}
			snapshot.Models = sortedModels(byModel)
		}
	}()
	wg.Wait()
	addFetchIssues(&snapshot, costErr, usageErr)
	return snapshot
}

func sumValues(values map[string]float64) float64 {
	var total float64
	for _, value := range values {
		total += value
	}
	return total
}

func anthropicURL(endpoint string, period meter.Period, page string, groupByModel bool) string {
	values := url.Values{}
	values.Set("starting_at", period.Start.UTC().Format(time.RFC3339))
	values.Set("ending_at", period.End.UTC().Format(time.RFC3339))
	values.Set("bucket_width", "1d")
	values.Set("limit", "31")
	if groupByModel {
		values.Add("group_by[]", "model")
	}
	if page != "" {
		values.Set("page", page)
	}
	return endpoint + "?" + values.Encode()
}

func fetchAnthropicCosts(ctx context.Context, client httpClient, base string, headers map[string]string, period meter.Period) ([]anthropicCostResponse, error) {
	rawURL := anthropicURL(base+"/v1/organizations/cost_report", period, "", false)
	var pages []anthropicCostResponse
	for {
		var page anthropicCostResponse
		if err := getJSON(ctx, client, rawURL, headers, &page); err != nil {
			return nil, err
		}
		pages = append(pages, page)
		if !page.HasMore || page.NextPage == "" {
			return pages, nil
		}
		rawURL = anthropicURL(base+"/v1/organizations/cost_report", period, page.NextPage, false)
	}
}

func fetchAnthropicUsage(ctx context.Context, client httpClient, base string, headers map[string]string, period meter.Period) ([]anthropicUsageResponse, error) {
	rawURL := anthropicURL(base+"/v1/organizations/usage_report/messages", period, "", true)
	var pages []anthropicUsageResponse
	for {
		var page anthropicUsageResponse
		if err := getJSON(ctx, client, rawURL, headers, &page); err != nil {
			return nil, err
		}
		pages = append(pages, page)
		if !page.HasMore || page.NextPage == "" {
			return pages, nil
		}
		rawURL = anthropicURL(base+"/v1/organizations/usage_report/messages", period, page.NextPage, true)
	}
}

type anthropicCostResponse struct {
	HasMore  bool   `json:"has_more"`
	NextPage string `json:"next_page"`
	Data     []struct {
		Results []struct {
			Amount   string `json:"amount"`
			Currency string `json:"currency"`
		} `json:"results"`
	} `json:"data"`
}

type anthropicUsageResponse struct {
	HasMore  bool   `json:"has_more"`
	NextPage string `json:"next_page"`
	Data     []struct {
		Results []struct {
			UncachedInputTokens  float64 `json:"uncached_input_tokens"`
			CacheReadInputTokens float64 `json:"cache_read_input_tokens"`
			OutputTokens         float64 `json:"output_tokens"`
			Model                string  `json:"model"`
			CacheCreation        struct {
				Ephemeral1h float64 `json:"ephemeral_1h_input_tokens"`
				Ephemeral5m float64 `json:"ephemeral_5m_input_tokens"`
			} `json:"cache_creation"`
		} `json:"results"`
	} `json:"data"`
}
