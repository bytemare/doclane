// SPDX-License-Identifier: MIT
//
// Copyright (C) 2026 Daniel Bourdrez. All Rights Reserved.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree or at
// https://spdx.org/licenses/MIT.html

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

	tests := []struct {
		selector string
		pattern  string
		want     bool
	}{
		{selector: "latest", pattern: "latest", want: true},
		{selector: "branch:main", pattern: "branch:*", want: true},
		{selector: "branch:main", pattern: "branch:main", want: true},
		{selector: "branch:main", pattern: "branch:dev", want: false},
		{selector: "branch:main", pattern: "tag:*", want: false},
		{selector: "tag:v1.2.3", pattern: "tag:*", want: true},
		{selector: "commit:0123456789abcdef0123456789abcdef01234567", pattern: "commit:*", want: true},
		{selector: "commit:0123456789abcdef0123456789abcdef01234567", pattern: "latest", want: false},
		{selector: "latest", pattern: "", want: false},
		{selector: "latest", pattern: " ", want: false},
	}

	for _, tt := range tests {
		sel, err := Parse(tt.selector)
		if err != nil {
			t.Fatalf("Parse(%q) returned error: %v", tt.selector, err)
		}
		if got := MatchesPolicyPattern(sel, tt.pattern); got != tt.want {
			t.Fatalf("MatchesPolicyPattern(%q, %q)=%t want %t", tt.selector, tt.pattern, got, tt.want)
		}
	}
}
