package lockfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOptionalMissing(t *testing.T) {
	t.Parallel()

	lf, raw, err := LoadOptional(filepath.Join(t.TempDir(), "missing.yml"))
	if err != nil {
		t.Fatalf("LoadOptional returned error: %v", err)
	}
	if lf != nil || raw != nil {
		t.Fatalf("expected nil lockfile/raw for missing file, got %#v / %#v", lf, raw)
	}
}

func TestLoadOptionalParseError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".github", ".doclane.lock.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("version: [\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, _, err := LoadOptional(path); err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestLoadOptionalValid(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".github", ".doclane.lock.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data := strings.TrimSpace(`
version: 1
resolved:
  source_repo: acme/shared
  selector: commit:0123456789abcdef0123456789abcdef01234567
  commit_sha: 0123456789abcdef0123456789abcdef01234567
files:
  - id: readme
    source: README.md
    target: docs/README.md
    upstream_sha256: a
    rendered_sha256: b
    templated: false
`) + "\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	lf, raw, err := LoadOptional(path)
	if err != nil {
		t.Fatalf("LoadOptional returned error: %v", err)
	}
	if string(raw) != data {
		t.Fatalf("unexpected raw yaml: %q", string(raw))
	}
	if lf == nil || lf.Resolved.SourceRepo != "acme/shared" || len(lf.Files) != 1 {
		t.Fatalf("unexpected lockfile: %#v", lf)
	}
}

func TestMarshal(t *testing.T) {
	t.Parallel()

	if _, err := Marshal(nil); err == nil {
		t.Fatal("expected nil lockfile error")
	}

	lf := &Lockfile{
		Resolved: Resolved{
			SourceRepo: "acme/shared",
			Selector:   "commit:0123456789abcdef0123456789abcdef01234567",
			CommitSHA:  "0123456789abcdef0123456789abcdef01234567",
		},
		Files: []FileState{{
			ID:          "readme",
			Source:      "README.md",
			Target:      "docs/README.md",
			UpstreamSHA: "a",
			RenderedSHA: "b",
		}},
	}
	data, err := Marshal(lf)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if lf.Version != 1 {
		t.Fatalf("expected version default to 1, got %d", lf.Version)
	}
	if !strings.Contains(string(data), "version: 1") {
		t.Fatalf("expected marshaled yaml to contain version, got:\n%s", data)
	}
}

func TestWrite(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".github", ".doclane.lock.yml")
	lf := &Lockfile{
		Version: 1,
		Resolved: Resolved{
			SourceRepo: "acme/shared",
			Selector:   "tag:v1.0.0",
			CommitSHA:  "0123456789abcdef0123456789abcdef01234567",
		},
		Files: []FileState{
			{
				ID:          "x",
				Source:      "README.md",
				Target:      "docs/README.md",
				UpstreamSHA: "u",
				RenderedSHA: "r",
			},
		},
	}
	if err := Write(path, lf); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written lockfile: %v", err)
	}
	if !strings.Contains(string(got), "source_repo: acme/shared") {
		t.Fatalf("unexpected lockfile contents:\n%s", got)
	}
}
