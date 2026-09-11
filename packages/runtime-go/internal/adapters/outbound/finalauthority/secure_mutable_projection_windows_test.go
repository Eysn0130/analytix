//go:build windows

package finalauthority

import (
	"errors"
	"testing"
)

func TestSecureMutableProjectionWindowsFailsClosedUntilDurabilityIsProven(t *testing.T) {
	store, err := OpenSecureMutableProjection(`C:\analytix-test-projection`, "head.json", 4096)
	if store != nil || !errors.Is(err, ErrSecureMutableProjectionUnsupported) {
		t.Fatalf("Windows mutable projection did not fail closed: store=%#v err=%v", store, err)
	}
}
