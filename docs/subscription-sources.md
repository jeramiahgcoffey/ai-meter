# Subscription source design

## Problem

Codex and Claude subscription logins are not organization admin credentials. Codex provides current account limits through its local app server, and both clients write usage metadata locally. ai-meter turns each normal or alternate client home into a dashboard row without parsing bearer tokens or message content. The existing `meter.Provider` and `Snapshot` types continue to support the admin API adapters.

## Usage

No setup is required for local subscriptions:

```sh
ai-meter config doctor
ai-meter
ai-meter --json
```

`config doctor` reports IDs such as `codex-local`, `codex-local-alt`, `claude-local`, and `claude-local-alt`. A config overlay can rename one without restating its source:

```json
{"providers":[{"id":"codex-local-alt","label":"Codex personal"}]}
```

## Shape

`config.Resolve` discovers sibling client homes and emits credential-free providers with a `local_root`. `main.makeProviders` constructs either `CodexLocal` or `ClaudeLocal`. Both implement the existing one-method `meter.Provider` interface, so concurrency, snapshot caching, JSON, and TUI rendering need no second path.

The parsers use provider-specific structs. Codex accepts `turn_context`, `token_usage_record`, and `event_msg/token_count` envelopes. It prefers per-response usage and uses positive cumulative deltas only for older files without response records. Claude accepts top-level assistant envelopes and reads only `message.model`, `message.id`, and `message.usage`. Stable response IDs deduplicate copied history and subagent records.

`CodexLocal` starts the installed `codex app-server` for each detected Codex home. It initializes the JSON-lines connection and sends `account/rateLimits/read`. The response can contain several named limit buckets. If several buckets cover one duration, ai-meter keeps the bucket with the highest used percentage. A request failure falls back to the latest unexpired values from local `token_count` events.

`ClaudeLocal` reads `cachedUsageUtilization` from the profile's `.claude.json`. Claude Code writes the account-wide 5-hour and 7-day utilization to this object. The `limits` array contains model-scoped limits such as Fable's 7-day utilization. ai-meter reads only the percentages, reset times, model IDs, display names, and fetch time.

`ZaiLocal` covers a Claude home that runs on z.ai's Anthropic-compatible GLM endpoint. Such a home has no cached Anthropic limits, so a config overlay retypes the discovered profile and the provider composes the Claude session scan with z.ai's monitor API:

```json
{"providers":[{"id":"claude-local-zai","kind":"zai-local","label":"GLM"}]}
```

The key is resolved the same way the `claude-zai` launcher resolves it: `ZAI_API_KEY` first, then the macOS Keychain service `zai-api-key`. It is sent only to `GET https://api.z.ai/api/monitor/usage/quota/limit` and never stored or logged. The response's limit buckets map onto the shared usage windows — unit 3 with number 5 is the 5-hour cycle and unit 6 with number 1 is the weekly cycle, for both credit and token plans. A missing key or a failed quota request marks the provider partial and keeps the local token totals.

Each provider caches parsed metadata by file path, size, and modification time for the life of the process. A refresh reuses unchanged files. Context cancellation is checked during directory walking and line scanning. Files older than the requested period are skipped by modification time.

Subscription windows remain separate from dollar budgets. The TUI reports capacity left for both providers. Local token totals are machine-observed activity, not invoices.

## Synthesis decision

Candidate A's typed, provider-specific design became the base. The generic candidate compiled and passed synthetic tests, but a real run took 18 seconds and reported trillions of Codex tokens because it recursively summed nested cumulative and per-response values. The cross-judge scored the typed design 27/35 and the generic design 14/35.

The final design kept the generic candidate's working admin adapters, config precedence, additive model/details fields, cache compatibility, and TUI. It rejected recursive `any` traversal, missing-timestamp substitution, whole-home scans, synthetic quota key names, and credential checks for local providers. File metadata caching, actual Codex quota fields, response-ID deduplication, and context cancellation were added during synthesis.

## Tradeoffs accepted

- We use the installed Codex app server for current limits instead of reading or sending OAuth tokens ourselves.
- We accept machine-local token totals because Codex and Claude do not expose subscription-wide model totals through this interface.
- We rescan a changed file from its beginning in exchange for a small cache with no persistent index migration.
- We retain unknown Codex model rows when old records cannot be attributed honestly.
- We use Claude Code's last cached subscription limits instead of scraping the `/usage` screen.
- We read the z.ai key from the environment or the macOS Keychain to call z.ai's monitor API because z.ai has no local client process that owns credentials on our behalf; the key travels only to the monitor endpoint.

## Alternatives considered

Calling subscription endpoints directly with saved OAuth bearer tokens would couple ai-meter to token storage and refresh behavior. The installed Codex app server already owns that work and exposes the account limit request to local clients. A generic recursive JSON parser has a smaller implementation, but it cannot distinguish nested copies, cumulative counters, or message content from usage metadata. A persistent SQLite index would make first startup after a restart faster, but the current typed scan completes in under two seconds on the target machine and in-memory reuse makes TUI refresh cheap.

## Open questions and risks

- Will future client versions retain the accepted event envelopes and stable response IDs?
- Should a later release persist the file index for machines with larger histories?
- Will Anthropic publish a supported subscription quota API that can replace Claude Code's local cache?

## Next implementation step

Add new provider adapters only when their supported local metadata or reporting API can map into `Snapshot` without weakening the privacy boundary.
