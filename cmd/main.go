package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/LFroesch/logdog/internal/detector"
	"github.com/LFroesch/logdog/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var isTTY = isatty.IsTerminal(os.Stdout.Fd())

// --- styles (only used when stdout is a TTY) ---

var (
	styleError   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	styleWarn    = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
	styleInfo    = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true)
	styleDebug   = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Bold(true)
	styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleBold    = lipgloss.NewStyle().Bold(true)
	styleHeader  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	styleSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
)

func render(s lipgloss.Style, text string) string {
	if isTTY {
		return s.Render(text)
	}
	return text
}

// --- log entry ---

type logEntry struct {
	Timestamp string         `json:"timestamp"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Data      map[string]any `json:"data,omitempty"`
}

func formatEntry(e logEntry) string {
	var sb strings.Builder

	// timestamp
	ts := e.Timestamp
	if t, err := time.Parse(time.RFC3339, e.Timestamp); err == nil {
		ts = t.Format("Jan 02 15:04:05")
	} else if t, err := time.Parse("2006-01-02 15:04:05", e.Timestamp); err == nil {
		ts = t.Format("Jan 02 15:04:05")
	}
	sb.WriteString(render(styleDim, ts))
	sb.WriteString(" ")

	// level
	var ls lipgloss.Style
	switch strings.ToUpper(e.Level) {
	case "ERROR":
		ls = styleError
	case "WARN":
		ls = styleWarn
	case "INFO":
		ls = styleInfo
	case "DEBUG":
		ls = styleDebug
	default:
		ls = styleDim
	}
	sb.WriteString(render(ls, fmt.Sprintf("[%-5s]", strings.ToUpper(e.Level))))
	sb.WriteString(" ")
	sb.WriteString(e.Message)

	if len(e.Data) > 0 {
		var pairs []string
		for k, v := range e.Data {
			pairs = append(pairs, fmt.Sprintf("%s=%v", k, v))
		}
		sb.WriteString(" ")
		sb.WriteString(render(styleDim, "{"+strings.Join(pairs, " ")+"}"))
	}

	return sb.String()
}

// --- helpers ---

func getLogFiles(projectPath string) []string {
	lang := detector.DetectLanguage(projectPath)
	if lang == nil {
		return nil
	}
	return lang.GetLogPaths(projectPath)
}

func readEntries(path string) ([]logEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []logEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e logEntry
		if err := json.Unmarshal([]byte(line), &e); err == nil {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

func latestLogFile(files []string) string {
	if len(files) == 0 {
		return ""
	}
	latest := files[0]
	latestTime := time.Time{}
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latest = f
		}
	}
	return latest
}

func matchesLevel(entry logEntry, level string) bool {
	if level == "" {
		return true
	}
	return strings.EqualFold(entry.Level, level)
}

func globalLogDir() string {
	usr, _ := user.Current()
	return filepath.Join(usr.HomeDir, "logdog")
}

// --- commands ---

var rootCmd = &cobra.Command{
	Use:   "logdog",
	Short: "Structured log generator and viewer for Go projects",
	Long:  "Logdog installs structured logging into your project and lets you view, search, and tail logs from the CLI or TUI.",
	RunE: func(cmd *cobra.Command, args []string) error {
		p := tea.NewProgram(tui.NewModel(), tea.WithAltScreen())
		_, err := p.Run()
		return err
	},
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install logger into current Go project",
	RunE: func(cmd *cobra.Command, args []string) error {
		wd, _ := os.Getwd()
		lang := detector.DetectLanguage(wd)
		if lang == nil {
			return fmt.Errorf("no supported project detected in %s", wd)
		}
		cfg := detector.Config{
			LogLevel:   "INFO",
			OutputDir:  globalLogDir(),
			MaxFiles:   30,
			DateFormat: "2006-01-02",
		}
		if err := lang.Install(wd, cfg); err != nil {
			return err
		}
		fmt.Println(render(styleSuccess, "✓") + " logger installed → internal/logdog/logger.go")
		fmt.Println(render(styleDim, "  import \""+filepath.Base(wd)+"/internal/logdog\" to use"))
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show project logging status",
	RunE: func(cmd *cobra.Command, args []string) error {
		wd, _ := os.Getwd()
		lang := detector.DetectLanguage(wd)
		if lang == nil {
			fmt.Println(render(styleDim, "no supported project detected in "+wd))
			return nil
		}

		files := lang.GetLogPaths(wd)
		fmt.Println(render(styleHeader, "logdog status"))
		fmt.Printf("  project : %s (%s)\n", filepath.Base(wd), lang.Name())
		fmt.Printf("  log dir : %s\n", filepath.Join(globalLogDir(), filepath.Base(wd)))
		fmt.Printf("  files   : %d\n", len(files))

		if len(files) == 0 {
			return nil
		}

		// last entry across all files
		latest := latestLogFile(files)
		entries, _ := readEntries(latest)
		if len(entries) > 0 {
			last := entries[len(entries)-1]
			fmt.Printf("  last    : %s — %s\n",
				render(styleInfo, last.Level),
				last.Message,
			)
		}

		// total entry count
		total := 0
		for _, f := range files {
			e, _ := readEntries(f)
			total += len(e)
		}
		fmt.Printf("  entries : %d total\n", total)
		return nil
	},
}

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "List log files",
	RunE: func(cmd *cobra.Command, args []string) error {
		wd, _ := os.Getwd()
		files := getLogFiles(wd)
		if len(files) == 0 {
			fmt.Println(render(styleDim, "no log files found"))
			return nil
		}
		fmt.Println(render(styleHeader, "log files"))
		for _, f := range files {
			info, _ := os.Stat(f)
			entries, _ := readEntries(f)
			date := ""
			if info != nil {
				date = info.ModTime().Format("2006-01-02")
			}
			fmt.Printf("  %-40s  %s  %d entries\n",
				filepath.Base(f),
				render(styleDim, date),
				len(entries),
			)
		}
		return nil
	},
}

var catLevel string

var catCmd = &cobra.Command{
	Use:   "cat [file]",
	Short: "Print log file contents (latest if no file given)",
	RunE: func(cmd *cobra.Command, args []string) error {
		wd, _ := os.Getwd()
		var filePath string
		if len(args) > 0 {
			filePath = args[0]
		} else {
			files := getLogFiles(wd)
			filePath = latestLogFile(files)
		}
		if filePath == "" {
			return fmt.Errorf("no log file found")
		}

		entries, err := readEntries(filePath)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if !matchesLevel(e, catLevel) {
				continue
			}
			fmt.Println(formatEntry(e))
		}
		return nil
	},
}

var tailLevel string

var tailCmd = &cobra.Command{
	Use:   "tail",
	Short: "Live-follow the latest log file",
	RunE: func(cmd *cobra.Command, args []string) error {
		wd, _ := os.Getwd()
		files := getLogFiles(wd)
		filePath := latestLogFile(files)
		if filePath == "" {
			return fmt.Errorf("no log file found")
		}

		fmt.Println(render(styleDim, "tailing "+filepath.Base(filePath)+" (ctrl+c to stop)"))

		f, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer f.Close()

		// seek to end
		f.Seek(0, 2)
		scanner := bufio.NewScanner(f)

		for {
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}
				var e logEntry
				if err := json.Unmarshal([]byte(line), &e); err == nil {
					if matchesLevel(e, tailLevel) {
						fmt.Println(formatEntry(e))
					}
				} else {
					fmt.Println(line)
				}
			}
			time.Sleep(500 * time.Millisecond)
			// re-scan after sleep (scanner hits EOF, we keep polling)
			scanner = bufio.NewScanner(f)
			// re-seek to current position via reading all remaining
			for scanner.Scan() {
			}
		}
	},
}

var grepLevel string

var grepCmd = &cobra.Command{
	Use:   "grep <pattern>",
	Short: "Search log files for a pattern",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pattern := strings.ToLower(args[0])
		wd, _ := os.Getwd()
		files := getLogFiles(wd)
		if len(files) == 0 {
			fmt.Println(render(styleDim, "no log files found"))
			return nil
		}

		found := 0
		for _, f := range files {
			entries, err := readEntries(f)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if !matchesLevel(e, grepLevel) {
					continue
				}
				line := strings.ToLower(e.Message)
				dataStr := ""
				for k, v := range e.Data {
					dataStr += fmt.Sprintf("%s=%v ", k, v)
				}
				if strings.Contains(line, pattern) || strings.Contains(strings.ToLower(dataStr), pattern) {
					fmt.Println(formatEntry(e))
					found++
				}
			}
		}

		if found == 0 {
			fmt.Println(render(styleDim, "no matches"))
		}
		return nil
	},
}

var clearOlderThan int

var clearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Delete old log files",
	RunE: func(cmd *cobra.Command, args []string) error {
		wd, _ := os.Getwd()
		files := getLogFiles(wd)
		if len(files) == 0 {
			fmt.Println(render(styleDim, "no log files found"))
			return nil
		}

		days := clearOlderThan
		cutoff := time.Now().AddDate(0, 0, -days)
		var toDelete []string
		for _, f := range files {
			info, err := os.Stat(f)
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				toDelete = append(toDelete, f)
			}
		}

		if len(toDelete) == 0 {
			fmt.Printf(render(styleDim, "no files older than %d days\n"), days)
			return nil
		}

		deleted := 0
		for _, f := range toDelete {
			if err := os.Remove(f); err == nil {
				fmt.Println(render(styleDim, "  deleted "+filepath.Base(f)))
				deleted++
			}
		}
		fmt.Println(render(styleSuccess, fmt.Sprintf("✓ cleared %d file(s)", deleted)))
		return nil
	},
}

func init() {
	catCmd.Flags().StringVarP(&catLevel, "level", "l", "", "filter by level (error/warn/info/debug)")
	tailCmd.Flags().StringVarP(&tailLevel, "level", "l", "", "filter by level (error/warn/info/debug)")
	grepCmd.Flags().StringVarP(&grepLevel, "level", "l", "", "filter by level (error/warn/info/debug)")
	clearCmd.Flags().IntVarP(&clearOlderThan, "older-than", "n", 7, "delete files older than N days")

	rootCmd.AddCommand(installCmd, statusCmd, logsCmd, catCmd, tailCmd, grepCmd, clearCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
