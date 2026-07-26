package localconfig

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type LocalArtifact struct {
	ArtifactID string `json:"artifact_id"`
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	ByteSize   int64  `json:"byte_size"`
}

func SaveLocalArtifact(value LocalArtifact) error {
	path, err := localArtifactsPath()
	if err != nil {
		return err
	}
	values, err := readLocalArtifacts(path)
	if err != nil {
		return err
	}
	values[value.ArtifactID] = value
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, body, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func LocalArtifactByID(artifactID string) (LocalArtifact, error) {
	path, err := localArtifactsPath()
	if err != nil {
		return LocalArtifact{}, err
	}
	values, err := readLocalArtifacts(path)
	if err != nil {
		return LocalArtifact{}, err
	}
	value, exists := values[artifactID]
	if !exists {
		return LocalArtifact{}, os.ErrNotExist
	}
	return value, nil
}

func localArtifactsPath() (string, error) {
	configPath, err := Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(configPath), "artifacts.json"), nil
}

func readLocalArtifacts(path string) (map[string]LocalArtifact, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]LocalArtifact{}, nil
	}
	if err != nil {
		return nil, err
	}
	values := map[string]LocalArtifact{}
	if err := json.Unmarshal(body, &values); err != nil {
		return nil, err
	}
	return values, nil
}
