package security

import (
	"strings"
	"testing"
	"time"
)

func TestLegacyDSV1VerifiedSourceProbeIsAuditOnly(t *testing.T) {
	snapshotID := DatasetSnapshotIDPrefixV1 + SHA256Hex([]byte("snapshot-1"))
	response := SourceProbeResponse{
		Version: SourceProbeVersion, ServerName: "analytix_funds", ServerVersion: "0.16.15", CaseID: "case_1234",
		CaseBindingHash: SHA256Hex([]byte("binding")), DatasetSnapshotID: snapshotID, Ready: true, ReadOnly: true,
		CheckedAt: time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	}
	serverIdentity, err := NewVerifiedMCPServerIdentity("analytix_funds", response.ServerName, response.ServerVersion, SHA256Hex([]byte("source-probe-test-instance")), 3)
	if err != nil {
		t.Fatal(err)
	}
	probe, err := NewVerifiedSourceProbe(VerifiedSourceProbeInput{
		ServerID: "analytix_funds", ServerIdentity: serverIdentity, ConnectionEpoch: 3,
		CatalogFingerprint: SHA256Hex([]byte("catalog")), SpecFingerprint: SHA256Hex([]byte("spec")),
		ThreadID: "thread_1", TurnID: "turn_1", ContextEpoch: 2, ContextDigest: SHA256Hex([]byte("context")),
		DatasetSnapshotID: snapshotID, CheckedAt: time.Date(2026, 7, 10, 10, 0, 1, 0, time.UTC), Response: response,
	})
	if err != nil {
		t.Fatal(err)
	}
	if probe.DatasetSnapshotID != snapshotID || probe.ProbeDigest == "" || probe.ThreadID != "thread_1" || probe.TurnID != "turn_1" || probe.ContextEpoch != 2 || probe.CheckedAt != "2026-07-10T10:00:01Z" {
		t.Fatalf("verified probe mismatch: %#v", probe)
	}
	if SourceProbeCanAuthorizeFacts(probe) {
		t.Fatal("structurally valid legacy DSV1 probe authorized case facts")
	}
	if blocker := DatasetSnapshotFactAuthorityBlocker(snapshotID); blocker != SourceProbeBlockerDatasetSnapshotAuthorityUnavailable {
		t.Fatalf("legacy snapshot blocker is not fixed: %q", blocker)
	}
	tampered := probe
	tampered.DatasetSnapshotID = DatasetSnapshotIDPrefixV1 + SHA256Hex([]byte("snapshot-2"))
	if err := ValidateVerifiedSourceProbe(tampered); err == nil {
		t.Fatal("verified source snapshot tamper was accepted")
	}
	missingExpected := VerifiedSourceProbeInput{
		ServerID: "analytix_funds", ServerIdentity: serverIdentity, ConnectionEpoch: 3,
		CatalogFingerprint: SHA256Hex([]byte("catalog")), SpecFingerprint: SHA256Hex([]byte("spec")),
		ThreadID: "thread_1", TurnID: "turn_1", ContextEpoch: 2, ContextDigest: SHA256Hex([]byte("context")), Response: response,
	}
	if _, err := NewVerifiedSourceProbe(missingExpected); err == nil {
		t.Fatal("source probe without a frozen host snapshot was accepted")
	}
	mismatchedExpected := missingExpected
	mismatchedExpected.DatasetSnapshotID = DatasetSnapshotIDPrefixV1 + SHA256Hex([]byte("other-snapshot"))
	if _, err := NewVerifiedSourceProbe(mismatchedExpected); err == nil {
		t.Fatal("plugin-selected snapshot replaced frozen host snapshot")
	}
	nonCanonicalExpected := missingExpected
	nonCanonicalExpected.DatasetSnapshotID = " " + snapshotID
	if _, err := NewVerifiedSourceProbe(nonCanonicalExpected); err == nil {
		t.Fatal("non-canonical frozen host snapshot was accepted")
	}
}

func TestCanonicalDSV2VerifiedSourceProbeIsStructurallyPreservedButNotFactAuthority(t *testing.T) {
	snapshotID := DatasetSnapshotIDPrefixV2 + SHA256Hex([]byte("witness-selected-v2"))
	response := SourceProbeResponse{
		Version: SourceProbeVersion, ServerName: "analytix_funds", ServerVersion: "0.16.16", CaseID: "case_v2",
		CaseBindingHash: SHA256Hex([]byte("binding-v2")), DatasetSnapshotID: snapshotID,
		Ready: true, ReadOnly: true, CheckedAt: "2026-07-23T10:00:00Z",
	}
	identity, err := NewVerifiedMCPServerIdentity(
		"analytix_funds", response.ServerName, response.ServerVersion,
		SHA256Hex([]byte("source-probe-v2-instance")), 8,
	)
	if err != nil {
		t.Fatal(err)
	}
	probe, err := NewVerifiedSourceProbe(VerifiedSourceProbeInput{
		ServerID: "analytix_funds", ServerIdentity: identity, ConnectionEpoch: 8,
		CatalogFingerprint: SHA256Hex([]byte("catalog-v2")), SpecFingerprint: SHA256Hex([]byte("spec-v2")),
		ThreadID: "thread_v2", TurnID: "turn_v2", ContextEpoch: 4, ContextDigest: SHA256Hex([]byte("context-v2")),
		DatasetSnapshotID: snapshotID, CheckedAt: time.Date(2026, 7, 23, 10, 0, 1, 0, time.UTC), Response: response,
	})
	if err != nil || ValidateVerifiedSourceProbe(probe) != nil || probe.DatasetSnapshotID != snapshotID {
		t.Fatalf("canonical DSV2 source probe was not structurally preserved: probe=%#v err=%v", probe, err)
	}
	if SourceProbeCanAuthorizeFacts(probe) || DatasetSnapshotFactAuthorityBlocker(snapshotID) == "" {
		t.Fatal("bare DSV2 source probe bypassed current witnessed AuthorityV2 admission")
	}
	if !SourceProbeEligibleForHostAuthorityV2(probe) {
		t.Fatal("canonical DSV2 source probe lost structural host-authority eligibility")
	}
	if SourceProbeEligibleForHostAuthorityV2(VerifiedSourceProbe{}) {
		t.Fatal("invalid source probe gained structural host-authority eligibility")
	}
}

func TestBareDSV2IdentifierRequiresRegistryBackedAuthority(t *testing.T) {
	dsv2 := DatasetSnapshotIDPrefixV2 + SHA256Hex([]byte("bare-v2"))
	if !IsDatasetSnapshotIDV2Syntax(dsv2) {
		t.Fatal("canonical DSV2 syntax was not recognized")
	}
	if blocker := DatasetSnapshotFactAuthorityBlocker(dsv2); blocker != SourceProbeBlockerDatasetSnapshotAuthorityUnavailable {
		t.Fatalf("bare DSV2 did not receive the fixed authority blocker: %q", blocker)
	}
	for _, invalid := range []string{
		DatasetSnapshotIDPrefixV2 + strings.ToUpper(SHA256Hex([]byte("uppercase"))),
		DatasetSnapshotIDPrefixV2 + strings.Repeat("a", 63),
		" " + dsv2,
		"dsv20_" + SHA256Hex([]byte("lookalike")),
	} {
		if IsDatasetSnapshotIDV2Syntax(invalid) {
			t.Fatalf("non-canonical DSV2 syntax was accepted: %q", invalid)
		}
		if blocker := DatasetSnapshotFactAuthorityBlocker(invalid); blocker != SourceProbeBlockerDatasetSnapshotAuthorityUnavailable {
			t.Fatalf("invalid snapshot leaked a distinct external blocker: %q", blocker)
		}
	}
}

func TestSourceProbeResponseFailsClosedForUnknownOrInconsistentState(t *testing.T) {
	snapshotID := DatasetSnapshotIDPrefixV1 + SHA256Hex([]byte("snapshot-1"))
	base := map[string]any{
		"version": float64(1), "serverName": "analytix_funds", "serverVersion": "0.16.15", "caseId": "case_1234",
		"caseBindingHash": SHA256Hex([]byte("binding")), "datasetSnapshotId": snapshotID, "ready": true,
		"readOnly": true, "blocker": "", "checkedAt": "2026-07-10T10:00:00Z",
	}
	base["safeToAnswer"] = true
	if _, err := ParseSourceProbeResponse(base); err == nil {
		t.Fatal("unknown source-probe authority field was accepted")
	}
	delete(base, "safeToAnswer")
	base["ready"] = false
	if _, err := ParseSourceProbeResponse(base); err == nil {
		t.Fatal("unready probe with a snapshot was accepted")
	}
	base["ready"] = true
	for _, invalid := range []string{
		"snapshot_1",
		NoDatasetSnapshotID,
		UnresolvedSnapshotMark + SHA256Hex([]byte("binding")),
		DatasetSnapshotIDPrefixV1 + strings.ToUpper(SHA256Hex([]byte("uppercase"))),
		DatasetSnapshotIDPrefixV1 + strings.Repeat("a", 63),
		" " + snapshotID,
	} {
		base["datasetSnapshotId"] = invalid
		if _, err := ParseSourceProbeResponse(base); err == nil {
			t.Fatalf("invalid source-probe snapshot was accepted: %q", invalid)
		}
	}
}
