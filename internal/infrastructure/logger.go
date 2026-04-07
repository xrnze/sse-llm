package infrastructure

import (
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"time"

	"sse-streaming-chat/internal/domain"
)

// simpleLogger implements a basic logger for the domain.Logger interface
// In production, you might want to use a more sophisticated logging library
type simpleLogger struct {
	prefix string
}

// NewSimpleLogger creates a new simple logger instance
func NewSimpleLogger() domain.Logger {
	return &simpleLogger{
		prefix: "",
	}
}

// Debug logs debug-level messages
func (l *simpleLogger) Debug(msg string, fields ...interface{}) {
	l.logWithLevel("DEBUG", msg, fields...)
}

// Info logs info-level messages
func (l *simpleLogger) Info(msg string, fields ...interface{}) {
	l.logWithLevel("INFO", msg, fields...)
}

// Warn logs warning-level messages
func (l *simpleLogger) Warn(msg string, fields ...interface{}) {
	l.logWithLevel("WARN", msg, fields...)
}

// Error logs error-level messages
func (l *simpleLogger) Error(msg string, err error, fields ...interface{}) {
	if err != nil {
		allFields := append(fields, "error", err.Error())
		l.logWithLevel("ERROR", msg, allFields...)
	} else {
		l.logWithLevel("ERROR", msg, fields...)
	}
}

// With returns a logger with additional context fields
func (l *simpleLogger) With(fields ...interface{}) domain.Logger {
	prefix := l.formatFields(fields...)
	return &simpleLogger{
		prefix: l.prefix + prefix,
	}
}

// logWithLevel logs a message with the specified level
func (l *simpleLogger) logWithLevel(level, msg string, fields ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fieldStr := l.formatFields(fields...)

	_, file, line, ok := runtime.Caller(2)
	callerInfo := "unknown:0"
	if ok {
		callerInfo = fmt.Sprintf("%s:%d", filepath.Base(file), line)
	}

	logMsg := fmt.Sprintf("[%s] [%s] %s: %s%s%s",
		timestamp,
		callerInfo,
		level,
		l.prefix,
		msg,
		fieldStr,
	)

	// Writing directly rather than log.Println to avoid duplicating date/time if log flags are set
	log.Writer().Write([]byte(logMsg + "\n"))
}

// formatFields formats key-value pairs into a readable string
func (l *simpleLogger) formatFields(fields ...interface{}) string {
	if len(fields) == 0 {
		return ""
	}

	var result string
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			if result != "" {
				result += " "
			}
			result += fmt.Sprintf("%v=%v", fields[i], fields[i+1])
		}
	}

	if result != "" {
		return " " + result
	}
	return ""
}
