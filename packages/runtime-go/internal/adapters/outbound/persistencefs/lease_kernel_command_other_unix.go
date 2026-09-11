//go:build !windows && !darwin && !linux

package persistencefs

import "golang.org/x/sys/unix"

const persistenceKernelLockCommand = unix.F_SETLK
