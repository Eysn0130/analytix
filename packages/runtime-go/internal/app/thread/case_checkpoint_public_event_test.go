package thread

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	"analytix.local/runtime-go/internal/contracts"
)

func TestCaseCheckpointPublicEventsExposeAuditMetadataOnly(t *testing.T) {
	thread, events, privateValues := caseCheckpointPublicEventFixtures(t)
	for _, event := range events {
		projected, visible, err := ProjectPublicThreadEvent("thread-case-checkpoint", thread, event)
		if err != nil || !visible || projected == nil {
			t.Fatalf("case checkpoint event was not publicly projected: event=%#v projected=%#v visible=%t err=%v", event, projected, visible, err)
		}
		body, _ := json.Marshal(projected)
		for _, forbidden := range append(privateValues,
			"checkpointId", "planId", "applyId", "rescueId", "workspace", "relativePath", "beforeHash", "afterHash", "content",
			"executionGrant", "publicationReceipt", "publicationAuthority") {
			if strings.Contains(string(body), forbidden) {
				t.Fatalf("case checkpoint public event leaked %q: %s", forbidden, body)
			}
		}
		payloadKey := map[string]string{
			"checkpoint_captured":              "checkpoint",
			"checkpoint_rewind_rescue_created": "rescue",
			"checkpoint_rewind_applied":        "apply",
		}[projected["kind"].(string)]
		metadata, _ := projected[payloadKey].(map[string]any)
		if metadata["projectionKind"] != "checkpoint_status" || metadata["disclosure"] != "metadata_only" ||
			metadata["privatePayloadWithheld"] != true || metadata["factAnswerAllowed"] != false || metadata["evidenceAuthority"] != false {
			t.Fatalf("case checkpoint public event acquired authority: %#v", metadata)
		}
	}
}

func TestCaseCheckpointPublicEventsFailClosedOnMismatchedAuthority(t *testing.T) {
	thread, events, _ := caseCheckpointPublicEventFixtures(t)
	tests := map[string]map[string]any{}

	wrongThread := contracts.CloneMap(events[1])
	wrongThread["threadId"] = "thread-other"
	tests["outer thread"] = wrongThread

	wrongTurn := contracts.CloneMap(events[0])
	wrongTurn["turnId"] = "turn-other"
	wrongTurn["checkpoint"].(map[string]any)["turnId"] = "turn-other"
	tests["unknown turn"] = wrongTurn

	mismatchedNestedTurn := contracts.CloneMap(events[0])
	mismatchedNestedTurn["checkpoint"].(map[string]any)["turnId"] = "turn-other"
	tests["nested turn"] = mismatchedNestedTurn

	wrongPlan := contracts.CloneMap(events[2])
	wrongPlan["apply"].(map[string]any)["planId"] = checkpointapp.PlanID(checkpointapp.RuntimeCheckpointIDFromWorkspaceCheckpointID("other"))
	tests["plan link"] = wrongPlan

	wrongApply := contracts.CloneMap(events[2])
	wrongApply["apply"].(map[string]any)["applyId"] = checkpointapp.ApplyID(checkpointapp.RuntimeCheckpointIDFromWorkspaceCheckpointID("other"))
	tests["apply link"] = wrongApply

	wrongRescue := contracts.CloneMap(events[1])
	wrongRescue["rescue"].(map[string]any)["rescueId"] = checkpointapp.RescueID(checkpointapp.RuntimeCheckpointIDFromWorkspaceCheckpointID("other"))
	tests["rescue link"] = wrongRescue

	privatePayload := contracts.CloneMap(events[1])
	privatePayload["rescue"].(map[string]any)["content"] = "BANK_CARD_6222020202020202020"
	tests["private payload"] = privatePayload

	for name, event := range tests {
		if projected, visible, err := ProjectPublicThreadEvent("thread-case-checkpoint", thread, event); err != nil || visible || projected != nil {
			t.Fatalf("mismatched case checkpoint %s was published: projected=%#v visible=%t err=%v", name, projected, visible, err)
		}
	}
	if projected, visible, err := ProjectPublicThreadEvent("thread-other", thread, events[0]); err != nil || visible || projected != nil {
		t.Fatalf("cross-route checkpoint event was published: projected=%#v visible=%t err=%v", projected, visible, err)
	}
}

func caseCheckpointPublicEventFixtures(t *testing.T) (map[string]any, []map[string]any, []string) {
	t.Helper()
	securityContext := newTrustedProjectionContext("thread-case-checkpoint", "turn-case-checkpoint", "case-checkpoint", "snapshot-checkpoint", 3)
	thread := map[string]any{
		"id": securityContext.ThreadID, "securityState": publicProjectionSecurityRecord(securityContext),
		"turns": []any{map[string]any{
			"id": securityContext.TurnID, "securityContext": publicProjectionSecurityRecord(securityContext), "items": []any{},
		}},
	}
	checkpointID := checkpointapp.RuntimeCheckpointIDFromWorkspaceCheckpointID("workspace/checkpoint:case-public")
	createdAt := time.Date(2026, 7, 15, 2, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	rescue := checkpointapp.BuildRescueRecord(checkpointapp.RescueRecordInput{
		ThreadID: securityContext.ThreadID, CheckpointID: checkpointID, PlanID: checkpointapp.PlanID(checkpointID),
		Workspace: "/private/BANK_CARD_6222020202020202020", CreatedAt: createdAt,
		Files: []checkpointapp.RescueFile{{
			RelativePath: "private-account.txt", Existed: true, Hash: "PRIVATE_HASH", Content: "BANK_CARD_6222020202020202020",
		}},
	})
	rescueEvent := checkpointapp.BuildRescueCreatedEvent(securityContext.ThreadID, rescue)
	apply := checkpointapp.BuildApplyResponse(checkpointapp.ApplyResponseInput{
		ThreadID: securityContext.ThreadID, CheckpointID: checkpointID, Workspace: "/private/BANK_CARD_6222020202020202020",
		PlanID: checkpointapp.PlanID(checkpointID), Scope: "code", CreatedAt: createdAt, Status: "applied",
		Files: []map[string]any{{"relativePath": "private-account.txt", "status": "applied", "reason": "PRIVATE_REASON"}}, Rescue: rescue,
	})
	applyEvent := checkpointapp.BuildRewindAppliedEvent(securityContext.ThreadID, apply)
	capturedEvent := map[string]any{
		"kind": "checkpoint_captured", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
		"checkpoint": map[string]any{
			"schemaVersion": float64(1), "checkpointId": checkpointID, "threadId": securityContext.ThreadID,
			"turnId": securityContext.TurnID, "createdAt": createdAt, "status": "captured",
			"changedFileCount": float64(1), "snapshotStorage": "runtime_private_cas",
		},
	}
	events := []map[string]any{capturedEvent, rescueEvent, applyEvent}
	for index, event := range events {
		event["seq"] = float64(index + 1)
		event["timestamp"] = createdAt
	}
	return thread, events, []string{
		checkpointID, checkpointapp.PlanID(checkpointID), checkpointapp.RescueID(checkpointID), checkpointapp.ApplyID(checkpointID),
		"BANK_CARD_6222020202020202020", "private-account.txt", "PRIVATE_HASH", "PRIVATE_REASON",
	}
}
