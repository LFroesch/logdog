package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

const uiOverhead = 4 // header + footer + padding

func (m Model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	header := m.renderHeader()
	footer := m.renderFooter()

	// content height between header and footer
	contentH := m.height - 4
	if contentH < 3 {
		contentH = 3
	}

	var content string
	switch m.screen {
	case screenMain:
		content = m.renderMain()
	case screenInstall:
		content = m.renderInstall()
	case screenLogs:
		content = m.renderLogs(contentH)
	case screenLogView:
		content = m.renderLogView(contentH)
	case screenSettings:
		content = m.renderSettings()
	case screenGlobalProjects:
		content = m.renderGlobalProjects(contentH)
	default:
		content = m.renderMain()
	}

	// overlay detail panel on top of log viewer
	if m.screen == screenLogView && m.detailEntry != nil {
		base := lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
		overlay := m.renderDetailOverlay()
		return placeOverlay(base, overlay, m.width, m.height)
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

// --- header ---

func (m Model) renderHeader() string {
	var parts []string

	parts = append(parts, styleTitle.Render("🐕 logdog"))

	if m.language != nil {
		parts = append(parts, styleDim.Render("│"))
		parts = append(parts, styleDim.Render(m.language.Name()))
		parts = append(parts, styleDim.Render(filepath.Base(m.projectPath)))
	}

	if len(m.logFiles) > 0 {
		parts = append(parts, styleDim.Render(fmt.Sprintf("%d files", len(m.logFiles))))
	}

	// level filter indicator in log viewer
	if m.screen == screenLogView && m.levelFilter != "" {
		parts = append(parts, levelStyle(m.levelFilter).Render("["+m.levelFilter+"]"))
	}
	if m.screen == screenLogView && m.searchQuery != "" {
		parts = append(parts, styleDim.Render("search: "+m.searchQuery))
	}

	title := strings.Join(parts, "  ")
	return styleHeader.Width(m.width).Render(title)
}

// --- footer ---

func (m Model) renderFooter() string {
	var hints string
	switch m.screen {
	case screenMain:
		hints = "j/k: move  enter: select  q: quit"
	case screenInstall:
		hints = "enter: install  esc: back"
	case screenLogs:
		hints = "j/k: move  enter/v: view  d: delete  c: clear old  esc: back"
	case screenLogView:
		if m.searching {
			hints = fmt.Sprintf("search: %s█  enter: confirm  esc: cancel", m.searchQuery)
		} else {
			hints = "j/k: scroll  enter: detail  /: search  e/w/i/d: filter level  c: clear  esc: back"
		}
	case screenSettings:
		hints = "+/-: retention days  esc: back"
	case screenGlobalProjects:
		hints = "j/k: move  enter: select  esc: back"
	}

	if m.message != "" {
		hints = m.message
	}

	return styleFooter.Width(m.width).Render(hints)
}

// --- screens ---

func (m Model) renderMain() string {
	var sb strings.Builder

	// project status card
	if m.language != nil {
		sb.WriteString(styleBold.Render("project") + "  " + m.projectPath + "\n")
		sb.WriteString(styleBold.Render("language") + " " + m.language.Name() + "\n")
		sb.WriteString(styleBold.Render("logs") + "     " + fmt.Sprintf("%d file(s)", len(m.logFiles)) + "\n")
	} else {
		sb.WriteString(styleDim.Render("no supported project detected in "+m.projectPath) + "\n")
	}
	sb.WriteString("\n")

	options := []string{
		"Install / Setup Logger",
		"View Logs",
		"View All Logs (Global)",
		"Settings",
		"Quit",
	}

	for i, opt := range options {
		if i == m.cursor {
			sb.WriteString(styleSelected.Render(fmt.Sprintf("  > %-30s", opt)))
		} else {
			sb.WriteString(styleNormal.Render(fmt.Sprintf("    %-30s", opt)))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m Model) renderInstall() string {
	if m.language == nil {
		return styleDim.Render("no supported language detected\n\npress esc to go back")
	}
	var sb strings.Builder
	sb.WriteString(styleBold.Render("Install Logger") + "\n\n")
	sb.WriteString(fmt.Sprintf("Project: %s (%s)\n\n", filepath.Base(m.projectPath), m.language.Name()))
	sb.WriteString("This will create:\n")
	sb.WriteString(styleDim.Render("  internal/logdog/logger.go\n"))
	sb.WriteString(styleDim.Render("  ~/logdog/"+filepath.Base(m.projectPath)+"/\n\n"))
	sb.WriteString("Press enter to install, esc to cancel.")
	return sb.String()
}

func (m Model) renderLogs(contentH int) string {
	if len(m.logFiles) == 0 {
		return styleDim.Render("no log files found\n\nrun 'logdog install' first")
	}

	var sb strings.Builder
	sb.WriteString(styleBold.Render("Log Files"))
	if m.selectedProject != "" {
		sb.WriteString(styleDim.Render("  " + m.selectedProject))
	}
	sb.WriteString("\n\n")

	// sort by mod time newest first
	type fileEntry struct {
		path    string
		modTime time.Time
		count   int
	}
	var files []fileEntry
	for _, f := range m.logFiles {
		info, _ := os.Stat(f)
		mt := time.Time{}
		if info != nil {
			mt = info.ModTime()
		}
		files = append(files, fileEntry{f, mt, getLogEntryCount(f)})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	visible := files
	if len(visible) > contentH-4 {
		visible = visible[:contentH-4]
	}

	for i, f := range visible {
		name := filepath.Base(f.path)
		date := styleDim.Render(f.modTime.Format("2006-01-02"))
		count := styleDim.Render(fmt.Sprintf("%4d entries", f.count))
		row := fmt.Sprintf("  %-36s %s  %s", name, date, count)
		if i == m.cursor {
			sb.WriteString(styleSelected.Render(row))
		} else {
			sb.WriteString(styleNormal.Render(row))
		}
		sb.WriteString("\n")
	}

	if m.confirmingDelete || m.confirmingClear {
		sb.WriteString("\n" + styleWarn.Render(m.message))
	}

	return sb.String()
}

func (m Model) renderLogView(contentH int) string {
	entries := m.filteredEntries()

	var sb strings.Builder

	// stats line
	total := len(m.logEntries)
	showing := len(entries)
	if m.levelFilter != "" || m.searchQuery != "" {
		sb.WriteString(styleDim.Render(fmt.Sprintf("showing %d / %d entries", showing, total)))
	} else {
		sb.WriteString(styleDim.Render(fmt.Sprintf("%d entries", total)))
	}
	sb.WriteString("\n\n")

	if len(entries) == 0 {
		sb.WriteString(styleDim.Render("no entries match filter"))
		return sb.String()
	}

	// clamp scroll
	maxScroll := len(entries) - 1
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := m.logScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}

	viewH := contentH - 3
	if viewH < 1 {
		viewH = 1
	}
	end := scroll + viewH
	if end > len(entries) {
		end = len(entries)
	}

	for i, e := range entries[scroll:end] {
		line := formatLogLine(e, m.width-4)
		if scroll+i == scroll { // highlight "cursor" line for enter→detail
			sb.WriteString(styleNormal.Render(line))
		} else {
			sb.WriteString(line)
		}
		sb.WriteString("\n")
	}

	// scroll indicator
	if len(entries) > viewH {
		pct := int(float64(scroll) / float64(maxScroll) * 100)
		sb.WriteString(styleDim.Render(fmt.Sprintf("\n  [%d%%  line %d/%d]", pct, scroll+1, len(entries))))
	}

	return sb.String()
}

func formatLogLine(e LogEntry, maxWidth int) string {
	ts := formatTimestamp(e.Timestamp)
	lvl := levelStyle(strings.ToUpper(e.Level)).Render(fmt.Sprintf("%-5s", strings.ToUpper(e.Level)))
	msg := styleNormal.Render(e.Message)

	var dataParts []string
	for k, v := range e.Data {
		dataParts = append(dataParts, fmt.Sprintf("%s=%v", k, v))
	}
	dataStr := ""
	if len(dataParts) > 0 {
		dataStr = "  " + styleDim.Render("{"+strings.Join(dataParts, " ")+"}")
	}

	return styleDim.Render(ts) + "  " + lvl + "  " + msg + dataStr
}

func (m Model) renderDetailOverlay() string {
	e := m.detailEntry
	if e == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(levelStyle(strings.ToUpper(e.Level)).Render(strings.ToUpper(e.Level)))
	sb.WriteString("  ")
	sb.WriteString(styleDim.Render(formatTimestamp(e.Timestamp)))
	sb.WriteString("\n\n")
	sb.WriteString(styleBold.Render(e.Message))
	sb.WriteString("\n")

	if len(e.Data) > 0 {
		sb.WriteString("\n")
		// sort keys for stable display
		keys := make([]string, 0, len(e.Data))
		for k := range e.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString(fmt.Sprintf("  %-20s %v\n", styleDim.Render(k), e.Data[k]))
		}
	}

	sb.WriteString("\n" + styleDim.Render("esc/enter to close"))

	w := m.width / 2
	if w < 50 {
		w = 50
	}
	return styleOverlay.Width(w).Render(sb.String())
}

func (m Model) renderGlobalProjects(contentH int) string {
	if len(m.globalProjects) == 0 {
		return styleDim.Render("no projects found in ~/logdog/")
	}

	var sb strings.Builder
	sb.WriteString(styleBold.Render("All Projects") + "\n\n")

	for i, p := range m.globalProjects {
		files := getLogFilesForProject(p)
		row := fmt.Sprintf("  %-30s  %d file(s)", p, len(files))
		if i == m.cursor {
			sb.WriteString(styleSelected.Render(row))
		} else {
			sb.WriteString(styleNormal.Render(row))
		}
		sb.WriteString("\n")
	}
	_ = contentH
	return sb.String()
}

func (m Model) renderSettings() string {
	var sb strings.Builder
	sb.WriteString(styleBold.Render("Settings") + "\n\n")
	sb.WriteString(fmt.Sprintf("  Log retention:  %s days\n",
		styleBold.Render(fmt.Sprintf("%d", m.retentionDays))))
	sb.WriteString(styleDim.Render("  Use +/- to adjust\n"))
	if m.message != "" {
		sb.WriteString("\n" + styleSuccess.Render(m.message))
	}
	return sb.String()
}

// placeOverlay centers fg over bg string content.
func placeOverlay(bg, fg string, w, h int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	fgH := len(fgLines)
	fgW := 0
	for _, l := range fgLines {
		if lw := lipgloss.Width(l); lw > fgW {
			fgW = lw
		}
	}

	startX := (w - fgW) / 2
	startY := (h - fgH) / 2
	if startX < 0 {
		startX = 0
	}
	if startY < 0 {
		startY = 0
	}

	result := make([]string, len(bgLines))
	copy(result, bgLines)

	for i, fgLine := range fgLines {
		idx := startY + i
		if idx < 0 || idx >= len(bgLines) {
			continue
		}
		bgLine := bgLines[idx]
		bgW := lipgloss.Width(bgLine)

		left := truncateAnsi(bgLine, startX)
		lw := lipgloss.Width(left)
		if lw < startX {
			left += strings.Repeat(" ", startX-lw)
		}

		right := ""
		rs := startX + fgW
		if rs < bgW {
			right = cutAnsi(bgLine, rs, bgW)
		}

		result[idx] = left + fgLine + right
	}

	return strings.Join(result, "\n")
}

// Simple ANSI-unaware truncate/cut — good enough for our overlay use case.
func truncateAnsi(s string, n int) string {
	w := 0
	for i, r := range s {
		if w >= n {
			return s[:i]
		}
		w += lipgloss.Width(string(r))
	}
	return s
}

func cutAnsi(s string, start, _ int) string {
	w := 0
	for i, r := range s {
		rw := lipgloss.Width(string(r))
		if w+rw > start {
			return s[i:]
		}
		w += rw
	}
	return ""
}
