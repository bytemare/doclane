package selector

import (
	"strings"
	"testing"
)

func FuzzParseSelector(f *testing.F) {
	f.Add("latest")
	f.Add("branch:main")
	f.Add("tag:v1.2.3")
	f.Add("commit:0123456789abcdef0123456789abcdef01234567")
	f.Add("commit:DEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEF")
	f.Add("commit:deadbeef")
	f.Add("branch")
	f.Add("foo:bar")
	f.Add("")

	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 1024 {
			return
		}

		sel, err := Parse(raw)
		if err != nil {
			return
		}

		if sel.Kind == "" {
			t.Fatalf("parsed selector has empty kind for %q", raw)
		}
		normalized := sel.String()
		if normalized == "" {
			t.Fatalf("parsed selector has empty normalized string for %q", raw)
		}
		if sel.Kind == KindCommit && sel.Value != strings.ToLower(sel.Value) {
			t.Fatalf("commit selector value not normalized to lowercase: %q", sel.Value)
		}

		reparsed, err := Parse(normalized)
		if err != nil {
			t.Fatalf("reparse of normalized selector %q failed: %v", normalized, err)
		}
		if reparsed.String() != normalized {
			t.Fatalf("normalized selector not stable: got %q want %q", reparsed.String(), normalized)
		}
	})
}
