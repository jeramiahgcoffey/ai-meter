# Configure accounts in the TUI

Run `ai-meter`, then press `s` to open **Settings**. The page lists the config files and provider sources that `ai-meter` resolved. It shows credential references such as `$OPENAI_ADMIN_KEY`, but it never shows credential values.

Codex and Claude profiles appear automatically after their command-line tools create a local login. You do not need to add those profiles in the wizard.

## Add an API organization account

1. Press `a` on the **Settings** page.
2. Choose OpenAI or Anthropic.
3. Enter a display name and a unique account ID.
4. Choose an environment variable or a mode `0600` key file.
5. Enter the credential reference. Do not enter the credential value.
6. Enter an optional monthly budget.
7. Review the account, then press `Enter` to save it.

The wizard writes to the default config file. Saving an existing account ID replaces that account. After the save, `ai-meter` reloads the config and refreshes the dashboard.

Use `Esc` to move back one step. Press `Ctrl+C` to quit from the wizard.
