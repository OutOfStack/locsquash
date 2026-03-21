package main

// UserInput holds CLI flags provided by the user
type UserInput struct {
	SquashCount   int    // Number of recent commits to squash
	From          string // Oldest commit in squash range: HEAD~N integer offset or commit hash/ref
	NewMessage    string // Custom commit message
	AllowStash    bool   // Auto-stash uncommitted changes before squashing
	AllowEmpty    bool   // Allow empty commits if squashed changes cancel out
	DryRun        bool   // Print planned commands without executing
	PrintRecovery bool   // Print recovery instructions and exit
	NoBackup      bool   // Skip creating backup branch
	Yes           bool   // Skip confirmation prompt
	ListBackups   bool   // List all backup branches and exit
}

// CommitInfo holds information about a single commit
type CommitInfo struct {
	Hash    string // Short commit hash
	Subject string // First line of commit message
}

// SquashInfo extends UserInput with computed values relevant to the squash operation
type SquashInfo struct {
	UserInput
	BackupName    string       // Name of the backup branch created before squashing
	RecentDate    string       // ISO date of the most recent commit
	ResetRef      string       // Git ref to reset to (HEAD~N), relative to position after hard reset
	CommitMessage string       // Final commit message for the squashed commit
	Dirty         bool         // Whether working directory has uncommitted changes
	Commits       []CommitInfo // List of commits that will be squashed
	SkipCount     int          // K: commits above squash range to cherry-pick back
	SkipHashes    []string     // hashes of those K commits (newest first)
}
