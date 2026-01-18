package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

// Logger provides structured logging
type Logger struct {
	mu     sync.Mutex
	writer io.Writer
	logger *log.Logger
}

// New creates a new logger that writes to the given writer
func New(w io.Writer) *Logger {
	return &Logger{
		writer: w,
		logger: log.New(w, "", log.LstdFlags),
	}
}

// NewFile creates a logger that writes to a file
func NewFile(path string) (*Logger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return New(f), nil
}

// Info logs an informational message
func (l *Logger) Info(format string, args ...interface{}) {
	l.log("INFO", format, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log("WARN", format, args...)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	l.log("ERROR", format, args...)
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log("DEBUG", format, args...)
}

// log formats and writes a log message
func (l *Logger) log(level, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	msg := fmt.Sprintf(format, args...)
	l.logger.Printf("[%s] %s", level, msg)
}

// Close closes the logger (if backed by a file)
func (l *Logger) Close() error {
	if f, ok := l.writer.(*os.File); ok {
		return f.Close()
	}
	return nil
}
