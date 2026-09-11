//go:build windows

package finalauthority

import (
	"testing"

	"analytix.local/runtime-go/internal/testsupport/userconfigtest"
)

func TestMain(m *testing.M) {
	userconfigtest.Run(m)
}
