package pendingwork

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func childExecutionFixture(t *testing.T, tool string) (*serviceFixture, SideEffectIntentRequest, SideEffectIntentLease) {
	t.Helper()
	fixture, request := childProducerRequestFixture(t, tool)
	count := 1
	if tool == "parallel_tasks" {
		count = 2
	}
	plan, err := NewChildProducerPlanV1(request.Pending, request.SemanticIdentity, childProducerTargetsForTest(count), func(ctx context.Context) error { return ctx.Err() })
	if err != nil {
		t.Fatal(err)
	}
	request.ChildProducer = &plan
	lease, err := fixture.service.BeginSideEffectIntent(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return fixture, request, lease
}

func claimChildExecutionForTest(t *testing.T, fixture *serviceFixture, request SideEffectIntentRequest, lease SideEffectIntentLease) context.Context {
	t.Helper()
	if err := fixture.service.VerifySideEffectIntentAtSend(context.Background(), lease, request, request.IssuedAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	ctx, err := fixture.service.BindChildProducerExecutionV1(context.Background(), lease, request)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func TestChildProducerExecutionRequiresClaimedExactProcessLease(t *testing.T) {
	fixture, request, lease := childExecutionFixture(t, "task")
	target := childProducerTargetsForTest(1)[0]
	writes := 0
	produce := func() error { writes++; return nil }
	if _, err := fixture.service.BindChildProducerExecutionV1(context.Background(), lease, request); err == nil {
		t.Fatal("unclaimed parent became child authority")
	}
	if err := fixture.service.UseChildProducerV1(context.Background(), request.Pending, target, request.IssuedAt, produce); err == nil {
		t.Fatal("bare context reached child producer")
	}
	ctx := claimChildExecutionForTest(t, fixture, request, lease)
	foreign := NewService(fixture.authority, fixture.store, fixture.threads)
	if _, err := foreign.BindChildProducerExecutionV1(ctx, lease, request); err == nil {
		t.Fatal("old process lease rebound after service reconstruction")
	}
	if err := foreign.UseChildProducerV1(ctx, request.Pending, target, request.IssuedAt, produce); err == nil {
		t.Fatal("receipt reader recovered execution authority")
	}
	changed := target
	changed.ChildTurnID = "turn_999"
	if err := fixture.service.UseChildProducerV1(ctx, request.Pending, changed, request.IssuedAt, produce); err == nil {
		t.Fatal("different child target was executed")
	}
	pending := request.Pending
	pending.SecurityContext.UserID = "foreign-user"
	if err := fixture.service.UseChildProducerV1(ctx, pending, target, request.IssuedAt, produce); err == nil {
		t.Fatal("parent binding was transplanted")
	}
	if writes != 0 {
		t.Fatal("rejected child crossed first write")
	}
	if err := fixture.service.UseChildProducerV1(ctx, request.Pending, target, request.IssuedAt, produce); err != nil || writes != 1 {
		t.Fatalf("original exact child failed: %v", err)
	}
}

func TestChildProducerParallelSlotsAreOneShotAcrossContextsAndAmbiguousWrites(t *testing.T) {
	fixture, request, lease := childExecutionFixture(t, "parallel_tasks")
	ctx := claimChildExecutionForTest(t, fixture, request, lease)
	otherCtx, err := fixture.service.BindChildProducerExecutionV1(context.Background(), lease, request)
	if err != nil {
		t.Fatal(err)
	}
	before, err := fixture.service.TrustedInventoryV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Whole-plan reservation validation is no longer valid after a sibling
	// consumes its token. Each first-write callback owns its individual token.
	request.ChildProducer.revalidate = func(context.Context) error { return errors.New("synthetic consumed allocator token") }
	var writes atomic.Int32
	var wg sync.WaitGroup
	for index := 0; index < 12; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = fixture.service.UseChildProducerV1(ctx, request.Pending, childProducerTargetsForTest(2)[0], request.IssuedAt, func() error { writes.Add(1); return errors.New("synthetic lost write acknowledgement") })
		}()
	}
	wg.Wait()
	if writes.Load() != 1 {
		t.Fatalf("same ordinal wrote %d times", writes.Load())
	}
	if err := fixture.service.UseChildProducerV1(otherCtx, request.Pending, childProducerTargetsForTest(2)[0], request.IssuedAt, func() error { writes.Add(1); return nil }); !errors.Is(err, ErrSideEffectIntentAlreadyClaimed) {
		t.Fatalf("context copy retried ambiguous effect: %v", err)
	}
	if err := fixture.service.UseChildProducerV1(otherCtx, request.Pending, childProducerTargetsForTest(2)[1], request.IssuedAt, func() error { writes.Add(1); return nil }); err != nil {
		t.Fatal(err)
	}
	if writes.Load() != 2 {
		t.Fatal("independent original ordinal was not executed once")
	}
	after, err := fixture.service.TrustedInventoryV1(context.Background())
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("process execution claim rewrote original durable receipt")
	}
}

func TestChildProducerExecutionRechecksOpenReceiptCancellationAndHoldBeforeFirstWrite(t *testing.T) {
	for _, mode := range []string{"cancel", "expired", "closed", "tampered_vector", "held_parent", "held_child"} {
		t.Run(mode, func(t *testing.T) {
			fixture, request, lease := childExecutionFixture(t, "task")
			ctx := claimChildExecutionForTest(t, fixture, request, lease)
			target := childProducerTargetsForTest(1)[0]
			now := request.IssuedAt.Add(time.Second)
			switch mode {
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "expired":
				now = lease.expiresAt
			case "closed":
				fixture.addResult(request.Pending.ExecutionGrant, false, "synthetic settled parent", now)
				if _, err := fixture.service.CloseSideEffectIntentAfterSettlement(ctx, lease, request, now.Add(time.Second)); err != nil {
					t.Fatal(err)
				}
			case "tampered_vector":
				receipt, err := fixture.store.ReadReceipt(ctx, lease.workID)
				if err != nil {
					t.Fatal(err)
				}
				receipt.ChildProducer.Children[0].ChildTurnID = "turn_999"
				fixture.store.receipts[lease.workID] = receipt
			case "held_parent":
				fixture.service.restartPreserved.primaryDigests = map[string]string{request.Pending.ThreadID: "synthetic hold"}
			case "held_child":
				fixture.service.restartPreserved.primaryDigests = map[string]string{target.ChildThreadID: "synthetic hold"}
			}
			writes := 0
			if err := fixture.service.UseChildProducerV1(ctx, request.Pending, target, now, func() error { writes++; return nil }); err == nil {
				t.Fatal("stale child admission succeeded")
			}
			if writes != 0 || lease.claim.children[0].Load() {
				t.Fatal("rejection reached or consumed first-write callback")
			}
		})
	}
}

func TestChildProducerExecutionCannotDetachFromClaimedEffectCancellation(t *testing.T) {
	for _, mode := range []string{"without_cancel", "fresh_bind"} {
		t.Run(mode, func(t *testing.T) {
			fixture, request, lease := childExecutionFixture(t, "task")
			effectCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := fixture.service.VerifySideEffectIntentAtSend(effectCtx, lease, request, request.IssuedAt); err != nil {
				t.Fatal(err)
			}
			bound, err := fixture.service.BindChildProducerExecutionV1(effectCtx, lease, request)
			if err != nil {
				t.Fatal(err)
			}
			cancel()
			detached := context.WithoutCancel(bound)
			if mode == "fresh_bind" {
				detached, err = fixture.service.BindChildProducerExecutionV1(context.Background(), lease, request)
				if err != nil {
					return
				}
			}
			writes := 0
			if err := fixture.service.UseChildProducerV1(detached, request.Pending, childProducerTargetsForTest(1)[0], request.IssuedAt, func() error { writes++; return nil }); err == nil {
				t.Error("cancelled effect context was detached into child execution")
			}
			if writes != 0 {
				t.Error("cancelled original effect still performed first child write")
			}
		})
	}
}

func TestCancelledChildExecutionStillAllowsExactDetachedParentSettlement(t *testing.T) {
	fixture, request, lease := childExecutionFixture(t, "task")
	effectCtx, cancel := context.WithCancel(context.Background())
	if err := fixture.service.VerifySideEffectIntentAtSend(effectCtx, lease, request, request.IssuedAt); err != nil {
		cancel()
		t.Fatal(err)
	}
	bound, err := fixture.service.BindChildProducerExecutionV1(effectCtx, lease, request)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	fixture.addResult(request.Pending.ExecutionGrant, true, "synthetic cancelled parent outcome", request.IssuedAt.Add(time.Second))
	if _, err := fixture.service.CloseSideEffectIntentAfterSettlement(context.WithoutCancel(bound), lease, request, request.IssuedAt.Add(2*time.Second)); err != nil {
		t.Fatalf("cancelled child authority blocked exact durable parent settlement: %v", err)
	}
}
