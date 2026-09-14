package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Provider struct {
	ID               string  `json:"id"`
	Kind             string  `json:"kind"`
	Label            string  `json:"label"`
	CredentialEnv    string  `json:"credential_env,omitempty"`
	CredentialFile   string  `json:"credential_file,omitempty"`
	LocalRoot        string  `json:"local_root,omitempty"`
	BaseURL          string  `json:"base_url,omitempty"`
	MonthlyBudgetUSD float64 `json:"monthly_budget_usd,omitempty"`
}

type Config struct {
	Include   []string   `json:"include,omitempty"`
	Providers []Provider `json:"providers"`
}

type Detection struct {
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Usable bool   `json:"usable"`
	Note   string `json:"note"`
}

type Resolution struct {
	Config     Config      `json:"config"`
	Files      []string    `json:"files"`
	Detections []Detection `json:"detections"`
}

type ResolveOptions struct {
	Paths     []string
	HomeDir   string
	WorkDir   string
	LookupEnv func(string) (string, bool)
}

func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(dir, "ai-meter", "config.json")
}

func CacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ".ai-meter-cache"
	}
	return filepath.Join(dir, "ai-meter")
}

func Resolve(options ResolveOptions) (Resolution, error) {
	if options.LookupEnv == nil {
		options.LookupEnv = os.LookupEnv
	}
	if options.HomeDir == "" {
		options.HomeDir, _ = os.UserHomeDir()
	}
	if options.WorkDir == "" {
		options.WorkDir, _ = os.Getwd()
	}
	discovered, detections := discover(options)
	merged := map[string]Provider{}
	order := []string{}
	mergeProviders(merged, &order, discovered.Providers)
	result := Resolution{Detections: detections}
	visiting, loaded := map[string]bool{}, map[string]bool{}
	for _, path := range []string{DefaultPath(), filepath.Join(options.WorkDir, ".ai-meter.json")} {
		path = expandPath(path, options.HomeDir, options.WorkDir)
		if err := loadRecursive(path, options.HomeDir, merged, &order, &result.Files, visiting, loaded, false); err != nil {
			return Resolution{}, err
		}
	}
	if value, ok := options.LookupEnv("AI_METER_CONFIG"); ok {
		for _, path := range filepath.SplitList(value) {
			if strings.TrimSpace(path) == "" {
				continue
			}
			path = expandPath(path, options.HomeDir, options.WorkDir)
			if err := loadRecursive(path, options.HomeDir, merged, &order, &result.Files, visiting, loaded, true); err != nil {
				return Resolution{}, err
			}
		}
	}
	for _, path := range options.Paths {
		path = expandPath(path, options.HomeDir, options.WorkDir)
		if err := loadRecursive(path, options.HomeDir, merged, &order, &result.Files, visiting, loaded, true); err != nil {
			return Resolution{}, err
		}
	}
	for _, id := range order {
		if provider, ok := merged[id]; ok {
			result.Config.Providers = append(result.Config.Providers, provider)
		}
	}
	if err := Validate(result.Config); err != nil {
		return Resolution{}, err
	}
	return result, nil
}

func loadRecursive(path, home string, merged map[string]Provider, order, files *[]string, visiting, loaded map[string]bool, required bool) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if loaded[abs] {
		return nil
	}
	if visiting[abs] {
		return fmt.Errorf("config include cycle at %s", abs)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !required {
			return nil
		}
		return fmt.Errorf("read %s: %w", abs, err)
	}
	visiting[abs] = true
	var cfg Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return fmt.Errorf("parse %s: %w", abs, err)
	}
	for _, include := range cfg.Include {
		include = expandPath(include, home, filepath.Dir(abs))
		if err := loadRecursive(include, home, merged, order, files, visiting, loaded, true); err != nil {
			return err
		}
	}
	for i := range cfg.Providers {
		if cfg.Providers[i].CredentialFile != "" {
			cfg.Providers[i].CredentialFile = expandPath(cfg.Providers[i].CredentialFile, home, filepath.Dir(abs))
		}
		if cfg.Providers[i].LocalRoot != "" {
			cfg.Providers[i].LocalRoot = expandPath(cfg.Providers[i].LocalRoot, home, filepath.Dir(abs))
		}
	}
	mergeProviders(merged, order, cfg.Providers)
	delete(visiting, abs)
	loaded[abs] = true
	*files = append(*files, abs)
	return nil
}

func mergeProviders(target map[string]Provider, order *[]string, providers []Provider) {
	for _, provider := range providers {
		existing, exists := target[provider.ID]
		if !exists {
			*order = append(*order, provider.ID)
			target[provider.ID] = provider
			continue
		}
		target[provider.ID] = mergeProvider(existing, provider)
	}
}

func mergeProvider(base, overlay Provider) Provider {
	if overlay.Kind != "" {
		base.Kind = overlay.Kind
	}
	if overlay.Label != "" {
		base.Label = overlay.Label
	}
	if overlay.CredentialEnv != "" {
		base.CredentialEnv = overlay.CredentialEnv
		base.CredentialFile = ""
		base.LocalRoot = ""
	}
	if overlay.CredentialFile != "" {
		base.CredentialFile = overlay.CredentialFile
		base.CredentialEnv = ""
		base.LocalRoot = ""
	}
	if overlay.LocalRoot != "" {
		base.LocalRoot = overlay.LocalRoot
		base.CredentialEnv = ""
		base.CredentialFile = ""
	}
	if overlay.BaseURL != "" {
		base.BaseURL = overlay.BaseURL
	}
	if overlay.MonthlyBudgetUSD != 0 {
		base.MonthlyBudgetUSD = overlay.MonthlyBudgetUSD
	}
	return base
}

func Validate(cfg Config) error {
	seen := map[string]bool{}
	for i, provider := range cfg.Providers {
		if provider.ID == "" || provider.Kind == "" || provider.Label == "" {
			return fmt.Errorf("provider %d requires id, kind, and label", i+1)
		}
		if seen[provider.ID] {
			return fmt.Errorf("duplicate provider id %q", provider.ID)
		}
		seen[provider.ID] = true
		if localProviderKind(provider.Kind) {
			if provider.CredentialEnv != "" || provider.CredentialFile != "" {
				return fmt.Errorf("provider %q must not set credentials for local subscription usage", provider.ID)
			}
			if provider.LocalRoot == "" {
				return fmt.Errorf("provider %q requires local_root", provider.ID)
			}
			continue
		}
		if (provider.CredentialEnv == "") == (provider.CredentialFile == "") {
			return fmt.Errorf("provider %q requires exactly one of credential_env or credential_file", provider.ID)
		}
	}
	return nil
}

func localProviderKind(kind string) bool {
	switch strings.ToLower(kind) {
	case "codex-local", "claude-local":
		return true
	default:
		return false
	}
}

func ResolveCredential(provider Provider) (string, error) {
	if localProviderKind(provider.Kind) {
		return "", nil
	}
	if provider.CredentialEnv != "" {
		value := os.Getenv(provider.CredentialEnv)
		if value == "" {
			return "", fmt.Errorf("%s is not set", provider.CredentialEnv)
		}
		return value, nil
	}
	info, err := os.Stat(provider.CredentialFile)
	if err != nil {
		return "", err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("credential file %s must use mode 0600", provider.CredentialFile)
	}
	data, err := os.ReadFile(provider.CredentialFile)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("credential file %s is empty", provider.CredentialFile)
	}
	return value, nil
}

func ReadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, err
	}
	var cfg Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Write(path string, cfg Config) error {
	if err := Validate(cfg); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func expandPath(path, home, relativeTo string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	if !filepath.IsAbs(path) {
		return filepath.Join(relativeTo, path)
	}
	return path
}

func discover(options ResolveOptions) (Config, []Detection) {
	var cfg Config
	var detections []Detection
	for _, descriptor := range providerDescriptors(options) {
		provider := descriptor.DefaultProvider
		if value, ok := options.LookupEnv(provider.CredentialEnv); ok && value != "" {
			cfg.Providers = append(cfg.Providers, provider)
			detections = append(detections, Detection{Kind: provider.Kind, Source: provider.CredentialEnv, Usable: true, Note: "organization reporting credential"})
		}
		for _, name := range descriptor.NonReportingEnvs {
			if value, ok := options.LookupEnv(name); ok && value != "" {
				detections = append(detections, Detection{Kind: provider.Kind, Source: name, Note: "API key found; organization reports may require a separate admin key"})
			}
		}
	}
	localProviders, localDetections := discoverLocalProviders(options.HomeDir)
	cfg.Providers = append(cfg.Providers, localProviders...)
	detections = append(detections, localDetections...)
	return cfg, detections
}

func discoverLocalProviders(home string) ([]Provider, []Detection) {
	var providers []Provider
	var detections []Detection
	for _, root := range matchingProviderHomes(home, ".codex") {
		if _, err := os.Stat(filepath.Join(root, "auth.json")); err != nil || !hasDirectory(root, "sessions") {
			continue
		}
		id := localProviderID("codex-local", root, ".codex")
		providers = append(providers, Provider{ID: id, Kind: "codex-local", Label: localProviderLabel("Codex", root, ".codex"), LocalRoot: root})
		detections = append(detections, Detection{Kind: "codex-local", Source: root, Usable: true, Note: "subscription session metadata; auth values are never read"})
	}
	for _, root := range matchingProviderHomes(home, ".claude") {
		if !hasAny(root, ".credentials.json", "settings.json") || !hasDirectory(root, "projects") {
			continue
		}
		id := localProviderID("claude-local", root, ".claude")
		providers = append(providers, Provider{ID: id, Kind: "claude-local", Label: localProviderLabel("Claude", root, ".claude"), LocalRoot: root})
		detections = append(detections, Detection{Kind: "claude-local", Source: root, Usable: true, Note: "subscription session metadata; auth values are never read"})
	}
	return providers, detections
}

func matchingProviderHomes(home, base string) []string {
	paths := []string{filepath.Join(home, base)}
	matches, _ := filepath.Glob(filepath.Join(home, base+"-*"))
	paths = append(paths, matches...)
	result := paths[:0]
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			result = append(result, path)
		}
	}
	return result
}

func localProviderID(prefix, root, base string) string {
	if filepath.Base(root) == base {
		return prefix
	}
	return prefix + "-" + strings.TrimPrefix(filepath.Base(root), base+"-")
}

func localHomeLabel(path, base string) string {
	if filepath.Base(path) == base {
		return ""
	}
	return strings.ReplaceAll(strings.TrimPrefix(filepath.Base(path), base+"-"), "-", " ")
}

func localProviderLabel(provider, path, base string) string {
	if profile := localHomeLabel(path, base); profile != "" {
		return provider + " " + profile
	}
	return provider
}

func hasAny(root string, names ...string) bool {
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	return false
}

func hasDirectory(root, name string) bool {
	info, err := os.Stat(filepath.Join(root, name))
	return err == nil && info.IsDir()
}

type providerDescriptor struct {
	DefaultProvider  Provider
	NonReportingEnvs []string
}

func providerDescriptors(_ ResolveOptions) []providerDescriptor {
	return []providerDescriptor{
		{
			DefaultProvider:  Provider{ID: "openai", Kind: "openai", Label: "OpenAI API", CredentialEnv: "OPENAI_ADMIN_KEY"},
			NonReportingEnvs: []string{"OPENAI_API_KEY"},
		},
		{
			DefaultProvider:  Provider{ID: "anthropic", Kind: "anthropic", Label: "Anthropic API", CredentialEnv: "ANTHROPIC_ADMIN_KEY"},
			NonReportingEnvs: []string{"ANTHROPIC_API_KEY"},
		},
	}
}
