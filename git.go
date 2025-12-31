package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// gitStdout runs a git command and returns its stdout
func gitStdout(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(errBuf.String()))
	}
	return out.String(), nil
}

// gitStdoutInDir runs a git command in a specific directory and returns its stdout
func gitStdoutInDir(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(errBuf.String()))
	}
	return out.String(), nil
}

// runGitCommand runs a git command with output to stdout/stderr
func runGitCommand(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ensureInsideGitRepo checks if the current directory is inside a git repository
func ensureInsideGitRepo() error {
	out, err := gitStdout("rev-parse", "--is-inside-work-tree")
	if err != nil {
		return errors.New("not a git repository (or any of the parent directories)")
	}
	if strings.TrimSpace(out) != "true" {
		return errors.New("not inside a git work tree")
	}
	return nil
}

// ensureNoInProgressOps checks that no git operation (rebase, merge, etc.) is in progress
func ensureNoInProgressOps() error {
	checks := []string{"REBASE_HEAD", "MERGE_HEAD", "CHERRY_PICK_HEAD", "BISECT_LOG"}
	for _, ref := range checks {
		_, err := gitStdout("rev-parse", "-q", "--verify", ref)
		if err == nil {
			return fmt.Errorf("git operation in progress (%s exists); abort/finish it first", ref)
		}
	}
	return nil
}

// hasUncommittedChanges returns true if there are uncommitted changes in the working directory
func hasUncommittedChanges() (bool, error) {
	out, err := gitStdout("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// stashPushAndGetRef stashes uncommitted changes and returns the stash reference
func stashPushAndGetRef() (string, error) {
	msg := "gosquash auto-stash"
	if err := runGitCommand("stash", "push", "-u", "-m", msg); err != nil {
		return "", err
	}
	if _, err := gitStdout("rev-parse", "-q", "--verify", "refs/stash"); err != nil {
		return "", errors.New("stash push reported success but refs/stash not found")
	}
	return "stash@{0}", nil
}

// gitCommitCount returns the total number of commits in the current branch
func gitCommitCount() (int, error) {
	out, err := gitStdout("rev-list", "--count", "HEAD")
	if err != nil {
		return 0, errors.New("cannot count commits (does HEAD exist?)")
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, err
	}
	return n, nil
}

// gitLogSingle retrieves a single piece of information from a commit
func gitLogSingle(ref, formatStr string) (string, error) {
	return gitStdout("log", "-1", "--format="+formatStr, ref)
}

// gitCommitWithDates creates a commit with specific author and committer dates
func gitCommitWithDates(isoDate, message string) error {
	cmd := exec.Command("git", "commit", "--date", isoDate, "-m", message)
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE="+isoDate)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
