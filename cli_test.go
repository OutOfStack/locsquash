package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCLI_SquashTwoCommits tests squashing 2 commits into 1
func TestCLI_SquashTwoCommits(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("first", "second", "third")

	initialCount := tr.commitCount()
	if initialCount != 3 {
		t.Fatalf("expected 3 commits, got %d", initialCount)
	}

	tr.runCLISuccess("-n", "2", "-m", "squashed commit")

	finalCount := tr.commitCount()
	if finalCount != 2 {
		t.Errorf("expected 2 commits after squash, got %d", finalCount)
	}

	lastMsg := tr.lastCommitMessage()
	if lastMsg != "squashed commit" {
		t.Errorf("expected commit message 'squashed commit', got %q", lastMsg)
	}
}

// TestCLI_SquashThreeCommits tests squashing 3 commits into 1
func TestCLI_SquashThreeCommits(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("one", "two", "three", "four")

	tr.runCLISuccess("-n", "3", "-m", "combined")

	if count := tr.commitCount(); count != 2 {
		t.Errorf("expected 2 commits after squashing 3, got %d", count)
	}
}

// TestCLI_UsesOldestMessageByDefault tests that without -m, oldest commit message is used
func TestCLI_UsesOldestMessageByDefault(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("base", "oldest of squash", "middle", "newest")

	tr.runCLISuccess("-n", "3") // Squash last 3: oldest of squash, middle, newest

	lastMsg := tr.lastCommitMessage()
	if lastMsg != "oldest of squash" {
		t.Errorf("expected 'oldest of squash', got %q", lastMsg)
	}
}

// TestCLI_CreatesBackupBranch tests that a backup branch is created
func TestCLI_CreatesBackupBranch(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("a", "b", "c")

	tr.runCLISuccess("-n", "2", "-m", "squashed")

	// Check for gosquash/backup-* branch
	out := tr.git("branch", "-a")
	if !strings.Contains(out, "gosquash/backup-") {
		t.Errorf("expected backup branch to be created, branches: %s", out)
	}
}

// TestCLI_DryRunNoChanges tests that dry-run doesn't modify the repository
func TestCLI_DryRunNoChanges(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("first", "second", "third")

	beforeHead := tr.git("rev-parse", "HEAD")

	out := tr.runCLISuccess("-n", "2", "-dry-run")

	afterHead := tr.git("rev-parse", "HEAD")

	if beforeHead != afterHead {
		t.Errorf("dry-run modified HEAD: before=%s, after=%s", beforeHead, afterHead)
	}

	if !strings.Contains(out, "Dry run") {
		t.Errorf("expected 'Dry run' in output, got: %s", out)
	}
}

// TestCLI_PrintRecovery tests the print-recovery flag
func TestCLI_PrintRecovery(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("a", "b", "c")

	out := tr.runCLISuccess("-n", "2", "-print-recovery")

	if !strings.Contains(out, "Recovery instructions") {
		t.Errorf("expected recovery instructions in output, got: %s", out)
	}

	if !strings.Contains(out, "git reset --hard") {
		t.Errorf("expected reset command in recovery output, got: %s", out)
	}
}

// TestCLI_FailsWithNLessThan2 tests that -n < 2 fails
func TestCLI_FailsWithNLessThan2(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("a", "b", "c")

	out := tr.runCLIFailure("-n", "1")

	if !strings.Contains(out, "must be at least 2") {
		t.Errorf("expected error about -n minimum, got: %s", out)
	}
}

// TestCLI_FailsWithNZero tests that -n=0 fails
func TestCLI_FailsWithNZero(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("a", "b")

	out := tr.runCLIFailure("-n", "0")

	if !strings.Contains(out, "must be at least 2") {
		t.Errorf("expected error about -n minimum, got: %s", out)
	}
}

// TestCLI_FailsOutsideGitRepo tests that running outside a git repo fails
func TestCLI_FailsOutsideGitRepo(t *testing.T) {
	// Create temp dir without git init
	dir, err := os.MkdirTemp("", "locsquash-nogit-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	binary := buildTestBinary(t)

	cmd := exec.Command(binary, "-n", "2")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatalf("expected failure outside git repo, got success")
	}

	if !strings.Contains(string(out), "not a git repository") {
		t.Errorf("expected 'not a git repository' error, got: %s", out)
	}
}

// TestCLI_FailsWhenSquashingEntireHistory tests that you can't squash all commits
func TestCLI_FailsWhenSquashingEntireHistory(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("only", "two")

	out := tr.runCLIFailure("-n", "2") // Can't squash both commits

	if !strings.Contains(out, "can't squash the entire history") {
		t.Errorf("expected error about entire history, got: %s", out)
	}
}

// TestCLI_FailsWithUncommittedChanges tests dirty working directory handling
func TestCLI_FailsWithUncommittedChanges(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("a", "b", "c")

	// Create uncommitted change
	tr.writeFile("dirty.txt", "uncommitted content")
	tr.git("add", "dirty.txt")

	out := tr.runCLIFailure("-n", "2")

	if !strings.Contains(out, "uncommitted changes") {
		t.Errorf("expected error about uncommitted changes, got: %s", out)
	}
}

// TestCLI_StashFlagAllowsDirtyRepo tests that -stash allows dirty working directory
func TestCLI_StashFlagAllowsDirtyRepo(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("a", "b", "c")

	// Create uncommitted change
	tr.writeFile("dirty.txt", "uncommitted content")
	tr.git("add", "dirty.txt")

	tr.runCLISuccess("-n", "2", "-m", "squashed", "-stash")

	// Verify squash happened
	if count := tr.commitCount(); count != 2 {
		t.Errorf("expected 2 commits after squash, got %d", count)
	}

	// Verify dirty file is restored
	content, err := os.ReadFile(filepath.Join(tr.Dir, "dirty.txt"))
	if err != nil {
		t.Errorf("dirty file should be restored after stash, got error: %v", err)
	}
	if string(content) != "uncommitted content" {
		t.Errorf("dirty file content mismatch: %s", content)
	}
}

// TestCLI_PreservesRecentCommitDate tests that the squashed commit has the date of the most recent commit
func TestCLI_PreservesRecentCommitDate(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("old", "newer", "newest")

	// Get date of HEAD before squash
	dateBefore := tr.git("log", "-1", "--format=%cI")

	tr.runCLISuccess("-n", "2", "-m", "squashed")

	// Get date of HEAD after squash
	dateAfter := tr.git("log", "-1", "--format=%cI")

	if dateBefore != dateAfter {
		t.Errorf("commit date changed: before=%s, after=%s", dateBefore, dateAfter)
	}
}

// TestCLI_RecoveryFromBackup tests that we can recover using the backup branch
func TestCLI_RecoveryFromBackup(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("a", "b", "c", "d")

	headBefore := tr.git("rev-parse", "HEAD")

	tr.runCLISuccess("-n", "2", "-m", "squashed")

	// Find backup branch
	branches := tr.git("branch", "-a")
	var backupBranch string
	for _, line := range strings.Split(branches, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "gosquash/backup-") {
			backupBranch = strings.TrimPrefix(line, "* ")
			backupBranch = strings.TrimSpace(backupBranch)
			break
		}
	}

	if backupBranch == "" {
		t.Fatal("backup branch not found")
	}

	// Recover
	tr.git("reset", "--hard", backupBranch)

	headAfter := tr.git("rev-parse", "HEAD")
	if headBefore != headAfter {
		t.Errorf("recovery failed: before=%s, after=%s", headBefore, headAfter)
	}
}

// TestCLI_MultipleSquashesCreateUniqueBackups tests that each squash creates a unique backup
func TestCLI_MultipleSquashesCreateUniqueBackups(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("1", "2", "3", "4", "5", "6")

	tr.runCLISuccess("-n", "2", "-m", "first squash")

	// Wait to ensure unique backup branch name (uses second precision)
	time.Sleep(1100 * time.Millisecond)

	// Need to create more commits for second squash
	tr.createCommitsWithMessages("7", "8")

	tr.runCLISuccess("-n", "2", "-m", "second squash")

	// Count backup branches
	branches := tr.git("branch", "-a")
	backupCount := strings.Count(branches, "gosquash/backup-")

	if backupCount < 2 {
		t.Errorf("expected at least 2 backup branches, found %d in:\n%s", backupCount, branches)
	}
}
