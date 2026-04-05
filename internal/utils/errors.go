// Package utils provides cross-cutting helpers used throughout openshitctl,
// including error wrapping and the shared Logger interface consumed by the
// executor, distribution phases, and addon implementations.
package utils

import "fmt"

func WrapError(context string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", context, err)
}

func WrapErrorf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(format+": %w", append(args, err)...)
}
