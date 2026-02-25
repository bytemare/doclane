package syncer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bytemare/doclane/internal/githubapi"
	"github.com/bytemare/doclane/internal/selector"
)

type fakeSourceClient struct {
	resolved githubapi.ResolvedRef
	files    map[string][]byte
}

func (f fakeSourceClient) ResolveSelector(_ context.Context, _ string, _ selector.Selector) (githubapi.ResolvedRef, error) {
	return f.resolved, nil
}

func (f fakeSourceClient) FetchFile(_ context.Context, _ string, filePath, _ string) ([]byte, error) {
	data, ok := f.files[filePath]
	if !ok {
		return nil, githubapi.ErrNotFound
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	return cp, nil
}

func TestRunDryRunWritesNoWorkspaceFiles(t *testing.T) {
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, ".github"), 0o755); err != nil {
		t.Fatalf("mkdir .github: %v", err)
	}
	configData := strings.TrimSpace(`
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
`) + "\n"
	configPath := filepath.Join(workdir, ".github", ".doclane.yml")
	if err := os.WriteFile(configPath, []byte(configData), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	oldFactory := newSourceClient
	newSourceClient = func(string) sourceClient {
		return fakeSourceClient{
			resolved: githubapi.ResolvedRef{
				Selector:         "commit:0123456789abcdef0123456789abcdef01234567",
				ResolvedSelector: "commit:0123456789abcdef0123456789abcdef01234567",
				CommitSHA:        "0123456789abcdef0123456789abcdef01234567",
				CommitDate:       time.Date(2026, 2, 25, 0, 0, 0, 0, time.UTC),
			},
			files: map[string][]byte{
				"README.md": []byte("hello"),
			},
		}
	}
	defer func() {
		newSourceClient = oldFactory
	}()

	result, err := Run(context.Background(), Options{
		WorkDir:        workdir,
		ConfigPath:     ".github/.doclane.yml",
		LockfilePath:   ".github/.doclane.lock.yml",
		ManifestPath:   "doclane-sync-manifest.json",
		GitHubToken:    "token",
		CreatePR:       false,
		TemplateStrict: true,
		DryRun:         true,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(result.Files) != 1 || !result.Files[0].Changed {
		t.Fatalf("unexpected dry-run file results: %+v", result.Files)
	}

	assertMissing := func(rel string) {
		t.Helper()
		if _, err := os.Stat(filepath.Join(workdir, rel)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be absent after dry-run, stat err=%v", rel, err)
		}
	}
	assertMissing(filepath.Join("docs", "README.md"))
	assertMissing(filepath.Join(".github", ".doclane.lock.yml"))
	assertMissing("doclane-sync-manifest.json")
}

func TestRunRejectsEscapingConfigPath(t *testing.T) {
	t.Parallel()

	_, err := Run(context.Background(), Options{
		WorkDir:      t.TempDir(),
		ConfigPath:   "../outside.yml",
		LockfilePath: ".github/.doclane.lock.yml",
		ManifestPath: "doclane-sync-manifest.json",
		GitHubToken:  "token",
	})
	if err == nil {
		t.Fatal("expected config-path escape to fail")
	}
}
