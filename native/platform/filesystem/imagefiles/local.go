// Package imagefiles implements the markdown image module's local file port.
package imagefiles

import (
	"io/fs"
	"os"
	"path/filepath"

	"muse/markdown/images"
)

// Local uses the host filesystem for local image sources.
type Local struct{}

func New() *Local { return &Local{} }

func (*Local) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }

func (*Local) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (*Local) EvalSymlinks(path string) (string, error) { return filepath.EvalSymlinks(path) }

var _ images.LocalFiles = (*Local)(nil)
