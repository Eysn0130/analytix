package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestStartupOwnerPhaseTraceIsOptInAndFixed(t *testing.T) {
	t.Setenv("ANALYTIX_STARTUP_TRACE", "0")
	var output bytes.Buffer
	started := time.Now().Add(-25 * time.Millisecond)
	emitStartupOwnerPhaseV1(&output, started, startupFundsPackageInspectedV1)
	if output.Len() != 0 {
		t.Fatal("startup phase was emitted without opt-in")
	}
	t.Setenv("ANALYTIX_STARTUP_TRACE", "1")
	emitStartupOwnerPhaseV1(&output, started, startupFundsPackageInspectedV1)
	if !regexp.MustCompile(`^ANALYTIX_STARTUP_OWNER_PHASE_V1 funds_package_inspected [0-9]+\n$`).Match(output.Bytes()) ||
		strings.Contains(output.String(), "/") {
		t.Fatalf("startup phase was not a fixed, value-free marker: %q", output.String())
	}
	output.Reset()
	emitStartupOwnerPhaseV1(&output, started, startupOwnerPhaseV1("unsafe /private/path"))
	if output.Len() != 0 {
		t.Fatal("unlisted startup phase was emitted")
	}
}
