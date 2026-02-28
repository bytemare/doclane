// SPDX-License-Identifier: MIT
//
// Copyright (C) 2026 Daniel Bourdrez. All Rights Reserved.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree or at
// https://spdx.org/licenses/MIT.html

package syncer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bytemare/doclane/internal/config"
	"github.com/bytemare/doclane/internal/githubapi"
	"github.com/bytemare/doclane/internal/hashutil"
	"github.com/bytemare/doclane/internal/lockfile"
	"github.com/bytemare/doclane/internal/manifest"
	"github.com/bytemare/doclane/internal/templatex"
)

type stagedFile struct {
	entry       config.SyncEntry
	rendered    []byte
	upstreamSHA string
	renderedSHA string
	targetPath  string
	changed     bool
}

// stageFiles fetches source files, applies templating, hashes outputs, and computes diffs.
func stageFiles(
	ctx context.Context,
	client sourceFileFetcher,
	cfg *config.Config,
	commitSHA string,
	workdir string,
	templateStrict bool,
) ([]stagedFile, []FileResult, error) {
	staged := make([]stagedFile, 0, len(cfg.Sync))
	results := make([]FileResult, 0, len(cfg.Sync))

	for _, entry := range cfg.Sync {
		upstream, err := client.FetchFile(ctx, cfg.Source.Repo, entry.Source, commitSHA)
		if err != nil {
			return nil, nil, fmt.Errorf("fetch %s: %w", entry.Source, err)
		}

		rendered := upstream
		if entry.Template != nil {
			renderedResult, err := templatex.Render(upstream, entry.Template.Vars, templateStrict)
			if err != nil {
				return nil, nil, fmt.Errorf("render template for %s: %w", entry.ID, err)
			}
			rendered = renderedResult.Rendered
		}

		targetPath, err := safeTargetPath(workdir, entry.Target)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid target path for %s: %w", entry.ID, err)
		}

		current, err := os.ReadFile(targetPath)
		targetExists := true
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				targetExists = false
				current = nil
			} else {
				return nil, nil, fmt.Errorf("read existing target %s: %w", entry.Target, err)
			}
		}

		upstreamSHA := hashutil.SHA256Hex(upstream)
		renderedSHA := hashutil.SHA256Hex(rendered)
		changed := !targetExists || !bytes.Equal(current, rendered)

		results = append(results, FileResult{
			ID:          entry.ID,
			Source:      entry.Source,
			Target:      entry.Target,
			Templated:   entry.Template != nil,
			UpstreamSHA: upstreamSHA,
			RenderedSHA: renderedSHA,
			Changed:     changed,
		})
		staged = append(staged, stagedFile{
			entry:       entry,
			rendered:    rendered,
			upstreamSHA: upstreamSHA,
			renderedSHA: renderedSHA,
			targetPath:  targetPath,
			changed:     changed,
		})
	}

	return staged, results, nil
}

// buildLockfile converts staged sync results into the machine-managed lockfile structure.
func buildLockfile(cfg *config.Config, resolved githubapi.ResolvedRef, polState *lockfile.PolicyState, staged []stagedFile) *lockfile.Lockfile {
	lf := &lockfile.Lockfile{
		Version: 1,
		Resolved: lockfile.Resolved{
			SourceRepo:       cfg.Source.Repo,
			Selector:         resolved.Selector,
			ResolvedSelector: resolved.ResolvedSelector,
			CommitSHA:        resolved.CommitSHA,
			CommitDate:       resolved.CommitDate.UTC().Format(time.RFC3339),
		},
		Files: make([]lockfile.FileState, 0, len(staged)),
	}
	if resolved.TagVerification != nil {
		lf.Resolved.TagVerified = resolved.TagVerification.Verified
		lf.Resolved.TagVerificationBy = resolved.TagVerification.Source
	}
	if polState != nil {
		lf.Policy = polState
	}
	for _, sf := range staged {
		lf.Files = append(lf.Files, lockfile.FileState{
			ID:          sf.entry.ID,
			Source:      sf.entry.Source,
			Target:      sf.entry.Target,
			UpstreamSHA: sf.upstreamSHA,
			RenderedSHA: sf.renderedSHA,
			Templated:   sf.entry.Template != nil,
		})
	}
	return lf
}

// buildManifest builds the JSON artifact summary for a sync run.
func buildManifest(sourceRepo string, resolved githubapi.ResolvedRef, polState *lockfile.PolicyState, files []FileResult, warnings []string) manifest.Manifest {
	m := manifest.Manifest{
		SourceRepo:       sourceRepo,
		Selector:         resolved.Selector,
		ResolvedSelector: resolved.ResolvedSelector,
		CommitSHA:        resolved.CommitSHA,
		CommitDate:       resolved.CommitDate.UTC().Format(time.RFC3339),
		Warnings:         append([]string(nil), warnings...),
	}
	if polState != nil {
		m.Policy = &manifest.PolicyRecord{
			Path:        polState.Path,
			UpstreamSHA: polState.UpstreamSHA,
			Enforced:    polState.Enforced,
			Present:     polState.Present,
		}
	}
	for _, f := range files {
		m.Files = append(m.Files, manifest.FileRecord{
			ID:          f.ID,
			Source:      f.Source,
			Target:      f.Target,
			Templated:   f.Templated,
			UpstreamSHA: f.UpstreamSHA,
			RenderedSHA: f.RenderedSHA,
			Changed:     f.Changed,
		})
	}
	return m
}

// writeStagedFiles writes changed files only.
func writeStagedFiles(staged []stagedFile) error {
	for _, sf := range staged {
		if !sf.changed {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(sf.targetPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(sf.targetPath, sf.rendered, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", sf.entry.Target, err)
		}
	}
	return nil
}
