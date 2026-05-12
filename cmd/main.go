package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/LFroesch/logdog/internal/config"
	"github.com/LFroesch/logdog/internal/detector"
	"github.com/LFroesch/logdog/internal/logs"
	"github.com/LFroesch/logdog/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var version = "dev"

var (
	projectPath string
	jsonOutput  bool
	levelFilter string
	searchTerm  string
	pruneDays   int
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "logdog",
		Short: "Logging control plane for viewing, tailing, and bootstrapping logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isatty.IsTerminal(os.Stdin.Fd()) {
				return streamEntries(os.Stdin, logs.Query{Level: levelFilter, Search: searchTerm})
			}
			tui.SetVersion(version)
			p := tea.NewProgram(
				tui.NewModel(resolveProjectPath()),
				tea.WithAltScreen(),
				tea.WithMouseCellMotion(),
			)
			_, err := p.Run()
			return err
		},
	}

	root.Version = version
	root.PersistentFlags().StringVar(&projectPath, "project", "", "directory to scan first")
	root.PersistentFlags().BoolVar(&jsonOutput, "json", false, "emit JSON output for CLI results")
	root.PersistentFlags().StringVarP(&levelFilter, "level", "l", "", "filter by level")
	root.PersistentFlags().StringVarP(&searchTerm, "search", "s", "", "filter by search term")

	root.AddCommand(newInstallCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newFilesCmd())
	root.AddCommand(newCatCmd())
	root.AddCommand(newGrepCmd())
	root.AddCommand(newTailCmd())
	root.AddCommand(newPruneCmd())
	root.AddCommand(newConfigCmd())

	return root
}

func newInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install the generated Go logger into the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir := resolveProjectPath()
			lang := detector.DetectLanguage(projectDir)
			if lang == nil {
				return fmt.Errorf("no supported project detected in %s", projectDir)
			}

			cfg := detector.Config{
				LogLevel:   "INFO",
				OutputDir:  config.ProjectLogDir(projectDir),
				MaxFiles:   30,
				DateFormat: "2006-01-02",
			}
			if err := lang.Install(projectDir, cfg); err != nil {
				return err
			}

			fmt.Printf("installed logger at %s\n", lang.InstalledPath(projectDir))
			fmt.Printf("logs directory: %s\n", config.ProjectLogDir(projectDir))
			return nil
		},
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show discovery and configuration status",
		RunE: func(cmd *cobra.Command, args []string) error {
			store := newStore()
			cfg := store.Config()
			files, err := store.DiscoverFiles()
			if err != nil {
				return err
			}

			fmt.Printf("cwd: %s\n", store.CWD())
			fmt.Printf("config: %s\n", config.ResolvePaths().ConfigFile)
			fmt.Printf("roots: %d\n", len(cfg.Roots)+1)
			fmt.Printf("files: %d\n", len(files))
			if len(files) > 0 {
				fmt.Printf("latest: %s\n", files[0].Path)
			}
			return nil
		},
	}
}

func newFilesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "files",
		Short: "List discovered log files",
		RunE: func(cmd *cobra.Command, args []string) error {
			store := newStore()
			files, err := store.DiscoverFiles()
			if err != nil {
				return err
			}
			for _, file := range files {
				fmt.Printf("%s\t%s\t%s\t%d\n", file.ModTime.Format("2006-01-02 15:04:05"), file.Format, file.Path, file.Size)
			}
			return nil
		},
	}
}

func newCatCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cat [file|-]",
		Short: "Print log entries from a file, latest discovered file, or stdin",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := logs.Query{Level: levelFilter, Search: searchTerm}
			if len(args) > 0 && args[0] == "-" {
				return streamEntries(os.Stdin, query)
			}

			store := newStore()
			path, err := resolveInputFile(store, args)
			if err != nil {
				return err
			}
			entries, err := store.ReadEntries(path, query)
			if err != nil {
				return err
			}
			return writeEntries(entries)
		},
	}
}

func newGrepCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "grep <pattern>",
		Short: "Search discovered log entries or piped input",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := logs.Query{Level: levelFilter, Search: args[0]}
			if !isatty.IsTerminal(os.Stdin.Fd()) {
				return streamEntries(os.Stdin, query)
			}

			store := newStore()
			files, err := store.DiscoverFiles()
			if err != nil {
				return err
			}

			var matches []logs.Entry
			for _, file := range files {
				entries, err := store.ReadEntries(file.Path, query)
				if err != nil {
					continue
				}
				matches = append(matches, entries...)
			}
			return writeEntries(matches)
		},
	}
}

func newTailCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tail [file]",
		Short: "Follow a discovered or explicit log file",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := resolveInputFile(newStore(), args)
			if err != nil {
				return err
			}

			stop := make(chan struct{})
			return logs.TailFile(path, true, func(entry logs.Entry) {
				if levelFilter != "" && !strings.EqualFold(entry.Level, levelFilter) {
					return
				}
				if searchTerm != "" && !matchesSearch(entry, searchTerm) {
					return
				}
				_ = writeEntries([]logs.Entry{entry})
			}, func(line string) {
				entry := logs.Entry{Raw: line, Message: line}
				if searchTerm != "" && !matchesSearch(entry, searchTerm) {
					return
				}
				_ = writeEntries([]logs.Entry{entry})
			}, stop)
		},
	}
}

func newPruneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Delete old log files and empty log directories",
		RunE: func(cmd *cobra.Command, args []string) error {
			store := newStore()
			deleted, err := store.PruneOlderThan(pruneDays)
			if err != nil {
				return err
			}
			fmt.Printf("deleted %d cleanup candidates\n", deleted)
			return nil
		},
	}
	cmd.Flags().IntVarP(&pruneDays, "days", "d", -1, "delete candidates older than this many days (0 = all)")
	return cmd
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and manage logdog configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Ensure()
			if err != nil {
				return err
			}
			data, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return err
			}
			fmt.Printf("config: %s\n", config.ResolvePaths().ConfigFile)
			fmt.Printf("data: %s\n", config.ResolvePaths().DataDir)
			fmt.Println(string(data))
			return nil
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "add-root <dir>",
		Short: "Add a discovery root to config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Ensure()
			if err != nil {
				return err
			}
			for _, existing := range cfg.Roots {
				if samePath(existing, args[0]) {
					fmt.Printf("root already configured: %s\n", args[0])
					return nil
				}
			}
			cfg.Roots = append(cfg.Roots, args[0])
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Printf("added root: %s\n", args[0])
			fmt.Printf("config: %s\n", config.ResolvePaths().ConfigFile)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "remove-root <dir>",
		Short: "Remove a discovery root from config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Ensure()
			if err != nil {
				return err
			}
			target := args[0]
			if samePath(target, config.ResolvePaths().DataDir) {
				return fmt.Errorf("cannot remove default log root: %s", config.ResolvePaths().DataDir)
			}
			next := make([]string, 0, len(cfg.Roots))
			removed := false
			for _, root := range cfg.Roots {
				if samePath(root, target) {
					removed = true
					continue
				}
				next = append(next, root)
			}
			if !removed {
				return fmt.Errorf("root not found: %s", target)
			}
			cfg.Roots = next
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Printf("removed root: %s\n", target)
			fmt.Printf("config: %s\n", config.ResolvePaths().ConfigFile)
			return nil
		},
	})

	return cmd
}

func resolveProjectPath() string {
	if projectPath != "" {
		abs, err := filepath.Abs(projectPath)
		if err == nil {
			return abs
		}
		return projectPath
	}
	wd, _ := os.Getwd()
	return wd
}

func newStore() logs.Store {
	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}
	return logs.NewStoreWithConfig(resolveProjectPath(), cfg)
}

func resolveInputFile(store logs.Store, args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	file, err := store.LatestFile()
	if err != nil {
		if errors.Is(err, logs.ErrNoFiles) {
			return "", fmt.Errorf("no log files discovered")
		}
		return "", err
	}
	return file.Path, nil
}

func streamEntries(r io.Reader, query logs.Query) error {
	entries, err := logs.DecodeEntries(r, "stdin", query)
	if err != nil {
		return err
	}
	return writeEntries(entries)
}

func writeEntries(entries []logs.Entry) error {
	for _, entry := range entries {
		if jsonOutput || !isatty.IsTerminal(os.Stdout.Fd()) {
			payload := struct {
				Timestamp string          `json:"timestamp,omitempty"`
				Level     string          `json:"level,omitempty"`
				Message   string          `json:"message,omitempty"`
				Data      map[string]any  `json:"data,omitempty"`
				Source    string          `json:"source,omitempty"`
				Raw       string          `json:"raw,omitempty"`
				Format    logs.FormatKind `json:"format,omitempty"`
			}{
				Timestamp: entry.Timestamp,
				Level:     entry.Level,
				Message:   entry.Message,
				Data:      entry.Data,
				Source:    entry.Source,
				Raw:       entry.Raw,
				Format:    entry.Format,
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			fmt.Println(string(encoded))
			continue
		}
		fmt.Println(logs.FormatEntry(entry))
	}
	return nil
}

func matchesSearch(entry logs.Entry, search string) bool {
	q := strings.ToLower(search)
	if strings.Contains(strings.ToLower(entry.Raw), q) || strings.Contains(strings.ToLower(entry.Message), q) {
		return true
	}
	for key, value := range entry.Data {
		if strings.Contains(strings.ToLower(key), q) || strings.Contains(strings.ToLower(fmt.Sprintf("%v", value)), q) {
			return true
		}
	}
	return false
}

func samePath(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA == nil {
		a = absA
	}
	if errB == nil {
		b = absB
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
