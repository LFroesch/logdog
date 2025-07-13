# 🐕 Logdog

A TUI-based logging utility that makes structured logging simple and consistent across Go projects.

## What is Logdog?

Logdog is a terminal-based app that:
- **Detects** Go projects automatically
- **Generates** a logging package for your project
- **Creates** structured JSON logs with daily rotation
- **Provides** a user-friendly API for logging

## Installation

1. Clone and build logdog:
```bash
git clone <your-repo>/logdog
cd logdog
go build -o logdog cmd/main.go
mv logdog ~/bin/logdog  # or add to your PATH
```

2. Use in any Go project:
```bash
cd /path/to/your/go/project
logdog
```

## Quick Start

1. **Navigate to your Go project** (must have `go.mod`)
2. **Run `logdog`** to open the TUI
3. **Press Enter** on "Install/Setup Logger"
4. **Start logging** in your code:

```go
import "your-project/internal/logdog"

func main() {
    // Simple message
    logdog.Info("Application started")
    
    // With additional data
    logdog.Info("User logged in", "user_id", 123, "username", "john")
    
    // Error with context
    logdog.Error("Database error", "table", "users", "operation", "insert")
}
```

## API Reference

### Basic Logging
```go
logdog.Debug("Debug message")
logdog.Info("Info message") 
logdog.Warn("Warning message")
logdog.Error("Error message")
```

### With Additional Data
```go
// Pass key-value pairs as arguments
logdog.Info("User action", 
    "user_id", 123,
    "action", "login",
    "ip", "192.168.1.1")
```

### Convenience Functions
```go
// Error with Go error
logdog.ErrorWithErr("Operation failed", err)
logdog.ErrorWithErr("DB error", err, "table", "users")

// User-specific logging
logdog.InfoWithUser("Profile updated", userID)
logdog.InfoWithUser("Purchase made", userID, "amount", 99.99)

// Combined user + error
logdog.ErrorWithUser("Payment failed", userID, err, "amount", 149.99)
```

## Log Output

Logs are written as JSON to `logdog/logs/logdog-YYYY-MM-DD.json`:

```json
{
  "timestamp": "2024-01-15T14:30:45Z",
  "level": "INFO",
  "message": "User logged in",
  "data": {
    "user_id": 123,
    "username": "john",
    "ip": "192.168.1.1"
  }
}
```

## File Structure

After installation, your project will have:
```
your-project/
├── internal/logdog/
│   ├── logger.go          # Generated logging package
│   └── README.md          # This documentation
├── logdog/
│   └── logs/
│       └── logdog-2024-01-15.json
└── go.mod
```

## TUI Features

- **🔍 Auto-detection** of Go projects
- **📦 One-click installation** of logging package  
- **📋 Log file browser** to view existing logs
- **⚙️ Settings** for future configuration options

## Best Practices

1. **Use descriptive messages**: `"User authentication failed"` not `"Error"`
2. **Include relevant context**: Always add user IDs, request IDs, etc.
3. **Use appropriate log levels**: 
   - `Debug`: Development/troubleshooting info
   - `Info`: Normal application events
   - `Warn`: Unusual but handled situations
   - `Error`: Actual problems that need attention
4. **Be consistent**: Use the same field names across your app (`user_id`, not `userId` sometimes and `user_id` other times)

## Examples

### Web Server Logging
```go
// Request logging
logdog.Info("HTTP request", 
    "method", r.Method,
    "path", r.URL.Path,
    "user_id", userID,
    "ip", r.RemoteAddr)

// Error handling
if err != nil {
    logdog.ErrorWithUser("Database query failed", userID, err,
        "query", "SELECT * FROM users",
        "table", "users")
    return
}

// Business events
logdog.InfoWithUser("Order created", userID,
    "order_id", order.ID,
    "total", order.Total,
    "items_count", len(order.Items))
```

### Background Jobs
```go
logdog.Info("Job started", "job_type", "email_sender", "batch_size", 100)

for _, email := range emails {
    if err := sendEmail(email); err != nil {
        logdog.Error("Email send failed", 
            "recipient", email.To,
            "template", email.Template,
            "error", err.Error())
        continue
    }
    logdog.Debug("Email sent", "recipient", email.To)
}

logdog.Info("Job completed", "job_type", "email_sender", "sent", sentCount, "failed", failedCount)
```

## Contributing

Logdog is designed to be simple and focused. Current roadmap:
- [ ] Python project support
- [ ] Node.js project support  
- [ ] Log filtering/search in TUI
- [ ] Configuration options
- [ ] Log rotation settings

## License

[Your License Here]