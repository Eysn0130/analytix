package piiauthorization

import (
	"context"
	"sort"
	"testing"

	domainartifactdelivery "analytix.local/runtime-go/internal/domain/artifactdelivery"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	artifactdeliveryport "analytix.local/runtime-go/internal/ports/artifactdelivery"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
)

func TestControlledAccessV2RestartTerminalizesCrashOpenAsFullLengthIndeterminate(t *testing.T) {
	fixture := newLiveControlledAccessFixtureV2(t)
	result, err := fixture.service.ReleaseV2(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	delete(fixture.access.dispositions, result.AccessID)
	fixture.access.semanticStage = true
	dependencies := controlledAccessInventoryDependenciesV2(fixture)
	plan, err := VerifyControlledAccessInventoryV2(context.Background(), dependencies)
	if err != nil || plan.OpenReceiptCount() != 1 {
		t.Fatalf("V2 crash-open reservation was not recovered: open=%d err=%v", plan.OpenReceiptCount(), err)
	}
	if err := ApplyControlledAccessRestartPlanV2(
		context.Background(), plan, fixture.access, fixture.base.authority, fixture.now.Add(4),
	); err != nil {
		t.Fatal(err)
	}
	closed, err := VerifyControlledAccessInventoryV2(context.Background(), dependencies)
	if err != nil || closed.OpenReceiptCount() != 0 {
		t.Fatalf("V2 restart left an open reservation: open=%d err=%v", closed.OpenReceiptCount(), err)
	}
	disposition, err := fixture.access.ResolveAccessDispositionV2(context.Background(), result.AccessID)
	if err != nil || disposition.Status != domainpii.ControlledArtifactAccessDispositionReleaseIndeterminateV1 ||
		disposition.ReasonCode != domainpii.ControlledArtifactAccessReasonReleaseIndeterminateV1 ||
		disposition.ReleasedByteLength != disposition.ArtifactByteLength {
		t.Fatalf("V2 restart understated possible PII exposure: disposition=%#v err=%v", disposition, err)
	}
}

func TestControlledAccessV2RestartRefusesLiveTerminalizationAuthority(t *testing.T) {
	fixture := newLiveControlledAccessFixtureV2(t)
	result, err := fixture.service.ReleaseV2(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	delete(fixture.access.dispositions, result.AccessID)
	plan, err := VerifyControlledAccessInventoryV2(context.Background(), controlledAccessInventoryDependenciesV2(fixture))
	if err != nil || plan.OpenReceiptCount() != 1 {
		t.Fatalf("V2 open plan failed: open=%d err=%v", plan.OpenReceiptCount(), err)
	}
	fixture.access.semanticStage = false
	if err := ApplyControlledAccessRestartPlanV2(
		context.Background(), plan, fixture.access, fixture.base.authority, fixture.now.Add(4),
	); err == nil {
		t.Fatal("live V2 store acquired restart terminalization authority")
	}
	if len(fixture.access.dispositions) != 0 {
		t.Fatalf("failed live V2 restart mutated the journal: %d", len(fixture.access.dispositions))
	}
}

func TestControlledAccessV2InventoryRequiresExactHistoricalProjectedOutcome(t *testing.T) {
	fixture := newLiveControlledAccessFixtureV2(t)
	if _, err := fixture.service.ReleaseV2(context.Background(), fixture.input); err != nil {
		t.Fatal(err)
	}
	fixtureHistorical := controlledAccessHistoricalStubV2{
		delivery: fixture.delivery,
		err:      artifactdeliveryport.ErrIntegrity,
	}
	dependencies := controlledAccessInventoryDependenciesV2(fixture)
	dependencies.Historical = fixtureHistorical
	if _, err := VerifyControlledAccessInventoryV2(context.Background(), dependencies); err == nil {
		t.Fatal("V2 inventory accepted an access receipt without its exact historical projected outcome")
	}
}

func controlledAccessInventoryDependenciesV2(
	fixture *controlledAccessLiveFixtureV2,
) ControlledAccessInventoryDependenciesV2 {
	return ControlledAccessInventoryDependenciesV2{
		Access: fixture.access,
		Historical: controlledAccessHistoricalStubV2{
			delivery: fixture.delivery,
		},
		Grants:    fixture.base.grants,
		Artifacts: fixture.base.artifacts, Authority: fixture.base.authority,
	}
}

type controlledAccessHistoricalStubV2 struct {
	delivery domainartifactdelivery.VerifiedArtifactDeliveryV1
	err      error
}

func (stub controlledAccessHistoricalStubV2) ResolveTrustedHistorical(
	_ context.Context,
	selector artifactdeliveryport.HistoricalSelectorV1,
) (domainartifactdelivery.VerifiedArtifactDeliveryV1, error) {
	if stub.err != nil {
		return domainartifactdelivery.VerifiedArtifactDeliveryV1{}, stub.err
	}
	delivery := stub.delivery
	if selector.DeliveryID != delivery.DeliveryID || selector.OutcomeRecordDigest != delivery.OutcomeRecordDigest ||
		selector.PublicationCommitDigest != delivery.PublicationCommitDigest ||
		selector.Context != delivery.Context {
		return domainartifactdelivery.VerifiedArtifactDeliveryV1{}, artifactdeliveryport.ErrIntegrity
	}
	return delivery, nil
}

func (store *memoryControlledAccessStoreV2) IsSemanticStageControlledAccessStoreV2() bool {
	return store != nil && store.semanticStage
}

func (store *memoryControlledAccessStoreV2) VisitAccessReceiptsV2(
	_ context.Context,
	visit func(domainpii.ControlledArtifactAccessReceiptV2) error,
) error {
	keys := make([]string, 0, len(store.receipts))
	for key := range store.receipts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := visit(store.receipts[key]); err != nil {
			return err
		}
	}
	return nil
}

func (store *memoryControlledAccessStoreV2) VisitAccessDispositionsV2(
	_ context.Context,
	visit func(domainpii.ControlledArtifactAccessDispositionV2) error,
) error {
	keys := make([]string, 0, len(store.dispositions))
	for key := range store.dispositions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		disposition := store.dispositions[key]
		receipt, found := store.receipts[key]
		if !found || domainpii.ValidateControlledArtifactAccessDispositionForReceiptV2(disposition, receipt) != nil {
			return piiauthorizationport.ErrCorrupt
		}
		if err := visit(disposition); err != nil {
			return err
		}
	}
	return nil
}

var _ piiauthorizationport.RestartAccessStoreV2 = (*memoryControlledAccessStoreV2)(nil)
var _ piiauthorizationport.AccessInventoryStoreV2 = (*memoryControlledAccessStoreV2)(nil)
var _ artifactdeliveryport.HistoricalAuthority = controlledAccessHistoricalStubV2{}
