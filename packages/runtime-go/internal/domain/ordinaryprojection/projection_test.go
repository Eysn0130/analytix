package ordinaryprojection

import (
	"strings"
	"testing"

	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	domainsecret "analytix.local/runtime-go/internal/domain/secretprojection"
)

func TestProjectTextV1ReachesFixedPointAcrossPrivacyAndCredentialProjection(t *testing.T) {
	const account = "6222020000000000000"
	const token = "sk-abcdefghijk"
	input := "账户 " + account + token

	credentialFirst := domainsecret.ProjectTextV1(input)
	privacySecond := domainprivacy.ProjectText(credentialFirst).Text
	if !strings.Contains(privacySecond, token) {
		t.Fatalf("fixture does not exercise the cross-projector boundary: %q", privacySecond)
	}

	projected := ProjectTextV1(input)
	if strings.Contains(projected, account) || strings.Contains(projected, token) ||
		!strings.Contains(projected, "[ACCOUNT]") || !strings.Contains(projected, domainsecret.RedactedV1) {
		t.Fatalf("fixed-point ordinary projection leaked a restricted value: %q", projected)
	}
	if second := ProjectTextV1(projected); second != projected {
		t.Fatalf("ordinary projection is not idempotent: first=%q second=%q", projected, second)
	}
}

func TestProjectValueV1ReachesFixedPointWithoutMutatingInput(t *testing.T) {
	const account = "6222020000000000000"
	const token = "sk-abcdefghijk"
	input := map[string]any{"text": "账户 " + account + token, "contextEpoch": float64(7)}

	projected, ok := ProjectValueV1(input).(map[string]any)
	if !ok || projected["contextEpoch"] != float64(7) {
		t.Fatalf("ordinary value projection changed authority: %#v", projected)
	}
	text, _ := projected["text"].(string)
	if strings.Contains(text, account) || strings.Contains(text, token) {
		t.Fatalf("ordinary value projection leaked a restricted value: %#v", projected)
	}
	if input["text"] != "账户 "+account+token {
		t.Fatalf("ordinary value projection mutated its input: %#v", input)
	}
	if domainsecret.ValidateValueV1(projected) != nil || domainprivacy.ValidatePublicValue(projected) != nil {
		t.Fatalf("ordinary value projection did not reach a valid fixed point: %#v", projected)
	}
}

func TestOrdinaryProjectionNeverDropsSiblingOnKeyCollision(t *testing.T) {
	projected := ProjectValueV1(map[string]any{
		"Authorization: Bearer first-secret":  "left",
		"Authorization: Bearer second-secret": "right",
	})
	if projected != domainsecret.RedactedV1 {
		t.Fatalf("ordinary projected key collision = %#v, want fail-closed sentinel", projected)
	}
}
