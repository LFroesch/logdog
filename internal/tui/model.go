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
	"github.com/charmbracelet/lipgloss"
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

type Model struct {
	screen           screen
	projectPath      string
	language         detector.Language
	config           detector.Config
	logFiles         []string
	cursor           int
	message          string
	confirmingDelete bool
	confirmingClear  bool
	deleteFileIndex  int
	logContent       string
	// Global project selection
	globalProjects    []string
	selectedProject   string
	// Settings
	retentionDays     int
}

func scanGlobalProjects() []string {
	usr, err := user.Current()
	if err != nil {
		return []string{}
	}

	logdogDir := filepath.Join(usr.HomeDir, "logdog")
	var projects []string

	entries, err := os.ReadDir(logdogDir)
	if err != nil {
		return []string{}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			projects = append(projects, entry.Name())
		}
	}

	return projects
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
		logFiles:         logFiles,
		globalProjects:   scanGlobalProjects(),
		retentionDays:    7,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if !m.confirmingDelete && !m.confirmingClear {
				if m.cursor > 0 {
					m.cursor--
				}
				m.message = ""
			}
		case "down", "j":
			if !m.confirmingDelete && !m.confirmingClear {
				if m.cursor < m.getMaxCursor() {
					m.cursor++
				}
				m.message = ""
			}
		case "enter":
			if !m.confirmingDelete && !m.confirmingClear {
				if m.screen == screenInstall {
					if m.language != nil {
						err := m.language.Install(m.projectPath, m.config)
						if err != nil {
							m.message = fmt.Sprintf("❌ Error: %v", err)
						} else {
							m.message = "✅ Logger installed successfully! Check internal/logdog/logger.go"
							m.logFiles = m.language.GetLogPaths(m.projectPath)
						}
					} else {
						m.message = "❌ No supported language detected"
					}
					m.screen = screenMain
					m.cursor = 0
					return m, tea.ClearScreen
				} else {
					return m.handleEnter()
				}
			}
		case "v":
			if m.screen == screenLogs && len(m.logFiles) > 0 && !m.confirmingDelete && !m.confirmingClear {
				return m.handleViewLog()
			}
		case "d":
			if m.screen == screenLogs && len(m.logFiles) > 0 && !m.confirmingDelete && !m.confirmingClear {
				return m.handleDeleteLog()
			}
		case "c":
			if m.screen == screenLogs && len(m.logFiles) > 0 && !m.confirmingDelete && !m.confirmingClear {
				return m.handleClearOldLogs()
			}
		case "y":
			if m.confirmingDelete {
				return m.confirmDelete()
			} else if m.confirmingClear {
				return m.confirmClearOldLogs()
			}
		case "+", "=":
			if m.screen == screenSettings && !m.confirmingDelete && !m.confirmingClear {
				if m.retentionDays < 365 {
					m.retentionDays++
					m.message = fmt.Sprintf("Retention set to %d days", m.retentionDays)
				}
			}
		case "-", "_":
			if m.screen == screenSettings && !m.confirmingDelete && !m.confirmingClear {
				if m.retentionDays > 1 {
					m.retentionDays--
					m.message = fmt.Sprintf("Retention set to %d days", m.retentionDays)
				}
			}
		case "esc":
			if m.confirmingDelete || m.confirmingClear {
				m.confirmingDelete = false
				m.confirmingClear = false
				m.message = ""
			} else {
				m.screen = screenMain
				m.cursor = 0
				m.message = ""
				m.confirmingDelete = false
				m.confirmingClear = false
				m.logContent = ""
				m.selectedProject = ""
			}
		default:
			if m.confirmingDelete || m.confirmingClear {
				m.confirmingDelete = false
				m.confirmingClear = false
				m.message = ""
			} else {
				m.message = ""
			}
		}
	}
	return m, nil
}

func (m Model) handleClearOldLogs() (Model, tea.Cmd) {
	// Count logs older than retentionDays
	cutoffDate := time.Now().AddDate(0, 0, -m.retentionDays)
	var oldLogs []string

	for _, logFile := range m.logFiles {
		if info, err := os.Stat(logFile); err == nil {
			if info.ModTime().Before(cutoffDate) {
				oldLogs = append(oldLogs, logFile)
			}
		}
	}

	if len(oldLogs) == 0 {
		m.message = fmt.Sprintf("No log files older than %d days found", m.retentionDays)
		return m, nil
	}

	m.message = fmt.Sprintf("Clear %d log files older than %d days? Press 'y' to confirm, any other key to cancel", len(oldLogs), m.retentionDays)
	m.confirmingClear = true
	return m, nil
}

func (m Model) confirmClearOldLogs() (Model, tea.Cmd) {
	cutoffDate := time.Now().AddDate(0, 0, -m.retentionDays)
	var deletedCount int
	var errors []string

	for _, logFile := range m.logFiles {
		if info, err := os.Stat(logFile); err == nil {
			if info.ModTime().Before(cutoffDate) {
				if err := os.Remove(logFile); err != nil {
					errors = append(errors, fmt.Sprintf("Failed to delete %s: %v", filepath.Base(logFile), err))
				} else {
					deletedCount++
				}
			}
		}
	}

	// Refresh log files list
	if m.language != nil {
		m.logFiles = m.language.GetLogPaths(m.projectPath)
	}

	// Adjust cursor if needed
	if m.cursor >= len(m.logFiles) && len(m.logFiles) > 0 {
		m.cursor = len(m.logFiles) - 1
	} else if len(m.logFiles) == 0 {
		m.cursor = 0
	}

	if len(errors) > 0 {
		m.message = fmt.Sprintf("✅ Deleted %d files. ❌ Errors: %s", deletedCount, strings.Join(errors, "; "))
	} else {
		m.message = fmt.Sprintf("✅ Cleared %d old log files", deletedCount)
	}

	m.confirmingClear = false
	return m, nil
}

func (m Model) handleViewLog() (Model, tea.Cmd) {
	if m.cursor < len(m.logFiles) {
		return m.viewLogContent()
	}
	return m, nil
}

func (m Model) viewLogContent() (Model, tea.Cmd) {
	filePath := m.logFiles[m.cursor]

	file, err := os.Open(filePath)
	if err != nil {
		m.message = fmt.Sprintf("❌ Error reading log: %v", err)
		return m, nil
	}
	defer file.Close()

	var formattedLogs strings.Builder
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var logEntry map[string]any
		if err := json.Unmarshal([]byte(line), &logEntry); err == nil {
			formattedLogs.WriteString(m.formatLogEntry(logEntry))
			formattedLogs.WriteString("\n")
		} else {
			formattedLogs.WriteString(line)
			formattedLogs.WriteString("\n")
		}
	}

	m.screen = screenLogView
	m.logContent = formattedLogs.String()
	m.cursor = 0

	return m, nil
}

func (m Model) formatLogEntry(entry map[string]any) string {
	timestamp, _ := entry["timestamp"].(string)
	level, _ := entry["level"].(string)
	message, _ := entry["message"].(string)
	data, _ := entry["data"].(map[string]any)

	var result strings.Builder

	if timestamp != "" {
		if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
			humanTime := t.Format("Jan 02 15:04:05")
			result.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Render(humanTime))
		} else {
			result.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Render(timestamp))
		}
		result.WriteString(" ")
	}

	if level != "" {
		var levelColor lipgloss.Color
		switch level {
		case "ERROR":
			levelColor = lipgloss.Color("196")
		case "WARN":
			levelColor = lipgloss.Color("208")
		case "INFO":
			levelColor = lipgloss.Color("46")
		case "DEBUG":
			levelColor = lipgloss.Color("240")
		default:
			levelColor = lipgloss.Color("252")
		}

		result.WriteString(lipgloss.NewStyle().
			Foreground(levelColor).
			Bold(true).
			Render(fmt.Sprintf("[%s]", level)))
		result.WriteString(" ")
	}

	if message != "" {
		result.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Render(message))
	}

	if len(data) > 0 {
		result.WriteString(" ")
		var pairs []string
		for k, v := range data {
			pairs = append(pairs, fmt.Sprintf("%s=%v", k, v))
		}
		result.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("99")).
			Render(fmt.Sprintf("{%s}", strings.Join(pairs, ", "))))
	}

	return result.String()
}

func (m Model) handleDeleteLog() (Model, tea.Cmd) {
	if m.cursor < len(m.logFiles) {
		filename := filepath.Base(m.logFiles[m.cursor])
		m.message = fmt.Sprintf("Delete %s? Press 'y' to confirm, any other key to cancel", filename)
		m.confirmingDelete = true
		m.deleteFileIndex = m.cursor
	}
	return m, nil
}

func (m Model) confirmDelete() (Model, tea.Cmd) {
	if m.deleteFileIndex < len(m.logFiles) {
		filePath := m.logFiles[m.deleteFileIndex]
		err := os.Remove(filePath)
		if err != nil {
			m.message = fmt.Sprintf("❌ Failed to delete: %v", err)
		} else {
			filename := filepath.Base(filePath)
			m.message = fmt.Sprintf("✅ Deleted %s", filename)
			if m.language != nil {
				m.logFiles = m.language.GetLogPaths(m.projectPath)
			}
			if m.cursor >= len(m.logFiles) && len(m.logFiles) > 0 {
				m.cursor = len(m.logFiles) - 1
			}
		}
	}
	m.confirmingDelete = false
	return m, nil
}

func (m Model) View() string {
	var s string

	switch m.screen {
	case screenMain:
		s = m.renderMain()
	case screenInstall:
		s = m.renderInstall()
	case screenLogs:
		s = m.renderLogs()
	case screenLogView:
		s = m.renderLogView()
	case screenSettings:
		s = m.renderSettings()
	case screenGlobalProjects:
		s = m.renderGlobalProjects()
	default:
		s = m.renderMain()
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(s)
}

func (m Model) renderLogView() string {
	filename := ""
	if m.deleteFileIndex < len(m.logFiles) {
		filename = filepath.Base(m.logFiles[m.deleteFileIndex])
	}

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99")).
		Render(fmt.Sprintf("📋 Viewing: %s", filename))

	instructions := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render("Press ESC to go back")

	content := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Render(m.logContent)

	return fmt.Sprintf("%s\n\n%s\n\n%s", header, content, instructions)
}

func (m Model) renderMain() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99")).
		Render("🐕 Logdog")

	var status string
	if m.language != nil {
		status = fmt.Sprintf("Detected: %s project in %s", m.language.Name(), m.projectPath)
		if len(m.logFiles) > 0 {
			status += fmt.Sprintf(" (%d log files)", len(m.logFiles))
		}
	} else {
		status = fmt.Sprintf("No supported project detected in %s", m.projectPath)
	}

	options := []string{
		"📦 Install/Setup Logger",
		"📋 View Logs",
		"🌐 View All Logs (Global)",
		"⚙️  Settings",
		"❌ Quit",
	}

	var optionsStr string
	for i, option := range options {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		optionsStr += fmt.Sprintf("%s %s\n", cursor, option)
	}

	messageStr := ""
	if m.message != "" {
		messageStr = "\n" + lipgloss.NewStyle().
			Foreground(lipgloss.Color("226")).
			Render(m.message)
	}

	return fmt.Sprintf("%s\n\n%s\n\n%s%s", title, status, optionsStr, messageStr)
}

func (m Model) renderInstall() string {
	if m.language == nil {
		return "No supported language detected. Press ESC to go back."
	}

	status := fmt.Sprintf("Installing logger for %s project...\n\nThis will create:\n- internal/logdog/logger.go\n- logdog/logs/ directory\n\nPress ENTER to install or ESC to cancel", m.language.Name())

	return status
}

func (m Model) getLogEntryCount(filepath string) int {
	file, err := os.Open(filepath)
	if err != nil {
		return 0
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			count++
		}
	}

	return count
}

func (m Model) renderLogs() string {
	if len(m.logFiles) == 0 {
		return "No log files found. Press ESC to go back."
	}

	headerText := "Log Files"
	if m.selectedProject != "" {
		headerText = fmt.Sprintf("Log Files - %s", m.selectedProject)
	}

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99")).
		Render(headerText)

	selectedStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("57")).
		Foreground(lipgloss.Color("230"))

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	var rows []string
	for i, file := range m.logFiles {
		entryCount := m.getLogEntryCount(file)
		filename := filepath.Base(file)
		row := fmt.Sprintf("%-25s %8d entries", filename, entryCount)

		if i == m.cursor {
			row = selectedStyle.Render("> " + row)
		} else {
			row = normalStyle.Render("  " + row)
		}

		rows = append(rows, row)
	}

	instructions := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render("\nPress 'v' to view, 'd' to delete, 'c' to clear old logs, ESC to go back")

	messageStr := ""
	if m.message != "" {
		messageStr = "\n\n" + lipgloss.NewStyle().
			Foreground(lipgloss.Color("226")).
			Render(m.message)
	}

	return fmt.Sprintf("%s\n\n%s%s%s", header, strings.Join(rows, "\n"), instructions, messageStr)
}

func (m Model) renderSettings() string {
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99")).
		Render("⚙️ Settings")

	settingsText := fmt.Sprintf("Log Retention: %d days\n\nUse +/- to adjust retention days", m.retentionDays)

	instructions := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render("\nPress +/- to adjust, ESC to go back")

	messageStr := ""
	if m.message != "" {
		messageStr = "\n\n" + lipgloss.NewStyle().
			Foreground(lipgloss.Color("226")).
			Render(m.message)
	}

	return fmt.Sprintf("%s\n\n%s%s%s", header, settingsText, instructions, messageStr)
}

func (m Model) renderGlobalProjects() string {
	if len(m.globalProjects) == 0 {
		return "No projects found in ~/logdog/\n\nPress ESC to go back"
	}

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99")).
		Render("🌐 Select Project")

	selectedStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("57")).
		Foreground(lipgloss.Color("230"))

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	var rows []string
	for i, project := range m.globalProjects {
		row := project
		if i == m.cursor {
			row = selectedStyle.Render("> " + row)
		} else {
			row = normalStyle.Render("  " + row)
		}
		rows = append(rows, row)
	}

	instructions := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render("\nPress ENTER to view logs, ESC to go back")

	messageStr := ""
	if m.message != "" {
		messageStr = "\n\n" + lipgloss.NewStyle().
			Foreground(lipgloss.Color("226")).
			Render(m.message)
	}

	return fmt.Sprintf("%s\n\n%s%s%s", header, strings.Join(rows, "\n"), instructions, messageStr)
}

func (m Model) getLogFilesForProject(projectName string) []string {
	usr, err := user.Current()
	if err != nil {
		return []string{}
	}

	logsDir := filepath.Join(usr.HomeDir, "logdog", projectName)
	var paths []string

	filepath.Walk(logsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && filepath.Ext(path) == ".json" {
			paths = append(paths, path)
		}
		return nil
	})

	return paths
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	switch m.screen {
	case screenMain:
		switch m.cursor {
		case 0:
			m.screen = screenInstall
		case 1:
			m.screen = screenLogs
		case 2:
			m.screen = screenGlobalProjects
		case 3:
			m.screen = screenSettings
		case 4:
			return m, tea.Quit
		}
		m.cursor = 0
		m.message = ""
	case screenGlobalProjects:
		if m.cursor < len(m.globalProjects) {
			m.selectedProject = m.globalProjects[m.cursor]
			m.logFiles = m.getLogFilesForProject(m.selectedProject)
			m.screen = screenLogs
			m.cursor = 0
			m.message = ""
		}
	}

	return m, nil
}

func (m Model) getMaxCursor() int {
	switch m.screen {
	case screenMain:
		return 4
	case screenLogs:
		return len(m.logFiles) - 1
	case screenGlobalProjects:
		return len(m.globalProjects) - 1
	default:
		return 0
	}
}
