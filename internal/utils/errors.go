// Package utils provides shared utility functions for the CLI.
package utils

import "fmt"

// WrapError wraps an error with context message.
// Returns nil if err is nil, otherwise wraps with the provided context.
func WrapError(context string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", context, err)
}

// WrapErrorf wraps an error with a formatted context message.
// Returns nil if err is nil, otherwise wraps with the formatted context.
func WrapErrorf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(fmt.Sprintf(format, args...)+": %w", err)
}
