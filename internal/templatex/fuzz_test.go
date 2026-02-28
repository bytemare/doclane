// SPDX-License-Identifier: MIT
//
// Copyright (C) 2026 Daniel Bourdrez. All Rights Reserved.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree or at
// https://spdx.org/licenses/MIT.html

package templatex

import (
	"bytes"
	"reflect"
	"testing"
	"unicode/utf8"
)

func FuzzRender(f *testing.F) {
	f.Add([]byte("Hello {{ shared.project_name }}"), true)
	f.Add([]byte("Hello world"), true)
	f.Add([]byte("{{ shared.project_name }} {{ shared.security_email }}"), false)
	f.Add([]byte{0xff, 0xfe, 0xfd}, true)

	f.Fuzz(func(t *testing.T, input []byte, strict bool) {
		if len(input) > 8<<10 {
			return
		}
		vars := map[string]string{
			"project_name":   "Doclane",
			"security_email": "security@example.com",
		}

		res1, err1 := Render(input, vars, strict)
		if !utf8.Valid(input) {
			if err1 == nil {
				t.Fatalf("expected invalid UTF-8 input to error")
			}
			return
		}

		res2, err2 := Render(input, vars, strict)
		if (err1 == nil) != (err2 == nil) {
			t.Fatalf("non-deterministic error result: err1=%v err2=%v", err1, err2)
		}
		if err1 != nil {
			return
		}
		if !bytes.Equal(res1.Rendered, res2.Rendered) {
			t.Fatalf("non-deterministic render output")
		}
		if !reflect.DeepEqual(res1.UsedVars, res2.UsedVars) {
			t.Fatalf("non-deterministic used vars")
		}
	})
}
