package main

import (
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
		fatal("Error: git is not installed or not found in PATH.")
	}

	var input UserInput
	var showVersion bool

	flag.IntVar(&input.SquashCount, "n", 0, "Number of last commits to squash (must be at least 2)")
	flag.StringVar(&input.NewMessage, "m", "", "New commit message for the squashed commit")
	flag.BoolVar(&input.AllowStash, "stash", false, "Auto-stash uncommitted changes (default requires clean state)")
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
		fatal("Error: -n (Number of last commits to squash) must be at least 2.")
	}

	// Check if in git repo
	if err := ensureInsideGitRepo(); err != nil {
		fatal("Error: %v", err)
	}

	// Check if git has an operation in progress
	if err := ensureNoInProgressOps(); err != nil {
		fatal("Error: %v", err)
	}

	totalCommits, err := gitCommitCount()
	if err != nil {
		fatal("Error retrieving commit count: %v", err)
	}
	if input.SquashCount >= totalCommits {
		fatal("Error: repository has %d commits; -n must be at most %d (you can't squash the entire history).", totalCommits, totalCommits-1)
	}

	info := SquashInfo{UserInput: input}

	// Check for uncommitted changes
	info.Dirty, err = hasUncommittedChanges()
	if err != nil {
		fatal("Error checking git status: %v", err)
	}
	if info.Dirty && !input.AllowStash {
		fatal("Error: uncommitted changes detected. Commit/stash them or rerun with --stash / -st.")
	}

	// Compute result commit
	oldestCommitRef := fmt.Sprintf("HEAD~%d", info.SquashCount-1)
	oldestMessage, err := gitLogSingle(oldestCommitRef, "%B")
	if err != nil {
		fatal("Failed to retrieve oldest commit message: %v", err)
	}
	oldestMessage = strings.TrimSpace(oldestMessage)

	info.CommitMessage = strings.TrimSpace(info.NewMessage)
	if info.CommitMessage == "" {
		info.CommitMessage = oldestMessage
	}

	recentDate, err := gitLogSingle("HEAD", "%cI")
	if err != nil {
		fatal("Failed to retrieve HEAD commit date: %v", err)
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
			fatal("Failed to stash changes: %v", sErr)
		}
		stashedRef = ref
		fmt.Printf("Stashed working directory changes as %s\n", stashedRef)
	}

	// Create recovery branch before rewriting history.
	if err = runGitCommand("branch", info.BackupName, "HEAD"); err != nil {
		fatal("Failed to create backup branch %q: %v", info.BackupName, err)
	}
	fmt.Printf("Created backup branch: %s (recovery point)\n", info.BackupName)

	// Soft reset to HEAD~N
	fmt.Printf("Performing soft reset to %s...\n", info.ResetRef)
	if err = runGitCommand("reset", "--soft", info.ResetRef); err != nil {
		fatal("Failed to perform soft reset: %v\nRecovery: git reset --hard %s", err, info.BackupName)
	}

	// Commit staged changes as one, with date = most recent commit date
	fmt.Println("Creating squashed commit...")
	if err = gitCommitWithDates(info.RecentDate, info.CommitMessage); err != nil {
		fatal("Failed to create squashed commit: %v\nRecovery: git reset --hard %s", err, info.BackupName)
	}

	// Reapply stash if we created one: apply first, then drop only if success
	if stashedRef != "" {
		fmt.Printf("Reapplying stashed changes from %s...\n", stashedRef)
		if err = runGitCommand("stash", "apply", stashedRef); err != nil {
			fatal("Stash apply failed (stash preserved as %s): %v\nRecovery: git reset --hard %s", stashedRef, err, info.BackupName)
		}
		if err = runGitCommand("stash", "drop", stashedRef); err != nil {
			fatal("Applied stash but failed to drop %s: %v\nYou can drop it manually later.\nRecovery: git reset --hard %s", stashedRef, err, info.BackupName)
		}
	}

	fmt.Printf("Successfully squashed the last %d commits.\nBackup branch (optional): %s\n", info.SquashCount, info.BackupName)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
