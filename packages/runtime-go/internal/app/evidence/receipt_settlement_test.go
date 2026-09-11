package evidence

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

func TestPreparedEvidenceCannotResolveUntilExactDurableSettlementCommits(t *testing.T) {
	issuer, input, prepared, marker := legacyPreparedSettlementFixture(t)
	registry, err := issuer.Registry.Replay(context.Background(), input.Context)
	if err != nil || registry.Sequence != 0 {
		t.Fatalf("legacy prepared record created registry authority: sequence=%d err=%v", registry.Sequence, err)
	}
	if _, err := issuer.Registry.Resolve(context.Background(), registryport.MembershipQuery{Context: input.Context, ReceiptID: prepared.ReceiptID}); err == nil {
		t.Fatal("legacy prepared settlement resolved as current evidence")
	}
	authority := durableEvidenceSettlementAuthority(t, input, marker, evidenceIssuerTime().Add(time.Minute))
	receipt, err := issuer.Commit(context.Background(), CommitEvidenceInput{
		Context: input.Context, Marker: marker, Authority: authority,
	})
	if err == nil || receipt.ReceiptID != "" {
		t.Fatalf("legacy prepared settlement committed current authority: receipt=%#v err=%v", receipt, err)
	}
	again, err := issuer.Commit(context.Background(), CommitEvidenceInput{
		Context: input.Context, Marker: marker, Authority: authority,
	})
	if err == nil || again.ReceiptID != "" {
		t.Fatalf("legacy restart retry minted authority: receipt=%#v err=%v", again, err)
	}
	registry, err = issuer.Registry.Replay(context.Background(), input.Context)
	if err != nil || registry.Sequence != 0 || issuer.Registry.(*memoryEvidenceRegistry).commitCalls != 0 {
		t.Fatalf("legacy commit attempt mutated registry: registry=%#v err=%v", registry, err)
	}
}

func TestEvidenceSettlementRejectsProviderMarkerAndWrongDurableBinding(t *testing.T) {
	issuer, input, _, marker := legacyPreparedSettlementFixture(t)
	authority := durableEvidenceSettlementAuthority(t, input, marker, evidenceIssuerTime().Add(time.Minute))
	delete(authority.ResultItem, "hostEvidenceSettlement")
	authority.ResultItem["output"] = map[string]any{"hostEvidenceSettlement": settlementMarkerRecord(marker)}
	if receipt, err := issuer.Commit(context.Background(), CommitEvidenceInput{Context: input.Context, Marker: marker, Authority: authority}); err == nil || receipt.ReceiptID != "" {
		t.Fatalf("nested provider marker escaped legacy snapshot quarantine: receipt=%#v err=%v", receipt, err)
	}
	authority = durableEvidenceSettlementAuthority(t, input, marker, evidenceIssuerTime().Add(time.Minute))
	authority.ResultItem["callId"] = "call-other"
	if receipt, err := issuer.Commit(context.Background(), CommitEvidenceInput{Context: input.Context, Marker: marker, Authority: authority}); err == nil || receipt.ReceiptID != "" {
		t.Fatalf("wrong durable call escaped legacy snapshot quarantine: receipt=%#v err=%v", receipt, err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"wrong kind":   func(item map[string]any) { item["kind"] = "assistant_text" },
		"wrong status": func(item map[string]any) { item["status"] = "failed" },
		"wrong role":   func(item map[string]any) { item["role"] = "assistant" },
		"wrong epoch":  func(item map[string]any) { item["contextEpoch"] = float64(input.Context.ContextEpoch + 1) },
		"time inversion": func(item map[string]any) {
			item["createdAt"] = evidenceIssuerTime().Add(2 * time.Minute).Format(time.RFC3339Nano)
		},
		"missing finished": func(item map[string]any) { delete(item, "finishedAt") },
	} {
		t.Run(name, func(t *testing.T) {
			authority := durableEvidenceSettlementAuthority(t, input, marker, evidenceIssuerTime().Add(time.Minute))
			mutate(authority.ResultItem)
			if receipt, err := issuer.Commit(context.Background(), CommitEvidenceInput{Context: input.Context, Marker: marker, Authority: authority}); err == nil || receipt.ReceiptID != "" {
				t.Fatalf("invalid durable result escaped legacy snapshot quarantine: receipt=%#v err=%v", receipt, err)
			}
		})
	}
	registry, err := issuer.Registry.Replay(context.Background(), input.Context)
	if err != nil || registry.Sequence != 0 {
		t.Fatalf("rejected settlement mutated registry: sequence=%d err=%v", registry.Sequence, err)
	}
}

func durableEvidenceSettlementAuthority(t *testing.T, input IssueEvidenceInput, marker domainevidence.HostEvidenceSettlementMarker, settledAt time.Time) executiongrantapp.DurableSettlementAuthority {
	t.Helper()
	active, err := domainsecurity.ParseExecutionGrantRegistry(domainsecurity.ExecutionGrantRegistryRecord(input.GrantRegistry))
	if err != nil {
		t.Fatal(err)
	}
	settlementBase, err := domainsecurity.ParseExecutionGrantRegistry(domainsecurity.ExecutionGrantRegistryRecord(input.GrantRegistry))
	if err != nil {
		t.Fatal(err)
	}
	settled, err := domainsecurity.SettleRegisteredExecutionGrant(settlementBase, input.Grant.GrantID, settledAt)
	if err != nil {
		t.Fatal(err)
	}
	return executiongrantapp.DurableSettlementAuthority{
		ActiveRegistry: active, SettledRegistry: settled, SettledAt: settledAt,
		ResultItem: map[string]any{
			"id": input.ResultItemID, "kind": "tool_result", "threadId": input.Context.ThreadID, "turnId": input.Context.TurnID,
			"role": "tool", "status": "completed",
			"toolName": input.Grant.ToolName, "callId": input.Grant.ToolCallID, "contextDigest": input.Context.ContextDigest,
			"contextEpoch": float64(input.Context.ContextEpoch), "executionGrantId": input.Grant.GrantID,
			"createdAt": settledAt.Format(time.RFC3339Nano), "finishedAt": settledAt.Format(time.RFC3339Nano),
			"isError": false, "hostEvidenceSettlement": marker,
		},
	}
}

type memoryEvidenceSettlementStore struct {
	records map[string]domainevidence.PreparedEvidenceSettlement
}

func (store *memoryEvidenceSettlementStore) PutPreparedIfAbsent(_ context.Context, record domainevidence.PreparedEvidenceSettlement) error {
	if domainevidence.ValidatePreparedEvidenceSettlement(record) != nil {
		return errors.New("invalid prepared settlement")
	}
	if store.records == nil {
		store.records = map[string]domainevidence.PreparedEvidenceSettlement{}
	}
	if existing, ok := store.records[record.SettlementID]; ok && existing.RecordDigest != record.RecordDigest {
		return errors.New("conflicting prepared settlement")
	}
	store.records[record.SettlementID] = record
	return nil
}

func (store *memoryEvidenceSettlementStore) ResolvePrepared(_ context.Context, settlementID string) (domainevidence.PreparedEvidenceSettlement, error) {
	record, ok := store.records[settlementID]
	if !ok {
		return domainevidence.PreparedEvidenceSettlement{}, errors.New("prepared settlement not found")
	}
	return record, nil
}

func (store *memoryEvidenceSettlementStore) ListPrepared(context.Context) ([]domainevidence.PreparedEvidenceSettlement, error) {
	records := make([]domainevidence.PreparedEvidenceSettlement, 0, len(store.records))
	for _, record := range store.records {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].SettlementID < records[j].SettlementID })
	return records, nil
}

func (store *memoryEvidenceSettlementStore) HasRecords(ctx context.Context) (bool, error) {
	records, err := store.ListPrepared(ctx)
	return len(records) > 0, err
}
