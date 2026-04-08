package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global quit
	if key == "ctrl+c" {
		return m, tea.Quit
	}

	// Help overlay
	if m.showHelp {
		if key == "?" || key == "q" || key == "esc" {
			m.showHelp = false
		}
		return m, nil
	}

	// Search input mode (log viewer)
	if m.searching {
		return m.handleSearchInput(key)
	}

	switch m.screen {
	case screenLogView:
		return m.handleLogViewKey(key)
	default:
		return m.handleNavKey(key)
	}
}

func (m Model) handleSearchInput(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.searching = false
		m.searchQuery = ""
		m.logScroll = 0
	case "enter":
		m.searching = false
		m.logScroll = 0
	case "backspace":
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m.logScroll = 0
		}
	default:
		if len(key) == 1 {
			m.searchQuery += key
			m.logScroll = 0
		}
	}
	return m, nil
}

func (m Model) handleLogViewKey(key string) (tea.Model, tea.Cmd) {
	// detail overlay
	if m.detailEntry != nil {
		if key == "esc" || key == "q" || key == "enter" {
			m.detailEntry = nil
		}
		return m, nil
	}

	switch key {
	case "q":
		return m, tea.Quit
	case "esc":
		m.screen = screenLogs
		m.logEntries = nil
		m.logScroll = 0
		m.levelFilter = ""
		m.searchQuery = ""
		m.searching = false
	case "up", "k":
		if m.logScroll > 0 {
			m.logScroll--
		}
	case "down", "j":
		m.logScroll++
	case "enter":
		// show detail for current visible entry
		entries := m.filteredEntries()
		if m.logScroll < len(entries) {
			e := entries[m.logScroll]
			m.detailEntry = &e
		}
	case "?":
		m.showHelp = true
		return m, nil
	case "/":
		m.searching = true
	case "e":
		m.toggleLevelFilter("ERROR")
	case "w":
		m.toggleLevelFilter("WARN")
	case "i":
		m.toggleLevelFilter("INFO")
	case "d":
		m.toggleLevelFilter("DEBUG")
	case "c":
		m.levelFilter = ""
		m.searchQuery = ""
		m.logScroll = 0
	}
	return m, nil
}

func (m *Model) toggleLevelFilter(level string) {
	if m.levelFilter == level {
		m.levelFilter = ""
	} else {
		m.levelFilter = level
	}
	m.logScroll = 0
}

func (m Model) handleNavKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "?":
		m.showHelp = true
		return m, nil
	case "q":
		if m.screen == screenMain {
			return m, tea.Quit
		}
		m.screen = screenMain
		m.cursor = 0
		m.message = ""

	case "esc":
		if m.confirmingDelete || m.confirmingClear {
			m.confirmingDelete = false
			m.confirmingClear = false
			m.message = ""
			return m, nil
		}
		m.screen = screenMain
		m.cursor = 0
		m.message = ""
		m.selectedProject = ""

	case "up", "k":
		if !m.confirmingDelete && !m.confirmingClear && m.cursor > 0 {
			m.cursor--
			m.message = ""
		}

	case "down", "j":
		if !m.confirmingDelete && !m.confirmingClear && m.cursor < m.maxCursor() {
			m.cursor++
			m.message = ""
		}

	case "enter":
		if m.confirmingDelete || m.confirmingClear {
			return m, nil
		}
		return m.handleEnter()

	case "v":
		if m.screen == screenLogs && len(m.logFiles) > 0 && !m.confirmingDelete && !m.confirmingClear {
			return m.openLogViewer()
		}

	case "d":
		if m.screen == screenLogs && len(m.logFiles) > 0 && !m.confirmingDelete && !m.confirmingClear {
			filename := filepath.Base(m.logFiles[m.cursor])
			m.message = fmt.Sprintf("Delete %s? y to confirm, any key to cancel", filename)
			m.confirmingDelete = true
			m.deleteFileIndex = m.cursor
		}

	case "c":
		if m.screen == screenLogs && !m.confirmingDelete && !m.confirmingClear {
			return m.startClearOld()
		}

	case "y":
		if m.confirmingDelete {
			return m.confirmDelete()
		}
		if m.confirmingClear {
			return m.confirmClearOld()
		}

	case "+", "=":
		if m.screen == screenSettings && m.retentionDays < 365 {
			m.retentionDays++
			m.message = fmt.Sprintf("Retention: %d days", m.retentionDays)
		}

	case "-", "_":
		if m.screen == screenSettings && m.retentionDays > 1 {
			m.retentionDays--
			m.message = fmt.Sprintf("Retention: %d days", m.retentionDays)
		}

	default:
		if m.confirmingDelete || m.confirmingClear {
			m.confirmingDelete = false
			m.confirmingClear = false
			m.message = ""
		}
	}
	return m, nil
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

	case screenInstall:
		if m.language != nil {
			if err := m.language.Install(m.projectPath, m.config); err != nil {
				m.message = "error: " + err.Error()
			} else {
				m.message = "logger installed → internal/logdog/logger.go"
				m.logFiles = m.language.GetLogPaths(m.projectPath)
			}
		} else {
			m.message = "no supported language detected"
		}
		m.screen = screenMain
		m.cursor = 0
		return m, tea.ClearScreen

	case screenLogs:
		return m.openLogViewer()

	case screenGlobalProjects:
		if m.cursor < len(m.globalProjects) {
			m.selectedProject = m.globalProjects[m.cursor]
			m.logFiles = getLogFilesForProject(m.selectedProject)
			m.screen = screenLogs
			m.cursor = 0
		}
	}
	return m, nil
}

func (m Model) openLogViewer() (Model, tea.Cmd) {
	if m.cursor >= len(m.logFiles) {
		return m, nil
	}
	m.logEntries = readLogEntries(m.logFiles[m.cursor])
	m.logScroll = 0
	m.levelFilter = ""
	m.searchQuery = ""
	m.searching = false
	m.detailEntry = nil
	m.screen = screenLogView
	return m, nil
}

func (m Model) startClearOld() (Model, tea.Cmd) {
	cutoff := time.Now().AddDate(0, 0, -m.retentionDays)
	count := 0
	for _, f := range m.logFiles {
		if info, err := os.Stat(f); err == nil && info.ModTime().Before(cutoff) {
			count++
		}
	}
	if count == 0 {
		m.message = fmt.Sprintf("no files older than %d days", m.retentionDays)
		return m, nil
	}
	m.message = fmt.Sprintf("clear %d file(s) older than %d days? y to confirm", count, m.retentionDays)
	m.confirmingClear = true
	return m, nil
}

func (m Model) confirmClearOld() (Model, tea.Cmd) {
	cutoff := time.Now().AddDate(0, 0, -m.retentionDays)
	deleted := 0
	for _, f := range m.logFiles {
		if info, err := os.Stat(f); err == nil && info.ModTime().Before(cutoff) {
			if os.Remove(f) == nil {
				deleted++
			}
		}
	}
	if m.language != nil {
		m.logFiles = m.language.GetLogPaths(m.projectPath)
	}
	if m.cursor >= len(m.logFiles) && len(m.logFiles) > 0 {
		m.cursor = len(m.logFiles) - 1
	}
	m.message = fmt.Sprintf("cleared %d file(s)", deleted)
	m.confirmingClear = false
	return m, nil
}

func (m Model) confirmDelete() (Model, tea.Cmd) {
	if m.deleteFileIndex >= len(m.logFiles) {
		m.confirmingDelete = false
		return m, nil
	}
	f := m.logFiles[m.deleteFileIndex]
	if err := os.Remove(f); err != nil {
		m.message = "delete failed: " + err.Error()
	} else {
		m.message = "deleted " + filepath.Base(f)
		if m.language != nil {
			m.logFiles = m.language.GetLogPaths(m.projectPath)
		} else {
			m.logFiles = getLogFilesForProject(m.selectedProject)
		}
		if m.cursor >= len(m.logFiles) && len(m.logFiles) > 0 {
			m.cursor = len(m.logFiles) - 1
		}
	}
	m.confirmingDelete = false
	return m, nil
}

func (m Model) maxCursor() int {
	switch m.screen {
	case screenMain:
		return 4
	case screenLogs:
		if len(m.logFiles) == 0 {
			return 0
		}
		return len(m.logFiles) - 1
	case screenGlobalProjects:
		if len(m.globalProjects) == 0 {
			return 0
		}
		return len(m.globalProjects) - 1
	default:
		return 0
	}
}

// unused but keep for future live tail
func (m Model) refreshLogs() (Model, tea.Cmd) {
	if m.language != nil {
		m.logFiles = m.language.GetLogPaths(m.projectPath)
	}
	return m, nil
}

