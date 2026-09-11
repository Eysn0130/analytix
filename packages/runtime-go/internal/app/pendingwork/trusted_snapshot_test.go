package pendingwork

import (
	"context"
	"reflect"
	"testing"
)

func TestPreparedTrustedSnapshotUsesCompleteLiveVerifierAndImmutableCopies(t *testing.T) {
	fixture, request := childProducerRequestFixture(t, "parallel_tasks")
	plan, err := NewChildProducerPlanV1(request.Pending, request.SemanticIdentity, childProducerTargetsForTest(2), func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	request.ChildProducer = &plan
	if _, err := fixture.service.BeginSideEffectIntent(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	receipts, dispositions, err := fixture.store.SnapshotInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	beforeSigns := fixture.authority.SignCount()
	trusted, err := VerifyTrustedSnapshotV1(context.Background(), receipts, dispositions, fixture.authority)
	if err != nil || len(trusted.Receipts) != 1 {
		t.Fatalf("prepared inventory verification failed: %v", err)
	}
	live, err := fixture.service.TrustedInventoryV1(context.Background())
	if err != nil || !reflect.DeepEqual(live, trusted) {
		t.Fatal("prepared snapshot uses different authority checks")
	}
	receipts[0].ChildProducer.Children[0].ChildTurnID = "turn_999"
	if !reflect.DeepEqual(live, trusted) {
		t.Fatal("input alias changed trusted prepared inventory")
	}
	if _, err := VerifyTrustedSnapshotV1(context.Background(), receipts, dispositions, fixture.authority); err == nil {
		t.Fatal("changed signed child allocation was trusted")
	}
	if _, err := VerifyTrustedSnapshotV1(context.Background(), live.Receipts, dispositions, newMemoryPendingWorkAuthority(t)); err == nil {
		t.Fatal("foreign installation accepted prepared allocation")
	}
	if fixture.authority.SignCount() != beforeSigns {
		t.Fatal("read-only verification re-signed prepared inventory")
	}
}
