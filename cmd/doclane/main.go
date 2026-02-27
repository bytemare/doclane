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
