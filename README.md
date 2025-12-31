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

Build with version:

```bash
go build -ldflags "-X main.version=v1.0.0" -o locsquash
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
- `-v`, `-version` - Print version and exit

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

## Testing

Run tests locally:

```bash
go test -v ./...
```

Run tests in Docker:

```bash
docker build -f Dockerfile.test -t locsquash-test .
docker run --rm locsquash-test
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
git reset --hard gosquash/backup-<timestamp>
```

To see recovery instructions before running:

```bash
locsquash -n 3 --print-recovery
```
