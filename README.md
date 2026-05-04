# Logdog

Logging control plane for `tui-suite`. `logdog` auto-discovers logs from the directory you launch it in, lets you add more roots in config, gives you a dashboard/setup/cleanup TUI, and keeps a real CLI for `files`, `cat`, `grep`, `tail`, and `prune`.

Built with Go, Cobra, and [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Quick Install

Supported platforms: Linux and macOS. On Windows, use WSL.

Recommended (installs to `~/.local/bin`):

```bash
curl -fsSL https://raw.githubusercontent.com/LFroesch/logdog/main/install.sh | bash
```

Or download a binary from [GitHub Releases](https://github.com/LFroesch/logdog/releases).

Or install with Go:

```bash
go install github.com/LFroesch/logdog@latest
```

Or build from source:

```bash
make install
```

Verify:

```bash
logdog status
```

If `logdog` is not on your `PATH`, add the install dir printed by `install.sh` to your shell config.

## What It Does

- **Discover logs automatically** from the current directory, recursively
- **Scan additional roots** from `~/.config/logdog/logdog-config.json`
- **View structured and plain logs** in a 2-panel dashboard with entry navigation that keeps the selected result visible
- **Bootstrap logging** into Go projects with `logdog install`
- **Clean up old logs** including rotated/compressed variants and empty log dirs
- **Work from the CLI** for scripting and pipelines

## TUI

| Page | Purpose |
|------|---------|
| Dashboard | Left pane shows discovered logs with source root labels, right pane shows metadata, parsed entries, and entry detail |
| Setup | Install the Go logger, inspect config, add/remove discovery roots, and copy usage snippets |
| Cleanup | Review log cleanup candidates and prune/delete safely |

### Keybindings

| Key | Action |
|-----|--------|
| `1/2/3` | Dashboard / Setup / Cleanup |
| `tab`, `shift+tab` | Switch focused panel |
| `j/k`, `up/down` | Move selection |
| `g/G` | Jump to top / bottom |
| `ctrl+u`, `ctrl+d` | Page through long lists |
| `/` | Search within the selected log |
| `l` | Cycle level filter |
| `t` | Toggle live follow |
| `enter` | Focus viewer or open the selected file/root |
| `o` | Open selected file or config in editor |
| `a`, `A` | Add project root or parent root from Setup |
| `d` | Remove selected configured root from Setup |
| `r` | Reload discovery/config |
| `?` | Help |
| `q` | Quit or return to Dashboard |

## Config

`~/.config/logdog/logdog-config.json` is created automatically.

Current working directory scanning is always enabled. Config adds more roots and tuning knobs:

```json
{
  "roots": [
    "~/.local/share/logdog"
  ],
  "ignore_patterns": [".git", "node_modules", "vendor", "dist", "build", ".next", "coverage"],
  "extensions": [".log", ".txt", ".out", ".json", ".jsonl", ".ndjson"],
  "max_file_bytes": 52428800,
  "max_preview_lines": 5000,
  "cleanup_keep_days": 14,
  "default_level_filter": ""
}
```

## CLI

The TUI is the default when run in a terminal:

```bash
logdog
```

Core commands:

```bash
logdog status
logdog files
logdog cat
logdog cat /var/log/system.log
logdog grep error
logdog tail /tmp/app.jsonl
logdog prune --days 7
logdog config
logdog config add-root ~/src
logdog config remove-root ~/.local/share/logdog
tail -f app.jsonl | logdog --level error --search timeout
```

Examples:

```bash
# inspect the newest discovered log
logdog cat

# search every discovered file for a term
logdog grep websocket

# follow the latest file and only show errors
logdog tail --level error

# print the current config JSON and paths
logdog config
```

## Go Logger Install

Inside a Go project:

```bash
logdog install
```

This generates:

```text
your-project/
├── internal/logdog/
│   ├── logger.go
│   └── README.md
```

Generated logs are written as **JSON Lines** to:

```text
~/.local/share/logdog/<project>/logs/<name>-YYYY-MM-DD.jsonl
```

The default logger writes to `default-YYYY-MM-DD.jsonl`. Each entry is stamped with `"logger":"<name>"` so you can also filter by field.

Usage:

```go
import "your-project/internal/logdog"

// Default logger — writes to default-YYYY-MM-DD.jsonl
func main() {
    logdog.Info("server started", "port", 8080)
    logdog.Warn("slow query", "sql", "select * from users")
    logdog.Error("request failed", "path", "/api/users", "status", 500)
}

// Named loggers — split by concern, written to <name>-YYYY-MM-DD.jsonl
var (
    authLog = logdog.New("auth")
    dbLog   = logdog.New("db")
)

authLog.Info("login failed", "user_id", 123)
dbLog.Error("query timeout", "table", "users")
```

Options for named loggers: `logdog.WithDir("/some/path")` redirects a logger to a different folder, `logdog.WithLevel(logdog.DEBUG)` sets its minimum level.

## Workflow

Typical loop:

```bash
logdog
# 1. browse recent files on Dashboard
# 2. press / to filter the current file
# 3. press 2 for Setup if you need more roots or logger install
# 4. use logdog grep / tail / cat for shell workflows
```

## License

[AGPL-3.0](LICENSE)
