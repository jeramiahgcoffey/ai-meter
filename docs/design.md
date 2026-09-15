# Snapshot design

## Problem

AI providers disagree on units, billing windows, account scopes, and endpoint availability. Consumer subscriptions and API organizations are often separate products. A lowest-common-denominator number would hide those differences and show confident but false comparisons.

## Usage

The TUI and JSON command both ask one `Collector` for a `Dashboard`:

```go
dashboard := collector.Collect(ctx, meter.Period{Start: start, End: now})
```

A provider adapter implements one operation:

```go
type Provider interface {
	ID() string
	Fetch(context.Context, Period) Snapshot
}
```

## Shape

Each adapter returns an immutable provider snapshot. `Value.State` distinguishes `known`, `unsupported`, and `unavailable`, so a missing limit cannot become a zero limit. Values retain their unit and a short derivation note. `Snapshot.Models` keeps provider model IDs and per-model measurements. The collector runs adapters concurrently, stores each successful snapshot in its own cache file, and replaces a failed result with stale data when possible.

The `Collector` interface is small. It hides provider concurrency, partial failure, sorting, cache writes, and stale fallback. Provider wire types stay inside each adapter. The TUI receives only a complete `Dashboard` and does not interpret HTTP payloads.

Cache writes use a mode `0600` temporary file, `fsync`, and atomic rename. Each provider owns one file, so concurrent adapters do not share a write target.

`config.Resolve` owns discovery and config precedence. It combines admin-key environment variables, discovered Codex and Claude homes, the default file, the current directory file, `AI_METER_CONFIG`, and repeated `--config` flags. Config files can include other files. A later definition overrides the non-empty fields on an earlier provider with the same ID. Local homes become credential-free providers. `CodexLocal` passes the home path to the installed Codex app server for current limits, but `ai-meter` never parses the OAuth file or passes it to an organization adapter.

## Synthesis decision

Two designs were compared. The snapshot design became the base because it provides the latest dashboard with one storage operation per provider. An event-ledger design offered better history and correction tracking, but SQLite transactions, replay, and stable event identities would make the first release much larger.

Three ideas from the ledger design remain: values carry derivation notes, provider adapters have fixture-based contract tests, and offline reads are explicit. Long-term history can move to a ledger after real provider payloads establish the required correction rules.

The version 0.2 extension kept the snapshot design. A generic provider-discovery registry was considered, but two adapters do not justify another public interface. Config discovery stays in one module until a third provider shows which parts truly vary.

Two independent cross-judge runs did not return before their timeout. The direct rubric comparison selected the smaller config resolver and kept provider-owned discovery descriptors as a later option.

The subscription extension compared a generic recursive JSON reader with typed Codex and Claude readers. The typed design won because provider logs contain cumulative counters, nested copies of usage, and arbitrary message content. Each reader decodes only its provider's metadata envelope. Codex response IDs and Claude message IDs remove copied records. Unchanged files reuse an in-memory parse result during TUI refresh.

The active-capacity extension compared a dedicated dashboard with an overview embedded above the account list. The dedicated dashboard won because the overview concerns every active subscription, while the account list and detail pane concern one selected provider. `Snapshot.LastActivityAt` records the newest accepted local usage event. `meter.ActiveSubscriptions` selects recent accounts and their applicable limits, so a later scheduler can use the same policy without importing TUI code. The selector and the dashboard share one remaining-capacity color scale.

## Tradeoffs accepted

- We store only the latest successful snapshot in exchange for small, atomic cache files.
- We support compile-time adapters in exchange for typed Go code and one binary.
- We keep provider-specific gaps visible in exchange for honest comparisons.
- We use environment references or mode `0600` credential files. An operating system keyring is a later addition.
- Local totals cover records written on this machine in the selected period, in exchange for running without provider admin access.

## Alternatives considered

An append-only claim ledger makes history auditable, but it requires deduplication, correction, migration, and retention rules. A split interface with separate usage, cost, plan, and limit methods makes partial failure the caller's problem. Provider-owned TUI rows prevent one responsive and accessible layout.

## Risks

Provider report and local JSONL schemas can change. Typed fixture tests catch known schema drift, and unknown records are ignored. Some consumer subscriptions expose quota only in a web page. Browser scraping would be brittle, so ai-meter reports that quota as unsupported instead of guessing.
