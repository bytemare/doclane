# Doclane

Doclane syncs governed shared documentation files from a central GitHub repository into consumer repositories.

## What it does

- Resolves a source `selector` (`latest`, `branch:*`, `tag:*`, `commit:*`) to an immutable commit SHA
- Fetches selected files from the source repo at that commit
- Applies strict placeholder templating (optional)
- Records provenance and file hashes in `.github/.doclane.lock.yml`
- Enforces a central `.doclane-policy.yml`
- Opens a PR with DCO + cryptographically signed commit support (when git signing is preconfigured)

## Consumer Files

- Config: `.github/.doclane.yml`
- Lockfile: `.github/.doclane.lock.yml`
- Central policy in source repo: `.doclane-policy.yml`

## Example Consumer Workflow

See `examples/consumer/.github/workflows/doclane.yml`.

## Security Notes

- `latest` is supported only with explicit `allow_latest: true`
- Doclane always resolves mutable selectors to an immutable commit SHA and records it in the lockfile
- Signed tag enforcement is supported for tag selectors using GitHub API verification metadata
- Commit signing is enforced by passing `git commit -S`; the workflow must configure SSH or GPG signing before running the action
- Consumers should pin `uses: bytemare/doclane@<commit-sha>` to an immutable commit SHA
- Full immutability also depends on Doclane pinning its nested actions by SHA in `action.yml` (the repository does this)

## Current Implementation Notes

- The action is Go-based and currently builds the binary at runtime in the composite action
- Releases are source/tag-focused and publish source-level release metadata rather than standalone CLI binaries
- Consumers should pin the Doclane action by commit SHA

## Release Model

- Doclane is currently distributed for use as a GitHub Action, not as a standalone CLI binary
- The security-relevant execution path is: consumer workflow pins action commit SHA -> composite action builds from source at that SHA
- GitHub releases are used for tag/version records and release metadata provenance
- The release workflow verifies tag context and publishes source-level release metadata; CI does not currently perform local `git tag -v` verification with a trusted keyring
