package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/LFroesch/logdog/internal/config"
	"github.com/LFroesch/logdog/internal/detector"
	"github.com/LFroesch/logdog/internal/logs"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type page int

const (
	pageDashboard page = iota
	pageSetup
	pageCleanup
)

type mode int

const (
	modeNormal mode = iota
	modeHelp
	modeSearch
	modeInput
	modeConfirm
)

type focusSection int

const (
	focusFiles focusSection = iota
	focusViewer
	focusSetupRoots
	focusCleanupList
	focusCleanupDetail
)

type tickMsg time.Time

type editorDoneMsg struct {
	err error
}

type copyDoneMsg struct {
	label string
	err   error
}

type Model struct {
	store       logs.Store
	cfg         config.Config
	projectPath string
	projectName string
	language    detector.Language
	loggerPath  string

	width  int
	height int

	page  page
	mode  mode
	focus focusSection

	statusMsg    string
	statusExpiry time.Time

	search      string
	searchInput string
	levelFilter string
	tailing     bool

	inputBuf   string
	helpScroll int

	confirmKind   string
	confirmPrompt string

	allFiles   []logs.FileInfo
	files      []logs.FileInfo
	fileCursor int
	fileScroll int

	entries     []logs.Entry
	entryCursor int
	entryScroll int

	cleanup       []logs.CleanupCandidate
	cleanupCursor int
	cleanupScroll int
	selected      map[string]bool

	configPath string
	installed  bool

	rootCursor int
	rootScroll int

	detailScroll int
}

func NewModel(projectPath string) Model {
	if projectPath == "" {
		projectPath, _ = filepath.Abs(".")
	}
	absPath, _ := filepath.Abs(projectPath)
	cfg, _ := config.Ensure()
	store := logs.NewStoreWithConfig(absPath, cfg)
	lang := detector.DetectLanguage(absPath)
	loggerPath := filepath.Join(absPath, "internal", "logdog", "logger.go")
	if lang != nil {
		loggerPath = lang.InstalledPath(absPath)
	}
	m := Model{
		store:       store,
		cfg:         cfg,
		projectPath: absPath,
		projectName: filepath.Base(absPath),
		language:    lang,
		loggerPath:  loggerPath,
		page:        pageDashboard,
		mode:        modeNormal,
		focus:       focusFiles,
		selected:    map[string]bool{},
		configPath:  config.ResolvePaths().ConfigFile,
		levelFilter: strings.ToUpper(cfg.DefaultLevelFilter),
	}
	m.reloadAll()
	return m
}

func (m Model) Init() tea.Cmd {
	return m.nextTickCmd()
}

// --- State management ---

func (m *Model) reloadAll() {
	m.installed = fileExists(m.loggerPath)
	m.reloadFiles()
	m.reloadEntries()
	m.reloadCleanup()
}

func (m *Model) reloadFiles() {
	currentPath := ""
	if len(m.files) > 0 && m.fileCursor >= 0 && m.fileCursor < len(m.files) {
		currentPath = m.files[m.fileCursor].Path
	}
	files, err := m.store.DiscoverFiles()
	if err != nil {
		m.showStatus(err.Error())
		return
	}
	m.allFiles = files
	m.applyFileFilter()
	if currentPath != "" {
		for i, file := range m.files {
			if file.Path == currentPath {
				m.fileCursor = i
				m.ensureFileVisible()
				return
			}
		}
	}
	if m.fileCursor >= len(m.files) {
		m.fileCursor = max(0, len(m.files)-1)
	}
	m.ensureFileVisible()
}

// applyFileFilter sets m.files to the subset of m.allFiles that contain entries
// matching the current search and level filter. When search is empty, m.files == m.allFiles.
func (m *Model) applyFileFilter() {
	if m.search == "" {
		m.files = m.allFiles
		return
	}
	query := logs.Query{Level: m.levelFilter, Search: m.search, Limit: 1}
	var filtered []logs.FileInfo
	for _, file := range m.allFiles {
		entries, err := m.store.ReadEntries(file.Path, query)
		if err == nil && len(entries) > 0 {
			filtered = append(filtered, file)
		}
	}
	m.files = filtered
}

func (m *Model) reloadEntries() {
	if len(m.files) == 0 {
		m.entries = nil
		m.entryCursor = 0
		m.entryScroll = 0
		m.detailScroll = 0
		return
	}
	query := logs.Query{
		Level:   m.levelFilter,
		Search:  m.search,
		Limit:   m.cfg.MaxPreviewLines,
		Reverse: false,
	}
	entries, err := m.store.ReadEntries(m.files[m.fileCursor].Path, query)
	if err != nil {
		m.showStatus(err.Error())
		return
	}
	m.entries = entries
	// Update entry count now that we've read the file
	m.files[m.fileCursor] = logs.FileInfo{
		Path:       m.files[m.fileCursor].Path,
		Root:       m.files[m.fileCursor].Root,
		Name:       m.files[m.fileCursor].Name,
		Size:       m.files[m.fileCursor].Size,
		ModTime:    m.files[m.fileCursor].ModTime,
		Format:     m.files[m.fileCursor].Format,
		Compressed: m.files[m.fileCursor].Compressed,
		Rotated:    m.files[m.fileCursor].Rotated,
		EntryCount: len(entries),
	}
	if m.entryCursor >= len(m.entries) {
		m.entryCursor = max(0, len(m.entries)-1)
	}
	m.detailScroll = 0
	m.ensureEntryVisible()
}

// reloadEntriesOnly refreshes entries without resetting detailScroll — used during live tail
// so that the detail panel scroll position is preserved between ticks.
func (m *Model) reloadEntriesOnly() {
	if len(m.files) == 0 {
		m.entries = nil
		m.entryCursor = 0
		m.entryScroll = 0
		return
	}
	query := logs.Query{
		Level:   m.levelFilter,
		Search:  m.search,
		Limit:   m.cfg.MaxPreviewLines,
		Reverse: false,
	}
	entries, err := m.store.ReadEntries(m.files[m.fileCursor].Path, query)
	if err != nil {
		m.showStatus(err.Error())
		return
	}
	savedDetailScroll := m.detailScroll
	m.entries = entries
	if m.entryCursor >= len(m.entries) {
		m.entryCursor = max(0, len(m.entries)-1)
	}
	m.detailScroll = savedDetailScroll
	m.ensureEntryVisible()
}

func (m *Model) reloadCleanup() {
	candidates, err := m.store.DiscoverCleanupCandidates()
	if err != nil {
		m.showStatus(err.Error())
		return
	}
	m.cleanup = candidates
	if m.cleanupCursor >= len(m.cleanup) {
		m.cleanupCursor = max(0, len(m.cleanup)-1)
	}
	m.ensureCleanupVisible()
}

func (m *Model) reloadConfig() {
	cfg, err := config.Load()
	if err != nil {
		m.showStatus("config reload failed: " + err.Error())
		return
	}
	m.cfg = cfg
	m.store = logs.NewStoreWithConfig(m.projectPath, cfg)
}

func (m *Model) installLogger() {
	if m.language == nil {
		m.showStatus("no supported project detected")
		return
	}
	cfg := detector.Config{
		LogLevel:   "INFO",
		OutputDir:  config.ProjectLogDir(m.projectPath),
		MaxFiles:   30,
		DateFormat: "2006-01-02",
	}
	if err := m.language.Install(m.projectPath, cfg); err != nil {
		m.showStatus("install failed: " + err.Error())
		return
	}
	m.installed = true
	m.reloadAll()
	m.showStatus("logger installed")
}

func (m *Model) cycleLevel() {
	levels := []string{"", "ERROR", "WARN", "INFO", "DEBUG"}
	for i, level := range levels {
		if m.levelFilter == level {
			m.levelFilter = levels[(i+1)%len(levels)]
			break
		}
	}
	if m.levelFilter == "" {
		m.showStatus("level filter cleared")
	} else {
		m.showStatus("level: " + m.levelFilter)
	}
}

// --- Navigation ---

func (m *Model) selectFile(index int) {
	if len(m.files) == 0 {
		m.fileCursor = 0
		m.fileScroll = 0
		m.entries = nil
		m.entryCursor = 0
		m.entryScroll = 0
		m.detailScroll = 0
		return
	}
	if index < 0 {
		index = 0
	}
	if index >= len(m.files) {
		index = len(m.files) - 1
	}
	if index == m.fileCursor {
		m.ensureFileVisible()
		return
	}
	m.fileCursor = index
	m.entryCursor = 0
	m.entryScroll = 0
	m.detailScroll = 0
	m.reloadEntries()
	m.ensureFileVisible()
}

func (m *Model) moveTop() {
	switch m.page {
	case pageDashboard:
		if m.focus == focusFiles {
			m.selectFile(0)
		} else {
			m.entryCursor = 0
			m.entryScroll = 0
		}
	case pageCleanup:
		m.cleanupCursor = 0
		m.cleanupScroll = 0
	}
}

func (m *Model) moveBottom() {
	switch m.page {
	case pageDashboard:
		if m.focus == focusFiles {
			if len(m.files) > 0 {
				m.selectFile(len(m.files) - 1)
			}
		} else if len(m.entries) > 0 {
			m.entryCursor = len(m.entries) - 1
			m.ensureEntryVisible()
		}
	case pageCleanup:
		if len(m.cleanup) > 0 {
			m.cleanupCursor = len(m.cleanup) - 1
			m.ensureCleanupVisible()
		}
	}
}

func (m *Model) pageDown() {
	switch m.page {
	case pageDashboard:
		if m.focus == focusViewer && len(m.entries) > 0 {
			step := max(1, m.paneHeight()/4)
			m.entryCursor = min(len(m.entries)-1, m.entryCursor+step)
			m.ensureEntryVisible()
			m.detailScroll = 0
		}
	case pageSetup:
		if m.focus == focusSetupRoots && len(m.cfg.Roots) > 0 {
			step := max(1, m.paneHeight()/4)
			m.rootCursor = min(len(m.cfg.Roots)-1, m.rootCursor+step)
			m.ensureRootVisible()
		}
	}
}

func (m *Model) pageUp() {
	switch m.page {
	case pageDashboard:
		if m.focus == focusViewer && len(m.entries) > 0 {
			step := max(1, m.paneHeight()/4)
			m.entryCursor = max(0, m.entryCursor-step)
			m.ensureEntryVisible()
			m.detailScroll = 0
		}
	case pageSetup:
		if m.focus == focusSetupRoots && len(m.cfg.Roots) > 0 {
			step := max(1, m.paneHeight()/4)
			m.rootCursor = max(0, m.rootCursor-step)
			m.ensureRootVisible()
		}
	}
}

func (m *Model) ensureFileVisible() {
	if len(m.files) == 0 {
		m.fileScroll = 0
		return
	}
	if m.fileCursor < 0 {
		m.fileCursor = 0
	}
	if m.fileCursor >= len(m.files) {
		m.fileCursor = len(m.files) - 1
	}
	if m.fileCursor < m.fileScroll {
		m.fileScroll = m.fileCursor
	}
	rows := m.fileRowBudget()
	for m.fileScroll <= m.fileCursor {
		count := m.countFilesInWindow(m.fileScroll, rows)
		if m.fileScroll+count > m.fileCursor {
			break
		}
		m.fileScroll++
	}
	if m.fileScroll < 0 {
		m.fileScroll = 0
	}
}

// fileRowBudget is the number of content rows available for the file list,
// inside the panel frame and below the "Logs (N)" header.
func (m Model) fileRowBudget() int {
	return max(1, m.paneHeight()-1)
}

// countFilesInWindow returns how many files starting at `scroll` can be rendered
// within `rows` content rows, accounting for group-header overhead (a blank
// separator + the group label on each group change).
func (m Model) countFilesInWindow(scroll, rows int) int {
	if rows < 1 || scroll >= len(m.files) {
		return 0
	}
	lastGroup := ""
	used := 0
	count := 0
	for i := scroll; i < len(m.files); i++ {
		key := logs.FileSortGroup(m.files[i])
		overhead := 0
		if key != lastGroup {
			overhead = 1
			if lastGroup != "" {
				overhead = 2
			}
		}
		if used+overhead+1 > rows {
			break
		}
		used += overhead + 1
		count++
		lastGroup = key
	}
	return count
}

func (m *Model) ensureEntryVisible() {
	visible := m.viewerEntryListHeight()
	if m.entryCursor < m.entryScroll {
		m.entryScroll = m.entryCursor
	}
	if m.entryCursor >= m.entryScroll+visible {
		m.entryScroll = m.entryCursor - visible + 1
	}
	if m.entryScroll < 0 {
		m.entryScroll = 0
	}
}

func (m *Model) ensureCleanupVisible() {
	visible := max(3, m.paneHeight()-4)
	if m.cleanupCursor < m.cleanupScroll {
		m.cleanupScroll = m.cleanupCursor
	}
	if m.cleanupCursor >= m.cleanupScroll+visible {
		m.cleanupScroll = m.cleanupCursor - visible + 1
	}
	if m.cleanupScroll < 0 {
		m.cleanupScroll = 0
	}
}

func (m *Model) ensureRootVisible() {
	visible := max(3, m.paneHeight()-12)
	if m.rootCursor < m.rootScroll {
		m.rootScroll = m.rootCursor
	}
	if m.rootCursor >= m.rootScroll+visible {
		m.rootScroll = m.rootCursor - visible + 1
	}
	if m.rootScroll < 0 {
		m.rootScroll = 0
	}
}

func (m *Model) askConfirm(kind, prompt string) {
	m.mode = modeConfirm
	m.confirmKind = kind
	m.confirmPrompt = prompt
}

func (m *Model) clearConfirm() {
	m.mode = modeNormal
	m.confirmKind = ""
	m.confirmPrompt = ""
}

func (m *Model) showStatus(msg string) {
	m.statusMsg = msg
	m.statusExpiry = time.Now().Add(3 * time.Second)
}

func (m Model) nextFocus() focusSection {
	order := m.focusOrder()
	for i, current := range order {
		if current == m.focus {
			return order[(i+1)%len(order)]
		}
	}
	return order[0]
}

func (m Model) prevFocus() focusSection {
	order := m.focusOrder()
	for i, current := range order {
		if current == m.focus {
			return order[(i-1+len(order))%len(order)]
		}
	}
	return order[0]
}

func (m Model) focusOrder() []focusSection {
	switch m.page {
	case pageCleanup:
		return []focusSection{focusCleanupList, focusCleanupDetail}
	case pageDashboard:
		return []focusSection{focusFiles, focusViewer}
	case pageSetup:
		return []focusSection{focusSetupRoots}
	default:
		return []focusSection{focusViewer}
	}
}

func (m Model) dashboardPanelAtX(x int) focusSection {
	leftWidth, _ := splitPaneWidths(m.width, 42, 40)
	leftPaneTotal := leftWidth + panelStyle.GetHorizontalFrameSize()
	if x < leftPaneTotal {
		return focusFiles
	}
	return focusViewer
}

// --- Config / root management ---

func (m *Model) addRoot(path string) bool {
	if path == "" {
		return false
	}
	for _, existing := range m.cfg.Roots {
		if pathsMatch(existing, path) {
			return false
		}
	}
	next := append([]string{}, m.cfg.Roots...)
	next = append(next, path)
	m.cfg.Roots = next
	if err := config.Save(m.cfg); err != nil {
		m.showStatus("save failed: " + err.Error())
		return false
	}
	m.reloadConfig()
	m.reloadAll()
	m.rootCursor = max(0, len(m.cfg.Roots)-1)
	m.ensureRootVisible()
	return true
}

func (m *Model) removeSelectedRoot() bool {
	if len(m.cfg.Roots) == 0 || m.rootCursor >= len(m.cfg.Roots) {
		return false
	}
	protectedRoot := config.ResolvePaths().DataDir
	if pathsMatch(m.cfg.Roots[m.rootCursor], protectedRoot) {
		m.showStatus("cannot remove default log root")
		return false
	}
	next := append([]string{}, m.cfg.Roots[:m.rootCursor]...)
	next = append(next, m.cfg.Roots[m.rootCursor+1:]...)
	m.cfg.Roots = next
	if err := config.Save(m.cfg); err != nil {
		m.showStatus("save failed: " + err.Error())
		return false
	}
	m.reloadConfig()
	m.reloadAll()
	if m.rootCursor >= len(m.cfg.Roots) {
		m.rootCursor = max(0, len(m.cfg.Roots)-1)
	}
	m.ensureRootVisible()
	return true
}

// --- Layout helpers ---

func (m Model) contentHeight() int {
	headerH := 1
	fixed := headerH + 1 + 1 + 1
	if m.statusMsg != "" && time.Now().Before(m.statusExpiry) {
		fixed++
	}
	height := m.height - fixed
	if height < 6 {
		return 6
	}
	return height
}

func (m Model) paneHeight() int {
	height := m.contentHeight() - panelStyle.GetVerticalFrameSize()
	if height < 3 {
		return 3
	}
	return height
}

func (m Model) viewerEntryListHeight() int {
	return max(5, m.paneHeight()-15)
}

func (m Model) copyHint() string {
	if m.focus == focusViewer {
		return "copy entry"
	}
	return "copy path"
}

func (m Model) fileGroupLabel(file logs.FileInfo, maxWidth int) string {
	rootLabel := "cwd"
	if !pathsMatch(file.Root, m.projectPath) {
		rootLabel = filepath.Base(file.Root)
	}
	dir := relPath(file.Root, filepath.Dir(file.Path))
	if dir == "." {
		return trimMiddle(rootLabel, maxWidth)
	}
	return trimMiddle(rootLabel+"/"+dir, maxWidth)
}

func (m Model) copyEntryPayload() string {
	if len(m.entries) == 0 || m.entryCursor >= len(m.entries) {
		return ""
	}
	entry := m.entries[m.entryCursor]
	payload := struct {
		Timestamp string         `json:"timestamp,omitempty"`
		Level     string         `json:"level,omitempty"`
		Message   string         `json:"message,omitempty"`
		Data      map[string]any `json:"data,omitempty"`
		Source    string         `json:"source,omitempty"`
		Line      int            `json:"line,omitempty"`
		Raw       string         `json:"raw,omitempty"`
	}{
		Timestamp: entry.Timestamp,
		Level:     entry.Level,
		Message:   entry.Message,
		Data:      entry.Data,
		Source:    entry.Source,
		Line:      entry.LineNumber,
		Raw:       entry.Raw,
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return entry.Raw
	}
	return string(encoded)
}

// --- Commands ---

func (m Model) nextTickCmd() tea.Cmd {
	if m.statusMsg != "" && time.Now().Before(m.statusExpiry) {
		return tickCmd()
	}
	if m.tailing && m.page == pageDashboard {
		return tickCmd()
	}
	return nil
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func batchCmds(cmds ...tea.Cmd) tea.Cmd {
	var active []tea.Cmd
	for _, cmd := range cmds {
		if cmd != nil {
			active = append(active, cmd)
		}
	}
	if len(active) == 0 {
		return nil
	}
	return tea.Batch(active...)
}

func openEditorCmd(path string) tea.Cmd {
	editor, args := resolveEditor()
	if editor == "" {
		return func() tea.Msg { return editorDoneMsg{err: fmt.Errorf("no editor found")} }
	}
	args = append(args, path)
	cmd := exec.Command(editor, args...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorDoneMsg{err: err}
	})
}

func resolveEditor() (string, []string) {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		value := strings.TrimSpace(os.Getenv(env))
		if value == "" {
			continue
		}
		parts := strings.Fields(value)
		if len(parts) == 0 {
			continue
		}
		return parts[0], parts[1:]
	}
	for _, candidate := range []string{"nvim", "vim", "nano", "vi"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", nil
}

func copyToClipboardCmd(text, label string) tea.Cmd {
	return func() tea.Msg {
		if strings.TrimSpace(text) == "" {
			return copyDoneMsg{label: label, err: fmt.Errorf("nothing to copy")}
		}
		cmd, args, err := resolveClipboard()
		if err != nil {
			return copyDoneMsg{label: label, err: err}
		}
		process := exec.Command(cmd, args...)
		process.Stdin = strings.NewReader(text)
		return copyDoneMsg{label: label, err: process.Run()}
	}
}

func resolveClipboard() (string, []string, error) {
	for _, candidate := range []struct {
		cmd  string
		args []string
	}{
		{cmd: "pbcopy"},
		{cmd: "wl-copy"},
		{cmd: "xclip", args: []string{"-selection", "clipboard"}},
		{cmd: "xsel", args: []string{"--clipboard", "--input"}},
	} {
		if path, err := exec.LookPath(candidate.cmd); err == nil {
			return path, candidate.args, nil
		}
	}
	return "", nil, fmt.Errorf("no clipboard command found")
}

// --- Pure helpers ---

func splitPaneWidths(total, leftMin, rightMin int) (int, int) {
	frame := panelStyle.GetHorizontalFrameSize()
	available := total - frame*4
	if available < leftMin+rightMin {
		available = leftMin + rightMin
	}
	left := available / 3
	if left < leftMin {
		left = leftMin
	}
	right := available - left
	if right < rightMin {
		right = rightMin
		left = available - right
		if left < leftMin {
			left = leftMin
		}
	}
	return left, max(rightMin, available-left)
}

func joinHeaderSides(left, right string, width int) string {
	if right == "" {
		return fitHeaderLine(left, width)
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		return fitHeaderLine(left, width)
	}
	return fitHeaderLine(left+strings.Repeat(" ", gap)+right, width)
}

func joinPaneSides(left, right string, width int) string {
	if right == "" {
		return fitHeaderLine(left, width)
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return fitHeaderLine(left, width)
	}
	return fitHeaderLine(left+strings.Repeat(" ", gap)+right, width)
}

func fitHeaderLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(line) <= width {
		return line + strings.Repeat(" ", width-lipgloss.Width(line))
	}
	return trim(line, width)
}

func entryWindow(total, scroll, height int) (int, int) {
	if scroll < 0 {
		scroll = 0
	}
	end := scroll + height
	if end > total {
		end = total
	}
	return scroll, end
}

func trim(s string, maxLen int) string {
	if maxLen < 4 || lipgloss.Width(s) <= maxLen {
		return s
	}
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen-3]) + "..."
}

func trimMiddle(s string, maxLen int) string {
	if maxLen < 8 || lipgloss.Width(s) <= maxLen {
		return s
	}
	r := []rune(s)
	head := (maxLen - 3) / 2
	tail := maxLen - 3 - head
	return string(r[:head]) + "..." + string(r[len(r)-tail:])
}

func relPath(root, target string) string {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return target
	}
	return rel
}

func byteLabel(size int64) string {
	switch {
	case size > 1024*1024*1024:
		return fmt.Sprintf("%.1fG", float64(size)/float64(1024*1024*1024))
	case size > 1024*1024:
		return fmt.Sprintf("%.1fM", float64(size)/float64(1024*1024))
	case size > 1024:
		return fmt.Sprintf("%.1fK", float64(size)/float64(1024))
	default:
		return fmt.Sprintf("%dB", size)
	}
}

func intStr(n int) string {
	return fmt.Sprintf("%d", n)
}

func pathsMatch(a, b string) bool {
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
