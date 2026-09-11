//go:build darwin

package processauthority

import (
	"errors"
	"sync"

	"golang.org/x/sys/unix"
)

const darwinForbiddenProcessTransitions = unix.NOTE_EXEC | unix.NOTE_FORK | unix.NOTE_EXIT
const darwinMaximumKeventInterruptions = 8

type darwinKeventCaller func(
	kqueue int,
	changes []unix.Kevent_t,
	events []unix.Kevent_t,
	timeout *unix.Timespec,
) (int, error)

// darwinProcessTransitionMonitor is a monotonic kernel-event latch. The legacy
// build-probe path registers it after its authorized frozen handoff; the
// production direct path registers it immediately after the admitted spawn and
// combines it with exact image and descendant checks. Any later exec/fork/exit
// event permanently invalidates the session, including an exec-away/exec-back
// sequence whose final vnode and bytes again match the admitted target.
type darwinProcessTransitionMonitor struct {
	mu       sync.Mutex
	kqueue   int
	pid      int
	kevent   darwinKeventCaller
	violated bool
	closed   bool
}

func openDarwinProcessTransitionMonitor(pid int) (*darwinProcessTransitionMonitor, error) {
	if pid <= 0 {
		return nil, ErrExecutableIdentity
	}
	kqueue, err := unix.Kqueue()
	if err != nil {
		return nil, ErrUnavailable
	}
	monitor := &darwinProcessTransitionMonitor{kqueue: kqueue, pid: pid, kevent: unix.Kevent}
	change := unix.Kevent_t{
		Ident:  uint64(pid),
		Filter: unix.EVFILT_PROC,
		Flags:  unix.EV_ADD | unix.EV_ENABLE | unix.EV_CLEAR,
		Fflags: darwinForbiddenProcessTransitions,
	}
	if _, err := callDarwinKevent(monitor.kevent, kqueue, []unix.Kevent_t{change}, nil, nil); err != nil {
		_ = unix.Close(kqueue)
		return nil, ErrUnavailable
	}
	return monitor, nil
}

func (monitor *darwinProcessTransitionMonitor) Unchanged() bool {
	if monitor == nil {
		return false
	}
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	if monitor.closed || monitor.violated || monitor.kqueue < 0 || monitor.pid <= 0 || monitor.kevent == nil {
		return false
	}
	events := make([]unix.Kevent_t, 4)
	timeout := unix.Timespec{}
	count, err := callDarwinKevent(monitor.kevent, monitor.kqueue, nil, events, &timeout)
	if err != nil {
		monitor.violated = true
		return false
	}
	for index := 0; index < count; index++ {
		event := events[index]
		if int(event.Ident) != monitor.pid || event.Filter != unix.EVFILT_PROC ||
			event.Flags&unix.EV_ERROR != 0 || event.Fflags&darwinForbiddenProcessTransitions != 0 {
			monitor.violated = true
			return false
		}
	}
	return true
}

func callDarwinKevent(
	caller darwinKeventCaller,
	kqueue int,
	changes []unix.Kevent_t,
	events []unix.Kevent_t,
	timeout *unix.Timespec,
) (int, error) {
	if caller == nil {
		return 0, unix.EINVAL
	}
	// EINTR means the kernel observation did not complete; it is not evidence of
	// an exec/fork/exit transition. Retry synchronously, but keep the attempt
	// count bounded so a signal storm still fails closed instead of hanging the
	// session gate indefinitely.
	for attempt := 0; attempt < darwinMaximumKeventInterruptions; attempt++ {
		count, err := caller(kqueue, changes, events, timeout)
		if !errors.Is(err, unix.EINTR) {
			return count, err
		}
	}
	return 0, unix.EINTR
}

func (monitor *darwinProcessTransitionMonitor) Close() error {
	if monitor == nil {
		return nil
	}
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	if monitor.closed {
		return nil
	}
	monitor.closed = true
	kqueue := monitor.kqueue
	monitor.kqueue = -1
	if kqueue < 0 {
		return nil
	}
	return unix.Close(kqueue)
}
