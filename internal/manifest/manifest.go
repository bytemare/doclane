package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Manifest is the machine-readable summary emitted for each sync run.
type Manifest struct {
	GeneratedAt      time.Time     `json:"generated_at"`
	SourceRepo       string        `json:"source_repo"`
	Selector         string        `json:"selector"`
	ResolvedSelector string        `json:"resolved_selector"`
	CommitSHA        string        `json:"commit_sha"`
	CommitDate       string        `json:"commit_date,omitempty"`
	Policy           *PolicyRecord `json:"policy,omitempty"`
	Files            []FileRecord  `json:"files"`
	Warnings         []string      `json:"warnings,omitempty"`
}

// PolicyRecord describes the policy file provenance used in a sync run.
type PolicyRecord struct {
	Path        string `json:"path"`
	UpstreamSHA string `json:"upstream_sha256"`
	Enforced    bool   `json:"enforced"`
	Present     bool   `json:"present"`
}

// FileRecord describes one synced file in the manifest.
type FileRecord struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	Target      string `json:"target"`
	Templated   bool   `json:"templated"`
	UpstreamSHA string `json:"upstream_sha256"`
	RenderedSHA string `json:"rendered_sha256"`
	Changed     bool   `json:"changed"`
}

// Write serializes and writes a manifest JSON file.
func Write(path string, m Manifest) error {
	m.GeneratedAt = time.Now().UTC()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
