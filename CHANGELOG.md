# Changelog

## v0.4.1 - 2026-09-18

- Show GLM plan windows in the account selector and capacity dashboard by treating a GLM-scoped limit as the general scope for Claude homes that run against z.ai.

## v0.4.0 - 2026-09-18

- Add GLM plan capacity for Claude homes that run on z.ai: 5-hour and weekly windows from z.ai's monitor API.
- Retype a discovered profile with a `zai-local` config overlay; the z.ai key comes from `ZAI_API_KEY` or the macOS Keychain service `zai-api-key`.
- Keep local token totals and mark the provider partial when the plan quota is unavailable or the key is missing.

## v0.3.0 - 2026-09-15

- Add an active-capacity dashboard for local subscriptions used in the last five hours.
- Show weekly limits in selector summaries, including both Claude's general and Fable 7-day limits.
- Sort account and settings lists alphabetically by label.
- Align selector quota fields and color low remaining capacity orange or red.
- Preserve scoped Codex limits and record recent activity from local usage events.

## v0.2.0 - 2026-09-14

- Add an in-app settings page and API account wizard.
- Reload providers and usage after a settings change without restarting.
- Use an adaptive light and dark terminal palette.
- Align status, account statistics, and quota bars across the dashboard.
- Add `ai-meter --version`.
- Add contributor, security, ownership, issue, pull request, and dependency-update files.
- Run formatting, vet, race tests, and builds in CI.

## v0.1.2 - 2026-09-14

- Sort base Codex and Claude profiles above API accounts and alternate profiles.
- Label base profiles `Codex` and `Claude` without the word `default`.
- Keep selector truncation from splitting Unicode reset symbols.

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
