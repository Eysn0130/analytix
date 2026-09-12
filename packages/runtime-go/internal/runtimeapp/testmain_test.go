package runtimeapp

import (
	"context"
	"errors"
	"os"
	"testing"

	packagedauthorityfs "analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
	"analytix.local/runtime-go/internal/testsupport/userconfigtest"
)

func TestMain(m *testing.M) {
	if code, handled := runRuntimeAppRaceTestShards(); handled {
		os.Exit(code)
	}
	userconfigtest.Run(m)
}

func TestRuntimeTestExecutableIsUnpackagedAndCanonical(t *testing.T) {
	_, err := packagedauthorityfs.InspectCurrentPackageV2(context.Background())
	if !errors.Is(err, packagedauthorityfs.ErrNotPackagedRuntimeV2) {
		t.Fatalf("isolated Go test executable must satisfy canonical unpackaged inspection: %v", err)
	}
}
