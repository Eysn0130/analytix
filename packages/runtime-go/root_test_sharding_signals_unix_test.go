//go:build !windows

package runtimego

import (
	"os"
	"syscall"
)

func runtimeServerRootTestTerminationSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT}
}
