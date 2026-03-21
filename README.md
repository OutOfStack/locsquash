# locsquash

A CLI tool to squash the last commits in your Git repository into a single commit.

## Installation

```bash
go install github.com/OutOfStack/locsquash@latest
```

Or build from source:

```bash
git clone https://github.com/OutOfStack/locsquash.git
cd locsquash
make build                    # version = dev
make build VERSION=v1.0.0     # version = v1.0.0
```

## Usage

```bash
locsquash -n <count> [options]
```

### Required

- `-n <count>` - Number of commits to squash (must be at least 2)

### Options

- `-from <ref>` - Anchor the squash range: integer `N` skips the top N commits from HEAD (they are reapplied after squash), or a commit hash sets that commit as the newest one to squash (`ref` and `N-1` commits before it are squashed)
- `-m <msg>` - Custom commit message for the squashed commit (defaults to the oldest commit's message)
- `-y`, `-yes` - Skip confirmation prompt (useful for scripting)
- `-no-backup` - Skip creating backup branch
- `-stash` - Auto-stash uncommitted changes before squashing
- `-allow-empty` - Allow creating an empty commit if squashed changes cancel out
- `-dry-run` - Preview the git commands without executing them
- `-print-recovery` - Print recovery commands and exit
- `-list-backups` - List all backup branches and exit
- `-v`, `-version` - Print version and exit

## Examples

Squash the last 3 commits (will show commits and ask for confirmation):

```bash
locsquash -n 3
```

Squash the last 5 commits with a custom message:

```bash
locsquash -n 5 -m "feat: consolidated feature implementation"
```

Squash without confirmation prompt (for scripting):

```bash
locsquash -n 3 -y
```

Squash without creating a backup branch:

```bash
locsquash -n 3 -y -no-backup
```

Preview what would happen without making changes:

```bash
locsquash -n 3 -dry-run
```

Squash with uncommitted changes (auto-stash):

```bash
locsquash -n 3 -stash
```

Squash 2 commits in the middle of history, skipping the top 1 (reapplied after):

```bash
locsquash -n 2 -from 1 -y
```

Same using a commit hash — `<hash-of-B>` is the newest commit to squash:

```bash
locsquash -n 2 -from <hash-of-B> -y
```

List all backup branches:

```bash
locsquash -list-backups
```

## How It Works

1. Shows the commits that will be squashed and asks for confirmation (skip with `-y`)
2. Creates a backup branch (`locsquash/backup-<timestamp>`) before any changes (skip with `-no-backup`)
3. Optionally stashes uncommitted changes if `-stash` is provided
4. If `-from` targets a commit below HEAD, hard-resets past the commits above the squash range
5. Performs a soft reset to `HEAD~N`
6. Creates a new commit with all changes, preserving the top squash commit's date and using the oldest commit message (unless `-m` is provided)
7. Cherry-picks back any commits that were above the squash range (when `-from` is used)
8. Restores stashed changes if applicable

## Development

```bash
make build                # Build binary to bin/
make build VERSION=v1.0.0 # Build with specific version
make run                  # Run without building
make test                 # Run tests with race detector
make test-docker          # Run tests in Docker
make lint                 # Run linter
```

## Releasing

To create a new release:

```bash
git tag v1.0.0
git push --tags
```

This triggers CI to build binaries for all platforms (Linux, macOS, Windows) and create a GitHub Release with that version.

## Recovery

If something goes wrong, recover using the backup branch:

```bash
git reset --hard locsquash/backup-<timestamp>
```

To list all backup branches:

```bash
locsquash -list-backups
```

To see recovery instructions before running:

```bash
locsquash -n 3 -print-recovery
```

If you used `-no-backup`, recovery is only possible via git reflog:

```bash
git reflog
git reset --hard <commit-hash-before-squash>
```
