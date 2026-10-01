package datasetsnapshot

import (
	"context"
	"errors"
	"testing"

	evidenceauthorityapp "analytix.local/runtime-go/internal/app/evidenceauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
)

func TestHistoricalFactSelectionUsesBoundFreshChallenges(t *testing.T) {
	fixture := newRealAuthoritySealedServiceFixtureV2(t)
	fixture.base.service = fixture.service
	var historicalHead evidenceauthorityport.FreshHead
	old, current := admitRetainedPairV2(t, fixture.base, func(evidenceauthorityport.FreshHead) {
		var err error
		historicalHead, err = fixture.service.observeFresh(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	})
	currentHead, err := fixture.service.observeFresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	beforePut := fixture.authorityObservations.putCalls.Load()
	beforeResolve := fixture.authorityObservations.resolveCalls.Load()
	beforeProjection := fixture.authorityProjection.projectCalls.Load()
	beforeWitness := fixture.authorityWitness.observeCalls.Load()
	beforeMaterial := historicalMaterialReadCountV2(fixture.base)
	callbacks := 0
	err = fixture.service.WithHistoricalFactSelectionV2(
		context.Background(), fixture.base.resolveInput(old), fixture.base.securityContext(t, old, "historical-old"), historicalHead,
		func(selection datasetsnapshotport.CurrentSelectionV2) error {
			callbacks++
			if selection.Snapshot != old || selection.Head.Bundle != historicalHead.Bundle {
				return errors.New("old historical fact rebound to current selection")
			}
			return fixture.service.WithHistoricalFactSelectionV2(
				context.Background(), fixture.base.resolveInput(current), fixture.base.securityContext(t, current, "historical-current"), currentHead,
				func(nested datasetsnapshotport.CurrentSelectionV2) error {
					callbacks++
					if nested.Snapshot != current || nested.Head.Bundle != currentHead.Bundle {
						return errors.New("current historical fact changed selection")
					}
					return nil
				},
			)
		},
	)
	puts := fixture.authorityObservations.putCalls.Load() - beforePut
	resolves := fixture.authorityObservations.resolveCalls.Load() - beforeResolve
	projections := fixture.authorityProjection.projectCalls.Load() - beforeProjection
	witnesses := fixture.authorityWitness.observeCalls.Load() - beforeWitness
	materials := historicalMaterialReadCountV2(fixture.base) - beforeMaterial
	if err != nil || callbacks != 2 || puts != 2 || resolves != 2 || projections != 2 || witnesses != 6 || materials != 152 {
		t.Fatalf("historical full observations repeated or fresh challenge lost: callbacks=%d puts=%d resolves=%d projections=%d witnesses=%d material_reads=%d err=%v", callbacks, puts, resolves, projections, witnesses, materials, err)
	}
	t.Logf("historical two selections: full_observations=%d fresh_witnesses=%d material_reads=%d", puts, witnesses, materials)
}

func historicalMaterialReadCountV2(fixture *sealedServiceFixtureV2) int {
	fixture.materials.mu.Lock()
	defer fixture.materials.mu.Unlock()
	count := 0
	for _, reads := range fixture.materials.reads {
		count += reads
	}
	return count
}

func TestHistoricalFactSelectionRejectsSignedSharedHeadAdvance(t *testing.T) {
	fixture := newRealAuthoritySealedServiceFixtureV2(t)
	fixture.base.service = fixture.service
	var historicalHead evidenceauthorityport.FreshHead
	old, _ := admitRetainedPairV2(t, fixture.base, func(evidenceauthorityport.FreshHead) {
		var err error
		historicalHead, err = fixture.service.observeFresh(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	})
	calls := 0
	err := fixture.service.WithHistoricalFactSelectionV2(context.Background(), fixture.base.resolveInput(old), fixture.base.securityContext(t, old, "historical-signed-head-change"), historicalHead, func(datasetsnapshotport.CurrentSelectionV2) error {
		calls++
		current, err := fixture.service.observeFresh(context.Background())
		if err != nil {
			return err
		}
		// The synthetic successor is independently signed and witnessed; its
		// child material is intentionally absent. The historical reader must
		// reject the changed shared head before accepting its old candidate.
		advanced, err := fixture.service.coordinator.AdvanceDatasetSnapshot(context.Background(), evidenceauthorityport.DatasetAdvanceInput{
			ExpectedBundleDigest: current.Bundle.RecordDigest,
			NextIndexDigest:      domainsecurity.SHA256Hex([]byte("synthetic-shared-head-successor")),
		})
		if err != nil || fixture.service.validateHead(advanced) != nil || advanced.Bundle.Generation != current.Bundle.Generation+1 {
			t.Fatal("synthetic signed successor was not established", err)
		}
		return nil
	})
	if calls != 1 || !errors.Is(err, evidenceauthorityapp.ErrCurrentChanged) {
		t.Fatalf("historical callback accepted a changed signed head: callbacks=%d err=%v", calls, err)
	}
}

func TestHistoricalFactSelectionWithoutChallengeKeepsFullObservations(t *testing.T) {
	fixture := newSealedServiceFixtureV2(t)
	var head evidenceauthorityport.FreshHead
	old, _ := admitRetainedPairV2(t, fixture, func(value evidenceauthorityport.FreshHead) { head = value })
	coordinator := &sealedHeadRacingCoordinatorV2{delegate: fixture.coordinator}
	service := fixture.newService(t, coordinator)
	calls := 0
	err := service.WithHistoricalFactSelectionV2(context.Background(), fixture.resolveInput(old), fixture.securityContext(t, old, "historical-no-challenge"), head, func(datasetsnapshotport.CurrentSelectionV2) error {
		calls++
		return nil
	})
	if err != nil || calls != 1 || coordinator.observeCalls != 3 {
		t.Fatalf("historical fallback skipped full observations: callbacks=%d full_observations=%d err=%v", calls, coordinator.observeCalls, err)
	}
}

func TestHistoricalFactSelectionBoundChallengeDetectsCallbackDrift(t *testing.T) {
	for _, mode := range []string{"historical-material", "current-index", "head", "cancelled", "callback-error"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newSealedServiceFixtureV2(t)
			var head evidenceauthorityport.FreshHead
			old, _ := admitRetainedPairV2(t, fixture, func(value evidenceauthorityport.FreshHead) { head = value })
			coordinator := &sealedCountingFreshHeadCoordinatorV2{delegate: fixture.coordinator}
			service := fixture.newService(t, coordinator)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			callbackErr := errors.New("historical callback failed")
			calls := 0
			err := service.WithHistoricalFactSelectionV2(ctx, fixture.resolveInput(old), fixture.securityContext(t, old, "historical-drift-"+mode), head, func(datasetsnapshotport.CurrentSelectionV2) error {
				calls++
				switch mode {
				case "historical-material":
					body, _ := domainsecurity.DatasetSnapshotManifestV2Bytes(old.Manifest)
					fixture.materials.mu.Lock()
					fixture.materials.values[datasetsnapshotport.MaterialSnapshotManifestV2][domainsecurity.SHA256Hex(body)] = []byte("tampered")
					fixture.materials.mu.Unlock()
				case "current-index":
					fixture.coordinator.mu.Lock()
					currentRoot := fixture.coordinator.bundle.DatasetSnapshotIndexDigest
					fixture.coordinator.mu.Unlock()
					fixture.indexes.mu.Lock()
					delete(fixture.indexes.records, currentRoot)
					fixture.indexes.mu.Unlock()
				case "head":
					fixture.coordinator.mu.Lock()
					fixture.coordinator.bundle.RecordDigest = domainsecurity.SHA256Hex([]byte("changed-historical-head"))
					fixture.coordinator.mu.Unlock()
				case "cancelled":
					cancel()
				case "callback-error":
					return callbackErr
				}
				return nil
			})
			if err == nil || calls != 1 || (mode == "cancelled" && !errors.Is(err, context.Canceled)) || (mode == "callback-error" && !errors.Is(err, callbackErr)) {
				t.Fatalf("historical callback drift passed: callbacks=%d err=%v", calls, err)
			}
		})
	}
}
