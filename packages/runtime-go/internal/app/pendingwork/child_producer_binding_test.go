package pendingwork

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
)

// The ordinary parent effect lease is insufficient to name a child's durable
// scope. A child-producing call must bind its complete host allocation before
// receiving authority that can reach the queued job or child thread producer.
func TestChildProducerIntentRejectsMissingHostAllocationBeforeReceipt(t *testing.T) {
	for _, tool := range []string{"task", "delegate_task", "parallel_tasks", "run_skill"} {
		t.Run(tool, func(t *testing.T) {
			fixture := newServiceFixture(t)
			grant := fixture.addGrant(t, tool, false, "not_required", fixture.now)
			request := sideEffectIntentRequestForTest(fixture.pendingCall(grant), fixture.now.Add(time.Second))
			_, err := fixture.service.BeginSideEffectIntent(context.Background(), request)
			if err == nil {
				t.Error("child producer received an effect lease without a host-bound child allocation")
			}
			receipts, readErr := fixture.store.ListReceipts(context.Background())
			if readErr != nil || len(receipts) != 0 {
				t.Errorf("missing child allocation crossed receipt write: count=%d err=%v", len(receipts), readErr)
			}
		})
	}
}

func childProducerRequestFixture(t *testing.T, tool string) (*serviceFixture, SideEffectIntentRequest) {
	t.Helper()
	fixture := newServiceFixture(t)
	seed := fixture.addGrant(t, "read", true, "not_required", fixture.now)
	args := map[string]any{"prompt": "synthetic child"}
	if tool == "parallel_tasks" {
		args = map[string]any{"tasks": []any{map[string]any{"prompt": "synthetic first"}, map[string]any{"prompt": "synthetic second"}}}
	}
	grant := addEquivalentSideEffectGrant(t, fixture, seed, tool, "child-binding", args, fixture.now.Add(time.Second))
	request := sideEffectIntentRequestForTest(fixture.pendingCall(grant), fixture.now.Add(2*time.Second))
	return fixture, request
}

func childProducerTargetsForTest(count int) []domainpendingwork.ChildProducerTargetV1 {
	all := []domainpendingwork.ChildProducerTargetV1{
		{Ordinal: 1, JobID: "job-11", ChildThreadID: "thr_durable_21", ChildTurnID: "turn_31"},
		{Ordinal: 2, JobID: "job-12", ChildThreadID: "thr_durable_21", ChildTurnID: "turn_32"},
	}
	return all[:count]
}

func TestChildProducerLeaseRequiresExactImmutableHostPlan(t *testing.T) {
	for _, tool := range []string{"task", "delegate_task", "parallel_tasks", "run_skill"} {
		t.Run(tool, func(t *testing.T) {
			fixture, request := childProducerRequestFixture(t, tool)
			count := 1
			if tool == "parallel_tasks" {
				count = 2
			}
			targets := childProducerTargetsForTest(count)
			revalidations, allocationLive := 0, true
			plan, err := NewChildProducerPlanV1(request.Pending, request.SemanticIdentity, targets, func(ctx context.Context) error {
				revalidations++
				if !allocationLive {
					return errors.New("synthetic reservation is no longer live")
				}
				return ctx.Err()
			})
			if err != nil {
				t.Fatal(err)
			}
			request.ChildProducer = &plan
			targets[0].JobID = "job-99"
			lease, err := fixture.service.BeginSideEffectIntent(context.Background(), request)
			if err != nil || revalidations != 1 {
				t.Fatalf("host preparation did not issue once: err=%v revalidations=%d", err, revalidations)
			}
			receipt, err := fixture.store.ReadReceipt(context.Background(), lease.WorkID())
			if err != nil || receipt.ChildProducer.Children[0].JobID != "job-11" {
				t.Fatal("input alias changed exact receipt allocation")
			}
			before := domainpendingwork.CloneChildProducerV1(receipt.ChildProducer)
			changedTargets := childProducerTargetsForTest(count)
			changedTargets[0].ChildTurnID = "turn_99"
			changedPlan, err := NewChildProducerPlanV1(request.Pending, request.SemanticIdentity, changedTargets, func(context.Context) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			changed := request
			changed.ChildProducer = &changedPlan
			if err := fixture.service.VerifySideEffectIntentAtSend(context.Background(), lease, changed, request.IssuedAt.Add(time.Second)); err == nil {
				t.Fatal("different child turn consumed the original send claim")
			}
			if err := fixture.service.VerifySideEffectIntentAtSend(context.Background(), lease, request, request.IssuedAt.Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if err := fixture.service.VerifySideEffectIntentAtSend(context.Background(), lease, request, request.IssuedAt.Add(time.Second)); !errors.Is(err, ErrSideEffectIntentAlreadyClaimed) {
				t.Fatalf("child claim replay was not denied: %v", err)
			}
			if _, err := fixture.service.BeginSideEffectIntent(context.Background(), changed); !errors.Is(err, ErrWorkAlreadyOpen) {
				t.Fatalf("changed allocation reminted a semantic effect lease: %v", err)
			}
			inventory, err := fixture.service.TrustedInventoryV1(context.Background())
			if err != nil || len(inventory.Receipts) != 1 {
				t.Fatal("child receipt inventory lost original authority")
			}
			inventory.Receipts[0].ChildProducer.Children[0].JobID = "job-99"
			reread, err := fixture.service.TrustedInventoryV1(context.Background())
			if err != nil || !reflect.DeepEqual(reread.Receipts[0].ChildProducer, before) {
				t.Fatal("inventory reader mutated signed child allocation")
			}
			fixture.addResult(request.Pending.ExecutionGrant, false, "synthetic parent tool settlement", request.IssuedAt.Add(2*time.Second))
			allocationLive = false // Consumed child reservations are not required to settle the parent tool.
			if _, err := fixture.service.CloseSideEffectIntentAfterSettlement(context.Background(), lease, request, request.IssuedAt.Add(3*time.Second)); err != nil {
				t.Fatal(err)
			}
			restarted := NewService(fixture.authority, fixture.store, fixture.threads)
			closed, err := restarted.TrustedInventoryV1(context.Background())
			if err != nil || len(closed.Receipts) != 1 || !reflect.DeepEqual(closed.Receipts[0].ChildProducer, before) {
				t.Fatal("closed receipt lost the complete child reservation vector")
			}
			if _, err := restarted.BeginSideEffectIntent(context.Background(), changed); !errors.Is(err, ErrGrantAuthority) {
				t.Fatalf("settled parent grant retained child execution authority: %v", err)
			}
			freshGrant := addEquivalentSideEffectGrant(t, fixture, request.Pending.ExecutionGrant, tool, "retry-after-close",
				fixture.grantArguments[request.Pending.ExecutionGrant.GrantID], request.IssuedAt.Add(4*time.Second))
			fresh := sideEffectIntentRequestForTest(fixture.pendingCall(freshGrant), request.IssuedAt.Add(5*time.Second))
			fresh.SemanticIdentity = request.SemanticIdentity
			freshPlan, err := NewChildProducerPlanV1(fresh.Pending, fresh.SemanticIdentity, changedTargets, func(context.Context) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			fresh.ChildProducer = &freshPlan
			if _, err := restarted.BeginSideEffectIntent(context.Background(), fresh); !errors.Is(err, ErrWorkClosed) {
				t.Fatalf("fresh equivalent grant bypassed closed child semantic CAS: %v", err)
			}
		})
	}
}

func TestChildProducerPlanRejectsMissingPartialForeignOrStaleAllocation(t *testing.T) {
	fixture, request := childProducerRequestFixture(t, "parallel_tasks")
	for _, targets := range [][]domainpendingwork.ChildProducerTargetV1{nil, childProducerTargetsForTest(1)} {
		if _, err := NewChildProducerPlanV1(request.Pending, request.SemanticIdentity, targets, func(context.Context) error { return nil }); err == nil {
			t.Fatal("incomplete parallel child denominator was accepted")
		}
	}
	if _, err := NewChildProducerPlanV1(request.Pending, request.SemanticIdentity, childProducerTargetsForTest(2), nil); err == nil {
		t.Fatal("unowned caller vector became a host plan")
	}
	plan, err := NewChildProducerPlanV1(request.Pending, request.SemanticIdentity, childProducerTargetsForTest(2), func(context.Context) error { return errors.New("stale allocator reservation") })
	if err != nil {
		t.Fatal(err)
	}
	request.ChildProducer = &plan
	if _, err := fixture.service.BeginSideEffectIntent(context.Background(), request); err == nil {
		t.Fatal("stale host allocation reached receipt issuance")
	}
	assertNoPendingWorkAuthorityEffect(t, fixture, 0)
	plan.revalidate = func(context.Context) error { return nil }
	foreign := request
	foreign.Pending.SecurityContext.UserID = "foreign-principal"
	if _, err := fixture.service.BeginSideEffectIntent(context.Background(), foreign); err == nil {
		t.Fatal("foreign principal used a prepared child allocation")
	}
	assertNoPendingWorkAuthorityEffect(t, fixture, 0)
	lease, err := fixture.service.BeginSideEffectIntent(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	plan.revalidate = func(context.Context) error { return errors.New("reservation changed after durable receipt") }
	if err := fixture.service.VerifySideEffectIntentAtSend(context.Background(), lease, request, request.IssuedAt.Add(time.Second)); err == nil {
		t.Fatal("stale reservation crossed the send boundary")
	}
	if lease.claim.claimed.Load() {
		t.Fatal("failed allocation verification consumed an executable send claim")
	}
}

func TestLegacyReceiptNeverGainsChildProducerAuthorityOrReplacement(t *testing.T) {
	fixture, request := childProducerRequestFixture(t, "task")
	plan, err := NewChildProducerPlanV1(request.Pending, request.SemanticIdentity, childProducerTargetsForTest(1), func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	request.ChildProducer = &plan
	authority, err := sideEffectIntentSemanticAuthority(request)
	if err != nil {
		t.Fatal(err)
	}
	payloadHash, err := fixture.service.KeyedPayloadHash(context.Background(), sideEffectIntentPayloadPurpose, authority.payload)
	if err != nil {
		t.Fatal(err)
	}
	// Construct historical bytes directly with the old field-free domain shape;
	// this fixture does not invoke the new live admission API.
	legacy, err := fixture.service.prepareIssue(context.Background(), IssueInput{
		Kind: domainpendingwork.KindSideEffectIntent, SecurityContext: request.Pending.SecurityContext,
		GrantReferences: []GrantReference{{GrantID: request.Pending.ExecutionGrant.GrantID}},
		PayloadHash:     payloadHash, RouteHash: authority.routeHash, IssuedAt: request.IssuedAt, ExpiresAt: authority.expiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.PutReceiptIfAbsent(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.BeginSideEffectIntent(context.Background(), request); !errors.Is(err, ErrWorkAlreadyOpen) {
		t.Fatalf("legacy witness gap gained new executable child authority: %v", err)
	}
	after, err := fixture.store.ReadReceipt(context.Background(), legacy.WorkID)
	if err != nil || !reflect.DeepEqual(legacy, after) || after.ChildProducer != nil {
		t.Fatal("legacy record was replaced or backfilled")
	}
}

func TestChildProducerCannotUseLegacyApprovedDispatchLease(t *testing.T) {
	for _, tool := range []string{"task", "delegate_task", "parallel_tasks", "run_skill"} {
		t.Run(tool+"/begin", func(t *testing.T) {
			fixture, request := approvedDispatchFixtureForTool(t, tool)
			if _, err := fixture.service.BeginApprovedToolDispatch(context.Background(), request); err == nil {
				t.Error("child producer bypassed host allocation through legacy approved dispatch")
			}
			assertNoPendingWorkAuthorityEffect(t, fixture, 0)
		})
		t.Run(tool+"/old_send_and_settlement", func(t *testing.T) {
			fixture, request := approvedDispatchFixtureForTool(t, tool)
			authority, err := approvedToolDispatchSemanticAuthority(request)
			if err != nil {
				t.Fatal(err)
			}
			_, registry, err := fixture.service.currentAuthority(authority.securityContext)
			if err != nil {
				t.Fatal(err)
			}
			payloadHash, err := fixture.service.KeyedPayloadHash(context.Background(), approvedToolDispatchPayloadPurpose, authority.payload)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := fixture.service.issueExclusive(context.Background(), IssueInput{
				Kind: domainpendingwork.KindApprovedToolDispatch, SecurityContext: authority.securityContext,
				GrantReferences: []GrantReference{{GrantID: authority.grant.GrantID}}, ExpectedRegistryDigest: registry.StateDigest,
				PayloadHash: payloadHash, RouteHash: authority.routeHash, IssuedAt: authority.issuedAt, ExpiresAt: authority.expiresAt,
			})
			if err != nil {
				t.Fatal(err)
			}
			lease := ApprovedToolDispatchLease{
				workID: receipt.WorkID, securityContext: authority.securityContext, grant: authority.grant,
				transition: authority.transition, payload: append([]byte(nil), authority.payload...), routeHash: authority.routeHash,
				registryDigest: registry.StateDigest, issuedAt: authority.issuedAt, expiresAt: authority.expiresAt,
				claim: &approvedToolDispatchClaim{},
			}
			signs := fixture.authority.SignCount()
			if err := fixture.service.VerifyApprovedToolDispatchAtSend(context.Background(), lease, request, request.IssuedAt.Add(time.Second)); err == nil {
				t.Error("old field-free child lease retained send authority")
			}
			if lease.claim.claimed.Load() || fixture.authority.SignCount() != signs {
				t.Error("old child send crossed claim or signer boundary")
			}
			stored, err := fixture.store.ReadReceipt(context.Background(), receipt.WorkID)
			if err != nil || !reflect.DeepEqual(stored, receipt) {
				t.Fatal("old child send rejection changed receipt authority")
			}
			// A historical call that was already sent still has its original
			// settlement contract; this does not permit a fresh send or retry.
			lease.claim.claimed.Store(true)
			fixture.addResult(request.Pending.ExecutionGrant, false, "synthetic historical settled child call", request.IssuedAt.Add(2*time.Second))
			if _, err := fixture.service.CloseApprovedToolDispatchAfterSettlement(context.Background(), lease, request, request.IssuedAt.Add(3*time.Second)); err != nil {
				t.Fatalf("legacy sent call lost exact durable settlement: %v", err)
			}
		})
	}
}
