// SPDX-License-Identifier: MIT
//
// Copyright (C) 2026 Daniel Bourdrez. All Rights Reserved.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree or at
// https://spdx.org/licenses/MIT.html

package templatex

import "testing"

func TestRenderStrictSuccess(t *testing.T) {
	t.Parallel()

	res, err := Render([]byte("Hello {{ shared.project_name }}"), map[string]string{
		"project_name": "Doclane",
	}, true)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if got := string(res.Rendered); got != "Hello Doclane" {
		t.Fatalf("unexpected render output: %q", got)
	}
}

func TestRenderStrictMissingVar(t *testing.T) {
	t.Parallel()

	_, err := Render([]byte("Hello {{ shared.project_name }}"), map[string]string{}, true)
	if err == nil {
		t.Fatal("expected missing var error")
	}
}

func TestRenderStrictUnusedVar(t *testing.T) {
	t.Parallel()

	_, err := Render([]byte("Hello world"), map[string]string{"project_name": "Doclane"}, true)
	if err == nil {
		t.Fatal("expected unused var error")
	}
}

func TestRenderStrictMultilineBlock(t *testing.T) {
	t.Parallel()

	input := []byte(
		"Header\n{{ shared.security_block }}\nFooter\n",
	)
	vars := map[string]string{
		"security_block": "Line 1\nLine 2\nLine 3",
	}
	res, err := Render(input, vars, true)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	want := "Header\nLine 1\nLine 2\nLine 3\nFooter\n"
	if got := string(res.Rendered); got != want {
		t.Fatalf("unexpected multiline render output:\n%s", got)
	}
	if len(res.UsedVars) != 1 || res.UsedVars[0] != "security_block" {
		t.Fatalf("unexpected used vars: %v", res.UsedVars)
	}
}
