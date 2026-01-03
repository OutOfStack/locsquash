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
	if input.SquashCount >= totalCommits {
		fatalf("Error: repository has %d commits; -n must be at most %d (you can't squash the entire history).", totalCommits, totalCommits-1)
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
		ref, sErr := stashPushAndGetRef(ctx)
		if sErr != nil {
			fatalf("Failed to stash changes: %v", sErr)
		}
		stashedRef = ref
		fmt.Printf("Stashed working directory changes as %s\n", stashedRef)
	}

	// Create recovery branch before rewriting history
	createdName, err := createBackupBranch(ctx, info.BackupName)
	if err != nil {
		fatalf("Failed to create backup branch %q: %v", info.BackupName, err)
	}
	info.BackupName = createdName
	fmt.Printf("Created backup branch: %s (recovery point)\n", info.BackupName)

	// Soft reset to HEAD~N
	fmt.Printf("Performing soft reset to %s...\n", info.ResetRef)
	if err = runGitCommand(ctx, "reset", "--soft", info.ResetRef); err != nil {
		fatalf("Failed to perform soft reset: %v\nRecovery: git reset --hard %s", err, info.BackupName)
	}

	// Commit staged changes as one, with date = most recent commit date
	fmt.Println("Creating squashed commit...")
	if err = gitCommitWithDates(ctx, info.RecentDate, info.CommitMessage, info.AllowEmpty); err != nil {
		fatalf("Failed to create squashed commit: %v\nRecovery: git reset --hard %s", err, info.BackupName)
	}

	// Reapply stash if we created one: apply first, then drop only if success
	if stashedRef != "" {
		fmt.Printf("Reapplying stashed changes from %s...\n", stashedRef)
		if err = runGitCommand(ctx, "stash", "apply", stashedRef); err != nil {
			fatalf("Stash apply failed (stash preserved as %s): %v\nRecovery: git reset --hard %s", stashedRef, err, info.BackupName)
		}
		if err = runGitCommand(ctx, "stash", "drop", stashedRef); err != nil {
			fatalf("Applied stash but failed to drop %s: %v\nYou can drop it manually later.\nRecovery: git reset --hard %s", stashedRef, err, info.BackupName)
		}
	}

	fmt.Printf("Successfully squashed the last %d commits.\nBackup branch (optional): %s\n", info.SquashCount, info.BackupName)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
