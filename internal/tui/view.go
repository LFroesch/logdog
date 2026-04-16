package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/LFroesch/logdog/internal/config"
	"github.com/LFroesch/logdog/internal/detector"
	"github.com/LFroesch/logdog/internal/logs"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}
	if m.mode == modeHelp {
		header := m.renderHeader()
		sep := dimStyle.Render(strings.Repeat("─", m.width))
		footer := m.renderFooter()
		return lipgloss.JoinVertical(lipgloss.Left, header, sep, m.renderHelp(), sep, footer)
	}

	header := m.renderHeader()
	sep := dimStyle.Render(strings.Repeat("─", m.width))

	var content string
	switch m.page {
	case pageDashboard:
		content = m.renderDashboard()
	case pageSetup:
		content = m.renderSetupPage()
	case pageCleanup:
		content = m.renderCleanupPage()
	}

	var statusLine string
	if m.statusMsg != "" && time.Now().Before(m.statusExpiry) {
		statusLine = statusStyle.Render("  " + m.statusMsg)
	}

	footer := m.renderFooter()
	parts := []string{header, sep, content}
	if statusLine != "" {
		parts = append(parts, statusLine)
	}
	parts = append(parts, sep, footer)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) renderHeader() string {
	tabs := []struct {
		name string
		page page
	}{
		{"Dashboard", pageDashboard},
		{"Setup", pageSetup},
		{"Cleanup", pageCleanup},
	}

	var renderedTabs []string
	for i, tab := range tabs {
		if i > 0 {
			renderedTabs = append(renderedTabs, dimStyle.Render(" │ "))
		}
		if tab.page == m.page {
			renderedTabs = append(renderedTabs, activeTabStyle.Render(tab.name))
		} else {
			renderedTabs = append(renderedTabs, dimStyle.Render(tab.name))
		}
	}

	left := titleStyle.Render("logdog") + "  " + strings.Join(renderedTabs, "")
	right := m.headerStatusText()
	switch m.mode {
	case modeSearch:
		right = "/ " + m.searchInput + "█"
	case modeInput:
		right = "add root: " + m.inputBuf + "█"
	}
	return joinHeaderSides(left, dimStyle.Render(trim(right, max(12, m.width/2))), m.width)
}

func (m Model) renderFooter() string {
	var parts []string
	add := func(key, action string) {
		if len(parts) > 0 {
			parts = append(parts, bulletStyle.Render(" · "))
		}
		parts = append(parts, keyStyle.Render(key), " ", actionStyle.Render(action))
	}

	switch m.mode {
	case modeSearch:
		add("type", "search")
		add("enter", "apply")
		add("esc", "cancel")
		return strings.Join(parts, "")
	case modeInput:
		add("type", "path")
		add("enter", "add root")
		add("esc", "cancel")
		return strings.Join(parts, "")
	}

	switch m.page {
	case pageDashboard:
		add("tab", "panel")
		add("j/k", "move")
		add("enter", "focus/open")
		if m.focus == focusViewer {
			add("esc", "back")
		}
		add("/", "search")
		add("y", m.copyHint())
		if m.focus == focusViewer {
			add("Y", "copy raw")
		}
		add("l", "level")
		add("t", "follow")
		add("o", "open")
		add("pgup/pgdn", "page")
	case pageSetup:
		add("i", "install")
		add("n", "add root")
		add("a/A", "add cwd/parent")
		add("enter", "open root")
		add("d", "remove root")
		add("y", "copy path")
		add("o", "open config")
	case pageCleanup:
		add("space", "select")
		add("x", "delete")
		add("D", "delete selected")
		add("X", "prune old")
		add("y", "copy path")
		add("o", "open")
	}

	add("1-3", "pages")
	add("r", "reload")
	add("?", "help")
	if m.page == pageDashboard {
		add("q", "quit")
	} else {
		add("q", "dashboard")
	}

	return strings.Join(parts, "")
}

func (m Model) renderDashboard() string {
	leftW, rightW := splitPaneWidths(m.width, 42, 40)
	left := m.renderFilesPane(leftW, m.focus == focusFiles)
	right := m.renderViewerPane(rightW, m.focus == focusViewer)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m Model) renderFilesPane(width int, active bool) string {
	header := fmt.Sprintf("Logs (%d)", len(m.files))
	if m.search != "" && len(m.files) < len(m.allFiles) {
		header = fmt.Sprintf("Logs (%d/%d filtered)", len(m.files), len(m.allFiles))
	}
	lines := []string{panelHeaderStyle.Render(header)}
	if len(m.files) == 0 {
		lines = append(lines, "", dimStyle.Render("No logs found"), "", dimStyle.Render("logdog always scans the current directory recursively."))
		return renderPanel(panelStyleFor(active), width, m.paneHeight(), lines)
	}

	start, end := entryWindow(len(m.files), m.fileScroll, max(1, m.paneHeight()-2))
	lastGroup := ""
	for i := start; i < end; i++ {
		file := m.files[i]
		group := m.fileGroupLabel(file, width-6)
		if group != lastGroup {
			if lastGroup != "" {
				lines = append(lines, "")
			}
			lines = append(lines, dimStyle.Render(group+"/"))
			lastGroup = group
		}
		name := trim(filepath.Base(file.Path), max(12, width/2))
		meta := dimStyle.Render(fmt.Sprintf("%s  %s", byteLabel(file.Size), file.ModTime.Format("01-02 15:04")))
		row := joinPaneSides("    "+name, meta, width-4)
		if i == m.fileCursor {
			row = selectedItemStyle.Render(joinPaneSides("  ▸ "+name, meta, width-4))
		}
		lines = append(lines, row)
	}
	return renderPanel(panelStyleFor(active), width, m.paneHeight(), lines)
}

func (m Model) renderViewerPane(width int, active bool) string {
	lines := []string{panelHeaderStyle.Render("Viewer")}
	if len(m.files) == 0 {
		lines = append(lines, "", dimStyle.Render("Select a log file"))
		return renderPanel(panelStyleFor(active), width, m.paneHeight(), lines)
	}

	file := m.files[m.fileCursor]
	lines = append(lines,
		labelStyle.Render("Path")+detailStyle.Render(trimMiddle(file.Path, width-10)),
		labelStyle.Render("Dir")+detailStyle.Render(trimMiddle(filepath.Dir(file.Path), width-10)),
		labelStyle.Render("Type")+detailStyle.Render(string(file.Format)),
		labelStyle.Render("Size")+detailStyle.Render(byteLabel(file.Size)),
		labelStyle.Render("Lines")+detailStyle.Render(intStr(file.EntryCount)),
		labelStyle.Render("Mod")+detailStyle.Render(file.ModTime.Format("2006-01-02 15:04:05")),
		"",
	)

	if len(m.entries) == 0 {
		lines = append(lines, dimStyle.Render("No entries match current filters"))
		return renderPanel(panelStyleFor(active), width, m.paneHeight(), lines)
	}

	listHeight := m.viewerEntryListHeight()
	start, end := entryWindow(len(m.entries), m.entryScroll, listHeight)
	for i := start; i < end; i++ {
		row := formatEntryRow(m.entries[i], width-4)
		if i == m.entryCursor {
			row = selectedItemStyle.Render(row)
		}
		lines = append(lines, row)
	}
	lines = append(lines, "")
	lines = append(lines, panelHeaderStyle.Render("Detail"))
	lines = append(lines, m.renderEntryDetail(width-4, max(4, m.paneHeight()-len(lines)-3))...)
	return renderPanel(panelStyleFor(active), width, m.paneHeight(), lines)
}

func (m Model) renderSetupPage() string {
	leftW, rightW := splitPaneWidths(m.width, 38, 40)
	left := m.renderProjectPane(leftW)
	right := m.renderSetupPane(rightW)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m Model) renderProjectPane(width int) string {
	defaultRoot := config.ResolvePaths().DataDir
	lines := []string{
		panelHeaderStyle.Render("Project"),
		"",
		labelStyle.Render("Name") + detailStyle.Render(m.projectName),
		labelStyle.Render("Path") + detailStyle.Render(trimMiddle(m.projectPath, width-10)),
		labelStyle.Render("Lang") + detailStyle.Render(languageName(m.language)),
		labelStyle.Render("Setup") + statusValue(m.installed),
		labelStyle.Render("Logs") + detailStyle.Render(trimMiddle(config.ProjectLogDir(m.projectPath), width-10)),
		"",
		panelHeaderStyle.Render("Discovery"),
		"Current project is always scanned.",
		labelStyle.Render("Default") + detailStyle.Render(trimMiddle(defaultRoot, width-10)),
		"Configured roots are extra search locations.",
	}
	return renderPanel(panelStyle, width, m.paneHeight(), lines)
}

func (m Model) renderSetupPane(width int) string {
	defaultRoot := config.ResolvePaths().DataDir
	lines := []string{
		panelHeaderStyle.Render("Roots"),
		"",
		labelStyle.Render("Config") + detailStyle.Render(trimMiddle(m.configPath, width-10)),
		labelStyle.Render("Roots") + detailStyle.Render(fmt.Sprintf("%d extra configured", len(m.cfg.Roots))),
		labelStyle.Render("Default") + detailStyle.Render(trimMiddle(defaultRoot, width-10)),
		"",
		panelHeaderStyle.Render("Install"),
		"Press " + keyStyle.Render("i") + " to install/update logger files for this project.",
		"",
		panelHeaderStyle.Render("Extra Roots"),
		"Press " + keyStyle.Render("n") + " to add a custom directory root.",
	}
	if len(m.cfg.Roots) == 0 {
		lines = append(lines, "  "+dimStyle.Render("No extra roots configured"))
	} else {
		start, end := entryWindow(len(m.cfg.Roots), m.rootScroll, max(4, m.paneHeight()-12))
		for i := start; i < end; i++ {
			root := trimMiddle(m.cfg.Roots[i], width-8)
			row := "  " + root
			if i == m.rootCursor && m.focus == focusSetupRoots {
				row = selectedItemStyle.Render("▸ " + root)
			}
			lines = append(lines, row)
		}
	}
	return renderPanel(panelStyleFor(m.focus == focusSetupRoots), width, m.paneHeight(), lines)
}

func (m Model) renderCleanupPage() string {
	leftW, rightW := splitPaneWidths(m.width, 42, 40)
	left := m.renderCleanupList(leftW)
	right := m.renderCleanupDetail(rightW)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m Model) renderCleanupList(width int) string {
	lines := []string{panelHeaderStyle.Render(fmt.Sprintf("Cleanup (%d)", len(m.cleanup)))}
	if len(m.cleanup) == 0 {
		lines = append(lines, "", dimStyle.Render("No cleanup candidates"))
		return renderPanel(panelStyleFor(m.focus == focusCleanupList), width, m.paneHeight(), lines)
	}
	start, end := entryWindow(len(m.cleanup), m.cleanupScroll, max(1, m.paneHeight()-2))
	for i := start; i < end; i++ {
		item := m.cleanup[i]
		name := trim(filepath.Base(item.Path), max(12, width/2))
		metaParts := []string{item.Kind, byteLabel(item.Size), item.ModTime.Format("01-02 15:04")}
		if m.selected[item.Path] {
			metaParts = append([]string{"selected"}, metaParts...)
		}
		meta := dimStyle.Render(strings.Join(metaParts, "  "))
		row := joinPaneSides("  "+name, meta, width-4)
		if i == m.cleanupCursor {
			row = selectedItemStyle.Render(joinPaneSides("▸ "+name, meta, width-4))
		}
		lines = append(lines, row)
	}
	return renderPanel(panelStyleFor(m.focus == focusCleanupList), width, m.paneHeight(), lines)
}

func (m Model) renderCleanupDetail(width int) string {
	lines := []string{panelHeaderStyle.Render("Detail")}
	if len(m.cleanup) == 0 {
		lines = append(lines, "", dimStyle.Render("Select a cleanup candidate"))
		return renderPanel(panelStyleFor(m.focus == focusCleanupDetail), width, m.paneHeight(), lines)
	}
	item := m.cleanup[m.cleanupCursor]
	lines = append(lines,
		labelStyle.Render("Path")+detailStyle.Render(trimMiddle(item.Path, width-10)),
		labelStyle.Render("Kind")+detailStyle.Render(item.Kind),
		labelStyle.Render("Size")+detailStyle.Render(byteLabel(item.Size)),
		labelStyle.Render("Mod")+detailStyle.Render(item.ModTime.Format("2006-01-02 15:04:05")),
		"",
		panelHeaderStyle.Render("Policy"),
		detailStyle.Render(fmt.Sprintf("Prune threshold: %d days", m.cfg.CleanupKeepDays)),
	)
	return renderPanel(panelStyleFor(m.focus == focusCleanupDetail), width, m.paneHeight(), lines)
}

func (m Model) renderEntryDetail(width, height int) []string {
	if len(m.entries) == 0 || m.entryCursor >= len(m.entries) {
		return []string{dimStyle.Render("Select an entry")}
	}
	entry := m.entries[m.entryCursor]
	lines := []string{
		labelStyle.Render("Time") + detailStyle.Render(formatTime(entry.Timestamp)),
		labelStyle.Render("Level") + levelStyleText(entry.Level),
		labelStyle.Render("Msg") + detailStyle.Render(trim(entry.Message, width-8)),
	}
	if len(entry.Data) == 0 {
		lines = append(lines, dimStyle.Render(trim(entry.Raw, width)))
		return lines
	}
	keys := make([]string, 0, len(entry.Data))
	for key := range entry.Data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		lines = append(lines, dimStyle.Render(trim(fmt.Sprintf("%s=%v", key, entry.Data[key]), width)))
	}
	if height <= 0 || len(lines) <= height {
		return lines
	}
	maxScroll := len(lines) - height
	scroll := m.detailScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}
	start := scroll
	end := min(len(lines), start+height)
	out := append([]string{}, lines[start:end]...)
	if end < len(lines) && len(out) > 0 {
		out[len(out)-1] = dimStyle.Render("... more detail below")
	}
	return out
}

func (m Model) renderHelp() string {
	lines := []string{
		titleStyle.Render("Logdog Help"),
		"",
		panelHeaderStyle.Render("Pages"),
		"  1  dashboard: browse files and inspect entries",
		"  2  setup: install logger and manage discovery roots",
		"  3  cleanup: delete rotated/archive logs and prune old data",
		"",
		panelHeaderStyle.Render("Dashboard"),
		"  tab / shift+tab  switch list vs viewer",
		"  enter            focus viewer or open file in editor",
		"  esc              return to file list from viewer",
		"  o                open selected file immediately",
		"  /                search/filter by text (hides non-matching files)",
		"  y                copy selected path or entry",
		"  Y                copy raw entry in viewer",
		"  l                cycle level filter",
		"  t                toggle live follow",
		"  pgup/pgdn        page entry list",
		"  ctrl+u / ctrl+d  page entry list",
		"",
		panelHeaderStyle.Render("Setup"),
		"  i                install logger into current Go project",
		"  n                add a custom directory root (type path + enter)",
		"  a / A            add project or parent directory to roots",
		"  enter            open selected configured root",
		"  d                remove selected configured root",
		"  y                copy selected root path",
		"  o                open config JSON in editor",
		"",
		panelHeaderStyle.Render("CLI"),
		"  logdog status              show discovery summary",
		"  logdog files               list discovered logs",
		"  logdog cat [file]          print parsed entries",
		"  logdog grep <pattern>      search logs or stdin",
		"  logdog tail [file]         follow a file",
		"  logdog config              print config and paths",
		"  logdog config add-root DIR add discovery root",
		"  logdog config remove-root DIR",
		"",
		panelHeaderStyle.Render("Cleanup"),
		"  space            select candidate",
		"  x                delete current candidate",
		"  D                delete selected candidates",
		"  X                prune by age",
		"",
		panelHeaderStyle.Render("Navigation"),
		"  g / G            top / bottom",
		"  j / k            move down / up",
		"  q                quit or back to dashboard",
		"  r                reload",
		"  ?                toggle this help  (j/k/g/G to scroll)",
	}

	padding := helpStyle.GetVerticalPadding()
	available := m.contentHeight() - padding
	if available < 3 {
		available = 3
	}

	maxScroll := max(0, len(lines)-available+1)
	scroll := m.helpScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}

	end := min(len(lines), scroll+available-1)
	visible := append([]string{}, lines[scroll:end]...)

	indicator := fmt.Sprintf("  %d/%d", end, len(lines))
	if end < len(lines) {
		indicator += "  ↓ more"
	}
	visible = append(visible, dimStyle.Render(indicator))

	return helpStyle.Width(m.width).Height(m.contentHeight()).Render(strings.Join(visible, "\n"))
}

func formatEntryRow(entry logs.Entry, width int) string {
	ts := formatTime(entry.Timestamp)
	level := strings.ToUpper(entry.Level)
	if level == "" {
		level = "LINE"
	}
	msg := trim(entry.Message, width-24)
	row := fmt.Sprintf("  %s  %-5s  %s", ts, level, msg)
	switch level {
	case "ERROR", "FATAL":
		return errorTextStyle.Render(row)
	case "WARN", "WARNING":
		return warnStyle.Render(row)
	case "INFO":
		return currentStyle.Render(row)
	case "DEBUG", "TRACE":
		return dimStyle.Render(row)
	default:
		return row
	}
}

func (m Model) headerStatusText() string {
	switch m.page {
	case pageDashboard:
		parts := []string{fmt.Sprintf("%d files", len(m.files))}
		if len(m.files) > 0 {
			parts = append(parts, trim(filepath.Base(m.files[m.fileCursor].Path), 28))
		}
		parts = append(parts, fmt.Sprintf("%d entries", len(m.entries)))
		if m.levelFilter != "" {
			parts = append(parts, "level "+m.levelFilter)
		}
		if m.search != "" {
			parts = append(parts, fmt.Sprintf("search %q", trim(m.search, 18)))
		}
		if m.tailing {
			parts = append(parts, "live")
		}
		return strings.Join(parts, " · ")
	case pageSetup:
		return fmt.Sprintf("%s · %d roots · %s", m.projectName, len(m.cfg.Roots), trimMiddle(m.configPath, 28))
	case pageCleanup:
		return fmt.Sprintf("%d candidates · %d selected · keep %d days", len(m.cleanup), len(m.selected), m.cfg.CleanupKeepDays)
	default:
		return m.projectName
	}
}

func panelStyleFor(active bool) lipgloss.Style {
	if active {
		return panelActiveStyle
	}
	return panelStyle
}

func renderPanel(style lipgloss.Style, width, height int, lines []string) string {
	if height < 1 {
		height = 1
	}
	if len(lines) > height {
		lines = append([]string{}, lines[:height]...)
		if height > 0 {
			lines[height-1] = dimStyle.Render("...")
		}
	}
	return style.Width(width).Height(height).Render(strings.Join(lines, "\n"))
}

func levelStyle(level string) lipgloss.Style {
	switch strings.ToUpper(level) {
	case "ERROR", "FATAL":
		return errorTextStyle
	case "WARN", "WARNING":
		return warnStyle
	case "INFO":
		return currentStyle
	case "DEBUG", "TRACE":
		return dimStyle
	default:
		return dimStyle
	}
}

func levelStyleText(level string) string {
	if level == "" {
		level = "LINE"
	}
	return levelStyle(level).Render(strings.ToUpper(level))
}

func statusValue(installed bool) string {
	if installed {
		return accentStyle.Render("installed")
	}
	return warnStyle.Render("not installed")
}

func languageName(lang detector.Language) string {
	if lang == nil {
		return "unknown"
	}
	return lang.Name()
}

func formatTime(value string) string {
	if value == "" {
		return "-- --:--:--"
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("01-02 15:04:05")
		}
	}
	return trim(value, 14)
}
