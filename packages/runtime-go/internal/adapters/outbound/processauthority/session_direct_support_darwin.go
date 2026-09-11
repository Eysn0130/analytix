//go:build darwin

package processauthority

import (
	"context"
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// duplicateDarwinSessionAuthority pins a process-authority-owned directory
// handle without consuming the caller's handle. SyscallConn serializes the
// raw-descriptor duplication against a concurrent Close, so a caller cannot
// substitute a reused descriptor after OpenSession returns.
func duplicateDarwinSessionAuthority(source *os.File, name string) (*os.File, error) {
	if source == nil || name == "" {
		return nil, ErrRequestInvalid
	}
	raw, err := source.SyscallConn()
	if err != nil {
		return nil, ErrExecutableIdentity
	}
	duplicateFD := -1
	var duplicateErr error
	controlErr := raw.Control(func(fd uintptr) {
		duplicateFD, duplicateErr = unix.FcntlInt(fd, unix.F_DUPFD_CLOEXEC, 8)
	})
	if controlErr != nil || duplicateErr != nil || duplicateFD < 0 {
		if duplicateFD >= 0 {
			_ = unix.Close(duplicateFD)
		}
		return nil, ErrExecutableIdentity
	}
	owned := os.NewFile(uintptr(duplicateFD), name)
	if owned == nil {
		_ = unix.Close(duplicateFD)
		return nil, ErrTermination
	}
	return owned, nil
}

// terminateDirectDarwinProcess never freezes the production target. It kills
// the original process group immediately, reaps the retained direct child,
// and then proves both the exact pid identity and group are gone.
func terminateDirectDarwinProcess(
	ctx context.Context,
	process *os.Process,
	pid int,
	identity darwinProcessIdentity,
) error {
	if ctx == nil || process == nil || pid <= 0 || process.Pid != pid {
		return ErrTermination
	}
	killDarwinProcessGroup(pid)
	if !killAndWaitDarwinProcess(process, pid) {
		return ErrTermination
	}
	failed := false
	if identity.PID == pid {
		if err := waitDarwinIdentityESRCHWith(ctx, identity, realDarwinProcInfoCaller); err != nil &&
			!errors.Is(err, syscall.ESRCH) {
			failed = true
		}
	}
	if waitDarwinProcessGroupESRCH(ctx, pid) != nil {
		failed = true
	}
	if failed {
		return ErrTermination
	}
	return nil
}
