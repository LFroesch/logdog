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
	Timestamp string              `json:"timestamp"`
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
		defaultLogger = &Logger{
			logLevel: INFO,
			logDir:   "/home/lucas/logdog/logdog",
		}
	})
}

func (l *Logger) log(level LogLevel, message string, data map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry := LogEntry{
	Timestamp: time.Now().Format("2006-01-02 15:04:05"), // Human readable format
	Level:     level,
	Message:   message,
	Data:      data,
	}

	// Get today's log file with new format: projectname-logdog-MM-DD-YYYY.json
	projectName := filepath.Base(l.logDir)
	filename := fmt.Sprintf("%s-logdog-%s.json", projectName, time.Now().Format("01-02-2006"))
	filepath := filepath.Join(l.logDir, filename)

	// Ensure directory exists
	if err := os.MkdirAll(l.logDir, 0755); err != nil {
		return
	}

	// Open file for appending
	file, err := os.OpenFile(filepath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer file.Close()

	// Write JSON entry
	jsonData, _ := json.Marshal(entry)
	file.WriteString(string(jsonData) + "\n")
}

func buildData(args ...interface{}) map[string]interface{} {
	data := make(map[string]interface{})
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			if key, ok := args[i].(string); ok {
				data[key] = args[i+1]
			}
		}
	}
	return data
}

// Public API
func Error(message string, args ...interface{}) {
	defaultLogger.log(ERROR, message, buildData(args...))
}

func Warn(message string, args ...interface{}) {
	defaultLogger.log(WARN, message, buildData(args...))
}

func Info(message string, args ...interface{}) {
	defaultLogger.log(INFO, message, buildData(args...))
}

func Debug(message string, args ...interface{}) {
	defaultLogger.log(DEBUG, message, buildData(args...))
}
