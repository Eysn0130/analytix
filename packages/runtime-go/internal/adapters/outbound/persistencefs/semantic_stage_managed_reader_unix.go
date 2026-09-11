//go:build darwin || linux

package persistencefs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	"golang.org/x/sys/unix"
)

// readSemanticStageManagedFile reads only a plan-bound managed snapshot below
// an already pinned private stage root. Managed data intentionally preserves
// its ordinary modes (for example 0755 directories and 0644 files); startup
// authority keys, journals, and marker files continue to use the stricter
// private-directory reader.
func readSemanticStageManagedFile(
	ctx context.Context,
	root *startupPrivateDirectory,
	relative string,
	expected domainstartup.SemanticEntryStateV1,
) (body []byte, resultErr error) {
	return readSemanticStageManagedFileWithHook(ctx, root, relative, expected, nil)
}

func readSemanticStageManagedFileWithHook(
	ctx context.Context,
	root *startupPrivateDirectory,
	relative string,
	expected domainstartup.SemanticEntryStateV1,
	hook func(string) error,
) (body []byte, resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if root == nil || root.fd < 0 || expected.Type != domainstartup.ManagedEntryTypeFile ||
		expected.Mode == 0 || expected.Size < 0 || expected.Size > domainstartup.MaxSemanticManagedFileBytesV1 ||
		!domainsecurity.IsSHA256Hex(expected.SHA256) {
		return nil, errors.New("semantic stage managed file authority is invalid")
	}
	parts, err := secureRelativeComponents(relative)
	if err != nil {
		return nil, err
	}
	rootFD, err := unix.Dup(root.fd)
	if err != nil {
		return nil, err
	}
	unix.CloseOnExec(rootFD)
	directories := []int{rootFD}
	directoryNames := make([]string, 0, len(parts)-1)
	defer func() {
		for index := len(directories) - 1; index >= 0; index-- {
			resultErr = errors.Join(resultErr, unix.Close(directories[index]))
		}
		if resultErr != nil {
			clear(body)
			body = nil
		}
	}()
	if identity, err := unixOpenedDirectoryIdentity(rootFD); err != nil || identity != root.Identity() {
		return nil, errors.New("semantic stage managed root identity changed")
	}
	for _, component := range parts[:len(parts)-1] {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		parent := directories[len(directories)-1]
		next, err := unix.Openat(parent, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return nil, err
		}
		var stat unix.Stat_t
		if err := unix.Fstat(next, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR {
			_ = unix.Close(next)
			return nil, errors.New("semantic stage managed directory is unsafe")
		}
		directories = append(directories, next)
		directoryNames = append(directoryNames, component)
	}
	parent := directories[len(directories)-1]
	name := parts[len(parts)-1]
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("semantic stage managed file handle is invalid")
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	initialInfo, err := file.Stat()
	var initial unix.Stat_t
	if err != nil || unix.Fstat(fd, &initial) != nil || !initialInfo.Mode().IsRegular() ||
		initial.Mode&unix.S_IFMT != unix.S_IFREG || initial.Nlink != 1 ||
		uint32(initialInfo.Mode()) != expected.Mode || initialInfo.Size() != expected.Size {
		return nil, errors.New("semantic stage managed file state is invalid")
	}
	capacity := expected.Size
	if capacity > 1<<20 {
		capacity = 1 << 20
	}
	body = make([]byte, 0, int(capacity))
	hasher := sha256.New()
	buffer := make([]byte, 1<<20)
	readBytes := int64(0)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		count, readErr := file.Read(buffer)
		if count > 0 {
			readBytes += int64(count)
			if readBytes > expected.Size || readBytes > domainstartup.MaxSemanticManagedFileBytesV1 {
				return nil, errors.New("semantic stage managed file exceeded its bound")
			}
			_, _ = hasher.Write(buffer[:count])
			body = append(body, buffer[:count]...)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	clear(buffer)
	if readBytes != expected.Size || hex.EncodeToString(hasher.Sum(nil)) != expected.SHA256 {
		return nil, errors.New("semantic stage managed file content changed")
	}
	if hook != nil {
		if err := hook("before_final_name_check"); err != nil {
			return nil, err
		}
	}
	refreshedInfo, statErr := file.Stat()
	var refreshed unix.Stat_t
	if statErr != nil || unix.Fstat(fd, &refreshed) != nil || !os.SameFile(initialInfo, refreshedInfo) ||
		refreshed.Dev != initial.Dev || refreshed.Ino != initial.Ino || refreshed.Mode != initial.Mode ||
		refreshed.Nlink != 1 || refreshed.Size != initial.Size || uint32(refreshedInfo.Mode()) != expected.Mode {
		return nil, errors.New("semantic stage managed file identity changed while reading")
	}
	var mapped unix.Stat_t
	if err := unix.Fstatat(parent, name, &mapped, unix.AT_SYMLINK_NOFOLLOW); err != nil ||
		mapped.Mode&unix.S_IFMT != unix.S_IFREG || mapped.Dev != initial.Dev || mapped.Ino != initial.Ino || mapped.Nlink != 1 {
		return nil, errors.New("semantic stage managed file name mapping changed")
	}
	for index, component := range directoryNames {
		var opened unix.Stat_t
		var nameMapped unix.Stat_t
		if err := unix.Fstat(directories[index+1], &opened); err != nil ||
			unix.Fstatat(directories[index], component, &nameMapped, unix.AT_SYMLINK_NOFOLLOW) != nil ||
			opened.Mode&unix.S_IFMT != unix.S_IFDIR || nameMapped.Mode&unix.S_IFMT != unix.S_IFDIR ||
			opened.Dev != nameMapped.Dev || opened.Ino != nameMapped.Ino {
			return nil, errors.New("semantic stage managed directory name mapping changed")
		}
	}
	if identity, err := unixOpenedDirectoryIdentity(rootFD); err != nil || identity != root.Identity() || ctx.Err() != nil {
		return nil, errors.Join(errors.New("semantic stage managed root identity changed"), err, ctx.Err())
	}
	return body, nil
}
