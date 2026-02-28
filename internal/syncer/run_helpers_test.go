// SPDX-License-Identifier: MIT
//
// Copyright (C) 2026 Daniel Bourdrez. All Rights Reserved.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree or at
// https://spdx.org/licenses/MIT.html

package syncer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bytemare/doclane/internal/config"
	"github.com/bytemare/doclane/internal/githubapi"
	"github.com/bytemare/doclane/internal/lockfile"
)

type fakeFileFetcher struct {
	files map[string][]byte
	err   error
}

func (f fakeFileFetcher) FetchFile(_ context.Context, _ string, filePath, _ string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	if data, ok := f.files[filePath]; ok {
		cp := make([]byte, len(data))
		copy(cp, data)
		return cp, nil
	}
	return nil, nil
}

func TestStageFilesMissingTargetEmptyFileCountsAsChanged(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	cfg := &config.Config{
		Source: config.SourceConfig{Repo: "acme/shared"},
		Sync: []config.SyncEntry{
			{ID: "empty", Source: "docs/EMPTY.md", Target: "docs/EMPTY.md"},
		},
	}
	staged, results, err := stageFiles(context.Background(), fakeFileFetcher{
		files: map[string][]byte{"docs/EMPTY.md": {}},
	}, cfg, "deadbeef", workdir, true)
	if err != nil {
		t.Fatalf("stageFiles returned error: %v", err)
	}
	if len(staged) != 1 || len(results) != 1 {
		t.Fatalf("unexpected staged/results lengths: %d/%d", len(staged), len(results))
	}
	if !staged[0].changed || !results[0].Changed {
		t.Fatal("expected missing target with empty upstream file to be treated as changed")
	}
}

func TestStageFilesTemplateAndUnchanged(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	targetPath := filepath.Join(workdir, "docs", "README.md")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("Hello Doclane"), 0o644); err != nil {
		t.Fatalf("seed target file: %v", err)
	}

	cfg := &config.Config{
		Source: config.SourceConfig{Repo: "acme/shared"},
		Sync: []config.SyncEntry{
			{
				ID:     "readme",
				Source: "README.md",
				Target: "docs/README.md",
				Template: &config.TemplateSpec{
					Vars: map[string]string{"name": "Doclane"},
				},
			},
		},
	}

	staged, results, err := stageFiles(context.Background(), fakeFileFetcher{
		files: map[string][]byte{
			"README.md": []byte("Hello {{ shared.name }}"),
		},
	}, cfg, "deadbeef", workdir, true)
	if err != nil {
		t.Fatalf("stageFiles returned error: %v", err)
	}
	if len(staged) != 1 || len(results) != 1 {
		t.Fatalf("unexpected staged/results lengths: %d/%d", len(staged), len(results))
	}
	if staged[0].changed || results[0].Changed {
		t.Fatalf("expected unchanged result, got staged=%+v result=%+v", staged[0], results[0])
	}
	if !results[0].Templated {
		t.Fatalf("expected templated result, got %+v", results[0])
	}
}

func TestStageFilesTemplateSingleWordAndBlock(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	cfg := &config.Config{
		Source: config.SourceConfig{Repo: "acme/shared"},
		Sync: []config.SyncEntry{
			{
				ID:     "repo_overview",
				Source: "templates/REPO_OVERVIEW.md.tmpl",
				Target: "docs/REPO_OVERVIEW.md",
				Template: &config.TemplateSpec{
					Vars: map[string]string{
						"repo_name": "doclane",
					},
				},
			},
			{
				ID:     "security_block",
				Source: "templates/SECURITY_BLOCK.md.tmpl",
				Target: "docs/SECURITY_BLOCK.md",
				Template: &config.TemplateSpec{
					Vars: map[string]string{
						"security_block": "Report to security@example.com.\n\nSLA: 72 hours.",
					},
				},
			},
		},
	}
	staged, results, err := stageFiles(context.Background(), fakeFileFetcher{
		files: map[string][]byte{
			"templates/REPO_OVERVIEW.md.tmpl": []byte(
				"# Shared Repository Overview\n\nRepository: {{ shared.repo_name }}\n",
			),
			"templates/SECURITY_BLOCK.md.tmpl": []byte(
				"# Security Reporting\n\n{{ shared.security_block }}\n",
			),
		},
	}, cfg, "deadbeef", workdir, true)
	if err != nil {
		t.Fatalf("stageFiles returned error: %v", err)
	}
	if len(staged) != 2 || len(results) != 2 {
		t.Fatalf(
			"unexpected staged/results lengths: %d/%d",
			len(staged),
			len(results),
		)
	}

	want := map[string]string{
		"repo_overview": "# Shared Repository Overview\n\nRepository: doclane\n",
		"security_block": "# Security Reporting\n\n" +
			"Report to security@example.com.\n\nSLA: 72 hours.\n",
	}

	for _, sf := range staged {
		got := string(sf.rendered)
		if got != want[sf.entry.ID] {
			t.Fatalf("unexpected rendered output for %s:\n%s", sf.entry.ID, got)
		}
		if sf.renderedSHA == "" || sf.upstreamSHA == "" {
			t.Fatalf("expected non-empty hashes for %s: %+v", sf.entry.ID, sf)
		}
		if !sf.changed {
			t.Fatalf("expected changed=true for new target file %s", sf.entry.Target)
		}
	}
	for _, r := range results {
		if !r.Templated {
			t.Fatalf("expected templated=true for %s", r.ID)
		}
		if !r.Changed {
			t.Fatalf("expected changed=true for %s", r.ID)
		}
	}
}

func TestStageFilesErrors(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Source: config.SourceConfig{Repo: "acme/shared"},
		Sync: []config.SyncEntry{
			{ID: "x", Source: "README.md", Target: "docs/README.md"},
		},
	}
	if _, _, err := stageFiles(context.Background(), fakeFileFetcher{err: errors.New("boom")}, cfg, "deadbeef", t.TempDir(), true); err == nil {
		t.Fatal("expected fetch error")
	}

	cfg.Sync[0].Template = &config.TemplateSpec{Vars: map[string]string{"unused": "x"}}
	if _, _, err := stageFiles(context.Background(), fakeFileFetcher{
		files: map[string][]byte{"README.md": []byte("Hello")},
	}, cfg, "deadbeef", t.TempDir(), true); err == nil || !strings.Contains(err.Error(), "render template") {
		t.Fatalf("expected template rendering error, got %v", err)
	}

	cfg.Sync[0].Template = &config.TemplateSpec{Vars: map[string]string{}}
	if _, _, err := stageFiles(context.Background(), fakeFileFetcher{
		files: map[string][]byte{"README.md": []byte("Hello {{ shared.name }}")},
	}, cfg, "deadbeef", t.TempDir(), true); err == nil || !strings.Contains(err.Error(), "render template") {
		t.Fatalf("expected missing template var error, got %v", err)
	}
}

func TestBuildLockfileAndManifest(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Source: config.SourceConfig{
			Repo: "acme/shared",
		},
	}
	resolved := githubapi.ResolvedRef{
		Selector:         "tag:v1.2.3",
		ResolvedSelector: "tag:v1.2.3",
		CommitSHA:        "0123456789abcdef0123456789abcdef01234567",
		CommitDate:       time.Date(2026, 2, 26, 10, 0, 0, 0, time.UTC),
		TagVerification: &githubapi.TagVerification{
			Verified: true,
			Reason:   "valid",
			Source:   "github-api",
		},
	}
	polState := &lockfile.PolicyState{
		Path:        ".doclane-policy.yml",
		UpstreamSHA: "abc",
		Enforced:    true,
		Present:     true,
	}
	staged := []stagedFile{
		{
			entry: config.SyncEntry{
				ID:       "readme",
				Source:   "README.md",
				Target:   "docs/README.md",
				Template: &config.TemplateSpec{Vars: map[string]string{"k": "v"}},
			},
			upstreamSHA: "u",
			renderedSHA: "r",
		},
	}
	lf := buildLockfile(cfg, resolved, polState, staged)
	if lf.Resolved.SourceRepo != "acme/shared" || !lf.Resolved.TagVerified || lf.Resolved.TagVerificationBy != "github-api" {
		t.Fatalf("unexpected lockfile resolved metadata: %+v", lf.Resolved)
	}
	if lf.Policy == nil || !lf.Policy.Enforced {
		t.Fatalf("unexpected lockfile policy state: %+v", lf.Policy)
	}
	if len(lf.Files) != 1 || !lf.Files[0].Templated {
		t.Fatalf("unexpected lockfile file records: %+v", lf.Files)
	}

	files := []FileResult{
		{
			ID:          "readme",
			Source:      "README.md",
			Target:      "docs/README.md",
			Templated:   true,
			UpstreamSHA: "u",
			RenderedSHA: "r",
			Changed:     true,
		},
	}
	m := buildManifest(cfg.Source.Repo, resolved, polState, files, []string{"warn"})
	if m.Policy == nil || m.Policy.Path != ".doclane-policy.yml" {
		t.Fatalf("unexpected manifest policy: %+v", m.Policy)
	}
	if len(m.Warnings) != 1 || m.Warnings[0] != "warn" {
		t.Fatalf("unexpected manifest warnings: %+v", m.Warnings)
	}
	if len(m.Files) != 1 || !m.Files[0].Changed {
		t.Fatalf("unexpected manifest files: %+v", m.Files)
	}
}

func TestWriteStagedFiles(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	changed := filepath.Join(workdir, "docs", "changed.md")
	unchanged := filepath.Join(workdir, "docs", "unchanged.md")
	if err := os.MkdirAll(filepath.Dir(unchanged), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(unchanged, []byte("existing"), 0o644); err != nil {
		t.Fatalf("seed unchanged file: %v", err)
	}

	err := writeStagedFiles([]stagedFile{
		{
			entry:      config.SyncEntry{Target: "docs/changed.md"},
			rendered:   []byte("new"),
			targetPath: changed,
			changed:    true,
		},
		{
			entry:      config.SyncEntry{Target: "docs/unchanged.md"},
			rendered:   []byte("different-but-should-not-write"),
			targetPath: unchanged,
			changed:    false,
		},
	})
	if err != nil {
		t.Fatalf("writeStagedFiles returned error: %v", err)
	}

	gotChanged, err := os.ReadFile(changed)
	if err != nil {
		t.Fatalf("read changed file: %v", err)
	}
	if string(gotChanged) != "new" {
		t.Fatalf("unexpected changed file contents: %q", gotChanged)
	}
	gotUnchanged, err := os.ReadFile(unchanged)
	if err != nil {
		t.Fatalf("read unchanged file: %v", err)
	}
	if string(gotUnchanged) != "existing" {
		t.Fatalf("unchanged file should not be rewritten: %q", gotUnchanged)
	}
}
