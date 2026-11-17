package document

import "strings"

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
