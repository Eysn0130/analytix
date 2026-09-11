package pendingwork

import (
	"context"
	"testing"
	"time"

	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
)

func TestTrustedPendingWorkInventoryPinsInstallationAndExactDispositionGraph(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, "read_inventory", true, "not_required", fixture.now)
	receipt, err := fixture.service.Issue(context.Background(), fixture.issueInput(
		domainpendingwork.KindToolBatch, []GrantReference{{GrantID: grant.GrantID}}, "trusted-inventory",
	))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.Close(
		context.Background(), receipt.WorkID, fixture.securityContext,
		domainpendingwork.StatusFailed, "batch_failed", fixture.now.Add(3*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	if err := VerifyTrustedInventoryV1(context.Background(), fixture.store, fixture.authority); err != nil {
		t.Fatalf("current installation pending-work inventory was rejected: %v", err)
	}

	foreign := newMemoryPendingWorkAuthority(t)
	if err := VerifyTrustedInventoryV1(context.Background(), fixture.store, foreign); err == nil {
		t.Fatal("foreign installation authority accepted pending-work inventory")
	}

	fixture.store.mu.Lock()
	delete(fixture.store.receipts, receipt.WorkID)
	fixture.store.mu.Unlock()
	if err := VerifyTrustedInventoryV1(context.Background(), fixture.store, fixture.authority); err == nil {
		t.Fatal("orphan pending-work disposition acquired authority without its receipt")
	}
}
