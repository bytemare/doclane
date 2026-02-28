// SPDX-License-Identifier: MIT
//
// Copyright (C) 2026 Daniel Bourdrez. All Rights Reserved.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree or at
// https://spdx.org/licenses/MIT.html

package policy

import "testing"

func FuzzParsePolicy(f *testing.F) {
	f.Add([]byte(`
version: 1
constraints:
  allowed_target_prefixes:
    - docs/
source_requirements:
  allow_selectors:
    - tag:*
`))
	f.Add([]byte(`version: 1`))
	f.Add([]byte(`version: [not-a-scalar]`))
	f.Add([]byte(`constraints: { unknown_flag: true }`))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 16<<10 {
			return
		}
		p, err := Parse(data)
		if err != nil {
			return
		}
		if p == nil {
			t.Fatal("Parse returned nil policy without error")
		}
		if err := p.Validate(); err != nil {
			t.Fatalf("Validate failed after successful Parse: %v", err)
		}
	})
}
