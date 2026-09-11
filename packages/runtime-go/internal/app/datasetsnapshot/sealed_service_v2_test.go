package datasetsnapshot

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	evidenceauthorityapp "analytix.local/runtime-go/internal/app/evidenceauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
	fixturev2 "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
	securitycontextfixture "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestSealedDatasetSnapshotV2AdmitResolveAndExpectedCurrent(t *testing.T) {
	fixture := newSealedServiceFixtureV2(t)
	input := fixture.admitInput()
	resolved, err := fixture.service.AdmitExactV2(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Record.DatasetSnapshotID != domainsecurity.DeriveDatasetSnapshotIDV2(fixture.manifest) ||
		resolved.Manifest != fixture.manifest || resolved.FundsProducerContent != fixture.producer ||
		fixture.coordinator.successfulAdvances != 1 {
		t.Fatalf("sealed DSV2 admission returned a mismatched result: %#v", resolved)
	}
	selected, err := fixture.service.ResolveWitnessedV2(context.Background(), datasetsnapshotport.ResolveInputV2{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: fixture.material.Observation, ExpectedDatasetSnapshotID: resolved.Record.DatasetSnapshotID,
	})
	if err != nil || selected.Record.RecordDigest != resolved.Record.RecordDigest {
		t.Fatalf("witnessed DSV2 resolution failed: selected=%#v err=%v", selected, err)
	}
	wrongExpected := domainsecurity.DatasetSnapshotIDPrefixV2 + domainsecurity.SHA256Hex([]byte("old-snapshot"))
	if _, err := fixture.service.ResolveWitnessedV2(context.Background(), datasetsnapshotport.ResolveInputV2{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: fixture.material.Observation, ExpectedDatasetSnapshotID: wrongExpected,
	}); !errors.Is(err, datasetsnapshotport.ErrStale) {
		t.Fatalf("old expected DSV2 id was not rejected: %v", err)
	}
	if _, err := fixture.service.AdmitAfterExactV2(context.Background(), AdmitAfterInputV2{
		AdmitInputV2: fixture.admitInput(), ExpectedCurrentDatasetSnapshotID: wrongExpected,
	}); !errors.Is(err, datasetsnapshotport.ErrStale) {
		t.Fatalf("admission based on a non-current DSV2 predecessor was not rejected: %v", err)
	}
	before := fixture.coordinator.successfulAdvances
	idempotent, err := fixture.service.AdmitExactV2(context.Background(), input)
	if err != nil || idempotent.Record.RecordDigest != resolved.Record.RecordDigest ||
		fixture.coordinator.successfulAdvances != before {
		t.Fatalf("identical DSV2 content was reminted: result=%#v err=%v", idempotent, err)
	}
}

func TestSealedDatasetSnapshotV2FailsClosedOnMissingDifferentAndChangingMaterial(t *testing.T) {
	t.Run("missing raw chunk", func(t *testing.T) {
		fixture := newSealedServiceFixtureV2(t)
		delete(fixture.materials.values, datasetsnapshotport.MaterialRawContentChunkV1)
		if _, err := fixture.service.AdmitExactV2(context.Background(), fixture.admitInput()); !errors.Is(err, datasetsnapshotport.ErrNotFound) {
			t.Fatalf("missing raw chunk was not rejected: %v", err)
		}
	})

	t.Run("syntactically valid different FPC", func(t *testing.T) {
		fixture := newSealedServiceFixtureV2(t)
		other := fixture.producer
		other.AccountContentSHA256 = domainsecurity.SHA256Hex([]byte("different-account-content"))
		body, err := domainsecurity.FundsProducerContentManifestV1Bytes(other)
		if err != nil {
			t.Fatal(err)
		}
		digest := domainsecurity.SHA256Hex(body)
		fixture.materials.values[datasetsnapshotport.MaterialFundsProducerContentV1][digest] = body
		input := fixture.admitInput()
		input.FundsProducerReference = datasetsnapshotport.ExactMaterialReferenceV2{
			Address: digest, SHA256: digest, ByteLength: uint64(len(body)),
		}
		if _, err := fixture.service.AdmitExactV2(context.Background(), input); !errors.Is(err, datasetsnapshotport.ErrMismatch) {
			t.Fatalf("valid but different FPC entered DSV2 authority: %v", err)
		}
	})

	t.Run("bytes change between the two exact reads", func(t *testing.T) {
		fixture := newSealedServiceFixtureV2(t)
		fixture.materials.changeOnSecond = datasetsnapshotport.MaterialRawManifestV1
		if _, err := fixture.service.AdmitExactV2(context.Background(), fixture.admitInput()); !errors.Is(err, datasetsnapshotport.ErrCorrupt) {
			t.Fatalf("changing private CAS read was not rejected: %v", err)
		}
	})
}

func TestSealedDatasetSnapshotV2CrashRollbackAndConcurrentCAS(t *testing.T) {
	t.Run("crash after atomic bundle before index remains inert and retries", func(t *testing.T) {
		fixture := newSealedServiceFixtureV2(t)
		fixture.indexes.failPut = true
		input := fixture.admitInput()
		if _, err := fixture.service.AdmitExactV2(context.Background(), input); err == nil {
			t.Fatal("simulated index crash unexpectedly admitted DSV2")
		}
		if fixture.coordinator.currentBundle().DatasetSnapshotCount != 0 || fixture.bundles.count() != 1 {
			t.Fatal("crash candidate was either witnessed or failed to retain one atomic inert bundle")
		}
		if _, err := fixture.service.ResolveWitnessedV2(context.Background(), datasetsnapshotport.ResolveInputV2{
			TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
			Observation: fixture.material.Observation,
		}); !errors.Is(err, datasetsnapshotport.ErrUnavailable) {
			t.Fatalf("unindexed crash residue became current: %v", err)
		}
		fixture.indexes.failPut = false
		if _, err := fixture.service.AdmitExactV2(context.Background(), input); err != nil {
			t.Fatalf("exact retry after inert crash residue failed: %v", err)
		}
	})

	t.Run("witness rollback or stale CAS never selects candidate", func(t *testing.T) {
		fixture := newSealedServiceFixtureV2(t)
		fixture.coordinator.advanceErr = errors.New("witness rollback rejected")
		if _, err := fixture.service.AdmitExactV2(context.Background(), fixture.admitInput()); !errors.Is(err, datasetsnapshotport.ErrUnavailable) {
			t.Fatalf("witness rollback failure was not fail-closed: %v", err)
		}
		if fixture.coordinator.currentBundle().DatasetSnapshotCount != 0 {
			t.Fatal("failed witness advance selected an inert DSV2 candidate")
		}
	})

	t.Run("head change after material readback blocks return", func(t *testing.T) {
		fixture := newSealedServiceFixtureV2(t)
		racing := &sealedHeadRacingCoordinatorV2{delegate: fixture.coordinator, raceOnObserve: 2}
		service, err := NewSealedV2(SealedConfigV2{
			InstallationID: fixture.coordinator.installationID, EnrollmentID: fixture.coordinator.enrollmentID,
			Authority: fixture.authority, Coordinator: racing, LegacyRecords: sealedAbsentLegacyStoreV2{},
			Bundles: fixture.bundles, Indexes: fixture.indexes, Materials: fixture.materials,
			Random: &datasetSnapshotCounterReader{},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.AdmitExactV2(context.Background(), fixture.admitInput()); !errors.Is(err, datasetsnapshotport.ErrStale) {
			t.Fatalf("head changed after exact readback but admission returned: %v", err)
		}
	})

	t.Run("concurrent exact admissions have one witnessed transition", func(t *testing.T) {
		fixture := newSealedServiceFixtureV2(t)
		input := fixture.admitInput()
		var wait sync.WaitGroup
		errorsOut := make(chan error, 2)
		for range 2 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				_, err := fixture.service.AdmitExactV2(context.Background(), input)
				errorsOut <- err
			}()
		}
		wait.Wait()
		close(errorsOut)
		for err := range errorsOut {
			if err != nil && !errors.Is(err, datasetsnapshotport.ErrStale) {
				t.Fatalf("concurrent admission failed with unexpected error: %v", err)
			}
		}
		if fixture.coordinator.successfulAdvances != 1 || fixture.coordinator.currentBundle().DatasetSnapshotCount != 1 {
			t.Fatalf("concurrent admission advanced %d times", fixture.coordinator.successfulAdvances)
		}
	})
}

func TestSealedDatasetSnapshotV2RejectsForgedBundleAfterWitnessing(t *testing.T) {
	fixture := newSealedServiceFixtureV2(t)
	resolved, err := fixture.service.AdmitExactV2(context.Background(), fixture.admitInput())
	if err != nil {
		t.Fatal(err)
	}
	fixture.bundles.mu.Lock()
	forged := fixture.bundles.records[resolved.Record.RecordDigest]
	forged.Record.AuthoritySignature = base64.RawURLEncoding.EncodeToString(make([]byte, 64))
	fixture.bundles.records[resolved.Record.RecordDigest] = forged
	fixture.bundles.mu.Unlock()
	if _, err := fixture.service.ResolveWitnessedV2(context.Background(), datasetsnapshotport.ResolveInputV2{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: fixture.material.Observation,
	}); !errors.Is(err, datasetsnapshotport.ErrCorrupt) {
		t.Fatalf("forged authority bundle was accepted: %v", err)
	}
}

func TestCurrentDatasetSnapshotSelectionCapabilityIsExactAndCallbackScoped(t *testing.T) {
	fixture := newSealedServiceFixtureV2(t)
	resolved, err := fixture.service.AdmitExactV2(context.Background(), fixture.admitInput())
	if err != nil {
		t.Fatal(err)
	}
	securityContext := fixture.securityContext(t, resolved, "turn-current-selection")
	resolveInput := fixture.resolveInput(resolved)
	var (
		leaked          datasetsnapshotport.CurrentSelectionCapabilityV2
		leakedSelection datasetsnapshotport.CurrentSelectionV2
	)
	err = fixture.service.WithCurrentSelectionV2(
		context.Background(), resolveInput, securityContext,
		func(
			selection datasetsnapshotport.CurrentSelectionV2,
			capability datasetsnapshotport.CurrentSelectionCapabilityV2,
		) error {
			leaked, leakedSelection = capability, cloneCurrentSelectionV2(selection)
			if _, err := json.Marshal(capability); err == nil {
				t.Fatal("current DSV2 capability serialized into a portable value")
			}
			if selection.Head.Bundle.DatasetSnapshotIndexDigest != selection.DatasetIndexPath[0].IndexDigest ||
				selection.Head.Bundle.DatasetSnapshotCount != uint64(len(selection.DatasetIndexPath)) ||
				selection.SelectedIndex.SnapshotRecordDigest != selection.Snapshot.Record.RecordDigest ||
				selection.Snapshot != resolved || selection.SelectionDigest == "" {
				t.Fatalf("current DSV2 selection is incomplete: %#v", selection)
			}
			called := false
			if err := capability.UseExact(cloneCurrentSelectionV2(selection), securityContext, func(lease context.Context) error {
				if lease == nil || lease.Err() != nil {
					t.Fatal("current DSV2 use did not receive a live lease context")
				}
				nestedCalled := false
				if nestedErr := capability.UseExact(
					cloneCurrentSelectionV2(selection),
					securityContext,
					func(nestedLease context.Context) error {
						if nestedLease == nil || nestedLease.Err() != nil {
							t.Fatal("nested current DSV2 use did not receive a live lease context")
						}
						nestedCalled = true
						return nil
					},
				); nestedErr != nil || !nestedCalled {
					t.Fatalf("nested exact current DSV2 use failed: called=%t err=%v", nestedCalled, nestedErr)
				}
				called = true
				return nil
			}); err != nil || !called {
				t.Fatalf("exact current DSV2 use failed: called=%t err=%v", called, err)
			}

			tampered := cloneCurrentSelectionV2(selection)
			tampered.DatasetIndexPath[0].MutationID = domainsecurity.SHA256Hex([]byte("tampered-index-mutation"))
			called = false
			if err := capability.UseExact(tampered, securityContext, func(context.Context) error {
				called = true
				return nil
			}); err == nil || called {
				t.Fatal("copied and tampered DSV2 selection retained capability authority")
			}

			otherContext := fixture.securityContext(t, resolved, "turn-other-valid-context")
			called = false
			if err := capability.UseExact(selection, otherContext, func(context.Context) error {
				called = true
				return nil
			}); err == nil || called {
				t.Fatal("another valid TurnSecurityContext V2 retained exact DSV2 capability authority")
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	if leaked == nil || leaked.UseExact(leakedSelection, securityContext, func(context.Context) error {
		called = true
		return nil
	}) == nil || called {
		t.Fatal("current DSV2 capability remained usable after the issuing callback")
	}
}

func TestCurrentDatasetSnapshotPostNativeChallengeSkipsInnerFullObservation(t *testing.T) {
	fixture := newSealedServiceFixtureV2(t)
	resolved, err := fixture.service.AdmitExactV2(context.Background(), fixture.admitInput())
	if err != nil {
		t.Fatal(err)
	}
	coordinator := &sealedCountingFreshHeadCoordinatorV2{delegate: fixture.coordinator}
	service := fixture.newService(t, coordinator)
	securityContext := fixture.securityContext(t, resolved, "turn-post-native-fresh-head")
	continuations := 0
	err = service.WithCurrentSelectionV2(
		context.Background(),
		fixture.resolveInput(resolved),
		securityContext,
		func(
			selection datasetsnapshotport.CurrentSelectionV2,
			capability datasetsnapshotport.CurrentSelectionCapabilityV2,
		) error {
			return capability.UseExact(selection, securityContext, func(context.Context) error {
				type postNativeExactCapabilityV2 interface {
					UsePostNativeExact(
						datasetsnapshotport.CurrentSelectionV2,
						domainsecurity.TurnSecurityContext,
						func(context.Context) error,
					) error
				}
				usePostNative := capability.UseExact
				if exact, ok := capability.(postNativeExactCapabilityV2); ok {
					usePostNative = exact.UsePostNativeExact
				}
				return usePostNative(selection, securityContext, func(context.Context) error {
					continuations++
					return nil
				})
			})
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if continuations != 1 || coordinator.fullObservationCount() != 4 || coordinator.challengeCount() != 2 {
		t.Fatalf(
			"post-native currentness repeated full inventory: continuations=%d full_observations=%d challenges=%d",
			continuations,
			coordinator.fullObservationCount(),
			coordinator.challengeCount(),
		)
	}
}

func TestCurrentDatasetSnapshotPostNativeChallengeSkipsRealAuthorityInventory(t *testing.T) {
	fixture := newRealAuthoritySealedServiceFixtureV2(t)
	resolved, err := fixture.service.AdmitExactV2(context.Background(), fixture.base.admitInput())
	if err != nil {
		t.Fatal(err)
	}
	securityContext := fixture.base.securityContext(t, resolved, "turn-real-authority-post-native")
	continuations := 0
	var innerEnd struct {
		bundleResolve      int32
		observationPut     int32
		observationResolve int32
		projection         int32
	}
	err = fixture.service.WithCurrentSelectionV2(
		context.Background(),
		fixture.base.resolveInput(resolved),
		securityContext,
		func(
			selection datasetsnapshotport.CurrentSelectionV2,
			capability datasetsnapshotport.CurrentSelectionCapabilityV2,
		) error {
			return capability.UseExact(selection, securityContext, func(context.Context) error {
				postNative, ok := capability.(interface {
					UsePostNativeExact(
						datasetsnapshotport.CurrentSelectionV2,
						domainsecurity.TurnSecurityContext,
						func(context.Context) error,
					) error
				})
				if !ok {
					return errors.New("real current selection capability has no post-native challenge")
				}
				beforeBundle := fixture.authorityBundles.resolveCalls.Load()
				beforeObservationPut := fixture.authorityObservations.putCalls.Load()
				beforeObservationResolve := fixture.authorityObservations.resolveCalls.Load()
				beforeProjection := fixture.authorityProjection.projectCalls.Load()
				beforeWitness := fixture.authorityWitness.observeCalls.Load()
				if err := postNative.UsePostNativeExact(
					selection,
					securityContext,
					func(context.Context) error {
						continuations++
						return nil
					},
				); err != nil {
					return err
				}
				innerEnd.bundleResolve = fixture.authorityBundles.resolveCalls.Load()
				innerEnd.observationPut = fixture.authorityObservations.putCalls.Load()
				innerEnd.observationResolve = fixture.authorityObservations.resolveCalls.Load()
				innerEnd.projection = fixture.authorityProjection.projectCalls.Load()
				if innerEnd.bundleResolve != beforeBundle || innerEnd.observationPut != beforeObservationPut ||
					innerEnd.observationResolve != beforeObservationResolve || innerEnd.projection != beforeProjection {
					return errors.New("post-native challenge entered real authority inventory")
				}
				if fixture.authorityWitness.observeCalls.Load()-beforeWitness != 2 {
					return errors.New("post-native challenge did not issue two fresh witness observations")
				}
				return nil
			})
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if continuations != 1 {
		t.Fatalf("real post-native continuation count=%d", continuations)
	}
	if fixture.authorityBundles.resolveCalls.Load() <= innerEnd.bundleResolve ||
		fixture.authorityObservations.putCalls.Load() <= innerEnd.observationPut ||
		fixture.authorityObservations.resolveCalls.Load() <= innerEnd.observationResolve ||
		fixture.authorityProjection.projectCalls.Load() <= innerEnd.projection {
		t.Fatal("outer UseExact postcheck no longer performs the complete authority observation")
	}
}

func TestCurrentDatasetSnapshotSelectionRejectsContextBindingAndCancellation(t *testing.T) {
	fixture := newSealedServiceFixtureV2(t)
	resolved, err := fixture.service.AdmitExactV2(context.Background(), fixture.admitInput())
	if err != nil {
		t.Fatal(err)
	}
	securityContext := fixture.securityContext(t, resolved, "turn-binding")
	resolveInput := fixture.resolveInput(resolved)

	otherObservation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: fixture.material.Observation.WorkspaceRealPath,
		State:             domainsecurity.CaseBindingStateValid,
		CaseID:            "case-other",
		BindingSHA256:     domainsecurity.SHA256Hex([]byte("case-other-binding-file")),
		CaseBindingHash:   domainsecurity.SHA256Hex([]byte("case-other-binding")),
	})
	if err != nil {
		t.Fatal(err)
	}
	mismatched := resolveInput
	mismatched.Observation = otherObservation
	called := false
	if err := fixture.service.WithCurrentSelectionV2(
		context.Background(), mismatched, securityContext,
		func(datasetsnapshotport.CurrentSelectionV2, datasetsnapshotport.CurrentSelectionCapabilityV2) error {
			called = true
			return nil
		},
	); !errors.Is(err, datasetsnapshotport.ErrMismatch) || called {
		t.Fatalf("mismatched case binding entered current DSV2 callback: called=%t err=%v", called, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	err = fixture.service.WithCurrentSelectionV2(
		ctx, resolveInput, securityContext,
		func(selection datasetsnapshotport.CurrentSelectionV2, capability datasetsnapshotport.CurrentSelectionCapabilityV2) error {
			cancel()
			called = false
			if err := capability.UseExact(selection, securityContext, func(context.Context) error {
				called = true
				return nil
			}); err == nil || called {
				t.Fatal("cancelled DSV2 capability crossed its exact use boundary")
			}
			return nil
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled current DSV2 callback did not fail closed: %v", err)
	}
}

func TestCurrentDatasetSnapshotSelectionRejectsWitnessAndRootDrift(t *testing.T) {
	tests := []struct {
		name          string
		raceOnObserve int
		otherChild    bool
		wantUse       bool
	}{
		{name: "dataset root changes before use", raceOnObserve: 3, wantUse: false},
		{name: "dataset root changes during use", raceOnObserve: 4, wantUse: true},
		{name: "shared registry child changes during use", raceOnObserve: 4, otherChild: true, wantUse: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newSealedServiceFixtureV2(t)
			resolved, err := fixture.service.AdmitExactV2(context.Background(), fixture.admitInput())
			if err != nil {
				t.Fatal(err)
			}
			racing := &sealedHeadRacingCoordinatorV2{
				delegate: fixture.coordinator, raceOnObserve: test.raceOnObserve, otherChild: test.otherChild,
			}
			service := fixture.newService(t, racing)
			securityContext := fixture.securityContext(t, resolved, "turn-"+test.name)
			useCalled := false
			err = service.WithCurrentSelectionV2(
				context.Background(), fixture.resolveInput(resolved), securityContext,
				func(selection datasetsnapshotport.CurrentSelectionV2, capability datasetsnapshotport.CurrentSelectionCapabilityV2) error {
					return capability.UseExact(selection, securityContext, func(context.Context) error {
						useCalled = true
						return nil
					})
				},
			)
			if !errors.Is(err, datasetsnapshotport.ErrStale) || useCalled != test.wantUse {
				t.Fatalf("fresh shared-head drift was not rejected: useCalled=%t want=%t err=%v", useCalled, test.wantUse, err)
			}
		})
	}
}

func TestCurrentDatasetSnapshotSelectionClonesCompleteMultiGenerationPath(t *testing.T) {
	fixture := newSealedServiceFixtureV2(t)
	if _, err := fixture.service.Accept(context.Background(), AcceptInput{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: fixture.material.Observation, SourceManifestHash: fixture.manifest.SourceManifestHash,
		RawManifestSHA256: fixture.producer.RawManifestSHA256, ParserVersion: "legacy-parser-v1",
		AcceptedAt: time.Date(2026, 7, 21, 8, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	resolved, err := fixture.service.AdmitExactV2(context.Background(), fixture.admitInput())
	if err != nil {
		t.Fatal(err)
	}
	securityContext := fixture.securityContext(t, resolved, "turn-multi-generation")
	err = fixture.service.WithCurrentSelectionV2(
		context.Background(), fixture.resolveInput(resolved), securityContext,
		func(selection datasetsnapshotport.CurrentSelectionV2, capability datasetsnapshotport.CurrentSelectionCapabilityV2) error {
			if len(selection.DatasetIndexPath) != 2 ||
				selection.DatasetIndexPath[0] != selection.SelectedIndex ||
				selection.DatasetIndexPath[0].PreviousIndexDigest != selection.DatasetIndexPath[1].IndexDigest ||
				selection.DatasetIndexPath[1].PreviousIndexDigest != domainsecurity.DatasetSnapshotIndexGenesisDigestV1() {
				t.Fatalf("complete root-to-genesis path was not exposed: %#v", selection.DatasetIndexPath)
			}
			selection.DatasetIndexPath[1].IndexDigest = domainsecurity.SHA256Hex([]byte("caller-path-copy-change"))
			called := false
			if err := capability.UseExact(selection, securityContext, func(context.Context) error {
				called = true
				return nil
			}); err == nil || called {
				t.Fatal("mutated cloned ancestry retained current DSV2 authority")
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRetainedDatasetSnapshotSelectionUsesWitnessedHistoricalPath(t *testing.T) {
	fixture := newSealedServiceFixtureV2(t)
	historical, current := admitRetainedPairV2(t, fixture)
	currentContext := fixture.securityContext(t, current, "turn-retained-current")
	var (
		callbackCalls int
		resolved      datasetsnapshotport.RetainedSelectionV2
	)
	err := fixture.service.WithRetainedSelectionV2(
		context.Background(),
		datasetsnapshotport.RetainedSelectionInputV2{
			CurrentResolveInput:        fixture.resolveInput(current),
			CurrentSecurityContext:     currentContext,
			RetainedDatasetSnapshotID:  historical.Record.DatasetSnapshotID,
			RetainedSourceManifestHash: historical.Record.SourceManifestHash,
		},
		func(_ context.Context, selection datasetsnapshotport.RetainedSelectionV2) error {
			callbackCalls++
			resolved = selection
			return nil
		},
	)
	if err != nil || callbackCalls != 1 ||
		resolved.Snapshot.Record.DatasetSnapshotID != historical.Record.DatasetSnapshotID ||
		resolved.Snapshot.Record.SourceManifestHash != historical.Record.SourceManifestHash ||
		resolved.Current.Snapshot.Record.DatasetSnapshotID != current.Record.DatasetSnapshotID ||
		resolved.SelectedIndex.SnapshotRecordDigest != historical.Record.RecordDigest {
		t.Fatalf("retained DSV2 selection mismatch: callbacks=%d selection=%#v err=%v", callbackCalls, resolved, err)
	}
}

func TestRetainedDatasetSnapshotSelectionFailsClosedBeforeCallback(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*sealedServiceFixtureV2, datasetsnapshotport.ResolvedSnapshotV2)
	}{
		{
			name: "missing historical bundle",
			mutate: func(fixture *sealedServiceFixtureV2, historical datasetsnapshotport.ResolvedSnapshotV2) {
				fixture.bundles.mu.Lock()
				delete(fixture.bundles.records, historical.Record.RecordDigest)
				fixture.bundles.mu.Unlock()
			},
		},
		{
			name: "corrupt historical manifest material",
			mutate: func(fixture *sealedServiceFixtureV2, historical datasetsnapshotport.ResolvedSnapshotV2) {
				body, err := domainsecurity.DatasetSnapshotManifestV2Bytes(historical.Manifest)
				if err != nil {
					panic(err)
				}
				fixture.materials.mu.Lock()
				fixture.materials.values[datasetsnapshotport.MaterialSnapshotManifestV2][domainsecurity.SHA256Hex(body)][0] ^= 0xff
				fixture.materials.mu.Unlock()
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newSealedServiceFixtureV2(t)
			historical, current := admitRetainedPairV2(t, fixture)
			currentContext := fixture.securityContext(t, current, "turn-retained-current")
			test.mutate(fixture, historical)
			callbackCalls := 0
			err := fixture.service.WithRetainedSelectionV2(
				context.Background(),
				datasetsnapshotport.RetainedSelectionInputV2{
					CurrentResolveInput:        fixture.resolveInput(current),
					CurrentSecurityContext:     currentContext,
					RetainedDatasetSnapshotID:  historical.Record.DatasetSnapshotID,
					RetainedSourceManifestHash: historical.Record.SourceManifestHash,
				},
				func(context.Context, datasetsnapshotport.RetainedSelectionV2) error {
					callbackCalls++
					return nil
				},
			)
			if err == nil || callbackCalls != 0 {
				t.Fatalf("retained material failure crossed callback: callbacks=%d err=%v", callbackCalls, err)
			}
		})
	}

	fixture := newSealedServiceFixtureV2(t)
	historical, current := admitRetainedPairV2(t, fixture)
	currentContext := fixture.securityContext(t, current, "turn-retained-target-missing")
	callbackCalls := 0
	err := fixture.service.WithRetainedSelectionV2(
		context.Background(),
		datasetsnapshotport.RetainedSelectionInputV2{
			CurrentResolveInput:        fixture.resolveInput(current),
			CurrentSecurityContext:     currentContext,
			RetainedDatasetSnapshotID:  domainsecurity.DatasetSnapshotIDPrefixV2 + domainsecurity.SHA256Hex([]byte("not-witnessed")),
			RetainedSourceManifestHash: historical.Record.SourceManifestHash,
		},
		func(context.Context, datasetsnapshotport.RetainedSelectionV2) error {
			callbackCalls++
			return nil
		},
	)
	if !errors.Is(err, datasetsnapshotport.ErrNotFound) || callbackCalls != 0 {
		t.Fatalf("unwitnessed retained target was accepted: callbacks=%d err=%v", callbackCalls, err)
	}
}

func admitRetainedPairV2(
	t *testing.T,
	fixture *sealedServiceFixtureV2,
) (datasetsnapshotport.ResolvedSnapshotV2, datasetsnapshotport.ResolvedSnapshotV2) {
	t.Helper()
	historical, err := fixture.service.AdmitExactV2(context.Background(), fixture.admitInput())
	if err != nil {
		t.Fatal(err)
	}
	producer := fixture.producer
	producer.SourceRevision++
	producerBody, err := domainsecurity.FundsProducerContentManifestV1Bytes(producer)
	if err != nil {
		t.Fatal(err)
	}
	producerDigest := domainsecurity.SHA256Hex(producerBody)
	fixture.materials.mu.Lock()
	fixture.materials.values[datasetsnapshotport.MaterialFundsProducerContentV1][producerDigest] = producerBody
	fixture.materials.mu.Unlock()
	manifestInput, err := fixturev2.CloneManifestInputV2(historical)
	if err != nil {
		t.Fatal(err)
	}
	manifestInput.FundsProducerContentManifest = producer
	manifest, err := domainsecurity.NewDatasetSnapshotManifestV2(manifestInput)
	if err != nil {
		t.Fatal(err)
	}
	manifestBody, err := domainsecurity.DatasetSnapshotManifestV2Bytes(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest := domainsecurity.SHA256Hex(manifestBody)
	fixture.materials.mu.Lock()
	fixture.materials.values[datasetsnapshotport.MaterialSnapshotManifestV2][manifestDigest] = manifestBody
	fixture.materials.mu.Unlock()
	nextInput := fixture.admitInput()
	nextInput.ManifestReference = exactReferenceV2(manifestDigest, uint64(len(manifestBody)))
	nextInput.FundsProducerReference = exactReferenceV2(producerDigest, uint64(len(producerBody)))
	nextInput.AcceptedAt = time.Date(2026, 7, 21, 9, 30, 0, 0, time.UTC)
	current, err := fixture.service.AdmitExactV2(context.Background(), nextInput)
	if err != nil {
		t.Fatal(err)
	}
	if historical.Record.DatasetSnapshotID == current.Record.DatasetSnapshotID {
		t.Fatal("retained pair did not create two distinct DSV2 snapshots")
	}
	return historical, current
}

type sealedServiceFixtureV2 struct {
	material    fixturev2.Fixture
	manifest    domainsecurity.DatasetSnapshotManifestV2
	producer    domainsecurity.FundsProducerContentManifestV1
	authority   *datasetSnapshotTestAuthority
	coordinator *datasetSnapshotTestCoordinator
	materials   *sealedMemoryMaterialReaderV2
	bundles     *sealedMemoryBundleStoreV2
	indexes     *sealedMemoryIndexStoreV2
	legacy      *sealedMemoryLegacyStoreV2
	service     *SealedServiceV2
}

func newSealedServiceFixtureV2(t *testing.T) *sealedServiceFixtureV2 {
	t.Helper()
	material, err := fixturev2.Load()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := domainsecurity.ParseDatasetSnapshotManifestV2(
		material.Materials[datasetsnapshotport.MaterialSnapshotManifestV2][material.ManifestReference.Address],
	)
	if err != nil {
		t.Fatal(err)
	}
	producer, err := domainsecurity.ParseFundsProducerContentManifestV1(
		material.Materials[datasetsnapshotport.MaterialFundsProducerContentV1][material.ProducerReference.Address],
	)
	if err != nil || domainsecurity.ValidateDatasetSnapshotManifestV2FundsProducerContentV1(manifest, producer) != nil {
		t.Fatal("sealed DSV2 test fixture top-level binding is invalid")
	}
	authority := newDatasetSnapshotTestAuthority(0x31)
	coordinator := newDatasetSnapshotTestCoordinator(t, authority)
	materials := &sealedMemoryMaterialReaderV2{values: fixturev2.CloneMaterials(material.Materials), reads: map[datasetsnapshotport.MaterialKindV2]int{}}
	bundles := &sealedMemoryBundleStoreV2{records: map[string]datasetsnapshotport.AuthorityBundleV2{}}
	indexes := &sealedMemoryIndexStoreV2{records: map[string]domainsecurity.DatasetSnapshotIndexV1{}}
	legacy := &sealedMemoryLegacyStoreV2{records: map[string]domainsecurity.DatasetSnapshotAuthorityRecordV1{}}
	service, err := NewSealedV2(SealedConfigV2{
		InstallationID: coordinator.installationID, EnrollmentID: coordinator.enrollmentID,
		Authority: authority, Coordinator: coordinator, LegacyRecords: legacy,
		Bundles: bundles, Indexes: indexes, Materials: materials, Random: &datasetSnapshotCounterReader{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &sealedServiceFixtureV2{
		material: material, manifest: manifest, producer: producer,
		authority: authority, coordinator: coordinator, materials: materials,
		bundles: bundles, indexes: indexes, legacy: legacy, service: service,
	}
}

func (fixture *sealedServiceFixtureV2) admitInput() AdmitInputV2 {
	return AdmitInputV2{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: fixture.material.Observation, ManifestReference: fixture.material.ManifestReference,
		FundsProducerReference: fixture.material.ProducerReference,
		AcceptedAt:             time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC),
	}
}

func (fixture *sealedServiceFixtureV2) resolveInput(
	resolved datasetsnapshotport.ResolvedSnapshotV2,
) datasetsnapshotport.ResolveInputV2 {
	return datasetsnapshotport.ResolveInputV2{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation:               fixture.material.Observation,
		ExpectedDatasetSnapshotID: resolved.Record.DatasetSnapshotID,
	}
}

func (fixture *sealedServiceFixtureV2) securityContext(
	t *testing.T,
	resolved datasetsnapshotport.ResolvedSnapshotV2,
	turnID string,
) domainsecurity.TurnSecurityContext {
	t.Helper()
	threadID := "thread-current-dataset-snapshot"
	policyDigest := domainsecurity.SHA256Hex([]byte("current-dataset-snapshot-risk-policy"))
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest:   policyDigest,
		RiskClass:                domainsecurity.RiskClassCase,
		Disposition:              domainsecurity.PublicationDispositionCaseEvidenceGate,
		CaseBindingState:         domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: fixture.material.Observation.ObservationDigest,
		BlockerCode:              domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	riskBinding, err := securitycontextfixture.WitnessedRiskBinding(
		threadID, fixture.material.Observation.WorkspaceRealPath, domainsecurity.RiskClassCase, policyDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID,
		WorkspaceRealPath: fixture.material.Observation.WorkspaceRealPath,
		TenantID:          domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: fixture.material.Observation.CaseID, CaseBindingHash: fixture.material.Observation.CaseBindingHash,
		DatasetSnapshotID: resolved.Record.DatasetSnapshotID, SourceManifestHash: resolved.Record.SourceManifestHash,
		ContextEpoch: 1, IssuedAt: time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC),
		PublicationPolicy: publication, RiskAuthorityBinding: riskBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func (fixture *sealedServiceFixtureV2) newService(
	t *testing.T,
	coordinator evidenceauthorityport.DatasetCoordinator,
) *SealedServiceV2 {
	t.Helper()
	service, err := NewSealedV2(SealedConfigV2{
		InstallationID: fixture.coordinator.installationID, EnrollmentID: fixture.coordinator.enrollmentID,
		Authority: fixture.authority, Coordinator: coordinator, LegacyRecords: fixture.legacy,
		Bundles: fixture.bundles, Indexes: fixture.indexes, Materials: fixture.materials,
		Random: &datasetSnapshotCounterReader{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

type sealedMemoryMaterialReaderV2 struct {
	mu             sync.Mutex
	values         map[datasetsnapshotport.MaterialKindV2]map[string][]byte
	reads          map[datasetsnapshotport.MaterialKindV2]int
	changeOnSecond datasetsnapshotport.MaterialKindV2
}

func (reader *sealedMemoryMaterialReaderV2) ResolveExact(
	_ context.Context,
	kind datasetsnapshotport.MaterialKindV2,
	reference datasetsnapshotport.ExactMaterialReferenceV2,
) ([]byte, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	body, ok := reader.values[kind][reference.Address]
	if !ok {
		return nil, datasetsnapshotport.ErrNotFound
	}
	reader.reads[kind]++
	result := append([]byte(nil), body...)
	if kind == reader.changeOnSecond && reader.reads[kind] == 2 {
		result[len(result)-1] ^= 0xff
	}
	return result, nil
}

type sealedMemoryBundleStoreV2 struct {
	mu      sync.Mutex
	records map[string]datasetsnapshotport.AuthorityBundleV2
}

func (store *sealedMemoryBundleStoreV2) PutIfAbsent(_ context.Context, bundle datasetsnapshotport.AuthorityBundleV2) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if current, exists := store.records[bundle.Record.RecordDigest]; exists &&
		(current.Record != bundle.Record || current.Manifest != bundle.Manifest) {
		return errors.New("bundle conflict")
	}
	store.records[bundle.Record.RecordDigest] = bundle
	return nil
}

func (store *sealedMemoryBundleStoreV2) Resolve(_ context.Context, digest string) (datasetsnapshotport.AuthorityBundleV2, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	bundle, exists := store.records[digest]
	if !exists {
		return datasetsnapshotport.AuthorityBundleV2{}, datasetsnapshotport.ErrNotFound
	}
	return bundle, nil
}

func (store *sealedMemoryBundleStoreV2) count() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return len(store.records)
}

type sealedMemoryIndexStoreV2 struct {
	mu      sync.Mutex
	records map[string]domainsecurity.DatasetSnapshotIndexV1
	failPut bool
}

func (store *sealedMemoryIndexStoreV2) PutIfAbsent(_ context.Context, index domainsecurity.DatasetSnapshotIndexV1) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.failPut {
		return errors.New("simulated index crash")
	}
	if current, exists := store.records[index.IndexDigest]; exists && current != index {
		return errors.New("index conflict")
	}
	store.records[index.IndexDigest] = index
	return nil
}

func (store *sealedMemoryIndexStoreV2) Resolve(_ context.Context, digest string) (domainsecurity.DatasetSnapshotIndexV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	index, exists := store.records[digest]
	if !exists {
		return domainsecurity.DatasetSnapshotIndexV1{}, errors.New("index absent")
	}
	return index, nil
}

type sealedAbsentLegacyStoreV2 struct{}

func (sealedAbsentLegacyStoreV2) PutIfAbsent(context.Context, domainsecurity.DatasetSnapshotAuthorityRecordV1) error {
	return errors.New("legacy store is read-only")
}

func (sealedAbsentLegacyStoreV2) Resolve(context.Context, string) (domainsecurity.DatasetSnapshotAuthorityRecordV1, error) {
	return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, datasetsnapshotport.ErrNotFound
}

type sealedMemoryLegacyStoreV2 struct {
	mu      sync.Mutex
	records map[string]domainsecurity.DatasetSnapshotAuthorityRecordV1
}

func (store *sealedMemoryLegacyStoreV2) PutIfAbsent(
	_ context.Context,
	record domainsecurity.DatasetSnapshotAuthorityRecordV1,
) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if current, exists := store.records[record.RecordDigest]; exists && current != record {
		return errors.New("legacy record conflict")
	}
	store.records[record.RecordDigest] = record
	return nil
}

func (store *sealedMemoryLegacyStoreV2) Resolve(
	_ context.Context,
	digest string,
) (domainsecurity.DatasetSnapshotAuthorityRecordV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	record, exists := store.records[digest]
	if !exists {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, datasetsnapshotport.ErrNotFound
	}
	return record, nil
}

type sealedHeadRacingCoordinatorV2 struct {
	mu            sync.Mutex
	delegate      *datasetSnapshotTestCoordinator
	observeCalls  int
	raceOnObserve int
	otherChild    bool
}

type sealedCountingFreshHeadCoordinatorV2 struct {
	mu           sync.Mutex
	delegate     *datasetSnapshotTestCoordinator
	observations int
	challenges   int
}

func (coordinator *sealedCountingFreshHeadCoordinatorV2) ObserveFresh(
	ctx context.Context,
) (evidenceauthorityport.FreshHead, error) {
	coordinator.mu.Lock()
	coordinator.observations++
	coordinator.mu.Unlock()
	return coordinator.delegate.ObserveFresh(ctx)
}

func (coordinator *sealedCountingFreshHeadCoordinatorV2) AdvanceDatasetSnapshot(
	ctx context.Context,
	input evidenceauthorityport.DatasetAdvanceInput,
) (evidenceauthorityport.FreshHead, error) {
	return coordinator.delegate.AdvanceDatasetSnapshot(ctx, input)
}

func (coordinator *sealedCountingFreshHeadCoordinatorV2) WithFreshHeadChallenge(
	ctx context.Context,
	use func(evidenceauthorityport.FreshHead, func(context.Context) error) error,
) error {
	head, err := coordinator.ObserveFresh(ctx)
	if err != nil {
		return err
	}
	active := true
	defer func() { active = false }()
	return use(head, func(challengeContext context.Context) error {
		if !active || challengeContext == nil || challengeContext.Err() != nil {
			return errors.New("test fresh-head challenge is inactive")
		}
		coordinator.mu.Lock()
		coordinator.challenges++
		coordinator.mu.Unlock()
		current, err := coordinator.delegate.ObserveFresh(challengeContext)
		if err != nil {
			return err
		}
		if current.Bundle.RecordDigest != head.Bundle.RecordDigest {
			return errors.New("test fresh-head challenge changed")
		}
		return nil
	})
}

func (coordinator *sealedCountingFreshHeadCoordinatorV2) fullObservationCount() int {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	return coordinator.observations
}

func (coordinator *sealedCountingFreshHeadCoordinatorV2) challengeCount() int {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	return coordinator.challenges
}

type realAuthoritySealedServiceFixtureV2 struct {
	base                  *sealedServiceFixtureV2
	service               *SealedServiceV2
	authorityBundles      *sealedEvidenceBundleStoreV2
	authorityObservations *sealedEvidenceObservationStoreV2
	authorityProjection   *sealedEvidenceProjectionV2
	authorityWitness      *sealedEvidenceWitnessV2
}

func newRealAuthoritySealedServiceFixtureV2(t *testing.T) *realAuthoritySealedServiceFixtureV2 {
	t.Helper()
	base := newSealedServiceFixtureV2(t)
	installationID := domainsecurity.SHA256Hex([]byte("real-sealed-authority-installation"))
	enrollmentID := domainsecurity.SHA256Hex([]byte("real-sealed-authority-enrollment"))
	witnessPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x5a}, ed25519.SeedSize))
	witnessPublic := witnessPrivate.Public().(ed25519.PublicKey)
	witness := newSealedEvidenceWitnessV2(t, installationID, enrollmentID, witnessPrivate)
	bundles := &sealedEvidenceBundleStoreV2{records: map[string]domainevidence.EvidenceAuthorityBundleV1{}}
	observations := &sealedEvidenceObservationStoreV2{records: map[string]evidenceauthorityport.ObservationBundle{}}
	projection := &sealedEvidenceProjectionV2{}
	authority, err := evidenceauthorityapp.New(evidenceauthorityapp.Config{
		InstallationID:   installationID,
		EnrollmentID:     enrollmentID,
		Authority:        base.authority,
		WitnessKeyID:     domainsecurity.SHA256Hex(witnessPublic),
		WitnessPublicKey: witnessPublic,
		Random:           &datasetSnapshotCounterReader{},
		Witness:          witness,
		CheckpointFloor:  &sealedEvidenceCheckpointFloorV2{},
		Bundles:          bundles,
		Observations:     observations,
		Projection:       projection,
		Genesis: evidenceauthorityapp.Genesis{
			DatasetSnapshotIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
			EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
			PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authority.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	service, err := NewSealedV2(SealedConfigV2{
		InstallationID: installationID,
		EnrollmentID:   enrollmentID,
		Authority:      base.authority,
		Coordinator:    authority,
		LegacyRecords:  base.legacy,
		Bundles:        base.bundles,
		Indexes:        base.indexes,
		Materials:      base.materials,
		Random:         &datasetSnapshotCounterReader{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &realAuthoritySealedServiceFixtureV2{
		base: base, service: service, authorityBundles: bundles,
		authorityObservations: observations, authorityProjection: projection,
		authorityWitness: witness,
	}
}

type sealedEvidenceBundleStoreV2 struct {
	mu           sync.Mutex
	records      map[string]domainevidence.EvidenceAuthorityBundleV1
	resolveCalls atomic.Int32
}

func (store *sealedEvidenceBundleStoreV2) PutIfAbsent(
	ctx context.Context,
	bundle domainevidence.EvidenceAuthorityBundleV1,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if current, exists := store.records[bundle.RecordDigest]; exists && current != bundle {
		return errors.New("evidence bundle conflict")
	}
	store.records[bundle.RecordDigest] = bundle
	return nil
}

func (store *sealedEvidenceBundleStoreV2) Resolve(
	ctx context.Context,
	digest string,
) (domainevidence.EvidenceAuthorityBundleV1, error) {
	store.resolveCalls.Add(1)
	if err := ctx.Err(); err != nil {
		return domainevidence.EvidenceAuthorityBundleV1{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	bundle, exists := store.records[digest]
	if !exists {
		return domainevidence.EvidenceAuthorityBundleV1{}, errors.New("evidence bundle absent")
	}
	return bundle, nil
}

type sealedEvidenceObservationStoreV2 struct {
	mu           sync.Mutex
	records      map[string]evidenceauthorityport.ObservationBundle
	putCalls     atomic.Int32
	resolveCalls atomic.Int32
}

func (store *sealedEvidenceObservationStoreV2) PutIfAbsent(
	ctx context.Context,
	bundle evidenceauthorityport.ObservationBundle,
) error {
	store.putCalls.Add(1)
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.records[bundle.Observation.ObservationDigest] = bundle
	return nil
}

func (store *sealedEvidenceObservationStoreV2) Resolve(
	ctx context.Context,
	digest string,
) (evidenceauthorityport.ObservationBundle, error) {
	store.resolveCalls.Add(1)
	if err := ctx.Err(); err != nil {
		return evidenceauthorityport.ObservationBundle{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	bundle, exists := store.records[digest]
	if !exists {
		return evidenceauthorityport.ObservationBundle{}, errors.New("evidence observation absent")
	}
	return bundle, nil
}

type sealedEvidenceProjectionV2 struct {
	mu           sync.Mutex
	bundle       domainevidence.EvidenceAuthorityBundleV1
	projectCalls atomic.Int32
}

func (projection *sealedEvidenceProjectionV2) ValidateWitnessEmpty(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	projection.mu.Lock()
	defer projection.mu.Unlock()
	if projection.bundle.Generation != 0 {
		return errors.New("evidence projection is not empty")
	}
	return nil
}

func (projection *sealedEvidenceProjectionV2) ProjectWitnessSelected(
	ctx context.Context,
	bundle domainevidence.EvidenceAuthorityBundleV1,
) error {
	projection.projectCalls.Add(1)
	if err := ctx.Err(); err != nil {
		return err
	}
	projection.mu.Lock()
	projection.bundle = bundle
	projection.mu.Unlock()
	return nil
}

type sealedEvidenceCheckpointFloorV2 struct {
	mu         sync.Mutex
	checkpoint *domainsecurity.MonotonicHeadCheckpointV1
}

func (floor *sealedEvidenceCheckpointFloorV2) ProjectWitnessSelected(
	ctx context.Context,
	selected domainsecurity.MonotonicHeadCheckpointV1,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	floor.mu.Lock()
	defer floor.mu.Unlock()
	if floor.checkpoint == nil {
		if selected.Generation != 0 {
			return monotonicheadport.ErrCheckpointFloorBootstrap
		}
		copy := selected
		floor.checkpoint = &copy
		return nil
	}
	if *floor.checkpoint == selected {
		return nil
	}
	if domainsecurity.ValidateMonotonicHeadCheckpointDirectSuccessorV1(*floor.checkpoint, selected) != nil {
		return monotonicheadport.ErrCheckpointFloorConflict
	}
	copy := selected
	floor.checkpoint = &copy
	return nil
}

type sealedEvidenceWitnessV2 struct {
	mu           sync.Mutex
	private      ed25519.PrivateKey
	public       ed25519.PublicKey
	checkpoint   domainsecurity.MonotonicHeadCheckpointV1
	observeCalls atomic.Int32
}

func newSealedEvidenceWitnessV2(
	t *testing.T,
	installationID string,
	enrollmentID string,
	private ed25519.PrivateKey,
) *sealedEvidenceWitnessV2 {
	t.Helper()
	public := private.Public().(ed25519.PublicKey)
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID:     installationID,
		EnrollmentID:       enrollmentID,
		Namespace:          domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1,
		Generation:         0,
		CurrentStateDigest: domainsecurity.SHA256Hex([]byte("real-sealed-authority-empty")),
		FenceNonce:         domainsecurity.SHA256Hex([]byte("real-sealed-authority-fence")),
		WitnessKeyID:       domainsecurity.SHA256Hex(public),
		WitnessPublicKey:   public,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(private, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return &sealedEvidenceWitnessV2{private: private, public: public, checkpoint: checkpoint}
}

func (witness *sealedEvidenceWitnessV2) Observe(
	ctx context.Context,
	request domainsecurity.MonotonicHeadObserveRequestV1,
) (domainsecurity.MonotonicHeadObservationV1, error) {
	witness.observeCalls.Add(1)
	if err := ctx.Err(); err != nil {
		return domainsecurity.MonotonicHeadObservationV1{}, err
	}
	witness.mu.Lock()
	checkpoint := witness.checkpoint
	witness.mu.Unlock()
	return domainsecurity.NewMonotonicHeadObservationV1(request, checkpoint, func(message []byte) ([]byte, error) {
		return ed25519.Sign(witness.private, message), nil
	})
}

func (witness *sealedEvidenceWitnessV2) Advance(
	ctx context.Context,
	request domainsecurity.MonotonicHeadAdvanceRequestV1,
) (domainsecurity.MonotonicHeadAdvanceReceiptV1, error) {
	if err := ctx.Err(); err != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, err
	}
	witness.mu.Lock()
	defer witness.mu.Unlock()
	previous := witness.checkpoint
	if request.ExpectedGeneration != previous.Generation ||
		request.ExpectedCheckpointDigest != previous.CheckpointDigest ||
		request.ExpectedStateDigest != previous.CurrentStateDigest ||
		request.ExpectedFenceNonce != previous.FenceNonce {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonicheadport.ErrCASConflict
	}
	next, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID:           previous.InstallationID,
		EnrollmentID:             previous.EnrollmentID,
		Namespace:                previous.Namespace,
		Generation:               request.NextGeneration,
		CurrentStateDigest:       request.NextStateDigest,
		PreviousStateDigest:      previous.CurrentStateDigest,
		PreviousCheckpointDigest: previous.CheckpointDigest,
		FenceNonce:               domainsecurity.SHA256Hex([]byte("real-sealed-authority-fence:" + request.MutationID)),
		MutationID:               request.MutationID,
		WitnessKeyID:             domainsecurity.SHA256Hex(witness.public),
		WitnessPublicKey:         witness.public,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(witness.private, message), nil })
	if err != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, err
	}
	witness.checkpoint = next
	return domainsecurity.NewMonotonicHeadAdvanceReceiptV1(request, next, func(message []byte) ([]byte, error) {
		return ed25519.Sign(witness.private, message), nil
	})
}

func (coordinator *sealedHeadRacingCoordinatorV2) ObserveFresh(
	ctx context.Context,
) (evidenceauthorityport.FreshHead, error) {
	coordinator.mu.Lock()
	coordinator.observeCalls++
	race := coordinator.observeCalls == coordinator.raceOnObserve
	otherChild := coordinator.otherChild
	coordinator.mu.Unlock()
	if race {
		if otherChild {
			return advanceSealedRegistryChildV2(ctx, coordinator.delegate)
		}
		current := coordinator.delegate.currentBundle()
		return coordinator.delegate.AdvanceDatasetSnapshot(ctx, evidenceauthorityport.DatasetAdvanceInput{
			ExpectedBundleDigest: current.RecordDigest,
			NextIndexDigest:      domainsecurity.SHA256Hex([]byte("external-current-head-race")),
		})
	}
	return coordinator.delegate.ObserveFresh(ctx)
}

func (coordinator *sealedHeadRacingCoordinatorV2) AdvanceDatasetSnapshot(
	ctx context.Context,
	input evidenceauthorityport.DatasetAdvanceInput,
) (evidenceauthorityport.FreshHead, error) {
	return coordinator.delegate.AdvanceDatasetSnapshot(ctx, input)
}

func advanceSealedRegistryChildV2(
	ctx context.Context,
	coordinator *datasetSnapshotTestCoordinator,
) (evidenceauthorityport.FreshHead, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	previous := coordinator.bundle
	nextGeneration := previous.Generation + 1
	next, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: previous.InstallationID, EnrollmentID: previous.EnrollmentID,
		Generation: nextGeneration, PreviousBundleDigest: previous.RecordDigest,
		MutationID: domainsecurity.SHA256Hex([]byte(fmt.Sprintf(
			"sealed-registry-child-mutation:%d", nextGeneration,
		))),
		DatasetSnapshotIndexDigest: previous.DatasetSnapshotIndexDigest,
		DatasetSnapshotCount:       previous.DatasetSnapshotCount,
		EvidenceRegistryIndexDigest: domainsecurity.SHA256Hex([]byte(fmt.Sprintf(
			"sealed-registry-child-root:%d", nextGeneration,
		))),
		EvidenceRegistryCount:  previous.EvidenceRegistryCount + 1,
		PublicationIndexDigest: previous.PublicationIndexDigest,
		PublicationCount:       previous.PublicationCount,
		AuthorityKeyID:         coordinator.authority.KeyID(),
		AuthorityPublicKey:     coordinator.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) {
		return coordinator.authority.Sign(ctx, message)
	})
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	checkpoint, err := coordinator.checkpointFor(next, coordinator.checkpoint)
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	coordinator.bundle = next
	coordinator.checkpoint = checkpoint
	coordinator.successfulAdvances++
	return coordinator.observeLocked()
}
