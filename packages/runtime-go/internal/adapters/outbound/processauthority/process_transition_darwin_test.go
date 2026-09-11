//go:build darwin

package processauthority

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDarwinProcessTransitionMonitorRetriesInterruptedObservation(t *testing.T) {
	const pid = 4242
	calls := 0
	monitor := &darwinProcessTransitionMonitor{
		kqueue: 1,
		pid:    pid,
		kevent: func(_ int, _ []unix.Kevent_t, _ []unix.Kevent_t, _ *unix.Timespec) (int, error) {
			calls++
			if calls == 1 {
				return 0, unix.EINTR
			}
			return 0, nil
		},
	}

	if !monitor.Unchanged() {
		t.Fatal("an interrupted observation was latched as a process transition")
	}
	if calls != 2 || monitor.violated {
		t.Fatalf("observation calls/violation = %d/%t, want 2/false", calls, monitor.violated)
	}
}

func TestDarwinProcessTransitionMonitorRejectsEventAfterInterruptedObservation(t *testing.T) {
	const pid = 4242
	calls := 0
	monitor := &darwinProcessTransitionMonitor{
		kqueue: 1,
		pid:    pid,
		kevent: func(_ int, _ []unix.Kevent_t, events []unix.Kevent_t, _ *unix.Timespec) (int, error) {
			calls++
			if calls == 1 {
				return 0, unix.EINTR
			}
			events[0] = unix.Kevent_t{
				Ident: uint64(pid), Filter: unix.EVFILT_PROC, Fflags: unix.NOTE_EXEC,
			}
			return 1, nil
		},
	}

	if monitor.Unchanged() {
		t.Fatal("a real exec event after an interrupted observation was accepted")
	}
	if calls != 2 || !monitor.violated {
		t.Fatalf("observation calls/violation = %d/%t, want 2/true", calls, monitor.violated)
	}
	if monitor.Unchanged() || calls != 2 {
		t.Fatal("a process transition violation was not latched monotonically")
	}
}

func TestDarwinProcessTransitionMonitorFailsClosedAfterRepeatedInterruptions(t *testing.T) {
	calls := 0
	monitor := &darwinProcessTransitionMonitor{
		kqueue: 1,
		pid:    4242,
		kevent: func(_ int, _ []unix.Kevent_t, _ []unix.Kevent_t, _ *unix.Timespec) (int, error) {
			calls++
			return 0, unix.EINTR
		},
	}

	if monitor.Unchanged() {
		t.Fatal("an observation that never completed was accepted")
	}
	if calls != darwinMaximumKeventInterruptions || !monitor.violated {
		t.Fatalf(
			"observation calls/violation = %d/%t, want %d/true",
			calls, monitor.violated, darwinMaximumKeventInterruptions,
		)
	}
}

func TestCallDarwinKeventDoesNotRetryNonInterruptErrors(t *testing.T) {
	calls := 0
	_, err := callDarwinKevent(
		func(_ int, _ []unix.Kevent_t, _ []unix.Kevent_t, _ *unix.Timespec) (int, error) {
			calls++
			return 0, unix.EBADF
		},
		1, nil, nil, nil,
	)
	if !errors.Is(err, unix.EBADF) || calls != 1 {
		t.Fatalf("kevent error/calls = %v/%d, want EBADF/1", err, calls)
	}
}
