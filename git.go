package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const (
	headRef = "HEAD"
)

// gitStdout runs a git command and returns its stdout
func gitStdout(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // args are always hardcoded strings from internal callers
	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(errBuf.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// runGitCommand runs a git command with output to stdout/stderr
func runGitCommand(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // args are always hardcoded strings from internal callers
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// branchExists checks if a branch with the given name exists.
// Uses git show-ref which is locale-independent (avoids parsing error messages).
func branchExists(ctx context.Context, name string) bool {
	cmd := exec.CommandContext(ctx, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+name) //nolint:gosec // name is controlled internally (timestamp-based backup name)
	return cmd.Run() == nil
}

// createBackupBranch creates a branch from HEAD, retrying with a numeric suffix
// if the base name already exists
func createBackupBranch(ctx context.Context, baseName string) (string, error) {
	const maxAttempts = 10
	for i := range maxAttempts {
		name := baseName
		if i > 0 {
			name = fmt.Sprintf("%s-%d", baseName, i+1)
		}

		if branchExists(ctx, name) {
			continue
		}

		if _, err := gitStdout(ctx, "branch", name, headRef); err != nil {
			return "", err
		}
		return name, nil
	}
	return "", fmt.Errorf("failed to create backup branch %s after %d attempts", baseName, maxAttempts)
}

// ensureInsideGitRepo checks if the current directory is inside a git repository
func ensureInsideGitRepo(ctx context.Context) error {
	out, err := gitStdout(ctx, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return errors.New("not a git repository (or any of the parent directories)")
	}
	if out != "true" {
		return errors.New("not inside a git work tree")
	}
	return nil
}

// ensureNoInProgressOps checks that no git operation (rebase, merge, etc.) is in progress
func ensureNoInProgressOps(ctx context.Context) error {
	checks := []string{"REBASE_HEAD", "MERGE_HEAD", "CHERRY_PICK_HEAD", "BISECT_LOG"}
	for _, ref := range checks {
		_, err := gitStdout(ctx, "rev-parse", "-q", "--verify", ref)
		if err == nil {
			return fmt.Errorf("git operation in progress (%s exists); abort/finish it first", ref)
		}
	}
	return nil
}

// hasUncommittedChanges returns true if there are uncommitted changes in the working directory
func hasUncommittedChanges(ctx context.Context) (bool, error) {
	out, err := gitStdout(ctx, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// gitHasChangesBetween returns true if there are changes between two refs.
func gitHasChangesBetween(ctx context.Context, baseRef, headRef string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--quiet", baseRef, headRef) //nolint:gosec // refs are internally computed HEAD~N strings
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// stashPushAndGetRef stashes uncommitted changes and returns the stash reference
func stashPushAndGetRef(ctx context.Context) (string, error) {
	msg := "locsquash auto-stash"
	if err := runGitCommand(ctx, "stash", "push", "-u", "-m", msg); err != nil {
		return "", err
	}
	if _, err := gitStdout(ctx, "rev-parse", "-q", "--verify", "refs/stash"); err != nil {
		return "", errors.New("stash push reported success but refs/stash not found")
	}
	return "stash@{0}", nil
}

// gitCommitCount returns the total number of commits in the current branch
func gitCommitCount(ctx context.Context) (int, error) {
	out, err := gitStdout(ctx, "rev-list", "--count", headRef)
	if err != nil {
		return 0, errors.New("cannot count commits (does HEAD exist?)")
	}
	n, err := strconv.Atoi(out)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// gitLogSingle retrieves a single piece of information from a commit
func gitLogSingle(ctx context.Context, ref, formatStr string) (string, error) {
	return gitStdout(ctx, "log", "-1", "--format="+formatStr, ref)
}

// gitResolveRef resolves any ref to a canonical SHA (handles short hashes, branch names, HEAD~N, etc.)
func gitResolveRef(ctx context.Context, ref string) (string, error) {
	out, err := gitStdout(ctx, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("ref %q not found: %v", ref, err)
	}
	return out, nil
}

// gitCountCommitsAfter returns the number of commits strictly between hash and HEAD.
// Returns an error if hash is not an ancestor of HEAD.
func gitCountCommitsAfter(ctx context.Context, hash string) (int, error) {
	cmd := exec.CommandContext(ctx, "git", "merge-base", "--is-ancestor", hash, headRef) //nolint:gosec // hash is a resolved SHA from gitResolveRef
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return 0, fmt.Errorf("commit %.8s is not an ancestor of HEAD", hash)
		}
		return 0, err
	}
	out, err := gitStdout(ctx, "rev-list", "--first-parent", "--count", hash+"..HEAD")
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(out)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// gitFindMergeCommitInTop returns the hash of the first merge commit found among the top count
// commits from HEAD (first-parent only), or empty string if none exist.
// Uses range HEAD~count..HEAD so only those exact commits are inspected, not all of history.
func gitFindMergeCommitInTop(ctx context.Context, count int) (string, error) {
	out, err := gitStdout(ctx, "log", "--first-parent", "--merges", "--format=%H",
		fmt.Sprintf("%s~%d..%s", headRef, count, headRef))
	if err != nil {
		return "", err
	}
	if out == "" {
		return "", nil
	}
	return strings.SplitN(out, "\n", 2)[0], nil
}

// gitGetCommitHashes returns hashes of the top count commits from HEAD (newest first)
func gitGetCommitHashes(ctx context.Context, count int) ([]string, error) {
	out, err := gitStdout(ctx, "log", "--first-parent", "-"+strconv.Itoa(count), "--format=%H", headRef)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(out, "\n")
	hashes := make([]string, 0, count)
	for _, l := range lines {
		if l != "" {
			hashes = append(hashes, l)
		}
	}
	return hashes, nil
}

// gitLogCommits retrieves the list of commits that will be squashed.
// skip offsets the starting point down from HEAD (0 = start at HEAD).
func gitLogCommits(ctx context.Context, count, skip int) ([]CommitInfo, error) {
	startRef := headRef
	if skip > 0 {
		startRef = fmt.Sprintf("%s~%d", headRef, skip)
	}
	// Format: short hash + tab + subject
	// Use --first-parent to match HEAD~N traversal used by git reset
	out, err := gitStdout(ctx, "log", "--first-parent", "-"+strconv.Itoa(count), "--format=%h\t%s", startRef)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(out, "\n")
	commits := make([]CommitInfo, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			commits = append(commits, CommitInfo{Hash: parts[0], Subject: parts[1]})
		}
	}
	return commits, nil
}

// gitCommitWithDates creates a commit with specific author and committer dates
func gitCommitWithDates(ctx context.Context, isoDate, message string, allowEmpty bool) error {
	args := []string{"commit", "--date", isoDate}
	if allowEmpty {
		args = append(args, "--allow-empty")
	}
	args = append(args, "-m", message)
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // Arguments are fixed git flags
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE="+isoDate)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// BackupBranch holds information about a backup branch
type BackupBranch struct {
	Name      string // Full branch name (e.g., locsquash/backup-20240115-143022)
	CommitRef string // Short commit hash the branch points to
	Subject   string // Commit subject
}

// listBackupBranches returns all branches matching the locsquash/backup-* pattern
func listBackupBranches(ctx context.Context) ([]BackupBranch, error) {
	// List branches matching pattern with commit hash and subject
	// Format: refname:short + tab + objectname:short + tab + subject
	out, err := gitStdout(ctx, "for-each-ref",
		"--format=%(refname:short)\t%(objectname:short)\t%(subject)",
		"--sort=-creatordate",
		"refs/heads/locsquash/backup-*")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	lines := strings.Split(out, "\n")
	branches := make([]BackupBranch, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) >= 2 {
			b := BackupBranch{Name: parts[0], CommitRef: parts[1]}
			if len(parts) == 3 {
				b.Subject = parts[2]
			}
			branches = append(branches, b)
		}
	}
	return branches, nil
}
