package main

import (
	"fmt"
	"io"
	"os"
	"time"
)

const startupOwnerPhasePrefixV1 = "ANALYTIX_STARTUP_OWNER_PHASE_V1 "

type startupOwnerPhaseV1 string

const (
	startupFundsPackageInspectedV1 startupOwnerPhaseV1 = "funds_package_inspected"
	startupFundsSourceInspectedV1  startupOwnerPhaseV1 = "funds_source_inspected"
	startupFundsStaticAdmittedV1   startupOwnerPhaseV1 = "funds_static_admitted"
	startupFundsActiveResolvedV1   startupOwnerPhaseV1 = "funds_active_resolved"
	startupFundsReadyBuiltV1       startupOwnerPhaseV1 = "funds_ready_built"
	startupGoLeaseAcquiredV1       startupOwnerPhaseV1 = "go_lease_acquired"
	startupGoSemanticPreparedV1    startupOwnerPhaseV1 = "go_semantic_prepared"
	startupGoListenerBoundV1       startupOwnerPhaseV1 = "go_listener_bound"
	startupGoActivatedV1           startupOwnerPhaseV1 = "go_activated"
	startupGoHostProbedV1          startupOwnerPhaseV1 = "go_host_probed"
	startupGoReadyBuiltV1          startupOwnerPhaseV1 = "go_ready_built"
)

func emitStartupOwnerPhaseV1(output io.Writer, started time.Time, phase startupOwnerPhaseV1) {
	if os.Getenv("ANALYTIX_STARTUP_TRACE") != "1" || output == nil || started.IsZero() {
		return
	}
	switch phase {
	case startupFundsPackageInspectedV1, startupFundsSourceInspectedV1,
		startupFundsStaticAdmittedV1, startupFundsActiveResolvedV1, startupFundsReadyBuiltV1,
		startupGoLeaseAcquiredV1, startupGoSemanticPreparedV1, startupGoListenerBoundV1,
		startupGoActivatedV1, startupGoHostProbedV1, startupGoReadyBuiltV1:
	default:
		return
	}
	elapsed := time.Since(started).Milliseconds()
	if elapsed < 0 || elapsed > int64((30*time.Minute)/time.Millisecond) {
		return
	}
	// This opt-in diagnostic has a fixed phase and integer duration only. It is
	// never part of the readiness or package authority protocol.
	_, _ = fmt.Fprintf(output, "%s%s %d\n", startupOwnerPhasePrefixV1, phase, elapsed)
}
