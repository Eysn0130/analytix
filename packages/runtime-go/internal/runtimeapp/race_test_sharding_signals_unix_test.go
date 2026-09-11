//go:build race && !windows

package runtimeapp

import (
	"os"
	"syscall"
)

func runtimeAppRaceTestTerminationSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT}
}
