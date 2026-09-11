//go:build darwin

package persistencefs

import "golang.org/x/sys/unix"

// Darwin only exposes process-owned POSIX record locks. Production source is
// architecture-guarded so no other component opens the reserved device and
// accidentally releases these ranges by closing an unrelated descriptor.
const persistenceKernelLockCommand = unix.F_SETLK
