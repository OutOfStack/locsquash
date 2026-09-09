package main_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstall_CreatesGitSubcommand(t *testing.T) {
	bin := buildTestBinary(t)
	target := gitSubcommandPath(bin)

	t.Cleanup(func() { _ = os.Remove(target) })

	out, err := runBinary(t, bin, "-install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}

	info, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("expected %s to exist: %v", target, err)
	}

	if runtime.GOOS != "windows" {
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("expected %s to be a symlink, got mode %v", target, info.Mode())
		}
		resolved, rErr := filepath.EvalSymlinks(target)
		if rErr != nil {
			t.Fatalf("failed to resolve symlink: %v", rErr)
		}
		expected, _ := filepath.EvalSymlinks(bin)
		if resolved != expected {
			t.Errorf("symlink resolves to %s, expected %s", resolved, expected)
		}
	}
}

func TestInstall_IdempotentWhenAlreadyInstalled(t *testing.T) {
	bin := buildTestBinary(t)
	target := gitSubcommandPath(bin)
	t.Cleanup(func() { _ = os.Remove(target) })

	if _, err := runBinary(t, bin, "-install"); err != nil {
		t.Fatalf("first install failed: %v", err)
	}

	out, err := runBinary(t, bin, "-install")
	if err != nil {
		t.Fatalf("second install should succeed, got: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Already installed") {
		t.Errorf("expected 'Already installed' message, got: %s", out)
	}
}

func TestInstall_RefusesToOverwriteUnrelatedFile(t *testing.T) {
	bin := buildTestBinary(t)
	target := gitSubcommandPath(bin)
	t.Cleanup(func() { _ = os.Remove(target) })

	if err := os.WriteFile(target, []byte("not locsquash"), 0600); err != nil {
		t.Fatalf("failed to seed target file: %v", err)
	}

	out, err := runBinary(t, bin, "-install")
	if err == nil {
		t.Fatalf("expected install to fail when target exists, got success:\n%s", out)
	}
	if !strings.Contains(out, "already exists") {
		t.Errorf("expected 'already exists' in error, got: %s", out)
	}
}

func TestUninstall_RemovesGitSubcommand(t *testing.T) {
	bin := buildTestBinary(t)
	target := gitSubcommandPath(bin)
	t.Cleanup(func() { _ = os.Remove(target) })

	if _, err := runBinary(t, bin, "-install"); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	if _, err := runBinary(t, bin, "-uninstall"); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}

	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Errorf("expected %s to be removed, lstat err: %v", target, err)
	}
}

func TestUninstall_NoopWhenAbsent(t *testing.T) {
	bin := buildTestBinary(t)
	target := gitSubcommandPath(bin)
	_ = os.Remove(target)

	out, err := runBinary(t, bin, "-uninstall")
	if err != nil {
		t.Fatalf("uninstall of absent file should succeed, got: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Nothing to uninstall") {
		t.Errorf("expected 'Nothing to uninstall' message, got: %s", out)
	}
}

func TestUninstall_RefusesUnrelatedFile(t *testing.T) {
	bin := buildTestBinary(t)
	target := gitSubcommandPath(bin)
	t.Cleanup(func() { _ = os.Remove(target) })

	if err := os.WriteFile(target, []byte("not locsquash"), 0600); err != nil {
		t.Fatalf("failed to seed target file: %v", err)
	}

	out, err := runBinary(t, bin, "-uninstall")
	if err == nil {
		t.Fatalf("expected uninstall to fail for unrelated file, got success:\n%s", out)
	}
	if !strings.Contains(out, "refusing to remove") {
		t.Errorf("expected 'refusing to remove' in error, got: %s", out)
	}
}

// TestInstall_ViaSymlinkedInvocation guards the rule that the shim is installed
// next to the invoked path, not the resolved binary — this is what makes
// `git lsq` discoverable when locsquash sits behind a PATH shim.
func TestInstall_ViaSymlinkedInvocation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink invocation test is unix-only")
	}

	for _, invocation := range []string{"absolute", "relative", "PATH"} {
		t.Run(invocation, func(t *testing.T) {
			bin := buildTestBinary(t)
			shimDir := t.TempDir()
			shim := filepath.Join(shimDir, "locsquash")
			if err := os.Symlink(bin, shim); err != nil {
				t.Fatalf("failed to create shim: %v", err)
			}
			t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

			invokedPath := shim
			switch invocation {
			case "relative":
				invokedPath = "./locsquash"
			case "PATH":
				invokedPath = "locsquash"
			}
			run := func(name string, args ...string) (string, error) {
				t.Helper()
				cmd := exec.CommandContext(t.Context(), name, args...) //nolint:gosec // controlled test invocation
				cmd.Dir = shimDir
				out, err := cmd.CombinedOutput()
				return string(out), err
			}

			shimTarget := filepath.Join(shimDir, "git-lsq")
			realTarget := gitSubcommandPath(bin)
			t.Cleanup(func() { _ = os.Remove(realTarget) })

			if out, err := run(invokedPath, "-install"); err != nil {
				t.Fatalf("install via shim failed: %v\n%s", err, out)
			}
			if _, err := os.Lstat(shimTarget); err != nil {
				t.Errorf("expected git-lsq next to shim at %s, got: %v", shimTarget, err)
			}
			if _, err := os.Lstat(realTarget); err == nil {
				t.Errorf("did not expect git-lsq next to real binary at %s", realTarget)
			}
			if out, err := run("git", "lsq", "-version"); err != nil || !strings.Contains(out, "locsquash ") {
				t.Fatalf("git lsq should invoke the installed shim: %v\n%s", err, out)
			}

			// Uninstalling via the original invocation should also clean it up.
			if out, err := run(invokedPath, "-uninstall"); err != nil {
				t.Fatalf("uninstall via shim failed: %v\n%s", err, out)
			}
			if _, err := os.Lstat(shimTarget); !os.IsNotExist(err) {
				t.Errorf("expected shim-adjacent git-lsq to be removed, got: %v", err)
			}
		})
	}
}

// TestUninstall_RefusesSameSizeDifferentContent is a regression test: an earlier
// version compared by size only, which would have falsely owned this file.
func TestUninstall_RefusesSameSizeDifferentContent(t *testing.T) {
	bin := buildTestBinary(t)
	target := gitSubcommandPath(bin)
	t.Cleanup(func() { _ = os.Remove(target) })

	binInfo, err := os.Stat(bin)
	if err != nil {
		t.Fatalf("stat bin: %v", err)
	}
	// Create a file of exactly the same size, different bytes.
	data := make([]byte, binInfo.Size())
	for i := range data {
		data[i] = 'X'
	}
	if err = os.WriteFile(target, data, 0600); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	out, err := runBinary(t, bin, "-uninstall")
	if err == nil {
		t.Fatalf("expected uninstall to refuse size-colliding file, got success:\n%s", out)
	}
	if !strings.Contains(out, "refusing to remove") {
		t.Errorf("expected 'refusing to remove' in error, got: %s", out)
	}
}

func TestInstall_CustomName(t *testing.T) {
	bin := buildTestBinary(t)
	customTarget := gitSubcommandPathNamed(bin, "squash")
	defaultTarget := gitSubcommandPath(bin)
	t.Cleanup(func() {
		_ = os.Remove(customTarget)
		_ = os.Remove(defaultTarget)
	})

	if _, err := runBinary(t, bin, "-install", "-as", "squash"); err != nil {
		t.Fatalf("install -as squash failed: %v", err)
	}
	if _, err := os.Lstat(customTarget); err != nil {
		t.Errorf("expected %s to exist: %v", customTarget, err)
	}
	if _, err := os.Lstat(defaultTarget); err == nil {
		t.Errorf("did not expect default target %s to be created", defaultTarget)
	}

	// Uninstall without -as must NOT remove the custom shim.
	if _, err := runBinary(t, bin, "-uninstall"); err != nil {
		t.Fatalf("uninstall (default name) should no-op: %v", err)
	}
	if _, err := os.Lstat(customTarget); err != nil {
		t.Errorf("uninstall without -as incorrectly removed custom shim: %v", err)
	}

	if _, err := runBinary(t, bin, "-uninstall", "-as", "squash"); err != nil {
		t.Fatalf("uninstall -as squash failed: %v", err)
	}
	if _, err := os.Lstat(customTarget); !os.IsNotExist(err) {
		t.Errorf("expected %s to be removed, lstat err: %v", customTarget, err)
	}
}

func TestInstall_RejectsInvalidName(t *testing.T) {
	bin := buildTestBinary(t)
	for _, bad := range []string{"", "foo/bar", "foo bar", "../evil", "foo.bar", "-", "-review", "--review"} {
		out, err := runBinary(t, bin, "-install", "-as", bad)
		if err == nil {
			t.Errorf("expected install -as %q to fail, got success:\n%s", bad, out)
		}
	}
}

func gitSubcommandPath(bin string) string {
	return gitSubcommandPathNamed(bin, "lsq")
}

func gitSubcommandPathNamed(bin, name string) string {
	fileName := "git-" + name
	if runtime.GOOS == "windows" {
		fileName += ".exe"
	}
	return filepath.Join(filepath.Dir(bin), fileName)
}

func runBinary(t *testing.T, bin string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), bin, args...) //nolint:gosec
	out, err := cmd.CombinedOutput()
	return string(out), err
}
