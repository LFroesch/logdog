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
	mu       sync.Mutex
	logLevel LogLevel
	logDir   string
}

var defaultLogger *Logger
var once sync.Once

func init() {
	once.Do(func() {
		dir := os.Getenv("LOGDOG_DIR")
		if dir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				home = "."
			}
			dir = filepath.Join(home, ".local", "share", "logdog")
		}
		defaultLogger = &Logger{
			logLevel: INFO,
			logDir:   dir,
		}
	})
}

func SetLevel(level LogLevel) {
	defaultLogger.mu.Lock()
	defer defaultLogger.mu.Unlock()
	defaultLogger.logLevel = level
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

func (l *Logger) log(level LogLevel, message string, data map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if levelRank(level) < levelRank(l.logLevel) {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Level:     level,
		Message:   message,
		Data:      data,
	}

	projectName := filepath.Base(l.logDir)
	filename := fmt.Sprintf("%s-logdog-%s.json", projectName, time.Now().Format("2006-01-02"))
	logPath := filepath.Join(l.logDir, filename)

	if err := os.MkdirAll(l.logDir, 0755); err != nil {
		return
	}

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer file.Close()

	jsonData, _ := json.Marshal(entry)
	file.WriteString(string(jsonData) + "\n")
}

func buildData(args ...interface{}) map[string]interface{} {
	data := make(map[string]interface{})
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
