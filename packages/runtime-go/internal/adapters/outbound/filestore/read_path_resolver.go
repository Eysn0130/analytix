package filestore

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
)

const ExternalReadRootAliasPrefix = "__analytix_external_folder"

type ResolvedReadPath struct {
	Path          string
	DisplayPath   string
	Root          string
	DisplayRoot   string
	ExternalAlias bool
}

func ResolveReadPathInfo(workspace string, path string, sandboxMode string, readRoots []string) (ResolvedReadPath, bool) {
	if resolved, ok := ResolveExternalReadAlias(readRoots, path); ok {
		return resolved, true
	}
	if LooksLikeExternalReadAlias(path) {
		return ResolvedReadPath{}, false
	}
	resolved, ok := ResolveReadPathWithRoots(workspace, path, sandboxMode, readRoots)
	if !ok {
		return ResolvedReadPath{}, false
	}
	return ResolvedReadPath{
		Path:        resolved,
		DisplayPath: resolved,
		Root:        resolved,
		DisplayRoot: resolved,
	}, true
}

func LooksLikeExternalReadAlias(path string) bool {
	key := normalizeExternalReadAlias(path)
	return key == ExternalReadRootAliasPrefix || strings.HasPrefix(key, ExternalReadRootAliasPrefix+"/")
}

func ResolveExternalReadAlias(readRoots []string, path string) (ResolvedReadPath, bool) {
	key := normalizeExternalReadAlias(path)
	if key == "" {
		return ResolvedReadPath{}, false
	}
	for _, root := range NormalizeRealRoots(readRoots) {
		token := ExternalReadRootAlias(root)
		if key != token && !strings.HasPrefix(key, token+"/") {
			continue
		}
		sub := "."
		if strings.HasPrefix(key, token+"/") {
			var ok bool
			sub, ok = cleanExternalReadSubpath(strings.TrimPrefix(key, token+"/"))
			if !ok {
				return ResolvedReadPath{}, false
			}
		}
		resolved := root
		display := token
		if sub != "." {
			resolved = filepath.Join(root, filepath.FromSlash(sub))
			display = token + "/" + sub
		}
		resolvedReal, err := WorkspaceRealPath(resolved)
		if err != nil || !PathWithinRoot(root, resolvedReal) {
			return ResolvedReadPath{}, false
		}
		return ResolvedReadPath{
			Path:          filepath.Clean(resolvedReal),
			DisplayPath:   display,
			Root:          root,
			DisplayRoot:   token,
			ExternalAlias: true,
		}, true
	}
	return ResolvedReadPath{}, false
}

func ExternalReadRootAlias(root string) string {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return ""
	}
	if realRoot, err := WorkspaceRealPath(root); err == nil {
		root = filepath.Clean(realRoot)
	}
	sum := sha256.Sum256([]byte(root))
	hash := hex.EncodeToString(sum[:])[:12]
	name := safeExternalReadAliasComponent(filepath.Base(root))
	return ExternalReadRootAliasPrefix + "/" + hash + "/" + name
}

func (p ResolvedReadPath) OutputPath() string {
	if strings.TrimSpace(p.DisplayPath) != "" {
		return p.DisplayPath
	}
	return p.Path
}

func (p ResolvedReadPath) OutputRelativePath(workspace string) string {
	if p.ExternalAlias {
		return p.OutputPath()
	}
	rel := WorkspaceRelativePath(workspace, p.Path)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return p.OutputPath()
	}
	return rel
}

func (p ResolvedReadPath) DisplayFor(path string) string {
	if !p.ExternalAlias {
		return path
	}
	if display, ok := p.displayForCleanPath(path); ok {
		return display
	}
	if realPath, err := WorkspaceRealPath(path); err == nil {
		if display, ok := p.displayForCleanPath(realPath); ok {
			return display
		}
	}
	return path
}

func (p ResolvedReadPath) displayForCleanPath(path string) (string, bool) {
	path = filepath.Clean(path)
	rel, err := filepath.Rel(p.Root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	if rel == "." {
		return p.DisplayRoot, true
	}
	return filepath.ToSlash(filepath.Join(p.DisplayRoot, rel)), true
}

func (p ResolvedReadPath) ErrorText(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if !p.ExternalAlias {
		return message
	}
	return strings.ReplaceAll(message, p.Root, p.DisplayRoot)
}

func (p ResolvedReadPath) DisplayOutput(output map[string]any) map[string]any {
	if !p.ExternalAlias || output == nil {
		return output
	}
	cloned := make(map[string]any, len(output))
	for key, value := range output {
		cloned[key] = value
	}
	actualPath := ""
	if path, ok := cloned["path"].(string); ok && path != "" {
		actualPath = path
		cloned["path"] = p.DisplayFor(path)
	}
	if rel, ok := cloned["relative_path"].(string); ok && rel != "" {
		if actualPath != "" {
			cloned["relative_path"] = p.DisplayFor(actualPath)
		} else if cleanRel, ok := cleanExternalReadSubpath(rel); ok {
			cloned["relative_path"] = p.DisplayFor(filepath.Join(p.Root, filepath.FromSlash(cleanRel)))
		}
	}
	return cloned
}

func normalizeExternalReadAlias(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "@")
	path = filepath.ToSlash(path)
	path = strings.Trim(path, "/")
	return path
}

func cleanExternalReadSubpath(sub string) (string, bool) {
	sub = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(sub)), "/")
	if sub == "" || sub == "." {
		return ".", true
	}
	cleaned := filepath.Clean(filepath.FromSlash(sub))
	if cleaned == "." {
		return ".", true
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || !filepath.IsLocal(cleaned) {
		return "", false
	}
	return filepath.ToSlash(cleaned), true
}

func safeExternalReadAliasComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == string(filepath.Separator) {
		return "root"
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	out := strings.Trim(builder.String(), "._-")
	if out == "" {
		return "root"
	}
	return out
}
