//go:build !darwin && !linux && !windows

package persistencefs

import (
	"context"
	"errors"
)

func secureRemovePrivateTree(string, string) error {
	return errors.New("private tree removal is unsupported on this platform")
}

func secureRemovePrivateTreeContext(context.Context, string, string) error {
	return errors.New("private tree removal is unsupported on this platform")
}
