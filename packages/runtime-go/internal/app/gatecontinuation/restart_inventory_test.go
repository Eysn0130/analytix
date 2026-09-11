package gatecontinuation

import (
	"testing"

	contracts "analytix.local/runtime-go/internal/contracts"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestRestartInventoryConstructorFailsClosedOnIncompleteAuthority(t *testing.T) {
	if inventory, err := NewRestartInventory(RestartInventoryDependencies{}); err == nil || inventory != nil {
		t.Fatalf("incomplete restart inventory authority was accepted: inventory=%#v err=%v", inventory, err)
	}
}

func TestRestartInvalidReasonSeparatesAuditOnlyAndCurrentReceipts(t *testing.T) {
	legacyV2 := domaincontinuation.Receipt{Payload: domaincontinuation.Payload{Version: domaincontinuation.ContractVersionV2}}
	if got := restartInvalidReasonV1(legacyV2, "restart_nonresumable"); got != "restart_legacy_continuation_contract" {
		t.Fatalf("legacy V2 restart reason = %q", got)
	}
	preBindingV3 := domaincontinuation.Receipt{Payload: domaincontinuation.Payload{Version: domaincontinuation.ContractVersionV3}}
	if got := restartInvalidReasonV1(preBindingV3, "restart_nonresumable"); got != "restart_provider_step_binding_missing" {
		t.Fatalf("pre-binding V3 restart reason = %q", got)
	}
	currentV3 := domaincontinuation.Receipt{Payload: domaincontinuation.Payload{
		Version: domaincontinuation.ContractVersionV3, LogicalEffect: domainsecurity.LogicalEffectOrdinary,
		OrdinaryWork: true, ProviderStepExact: true,
		ProviderStepPromptSHA256: domaincontinuation.CanonicalProviderStepPromptSHA256("continue exact step"),
	}}
	if got := restartInvalidReasonV1(currentV3, "restart_thread_quarantined"); got != "restart_thread_quarantined" {
		t.Fatalf("current V3 restart reason = %q", got)
	}
}

func TestRestartDispositionProjectionIsDeterministicAndFailClosed(t *testing.T) {
	tests := []struct {
		name        string
		disposition domaincontinuation.Disposition
		wantStatus  string
		wantReason  string
		wantError   bool
	}{
		{
			name: "signed_approval_allowed",
			disposition: domaincontinuation.Disposition{
				Kind: domaincontinuation.KindApproval, Status: domaincontinuation.StatusAllowed, ReasonCode: "approval_allowed",
			},
			wantStatus: "allowed",
		},
		{
			name: "restart_invalid_approval",
			disposition: domaincontinuation.Disposition{
				Kind: domaincontinuation.KindApproval, Status: domaincontinuation.StatusRestartInvalid, ReasonCode: "restart_nonresumable",
			},
			wantStatus: "expired", wantReason: "restart_nonresumable",
		},
		{
			name: "restart_invalid_user_input",
			disposition: domaincontinuation.Disposition{
				Kind: domaincontinuation.KindUserInput, Status: domaincontinuation.StatusRestartInvalid, ReasonCode: "restart_thread_unavailable",
			},
			wantStatus: "cancelled", wantReason: "restart_thread_unavailable",
		},
		{
			name: "missing_reason",
			disposition: domaincontinuation.Disposition{
				Kind: domaincontinuation.KindApproval, Status: domaincontinuation.StatusRejected,
			},
			wantError: true,
		},
		{
			name: "unsupported_cross_kind_status",
			disposition: domaincontinuation.Disposition{
				Kind: domaincontinuation.KindUserInput, Status: domaincontinuation.StatusAllowed, ReasonCode: "approval_allowed",
			},
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, reason, err := RestartDispositionProjectionV1(test.disposition)
			if test.wantError {
				if err == nil {
					t.Fatalf("unsupported disposition projected as status=%q reason=%q", status, reason)
				}
				return
			}
			if err != nil || status != test.wantStatus || reason != test.wantReason {
				t.Fatalf("projection = status:%q reason:%q err:%v", status, reason, err)
			}
		})
	}
}

func TestRestartInventoryTurnAndGateItemRequireExactUniqueOwnership(t *testing.T) {
	expected := map[string]any{
		"id": "item-a", "kind": "approval", "approvalId": "approval-a", "status": "pending",
	}
	thread := map[string]any{"turns": []any{map[string]any{
		"id": "turn-a", "status": "waiting", "items": []any{contracts.CloneMap(expected)},
	}}}
	turn, status, found := RestartInventoryTurnV1(thread, "turn-a")
	if !found || status != "waiting" {
		t.Fatalf("owning turn lookup = found:%t status:%q", found, status)
	}
	itemFound, itemStatus, err := InspectRestartGateRequestItemV1(turn, expected)
	if err != nil || !itemFound || itemStatus != "pending" {
		t.Fatalf("exact gate item lookup = found:%t status:%q err:%v", itemFound, itemStatus, err)
	}

	conflict := contracts.CloneMap(expected)
	conflict["approvalId"] = "approval-forged"
	if found, _, err := InspectRestartGateRequestItemV1(turn, conflict); err == nil || found {
		t.Fatalf("conflicting signed gate identity was accepted: found=%t err=%v", found, err)
	}
	turn["items"] = []any{contracts.CloneMap(expected), contracts.CloneMap(expected)}
	if found, _, err := InspectRestartGateRequestItemV1(turn, expected); err == nil || found {
		t.Fatalf("duplicate gate items were accepted: found=%t err=%v", found, err)
	}
	if _, _, found := RestartInventoryTurnV1(thread, "turn-other"); found {
		t.Fatal("restart inventory crossed turn ownership")
	}
}
