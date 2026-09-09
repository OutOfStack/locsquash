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
