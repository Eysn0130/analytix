package filestore

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"

	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
)

type ListDirEntry = filetoolsapp.ListDirEntry

type ListDirResult struct {
	Entries   []ListDirEntry
	Names     []string
	Truncated bool
}

func ListDir(path string, limit int) (ListDirResult, error) {
	return ListDirFiltered(path, limit, nil)
}

func ListDirFiltered(path string, limit int, skip func(name string) bool) (ListDirResult, error) {
	result := ListDirResult{}
	if limit <= 0 {
		limit = 1
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return result, err
	}
	for _, entry := range entries {
		if skip != nil && skip(entry.Name()) {
			continue
		}
		if len(result.Entries) >= limit {
			break
		}
		kind := "file"
		displayName := entry.Name()
		if entry.IsDir() {
			kind = "directory"
			displayName += "/"
		}
		result.Names = append(result.Names, displayName)
		result.Entries = append(result.Entries, ListDirEntry{
			Name:        entry.Name(),
			DisplayName: displayName,
			Kind:        kind,
		})
	}
	visibleCount := 0
	for _, entry := range entries {
		if skip == nil || !skip(entry.Name()) {
			visibleCount++
		}
	}
	result.Truncated = visibleCount > len(result.Entries)
	return result, nil
}

type SearchPathMatch = filetoolsapp.SearchPathMatch

func NormalizeWorkspaceGlobPattern(workspace string, pattern string) (string, bool) {
	pattern = strings.TrimSpace(strings.ReplaceAll(ExpandHomePath(pattern), "\\", "/"))
	if pattern == "" {
		return "", false
	}
	workspace = filepath.Clean(ExpandHomePath(workspace))
	if filepath.IsAbs(filepath.FromSlash(pattern)) || strings.HasPrefix(pattern, "/") {
		rel, err := filepath.Rel(workspace, filepath.Clean(filepath.FromSlash(pattern)))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return "", false
		}
		pattern = filepath.ToSlash(rel)
	}
	return filetoolsapp.NormalizeRequiredGlob(pattern)
}

func NormalizeRelativeSearchGlob(pattern string) (string, bool) {
	if filepath.IsAbs(filepath.FromSlash(strings.TrimSpace(pattern))) || strings.HasPrefix(strings.TrimSpace(pattern), "/") {
		return "", false
	}
	return filetoolsapp.NormalizeOptionalGlob(pattern)
}

type ResolvedGlobPattern struct {
	Root              ResolvedReadPath
	Pattern           string
	NormalizedPattern string
}

func ResolveGlobPattern(workspace string, pattern string, sandboxMode string, readRoots []string) (ResolvedGlobPattern, bool) {
	pattern = strings.TrimSpace(strings.ReplaceAll(ExpandHomePath(pattern), "\\", "/"))
	if pattern == "" {
		return ResolvedGlobPattern{}, false
	}
	if aliasPath, ok := ResolveExternalReadAlias(readRoots, pattern); ok {
		root, relativePattern, splitOK := SplitAbsoluteGlobPattern(aliasPath.Path)
		if !splitOK {
			return ResolvedGlobPattern{}, false
		}
		normalized, globOK := filetoolsapp.NormalizeRequiredGlob(relativePattern)
		if !globOK {
			return ResolvedGlobPattern{}, false
		}
		return ResolvedGlobPattern{
			Root: ResolvedReadPath{
				Path:          root,
				DisplayPath:   aliasPath.DisplayFor(root),
				Root:          aliasPath.Root,
				DisplayRoot:   aliasPath.DisplayRoot,
				ExternalAlias: true,
			},
			Pattern:           pattern,
			NormalizedPattern: normalized,
		}, true
	}
	if LooksLikeExternalReadAlias(pattern) {
		return ResolvedGlobPattern{}, false
	}
	if filepath.IsAbs(filepath.FromSlash(pattern)) || strings.HasPrefix(pattern, "/") {
		root, relativePattern, splitOK := SplitAbsoluteGlobPattern(pattern)
		if !splitOK {
			return ResolvedGlobPattern{}, false
		}
		resolvedRoot, rootOK := ResolveReadPathInfo(workspace, root, sandboxMode, readRoots)
		if !rootOK {
			return ResolvedGlobPattern{}, false
		}
		normalized, globOK := filetoolsapp.NormalizeRequiredGlob(relativePattern)
		if !globOK {
			return ResolvedGlobPattern{}, false
		}
		resolvedRoot.DisplayPath = resolvedRoot.DisplayFor(root)
		resolvedRoot.Path = root
		return ResolvedGlobPattern{Root: resolvedRoot, Pattern: pattern, NormalizedPattern: normalized}, true
	}
	normalized, ok := NormalizeWorkspaceGlobPattern(workspace, pattern)
	if !ok {
		return ResolvedGlobPattern{}, false
	}
	return ResolvedGlobPattern{
		Root: ResolvedReadPath{
			Path:        filepath.Clean(ExpandHomePath(workspace)),
			DisplayPath: filepath.Clean(ExpandHomePath(workspace)),
			Root:        filepath.Clean(ExpandHomePath(workspace)),
			DisplayRoot: filepath.Clean(ExpandHomePath(workspace)),
		},
		Pattern:           pattern,
		NormalizedPattern: normalized,
	}, true
}

func SplitAbsoluteGlobPattern(pattern string) (string, string, bool) {
	pattern = filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(pattern))))
	if pattern == "" || (!filepath.IsAbs(filepath.FromSlash(pattern)) && !strings.HasPrefix(pattern, "/")) {
		return "", "", false
	}
	metaIndex := strings.IndexAny(pattern, "*?[")
	if metaIndex < 0 {
		root := path.Dir(pattern)
		relativePattern := path.Base(pattern)
		return filepath.Clean(filepath.FromSlash(root)), relativePattern, true
	}
	prefix := pattern[:metaIndex]
	slashIndex := strings.LastIndex(prefix, "/")
	if slashIndex < 0 {
		return "", "", false
	}
	root := pattern[:slashIndex]
	if root == "" {
		root = "/"
	}
	relativePattern := strings.TrimPrefix(pattern[slashIndex+1:], "/")
	return filepath.Clean(filepath.FromSlash(root)), relativePattern, true
}

type FindFilesRequest struct {
	Root         string
	Limit        int
	RelativePath func(path string) string
	ProtectedDir func(path string) (string, bool)
	Match        func(relativePath string, name string) bool
}

type FindFilesResult struct {
	Matches          []SearchPathMatch
	SkippedProtected []string
	LimitReached     bool
}

func FindFiles(request FindFilesRequest) (FindFilesResult, error) {
	result := FindFilesResult{}
	if request.Match == nil {
		return result, nil
	}
	limit := request.Limit
	if limit <= 0 {
		limit = 1
	}
	relativePath := request.RelativePath
	if relativePath == nil {
		relativePath = filepath.ToSlash
	}
	err := filepath.WalkDir(request.Root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			if request.ProtectedDir != nil {
				if rel, blocked := request.ProtectedDir(path); blocked {
					result.SkippedProtected = appendUniqueString(result.SkippedProtected, rel)
					return filepath.SkipDir
				}
			}
			return nil
		}
		rel := relativePath(path)
		if request.Match(rel, entry.Name()) {
			result.Matches = append(result.Matches, SearchPathMatch{Path: path, RelativePath: rel})
			if len(result.Matches) >= limit {
				return filepath.SkipAll
			}
		}
		return nil
	})
	result.LimitReached = len(result.Matches) >= limit
	return result, err
}

type GlobFilesRequest struct {
	Context      context.Context
	Root         string
	Limit        int
	RelativePath func(path string) string
	ProtectedDir func(path string) (string, bool)
	SkipDirName  func(name string) bool
	Match        func(relativePath string) bool
}

type GlobFilesResult struct {
	Matches          []SearchPathMatch
	SkippedProtected []string
	SkippedDirs      []string
	LimitReached     bool
}

func GlobFiles(request GlobFilesRequest) (GlobFilesResult, error) {
	result := GlobFilesResult{}
	if request.Match == nil {
		return result, nil
	}
	ctx := request.Context
	if ctx == nil {
		ctx = context.Background()
	}
	limit := request.Limit
	if limit <= 0 {
		limit = 1
	}
	relativePath := request.RelativePath
	if relativePath == nil {
		relativePath = filepath.ToSlash
	}
	skipDirName := request.SkipDirName
	if skipDirName == nil {
		skipDirName = func(string) bool { return false }
	}
	err := filepath.WalkDir(request.Root, func(path string, entry os.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			if path == request.Root {
				return nil
			}
			if request.ProtectedDir != nil {
				if rel, blocked := request.ProtectedDir(path); blocked {
					result.SkippedProtected = appendUniqueString(result.SkippedProtected, rel)
					return filepath.SkipDir
				}
			}
			if skipDirName(entry.Name()) {
				result.SkippedDirs = appendUniqueString(result.SkippedDirs, relativePath(path))
				return filepath.SkipDir
			}
			return nil
		}
		rel := relativePath(path)
		if request.Match(rel) {
			result.Matches = append(result.Matches, SearchPathMatch{Path: path, RelativePath: rel})
			if len(result.Matches) >= limit {
				return filepath.SkipAll
			}
		}
		return nil
	})
	result.LimitReached = len(result.Matches) >= limit
	return result, err
}
