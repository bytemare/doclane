package syncer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bytemare/doclane/internal/config"
	"github.com/bytemare/doclane/internal/githubapi"
	"github.com/bytemare/doclane/internal/selector"
)

func writeSyncConfig(t *testing.T, workdir, data string) {
	t.Helper()

	path := filepath.Join(workdir, ".github", ".doclane.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir .github: %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(data)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestOptionsValidate(t *testing.T) {
	t.Parallel()

	valid := Options{
		ConfigPath:   ".github/.doclane.yml",
		LockfilePath: ".github/.doclane.lock.yml",
		ManifestPath: "doclane-sync-manifest.json",
		GitHubToken:  "token",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid options, got %v", err)
	}

	tests := []struct {
		name string
		opts Options
	}{
		{name: "missing config", opts: Options{LockfilePath: "a", ManifestPath: "b", GitHubToken: "t"}},
		{name: "missing lockfile", opts: Options{ConfigPath: "a", ManifestPath: "b", GitHubToken: "t"}},
		{name: "missing manifest", opts: Options{ConfigPath: "a", LockfilePath: "b", GitHubToken: "t"}},
		{name: "missing token", opts: Options{ConfigPath: "a", LockfilePath: "b", ManifestPath: "c"}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.opts.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateConfigOnly(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	configPath := filepath.Join(workdir, ".github", ".doclane.yml")
	writeSyncConfig(t, workdir, `
version: 1
source:
  repo: acme/shared
  selector: commit:0123456789abcdef0123456789abcdef01234567
sync:
  - id: readme
    source: README.md
    target: docs/README.md
`)
	if err := ValidateConfigOnly(configPath); err != nil {
		t.Fatalf("ValidateConfigOnly returned error: %v", err)
	}

	if err := os.WriteFile(configPath, []byte("version: ["), 0o644); err != nil {
		t.Fatalf("overwrite config: %v", err)
	}
	if err := ValidateConfigOnly(configPath); err == nil {
		t.Fatal("expected invalid config parse error")
	}
}

func TestRunAllowlistReject(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	writeSyncConfig(t, workdir, `
version: 1
source:
  repo: acme/shared
  selector: commit:0123456789abcdef0123456789abcdef01234567
policy:
  mode: warn
sync:
  - id: readme
    source: README.md
    target: docs/README.md
`)
	_, err := Run(context.Background(), Options{
		WorkDir:             workdir,
		ConfigPath:          ".github/.doclane.yml",
		LockfilePath:        ".github/.doclane.lock.yml",
		ManifestPath:        "doclane-sync-manifest.json",
		GitHubToken:         "token",
		SourceRepoAllowlist: []string{"other/*"},
	})
	if err == nil || !strings.Contains(err.Error(), "allowlist") {
		t.Fatalf("expected allowlist rejection, got %v", err)
	}
}

func TestRunWriteThenNoop(t *testing.T) {
	workdir := t.TempDir()
	writeSyncConfig(t, workdir, `
version: 1
source:
  repo: acme/shared
  selector: commit:0123456789abcdef0123456789abcdef01234567
policy:
  mode: warn
sync:
  - id: readme
    source: README.md
    target: docs/README.md
`)

	oldFactory := newSourceClient
	newSourceClient = func(string) sourceClient {
		return fakeSourceClient{
			resolved: githubapi.ResolvedRef{
				Selector:         "commit:0123456789abcdef0123456789abcdef01234567",
				ResolvedSelector: "commit:0123456789abcdef0123456789abcdef01234567",
				CommitSHA:        "0123456789abcdef0123456789abcdef01234567",
				CommitDate:       time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC),
			},
			files: map[string][]byte{
				"README.md": []byte("hello"),
			},
		}
	}
	defer func() { newSourceClient = oldFactory }()

	first, err := Run(context.Background(), Options{
		WorkDir:      workdir,
		ConfigPath:   ".github/.doclane.yml",
		LockfilePath: ".github/.doclane.lock.yml",
		ManifestPath: "doclane-sync-manifest.json",
		GitHubToken:  "token",
		CreatePR:     false,
	})
	if err != nil {
		t.Fatalf("first Run returned error: %v", err)
	}
	if first.Noop {
		t.Fatalf("expected first run to write files, got %+v", first)
	}
	for _, rel := range []string{
		filepath.Join("docs", "README.md"),
		filepath.Join(".github", ".doclane.lock.yml"),
		"doclane-sync-manifest.json",
	} {
		if _, err := os.Stat(filepath.Join(workdir, rel)); err != nil {
			t.Fatalf("expected %s to be written: %v", rel, err)
		}
	}

	second, err := Run(context.Background(), Options{
		WorkDir:      workdir,
		ConfigPath:   ".github/.doclane.yml",
		LockfilePath: ".github/.doclane.lock.yml",
		ManifestPath: "doclane-sync-manifest.json",
		GitHubToken:  "token",
		CreatePR:     false,
	})
	if err != nil {
		t.Fatalf("second Run returned error: %v", err)
	}
	if !second.Noop {
		t.Fatalf("expected second run no-op, got %+v", second)
	}
}

func TestRunEnforcesSignedTags(t *testing.T) {
	workdir := t.TempDir()
	writeSyncConfig(t, workdir, `
version: 1
source:
  repo: acme/shared
  selector: tag:v1.2.3
  enforce_signed_tags: true
policy:
  mode: warn
sync:
  - id: readme
    source: README.md
    target: docs/README.md
`)

	oldFactory := newSourceClient
	newSourceClient = func(string) sourceClient {
		return fakeSourceClient{
			resolved: githubapi.ResolvedRef{
				Selector:         "tag:v1.2.3",
				ResolvedSelector: "tag:v1.2.3",
				CommitSHA:        "0123456789abcdef0123456789abcdef01234567",
				CommitDate:       time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC),
			},
			files: map[string][]byte{
				"README.md": []byte("hello"),
			},
		}
	}
	defer func() { newSourceClient = oldFactory }()

	_, err := Run(context.Background(), Options{
		WorkDir:      workdir,
		ConfigPath:   ".github/.doclane.yml",
		LockfilePath: ".github/.doclane.lock.yml",
		ManifestPath: "doclane-sync-manifest.json",
		GitHubToken:  "token",
		CreatePR:     false,
		DryRun:       true,
	})
	if err == nil || !strings.Contains(err.Error(), "tag verification metadata") {
		t.Fatalf("expected signed tag verification error, got %v", err)
	}
}

func TestHelpers(t *testing.T) {
	t.Parallel()

	if anyFileChanged(nil) {
		t.Fatal("nil files should not be reported as changed")
	}
	if !anyFileChanged([]FileResult{{Changed: false}, {Changed: true}}) {
		t.Fatal("expected changed file detection")
	}
	if !yamlBytesEqual([]byte("x: 1\n"), []byte("x: 1\n\n")) {
		t.Fatal("expected yaml bytes to be equal ignoring surrounding whitespace")
	}
	if yamlBytesEqual([]byte("x: 1"), []byte("x: 2")) {
		t.Fatal("expected different yaml to compare as different")
	}
}

func TestToResolvedResultAndEnforceTagVerification(t *testing.T) {
	t.Parallel()

	resolved := githubapi.ResolvedRef{
		Selector:         "tag:v1.0.0",
		ResolvedSelector: "tag:v1.0.0",
		CommitSHA:        "0123456789abcdef0123456789abcdef01234567",
		TagVerification: &githubapi.TagVerification{
			Verified: true,
			Reason:   "valid",
			Source:   "github-api",
		},
	}
	out := toResolvedResult("acme/shared", resolved)
	if !out.TagVerified || out.TagVerificationBy != "github-api" {
		t.Fatalf("unexpected resolved result: %+v", out)
	}

	tagSel, err := selector.Parse("tag:v1.0.0")
	if err != nil {
		t.Fatalf("parse tag selector: %v", err)
	}
	branchSel, err := selector.Parse("branch:main")
	if err != nil {
		t.Fatalf("parse branch selector: %v", err)
	}
	cfg := &config.Config{
		Source: config.SourceConfig{EnforceSignedTags: true},
	}
	if err := enforceTagVerification(cfg, nil, branchSel, githubapi.ResolvedRef{}); err != nil {
		t.Fatalf("branch selector should skip tag verification: %v", err)
	}
	if err := enforceTagVerification(cfg, nil, tagSel, githubapi.ResolvedRef{}); err == nil {
		t.Fatal("expected missing tag verification metadata error")
	}
	if err := enforceTagVerification(cfg, nil, tagSel, githubapi.ResolvedRef{
		TagVerification: &githubapi.TagVerification{
			Verified: false,
			Reason:   "invalid",
		},
	}); err == nil {
		t.Fatal("expected unverified tag error")
	}
	if err := enforceTagVerification(cfg, nil, tagSel, githubapi.ResolvedRef{
		TagVerification: &githubapi.TagVerification{
			Verified: true,
			Reason:   "valid",
		},
	}); err != nil {
		t.Fatalf("expected verified tag to pass, got %v", err)
	}
}
