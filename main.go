package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type UserInput struct {
	SquashCount   int    // Number of recent commits to squash
	NewMessage    string // Custom commit message
	AllowStash    bool   // Auto-stash uncommitted changes before squashing
	DryRun        bool   // Print planned commands without executing
	PrintRecovery bool   // Print recovery instructions and exit
}

type SquashInfo struct {
	UserInput
	BackupName    string // Name of the backup branch created before squashing
	RecentDate    string // ISO date of the most recent commit
	ResetRef      string // Git ref to reset to (HEAD~N)
	CommitMessage string // Final commit message for the squashed commit
	Dirty         bool   // Whether working directory has uncommitted changes
}

func main() {
	// Check git installed
	if _, err := exec.LookPath("git"); err != nil {
		log.Fatal("Error: git is not installed or not found in PATH.")
	}

	var input UserInput

	flag.IntVar(&input.SquashCount, "n", 0, "Number of last commits to squash (must be at least 2)")

	flag.StringVar(&input.NewMessage, "m", "", "New commit message for the squashed commit")

	flag.BoolVar(&input.AllowStash, "stash", false, "Auto-stash uncommitted changes (default requires clean state)")

	flag.BoolVar(&input.DryRun, "dry-run", false, "Print the git commands that would run, without making changes")

	flag.BoolVar(&input.PrintRecovery, "print-recovery", false, "Print recovery commands and exit")

	flag.Parse()

	if input.SquashCount < 2 {
		log.Fatal("Error: -n (Number of last commits to squash) must be at least 2.")
	}

	// Check if in git repo
	if err := ensureInsideGitRepo(); err != nil {
		log.Fatalf("Error: %v", err)
	}

	// Check if git has an operation in progress
	if err := ensureNoInProgressOps(); err != nil {
		log.Fatalf("Error: %v", err)
	}

	totalCommits, err := gitCommitCount()
	if err != nil {
		log.Fatalf("Error retrieving commit count: %v", err)
	}
	if input.SquashCount >= totalCommits {
		log.Fatalf("Error: repository has %d commits; -n must be at most %d (you can't squash the entire history).", totalCommits, totalCommits-1)
	}

	info := SquashInfo{UserInput: input}

	// Check for uncommitted changes
	info.Dirty, err = hasUncommittedChanges()
	if err != nil {
		log.Fatalf("Error checking git status: %v", err)
	}
	if info.Dirty && !input.AllowStash {
		log.Fatal("Error: uncommitted changes detected. Commit/stash them or rerun with --stash / -st.")
	}

	// Compute result commit
	oldestCommitRef := fmt.Sprintf("HEAD~%d", info.SquashCount-1)
	oldestMessage, err := gitLogSingle(oldestCommitRef, "%B")
	if err != nil {
		log.Fatalf("Failed to retrieve oldest commit message: %v", err)
	}
	oldestMessage = strings.TrimSpace(oldestMessage)

	info.CommitMessage = strings.TrimSpace(info.NewMessage)
	if info.CommitMessage == "" {
		info.CommitMessage = oldestMessage
	}

	recentDate, err := gitLogSingle("HEAD", "%cI")
	if err != nil {
		log.Fatalf("Failed to retrieve HEAD commit date: %v", err)
	}
	info.RecentDate = strings.TrimSpace(recentDate)

	info.BackupName = fmt.Sprintf("gosquash/backup-%s", time.Now().UTC().Format("20060102-150405"))
	info.ResetRef = fmt.Sprintf("HEAD~%d", info.SquashCount)

	if info.DryRun {
		info.printDryRun()
	}

	if info.PrintRecovery {
		info.printRecovery()
	}

	if info.DryRun || info.PrintRecovery {
		return
	}

	// Stash if needed
	stashedRef := ""
	if info.Dirty && info.AllowStash {
		ref, sErr := stashPushAndGetRef()
		if sErr != nil {
			log.Fatalf("Failed to stash changes: %v", sErr)
		}
		stashedRef = ref
		fmt.Printf("Stashed working directory changes as %s\n", stashedRef)
	}

	// Create recovery branch before rewriting history.
	if err = runGitCommand("branch", info.BackupName, "HEAD"); err != nil {
		log.Fatalf("Failed to create backup branch %q: %v", info.BackupName, err)
	}
	fmt.Printf("Created backup branch: %s (recovery point)\n", info.BackupName)

	// Soft reset to HEAD~N
	fmt.Printf("Performing soft reset to %s...\n", info.ResetRef)
	if err = runGitCommand("reset", "--soft", info.ResetRef); err != nil {
		log.Fatalf("Failed to perform soft reset: %v\nRecovery: git reset --hard %s", err, info.BackupName)
	}

	// Commit staged changes as one, with date = most recent commit date
	fmt.Println("Creating squashed commit...")
	if err = gitCommitWithDates(info.RecentDate, info.CommitMessage); err != nil {
		log.Fatalf("Failed to create squashed commit: %v\nRecovery: git reset --hard %s", err, info.BackupName)
	}

	// Reapply stash if we created one: apply first, then drop only if success
	if stashedRef != "" {
		fmt.Printf("Reapplying stashed changes from %s...\n", stashedRef)
		if err = runGitCommand("stash", "apply", stashedRef); err != nil {
			log.Fatalf("Stash apply failed (stash preserved as %s): %v\nRecovery: git reset --hard %s", stashedRef, err, info.BackupName)
		}
		if err = runGitCommand("stash", "drop", stashedRef); err != nil {
			log.Fatalf("Applied stash but failed to drop %s: %v\nYou can drop it manually later.\nRecovery: git reset --hard %s", stashedRef, err, info.BackupName)
		}
	}

	fmt.Printf("Successfully squashed the last %d commits.\nBackup branch (optional): %s\n", info.SquashCount, info.BackupName)
}

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

func hasUncommittedChanges() (bool, error) {
	out, err := gitStdout("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

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

func gitLogSingle(ref, formatStr string) (string, error) {
	return gitStdout("log", "-1", "--format="+formatStr, ref)
}

func gitCommitWithDates(isoDate, message string) error {
	cmd := exec.Command("git", "commit", "--date", isoDate, "-m", message)
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE="+isoDate)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runGitCommand(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

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

func (info SquashInfo) printDryRun() {
	fmt.Println("Dry run. No changes will be made.")
	fmt.Println()
	fmt.Println("# Planned operations (copy-paste friendly):")
	fmt.Println()

	fmt.Printf("# Backup branch\n")
	fmt.Printf("git branch %s HEAD\n\n", info.BackupName)

	if info.Dirty && info.AllowStash {
		fmt.Printf("# Stash working tree\n")
		fmt.Printf("git stash push -u -m \"gosquash auto-stash\"\n")
		fmt.Printf("# (stash ref will be: stash@{0})\n\n")
	}

	fmt.Printf("# Rewrite history\n")
	fmt.Printf("git reset --soft %s\n\n", info.ResetRef)

	fmt.Printf("# Create squashed commit\n")
	fmt.Printf("GIT_COMMITTER_DATE=%s git commit --date %s -m %q\n\n", info.RecentDate, info.RecentDate, info.CommitMessage)

	if info.Dirty && info.AllowStash {
		fmt.Printf("# Restore working tree\n")
		fmt.Printf("git stash apply stash@{0}\n")
		fmt.Printf("git stash drop stash@{0}\n\n")
	}

	fmt.Println("# End of dry run")
}

func (info SquashInfo) printRecovery() {
	fmt.Println("# Recovery instructions")
	fmt.Println("# These commands will restore the repository to its pre-run state")
	fmt.Println()

	fmt.Printf("# Hard reset branch to backup\n")
	fmt.Printf("git reset --hard %s\n\n", info.BackupName)

	fmt.Println("# Optional: delete backup branch after verification")
	fmt.Printf("git branch -D %s\n\n", info.BackupName)

	fmt.Println("# If a stash was involved and conflicts occurred:")
	fmt.Println("# git stash list")
	fmt.Println("# git stash apply <stash-ref>")
	fmt.Println("# git stash drop <stash-ref>")
	fmt.Println()

	fmt.Println("# End of recovery instructions")
}
