package filestore

import (
	"os"
	"path/filepath"
	"strings"
)

type GitIgnoreMatcher struct {
	repoRoot string
	cache    map[string][]GitIgnoreRule
}

type GitIgnoreRule struct {
	baseDir  string
	pattern  string
	negated  bool
	dirOnly  bool
	anchored bool
	hasSlash bool
}

func NewGitIgnoreMatcher(workspace string, root string) *GitIgnoreMatcher {
	repoRoot := FindGitRepoRoot(firstNonEmptyString(root, workspace))
	if repoRoot == "" {
		return &GitIgnoreMatcher{}
	}
	return &GitIgnoreMatcher{
		repoRoot: repoRoot,
		cache:    map[string][]GitIgnoreRule{},
	}
}

func (matcher *GitIgnoreMatcher) Ignored(path string, isDir bool) bool {
	if matcher == nil || matcher.repoRoot == "" {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = filepath.Clean(path)
	}
	parent := filepath.Dir(abs)
	if parent == "." || parent == "" {
		return false
	}
	rules := matcher.rulesFor(parent)
	ignored := false
	for _, rule := range rules {
		if rule.Matches(abs, isDir) {
			ignored = !rule.negated
		}
	}
	return ignored
}

func (matcher *GitIgnoreMatcher) rulesFor(dir string) []GitIgnoreRule {
	if matcher == nil || matcher.repoRoot == "" {
		return nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = filepath.Clean(dir)
	}
	if !PathUnderOrEqual(matcher.repoRoot, abs) {
		return nil
	}
	if cached, ok := matcher.cache[abs]; ok {
		return cached
	}
	dirs := DirsBetween(matcher.repoRoot, abs)
	rules := []GitIgnoreRule{}
	for _, item := range dirs {
		rules = append(rules, ReadGitIgnoreRules(item)...)
	}
	matcher.cache[abs] = rules
	return rules
}

func ReadGitIgnoreRules(dir string) []GitIgnoreRule {
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		return nil
	}
	rules := []GitIgnoreRule{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, " \t\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		negated := strings.HasPrefix(line, "!")
		if negated {
			line = strings.TrimPrefix(line, "!")
		}
		line = strings.TrimPrefix(line, `\`)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		dirOnly := strings.HasSuffix(line, "/")
		line = strings.TrimSuffix(line, "/")
		anchored := strings.HasPrefix(line, "/")
		line = strings.TrimPrefix(line, "/")
		if line == "" {
			continue
		}
		rules = append(rules, GitIgnoreRule{
			baseDir:  dir,
			pattern:  filepath.ToSlash(line),
			negated:  negated,
			dirOnly:  dirOnly,
			anchored: anchored,
			hasSlash: strings.Contains(line, "/"),
		})
	}
	return rules
}

func (rule GitIgnoreRule) Matches(path string, isDir bool) bool {
	rel, err := filepath.Rel(rule.baseDir, path)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return false
	}
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "./")
	if rel == "" || rel == "." {
		return false
	}
	pattern := strings.TrimPrefix(filepath.ToSlash(rule.pattern), "./")
	if pattern == "" {
		return false
	}
	if rule.anchored || rule.hasSlash {
		return gitIgnoreAnchoredMatch(pattern, rel, rule.dirOnly, isDir)
	}
	segments := strings.Split(rel, "/")
	for i, segment := range segments {
		ok, _ := filepath.Match(filepath.FromSlash(pattern), filepath.FromSlash(segment))
		if !ok {
			continue
		}
		if !rule.dirOnly {
			return true
		}
		if i < len(segments)-1 || isDir {
			return true
		}
	}
	return false
}

func gitIgnoreAnchoredMatch(pattern string, rel string, dirOnly bool, isDir bool) bool {
	if dirOnly {
		if rel == pattern {
			return isDir
		}
		return strings.HasPrefix(rel, pattern+"/")
	}
	if ok, _ := filepath.Match(filepath.FromSlash(pattern), filepath.FromSlash(rel)); ok {
		return true
	}
	if strings.Contains(pattern, "**") {
		return gitIgnoreDoubleStarMatches(pattern, rel)
	}
	return false
}

func FindGitRepoRoot(start string) string {
	if strings.TrimSpace(start) == "" {
		return ""
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		abs = filepath.Clean(start)
	}
	if info, statErr := os.Stat(abs); statErr == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
			return abs
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return ""
		}
		abs = parent
	}
}

func DirsBetween(root string, dir string) []string {
	root, _ = filepath.Abs(root)
	dir, _ = filepath.Abs(dir)
	if !PathUnderOrEqual(root, dir) {
		return nil
	}
	dirs := []string{dir}
	for dirs[len(dirs)-1] != root {
		parent := filepath.Dir(dirs[len(dirs)-1])
		if parent == dirs[len(dirs)-1] {
			break
		}
		dirs = append(dirs, parent)
	}
	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}
	return dirs
}

func PathUnderOrEqual(root string, path string) bool {
	root, _ = filepath.Abs(root)
	path, _ = filepath.Abs(path)
	if path == root {
		return true
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func gitIgnoreDoubleStarMatches(pattern string, relativePath string) bool {
	parts := strings.SplitN(pattern, "**", 2)
	prefix := strings.TrimSuffix(parts[0], "/")
	remaining := strings.TrimPrefix(filepath.ToSlash(relativePath), "./")
	if prefix != "" {
		if remaining != prefix && !strings.HasPrefix(remaining, prefix+"/") {
			return false
		}
		remaining = strings.TrimPrefix(strings.TrimPrefix(remaining, prefix), "/")
	}
	suffix := ""
	if len(parts) > 1 {
		suffix = strings.TrimPrefix(parts[1], "/")
	}
	if suffix == "" {
		return true
	}
	if ok, _ := filepath.Match(filepath.FromSlash(suffix), filepath.FromSlash(remaining)); ok {
		return true
	}
	segments := strings.Split(remaining, "/")
	for index := range segments {
		candidate := strings.Join(segments[index:], "/")
		if ok, _ := filepath.Match(filepath.FromSlash(suffix), filepath.FromSlash(candidate)); ok {
			return true
		}
	}
	if !strings.Contains(suffix, "/") {
		if ok, _ := filepath.Match(suffix, filepath.Base(filepath.FromSlash(remaining))); ok {
			return true
		}
	}
	return false
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
