package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func FuzzNormalizeRepoRelativePath(f *testing.F) {
	f.Add(".github/.doclane.yml")
	f.Add("docs/CODE_OF_CONDUCT.md")
	f.Add("docs/../SECURITY.md")
	f.Add("../escape")
	f.Add(".")
	f.Add("")
	f.Add(`/absolute/path`)
	f.Add(`C:\Windows\System32`)

	f.Fuzz(func(t *testing.T, in string) {
		if len(in) > 4096 {
			return
		}

		out, err := NormalizeRepoRelativePath(in)
		if err != nil {
			return
		}

		if out == "" {
			t.Fatalf("accepted empty normalized path for %q", in)
		}
		if filepath.IsAbs(out) {
			t.Fatalf("accepted absolute normalized path %q for %q", out, in)
		}
		if out == "." || out == ".." {
			t.Fatalf("accepted invalid normalized path %q for %q", out, in)
		}
		prefix := ".." + string(filepath.Separator)
		if strings.HasPrefix(out, prefix) {
			t.Fatalf("accepted escaping normalized path %q for %q", out, in)
		}
	})
}
