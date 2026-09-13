package thread

import (
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	"analytix.local/runtime-go/internal/contracts"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

const trustedPlanDigestWithDecimalRunV1 = "93887a2fccaf8b3b1b51350c0abfc65390966231128774816dce99bc362505e4"
const trustedPlanNanosecondTimestampV1 = "2026-09-13T13:35:15.123456789Z"

func TestTrustedPublicProjectorKeepsClosedPlanDigestByteStable(t *testing.T) {
	for _, savedAt := range []string{"2026-08-01T08:00:00Z", trustedPlanNanosecondTimestampV1} {
		t.Run(savedAt, func(t *testing.T) {
			assertTrustedPlanPublicProjectionV1(t, savedAt)
		})
	}
}

func assertTrustedPlanPublicProjectionV1(t *testing.T, savedAt string) {
	t.Helper()
	if savedAt == trustedPlanNanosecondTimestampV1 && domainprivacy.ProjectText(savedAt).Text == savedAt {
		t.Fatal("timestamp fixture did not exercise ordinary phone classification")
	}
	threadID := "thread_plan_digest_projection"
	turnID := "turn_plan_digest_projection"
	records := trustedPlanDigestRecordsV1(t, threadID, turnID, savedAt)
	thread := map[string]any{
		"id": threadID, "status": "running", "title": "Plan digest projection",
		"turns": []any{map[string]any{
			"id": turnID, "threadId": threadID, "status": "running",
			"items": []any{records.ResultItem},
		}},
	}
	originalThread := contracts.CloneMap(thread)
	originalEvent := contracts.CloneMap(records.Event)
	projector := NewTrustedPublicProjector(nil)
	t.Run("thread", func(t *testing.T) {
		projectedThread, err := projector.ProjectThread(thread)
		if err != nil {
			t.Fatal(err)
		}
		turns, _ := projectedThread["turns"].([]any)
		turn, _ := turns[0].(map[string]any)
		items, _ := turn["items"].([]any)
		item, _ := items[0].(map[string]any)
		assertTrustedPlanDigestV1(t, item, savedAt)
	})

	t.Run("event", func(t *testing.T) {
		projectedEvent, visible, err := projector.ProjectEvent(threadID, thread, records.Event)
		if err != nil || !visible {
			t.Fatalf("closed plan event projection failed: visible=%t err=%v", visible, err)
		}
		eventItem, _ := projectedEvent["item"].(map[string]any)
		assertTrustedPlanDigestV1(t, eventItem, savedAt)
	})
	if !reflect.DeepEqual(thread, originalThread) || !reflect.DeepEqual(records.Event, originalEvent) {
		t.Fatal("public plan projection changed its canonical source")
	}
}

func TestTrustedPlanMetadataProtectionDoesNotExemptDisplayOrLookalikes(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
		closed bool
	}{
		{"display phone", func(item map[string]any) {
			item["output"].(map[string]any)["plan"].(map[string]any)["planId"] = "plan-13812345678"
		}, true},
		{"invalid timestamp", func(item map[string]any) {
			item["output"].(map[string]any)["plan"].(map[string]any)["savedAt"] = "2026-99-13T13:35:15.123456789Z"
		}, false},
		{"lookalike item", func(item map[string]any) { item["id"] = "item_result_forged" }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			item := trustedPlanDigestRecordsV1(t, "thread_plan_metadata", "turn_plan_metadata", trustedPlanNanosecondTimestampV1).ResultItem
			test.mutate(item)
			original := contracts.CloneMap(item)
			if _, closed := domaintoolresult.ClosedPlanToolResultDigestV1(item); closed != test.closed {
				t.Fatal("fixture did not exercise its intended canonical boundary")
			}
			projected := projectTrustedStructuredOrdinaryV1(item, trustedProjectionItemV1)
			body, err := json.Marshal(projected)
			if err != nil || strings.Contains(string(body), "13812345678") {
				t.Fatal("plan metadata protection exposed display PII")
			}
			if !test.closed && (strings.Contains(string(body), "15.123456789") || strings.Contains(string(body), trustedPlanDigestWithDecimalRunV1)) {
				t.Fatal("invalid or lookalike plan gained exact metadata protection")
			}
			if test.closed {
				assertTrustedPlanDigestV1(t, projected.(map[string]any), trustedPlanNanosecondTimestampV1)
			}
			if !reflect.DeepEqual(item, original) {
				t.Fatal("plan metadata projection changed its source")
			}
		})
	}
}

func trustedPlanDigestRecordsV1(t *testing.T, threadID, turnID, savedAt string) toolcatalogapp.ToolResultRecords {
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
			SavedAt:      savedAt,
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

func assertTrustedPlanDigestV1(t *testing.T, item map[string]any, savedAt string) {
	t.Helper()
	projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(item["output"])
	if err != nil {
		t.Fatalf("projected plan output is no longer closed: %v", err)
	}
	if projection.Plan == nil || projection.Plan.ContentHash != trustedPlanDigestWithDecimalRunV1 || projection.Plan.SavedAt != savedAt {
		t.Fatalf("projected plan metadata changed: %#v", projection.Plan)
	}
}
