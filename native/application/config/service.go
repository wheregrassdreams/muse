package config

import userconfig "muse/config"

// Service applies application-level defaults and normalization around the
// settings persistence capability. The filesystem repository remains an
// outer adapter; presentation code only translates between these values and
// live UI state.
type Service struct {
	repository Repository
}

func NewService(repository Repository) Service {
	return Service{repository: repository}
}

func (s Service) LoadPreferences() (userconfig.Preferences, error) {
	if s.repository == nil {
		return userconfig.DefaultPreferences(), nil
	}
	preferences, err := s.repository.LoadPreferences()
	if err != nil {
		return userconfig.DefaultPreferences(), err
	}
	return userconfig.NormalizePreferences(preferences), nil
}

func (s Service) SavePreferences(preferences userconfig.Preferences) error {
	if s.repository == nil {
		return nil
	}
	return s.repository.SavePreferences(userconfig.NormalizePreferences(preferences))
}

func (s Service) LoadEditorSettings() (userconfig.EditorSettings, error) {
	if s.repository == nil {
		return userconfig.DefaultEditorSettings(), nil
	}
	settings, err := s.repository.LoadEditorSettings()
	if err != nil {
		return userconfig.DefaultEditorSettings(), err
	}
	return userconfig.NormalizeEditorSettings(settings), nil
}

func (s Service) SaveEditorSettings(settings userconfig.EditorSettings) error {
	if s.repository == nil {
		return nil
	}
	return s.repository.SaveEditorSettings(userconfig.NormalizeEditorSettings(settings))
}
