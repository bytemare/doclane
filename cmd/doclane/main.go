// SPDX-License-Identifier: MIT
//
// Copyright (C) 2026 Daniel Bourdrez. All Rights Reserved.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree or at
// https://spdx.org/licenses/MIT.html

package main

import (
	"os"

	"github.com/bytemare/doclane/internal/cli"
)

var (
	runCLI = cli.Run
	exit   = os.Exit
)

func main() {
	exit(runCLI(os.Args[1:]))
}
