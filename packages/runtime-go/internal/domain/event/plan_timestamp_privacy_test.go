package event

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func TestClosedPlanNanosecondTimestampRemainsExact(t *testing.T) {
	for second := 13; second <= 19; second++ {
		t.Run(fmt.Sprint(second), func(t *testing.T) {
			savedAt := fmt.Sprintf("2026-09-12T12:35:%02d.123456789Z", second)
			// The ordinary scanner deliberately treats the separated eleven
			// digits as a phone. Only the validated plan metadata may retain it.
			if domainprivacy.ProjectText(savedAt).Text == savedAt {
				t.Fatal("fixture did not exercise ordinary phone classification")
			}
			item := exactPlanToolResultItemV1(t, planDigestWithDecimalRunV1)
			item["output"].(map[string]any)["plan"].(map[string]any)["savedAt"] = savedAt
			if _, err := domaintoolresult.ParsePublicToolResultProjectionV1(item["output"]); err != nil {
				t.Fatal("fixture timestamp is not valid typed plan metadata")
			}
			if err := ValidatePublicRecord(item); err != nil {
				t.Fatal("typed plan timestamp was rejected by public privacy validation")
			}
			projected, changed := ProjectPublicValuePreservingClosedPlanDigestV1(item)
			if changed || !reflect.DeepEqual(projected, item) {
				t.Fatal("public projection changed the exact typed plan timestamp")
			}
			if err := ValidatePublicRecord(projected); err != nil {
				t.Fatal("projected typed plan is not publicly valid")
			}
		})
	}
}

func TestClosedPlanTimestampDoesNotExemptUntrustedPhoneOrInvalidMetadata(t *testing.T) {
	for _, field := range []string{"planId", "relativePath"} {
		item := exactPlanToolResultItemV1(t, planDigestWithDecimalRunV1)
		plan := item["output"].(map[string]any)["plan"].(map[string]any)
		plan["savedAt"] = "2026-09-12T12:35:15.123456789Z"
		plan[field] = "contact-13812345678"
		if err := ValidatePublicRecord(item); !errors.Is(err, ErrPrivacyProjection) {
			t.Fatal("actual phone in plan display metadata bypassed privacy validation")
		}
	}
	for _, savedAt := range []string{"13812345678", "2026-99-12T12:35:15.123456789Z"} {
		item := exactPlanToolResultItemV1(t, planDigestWithDecimalRunV1)
		item["output"].(map[string]any)["plan"].(map[string]any)["savedAt"] = savedAt
		if _, err := domaintoolresult.ParsePublicToolResultProjectionV1(item["output"]); err == nil {
			t.Fatal("invalid timestamp passed the typed plan owner")
		}
		if _, ok := canonicalClosedPlanToolResultDigestV1(item); ok {
			t.Fatal("invalid timestamp obtained closed-plan metadata protection")
		}
		if err := ValidatePublicRecord(item); !errors.Is(err, ErrPrivacyProjection) {
			t.Fatal("invalid timestamp hid phone material from privacy validation")
		}
	}
	for _, item := range []map[string]any{
		{"kind": "tool_progress", "output": map[string]any{"plan": map[string]any{"savedAt": "2026-09-12T12:35:15.123456789Z"}}},
		{"kind": "tool_progress", "text": "13812345678"},
	} {
		if err := ValidatePublicRecord(item); !errors.Is(err, ErrPrivacyProjection) {
			t.Fatal("untrusted prose or plan lookalike gained typed timestamp protection")
		}
	}
}
