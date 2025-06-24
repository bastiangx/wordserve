package logger

import (
	"os"

	"github.com/charmbracelet/log"
)

// Default creates a new default charm log that respects the global log level
func Default(prefix string) *log.Logger {
	return log.NewWithOptions(os.Stdout, log.Options{
		Prefix:          prefix,
		ReportCaller:    false,
		ReportTimestamp: false,
		Formatter:       log.TextFormatter,
		Level:           log.GetLevel(),
	})
}

// NewWithConfig creates a new charm log with custom config
func NewWithConfig(prefix string, level log.Level, caller bool, showTimestamp bool, fmt log.Formatter) *log.Logger {
	return log.NewWithOptions(os.Stdout, log.Options{
		Prefix:          prefix,
		Level:           level,
		ReportCaller:    caller,
		ReportTimestamp: showTimestamp,
		Formatter:       fmt,
	})
}
