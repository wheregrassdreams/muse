package document

import "time"

// FileMetadata is the filesystem metadata used by document presentation.
type FileMetadata struct {
	CreatedAt  time.Time
	ModifiedAt time.Time
	Size       int64
}

// MetadataReader is the application port for reading document filesystem
// metadata. Platform adapters decide whether creation time is available.
type MetadataReader interface {
	ReadMetadata(path string) (FileMetadata, error)
}
