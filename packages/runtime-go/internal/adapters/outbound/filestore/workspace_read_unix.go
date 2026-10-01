//go:build darwin || linux

package filestore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"analytix.local/runtime-go/internal/ports/workspaceread"
	"golang.org/x/sys/unix"
)

// Include ancestor identities: replacing a parent and moving the old root into
// it must invalidate a cached binding even when the root inode survives.
func workspaceReadOpenRoot(path string) (int, string, error) {
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, "", workspaceread.ErrUnavailable
	}
	chain := sha256.New()
	var stat unix.Stat_t
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, part := range append([]string{""}, parts...) {
		if part != "" {
			next, openErr := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			unix.Close(fd)
			if openErr != nil {
				return -1, "", workspaceread.ErrUnavailable
			}
			fd = next
		}
		if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR {
			unix.Close(fd)
			return -1, "", workspaceread.ErrUnavailable
		}
		fmt.Fprintf(chain, "%d:%d/", stat.Dev, stat.Ino)
	}
	return fd, fmt.Sprintf("%d:%d:%x", stat.Dev, stat.Ino, chain.Sum(nil)), nil
}

func (f *WorkspaceReadFiles) InspectRoot(ctx context.Context, workspace string) (workspaceread.Root, error) {
	if ctx == nil || ctx.Err() != nil || !filepath.IsAbs(workspace) || strings.TrimSpace(workspace) != workspace {
		return workspaceread.Root{}, workspaceread.ErrUnavailable
	}
	path, err := canonicalAtomicUnixSystemAlias(workspace)
	if err != nil || f.protected(path, path) {
		return workspaceread.Root{}, workspaceread.ErrUnavailable
	}
	// Rooting a workspace inside hidden authority metadata must not turn it
	// into an ordinary directory by changing the active workspace boundary.
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if strings.EqualFold(part, ".git") || strings.EqualFold(part, ".analytix") {
			return workspaceread.Root{}, workspaceread.ErrUnavailable
		}
	}
	policy, err := f.policy()
	if err != nil {
		return workspaceread.Root{}, err
	}
	fd, identity, err := workspaceReadOpenRoot(path)
	if err != nil {
		return workspaceread.Root{}, err
	}
	unix.Close(fd)
	if ctx.Err() != nil {
		return workspaceread.Root{}, workspaceread.ErrUnavailable
	}
	return workspaceread.Root{Workspace: path, Identity: identity, Policy: policy}, nil
}

type workspaceReadObservation struct {
	path string
	stat unix.Stat_t
}

func sameWorkspaceReadStat(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode &&
		a.Size == b.Size && a.Nlink == b.Nlink && a.Mtim == b.Mtim && a.Ctim == b.Ctim
}

func (f *WorkspaceReadFiles) Scan(ctx context.Context, root workspaceread.Root, includePDF bool, budget time.Duration, authorize func() error) ([]workspaceread.File, error) {
	if ctx == nil || authorize == nil || budget <= 0 {
		return nil, workspaceread.ErrUnavailable
	}
	// Callers cannot turn this bounded local-display path into a long scan.
	if budget > 2500*time.Millisecond {
		budget = 2500 * time.Millisecond
	}
	deadline := time.Now().Add(budget)
	check := func() error {
		if ctx.Err() != nil || time.Now().After(deadline) || authorize() != nil {
			return workspaceread.ErrUnavailable
		}
		fresh, err := f.InspectRoot(ctx, root.Workspace)
		if err != nil || fresh != root {
			return workspaceread.ErrUnavailable
		}
		return nil
	}
	if check() != nil {
		return nil, workspaceread.ErrUnavailable
	}
	rootFD, identity, err := workspaceReadOpenRoot(root.Workspace)
	if err != nil || identity != root.Identity {
		if rootFD >= 0 {
			unix.Close(rootFD)
		}
		return nil, workspaceread.ErrUnavailable
	}
	defer unix.Close(rootFD)
	files := make([]workspaceread.File, 0)
	observations := make([]workspaceReadObservation, 0)
	entries, total := 0, 0
	full := func() bool {
		return len(files) >= workspaceread.MaxFiles || entries >= workspaceread.MaxEntries || total >= workspaceread.MaxSnapshotBytes
	}
	var walk func(int, string, int) error
	walk = func(fd int, relative string, depth int) error {
		if depth > 64 || check() != nil {
			return workspaceread.ErrUnavailable
		}
		var before unix.Stat_t
		if unix.Fstat(fd, &before) != nil {
			return workspaceread.ErrUnavailable
		}
		observations = append(observations, workspaceReadObservation{relative, before})
		// A separate descriptor owns the enumeration offset and is always closed.
		copyFD, err := unix.Openat(fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return workspaceread.ErrUnavailable
		}
		directory := os.NewFile(uintptr(copyFD), "workspace-read-directory")
		defer directory.Close()
		for !full() {
			if check() != nil {
				return workspaceread.ErrUnavailable
			}
			names, readErr := directory.Readdirnames(min(64, workspaceread.MaxEntries-entries))
			if check() != nil {
				return workspaceread.ErrUnavailable
			}
			if readErr != nil && readErr != io.EOF {
				return workspaceread.ErrUnavailable
			}
			for _, name := range names {
				entries++
				if name == "." || name == ".." || strings.ContainsRune(name, filepath.Separator) {
					return workspaceread.ErrUnavailable
				}
				if workspaceReadSkipDirs[name] || strings.EqualFold(name, ".git") || strings.EqualFold(name, ".analytix") {
					continue
				}
				rel := filepath.Join(relative, name)
				absolute := filepath.Join(root.Workspace, rel)
				if f.protected(root.Workspace, absolute) {
					continue
				}
				if check() != nil {
					return workspaceread.ErrUnavailable
				}
				var stat unix.Stat_t
				if unix.Fstatat(fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW) != nil {
					return workspaceread.ErrUnavailable
				}
				if stat.Mode&unix.S_IFMT == unix.S_IFDIR {
					child, err := unix.Openat(fd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
					if err != nil {
						return workspaceread.ErrUnavailable
					}
					var opened unix.Stat_t
					if unix.Fstat(child, &opened) != nil || !sameWorkspaceReadStat(stat, opened) {
						unix.Close(child)
						return workspaceread.ErrUnavailable
					}
					err = walk(child, rel, depth+1)
					unix.Close(child)
					if err != nil {
						return err
					}
				} else if kind := workspaceReadKind(name, includePDF); kind != "" && stat.Mode&unix.S_IFMT == unix.S_IFREG && stat.Nlink == 1 {
					limit := workspaceread.MaxSnapshotBytes - total
					if kind == "text" {
						limit = min(limit, workspaceread.MaxTextBytes)
					}
					if stat.Size < 0 || stat.Size > int64(limit) {
						continue
					}
					content, observed, err := f.readWorkspaceFile(fd, name, stat, check)
					if err != nil {
						return err
					}
					if kind == "text" && (!utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0) {
						continue
					}
					sum := sha256.Sum256(content)
					files = append(files, workspaceread.File{Path: filepath.ToSlash(rel), Kind: kind, Revision: hex.EncodeToString(sum[:]), Content: content})
					total += len(content)
					observations = append(observations, workspaceReadObservation{rel, observed})
				}
				if full() {
					break
				}
			}
			if readErr == io.EOF {
				break
			}
		}
		var after unix.Stat_t
		if check() != nil || unix.Fstat(fd, &after) != nil || !sameWorkspaceReadStat(before, after) {
			return workspaceread.ErrUnavailable
		}
		return nil
	}
	if err := walk(rootFD, "", 0); err != nil {
		return nil, workspaceread.ErrUnavailable
	}
	// Recheck every observed object after the scan, including directories whose
	// handles were closed. Nothing from an invalidated scan is delivered.
	for _, observed := range observations {
		if check() != nil {
			return nil, workspaceread.ErrUnavailable
		}
		path := filepath.Join(root.Workspace, observed.path)
		if f.protected(root.Workspace, path) {
			return nil, workspaceread.ErrUnavailable
		}
		var current unix.Stat_t
		if observed.path == "" {
			err = unix.Fstat(rootFD, &current)
		} else {
			parent, name, missing, openErr := openAtomicUnixParent(path, false)
			if openErr != nil || missing {
				return nil, workspaceread.ErrUnavailable
			}
			err = unix.Fstatat(parent, name, &current, unix.AT_SYMLINK_NOFOLLOW)
			unix.Close(parent)
		}
		if err != nil || !sameWorkspaceReadStat(observed.stat, current) {
			return nil, workspaceread.ErrUnavailable
		}
	}
	if check() != nil {
		return nil, workspaceread.ErrUnavailable
	}
	return files, nil
}

func (f *WorkspaceReadFiles) readWorkspaceFile(parent int, name string, expected unix.Stat_t, check func() error) ([]byte, unix.Stat_t, error) {
	if check() != nil {
		return nil, unix.Stat_t{}, workspaceread.ErrUnavailable
	}
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, unix.Stat_t{}, workspaceread.ErrUnavailable
	}
	file := os.NewFile(uintptr(fd), "workspace-read-file")
	defer file.Close()
	var before, after, linked unix.Stat_t
	if unix.Fstat(fd, &before) != nil || !sameWorkspaceReadStat(expected, before) || before.Nlink != 1 || before.Mode&unix.S_IFMT != unix.S_IFREG {
		return nil, unix.Stat_t{}, workspaceread.ErrUnavailable
	}
	// Read exactly the admitted size, never a limit+1 sentinel byte. Growth is
	// detected by the post-read size/ctime checks without reading excess bytes.
	content := make([]byte, int(before.Size))
	if check() != nil {
		return nil, unix.Stat_t{}, workspaceread.ErrUnavailable
	}
	_, err = io.ReadFull(file, content)
	if err != nil || check() != nil || unix.Fstat(fd, &after) != nil || unix.Fstatat(parent, name, &linked, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		!sameWorkspaceReadStat(before, after) || !sameWorkspaceReadStat(after, linked) {
		return nil, unix.Stat_t{}, workspaceread.ErrUnavailable
	}
	return content, after, nil
}
