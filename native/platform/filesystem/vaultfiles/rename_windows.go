//go:build windows

package vaultfiles

import "os"

func renameNoReplace(oldPath, newPath string) error {
	// Windows MoveFile semantics exposed by os.Rename fail when the target
	// exists, so this remains an atomic no-replace operation.
	return os.Rename(oldPath, newPath)
}
