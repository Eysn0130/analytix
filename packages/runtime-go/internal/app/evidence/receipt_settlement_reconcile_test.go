package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

func TestEvidenceSettlementStartupReconcilesExactDurableMarkerOnce(t *testing.T) {
	issuer, input, prepared, marker := legacyPreparedSettlementFixture(t)
	reader := settlementReader(input, marker)
	inventory, err := PreflightEvidenceSettlementInventory(context.Background(), reader, issuer)
	if err != nil || len(inventory.Quarantined) != 1 || len(inventory.Pending) != 0 || len(inventory.Committed) != 0 || len(inventory.Abandoned) != 0 ||
		inventory.Quarantined[0].SettlementID != prepared.SettlementID {
		t.Fatalf("legacy durable marker was not quarantined: inventory=%#v err=%v", inventory, err)
	}
	registry := issuer.Registry.(*memoryEvidenceRegistry)
	if registry.commitCalls != 0 {
		t.Fatal("preflight mutated the evidence registry")
	}
	if err := ApplyEvidenceSettlementReconciliationInventory(context.Background(), reader, issuer, inventory); err != nil {
		t.Fatal(err)
	}
	if registry.commitCalls != 0 {
		t.Fatalf("legacy startup quarantine appended %d receipts", registry.commitCalls)
	}
	verified, err := PreflightEvidenceSettlementInventory(context.Background(), reader, issuer)
	if err != nil || len(verified.Pending) != 0 || len(verified.Committed) != 0 || len(verified.Quarantined) != 1 {
		t.Fatalf("legacy startup quarantine did not reach an exact fixed point: inventory=%#v err=%v", verified, err)
	}
	if err := ApplyEvidenceSettlementReconciliationInventory(context.Background(), reader, issuer, verified); err != nil {
		t.Fatal(err)
	}
	if registry.commitCalls != 0 {
		t.Fatal("idempotent legacy restart issued an evidence receipt")
	}
}

func TestUnavailableRegistryQuarantinesPreparedAuthorityWithoutInventoryOrCommit(t *testing.T) {
	issuer, input, prepared, marker := legacyPreparedSettlementFixture(t)
	registry := issuer.Registry.(*memoryEvidenceRegistry)
	blocked := &unavailableSettlementRegistryV1{memoryEvidenceRegistry: registry}
	issuer.Registry = blocked
	inventory, err := PreflightEvidenceSettlementInventory(
		context.Background(), settlementReader(input, marker), issuer,
	)
	if err != nil || len(inventory.Quarantined) != 1 || inventory.Quarantined[0].SettlementID != prepared.SettlementID ||
		len(inventory.Pending) != 0 || len(inventory.Committed) != 0 || blocked.listCalls != 0 || registry.commitCalls != 0 {
		t.Fatalf("blocked registry settlement was not quarantined without registry use: inventory=%#v list=%d commits=%d err=%v", inventory, blocked.listCalls, registry.commitCalls, err)
	}
	if err := ApplyEvidenceSettlementReconciliationInventory(
		context.Background(), settlementReader(input, marker), issuer, inventory,
	); err != nil || blocked.listCalls != 0 || registry.commitCalls != 0 {
		t.Fatalf("blocked registry settlement apply reached registry authority: list=%d commits=%d err=%v", blocked.listCalls, registry.commitCalls, err)
	}
}

func TestHistoricalV1RegistryIssueSurvivesWithoutPostV1SettlementAuthority(t *testing.T) {
	issuer, input, prepared, _ := legacyPreparedSettlementFixture(t)
	registry := issuer.Registry.(*memoryEvidenceRegistry)
	receipt, err := registry.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: input.Context, Draft: prepared.ReceiptDraft, CanonicalEvidence: prepared.CanonicalEvidence,
		SettlementProof: domainevidence.EvidenceSettlementProof{
			SettlementID: prepared.SettlementID, PreparedRecordDigest: prepared.RecordDigest,
		},
		RegisteredAt: evidenceIssuerTime().Add(time.Minute),
	})
	if err != nil || receipt.ReceiptID != prepared.ReceiptID {
		t.Fatalf("seed historical V1 receipt: receipt=%#v err=%v", receipt, err)
	}
	delete(issuer.SettlementStore.(*memoryEvidenceSettlementStore).records, prepared.SettlementID)
	reader := settlementReaderWithoutResult(input)
	issuer.Registry = &historicalV1SettlementRegistry{memoryEvidenceRegistry: registry}

	inventory, err := PreflightEvidenceSettlementInventory(context.Background(), reader, issuer)
	if err != nil || len(inventory.Pending) != 0 || len(inventory.Committed) != 0 ||
		len(inventory.Abandoned) != 0 || len(inventory.Quarantined) != 0 {
		t.Fatalf("historical V1 receipt required a post-V1 settlement record: inventory=%#v err=%v", inventory, err)
	}
	if err := ApplyEvidenceSettlementReconciliationInventory(context.Background(), reader, issuer, inventory); err != nil {
		t.Fatal(err)
	}
	if registry.commitCalls != 1 {
		t.Fatalf("historical V1 startup rewrote registry authority: commits=%d", registry.commitCalls)
	}
	if resolved, err := issuer.Registry.Resolve(context.Background(), registryport.MembershipQuery{
		Context: input.Context, ReceiptID: receipt.ReceiptID,
	}); err != nil || resolved.Receipt.ReceiptID != receipt.ReceiptID {
		t.Fatalf("historical V1 receipt lost membership after restart preflight: resolved=%#v err=%v", resolved, err)
	}

	issuer.Registry = registry
	if _, err := PreflightEvidenceSettlementInventory(context.Background(), reader, issuer); err == nil {
		t.Fatal("non-V1 registry accepted an orphan issue through the historical compatibility boundary")
	}
}

type historicalV1SettlementRegistry struct {
	*memoryEvidenceRegistry
}

func (*historicalV1SettlementRegistry) EvidenceRegistryHistoricalV1Authority() bool { return true }

type unavailableSettlementRegistryV1 struct {
	*memoryEvidenceRegistry
	listCalls int
}

func (*unavailableSettlementRegistryV1) CaseEvidenceAuthorityUnavailableV1() bool { return true }

func (registry *unavailableSettlementRegistryV1) ListRegistries(context.Context, []domainsecurity.TurnSecurityContext) ([]registryport.InventoryRecord, error) {
	registry.listCalls++
	return nil, errors.New("blocked inventory reached")
}

func (registry *unavailableSettlementRegistryV1) HasRecords(context.Context) (bool, error) {
	return false, errors.New("blocked inventory reached")
}

func TestEvidenceSettlementStartupLeavesPreparedOnlyNonAuthoritative(t *testing.T) {
	issuer, input, prepared, _ := legacyPreparedSettlementFixture(t)
	reader := settlementReaderWithoutResult(input)
	inventory, err := PreflightEvidenceSettlementInventory(context.Background(), reader, issuer)
	if err != nil || len(inventory.Pending) != 0 || len(inventory.Committed) != 0 || len(inventory.Abandoned) != 0 || len(inventory.Quarantined) != 1 {
		t.Fatalf("legacy prepared-only state was not quarantined: inventory=%#v err=%v", inventory, err)
	}
	if err := ApplyEvidenceSettlementReconciliationInventory(context.Background(), reader, issuer, inventory); err != nil {
		t.Fatal(err)
	}
	if _, err := issuer.Registry.Resolve(context.Background(), registryport.MembershipQuery{Context: input.Context, ReceiptID: prepared.ReceiptID}); err == nil {
		t.Fatal("prepared-only state resolved as evidence after restart")
	}
}

func TestEvidenceSettlementStartupRejectsOrphanAndDuplicateAuthority(t *testing.T) {
	t.Run("marker without prepared", func(t *testing.T) {
		issuer, input, prepared, marker := legacyPreparedSettlementFixture(t)
		delete(issuer.SettlementStore.(*memoryEvidenceSettlementStore).records, prepared.SettlementID)
		if inventory, err := PreflightEvidenceSettlementInventory(context.Background(), settlementReader(input, marker), issuer); err == nil || len(inventory.Pending) != 0 {
			t.Fatalf("orphan marker was accepted: inventory=%#v err=%v", inventory, err)
		}
	})

	t.Run("receipt without marker", func(t *testing.T) {
		issuer, input, _, marker := legacyPreparedSettlementFixture(t)
		authority := durableEvidenceSettlementAuthority(t, input, marker, evidenceIssuerTime().Add(time.Minute))
		if receipt, err := issuer.Commit(context.Background(), CommitEvidenceInput{Context: input.Context, Marker: marker, Authority: authority}); err == nil || receipt.ReceiptID != "" {
			t.Fatalf("legacy settlement created a receipt to orphan: receipt=%#v err=%v", receipt, err)
		}
		if inventory, err := PreflightEvidenceSettlementInventory(context.Background(), settlementReaderWithoutResult(input), issuer); err != nil || len(inventory.Committed) != 0 || len(inventory.Quarantined) != 1 {
			t.Fatalf("legacy receipt-free state escaped quarantine: inventory=%#v err=%v", inventory, err)
		}
	})

	t.Run("duplicate marker", func(t *testing.T) {
		issuer, input, _, marker := legacyPreparedSettlementFixture(t)
		reader := settlementReader(input, marker)
		turn := reader.threads[input.Context.ThreadID]["turns"].([]any)[0].(map[string]any)
		items := turn["items"].([]any)
		duplicate := cloneSettlementMap(items[len(items)-1].(map[string]any))
		duplicate["id"] = "item_result_duplicate"
		turn["items"] = append(items, duplicate)
		if inventory, err := PreflightEvidenceSettlementInventory(context.Background(), reader, issuer); err == nil || len(inventory.Pending) != 0 {
			t.Fatalf("duplicate marker was accepted: inventory=%#v err=%v", inventory, err)
		}
	})
}

func TestRevokedEvidenceSettlementRemainsSettledButNotMembership(t *testing.T) {
	issuer, input, prepared, marker := legacyPreparedSettlementFixture(t)
	reader := settlementReader(input, marker)
	inventory, err := PreflightEvidenceSettlementInventory(context.Background(), reader, issuer)
	if err != nil || len(inventory.Quarantined) != 1 || len(inventory.Pending) != 0 {
		t.Fatalf("legacy settlement was not quarantined before revocation: inventory=%#v err=%v", inventory, err)
	}
	if err := ApplyEvidenceSettlementReconciliationInventory(context.Background(), reader, issuer, inventory); err != nil {
		t.Fatal(err)
	}
	registry := issuer.Registry.(*memoryEvidenceRegistry)
	if _, err := registry.Resolve(context.Background(), registryport.MembershipQuery{Context: input.Context, ReceiptID: prepared.ReceiptID}); err == nil {
		t.Fatal("legacy quarantined settlement resolved as current membership")
	}
	verified, err := PreflightEvidenceSettlementInventory(context.Background(), reader, issuer)
	if err != nil || len(verified.Committed) != 0 || len(verified.Pending) != 0 || len(verified.Quarantined) != 1 {
		t.Fatalf("legacy settlement was reclassified for issuance: inventory=%#v err=%v", verified, err)
	}
	if registry.commitCalls != 0 || registry.registry.Sequence != 0 {
		t.Fatalf("legacy quarantine mutated evidence membership: calls=%d registry=%#v", registry.commitCalls, registry.registry)
	}
}

func settlementReader(input IssueEvidenceInput, marker domainevidence.HostEvidenceSettlementMarker) acceptedFinalPublicReaderStub {
	reader := settlementReaderWithoutResult(input)
	turn := reader.threads[input.Context.ThreadID]["turns"].([]any)[0].(map[string]any)
	settledAt := evidenceIssuerTime().Add(time.Minute)
	result := map[string]any{
		"id": input.ResultItemID, "kind": "tool_result", "role": "tool", "status": "completed",
		"threadId": input.Context.ThreadID, "turnId": input.Context.TurnID, "toolName": input.Grant.ToolName,
		"callId": input.Grant.ToolCallID, "contextDigest": input.Context.ContextDigest, "contextEpoch": float64(input.Context.ContextEpoch),
		"executionGrantId": input.Grant.GrantID, "createdAt": settledAt.Format(time.RFC3339Nano),
		"finishedAt": settledAt.Format(time.RFC3339Nano), "isError": false, "hostEvidenceSettlement": marker,
	}
	turn["items"] = append(turn["items"].([]any), result)
	return reader
}

func settlementReaderWithoutResult(input IssueEvidenceInput) acceptedFinalPublicReaderStub {
	contextRecord := strictTestRecord(input.Context)
	grantRecord := strictTestRecord(input.Grant)
	issuedAt := input.Grant.IssuedAt
	callItem := map[string]any{
		"id": "item_tool_" + input.Context.TurnID, "kind": "tool_call", "role": "assistant", "status": "completed",
		"threadId": input.Context.ThreadID, "turnId": input.Context.TurnID, "toolName": input.Grant.ToolName,
		"callId": input.Grant.ToolCallID, "contextDigest": input.Context.ContextDigest, "contextEpoch": float64(input.Context.ContextEpoch),
		"executionGrantId": input.Grant.GrantID, "executionGrant": grantRecord, "arguments": domaintoolcall.WithheldArgumentsProjectionV1(),
		"createdAt": issuedAt,
	}
	thread := map[string]any{
		"id": input.Context.ThreadID,
		"turns": []any{map[string]any{
			"id": input.Context.TurnID, "threadId": input.Context.ThreadID, "status": "completed",
			"securityContext": contextRecord, "items": []any{callItem},
		}},
	}
	return acceptedFinalPublicReaderStub{threads: map[string]map[string]any{input.Context.ThreadID: thread}}
}

func strictTestRecord(value any) map[string]any {
	body, _ := json.Marshal(value)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}

func cloneSettlementMap(value map[string]any) map[string]any {
	return strictTestRecord(value)
}
