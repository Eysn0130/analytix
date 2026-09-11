package toolresult

import (
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestPrivateDurableToolResultAcceptsStrictJSONContextEpoch(t *testing.T) {
	item := map[string]any{
		"id": "item-1", "turnId": "turn-1", "threadId": "thread-1", "role": "tool", "status": "completed",
		"createdAt": "2026-08-01T00:00:00Z", "finishedAt": "2026-08-01T00:00:01Z", "kind": "tool_result",
		"toolName": "read_file", "callId": "call-1", "toolKind": "tool_call", "isError": false,
		"contextDigest": strings.Repeat("a", 64), "contextEpoch": json.Number("7"),
		"executionGrantId": strings.Repeat("b", 64),
		"output":           PublicToolResultProjectionRecordV1(WithheldProjectionV1("completed", "tool_output_private")),
	}
	projected, ok := PrivateDurableToolResultItemRecordV1(item)
	if !ok || projected["contextEpoch"] != uint64(7) {
		t.Fatalf("strict JSON context epoch was not admitted: ok=%t epoch=%#v", ok, projected["contextEpoch"])
	}

	for _, invalid := range []json.Number{"0", "+7", "07", "7.0", "9007199254740992"} {
		item["contextEpoch"] = invalid
		if _, ok := PrivateDurableToolResultItemRecordV1(item); ok {
			t.Fatalf("invalid strict JSON context epoch was admitted: %q", invalid)
		}
	}
}

func TestClosedPlanToolResultDigestRequiresExactHostIdentity(t *testing.T) {
	item := closedPlanToolResultFixtureV1(t)
	digest, ok := ClosedPlanToolResultDigestV1(item)
	if !ok || digest != item["output"].(map[string]any)["plan"].(map[string]any)["contentHash"] {
		t.Fatalf("exact host plan result was rejected: ok=%t digest=%q", ok, digest)
	}

	for name, mutate := range map[string]func(map[string]any){
		"missing item id":   func(record map[string]any) { delete(record, "id") },
		"wrong item id":     func(record map[string]any) { record["id"] = "item_result_wrong" },
		"missing thread id": func(record map[string]any) { delete(record, "threadId") },
		"missing turn id":   func(record map[string]any) { delete(record, "turnId") },
		"missing call id":   func(record map[string]any) { delete(record, "callId") },
		"invalid call id":   func(record map[string]any) { record["callId"] = "call-provider" },
		"missing tool name": func(record map[string]any) { delete(record, "toolName") },
		"wrong tool name":   func(record map[string]any) { record["toolName"] = "read_file" },
		"wrong tool kind":   func(record map[string]any) { record["toolKind"] = "tool_call" },
	} {
		t.Run(name, func(t *testing.T) {
			record := cloneToolResultJSONValue(item).(map[string]any)
			mutate(record)
			if _, ok := ClosedPlanToolResultDigestV1(record); ok {
				t.Fatal("non-host-bound plan result gained a digest privacy classification")
			}
		})
	}

	typedOutput := cloneToolResultJSONValue(item).(map[string]any)
	projection, err := ParsePublicToolResultProjectionV1(typedOutput["output"])
	if err != nil {
		t.Fatal(err)
	}
	typedOutput["output"] = projection
	if _, ok := ClosedPlanToolResultDigestV1(typedOutput); ok {
		t.Fatal("typed output struct gained a JSON-shape digest classification")
	}

	typedPlan := cloneToolResultJSONValue(item).(map[string]any)
	typedPlan["output"].(map[string]any)["plan"] = *projection.Plan
	if _, ok := ClosedPlanToolResultDigestV1(typedPlan); ok {
		t.Fatal("typed plan struct gained a JSON-shape digest classification")
	}
}

func closedPlanToolResultFixtureV1(t *testing.T) map[string]any {
	t.Helper()
	entropy := sha256.Sum256([]byte("analytix.closed-plan-tool-result-test/v1"))
	callID, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		t.Fatal(err)
	}
	turnID := "turn-plan-result"
	projection := PublicToolResultProjectionV1{
		SchemaVersion: PublicProjectionSchemaVersion, ProjectionKind: ProjectionPlanStatus,
		Disclosure: MetadataOnlyDisclosure, MessageKey: "plan_updated", Status: "completed", Code: "plan_updated",
		PrivatePayloadWithheld: true, Plan: &PlanStatusV1{
			PlanID: "workspace:.analytixsdd/plan/validation.md", RelativePath: ".analytixsdd/plan/validation.md",
			Operation: "draft", ContentHash: strings.Repeat("a", 64), ByteSize: 39,
			SavedAt: time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		},
	}
	return map[string]any{
		"id": ToolResultItemIDV1(turnID, callID), "turnId": turnID, "threadId": "thread-plan-result",
		"role": "tool", "status": "completed", "createdAt": "2026-08-01T08:00:00Z", "finishedAt": "2026-08-01T08:00:01Z",
		"kind": "tool_result", "toolName": "create_plan", "callId": callID, "toolKind": "file_change",
		"output": PublicToolResultProjectionRecordV1(projection), "isError": false,
	}
}
