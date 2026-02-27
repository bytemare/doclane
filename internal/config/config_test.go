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

func TestLoadAppliesDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".doclane.yml")
	data := `
version: 1
source:
  repo: acme/shared
  selector: commit:0123456789abcdef0123456789abcdef01234567
sync:
  - id: x
    source: README.md
    target: docs/README.md
`
	if err := os.WriteFile(path, []byte(strings.TrimSpace(data)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, _, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Policy.Mode != "enforce" {
		t.Fatalf("expected default policy.mode enforce, got %q", cfg.Policy.Mode)
	}
	if cfg.Policy.Path != DefaultPolicyPath {
		t.Fatalf("expected default policy.path %q, got %q", DefaultPolicyPath, cfg.Policy.Path)
	}
}

func TestValidateRejectsDuplicateIDsAndTemplateVars(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Version: 1,
		Source: SourceConfig{
			Repo:     "acme/shared",
			Selector: "commit:0123456789abcdef0123456789abcdef01234567",
		},
		Policy: PolicyConfig{
			Mode: "warn",
			Path: ".doclane-policy.yml",
		},
		Sync: []SyncEntry{
			{
				ID:     "dup",
				Source: "README.md",
				Target: "docs/README.md",
			},
			{
				ID:     "dup",
				Source: "SECURITY.md",
				Target: "docs/SECURITY.md",
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected duplicate id validation error")
	}

	cfg.Sync = []SyncEntry{
		{
			ID:       "tmpl",
			Source:   "README.md",
			Target:   "docs/README.md",
			Template: &TemplateSpec{},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected template vars validation error")
	}
}

func validConfig() *Config {
	return &Config{
		Version: 1,
		Source: SourceConfig{
			Repo:        "acme/shared",
			Selector:    "commit:0123456789abcdef0123456789abcdef01234567",
			AllowLatest: false,
		},
		Policy: PolicyConfig{
			Mode: "warn",
			Path: ".doclane-policy.yml",
		},
		Sync: []SyncEntry{
			{ID: "x", Source: "README.md", Target: "docs/README.md"},
		},
	}
}

func TestValidateFailureMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mut  func(*Config)
	}{
		{
			name: "unsupported version",
			mut: func(c *Config) {
				c.Version = 2
			},
		},
		{
			name: "missing source repo",
			mut: func(c *Config) {
				c.Source.Repo = ""
			},
		},
		{
			name: "invalid source repo form",
			mut: func(c *Config) {
				c.Source.Repo = "acme-shared"
			},
		},
		{
			name: "missing source selector",
			mut: func(c *Config) {
				c.Source.Selector = ""
			},
		},
		{
			name: "invalid source selector",
			mut: func(c *Config) {
				c.Source.Selector = "foo:bar"
			},
		},
		{
			name: "latest without allow flag",
			mut: func(c *Config) {
				c.Source.Selector = "latest"
				c.Source.AllowLatest = false
			},
		},
		{
			name: "invalid policy mode",
			mut: func(c *Config) {
				c.Policy.Mode = "strict"
			},
		},
		{
			name: "empty policy path",
			mut: func(c *Config) {
				c.Policy.Path = ""
			},
		},
		{
			name: "policy path escape",
			mut: func(c *Config) {
				c.Policy.Path = "../policy.yml"
			},
		},
		{
			name: "empty sync list",
			mut: func(c *Config) {
				c.Sync = nil
			},
		},
		{
			name: "missing sync id",
			mut: func(c *Config) {
				c.Sync[0].ID = ""
			},
		},
		{
			name: "missing sync source",
			mut: func(c *Config) {
				c.Sync[0].Source = ""
			},
		},
		{
			name: "missing sync target",
			mut: func(c *Config) {
				c.Sync[0].Target = ""
			},
		},
		{
			name: "invalid sync target path",
			mut: func(c *Config) {
				c.Sync[0].Target = "../escape.md"
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := validConfig()
			tt.mut(cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatalf("expected validation error for case %q", tt.name)
			}
		})
	}
}
