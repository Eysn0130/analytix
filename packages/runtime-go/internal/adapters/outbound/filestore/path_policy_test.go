package filestore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWorkspacePathKeepsAccessInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	resolved, ok := ResolveWorkspacePath(root, "nested/file.txt")
	if !ok {
		t.Fatalf("expected relative workspace path to resolve")
	}
	if resolved != filepath.Join(root, "nested", "file.txt") {
		t.Fatalf("unexpected resolved path: %q", resolved)
	}
	if _, ok := ResolveWorkspacePath(root, "../outside.txt"); ok {
		t.Fatalf("parent escape should be rejected")
	}
	if _, ok := ResolveWorkspacePath("relative-workspace", "file.txt"); ok {
		t.Fatalf("relative workspace roots should be rejected")
	}
}

func TestEffectiveProcessProtectedRootsPreserveMandatoryFloor(t *testing.T) {
	optional := filepath.Join(t.TempDir(), "optional")
	mandatory := filepath.Join(t.TempDir(), "mandatory")
	for _, root := range []string{optional, mandatory} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	configured := []string{
		optional,
		MandatoryProtectedRoot(mandatory),
		MandatoryProtectedRoot(mandatory),
		"relative-root",
	}
	restricted := EffectiveProcessProtectedRoots(configured, "workspace-write")
	if len(restricted) != 3 || restricted[0] != optional || restricted[1] != mandatory || restricted[2] != "relative-root" {
		t.Fatalf("restricted process roots mismatch: %#v", restricted)
	}
	fullAccess := EffectiveProcessProtectedRoots(configured, "danger-full-access")
	if len(fullAccess) != 1 || fullAccess[0] != mandatory {
		t.Fatalf("danger-full-access escaped mandatory process root: %#v", fullAccess)
	}
	mandatoryOnly := MandatoryProcessProtectedRoots(configured)
	if len(mandatoryOnly) != 1 || mandatoryOnly[0] != mandatory {
		t.Fatalf("mandatory process roots mismatch: %#v", mandatoryOnly)
	}
	invalidMandatory := MandatoryProcessProtectedRoots([]string{
		mandatoryProtectedRootPrefix + "relative-mandatory",
	})
	if len(invalidMandatory) != 1 || invalidMandatory[0] != "relative-mandatory" {
		t.Fatalf("invalid mandatory root was silently removed: %#v", invalidMandatory)
	}
}

func TestWorkspaceHostMetadataProtectionMatchesExactComponent(t *testing.T) {
	workspace := t.TempDir()
	metadata := filepath.Join(workspace, workspaceHostMetadataDir)
	if err := os.MkdirAll(metadata, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{metadata, filepath.Join(metadata, "case-project.json")} {
		if root, blocked := PathWithinWorkspaceHostMetadata(workspace, path); !blocked || root == "" {
			t.Fatalf("host metadata path was not protected: path=%s root=%s blocked=%v", path, root, blocked)
		}
	}
	lookalike := filepath.Join(workspace, ".analytixsdd", "plan", "safe.md")
	if _, blocked := PathWithinWorkspaceHostMetadata(workspace, lookalike); blocked || WorkspaceHostMetadataPathOutput(workspace, lookalike) != nil {
		t.Fatalf("lookalike directory was overblocked: %s", lookalike)
	}
	if output := WorkspaceHostMetadataPathOutput(workspace, filepath.Join(metadata, "case-project.json")); output == nil || output["code"] != "host_metadata_protected" {
		t.Fatalf("missing host metadata output: %#v", output)
	}
}

func TestExpandHomePathSupportsPosixAndWindowsSeparators(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("home directory unavailable")
	}
	if got := ExpandHomePath("~/Desktop"); got != filepath.Join(home, "Desktop") {
		t.Fatalf("posix home expansion mismatch: %q", got)
	}
	if got := ExpandHomePath("~\\Desktop"); got != filepath.Join(home, "Desktop") {
		t.Fatalf("windows-style home expansion mismatch: %q", got)
	}
	if got := ExpandHomePath("~other/Desktop"); got != "~other/Desktop" {
		t.Fatalf("non-home tilde prefix should not expand: %q", got)
	}
}

func TestResolveReadPathExpandsHomeBeforeWorkspaceResolution(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("home directory unavailable")
	}
	workspace := t.TempDir()
	expected := filepath.Join(home, "Desktop")
	resolved, ok := ResolveReadPath(workspace, "~/Desktop", "danger-full-access")
	if !ok || resolved != expected {
		t.Fatalf("expected home-relative full access read path %q, got %q ok=%v", expected, resolved, ok)
	}
	if _, ok := ResolveReadPath(workspace, "~/Desktop", "workspace-write"); ok {
		t.Fatalf("workspace-write should reject home-relative paths outside the workspace")
	}
}

func TestResolveReadPathAllowsExplicitExternalPathOnlyInFullAccess(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	resolved, ok := ResolveReadPath(root, outside, "danger-full-access")
	if !ok || resolved != outside {
		t.Fatalf("expected full access read path to allow %q, got %q ok=%v", outside, resolved, ok)
	}
	if _, ok := ResolveReadPath(root, outside, "workspace-write"); ok {
		t.Fatalf("workspace-write read path should reject explicit outside paths")
	}
}

func TestResolveReadPathAllowsParentTraversalOnlyInFullAccess(t *testing.T) {
	base := t.TempDir()
	workspace := filepath.Join(base, "workspace")
	outside := filepath.Join(base, "outside.txt")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	resolved, ok := ResolveReadPath(workspace, "../outside.txt", "danger-full-access")
	if !ok || resolved != outside {
		t.Fatalf("expected parent traversal to resolve under full access, got %q ok=%v", resolved, ok)
	}
	if _, ok := ResolveReadPath(workspace, "../outside.txt", "workspace-write"); ok {
		t.Fatalf("workspace-write read path should reject parent traversal")
	}
}

func TestResolveReadPathAllowsConfiguredReadRoots(t *testing.T) {
	base := t.TempDir()
	workspace := filepath.Join(base, "workspace")
	readRoot := filepath.Join(base, "allowed")
	deniedRoot := filepath.Join(base, "denied")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.MkdirAll(readRoot, 0o755); err != nil {
		t.Fatalf("mkdir read root: %v", err)
	}
	if err := os.MkdirAll(deniedRoot, 0o755); err != nil {
		t.Fatalf("mkdir denied root: %v", err)
	}
	allowed := filepath.Join(readRoot, "note.txt")
	if err := os.WriteFile(allowed, []byte("allowed"), 0o644); err != nil {
		t.Fatalf("write allowed: %v", err)
	}
	denied := filepath.Join(deniedRoot, "note.txt")
	if err := os.WriteFile(denied, []byte("denied"), 0o644); err != nil {
		t.Fatalf("write denied: %v", err)
	}

	resolved, ok := ResolveReadPathWithRoots(workspace, allowed, "workspace-write", []string{readRoot})
	if !ok || resolved != allowed {
		t.Fatalf("expected configured read root to allow %q, got %q ok=%v", allowed, resolved, ok)
	}
	if _, ok := ResolveReadPathWithRoots(workspace, denied, "workspace-write", []string{readRoot}); ok {
		t.Fatalf("path outside workspace/read roots should be rejected")
	}
}

func TestResolveExternalReadAliasMapsConfiguredRoot(t *testing.T) {
	readRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(readRoot, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	alias := ExternalReadRootAlias(readRoot)
	resolved, ok := ResolveReadPathInfo(t.TempDir(), "@"+alias+"/sub/file.txt", "workspace-write", []string{readRoot})
	if !ok {
		t.Fatalf("expected external read alias to resolve")
	}
	expectedRoot, err := WorkspaceRealPath(readRoot)
	if err != nil {
		t.Fatalf("real path: %v", err)
	}
	expected := filepath.Join(expectedRoot, "sub", "file.txt")
	if resolved.Path != expected || resolved.OutputPath() != alias+"/sub/file.txt" || !resolved.ExternalAlias {
		t.Fatalf("unexpected resolved alias: %#v expected path %q", resolved, expected)
	}
	if _, ok := ResolveReadPathInfo(t.TempDir(), alias+"/../escape.txt", "workspace-write", []string{readRoot}); ok {
		t.Fatalf("external read alias must reject parent traversal")
	}
}

func TestResolveExternalReadAliasRejectsSymlinkEscape(t *testing.T) {
	readRoot := t.TempDir()
	outsideRoot := t.TempDir()
	outsideFile := filepath.Join(outsideRoot, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	link := filepath.Join(readRoot, "escape")
	if err := os.Symlink(outsideRoot, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	alias := ExternalReadRootAlias(readRoot)
	if resolved, ok := ResolveExternalReadAlias([]string{readRoot}, alias+"/escape/secret.txt"); ok {
		t.Fatalf("external alias symlink escape should be rejected: %#v", resolved)
	}

	inside := filepath.Join(readRoot, "inside")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatalf("mkdir inside: %v", err)
	}
	insideLink := filepath.Join(readRoot, "inside-link")
	if err := os.Symlink(inside, insideLink); err != nil {
		t.Skipf("inside symlink unavailable: %v", err)
	}
	resolved, ok := ResolveExternalReadAlias([]string{readRoot}, alias+"/inside-link/note.txt")
	if !ok || !PathWithinRoot(readRoot, resolved.Path) {
		t.Fatalf("contained alias symlink should remain allowed: %#v ok=%v", resolved, ok)
	}
}

func TestExternalReadAliasDisplayOutputDoesNotLeakRealRelativePath(t *testing.T) {
	workspace := t.TempDir()
	readRoot := t.TempDir()
	path := filepath.Join(readRoot, "sub", "secret.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	alias := ExternalReadRootAlias(readRoot)
	resolved, ok := ResolveReadPathInfo(workspace, alias+"/sub/secret.txt", "workspace-write", []string{readRoot})
	if !ok {
		t.Fatalf("expected external read alias to resolve")
	}
	output := ProtectedPathOutput(workspace, path, []string{readRoot})
	if output == nil {
		t.Fatalf("expected protected path output")
	}
	display := resolved.DisplayOutput(output)
	expected := alias + "/sub/secret.txt"
	if display["path"] != expected || display["relative_path"] != expected {
		t.Fatalf("expected tokenized protected output, got %#v", display)
	}
}

func TestResolveGlobPatternAllowsExternalAbsoluteRoots(t *testing.T) {
	workspace := t.TempDir()
	external := t.TempDir()
	pattern := filepath.Join(external, "*.pdf")
	resolved, ok := ResolveGlobPattern(workspace, pattern, "danger-full-access", nil)
	if !ok {
		t.Fatalf("expected full access external glob pattern to resolve")
	}
	if resolved.Root.Path != external || resolved.NormalizedPattern != "*.pdf" {
		t.Fatalf("unexpected resolved glob pattern: %#v", resolved)
	}
	if _, ok := ResolveGlobPattern(workspace, pattern, "workspace-write", nil); ok {
		t.Fatalf("workspace-write should reject external glob pattern without a configured read root")
	}
}

func TestResolveGlobPatternExpandsHomeInFullAccess(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("home directory unavailable")
	}
	workspace := t.TempDir()
	resolved, ok := ResolveGlobPattern(workspace, "~/Desktop/*.pdf", "danger-full-access", nil)
	expectedRoot := filepath.Join(home, "Desktop")
	if !ok || resolved.Root.Path != expectedRoot || resolved.NormalizedPattern != "*.pdf" {
		t.Fatalf("expected home-relative glob root %q, got %#v ok=%v", expectedRoot, resolved, ok)
	}
	if _, ok := ResolveGlobPattern(workspace, "~/Desktop/*.pdf", "workspace-write", nil); ok {
		t.Fatalf("workspace-write should reject home-relative external glob pattern")
	}
}

func TestResolveWritePathForSandboxAllowsExternalPathOnlyInFullAccess(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(t.TempDir(), "outside.txt")
	if _, ok := ResolveWritePath(workspace, target, nil); ok {
		t.Fatalf("plain write path should reject external paths")
	}
	resolved, ok := ResolveWritePathForSandbox(workspace, target, "danger-full-access", nil)
	if !ok || resolved != target {
		t.Fatalf("full access write path should allow %q, got %q ok=%v", target, resolved, ok)
	}
	if _, ok := ResolveWritePathForSandbox(workspace, target, "workspace-write", nil); ok {
		t.Fatalf("workspace-write sandbox should reject external write paths")
	}
}

func TestResolveWritePathForSandboxExpandsHomeInFullAccess(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("home directory unavailable")
	}
	workspace := t.TempDir()
	resolved, ok := ResolveWritePathForSandbox(workspace, "~\\Desktop\\notes.txt", "danger-full-access", nil)
	expected := filepath.Join(home, "Desktop", "notes.txt")
	if !ok || resolved != expected {
		t.Fatalf("expected home-relative full access write path %q, got %q ok=%v", expected, resolved, ok)
	}
	if _, ok := ResolveWritePathForSandbox(workspace, "~\\Desktop\\notes.txt", "workspace-write", nil); ok {
		t.Fatalf("workspace-write should reject home-relative writes outside workspace")
	}
}

func TestResolveWritePathAllowsConfiguredAbsoluteRoots(t *testing.T) {
	workspace := t.TempDir()
	writeRoot := t.TempDir()
	target := filepath.Join(writeRoot, "allowed.txt")
	resolved, ok := ResolveWritePath(workspace, target, []string{writeRoot})
	if !ok || resolved != target {
		t.Fatalf("expected configured write root to allow %q, got %q ok=%v", target, resolved, ok)
	}
	if _, ok := ResolveWritePath(workspace, filepath.Join(t.TempDir(), "blocked.txt"), []string{writeRoot}); ok {
		t.Fatalf("absolute path outside workspace/write roots should be rejected")
	}
}

func TestProtectedPathOutputRedactsProtectedRoot(t *testing.T) {
	workspace := t.TempDir()
	protected := filepath.Join(workspace, "private")
	path := filepath.Join(protected, "secret.txt")
	output := ProtectedPathOutput(workspace, path, []string{protected})
	if output == nil {
		t.Fatalf("expected protected path output")
	}
	if output["code"] != "protected_dir" || output["path"] != path || output["relative_path"] != "private/secret.txt" {
		t.Fatalf("unexpected protected output: %#v", output)
	}
	hash, ok := output["protected_root_hash"].(string)
	if !ok || hash == "" || hash == protected {
		t.Fatalf("protected root should be hashed, got %#v", output["protected_root_hash"])
	}
	if ProtectedPathOutput(workspace, filepath.Join(workspace, "public.txt"), []string{protected}) != nil {
		t.Fatalf("unprotected path should not produce a block output")
	}
}

func TestWorkspaceRelativePathNormalizesForRuntimeToolOutput(t *testing.T) {
	workspace := t.TempDir()
	if got := WorkspaceRelativePath(workspace, filepath.Join(workspace, "dir", "file.txt")); got != "dir/file.txt" {
		t.Fatalf("unexpected relative path: %q", got)
	}
	if got := WorkspaceRelativePath(workspace, workspace); got != "." {
		t.Fatalf("workspace root should be '.', got %q", got)
	}
	if got := WorkspaceRelativePath("", "/tmp/file.txt"); got != "/tmp/file.txt" {
		t.Fatalf("empty workspace should preserve path, got %q", got)
	}
	external := filepath.Join(t.TempDir(), "outside.txt")
	if got := WorkspaceRelativePath(workspace, external); got != external {
		t.Fatalf("external path should stay absolute, got %q", got)
	}
}

func TestNormalizeRealRootsCleansAbsoluteUniqueRoots(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	roots := NormalizeRealRoots([]string{nested, nested, ""})
	if len(roots) != 1 {
		t.Fatalf("expected one unique root, got %#v", roots)
	}
	expected, err := WorkspaceRealPath(nested)
	if err != nil {
		t.Fatalf("real path: %v", err)
	}
	if !filepath.IsAbs(roots[0]) || roots[0] != expected {
		t.Fatalf("unexpected normalized root: %#v", roots)
	}
}
