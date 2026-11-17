package app

import (
	"os"
	"runtime/debug"
)

// ConfigureRuntimeGCPolicy lowers Go's heap growth target for the desktop
// process. An explicit GOGC setting remains authoritative for diagnostics and
// users who choose a different memory/CPU tradeoff.
func ConfigureRuntimeGCPolicy() {
	if _, configured := os.LookupEnv("GOGC"); configured {
		return
	}
	debug.SetGCPercent(50)
}
