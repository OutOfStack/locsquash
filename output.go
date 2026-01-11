package main

import "fmt"

// printCommitList displays the commits that will be squashed
func (info SquashInfo) printCommitList() {
	fmt.Printf("The following %d commits will be squashed:\n\n", len(info.Commits))
	for _, c := range info.Commits {
		fmt.Printf("  %s %s\n", c.Hash, c.Subject)
	}
	fmt.Println()
	fmt.Printf("Result commit message: %q\n\n", info.CommitMessage)
}

// printDryRun outputs the planned git commands without executing them
func (info SquashInfo) printDryRun() {
	fmt.Println("Dry run. No changes will be made.")
	fmt.Println()

	info.printCommitList()

	fmt.Println("# Planned operations (copy-paste friendly):")
	fmt.Println()

	if !info.NoBackup {
		fmt.Printf("# Backup branch\n")
		fmt.Printf("git branch %s HEAD\n\n", info.BackupName)
	}

	if info.Dirty && info.AllowStash {
		fmt.Printf("# Stash working tree\n")
		fmt.Printf("git stash push -u -m \"locsquash auto-stash\"\n")
		fmt.Printf("# (stash ref will be: stash@{0})\n\n")
	}

	fmt.Printf("# Rewrite history\n")
	fmt.Printf("git reset --soft %s\n\n", info.ResetRef)

	fmt.Printf("# Create squashed commit\n")
	allowEmptyFlag := ""
	if info.AllowEmpty {
		allowEmptyFlag = " --allow-empty"
	}
	fmt.Printf("GIT_COMMITTER_DATE=%s git commit --date %s%s -m %q\n\n", info.RecentDate, info.RecentDate, allowEmptyFlag, info.CommitMessage)

	if info.Dirty && info.AllowStash {
		fmt.Printf("# Restore working tree\n")
		fmt.Printf("git stash apply stash@{0}\n")
		fmt.Printf("git stash drop stash@{0}\n\n")
	}

	fmt.Println("# End of dry run")
}

// printRecovery outputs instructions for recovering from a failed or unwanted squash
func (info SquashInfo) printRecovery() {
	fmt.Println("# Recovery instructions")
	fmt.Println("# These commands will restore the repository to its pre-run state")
	fmt.Println()

	if info.NoBackup {
		fmt.Println("# WARNING: --no-backup was specified, no backup branch will be created")
		fmt.Println("# Recovery will only be possible via git reflog")
		fmt.Println("# git reflog")
		fmt.Println("# git reset --hard <commit-hash-before-squash>")
	} else {
		fmt.Printf("# Hard reset branch to backup\n")
		fmt.Printf("git reset --hard %s\n\n", info.BackupName)

		fmt.Println("# Optional: delete backup branch after verification")
		fmt.Printf("git branch -D %s\n\n", info.BackupName)
	}

	fmt.Println()
	fmt.Println("# If a stash was involved and conflicts occurred:")
	fmt.Println("# git stash list")
	fmt.Println("# git stash apply <stash-ref>")
	fmt.Println("# git stash drop <stash-ref>")
	fmt.Println()

	fmt.Println("# End of recovery instructions")
}
