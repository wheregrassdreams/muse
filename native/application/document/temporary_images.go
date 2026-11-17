package document

// TemporaryImageStore writes images for documents that do not yet have a
// directory. The returned path is absolute so the unsaved document can refer
// to the image until it is saved into a vault.
type TemporaryImageStore interface {
	SaveTemporaryImage(data []byte, extension string) (absolutePath string, err error)
}
