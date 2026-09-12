package processsandbox

import (
	"testing"

	"analytix.local/runtime-go/internal/testsupport/userconfigtest"
)

func TestMain(m *testing.M) {
	// Match production's canonical root inputs and isolate all child processes
	// from the invoking user's home/configuration, including Git configuration.
	userconfigtest.Run(m)
}
