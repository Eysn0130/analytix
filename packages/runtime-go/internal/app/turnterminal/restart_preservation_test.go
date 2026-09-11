package turnterminal

import (
	"context"
	"errors"
	"reflect"
	"testing"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	terminaltest "analytix.local/runtime-go/internal/testsupport/turnterminal"
)

func installTerminalPreservationForTest(t *testing.T, coordinator *Coordinator, contexts []domainsecurity.TurnSecurityContext) {
	t.Helper()
	if err := coordinator.PreserveRestartContextsV1(context.Background(), contexts); err != nil {
		t.Fatal(err)
	}
}

func TestRestartPreservedTerminalPrefixesNeverCompleteOrReclassify(t *testing.T) {
	for _, prefix := range []string{"private_only", "intent", "complete", "legacy_winner", "advanced_private", "advanced_intent"} {
		t.Run(prefix, func(t *testing.T) {
			fixture, err := terminaltest.NewFixtureV1()
			if err != nil {
				t.Fatal(err)
			}
			trace := &coordinatorTraceV1{}
			terminals := &memoryTerminalStoreV1{trace: trace}
			privateFinals := &memoryPrivateFinalStoreV1{trace: trace, private: fixture.PrivateFinal}
			completion := &memoryCompletionCASV1{trace: trace, context: fixture.Context}
			closer := &memoryProviderCloserV1{trace: trace, closure: fixture.Closure}
			newOwner := func() *Coordinator {
				owner, err := NewCoordinator(newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals, terminals, closer)
				if err != nil {
					t.Fatal(err)
				}
				return owner
			}
			if prefix == "intent" || prefix == "advanced_intent" {
				terminals.intent = &fixture.Intent
			}
			if prefix == "legacy_winner" {
				completion.winner = fixture.PrivateFinal.AcceptedFinal
			}
			if prefix == "complete" {
				if _, err := newOwner().CommitV1(context.Background(), CommitInputV1{
					CompletionStore: completion, CASReader: completion, PrivateFinal: fixture.PrivateFinal,
				}); err != nil {
					t.Fatal(err)
				}
			}
			coordinator := newOwner()
			contexts := []domainsecurity.TurnSecurityContext{fixture.Context}
			if prefix == "advanced_private" || prefix == "advanced_intent" {
				completion.current = newerTerminalContextV1(t, fixture.Context)
				contexts = append(contexts, completion.current)
			}
			installTerminalPreservationForTest(t, coordinator, contexts)
			beforePublic, beforeClose := completion.publicCalls, closer.calls
			trace.entries = nil
			result, err := coordinator.RecoverV1(context.Background(), RestartRecoveryInputV1{
				CompletionStore: completion, CASReader: completion,
				PrivateInventory: []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal},
				Candidates:       []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal},
			})
			if err != nil {
				t.Fatal(err)
			}
			if completion.publicCalls != beforePublic || closer.calls != beforeClose || terminalRecoveryWriteCountV1(trace.snapshot()) != 0 {
				t.Error("held prefix acquired terminal recovery effects")
			}
			if len(result.Complete) != 0 || len(result.LegacyQuarantined) != 0 || len(result.AuditOnly) != 0 || len(result.NonExecutableAuditOnly) != 0 {
				t.Error("held prefix became completed or audit/quarantine authority")
			}
			if !reflect.DeepEqual(result.Preserved, []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal}) {
				t.Fatal("original held record was lost or rewritten")
			}
			input := CommitInputV1{CompletionStore: completion, CASReader: completion, PrivateFinal: fixture.PrivateFinal}
			if _, err := coordinator.CommitV1(context.Background(), input); !errors.Is(err, ErrRestartPreserved) {
				t.Fatalf("held live commit was admitted: %v", err)
			}
			if _, err := coordinator.ResolveCommittedV1(context.Background(), input); !errors.Is(err, ErrRestartPreserved) {
				t.Fatalf("held authority was returned for replay: %v", err)
			}
			if terminalRecoveryWriteCountV1(trace.snapshot()) != 0 {
				t.Fatal("held live/replay entry changed terminal state")
			}
		})
	}
}

func TestRestartPreservedTerminalStillRejectsInvalidWholeInventory(t *testing.T) {
	for _, fault := range []string{"changed_intent", "omitted_private", "duplicate_candidate", "wrong_held_context", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			fixture, err := terminaltest.NewFixtureV1()
			if err != nil {
				t.Fatal(err)
			}
			trace := &coordinatorTraceV1{}
			terminals := &memoryTerminalStoreV1{trace: trace}
			completion := &memoryCompletionCASV1{trace: trace, context: fixture.Context}
			coordinator, err := NewCoordinator(newTrustedInventoryAuthorityV1(fixture.PrivateKey),
				&memoryPrivateFinalStoreV1{trace: trace, private: fixture.PrivateFinal}, terminals,
				&memoryProviderCloserV1{trace: trace, closure: fixture.Closure})
			if err != nil {
				t.Fatal(err)
			}
			heldContext := fixture.Context
			if fault == "wrong_held_context" {
				heldContext = newerTerminalContextV1(t, fixture.Context)
			}
			installTerminalPreservationForTest(t, coordinator, []domainsecurity.TurnSecurityContext{heldContext})
			input := RestartRecoveryInputV1{CompletionStore: completion, CASReader: completion,
				PrivateInventory: []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal},
				Candidates:       []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal}}
			ctx := context.Background()
			switch fault {
			case "changed_intent":
				changed := fixture.Intent
				changed.AcceptedFinalDigest = domainsecurity.SHA256Hex([]byte("different-final"))
				terminals.intent = &changed
			case "omitted_private":
				input.PrivateInventory, input.Candidates = nil, nil
			case "duplicate_candidate":
				input.Candidates = append(input.Candidates, fixture.PrivateFinal)
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			if _, err := coordinator.RecoverV1(ctx, input); err == nil {
				t.Fatal("preservation hid invalid terminal authority")
			}
			if terminalRecoveryWriteCountV1(trace.snapshot()) != 0 {
				t.Fatal("invalid inventory caused terminal effects")
			}
		})
	}
}

func TestTerminalPreservationInstallationClosesBeforeIndependentRecovery(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	trace := &coordinatorTraceV1{}
	terminals := &memoryTerminalStoreV1{trace: trace}
	completion := &memoryCompletionCASV1{trace: trace, context: fixture.Context}
	coordinator, err := NewCoordinator(newTrustedInventoryAuthorityV1(fixture.PrivateKey),
		&memoryPrivateFinalStoreV1{trace: trace, private: fixture.PrivateFinal}, terminals,
		&memoryProviderCloserV1{trace: trace, closure: fixture.Closure})
	if err != nil {
		t.Fatal(err)
	}
	frozen := fixture.Context
	held, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-other-held", TurnID: "turn-other-held", WorkspaceRealPath: frozen.WorkspaceRealPath,
		TenantID: frozen.TenantID, UserID: frozen.UserID, CaseID: frozen.CaseID, CaseBindingHash: frozen.CaseBindingHash,
		DatasetSnapshotID: frozen.DatasetSnapshotID, SourceManifestHash: frozen.SourceManifestHash,
		ContextEpoch: frozen.ContextEpoch, IssuedAt: terminaltest.FixtureTimeV1(),
		PublicationPolicy: frozen.PublicationPolicy, RiskAuthorityBinding: frozen.RiskAuthorityBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]domainsecurity.TurnSecurityContext{{{}}, {held, held}} {
		if err := coordinator.PreserveRestartContextsV1(context.Background(), invalid); err == nil || coordinator.restartPreserved != nil {
			t.Fatal("invalid scope installed partial terminal preservation")
		}
	}
	installTerminalPreservationForTest(t, coordinator, []domainsecurity.TurnSecurityContext{held})
	if err := coordinator.PreserveRestartContextsV1(context.Background(), nil); err == nil {
		t.Fatal("empty replacement released terminal preservation")
	}
	result, err := coordinator.RecoverV1(context.Background(), RestartRecoveryInputV1{
		CompletionStore: completion, CASReader: completion,
		Candidates: []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal},
	})
	if err != nil || len(result.Complete) != 1 || len(result.Preserved) != 0 || completion.publicCalls != 1 {
		t.Fatalf("independent terminal recovery failed: complete=%d preserved=%d public=%d err=%v", len(result.Complete), len(result.Preserved), completion.publicCalls, err)
	}
	if err := coordinator.PreserveRestartContextsV1(context.Background(), []domainsecurity.TurnSecurityContext{frozen}); err == nil {
		t.Fatal("scope installed after terminal effect admission")
	}
	plain, err := NewCoordinator(coordinator.authority, coordinator.privateFinals, coordinator.terminals, coordinator.providerTurns)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plain.RecoverV1(context.Background(), RestartRecoveryInputV1{
		CompletionStore: completion, CASReader: completion,
		Candidates: []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal},
	}); err != nil {
		t.Fatal(err)
	}
	if err := plain.PreserveRestartContextsV1(context.Background(), []domainsecurity.TurnSecurityContext{frozen}); err == nil {
		t.Fatal("first scope installed after an unheld terminal consumer returned authority")
	}
}
