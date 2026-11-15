package config

import (
	"encoding/json"
	"os"
	"path/filepath"
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

type documentPropertyFile map[string]DocumentPropertyPreferences

func documentPropertyPreferencesPath() string {
	return filepath.Join(filepath.Dir(PreferencesPath()), "document-properties.json")
}

// LoadDocumentPropertyPreferences returns an empty preference set when no
// saved state exists. JSON errors are reported to let callers surface them.
func LoadDocumentPropertyPreferences(notePath string) (DocumentPropertyPreferences, error) {
	var all documentPropertyFile
	data, err := os.ReadFile(documentPropertyPreferencesPath())
	if os.IsNotExist(err) {
		return DocumentPropertyPreferences{}, nil
	}
	if err != nil {
		return DocumentPropertyPreferences{}, err
	}
	if err := json.Unmarshal(data, &all); err != nil {
		return DocumentPropertyPreferences{}, err
	}
	return all[notePath], nil
}

// SaveDocumentPropertyPreferences persists view preferences outside Markdown.
func SaveDocumentPropertyPreferences(notePath string, preferences DocumentPropertyPreferences) error {
	filename := documentPropertyPreferencesPath()
	var all documentPropertyFile
	data, err := os.ReadFile(filename)
	if err == nil {
		if err := json.Unmarshal(data, &all); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if all == nil {
		all = documentPropertyFile{}
	}
	all[notePath] = preferences
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, encoded, 0o600)
}
