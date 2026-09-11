//go:build darwin

package processauthority

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	posixSpawnStartSuspended = 0x0080
	posixSpawnSetSID         = 0x0400
	posixSpawnCloseOnExec    = 0x4000
	darwinWaitIDProcess      = 1
	darwinWaitIDExited       = 0x00000004
	darwinWaitIDNoReap       = 0x00000020
	darwinWaitIDNoHang       = 0x00000001
)

var errDarwinSpawnUnavailable = errors.New("darwin suspended spawn unavailable")

// spawnSuspended starts an already selected executable without allowing its
// first instruction to run. The caller owns identity verification and must
// either SIGCONT the returned pid or kill and wait for it.
func spawnSuspended(
	executable string,
	argv []string,
	environment []string,
	cwdFD int,
	stdinFD int,
	stdoutFD int,
	stderrFD int,
) (int, error) {
	return spawnSuspendedWithFiles(
		executable, argv, environment, cwdFD, stdinFD, stdoutFD, stderrFD, true, nil,
	)
}

func spawnSuspendedInInheritedProcessGroup(
	executable string,
	argv []string,
	environment []string,
	cwdFD int,
	stdinFD int,
	stdoutFD int,
	stderrFD int,
) (int, error) {
	return spawnSuspendedWithFiles(
		executable, argv, environment, cwdFD, stdinFD, stdoutFD, stderrFD, false, nil,
	)
}

type darwinSpawnFile struct {
	source int
	target int
}

func spawnSuspendedWithExtraFiles(
	executable string,
	argv []string,
	environment []string,
	cwdFD int,
	stdinFD int,
	stdoutFD int,
	stderrFD int,
	extra []darwinSpawnFile,
) (int, error) {
	return spawnSuspendedWithFiles(
		executable, argv, environment, cwdFD, stdinFD, stdoutFD, stderrFD, true, extra,
	)
}

// spawnDirectWithExtraFiles starts the already admitted production target in
// a fresh process group without POSIX_SPAWN_START_SUSPENDED. The target owns
// its first-instruction bootstrap and must establish its fixed containment and
// descriptor inventory before emitting readiness or reading stdin.
func spawnDirectWithExtraFiles(
	executable string,
	argv []string,
	environment []string,
	cwdFD int,
	stdinFD int,
	stdoutFD int,
	stderrFD int,
	extra []darwinSpawnFile,
) (int, error) {
	return spawnDarwinWithFiles(
		executable, argv, environment, cwdFD, stdinFD, stdoutFD, stderrFD, true, false, extra,
	)
}

func spawnSuspendedWithFiles(
	executable string,
	argv []string,
	environment []string,
	cwdFD int,
	stdinFD int,
	stdoutFD int,
	stderrFD int,
	setSID bool,
	extra []darwinSpawnFile,
) (int, error) {
	return spawnDarwinWithFiles(
		executable, argv, environment, cwdFD, stdinFD, stdoutFD, stderrFD, setSID, true, extra,
	)
}

func spawnDarwinWithFiles(
	executable string,
	argv []string,
	environment []string,
	cwdFD int,
	stdinFD int,
	stdoutFD int,
	stderrFD int,
	setSID bool,
	startSuspended bool,
	extra []darwinSpawnFile,
) (int, error) {
	path, err := syscall.BytePtrFromString(executable)
	if err != nil || len(argv) == 0 || cwdFD < 0 || stdinFD < 0 || stdoutFD < 0 || stderrFD < 0 {
		return 0, errDarwinSpawnUnavailable
	}
	argvPointers, err := darwinCStringVector(argv)
	if err != nil {
		return 0, errDarwinSpawnUnavailable
	}
	seenTargets := map[int]struct{}{0: {}, 1: {}, 2: {}}
	maximumTarget := 2
	sources := []int{cwdFD, stdinFD, stdoutFD, stderrFD}
	for _, descriptor := range extra {
		if descriptor.source < 0 || descriptor.target < 3 || descriptor.target > 64 {
			return 0, errDarwinSpawnUnavailable
		}
		if _, duplicate := seenTargets[descriptor.target]; duplicate {
			return 0, errDarwinSpawnUnavailable
		}
		seenTargets[descriptor.target] = struct{}{}
		if descriptor.target > maximumTarget {
			maximumTarget = descriptor.target
		}
		sources = append(sources, descriptor.source)
	}
	spawnFDs, err := duplicateDarwinSpawnFileDescriptorsAbove(maximumTarget+16, sources...)
	if err != nil {
		return 0, errDarwinSpawnUnavailable
	}
	defer func() {
		for _, descriptor := range spawnFDs {
			_ = unix.Close(descriptor)
		}
	}()
	cwdFD, stdinFD, stdoutFD, stderrFD = spawnFDs[0], spawnFDs[1], spawnFDs[2], spawnFDs[3]
	spawnExtra := make([]darwinSpawnFile, len(extra))
	for index, descriptor := range extra {
		spawnExtra[index] = darwinSpawnFile{source: spawnFDs[index+4], target: descriptor.target}
	}
	environmentPointers, err := darwinCStringVector(environment)
	if err != nil {
		return 0, errDarwinSpawnUnavailable
	}

	var attributes unsafe.Pointer
	if result := darwinLibcPointer1(libc_posix_spawnattr_init_trampoline_addr, unsafe.Pointer(&attributes)); result != 0 || attributes == nil {
		return 0, fmt.Errorf("%w: attr_init=%d", errDarwinSpawnUnavailable, result)
	}
	defer func() {
		_ = darwinLibcPointer1(libc_posix_spawnattr_destroy_trampoline_addr, unsafe.Pointer(&attributes))
	}()
	flags := int16(posixSpawnCloseOnExec)
	if startSuspended {
		flags |= posixSpawnStartSuspended
	}
	if setSID {
		flags |= posixSpawnSetSID
	}
	if result := darwinLibcPointerUint(
		libc_posix_spawnattr_setflags_trampoline_addr,
		unsafe.Pointer(&attributes),
		uintptr(flags),
	); result != 0 {
		return 0, fmt.Errorf("%w: attr_flags=%d", errDarwinSpawnUnavailable, result)
	}

	var actions unsafe.Pointer
	if result := darwinLibcPointer1(libc_posix_spawn_file_actions_init_trampoline_addr, unsafe.Pointer(&actions)); result != 0 || actions == nil {
		return 0, fmt.Errorf("%w: actions_init=%d", errDarwinSpawnUnavailable, result)
	}
	defer func() {
		_ = darwinLibcPointer1(libc_posix_spawn_file_actions_destroy_trampoline_addr, unsafe.Pointer(&actions))
	}()
	for _, descriptor := range [][2]int{{stdinFD, 0}, {stdoutFD, 1}, {stderrFD, 2}} {
		if result := darwinLibcPointerUint2(
			libc_posix_spawn_file_actions_adddup2_trampoline_addr,
			unsafe.Pointer(&actions),
			uintptr(descriptor[0]),
			uintptr(descriptor[1]),
		); result != 0 {
			return 0, fmt.Errorf("%w: actions_dup2=%d", errDarwinSpawnUnavailable, result)
		}
	}
	for _, descriptor := range spawnExtra {
		if result := darwinLibcPointerUint2(
			libc_posix_spawn_file_actions_adddup2_trampoline_addr,
			unsafe.Pointer(&actions),
			uintptr(descriptor.source),
			uintptr(descriptor.target),
		); result != 0 {
			return 0, fmt.Errorf("%w: actions_extra_dup2=%d", errDarwinSpawnUnavailable, result)
		}
	}
	if result := darwinLibcPointerUint(
		libc_posix_spawn_file_actions_addfchdir_np_trampoline_addr,
		unsafe.Pointer(&actions),
		uintptr(cwdFD),
	); result != 0 {
		return 0, fmt.Errorf("%w: actions_fchdir=%d", errDarwinSpawnUnavailable, result)
	}

	var pid int32
	result := darwinLibcPointers6(
		libc_posix_spawn_trampoline_addr,
		unsafe.Pointer(&pid),
		unsafe.Pointer(path),
		unsafe.Pointer(&actions),
		unsafe.Pointer(&attributes),
		unsafe.Pointer(&argvPointers[0]),
		unsafe.Pointer(&environmentPointers[0]),
	)
	runtime.KeepAlive(path)
	runtime.KeepAlive(argvPointers)
	runtime.KeepAlive(environmentPointers)
	if result != 0 || pid <= 0 {
		return 0, fmt.Errorf("%w: spawn=%d", errDarwinSpawnUnavailable, result)
	}
	return int(pid), nil
}

func duplicateDarwinSpawnFileDescriptors(descriptors ...int) ([]int, error) {
	return duplicateDarwinSpawnFileDescriptorsAbove(3, descriptors...)
}

func duplicateDarwinSpawnFileDescriptorsAbove(minimum int, descriptors ...int) ([]int, error) {
	if minimum < 3 {
		return nil, errDarwinSpawnUnavailable
	}
	duplicated := make([]int, 0, len(descriptors))
	for _, descriptor := range descriptors {
		if descriptor < 0 {
			for _, opened := range duplicated {
				_ = unix.Close(opened)
			}
			return nil, errDarwinSpawnUnavailable
		}
		opened, err := unix.FcntlInt(uintptr(descriptor), unix.F_DUPFD_CLOEXEC, minimum)
		if err != nil || opened < minimum {
			if err == nil && opened >= 0 {
				_ = unix.Close(opened)
			}
			for _, prior := range duplicated {
				_ = unix.Close(prior)
			}
			return nil, errDarwinSpawnUnavailable
		}
		duplicated = append(duplicated, opened)
	}
	return duplicated, nil
}

func darwinCStringVector(values []string) ([]*byte, error) {
	pointers := make([]*byte, len(values)+1)
	for index, value := range values {
		pointer, err := syscall.BytePtrFromString(value)
		if err != nil {
			return nil, err
		}
		pointers[index] = pointer
	}
	return pointers, nil
}

func darwinLibcPointer1(function uintptr, argument1 unsafe.Pointer) int32 {
	if function == 0 || argument1 == nil {
		return -1
	}
	result, _, _ := darwinSyscall6(
		function,
		uintptr(argument1),
		0,
		0,
		0,
		0,
		0,
	)
	runtime.KeepAlive(argument1)
	return int32(result)
}

func darwinLibcPointerUint(function uintptr, argument1 unsafe.Pointer, argument2 uintptr) int32 {
	if function == 0 || argument1 == nil {
		return -1
	}
	result, _, _ := darwinSyscall6(function, uintptr(argument1), argument2, 0, 0, 0, 0)
	runtime.KeepAlive(argument1)
	return int32(result)
}

func darwinLibcPointerUint2(function uintptr, argument1 unsafe.Pointer, argument2, argument3 uintptr) int32 {
	if function == 0 || argument1 == nil {
		return -1
	}
	result, _, _ := darwinSyscall6(function, uintptr(argument1), argument2, argument3, 0, 0, 0)
	runtime.KeepAlive(argument1)
	return int32(result)
}

func darwinLibcPointers6(function uintptr, argument1, argument2, argument3, argument4, argument5, argument6 unsafe.Pointer) int32 {
	if function == 0 || argument1 == nil || argument2 == nil || argument3 == nil || argument4 == nil || argument5 == nil || argument6 == nil {
		return -1
	}
	result, _, _ := darwinSyscall6(
		function,
		uintptr(argument1),
		uintptr(argument2),
		uintptr(argument3),
		uintptr(argument4),
		uintptr(argument5),
		uintptr(argument6),
	)
	runtime.KeepAlive(argument1)
	runtime.KeepAlive(argument2)
	runtime.KeepAlive(argument3)
	runtime.KeepAlive(argument4)
	runtime.KeepAlive(argument5)
	runtime.KeepAlive(argument6)
	return int32(result)
}

// observeDarwinProcessExit waits for pid to exit but deliberately leaves the
// leader waitable. Keeping the zombie reserved prevents pid reuse while the
// caller kills the process group and only then reaps the leader.
func observeDarwinProcessExit(pid int) error {
	if pid <= 0 {
		return errDarwinSpawnUnavailable
	}
	var signalInfo [128]byte
	for {
		result, _, errno := darwinSyscall6(
			libc_waitid_trampoline_addr,
			darwinWaitIDProcess,
			uintptr(pid),
			uintptr(unsafe.Pointer(&signalInfo[0])),
			darwinWaitIDExited|darwinWaitIDNoReap,
			0,
			0,
		)
		runtime.KeepAlive(&signalInfo)
		if int64(result) == 0 {
			return nil
		}
		if errno == syscall.EINTR {
			continue
		}
		return fmt.Errorf("%w: waitid=%d", errDarwinSpawnUnavailable, errno)
	}
}

// pollDarwinProcessExit observes an exited child without reaping it. Keeping
// the leader waitable reserves the pid/process-group identity until the caller
// has issued its final group kill and then calls Process.Wait.
func pollDarwinProcessExit(pid int) (bool, error) {
	if pid <= 0 {
		return false, errDarwinSpawnUnavailable
	}
	var signalInfo [128]byte
	result, _, errno := darwinSyscall6(
		libc_waitid_trampoline_addr,
		darwinWaitIDProcess,
		uintptr(pid),
		uintptr(unsafe.Pointer(&signalInfo[0])),
		darwinWaitIDExited|darwinWaitIDNoReap|darwinWaitIDNoHang,
		0,
		0,
	)
	runtime.KeepAlive(&signalInfo)
	if int64(result) != 0 {
		if errno == syscall.EINTR {
			return false, nil
		}
		return false, fmt.Errorf("%w: waitid=%d", errDarwinSpawnUnavailable, errno)
	}
	for _, value := range signalInfo {
		if value != 0 {
			return true, nil
		}
	}
	return false, nil
}

func waitDarwinProcessExitUntil(pid int, deadline time.Time) error {
	if pid <= 0 || deadline.IsZero() {
		return errDarwinSpawnUnavailable
	}
	for {
		exited, err := pollDarwinProcessExit(pid)
		if err != nil || exited {
			return err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return errDarwinSpawnUnavailable
		}
		if remaining > 2*time.Millisecond {
			remaining = 2 * time.Millisecond
		}
		time.Sleep(remaining)
	}
}

func darwinSyscall6(function, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, errno syscall.Errno)

//go:linkname darwinSyscall6 syscall.syscall6

var libc_posix_spawn_trampoline_addr uintptr

//go:cgo_import_dynamic libc_posix_spawn posix_spawn "/usr/lib/libSystem.B.dylib"

var libc_posix_spawnattr_init_trampoline_addr uintptr

//go:cgo_import_dynamic libc_posix_spawnattr_init posix_spawnattr_init "/usr/lib/libSystem.B.dylib"

var libc_posix_spawnattr_destroy_trampoline_addr uintptr

//go:cgo_import_dynamic libc_posix_spawnattr_destroy posix_spawnattr_destroy "/usr/lib/libSystem.B.dylib"

var libc_posix_spawnattr_setflags_trampoline_addr uintptr

//go:cgo_import_dynamic libc_posix_spawnattr_setflags posix_spawnattr_setflags "/usr/lib/libSystem.B.dylib"

var libc_posix_spawn_file_actions_init_trampoline_addr uintptr

//go:cgo_import_dynamic libc_posix_spawn_file_actions_init posix_spawn_file_actions_init "/usr/lib/libSystem.B.dylib"

var libc_posix_spawn_file_actions_destroy_trampoline_addr uintptr

//go:cgo_import_dynamic libc_posix_spawn_file_actions_destroy posix_spawn_file_actions_destroy "/usr/lib/libSystem.B.dylib"

var libc_posix_spawn_file_actions_adddup2_trampoline_addr uintptr

//go:cgo_import_dynamic libc_posix_spawn_file_actions_adddup2 posix_spawn_file_actions_adddup2 "/usr/lib/libSystem.B.dylib"

var libc_posix_spawn_file_actions_addfchdir_np_trampoline_addr uintptr

//go:cgo_import_dynamic libc_posix_spawn_file_actions_addfchdir_np posix_spawn_file_actions_addfchdir_np "/usr/lib/libSystem.B.dylib"

var libc_waitid_trampoline_addr uintptr

//go:cgo_import_dynamic libc_waitid waitid "/usr/lib/libSystem.B.dylib"
