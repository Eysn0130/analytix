//go:build race && windows

package runtimeapp

import "os"

func runtimeAppRaceTestTerminationSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
