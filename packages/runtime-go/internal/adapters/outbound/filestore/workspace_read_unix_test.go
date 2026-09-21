//go:build darwin || linux

package filestore

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/ports/workspaceread"
)

func workspaceReadWrite(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func workspaceReadRoot(t *testing.T, files *WorkspaceReadFiles, path string) workspaceread.Root {
	t.Helper()
	root, err := files.InspectRoot(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestWorkspaceReadNormalAndProtected(t *testing.T) {
	path := t.TempDir()
	for name, content := range map[string]string{
		"nested/read.md": "approved text", "document.pdf": "%PDF-fixture",
		"private/secret.md": "never deliver", ".analytix/authority.md": "never deliver",
		".git/config.md": "never deliver", "node_modules/pkg/read.md": "never deliver",
		"nul.md": "binary\x00text", "invalid.md": string([]byte{0xff}), "code.go": "not a writing document",
	} {
		workspaceReadWrite(t, path, name, content)
	}
	files := NewWorkspaceReadFiles([]string{MandatoryProtectedRoot(filepath.Join(path, "private"))})
	root := workspaceReadRoot(t, files, path)
	for _, includePDF := range []bool{false, true} {
		got, err := files.Scan(context.Background(), root, includePDF, 2500*time.Millisecond, func() error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		want := 1
		if includePDF {
			want++
		}
		if len(got) != want {
			t.Fatalf("got %d files, want %d", len(got), want)
		}
		for _, file := range got {
			if file.Path != "nested/read.md" && file.Path != "document.pdf" {
				t.Fatalf("unexpected path: %s", file.Path)
			}
			if file.Revision != fmt.Sprintf("%x", sha256.Sum256(file.Content)) {
				t.Fatal("revision is not content hash")
			}
		}
	}
	if _, err := files.InspectRoot(context.Background(), filepath.Join(path, "private")); err == nil {
		t.Fatal("protected root admitted")
	}
	if _, err := files.InspectRoot(context.Background(), filepath.Join(path, ".analytix")); err == nil {
		t.Fatal("metadata root admitted")
	}
	other := NewWorkspaceReadFiles([]string{filepath.Join(path, "nested")})
	if _, err := other.Scan(context.Background(), root, false, time.Second, func() error { return nil }); err == nil {
		t.Fatal("changed policy admitted")
	}
}

func TestWorkspaceReadRejectsLinks(t *testing.T) {
	path := t.TempDir()
	outside := t.TempDir()
	workspaceReadWrite(t, outside, "secret.md", "secret")
	if err := os.Link(filepath.Join(outside, "secret.md"), filepath.Join(path, "hard.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(path, "sym.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(path, "dir")); err != nil {
		t.Fatal(err)
	}
	files := NewWorkspaceReadFiles(nil)
	root := workspaceReadRoot(t, files, path)
	got, err := files.Scan(context.Background(), root, false, time.Second, func() error { return nil })
	if err != nil || len(got) != 0 {
		t.Fatalf("links delivered: count=%d err=%v", len(got), err)
	}
	if _, err := files.InspectRoot(context.Background(), filepath.Join(path, "dir")); err == nil {
		t.Fatal("symlink root admitted")
	}
}

func TestWorkspaceReadRootAndAncestorReplacement(t *testing.T) {
	for _, ancestor := range []bool{false, true} {
		t.Run(fmt.Sprint(ancestor), func(t *testing.T) {
			base := t.TempDir()
			path := filepath.Join(base, "parent", "workspace")
			workspaceReadWrite(t, path, "read.md", "original")
			files := NewWorkspaceReadFiles(nil)
			root := workspaceReadRoot(t, files, path)
			if ancestor {
				old := filepath.Join(base, "previous")
				if err := os.Rename(filepath.Dir(path), old); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(filepath.Join(old, "workspace"), path); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Rename(path, path+"-old"); err != nil {
					t.Fatal(err)
				}
				workspaceReadWrite(t, path, "read.md", "replacement")
			}
			got, err := files.Scan(context.Background(), root, false, time.Second, func() error { return nil })
			if err == nil || got != nil {
				t.Fatal("replacement admitted")
			}
		})
	}
}

func TestWorkspaceReadRevocationAndMutationDiscardAll(t *testing.T) {
	for _, revokeAt := range []int{1, 5, 8, 10} {
		t.Run(fmt.Sprint(revokeAt), func(t *testing.T) {
			path := t.TempDir()
			workspaceReadWrite(t, path, "read.md", "original")
			files := NewWorkspaceReadFiles(nil)
			root := workspaceReadRoot(t, files, path)
			calls := 0
			got, err := files.Scan(context.Background(), root, false, time.Second, func() error {
				calls++
				if calls >= revokeAt {
					return workspaceread.ErrUnavailable
				}
				return nil
			})
			if err == nil || got != nil {
				t.Fatalf("revocation admitted at call %d of %d", revokeAt, calls)
			}
		})
	}
	t.Run("same size content mutation", func(t *testing.T) {
		path := t.TempDir()
		workspaceReadWrite(t, path, "read.md", "original")
		files := NewWorkspaceReadFiles(nil)
		root := workspaceReadRoot(t, files, path)
		calls := 0
		got, err := files.Scan(context.Background(), root, false, time.Second, func() error {
			calls++
			if calls == 8 {
				workspaceReadWrite(t, path, "read.md", "modified")
			}
			return nil
		})
		if err == nil || got != nil {
			t.Fatalf("mutated content admitted (calls=%d)", calls)
		}
	})
}

func TestWorkspaceReadBounds(t *testing.T) {
	t.Run("snapshot bytes", func(t *testing.T) {
		path := t.TempDir()
		for _, name := range []string{"one.pdf", "two.pdf"} {
			workspaceReadWrite(t, path, name, "")
			if err := os.Truncate(filepath.Join(path, name), 9<<20); err != nil {
				t.Fatal(err)
			}
		}
		files := NewWorkspaceReadFiles(nil)
		got, err := files.Scan(context.Background(), workspaceReadRoot(t, files, path), true, 2500*time.Millisecond, func() error { return nil })
		if err != nil || len(got) != 1 || len(got[0].Content) != 9<<20 {
			t.Fatalf("snapshot bounds failed: count=%d err=%v", len(got), err)
		}
	})
	t.Run("file sizes", func(t *testing.T) {
		path := t.TempDir()
		workspaceReadWrite(t, path, "large.md", strings.Repeat("a", workspaceread.MaxTextBytes+1))
		workspaceReadWrite(t, path, "huge.pdf", "")
		if err := os.Truncate(filepath.Join(path, "huge.pdf"), workspaceread.MaxSnapshotBytes+1); err != nil {
			t.Fatal(err)
		}
		workspaceReadWrite(t, path, "small.md", "small")
		files := NewWorkspaceReadFiles(nil)
		root := workspaceReadRoot(t, files, path)
		got, err := files.Scan(context.Background(), root, true, time.Second, func() error { return nil })
		if err != nil || len(got) != 1 || got[0].Path != "small.md" {
			t.Fatalf("size bounds failed: count=%d err=%v", len(got), err)
		}
		for _, budget := range []time.Duration{0, -1, time.Nanosecond} {
			got, err := files.Scan(context.Background(), root, true, budget, func() error { return nil })
			if err == nil || got != nil {
				t.Fatal("expired budget admitted")
			}
		}
	})
	t.Run("files", func(t *testing.T) {
		path := t.TempDir()
		for i := 0; i < workspaceread.MaxFiles+1; i++ {
			workspaceReadWrite(t, path, fmt.Sprintf("%03d.md", i), "small")
		}
		files := NewWorkspaceReadFiles(nil)
		got, err := files.Scan(context.Background(), workspaceReadRoot(t, files, path), false, 2500*time.Millisecond, func() error { return nil })
		if err != nil || len(got) != workspaceread.MaxFiles {
			t.Fatalf("file bounds failed: count=%d err=%v", len(got), err)
		}
	})
}
