//go:build !linux && !darwin && !windows

package vaultfiles

import "errors"

func renameNoReplace(_, _ string) error {
	return errors.New("atomic no-replace rename is unsupported on this platform")
}
