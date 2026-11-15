//go:build !darwin

package filemetadata

import (
	"os"
	"time"
)

func creationTime(os.FileInfo) time.Time { return time.Time{} }
