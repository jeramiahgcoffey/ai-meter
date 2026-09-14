# Settings design

## Problem

The dashboard originally captured a fixed provider collector at startup. A settings page could write a config file, but a new account would remain unavailable until the user restarted the program. The design also has to keep credential values outside the TUI state and preserve atomic config writes.

## Usage

The user presses `s` from the dashboard, reviews display-safe provider sources, and presses `a` to add an API account. Saving the account rebuilds the collector and refreshes usage without restarting the process.

The application supplies the UI with two callbacks:

```go
ui.WithSettings(ui.SettingsController{
	Load: loadDisplaySafeSettings,
	Save: saveProviderReferenceAndRebuildCollector,
})
```

The UI sends a `ui.ProviderDraft` to `Save`. The application converts the draft to `setup.Account`. Both the TUI wizard and the command-line wizard call `setup.Save`.

## Shape

`internal/ui/settings.go` owns the settings and wizard states, key handling, and rendering. Its controller exposes only `Load` and `Save`. The UI cannot read or write config files directly.

`internal/setup` owns account validation, replacement by ID, and atomic persistence. `cmd/ai-meter` adapts between UI drafts and setup accounts. After a successful save, the application resolves all config layers again and replaces the collector captured by the reload closure.

Credential references are strings because users must enter an environment variable name or a file path. Credential values never enter `ProviderDraft`, `SettingsSnapshot`, or the Bubble Tea model.

## Synthesis decision

An embedded state machine became the base because it refreshes the running dashboard after a save. The separate-program design contributed the narrow persistence boundary, which keeps `setup.Save` useful to both interfaces. A UI that imported `internal/config` directly was rejected because it would spread config validation and secret-handling rules into rendering code.

## Tradeoffs accepted

- We accept a synchronous local config save in exchange for a small controller with no background write state.
- We support API organization accounts in the wizard. Local OAuth profiles remain automatic.
- We replace accounts by ID instead of adding edit and delete modes to the first settings release.

## Alternatives considered

A separate `ai-meter settings` program had fewer UI states, but users would have to restart the dashboard. A controller with separate validate, write, rebuild, and refresh methods exposed execution order to the UI and was rejected as a shallow interface.

## Open questions and risks

- Should a later release support deleting API accounts from the settings page?
- Should label overrides for detected OAuth profiles get a smaller edit flow?

## Next implementation step

Add edit and delete actions only after account replacement has real usage feedback.
