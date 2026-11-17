package editor

import (
	"path/filepath"
	"runtime"
)

func editorTestdataPath(parts ...string) string {
	_, sourceFile, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", "..", "testdata")
	return filepath.Join(append([]string{root}, parts...)...)
}
