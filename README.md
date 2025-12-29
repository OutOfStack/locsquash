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
go build -o locsquash
```

## Usage

```bash
locsquash -n <count> [options]
```

### Required

- `-n <count>` - Number of commits to squash (must be at least 2)

### Options

- `-m <msg>` - Custom commit message for the squashed commit (defaults to the oldest commit's message)
- `-stash` - Auto-stash uncommitted changes before squashing
- `-dry-run` - Preview the git commands without executing them
- `-print-recovery` - Print recovery commands and exit

## Examples

Squash the last 3 commits:

```bash
locsquash -n 3
```

Squash the last 5 commits with a custom message:

```bash
locsquash -n 5 -m "feat: consolidated feature implementation"
```

Preview what would happen without making changes:

```bash
locsquash -n 3 --dry-run
```

Squash with uncommitted changes (auto-stash):

```bash
locsquash -n 3 --stash
```

## How It Works

1. Creates a backup branch (`gosquash/backup-<timestamp>`) before any changes
2. Optionally stashes uncommitted changes if `--stash` is provided
3. Performs a soft reset to `HEAD~N`
4. Creates a new commit with all changes, preserving the most recent commit's date
5. Restores stashed changes if applicable

## Recovery

If something goes wrong, recover using the backup branch:

```bash
git reset --hard gosquash/backup-<timestamp>
```

To see recovery instructions before running:

```bash
locsquash -n 3 --print-recovery
```
