package syncer

import (
	"context"
	"errors"
	"testing"

	"github.com/bytemare/doclane/internal/config"
	"github.com/bytemare/doclane/internal/githubapi"
	"github.com/bytemare/doclane/internal/policy"
	"github.com/bytemare/doclane/internal/selector"
)

type policyFetcher struct {
	data map[string][]byte
	err  error
}

func (f policyFetcher) FetchFile(_ context.Context, _ string, filePath, _ string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	data, ok := f.data[filePath]
	if !ok {
		return nil, githubapi.ErrNotFound
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	return cp, nil
}

func TestLoadPolicy(t *testing.T) {
	t.Parallel()

	resolved := githubapi.ResolvedRef{
		CommitSHA: "0123456789abcdef0123456789abcdef01234567",
	}
	cfgWarn := &config.Config{
		Source: config.SourceConfig{Repo: "acme/shared"},
		Policy: config.PolicyConfig{Mode: "warn", Path: ".doclane-policy.yml"},
	}
	cfgEnforce := &config.Config{
		Source: config.SourceConfig{Repo: "acme/shared"},
		Policy: config.PolicyConfig{Mode: "enforce", Path: ".doclane-policy.yml"},
	}

	state, warnings, parsedPolicy, err := loadPolicy(context.Background(), policyFetcher{}, cfgWarn, resolved)
	if err != nil {
		t.Fatalf("loadPolicy warn/missing returned error: %v", err)
	}
	if state == nil || state.Present || state.Enforced {
		t.Fatalf("unexpected warn/missing policy state: %+v", state)
	}
	if len(warnings) != 1 || parsedPolicy != nil {
		t.Fatalf("unexpected warn/missing outputs: warnings=%v policy=%+v", warnings, parsedPolicy)
	}

	if _, _, _, err := loadPolicy(context.Background(), policyFetcher{}, cfgEnforce, resolved); err == nil {
		t.Fatal("expected enforce/missing policy error")
	}

	invalidFetcher := policyFetcher{
		data: map[string][]byte{".doclane-policy.yml": []byte("version: [")},
	}
	state, warnings, parsedPolicy, err = loadPolicy(context.Background(), invalidFetcher, cfgWarn, resolved)
	if err != nil {
		t.Fatalf("loadPolicy warn/invalid returned error: %v", err)
	}
	if state == nil || !state.Present || state.Enforced || state.UpstreamSHA == "" {
		t.Fatalf("unexpected warn/invalid policy state: %+v", state)
	}
	if len(warnings) != 1 || parsedPolicy != nil {
		t.Fatalf("unexpected warn/invalid outputs: warnings=%v policy=%+v", warnings, parsedPolicy)
	}

	validFetcher := policyFetcher{
		data: map[string][]byte{".doclane-policy.yml": []byte("version: 1\n")},
	}
	state, warnings, parsedPolicy, err = loadPolicy(context.Background(), validFetcher, cfgEnforce, resolved)
	if err != nil {
		t.Fatalf("loadPolicy enforce/valid returned error: %v", err)
	}
	if len(warnings) != 0 || parsedPolicy == nil {
		t.Fatalf("unexpected enforce/valid outputs: warnings=%v policy=%+v", warnings, parsedPolicy)
	}
	if !state.Present || !state.Enforced || state.UpstreamSHA == "" {
		t.Fatalf("unexpected enforce/valid policy state: %+v", state)
	}
}

func TestLoadPolicyFetchError(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Source: config.SourceConfig{Repo: "acme/shared"},
		Policy: config.PolicyConfig{Mode: "enforce", Path: ".doclane-policy.yml"},
	}
	_, _, _, err := loadPolicy(context.Background(), policyFetcher{
		err: errors.New("network down"),
	}, cfg, githubapi.ResolvedRef{CommitSHA: "abc"})
	if err == nil {
		t.Fatal("expected fetch error")
	}
}

func TestRequiresSignedTag(t *testing.T) {
	t.Parallel()

	tagSel, err := selector.Parse("tag:v1.0.0")
	if err != nil {
		t.Fatalf("parse selector: %v", err)
	}
	branchSel, err := selector.Parse("branch:main")
	if err != nil {
		t.Fatalf("parse selector: %v", err)
	}
	cfg := &config.Config{
		Source: config.SourceConfig{
			EnforceSignedTags: false,
		},
	}
	pol := &policy.Policy{
		SourceRequirements: policy.SourceRequirements{RequireSignedTags: false},
	}
	if requiresSignedTag(cfg, pol, branchSel) {
		t.Fatal("branch selector should never require signed tag")
	}
	if requiresSignedTag(cfg, pol, tagSel) {
		t.Fatal("tag selector should not require signed tag when disabled in cfg/policy")
	}
	cfg.Source.EnforceSignedTags = true
	if !requiresSignedTag(cfg, pol, tagSel) {
		t.Fatal("expected cfg enforce_signed_tags to require signed tags")
	}
	cfg.Source.EnforceSignedTags = false
	pol.SourceRequirements.RequireSignedTags = true
	if !requiresSignedTag(cfg, pol, tagSel) {
		t.Fatal("expected policy require_signed_tags to require signed tags")
	}
}
