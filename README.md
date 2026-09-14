# ai-meter

`ai-meter` puts AI spend and usage in one terminal dashboard. It supports OpenAI and Anthropic organization accounts, plus local Codex and Claude subscription usage metadata when those tools have written JSONL logs. It keeps dollars, tokens, requests, and provider-specific details separate, and it marks missing provider data instead of turning missing values into zero.

![Terminal dashboard concept](docs/dashboard.txt)

## Run the dashboard

Preview the UI without credentials:

```sh
go run ./cmd/ai-meter --demo
```

`ai-meter` detects `OPENAI_ADMIN_KEY` and `ANTHROPIC_ADMIN_KEY`. It also detects local `~/.codex`, `~/.codex-*`, `~/.claude`, and `~/.claude-*` homes when they contain the client marker and session directory. Use either admin variable to add organization API reporting without a config file:

```sh
export OPENAI_ADMIN_KEY='...'
export ANTHROPIC_ADMIN_KEY='...'
go run ./cmd/ai-meter
```

If no usage source is available, an interactive terminal asks whether you want to run admin API setup. You can also start setup directly:

```sh
go run ./cmd/ai-meter setup
```

Write another config file with `--config`:

```sh
go run ./cmd/ai-meter setup --config ./work.ai-meter.json
go run ./cmd/ai-meter --config ./work.ai-meter.json
```

Repeat `--config` to merge several files. A later file overlays non-empty fields when it uses the same provider `id`, so a project file can set only a label or budget for an auto-detected provider:

```sh
go run ./cmd/ai-meter \
  --config ~/.config/ai-meter/work.json \
  --config ./project.ai-meter.json
```

`AI_METER_CONFIG` accepts the operating system path-list format. A config can also include files relative to its own location:

```json
{
  "include": ["team.json"],
  "providers": []
}
```

Resolution uses this order. Later entries win:

1. `OPENAI_ADMIN_KEY` and `ANTHROPIC_ADMIN_KEY`
2. The default config file
3. `.ai-meter.json` in the current directory
4. Files in `AI_METER_CONFIG`
5. Repeated `--config` flags

The default config path is the operating system config directory plus `ai-meter/config.json`. A config file lets you set labels, base URLs for testing, and monthly budgets:

```json
{
  "providers": [
    {
      "id": "openai-work",
      "kind": "openai",
      "label": "OpenAI work",
      "credential_env": "OPENAI_ADMIN_KEY",
      "monthly_budget_usd": 120
    },
    {
      "id": "anthropic-personal",
      "kind": "anthropic",
      "label": "Claude API",
      "credential_env": "ANTHROPIC_ADMIN_KEY",
      "monthly_budget_usd": 75
    }
  ]
}
```

For example, this project-local overlay keeps the detected `OPENAI_ADMIN_KEY` credential and only adds a budget:

```json
{
  "providers": [
    {
      "id": "openai",
      "monthly_budget_usd": 120
    }
  ]
}
```

Detected subscription homes also accept label overlays. The provider IDs come from `config doctor`:

```json
{
  "providers": [
    {"id": "codex-local-alt", "label": "Codex personal"},
    {"id": "claude-local-alt", "label": "Claude personal"}
  ]
}
```

Use `credential_file` instead of `credential_env` if you keep an admin key in a mode `0600` file. Relative credential paths resolve from the config file. `ai-meter` refuses credential files that are readable by other users.

The config stores a credential reference, never the secret. Press `r` to refresh. Use the arrow keys or `j` and `k` to select a provider. Press `Enter` for model details in an 80-column terminal, and press `q` to quit.

Inspect every discovered source and merged config file without printing credentials:

```sh
go run ./cmd/ai-meter config doctor
```

## Use it in scripts

Print the same normalized snapshot as JSON:

```sh
go run ./cmd/ai-meter --json
```

Read the most recent snapshots without calling a provider:

```sh
go run ./cmd/ai-meter --offline
```

Successful provider responses are cached as mode `0600` files in the operating system cache directory. A failed refresh uses cached data when one exists and marks the row `cached`.

## Know what the dashboard measures

This version reads API organization data and local subscription usage metadata. It does not scrape consumer dashboards or read message content. The detail pane and JSON output include a row for each provider model, including Claude Fable when the account or local logs report Fable usage. Provider-specific details, such as Anthropic cache-read and cache-creation input tokens, appear under `details`.

- OpenAI uses the organization [Usage API and Costs endpoint](https://developers.openai.com/api/reference/resources/admin/subresources/organization/subresources/usage). Both require an admin key. Usage is grouped by the provider model ID.
- Anthropic uses the organization [Messages Usage Report](https://platform.claude.com/docs/en/api/admin-api/usage-cost/get-messages-usage-report) and [Cost Report](https://platform.claude.com/docs/en/api/admin-api/usage-cost/get-cost-report). Both require an Admin API key.
- Codex local rows ask the installed Codex app server for current subscription limits through `account/rateLimits/read`. The app server uses the existing login for each detected Codex home. If several buckets cover the same duration, the dashboard shows the bucket with the least capacity left. Local session logs provide per-model token totals and an offline quota fallback. `ai-meter` checks that `auth.json` exists but never parses or copies its credentials.
- Claude local rows read the 5-hour, 7-day, and Fable 7-day subscription limits cached by Claude Code. They also read typed assistant usage records, deduplicate copied response IDs, and show model and cache details. Claude Pro is a separate consumer subscription and [does not include API usage](https://support.anthropic.com/en/articles/8325606-what-is-the-pro-plan).

Local token totals mean "observed on this machine." They can omit work done on another computer, and they are not invoices. Codex limit percentages come from the account service and cover the detected subscription. Parsers retain only timestamps, model IDs, token counters, response IDs used for deduplication, and Codex rate-limit fields. Prompt text, responses, tool payloads, and OAuth values never enter a snapshot or cache file.

The clean next provider is GitHub Copilot because GitHub now exposes [personal and organization AI-credit billing endpoints](https://docs.github.com/en/rest/billing/usage). Gemini needs a Google Cloud Billing adapter, which has a broader credential and project model.

## Test and build

```sh
go test ./...
go build ./cmd/ai-meter
```

Provider tests use local HTTP fixtures. They check authentication headers and wire-to-domain translation without sending real requests.
