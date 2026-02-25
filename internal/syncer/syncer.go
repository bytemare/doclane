package syncer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/bytemare/doclane/internal/config"
	"github.com/bytemare/doclane/internal/githubapi"
	"github.com/bytemare/doclane/internal/lockfile"
	"github.com/bytemare/doclane/internal/manifest"
	"github.com/bytemare/doclane/internal/policy"
	"github.com/bytemare/doclane/internal/selector"
)

// Options configures a Doclane sync run.
type Options struct {
	WorkDir             string
	ConfigPath          string
	LockfilePath        string
	ManifestPath        string
	GitHubToken         string
	SourceRepoAllowlist []string
	CreatePR            bool
	PRBranchPrefix      string
	PRBase              string
	PRTitlePrefix       string
	CommitSignoff       bool
	CommitSign          bool
	TemplateStrict      bool
	DryRun              bool
	GitUserName         string
	GitUserEmail        string
}

// Result summarizes a sync run.
type Result struct {
	Noop     bool
	Warnings []string
	Files    []FileResult
	Resolved ResolvedResult
	PRURL    string
}

// ResolvedResult describes the immutable source version used for the run.
type ResolvedResult struct {
	SourceRepo        string
	Selector          string
	ResolvedSelector  string
	CommitSHA         string
	CommitDate        time.Time
	TagVerified       bool
	TagVerificationBy string
}

// FileResult describes one synced file and its integrity hashes.
type FileResult struct {
	ID          string
	Source      string
	Target      string
	Templated   bool
	UpstreamSHA string
	RenderedSHA string
	Changed     bool
}

// Validate checks sync options for required values.
func (o Options) Validate() error {
	if o.ConfigPath == "" {
		return errors.New("config-path is required")
	}
	if o.LockfilePath == "" {
		return errors.New("lockfile-path is required")
	}
	if o.ManifestPath == "" {
		return errors.New("manifest-path is required")
	}
	if strings.TrimSpace(o.GitHubToken) == "" {
		return errors.New("github-token is required")
	}
	return nil
}

// ValidateConfigOnly parses and validates a consumer config file.
func ValidateConfigOnly(configPath string) error {
	_, _, err := config.Load(configPath)
	return err
}

// Run executes the Doclane sync workflow for a consumer repository checkout.
func Run(ctx context.Context, opts Options) (Result, error) {
	if err := opts.Validate(); err != nil {
		return Result{}, err
	}

	workdir := opts.WorkDir
	if strings.TrimSpace(workdir) == "" {
		workdir = "."
	}
	workdir = filepath.Clean(workdir)

	cfgPath := filepath.Join(workdir, opts.ConfigPath)
	lockPath := filepath.Join(workdir, opts.LockfilePath)
	manifestPath := filepath.Join(workdir, opts.ManifestPath)

	cfg, _, err := config.Load(cfgPath)
	if err != nil {
		return Result{}, err
	}

	sel, err := selector.Parse(cfg.Source.Selector)
	if err != nil {
		return Result{}, fmt.Errorf("source.selector: %w", err)
	}
	// Defense in depth: config validation already enforces this.
	if sel.Kind == selector.KindLatest && !cfg.Source.AllowLatest {
		return Result{}, errors.New("source.selector=latest requires source.allow_latest=true")
	}

	if err := checkAllowlist(cfg.Source.Repo, opts.SourceRepoAllowlist); err != nil {
		return Result{}, err
	}

	var client sourceClient = githubapi.New(opts.GitHubToken)
	resolved, err := client.ResolveSelector(ctx, cfg.Source.Repo, sel)
	if err != nil {
		return Result{}, fmt.Errorf("resolve selector %q: %w", sel.String(), err)
	}

	result := Result{
		Resolved: toResolvedResult(cfg.Source.Repo, resolved),
	}

	_, existingLockBytes, err := lockfile.LoadOptional(lockPath)
	if err != nil {
		return Result{}, err
	}

	polState, warnings, pol, err := loadPolicy(ctx, client, cfg, resolved)
	if err != nil {
		return Result{}, err
	}
	result.Warnings = append(result.Warnings, warnings...)

	policyWarnings, err := policy.Enforce(cfg, sel, pol, policy.EnforceOptions{
		PolicyMode:   cfg.Policy.Mode,
		CommitDCO:    opts.CommitSignoff,
		CommitSigned: opts.CommitSign,
	})
	if err != nil {
		return Result{}, err
	}
	result.Warnings = append(result.Warnings, policyWarnings...)

	if err := enforceTagVerification(cfg, pol, sel, resolved); err != nil {
		return Result{}, err
	}

	staged, fileResults, err := stageFiles(ctx, client, cfg, resolved.CommitSHA, workdir, opts.TemplateStrict)
	if err != nil {
		return Result{}, err
	}
	result.Files = fileResults

	newLock := buildLockfile(cfg, resolved, polState, staged)
	newLockBytes, err := lockfile.Marshal(newLock)
	if err != nil {
		return Result{}, err
	}

	lockChanged := !yamlBytesEqual(existingLockBytes, newLockBytes)
	fileChanged := anyFileChanged(result.Files)
	result.Noop = !fileChanged && !lockChanged

	manifestData := buildManifest(cfg.Source.Repo, resolved, polState, result.Files, result.Warnings)

	if opts.DryRun {
		if err := manifest.Write(manifestPath, manifestData); err != nil {
			return Result{}, err
		}
		return result, nil
	}

	if err := writeStagedFiles(staged); err != nil {
		return Result{}, err
	}
	if lockChanged {
		if err := lockfile.Write(lockPath, newLock); err != nil {
			return Result{}, err
		}
	}
	if err := manifest.Write(manifestPath, manifestData); err != nil {
		return Result{}, err
	}

	if result.Noop {
		return result, nil
	}

	if opts.CreatePR {
		prURL, err := createPullRequestFlow(ctx, opts, cfg, resolved, result.Files, workdir)
		if err != nil {
			return Result{}, err
		}
		result.PRURL = prURL
	}

	return result, nil
}

func toResolvedResult(sourceRepo string, resolved githubapi.ResolvedRef) ResolvedResult {
	out := ResolvedResult{
		SourceRepo:       sourceRepo,
		Selector:         resolved.Selector,
		ResolvedSelector: resolved.ResolvedSelector,
		CommitSHA:        resolved.CommitSHA,
		CommitDate:       resolved.CommitDate,
	}
	if resolved.TagVerification != nil {
		out.TagVerified = resolved.TagVerification.Verified
		out.TagVerificationBy = resolved.TagVerification.Source
	}
	return out
}

func enforceTagVerification(cfg *config.Config, pol *policy.Policy, sel selector.Selector, resolved githubapi.ResolvedRef) error {
	if !requiresSignedTag(cfg, pol, sel) {
		return nil
	}
	if resolved.TagVerification == nil {
		return fmt.Errorf("signed tag enforcement enabled but selector %q did not produce tag verification metadata", sel.String())
	}
	if !resolved.TagVerification.Verified {
		return fmt.Errorf("tag %q is not verified (reason: %s)", sel.Value, resolved.TagVerification.Reason)
	}
	return nil
}

func anyFileChanged(files []FileResult) bool {
	for _, f := range files {
		if f.Changed {
			return true
		}
	}
	return false
}

func yamlBytesEqual(a, b []byte) bool {
	return bytes.Equal(bytes.TrimSpace(a), bytes.TrimSpace(b))
}
