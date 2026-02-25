package selector

import "testing"

func TestParseSelector(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		kind    Kind
		value   string
		wantErr bool
	}{
		{name: "latest", in: "latest", kind: KindLatest},
		{name: "branch", in: "branch:main", kind: KindBranch, value: "main"},
		{name: "tag", in: "tag:v1.2.3", kind: KindTag, value: "v1.2.3"},
		{name: "commit", in: "commit:0123456789abcdef0123456789abcdef01234567", kind: KindCommit, value: "0123456789abcdef0123456789abcdef01234567"},
		{name: "short commit rejected", in: "commit:deadbeef", wantErr: true},
		{name: "invalid kind", in: "foo:bar", wantErr: true},
		{name: "missing colon", in: "branch", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", tt.in, err)
			}
			if got.Kind != tt.kind || got.Value != tt.value {
				t.Fatalf("Parse(%q) = kind=%q value=%q, want kind=%q value=%q", tt.in, got.Kind, got.Value, tt.kind, tt.value)
			}
		})
	}
}

func TestMatchesPolicyPattern(t *testing.T) {
	t.Parallel()

	sel, _ := Parse("branch:main")
	if !MatchesPolicyPattern(sel, "branch:*") {
		t.Fatal("expected branch selector to match branch:*")
	}
	if MatchesPolicyPattern(sel, "tag:*") {
		t.Fatal("did not expect branch selector to match tag:*")
	}
}
