package runtimeapp

import (
	"os"
	"testing"

	"analytix.local/runtime-go/internal/testsupport/userconfigtest"
)

func TestMain(m *testing.M) {
	if code, handled := runRuntimeAppRaceTestShards(); handled {
		os.Exit(code)
	}
	userconfigtest.Run(m)
}
