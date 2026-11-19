package document

import (
	"strings"

	"muse/document/frontmatter"
)

// TextStatistics is a backend-neutral projection of text facts used by the
// status bar and document inspector.
type TextStatistics struct {
	Characters int
	Words      int
	Lines      int
}

// AnalyzeText computes the application's text-counting convention: Unicode
// runes excluding whitespace for Characters, Fields for Words, and newline
// separators plus one for Lines.
func AnalyzeText(source string) TextStatistics {
	statistics := TextStatistics{Lines: strings.Count(source, "\n") + 1, Words: len(strings.Fields(source))}
	for _, r := range source {
		if r == '\n' || r == '\r' || r == '\t' || r == ' ' {
			continue
		}
		statistics.Characters++
	}
	return statistics
}

// InspectorSnapshot collects the data facts for the document properties UI.
// The caller supplies bodySource because frontmatter parsing and range policy
// belong to the document format adapter.
type InspectorSnapshot struct {
	Text        TextStatistics
	BodyWords   int
	Metadata    FileMetadata
	HasMetadata bool
}

// DocumentInspectorSnapshot combines the parsed frontmatter projection and
// detached file facts needed by the properties feature. It is a read-only
// application snapshot; callers supply the source and metadata port.
type DocumentInspectorSnapshot struct {
	Frontmatter frontmatter.Document
	Facts       InspectorSnapshot
	YAMLSource  string
}

// InspectDocumentProperties centralizes source parsing and metadata reads for
// document inspection so UI composition code only maps a detached snapshot.
func InspectDocumentProperties(source, path string, reader MetadataReader) DocumentInspectorSnapshot {
	parsed := frontmatter.Parse(source)
	body := source
	if parsed.Err == nil && parsed.End > 0 && parsed.End <= len(body) {
		body = body[parsed.End:]
	}
	return DocumentInspectorSnapshot{
		Frontmatter: parsed,
		Facts:       Inspect(source, body, path, reader),
		YAMLSource:  frontmatter.YAMLSource(source),
	}
}

func Inspect(source, bodySource, path string, reader MetadataReader) InspectorSnapshot {
	snapshot := InspectorSnapshot{
		Text:      AnalyzeText(source),
		BodyWords: len(strings.Fields(bodySource)),
	}
	if reader != nil {
		if metadata, err := reader.ReadMetadata(path); err == nil {
			snapshot.Metadata = metadata
			snapshot.HasMetadata = true
		}
	}
	return snapshot
}
