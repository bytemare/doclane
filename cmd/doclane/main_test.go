package main

import (
	"os"
	"testing"
)

func TestMainCallsCLIAndExit(t *testing.T) {
	oldRun := runCLI
	oldExit := exit
	oldArgs := os.Args
	t.Cleanup(func() {
		runCLI = oldRun
		exit = oldExit
		os.Args = oldArgs
	})
	os.Args = []string{"doclane"}

	calledRun := false
	runCLI = func(args []string) int {
		calledRun = true
		if len(args) != 0 {
			t.Fatalf("unexpected args passed to runCLI: %v", args)
		}
		return 7
	}

	exitCode := -1
	exit = func(code int) {
		exitCode = code
	}

	main()

	if !calledRun {
		t.Fatal("expected main to call runCLI")
	}
	if exitCode != 7 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}
}
