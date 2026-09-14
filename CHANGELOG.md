# Changelog

## v0.1.1 - 2026-09-14

- Add remaining-capacity bars to the usage-limit detail view.
- Show the time until each limit resets in selector summaries.
- Format reset countdowns as days, hours, and minutes.
- Keep Fable visible by widening the selector when terminal space permits and removing duplicate reset times.

## v0.1.0 - 2026-09-14

- Show live Codex 5-hour and 7-day subscription limits through the existing Codex login.
- Show Claude 5-hour, 7-day, and Fable 7-day limits from the local Claude Code cache.
- Discover default and alternate Codex and Claude profiles automatically.
- Combine subscription limits, observed model usage, organization API usage, and costs in one compact TUI.
- Support layered configuration files, interactive setup, diagnostics, JSON output, and offline cached snapshots.
- Keep OAuth credentials out of snapshots, cache files, and diagnostic output.
