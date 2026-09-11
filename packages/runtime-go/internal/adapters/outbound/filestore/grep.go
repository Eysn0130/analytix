package filestore

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"

	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
)

type GrepLineMatcher func(line string) (bool, int)

type GrepScanRequest struct {
	Workspace        string
	Root             string
	Limit            int
	ContextLines     int
	Match            GrepLineMatcher
	IncludePath      func(relativePath string) bool
	RelativePath     func(path string) string
	ProtectedDir     func(path string) (string, bool)
	SkipDirName      func(name string) bool
	HiddenEntryName  func(name string) bool
	LooksUTF16Text   func(data []byte) bool
	DecodeText       func(data []byte) (string, bool)
	SplitTextLines   func(content string) []string
	GitIgnoreMatcher *GitIgnoreMatcher
}

type GrepScanResult struct {
	Matches          []GrepMatch
	SkippedProtected []string
	SkippedDirs      []string
	LimitReached     bool
}

type GrepMatch = filetoolsapp.GrepMatch

func GrepScan(ctx context.Context, request GrepScanRequest) (GrepScanResult, error) {
	result := GrepScanResult{}
	if request.Match == nil {
		return result, nil
	}
	limit := request.Limit
	if limit <= 0 {
		limit = 1
	}
	contextLines := request.ContextLines
	if contextLines < 0 {
		contextLines = 0
	}
	relativePath := request.RelativePath
	if relativePath == nil {
		relativePath = func(path string) string {
			if request.Workspace == "" {
				return filepath.ToSlash(path)
			}
			rel, err := filepath.Rel(request.Workspace, path)
			if err != nil {
				return filepath.ToSlash(path)
			}
			return filepath.ToSlash(rel)
		}
	}
	includePath := request.IncludePath
	if includePath == nil {
		includePath = func(string) bool { return true }
	}
	skipDirName := request.SkipDirName
	if skipDirName == nil {
		skipDirName = func(string) bool { return false }
	}
	hiddenEntryName := request.HiddenEntryName
	if hiddenEntryName == nil {
		hiddenEntryName = func(string) bool { return false }
	}
	looksUTF16Text := request.LooksUTF16Text
	if looksUTF16Text == nil {
		looksUTF16Text = func([]byte) bool { return false }
	}
	decodeText := request.DecodeText
	if decodeText == nil {
		decodeText = func(data []byte) (string, bool) { return string(data), true }
	}
	splitTextLines := request.SplitTextLines
	if splitTextLines == nil {
		splitTextLines = func(content string) []string { return strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") }
	}
	ignorer := request.GitIgnoreMatcher
	if ignorer == nil {
		ignorer = NewGitIgnoreMatcher(request.Workspace, request.Root)
	}

	scanFile := func(path string) {
		if len(result.Matches) >= limit || ctx.Err() != nil {
			return
		}
		rel := relativePath(path)
		if !includePath(rel) {
			return
		}
		file, err := os.Open(path)
		if err != nil {
			return
		}
		defer file.Close()
		reader := bufio.NewReader(file)
		peek, _ := reader.Peek(8 * 1024)
		lineNumber := 0
		contextBefore := make([]string, 0, contextLines)
		type pendingContextAfter struct {
			index     int
			remaining int
			lines     []string
		}
		pendingAfter := []pendingContextAfter{}
		consumeLine := func(line string) bool {
			if ctx.Err() != nil {
				return true
			}
			lineNumber++
			if strings.IndexByte(line, 0) >= 0 {
				return true
			}
			if len(pendingAfter) > 0 {
				nextPending := pendingAfter[:0]
				for _, pending := range pendingAfter {
					pending.lines = append(pending.lines, line)
					pending.remaining--
					if pending.remaining <= 0 {
						result.Matches[pending.index].ContextAfter = append([]string(nil), pending.lines...)
						continue
					}
					nextPending = append(nextPending, pending)
				}
				pendingAfter = nextPending
			}
			if ok, column := request.Match(line); ok && len(result.Matches) < limit {
				item := GrepMatch{
					Path:         path,
					RelativePath: rel,
					Line:         lineNumber,
					Column:       column,
					Text:         line,
				}
				if contextLines > 0 {
					item.ContextBefore = append([]string(nil), contextBefore...)
				}
				result.Matches = append(result.Matches, item)
				if contextLines > 0 {
					pendingAfter = append(pendingAfter, pendingContextAfter{index: len(result.Matches) - 1, remaining: contextLines})
				}
			}
			if contextLines > 0 {
				contextBefore = append(contextBefore, line)
				if len(contextBefore) > contextLines {
					contextBefore = append([]string(nil), contextBefore[len(contextBefore)-contextLines:]...)
				}
			}
			if len(result.Matches) >= limit && len(pendingAfter) == 0 {
				return true
			}
			return false
		}
		finishPending := func() {
			for _, pending := range pendingAfter {
				result.Matches[pending.index].ContextAfter = append([]string(nil), pending.lines...)
			}
		}
		if looksUTF16Text(peek) {
			data, err := os.ReadFile(path)
			if err != nil {
				return
			}
			content, textOK := decodeText(data)
			if !textOK {
				return
			}
			for _, line := range splitTextLines(content) {
				if consumeLine(line) {
					finishPending()
					return
				}
			}
			finishPending()
			return
		}
		if BytesContainNUL(peek) {
			return
		}
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			if consumeLine(scanner.Text()) {
				return
			}
		}
		finishPending()
	}

	info, err := os.Stat(request.Root)
	if err != nil {
		return result, err
	}
	if !info.IsDir() {
		scanFile(request.Root)
		result.LimitReached = len(result.Matches) >= limit
		return result, nil
	}
	_ = filepath.WalkDir(request.Root, func(path string, entry os.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return filepath.SkipAll
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
			if skipDirName(entry.Name()) || hiddenEntryName(entry.Name()) {
				result.SkippedDirs = appendUniqueString(result.SkippedDirs, relativePath(path))
				return filepath.SkipDir
			}
			if ignorer.Ignored(path, true) {
				result.SkippedDirs = appendUniqueString(result.SkippedDirs, relativePath(path))
				return filepath.SkipDir
			}
			return nil
		}
		if hiddenEntryName(entry.Name()) {
			return nil
		}
		if ignorer.Ignored(path, false) {
			return nil
		}
		scanFile(path)
		if len(result.Matches) >= limit {
			return filepath.SkipAll
		}
		return nil
	})
	result.LimitReached = len(result.Matches) >= limit
	return result, nil
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func BytesContainNUL(data []byte) bool {
	for _, item := range data {
		if item == 0 {
			return true
		}
	}
	return false
}
