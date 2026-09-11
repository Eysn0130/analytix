package filestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
)

const (
	DefaultGlobMaxResults       = 1000
	DefaultCodeIndexLimit       = 100
	DefaultCodeIndexMaxLimit    = 200
	DefaultCodeIndexMaxFileSize = 1 << 20
	DefaultCodeIndexMaxFiles    = 2000
	DefaultGrepTimeout          = 30 * time.Second
)

type ReadTextToolInput struct {
	Workspace         string
	Args              map[string]any
	ProtectedReadDirs []string
	ReadRoots         []string
	Mode              string
	SandboxMode       string
}

type WorkspaceSearchToolInput struct {
	Context           context.Context
	Workspace         string
	Args              map[string]any
	ProtectedReadDirs []string
	ReadRoots         []string
	SandboxMode       string
}

func ExecuteReadTextTool(input ReadTextToolInput) (map[string]any, string, bool) {
	args := input.Args
	path := strings.TrimSpace(firstNonEmptyAnyString(args["path"], args["filePath"], args["FilePath"]))
	if path == "" {
		return map[string]any{"code": "validation_error", "error": "path is required"}, "", true
	}
	resolved, ok := ResolveReadPathInfo(input.Workspace, path, input.SandboxMode, input.ReadRoots)
	if !ok {
		return readWorkspaceEscapeOutput("path", path, input.Workspace, input.SandboxMode), "", true
	}
	if blocked := protectedPathOutputForSandbox(input.Workspace, resolved.Path, input.ProtectedReadDirs, input.SandboxMode); blocked != nil {
		return resolved.DisplayOutput(blocked), "", true
	}
	textFile, err := ReadTextFile(resolved.Path)
	if errors.Is(err, ErrTextFileIsDirectory) {
		return map[string]any{"code": "read_failed", "error": resolved.OutputPath() + " is a directory; use ls to list it"}, "", true
	} else if errors.Is(err, ErrTextFileBinary) {
		return map[string]any{"code": "binary_file", "error": "read only supports text files", "path": resolved.OutputPath(), "relative_path": resolved.OutputRelativePath(input.Workspace)}, "", true
	} else if err != nil {
		return readPathFailureOutput("read_failed", resolved, path, input.Workspace, err), "", true
	}
	offset, hasOffset := numericAny(args["offset"])
	limit, _ := numericAny(args["limit"])
	view := filetoolsapp.ReadText(textFile.Content, offset, limit, input.Mode, hasOffset)
	output := filetoolsapp.BuildReadTextToolOutput(filetoolsapp.ReadTextToolOutputInput{
		Path:         resolved.OutputPath(),
		RelativePath: resolved.OutputRelativePath(input.Workspace),
		Encoding:     textFile.Encoding,
		View:         view,
	})
	return output, view.RawContent, false
}

func ExecuteListTool(input WorkspaceSearchToolInput) (any, bool) {
	args := input.Args
	rawPath := strings.TrimSpace(firstNonEmptyAnyString(args["path"], "."))
	resolved, ok := ResolveReadPathInfo(input.Workspace, rawPath, input.SandboxMode, input.ReadRoots)
	if !ok {
		return readWorkspaceEscapeOutput("path", rawPath, input.Workspace, input.SandboxMode), true
	}
	if blocked := protectedPathOutputForSandbox(input.Workspace, resolved.Path, input.ProtectedReadDirs, input.SandboxMode); blocked != nil {
		return resolved.DisplayOutput(blocked), true
	}
	limit := filetoolsapp.PositiveLimit(args["limit"], 200, 1000)
	listResult, err := ListDirFiltered(resolved.Path, limit, func(name string) bool {
		return IsWorkspaceHostMetadataEntry(input.Workspace, resolved.Path, name)
	})
	if err != nil {
		return readPathFailureOutput("list_failed", resolved, rawPath, input.Workspace, err), true
	}
	return filetoolsapp.BuildListDirToolOutput(filetoolsapp.ListDirToolOutputInput{
		Path:         resolved.OutputPath(),
		RelativePath: resolved.OutputRelativePath(input.Workspace),
		Entries:      listResult.Entries,
		Names:        listResult.Names,
		Truncated:    listResult.Truncated,
		Limit:        limit,
	}), false
}

func ExecuteFindTool(input WorkspaceSearchToolInput) (any, bool) {
	args := input.Args
	pattern := strings.TrimSpace(firstNonEmptyAnyString(args["pattern"]))
	if pattern == "" {
		return map[string]any{"code": "validation_error", "error": "pattern is required"}, true
	}
	rawPath := strings.TrimSpace(firstNonEmptyAnyString(args["path"], "."))
	root, ok := ResolveReadPathInfo(input.Workspace, rawPath, input.SandboxMode, input.ReadRoots)
	if !ok {
		return readWorkspaceEscapeOutput("path", rawPath, input.Workspace, input.SandboxMode), true
	}
	if blocked := protectedPathOutputForSandbox(input.Workspace, root.Path, input.ProtectedReadDirs, input.SandboxMode); blocked != nil {
		return root.DisplayOutput(blocked), true
	}
	limit := filetoolsapp.PositiveLimit(args["limit"], 200, 1000)
	findResult, err := FindFiles(FindFilesRequest{
		Root:         root.Path,
		Limit:        limit,
		RelativePath: relativeReadPath(input.Workspace, root),
		ProtectedDir: protectedReadDir(input.Workspace, input.ProtectedReadDirs, root),
		Match: func(relativePath string, name string) bool {
			return filetoolsapp.FindMatches(pattern, relativePath, name)
		},
	})
	if err != nil {
		return readPathFailureOutput("find_failed", root, rawPath, input.Workspace, err), true
	}
	return filetoolsapp.BuildFindToolOutput(filetoolsapp.FindToolOutputInput{
		Path:             root.OutputPath(),
		RelativePath:     root.OutputRelativePath(input.Workspace),
		Pattern:          pattern,
		Matches:          displaySearchPathMatches(findResult.Matches, root),
		SkippedProtected: findResult.SkippedProtected,
		Truncated:        findResult.LimitReached,
		Limit:            limit,
	}), false
}

func ExecuteGlobTool(input WorkspaceSearchToolInput) (any, bool) {
	args := input.Args
	pattern := strings.TrimSpace(firstNonEmptyAnyString(args["pattern"]))
	if pattern == "" {
		return map[string]any{"code": "validation_error", "error": "pattern is required"}, true
	}
	resolvedPattern, ok := ResolveGlobPattern(input.Workspace, pattern, input.SandboxMode, input.ReadRoots)
	if !ok {
		return readWorkspaceEscapeOutput("pattern", pattern, input.Workspace, input.SandboxMode), true
	}
	root := resolvedPattern.Root
	if blocked := protectedPathOutputForSandbox(input.Workspace, root.Path, input.ProtectedReadDirs, input.SandboxMode); blocked != nil {
		return root.DisplayOutput(blocked), true
	}
	limit := filetoolsapp.PositiveLimit(args["limit"], 200, DefaultGlobMaxResults)
	globResult, err := GlobFiles(GlobFilesRequest{
		Context:      input.Context,
		Root:         root.Path,
		Limit:        limit,
		RelativePath: relativeReadPath(input.Workspace, root),
		ProtectedDir: protectedReadDir(input.Workspace, input.ProtectedReadDirs, root),
		SkipDirName:  filetoolsapp.GlobSkipDirName,
		Match: func(relativePath string) bool {
			return filetoolsapp.GlobMatches(resolvedPattern.NormalizedPattern, relativePath)
		},
	})
	if err != nil {
		return map[string]any{"code": "glob_failed", "error": root.ErrorText(err)}, true
	}
	return filetoolsapp.BuildGlobToolOutput(filetoolsapp.GlobToolOutputInput{
		Path:              root.OutputPath(),
		RelativePath:      root.OutputRelativePath(input.Workspace),
		Pattern:           pattern,
		NormalizedPattern: resolvedPattern.NormalizedPattern,
		Matches:           displaySearchPathMatches(globResult.Matches, root),
		SkippedProtected:  globResult.SkippedProtected,
		SkippedDirs:       globResult.SkippedDirs,
		Truncated:         globResult.LimitReached,
		Limit:             limit,
	}), false
}

func ExecuteCodeIndexTool(input WorkspaceSearchToolInput) (any, bool) {
	args := input.Args
	request, err := filetoolsapp.ParseCodeIndexRequest(args, DefaultCodeIndexLimit, DefaultCodeIndexMaxLimit)
	if err != nil {
		return filetoolsapp.ToolErrorOutput(err), true
	}
	root, ok := ResolveReadPathInfo(input.Workspace, request.Path, input.SandboxMode, input.ReadRoots)
	if !ok {
		return readWorkspaceEscapeOutput("path", request.Path, input.Workspace, input.SandboxMode), true
	}
	if blocked := protectedPathOutputForSandbox(input.Workspace, root.Path, input.ProtectedReadDirs, input.SandboxMode); blocked != nil {
		return root.DisplayOutput(blocked), true
	}
	collectionLimit := request.Limit
	if filetoolsapp.CodeIndexHasFilter(request) {
		collectionLimit = 0
	}
	scanResult, err := CodeIndexScan(CodeIndexScanRequest{
		Context:      input.Context,
		Workspace:    input.Workspace,
		Root:         root.Path,
		Limit:        collectionLimit,
		MaxFiles:     DefaultCodeIndexMaxFiles,
		MaxFileBytes: DefaultCodeIndexMaxFileSize,
		Outline:      request.Action == "outline",
		RelativePath: relativeReadPath(input.Workspace, root),
		ProtectedDir: protectedReadDir(input.Workspace, input.ProtectedReadDirs, root),
	})
	if err != nil {
		return map[string]any{"code": "code_index_failed", "error": root.ErrorText(err), "path": root.OutputPath(), "relative_path": root.OutputRelativePath(input.Workspace)}, true
	}
	return filetoolsapp.BuildCodeIndexToolOutput(filetoolsapp.CodeIndexToolOutputInput{
		Request:          request,
		Path:             root.OutputPath(),
		RelativePath:     root.OutputRelativePath(input.Workspace),
		Symbols:          displayCodeIndexSymbols(scanResult.Symbols, root),
		SkippedProtected: scanResult.SkippedProtected,
		SkippedDirs:      scanResult.SkippedDirs,
		Truncated:        scanResult.Truncated,
	}), false
}

func ExecuteGrepTool(input WorkspaceSearchToolInput) (any, bool) {
	args := input.Args
	pattern := firstNonEmptyAnyString(args["pattern"])
	if strings.TrimSpace(pattern) == "" {
		return map[string]any{"code": "validation_error", "error": "pattern is required"}, true
	}
	rawPath := strings.TrimSpace(firstNonEmptyAnyString(args["path"], "."))
	root, ok := ResolveReadPathInfo(input.Workspace, rawPath, input.SandboxMode, input.ReadRoots)
	if !ok {
		return readWorkspaceEscapeOutput("path", rawPath, input.Workspace, input.SandboxMode), true
	}
	if blocked := protectedPathOutputForSandbox(input.Workspace, root.Path, input.ProtectedReadDirs, input.SandboxMode); blocked != nil {
		return root.DisplayOutput(blocked), true
	}
	limit := filetoolsapp.PositiveLimit(args["limit"], 100, 500)
	ignoreCase := boolField(args, "ignoreCase")
	literal := boolField(args, "literal")
	glob := strings.TrimSpace(firstNonEmptyAnyString(args["glob"]))
	globPattern, globOK := NormalizeRelativeSearchGlob(glob)
	if !globOK {
		return map[string]any{"code": "validation_error", "error": "glob must be a relative pattern"}, true
	}
	contextLines := filetoolsapp.BoundedNonNegativeInteger(args["context"], 0, 20)
	lineMatcher, err := filetoolsapp.NewLineMatcher(pattern, ignoreCase, literal)
	if err != nil {
		return map[string]any{"code": "invalid_pattern", "error": err.Error()}, true
	}
	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}
	grepCtx, cancel := context.WithTimeout(ctx, DefaultGrepTimeout)
	defer cancel()
	grepResult, err := GrepScan(grepCtx, GrepScanRequest{
		Workspace:    input.Workspace,
		Root:         root.Path,
		Limit:        limit,
		ContextLines: contextLines,
		Match:        func(line string) (bool, int) { return lineMatcher(line) },
		IncludePath: func(relativePath string) bool {
			return globPattern == "" || filetoolsapp.GlobMatches(globPattern, relativePath)
		},
		RelativePath:    relativeReadPath(input.Workspace, root),
		ProtectedDir:    protectedReadDir(input.Workspace, input.ProtectedReadDirs, root),
		SkipDirName:     filetoolsapp.GrepSkipDirName,
		HiddenEntryName: filetoolsapp.HiddenSearchEntry,
		LooksUTF16Text:  filetoolsapp.LooksUTF16Text,
		DecodeText: func(data []byte) (string, bool) {
			content, _, ok := filetoolsapp.DecodeTextBytes(data)
			return content, ok
		},
		SplitTextLines: func(content string) []string {
			return filetoolsapp.SplitTextLines(content, false)
		},
		GitIgnoreMatcher: NewGitIgnoreMatcher(input.Workspace, root.Path),
	})
	if err != nil {
		return map[string]any{"code": "grep_failed", "error": root.ErrorText(err)}, true
	}
	timedOut := grepCtx.Err() == context.DeadlineExceeded
	return filetoolsapp.BuildGrepToolOutput(filetoolsapp.GrepToolOutputInput{
		Path:             root.OutputPath(),
		RelativePath:     root.OutputRelativePath(input.Workspace),
		Pattern:          pattern,
		Matches:          displayGrepMatches(grepResult.Matches, root),
		SkippedProtected: grepResult.SkippedProtected,
		SkippedDirs:      grepResult.SkippedDirs,
		TimedOut:         timedOut,
		Truncated:        grepResult.LimitReached,
		Limit:            limit,
		IgnoreCase:       ignoreCase,
		Literal:          literal,
		Glob:             glob,
		ContextLines:     contextLines,
	}), false
}

func readPathFailureOutput(code string, resolved ResolvedReadPath, requestedPath string, workspace string, err error) map[string]any {
	out := map[string]any{"code": code, "error": resolved.ErrorText(err)}
	if outputPath := resolved.OutputPath(); outputPath != "" {
		out["path"] = outputPath
	}
	if rel := resolved.OutputRelativePath(workspace); rel != "" {
		out["relative_path"] = rel
	}
	if suggestion, ok := desktopPathSuggestion(requestedPath, resolved.Path); ok {
		out["suggested_path"] = suggestion
		out["hint"] = "The requested Desktop path appears to belong to a different macOS user. Use the current user's Desktop path or ~/Desktop."
	}
	return out
}

func desktopPathSuggestion(requestedPath string, resolvedPath string) (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", false
	}
	home = filepath.Clean(home)
	currentUser := filepath.Base(home)
	path := strings.TrimSpace(requestedPath)
	if path == "" {
		path = resolvedPath
	}
	requested := filepath.Clean(ExpandHomePath(path))
	parts := strings.Split(filepath.ToSlash(requested), "/")
	if len(parts) < 4 || parts[0] != "" || parts[1] != "Users" || parts[2] == "" || parts[2] == currentUser {
		return "", false
	}
	desktopName := parts[3]
	if desktopName != "Desktop" && desktopName != "桌面" {
		return "", false
	}
	suggestion := filepath.Join(home, desktopName)
	if desktopName == "桌面" {
		if _, statErr := os.Stat(suggestion); statErr != nil {
			suggestion = filepath.Join(home, "Desktop")
		}
	}
	for _, part := range parts[4:] {
		suggestion = filepath.Join(suggestion, filepath.FromSlash(part))
	}
	return suggestion, true
}

func relativeWorkspacePath(workspace string) func(path string) string {
	return func(path string) string {
		return WorkspaceRelativePath(workspace, path)
	}
}

func relativeReadPath(workspace string, root ResolvedReadPath) func(path string) string {
	if root.ExternalAlias || root.OutputRelativePath(workspace) == root.OutputPath() {
		return func(path string) string {
			rel, err := filepath.Rel(root.Path, path)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
				return root.DisplayFor(path)
			}
			return filepath.ToSlash(rel)
		}
	}
	return relativeWorkspacePath(workspace)
}

func protectedReadDir(workspace string, protectedReadDirs []string, root ResolvedReadPath) func(path string) (string, bool) {
	return func(path string) (string, bool) {
		if _, blocked := PathWithinWorkspaceHostMetadata(workspace, path); blocked {
			if root.ExternalAlias {
				return root.DisplayFor(path), true
			}
			return WorkspaceRelativePath(workspace, path), true
		}
		if _, blocked := PathWithinProtectedDir(path, protectedReadDirs); blocked {
			if root.ExternalAlias {
				return root.DisplayFor(path), true
			}
			return WorkspaceRelativePath(workspace, path), true
		}
		return "", false
	}
}

func displaySearchPathMatches(matches []SearchPathMatch, root ResolvedReadPath) []SearchPathMatch {
	if !root.ExternalAlias {
		return matches
	}
	out := make([]SearchPathMatch, 0, len(matches))
	for _, match := range matches {
		match.Path = root.DisplayFor(match.Path)
		out = append(out, match)
	}
	return out
}

func displayGrepMatches(matches []GrepMatch, root ResolvedReadPath) []GrepMatch {
	if !root.ExternalAlias {
		return matches
	}
	out := make([]GrepMatch, 0, len(matches))
	for _, match := range matches {
		match.Path = root.DisplayFor(match.Path)
		out = append(out, match)
	}
	return out
}

func displayCodeIndexSymbols(symbols []filetoolsapp.CodeIndexSymbol, root ResolvedReadPath) []filetoolsapp.CodeIndexSymbol {
	if !root.ExternalAlias {
		return symbols
	}
	out := make([]filetoolsapp.CodeIndexSymbol, 0, len(symbols))
	for _, symbol := range symbols {
		symbol.Path = root.DisplayFor(symbol.Path)
		out = append(out, symbol)
	}
	return out
}

func readWorkspaceEscapeOutput(field string, path string, workspace string, sandboxMode string) map[string]any {
	field = strings.TrimSpace(field)
	if field == "" {
		field = "path"
	}
	output := map[string]any{
		"code":  "workspace_escape",
		"error": field + " must stay inside workspace",
	}
	if trimmed := strings.TrimSpace(path); trimmed != "" {
		output[field] = trimmed
	}
	if trimmed := strings.TrimSpace(workspace); trimmed != "" {
		output["workspace"] = trimmed
	}
	if strings.TrimSpace(sandboxMode) != "danger-full-access" {
		output["hint"] = "Use a path inside the active workspace or a configured read root, switch the workspace, or run the turn with danger-full-access for explicit external reads."
	}
	return output
}

func protectedPathOutputForSandbox(workspace string, path string, protectedReadDirs []string, sandboxMode string) map[string]any {
	if blocked := MandatoryProtectedPathOutput(workspace, path, protectedReadDirs); blocked != nil {
		return blocked
	}
	if strings.TrimSpace(sandboxMode) == "danger-full-access" {
		return nil
	}
	return ProtectedPathOutput(workspace, path, protectedReadDirs)
}

func firstNonEmptyAnyString(values ...any) string {
	for _, value := range values {
		if typed, ok := value.(string); ok {
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func boolField(record map[string]any, key string) bool {
	value, _ := record[key].(bool)
	return value
}
