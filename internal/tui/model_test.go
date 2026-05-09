package tui

import (
	"testing"

	"github.com/LFroesch/logdog/internal/logs"
	"github.com/charmbracelet/lipgloss"
)

func TestEnsureEntryVisibleKeepsTopSelectionInView(t *testing.T) {
	m := Model{
		width:       120,
		height:      30,
		entries:     make([]logs.Entry, 20),
		entryCursor: 3,
		entryScroll: 0,
	}

	m.ensureEntryVisible()

	if m.entryScroll != 0 {
		t.Fatalf("entryScroll = %d, want 0", m.entryScroll)
	}
}

func TestEnsureEntryVisibleScrollsWhenCursorMovesPastWindow(t *testing.T) {
	m := Model{
		width:       120,
		height:      30,
		entries:     make([]logs.Entry, 20),
		entryCursor: 11,
		entryScroll: 0,
	}

	m.ensureEntryVisible()

	visible := m.viewerEntryListHeight()
	want := m.entryCursor - visible + 1
	if got := m.entryScroll; got != want {
		t.Fatalf("entryScroll = %d, want %d", got, want)
	}
}

func TestEnsureEntryVisibleKeepsBottomSelectionVisible(t *testing.T) {
	m := Model{
		width:       120,
		height:      30,
		entries:     make([]logs.Entry, 20),
		entryCursor: 19,
		entryScroll: 0,
	}

	m.ensureEntryVisible()

	want := len(m.entries) - m.viewerEntryListHeight()
	if got, want := m.entryScroll, max(0, want); got != want {
		t.Fatalf("entryScroll = %d, want %d", got, want)
	}
}

func TestEnsureEntryVisibleClampsNegativeScrollOnSmallScreens(t *testing.T) {
	m := Model{
		width:       80,
		height:      12,
		entries:     make([]logs.Entry, 3),
		entryCursor: 0,
		entryScroll: -4,
	}

	m.ensureEntryVisible()

	if m.entryScroll < 0 {
		t.Fatalf("entryScroll = %d, want non-negative", m.entryScroll)
	}
	if got := m.viewerEntryListHeight(); got < 5 {
		t.Fatalf("viewerEntryListHeight() = %d, want at least 5", got)
	}
}

func TestRenderFilesPaneClampsGroupedRowsToPaneHeight(t *testing.T) {
	projectPath := t.TempDir()
	m := Model{
		width:       120,
		height:      18,
		projectPath: projectPath,
	}
	for i := 0; i < 20; i++ {
		m.files = append(m.files, logs.FileInfo{
			Path:       projectPath + "/group" + string(rune('A'+i)) + "/app.log",
			Root:       projectPath,
			Format:     logs.FormatJSON,
			Size:       1234,
			EntryCount: 1,
		})
	}

	rendered := m.renderFilesPane(42, true)

	if got, want := lipgloss.Height(rendered), m.contentHeight(); got != want {
		t.Fatalf("files pane height = %d, want %d", got, want)
	}
}

func TestTrimPreservesAnsiSequences(t *testing.T) {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("abcdef")
	got := trim(styled, 5)

	if got == styled {
		t.Fatalf("trim should shorten styled text when width exceeds max")
	}
	if width := lipgloss.Width(got); width != 5 {
		t.Fatalf("trimmed width = %d, want 5", width)
	}
	if got[len(got)-4:] != "...\x1b[0m" && got[len(got)-3:] != "..." {
		t.Fatalf("trimmed string should preserve ellipsis, got %q", got)
	}
}
