// Package iconlibrary provides a UI-independent catalog of Lucide SVG icons.
// It separates searchable metadata from SVG assets, so consumers can browse
// the catalog without loading vector data they never render.
package iconlibrary

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"sync"
)

// Icon is one immutable catalog entry with searchable metadata.
type Icon struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// Asset adds the raw vector source for a catalog icon. SVG uses currentColor
// strokes so consumers can apply their own theme and rendering toolkit.
type Asset struct {
	Icon
	SVG string
}

//go:embed catalog.json.gz
var bundledCatalog []byte

//go:embed assets/lucide-icons.zip
var bundledSVGs []byte

var (
	catalogOnce sync.Once
	catalogData []Icon
	byID        map[string]int
	zipOnce     sync.Once
	zipFiles    map[string]*zip.File
)

// Catalog returns every icon's searchable metadata in stable ID order. The
// returned slice and tags are copies; callers may safely edit them.
func Catalog() []Icon {
	loadCatalog()
	out := make([]Icon, len(catalogData))
	for i, icon := range catalogData {
		out[i] = clone(icon)
	}
	return out
}

// Search returns icons whose ID, name, or Lucide tags contain every query
// word. Results are ranked by exact ID, ID prefix, name prefix, then stable ID.
// An empty query returns the complete catalog. SVG files are not opened.
func Search(query string) []Icon {
	loadCatalog()
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return Catalog()
	}
	terms := strings.Fields(query)
	type result struct {
		icon Icon
		rank int
	}
	matches := make([]result, 0, 32)
	for _, icon := range catalogData {
		id := strings.ReplaceAll(icon.ID, "-", " ")
		var haystack strings.Builder
		haystack.WriteString(id)
		haystack.WriteByte(' ')
		haystack.WriteString(strings.ToLower(icon.Name))
		for _, tag := range icon.Tags {
			haystack.WriteByte(' ')
			haystack.WriteString(strings.ToLower(tag))
		}
		all := haystack.String()
		matched := true
		for _, term := range terms {
			if !strings.Contains(all, term) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		rank := 3
		if icon.ID == strings.ReplaceAll(query, " ", "-") {
			rank = 0
		} else if strings.HasPrefix(id, query) {
			rank = 1
		} else if strings.HasPrefix(strings.ToLower(icon.Name), query) {
			rank = 2
		}
		matches = append(matches, result{icon: icon, rank: rank})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].rank != matches[j].rank {
			return matches[i].rank < matches[j].rank
		}
		return matches[i].icon.ID < matches[j].icon.ID
	})
	out := make([]Icon, len(matches))
	for i, match := range matches {
		out[i] = clone(match.icon)
	}
	return out
}

// Lookup loads one SVG on demand by stable kebab-case ID.
func Lookup(id string) (Asset, bool) {
	loadCatalog()
	id = strings.ToLower(strings.TrimSpace(id))
	i, ok := byID[id]
	if !ok {
		return Asset{}, false
	}
	loadZip()
	file := zipFiles[id+".svg"]
	if file == nil {
		return Asset{}, false
	}
	f, err := file.Open()
	if err != nil {
		return Asset{}, false
	}
	data, err := io.ReadAll(f)
	_ = f.Close()
	if err != nil {
		return Asset{}, false
	}
	return Asset{Icon: clone(catalogData[i]), SVG: string(data)}, true
}

// All loads every SVG. Applications should call this off their UI thread and
// prefer Search plus Lookup when they only need a subset.
func All() []Asset {
	entries := Catalog()
	out := make([]Asset, 0, len(entries))
	for _, icon := range entries {
		asset, ok := Lookup(icon.ID)
		if ok {
			out = append(out, asset)
		}
	}
	return out
}

func loadCatalog() {
	catalogOnce.Do(func() {
		zr, err := gzip.NewReader(bytes.NewReader(bundledCatalog))
		if err != nil {
			panic("iconlibrary: invalid bundled catalog: " + err.Error())
		}
		data, err := io.ReadAll(zr)
		closeErr := zr.Close()
		if err != nil {
			panic("iconlibrary: read bundled catalog: " + err.Error())
		}
		if closeErr != nil {
			panic("iconlibrary: close bundled catalog: " + closeErr.Error())
		}
		if err := json.Unmarshal(data, &catalogData); err != nil {
			panic("iconlibrary: decode bundled catalog: " + err.Error())
		}
		byID = make(map[string]int, len(catalogData))
		for i, icon := range catalogData {
			byID[icon.ID] = i
		}
	})
}

func loadZip() {
	zipOnce.Do(func() {
		reader, err := zip.NewReader(bytes.NewReader(bundledSVGs), int64(len(bundledSVGs)))
		if err != nil {
			panic("iconlibrary: invalid bundled SVG archive: " + err.Error())
		}
		zipFiles = make(map[string]*zip.File, len(reader.File))
		for _, file := range reader.File {
			zipFiles[file.Name] = file
		}
	})
}

func clone(icon Icon) Icon {
	icon.Tags = append([]string(nil), icon.Tags...)
	return icon
}
