//go:build !windows && !darwin

package util

import "time"

func setBirthTime(path string, t time.Time) error {
	// Birth time cannot be set on most Unix-like systems (Linux, BSDs, etc.).
	// This is a best-effort no-op to keep the code portable.
	return nil
}
