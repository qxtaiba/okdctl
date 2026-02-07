// Package utils provides common utilities for the application.
package utils

import (
	"sync"

	"github.com/qxtaiba/okd-proxmox-cli/internal/logging"
)

var (
	// defaultLogger is the package-level logger used by utility functions.
	defaultLogger logging.Logger = logging.NoopLogger()
	loggerMu      sync.RWMutex
)

// SetLogger sets the package-level logger for utility functions.
func SetLogger(l logging.Logger) {
	loggerMu.Lock()
	defer loggerMu.Unlock()
	if l == nil {
		defaultLogger = logging.NoopLogger()
		return
	}
	defaultLogger = l
}

// GetLogger returns the current package-level logger.
func GetLogger() logging.Logger {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	return defaultLogger
}
