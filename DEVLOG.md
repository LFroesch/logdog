## DevLog

### 2026-04-03: v1 — CLI + TUI overhaul
Full rebuild. Cobra CLI with `install`, `status`, `logs`, `cat`, `tail`, `grep`, `clear`.
TUI split into MUV (model/update/view/styles). Scout-style header/footer.
Log viewer with level filter (e/w/i/d keys), `/` search, `enter` detail overlay.
Generated logger fixed: RFC3339 timestamps, `LOGDOG_DIR` env var, `SetLevel()`.
Pipe-safe CLI output (TTY detection).

### 2026-03-23: Doc suite added
Added CLAUDE.md. Updated README. Updated WORK.md with feature ideas.

### 2026-03-20: Bug fixes — debug prints + resize handling
Removed 5 leftover fmt.Printf("DEBUG: ...") calls from go.go. Added tea.WindowSizeMsg handling + scroll support to log view.
