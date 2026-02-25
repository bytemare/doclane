package policy

import (
	"strings"
	"testing"

	"github.com/bytemare/doclane/internal/config"
	"github.com/bytemare/doclane/internal/selector"
)

func TestPolicyEnforceRequiredAndPrefix(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Version: 1,
		Policy:  config.PolicyConfig{Mode: "enforce"},
		Sync: []config.SyncEntry{
			{ID: "code_of_conduct", Source: "open-source/CODE_OF_CONDUCT.md", Target: "docs/CODE_OF_CONDUCT.md"},
		},
	}
	sel, _ := selector.Parse("branch:main")
	p := &Policy{
		Version: 1,
		Required: []RequiredRule{
			{ID: "code_of_conduct", Source: "open-source/CODE_OF_CONDUCT.md", Target: "docs/CODE_OF_CONDUCT.md"},
		},
		Constraints: Constraints{
			AllowedTargetPrefixes: []string{"docs/"},
			RequireDCO:            true,
			RequireCommitSigning:  true,
			TemplateMode:          "simple_placeholders_only",
		},
		SourceRequirements: SourceRequirements{
			AllowSelectors: []string{"branch:*"},
		},
	}

	warnings, err := Enforce(cfg, sel, p, EnforceOptions{PolicyMode: "enforce", CommitDCO: true, CommitSigned: true})
	if err != nil {
		t.Fatalf("Enforce returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
}

func TestPolicyEnforceWarnMode(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Version: 1,
		Policy:  config.PolicyConfig{Mode: "warn"},
		Sync: []config.SyncEntry{
			{ID: "x", Source: "a", Target: "README.md"},
		},
	}
	sel, _ := selector.Parse("latest")
	p := &Policy{
		Version: 1,
		Constraints: Constraints{
			AllowedTargetPrefixes: []string{"docs/"},
		},
		SourceRequirements: SourceRequirements{
			AllowSelectors: []string{"tag:*"},
		},
	}
	warnings, err := Enforce(cfg, sel, p, EnforceOptions{})
	if err != nil {
		t.Fatalf("expected warn-only policy to not error, got: %v", err)
	}
	if len(warnings) < 2 {
		t.Fatalf("expected warnings for prefix + selector, got %v", warnings)
	}
}

func TestParseRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	data := []byte(strings.TrimSpace(`
version: 1
constraints:
  allowed_target_prefixes: [docs/]
  unknown_flag: true
`))
	if _, err := Parse(data); err == nil {
		t.Fatal("expected unknown field parse error")
	}
}

func TestPolicyEnforceRejectsNormalizedPrefixBypass(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Version: 1,
		Policy:  config.PolicyConfig{Mode: "enforce"},
		Sync: []config.SyncEntry{
			{ID: "x", Source: "a", Target: "docs/../SECURITY.md"},
		},
	}
	sel, _ := selector.Parse("commit:0123456789abcdef0123456789abcdef01234567")
	p := &Policy{
		Version: 1,
		Constraints: Constraints{
			AllowedTargetPrefixes: []string{"docs/"},
		},
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("policy validate: %v", err)
	}
	if _, err := Enforce(cfg, sel, p, EnforceOptions{PolicyMode: "enforce"}); err == nil {
		t.Fatal("expected normalized prefix bypass to be rejected")
	}
}
