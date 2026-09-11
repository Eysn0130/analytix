package thread

import (
	"crypto/sha256"
	"testing"
	"time"

	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

const trustedPlanDigestWithDecimalRunV1 = "93887a2fccaf8b3b1b51350c0abfc65390966231128774816dce99bc362505e4"

func TestTrustedPublicProjectorKeepsClosedPlanDigestByteStable(t *testing.T) {
	threadID := "thread_plan_digest_projection"
	turnID := "turn_plan_digest_projection"
	records := trustedPlanDigestRecordsV1(t, threadID, turnID)
	thread := map[string]any{
		"id": threadID, "status": "running", "title": "Plan digest projection",
		"turns": []any{map[string]any{
			"id": turnID, "threadId": threadID, "status": "running",
			"items": []any{records.ResultItem},
		}},
	}
	projector := NewTrustedPublicProjector(nil)
	projectedThread, err := projector.ProjectThread(thread)
	if err != nil {
		t.Fatal(err)
	}
	turns, _ := projectedThread["turns"].([]any)
	turn, _ := turns[0].(map[string]any)
	items, _ := turn["items"].([]any)
	item, _ := items[0].(map[string]any)
	assertTrustedPlanDigestV1(t, item)

	projectedEvent, visible, err := projector.ProjectEvent(threadID, thread, records.Event)
	if err != nil || !visible {
		t.Fatalf("closed plan event projection failed: visible=%t err=%v", visible, err)
	}
	eventItem, _ := projectedEvent["item"].(map[string]any)
	assertTrustedPlanDigestV1(t, eventItem)
}

func trustedPlanDigestRecordsV1(t *testing.T, threadID, turnID string) toolcatalogapp.ToolResultRecords {
	t.Helper()
	entropy := sha256.Sum256([]byte("analytix.trusted-plan-digest-test/v1"))
	callID, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		t.Fatal(err)
	}
	projection := domaintoolresult.PublicToolResultProjectionV1{
		SchemaVersion:          domaintoolresult.PublicProjectionSchemaVersion,
		ProjectionKind:         domaintoolresult.ProjectionPlanStatus,
		Disclosure:             domaintoolresult.MetadataOnlyDisclosure,
		MessageKey:             "plan_updated",
		Status:                 "completed",
		Code:                   "plan_updated",
		PrivatePayloadWithheld: true,
		FactAnswerAllowed:      false,
		EvidenceAuthority:      false,
		Plan: &domaintoolresult.PlanStatusV1{
			PlanID:       "/workspace:.analytixsdd/plan/validation.md",
			RelativePath: ".analytixsdd/plan/validation.md",
			Operation:    "draft",
			ContentHash:  trustedPlanDigestWithDecimalRunV1,
			ByteSize:     38,
			SavedAt:      time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		},
	}
	records, err := toolcatalogapp.SettleToolResult(toolcatalogapp.ToolResultInput{
		ThreadID:   threadID,
		TurnID:     turnID,
		Call:       domainmodel.ToolCall{ID: callID, Name: "create_plan"},
		Projection: projection,
	})
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func assertTrustedPlanDigestV1(t *testing.T, item map[string]any) {
	t.Helper()
	projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(item["output"])
	if err != nil {
		t.Fatalf("projected plan output is no longer closed: %v", err)
	}
	if projection.Plan == nil || projection.Plan.ContentHash != trustedPlanDigestWithDecimalRunV1 {
		t.Fatalf("projected plan digest changed: %#v", projection.Plan)
	}
}
