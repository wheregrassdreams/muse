//go:build !darwin

package nativeui

import (
	"os"
	"time"
)

// Do not substitute modification time for an unavailable creation timestamp.
func fileCreationTime(os.FileInfo) time.Time { return time.Time{} }
