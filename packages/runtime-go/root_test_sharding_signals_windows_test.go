//go:build windows

package runtimego

import "os"

func runtimeServerRootTestTerminationSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
