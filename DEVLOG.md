## DevLog

### 2026-04-15: Viewer scroll follow fix
- Fixed dashboard right-panel entry navigation so the selected log entry stays visible while moving through long result sets
- Shared the viewer entry-list height calculation between rendering and `ensureEntryVisible()` to remove the scroll math mismatch
- Added focused `internal/tui` tests covering top, window overflow, bottom, and small-terminal scroll behavior

### 2026-04-13: Audit + cleanup — tui-suite standards
- Split 1700-line `model.go` monolith back into MUV: `model.go`, `update.go`, `view.go`, `styles.go`
- Added `CLAUDE.md` with rules + architecture
- Added `--version` flag (ldflags via Makefile + release workflow)
- Fixed discovery perf: removed `estimateEntries` and `DetectFormat` from `walkRoot` — no longer reads every file on startup
- Entry count now set lazily when a file is selected (from `reloadEntries`)
- Fixed CLI `config add-root` not deduplicating (TUI version already did)
- Fixed `config.normalize()` treating `CleanupKeepDays: 0` as invalid (reset to 14) — now only resets negative values
- Removed duplicate `max()` from `store.go` (use builtin)
- Cleaned `.gitignore`: removed stale binary names, added `.gocache/`, `test.log`, `.codex/`
- Added `test` target to Makefile
- Release workflow now injects version via `-X main.version`

### 2026-04-13: Bug fixes + UX cleanup
- Fixed hardcoded `/home/lucas/...` path in `internal/logdog/logger.go` — now derives from `os.UserHomeDir()` at runtime
- Removed `logdog.Info("server started")` from `main()` — was polluting own discovery on every CLI invocation
- Fixed `removeCandidate` duplicate branches (comment clarifies intent)
- Fixed `prune --days` sentinel: flag default changed to `-1`, `PruneOlderThan` now uses `< 0` check so `--days 0` actually means prune-all
- Fixed `newCatCmd` creating `newStore()` twice
- Fixed tail tick calling full `reloadAll()` — now calls `reloadEntriesOnly()` (no file walk per tick)
- Fixed `detailScroll` resetting on every tail tick — preserved during live follow
- Added `esc` to drop from viewer focus back to file list (dashboard)
- Removed dead `"p"` no-op case from dashboard key handler
- Fixed `addRoot` reporting "added" for duplicate roots — now checks before saving
- Simplified `copyHint()` — removed unreachable `pageSetup`/`pageCleanup` branches

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
