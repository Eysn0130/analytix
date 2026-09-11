//go:build darwin

package nativecomponentregistry

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDarwinPlatformVerifierChecksExactOpenedSignedBytes(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	rootAuthority, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer rootAuthority.Close()
	workingRoot, workingAuthority := darwinVerifierTestDirectory(t)
	defer workingAuthority.Close()
	policy := PlatformPolicy{SHA256: sha256Hex([]byte("darwin-signing-policy")), Mode: "ad-hoc"}
	verifier, err := NewPlatformVerifier(PlatformVerifierConfig{
		WorkingDirectory: workingRoot, WorkingDirectoryAuthority: workingAuthority,
		StagingRoot: root, StagingRootAuthority: rootAuthority, Policy: policy,
	})
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	componentPath := filepath.Join(t.TempDir(), "analytix-data-engine")
	copyDarwinTestExecutable(t, componentPath)
	resignDarwinTestExecutable(t, componentPath, emptyEntitlements(t), true)
	opened, err := os.Open(componentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	receipt := darwinTestComponentReceipt(t, componentPath, opened)
	if err := verifier.Verify(componentPath, opened, receipt); err != nil {
		t.Fatalf("verify signed opened file: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("verification staging residue = %#v, err=%v", entries, err)
	}

	wrongPath := filepath.Join(t.TempDir(), "analytix-data-engine")
	copyDarwinTestExecutable(t, wrongPath)
	resignDarwinTestExecutable(t, wrongPath, nonEmptyEntitlements(t), true)
	wrong, err := os.Open(wrongPath)
	if err != nil {
		t.Fatal(err)
	}
	defer wrong.Close()
	if err := verifier.Verify(wrongPath, wrong, darwinTestComponentReceipt(t, wrongPath, wrong)); !errors.Is(err, ErrComponentInvalid) {
		t.Fatalf("non-empty entitlement error = %v", err)
	}

	noRuntimePath := filepath.Join(t.TempDir(), "analytix-data-engine")
	copyDarwinTestExecutable(t, noRuntimePath)
	resignDarwinTestExecutable(t, noRuntimePath, emptyEntitlements(t), false)
	noRuntime, err := os.Open(noRuntimePath)
	if err != nil {
		t.Fatal(err)
	}
	defer noRuntime.Close()
	if err := verifier.Verify(noRuntimePath, noRuntime, darwinTestComponentReceipt(t, noRuntimePath, noRuntime)); !errors.Is(err, ErrComponentInvalid) {
		t.Fatalf("missing Hardened Runtime error = %v", err)
	}
}

func TestDarwinPlatformVerifierRejectsStagingRootReplacementAgainstRetainedAuthority(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	authority, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	workingRoot, workingAuthority := darwinVerifierTestDirectory(t)
	defer workingAuthority.Close()
	policy := PlatformPolicy{SHA256: sha256Hex([]byte("darwin-signing-policy")), Mode: "ad-hoc"}
	verifier, err := NewPlatformVerifier(PlatformVerifierConfig{
		WorkingDirectory: workingRoot, WorkingDirectoryAuthority: workingAuthority,
		StagingRoot: root, StagingRootAuthority: authority, Policy: policy,
	})
	if err != nil {
		t.Fatal(err)
	}
	retainedRoot := root + "-retained"
	if err := os.Rename(root, retainedRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(root)
		_ = os.Rename(retainedRoot, root)
	}()
	componentPath := filepath.Join(t.TempDir(), "analytix-data-engine")
	copyDarwinTestExecutable(t, componentPath)
	resignDarwinTestExecutable(t, componentPath, emptyEntitlements(t), true)
	opened, err := os.Open(componentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	receipt := darwinTestComponentReceipt(t, componentPath, opened)
	if err := verifier.Verify(componentPath, opened, receipt); !errors.Is(err, ErrComponentInvalid) {
		t.Fatalf("replaced verification root survived: %v", err)
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatalf("replacement root was mutated: entries=%#v err=%v", entries, err)
	}
}

func darwinVerifierTestDirectory(t *testing.T) (string, *os.File) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	authority, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return root, authority
}

func darwinTestComponentReceipt(t *testing.T, path string, opened *os.File) ComponentReceipt {
	t.Helper()
	info, err := opened.Stat()
	if err != nil {
		t.Fatal(err)
	}
	return ComponentReceipt{
		ID: "data-engine", BinaryName: filepath.Base(path), StagedImageSize: info.Size(),
	}
}

func TestDarwinSignatureMetadataRequiresFixedDeveloperID(t *testing.T) {
	policy := PlatformPolicy{
		SHA256: sha256Hex([]byte("developer-policy")), Mode: "developer-id", AppleTeamIdentifier: "ABCDE12345",
	}
	loadedCDHash := "0123456789abcdef0123456789abcdef01234567"
	valid := strings.Join([]string{
		"Format=pid diskrep",
		"CodeDirectory v=20500 size=100 flags=0x10000(runtime) hashes=1+0 location=embedded",
		"CDHash=" + loadedCDHash,
		"Signature size=10000",
		"Authority=Developer ID Application: Analytix (ABCDE12345)",
		"Authority=Developer ID Certification Authority",
		"Authority=Apple Root CA",
		"TeamIdentifier=ABCDE12345",
		"Timestamp=Jul 16, 2026 at 05:00:00",
	}, "\n")
	if err := validateDarwinSignatureMetadata([]byte(valid), policy, loadedCDHash); err != nil {
		t.Fatalf("valid Developer ID metadata: %v", err)
	}
	for name, mutation := range map[string]string{
		"team":      strings.Replace(valid, "TeamIdentifier=ABCDE12345", "TeamIdentifier=WRONG12345", 1),
		"timestamp": strings.Replace(valid, "Timestamp=Jul 16, 2026 at 05:00:00", "", 1),
		"authority": strings.Replace(valid, "Developer ID Application:", "Self Signed Application:", 1),
		"runtime":   strings.Replace(valid, "0x10000(runtime)", "0x0(none)", 1),
		"format":    strings.Replace(valid, "Format=pid diskrep", "Format=Mach-O thin", 1),
		"cdhash":    strings.Replace(valid, loadedCDHash, strings.Repeat("f", 40), 1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateDarwinSignatureMetadata([]byte(mutation), policy, loadedCDHash); !errors.Is(err, ErrComponentInvalid) {
				t.Fatalf("metadata error = %v", err)
			}
		})
	}
	if !strings.Contains(darwinDeveloperIDRequirement("ABCDE12345"), "anchor apple generic") ||
		!strings.Contains(darwinDeveloperIDRequirement("ABCDE12345"), `leaf[subject.OU] = "ABCDE12345"`) {
		t.Fatal("Developer ID requirement is not pinned")
	}
}

func copyDarwinTestExecutable(t *testing.T, destination string) {
	t.Helper()
	payload, err := os.ReadFile("/usr/bin/true")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, payload, 0o500); err != nil {
		t.Fatal(err)
	}
}

func resignDarwinTestExecutable(t *testing.T, path string, entitlements string, hardened bool) {
	t.Helper()
	arguments := []string{"--force", "--sign", "-", "--timestamp=none"}
	if hardened {
		arguments = append(arguments, "--options", "runtime")
	}
	arguments = append(arguments, "--entitlements", entitlements, path)
	if output, err := exec.Command("/usr/bin/codesign", arguments...).CombinedOutput(); err != nil {
		t.Fatalf("codesign fixture: %v\n%s", err, output)
	}
}

func emptyEntitlements(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "empty.plist")
	writeEntitlements(t, path, "")
	return path
}

func nonEmptyEntitlements(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "non-empty.plist")
	writeEntitlements(t, path, "<key>com.apple.security.cs.allow-jit</key><true/>")
	return path
}

func writeEntitlements(t *testing.T, path string, body string) {
	t.Helper()
	payload := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>` + body + `</dict></plist>
`
	if err := os.WriteFile(path, []byte(payload), 0o400); err != nil {
		t.Fatal(err)
	}
}
