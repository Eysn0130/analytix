//go:build darwin

package processauthority

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"syscall"
	"unsafe"
)

const (
	darwinProcInfoCallListPIDs = 1
	darwinProcInfoCallPIDInfo  = 2
	darwinProcPPIDOnly         = 6
	darwinProcPIDTBSDInfo      = 3
	darwinProcStatusStopped    = 4
	darwinProcStatusZombie     = 5
	darwinProcBSDInfoBytes     = 136
	darwinInitialChildCapacity = 16
	darwinMaximumChildCapacity = 1024
)

var errDarwinProcInfo = errors.New("darwin process information unavailable")

// darwinProcBSDInfo mirrors Darwin's proc_bsdinfo ABI on both supported
// arm64 and x86_64 targets. The compile-time size and offset assertions below
// make an SDK/ABI drift a build failure rather than a process-identity bypass.
type darwinProcBSDInfo struct {
	Flags     uint32
	Status    uint32
	XStatus   uint32
	PID       uint32
	PPID      uint32
	UID       uint32
	GID       uint32
	RUID      uint32
	RGID      uint32
	SVUID     uint32
	SVGID     uint32
	RFU1      uint32
	Comm      [16]byte
	Name      [32]byte
	NFiles    uint32
	PGID      uint32
	PJobC     uint32
	EDev      uint32
	ETPGID    uint32
	Nice      int32
	StartSec  uint64
	StartUSec uint64
}

var (
	_ [darwinProcBSDInfoBytes - int(unsafe.Sizeof(darwinProcBSDInfo{}))]byte
	_ [int(unsafe.Sizeof(darwinProcBSDInfo{})) - darwinProcBSDInfoBytes]byte
	_ [4 - int(unsafe.Offsetof(darwinProcBSDInfo{}.Status))]byte
	_ [int(unsafe.Offsetof(darwinProcBSDInfo{}.Status)) - 4]byte
	_ [12 - int(unsafe.Offsetof(darwinProcBSDInfo{}.PID))]byte
	_ [int(unsafe.Offsetof(darwinProcBSDInfo{}.PID)) - 12]byte
	_ [16 - int(unsafe.Offsetof(darwinProcBSDInfo{}.PPID))]byte
	_ [int(unsafe.Offsetof(darwinProcBSDInfo{}.PPID)) - 16]byte
	_ [100 - int(unsafe.Offsetof(darwinProcBSDInfo{}.PGID))]byte
	_ [int(unsafe.Offsetof(darwinProcBSDInfo{}.PGID)) - 100]byte
	_ [120 - int(unsafe.Offsetof(darwinProcBSDInfo{}.StartSec))]byte
	_ [int(unsafe.Offsetof(darwinProcBSDInfo{}.StartSec)) - 120]byte
	_ [128 - int(unsafe.Offsetof(darwinProcBSDInfo{}.StartUSec))]byte
	_ [int(unsafe.Offsetof(darwinProcBSDInfo{}.StartUSec)) - 128]byte
)

type darwinProcessIdentity struct {
	PID       int
	PPID      int
	PGID      int
	UID       uint32
	StartSec  uint64
	StartUSec uint64
}

type darwinProcessState struct {
	Identity darwinProcessIdentity
	Status   uint32
}

func (state darwinProcessState) sameIdentity(identity darwinProcessIdentity) bool {
	return state.Identity == identity
}

type darwinProcInfoCaller func(int, int, int, uint64, unsafe.Pointer, int) (int, syscall.Errno)

func realDarwinProcInfoCaller(callNumber, pid, flavor int, argument uint64, buffer unsafe.Pointer, bufferSize int) (int, syscall.Errno) {
	if callNumber <= 0 || pid <= 0 || flavor < 0 || buffer == nil || bufferSize <= 0 || libc___proc_info_trampoline_addr == 0 {
		return -1, syscall.EINVAL
	}
	result, _, errno := darwinSyscall6(
		libc___proc_info_trampoline_addr,
		uintptr(callNumber),
		uintptr(pid),
		uintptr(flavor),
		uintptr(argument),
		uintptr(buffer),
		uintptr(bufferSize),
	)
	runtime.KeepAlive(buffer)
	return int(int64(result)), errno
}

func readDarwinProcessState(ctx context.Context, pid int, includeZombie bool) (darwinProcessState, error) {
	return readDarwinProcessStateWith(ctx, pid, includeZombie, realDarwinProcInfoCaller)
}

func readDarwinProcessStateWith(
	ctx context.Context,
	pid int,
	includeZombie bool,
	caller darwinProcInfoCaller,
) (darwinProcessState, error) {
	if ctx == nil || pid <= 0 || pid > math.MaxInt32 || caller == nil {
		return darwinProcessState{}, errDarwinProcInfo
	}
	argument := uint64(0)
	if includeZombie {
		argument = 1
	}
	for {
		if err := darwinContextFailure(ctx); err != nil {
			return darwinProcessState{}, err
		}
		var info darwinProcBSDInfo
		result, errno := caller(
			darwinProcInfoCallPIDInfo,
			pid,
			darwinProcPIDTBSDInfo,
			argument,
			unsafe.Pointer(&info),
			darwinProcBSDInfoBytes,
		)
		runtime.KeepAlive(&info)
		if result == -1 && errno == syscall.EINTR {
			continue
		}
		if result == -1 {
			return darwinProcessState{}, fmt.Errorf("%w: %w", errDarwinProcInfo, errno)
		}
		if result != darwinProcBSDInfoBytes || errno != 0 || info.PID != uint32(pid) ||
			info.PPID > math.MaxInt32 || info.PGID == 0 || info.PGID > math.MaxInt32 ||
			info.UID != uint32(os.Geteuid()) || info.Status == 0 || info.Status > darwinProcStatusZombie ||
			(!includeZombie && info.Status == darwinProcStatusZombie) || info.StartSec == 0 || info.StartUSec >= 1_000_000 {
			return darwinProcessState{}, errDarwinProcInfo
		}
		return darwinProcessState{
			Identity: darwinProcessIdentity{
				PID: pid, PPID: int(info.PPID), PGID: int(info.PGID), UID: info.UID,
				StartSec: info.StartSec, StartUSec: info.StartUSec,
			},
			Status: info.Status,
		}, nil
	}
}

func listDarwinDirectChildren(
	ctx context.Context,
	parent darwinProcessIdentity,
) ([]darwinProcessState, error) {
	return listDarwinDirectChildrenWith(ctx, parent, realDarwinProcInfoCaller)
}

func listDarwinDirectChildrenWith(
	ctx context.Context,
	parent darwinProcessIdentity,
	caller darwinProcInfoCaller,
) ([]darwinProcessState, error) {
	if ctx == nil || parent.PID <= 0 || caller == nil {
		return nil, errDarwinProcInfo
	}
	before, err := readDarwinProcessStateWith(ctx, parent.PID, true, caller)
	if err != nil || !before.sameIdentity(parent) {
		return nil, errDarwinProcInfo
	}
	capacity := darwinInitialChildCapacity
	var childIDs []int
	for {
		buffer := make([]int32, capacity)
		result, errno := caller(
			darwinProcInfoCallListPIDs,
			darwinProcPPIDOnly,
			parent.PID,
			0,
			unsafe.Pointer(&buffer[0]),
			len(buffer)*int(unsafe.Sizeof(buffer[0])),
		)
		runtime.KeepAlive(buffer)
		if result == -1 && errno == syscall.EINTR {
			if err := darwinContextFailure(ctx); err != nil {
				return nil, err
			}
			continue
		}
		if result == -1 {
			return nil, fmt.Errorf("%w: %w", errDarwinProcInfo, errno)
		}
		byteCapacity := len(buffer) * int(unsafe.Sizeof(buffer[0]))
		if errno != 0 || result < 0 || result > byteCapacity || result%int(unsafe.Sizeof(buffer[0])) != 0 {
			return nil, errDarwinProcInfo
		}
		if result == byteCapacity {
			if capacity >= darwinMaximumChildCapacity {
				return nil, errDarwinProcInfo
			}
			capacity *= 2
			if capacity > darwinMaximumChildCapacity {
				capacity = darwinMaximumChildCapacity
			}
			continue
		}
		count := result / int(unsafe.Sizeof(buffer[0]))
		childIDs = make([]int, 0, count)
		seen := make(map[int]struct{}, count)
		for _, rawPID := range buffer[:count] {
			childPID := int(rawPID)
			if childPID <= 0 || childPID == parent.PID {
				return nil, errDarwinProcInfo
			}
			if _, duplicate := seen[childPID]; duplicate {
				return nil, errDarwinProcInfo
			}
			seen[childPID] = struct{}{}
			childIDs = append(childIDs, childPID)
		}
		break
	}

	children := make([]darwinProcessState, 0, len(childIDs))
	for _, childPID := range childIDs {
		child, childErr := readDarwinProcessStateWith(ctx, childPID, true, caller)
		if childErr != nil || child.Identity.PPID != parent.PID || child.Identity.UID != parent.UID {
			return nil, errDarwinProcInfo
		}
		children = append(children, child)
	}
	sort.Slice(children, func(left, right int) bool {
		return children[left].Identity.PID < children[right].Identity.PID
	})
	after, err := readDarwinProcessStateWith(ctx, parent.PID, true, caller)
	if err != nil || !after.sameIdentity(parent) {
		return nil, errDarwinProcInfo
	}
	return children, nil
}

var libc___proc_info_trampoline_addr uintptr

//go:cgo_import_dynamic libc___proc_info __proc_info "/usr/lib/libSystem.B.dylib"
