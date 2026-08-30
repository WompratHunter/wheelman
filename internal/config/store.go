// Package config persists Wheelman's configured Apps across runs and
// orchestrates App discovery through the ClusterClient seam.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/WompratHunter/wheelman/internal/domain"
)

// FileStore persists AppConfig entries to a JSON file on disk.
type FileStore struct {
	path string
}

// NewFileStore returns a FileStore backed by the file at path. The file
// need not exist yet; Load treats a missing file as no configured Apps.
func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

// Load returns the persisted AppConfig entries, or an empty slice if the
// store has never been saved to.
func (s *FileStore) Load() ([]domain.AppConfig, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var apps []domain.AppConfig
	if err := json.Unmarshal(data, &apps); err != nil {
		return nil, err
	}
	return apps, nil
}

// Save persists apps, replacing whatever was previously stored.
func (s *FileStore) Save(apps []domain.AppConfig) error {
	data, err := json.MarshalIndent(apps, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}
