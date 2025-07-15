package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
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
	deleteFileIndex  int
	logContent       string
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
		logFiles: logFiles,
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
			if !m.confirmingDelete { // Only navigate if not confirming
				if m.cursor > 0 {
					m.cursor--
				}
				m.message = ""
			}
		case "down", "j":
			if !m.confirmingDelete { // Only navigate if not confirming
				if m.cursor < m.getMaxCursor() {
					m.cursor++
				}
				m.message = ""
			}
		case "enter":
			if !m.confirmingDelete {
				// Handle screen transitions differently
				if m.screen == screenInstall {
					// Process installation and immediately return to main
					if m.language != nil {
						err := m.language.Install(m.projectPath, m.config)
						if err != nil {
							m.message = fmt.Sprintf("❌ Error: %v", err)
						} else {
							m.message = "✅ Logger installed successfully! Check internal/logdog/logger.go"
							// Refresh log files
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
			if m.screen == screenLogs && len(m.logFiles) > 0 && !m.confirmingDelete {
				return m.handleViewLog()
			}
		case "d":
			if m.screen == screenLogs && len(m.logFiles) > 0 && !m.confirmingDelete {
				return m.handleDeleteLog()
			}
		case "y":
			if m.confirmingDelete {
				return m.confirmDelete()
			}
		case "esc":
			if m.confirmingDelete {
				// Cancel delete confirmation
				m.confirmingDelete = false
				m.message = ""
			} else {
				// Clean state when going back to main
				m.screen = screenMain
				m.cursor = 0
				m.message = ""
				m.confirmingDelete = false
				m.logContent = ""
			}
		default:
			if m.confirmingDelete {
				// Any other key cancels the delete
				m.confirmingDelete = false
				m.message = ""
			} else {
				m.message = ""
			}
		}
	}
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

	// Read and format the log file
	file, err := os.Open(filePath)
	if err != nil {
		m.message = fmt.Sprintf("❌ Error reading log: %v", err)
		return m, nil
	}
	defer file.Close()

	// Create a formatted view
	var formattedLogs strings.Builder
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Parse JSON log entry
		var logEntry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &logEntry); err == nil {
			// Format with charmbracelet/log styling
			formattedLogs.WriteString(m.formatLogEntry(logEntry))
			formattedLogs.WriteString("\n")
		} else {
			// Fallback for non-JSON lines
			formattedLogs.WriteString(line)
			formattedLogs.WriteString("\n")
		}
	}

	// Switch to a new screen to show the log content
	m.screen = screenLogView
	m.logContent = formattedLogs.String()
	m.cursor = 0

	return m, nil
}

func (m Model) formatLogEntry(entry map[string]interface{}) string {
	// Extract common fields
	timestamp, _ := entry["timestamp"].(string)
	level, _ := entry["level"].(string)
	message, _ := entry["message"].(string)
	data, _ := entry["data"].(map[string]interface{})

	// Create styled output
	var result strings.Builder

	// Format timestamp to be more human readable
	if timestamp != "" {
		if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
			humanTime := t.Format("Jan 02 15:04:05") // e.g., "Dec 25 14:30:45"
			result.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Render(humanTime))
		} else {
			// Fallback to original if parsing fails
			result.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Render(timestamp))
		}
		result.WriteString(" ")
	}

	// Level with color
	if level != "" {
		var levelColor lipgloss.Color
		switch level {
		case "ERROR":
			levelColor = lipgloss.Color("196") // Red
		case "WARN":
			levelColor = lipgloss.Color("208") // Orange
		case "INFO":
			levelColor = lipgloss.Color("46") // Green
		case "DEBUG":
			levelColor = lipgloss.Color("240") // Gray
		default:
			levelColor = lipgloss.Color("252")
		}

		result.WriteString(lipgloss.NewStyle().
			Foreground(levelColor).
			Bold(true).
			Render(fmt.Sprintf("[%s]", level)))
		result.WriteString(" ")
	}

	// Message
	if message != "" {
		result.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Render(message))
	}

	// Data fields
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
			// Refresh log files list
			if m.language != nil {
				m.logFiles = m.language.GetLogPaths(m.projectPath)
			}
			// Adjust cursor if needed
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

	// Create table header
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99")).
		Render("Log Files")

	// Table styling
	selectedStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("57")).
		Foreground(lipgloss.Color("230"))

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	// Build table rows
	var rows []string
	for i, file := range m.logFiles {
		// Get entry count
		entryCount := m.getLogEntryCount(file)

		// Get just filename
		filename := filepath.Base(file)

		// Format row
		row := fmt.Sprintf("%-25s %8d entries", filename, entryCount)

		// Apply styling
		if i == m.cursor {
			row = selectedStyle.Render("> " + row)
		} else {
			row = normalStyle.Render("  " + row)
		}

		rows = append(rows, row)
	}

	// Instructions
	instructions := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render("\nPress 'v' to view, 'd' to delete, ESC to go back")

	// Message display
	messageStr := ""
	if m.message != "" {
		messageStr = "\n\n" + lipgloss.NewStyle().
			Foreground(lipgloss.Color("226")).
			Render(m.message)
	}

	return fmt.Sprintf("%s\n\n%s%s%s", header, strings.Join(rows, "\n"), instructions, messageStr)
}

func (m Model) renderSettings() string {
	return "Settings (Coming Soon)\n\nPress ESC to go back"
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	switch m.screen {
	case screenMain:
		switch m.cursor {
		case 0: // Install
			m.screen = screenInstall
		case 1: // Logs
			m.screen = screenLogs
		case 2: // Settings
			m.screen = screenSettings
		case 3: // Quit
			return m, tea.Quit
		}
		// Reset cursor and clear any messages when transitioning
		m.cursor = 0
		m.message = ""
	}

	return m, nil
}

func (m Model) getMaxCursor() int {
	switch m.screen {
	case screenMain:
		return 3
	case screenLogs:
		return len(m.logFiles) - 1
	default:
		return 0
	}
}
