//go:build darwin || linux

package filestore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/sys/unix"
)

type atomicUnixIdentity struct {
	dev uint64
	ino uint64
}

type atomicUnixState struct {
	state    atomicTextState
	identity atomicUnixIdentity
}

func inspectAtomicTextTargetPlatform(path string, missingParentsAreAbsent bool, maxBytes int64) (atomicTextState, error) {
	parent, base, missing, err := openAtomicUnixParent(path, false)
	if err != nil {
		return atomicTextState{}, err
	}
	if missing {
		if missingParentsAreAbsent {
			return atomicTextState{}, nil
		}
		return atomicTextState{}, os.ErrNotExist
	}
	defer unix.Close(parent)
	state, err := inspectAtomicUnixTarget(parent, base, maxBytes)
	return state.state, err
}

func atomicReplaceTextPlatform(request atomicTextReplaceRequest, hooks *atomicTextTestHooks) error {
	parent, base, missing, err := openAtomicUnixParent(request.Path, request.CreateParents)
	if err != nil {
		return err
	}
	if missing {
		return os.ErrNotExist
	}
	defer unix.Close(parent)

	initial, err := inspectAtomicUnixTarget(parent, base, request.MaxBytes)
	if err != nil {
		return err
	}
	if err := validateAtomicTextBefore(request, initial.state); err != nil {
		return err
	}
	if hooks != nil && hooks.AfterInitialValidation != nil {
		hooks.AfterInitialValidation()
	}

	tempName, err := atomicTextTempName(request.Path)
	if err != nil {
		return err
	}
	tempFD, err := unix.Openat(parent, tempName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return fmt.Errorf("create atomic text temporary file: %w", err)
	}
	tempOpen := true
	defer func() {
		if tempOpen {
			_ = unix.Close(tempFD)
		}
		_ = unix.Unlinkat(parent, tempName, 0)
	}()
	if err := writeAtomicTextContent(func(body []byte) (int, error) {
		return unix.Write(tempFD, body)
	}, request.Content, hooks); err != nil {
		return fmt.Errorf("write atomic text temporary file: %w", err)
	}
	mode := atomicTextReplacementMode(request, initial.state)
	if err := unix.Fchmod(tempFD, uint32(mode.Perm())); err != nil {
		return fmt.Errorf("set atomic text replacement mode before install: %w", err)
	}
	if err := unix.Fsync(tempFD); err != nil {
		return fmt.Errorf("sync atomic text temporary file: %w", err)
	}

	current, err := inspectAtomicUnixTarget(parent, base, request.MaxBytes)
	if err != nil {
		return err
	}
	if err := validateAtomicTextBefore(request, current.state); err != nil {
		return err
	}
	if !sameAtomicUnixState(initial, current) {
		return fmt.Errorf("%w: target identity changed before replace", ErrAtomicTextBeforeDrift)
	}
	if err := atomicTextBeforeReplace(hooks); err != nil {
		return fmt.Errorf("replace atomic text target: %w", err)
	}
	if err := installAtomicUnixText(parent, tempName, base, request.ExpectedExists); err != nil {
		return fmt.Errorf("replace atomic text target: %w", err)
	}
	if request.ExpectedExists {
		displaced, displacedErr := inspectAtomicUnixTarget(parent, tempName, request.MaxBytes)
		if displacedErr != nil || validateAtomicTextBefore(request, displaced.state) != nil || !sameAtomicUnixState(initial, displaced) {
			rollbackErr := rollbackAtomicUnixTextSwap(parent, tempName, base)
			if rollbackErr != nil {
				return fmt.Errorf("atomic text target changed at replace and rollback failed: %w", errors.Join(displacedErr, rollbackErr))
			}
			return fmt.Errorf("%w: target changed at atomic replace", ErrAtomicTextBeforeDrift)
		}
		if err := unix.Unlinkat(parent, tempName, 0); err != nil {
			return fmt.Errorf("remove displaced atomic text target: %w", err)
		}
	}
	if err := unix.Fsync(tempFD); err != nil {
		return fmt.Errorf("sync atomic text replacement metadata: %w", err)
	}
	if err := unix.Close(tempFD); err != nil {
		return fmt.Errorf("close atomic text replacement: %w", err)
	}
	tempOpen = false

	replaced, err := inspectAtomicUnixTarget(parent, base, request.MaxBytes)
	if err != nil {
		return fmt.Errorf("verify atomic text replacement: %w", err)
	}
	if !replaced.state.Exists || !equalAtomicTextBytes(replaced.state.Content, request.Content) || replaced.state.Mode.Perm() != mode.Perm() {
		return errors.New("atomic text replacement verification failed")
	}
	if err := unix.Fsync(parent); err != nil {
		return fmt.Errorf("sync atomic text parent directory: %w", err)
	}
	return nil
}

func openAtomicUnixParent(path string, createParents bool) (int, string, bool, error) {
	clean, err := canonicalAtomicUnixSystemAlias(path)
	if err != nil {
		return -1, "", false, err
	}
	if !filepath.IsAbs(clean) || clean == string(filepath.Separator) {
		return -1, "", false, fmt.Errorf("%w: invalid absolute target", ErrAtomicTextUnsafePath)
	}
	base := filepath.Base(clean)
	if base == "" || base == "." || base == ".." || strings.ContainsRune(base, filepath.Separator) {
		return -1, "", false, fmt.Errorf("%w: invalid target name", ErrAtomicTextUnsafePath)
	}
	current, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, "", false, fmt.Errorf("open filesystem root without following links: %w", err)
	}
	components := strings.Split(strings.TrimPrefix(filepath.Dir(clean), string(filepath.Separator)), string(filepath.Separator))
	for _, component := range components {
		if component == "" || component == "." {
			continue
		}
		if component == ".." {
			_ = unix.Close(current)
			return -1, "", false, fmt.Errorf("%w: parent traversal is invalid", ErrAtomicTextUnsafePath)
		}
		next, openErr := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if errors.Is(openErr, unix.ENOENT) && createParents {
			if mkdirErr := unix.Mkdirat(current, component, 0o755); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				_ = unix.Close(current)
				return -1, "", false, fmt.Errorf("create atomic text parent directory: %w", mkdirErr)
			}
			if syncErr := unix.Fsync(current); syncErr != nil {
				_ = unix.Close(current)
				return -1, "", false, fmt.Errorf("sync atomic text parent ancestor: %w", syncErr)
			}
			next, openErr = unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		if errors.Is(openErr, unix.ENOENT) {
			_ = unix.Close(current)
			return -1, base, true, nil
		}
		if openErr != nil {
			_ = unix.Close(current)
			return -1, "", false, fmt.Errorf("%w: open parent component %q without following links: %v", ErrAtomicTextUnsafePath, component, openErr)
		}
		var stat unix.Stat_t
		if statErr := unix.Fstat(next, &stat); statErr != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR {
			_ = unix.Close(next)
			_ = unix.Close(current)
			return -1, "", false, fmt.Errorf("%w: parent component %q is not a directory", ErrAtomicTextUnsafePath, component)
		}
		_ = unix.Close(current)
		current = next
	}
	return current, base, false, nil
}

func canonicalAtomicUnixSystemAlias(path string) (string, error) {
	clean := filepath.Clean(path)
	if runtime.GOOS != "darwin" {
		return clean, nil
	}
	aliases := []struct {
		alias  string
		target string
	}{
		{alias: "/var", target: "/private/var"},
		{alias: "/tmp", target: "/private/tmp"},
		{alias: "/etc", target: "/private/etc"},
	}
	for _, item := range aliases {
		if clean != item.alias && !strings.HasPrefix(clean, item.alias+string(filepath.Separator)) {
			continue
		}
		resolved, err := filepath.EvalSymlinks(item.alias)
		if err != nil || filepath.Clean(resolved) != item.target {
			return "", fmt.Errorf("%w: macOS system alias %s is not canonical", ErrAtomicTextUnsafePath, item.alias)
		}
		return filepath.Join(item.target, strings.TrimPrefix(clean, item.alias)), nil
	}
	return clean, nil
}

func inspectAtomicUnixTarget(parent int, base string, maxBytes int64) (atomicUnixState, error) {
	fd, err := unix.Openat(parent, base, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if errors.Is(err, unix.ENOENT) {
		return atomicUnixState{}, nil
	}
	if err != nil {
		return atomicUnixState{}, fmt.Errorf("%w: open target without following links: %v", ErrAtomicTextUnsafePath, err)
	}
	file := os.NewFile(uintptr(fd), base)
	if file == nil {
		_ = unix.Close(fd)
		return atomicUnixState{}, errors.New("atomic text target handle is invalid")
	}
	defer file.Close()
	var before unix.Stat_t
	if err := unix.Fstat(fd, &before); err != nil {
		return atomicUnixState{}, err
	}
	if before.Mode&unix.S_IFMT != unix.S_IFREG {
		return atomicUnixState{}, fmt.Errorf("%w: target is not a regular file", ErrAtomicTextUnsafePath)
	}
	if maxBytes > 0 && before.Size > maxBytes {
		return atomicUnixState{}, ErrAtomicTextTooLarge
	}
	content, readErr := readAtomicTextContent(file, maxBytes)
	if readErr != nil {
		return atomicUnixState{}, readErr
	}
	var after unix.Stat_t
	var linked unix.Stat_t
	if err := unix.Fstat(fd, &after); err != nil {
		return atomicUnixState{}, err
	}
	if err := unix.Fstatat(parent, base, &linked, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return atomicUnixState{}, fmt.Errorf("%w: target changed while being read: %v", ErrAtomicTextBeforeDrift, err)
	}
	if !sameAtomicUnixStat(before, after) || !sameAtomicUnixStat(after, linked) || int64(len(content)) != after.Size {
		return atomicUnixState{}, fmt.Errorf("%w: target changed while being read", ErrAtomicTextBeforeDrift)
	}
	return atomicUnixState{
		state:    atomicTextState{Exists: true, Content: content, Mode: os.FileMode(after.Mode & 0o777)},
		identity: atomicUnixIdentity{dev: uint64(after.Dev), ino: uint64(after.Ino)},
	}, nil
}

func sameAtomicUnixState(left, right atomicUnixState) bool {
	if left.state.Exists != right.state.Exists {
		return false
	}
	return !left.state.Exists || left.identity == right.identity
}

func sameAtomicUnixStat(left, right unix.Stat_t) bool {
	return left.Dev == right.Dev && left.Ino == right.Ino && left.Mode == right.Mode && left.Size == right.Size
}
