package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLI_SquashTwoCommits tests squashing 2 commits into 1
func TestCLI_SquashTwoCommits(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("first", "second", "third")

	initialCount := tr.commitCount()
	if initialCount != 3 {
		t.Fatalf("expected 3 commits, got %d", initialCount)
	}

	tr.runCLISuccess("-n", "2", "-m", "squashed commit", "-yes")

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

	tr.runCLISuccess("-n", "3", "-m", "combined", "-yes")

	if count := tr.commitCount(); count != 2 {
		t.Errorf("expected 2 commits after squashing 3, got %d", count)
	}
}

// TestCLI_UsesOldestMessageByDefault tests that without -m, oldest commit message is used
func TestCLI_UsesOldestMessageByDefault(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("base", "oldest of squash", "middle", "newest")

	tr.runCLISuccess("-n", "3", "-yes") // Squash last 3: oldest of squash, middle, newest

	lastMsg := tr.lastCommitMessage()
	if lastMsg != "oldest of squash" {
		t.Errorf("expected 'oldest of squash', got %q", lastMsg)
	}
}

// TestCLI_CreatesBackupBranch tests that a backup branch is created
func TestCLI_CreatesBackupBranch(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("a", "b", "c")

	tr.runCLISuccess("-n", "2", "-m", "squashed", "-yes")

	// Check for locsquash/backup-* branch
	out := tr.git(t.Context(), "branch", "-a")
	if !strings.Contains(out, "locsquash/backup-") {
		t.Errorf("expected backup branch to be created, branches: %s", out)
	}
}

// TestCLI_DryRunNoChanges tests that dry-run doesn't modify the repository
func TestCLI_DryRunNoChanges(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("first", "second", "third")

	beforeHead := tr.git(t.Context(), "rev-parse", "HEAD")

	out := tr.runCLISuccess("-n", "2", "-dry-run")

	afterHead := tr.git(t.Context(), "rev-parse", "HEAD")

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
	dir := t.TempDir()

	binary := buildTestBinary(t)

	cmd := exec.CommandContext(t.Context(), binary, "-n", "2") //nolint:gosec
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

	if !strings.Contains(out, "one commit must remain as the base") {
		t.Errorf("expected error about base commit, got: %s", out)
	}
}

// TestCLI_FailsWithSingleCommitRepo tests that repos with only one commit get a clear error
func TestCLI_FailsWithSingleCommitRepo(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("only-commit")

	out := tr.runCLIFailure("-n", "2")

	if !strings.Contains(out, "need at least 2 commits") {
		t.Errorf("expected error about needing 2 commits, got: %s", out)
	}
}

// TestCLI_FailsWithUncommittedChanges tests dirty working directory handling
func TestCLI_FailsWithUncommittedChanges(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("a", "b", "c")

	// Create uncommitted change
	tr.writeFile("dirty.txt", "uncommitted content")
	tr.git(t.Context(), "add", "dirty.txt")

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
	tr.git(t.Context(), "add", "dirty.txt")

	tr.runCLISuccess("-n", "2", "-m", "squashed", "-stash", "-yes")

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
	dateBefore := tr.git(t.Context(), "log", "-1", "--format=%cI")

	tr.runCLISuccess("-n", "2", "-m", "squashed", "-yes")

	// Get date of HEAD after squash
	dateAfter := tr.git(t.Context(), "log", "-1", "--format=%cI")

	if dateBefore != dateAfter {
		t.Errorf("commit date changed: before=%s, after=%s", dateBefore, dateAfter)
	}
}

// TestCLI_RecoveryFromBackup tests that we can recover using the backup branch
func TestCLI_RecoveryFromBackup(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("a", "b", "c", "d")

	headBefore := tr.git(t.Context(), "rev-parse", "HEAD")

	tr.runCLISuccess("-n", "2", "-m", "squashed", "-yes")

	// Find backup branch
	branches := tr.git(t.Context(), "branch", "-a")
	var backupBranch string
	for _, line := range strings.Split(branches, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "locsquash/backup-") {
			backupBranch = strings.TrimPrefix(line, "* ")
			backupBranch = strings.TrimSpace(backupBranch)
			break
		}
	}

	if backupBranch == "" {
		t.Fatal("backup branch not found")
	}

	// Recover
	tr.git(t.Context(), "reset", "--hard", backupBranch)

	headAfter := tr.git(t.Context(), "rev-parse", "HEAD")
	if headBefore != headAfter {
		t.Errorf("recovery failed: before=%s, after=%s", headBefore, headAfter)
	}
}

// TestCLI_MultipleSquashesCreateUniqueBackups tests that each squash creates a unique backup
func TestCLI_MultipleSquashesCreateUniqueBackups(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("1", "2", "3", "4", "5", "6")

	tr.runCLISuccess("-n", "2", "-m", "first squash", "-yes")

	// Need to create more commits for second squash
	tr.createCommitsWithMessages("7", "8")

	tr.runCLISuccess("-n", "2", "-m", "second squash", "-yes")

	// Count backup branches
	branches := tr.git(t.Context(), "branch", "-a")
	backupCount := strings.Count(branches, "locsquash/backup-")

	if backupCount < 2 {
		t.Errorf("expected at least 2 backup branches, found %d in:\n%s", backupCount, branches)
	}
}

// TestCLI_BackupBranchCollision tests that backup branches get unique suffixes
// when the base name already exists (tests branchExists using git show-ref).
// This runs multiple squashes in rapid succession to force collision handling
func TestCLI_BackupBranchCollision(t *testing.T) {
	tr := newTestRepo(t)

	// Create enough commits for 5 squashes (each squash needs 2+ commits, keeping 1)
	tr.createCommitsWithMessages("1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11")

	// Run 5 squashes in rapid succession - within the same second,
	// they should all try the same timestamp-based backup name and trigger collision handling
	for range 5 {
		tr.runCLISuccess("-n", "2", "-m", "squash", "-yes")
	}

	// Verify multiple backup branches exist
	branches := tr.git(t.Context(), "branch", "-a")
	backupCount := strings.Count(branches, "locsquash/backup-")

	if backupCount < 5 {
		t.Errorf("expected at least 5 backup branches, found %d in:\n%s", backupCount, branches)
	}

	// If they ran within the same second, we should see suffixed branches (-2, -3, etc.)
	// Check for at least one suffixed branch to verify collision handling works
	hasSuffix := strings.Contains(branches, "-2") || strings.Contains(branches, "-3")
	if !hasSuffix {
		t.Logf("Note: no suffixed branches found - squashes may have run across different seconds")
	}
}

// TestCLI_EmptySquashFailsWithoutAllowEmpty ensures empty squashes fail by default
func TestCLI_EmptySquashFailsWithoutAllowEmpty(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("base")

	// Create two commits that net to no changes (add then remove the same file)
	tempPath := filepath.Join(tr.Dir, "temp.txt")
	if err := os.WriteFile(tempPath, []byte("temp"), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tr.git(t.Context(), "add", "temp.txt")
	tr.git(t.Context(), "commit", "-m", "add temp")

	if err := os.Remove(tempPath); err != nil {
		t.Fatalf("failed to remove temp file: %v", err)
	}
	tr.git(t.Context(), "add", "-A")
	tr.git(t.Context(), "commit", "-m", "remove temp")

	out := tr.runCLIFailure("-n", "2")
	if !strings.Contains(out, "no net changes") {
		t.Errorf("expected error about no net changes, got: %s", out)
	}
}

// TestCLI_EmptySquashSucceedsWithAllowEmpty ensures empty squashes succeed with -allow-empty
func TestCLI_EmptySquashSucceedsWithAllowEmpty(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("base")

	// Create two commits that cancel out, then allow an empty squashed commit
	tempPath := filepath.Join(tr.Dir, "temp.txt")
	if err := os.WriteFile(tempPath, []byte("temp"), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tr.git(t.Context(), "add", "temp.txt")
	tr.git(t.Context(), "commit", "-m", "add temp")

	if err := os.Remove(tempPath); err != nil {
		t.Fatalf("failed to remove temp file: %v", err)
	}
	tr.git(t.Context(), "add", "-A")
	tr.git(t.Context(), "commit", "-m", "remove temp")

	tr.runCLISuccess("-n", "2", "-m", "squashed", "-allow-empty", "-yes")

	if count := tr.commitCount(); count != 2 {
		t.Errorf("expected 2 commits after squash, got %d", count)
	}
	lastMsg := tr.lastCommitMessage()
	if lastMsg != "squashed" {
		t.Errorf("expected commit message 'squashed', got %q", lastMsg)
	}
}

// TestCLI_NoBackupSkipsBackupBranch tests that -no-backup skips creating backup branch
func TestCLI_NoBackupSkipsBackupBranch(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("a", "b", "c")

	tr.runCLISuccess("-n", "2", "-m", "squashed", "-yes", "-no-backup")

	// Verify squash happened
	if count := tr.commitCount(); count != 2 {
		t.Errorf("expected 2 commits after squash, got %d", count)
	}

	// Verify no backup branch was created
	branches := tr.git(t.Context(), "branch", "-a")
	if strings.Contains(branches, "locsquash/backup-") {
		t.Errorf("expected no backup branch with -no-backup, but found one in: %s", branches)
	}
}

// TestCLI_NoBackupCannotRecoverViaBackup tests that with -no-backup, there's no backup branch to recover from
func TestCLI_NoBackupCannotRecoverViaBackup(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("a", "b", "c", "d")

	headBefore := tr.git(t.Context(), "rev-parse", "HEAD")

	tr.runCLISuccess("-n", "2", "-m", "squashed", "-yes", "-no-backup")

	headAfter := tr.git(t.Context(), "rev-parse", "HEAD")
	if headBefore == headAfter {
		t.Fatal("HEAD should have changed after squash")
	}

	// Verify no backup branch exists - recovery via backup is not possible
	branches := tr.git(t.Context(), "branch", "-a")
	if strings.Contains(branches, "locsquash/backup-") {
		t.Errorf("backup branch should not exist with -no-backup")
	}

	// Recovery would only be possible via reflog (not tested here as it's git internal behavior)
	// Verify we can still recover via reflog
	reflog := tr.git(t.Context(), "reflog", "show", "--format=%H", "-n", "5")
	if !strings.Contains(reflog, headBefore) {
		t.Errorf("original HEAD %s should still be in reflog for recovery", headBefore)
	}
}

// TestCLI_YesShorthandWorks tests that -y works as shorthand for -yes
func TestCLI_YesShorthandWorks(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("a", "b", "c")

	tr.runCLISuccess("-n", "2", "-m", "squashed", "-y")

	if count := tr.commitCount(); count != 2 {
		t.Errorf("expected 2 commits after squash, got %d", count)
	}
}

// TestCLI_DryRunShowsCommitList tests that dry-run displays the commits to be squashed
func TestCLI_DryRunShowsCommitList(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("first commit", "second commit", "third commit")

	out := tr.runCLISuccess("-n", "2", "-dry-run")

	// Should show commit list
	if !strings.Contains(out, "commits will be squashed") {
		t.Errorf("expected commit list in dry-run output, got: %s", out)
	}

	// Should show commit messages
	if !strings.Contains(out, "second commit") || !strings.Contains(out, "third commit") {
		t.Errorf("expected commit messages in dry-run output, got: %s", out)
	}
}

// TestCLI_PrintRecoveryWithNoBackup tests that print-recovery shows warning when -no-backup is used
func TestCLI_PrintRecoveryWithNoBackup(t *testing.T) {
	tr := newTestRepo(t)
	tr.createCommitsWithMessages("a", "b", "c")

	out := tr.runCLISuccess("-n", "2", "-print-recovery", "-no-backup")

	if !strings.Contains(out, "WARNING") || !strings.Contains(out, "reflog") {
		t.Errorf("expected reflog warning in recovery output with -no-backup, got: %s", out)
	}
}
