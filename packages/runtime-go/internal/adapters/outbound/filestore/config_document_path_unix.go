//go:build darwin || linux

package filestore

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func readRuntimeConfigPathSnapshotV1(path string, maxBytes int64) ([]byte, bool, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, errors.New("runtime configuration path cannot be opened without following links")
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, false, errors.New("runtime configuration handle is invalid")
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() < 0 || before.Size() > maxBytes {
		return nil, false, errors.New("runtime configuration path is not a bounded regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(raw)) != before.Size() || int64(len(raw)) > maxBytes {
		return nil, false, errors.New("runtime configuration changed during snapshot read")
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() ||
		before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return nil, false, errors.New("runtime configuration changed during snapshot read")
	}
	return raw, true, nil
}
