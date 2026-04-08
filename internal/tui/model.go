package tui

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
	tea "github.com/charmbracelet/bubbletea"
)

type screen int

const (
	screenMain screen = iota
	screenInstall
	screenLogs
	screenLogView
	screenSettings
	screenGlobalProjects
)

// LogEntry matches the JSON format written by the generated logger.
type LogEntry struct {
	Timestamp string         `json:"timestamp"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Data      map[string]any `json:"data,omitempty"`
}

type Model struct {
	screen      screen
	width       int
	height      int
	projectPath string
	language    detector.Language
	config      detector.Config

	// log browser
	logFiles []string
	cursor   int
	message  string

	// confirm dialogs
	confirmingDelete bool
	confirmingClear  bool
	deleteFileIndex  int

	// log viewer
	logEntries  []LogEntry
	logScroll   int
	levelFilter string // "", "ERROR", "WARN", "INFO", "DEBUG"
	searchQuery string
	searching   bool // search input active
	detailEntry *LogEntry

	// global project picker
	globalProjects  []string
	selectedProject string

	// settings
	retentionDays int

	showHelp bool
}

// tickMsg drives live tail in the future
type tickMsg time.Time

func scanGlobalProjects() []string {
	usr, err := user.Current()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(usr.HomeDir, "logdog"))
	if err != nil {
		return nil
	}
	var projects []string
	for _, e := range entries {
		if e.IsDir() {
			projects = append(projects, e.Name())
		}
	}
	return projects
}

func getLogFilesForProject(projectName string) []string {
	usr, err := user.Current()
	if err != nil {
		return nil
	}
	logsDir := filepath.Join(usr.HomeDir, "logdog", projectName)
	var paths []string
	_ = filepath.Walk(logsDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && filepath.Ext(path) == ".json" {
			paths = append(paths, path)
		}
		return nil
	})
	return paths
}

func readLogEntries(filePath string) []LogEntry {
	f, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var entries []LogEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e LogEntry
		if json.Unmarshal([]byte(line), &e) == nil {
			entries = append(entries, e)
		}
	}
	return entries
}

func getLogEntryCount(filePath string) int {
	f, err := os.Open(filePath)
	if err != nil {
		return 0
	}
	defer f.Close()
	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			count++
		}
	}
	return count
}

// filteredEntries returns logEntries matching current levelFilter + searchQuery.
func (m *Model) filteredEntries() []LogEntry {
	var out []LogEntry
	for _, e := range m.logEntries {
		if m.levelFilter != "" && strings.ToUpper(e.Level) != m.levelFilter {
			continue
		}
		if m.searchQuery != "" && !entryContains(e, strings.ToLower(m.searchQuery)) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func entryContains(e LogEntry, q string) bool {
	if strings.Contains(strings.ToLower(e.Message), q) {
		return true
	}
	for k, v := range e.Data {
		if strings.Contains(strings.ToLower(k), q) {
			return true
		}
		if strings.Contains(strings.ToLower(fmt.Sprintf("%v", v)), q) {
			return true
		}
	}
	return false
}

func formatTimestamp(ts string) string {
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.Format("Jan 02 15:04:05")
	}
	if t, err := time.Parse("2006-01-02 15:04:05", ts); err == nil {
		return t.Format("Jan 02 15:04:05")
	}
	return ts
}

func NewModel() Model {
	wd, _ := os.Getwd()
	lang := detector.DetectLanguage(wd)
	var logFiles []string
	if lang != nil {
		logFiles = lang.GetLogPaths(wd)
	}
	return Model{
		screen:      screenMain,
		projectPath: wd,
		language:    lang,
		config: detector.Config{
			LogLevel:   "INFO",
			OutputDir:  "logdog/logs",
			MaxFiles:   30,
			DateFormat: "2006-01-02",
		},
		logFiles:       logFiles,
		globalProjects: scanGlobalProjects(),
		retentionDays:  7,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}
