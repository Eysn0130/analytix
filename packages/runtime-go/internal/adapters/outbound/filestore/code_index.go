package filestore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
)

type CodeIndexScanRequest struct {
	Context      context.Context
	Workspace    string
	Root         string
	Limit        int
	MaxFiles     int
	MaxFileBytes int64
	Outline      bool
	RelativePath func(path string) string
	ProtectedDir func(path string) (string, bool)
}

type CodeIndexScanResult struct {
	Symbols          []filetoolsapp.CodeIndexSymbol
	SkippedProtected []string
	SkippedDirs      []string
	Truncated        bool
}

func CodeIndexScan(request CodeIndexScanRequest) (CodeIndexScanResult, error) {
	result := CodeIndexScanResult{}
	ctx := request.Context
	if ctx == nil {
		ctx = context.Background()
	}
	maxFiles := request.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 1
	}
	maxFileBytes := request.MaxFileBytes
	if maxFileBytes <= 0 {
		maxFileBytes = 1
	}
	relativePath := request.RelativePath
	if relativePath == nil {
		relativePath = func(path string) string {
			if strings.TrimSpace(request.Workspace) == "" {
				return filepath.ToSlash(path)
			}
			rel, err := filepath.Rel(request.Workspace, path)
			if err != nil {
				return filepath.ToSlash(path)
			}
			return filepath.ToSlash(rel)
		}
	}

	info, err := os.Stat(request.Root)
	if err != nil {
		return result, err
	}
	files := []string{}
	if !info.IsDir() {
		if filetoolsapp.SupportedCodeIndexFileExt(filepath.Ext(request.Root)) {
			files = append(files, request.Root)
		}
	} else {
		err = filepath.WalkDir(request.Root, func(path string, entry os.DirEntry, walkErr error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if walkErr != nil {
				return nil
			}
			if entry.IsDir() {
				if path != request.Root {
					if request.ProtectedDir != nil {
						if rel, blocked := request.ProtectedDir(path); blocked {
							result.SkippedProtected = appendUniqueString(result.SkippedProtected, rel)
							return filepath.SkipDir
						}
					}
					if filetoolsapp.SkipCodeIndexDir(entry.Name()) {
						result.SkippedDirs = appendUniqueString(result.SkippedDirs, relativePath(path))
						return filepath.SkipDir
					}
				}
				return nil
			}
			if filetoolsapp.SupportedCodeIndexFileExt(filepath.Ext(path)) {
				files = append(files, path)
				if len(files) >= maxFiles {
					return filepath.SkipAll
				}
			}
			return nil
		})
		if err != nil {
			return result, err
		}
	}

	sort.Strings(files)
	result.Truncated = len(files) >= maxFiles
	for _, file := range files {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		found, err := parseCodeIndexFile(file, relativePath(file), maxFileBytes)
		if err != nil {
			continue
		}
		result.Symbols = append(result.Symbols, found...)
		if request.Outline && request.Limit > 0 && len(result.Symbols) >= request.Limit {
			result.Truncated = true
			break
		}
	}
	sort.Slice(result.Symbols, func(i, j int) bool {
		if result.Symbols[i].File != result.Symbols[j].File {
			return result.Symbols[i].File < result.Symbols[j].File
		}
		if result.Symbols[i].Line != result.Symbols[j].Line {
			return result.Symbols[i].Line < result.Symbols[j].Line
		}
		if result.Symbols[i].Kind != result.Symbols[j].Kind {
			return result.Symbols[i].Kind < result.Symbols[j].Kind
		}
		return result.Symbols[i].Name < result.Symbols[j].Name
	})
	return result, nil
}

func parseCodeIndexFile(path string, relativePath string, maxFileBytes int64) ([]filetoolsapp.CodeIndexSymbol, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxFileBytes {
		return nil, fmt.Errorf("file too large")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".go" {
		return filetoolsapp.ParseCodeIndexGo(relativePath, path, data)
	}
	content, _, textOK := filetoolsapp.DecodeTextBytes(data)
	if !textOK {
		return nil, fmt.Errorf("not a text file")
	}
	return filetoolsapp.ParseCodeIndexText(ext, relativePath, path, content), nil
}
