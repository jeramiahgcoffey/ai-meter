package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jeramiahgcoffey/ai-meter/internal/config"
	"github.com/jeramiahgcoffey/ai-meter/internal/meter"
	"github.com/jeramiahgcoffey/ai-meter/internal/providers"
	"github.com/jeramiahgcoffey/ai-meter/internal/setup"
	"github.com/jeramiahgcoffey/ai-meter/internal/ui"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "setup" {
		runSetup(os.Args[2:])
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "config" && os.Args[2] == "doctor" {
		runDoctor(os.Args[3:])
		return
	}

	var configPaths stringList
	var demo, jsonOutput, offline bool
	flag.Var(&configPaths, "config", "additional JSON config path; repeat to merge")
	flag.BoolVar(&demo, "demo", false, "show representative provider data")
	flag.BoolVar(&jsonOutput, "json", false, "print a JSON snapshot instead of opening the TUI")
	flag.BoolVar(&offline, "offline", false, "read cached snapshots without provider requests")
	flag.Parse()

	period := currentMonth(time.Now())
	var configured []meter.Provider
	var configIssues []string
	if demo {
		configured = providers.DemoProviders()
	} else {
		resolution, err := config.Resolve(config.ResolveOptions{Paths: configPaths})
		if err != nil {
			fatal(err)
		}
		if len(resolution.Config.Providers) == 0 && !jsonOutput && !offline && terminal(os.Stdin) {
			showDetections(resolution.Detections)
			if confirmSetup() {
				if err := setup.Run(os.Stdin, os.Stdout, config.DefaultPath()); err != nil {
					fatal(err)
				}
				resolution, err = config.Resolve(config.ResolveOptions{Paths: configPaths})
				if err != nil {
					fatal(err)
				}
			}
		}
		configIssues = summarizeConfig(resolution)
		configured, err = makeProviders(resolution.Config)
		if err != nil {
			fatal(err)
		}
	}
	collector := meter.NewCollector(configured, &meter.Cache{Dir: config.CacheDir()})
	load := func() meter.Dashboard {
		period = currentMonth(time.Now())
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		var dashboard meter.Dashboard
		if offline {
			dashboard = collector.Offline(period)
		} else {
			dashboard = collector.Collect(ctx, period)
		}
		dashboard.Issues = append(dashboard.Issues, configIssues...)
		return dashboard
	}
	dashboard := load()
	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(dashboard); err != nil {
			fatal(err)
		}
		return
	}
	program := tea.NewProgram(ui.New(dashboard, load), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fatal(err)
	}
}

func makeProviders(cfg config.Config) ([]meter.Provider, error) {
	result := make([]meter.Provider, 0, len(cfg.Providers))
	for _, item := range cfg.Providers {
		switch strings.ToLower(item.Kind) {
		case "openai":
			key, _ := config.ResolveCredential(item)
			result = append(result, &providers.OpenAI{InstanceID: item.ID, Label: item.Label, Key: key, BaseURL: item.BaseURL, BudgetUSD: item.MonthlyBudgetUSD})
		case "anthropic":
			key, _ := config.ResolveCredential(item)
			result = append(result, &providers.Anthropic{InstanceID: item.ID, Label: item.Label, Key: key, BaseURL: item.BaseURL, BudgetUSD: item.MonthlyBudgetUSD})
		case "codex-local":
			result = append(result, &providers.CodexLocal{InstanceID: item.ID, Label: item.Label, Root: item.LocalRoot})
		case "claude-local":
			result = append(result, &providers.ClaudeLocal{InstanceID: item.ID, Label: item.Label, Root: item.LocalRoot})
		default:
			return nil, fmt.Errorf("unsupported provider kind %q", item.Kind)
		}
	}
	return result, nil
}

type stringList []string

func (values *stringList) String() string         { return strings.Join(*values, ",") }
func (values *stringList) Set(value string) error { *values = append(*values, value); return nil }

func summarizeConfig(resolution config.Resolution) []string {
	var issues []string
	if len(resolution.Files) > 0 {
		issues = append(issues, "config files: "+strings.Join(resolution.Files, ", "))
	}
	for _, detection := range resolution.Detections {
		if detection.Usable {
			continue
		}
		issues = append(issues, detection.Source+": "+detection.Note)
	}
	return issues
}

func runSetup(args []string) {
	flags := flag.NewFlagSet("setup", flag.ExitOnError)
	path := flags.String("config", config.DefaultPath(), "config file to create or update")
	_ = flags.Parse(args)
	if err := setup.Run(os.Stdin, os.Stdout, *path); err != nil {
		fatal(err)
	}
}

func runDoctor(args []string) {
	flags := flag.NewFlagSet("config doctor", flag.ExitOnError)
	var paths stringList
	flags.Var(&paths, "config", "additional JSON config path; repeat to merge")
	_ = flags.Parse(args)
	resolution, err := config.Resolve(config.ResolveOptions{Paths: paths})
	if err != nil {
		fatal(err)
	}
	fmt.Println("Config files:")
	if len(resolution.Files) == 0 {
		fmt.Println("  none")
	}
	for _, path := range resolution.Files {
		fmt.Println(" ", path)
	}
	fmt.Println("Provider detection:")
	if len(resolution.Detections) == 0 {
		fmt.Println("  none")
	}
	for _, item := range resolution.Detections {
		state := "needs setup"
		if item.Usable {
			state = "usable"
		}
		fmt.Printf("  %-13s %-11s %s (%s)\n", item.Kind, state, item.Source, item.Note)
	}
	fmt.Println("Resolved providers:")
	if len(resolution.Config.Providers) == 0 {
		fmt.Println("  none")
	}
	for _, provider := range resolution.Config.Providers {
		state := "ready"
		if _, err := config.ResolveCredential(provider); err != nil {
			state = "credential missing"
		}
		fmt.Printf("  %-16s %-18s %s\n", provider.ID, state, provider.Label)
	}
}

func showDetections(detections []config.Detection) {
	for _, item := range detections {
		fmt.Printf("Detected %s at %s. %s\n", item.Kind, item.Source, item.Note)
	}
}

func confirmSetup() bool {
	fmt.Print("No reporting accounts are configured. Run setup now? [Y/n]: ")
	var answer string
	_, _ = fmt.Fscanln(os.Stdin, &answer)
	return answer == "" || strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes")
}

func terminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func currentMonth(now time.Time) meter.Period {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	return meter.Period{Start: start, End: now}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "ai-meter:", err)
	os.Exit(1)
}
