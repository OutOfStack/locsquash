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

// gitStdout runs a git command and returns its stdout
func gitStdout(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
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
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// createBackupBranch creates a branch from HEAD, retrying with a numeric suffix
// if the base name already exists
func createBackupBranch(ctx context.Context, baseName string) (string, error) {
	for i := 0; ; i++ {
		name := baseName
		if i > 0 {
			name = fmt.Sprintf("%s-%d", baseName, i)
		}

		if _, err := gitStdout(ctx, "branch", name, "HEAD"); err == nil {
			return name, nil
		} else if !isBranchExistsError(err) {
			return "", err
		}
	}
}

func isBranchExistsError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "A branch named")
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
	cmd := exec.CommandContext(ctx, "git", "diff", "--quiet", baseRef, headRef)
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
	msg := "gosquash auto-stash"
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
	out, err := gitStdout(ctx, "rev-list", "--count", "HEAD")
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
