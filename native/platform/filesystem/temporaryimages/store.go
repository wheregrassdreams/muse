// Package temporaryimages stores image attachments for unsaved documents.
package temporaryimages

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"muse/application/document"
)

const directoryName = "muse-images"

// Store writes temporary document images below the operating system temp dir.
type Store struct {
	dir string
}

var _ document.TemporaryImageStore = (*Store)(nil)

func New() *Store {
	return &Store{dir: filepath.Join(os.TempDir(), directoryName)}
}

// NewAt returns a store rooted at dir. It is useful for application-specific
// composition and deterministic integration setup.
func NewAt(dir string) *Store { return &Store{dir: dir} }

func (s *Store) SaveTemporaryImage(data []byte, extension string) (string, error) {
	if s == nil {
		return "", errors.New("temporary image store is nil")
	}
	if extension == "" {
		extension = ".png"
	}
	if !strings.HasPrefix(extension, ".") || filepath.Base(extension) != extension || strings.ContainsAny(extension, `/\\`) {
		return "", fmt.Errorf("invalid temporary image extension %q", extension)
	}
	dir, err := filepath.Abs(s.dir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	base := "image-" + time.Now().Format("20060102-150405")
	for suffix := 1; ; suffix++ {
		name := base + extension
		if suffix > 1 {
			name = fmt.Sprintf("%s %d%s", base, suffix, extension)
		}
		path := filepath.Join(dir, name)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			_ = os.Remove(path)
			if writeErr != nil {
				return "", writeErr
			}
			return "", closeErr
		}
		return path, nil
	}
}
