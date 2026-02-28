// SPDX-License-Identifier: MIT
//
// Copyright (C) 2026 Daniel Bourdrez. All Rights Reserved.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree or at
// https://spdx.org/licenses/MIT.html

package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWrite(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "artifacts", "manifest.json")
	start := time.Now().UTC()
	m := Manifest{
		SourceRepo:       "acme/shared",
		Selector:         "tag:v1.2.3",
		ResolvedSelector: "tag:v1.2.3",
		CommitSHA:        "0123456789abcdef0123456789abcdef01234567",
		CommitDate:       "2026-02-26T12:00:00Z",
		Policy: &PolicyRecord{
			Path:        ".doclane-policy.yml",
			UpstreamSHA: "abc",
			Enforced:    true,
			Present:     true,
		},
		Files: []FileRecord{
			{
				ID:          "readme",
				Source:      "README.md",
				Target:      "docs/README.md",
				Templated:   false,
				UpstreamSHA: "a",
				RenderedSHA: "b",
				Changed:     true,
			},
		},
		Warnings: []string{"warn"},
	}

	if err := Write(path, m); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		t.Fatalf("manifest should end with newline, got: %q", raw)
	}

	var got Manifest
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if got.SourceRepo != m.SourceRepo || got.Selector != m.Selector || got.CommitSHA != m.CommitSHA {
		t.Fatalf("unexpected manifest fields: %+v", got)
	}
	if got.GeneratedAt.Before(start) || got.GeneratedAt.After(time.Now().UTC().Add(2*time.Second)) {
		t.Fatalf("generated_at outside expected time window: %s", got.GeneratedAt)
	}
	if len(got.Files) != 1 || got.Files[0].Target != "docs/README.md" {
		t.Fatalf("unexpected file records: %+v", got.Files)
	}
}
