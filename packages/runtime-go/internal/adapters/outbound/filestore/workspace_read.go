package filestore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"analytix.local/runtime-go/internal/ports/workspaceread"
)

// WorkspaceReadFiles supplies bounded bytes, never authority. Its caller must
// provide a live authorization check for every scan.
type WorkspaceReadFiles struct{ protectedRoots []string }

var _ workspaceread.Files = (*WorkspaceReadFiles)(nil)

func NewWorkspaceReadFiles(protectedRoots []string) *WorkspaceReadFiles {
	return &WorkspaceReadFiles{protectedRoots: append([]string(nil), protectedRoots...)}
}

var workspaceReadSkipDirs = map[string]bool{
	".git": true, ".analytix": true, ".hg": true, ".svn": true,
	"node_modules": true, "dist": true, "out": true, "build": true,
	".next": true, "coverage": true, ".cache": true, ".idea": true,
	".pnpm-store": true, ".turbo": true, ".venv": true, ".vscode": true,
	".yarn": true, ".yarn-cache": true, ".parcel-cache": true,
	"log": true, "logs": true, "target": true, "temp": true,
	"tmp": true, "vendor": true, "venv": true,
}

func workspaceReadKind(name string, includePDF bool) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown", ".mdx", ".txt", ".text":
		return "text"
	case ".pdf":
		if includePDF {
			return "pdf"
		}
	}
	return ""
}

func (f *WorkspaceReadFiles) policy() (string, error) {
	roots := EffectiveProcessProtectedRoots(f.protectedRoots, "workspace-write")
	for _, root := range roots {
		if root == "" || !filepath.IsAbs(root) {
			return "", workspaceread.ErrUnavailable
		}
	}
	sort.Strings(roots)
	skips := make([]string, 0, len(workspaceReadSkipDirs))
	for name := range workspaceReadSkipDirs {
		skips = append(skips, name)
	}
	sort.Strings(skips)
	value, _ := json.Marshal(struct {
		Version     string
		Roots, Skip []string
	}{"workspace-read-v1:text-600000:snapshot-16777216:files-160:entries-8000:utf8:nofollow:single-link", roots, skips})
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:]), nil
}

func (f *WorkspaceReadFiles) protected(workspace, path string) bool {
	return MandatoryProtectedPathOutput(workspace, path, f.protectedRoots) != nil ||
		ProtectedPathOutput(workspace, path, f.protectedRoots) != nil
}
