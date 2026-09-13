package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
)

const defaultSubcommandName = "lsq"

// validSubcommandName limits the suffix to characters git and every major
// shell/filesystem handle safely. Keeps us clear of path separators, globs,
// and other surprises. A leading hyphen would be parsed as a git option.
var validSubcommandName = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_-]*$`)

type targetState int

const (
	targetMissing targetState = iota
	targetIdentical
	targetOurs
	targetForeign
)

// classifyTarget decides how to treat an existing git-<name> entry next to the
// running binary. Hashes/buildinfo are read at most once per file per call.
func classifyTarget(target, self string) targetState {
	info, err := os.Lstat(target)
	if err != nil {
		return targetMissing
	}
	if sameFile(info, target, self) {
		return targetIdentical
	}
	if isStaleShim(info, target, self) {
		return targetOurs
	}
	return targetForeign
}

// isStaleShim reports whether target is an entry a previous locsquash  behind: a copied binary or a symlink whose
// destination is  this module, or a dangling symlink. Anything else is foreign.
func isStaleShim(info os.FileInfo, target, self string) bool {
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(target)
		if err != nil {
			// Dangling git-<name> link next to this binary: the old executable
			// was moved or deleted. Removing or replacing it cannot lose data.
			return true
		}
		return filesHaveSameMainModule(resolved, self)
	}
	return info.Mode().IsRegular() && filesHaveSameMainModule(target, self)
}

// reservedGitCommand reports whether git would dispatch <name> to one of its own commands instead of a git-<name>
// executable on PATH. Built-ins and the scripts shipped in git's exec-path both take precedence over PATH. If git is
// too old to answer, no name is treated as reserved.
func reservedGitCommand(ctx context.Context, name string) bool {
	out, err := gitStdout(ctx, "--list-cmds=builtins,main")
	if err != nil {
		return false
	}
	return slices.Contains(strings.Fields(out), name)
}

func installGitSubcommand(name string) error {
	self, target, err := resolveInstallPaths(name)
	if err != nil {
		return err
	}
	if reservedGitCommand(context.Background(), name) {
		return fmt.Errorf("%q is a built-in git command and would shadow the shim; choose another name with -as", name)
	}

	switch classifyTarget(target, self) {
	case targetIdentical:
		fmt.Printf("Already installed: %s\n", colorize(colorCyan, target))
		return nil
	case targetForeign:
		return fmt.Errorf("%s already exists and points elsewhere; remove it first or run 'locsquash -uninstall -as %s'", target, name)
	case targetOurs:
		if err = replaceInstallTarget(self, target); err != nil {
			return fmt.Errorf("failed to update %s: %w", target, err)
		}
		fmt.Printf("Updated %s -> %s\n", colorize(colorGreen, target), self)
	case targetMissing:
		if err = createInstallTarget(self, target); err != nil {
			return err
		}
		fmt.Printf("Installed %s -> %s\n", colorize(colorGreen, target), self)
	}
	fmt.Printf("You can now run: git %s -n <count>\n", name)
	return nil
}

func uninstallGitSubcommand(name string) error {
	self, target, err := resolveInstallPaths(name)
	if err != nil {
		return err
	}

	switch classifyTarget(target, self) {
	case targetMissing:
		fmt.Printf("Nothing to uninstall: %s does not exist\n", target)
		return nil
	case targetForeign:
		return fmt.Errorf("%s does not point to this locsquash binary; refusing to remove", target)
	case targetIdentical, targetOurs:
		if err = os.Remove(target); err != nil {
			return fmt.Errorf("failed to remove %s: %w", target, err)
		}
		fmt.Printf("Removed %s\n", colorize(colorGreen, target))
	}
	return nil
}

// resolveInstallPaths returns the running binary path as invoked and the
// adjacent git-<name> path. Symlinks are intentionally NOT resolved: if
// locsquash is invoked via a shim on PATH (e.g. ~/bin/locsquash -> /opt/.../locsquash),
// the shim must be installed in the PATH directory, not next to the real binary.
func resolveInstallPaths(name string) (self, target string, err error) {
	if !validSubcommandName.MatchString(name) {
		return "", "", fmt.Errorf("invalid subcommand name %q: only letters, digits, '-' and '_' are allowed, and the name must not start with '-'", name)
	}

	// os.Executable resolves symlinks on some platforms. Look up argv[0] instead, preserving the PATH entry or explicit
	// path used to invoke us. ErrDot means the shell found us through a relative PATH entry; the  returned path is still
	// correct, and filepath.Abs pins it to the cwd.
	self, err = exec.LookPath(os.Args[0])
	if err != nil && !errors.Is(err, exec.ErrDot) {
		return "", "", fmt.Errorf("cannot locate running binary: %w", err)
	}
	self, err = filepath.Abs(self)
	if err != nil {
		return "", "", fmt.Errorf("cannot resolve running binary path: %w", err)
	}

	fileName := "git-" + name
	if runtime.GOOS == "windows" {
		fileName += ".exe"
	}
	target = filepath.Join(filepath.Dir(self), fileName)
	return self, target, nil
}

// sameFile reports whether targetPath is the current git-<name> entry for selfPath.
// Symlinks are fully resolved so a shim chain still matches. Regular files
// are compared by SHA-256 since size alone is not a safe current-version signal.
func sameFile(info os.FileInfo, targetPath, selfPath string) bool {
	if info.Mode()&os.ModeSymlink != 0 {
		tResolved, tErr := filepath.EvalSymlinks(targetPath)
		sResolved, sErr := filepath.EvalSymlinks(selfPath)
		return tErr == nil && sErr == nil && tResolved == sResolved
	}
	if info.Mode().IsRegular() {
		return filesHaveSameContent(targetPath, selfPath)
	}
	return false
}

func createInstallTarget(self, target string) error {
	if runtime.GOOS == "windows" {
		if err := copyFile(self, target); err != nil {
			return fmt.Errorf("failed to copy binary: %w", err)
		}
		return nil
	}

	if err := os.Symlink(self, target); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}
	return nil
}

func replaceInstallTarget(self, target string) error {
	tmp, err := createTempInstallTarget(self, target)
	if err != nil {
		return err
	}
	if err = os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func createTempInstallTarget(self, target string) (string, error) {
	dir := filepath.Dir(target)
	base := "." + filepath.Base(target) + ".tmp-*"
	tmp, err := os.CreateTemp(dir, base)
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	if err = tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err = os.Remove(tmpPath); err != nil {
		return "", err
	}

	if err = createInstallTarget(self, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

func filesHaveSameContent(a, b string) bool {
	aInfo, err := os.Stat(a)
	if err != nil {
		return false
	}
	bInfo, err := os.Stat(b)
	if err != nil {
		return false
	}
	if aInfo.Size() != bInfo.Size() {
		return false
	}

	aHash, err := hashFile(a)
	if err != nil {
		return false
	}
	bHash, err := hashFile(b)
	if err != nil {
		return false
	}
	return bytes.Equal(aHash, bHash)
}

// filesHaveSameMainModule lets us recognize a prior copy of this module on
// Windows without executing the target file. Build info reads a small header
// section, so this is safe and cheap even for stale binaries.
func filesHaveSameMainModule(a, b string) bool {
	aInfo, err := buildinfo.ReadFile(a)
	if err != nil {
		return false
	}
	bInfo, err := buildinfo.ReadFile(b)
	if err != nil {
		return false
	}
	return aInfo.Main.Path != "" && aInfo.Main.Path == bInfo.Main.Path
}

func hashFile(path string) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // path is the invoked executable or adjacent file
	if err != nil {
		return nil, err
	}
	defer func() {
		if cErr := f.Close(); cErr != nil {
			log.Printf("failed to close file %s: %v", path, cErr)
		}
	}()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // src is the current running binary path
	if err != nil {
		return err
	}
	defer func() {
		if cErr := in.Close(); cErr != nil {
			log.Printf("failed to close file %s: %v", src, cErr)
		}
	}()

	srcInfo, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, srcInfo.Mode().Perm()) //nolint:gosec // dst is derived from the invoked executable directory
	if err != nil {
		return err
	}
	defer func() {
		if cErr := out.Close(); cErr != nil {
			log.Printf("failed to close file %s: %v", dst, cErr)
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}
