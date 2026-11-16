// Package configstore persists the user's application settings as private
// JSON files in the standard per-user configuration directory.
package configstore

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	applicationconfig "muse/application/config"
	"muse/config"
)

type Store struct {
	root string
	mu   sync.Mutex
}

var _ applicationconfig.Repository = (*Store)(nil)

// New resolves MUSE_CONFIG_DIR first, then the standard user configuration
// directory, and finally the system temporary directory when unavailable.
func New() *Store {
	root := os.Getenv("MUSE_CONFIG_DIR")
	if root == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			base = os.TempDir()
		}
		root = filepath.Join(base, "Muse")
	}
	return NewAt(root)
}

// NewAt constructs a store at an explicit directory, primarily for tests and
// callers that already resolved their platform configuration root.
func NewAt(root string) *Store {
	return &Store{root: filepath.Clean(root)}
}

func (s *Store) Root() string { return s.root }

func (s *Store) AssistantSettingsLocation() string { return filepath.Join(s.root, "ai.json") }

func (s *Store) LoadPreferences() (config.Preferences, error) {
	data, err := s.read("settings.json")
	if errors.Is(err, os.ErrNotExist) {
		return config.DefaultPreferences(), nil
	}
	if err != nil {
		return config.DefaultPreferences(), err
	}
	return config.DecodePreferences(data)
}

func (s *Store) SavePreferences(value config.Preferences) error {
	data, err := config.EncodePreferences(value)
	if err != nil {
		return err
	}
	return s.write("settings.json", data)
}

func (s *Store) LoadEditorSettings() (config.EditorSettings, error) {
	data, err := s.read("editor.json")
	if errors.Is(err, os.ErrNotExist) {
		return config.DefaultEditorSettings(), nil
	}
	if err != nil {
		return config.DefaultEditorSettings(), err
	}
	return config.DecodeEditorSettings(data)
}

func (s *Store) SaveEditorSettings(value config.EditorSettings) error {
	data, err := config.EncodeEditorSettings(value)
	if err != nil {
		return err
	}
	return s.write("editor.json", data)
}

func (s *Store) LoadAssistantSettings() (config.AssistantSettings, error) {
	data, err := s.read("ai.json")
	if errors.Is(err, os.ErrNotExist) {
		return config.DefaultAssistantSettings(), nil
	}
	if err != nil {
		return config.DefaultAssistantSettings(), err
	}
	return config.DecodeAssistantSettings(data)
}

func (s *Store) SaveAssistantSettings(value config.AssistantSettings) error {
	data, err := config.EncodeAssistantSettings(value)
	if err != nil {
		return err
	}
	return s.write("ai.json", data)
}

func (s *Store) LoadDocumentPropertyPreferences(notePath string) (config.DocumentPropertyPreferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.readDocumentPropertyFile()
	if errors.Is(err, os.ErrNotExist) {
		return config.DocumentPropertyPreferences{}, nil
	}
	if err != nil {
		return config.DocumentPropertyPreferences{}, err
	}
	return config.DocumentPreferences(all, notePath), nil
}

func (s *Store) SaveDocumentPropertyPreferences(notePath string, preferences config.DocumentPropertyPreferences) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.readDocumentPropertyFile()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	all = config.SetDocumentPreferences(all, notePath, preferences)
	data, err := config.EncodeDocumentPropertyFile(all)
	if err != nil {
		return err
	}
	return s.write("document-properties.json", data)
}

func (s *Store) readDocumentPropertyFile() (config.DocumentPropertyFile, error) {
	data, err := s.read("document-properties.json")
	if err != nil {
		return nil, err
	}
	return config.DecodeDocumentPropertyFile(data)
}

func (s *Store) read(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.root, name))
}

func (s *Store) write(name string, data []byte) error {
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(s.root, ".config-*.tmp")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.root, name))
}
