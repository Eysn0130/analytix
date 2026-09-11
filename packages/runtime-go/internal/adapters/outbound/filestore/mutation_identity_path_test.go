package filestore

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/text/unicode/norm"
)

func TestResolveMutationIdentityPathCollapsesHostFilesystemAliases(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	caseSensitive, known := mutationPathCaseSensitive(root)
	t.Logf("host path semantics: caseSensitive=%v known=%v", caseSensitive, known)
	lowerPath := filepath.Join(root, "effect.txt")
	upperPath := filepath.Join(root, "EFFECT.TXT")
	if err := os.WriteFile(lowerPath, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	lowerInfo, lowerErr := os.Lstat(lowerPath)
	upperInfo, upperErr := os.Lstat(upperPath)
	first, ok := ResolveMutationIdentityPath(root, "effect.txt")
	if !ok {
		t.Fatal("lower-case mutation identity path was rejected")
	}
	second, ok := ResolveMutationIdentityPath(root, upperPath)
	caseAliases := lowerErr == nil && upperErr == nil && os.SameFile(lowerInfo, upperInfo)
	if !ok || (caseAliases && first != second) || (!caseAliases && first == second) {
		t.Fatalf("case semantics diverged from host filesystem: aliases=%v first=%q second=%q", caseAliases, first, second)
	}
	missingLower, ok := ResolveMutationIdentityPath(root, "new-effect.txt")
	if !ok {
		t.Fatal("missing lower-case target was rejected")
	}
	missingUpper, ok := ResolveMutationIdentityPath(root, "NEW-EFFECT.TXT")
	if !ok || (caseSensitive && missingLower == missingUpper) || (!caseSensitive && missingLower != missingUpper) {
		t.Fatalf("future path case semantics diverged: caseSensitive=%v first=%q second=%q", caseSensitive, missingLower, missingUpper)
	}
}

func TestResolveMutationIdentityPathCollapsesHostUnicodeAliases(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	composedName := "caf\u00e9.txt"
	decomposedName := norm.NFD.String(composedName)
	composedPath := filepath.Join(root, composedName)
	if err := os.WriteFile(composedPath, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	composedInfo, composedErr := os.Lstat(composedPath)
	decomposedInfo, decomposedErr := os.Lstat(filepath.Join(root, decomposedName))
	first, ok := ResolveMutationIdentityPath(root, composedName)
	if !ok {
		t.Fatal("composed mutation identity path was rejected")
	}
	second, ok := ResolveMutationIdentityPath(root, decomposedName)
	unicodeAliases := composedErr == nil && decomposedErr == nil && os.SameFile(composedInfo, decomposedInfo)
	if !ok || (unicodeAliases && first != second) || (!unicodeAliases && first == second) {
		t.Fatalf("Unicode semantics diverged from host filesystem: aliases=%v first=%q second=%q", unicodeAliases, first, second)
	}
}

func TestResolveMutationIdentityPathMatchesMacOSSystemAliasExecution(t *testing.T) {
	if runtime.GOOS != "darwin" {
		return
	}
	alias := filepath.Join("/tmp", "analytix-side-effect-identity-missing")
	canonical := filepath.Join("/private/tmp", "analytix-side-effect-identity-missing")
	first, ok := ResolveMutationIdentityPath("/", alias)
	if !ok {
		t.Fatalf("execution-supported macOS system alias was rejected: %s", alias)
	}
	second, ok := ResolveMutationIdentityPath("/", canonical)
	if !ok || first != second {
		t.Fatalf("macOS system alias diverged from execution target: first=%q second=%q", first, second)
	}
}

func TestResolveMutationIdentityPathRejectsSymlinkComponents(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, ok := ResolveMutationIdentityPath(root, filepath.Join(link, "effect.txt")); ok {
		t.Fatal("symlink component was merged with its target identity")
	}
}
