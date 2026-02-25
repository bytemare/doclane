package selector

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Kind is the selector category used in `source.selector`.
type Kind string

const (
	// KindLatest tracks the source repository default branch head.
	KindLatest Kind = "latest"
	// KindBranch tracks a named branch.
	KindBranch Kind = "branch"
	// KindTag tracks a named tag.
	KindTag Kind = "tag"
	// KindCommit pins an immutable commit SHA.
	KindCommit Kind = "commit"
)

var commitSHARe = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)

// Selector is the parsed representation of `source.selector`.
type Selector struct {
	Raw   string
	Kind  Kind
	Value string
}

// Parse validates and parses a selector string.
func Parse(raw string) (Selector, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Selector{}, errors.New("selector is empty")
	}
	if raw == "latest" {
		return Selector{Raw: raw, Kind: KindLatest}, nil
	}

	prefix, value, ok := strings.Cut(raw, ":")
	if !ok || value == "" {
		return Selector{}, fmt.Errorf("invalid selector %q (expected latest or kind:value)", raw)
	}
	switch Kind(prefix) {
	case KindBranch:
		return Selector{Raw: raw, Kind: KindBranch, Value: value}, nil
	case KindTag:
		return Selector{Raw: raw, Kind: KindTag, Value: value}, nil
	case KindCommit:
		if !commitSHARe.MatchString(value) {
			return Selector{}, fmt.Errorf("commit selector must use a full 40-char SHA, got %q", value)
		}
		return Selector{Raw: raw, Kind: KindCommit, Value: strings.ToLower(value)}, nil
	default:
		return Selector{}, fmt.Errorf("unsupported selector kind %q", prefix)
	}
}

// String returns the normalized selector string.
func (s Selector) String() string {
	if s.Kind == KindLatest {
		return "latest"
	}
	return fmt.Sprintf("%s:%s", s.Kind, s.Value)
}

// MatchesPolicyPattern reports whether a selector matches a policy allow pattern.
func MatchesPolicyPattern(sel Selector, pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	if pattern == "latest" {
		return sel.Kind == KindLatest
	}
	if pattern == "branch:*" {
		return sel.Kind == KindBranch
	}
	if pattern == "tag:*" {
		return sel.Kind == KindTag
	}
	if pattern == "commit:*" {
		return sel.Kind == KindCommit
	}
	return sel.String() == pattern
}
