package event

import (
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

const planDigestWithDecimalRunV1 = "93887a2fccaf8b3b1b51350c0abfc65390966231128774816dce99bc362505e4"

func TestValidatePublicRecordPreservesClosedPlanDigestWithoutExemptingDisplayPII(t *testing.T) {
	item := exactPlanToolResultItemV1(t, planDigestWithDecimalRunV1)
	if err := ValidatePublicRecord(item); err != nil {
		t.Fatalf("closed plan digest was misclassified as ordinary PII: %v", err)
	}

	pii := exactPlanToolResultItemV1(t, planDigestWithDecimalRunV1)
	pii["output"].(map[string]any)["plan"].(map[string]any)["planId"] = "workspace:account-6222020000000000000"
	if err := ValidatePublicRecord(pii); !errors.Is(err, ErrPrivacyProjection) {
		t.Fatalf("plan display PII bypassed the closed digest view: %v", err)
	}

	lookalike := map[string]any{
		"kind":    "tool_progress",
		"details": item["output"],
	}
	if err := ValidatePublicRecord(lookalike); !errors.Is(err, ErrPrivacyProjection) {
		t.Fatalf("nested plan lookalike bypassed ordinary PII validation: %v", err)
	}

	for name, mutate := range map[string]func(map[string]any){
		"extra root field": func(record map[string]any) { record["unexpected"] = true },
		"missing item id":  func(record map[string]any) { delete(record, "id") },
		"wrong item id":    func(record map[string]any) { record["id"] = "item_result_wrong" },
		"missing call id":  func(record map[string]any) { delete(record, "callId") },
		"wrong tool name":  func(record map[string]any) { record["toolName"] = "read_file" },
	} {
		t.Run(name, func(t *testing.T) {
			nonCanonical := exactPlanToolResultItemV1(t, planDigestWithDecimalRunV1)
			mutate(nonCanonical)
			if err := ValidatePublicRecord(nonCanonical); !errors.Is(err, ErrPrivacyProjection) {
				t.Fatalf("non-host-bound tool result gained a digest exemption: %v", err)
			}
		})
	}
}

func exactPlanToolResultItemV1(t *testing.T, contentHash string) map[string]any {
	t.Helper()
	entropy := sha256.Sum256([]byte("analytix.plan-digest-privacy-test/v1"))
	callID, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		t.Fatal(err)
	}
	turnID := "turn_plan_digest_privacy"
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
			ContentHash:  contentHash,
			ByteSize:     39,
			SavedAt:      time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		},
	}
	return map[string]any{
		"id":         domaintoolresult.ToolResultItemIDV1(turnID, callID),
		"turnId":     turnID,
		"threadId":   "thread_plan_digest_privacy",
		"role":       "tool",
		"status":     "completed",
		"createdAt":  "2026-08-01T08:00:00Z",
		"finishedAt": "2026-08-01T08:00:01Z",
		"kind":       "tool_result",
		"toolName":   "create_plan",
		"callId":     callID,
		"toolKind":   "file_change",
		"output":     domaintoolresult.PublicToolResultProjectionRecordV1(projection),
		"isError":    false,
	}
}
