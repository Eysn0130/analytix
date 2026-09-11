//go:build !windows

package evidenceregistry

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

func acquireRegistryFileLock(ctx context.Context, path string) (func() error, error) {
	directoryFD, err := unix.Open(filepath.Dir(path), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(directoryFD)
	base := filepath.Base(path)
	created := false
	fd := -1
	for attempt := 0; attempt < 4; attempt++ {
		fd, err = unix.Openat(directoryFD, base, unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.ENOENT) {
			return nil, err
		}
		fd, err = unix.Openat(directoryFD, base, unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_CREAT|unix.O_EXCL, 0o600)
		if err == nil {
			created = true
			break
		}
		if !errors.Is(err, unix.EEXIST) {
			return nil, err
		}
	}
	if err != nil {
		return nil, errors.New("evidence registry lock creation race did not converge")
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("evidence registry lock file handle is invalid")
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 {
		_ = file.Close()
		return nil, errors.New("evidence registry lock file has unsafe identity")
	}
	if created {
		if err := unix.Fchmod(fd, 0o600); err != nil {
			_ = file.Close()
			return nil, err
		}
	} else if stat.Mode&0o077 != 0 {
		_ = file.Close()
		return nil, errors.New("evidence registry lock file permissions are too broad")
	}
	for {
		err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() error {
				return errors.Join(unix.Flock(int(file.Fd()), unix.LOCK_UN), file.Close())
			}, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			_ = file.Close()
			return nil, err
		}
		select {
		case <-contextDone(ctx):
			_ = file.Close()
			return nil, contextError(ctx)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func contextDone(ctx context.Context) <-chan struct{} {
	if ctx == nil {
		return make(chan struct{})
	}
	return ctx.Done()
}
