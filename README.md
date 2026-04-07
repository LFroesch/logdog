# Logdog

TUI logging utility for Go projects. Detects your project, generates a structured JSON logging package, and provides a log viewer. Built with Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Install

```bash
go install github.com/LFroesch/logdog@latest
```

Or build from source:

```bash
make install
```

## Usage

```bash
cd your-go-project
logdog
```

### Quick Start

1. Run `logdog` in a Go project (must have `go.mod`)
2. Select "Install/Setup Logger"
3. Use the generated logger in your code:

```go
import "your-project/internal/logdog"

func main() {
    logdog.Info("Application started")
    logdog.Info("User logged in", "user_id", 123, "username", "john")
    logdog.Error("Database error", "table", "users", "operation", "insert")
}
```

## Generated API

```go
// Basic levels
logdog.Debug("message")
logdog.Info("message")
logdog.Warn("message")
logdog.Error("message")

// With key-value data
logdog.Info("request", "method", "GET", "path", "/api/users")

// With Go error
logdog.ErrorWithErr("operation failed", err)

// With user context
logdog.InfoWithUser("profile updated", userID)
logdog.ErrorWithUser("payment failed", userID, err, "amount", 149.99)
```

## Log Output

JSON logs at `logdog/logs/logdog-YYYY-MM-DD.json`, rotated daily:

```json
{
  "timestamp": "2026-01-15T14:30:45Z",
  "level": "INFO",
  "message": "User logged in",
  "data": {
    "user_id": 123,
    "username": "john"
  }
}
```

## TUI Features

| Screen | What it does |
|--------|-------------|
| Install/Setup | Generate logger package into your project |
| Local Logs | Browse project-specific log files |
| Global Logs | Browse logs from all projects (`~/logdog/`) |
| Settings | Configure log retention days |

### Keybindings

| Key | Action |
|-----|--------|
| `j/k`, `up/down` | Navigate |
| `enter` | Select / open |
| `v` | View log contents |
| `d` | Delete log file |
| `c` | Clean old logs (retention-based) |
| `+/-` | Adjust retention days |
| `esc` | Back |
| `q` | Quit |

## File Structure (after install)

```
your-project/
├── internal/logdog/
│   ├── logger.go          # Generated logging package
│   └── README.md
├── logdog/logs/            # Project-local logs
└── go.mod
```

Global logs: `~/logdog/<project-name>/`

## License

[AGPL-3.0](LICENSE)
