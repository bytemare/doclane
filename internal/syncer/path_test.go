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
