//go:build analytix_native_build_probe && !analytix_prod

package main

import (
	"os"

	nativebuildcli "analytix.local/runtime-go/internal/adapters/inbound/nativebuildcli"
	nativecomponentpublication "analytix.local/runtime-go/internal/adapters/outbound/nativecomponentpublication"
	nativecomponentrunner "analytix.local/runtime-go/internal/adapters/outbound/nativecomponentrunner"
	nativecomponentsnapshot "analytix.local/runtime-go/internal/adapters/outbound/nativecomponentsnapshot"
	processauthority "analytix.local/runtime-go/internal/adapters/outbound/processauthority"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == nativebuildcli.ArgumentV1 {
		os.Exit(nativebuildcli.Main(openNativeBuildHostV1))
	}
	if len(os.Args) == 2 && os.Args[1] == nativecomponentpublication.PublisherArgumentV1 {
		os.Exit(nativecomponentpublication.Main())
	}
	if len(os.Args) == 2 && os.Args[1] == nativecomponentsnapshot.ArgumentV1 {
		os.Exit(nativecomponentsnapshot.Main())
	}
	if len(os.Args) == 2 && os.Args[1] == nativecomponentsnapshot.ArgumentDiscardV1 {
		os.Exit(nativecomponentsnapshot.MainDiscard())
	}
	os.Exit(processauthority.NativeBuildProbeMain(processauthority.BuildProbeProtocol{
		EncodePing:       nativecomponentrunner.EncodeProbePingRequest,
		ValidateReady:    nativecomponentrunner.ValidateProbeReadiness,
		ValidateResponse: nativecomponentrunner.ValidateProbePingResponse,
	}))
}
