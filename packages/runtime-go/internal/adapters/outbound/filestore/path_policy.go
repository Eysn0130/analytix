package filestore

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const mandatoryProtectedRootPrefix = "analytix-mandatory-protected:"

const workspaceHostMetadataDir = ".analytix"

func ResolveWorkspacePath(workspace string, path string) (string, bool) {
	path = ExpandHomePath(path)
	if strings.TrimSpace(path) == "" {
		return "", false
	}
	base := ExpandHomePath(workspace)
	if base == "" || !filepath.IsAbs(base) {
		return "", false
	}
	base = filepath.Clean(base)
	baseReal, err := WorkspaceRealPath(base)
	if err != nil {
		return "", false
	}
	resolved := path
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(base, path)
	}
	resolved = filepath.Clean(resolved)
	resolvedReal, err := WorkspaceRealPath(resolved)
	if err != nil {
		return "", false
	}
	if !PathWithinRoot(baseReal, resolvedReal) {
		return "", false
	}
	return resolved, true
}

func ResolveReadPath(workspace string, path string, sandboxMode string) (string, bool) {
	return ResolveReadPathWithRoots(workspace, path, sandboxMode, nil)
}

func ResolveReadPathWithRoots(workspace string, path string, sandboxMode string, readRoots []string) (string, bool) {
	if strings.TrimSpace(sandboxMode) == "danger-full-access" {
		return ResolveAnyPath(workspace, path)
	}
	if resolved, ok := ResolveWorkspacePath(workspace, path); ok {
		return resolved, true
	}
	if len(readRoots) == 0 {
		return "", false
	}
	resolved, ok := ResolveAnyPath(workspace, path)
	if !ok {
		return "", false
	}
	resolvedReal, err := WorkspaceRealPath(resolved)
	if err != nil {
		return "", false
	}
	for _, root := range readRoots {
		if PathWithinRoot(root, resolvedReal) {
			return resolved, true
		}
	}
	return "", false
}

func ResolveAnyPath(workspace string, path string) (string, bool) {
	path = ExpandHomePath(path)
	if path == "" {
		return "", false
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), true
	}
	base := ExpandHomePath(workspace)
	if base != "" {
		if !filepath.IsAbs(base) {
			absoluteBase, err := filepath.Abs(base)
			if err != nil {
				return "", false
			}
			base = absoluteBase
		}
		return filepath.Clean(filepath.Join(base, path)), true
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	return filepath.Clean(resolved), true
}

func ResolveWritePath(workspace string, path string, allowWriteRoots []string) (string, bool) {
	if resolved, ok := ResolveWorkspacePath(workspace, path); ok {
		return resolved, true
	}
	path = ExpandHomePath(path)
	if path == "" || !filepath.IsAbs(path) {
		return "", false
	}
	resolved := filepath.Clean(path)
	resolvedReal, err := WorkspaceRealPath(resolved)
	if err != nil {
		return "", false
	}
	for _, root := range allowWriteRoots {
		if PathWithinRoot(root, resolvedReal) {
			return resolved, true
		}
	}
	return "", false
}

func ResolveWritePathForSandbox(workspace string, path string, sandboxMode string, allowWriteRoots []string) (string, bool) {
	if strings.TrimSpace(sandboxMode) == "danger-full-access" {
		return ResolveAnyPath(workspace, path)
	}
	return ResolveWritePath(workspace, path, allowWriteRoots)
}

func ProtectedPathOutput(workspace string, path string, protectedReadDirs []string) map[string]any {
	protectedRoot, blocked := PathWithinProtectedDir(path, protectedReadDirs)
	if !blocked {
		return nil
	}
	return map[string]any{
		"code":                "protected_dir",
		"error":               "path is protected by the local privacy guard",
		"path":                path,
		"relative_path":       WorkspaceRelativePath(workspace, path),
		"protected_root_hash": PathHash(protectedRoot),
	}
}

func MandatoryProtectedPathOutput(workspace string, path string, protectedReadDirs []string) map[string]any {
	if output := WorkspaceHostMetadataPathOutput(workspace, path); output != nil {
		return output
	}
	protectedRoot, blocked := pathWithinProtectedDir(path, protectedReadDirs, true)
	if !blocked {
		return nil
	}
	return map[string]any{
		"code":                "protected_dir",
		"error":               "path is protected by the local privacy guard",
		"path":                path,
		"relative_path":       WorkspaceRelativePath(workspace, path),
		"protected_root_hash": PathHash(protectedRoot),
	}
}

// WorkspaceHostMetadataPathOutput protects the exact app-owned .analytix
// component inside the active workspace. It intentionally does not match
// sibling names such as .analytixsdd. This is a provider tool boundary, not
// the private signature authority for case bindings.
func WorkspaceHostMetadataPathOutput(workspace string, path string) map[string]any {
	protectedRoot, blocked := PathWithinWorkspaceHostMetadata(workspace, path)
	if !blocked {
		return nil
	}
	return map[string]any{
		"code":                "host_metadata_protected",
		"error":               "path is reserved for host-owned Analytix metadata",
		"path":                path,
		"relative_path":       WorkspaceRelativePath(workspace, path),
		"protected_root_hash": PathHash(protectedRoot),
	}
}

func PathWithinWorkspaceHostMetadata(workspace string, path string) (string, bool) {
	workspace = strings.TrimSpace(ExpandHomePath(workspace))
	path = strings.TrimSpace(ExpandHomePath(path))
	if workspace == "" || path == "" {
		return "", false
	}
	workspaceReal, err := WorkspaceRealPath(workspace)
	if err != nil {
		return "", false
	}
	metadataRoot := filepath.Join(workspaceReal, workspaceHostMetadataDir)
	metadataReal, err := WorkspaceRealPath(metadataRoot)
	if err != nil {
		return "", false
	}
	pathReal, err := WorkspaceRealPath(path)
	if err != nil || !PathWithinRoot(metadataReal, pathReal) {
		return "", false
	}
	return metadataReal, true
}

func IsWorkspaceHostMetadataEntry(workspace string, directory string, name string) bool {
	if strings.TrimSpace(name) != workspaceHostMetadataDir {
		return false
	}
	workspaceReal, workspaceErr := WorkspaceRealPath(workspace)
	directoryReal, directoryErr := WorkspaceRealPath(directory)
	return workspaceErr == nil && directoryErr == nil && filepath.Clean(workspaceReal) == filepath.Clean(directoryReal)
}

func MandatoryProtectedRoot(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	return mandatoryProtectedRootPrefix + root
}

// EffectiveProcessProtectedRoots returns the canonical filesystem roots that
// an untrusted child process must not open for this sandbox policy. Mandatory
// roots remain protected even when the turn explicitly uses danger-full-access.
func EffectiveProcessProtectedRoots(protectedReadDirs []string, sandboxMode string) []string {
	mandatoryOnly := strings.TrimSpace(sandboxMode) == "danger-full-access"
	return processProtectedRoots(protectedReadDirs, mandatoryOnly)
}

// MandatoryProcessProtectedRoots returns the canonical private roots that a
// long-lived product-owned child process must never open directly.
func MandatoryProcessProtectedRoots(protectedReadDirs []string) []string {
	return processProtectedRoots(protectedReadDirs, true)
}

func processProtectedRoots(protectedReadDirs []string, mandatoryOnly bool) []string {
	roots := make([]string, 0, len(protectedReadDirs))
	seen := map[string]bool{}
	for _, value := range protectedReadDirs {
		mandatory := strings.HasPrefix(value, mandatoryProtectedRootPrefix)
		if mandatoryOnly && !mandatory {
			continue
		}
		root := strings.TrimSpace(strings.TrimPrefix(value, mandatoryProtectedRootPrefix))
		// Keep malformed selected roots in the result. The process sandbox owns
		// strict root validation and will reject them before exec; silently
		// dropping one here would turn a bad mandatory policy into no policy.
		if root != "" && filepath.IsAbs(root) {
			root = filepath.Clean(root)
			if real, err := WorkspaceRealPath(root); err == nil {
				root = real
			}
		}
		if seen[root] {
			continue
		}
		seen[root] = true
		roots = append(roots, root)
	}
	return roots
}

func PathWithinProtectedDir(path string, protectedReadDirs []string) (string, bool) {
	return pathWithinProtectedDir(path, protectedReadDirs, false)
}

func pathWithinProtectedDir(path string, protectedReadDirs []string, mandatoryOnly bool) (string, bool) {
	if len(protectedReadDirs) == 0 {
		return "", false
	}
	resolvedReal, err := WorkspaceRealPath(path)
	if err != nil {
		return "", false
	}
	for _, root := range protectedReadDirs {
		mandatory := strings.HasPrefix(root, mandatoryProtectedRootPrefix)
		if mandatoryOnly && !mandatory {
			continue
		}
		root = strings.TrimPrefix(root, mandatoryProtectedRootPrefix)
		if PathWithinRoot(root, resolvedReal) {
			return root, true
		}
	}
	return "", false
}

func PathWithinRoot(root string, path string) bool {
	root = ExpandHomePath(root)
	path = ExpandHomePath(path)
	if root == "" || path == "" {
		return false
	}
	var err error
	root, err = WorkspaceRealPath(root)
	if err != nil {
		return false
	}
	path, err = WorkspaceRealPath(path)
	if err != nil {
		return false
	}
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

func PathHash(path string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	return hex.EncodeToString(sum[:])
}

func DefaultProtectedReadDirs() []string {
	if runtime.GOOS != "darwin" {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return nil
	}
	return []string{
		filepath.Join(home, "Music"),
		filepath.Join(home, "Pictures"),
		filepath.Join(home, "Movies"),
		filepath.Join(home, "Library"),
	}
}

func NormalizeRealRoots(roots []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, root := range roots {
		root = ExpandHomePath(root)
		if root == "" {
			continue
		}
		if !filepath.IsAbs(root) {
			if absolute, err := filepath.Abs(root); err == nil {
				root = absolute
			}
		}
		if real, err := WorkspaceRealPath(root); err == nil {
			root = real
		} else {
			root = filepath.Clean(root)
		}
		if !seen[root] {
			seen[root] = true
			out = append(out, root)
		}
	}
	return out
}

func WorkspaceRealPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	tail := ""
	current := abs
	for {
		if real, err := filepath.EvalSymlinks(current); err == nil {
			return filepath.Join(real, tail), nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return abs, nil
		}
		tail = filepath.Join(filepath.Base(current), tail)
		current = parent
	}
}

func WorkspaceRelativePath(workspace string, path string) string {
	base := ExpandHomePath(workspace)
	path = ExpandHomePath(path)
	if base == "" {
		return path
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	if rel == "." {
		return "."
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return path
	}
	return filepath.ToSlash(rel)
}

func ExpandHomePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, "~\\") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return path
	}
	if path == "~" {
		return filepath.Clean(home)
	}
	subpath := strings.ReplaceAll(path[2:], "\\", "/")
	if subpath == "" {
		return filepath.Clean(home)
	}
	return filepath.Join(home, filepath.FromSlash(subpath))
}
