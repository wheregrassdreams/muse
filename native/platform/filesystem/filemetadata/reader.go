// Package filemetadata adapts filesystem timestamps for application use cases.
package filemetadata

import (
	"os"

	documentapp "muse/application/document"
)

type Reader struct{}

func New() *Reader { return &Reader{} }

func (*Reader) ReadMetadata(path string) (documentapp.FileMetadata, error) {
	info, err := os.Stat(path)
	if err != nil {
		return documentapp.FileMetadata{}, err
	}
	return documentapp.FileMetadata{
		CreatedAt:  creationTime(info),
		ModifiedAt: info.ModTime(),
		Size:       info.Size(),
	}, nil
}

var _ documentapp.MetadataReader = (*Reader)(nil)
