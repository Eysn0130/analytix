package filetools

import (
	"path"
	"regexp"
	"strings"
)

type LineMatcher func(line string) (bool, int)

func FindMatches(pattern string, relativePath string, name string) bool {
	candidates := []string{toSlash(relativePath), name}
	pattern = strings.ReplaceAll(pattern, "\\", "/")
	for _, candidate := range candidates {
		if ok, _ := path.Match(pattern, candidate); ok {
			return true
		}
		if strings.Contains(candidate, pattern) {
			return true
		}
	}
	if !strings.Contains(pattern, "/") {
		if ok, _ := path.Match(pattern, name); ok {
			return true
		}
	}
	return false
}

func CleanGlobPattern(pattern string) string {
	parts := strings.Split(strings.ReplaceAll(pattern, "\\", "/"), "/")
	cleaned := []string{}
	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			if len(cleaned) > 0 && cleaned[len(cleaned)-1] != ".." {
				cleaned = cleaned[:len(cleaned)-1]
			} else {
				cleaned = append(cleaned, part)
			}
		default:
			cleaned = append(cleaned, part)
		}
	}
	if len(cleaned) == 0 {
		return "."
	}
	return strings.Join(cleaned, "/")
}

func NormalizeRequiredGlob(pattern string) (string, bool) {
	return normalizeRelativeGlob(pattern, false)
}

func NormalizeOptionalGlob(pattern string) (string, bool) {
	return normalizeRelativeGlob(pattern, true)
}

func normalizeRelativeGlob(pattern string, allowEmpty bool) (string, bool) {
	pattern = strings.TrimSpace(strings.ReplaceAll(pattern, "\\", "/"))
	if pattern == "" {
		return "", allowEmpty
	}
	if strings.HasPrefix(pattern, "/") {
		return "", false
	}
	pattern = CleanGlobPattern(pattern)
	if pattern == "." || pattern == "" {
		return "", false
	}
	if pattern == ".." || strings.HasPrefix(pattern, "../") || strings.Contains(pattern, "/../") {
		return "", false
	}
	return strings.TrimPrefix(pattern, "./"), true
}

func GlobMatches(pattern string, relativePath string) bool {
	relativePath = strings.TrimPrefix(toSlash(relativePath), "./")
	if relativePath == "" || relativePath == "." {
		return false
	}
	pattern = toSlash(pattern)
	if strings.Contains(pattern, "**") {
		return GlobDoubleStarMatches(pattern, relativePath)
	}
	if ok, _ := path.Match(pattern, relativePath); ok {
		return true
	}
	if !strings.Contains(pattern, "/") {
		if ok, _ := path.Match(pattern, path.Base(relativePath)); ok {
			return true
		}
	}
	return false
}

func GlobDoubleStarMatches(pattern string, relativePath string) bool {
	pattern = toSlash(pattern)
	relativePath = toSlash(relativePath)
	parts := strings.SplitN(pattern, "**", 2)
	prefix := strings.TrimSuffix(parts[0], "/")
	remaining := relativePath
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
	if ok, _ := path.Match(suffix, remaining); ok {
		return true
	}
	segments := strings.Split(remaining, "/")
	for index := range segments {
		candidate := strings.Join(segments[index:], "/")
		if ok, _ := path.Match(suffix, candidate); ok {
			return true
		}
	}
	if !strings.Contains(suffix, "/") {
		if ok, _ := path.Match(suffix, path.Base(remaining)); ok {
			return true
		}
	}
	return false
}

func NewLineMatcher(pattern string, ignoreCase bool, literal bool) (LineMatcher, error) {
	if literal {
		needle := pattern
		if ignoreCase {
			needle = strings.ToLower(needle)
			return func(line string) (bool, int) {
				index := strings.Index(strings.ToLower(line), needle)
				return index >= 0, index + 1
			}, nil
		}
		return func(line string) (bool, int) {
			index := strings.Index(line, needle)
			return index >= 0, index + 1
		}, nil
	}
	flags := ""
	if ignoreCase {
		flags = "(?i)"
	}
	re, err := regexp.Compile(flags + pattern)
	if err != nil {
		return nil, err
	}
	return func(line string) (bool, int) {
		match := re.FindStringIndex(line)
		if match == nil {
			return false, 0
		}
		return true, match[0] + 1
	}, nil
}

var defaultGlobSkippedDirs = map[string]bool{
	".git":          true,
	".hg":           true,
	".jj":           true,
	".mypy_cache":   true,
	".pytest_cache": true,
	".svn":          true,
	".venv":         true,
	"__pycache__":   true,
	"node_modules":  true,
	"vendor":        true,
}

var defaultGrepSkippedDirs = map[string]bool{
	".git":          true,
	".hg":           true,
	".idea":         true,
	".jj":           true,
	".mypy_cache":   true,
	".next":         true,
	".pytest_cache": true,
	".svn":          true,
	".venv":         true,
	".vscode":       true,
	"__pycache__":   true,
	"build":         true,
	"coverage":      true,
	"dist":          true,
	"node_modules":  true,
	"target":        true,
	"vendor":        true,
}

func GlobSkipDirName(name string) bool {
	return defaultGlobSkippedDirs[name]
}

func GrepSkipDirName(name string) bool {
	return defaultGrepSkippedDirs[name]
}

type SearchPathMatch struct {
	Path         string
	RelativePath string
}

type ListDirEntry struct {
	Name        string
	DisplayName string
	Kind        string
}

type ListDirToolOutputInput struct {
	Path         string
	RelativePath string
	Entries      []ListDirEntry
	Names        []string
	Truncated    bool
	Limit        int
}

type FindToolOutputInput struct {
	Path             string
	RelativePath     string
	Pattern          string
	Matches          []SearchPathMatch
	SkippedProtected []string
	Truncated        bool
	Limit            int
}

type GlobToolOutputInput struct {
	Path              string
	RelativePath      string
	Pattern           string
	NormalizedPattern string
	Matches           []SearchPathMatch
	SkippedProtected  []string
	SkippedDirs       []string
	Truncated         bool
	Limit             int
}

type GrepMatch struct {
	Path          string
	RelativePath  string
	Line          int
	Column        int
	Text          string
	ContextBefore []string
	ContextAfter  []string
}

type GrepToolOutputInput struct {
	Path             string
	RelativePath     string
	Pattern          string
	Glob             string
	IgnoreCase       bool
	Literal          bool
	ContextLines     int
	Matches          []GrepMatch
	SkippedProtected []string
	SkippedDirs      []string
	TimedOut         bool
	Truncated        bool
	Limit            int
}

func BuildListDirToolOutput(input ListDirToolOutputInput) map[string]any {
	entries := make([]map[string]any, 0, len(input.Entries))
	for _, entry := range input.Entries {
		entries = append(entries, map[string]any{
			"name":         entry.Name,
			"display_name": entry.DisplayName,
			"kind":         entry.Kind,
		})
	}
	return map[string]any{
		"path":                input.Path,
		"relative_path":       input.RelativePath,
		"entries":             entries,
		"names":               append([]string(nil), input.Names...),
		"truncated":           input.Truncated,
		"entry_limit_reached": float64(input.Limit),
	}
}

func SearchPathMatchesOutput(matches []SearchPathMatch) []map[string]any {
	out := make([]map[string]any, 0, len(matches))
	for _, match := range matches {
		out = append(out, map[string]any{"path": match.Path, "relative_path": match.RelativePath})
	}
	return out
}

func BuildFindToolOutput(input FindToolOutputInput) map[string]any {
	return map[string]any{
		"path":                 input.Path,
		"relative_path":        input.RelativePath,
		"pattern":              input.Pattern,
		"matches":              SearchPathMatchesOutput(input.Matches),
		"skipped_protected":    append([]string(nil), input.SkippedProtected...),
		"truncated":            input.Truncated,
		"result_limit_reached": float64(input.Limit),
	}
}

func BuildGlobToolOutput(input GlobToolOutputInput) map[string]any {
	return map[string]any{
		"path":                 input.Path,
		"relative_path":        input.RelativePath,
		"pattern":              input.Pattern,
		"normalized_pattern":   input.NormalizedPattern,
		"matches":              SearchPathMatchesOutput(input.Matches),
		"skipped_protected":    append([]string(nil), input.SkippedProtected...),
		"skipped_directories":  append([]string(nil), input.SkippedDirs...),
		"truncated":            input.Truncated,
		"result_limit_reached": float64(input.Limit),
	}
}

func BuildGrepToolOutput(input GrepToolOutputInput) map[string]any {
	matches := make([]map[string]any, 0, len(input.Matches))
	for _, match := range input.Matches {
		item := map[string]any{
			"path":          match.Path,
			"relative_path": match.RelativePath,
			"line":          float64(match.Line),
			"column":        float64(match.Column),
			"text":          match.Text,
		}
		if input.ContextLines > 0 {
			item["context_before"] = append([]string(nil), match.ContextBefore...)
			item["context_after"] = append([]string(nil), match.ContextAfter...)
		}
		matches = append(matches, item)
	}
	return map[string]any{
		"path":                input.Path,
		"relative_path":       input.RelativePath,
		"pattern":             input.Pattern,
		"glob":                input.Glob,
		"ignore_case":         input.IgnoreCase,
		"literal":             input.Literal,
		"context":             float64(input.ContextLines),
		"matches":             matches,
		"skipped_protected":   append([]string(nil), input.SkippedProtected...),
		"skipped_directories": append([]string(nil), input.SkippedDirs...),
		"timed_out":           input.TimedOut,
		"truncated":           input.Truncated || input.TimedOut,
		"match_limit_reached": float64(input.Limit),
	}
}

func HiddenSearchEntry(name string) bool {
	return len(name) > 1 && strings.HasPrefix(name, ".") && name != ".."
}

func toSlash(value string) string {
	return strings.TrimPrefix(strings.ReplaceAll(value, "\\", "/"), "./")
}
