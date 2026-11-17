package app

import (
	"os"
	"runtime/debug"
	"testing"
)

func TestConfigureRuntimeGCPolicy(t *testing.T) {
	oldPercent := debug.SetGCPercent(-1)
	oldValue, hadValue := os.LookupEnv("GOGC")
	t.Cleanup(func() {
		debug.SetGCPercent(oldPercent)
		if hadValue {
			_ = os.Setenv("GOGC", oldValue)
		} else {
			_ = os.Unsetenv("GOGC")
		}
	})
	if err := os.Unsetenv("GOGC"); err != nil {
		t.Fatal(err)
	}
	ConfigureRuntimeGCPolicy()
	if got := debug.SetGCPercent(-1); got != 50 {
		t.Fatalf("default GC percent = %d, want 50", got)
	}

	debug.SetGCPercent(73)
	if err := os.Setenv("GOGC", "100"); err != nil {
		t.Fatal(err)
	}
	ConfigureRuntimeGCPolicy()
	if got := debug.SetGCPercent(-1); got != 73 {
		t.Fatalf("explicit GOGC changed GC percent to %d, want 73", got)
	}
}
