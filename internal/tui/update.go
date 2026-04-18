package tui

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/LFroesch/logdog/internal/logs"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tickMsg:
		if m.statusMsg != "" && time.Now().After(m.statusExpiry) {
			m.statusMsg = ""
		}
		if m.tailing && m.page == pageDashboard {
			m.reloadEntriesOnly()
		}
	case editorDoneMsg:
		if msg.err != nil {
			m.showStatus("editor failed: " + msg.err.Error())
		} else {
			m.showStatus("editor closed")
			m.reloadConfig()
			m.reloadAll()
		}
	case copyDoneMsg:
		if msg.err != nil {
			m.showStatus("copy failed: " + msg.err.Error())
		} else {
			m.showStatus("copied " + msg.label)
		}
	case tea.MouseMsg:
		if m.mode != modeNormal {
			return m, m.nextTickCmd()
		}
		next, nextCmd := m.updateMouse(msg)
		m = next.(Model)
		cmd = nextCmd
		return m, batchCmds(cmd, m.nextTickCmd())
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.mode {
		case modeHelp:
			switch msg.String() {
			case "?", "q", "esc":
				m.mode = modeNormal
				m.helpScroll = 0
			case "j", "down":
				m.helpScroll++
			case "k", "up":
				if m.helpScroll > 0 {
					m.helpScroll--
				}
			case "g":
				m.helpScroll = 0
			case "G":
				m.helpScroll = 9999
			case "ctrl+d", "pgdown":
				m.helpScroll += max(1, m.contentHeight()/3)
			case "ctrl+u", "pgup":
				m.helpScroll -= max(1, m.contentHeight()/3)
				if m.helpScroll < 0 {
					m.helpScroll = 0
				}
			}
			return m, m.nextTickCmd()
		case modeSearch:
			next, nextCmd := m.updateSearch(msg)
			m = next.(Model)
			cmd = nextCmd
			return m, batchCmds(cmd, m.nextTickCmd())
		case modeInput:
			next, nextCmd := m.updateInput(msg)
			m = next.(Model)
			cmd = nextCmd
			return m, batchCmds(cmd, m.nextTickCmd())
		case modeConfirm:
			next, nextCmd := m.updateConfirm(msg)
			m = next.(Model)
			cmd = nextCmd
			return m, batchCmds(cmd, m.nextTickCmd())
		}
		next, nextCmd := m.updateNormal(msg)
		m = next.(Model)
		cmd = nextCmd
		return m, batchCmds(cmd, m.nextTickCmd())
	}
	return m, m.nextTickCmd()
}

func (m Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		if m.page == pageDashboard {
			return m, tea.Quit
		}
		m.page = pageDashboard
		m.focus = focusFiles
		return m, nil
	case "?":
		m.mode = modeHelp
		return m, nil
	case "1":
		m.page = pageDashboard
		m.focus = focusFiles
		return m, nil
	case "2":
		m.page = pageSetup
		m.focus = focusSetupRoots
		return m, nil
	case "3":
		m.page = pageCleanup
		m.focus = focusCleanupList
		return m, nil
	case "tab":
		m.focus = m.nextFocus()
		return m, nil
	case "shift+tab":
		m.focus = m.prevFocus()
		return m, nil
	case "/":
		if m.page == pageDashboard {
			m.mode = modeSearch
			m.searchInput = m.search
			m.showStatus("search current file and press enter")
		}
		return m, nil
	case "l":
		if m.page == pageDashboard {
			m.cycleLevel()
			m.applyFileFilter()
			m.reloadEntries()
		}
		return m, nil
	case "t":
		if m.page == pageDashboard {
			m.tailing = !m.tailing
			if m.tailing {
				m.showStatus("live follow enabled")
			} else {
				m.showStatus("live follow disabled")
			}
		}
		return m, nil
	case "r":
		m.reloadConfig()
		m.reloadAll()
		m.showStatus("reloaded")
		return m, nil
	case "g":
		m.moveTop()
		return m, nil
	case "G":
		m.moveBottom()
		return m, nil
	case "ctrl+d", "pgdown":
		m.pageDown()
		return m, nil
	case "ctrl+u", "pgup":
		m.pageUp()
		return m, nil
	}

	switch m.page {
	case pageDashboard:
		return m.updateDashboard(msg)
	case pageSetup:
		return m.updateSetup(msg)
	case pageCleanup:
		return m.updateCleanup(msg)
	default:
		return m, nil
	}
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		kind := m.confirmKind
		m.clearConfirm()
		switch kind {
		case "install":
			m.installLogger()
		case "deleteOne":
			if len(m.cleanup) == 0 {
				return m, nil
			}
			_, err := logs.DeleteCandidates([]logs.CleanupCandidate{m.cleanup[m.cleanupCursor]})
			if err != nil {
				m.showStatus("delete failed: " + err.Error())
				return m, nil
			}
			m.reloadCleanup()
			m.showStatus("deleted candidate")
		case "deleteBatch":
			var batch []logs.CleanupCandidate
			for _, candidate := range m.cleanup {
				if m.selected[candidate.Path] {
					batch = append(batch, candidate)
				}
			}
			if len(batch) == 0 {
				m.showStatus("nothing selected")
				return m, nil
			}
			deleted, err := logs.DeleteCandidates(batch)
			if err != nil {
				m.showStatus("delete failed: " + err.Error())
				return m, nil
			}
			m.selected = map[string]bool{}
			m.reloadCleanup()
			m.showStatus(strings.Join([]string{"deleted", intStr(deleted), "candidates"}, " "))
		case "prune":
			deleted, err := m.store.PruneOlderThan(m.cfg.CleanupKeepDays)
			if err != nil {
				m.showStatus("prune failed: " + err.Error())
				return m, nil
			}
			m.selected = map[string]bool{}
			m.reloadCleanup()
			m.showStatus(strings.Join([]string{"pruned", intStr(deleted), "candidates"}, " "))
		}
	case "n", "N", "esc", "q":
		m.clearConfirm()
		m.showStatus("cancelled")
	}
	return m, nil
}

func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.searchInput = ""
	case "enter":
		m.mode = modeNormal
		m.search = strings.TrimSpace(m.searchInput)
		m.fileCursor = 0
		m.fileScroll = 0
		m.entryCursor = 0
		m.entryScroll = 0
		m.applyFileFilter()
		m.reloadEntries()
		if m.search == "" {
			m.showStatus("search cleared")
		} else {
			m.showStatus(strings.Join([]string{"filter:", m.search, "·", intStr(len(m.files)), "files"}, " "))
		}
	case "backspace", "ctrl+h":
		if len(m.searchInput) > 0 {
			r := []rune(m.searchInput)
			m.searchInput = string(r[:len(r)-1])
		}
	default:
		if len(msg.Runes) > 0 {
			m.searchInput += string(msg.Runes)
		}
	}
	return m, nil
}

func (m Model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.inputBuf = ""
	case "enter":
		path := strings.TrimSpace(m.inputBuf)
		m.inputBuf = ""
		m.mode = modeNormal
		if path != "" {
			if m.addRoot(path) {
				m.showStatus("added root: " + path)
			} else {
				m.showStatus("root already configured")
			}
		}
	case "backspace", "ctrl+h":
		if len(m.inputBuf) > 0 {
			r := []rune(m.inputBuf)
			m.inputBuf = string(r[:len(r)-1])
		}
	default:
		if len(msg.Runes) > 0 {
			m.inputBuf += string(msg.Runes)
		}
	}
	return m, nil
}

func (m Model) updateDashboard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.focus == focusFiles {
			if m.fileCursor < len(m.files)-1 {
				m.selectFile(m.fileCursor + 1)
			}
		} else if m.focus == focusViewer && m.entryCursor < len(m.entries)-1 {
			m.entryCursor++
			m.ensureEntryVisible()
			m.detailScroll = 0
		}
	case "k", "up":
		if m.focus == focusFiles {
			if m.fileCursor > 0 {
				m.selectFile(m.fileCursor - 1)
			}
		} else if m.focus == focusViewer && m.entryCursor > 0 {
			m.entryCursor--
			m.ensureEntryVisible()
			m.detailScroll = 0
		}
	case "esc":
		if m.focus == focusViewer {
			m.focus = focusFiles
		}
		return m, nil
	case "enter":
		if m.focus == focusFiles {
			if len(m.files) > 0 {
				m.focus = focusViewer
			}
		} else if m.focus == focusViewer && len(m.files) > 0 {
			return m, openEditorCmd(m.files[m.fileCursor].Path)
		}
	case "o":
		target := m.configPath
		if m.focus == focusFiles && len(m.files) > 0 {
			target = m.files[m.fileCursor].Path
		}
		return m, openEditorCmd(target)
	case "y":
		if m.focus == focusFiles && len(m.files) > 0 {
			return m, copyToClipboardCmd(m.files[m.fileCursor].Path, "file path")
		}
		if m.focus == focusViewer && len(m.entries) > 0 {
			return m, copyToClipboardCmd(m.copyEntryPayload(), "entry payload")
		}
	case "Y":
		if m.focus == focusViewer && len(m.entries) > 0 {
			return m, copyToClipboardCmd(m.entries[m.entryCursor].Raw, "raw entry")
		}
	}
	return m, nil
}

func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		return m.scrollByMouse(msg.X, -1), nil
	case tea.MouseButtonWheelDown:
		return m.scrollByMouse(msg.X, 1), nil
	default:
		return m, nil
	}
}

func (m Model) scrollByMouse(x, delta int) Model {
	if m.page != pageDashboard {
		return m
	}

	if m.dashboardPanelAtX(x) == focusFiles {
		if delta > 0 && m.fileCursor < len(m.files)-1 {
			m.selectFile(m.fileCursor + 1)
		}
		if delta < 0 && m.fileCursor > 0 {
			m.selectFile(m.fileCursor - 1)
		}
		return m
	}

	m.focus = focusViewer
	if delta > 0 && m.entryCursor < len(m.entries)-1 {
		m.entryCursor++
		m.ensureEntryVisible()
		m.detailScroll = 0
	}
	if delta < 0 && m.entryCursor > 0 {
		m.entryCursor--
		m.ensureEntryVisible()
		m.detailScroll = 0
	}
	return m
}

func (m Model) updateSetup(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.focus == focusSetupRoots && m.rootCursor < len(m.cfg.Roots)-1 {
			m.rootCursor++
			m.ensureRootVisible()
		}
	case "k", "up":
		if m.focus == focusSetupRoots && m.rootCursor > 0 {
			m.rootCursor--
			m.ensureRootVisible()
		}
	case "i":
		if m.installed {
			m.showStatus("logger already installed in this project")
			return m, nil
		}
		if m.language == nil {
			m.showStatus("no supported project detected — install skipped")
			return m, nil
		}
		m.askConfirm("install", "Install logger files into "+m.projectName+"? (optional)")
	case "enter":
		if m.focus == focusSetupRoots && len(m.cfg.Roots) > 0 {
			return m, openEditorCmd(m.cfg.Roots[m.rootCursor])
		}
		return m, nil
	case "n":
		m.mode = modeInput
		m.inputBuf = ""
		return m, nil
	case "a":
		if m.addRoot(m.projectPath) {
			m.showStatus("added project root to config")
		} else {
			m.showStatus("project root already configured")
		}
	case "A":
		parent := filepath.Dir(m.projectPath)
		if m.addRoot(parent) {
			m.showStatus("added parent root to config")
		} else {
			m.showStatus("parent root already configured")
		}
	case "d":
		if m.focus == focusSetupRoots {
			if m.removeSelectedRoot() {
				m.showStatus("removed configured root")
			} else {
				m.showStatus("no configured root selected")
			}
		}
	case "o":
		return m, openEditorCmd(m.configPath)
	case "y":
		if m.focus == focusSetupRoots && len(m.cfg.Roots) > 0 {
			return m, copyToClipboardCmd(m.cfg.Roots[m.rootCursor], "root path")
		}
	}
	return m, nil
}

func (m Model) updateCleanup(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.cleanupCursor < len(m.cleanup)-1 {
			m.cleanupCursor++
			m.ensureCleanupVisible()
		}
	case "k", "up":
		if m.cleanupCursor > 0 {
			m.cleanupCursor--
			m.ensureCleanupVisible()
		}
	case "space":
		if len(m.cleanup) > 0 {
			path := m.cleanup[m.cleanupCursor].Path
			m.selected[path] = !m.selected[path]
			if !m.selected[path] {
				delete(m.selected, path)
			}
		}
	case "x":
		if len(m.cleanup) == 0 {
			return m, nil
		}
		name := filepath.Base(m.cleanup[m.cleanupCursor].Path)
		m.askConfirm("deleteOne", "Delete "+name+"?")
	case "D":
		count := 0
		for _, candidate := range m.cleanup {
			if m.selected[candidate.Path] {
				count++
			}
		}
		if count == 0 {
			m.showStatus("nothing selected")
			return m, nil
		}
		m.askConfirm("deleteBatch", "Delete "+intStr(count)+" selected candidates?")
	case "X":
		m.askConfirm("prune", "Prune candidates older than "+intStr(m.cfg.CleanupKeepDays)+" days?")
	case "o":
		if len(m.cleanup) > 0 {
			return m, openEditorCmd(m.cleanup[m.cleanupCursor].Path)
		}
	case "y":
		if len(m.cleanup) > 0 {
			return m, copyToClipboardCmd(m.cleanup[m.cleanupCursor].Path, "cleanup path")
		}
	}
	return m, nil
}
