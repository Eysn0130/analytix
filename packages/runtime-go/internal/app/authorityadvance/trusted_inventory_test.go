package authorityadvance

import (
	"context"
	"testing"

	domainauthority "analytix.local/runtime-go/internal/domain/authorityadvance"
)

func (store *coordinatorIntentStoreV2) VisitIntents(
	_ context.Context,
	visit func(domainauthority.MonotonicAdvanceIntentV2) error,
) error {
	for _, intent := range store.records {
		if err := visit(intent); err != nil {
			return err
		}
	}
	return nil
}

func (store *coordinatorSettlementStoreV2) VisitSettlements(
	_ context.Context,
	visit func(domainauthority.MonotonicAdvanceSettlementV2) error,
) error {
	for _, settlement := range store.records {
		if err := visit(settlement); err != nil {
			return err
		}
	}
	return nil
}

func TestAuthorityAdvanceTrustedInventoryPinsExactCurrentInstallationGraph(t *testing.T) {
	fixture := newCoordinatorFixtureV1(t, "trusted-inventory")
	if _, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent); err != nil {
		t.Fatal(err)
	}
	if err := VerifyTrustedInventoryV2(
		context.Background(), fixture.intents, fixture.settlements, fixture.authority,
	); err != nil {
		t.Fatalf("trusted exact authority advance graph was rejected: %v", err)
	}
}

func TestAuthorityAdvanceTrustedInventoryRejectsOrphanAndForeignAuthority(t *testing.T) {
	fixture := newCoordinatorFixtureV1(t, "trusted-orphan")
	if _, err := fixture.coordinator.CommitExact(context.Background(), fixture.intent); err != nil {
		t.Fatal(err)
	}
	delete(fixture.intents.records, fixture.intent.MutationID)
	if err := VerifyTrustedInventoryV2(
		context.Background(), fixture.intents, fixture.settlements, fixture.authority,
	); err == nil {
		t.Fatal("orphan settlement passed trusted inventory")
	}

	foreign := newCoordinatorFixtureV1(t, "trusted-foreign")
	fixture.intents.records[foreign.intent.MutationID] = foreign.intent
	fixture.settlements.records = map[string]domainauthority.MonotonicAdvanceSettlementV2{}
	if err := VerifyTrustedInventoryV2(
		context.Background(), fixture.intents, fixture.settlements, fixture.authority,
	); err == nil {
		t.Fatal("foreign authority intent passed trusted inventory")
	}
}
