package filestore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCodeIndexScanCollectsSymbolsAndSkipsDirs(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(`package demo

type Client struct{}
func NewClient() *Client { return &Client{} }
`), 0o600); err != nil {
		t.Fatalf("write go: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0o700); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "skip.ts"), []byte("export const hidden = () => {}\n"), 0o600); err != nil {
		t.Fatalf("write skipped: %v", err)
	}
	protectedDir := filepath.Join(root, "protected")
	if err := os.Mkdir(protectedDir, 0o700); err != nil {
		t.Fatalf("mkdir protected: %v", err)
	}
	if err := os.WriteFile(filepath.Join(protectedDir, "secret.ts"), []byte("export const secret = () => {}\n"), 0o600); err != nil {
		t.Fatalf("write protected: %v", err)
	}

	result, err := CodeIndexScan(CodeIndexScanRequest{
		Context:      context.Background(),
		Workspace:    root,
		Root:         root,
		Limit:        20,
		MaxFiles:     100,
		MaxFileBytes: 1 << 20,
		Outline:      true,
		RelativePath: func(path string) string {
			rel, _ := filepath.Rel(root, path)
			return filepath.ToSlash(rel)
		},
		ProtectedDir: func(path string) (string, bool) {
			if path == protectedDir {
				return "protected", true
			}
			return "", false
		},
	})
	if err != nil {
		t.Fatalf("code index scan: %v", err)
	}
	got := map[string]bool{}
	for _, symbol := range result.Symbols {
		got[symbol.Kind+":"+symbol.Name] = true
		if symbol.File != "main.go" {
			t.Fatalf("unexpected indexed file: %#v", symbol)
		}
	}
	if !got["struct:Client"] || !got["func:NewClient"] {
		t.Fatalf("symbols = %#v", result.Symbols)
	}
	if len(result.SkippedDirs) != 1 || result.SkippedDirs[0] != "node_modules" {
		t.Fatalf("skipped dirs = %#v", result.SkippedDirs)
	}
	if len(result.SkippedProtected) != 1 || result.SkippedProtected[0] != "protected" {
		t.Fatalf("skipped protected = %#v", result.SkippedProtected)
	}
}

func TestCodeIndexScanHonorsFileLimit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.ts"), []byte("export const alpha = () => {}\n"), 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.ts"), []byte("export const beta = () => {}\n"), 0o600); err != nil {
		t.Fatalf("write b: %v", err)
	}
	result, err := CodeIndexScan(CodeIndexScanRequest{
		Context:      context.Background(),
		Workspace:    root,
		Root:         root,
		Limit:        20,
		MaxFiles:     1,
		MaxFileBytes: 1 << 20,
		RelativePath: func(path string) string {
			rel, _ := filepath.Rel(root, path)
			return filepath.ToSlash(rel)
		},
	})
	if err != nil {
		t.Fatalf("code index scan: %v", err)
	}
	if !result.Truncated {
		t.Fatal("expected truncated result")
	}
	if len(result.Symbols) != 1 || result.Symbols[0].File != "a.ts" {
		t.Fatalf("symbols = %#v", result.Symbols)
	}
}
