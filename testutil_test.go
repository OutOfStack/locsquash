package main_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// testRepo provides a temporary git repository for testing
type testRepo struct {
	Dir    string
	t      *testing.T
	Binary string
}

var testBinaryPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "locsquash-bin-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	binaryName := "locsquash"
	if runtime.GOOS == "windows" {
		binaryName = "locsquash.exe"
	}

	testBinaryPath = filepath.Join(tmpDir, binaryName)
	cmd := exec.CommandContext(context.Background(), "go", "build", "-o", testBinaryPath, ".") //nolint:gosec
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build binary: %v\n", err)
		_ = os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
}

// buildTestBinary compiles the CLI binary once for all tests
func buildTestBinary(t *testing.T) string {
	t.Helper()

	if testBinaryPath == "" {
		t.Fatal("test binary not built; TestMain did not run")
	}
	return testBinaryPath
}

// newTestRepo creates a new temporary git repository for testing
func newTestRepo(t *testing.T) *testRepo {
	t.Helper()

	dir := t.TempDir()

	tr := &testRepo{
		Dir:    dir,
		t:      t,
		Binary: buildTestBinary(t),
	}

	// Initialize git repo
	tr.git(t.Context(), "init")
	tr.git(t.Context(), "config", "user.email", "test@test.local")
	tr.git(t.Context(), "config", "user.name", "Test User")

	return tr
}

// git runs a git command in the test repository
func (tr *testRepo) git(ctx context.Context, args ...string) string {
	tr.t.Helper()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = tr.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		tr.t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// createCommit creates a commit with the given message
func (tr *testRepo) createCommit(message string) {
	tr.t.Helper()
	// Create or modify a file
	filePath := filepath.Join(tr.Dir, "file.txt")
	content := message + "\n"

	// Append to file to ensure changes
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec
	if err != nil {
		tr.t.Fatalf("failed to open file: %v", err)
	}
	if _, err = f.WriteString(content); err != nil {
		if cErr := f.Close(); cErr != nil {
			tr.t.Logf("failed to close file: %v", cErr)
		}
		tr.t.Fatalf("failed to write file: %v", err)
	}
	if err = f.Close(); err != nil {
		tr.t.Fatalf("failed to close file: %v", err)
	}

	tr.git(tr.t.Context(), "add", ".")
	tr.git(tr.t.Context(), "commit", "-m", message)
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
	out := tr.git(tr.t.Context(), "rev-list", "--count", "HEAD")
	count, err := strconv.Atoi(out)
	if err != nil {
		tr.t.Fatalf("failed to parse commit count from %q: %v", out, err)
	}
	return count
}

// lastCommitMessage returns the message of the most recent commit
func (tr *testRepo) lastCommitMessage() string {
	tr.t.Helper()
	return tr.git(tr.t.Context(), "log", "-1", "--format=%s")
}

// runCLI runs the locsquash binary with the given arguments
func (tr *testRepo) runCLI(args ...string) (string, error) {
	tr.t.Helper()
	cmd := exec.CommandContext(tr.t.Context(), tr.Binary, args...) //nolint:gosec
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
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		tr.t.Fatalf("failed to write file %s: %v", name, err)
	}
}
