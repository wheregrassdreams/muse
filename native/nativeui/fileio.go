package nativeui

import (
	"os"
	"path/filepath"
)

// writeFileAtomic writes data to path through a temporary file beside it
// and a rename, so a crash, a full disk or a killed process never leaves a
// note half written: the old file stays whole until the new one is.
// os.WriteFile truncates first, and a note is the user's only copy.
//
// The temporary file is written in the same directory so the rename stays
// on one filesystem (a cross-device rename fails), and it keeps the mode
// the file had, as an editor should not change permissions on save.
func writeFileAtomic(path string, data []byte) error {
	perm := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		perm = st.Mode().Perm()
	}
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	// A no-op once the rename moved it; the cleanup on every early return.
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
