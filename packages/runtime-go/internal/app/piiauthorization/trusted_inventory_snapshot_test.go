package piiauthorization

import (
	"context"
	"errors"
	"testing"
	"time"

	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	piiport "analytix.local/runtime-go/internal/ports/piiauthorization"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

func TestTrustedPublicationInventoriesReleaseVisitorBeforeResolving(t *testing.T) {
	t.Run("grant ledger", func(t *testing.T) {
		fixture := newServiceFixture(t)
		if _, err := fixture.service.Issue(context.Background(), fixture.input); err != nil {
			t.Fatal(err)
		}
		active := false
		if err := VerifyTrustedInventoryV1(context.Background(), snapshotGrantInventory{fixture.grants, &active},
			snapshotLedgerStore{fixture.ledgers, &active}, fixture.authority); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("controlled publication", func(t *testing.T) {
		fixture, authorized, receipt := controlledPublicationInventoryFixture(t, "", time.Time{})
		projections := &controlledProjectionInventoryStore{records: map[string]domainpublication.PIIProjectionV1{
			authorized.PIIProjection.ProjectionDigest: authorized.PIIProjection,
		}}
		active := false
		if err := VerifyControlledPublicationInventoryV1(context.Background(),
			snapshotReceiptInventory{receiptInventoryStub{receipt}, &active}, snapshotProjectionStore{projections, &active},
			fixture.ledgers, fixture.grants, fixture.authority); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("controlled access V1", func(t *testing.T) {
		fixture := newControlledAccessInventoryFixtureV1(t)
		dependencies := fixture.dependencies()
		active := false
		dependencies.Access = snapshotAccessInventoryV1{dependencies.Access, &active}
		dependencies.Grants = snapshotGrantStore{dependencies.Grants, &active}
		if plan, err := VerifyControlledAccessInventoryV1(context.Background(), dependencies); err != nil || plan.OpenReceiptCount() != 1 {
			t.Fatalf("snapshot inventory did not validate the open receipt: %v", err)
		}
	})
	t.Run("controlled access V2", func(t *testing.T) {
		fixture := newLiveControlledAccessFixtureV2(t)
		if _, err := fixture.service.ReleaseV2(context.Background(), fixture.input); err != nil {
			t.Fatal(err)
		}
		dependencies := controlledAccessInventoryDependenciesV2(fixture)
		active := false
		dependencies.Access = snapshotAccessInventoryV2{dependencies.Access, &active}
		dependencies.Grants = snapshotGrantResolver{dependencies.Grants, &active}
		if plan, err := VerifyControlledAccessInventoryV2(context.Background(), dependencies); err != nil || plan.OpenReceiptCount() != 0 {
			t.Fatalf("snapshot inventory did not validate the closed receipt: %v", err)
		}
	})
}

type snapshotGrantInventory struct {
	piiport.InventoryStore
	active *bool
}

func (store snapshotGrantInventory) VisitGrants(ctx context.Context, visit func(domainpii.PIIProjectionGrantV1) error) error {
	*store.active = true
	defer func() { *store.active = false }()
	return store.InventoryStore.VisitGrants(ctx, visit)
}

type snapshotReceiptInventory struct {
	publicationport.ReceiptInventoryStore
	active *bool
}

func (store snapshotReceiptInventory) VisitReceipts(ctx context.Context, visit func(domainpublication.PublicationReceiptV1) error) error {
	*store.active = true
	defer func() { *store.active = false }()
	return store.ReceiptInventoryStore.VisitReceipts(ctx, visit)
}

type snapshotAccessInventoryV1 struct {
	piiport.AccessInventoryStore
	active *bool
}

func (store snapshotAccessInventoryV1) VisitAccessReceipts(ctx context.Context, visit func(domainpii.ControlledArtifactAccessReceiptV1) error) error {
	*store.active = true
	defer func() { *store.active = false }()
	return store.AccessInventoryStore.VisitAccessReceipts(ctx, visit)
}

type snapshotAccessInventoryV2 struct {
	piiport.AccessInventoryStoreV2
	active *bool
}

func (store snapshotAccessInventoryV2) VisitAccessReceiptsV2(ctx context.Context, visit func(domainpii.ControlledArtifactAccessReceiptV2) error) error {
	*store.active = true
	defer func() { *store.active = false }()
	return store.AccessInventoryStoreV2.VisitAccessReceiptsV2(ctx, visit)
}

type snapshotLedgerStore struct {
	publicationport.ClaimLedgerStore
	active *bool
}

func (store snapshotLedgerStore) Resolve(ctx context.Context, digest string) (domainpublication.ClaimLedgerV1, error) {
	if *store.active {
		return domainpublication.ClaimLedgerV1{}, errors.New("claim ledger resolver re-entered inventory CAS access")
	}
	return store.ClaimLedgerStore.Resolve(ctx, digest)
}

type snapshotProjectionStore struct {
	publicationport.PIIProjectionStore
	active *bool
}

func (store snapshotProjectionStore) Resolve(ctx context.Context, digest string) (domainpublication.PIIProjectionV1, error) {
	if *store.active {
		return domainpublication.PIIProjectionV1{}, errors.New("projection resolver re-entered inventory CAS access")
	}
	return store.PIIProjectionStore.Resolve(ctx, digest)
}

type snapshotGrantResolver struct {
	piiport.GrantResolver
	active *bool
}

func (store snapshotGrantResolver) ResolveGrant(ctx context.Context, digest string) (domainpii.PIIProjectionGrantV1, error) {
	if *store.active {
		return domainpii.PIIProjectionGrantV1{}, errors.New("grant resolver re-entered inventory CAS access")
	}
	return store.GrantResolver.ResolveGrant(ctx, digest)
}

type snapshotGrantStore struct {
	piiport.Store
	active *bool
}

func (store snapshotGrantStore) ResolveGrant(ctx context.Context, digest string) (domainpii.PIIProjectionGrantV1, error) {
	return (snapshotGrantResolver{store.Store, store.active}).ResolveGrant(ctx, digest)
}
