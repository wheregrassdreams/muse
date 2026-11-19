package imagepreview

import (
	"path/filepath"
	"strings"

	"muse/markdown/images"
	hosting "muse/nativeui/workbench/framework/hosting"
)

// NativePorts are the toolkit/workbench capabilities required by the
// image-preview feature. The feature owns native label policy and controller
// construction; the caller supplies only host and platform capabilities.
type NativePorts struct {
	Windows     *hosting.Manager
	ImageCache  images.CacheStore
	LocalFiles  images.LocalFiles
	DocumentDir func() string
	CopyImage   func(source string)
	OnMain      func(func())
	Invalidate  func()
	OnError     func(error)
}

func NewNativeHost(ports NativePorts) Host {
	return NewController(ControllerOptions{
		Windows: ports.Windows, Placement: hosting.Placement{Host: "main", Layer: hosting.ModalOverlay},
		ImageCache: ports.ImageCache, LocalFiles: ports.LocalFiles,
		Label: func(source string) string {
			base := ""
			if ports.DocumentDir != nil {
				base = ports.DocumentDir()
			}
			return DisplayLabel(source, base)
		},
		CopyImage: ports.CopyImage, OnMain: ports.OnMain,
		Invalidate: ports.Invalidate, OnError: ports.OnError,
	})
}

// DisplayLabel renders a source path relative to its document directory while
// leaving URLs and data URIs unchanged.
func DisplayLabel(source, base string) string {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "data:") {
		return source
	}
	if base == "" {
		return filepath.ToSlash(source)
	}
	if absolute, err := filepath.Abs(source); err == nil {
		source = absolute
	}
	if relative, err := filepath.Rel(base, source); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(relative)
	}
	return filepath.ToSlash(source)
}
