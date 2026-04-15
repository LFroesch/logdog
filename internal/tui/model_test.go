package tui

import (
	"testing"

	"github.com/LFroesch/logdog/internal/logs"
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
