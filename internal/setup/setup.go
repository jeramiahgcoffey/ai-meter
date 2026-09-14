package setup

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/jeramiahgcoffey/ai-meter/internal/config"
)

func Run(input io.Reader, output io.Writer, path string) error {
	current, err := config.ReadFile(path)
	if err != nil {
		return err
	}
	reader := bufio.NewReader(input)
	fmt.Fprintln(output, "ai-meter setup")
	fmt.Fprintln(output, "This stores a credential reference, never the key itself.")

	kind, err := ask(reader, output, "Provider (openai or anthropic)", "openai")
	if err != nil {
		return err
	}
	kind = strings.ToLower(kind)
	if kind != "openai" && kind != "anthropic" {
		return fmt.Errorf("provider must be openai or anthropic")
	}
	defaultLabel := "OpenAI API"
	defaultEnv := "OPENAI_ADMIN_KEY"
	if kind == "anthropic" {
		defaultLabel, defaultEnv = "Anthropic API", "ANTHROPIC_ADMIN_KEY"
	}
	label, err := ask(reader, output, "Label", defaultLabel)
	if err != nil {
		return err
	}
	id, err := ask(reader, output, "Account id", kind)
	if err != nil {
		return err
	}
	source, err := ask(reader, output, "Credential source (env or file)", "env")
	if err != nil {
		return err
	}
	provider := config.Provider{ID: id, Kind: kind, Label: label}
	if strings.EqualFold(source, "file") {
		provider.CredentialFile, err = ask(reader, output, "Path to a mode-0600 key file", "")
	} else {
		provider.CredentialEnv, err = ask(reader, output, "Environment variable", defaultEnv)
	}
	if err != nil {
		return err
	}
	budgetText, err := ask(reader, output, "Monthly budget in USD (blank for none)", "")
	if err != nil {
		return err
	}
	if budgetText != "" {
		provider.MonthlyBudgetUSD, err = strconv.ParseFloat(budgetText, 64)
		if err != nil || provider.MonthlyBudgetUSD <= 0 {
			return fmt.Errorf("budget must be a positive number")
		}
	}
	updated := false
	for i := range current.Providers {
		if current.Providers[i].ID == provider.ID {
			current.Providers[i] = provider
			updated = true
			break
		}
	}
	if !updated {
		current.Providers = append(current.Providers, provider)
	}
	if err := config.Write(path, current); err != nil {
		return err
	}
	fmt.Fprintf(output, "Saved %s to %s\n", provider.Label, path)
	if provider.CredentialEnv != "" {
		fmt.Fprintf(output, "Set %s before refreshing usage.\n", provider.CredentialEnv)
	}
	return nil
}

func ask(reader *bufio.Reader, output io.Writer, label, fallback string) (string, error) {
	if fallback == "" {
		fmt.Fprintf(output, "%s: ", label)
	} else {
		fmt.Fprintf(output, "%s [%s]: ", label, fallback)
	}
	value, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	if value == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	return value, nil
}
