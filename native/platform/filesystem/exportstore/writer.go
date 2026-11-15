package exportstore

import (
	"context"
	"errors"
	"os"

	documentapp "muse/application/document"
)

type Writer struct{}

func New() *Writer { return &Writer{} }

func (Writer) WriteExport(ctx context.Context, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if path == "" {
		return errors.New("exportstore: path is required")
	}
	return os.WriteFile(path, data, 0o644)
}

var _ documentapp.ExportWriter = Writer{}
