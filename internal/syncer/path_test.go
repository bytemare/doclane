package syncer

import (
	"path/filepath"
	"testing"
)

func TestSafeTargetPath(t *testing.T) {
	t.Parallel()

	root := filepath.Join(string(filepath.Separator), "tmp", "repo")
	if _, err := safeTargetPath(root, "../escape"); err == nil {
		t.Fatal("expected path traversal to fail")
	}
	if got, err := safeTargetPath(root, "docs/CODE_OF_CONDUCT.md"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else if filepath.Clean(got) != filepath.Join(root, "docs", "CODE_OF_CONDUCT.md") {
		t.Fatalf("unexpected safe path: %q", got)
	}
}

func TestSafeRepoRelativePath(t *testing.T) {
	t.Parallel()

	root := filepath.Join(string(filepath.Separator), "tmp", "repo")
	if _, _, err := safeRepoRelativePath(root, "../escape.yml"); err == nil {
		t.Fatal("expected repo-relative escape to fail")
	}
	if _, _, err := safeRepoRelativePath(root, filepath.Join(string(filepath.Separator), "abs.yml")); err == nil {
		t.Fatal("expected absolute path to fail")
	}
	if gotAbs, gotRel, err := safeRepoRelativePath(root, "./.github/../.github/.doclane.yml"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else {
		wantRel := filepath.Join(".github", ".doclane.yml")
		if gotRel != wantRel {
			t.Fatalf("unexpected normalized rel path: %q", gotRel)
		}
		if filepath.Clean(gotAbs) != filepath.Join(root, wantRel) {
			t.Fatalf("unexpected absolute path: %q", gotAbs)
		}
	}
}

func TestCheckAllowlist(t *testing.T) {
	t.Parallel()

	if err := checkAllowlist("acme/shared", nil); err != nil {
		t.Fatalf("unexpected error with empty allowlist: %v", err)
	}
	if err := checkAllowlist("acme/shared", []string{"acme/*"}); err != nil {
		t.Fatalf("unexpected allowlist miss: %v", err)
	}
	if err := checkAllowlist("acme/shared", []string{"other/*"}); err == nil {
		t.Fatal("expected source repo allowlist rejection")
	}
	if err := checkAllowlist("acme/shared", []string{"["}); err == nil {
		t.Fatal("expected invalid pattern error")
	}
}
