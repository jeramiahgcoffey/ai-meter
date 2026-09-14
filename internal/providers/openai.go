package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/jeramiahgcoffey/ai-meter/internal/meter"
)

type OpenAI struct {
	InstanceID string
	Label      string
	Key        string
	BaseURL    string
	BudgetUSD  float64
	Client     httpClient
}

func (p *OpenAI) ID() string { return p.InstanceID }

func (p *OpenAI) Fetch(ctx context.Context, period meter.Period) meter.Snapshot {
	base := p.BaseURL
	if base == "" {
		base = "https://api.openai.com"
	}
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	snapshot := meter.Snapshot{
		ID: p.InstanceID, Provider: "openai", Label: p.Label, Period: period,
		ObservedAt: time.Now(), Status: meter.Fresh,
		Spend:        meter.UnavailableValue("cost endpoint did not return data"),
		Budget:       budgetValue(p.BudgetUSD),
		InputTokens:  meter.UnavailableValue("usage endpoint did not return data"),
		OutputTokens: meter.UnavailableValue("usage endpoint did not return data"),
		Requests:     meter.UnavailableValue("usage endpoint did not return data"),
		Source:       "OpenAI organization usage and costs APIs",
	}
	if p.Key == "" {
		snapshot.Status = meter.Failed
		snapshot.Issues = []string{"Reporting credential is not available. Run ai-meter config doctor or ai-meter setup."}
		return snapshot
	}
	headers := map[string]string{"Authorization": "Bearer " + p.Key, "Content-Type": "application/json"}

	var wg sync.WaitGroup
	var costErr, usageErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		var responses []openAICostResponse
		responses, costErr = fetchOpenAICosts(ctx, client, base, headers, period)
		if costErr == nil {
			var total float64
			for _, response := range responses {
				for _, bucket := range response.Data {
					for _, result := range bucket.Results {
						total += result.Amount.Value
					}
				}
			}
			snapshot.Spend = meter.KnownValue(total, "USD")
			snapshot.Spend.Note = "Provider-reported invoice cost"
		}
	}()
	go func() {
		defer wg.Done()
		var responses []openAIUsageResponse
		responses, usageErr = fetchOpenAIUsage(ctx, client, base, headers, period)
		if usageErr == nil {
			var input, output, requests float64
			byModel := map[string]*meter.ModelUsage{}
			for _, response := range responses {
				for _, bucket := range response.Data {
					for _, result := range bucket.Results {
						input += result.InputTokens
						output += result.OutputTokens
						requests += result.NumModelRequests
						row := byModel[result.Model]
						if row == nil {
							row = &meter.ModelUsage{ID: result.Model, Spend: meter.UnsupportedValue("OpenAI costs are not grouped by model")}
							byModel[result.Model] = row
						}
						row.InputTokens = addKnown(row.InputTokens, result.InputTokens, "tokens")
						row.OutputTokens = addKnown(row.OutputTokens, result.OutputTokens, "tokens")
						row.Requests = addKnown(row.Requests, result.NumModelRequests, "requests")
					}
				}
			}
			snapshot.InputTokens = meter.KnownValue(input, "tokens")
			snapshot.OutputTokens = meter.KnownValue(output, "tokens")
			snapshot.Requests = meter.KnownValue(requests, "requests")
			snapshot.Models = sortedModels(byModel)
		}
	}()
	wg.Wait()
	addFetchIssues(&snapshot, costErr, usageErr)
	return snapshot
}

func openAIURL(endpoint string, period meter.Period, page string, groupByModel bool) string {
	values := url.Values{}
	values.Set("start_time", strconv.FormatInt(period.Start.Unix(), 10))
	values.Set("end_time", strconv.FormatInt(period.End.Unix(), 10))
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

func fetchOpenAICosts(ctx context.Context, client httpClient, base string, headers map[string]string, period meter.Period) ([]openAICostResponse, error) {
	rawURL := openAIURL(base+"/v1/organization/costs", period, "", false)
	var pages []openAICostResponse
	for {
		var page openAICostResponse
		if err := getJSON(ctx, client, rawURL, headers, &page); err != nil {
			return nil, err
		}
		pages = append(pages, page)
		if !page.HasMore || page.NextPage == "" {
			return pages, nil
		}
		rawURL = openAIURL(base+"/v1/organization/costs", period, page.NextPage, false)
	}
}

func fetchOpenAIUsage(ctx context.Context, client httpClient, base string, headers map[string]string, period meter.Period) ([]openAIUsageResponse, error) {
	rawURL := openAIURL(base+"/v1/organization/usage/completions", period, "", true)
	var pages []openAIUsageResponse
	for {
		var page openAIUsageResponse
		if err := getJSON(ctx, client, rawURL, headers, &page); err != nil {
			return nil, err
		}
		pages = append(pages, page)
		if !page.HasMore || page.NextPage == "" {
			return pages, nil
		}
		rawURL = openAIURL(base+"/v1/organization/usage/completions", period, page.NextPage, true)
	}
}

type openAICostResponse struct {
	HasMore  bool   `json:"has_more"`
	NextPage string `json:"next_page"`
	Data     []struct {
		Results []struct {
			Amount struct {
				Value    float64 `json:"value"`
				Currency string  `json:"currency"`
			} `json:"amount"`
		} `json:"results"`
	} `json:"data"`
}

type openAIUsageResponse struct {
	HasMore  bool   `json:"has_more"`
	NextPage string `json:"next_page"`
	Data     []struct {
		Results []struct {
			InputTokens      float64 `json:"input_tokens"`
			OutputTokens     float64 `json:"output_tokens"`
			NumModelRequests float64 `json:"num_model_requests"`
			Model            string  `json:"model"`
		} `json:"results"`
	} `json:"data"`
}

func addKnown(value meter.Value, amount float64, unit string) meter.Value {
	if value.State != meter.Known {
		return meter.KnownValue(amount, unit)
	}
	value.Value += amount
	return value
}

func sortedModels(models map[string]*meter.ModelUsage) []meter.ModelUsage {
	result := make([]meter.ModelUsage, 0, len(models))
	for id, model := range models {
		if id == "" {
			model.ID = "unknown model"
		}
		result = append(result, *model)
	}
	sort.Slice(result, func(i, j int) bool {
		left := result[i].InputTokens.Value + result[i].OutputTokens.Value
		right := result[j].InputTokens.Value + result[j].OutputTokens.Value
		if left == right {
			return result[i].ID < result[j].ID
		}
		return left > right
	})
	return result
}

func budgetValue(value float64) meter.Value {
	if value <= 0 {
		return meter.UnsupportedValue("set monthly_budget_usd in config")
	}
	result := meter.KnownValue(value, "USD")
	result.Note = "User-configured monthly budget"
	return result
}

func addFetchIssues(snapshot *meter.Snapshot, errs ...error) {
	failed := 0
	for _, err := range errs {
		if err != nil {
			failed++
			snapshot.Issues = append(snapshot.Issues, fmt.Sprintf("%v", err))
		}
	}
	if failed == len(errs) {
		snapshot.Status = meter.Failed
	} else if failed > 0 {
		snapshot.Status = meter.Partial
	}
}
