package logger

import (
	"log"
	"os"
)

type Logger struct {
	*log.Logger
	level string
}

func New(level string) *Logger {
	return &Logger{
		Logger: log.New(os.Stdout, "", log.LstdFlags),
		level:  level,
	}
}

func (l *Logger) Info(format string, args ...interface{}) {
	l.Printf("[INFO] "+format, args...)
}

func (l *Logger) Warn(format string, args ...interface{}) {
	l.Printf("[WARN] "+format, args...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.Printf("[ERROR] "+format, args...)
}

func (l *Logger) Debug(format string, args ...interface{}) {
	if l.level == "debug" {
		l.Printf("[DEBUG] "+format, args...)
	}
}

func (l *Logger) Fatal(format string, args ...interface{}) {
	l.Printf("[FATAL] "+format, args...)
	os.Exit(1)
}
