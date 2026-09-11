//go:build darwin && !analytix_prod

package processauthority

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestDarwinProcInfoReadsExactCurrentProcessIdentity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	state, err := readDarwinProcessState(ctx, os.Getpid(), true)
	if err != nil {
		t.Fatalf("read current process: %v", err)
	}
	if state.Identity.PID != os.Getpid() || state.Identity.UID != uint32(os.Geteuid()) ||
		state.Identity.PPID <= 0 || state.Identity.PGID <= 0 || state.Identity.StartSec == 0 {
		t.Fatalf("current process identity = %#v", state)
	}
	if _, err := listDarwinDirectChildren(ctx, state.Identity); err != nil {
		t.Fatalf("list current process children: %v", err)
	}
}

func TestDarwinSessionTreeFreezesAndResumesExactRoot(t *testing.T) {
	resetDarwinSessionAuthority(t)
	opened, stagingRoot := openDarwinTestSession(t, "session")
	session, ok := opened.(*darwinSession)
	if !ok {
		t.Fatalf("session type = %T", opened)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := session.tree.Freeze(ctx); err != nil {
		t.Fatalf("freeze root: %v root=%#v nodes=%#v", err, session.tree.root, session.tree.nodes)
	}
	if err := session.tree.RequireNoDescendants(); err != nil {
		t.Fatalf("unexpected descendants: %v nodes=%#v", err, session.tree.nodes)
	}
	if err := session.tree.ResumeRoot(ctx); err != nil {
		t.Fatalf("resume root: %v", err)
	}
	if err := session.acquire(ctx); err != nil {
		t.Fatalf("acquire session: %v", err)
	}
	frame, err := session.readFrameLocked(ctx, 4096)
	if err != nil {
		session.release()
		t.Fatalf("read readiness: %v", err)
	}
	if err := session.tree.Freeze(ctx); err != nil {
		session.release()
		t.Fatalf("freeze after readiness: %v", err)
	}
	if err := session.tree.RequireNoDescendants(); err != nil {
		session.release()
		t.Fatalf("descendants after readiness: %v", err)
	}
	if !session.healthyFrozenLocked(ctx) {
		session.release()
		t.Fatal("frozen session reported unhealthy")
	}
	if session.stdoutReader.Buffered() != 0 || len(frame) == 0 {
		session.release()
		t.Fatalf("readiness frame/buffer = %d/%d", len(frame), session.stdoutReader.Buffered())
	}
	if err := session.tree.ResumeRoot(ctx); err != nil {
		session.release()
		t.Fatalf("resume after readiness: %v", err)
	}
	if session.unhealthyLocked() {
		session.release()
		t.Fatal("resumed session reported unhealthy")
	}
	session.state = darwinSessionReady
	session.release()
	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}
	assertDarwinStageEmpty(t, stagingRoot)
}

func TestDarwinChildListingExpandsExactCapacityBeforeAccepting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	parent := darwinFakeProcessInfo(100, 1, 100, darwinProcStatusStopped, 1)
	states := map[int]darwinProcBSDInfo{100: parent}
	childIDs := make([]int32, darwinInitialChildCapacity)
	for index := range childIDs {
		pid := int32(200 + index)
		childIDs[index] = pid
		states[int(pid)] = darwinFakeProcessInfo(uint32(pid), 100, uint32(pid), darwinProcStatusStopped, uint64(index+2))
	}
	var listBufferSizes []int
	caller := func(callNumber, pid, flavor int, argument uint64, buffer unsafe.Pointer, bufferSize int) (int, syscall.Errno) {
		switch callNumber {
		case darwinProcInfoCallPIDInfo:
			info, ok := states[pid]
			if !ok || flavor != darwinProcPIDTBSDInfo || argument != 1 || bufferSize != darwinProcBSDInfoBytes {
				return -1, syscall.ESRCH
			}
			*(*darwinProcBSDInfo)(buffer) = info
			return darwinProcBSDInfoBytes, 0
		case darwinProcInfoCallListPIDs:
			if pid != darwinProcPPIDOnly || flavor != 100 {
				return -1, syscall.EINVAL
			}
			listBufferSizes = append(listBufferSizes, bufferSize)
			output := unsafe.Slice((*int32)(buffer), bufferSize/4)
			copy(output, childIDs)
			return len(childIDs) * 4, 0
		default:
			return -1, syscall.EINVAL
		}
	}
	children, err := listDarwinDirectChildrenWith(ctx, darwinIdentityFromInfo(parent), caller)
	if err != nil || len(children) != len(childIDs) {
		t.Fatalf("expanded children = %d err=%v", len(children), err)
	}
	if len(listBufferSizes) != 2 || listBufferSizes[0] != darwinInitialChildCapacity*4 ||
		listBufferSizes[1] != darwinInitialChildCapacity*8 {
		t.Fatalf("list buffer sizes = %v", listBufferSizes)
	}
}

func TestDarwinChildListingRejectsMaximumCapacityAsPossiblyTruncated(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	parent := darwinFakeProcessInfo(100, 1, 100, darwinProcStatusStopped, 1)
	listCalls := 0
	caller := func(callNumber, pid, flavor int, argument uint64, buffer unsafe.Pointer, bufferSize int) (int, syscall.Errno) {
		if callNumber == darwinProcInfoCallPIDInfo {
			*(*darwinProcBSDInfo)(buffer) = parent
			return darwinProcBSDInfoBytes, 0
		}
		if callNumber != darwinProcInfoCallListPIDs || pid != darwinProcPPIDOnly || flavor != 100 {
			return -1, syscall.EINVAL
		}
		listCalls++
		output := unsafe.Slice((*int32)(buffer), bufferSize/4)
		for index := range output {
			output[index] = int32(index + 200)
		}
		return bufferSize, 0
	}
	if children, err := listDarwinDirectChildrenWith(ctx, darwinIdentityFromInfo(parent), caller); err == nil || children != nil {
		t.Fatalf("possibly truncated list survived: children=%v err=%v", children, err)
	}
	if listCalls != 7 {
		t.Fatalf("list calls = %d, want capacities 16 through 1024", listCalls)
	}
}

func TestDarwinChildListingRejectsMalformedDuplicateAndMismatchedRows(t *testing.T) {
	for _, fixture := range []struct {
		name        string
		result      int
		childIDs    []int32
		childParent uint32
	}{
		{name: "non integral bytes", result: 3},
		{name: "duplicate pid", result: 8, childIDs: []int32{200, 200}, childParent: 100},
		{name: "self cycle", result: 4, childIDs: []int32{100}, childParent: 100},
		{name: "ppid mismatch", result: 4, childIDs: []int32{200}, childParent: 99},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			parent := darwinFakeProcessInfo(100, 1, 100, darwinProcStatusStopped, 1)
			child := darwinFakeProcessInfo(200, fixture.childParent, 200, darwinProcStatusStopped, 2)
			caller := func(callNumber, pid, flavor int, argument uint64, buffer unsafe.Pointer, bufferSize int) (int, syscall.Errno) {
				if callNumber == darwinProcInfoCallPIDInfo {
					if pid == 100 {
						*(*darwinProcBSDInfo)(buffer) = parent
					} else {
						*(*darwinProcBSDInfo)(buffer) = child
					}
					return darwinProcBSDInfoBytes, 0
				}
				output := unsafe.Slice((*int32)(buffer), bufferSize/4)
				copy(output, fixture.childIDs)
				return fixture.result, 0
			}
			if children, err := listDarwinDirectChildrenWith(ctx, darwinIdentityFromInfo(parent), caller); err == nil || children != nil {
				t.Fatalf("malformed child table survived: children=%v err=%v", children, err)
			}
		})
	}
}

func TestDarwinIdentityReplacementIsNotAcceptedAsTermination(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	expectedInfo := darwinFakeProcessInfo(200, 100, 200, darwinProcStatusStopped, 2)
	replacement := darwinFakeProcessInfo(200, 1, 200, darwinProcStatusStopped, 3)
	caller := func(callNumber, pid, flavor int, argument uint64, buffer unsafe.Pointer, bufferSize int) (int, syscall.Errno) {
		*(*darwinProcBSDInfo)(buffer) = replacement
		return darwinProcBSDInfoBytes, 0
	}
	if err := waitDarwinIdentityESRCHWith(ctx, darwinIdentityFromInfo(expectedInfo), caller); err == nil {
		t.Fatal("reused pid was accepted as ESRCH termination proof")
	}
}

func darwinFakeProcessInfo(pid, ppid, pgid uint32, status uint32, start uint64) darwinProcBSDInfo {
	return darwinProcBSDInfo{
		Status: status, PID: pid, PPID: ppid, UID: uint32(os.Geteuid()), PGID: pgid,
		StartSec: 1_700_000_000 + start, StartUSec: start,
	}
}

func darwinIdentityFromInfo(info darwinProcBSDInfo) darwinProcessIdentity {
	return darwinProcessIdentity{
		PID: int(info.PID), PPID: int(info.PPID), PGID: int(info.PGID), UID: info.UID,
		StartSec: info.StartSec, StartUSec: info.StartUSec,
	}
}
