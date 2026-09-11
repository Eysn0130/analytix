package turn

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/contracts"
	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
)

func TestCaseCheckpointAuditProjectionIsMetadataOnlyAndIdempotent(t *testing.T) {
	thread, events, privateValues := caseCheckpointAuditFixtures(t)
	for _, raw := range events {
		projected, err := SanitizeCaseEventPublication(thread, raw)
		if err != nil || projected == nil {
			t.Fatalf("valid case checkpoint audit was rejected: event=%#v projected=%#v err=%v", raw, projected, err)
		}
		projected["seq"] = float64(7)
		projected["timestamp"] = "2026-07-15T01:02:03Z"
		replayed, err := SanitizeCaseEventPublication(thread, projected)
		if err != nil || replayed == nil {
			t.Fatalf("durable case checkpoint audit was not idempotent: projected=%#v err=%v", replayed, err)
		}
		body, _ := json.Marshal(replayed)
		for _, forbidden := range append(privateValues,
			"checkpointId", "planId", "applyId", "rescueId", "workspace", "relativePath", "beforeHash", "afterHash", "content",
			"executionGrant", "publicationReceipt", "publicationAuthority") {
			if strings.Contains(string(body), forbidden) {
				t.Fatalf("case checkpoint audit leaked %q: %s", forbidden, body)
			}
		}
		payload, _ := replayed[caseCheckpointAuditPayloadKey(stringField(replayed, "kind"))].(map[string]any)
		if payload["projectionKind"] != "checkpoint_status" || payload["disclosure"] != "metadata_only" ||
			payload["privatePayloadWithheld"] != true || payload["factAnswerAllowed"] != false || payload["evidenceAuthority"] != false {
			t.Fatalf("case checkpoint audit acquired authority: %#v", payload)
		}
	}
}

func TestCaseCheckpointAuditProjectionRejectsWrongAuthorityAndPrivateFields(t *testing.T) {
	thread, events, _ := caseCheckpointAuditFixtures(t)
	checkpointID := events[0]["checkpoint"].(map[string]any)["checkpointId"].(string)
	otherCheckpointID := domaincheckpointref.RuntimeID("other-checkpoint")

	tests := map[string]func(map[string]any){
		"wrong outer thread": func(event map[string]any) { event["threadId"] = "thread-other" },
		"wrong captured turn": func(event map[string]any) {
			event["turnId"] = "turn-other"
			event["checkpoint"].(map[string]any)["turnId"] = "turn-other"
		},
		"mismatched captured turn": func(event map[string]any) { event["checkpoint"].(map[string]any)["turnId"] = "turn-other" },
		"forged plan link":         func(event map[string]any) { event["planId"] = domaincheckpointref.PlanID(otherCheckpointID) },
		"forged rescue link":       func(event map[string]any) { event["rescueId"] = domaincheckpointref.RescueID(otherCheckpointID) },
		"forged apply link":        func(event map[string]any) { event["applyId"] = domaincheckpointref.ApplyID(otherCheckpointID) },
		"private checkpoint field": func(event map[string]any) { event["content"] = "BANK_CARD_6222020202020202020" },
		"wrong checkpoint id":      func(event map[string]any) { event["checkpointId"] = checkpointID + "0" },
	}

	for name, mutate := range tests {
		var raw map[string]any
		switch name {
		case "wrong captured turn", "mismatched captured turn":
			raw = contracts.CloneMap(events[0])
			mutate(raw)
		case "forged rescue link":
			raw = contracts.CloneMap(events[1])
			mutate(raw["rescue"].(map[string]any))
		case "forged apply link":
			raw = contracts.CloneMap(events[2])
			mutate(raw["apply"].(map[string]any))
		case "forged plan link":
			raw = contracts.CloneMap(events[2])
			mutate(raw["apply"].(map[string]any))
		case "private checkpoint field":
			raw = contracts.CloneMap(events[1])
			mutate(raw["rescue"].(map[string]any))
		case "wrong checkpoint id":
			raw = contracts.CloneMap(events[2])
			mutate(raw["apply"].(map[string]any))
		default:
			raw = contracts.CloneMap(events[1])
			mutate(raw)
		}
		if projected, err := SanitizeCaseEventPublication(thread, raw); err == nil || projected != nil {
			t.Fatalf("%s case checkpoint audit was accepted: projected=%#v err=%v", name, projected, err)
		}
	}
}

func TestProjectedCaseCheckpointAuditCannotUpgradeAuthority(t *testing.T) {
	thread, events, _ := caseCheckpointAuditFixtures(t)
	projected, err := SanitizeCaseEventPublication(thread, events[2])
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"factAnswerAllowed", "evidenceAuthority", "privatePayloadWithheld"} {
		tampered := contracts.CloneMap(projected)
		metadata := tampered["apply"].(map[string]any)
		metadata[field] = field != "privatePayloadWithheld"
		if accepted, err := SanitizeCaseEventPublication(thread, tampered); err == nil || accepted != nil {
			t.Fatalf("projected checkpoint authority upgrade %q was accepted: %#v err=%v", field, accepted, err)
		}
	}
}

func TestCaseCheckpointCaptureMarkerIsDurableButNeverPublic(t *testing.T) {
	thread, events, _ := caseCheckpointAuditFixtures(t)
	raw := contracts.CloneMap(events[0])
	payload := raw["checkpoint"].(map[string]any)
	payload["captureEventId"] = domaincheckpointref.CaptureEventID(strings.Repeat("a", 64))
	payload["capturePayloadDigest"] = domaincheckpointref.CapturedPayloadDigest(payload)

	durable, err := SanitizeCaseEventPublication(thread, raw)
	if err != nil || durable == nil {
		t.Fatalf("valid marker-bound case checkpoint audit was rejected: %#v err=%v", durable, err)
	}
	durablePayload := durable["checkpoint"].(map[string]any)
	if durablePayload["captureEventId"] != payload["captureEventId"] ||
		!domaincheckpointref.CapturedPayloadDigestMatches(durablePayload) {
		t.Fatalf("durable case checkpoint audit lost internal exactly-once authority: %#v", durablePayload)
	}
	public, ok := ProjectCaseCheckpointAuditEvent(stringField(raw, "threadId"), durable)
	if !ok || public == nil {
		t.Fatal("durable case checkpoint audit could not be projected publicly")
	}
	publicPayload := public["checkpoint"].(map[string]any)
	if _, present := publicPayload["captureEventId"]; present {
		t.Fatalf("public case checkpoint audit leaked capture event id: %#v", publicPayload)
	}
	if _, present := publicPayload["capturePayloadDigest"]; present {
		t.Fatalf("public case checkpoint audit leaked capture payload digest: %#v", publicPayload)
	}

	tampered := contracts.CloneMap(raw)
	tampered["checkpoint"].(map[string]any)["changedFileCount"] = float64(2)
	if accepted, err := SanitizeCaseEventPublication(thread, tampered); err == nil || accepted != nil {
		t.Fatalf("marker-bound case checkpoint audit with a stale digest was accepted: %#v err=%v", accepted, err)
	}
}

func caseCheckpointAuditFixtures(t *testing.T) (map[string]any, []map[string]any, []string) {
	t.Helper()
	securityContext := acceptedFinalTestContext(t, "thread-checkpoint-case", "turn-checkpoint-case", 3)
	thread := map[string]any{"id": securityContext.ThreadID, "turns": []any{map[string]any{
		"id": securityContext.TurnID, "securityContext": turnSecurityContextRecord(securityContext), "items": []any{},
	}}}
	checkpointID := domaincheckpointref.RuntimeID("workspace/checkpoint:case")
	planID := domaincheckpointref.PlanID(checkpointID)
	rescueID := domaincheckpointref.RescueID(checkpointID)
	applyID := domaincheckpointref.ApplyID(checkpointID)
	createdAt := time.Date(2026, 7, 15, 1, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	summary := map[string]any{
		"fileAppliedCount": float64(1), "fileNoopCount": float64(0), "fileManualReviewCount": float64(0),
		"fileBlockedCount": float64(0), "fileFailedCount": float64(0),
	}
	events := []map[string]any{
		{
			"kind": "checkpoint_captured", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
			"checkpoint": map[string]any{
				"schemaVersion": float64(1), "checkpointId": checkpointID, "threadId": securityContext.ThreadID,
				"turnId": securityContext.TurnID, "createdAt": createdAt, "status": "captured",
				"changedFileCount": float64(1), "snapshotStorage": "runtime_private_cas",
			},
		},
		{
			"kind": "checkpoint_rewind_rescue_created", "threadId": securityContext.ThreadID,
			"rescue": map[string]any{
				"schemaVersion": float64(1), "rescueId": rescueID, "planId": planID, "checkpointId": checkpointID,
				"threadId": securityContext.ThreadID, "createdAt": createdAt, "fileCount": float64(1), "storage": "runtime_private_sidecar",
			},
		},
		{
			"kind": "checkpoint_rewind_applied", "threadId": securityContext.ThreadID,
			"apply": map[string]any{
				"schemaVersion": float64(1), "applyId": applyID, "planId": planID, "checkpointId": checkpointID,
				"threadId": securityContext.ThreadID, "createdAt": createdAt, "scope": "code", "status": "applied",
				"destructive": true, "fileCount": float64(1), "conversationStatus": "not_requested", "summary": summary,
			},
		},
	}
	return thread, events, []string{checkpointID, planID, rescueID, applyID, "6222020202020202020"}
}
