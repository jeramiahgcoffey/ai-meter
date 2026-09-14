# Contributing

## Set up the project

Install Go 1.24 or later, then clone the repository:

```sh
git clone https://github.com/jeramiahgcoffey/ai-meter.git
cd ai-meter
make check
make build
```

Run `make demo` to inspect the TUI without provider credentials.

## Make a change

Create a branch and keep each pull request focused on one problem. Add a test for behavior changes. Provider tests must use local fixtures and test servers. Never commit API keys, OAuth values, account IDs, prompts, responses, or copied session logs.

Run these checks before opening a pull request:

```sh
make check
make build
```

Describe the user-visible result in the pull request. Include terminal dimensions for layout changes and sanitized diagnostics for provider changes.

## Add a provider

Keep transport types inside `internal/providers`. Convert provider responses into the types in `internal/meter`, and represent missing values as `unsupported` or `unavailable`. Do not convert missing values to zero.

Document the credential type and the data source. Subscription OAuth and organization API keys are separate account types and must remain separate in the UI.
