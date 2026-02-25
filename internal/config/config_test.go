package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".doclane.yml")
	data := `
version: 1
source:
  repo: acme/shared
  selector: commit:0123456789abcdef0123456789abcdef01234567
  allow_latesst: true
sync:
  - id: x
    source: README.md
    target: docs/README.md
`
	if err := os.WriteFile(path, []byte(strings.TrimSpace(data)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, _, err := Load(path); err == nil {
		t.Fatal("expected unknown field parse error")
	}
}

func TestValidateRejectsNormalizedDuplicateTargets(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Version: 1,
		Source: SourceConfig{
			Repo:     "acme/shared",
			Selector: "commit:0123456789abcdef0123456789abcdef01234567",
		},
		Policy: PolicyConfig{Mode: "warn", Path: ".doclane-policy.yml"},
		Sync: []SyncEntry{
			{ID: "one", Source: "a.md", Target: "docs/../SECURITY.md"},
			{ID: "two", Source: "b.md", Target: "SECURITY.md"},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected duplicate normalized target error")
	}
}

func TestNormalizeRepoRelativePath(t *testing.T) {
	t.Parallel()

	if got, err := NormalizeRepoRelativePath("./docs/../README.md"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else if got != "README.md" {
		t.Fatalf("unexpected normalized path: %q", got)
	}

	if _, err := NormalizeRepoRelativePath("../escape"); err == nil {
		t.Fatal("expected path escape error")
	}
}
