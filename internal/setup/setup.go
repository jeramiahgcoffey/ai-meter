package setup

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/jeramiahgcoffey/ai-meter/internal/config"
)

type Account struct {
	Kind             string
	Label            string
	ID               string
	CredentialSource string
	CredentialRef    string
	MonthlyBudgetUSD float64
}

func Save(path string, account Account) error {
	provider, err := account.provider()
	if err != nil {
		return err
	}
	current, err := config.ReadFile(path)
	if err != nil {
		return err
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
	return config.Write(path, current)
}

func (account Account) provider() (config.Provider, error) {
	kind := strings.ToLower(strings.TrimSpace(account.Kind))
	if kind != "openai" && kind != "anthropic" {
		return config.Provider{}, fmt.Errorf("provider must be openai or anthropic")
	}
	if strings.TrimSpace(account.Label) == "" {
		return config.Provider{}, fmt.Errorf("label is required")
	}
	if strings.TrimSpace(account.ID) == "" {
		return config.Provider{}, fmt.Errorf("account id is required")
	}
	if strings.TrimSpace(account.CredentialRef) == "" {
		return config.Provider{}, fmt.Errorf("credential reference is required")
	}
	if account.MonthlyBudgetUSD < 0 {
		return config.Provider{}, fmt.Errorf("budget must be a positive number")
	}
	provider := config.Provider{
		ID: strings.TrimSpace(account.ID), Kind: kind, Label: strings.TrimSpace(account.Label),
		MonthlyBudgetUSD: account.MonthlyBudgetUSD,
	}
	switch strings.ToLower(strings.TrimSpace(account.CredentialSource)) {
	case "env":
		provider.CredentialEnv = strings.TrimSpace(account.CredentialRef)
	case "file":
		provider.CredentialFile = strings.TrimSpace(account.CredentialRef)
	default:
		return config.Provider{}, fmt.Errorf("credential source must be env or file")
	}
	return provider, nil
}

func Run(input io.Reader, output io.Writer, path string) error {
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
	account := Account{ID: id, Kind: kind, Label: label, CredentialSource: source}
	if strings.EqualFold(source, "file") {
		account.CredentialRef, err = ask(reader, output, "Path to a mode-0600 key file", "")
	} else {
		account.CredentialRef, err = ask(reader, output, "Environment variable", defaultEnv)
	}
	if err != nil {
		return err
	}
	budgetText, err := ask(reader, output, "Monthly budget in USD (blank for none)", "")
	if err != nil {
		return err
	}
	if budgetText != "" {
		account.MonthlyBudgetUSD, err = strconv.ParseFloat(budgetText, 64)
		if err != nil || account.MonthlyBudgetUSD <= 0 {
			return fmt.Errorf("budget must be a positive number")
		}
	}
	if err := Save(path, account); err != nil {
		return err
	}
	fmt.Fprintf(output, "Saved %s to %s\n", account.Label, path)
	if strings.EqualFold(account.CredentialSource, "env") {
		fmt.Fprintf(output, "Set %s before refreshing usage.\n", account.CredentialRef)
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
