//go:build darwin

package processauthority

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"unsafe"
)

const maxDarwinProcessPathBytes = 4096

// loadedDarwinExecutableMatchesOpenedFile binds the suspended process image to
// the exact read-only vnode that was hashed before spawn. CDHash alone is not a
// complete file identity because signed Mach-O files may contain unhashed
// regions and multiple CodeDirectories.
func loadedDarwinExecutableMatchesOpenedFile(
	ctx context.Context,
	pid int,
	staged *pinnedDarwinObject,
	expectedHash [32]byte,
) bool {
	if pid <= 0 || staged == nil || staged.file == nil || darwinContextFailure(ctx) != nil {
		return false
	}
	loadedPath, err := loadedDarwinProcessPath(pid)
	if err != nil {
		return false
	}
	loaded, err := openPinnedDarwinObject(loadedPath, false)
	if err != nil {
		return false
	}
	defer loaded.file.Close()
	if loaded.stat.Dev != staged.stat.Dev || loaded.stat.Ino != staged.stat.Ino ||
		loaded.stat.Size != staged.stat.Size || loaded.stat.Mode != staged.stat.Mode ||
		loaded.stat.Mtim != staged.stat.Mtim || loaded.stat.Ctim != staged.stat.Ctim {
		return false
	}
	loadedHash, loadedHashErr := sha256OpenedFile(ctx, loaded.file)
	stagedHash, stagedHashErr := sha256OpenedFile(ctx, staged.file)
	return loadedHashErr == nil && stagedHashErr == nil &&
		equalDarwinDigest(loadedHash[:], expectedHash[:]) &&
		equalDarwinDigest(stagedHash[:], expectedHash[:]) &&
		pinnedDarwinObjectUnchanged(loaded) && pinnedDarwinObjectUnchanged(staged)
}

func loadedDarwinProcessPath(pid int) (string, error) {
	if pid <= 0 {
		return "", errors.New("invalid process id")
	}
	buffer := make([]byte, maxDarwinProcessPathBytes)
	count := darwinLibcIntPointerUint(
		libc_proc_pidpath_trampoline_addr,
		pid,
		unsafe.Pointer(&buffer[0]),
		uintptr(len(buffer)),
	)
	if count <= 0 || int(count) >= len(buffer) {
		return "", errors.New("process path unavailable")
	}
	pathBytes := buffer[:count]
	if terminator := bytes.IndexByte(pathBytes, 0); terminator >= 0 {
		pathBytes = pathBytes[:terminator]
	}
	if len(pathBytes) == 0 {
		return "", errors.New("process path unavailable")
	}
	return string(pathBytes), nil
}

func darwinLibcIntPointerUint(function uintptr, argument1 int, argument2 unsafe.Pointer, argument3 uintptr) int32 {
	if function == 0 || argument1 <= 0 || argument2 == nil || argument3 == 0 {
		return -1
	}
	result, _, _ := darwinSyscall6(function, uintptr(argument1), uintptr(argument2), argument3, 0, 0, 0)
	runtime.KeepAlive(argument2)
	return int32(result)
}

var libc_proc_pidpath_trampoline_addr uintptr

//go:cgo_import_dynamic libc_proc_pidpath proc_pidpath "/usr/lib/libproc.dylib"
