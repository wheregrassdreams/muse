package links

import (
	"path/filepath"
	"strings"

	vaultapp "muse/application/vault"
	doctypes "muse/document/types"
	markdownimages "muse/markdown/images"
	vaultfiles "muse/vault/files"
	vaultlinks "muse/vault/links"
)

// LinkNavigationSession is the vault-scoped read model needed to resolve
// clicked links, wiki embeds, image targets, and preview note facts.
type LinkNavigationSession interface {
	Snapshot() vaultapp.Snapshot
	ResolveLink(current, target string) (vaultlinks.Location, bool)
	ResolveImageTarget(currentPath, target string) (string, bool)
	NoteContent(path string) (title, source string, ok bool)
	FindImage(name string) string
}

// LinkPathKindPort supplies filesystem kind for paths that are not represented
// in the vault catalog (for example links to files outside the current vault).
type LinkPathKindPort interface {
	Kind(path string) (vaultfiles.EntryKind, error)
}

type LinkValidity uint8

const (
	LinkUnknown LinkValidity = iota
	LinkValid
	LinkInvalid
)

type LinkDestinationKind uint8

const (
	LinkDestinationIgnored LinkDestinationKind = iota
	LinkDestinationPending
	LinkDestinationNote
	LinkDestinationFile
	LinkDestinationImage
	LinkDestinationHeading
	LinkDestinationExternal
)

// LinkDestination is a policy result for a user click. UI hosts decide how to
// enact it, while this use case owns target parsing, lookup, and path policy.
type LinkDestination struct {
	Kind   LinkDestinationKind
	Value  string
	Path   string
	Offset int
}

type WikiPreviewNote struct {
	Title  string
	Path   string
	Source string
	Offset int
	Root   string
}

type WikiEmbed struct {
	Path   string
	Source string
}

// LinkNavigation owns application policy for resolving document navigation
// against one active vault. It has no UI, renderer, or App dependency.
type LinkNavigation struct {
	pathKinds  LinkPathKindPort
	imageFiles markdownimages.LocalFiles
}

func NewLinkNavigation(pathKinds LinkPathKindPort, imageFiles markdownimages.LocalFiles) *LinkNavigation {
	return &LinkNavigation{pathKinds: pathKinds, imageFiles: imageFiles}
}

func (u *LinkNavigation) ResolveClick(session LinkNavigationSession, currentPath, href string) LinkDestination {
	target := vaultlinks.ParseNavigationTarget(href)
	switch target.Kind {
	case vaultlinks.NavigationEmpty:
		return LinkDestination{Kind: LinkDestinationIgnored}
	case vaultlinks.NavigationHeading:
		return LinkDestination{Kind: LinkDestinationHeading, Value: target.Value}
	case vaultlinks.NavigationExternal:
		return LinkDestination{Kind: LinkDestinationExternal, Value: target.Value}
	case vaultlinks.NavigationWiki:
		if image, ok := u.ResolveWikiImage(session, currentPath, target.Value); ok {
			return LinkDestination{Kind: LinkDestinationImage, Path: image}
		}
		if session == nil || !session.Snapshot().Ready {
			return LinkDestination{Kind: LinkDestinationPending, Value: target.Value}
		}
		location, ok := resolveNote(session, currentPath, target.Value)
		if !ok {
			return LinkDestination{Kind: LinkDestinationIgnored}
		}
		return LinkDestination{Kind: LinkDestinationNote, Path: location.Path, Offset: location.Offset}
	case vaultlinks.NavigationPath:
		if location, ok := resolveNote(session, currentPath, target.Value); ok {
			return LinkDestination{Kind: LinkDestinationNote, Path: location.Path, Offset: location.Offset}
		}
		path := target.Value
		if !filepath.IsAbs(path) {
			base := filepath.Dir(currentPath)
			if currentPath == "" && session != nil {
				base = session.Snapshot().Root
			}
			path = filepath.Join(base, filepath.FromSlash(path))
		}
		if u != nil && u.pathKinds != nil {
			if kind, err := u.pathKinds.Kind(path); err == nil && kind == vaultfiles.EntryRegular && doctypes.IsEditablePath(path) {
				return LinkDestination{Kind: LinkDestinationFile, Path: path}
			}
		}
		return LinkDestination{Kind: LinkDestinationExternal, Value: path}
	default:
		return LinkDestination{Kind: LinkDestinationIgnored}
	}
}

func (u *LinkNavigation) ResolveWikiImage(session LinkNavigationSession, currentPath, target string) (string, bool) {
	if u == nil || session == nil || !session.Snapshot().Ready {
		return "", false
	}
	path, ok := session.ResolveImageTarget(currentPath, target)
	if !ok || path == "" {
		return "", false
	}
	root := session.Snapshot().Root
	path = markdownimages.ResolveSource(path, markdownimages.SourceOptions{Root: root, LocalFiles: u.imageFiles})
	return path, path != ""
}

func (u *LinkNavigation) ResolveWikiEmbed(session LinkNavigationSession, currentPath, target string) (WikiEmbed, bool) {
	if session == nil || !session.Snapshot().Ready {
		return WikiEmbed{}, false
	}
	location, ok := resolveNote(session, currentPath, target)
	if !ok {
		return WikiEmbed{}, false
	}
	_, source, ok := session.NoteContent(location.Path)
	if !ok {
		return WikiEmbed{}, false
	}
	return WikiEmbed{Path: location.Path, Source: source}, true
}

func (u *LinkNavigation) ResolveWikiPreview(session LinkNavigationSession, currentPath, target string) (WikiPreviewNote, bool) {
	if session == nil {
		return WikiPreviewNote{}, false
	}
	snapshot := session.Snapshot()
	if !snapshot.Ready || snapshot.Root == "" {
		return WikiPreviewNote{}, false
	}
	location, ok := resolveNote(session, currentPath, target)
	if !ok {
		return WikiPreviewNote{}, false
	}
	title, source, ok := session.NoteContent(location.Path)
	if !ok {
		return WikiPreviewNote{}, false
	}
	return WikiPreviewNote{Title: title, Path: location.Path, Source: source, Offset: location.Offset, Root: snapshot.Root}, true
}

func (u *LinkNavigation) WikiLinkValidity(session LinkNavigationSession, currentPath, target string) LinkValidity {
	if session == nil || !session.Snapshot().Ready {
		return LinkUnknown
	}
	if _, ok := resolveNote(session, currentPath, target); ok {
		return LinkValid
	}
	if _, ok := u.ResolveWikiImage(session, currentPath, target); ok {
		return LinkValid
	}
	return LinkInvalid
}

// ResolveImageSource applies Markdown image path and vault containment policy
// with the active vault catalog and caller-supplied local-file capability.
func (u *LinkNavigation) ResolveImageSource(session LinkNavigationSession, source, base, root string, files markdownimages.LocalFiles) string {
	var lookup func(string) string
	if session != nil {
		lookup = session.FindImage
	}
	return markdownimages.ResolveSource(source, markdownimages.SourceOptions{
		BaseDir: base, Root: root, Lookup: lookup, LocalFiles: files,
	})
}

func resolveNote(session LinkNavigationSession, currentPath, target string) (vaultlinks.Location, bool) {
	if session == nil {
		return vaultlinks.Location{}, false
	}
	snapshot := session.Snapshot()
	if !snapshot.Ready || snapshot.Root == "" {
		return vaultlinks.Location{}, false
	}
	current := ""
	if currentPath != "" {
		relative, err := filepath.Rel(snapshot.Root, currentPath)
		if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return vaultlinks.Location{}, false
		}
		if relative != "." {
			current = filepath.ToSlash(relative)
		}
	}
	return session.ResolveLink(current, target)
}
