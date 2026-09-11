package evidenceregistry

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	appturn "analytix.local/runtime-go/internal/app/turn"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	datasetsnapshotv2fixture "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestWitnessedRegistryCommitResolveRevokeAndIdempotentRetries(t *testing.T) {
	fixture := newWitnessedRegistryFixture(t)
	input := witnessedRegistryIssueInput(t, fixture.securityContext, "first")
	rawProviderID := "provider_call_6222020202020202020"
	unsafe := input
	unsafe.Draft.ToolCallID = rawProviderID
	if receipt, err := fixture.service.CommitPrepared(context.Background(), unsafe); err == nil || receipt.ReceiptID != "" ||
		strings.Contains(err.Error(), rawProviderID) || fixture.coordinator.advanceCalls != 0 {
		t.Fatalf("raw provider identity reached witnessed evidence registry: receipt=%#v err=%v advances=%d", receipt, err, fixture.coordinator.advanceCalls)
	}
	receipt, err := fixture.service.CommitPrepared(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.coordinator.advanceCalls != 1 || fixture.coordinator.observeCalls == 0 {
		t.Fatalf("registration did not use fresh witness CAS: observe=%d advance=%d", fixture.coordinator.observeCalls, fixture.coordinator.advanceCalls)
	}
	resolved, err := fixture.service.Resolve(context.Background(), registryport.MembershipQuery{Context: fixture.securityContext, ReceiptID: receipt.ReceiptID})
	if err != nil || resolved.Revoked || resolved.Receipt.ReceiptDigest != receipt.ReceiptDigest {
		t.Fatalf("witness-selected receipt membership mismatch: resolved=%#v err=%v", resolved, err)
	}
	if replayed, err := fixture.service.Replay(context.Background(), fixture.securityContext); err != nil || replayed.Sequence != 1 {
		t.Fatalf("witness-selected registry replay mismatch: registry=%#v err=%v", replayed, err)
	}
	snapshotCalls := 0
	if err := fixture.service.WithWitnessedSnapshot(context.Background(), fixture.securityContext, func(snapshot registryport.WitnessedSnapshot) error {
		snapshotCalls++
		if snapshot.Head.Bundle.EvidenceRegistryIndexDigest != snapshot.RootIndex.IndexDigest ||
			snapshot.Head.Bundle.EvidenceRegistryCount != snapshot.RootIndex.Generation || snapshot.Registry.Sequence != 1 ||
			!snapshot.HasSelection || snapshot.SelectedIndex.IndexDigest != snapshot.RootIndex.IndexDigest ||
			snapshot.SelectedCapsule.RecordDigest != snapshot.SelectedIndex.Entry.CapsuleRecordDigest ||
			len(snapshot.RegistryIndexPath) != 1 || snapshot.RegistryIndexPath[0].IndexDigest != snapshot.RootIndex.IndexDigest {
			t.Fatalf("witnessed snapshot is internally inconsistent: %#v", snapshot)
		}
		return nil
	}); err != nil || snapshotCalls != 1 {
		t.Fatalf("witnessed snapshot callback mismatch: calls=%d err=%v", snapshotCalls, err)
	}
	if prefix, err := fixture.service.ReplayAt(context.Background(), fixture.securityContext, 0); err != nil || prefix.Sequence != 0 {
		t.Fatalf("historical prefix mismatch: prefix=%#v err=%v", prefix, err)
	}

	retried, err := fixture.service.CommitPrepared(context.Background(), input)
	if err != nil || retried.ReceiptDigest != receipt.ReceiptDigest || fixture.coordinator.advanceCalls != 1 {
		t.Fatalf("exact registration retry advanced authority: receipt=%#v err=%v advances=%d", retried, err, fixture.coordinator.advanceCalls)
	}
	revoke := registryport.RevokeInput{
		Context: fixture.securityContext, ReceiptID: receipt.ReceiptID, ReasonCode: "source_retracted", RevokedAt: fixture.now.Add(time.Minute),
	}
	if err := fixture.service.Revoke(context.Background(), revoke); err != nil {
		t.Fatal(err)
	}
	if fixture.coordinator.advanceCalls != 2 {
		t.Fatalf("revocation did not advance the witnessed registry child exactly once: %d", fixture.coordinator.advanceCalls)
	}
	if _, err := fixture.service.Resolve(context.Background(), registryport.MembershipQuery{Context: fixture.securityContext, ReceiptID: receipt.ReceiptID}); err == nil {
		t.Fatal("revoked receipt remained current membership")
	}
	if err := fixture.service.Revoke(context.Background(), revoke); err != nil || fixture.coordinator.advanceCalls != 2 {
		t.Fatalf("exact revocation retry advanced authority: err=%v advances=%d", err, fixture.coordinator.advanceCalls)
	}
	if fixture.coordinator.bundle.DatasetSnapshotCount != 1 ||
		fixture.coordinator.bundle.DatasetSnapshotIndexDigest != fixture.datasetSelection.SelectedIndex.IndexDigest ||
		fixture.coordinator.bundle.PublicationCount != 0 {
		t.Fatalf("registry operations changed another shared child: %#v", fixture.coordinator.bundle)
	}
}

func TestWitnessedRegistryInventorySkipsAuthorityFreeOrdinaryAndRejectsDetachedOrNonFactTurns(t *testing.T) {
	fixture := newWitnessedRegistryFixture(t)
	ordinary, err := testsecurity.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-ordinary-registry-inventory", TurnID: "turn-ordinary-registry-inventory",
		WorkspaceRealPath: "/workspace/ordinary-registry-inventory", ContextEpoch: 2, IssuedAt: fixture.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeEmptyInventoryObserve := fixture.coordinator.observeCalls
	if records, err := fixture.service.ListRegistries(context.Background(), []domainsecurity.TurnSecurityContext{ordinary}); err != nil || len(records) != 0 {
		t.Fatalf("ordinary turn without registry authority did not remain source-independent: records=%#v err=%v", records, err)
	}
	if fixture.coordinator.observeCalls != beforeEmptyInventoryObserve {
		t.Fatal("host-proven empty registry inventory reached the shared witness")
	}
	if _, err := fixture.service.CommitPrepared(
		context.Background(), witnessedRegistryIssueInput(t, fixture.securityContext, "inventory-case"),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.ListRegistries(
		context.Background(), []domainsecurity.TurnSecurityContext{ordinary},
	); err == nil {
		t.Fatal("non-empty registry head passed without its durable case turn context")
	}
	staleCase, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID,
		WorkspaceRealPath: fixture.securityContext.WorkspaceRealPath, CaseID: fixture.securityContext.CaseID,
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("stale-registry-inventory-binding")),
		ContextEpoch:    fixture.securityContext.ContextEpoch + 1, IssuedAt: fixture.now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.ListRegistries(
		context.Background(), []domainsecurity.TurnSecurityContext{staleCase},
	); err == nil {
		t.Fatal("same turn identity with a stale context digest absorbed current registry authority")
	}
	records, err := fixture.service.ListRegistries(
		context.Background(), []domainsecurity.TurnSecurityContext{fixture.securityContext, ordinary},
	)
	if err != nil || len(records) != 1 || !reflect.DeepEqual(records[0].Context, fixture.securityContext) ||
		records[0].Registry.Sequence != 1 {
		t.Fatalf("mixed case/ordinary inventory did not select only the exact case registry: records=%#v err=%v", records, err)
	}
	collidingOrdinary, err := testsecurity.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID,
		WorkspaceRealPath: fixture.securityContext.WorkspaceRealPath, ContextEpoch: 2, IssuedAt: fixture.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.ListRegistries(
		context.Background(), []domainsecurity.TurnSecurityContext{collidingOrdinary},
	); err == nil {
		t.Fatal("ordinary durable turn identity silently absorbed existing case registry authority")
	}
	boundaryOnly, err := testsecurity.BoundaryOnlyContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID,
		WorkspaceRealPath: fixture.securityContext.WorkspaceRealPath,
		ContextEpoch:      fixture.securityContext.ContextEpoch + 1, IssuedAt: fixture.now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.ListRegistries(
		context.Background(), []domainsecurity.TurnSecurityContext{boundaryOnly},
	); err == nil {
		t.Fatal("boundary-only case turn identity silently absorbed fact registry authority")
	}
}

func TestWitnessedRegistryInventoryRejectsPerIdentitySequenceGaps(t *testing.T) {
	t.Run("first sequence gap", func(t *testing.T) {
		fixture := newWitnessedRegistryFixture(t)
		registry, err := domainevidence.NewEvidenceReceiptRegistry(fixture.securityContext)
		if err != nil {
			t.Fatal(err)
		}
		for _, label := range []string{"gap-first-a", "gap-first-b"} {
			input := witnessedRegistryIssueInput(t, fixture.securityContext, label)
			registry, _, err = domainevidence.RegisterEvidenceReceipt(
				registry, input.Draft, input.CanonicalEvidence, input.SettlementProof, input.RegisteredAt,
			)
			if err != nil {
				t.Fatal(err)
			}
		}
		capsule, index := fixture.candidate(t, fixture.coordinator.bundle, registry, "first-sequence-gap")
		if err := fixture.capsules.PutIfAbsent(context.Background(), capsule); err != nil {
			t.Fatal(err)
		}
		if err := fixture.indexes.PutIfAbsent(context.Background(), index); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.coordinator.AdvanceEvidenceRegistry(context.Background(), evidenceauthorityport.RegistryAdvanceInput{
			ExpectedBundleDigest: fixture.coordinator.bundle.RecordDigest, NextIndexDigest: index.IndexDigest,
		}); err != nil {
			t.Fatal(err)
		}
		fixture.service.authorityKnownEmpty = false
		if _, err := fixture.service.ListRegistries(
			context.Background(), []domainsecurity.TurnSecurityContext{fixture.securityContext},
		); err == nil {
			t.Fatal("signed first registry index skipped per-identity sequence one")
		}
	})

	t.Run("extension gap", func(t *testing.T) {
		fixture := newWitnessedRegistryFixture(t)
		registry, err := domainevidence.NewEvidenceReceiptRegistry(fixture.securityContext)
		if err != nil {
			t.Fatal(err)
		}
		firstInput := witnessedRegistryIssueInput(t, fixture.securityContext, "gap-extension-a")
		registry, _, err = domainevidence.RegisterEvidenceReceipt(
			registry, firstInput.Draft, firstInput.CanonicalEvidence, firstInput.SettlementProof, firstInput.RegisteredAt,
		)
		if err != nil {
			t.Fatal(err)
		}
		firstCapsule, firstIndex := fixture.candidate(t, fixture.coordinator.bundle, registry, "extension-first")
		if err := fixture.capsules.PutIfAbsent(context.Background(), firstCapsule); err != nil {
			t.Fatal(err)
		}
		if err := fixture.indexes.PutIfAbsent(context.Background(), firstIndex); err != nil {
			t.Fatal(err)
		}
		firstHead, err := fixture.coordinator.AdvanceEvidenceRegistry(context.Background(), evidenceauthorityport.RegistryAdvanceInput{
			ExpectedBundleDigest: fixture.coordinator.bundle.RecordDigest, NextIndexDigest: firstIndex.IndexDigest,
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, label := range []string{"gap-extension-b", "gap-extension-c"} {
			input := witnessedRegistryIssueInput(t, fixture.securityContext, label)
			registry, _, err = domainevidence.RegisterEvidenceReceipt(
				registry, input.Draft, input.CanonicalEvidence, input.SettlementProof, input.RegisteredAt,
			)
			if err != nil {
				t.Fatal(err)
			}
		}
		gapCapsule, gapIndex := fixture.candidate(t, firstHead.Bundle, registry, "extension-gap")
		if err := fixture.capsules.PutIfAbsent(context.Background(), gapCapsule); err != nil {
			t.Fatal(err)
		}
		if err := fixture.indexes.PutIfAbsent(context.Background(), gapIndex); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.coordinator.AdvanceEvidenceRegistry(context.Background(), evidenceauthorityport.RegistryAdvanceInput{
			ExpectedBundleDigest: firstHead.Bundle.RecordDigest, NextIndexDigest: gapIndex.IndexDigest,
		}); err != nil {
			t.Fatal(err)
		}
		fixture.service.authorityKnownEmpty = false
		if _, err := fixture.service.ListRegistries(
			context.Background(), []domainsecurity.TurnSecurityContext{fixture.securityContext},
		); err == nil {
			t.Fatal("signed global chain skipped a per-identity registry extension")
		}
	})
}

func TestWitnessedSnapshotCapabilityIsExactCallbackScopedAndCancellationAware(t *testing.T) {
	t.Run("callback close independently expires capability", func(t *testing.T) {
		fixture := newWitnessedRegistryFixture(t)
		if _, err := fixture.service.CommitPrepared(context.Background(), witnessedRegistryIssueInput(t, fixture.securityContext, "snapshot-close")); err != nil {
			t.Fatal(err)
		}
		var leaked registryport.WitnessedSnapshotCapability
		var leakedSnapshot registryport.WitnessedSnapshot
		if err := fixture.service.WithWitnessedSnapshotAuthority(context.Background(), fixture.securityContext, func(
			snapshot registryport.WitnessedSnapshot,
			capability registryport.WitnessedSnapshotCapability,
		) error {
			leaked, leakedSnapshot = capability, snapshot
			return capability.UseExact(snapshot, func() error { return nil })
		}); err != nil {
			t.Fatal(err)
		}
		called := false
		if leaked == nil || leaked.UseExact(leakedSnapshot, func() error { called = true; return nil }) == nil || called {
			t.Fatal("witnessed snapshot capability remained active after its callback")
		}
	})

	t.Run("nested aliases cannot mutate frozen authority", func(t *testing.T) {
		fixture := newWitnessedRegistryFixture(t)
		if _, err := fixture.service.CommitPrepared(context.Background(), witnessedRegistryIssueInput(t, fixture.securityContext, "snapshot-alias")); err != nil {
			t.Fatal(err)
		}
		if err := fixture.service.WithWitnessedSnapshotAuthority(context.Background(), fixture.securityContext, func(
			snapshot registryport.WitnessedSnapshot,
			capability registryport.WitnessedSnapshotCapability,
		) error {
			tampered := snapshot
			tampered.Registry.StateDigest = domainsecurity.SHA256Hex([]byte("tampered-witnessed-registry"))
			if capability.UseExact(tampered, func() error { return nil }) == nil {
				t.Fatal("changed witnessed snapshot retained exact capability authority")
			}
			nestedTampered := snapshot
			nestedTampered.Registry.Entries[0].CanonicalEvidence[0] ^= 1
			called := false
			if capability.UseExact(nestedTampered, func() error { called = true; return nil }) == nil || called {
				t.Fatal("nested alias mutation retained exact witnessed snapshot authority")
			}
			nestedTampered.Registry.Entries[0].CanonicalEvidence[0] ^= 1
			mutatedDuringUse := snapshot
			if capability.UseExact(mutatedDuringUse, func() error {
				mutatedDuringUse.Registry.Entries[0].CanonicalEvidence[0] ^= 1
				return nil
			}) == nil {
				t.Fatal("nested snapshot changed during an authorized mutation")
			}
			mutatedDuringUse.Registry.Entries[0].CanonicalEvidence[0] ^= 1
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("cancellation independently blocks mutation", func(t *testing.T) {
		fixture := newWitnessedRegistryFixture(t)
		if _, err := fixture.service.CommitPrepared(context.Background(), witnessedRegistryIssueInput(t, fixture.securityContext, "snapshot-cancel")); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := fixture.service.WithWitnessedSnapshotAuthority(ctx, fixture.securityContext, func(
			snapshot registryport.WitnessedSnapshot,
			capability registryport.WitnessedSnapshotCapability,
		) error {
			cancel()
			called := false
			if capability.UseExact(snapshot, func() error { called = true; return nil }) == nil || called {
				t.Fatal("cancelled witnessed snapshot capability crossed its mutation boundary")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestFactFinalWitnessReplayRequiresExactAdmissionOnFreshAuthorityChain(t *testing.T) {
	fixture := newWitnessedRegistryFixture(t)
	input := witnessedRegistryIssueInput(t, fixture.securityContext, "fact-final")
	receipt, err := fixture.service.CommitPrepared(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	privateRecord := witnessedFactFinalRecord(t, fixture, input, receipt)
	var leaked registryport.FactFinalWitnessCapability
	if err := fixture.service.WithFactFinalWitnessAuthority(
		context.Background(),
		witnessedFactFinalRequest(t, fixture, privateRecord),
		func(capability registryport.FactFinalWitnessCapability) error {
			issued, err := capability.PrivateFinal()
			if err != nil {
				return err
			}
			privateRecord = issued
			if err := capability.UseExact(privateRecord, func() error { return nil }); err != nil {
				return err
			}
			issued.Envelope.Claims[0].AllowedWording[0] = "tampered"
			if err := capability.UseExact(issued, func() error { return nil }); err == nil {
				t.Fatal("mutated private-final copy retained exact witness authority")
			}
			fresh, err := capability.PrivateFinal()
			if err != nil || capability.UseExact(fresh, func() error { return nil }) != nil {
				t.Fatalf("mutating returned private final changed the capability: record=%#v err=%v", fresh, err)
			}
			privateRecord = fresh
			leaked = capability
			return nil
		},
	); err != nil {
		t.Fatalf("callback-scoped fact witness authority failed: %v", err)
	}
	if leaked == nil || leaked.UseExact(privateRecord, func() error { return nil }) == nil {
		t.Fatal("fact witness capability remained active after its callback")
	}
	if _, err := leaked.PrivateFinal(); err == nil {
		t.Fatal("inactive fact witness capability still exposed a private final")
	}
	if err := fixture.service.VerifyFactFinalWitnessCurrent(context.Background(), privateRecord); err != nil {
		t.Fatalf("fresh witness chain did not admit the exact V5 fact final: %v", err)
	}
	if err := domainevidence.ValidatePrivateAcceptedFinalPublicationAuthorityWithWitnessV1(
		privateRecord,
		func(candidate domainevidence.PrivateAcceptedFinalRecord) error {
			return fixture.service.VerifyFactFinalWitnessCurrent(context.Background(), candidate)
		},
	); err != nil {
		t.Fatalf("domain publication authority did not consume the fresh verifier: %v", err)
	}
	otherContext, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-other-registry", TurnID: "turn-other-registry", WorkspaceRealPath: "/workspace/witnessed-registry",
		CaseID: "case-other-registry", CaseBindingHash: domainsecurity.SHA256Hex([]byte("other-registry-binding")),
		ContextEpoch: 1, IssuedAt: fixture.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.CommitPrepared(
		context.Background(), witnessedRegistryIssueInput(t, otherContext, "other-context-receipt"),
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.VerifyFactFinalWitnessCurrent(context.Background(), privateRecord); err != nil {
		t.Fatalf("an unrelated context append detached the admitted registry selection: %v", err)
	}

	fixture.coordinator.mu.Lock()
	observationDigest := privateRecord.AcceptedFinal.FactFinalWitnessAdmission.WitnessBinding.ObservationDigest
	historical := fixture.coordinator.observations[observationDigest]
	delete(fixture.coordinator.observations, observationDigest)
	fixture.coordinator.mu.Unlock()
	if err := fixture.service.VerifyFactFinalWitnessCurrent(context.Background(), privateRecord); err == nil {
		t.Fatal("local fact record survived deletion of its fresh-chain witness exchange")
	}
	fixture.coordinator.mu.Lock()
	fixture.coordinator.observations[observationDigest] = historical
	fixture.coordinator.mu.Unlock()
	if _, err := fixture.service.CommitPrepared(
		context.Background(), witnessedRegistryIssueInput(t, fixture.securityContext, "later-same-context-receipt"),
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.VerifyFactFinalWitnessCurrent(context.Background(), privateRecord); err == nil {
		t.Fatal("a later same-context registry append retained stale fact-final replay authority")
	}
}

func TestFactFinalWitnessReplayRejectsDatasetAuthorityDrift(t *testing.T) {
	fixture := newWitnessedRegistryFixture(t)
	input := witnessedRegistryIssueInput(t, fixture.securityContext, "fact-final-dataset-drift")
	receipt, err := fixture.service.CommitPrepared(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	base := witnessedFactFinalRecord(t, fixture, input, receipt)
	var issued domainevidence.PrivateAcceptedFinalRecord
	if err := fixture.service.WithFactFinalWitnessAuthority(
		context.Background(),
		witnessedFactFinalRequest(t, fixture, base),
		func(capability registryport.FactFinalWitnessCapability) error {
			var resolveErr error
			issued, resolveErr = capability.PrivateFinal()
			return resolveErr
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.VerifyFactFinalWitnessCurrent(context.Background(), issued); err != nil {
		t.Fatalf("fresh dataset authority rejected the issued final: %v", err)
	}
	fixture.advanceDatasetChildForTest(t)
	if err := fixture.service.VerifyFactFinalWitnessCurrent(context.Background(), issued); err == nil {
		t.Fatal("stale fact final retained replay authority after the witnessed dataset child advanced")
	}
}

func TestFactFinalWitnessCapabilityUseExactCoversMutationAndClose(t *testing.T) {
	fixture := newWitnessedRegistryFixture(t)
	input := witnessedRegistryIssueInput(t, fixture.securityContext, "fact-lease")
	receipt, err := fixture.service.CommitPrepared(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	base := witnessedFactFinalRecord(t, fixture, input, receipt)
	started := make(chan struct{})
	release := make(chan struct{})
	useResult := make(chan error, 1)
	serviceResult := make(chan error, 1)
	var leaked registryport.FactFinalWitnessCapability
	request := witnessedFactFinalRequest(t, fixture, base)
	go func() {
		serviceResult <- fixture.service.WithFactFinalWitnessAuthority(
			context.Background(),
			request,
			func(capability registryport.FactFinalWitnessCapability) error {
				privateFinal, err := capability.PrivateFinal()
				if err != nil {
					return err
				}
				leaked = capability
				go func() {
					useResult <- capability.UseExact(privateFinal, func() error {
						close(started)
						<-release
						return nil
					})
				}()
				<-started
				return nil
			},
		)
	}()
	<-started
	select {
	case err := <-serviceResult:
		t.Fatalf("issuer callback closed while exact mutation lease was active: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	if err := <-useResult; err != nil {
		t.Fatalf("exact mutation failed while lease was active: %v", err)
	}
	if err := <-serviceResult; err != nil {
		t.Fatalf("issuer failed after exact mutation released its lease: %v", err)
	}
	if leaked == nil || leaked.UseExact(base, func() error { return nil }) == nil {
		t.Fatal("fact witness capability remained usable after callback close")
	}
}

func TestFactFinalWitnessCapabilityAuthorizesOnlyExactAcceptedFinalCASMutation(t *testing.T) {
	fixture := newWitnessedRegistryFixture(t)
	input := witnessedRegistryIssueInput(t, fixture.securityContext, "fact-cas")
	receipt, err := fixture.service.CommitPrepared(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	base := witnessedFactFinalRecord(t, fixture, input, receipt)
	var (
		issued domainevidence.PrivateAcceptedFinalRecord
		plan   appturn.AcceptedFinalPublicationPlan
		leaked registryport.FactFinalWitnessCapability
	)
	mutations := 0
	if err := fixture.service.WithFactFinalWitnessAuthority(
		context.Background(),
		witnessedFactFinalRequest(t, fixture, base),
		func(capability registryport.FactFinalWitnessCapability) error {
			var err error
			issued, err = capability.PrivateFinal()
			if err != nil {
				return err
			}
			plan, err = appturn.BuildAcceptedFinalPublicationPlan(
				issued.AcceptedFinal, issued.RenderedText, issued.PublicationIntent,
			)
			if err != nil {
				return err
			}
			if err := appturn.UseAcceptedFinalCASAuthority(
				issued.SecurityContext.ThreadID, issued.SecurityContext.TurnID, issued.PublicationIntent.TerminalStatus,
				plan.TurnItems, plan.TurnFields, issued, capability,
				func() error { mutations++; return nil },
			); err != nil {
				return err
			}
			tamperedFields := make(map[string]any, len(plan.TurnFields)+1)
			for key, value := range plan.TurnFields {
				tamperedFields[key] = value
			}
			tamperedFields["discard"] = true
			if err := appturn.UseAcceptedFinalCASAuthority(
				issued.SecurityContext.ThreadID, issued.SecurityContext.TurnID, issued.PublicationIntent.TerminalStatus,
				plan.TurnItems, tamperedFields, issued, capability,
				func() error { mutations++; return nil },
			); err == nil {
				t.Fatal("tampered accepted-final CAS plan used the live witness capability")
			}
			leaked = capability
			return nil
		},
	); err != nil {
		t.Fatalf("exact accepted-final CAS mutation was rejected: %v", err)
	}
	if mutations != 1 {
		t.Fatalf("unexpected accepted-final CAS mutation count: %d", mutations)
	}
	if err := appturn.UseAcceptedFinalCASAuthority(
		issued.SecurityContext.ThreadID, issued.SecurityContext.TurnID, issued.PublicationIntent.TerminalStatus,
		plan.TurnItems, plan.TurnFields, issued, leaked,
		func() error { mutations++; return nil },
	); err == nil || mutations != 1 {
		t.Fatalf("expired witness capability mutated the accepted-final CAS: mutations=%d err=%v", mutations, err)
	}
}

func TestWitnessedRegistryRejectsBundleWithoutFreshObservation(t *testing.T) {
	fixture := newWitnessedRegistryFixture(t)
	fixture.coordinator.omitWitness = true
	if _, err := fixture.service.CommitPrepared(context.Background(), witnessedRegistryIssueInput(t, fixture.securityContext, "missing-witness")); err == nil {
		t.Fatal("locally signed bundle without a fresh witness observation registered evidence")
	}
	if len(fixture.indexes.records) != 0 || len(fixture.capsules.records) != 0 || fixture.coordinator.advanceCalls != 0 {
		t.Fatalf("invalid fresh head caused registry side effects: indexes=%d capsules=%d advances=%d",
			len(fixture.indexes.records), len(fixture.capsules.records), fixture.coordinator.advanceCalls)
	}
}

func TestUnwitnessedCandidateAndOldLocalCapsuleCannotBecomeMembership(t *testing.T) {
	fixture := newWitnessedRegistryFixture(t)
	registered, err := fixture.service.CommitPrepared(context.Background(), witnessedRegistryIssueInput(t, fixture.securityContext, "committed"))
	if err != nil {
		t.Fatal(err)
	}
	committedHead := fixture.coordinator.bundle
	current, err := fixture.service.Replay(context.Background(), fixture.securityContext)
	if err != nil {
		t.Fatal(err)
	}
	candidateInput := witnessedRegistryIssueInput(t, fixture.securityContext, "orphan")
	orphanRegistry, orphanReceipt, err := domainevidence.RegisterEvidenceReceipt(
		current, candidateInput.Draft, candidateInput.CanonicalEvidence, candidateInput.SettlementProof, candidateInput.RegisteredAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	orphanCapsule, orphanIndex := fixture.candidate(t, committedHead, orphanRegistry, "orphan")
	if err := fixture.capsules.PutIfAbsent(context.Background(), orphanCapsule); err != nil {
		t.Fatal(err)
	}
	if err := fixture.indexes.PutIfAbsent(context.Background(), orphanIndex); err != nil {
		t.Fatal(err)
	}
	if fixture.coordinator.bundle.RecordDigest != committedHead.RecordDigest {
		t.Fatal("test candidate unexpectedly advanced the witness")
	}
	if _, err := fixture.service.Resolve(context.Background(), registryport.MembershipQuery{Context: fixture.securityContext, ReceiptID: orphanReceipt.ReceiptID}); err == nil {
		t.Fatal("locally present unwitnessed candidate became evidence membership")
	}
	if resolved, err := fixture.service.Resolve(context.Background(), registryport.MembershipQuery{Context: fixture.securityContext, ReceiptID: registered.ReceiptID}); err != nil || resolved.Revoked {
		t.Fatalf("unwitnessed candidate disturbed committed membership: resolved=%#v err=%v", resolved, err)
	}
}

func TestWitnessUnavailableNeverFallsBackToLocalRegistryAuthority(t *testing.T) {
	fixture := newWitnessedRegistryFixture(t)
	receipt, err := fixture.service.CommitPrepared(context.Background(), witnessedRegistryIssueInput(t, fixture.securityContext, "available"))
	if err != nil {
		t.Fatal(err)
	}
	fixture.coordinator.observeErr = errors.New("witness unavailable")
	if _, err := fixture.service.Resolve(context.Background(), registryport.MembershipQuery{Context: fixture.securityContext, ReceiptID: receipt.ReceiptID}); !errors.Is(err, fixture.coordinator.observeErr) {
		t.Fatalf("local CAS replaced unavailable witness: %v", err)
	}
	if _, err := fixture.service.Replay(context.Background(), fixture.securityContext); !errors.Is(err, fixture.coordinator.observeErr) {
		t.Fatalf("local replay replaced unavailable witness: %v", err)
	}
}

func TestAuditOnlyV1RegistryAuthorityRejectsBeforeWitnessOrCAS(t *testing.T) {
	fixture := newWitnessedRegistryFixture(t)
	legacy := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID,
		WorkspaceRealPath: fixture.securityContext.WorkspaceRealPath, CaseID: fixture.securityContext.CaseID,
		CaseBindingHash: fixture.securityContext.CaseBindingHash, DatasetSnapshotID: fixture.securityContext.DatasetSnapshotID,
		SourceManifestHash: fixture.securityContext.SourceManifestHash, ContextEpoch: fixture.securityContext.ContextEpoch, IssuedAt: fixture.now,
	})
	input := witnessedRegistryIssueInput(t, fixture.securityContext, "v1-audit")
	input.Context = legacy
	beforeObserve := fixture.coordinator.observeCalls
	if _, err := fixture.service.CommitPrepared(context.Background(), input); err == nil {
		t.Fatal("audit-only V1 context reached witnessed registration")
	}
	if fixture.coordinator.observeCalls != beforeObserve || fixture.coordinator.advanceCalls != 0 || len(fixture.indexes.records) != 0 || len(fixture.capsules.records) != 0 {
		t.Fatalf("V1 rejection crossed an authority boundary: observe=%d advance=%d indexes=%d capsules=%d",
			fixture.coordinator.observeCalls, fixture.coordinator.advanceCalls, len(fixture.indexes.records), len(fixture.capsules.records))
	}
}

type witnessedRegistryFixture struct {
	service            *Service
	coordinator        *witnessedRegistryCoordinator
	indexes            *memoryRegistryIndexStore
	capsules           *memoryRegistryCapsuleStore
	authority          *witnessedRegistryAuthority
	bindingObservation domainsecurity.CaseBindingObservationV1
	datasetSelection   datasetsnapshotport.CurrentSelectionV2
	datasetAuthority   *witnessedRegistryDatasetAuthority
	bindingObserver    *witnessedRegistryBindingObserver
	securityContext    domainsecurity.TurnSecurityContext
	now                time.Time
}

func newWitnessedRegistryFixture(t *testing.T) *witnessedRegistryFixture {
	t.Helper()
	now := time.Date(2026, 7, 13, 5, 0, 0, 0, time.UTC)
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x61}, ed25519.SeedSize))
	authority := &witnessedRegistryAuthority{privateKey: privateKey, publicKey: privateKey.Public().(ed25519.PublicKey)}
	authority.keyID = domainsecurity.SHA256Hex(authority.publicKey)
	witnessPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x71}, ed25519.SeedSize))
	witnessPublic := witnessPrivate.Public().(ed25519.PublicKey)
	installationID := domainsecurity.SHA256Hex([]byte("witnessed-registry-installation"))
	enrollmentID := domainsecurity.SHA256Hex([]byte("witnessed-registry-enrollment"))
	const (
		threadID  = "thread-witnessed-registry"
		turnID    = "turn-witnessed-registry"
		workspace = "/workspace/witnessed-registry"
		caseID    = "case-witnessed-registry"
	)
	caseBindingHash := domainsecurity.SHA256Hex([]byte("witnessed-registry-binding"))
	bindingObservation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: workspace,
		State:             domainsecurity.CaseBindingStateValid,
		CaseID:            caseID,
		BindingSHA256:     domainsecurity.SHA256Hex([]byte("witnessed-registry-binding-file")),
		CaseBindingHash:   caseBindingHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	resolvedSnapshot, err := datasetsnapshotv2fixture.NewResolvedSnapshotV2(
		datasetsnapshotv2fixture.ResolvedInput{
			TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
			Observation: bindingObservation, Material: "witnessed-registry",
			InstallationID: installationID, AcceptedAt: now.Add(-2 * time.Minute),
			AuthorityKeyID: authority.keyID, AuthorityPublicKey: authority.publicKey,
			Sign: func(message []byte) ([]byte, error) {
				return authority.Sign(context.Background(), message)
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	datasetIndex, err := domainsecurity.NewDatasetSnapshotIndexV1(domainsecurity.DatasetSnapshotIndexInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
		PreviousIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
		MutationID:           domainsecurity.SHA256Hex([]byte("witnessed-registry-dataset-index")),
		Binding:              resolvedSnapshot.Record.Binding,
		SnapshotRecordDigest: resolvedSnapshot.Record.RecordDigest,
		AuthorityKeyID:       authority.keyID, AuthorityPublicKey: authority.publicKey,
	}, func(message []byte) ([]byte, error) {
		return authority.Sign(context.Background(), message)
	})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
		MutationID:                  domainsecurity.SHA256Hex([]byte("witnessed-registry-bundle-genesis")),
		DatasetSnapshotIndexDigest:  datasetIndex.IndexDigest,
		DatasetSnapshotCount:        1,
		EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
		AuthorityKeyID:              authority.keyID, AuthorityPublicKey: authority.publicKey,
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	coordinator := &witnessedRegistryCoordinator{bundle: bundle, authority: authority, witnessPrivate: witnessPrivate, witnessPublic: witnessPublic}
	coordinator.bundles = map[string]domainevidence.EvidenceAuthorityBundleV1{bundle.RecordDigest: bundle}
	coordinator.observations = map[string]evidenceauthorityport.ObservationBundle{}
	indexes := &memoryRegistryIndexStore{records: map[string]domainevidence.EvidenceRegistryAuthorityIndexV2{}}
	capsules := &memoryRegistryCapsuleStore{records: map[string]domainevidence.EvidenceRegistryAuthorityCapsule{}}
	riskPolicyDigest := domainsecurity.SHA256Hex([]byte("witnessed-registry-risk-policy"))
	publicationPolicy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: riskPolicyDigest, RiskClass: domainsecurity.RiskClassCase,
		Disposition:              domainsecurity.PublicationDispositionCaseEvidenceGate,
		CaseBindingState:         domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: bindingObservation.ObservationDigest,
		BlockerCode:              domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	riskBinding, err := testsecurity.WitnessedRiskBinding(
		threadID, workspace, domainsecurity.RiskClassCase, riskPolicyDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: caseID, CaseBindingHash: caseBindingHash,
		DatasetSnapshotID:  resolvedSnapshot.Record.DatasetSnapshotID,
		SourceManifestHash: resolvedSnapshot.Record.SourceManifestHash,
		ContextEpoch:       4, IssuedAt: now, PublicationPolicy: publicationPolicy,
		RiskAuthorityBinding: riskBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	datasetSelection := datasetsnapshotport.CurrentSelectionV2{
		Head:             evidenceauthorityport.FreshHead{HasBundle: true, Bundle: bundle},
		DatasetIndexPath: []domainsecurity.DatasetSnapshotIndexV1{datasetIndex},
		SelectedIndex:    datasetIndex,
		Snapshot:         resolvedSnapshot,
	}
	datasetSelection.SelectionDigest, err = datasetsnapshotport.CanonicalCurrentSelectionDigestV2(datasetSelection)
	if err != nil {
		t.Fatal(err)
	}
	datasetAuthority := &witnessedRegistryDatasetAuthority{
		coordinator: coordinator, selection: datasetSelection, bindingObservation: bindingObservation,
	}
	bindingObserver := &witnessedRegistryBindingObserver{
		workspaceRealPath: workspace, observation: bindingObservation,
	}
	service, err := New(Config{
		InstallationID: installationID, EnrollmentID: enrollmentID, Authority: authority, Coordinator: coordinator,
		WitnessChain: coordinator,
		WitnessKeyID: domainsecurity.SHA256Hex(witnessPublic), WitnessKey: witnessPublic,
		DatasetAuthority: datasetAuthority, BindingObserver: bindingObserver,
		AuthorityKnownEmpty: true,
		Indexes:             indexes, Capsules: capsules, Random: bytes.NewReader(bytes.Repeat([]byte{0x37}, 4096)),
		Now: func() time.Time { return now.Add(6 * time.Minute) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return &witnessedRegistryFixture{
		service: service, coordinator: coordinator, indexes: indexes, capsules: capsules, authority: authority,
		bindingObservation: bindingObservation, datasetSelection: datasetSelection,
		datasetAuthority: datasetAuthority, bindingObserver: bindingObserver,
		securityContext: securityContext, now: now,
	}
}

func (fixture *witnessedRegistryFixture) candidate(t *testing.T, head domainevidence.EvidenceAuthorityBundleV1, registry domainevidence.EvidenceReceiptRegistry, label string) (domainevidence.EvidenceRegistryAuthorityCapsule, domainevidence.EvidenceRegistryAuthorityIndexV2) {
	t.Helper()
	capsule, err := domainevidence.NewEvidenceRegistryAuthorityCapsule(
		fixture.securityContext, registry, fixture.authority.keyID, fixture.authority.publicKey,
		func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatal(err)
	}
	index, err := domainevidence.NewEvidenceRegistryAuthorityIndexV2(domainevidence.EvidenceRegistryAuthorityIndexInputV2{
		InstallationID: head.InstallationID, EnrollmentID: head.EnrollmentID, Generation: head.EvidenceRegistryCount + 1,
		PreviousIndexDigest: head.EvidenceRegistryIndexDigest, MutationID: domainsecurity.SHA256Hex([]byte("candidate-" + label)),
	}, capsule, fixture.authority.keyID, fixture.authority.publicKey, func(message []byte) ([]byte, error) {
		return fixture.authority.Sign(context.Background(), message)
	})
	if err != nil {
		t.Fatal(err)
	}
	return capsule, index
}

func witnessedRegistryIssueInput(t *testing.T, securityContext domainsecurity.TurnSecurityContext, label string) registryport.CommitPreparedInput {
	t.Helper()
	material := json.RawMessage(`{"schemaVersion":1,"facts":[]}`)
	canonical, err := domainevidence.CanonicalEvidenceBytes(material)
	if err != nil {
		t.Fatal(err)
	}
	settlementID := domainsecurity.SHA256Hex([]byte("settlement-" + label))
	identity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"analytix_funds", "analytix_funds", "1.0.0", domainsecurity.SHA256Hex([]byte("instance-"+label)), 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := domainevidence.NewEvidenceReceiptDraft(domainevidence.EvidenceReceiptInput{
		ReceiptID: domainevidence.EvidenceSettlementReceiptID(settlementID), Context: securityContext,
		ExecutionGrantID: domainsecurity.SHA256Hex([]byte("grant-" + label)), ToolCallID: witnessedRegistryTestHostToolCallID(label),
		ServerIdentity: identity, ServerVersion: "1.0.0", ConnectionEpoch: 1,
		ToolName: "mcp__analytix_funds__count_case_rows", ArgsHash: domainsecurity.SHA256Hex([]byte("args-" + label)),
		ResultHash: domainsecurity.CanonicalJSONHash(canonical), SourceType: domainevidence.SourceTypeTransactionDatasetInventory,
		DatasetSnapshotID: securityContext.DatasetSnapshotID, QueryHash: domainsecurity.SHA256Hex([]byte("query-" + label)),
		QueryRange: domainevidence.EvidenceQueryRange{
			EntityIDs: []string{"entity-a"}, AccountIDs: []string{}, Directions: []string{}, SourceIDs: []string{"source-a"},
			FiltersHash: domainsecurity.SHA256Hex([]byte("filters-" + label)),
		},
		Granularity: "dataset_table_rows", Timezone: "Asia/Shanghai", PaginationCompleteness: domainevidence.PaginationComplete,
		SourceRecordIDs: []string{}, RawSHA256: domainsecurity.SHA256Hex([]byte("raw-" + label)),
		TransformationLineage: []domainevidence.TransformationLineageStep{}, PIIClassification: domainevidence.PIIMasked,
		IssuedAt: time.Date(2026, 7, 13, 5, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return registryport.CommitPreparedInput{
		Context: securityContext, Draft: draft, CanonicalEvidence: canonical,
		SettlementProof: domainevidence.EvidenceSettlementProof{
			SettlementID: settlementID, PreparedRecordDigest: domainsecurity.SHA256Hex([]byte("prepared-" + label)),
		},
		RegisteredAt: time.Date(2026, 7, 13, 5, 2, 0, 0, time.UTC),
	}
}

type witnessedRegistryAuthority struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	keyID      string
}

func (authority *witnessedRegistryAuthority) KeyID() string { return authority.keyID }
func (authority *witnessedRegistryAuthority) PublicKey() []byte {
	return append([]byte(nil), authority.publicKey...)
}
func (authority *witnessedRegistryAuthority) Sign(_ context.Context, message []byte) ([]byte, error) {
	return ed25519.Sign(authority.privateKey, message), nil
}
func (authority *witnessedRegistryAuthority) VerifyTrusted(_ context.Context, keyID string, publicKey, message, signature []byte) error {
	if keyID != authority.keyID || !bytes.Equal(publicKey, authority.publicKey) || !ed25519.Verify(authority.publicKey, message, signature) {
		return errors.New("test authority mismatch")
	}
	return nil
}

type witnessedRegistryCoordinator struct {
	mu             sync.Mutex
	bundle         domainevidence.EvidenceAuthorityBundleV1
	authority      *witnessedRegistryAuthority
	witnessPrivate ed25519.PrivateKey
	witnessPublic  ed25519.PublicKey
	observeCalls   int
	advanceCalls   int
	observeErr     error
	omitWitness    bool
	bundles        map[string]domainevidence.EvidenceAuthorityBundleV1
	observations   map[string]evidenceauthorityport.ObservationBundle
}

func (coordinator *witnessedRegistryCoordinator) ObserveFresh(context.Context) (evidenceauthorityport.FreshHead, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if coordinator.observeErr != nil {
		return evidenceauthorityport.FreshHead{}, coordinator.observeErr
	}
	if coordinator.omitWitness {
		return evidenceauthorityport.FreshHead{HasBundle: true, Bundle: coordinator.bundle}, nil
	}
	return coordinator.freshHeadLocked("observe")
}

func (coordinator *witnessedRegistryCoordinator) AdvanceEvidenceRegistry(_ context.Context, input evidenceauthorityport.RegistryAdvanceInput) (evidenceauthorityport.FreshHead, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.advanceCalls++
	previous := coordinator.bundle
	if input.ExpectedBundleDigest != previous.RecordDigest {
		return evidenceauthorityport.FreshHead{}, errors.New("test witness CAS conflict")
	}
	next, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: previous.InstallationID, EnrollmentID: previous.EnrollmentID, Generation: previous.Generation + 1,
		PreviousBundleDigest: previous.RecordDigest, MutationID: domainsecurity.SHA256Hex([]byte("bundle-mutation-" + input.NextIndexDigest)),
		DatasetSnapshotIndexDigest: previous.DatasetSnapshotIndexDigest, DatasetSnapshotCount: previous.DatasetSnapshotCount,
		EvidenceRegistryIndexDigest: input.NextIndexDigest, EvidenceRegistryCount: previous.EvidenceRegistryCount + 1,
		PublicationIndexDigest: previous.PublicationIndexDigest, PublicationCount: previous.PublicationCount,
		AuthorityKeyID: coordinator.authority.keyID, AuthorityPublicKey: coordinator.authority.publicKey,
	}, func(message []byte) ([]byte, error) { return coordinator.authority.Sign(context.Background(), message) })
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	coordinator.bundle = next
	coordinator.bundles[next.RecordDigest] = next
	return coordinator.freshHeadLocked("advance")
}

func (coordinator *witnessedRegistryCoordinator) ResolveWitnessBindingOnFreshChain(
	_ context.Context,
	binding domainevidence.EvidenceAuthorityWitnessBindingV1,
) (evidenceauthorityport.WitnessBindingChainResolution, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	current, err := coordinator.freshHeadLocked("resolve-binding")
	if err != nil {
		return evidenceauthorityport.WitnessBindingChainResolution{}, err
	}
	historical, ok := coordinator.observations[binding.ObservationDigest]
	if !ok || domainevidence.ValidateEvidenceAuthorityWitnessBindingExactV1(
		binding, historical.Bundle, historical.Request, historical.Observation,
		historical.Bundle.InstallationID, historical.Bundle.EnrollmentID,
		coordinator.authority.keyID, coordinator.authority.publicKey,
		domainsecurity.SHA256Hex(coordinator.witnessPublic), coordinator.witnessPublic,
	) != nil || !coordinator.bundleAncestorLocked(current.Bundle, historical.Bundle) {
		return evidenceauthorityport.WitnessBindingChainResolution{}, errors.New("test witness binding is outside the fresh chain")
	}
	return evidenceauthorityport.WitnessBindingChainResolution{Current: current, Historical: historical}, nil
}

func (coordinator *witnessedRegistryCoordinator) freshHeadLocked(label string) (evidenceauthorityport.FreshHead, error) {
	coordinator.observeCalls++
	bundle := coordinator.bundle
	witnessKeyID := domainsecurity.SHA256Hex(coordinator.witnessPublic)
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: bundle.InstallationID, EnrollmentID: bundle.EnrollmentID, Namespace: bundle.Namespace,
		Generation: bundle.Generation, CurrentStateDigest: bundle.RecordDigest,
		PreviousStateDigest:      domainsecurity.SHA256Hex([]byte("registry-previous-state")),
		PreviousCheckpointDigest: domainsecurity.SHA256Hex([]byte("registry-previous-checkpoint")),
		FenceNonce:               domainsecurity.SHA256Hex([]byte("registry-fence")), MutationID: bundle.MutationID,
		WitnessKeyID: witnessKeyID, WitnessPublicKey: coordinator.witnessPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(coordinator.witnessPrivate, message), nil })
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID: bundle.InstallationID, EnrollmentID: bundle.EnrollmentID, Namespace: bundle.Namespace,
		ChallengeNonce: domainsecurity.SHA256Hex([]byte(label + "-challenge-" + strconv.Itoa(coordinator.observeCalls) + "-" + bundle.RecordDigest)),
		AuthorityKeyID: coordinator.authority.keyID, AuthorityPublicKey: coordinator.authority.publicKey,
	}, func(message []byte) ([]byte, error) { return coordinator.authority.Sign(context.Background(), message) })
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(request, checkpoint, func(message []byte) ([]byte, error) {
		return ed25519.Sign(coordinator.witnessPrivate, message), nil
	})
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	coordinator.observations[observation.ObservationDigest] = evidenceauthorityport.ObservationBundle{
		Bundle: bundle, Request: request, Observation: observation,
	}
	return evidenceauthorityport.FreshHead{HasBundle: true, Bundle: bundle, Request: request, Observation: observation}, nil
}

func (coordinator *witnessedRegistryCoordinator) bundleAncestorLocked(
	current, ancestor domainevidence.EvidenceAuthorityBundleV1,
) bool {
	if current.Generation < ancestor.Generation {
		return false
	}
	cursor := current
	for cursor.Generation > ancestor.Generation {
		previous, ok := coordinator.bundles[cursor.PreviousBundleDigest]
		if !ok || domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(previous, cursor) != nil {
			return false
		}
		cursor = previous
	}
	return reflect.DeepEqual(cursor, ancestor)
}

func witnessedFactFinalRecord(
	t *testing.T,
	fixture *witnessedRegistryFixture,
	input registryport.CommitPreparedInput,
	receipt domainevidence.EvidenceReceipt,
) domainevidence.PrivateAcceptedFinalRecord {
	t.Helper()
	var snapshot registryport.WitnessedSnapshot
	if err := fixture.service.WithWitnessedSnapshot(context.Background(), fixture.securityContext, func(current registryport.WitnessedSnapshot) error {
		snapshot = current
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	payload := domainevidence.NormalizedClaimPayload{
		SubjectID: "entity-a", EntityID: "entity-a", AttributeName: "document_author",
		AttributeValue: "controlled-evidence-only", Granularity: "record",
	}
	proposal := domainevidence.ClaimProposal{
		SchemaVersion: domainevidence.ClaimProposalVersion, ProposalID: "proposal-witnessed-fact",
		ClaimType: domainevidence.ClaimBidEditMetadata, NormalizedPayload: payload,
		EvidenceIDs: []string{}, CounterEvidenceIDs: []string{},
	}
	scope := input.Draft.QueryRange
	verifiedAt := fixture.now.Add(2 * time.Minute)
	claim, err := domainevidence.NewClaimRecord(domainevidence.ClaimRecordInput{
		ClaimID: "claim-witnessed-fact", Proposal: proposal, SupportState: domainevidence.ClaimVerified,
		EvidenceIDs: []string{receipt.ReceiptID}, CounterEvidenceIDs: []string{}, SupportedScope: &scope,
		AllowedWording: []string{"exact_verified_fact"}, ProhibitedUpgrades: []string{"legal_characterization_without_review"},
		VerifierReceiptID: domainevidence.VerifierReceiptDigest(
			"claim-witnessed-fact", proposal.ClaimType, payload, []string{receipt.ReceiptID}, nil, domainevidence.ClaimVerified,
		),
		VerificationReason: "test", VerifiedAt: verifiedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.EvidenceBackedAnswer, Context: fixture.securityContext, TerminalReason: "success",
		Claims: []domainevidence.ClaimRecord{claim}, EvidenceReceiptIDs: []string{receipt.ReceiptID},
		CheckedScope: &scope, IssuedAt: verifiedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	head, err := domainevidence.NewEvidenceRegistryHead(snapshot.Registry)
	if err != nil {
		t.Fatal(err)
	}
	probeSource := domainevidence.PublicationSourceSnapshot{
		ReceiptID: receipt.ReceiptID, ServerID: "analytix_funds", ServerIdentity: receipt.ServerIdentity,
		ServerVersion: receipt.ServerVersion, ConnectionEpoch: receipt.ConnectionEpoch, ToolName: receipt.ToolName,
		DatasetSnapshotID:  receipt.DatasetSnapshotID,
		CatalogFingerprint: domainsecurity.SHA256Hex([]byte("fact-catalog")),
		SpecFingerprint:    domainsecurity.SHA256Hex([]byte("fact-spec")),
		CheckedAt:          verifiedAt.Format(time.RFC3339Nano),
	}
	probe := witnessedRegistryFactProbe(t, fixture.securityContext, probeSource)
	probeSource.ProbeDigest = probe.ProbeDigest
	proof, err := domainevidence.NewPublicationSnapshotProof(domainevidence.PublicationSnapshotProofInput{
		Context: fixture.securityContext, RegistryHead: head, EvidenceReceiptIDs: envelope.EvidenceReceiptIDs,
		Sources:   []domainevidence.PublicationSourceSnapshot{probeSource},
		CheckedAt: verifiedAt.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := domainevidence.RenderFinalAnswer(envelope)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domainevidence.NewTerminalPublicationIntent(domainevidence.TerminalPublicationIntentInput{
		CreatedAt: verifiedAt.Format(time.RFC3339Nano), TerminalStatus: "completed",
	}, envelope.TerminalReason)
	if err != nil {
		t.Fatal(err)
	}
	admittedAt := verifiedAt.Add(2 * time.Second)
	datasetSelection := fixture.datasetSelectionForHead(t, snapshot.Head)
	admissionInput := domainevidence.FactFinalWitnessAdmissionInputV1{
		Context: fixture.securityContext, Envelope: envelope, RenderedText: rendered, PublicationProof: &proof,
		Registry: snapshot.Registry, Bundle: snapshot.Head.Bundle, ObserveRequest: snapshot.Head.Request,
		Observation: snapshot.Head.Observation, RootIndex: snapshot.RootIndex, SelectedIndex: snapshot.SelectedIndex,
		SelectedCapsule: snapshot.SelectedCapsule, RegistryIndexPath: snapshot.RegistryIndexPath,
		DatasetRootIndex: datasetSelection.DatasetIndexPath[0], SelectedDatasetIndex: datasetSelection.SelectedIndex,
		DatasetIndexPath: append([]domainsecurity.DatasetSnapshotIndexV1(nil), datasetSelection.DatasetIndexPath...),
		DatasetRecord:    datasetSelection.Snapshot.Record, DatasetManifest: datasetSelection.Snapshot.Manifest,
		FundsProducerContent: datasetSelection.Snapshot.FundsProducerContent,
		BindingObservation:   fixture.bindingObservation,
		InstallationID:       snapshot.Head.Bundle.InstallationID, EnrollmentID: snapshot.Head.Bundle.EnrollmentID,
		AuthorityKeyID: fixture.authority.keyID, AuthorityPublicKey: fixture.authority.publicKey,
		WitnessKeyID:     domainsecurity.SHA256Hex(fixture.coordinator.witnessPublic),
		WitnessPublicKey: fixture.coordinator.witnessPublic, AdmittedAt: admittedAt,
	}
	admission, err := domainevidence.NewFactFinalWitnessAdmissionV1(admissionInput)
	if err != nil {
		t.Fatal(err)
	}
	privateDigest, err := domainevidence.PrivateAcceptedFinalDigestWithFactWitnessV1(
		fixture.securityContext, envelope, rendered, intent, &proof, &admission,
	)
	if err != nil {
		t.Fatal(err)
	}
	acceptedFinal, err := domainevidence.NewAcceptedFinalRecord(domainevidence.AcceptedFinalRecordInput{
		Context: fixture.securityContext, Envelope: envelope, RenderedText: rendered, RegistryHead: head,
		PublicationSnapshotProof: &proof, FactFinalWitnessAdmission: &admission, FactFinalWitnessAuthority: &admissionInput,
		PrivateRecordDigest: privateDigest, AcceptedAt: admittedAt,
		AuthorityKeyID: fixture.authority.keyID, AuthorityPublicKey: fixture.authority.publicKey,
	}, func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	privateRecord, err := domainevidence.NewPrivateAcceptedFinalRecord(
		fixture.securityContext, envelope, rendered, head, intent, acceptedFinal, &proof,
	)
	if err != nil {
		t.Fatal(err)
	}
	return privateRecord
}

type memoryRegistryIndexStore struct {
	mu      sync.Mutex
	records map[string]domainevidence.EvidenceRegistryAuthorityIndexV2
}

func (store *memoryRegistryIndexStore) PutIfAbsent(_ context.Context, index domainevidence.EvidenceRegistryAuthorityIndexV2) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if current, ok := store.records[index.IndexDigest]; ok && !reflect.DeepEqual(current, index) {
		return errors.New("test index conflict")
	}
	store.records[index.IndexDigest] = index
	return nil
}
func (store *memoryRegistryIndexStore) Resolve(_ context.Context, digest string) (domainevidence.EvidenceRegistryAuthorityIndexV2, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	record, ok := store.records[digest]
	if !ok {
		return domainevidence.EvidenceRegistryAuthorityIndexV2{}, errors.New("test index missing")
	}
	return record, nil
}

type memoryRegistryCapsuleStore struct {
	mu      sync.Mutex
	records map[string]domainevidence.EvidenceRegistryAuthorityCapsule
}

func (store *memoryRegistryCapsuleStore) PutIfAbsent(_ context.Context, capsule domainevidence.EvidenceRegistryAuthorityCapsule) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if current, ok := store.records[capsule.RecordDigest]; ok && !reflect.DeepEqual(current, capsule) {
		return errors.New("test capsule conflict")
	}
	store.records[capsule.RecordDigest] = capsule
	return nil
}
func (store *memoryRegistryCapsuleStore) Resolve(_ context.Context, digest string) (domainevidence.EvidenceRegistryAuthorityCapsule, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	record, ok := store.records[digest]
	if !ok {
		return domainevidence.EvidenceRegistryAuthorityCapsule{}, errors.New("test capsule missing")
	}
	return record, nil
}
