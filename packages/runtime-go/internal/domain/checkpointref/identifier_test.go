package checkpointref

import (
	"strings"
	"testing"
)

func TestRuntimeIDIsCollisionResistantAndIdempotent(t *testing.T) {
	id := RuntimeID("abc/def")
	if !IsCanonicalRuntimeIDV2(id) || RuntimeID(id) != id {
		t.Fatalf("runtime checkpoint id is not canonical or idempotent: %q", id)
	}
	for _, legacyCollision := range []string{"abc\\def", "abc:def", "abc_def"} {
		if RuntimeID(legacyCollision) == id {
			t.Fatalf("V2 checkpoint id collision for %q", legacyCollision)
		}
	}
}

func TestPublicReferenceDigestIsIrreversibleAndContextBound(t *testing.T) {
	checkpointID := RuntimeID("workspace-checkpoint-1")
	digest := PublicReferenceDigest("thread-1", "turn-1", checkpointID)
	if len(digest) != 64 || digest == checkpointID || digest != PublicReferenceDigest("thread-1", "turn-1", checkpointID) {
		t.Fatalf("public checkpoint reference is not deterministic metadata: %q", digest)
	}
	for _, changed := range []string{
		PublicReferenceDigest("thread-2", "turn-1", checkpointID),
		PublicReferenceDigest("thread-1", "turn-2", checkpointID),
		PublicReferenceDigest("thread-1", "turn-1", RuntimeID("workspace-checkpoint-2")),
	} {
		if changed == digest {
			t.Fatal("public checkpoint reference did not bind its authority dimensions")
		}
	}
}

func TestCaptureEventAndPayloadDigestsAreVersionedAndTamperEvident(t *testing.T) {
	frontier := strings.Repeat("a", 64)
	eventID := CaptureEventID(frontier)
	if !IsCaptureEventIDV2(eventID) || eventID != CaptureEventID(frontier) || CaptureEventID("invalid") != "" {
		t.Fatalf("capture event id is invalid: %q", eventID)
	}
	payload := map[string]any{
		"schemaVersion": float64(1), "captureEventId": eventID, "changedFileCount": float64(2),
	}
	payload["capturePayloadDigest"] = CapturedPayloadDigest(payload)
	if !CapturedPayloadDigestMatches(payload) {
		t.Fatal("valid captured payload digest was rejected")
	}
	payload["changedFileCount"] = float64(3)
	if CapturedPayloadDigestMatches(payload) {
		t.Fatal("mutated captured payload digest was accepted")
	}
}

func TestLinkedIDMatchesCurrentV2Relationship(t *testing.T) {
	checkpointID := RuntimeID("workspace-checkpoint-1")
	for _, test := range []struct {
		kind LinkKind
		id   string
	}{
		{kind: PlanLink, id: PlanID(checkpointID)},
		{kind: ApplyLink, id: ApplyID(checkpointID)},
		{kind: RescueLink, id: RescueID(checkpointID)},
	} {
		if !LinkedIDMatches(test.kind, checkpointID, test.id) {
			t.Fatalf("canonical linked id was rejected: kind=%s id=%q", test.kind, test.id)
		}
		if LinkedIDMatches(test.kind, checkpointID, test.id+"0") {
			t.Fatalf("mutated linked id was accepted: kind=%s", test.kind)
		}
	}
	if LinkedIDMatches(PlanLink, checkpointID, ApplyID(checkpointID)) {
		t.Fatal("cross-kind linked id was accepted")
	}
}

func TestLinkedIDMatchesLegacyReplayOnlyByExactMapping(t *testing.T) {
	checkpointID := "axcp_legacy-1"
	if !IsLegacyRuntimeIDV1(checkpointID) || !IsPublicReplayRuntimeID(checkpointID) {
		t.Fatalf("strict legacy replay id was rejected: %q", checkpointID)
	}
	if !LinkedIDMatches(PlanLink, checkpointID, "axrp_"+checkpointID) {
		t.Fatal("exact legacy linked id was rejected")
	}
	for _, invalid := range []string{
		"axrp_legacy-1",
		PlanID(checkpointID),
		"axrp_" + checkpointID + "-other",
		" " + "axrp_" + checkpointID,
		"axrp_" + checkpointID + " ",
		"axrp_axcp_案件",
	} {
		if LinkedIDMatches(PlanLink, checkpointID, invalid) {
			t.Fatalf("invalid legacy linked id was accepted: %q", invalid)
		}
	}
	for _, invalid := range []string{
		"axcp_", " axcp_legacy-1", "axcp_案件", "axcp_v2_deadbeef",
		"axcp_" + strings.Repeat("a", maxLegacyIDBytes),
	} {
		if IsPublicReplayRuntimeID(invalid) {
			t.Fatalf("invalid checkpoint replay id was accepted: %q", invalid)
		}
	}
}
