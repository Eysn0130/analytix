//go:build linux

package persistencefs

import "golang.org/x/sys/unix"

// Open-file-description locks survive unrelated closes of the same device in
// the process and are available on every supported production Linux kernel.
const persistenceKernelLockCommand = unix.F_OFD_SETLK
