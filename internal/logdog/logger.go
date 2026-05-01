package logdog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type LogLevel string

const (
	DEBUG LogLevel = "DEBUG"
	INFO  LogLevel = "INFO"
	WARN  LogLevel = "WARN"
	ERROR LogLevel = "ERROR"
)

type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     LogLevel               `json:"level"`
	Message   string                 `json:"message"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

type Logger struct {
	mu          sync.Mutex
	logLevel    LogLevel
	logDir      string
	file        *os.File
	currentDate string
}

var defaultLogger *Logger
var once sync.Once

func init() {
	once.Do(func() {
		dir := os.Getenv("LOGDOG_DIR")
		if dir == "" {
			if home, err := os.UserHomeDir(); err == nil {
				dir = filepath.Join(home, ".local", "share", "logdog", "logdog", "logs")
			} else {
				dir = filepath.Join(os.TempDir(), "logdog", "logs")
			}
		}
		defaultLogger = &Logger{
			logLevel: INFO,
			logDir:   dir,
		}
	})
}

// SetLevel sets the minimum log level. Messages below this level are dropped.
func SetLevel(level LogLevel) {
	defaultLogger.mu.Lock()
	defer defaultLogger.mu.Unlock()
	defaultLogger.logLevel = level
}

// Close flushes and closes the open log file. Call on shutdown.
func Close() {
	defaultLogger.mu.Lock()
	defer defaultLogger.mu.Unlock()
	if defaultLogger.file != nil {
		defaultLogger.file.Close()
		defaultLogger.file = nil
	}
}

func levelRank(l LogLevel) int {
	switch l {
	case DEBUG:
		return 0
	case INFO:
		return 1
	case WARN:
		return 2
	case ERROR:
		return 3
	}
	return 1
}

func (l *Logger) rotate(today string) error {
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
	if err := os.MkdirAll(l.logDir, 0755); err != nil {
		return err
	}
	projectName := filepath.Base(filepath.Dir(l.logDir))
	logPath := filepath.Join(l.logDir, fmt.Sprintf("%s-%s.jsonl", projectName, today))
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	l.file = f
	l.currentDate = today
	return nil
}

func (l *Logger) log(level LogLevel, message string, data map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if levelRank(level) < levelRank(l.logLevel) {
		return
	}

	now := time.Now()
	today := now.Format("2006-01-02")
	if l.file == nil || l.currentDate != today {
		if err := l.rotate(today); err != nil {
			return
		}
	}

	entry := LogEntry{
		Timestamp: now.Format(time.RFC3339),
		Level:     level,
		Message:   message,
		Data:      data,
	}
	jsonData, _ := json.Marshal(entry)
	fmt.Fprintln(l.file, string(jsonData))
}

func buildData(args ...interface{}) map[string]interface{} {
	if len(args) == 0 {
		return nil
	}
	data := make(map[string]interface{}, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		if key, ok := args[i].(string); ok {
			data[key] = args[i+1]
		}
	}
	return data
}

func Error(message string, args ...interface{}) { defaultLogger.log(ERROR, message, buildData(args...)) }
func Warn(message string, args ...interface{})  { defaultLogger.log(WARN, message, buildData(args...)) }
func Info(message string, args ...interface{})  { defaultLogger.log(INFO, message, buildData(args...)) }
func Debug(message string, args ...interface{}) { defaultLogger.log(DEBUG, message, buildData(args...)) }
