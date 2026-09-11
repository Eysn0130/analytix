package reportpublication

import (
	"context"
	"errors"
	"testing"

	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

func TestReportRestartReleasesAttemptsVisitorBeforeResolvingDecisionMaterials(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	if _, err := fixture.service.Publish(context.Background(), fixture.input); err != nil {
		t.Fatal(err)
	}
	active := false
	journal := &reportRestartJournalStub{intent: &fixture.coordinator.intent, settlement: &fixture.coordinator.settlement}
	plan, err := PreflightRestartV1(context.Background(), RestartPreflightConfigV1{
		Pending:  pendingworkapp.TrustedInventoryV1{Receipts: []domainpendingwork.PendingWorkReceiptV1{fixture.stages.receipt}},
		Attempts: snapshotAttemptInventory{fixture.attempts, &active}, Receipts: fixture.receipts, Indexes: fixture.indexes,
		Commits: fixture.commits, Selections: fixture.selections, Decisions: fixture.decisions,
		GrantSettlements: fixture.grantSettlements, StageCompletions: fixture.stageCompletions, DeliveryOutcomes: fixture.delivery,
		Threads: fixture.threads, Ledgers: snapshotRestartLedgerResolver{fixture.ledgers, &active},
		Projections: fixture.projections, Inspections: fixture.inspections,
		Intents: journal, Settlements: journal, Authority: fixture.authority,
	})
	if err != nil || len(plan.Attempts) != 1 || plan.Attempts[0].State != RestartAttemptDeliveryDecisionV1 {
		t.Fatalf("snapshot restart did not validate the exact committed decision: %v", err)
	}
}

type snapshotAttemptInventory struct {
	publicationport.AttemptInventoryStore
	active *bool
}

func (store snapshotAttemptInventory) VisitAttempts(ctx context.Context, visit func(domainpublication.PublicationAttemptV1) error) error {
	*store.active = true
	defer func() { *store.active = false }()
	return store.AttemptInventoryStore.VisitAttempts(ctx, visit)
}

type snapshotRestartLedgerResolver struct {
	restartClaimLedgerResolverV1
	active *bool
}

func (store snapshotRestartLedgerResolver) Resolve(ctx context.Context, digest string) (domainpublication.ClaimLedgerV1, error) {
	if *store.active {
		return domainpublication.ClaimLedgerV1{}, errors.New("restart ledger resolver re-entered attempts CAS access")
	}
	return store.restartClaimLedgerResolverV1.Resolve(ctx, digest)
}
