## DevLog

### 2026-04-18: Files pane scroll accounts for group headers
- `ensureFileVisible` and `renderFilesPane` used mismatched budgets (`paneHeight-4` vs `paneHeight-2`) and neither counted the blank-separator + group-label rows between groups — with grouped lists the cursor could run off the bottom of the visible area
- Added `countFilesInWindow` that simulates the render, charging 1 row per file plus 1–2 rows of group overhead on group changes; both scroll math and render use it and the same `fileRowBudget`
- Exported `logs.FileSortGroup` so the TUI compares group identity by the stable sort key (not the width-trimmed label, which could collide)
- Touched `internal/tui/model.go`, `internal/tui/view.go`, `internal/logs/store.go`

### 2026-04-18: Stricter log discovery — name-signal required
- `isLogCandidate` now requires the file *name* to signal a log; random `.tar.gz`, `.N`-rotated files, and `a.out`-style binaries no longer leak in
- Accepted shapes: `*.log`, `*.log.gz`, `*.log.N`, `*.log.*` (date-stamped), `*log*` word-token with allowed extension, or allowed extension inside a `/log/` or `/logs/` path component
- Removed `contentLooksLikeLog` / `lineLooksLikeLog` content sniffing — noisy and rarely right; if a file's name doesn't say "log", logdog skips it
- `walkRoot` and `findEmptyLogDirs` now skip hidden entries (name starts with `.`) unless the entry is the walk root itself — stops `~/.cache`, `~/.local`, `~/.npm` from drowning discovery when run from `$HOME`
- Added `hasBinaryContent` (null-byte + non-printable ratio on first 8KB) as a final guard; `.gz` files are trusted by name since inflating every candidate during walk is too expensive
- Touched `internal/logs/store.go`

### 2026-04-18: Named loggers in the generated template
- Generated logger now exposes `logdog.New(name, opts...)` returning a `*Logger` that writes to `<name>-YYYY-MM-DD.jsonl` in the project's log directory and stamps each entry with `"logger":"<name>"`
- Default package-level `Info/Warn/Error/Debug` now write to `default-YYYY-MM-DD.jsonl` (was `<projectname>-YYYY-MM-DD.jsonl`) — files for one project still group together since they share a folder
- Added `WithDir` and `WithLevel` options for named loggers
- `LogEntry` now includes a `logger` field; the JSON parser in `internal/logs/store.go` already passes unknown keys through to `Data`, so the field shows up in entry detail without changes
- Added `internal/detector/template_render_test.go` — runs `go vet` on the rendered template so future template edits can't ship broken Go
- Touched `internal/detector/go.go`, `internal/detector/template_render_test.go`, `README.md`, `CLAUDE.md`

### 2026-04-18: Confirm prompts for install + destructive cleanup
- Added `modeConfirm` to the TUI — header shows the prompt, footer shows `y confirm · n/esc cancel`
- Setup: `i` now requires a y/n confirmation and copy makes it clear installing the generated logger is optional
- Setup: when the logger is already installed, the install hint is replaced with an "installed" note and the `i` footer key is hidden; pressing `i` in that state just shows a status message
- Cleanup: `x` (delete current), `D` (delete selected), and `X` (prune by age) all route through the confirm prompt before anything is deleted
- Touched `internal/tui/model.go`, `internal/tui/update.go`, `internal/tui/view.go`

### 2026-04-15: Two-pane height normalization
- Fixed the side-by-side TUI pages so left/right panels render at the same final height even when one side wraps more content
- Added a dashboard render regression test to lock the panel-height behavior in place
- Touched `internal/tui/view.go` and `internal/tui/model_test.go`

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
