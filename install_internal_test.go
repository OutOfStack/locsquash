package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReplaceInstallTarget_UpdatesOlderCopiedShim(t *testing.T) {
	dir := t.TempDir()
	oldBin := filepath.Join(dir, binaryName("locsquash-old"))
	newBin := filepath.Join(dir, binaryName("locsquash-new"))
	target := filepath.Join(dir, binaryName("git-lsq"))

	buildInstallTestBinary(t, oldBin, "old")
	buildInstallTestBinary(t, newBin, "new")

	if err := copyFile(oldBin, target); err != nil {
		t.Fatalf("copy old shim: %v", err)
	}
	if classifyTarget(target, newBin) != targetOurs {
		t.Fatal("older copied locsquash shim should classify as ours")
	}

	if err := replaceInstallTarget(newBin, target); err != nil {
		t.Fatalf("replace older shim: %v", err)
	}
	if classifyTarget(target, newBin) != targetIdentical {
		t.Fatal("updated shim should classify as identical to the new binary")
	}
}

func buildInstallTestBinary(t *testing.T, out, version string) {
	t.Helper()
	//nolint:gosec // test-only build command with controlled arguments
	cmd := exec.CommandContext(
		context.Background(),
		"go",
		"build",
		"-ldflags",
		"-X main.ldflagsVersion="+version,
		"-o",
		out,
		".",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build %s: %v\n%s", version, err, output)
	}
}

func binaryName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func TestClassifyTarget_RejectsUnrelatedRegularFile(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, binaryName("locsquash"))
	target := filepath.Join(dir, binaryName("git-lsq"))

	buildInstallTestBinary(t, self, "self")
	if err := os.WriteFile(target, []byte("not locsquash"), 0600); err != nil {
		t.Fatalf("write unrelated target: %v", err)
	}

	if got := classifyTarget(target, self); got != targetForeign {
		t.Fatalf("unrelated regular file should classify as foreign, got %v", got)
	}
}

// TestClassifyTarget_StaleSymlinkToOlderBuildIsOurs covers upgrading by placing
// a new locsquash next to an old one: the existing git-lsq symlink still points
// at the old executable and must be recognized as ours so -install can update it.
func TestClassifyTarget_StaleSymlinkToOlderBuildIsOurs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink shims are unix-only")
	}
	dir := t.TempDir()
	oldBin := filepath.Join(dir, "locsquash-old")
	newBin := filepath.Join(dir, "locsquash-new")
	target := filepath.Join(dir, "git-lsq")

	buildInstallTestBinary(t, oldBin, "old")
	buildInstallTestBinary(t, newBin, "new")
	if err := os.Symlink(oldBin, target); err != nil {
		t.Fatalf("symlink old shim: %v", err)
	}

	if got := classifyTarget(target, newBin); got != targetOurs {
		t.Fatalf("symlink to older locsquash build should classify as ours, got %v", got)
	}
	if err := replaceInstallTarget(newBin, target); err != nil {
		t.Fatalf("replace stale symlink: %v", err)
	}
	if got := classifyTarget(target, newBin); got != targetIdentical {
		t.Fatalf("updated symlink should classify as identical, got %v", got)
	}
}

func TestClassifyTarget_DanglingSymlinkIsOurs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink shims are unix-only")
	}
	dir := t.TempDir()
	self := filepath.Join(dir, "locsquash")
	target := filepath.Join(dir, "git-lsq")

	buildInstallTestBinary(t, self, "self")
	if err := os.Symlink(filepath.Join(dir, "locsquash-gone"), target); err != nil {
		t.Fatalf("symlink dangling shim: %v", err)
	}

	if got := classifyTarget(target, self); got != targetOurs {
		t.Fatalf("dangling git-lsq symlink should classify as ours, got %v", got)
	}
}

func TestClassifyTarget_SymlinkToUnrelatedFileIsForeign(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink shims are unix-only")
	}
	dir := t.TempDir()
	self := filepath.Join(dir, "locsquash")
	other := filepath.Join(dir, "other-tool")
	target := filepath.Join(dir, "git-lsq")

	buildInstallTestBinary(t, self, "self")
	if err := os.WriteFile(other, []byte("not locsquash"), 0600); err != nil {
		t.Fatalf("write unrelated file: %v", err)
	}
	if err := os.Symlink(other, target); err != nil {
		t.Fatalf("symlink unrelated shim: %v", err)
	}

	if got := classifyTarget(target, self); got != targetForeign {
		t.Fatalf("symlink to unrelated file should classify as foreign, got %v", got)
	}
}

func TestReservedGitCommand(t *testing.T) {
	ctx := context.Background()
	for name, want := range map[string]bool{
		"status":    true, // compiled built-in
		"submodule": true, // script in git's exec-path, also shadows PATH
		"lsq":       false,
		"squash":    false,
	} {
		if got := reservedGitCommand(ctx, name); got != want {
			t.Errorf("reservedGitCommand(%q) = %v, want %v", name, got, want)
		}
	}
}
