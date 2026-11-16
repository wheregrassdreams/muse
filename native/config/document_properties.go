package config

import (
	"encoding/json"
	"path"
)

// FieldDisplayPreference persists per-note view choices for a metadata field.
type FieldDisplayPreference struct {
	Pinned bool `json:"pinned,omitempty"`
	Hidden bool `json:"hidden,omitempty"`
	Order  int  `json:"order,omitempty"`
}

// DocumentPropertyPreferences stores view preferences keyed by authored field
// name. It is independent of the note source and keyed by normalized note path.
type DocumentPropertyPreferences struct {
	Fields map[string]FieldDisplayPreference `json:"fields,omitempty"`
}

type DocumentPropertyFile map[string]DocumentPropertyPreferences

func DocumentPropertyKey(notePath string) string {
	if notePath == "" {
		return ""
	}
	return path.Clean(notePath)
}

func DecodeDocumentPropertyFile(data []byte) (DocumentPropertyFile, error) {
	var all DocumentPropertyFile
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, err
	}
	return all, nil
}

func EncodeDocumentPropertyFile(all DocumentPropertyFile) ([]byte, error) {
	return json.MarshalIndent(all, "", "  ")
}

func DocumentPreferences(all DocumentPropertyFile, notePath string) DocumentPropertyPreferences {
	return all[DocumentPropertyKey(notePath)]
}

func SetDocumentPreferences(all DocumentPropertyFile, notePath string, preferences DocumentPropertyPreferences) DocumentPropertyFile {
	if all == nil {
		all = DocumentPropertyFile{}
	}
	all[DocumentPropertyKey(notePath)] = preferences
	return all
}
