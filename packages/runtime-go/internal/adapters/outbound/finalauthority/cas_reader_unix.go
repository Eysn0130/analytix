//go:build darwin || linux

package finalauthority

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type acceptedFinalCASRootAuthority struct {
	path       string
	dev        uint64
	ino        uint64
	threadsDev uint64
	threadsIno uint64
}

func captureAcceptedFinalCASRoot(path string) (acceptedFinalCASRootAuthority, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil || strings.TrimSpace(path) == "" {
		return acceptedFinalCASRootAuthority{}, errors.New("accepted final CAS durable root is invalid")
	}
	absolute, err = canonicalPrivateRootPath(absolute)
	if err != nil {
		return acceptedFinalCASRootAuthority{}, err
	}
	root, err := securePrivateOpenAbsoluteDirectory(absolute, false)
	if err != nil {
		return acceptedFinalCASRootAuthority{}, err
	}
	defer unix.Close(root)
	rootStat, err := acceptedFinalCASUnixDirectoryStat(root)
	if err != nil {
		return acceptedFinalCASRootAuthority{}, err
	}
	threads, err := acceptedFinalCASUnixOpenDirectory(root, "threads")
	if err != nil {
		return acceptedFinalCASRootAuthority{}, errors.New("accepted final CAS threads authority is unavailable")
	}
	defer unix.Close(threads)
	threadsStat, err := acceptedFinalCASUnixDirectoryStat(threads)
	if err != nil {
		return acceptedFinalCASRootAuthority{}, err
	}
	return acceptedFinalCASRootAuthority{
		path: filepath.Clean(absolute), dev: uint64(rootStat.Dev), ino: rootStat.Ino,
		threadsDev: uint64(threadsStat.Dev), threadsIno: threadsStat.Ino,
	}, nil
}

func (authority acceptedFinalCASRootAuthority) readPrimaryThreadJSON(threadID string) ([]byte, error) {
	return authority.readThreadFile(threadID, "thread.json")
}

func (authority acceptedFinalCASRootAuthority) readThreadFile(threadID, name string) ([]byte, error) {
	if name != "thread.json" && name != "events.jsonl" {
		return nil, errors.New("Core snapshot file identity is invalid")
	}
	if !validCASRecordID(threadID) {
		return nil, errors.New("accepted final CAS thread identity is invalid")
	}
	root, err := authority.openRoot()
	if err != nil {
		return nil, err
	}
	defer unix.Close(root)
	threads, err := authority.openThreads(root)
	if err != nil {
		return nil, err
	}
	defer unix.Close(threads)
	thread, err := acceptedFinalCASUnixOpenDirectory(threads, threadID)
	if err != nil {
		return nil, errors.New("accepted final CAS primary thread directory is unavailable")
	}
	defer unix.Close(thread)
	threadStat, err := acceptedFinalCASUnixDirectoryStat(thread)
	if err != nil {
		return nil, err
	}
	fileFD, err := unix.Openat(thread, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, errors.New("accepted final CAS primary thread file is unavailable")
	}
	file := os.NewFile(uintptr(fileFD), name)
	if file == nil {
		_ = unix.Close(fileFD)
		return nil, errors.New("accepted final CAS primary thread handle is invalid")
	}
	defer file.Close()
	initial, err := acceptedFinalCASUnixFileStat(fileFD)
	if err != nil {
		return nil, err
	}
	body, err := acceptedFinalCASReadStableFile(file, initial.Size)
	if err != nil {
		return nil, err
	}
	refreshed, err := acceptedFinalCASUnixFileStat(fileFD)
	if err != nil || !acceptedFinalCASSameUnixFile(initial, refreshed) {
		return nil, errors.New("accepted final CAS primary thread changed during read")
	}
	if err := authority.verifyCurrentPath(root, threadID, name, threadStat, initial); err != nil {
		return nil, err
	}
	return body, nil
}

func (authority acceptedFinalCASRootAuthority) openRoot() (int, error) {
	root, err := securePrivateOpenAbsoluteDirectory(authority.path, false)
	if err != nil {
		return -1, err
	}
	stat, statErr := acceptedFinalCASUnixDirectoryStat(root)
	if statErr != nil || uint64(stat.Dev) != authority.dev || stat.Ino != authority.ino {
		_ = unix.Close(root)
		return -1, errors.New("accepted final CAS durable root identity changed")
	}
	return root, nil
}

func (authority acceptedFinalCASRootAuthority) openThreads(root int) (int, error) {
	threads, err := acceptedFinalCASUnixOpenDirectory(root, "threads")
	if err != nil {
		return -1, err
	}
	stat, statErr := acceptedFinalCASUnixDirectoryStat(threads)
	if statErr != nil || uint64(stat.Dev) != authority.threadsDev || stat.Ino != authority.threadsIno {
		_ = unix.Close(threads)
		return -1, errors.New("accepted final CAS threads authority identity changed")
	}
	return threads, nil
}

func (authority acceptedFinalCASRootAuthority) verifyCurrentPath(root int, threadID, name string, expectedThreadDir, expectedFile unix.Stat_t) error {
	reopenedRoot, err := authority.openRoot()
	if err != nil {
		return err
	}
	_ = unix.Close(reopenedRoot)
	reopenedThreads, err := authority.openThreads(root)
	if err != nil {
		return err
	}
	defer unix.Close(reopenedThreads)
	reopenedThread, err := acceptedFinalCASUnixOpenDirectory(reopenedThreads, threadID)
	if err != nil {
		return errors.New("accepted final CAS primary thread ancestor changed during read")
	}
	defer unix.Close(reopenedThread)
	threadStat, err := acceptedFinalCASUnixDirectoryStat(reopenedThread)
	if err != nil || uint64(threadStat.Dev) != uint64(expectedThreadDir.Dev) || threadStat.Ino != expectedThreadDir.Ino {
		return errors.New("accepted final CAS primary thread ancestor changed during read")
	}
	reopenedFile, err := unix.Openat(reopenedThread, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return errors.New("accepted final CAS primary thread identity changed during read")
	}
	defer unix.Close(reopenedFile)
	current, err := acceptedFinalCASUnixFileStat(reopenedFile)
	if err != nil || !acceptedFinalCASSameUnixFile(expectedFile, current) {
		return errors.New("accepted final CAS primary thread identity changed during read")
	}
	return nil
}

func acceptedFinalCASUnixOpenDirectory(parent int, name string) (int, error) {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsRune(name, 0) {
		return -1, errors.New("accepted final CAS directory component is invalid")
	}
	return unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
}

func acceptedFinalCASUnixDirectoryStat(fd int) (unix.Stat_t, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return unix.Stat_t{}, errors.New("accepted final CAS directory authority is invalid")
	}
	return stat, nil
}

func acceptedFinalCASUnixFileStat(fd int) (unix.Stat_t, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 ||
		stat.Size <= 0 || stat.Size > maxAcceptedFinalCASFileBytes {
		return unix.Stat_t{}, errors.New("accepted final CAS primary thread file is unsafe")
	}
	return stat, nil
}

func acceptedFinalCASSameUnixFile(left, right unix.Stat_t) bool {
	return uint64(left.Dev) == uint64(right.Dev) && left.Ino == right.Ino && left.Mode == right.Mode &&
		left.Nlink == right.Nlink && left.Size == right.Size && acceptedFinalCASUnixTimesEqual(left, right)
}
