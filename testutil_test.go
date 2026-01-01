package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// testRepo provides a temporary git repository for testing
type testRepo struct {
	Dir    string
	t      *testing.T
	Binary string
}

var (
	buildOnce  sync.Once
	binaryPath string
	buildErr   error
)

// buildTestBinary compiles the CLI binary once for all tests
func buildTestBinary(t *testing.T) string {
	t.Helper()

	buildOnce.Do(func() {
		tmpDir, err := os.MkdirTemp("", "locsquash-bin-")
		if err != nil {
			buildErr = err
			return
		}

		binaryName := "locsquash"
		if runtime.GOOS == "windows" {
			binaryName = "locsquash.exe"
		}

		binaryPath = filepath.Join(tmpDir, binaryName)
		cmd := exec.Command("go", "build", "-o", binaryPath, ".")
		cmd.Stderr = os.Stderr
		if err = cmd.Run(); err != nil {
			buildErr = err
		}
	})

	if buildErr != nil {
		t.Fatalf("failed to build binary: %v", buildErr)
	}

	return binaryPath
}

// newTestRepo creates a new temporary git repository for testing
func newTestRepo(t *testing.T) *testRepo {
	t.Helper()

	dir, err := os.MkdirTemp("", "locsquash-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	tr := &testRepo{
		Dir:    dir,
		t:      t,
		Binary: buildTestBinary(t),
	}

	// Initialize git repo
	tr.git("init")
	tr.git("config", "user.email", "test@test.local")
	tr.git("config", "user.name", "Test User")

	t.Cleanup(func() {
		os.RemoveAll(dir)
	})

	return tr
}

// git runs a git command in the test repository
func (tr *testRepo) git(args ...string) string {
	tr.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = tr.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		tr.t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// gitMayFail runs a git command that may fail, returning output and error
func (tr *testRepo) gitMayFail(args ...string) (string, error) {
	tr.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = tr.Dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// createCommit creates a commit with the given message
func (tr *testRepo) createCommit(message string) {
	tr.t.Helper()
	// Create or modify a file
	filePath := filepath.Join(tr.Dir, "file.txt")
	content := message + "\n"

	// Append to file to ensure changes
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		tr.t.Fatalf("failed to open file: %v", err)
	}
	if _, err = f.WriteString(content); err != nil {
		f.Close()
		tr.t.Fatalf("failed to write file: %v", err)
	}
	f.Close()

	tr.git("add", ".")
	tr.git("commit", "-m", message)
}

// createCommits creates multiple commits with numbered messages
func (tr *testRepo) createCommits(count int) {
	tr.t.Helper()
	for i := 1; i <= count; i++ {
		tr.createCommit(strings.Repeat("commit ", 1) + string(rune('0'+i)))
	}
}

// createCommitsWithMessages creates commits with specific messages
func (tr *testRepo) createCommitsWithMessages(messages ...string) {
	tr.t.Helper()
	for _, msg := range messages {
		tr.createCommit(msg)
	}
}

// commitCount returns the number of commits in the repository
func (tr *testRepo) commitCount() int {
	tr.t.Helper()
	out := tr.git("rev-list", "--count", "HEAD")
	count, err := strconv.Atoi(out)
	if err != nil {
		tr.t.Fatalf("failed to parse commit count from %q: %v", out, err)
	}
	return count
}

// lastCommitMessage returns the message of the most recent commit
func (tr *testRepo) lastCommitMessage() string {
	tr.t.Helper()
	return tr.git("log", "-1", "--format=%s")
}

// runCLI runs the locsquash binary with the given arguments
func (tr *testRepo) runCLI(args ...string) (string, error) {
	tr.t.Helper()
	cmd := exec.Command(tr.Binary, args...)
	cmd.Dir = tr.Dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// runCLISuccess runs the CLI and fails the test if it doesn't succeed
func (tr *testRepo) runCLISuccess(args ...string) string {
	tr.t.Helper()
	out, err := tr.runCLI(args...)
	if err != nil {
		tr.t.Fatalf("CLI failed unexpectedly: %v\nOutput: %s", err, out)
	}
	return out
}

// runCLIFailure runs the CLI and fails the test if it succeeds
func (tr *testRepo) runCLIFailure(args ...string) string {
	tr.t.Helper()
	out, err := tr.runCLI(args...)
	if err == nil {
		tr.t.Fatalf("CLI succeeded unexpectedly\nOutput: %s", out)
	}
	return out
}

// writeFile writes content to a file in the test repository
func (tr *testRepo) writeFile(name, content string) {
	tr.t.Helper()
	path := filepath.Join(tr.Dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		tr.t.Fatalf("failed to write file %s: %v", name, err)
	}
}

// branchExists checks if a branch exists in the repository
func (tr *testRepo) branchExists(name string) bool {
	tr.t.Helper()
	_, err := tr.gitMayFail("rev-parse", "--verify", name)
	return err == nil
}
