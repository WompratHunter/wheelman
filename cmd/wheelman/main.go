// Command wheelman is an interactive TUI for querying Kubernetes pod logs
// across configured Apps with natural-language Queries.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/WompratHunter/wheelman/internal/cluster"
	"github.com/WompratHunter/wheelman/internal/config"
	"github.com/WompratHunter/wheelman/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "wheelman:", err)
		os.Exit(1)
	}
}

func run() error {
	defaultConfig, err := defaultConfigPath()
	if err != nil {
		return err
	}
	configPath := flag.String("config", defaultConfig, "path to the configured Apps file")
	flag.Parse()

	client, err := cluster.NewKubeClusterClient()
	if err != nil {
		return fmt.Errorf("connecting to cluster: %w", err)
	}

	configurator := config.NewConfigurator(client, config.NewFileStore(*configPath))
	apps, err := configurator.ListApps()
	if err != nil {
		return fmt.Errorf("loading %s: %w", *configPath, err)
	}

	start := tui.ScreenQuery
	if len(apps) == 0 {
		start = tui.ScreenApps
	}

	_, err = tea.NewProgram(tui.New(configurator, client, start), tea.WithAltScreen()).Run()
	return err
}

func defaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating home directory: %w", err)
	}
	return filepath.Join(home, ".config", "wheelman", "apps.json"), nil
}
