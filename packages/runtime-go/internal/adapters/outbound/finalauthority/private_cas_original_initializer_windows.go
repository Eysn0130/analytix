//go:build windows

package finalauthority

import (
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	"golang.org/x/sys/windows"
)

func securePrivateCASInitializeOriginalLeafV1(binding privatecasport.RootBinding, initial privateCASOriginalLeafInitializationV1) error {
	handle, err := privateCASWindowsOpenBoundRoot(binding, true, initial)
	if err != nil {
		return err
	}
	return windows.CloseHandle(handle)
}
