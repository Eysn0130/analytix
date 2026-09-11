package jobs

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	jobsecuritytest "analytix.local/runtime-go/internal/testsupport/jobsecurity"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func childReservationRequestForTest(t *testing.T) StartRequest {
	t.Helper()
	now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	parent, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_durable_1", TurnID: "turn_1", WorkspaceRealPath: t.TempDir(), ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	callID := jobsTestHostToolCallID("child-reservation")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: parent, Provider: "synthetic", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: callID,
		ArgsHash: domainsecurity.SHA256Hex([]byte("synthetic child args")), SchemaHash: domainsecurity.SHA256Hex([]byte("synthetic schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("synthetic scope")), ReadOnly: false, ApprovalState: "not_required", IssuedAt: now,
	})
	binding, err := domainjob.NewSecurityBinding(parent, grant, callID)
	if err != nil {
		t.Fatal(err)
	}
	request := StartRequest{ParentGoalID: "goal-synthetic", ParentThreadID: parent.ThreadID, ParentTurnID: parent.TurnID,
		ParentToolCallID: callID, SecurityBinding: binding, Kind: "subagent", Status: "queued"}
	jobsecuritytest.BindDelegatedToolManifestRequest(t, &request)
	return request
}

func TestChildReservationsArePureAndShareOrdinaryCounter(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request := childReservationRequestForTest(t)
	before, err := BuildChildRunInventoryV1(manager.root)
	if err != nil {
		t.Fatal(err)
	}
	const count = 12
	reservations := make([]ChildRunReservationV1, count)
	errorsByIndex := make([]error, count)
	var group sync.WaitGroup
	for i := range count {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			reservations[i], errorsByIndex[i] = manager.ReserveChildRunV1(context.Background(), request.SecurityBinding, uint32(i+1), "thr_durable_21", fmt.Sprintf("turn_%d", i+31))
		}(i)
	}
	group.Wait()
	seen := map[string]bool{}
	for i, reservation := range reservations {
		if errorsByIndex[i] != nil {
			t.Fatal(errorsByIndex[i])
		}
		target := reservation.TargetV1()
		if seen[target.JobID] || target.Ordinal != uint32(i+1) {
			t.Fatal("concurrent reservations reused an identity or ordinal")
		}
		seen[target.JobID] = true
		if err := manager.RevalidateChildRunReservationV1(context.Background(), reservation); err != nil {
			t.Fatal(err)
		}
	}
	if err := ValidateChildRunInventoryV1(manager.root, before); err != nil {
		t.Fatalf("pure reservation wrote job inventory: %v", err)
	}
	ordinary, err := manager.Start("goal-independent", "thread-independent", "shell")
	if err != nil || ordinary.ID != "job-13" {
		t.Fatalf("ordinary allocator reused an unwritten reservation: id=%s err=%v", ordinary.ID, err)
	}
	for i := count - 1; i >= 0; i-- {
		childRequest := request
		childRequest.ParallelIndex = i + 1
		record, err := manager.StartReservedChildRunV1(context.Background(), childRequest, reservations[i])
		target := reservations[i].TargetV1()
		if err != nil || record.ID != target.JobID || record.ChildThreadID != target.ChildThreadID || record.ChildTurnID != target.ChildTurnID {
			t.Fatalf("first queued record differs from reserved identities: err=%v", err)
		}
	}
	if manager.seq != count+1 {
		t.Fatal("consuming reservations allocated another identity")
	}
}

func TestChildReservationRejectsForeignReplayAndCallerReplacement(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request := childReservationRequestForTest(t)
	reservation, err := manager.ReserveChildRunV1(context.Background(), request.SecurityBinding, 1, "thr_durable_21", "turn_31")
	if err != nil {
		t.Fatal(err)
	}
	before, _ := BuildChildRunInventoryV1(manager.root)
	for _, mutate := range []func(*StartRequest){
		func(r *StartRequest) { r.ChildThreadID = "thr_durable_99" },
		func(r *StartRequest) { r.ChildTurnID = "turn_99" },
		func(r *StartRequest) { r.ParallelIndex = 2 },
		func(r *StartRequest) { r.SecurityBinding = nil },
	} {
		changed := request
		mutate(&changed)
		if _, err := manager.StartReservedChildRunV1(context.Background(), changed, reservation); err == nil {
			t.Fatal("caller replacement consumed a reserved child")
		}
	}
	if _, err := manager.StartReservedChildRunV1(context.Background(), request, ChildRunReservationV1{}); err == nil {
		t.Fatal("empty token selected a job identity")
	}
	restarted, err := NewManager(manager.root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.StartReservedChildRunV1(context.Background(), request, reservation); err == nil {
		t.Fatal("process-local reservation became restart execution authority")
	}
	if err := ValidateChildRunInventoryV1(manager.root, before); err != nil {
		t.Fatal("rejected reservation wrote job inventory")
	}
	_, err = manager.StartReservedChildRunV1(context.Background(), request, reservation)
	if err != nil {
		t.Fatal(err)
	}
	committed, _ := BuildChildRunInventoryV1(manager.root)
	if _, err := manager.StartReservedChildRunV1(context.Background(), request, reservation); err == nil {
		t.Fatal("consumed reservation issued a second queued job")
	}
	if err := manager.RevalidateChildRunReservationV1(context.Background(), reservation); err == nil {
		t.Fatal("consumed reservation remains signable")
	}
	if err := ValidateChildRunInventoryV1(manager.root, committed); err != nil {
		t.Fatal("reservation replay changed durable state")
	}
}

func TestChildReservationHoldPhaseCancellationAndOverflow(t *testing.T) {
	request := childReservationRequestForTest(t)
	for _, held := range []bool{false, true} {
		manager, err := NewManager(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		before, _ := BuildChildRunInventoryV1(manager.root)
		if held {
			if err := manager.PreserveRestartScopeV1(context.Background(), []string{request.ParentThreadID}, nil); err != nil {
				t.Fatal(err)
			}
		}
		_, err = manager.ReserveChildRunV1(context.Background(), request.SecurityBinding, 1, "thr_durable_21", "turn_31")
		if held && err == nil {
			t.Fatal("held parent obtained a new child reservation")
		}
		if !held && err != nil {
			t.Fatal(err)
		}
		if !held && manager.PreserveRestartScopeV1(context.Background(), []string{request.ParentThreadID}, nil) == nil {
			t.Fatal("hold installed after host child preparation entered its admission phase")
		}
		if err := ValidateChildRunInventoryV1(manager.root, before); err != nil {
			t.Fatal("reservation/hold phase wrote inventory")
		}
	}
	manager, _ := NewManager(t.TempDir())
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.ReserveChildRunV1(cancelled, request.SecurityBinding, 1, "thr_durable_21", "turn_31"); err == nil || manager.seq != 0 {
		t.Fatal("cancelled preparation allocated an identity")
	}
	manager.seq = maxChildRunSequence
	before, _ := BuildChildRunInventoryV1(manager.root)
	if _, err := manager.ReserveChildRunV1(context.Background(), request.SecurityBinding, 1, "thr_durable_21", "turn_31"); err == nil {
		t.Fatal("child reservation overflow wrapped the counter")
	}
	if _, err := manager.Start("goal-independent", "thread-independent", "shell"); err == nil {
		t.Fatal("ordinary allocation overflow wrapped the counter")
	}
	after, err := BuildChildRunInventoryV1(manager.root)
	if err != nil || !reflect.DeepEqual(before, after) || manager.seq != maxChildRunSequence {
		t.Fatal("counter exhaustion mutated identity state")
	}
}

func TestReservedChildFirstRecordMustBeQueuedSubagent(t *testing.T) {
	for _, changed := range []struct{ kind, status string }{{"subagent", "running"}, {"subagent", ""}, {"child-run", "queued"}} {
		t.Run(changed.kind+"/"+changed.status, func(t *testing.T) {
			manager, err := NewManager(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			request := childReservationRequestForTest(t)
			reservation, err := manager.ReserveChildRunV1(context.Background(), request.SecurityBinding, 1, "thr_durable_21", "turn_31")
			if err != nil {
				t.Fatal(err)
			}
			before, _ := BuildChildRunInventoryV1(manager.root)
			request.Kind, request.Status = changed.kind, changed.status
			if _, err := manager.StartReservedChildRunV1(context.Background(), request, reservation); err == nil {
				t.Error("reserved child skipped its required queued subagent producer state")
			}
			if err := ValidateChildRunInventoryV1(manager.root, before); err != nil {
				t.Error("invalid first child state crossed the job writer")
			}
		})
	}
}

type cancelAfterReservationCheckContext struct {
	context.Context
	cancel context.CancelFunc
	checks int
}

func (ctx *cancelAfterReservationCheckContext) Err() error {
	err := ctx.Context.Err()
	ctx.checks++
	if ctx.checks == 2 {
		ctx.cancel()
	}
	return err
}

func TestChildReservationRechecksCancellationAfterInventoryObservation(t *testing.T) {
	for _, revalidate := range []bool{false, true} {
		manager, err := NewManager(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		request := childReservationRequestForTest(t)
		var reservation ChildRunReservationV1
		if revalidate {
			reservation, err = manager.ReserveChildRunV1(context.Background(), request.SecurityBinding, 1, "thr_durable_21", "turn_31")
			if err != nil {
				t.Fatal(err)
			}
		}
		beforeSeq := manager.seq
		inner, cancel := context.WithCancel(context.Background())
		ctx := &cancelAfterReservationCheckContext{Context: inner, cancel: cancel}
		if revalidate {
			err = manager.RevalidateChildRunReservationV1(ctx, reservation)
		} else {
			_, err = manager.ReserveChildRunV1(ctx, request.SecurityBinding, 1, "thr_durable_21", "turn_31")
		}
		cancel()
		if err == nil {
			t.Errorf("revalidate=%t accepted allocation after cancellation crossed the inventory seam", revalidate)
		}
		if manager.seq != beforeSeq {
			t.Error("cancelled inventory observation allocated another identity")
		}
	}
}
