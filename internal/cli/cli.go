package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bytemare/doclane/internal/syncer"
)

// Run executes the Doclane CLI and returns a process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		printRootUsage()
		return 2
	}

	switch args[0] {
	case "sync":
		return runSync(args[1:])
	case "validate":
		return runValidate(args[1:])
	case "version", "--version", "-v":
		fmt.Println("doclane dev")
		return 0
	default:
		printRootUsage()
		return 2
	}
}

func printRootUsage() {
	fmt.Fprintln(os.Stderr, "Usage: doclane <sync|validate|version> [flags]")
}

func runValidate(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config-path", ".github/.doclane.yml", "Path to Doclane config")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if err := syncer.ValidateConfigOnly(*configPath); err != nil {
		fmt.Fprintf(os.Stderr, "validate failed: %v\n", err)
		return 1
	}

	fmt.Println("doclane config is valid")
	return 0
}

func runSync(args []string) int {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var opts syncer.Options
	fs.StringVar(&opts.WorkDir, "workdir", ".", "Consumer repository working directory")
	fs.StringVar(&opts.ConfigPath, "config-path", ".github/.doclane.yml", "Path to Doclane config")
	fs.StringVar(&opts.LockfilePath, "lockfile-path", ".github/.doclane.lock.yml", "Path to Doclane lockfile")
	fs.StringVar(&opts.GitHubToken, "github-token", os.Getenv("GITHUB_TOKEN"), "GitHub token for source fetch and PR creation")
	allowlist := fs.String("source-repo-allowlist", "", "Comma-separated repo allowlist patterns (e.g. org/*)")
	fs.BoolVar(&opts.CreatePR, "create-pr", true, "Create a PR for updates")
	fs.StringVar(&opts.PRBranchPrefix, "pr-branch-prefix", "chore/doclane-sync", "Branch prefix for generated PR branches")
	fs.StringVar(&opts.PRBase, "pr-base", "", "Base branch for PR (defaults to current branch)")
	fs.StringVar(&opts.PRTitlePrefix, "pr-title-prefix", "chore(doclane): sync shared docs", "PR title prefix")
	fs.BoolVar(&opts.CommitSignoff, "commit-signoff", true, "Add DCO sign-off to commit")
	fs.BoolVar(&opts.CommitSign, "commit-sign", true, "Cryptographically sign commit (requires Git signing configured)")
	fs.BoolVar(&opts.TemplateStrict, "template-strict", true, "Fail on missing/unused template vars")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "Resolve and render without writing changes")
	fs.StringVar(&opts.GitUserName, "git-user-name", "doclane[bot]", "Git user.name for commits")
	fs.StringVar(&opts.GitUserEmail, "git-user-email", "41898282+github-actions[bot]@users.noreply.github.com", "Git user.email for commits")
	fs.StringVar(&opts.ManifestPath, "manifest-path", "doclane-sync-manifest.json", "Path to write the manifest artifact (relative to workdir)")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *allowlist != "" {
		for _, part := range strings.Split(*allowlist, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				opts.SourceRepoAllowlist = append(opts.SourceRepoAllowlist, part)
			}
		}
	}

	if err := opts.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "invalid flags: %v\n", err)
		return 2
	}

	result, err := syncer.Run(context.Background(), opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sync failed: %v\n", err)
		return 1
	}

	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	writeGitHubOutputs(result)
	if result.Noop {
		fmt.Println("doclane: no changes")
		return 0
	}
	fmt.Printf("doclane: synced %d file(s), resolved %s\n", len(result.Files), result.Resolved.CommitSHA)
	if result.PRURL != "" {
		fmt.Printf("doclane: PR %s\n", result.PRURL)
	}
	return 0
}

func writeGitHubOutputs(result syncer.Result) {
	outputPath := strings.TrimSpace(os.Getenv("GITHUB_OUTPUT"))
	if outputPath == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(outputPath), 0o755)
	f, err := os.OpenFile(outputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: unable to open GITHUB_OUTPUT: %v\n", err)
		return
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			fmt.Fprintf(os.Stderr, "warning: unable to close GITHUB_OUTPUT: %v\n", cerr)
		}
	}()

	if _, err := fmt.Fprintf(f, "noop=%t\n", result.Noop); err != nil {
		fmt.Fprintf(os.Stderr, "warning: unable to write noop output: %v\n", err)
		return
	}
	if result.Resolved.CommitSHA != "" {
		if _, err := fmt.Fprintf(f, "resolved-sha=%s\n", result.Resolved.CommitSHA); err != nil {
			fmt.Fprintf(os.Stderr, "warning: unable to write resolved-sha output: %v\n", err)
			return
		}
	}
	if result.PRURL != "" {
		if _, err := fmt.Fprintf(f, "pr-url=%s\n", result.PRURL); err != nil {
			fmt.Fprintf(os.Stderr, "warning: unable to write pr-url output: %v\n", err)
			return
		}
	}
}
