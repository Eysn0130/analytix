//go:build !darwin && !linux && !windows

package finalauthority

import (
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	"errors"
)

func securePrivateCASInitializeOriginalLeafV1(privatecasport.RootBinding, privateCASOriginalLeafInitializationV1) error {
	return errors.New("original CAS directory initialization is unsupported on this platform")
}
