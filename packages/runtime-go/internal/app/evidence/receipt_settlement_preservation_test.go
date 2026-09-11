package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

func TestHeldHistoricalEvidenceRegistryNeedsOriginalContextDenominator(t *testing.T) {
	issuer, input, prepared, _ := legacyPreparedSettlementFixture(t)
	registry := issuer.Registry.(*memoryEvidenceRegistry)
	if _, err := registry.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: input.Context, Draft: prepared.ReceiptDraft, CanonicalEvidence: prepared.CanonicalEvidence,
		SettlementProof: domainevidence.EvidenceSettlementProof{SettlementID: prepared.SettlementID, PreparedRecordDigest: prepared.RecordDigest},
		RegisteredAt:    evidenceIssuerTime().Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	delete(issuer.SettlementStore.(*memoryEvidenceSettlementStore).records, prepared.SettlementID)
	issuer.Registry = &historicalV1SettlementRegistry{memoryEvidenceRegistry: registry}
	executable := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{}}
	observer := originalSettlementObserverForTestV1(t, issuer, input, settlementReaderWithoutResult(input))
	inventory, err := PreflightEvidenceSettlementInventoryWithPreservationV1(context.Background(), executable, issuer, observer)
	if err != nil || len(inventory.Pending) != 0 || registry.commitCalls != 1 {
		t.Fatalf("held historical registry blocked independent settlement or acquired repair: pending=%d commits=%d err=%v", len(inventory.Pending), registry.commitCalls, err)
	}
	for restart := 0; restart < 2; restart++ {
		if err := ApplyEvidenceSettlementReconciliationInventoryWithPreservationV1(context.Background(), executable, issuer, inventory, observer); err != nil || registry.commitCalls != 1 {
			t.Fatalf("historical registry changed on restart %d: commits=%d err=%v", restart, registry.commitCalls, err)
		}
	}
}

type originalSettlementObserverTestV1 struct {
	input PreservedEvidenceSettlementInventoryV1
	err   error
	calls int
}

func (observer *originalSettlementObserverTestV1) ObserveOriginalEvidenceSettlementInventoryV1(ctx context.Context) (PreservedEvidenceSettlementInventoryV1, error) {
	observer.calls++
	if observer.err != nil || ctx.Err() != nil {
		return PreservedEvidenceSettlementInventoryV1{}, errors.Join(observer.err, ctx.Err())
	}
	body, err := json.Marshal(observer.input)
	var result PreservedEvidenceSettlementInventoryV1
	if err == nil {
		err = json.Unmarshal(body, &result)
	}
	return result, err
}

func originalSettlementObserverForTestV1(t *testing.T, issuer Issuer, input IssueEvidenceInput, reader acceptedFinalPublicReaderStub) *originalSettlementObserverTestV1 {
	t.Helper()
	prepared, err := issuer.SettlementStore.ListPrepared(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	registries, err := issuer.Registry.(registryport.Inventory).ListRegistries(context.Background(), []domainsecurity.TurnSecurityContext{input.Context})
	if err != nil {
		t.Fatal(err)
	}
	thread := cloneSettlementMap(reader.threads[input.Context.ThreadID])
	body, err := json.Marshal(thread)
	if err != nil {
		t.Fatal(err)
	}
	historical, _ := issuer.Registry.(historicalV1EvidenceRegistryAuthority)
	return &originalSettlementObserverTestV1{input: PreservedEvidenceSettlementInventoryV1{
		Threads:  []recoveryport.PrimaryThreadSnapshotV1{{ThreadID: input.Context.ThreadID, Thread: thread, ThreadFileSHA256: domainsecurity.SHA256Hex(body)}},
		Prepared: prepared, Registries: registries,
		HistoricalRegistryV1: historical != nil && historical.EvidenceRegistryHistoricalV1Authority(),
	}}
}

func TestHeldEvidenceSettlementPreservesValidatedRecordsWithoutCommit(t *testing.T) {
	for _, result := range []bool{false, true} {
		t.Run(map[bool]string{false: "prepared only", true: "durable marker"}[result], func(t *testing.T) {
			issuer, input, prepared, marker := legacyPreparedSettlementFixture(t)
			reader := settlementReaderWithoutResult(input)
			if result {
				reader = settlementReader(input, marker)
			}
			observer := originalSettlementObserverForTestV1(t, issuer, input, reader)
			executable := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{}}
			before, _ := json.Marshal(observer.input)
			inventory, err := PreflightEvidenceSettlementInventoryWithPreservationV1(context.Background(), executable, issuer, observer)
			if err != nil || len(inventory.Preserved) != 1 || !reflect.DeepEqual(inventory.Preserved[0], prepared) || len(inventory.Pending)+len(inventory.Committed)+len(inventory.Abandoned)+len(inventory.Quarantined) != 0 {
				t.Fatalf("held settlement classification: preserved=%d pending=%d err=%v", len(inventory.Preserved), len(inventory.Pending), err)
			}
			for restart := 0; restart < 2; restart++ {
				if err := ApplyEvidenceSettlementReconciliationInventoryWithPreservationV1(context.Background(), executable, issuer, inventory, observer); err != nil {
					t.Fatal(err)
				}
			}
			after, _ := json.Marshal(observer.input)
			if string(before) != string(after) || issuer.Registry.(*memoryEvidenceRegistry).commitCalls != 0 {
				t.Fatal("held settlement changed or committed")
			}
		})
	}
}

func TestHeldEvidenceSettlementRejectsIncompleteOrChangedAuthority(t *testing.T) {
	for _, name := range []string{"missing primary", "duplicate primary", "missing prepared", "duplicate prepared", "bad signature", "wrong result", "orphan marker", "missing item identity", "observer error", "executable overlap", "missing grant prefix", "padded call identity", "wrong error flag type", "missing call arguments", "wrong call arguments", "corrupt trailing grant"} {
		t.Run(name, func(t *testing.T) {
			issuer, input, _, marker := legacyPreparedSettlementFixture(t)
			reader := settlementReader(input, marker)
			observer := originalSettlementObserverForTestV1(t, issuer, input, reader)
			executable := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{}}
			items := observer.input.Threads[0].Thread["turns"].([]any)[0].(map[string]any)["items"].([]any)
			item := items[len(items)-1].(map[string]any)
			switch name {
			case "missing primary":
				observer.input.Threads = nil
			case "duplicate primary":
				observer.input.Threads = append(observer.input.Threads, observer.input.Threads[0])
			case "missing prepared":
				observer.input.Prepared = nil
			case "duplicate prepared":
				observer.input.Prepared = append(observer.input.Prepared, observer.input.Prepared[0])
			case "bad signature":
				observer.input.Prepared[0].AuthoritySignature = "bad"
			case "wrong result":
				item["callId"] = "foreign-call"
			case "orphan marker":
				item["hostEvidenceSettlement"] = map[string]any{"settlementId": "bad"}
			case "missing item identity":
				delete(item, "id")
			case "observer error":
				observer.err = errors.New("original settlement read failed")
			case "executable overlap":
				executable = reader
			case "missing grant prefix":
				observer.input.Threads[0].Thread["turns"].([]any)[0].(map[string]any)["items"] = items[1:]
			case "padded call identity":
				item["callId"] = " " + input.Grant.ToolCallID + " "
			case "wrong error flag type":
				item["isError"] = "true"
			case "missing call arguments":
				delete(items[0].(map[string]any), "arguments")
			case "wrong call arguments":
				items[0].(map[string]any)["arguments"] = map[string]any{"changed": true}
			case "corrupt trailing grant":
				trailing := cloneSettlementMap(items[0].(map[string]any))
				trailing["id"], trailing["executionGrant"] = "trailing-call", nil
				observer.input.Threads[0].Thread["turns"].([]any)[0].(map[string]any)["items"] = append(items, trailing)
			}
			if inventory, err := PreflightEvidenceSettlementInventoryWithPreservationV1(context.Background(), executable, issuer, observer); err == nil || len(inventory.Pending) != 0 || issuer.Registry.(*memoryEvidenceRegistry).commitCalls != 0 {
				t.Fatalf("invalid held settlement accepted or committed: pending=%d err=%v", len(inventory.Pending), err)
			}
		})
	}
}

func TestHeldHistoricalSettlementRegistryUnavailableKeepsObservedInventory(t *testing.T) {
	issuer, input, prepared, _ := legacyPreparedSettlementFixture(t)
	registry := issuer.Registry.(*memoryEvidenceRegistry)
	if _, err := registry.CommitPrepared(context.Background(), registryport.CommitPreparedInput{Context: input.Context, Draft: prepared.ReceiptDraft, CanonicalEvidence: prepared.CanonicalEvidence, SettlementProof: domainevidence.EvidenceSettlementProof{SettlementID: prepared.SettlementID, PreparedRecordDigest: prepared.RecordDigest}, RegisteredAt: evidenceIssuerTime().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	delete(issuer.SettlementStore.(*memoryEvidenceSettlementStore).records, prepared.SettlementID)
	issuer.Registry = &historicalV1SettlementRegistry{memoryEvidenceRegistry: registry}
	observer := originalSettlementObserverForTestV1(t, issuer, input, settlementReaderWithoutResult(input))
	blocked := &unavailableSettlementRegistryV1{memoryEvidenceRegistry: registry}
	issuer.Registry = blocked
	executable := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{}}
	inventory, err := PreflightEvidenceSettlementInventoryWithPreservationV1(context.Background(), executable, issuer, observer)
	if err != nil || len(inventory.Pending) != 0 || blocked.listCalls != 0 || registry.commitCalls != 1 {
		t.Fatalf("unavailable capability erased original held registry: pending=%d lists=%d commits=%d err=%v", len(inventory.Pending), blocked.listCalls, registry.commitCalls, err)
	}
	if err := ApplyEvidenceSettlementReconciliationInventoryWithPreservationV1(context.Background(), executable, issuer, inventory, observer); err != nil || blocked.listCalls != 0 || registry.commitCalls != 1 {
		t.Fatalf("held registry reached unavailable live owner: lists=%d commits=%d err=%v", blocked.listCalls, registry.commitCalls, err)
	}
}

func TestHistoricalSettlementPrefixKeepsPriorCreatedAtAndAuditCallIdentity(t *testing.T) {
	for _, legacyCall := range []bool{false, true} {
		t.Run(map[bool]string{false: "host call", true: "historical provider call"}[legacyCall], func(t *testing.T) {
			_, input, prepared, marker := legacyPreparedSettlementFixture(t)
			callID := evidenceTestHostToolCallID(t, "earlier-settlement")
			if legacyCall {
				callID = "historical-provider-call"
			}
			grant := input.Grant
			issued, _ := time.Parse(time.RFC3339Nano, grant.IssuedAt)
			expires, _ := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
			prior := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{Context: input.Context, Provider: grant.Provider, ServerIdentity: grant.ServerIdentity, ToolName: grant.ToolName, ToolCallID: callID, ConnectionEpoch: grant.ConnectionEpoch, ArgsHash: grant.ArgsHash, SchemaHash: grant.SchemaHash, ScopeHash: grant.ScopeHash, ReadOnly: grant.ReadOnly, ApprovalState: grant.ApprovalState, IssuedAt: issued, ExpiresAt: expires})
			reader := settlementReader(input, marker)
			turn := reader.threads[input.Context.ThreadID]["turns"].([]any)[0].(map[string]any)
			items := turn["items"].([]any)
			targetCall := items[0].(map[string]any)
			priorCall := cloneSettlementMap(targetCall)
			priorCall["id"], priorCall["executionGrantId"], priorCall["executionGrant"], priorCall["callId"] = "prior-call", prior.GrantID, strictTestRecord(prior), prior.ToolCallID
			priorResult := cloneSettlementMap(items[1].(map[string]any))
			priorResult["id"], priorResult["executionGrantId"], priorResult["callId"] = "prior-result", prior.GrantID, prior.ToolCallID
			delete(priorResult, "hostEvidenceSettlement")
			delete(priorResult, "finishedAt")
			priorResult["createdAt"] = issued.Add(10 * time.Second).Format(time.RFC3339Nano)
			targetCall["createdAt"] = issued.Add(20 * time.Second).Format(time.RFC3339Nano)
			// Only this read-only prefix verdict is under test; the outer owner
			// separately requires its prepared record's installation signature.
			registry := domainsecurity.SealExecutionGrantRegistry(domainsecurity.ExecutionGrantRegistry{Version: 1, ThreadID: input.Context.ThreadID, Entries: []domainsecurity.ExecutionGrantRegistryEntry{
				{Sequence: 1, ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, ContextDigest: input.Context.ContextDigest, Grant: prior, Status: domainsecurity.GrantRegistrySettled, RegisteredAt: issued.Format(time.RFC3339Nano), UpdatedAt: issued.Add(10 * time.Second).Format(time.RFC3339Nano)},
				{Sequence: 2, ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, ContextDigest: input.Context.ContextDigest, Grant: grant, Status: domainsecurity.GrantRegistryActive, RegisteredAt: issued.Add(20 * time.Second).Format(time.RFC3339Nano), UpdatedAt: issued.Add(20 * time.Second).Format(time.RFC3339Nano)},
			}})
			prepared.ActiveGrantRegistrySequence, prepared.ActiveGrantRegistryDigest = registry.Sequence, registry.StateDigest
			all := []map[string]any{priorCall, priorResult, targetCall, items[1].(map[string]any)}
			if err := validateHistoricalSettlementPrefixV1(prepared, all); err != nil {
				t.Fatal(err)
			}
			if legacyCall && domainsecurity.ValidateExecutionGrantRegistry(registry) == nil {
				t.Fatal("audit registry acquired current execution validity")
			}
			priorResult["createdAt"] = issued.Add(11 * time.Second).Format(time.RFC3339Nano)
			if err := validateHistoricalSettlementPrefixV1(prepared, all); err == nil {
				t.Fatal("historical changed prefix accepted")
			}
		})
	}
}

type originalRegistryInventoryObserverForTestV1 struct {
	*originalSettlementObserverTestV1
	records        []registryport.InventoryRecord
	err            error
	inventoryCalls int
}

func (observer *originalRegistryInventoryObserverForTestV1) ObserveRestartEvidenceRegistryInventoryV1(ctx context.Context, contexts []domainsecurity.TurnSecurityContext) ([]registryport.InventoryRecord, error) {
	observer.inventoryCalls++
	if observer.err != nil {
		return nil, observer.err
	}
	return append([]registryport.InventoryRecord{}, observer.records...), ctx.Err()
}

type rejectingLiveSettlementInventoryForTestV1 struct {
	*memoryEvidenceRegistry
	calls int
}

func (registry *rejectingLiveSettlementInventoryForTestV1) ListRegistries(context.Context, []domainsecurity.TurnSecurityContext) ([]registryport.InventoryRecord, error) {
	registry.calls++
	return nil, errors.New("live inventory cannot read held historical context")
}

func TestHeldSettlementUsesExplicitOriginalRegistryInventoryWithAvailableLiveOwner(t *testing.T) {
	issuer, input, _, _ := legacyPreparedSettlementFixture(t)
	base := issuer.Registry.(*memoryEvidenceRegistry)
	original := originalSettlementObserverForTestV1(t, issuer, input, settlementReaderWithoutResult(input))
	observer := &originalRegistryInventoryObserverForTestV1{originalSettlementObserverTestV1: original, records: original.input.Registries}
	live := &rejectingLiveSettlementInventoryForTestV1{memoryEvidenceRegistry: base}
	issuer.Registry = live
	executable := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{}}
	inventory, err := PreflightEvidenceSettlementInventoryWithPreservationV1(context.Background(), executable, issuer, observer)
	if err != nil || len(inventory.Preserved) != 1 || live.calls != 0 || observer.inventoryCalls == 0 || base.commitCalls != 0 {
		t.Fatalf("startup inventory reached live held reader: calls=%d original=%d commits=%d err=%v", live.calls, observer.inventoryCalls, base.commitCalls, err)
	}
	if err := ApplyEvidenceSettlementReconciliationInventoryWithPreservationV1(context.Background(), executable, issuer, inventory, observer); err != nil || live.calls != 0 || base.commitCalls != 0 {
		t.Fatalf("held startup inventory entered live effects: %v", err)
	}
	cause := errors.New("original registry observation failed")
	observer.err = cause
	if _, err := PreflightEvidenceSettlementInventoryWithPreservationV1(context.Background(), executable, issuer, observer); !errors.Is(err, cause) {
		t.Fatalf("original registry observation cause lost: %v", err)
	}
}
