// Package logger provides modifications to charmbracelet/log's default logger to be used in various files/packages.
package logger

import (
	"os"

	"github.com/charmbracelet/log"
)

// New creates a new default charm log.
func New(prefix string) *log.Logger {
	return log.NewWithOptions(os.Stdout, log.Options{
		Prefix:          prefix,
		ReportCaller:    false,
		ReportTimestamp: true,
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
