//go:build darwin

package processauthority

import (
	"context"
	"errors"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func waitDarwinProcessGroupESRCH(ctx context.Context, processGroupID int) error {
	if processGroupID <= 0 {
		return ErrTermination
	}
	for {
		if err := darwinContextFailure(ctx); err != nil {
			return err
		}
		err := syscall.Kill(-processGroupID, 0)
		if errors.Is(err, syscall.ESRCH) || errors.Is(err, unix.ESRCH) {
			return nil
		}
		if err != nil {
			return ErrTermination
		}
		time.Sleep(time.Millisecond)
	}
}
