package main

import (
	"bytes"
	"errors"
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

func TestStartupPrepareFailureTraceRemainsOptInAndClosed(t *testing.T) {
	var output bytes.Buffer
	started := time.Now().Add(-time.Millisecond)
	cause := errors.New("/private/unsafe cause must not be emitted")
	t.Setenv("ANALYTIX_STARTUP_TRACE", "0")
	emitStartupPrepareFailureV1(&output, started, cause)
	if output.Len() != 0 {
		t.Fatal("failure diagnostic emitted without opt-in")
	}
	t.Setenv("ANALYTIX_STARTUP_TRACE", "1")
	emitStartupPrepareFailureV1(&output, started, cause)
	if !regexp.MustCompile(`^ANALYTIX_STARTUP_OWNER_PHASE_V1 go_prepare_failed_unknown [0-9]+\n$`).Match(output.Bytes()) {
		t.Fatal("unknown cause was not projected as one closed marker")
	}
	output.Reset()
	emitStartupPrepareFailureV1(&output, started, nil)
	emitStartupOwnerPhaseV1(&output, started, startupOwnerPhaseV1("go_prepare_failed_/private"))
	emitStartupOwnerPhaseV1(&output, started, startupOwnerPhaseV1("go_prepare_failed_unknown extra"))
	if output.Len() != 0 {
		t.Fatal("nil or unlisted failure code was emitted")
	}
}

func TestStartupPersistenceRecoverySubstageTraceIsClosed(t *testing.T) {
	codes := []string{
		"persistence_authority_advance_unknown",
		"persistence_report_preservation_unknown",
		"persistence_final_history_bind_unknown",
		"persistence_retained_original_unknown",
		"persistence_journal_load_unknown",
		"persistence_child_revalidate_unknown",
		"persistence_authority_revalidate_unknown",
		"persistence_signed_recovery_validation_unknown",
		"persistence_create_residues_unknown",
		"persistence_semantic_recovery_unknown",
		"persistence_orphan_topology_unknown",
		"persistence_owner_recovery_unknown",
		"persistence_checkpoint_recovery_unknown",
	}
	var output bytes.Buffer
	started := time.Now().Add(-time.Millisecond)
	t.Setenv("ANALYTIX_STARTUP_TRACE", "0")
	for _, code := range codes {
		emitStartupOwnerPhaseV1(&output, started, startupOwnerPhaseV1("go_prepare_failed_"+code))
	}
	if output.Len() != 0 {
		t.Fatal("persistence recovery substage was emitted without opt-in")
	}
	t.Setenv("ANALYTIX_STARTUP_TRACE", "1")
	for _, code := range codes {
		output.Reset()
		phase := "go_prepare_failed_" + code
		emitStartupOwnerPhaseV1(&output, started, startupOwnerPhaseV1(phase))
		if !regexp.MustCompile(`^ANALYTIX_STARTUP_OWNER_PHASE_V1 ` + regexp.QuoteMeta(phase) + ` [0-9]+\n$`).Match(output.Bytes()) {
			t.Fatalf("persistence recovery substage %s was not one fixed marker", code)
		}
		for _, invalid := range []string{code + "/private/path", code + ` {"body":"private response"}`, " " + code, code + " ", code + "\n"} {
			output.Reset()
			emitStartupOwnerPhaseV1(&output, started, startupOwnerPhaseV1("go_prepare_failed_"+invalid))
			if output.Len() != 0 {
				t.Fatalf("noncanonical persistence recovery substage for %s was emitted", code)
			}
		}
	}
	for _, code := range []string{"", "persistence_unknown", "persistence_authority_advance_other"} {
		output.Reset()
		emitStartupOwnerPhaseV1(&output, started, startupOwnerPhaseV1("go_prepare_failed_"+code))
		if output.Len() != 0 {
			t.Fatal("unlisted persistence recovery substage was emitted")
		}
	}
}
