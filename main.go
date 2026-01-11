// Package main provides the locsquash CLI
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	// Check git installed
	if _, err := exec.LookPath("git"); err != nil {
		fatalf("Error: git is not installed or not found in PATH.")
	}

	var input UserInput
	var showVersion bool

	flag.IntVar(&input.SquashCount, "n", 0, "Number of last commits to squash (must be at least 2)")
	flag.StringVar(&input.NewMessage, "m", "", "New commit message for the squashed commit")
	flag.BoolVar(&input.AllowStash, "stash", false, "Auto-stash uncommitted changes (default requires clean state)")
	flag.BoolVar(&input.AllowEmpty, "allow-empty", false, "Allow creating an empty commit if squashed changes cancel out")
	flag.BoolVar(&input.DryRun, "dry-run", false, "Print the git commands that would run, without making changes")
	flag.BoolVar(&input.PrintRecovery, "print-recovery", false, "Print recovery commands and exit")
	flag.BoolVar(&input.NoBackup, "no-backup", false, "Skip creating backup branch")
	flag.BoolVar(&input.Yes, "yes", false, "Skip confirmation prompt")
	flag.BoolVar(&input.Yes, "y", false, "Skip confirmation prompt (shorthand)")
	flag.BoolVar(&showVersion, "version", false, "Print version and exit")
	flag.BoolVar(&showVersion, "v", false, "Print version and exit (shorthand)")

	flag.Parse()

	if showVersion {
		fmt.Println("locsquash", version)
		os.Exit(0)
	}

	if input.SquashCount < 2 {
		fatalf("Error: -n (Number of last commits to squash) must be at least 2.")
	}

	ctx := context.Background()

	// Check if in git repo
	if err := ensureInsideGitRepo(ctx); err != nil {
		fatalf("Error: %v", err)
	}

	// Check if git has an operation in progress
	if err := ensureNoInProgressOps(ctx); err != nil {
		fatalf("Error: %v", err)
	}

	totalCommits, err := gitCommitCount(ctx)
	if err != nil {
		fatalf("Error retrieving commit count: %v", err)
	}
	if totalCommits < 2 {
		fatalf("Error: repository only has %d commit; need at least 2 commits to squash.", totalCommits)
	}
	if input.SquashCount >= totalCommits {
		fatalf("Error: repository has %d commits; -n must be at most %d (one commit must remain as the base).", totalCommits, totalCommits-1)
	}

	info := SquashInfo{UserInput: input}

	// Check for uncommitted changes
	info.Dirty, err = hasUncommittedChanges(ctx)
	if err != nil {
		fatalf("Error checking git status: %v", err)
	}
	if info.Dirty && !input.AllowStash {
		if input.DryRun || input.PrintRecovery {
			fmt.Fprintln(os.Stderr, "Warning: uncommitted changes detected. Preview may not reflect a clean working tree; use --stash to simulate a clean state.")
		} else {
			fatalf("Error: uncommitted changes detected. Commit/stash them or rerun with --stash.")
		}
	}

	// Compute result commit
	oldestCommitRef := fmt.Sprintf("HEAD~%d", info.SquashCount-1)
	oldestMessage, err := gitLogSingle(ctx, oldestCommitRef, "%B")
	if err != nil {
		fatalf("Failed to retrieve oldest commit message: %v", err)
	}
	oldestMessage = strings.TrimSpace(oldestMessage)

	info.CommitMessage = strings.TrimSpace(info.NewMessage)
	if info.CommitMessage == "" {
		info.CommitMessage = oldestMessage
	}

	recentDate, err := gitLogSingle(ctx, "HEAD", "%cI")
	if err != nil {
		fatalf("Failed to retrieve HEAD commit date: %v", err)
	}
	info.RecentDate = strings.TrimSpace(recentDate)

	info.BackupName = "locsquash/backup-" + time.Now().UTC().Format("20060102-150405")
	info.ResetRef = fmt.Sprintf("HEAD~%d", info.SquashCount)

	hasChanges, err := gitHasChangesBetween(ctx, info.ResetRef, "HEAD")
	if err != nil {
		fatalf("Error checking commit diff: %v", err)
	}
	if !hasChanges && !info.AllowEmpty {
		fatalf("Error: selected commits result in no net changes. Use --allow-empty to create an empty commit.")
	}

	// Retrieve commit list for preview
	info.Commits, err = gitLogCommits(ctx, info.SquashCount)
	if err != nil {
		fatalf("Error retrieving commit list: %v", err)
	}

	if info.DryRun {
		info.printDryRun()
	}

	if info.PrintRecovery {
		info.printRecovery()
	}

	if info.DryRun || info.PrintRecovery {
		return
	}

	// Show commits and prompt for confirmation (unless --yes)
	if !info.Yes {
		info.printCommitList()
		if !promptConfirm() {
			fmt.Println("Aborted.")
			os.Exit(0)
		}
	}

	// Stash if needed
	stashedRef := ""
	if info.Dirty && info.AllowStash {
		ref, sErr := stashPushAndGetRef(ctx)
		if sErr != nil {
			fatalf("Failed to stash changes: %v", sErr)
		}
		stashedRef = ref
		fmt.Printf("Stashed working directory changes as %s\n", stashedRef)
	}

	// Create recovery branch before rewriting history (unless --no-backup)
	if !info.NoBackup {
		createdName, cErr := createBackupBranch(ctx, info.BackupName)
		if cErr != nil {
			fatalf("Failed to create backup branch %q: %v", info.BackupName, cErr)
		}
		info.BackupName = createdName
		fmt.Printf("Created backup branch: %s (recovery point)\n", info.BackupName)
	} else {
		info.BackupName = "" // Clear so recoveryHint knows no backup exists
	}

	// Soft reset to HEAD~N
	fmt.Printf("Performing soft reset to %s...\n", info.ResetRef)
	if err = runGitCommand(ctx, "reset", "--soft", info.ResetRef); err != nil {
		fatalf("Failed to perform soft reset: %v%s", err, recoveryHint(info.BackupName))
	}

	// Commit staged changes as one, with date = most recent commit date
	fmt.Println("Creating squashed commit...")
	if err = gitCommitWithDates(ctx, info.RecentDate, info.CommitMessage, info.AllowEmpty); err != nil {
		fatalf("Failed to create squashed commit: %v%s", err, recoveryHint(info.BackupName))
	}

	// Reapply stash if we created one: apply first, then drop only if success
	if stashedRef != "" {
		fmt.Printf("Reapplying stashed changes from %s...\n", stashedRef)
		if err = runGitCommand(ctx, "stash", "apply", stashedRef); err != nil {
			fatalf("Stash apply failed (stash preserved as %s): %v%s", stashedRef, err, recoveryHint(info.BackupName))
		}
		if err = runGitCommand(ctx, "stash", "drop", stashedRef); err != nil {
			fatalf("Applied stash but failed to drop %s: %v\nYou can drop it manually later.%s", stashedRef, err, recoveryHint(info.BackupName))
		}
	}

	fmt.Printf("Successfully squashed the last %d commits.\n", info.SquashCount)
	if !info.NoBackup {
		fmt.Printf("Backup branch: %s\n", info.BackupName)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// recoveryHint returns a recovery message based on whether backup branch exists
func recoveryHint(backupName string) string {
	if backupName == "" {
		return "\nRecovery: use 'git reflog' to find the commit hash before the squash, then 'git reset --hard <hash>'"
	}
	return "\nRecovery: git reset --hard " + backupName
}

// isTerminal checks if stdin is connected to a terminal
func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// promptConfirm asks the user for confirmation and returns true if they confirm.
// If stdin is not a terminal (e.g., piped input), it aborts with an error.
func promptConfirm() bool {
	if !isTerminal() {
		fatalf("Error: stdin is not a terminal. Use -y to skip confirmation in non-interactive mode.")
	}
	fmt.Print("Proceed? [y/N] ")
	var response string
	if _, err := fmt.Scanln(&response); err != nil {
		return false
	}
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}
