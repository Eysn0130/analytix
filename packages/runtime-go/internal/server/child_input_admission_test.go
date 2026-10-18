package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	"analytix.local/runtime-go/internal/jobs"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

func TestHostChildInputKeepsProfileTokenBudgetOutOfForegroundAuthorityV1(t *testing.T) {
	for _, budget := range []int{512, 4097} {
		t.Run(fmt.Sprintf("budget_%d", budget), func(t *testing.T) {
			carrier := &hostChildInputCarrierV1{created: domainjob.Record{ID: "continued-child", TokenBudget: budget}}
			preparation := subagentapp.TaskRunPreparation{
				Request: subagentapp.TaskRequest{
					Prompt: "ordinary host follow-up", RunInBackground: true,
					TokenBudget: budget, TokenBudgetSet: true,
				},
				ToolScope: []string{"read_file"},
			}
			request := startRuntimeTurnRequest{InternalOutputTokenBudget: 128}
			carrier.configureLoopRequestV1(&request, preparation)
			if request.InternalOutputTokenBudget != 0 || request.InternalChildRunID != carrier.created.ID ||
				request.InternalSubagentDepth != 1 || !reflect.DeepEqual(request.InternalToolScope, preparation.ToolScope) {
				t.Fatalf("ordinary follow-up inherited foreground budget authority: %#v", request)
			}
			if preparation.Request.TokenBudget != budget || !preparation.Request.TokenBudgetSet || carrier.created.TokenBudget != budget {
				t.Fatal("host follow-up changed the soft request/record budget")
			}
		})
	}
}

func hostChildInputFixtureV1(t *testing.T) (*runtimeServerHandler, domainjob.Record, map[string]any) {
	t.Helper()
	h, pending, ctx, slot, _, releaseParent := runtimePreparedChildExecutionFixtureV1(t)
	original := h.runRuntimeSubagentTask(ctx, pending, slot.request)
	releaseParent()
	if original.IsError {
		t.Fatal("original signed child execution failed", original.Err)
	}
	record, err := h.jobs.LoadChildRun(original.Record.ID)
	if err != nil || record.Status != "completed" || record.SecurityBinding == nil {
		t.Fatal("original child has no completed signed producer binding", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := h.Shutdown(shutdownCtx); err != nil {
			t.Error(err)
		}
	})
	input := map[string]any{"mode": "start_or_steer", "threadId": record.ParentThreadID, "jobId": record.ID, "expectedRunId": record.ID, "expectedChildTurnId": record.ChildTurnID, "message": "continue existing child work", "clientMessageId": "a13a71c0-56a0-4ccb-8ac3-0a4f786a8312"}
	return h, record, input
}

func hostChildInputHTTPV1(t *testing.T, h *runtimeServerHandler, input map[string]any, authorized bool) (int, subagentapp.ChildInputResponseV1) {
	t.Helper()
	return hostChildInputHTTPContextV1(t, context.Background(), h, input, authorized)
}

func hostChildInputHTTPContextV1(t *testing.T, ctx context.Context, h *runtimeServerHandler, input map[string]any, authorized bool) (int, subagentapp.ChildInputResponseV1) {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/steer", bytes.NewReader(raw)).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	if authorized {
		r.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var body any
	if json.Unmarshal(w.Body.Bytes(), &body) != nil {
		t.Fatalf("HTTP %d returned invalid JSON", w.Code)
	}
	response, err := subagentapp.DecodeChildInputResponseV1(body)
	if err != nil {
		t.Fatalf("HTTP %d violates closed response: %v (%s)", w.Code, err, w.Body.String())
	}
	return w.Code, response
}

func TestHostChildInputActualRunSteerStopAndReplayV1(t *testing.T) {
	h, source, input := hostChildInputFixtureV1(t)
	parentBefore, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), source.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	oldPendingBefore, err := h.pendingWork.TrustedInventoryV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := hostChildInputHTTPV1(t, h, input, false); status != http.StatusForbidden {
		t.Fatal("body mode obtained Host authority", status)
	}
	if result := h.admitHostChildInputV1(context.Background(), subagentapp.ChildInputRequestV1{}); result.ErrorCode != subagentapp.TaskJobErrorForbidden {
		t.Fatal("direct context obtained Host authority")
	}
	inspect := map[string]any{"mode": "inspect", "threadId": source.ParentThreadID, "jobId": source.ID}
	if status, r := hostChildInputHTTPV1(t, h, inspect, true); status != 200 || r.Capabilities.Send != "start" || r.Receipt != nil || len(h.jobs.AllRecords()) != 1 {
		t.Fatal("inspection mutated or misreported idle child", status, r)
	}
	p := &asyncTurnTerminalGuardFailureProvider{started: make(chan struct{}), release: make(chan struct{}), failure: errors.New("synthetic child execution failure")}
	capture := &scheduleModelCaptureV1{p, make(chan []domainmodel.Message, 1)}
	h.provider = capture
	t.Cleanup(func() {
		select {
		case <-p.release:
		default:
			close(p.release)
		}
	})
	status, start := hostChildInputHTTPV1(t, h, input, true)
	if status != 200 || start.Receipt == nil || start.Receipt.Action != "start" || start.Receipt.Admission != "admitted" || start.Receipt.JobID == source.ID || start.Receipt.ChildTurnID == source.ChildTurnID || start.Receipt.ChildThreadID != source.ChildThreadID {
		t.Fatalf("actual start: %d %#v", status, start)
	}
	record, err := h.jobs.LoadChildRun(start.Receipt.JobID)
	if err != nil || record.HostChildInput == nil || record.HostChildInput.ClaimedAt == "" || record.ParentToolCallID != "" || record.SecurityBinding != nil || record.ContinueFrom != source.ID {
		t.Fatal("run has no real Host claim or borrowed parent authority", err)
	}
	select {
	case messages := <-capture.messages:
		raw, _ := json.Marshal(messages)
		if bytes.Contains(raw, []byte(domainjob.HostChildInputMarkerFieldV1)) || bytes.Contains(raw, []byte("clientMessageId")) {
			t.Fatal("private admission leaked to model")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("real worker did not reach synthetic provider")
	}
	if status, repeat := hostChildInputHTTPV1(t, h, input, true); status != 200 || !reflect.DeepEqual(start.Receipt, repeat.Receipt) || len(h.jobs.AllRecords()) != 2 {
		t.Fatal("start replay duplicated execution", status, repeat)
	}
	conflict := cloneMap(input)
	conflict["message"] = "different content"
	if status, _ := hostChildInputHTTPV1(t, h, conflict, true); status != 409 {
		t.Fatal("same key different input accepted", status)
	}
	steer := cloneMap(input)
	steer["expectedRunId"] = record.ID
	steer["expectedChildTurnId"] = record.ChildTurnID
	steer["message"] = "follow up while active"
	steer["clientMessageId"] = "5f9894ce-a5f6-46ed-b62e-2a2215931839"
	status, queued := hostChildInputHTTPV1(t, h, steer, true)
	if status != 200 || queued.Receipt == nil || queued.Receipt.Action != "steer" || queued.Receipt.Admission != "queued" || queued.Receipt.JobID != record.ID {
		t.Fatal("steering falsely admitted or created new run", status, queued)
	}
	child, err := h.store.GetThread(record.ChildThreadID)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := turnapp.FrozenSecurityContextForTurn(child, record.ChildTurnID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = h.store.PromotePendingSteeringEntriesForContext(record.ChildThreadID, record.ChildTurnID, frozen.ContextDigest); err != nil {
		t.Fatal(err)
	}
	if status, promoted := hostChildInputHTTPV1(t, h, steer, true); status != 200 || promoted.Receipt.Admission != "admitted" || promoted.Receipt.MessageID != queued.Receipt.MessageID {
		t.Fatal("signed promotion did not confirm same input", status, promoted)
	}
	cancelInput := cloneMap(steer)
	cancelInput["message"] = "queued before stop"
	cancelInput["clientMessageId"] = "590e89d4-14d0-4c42-a6a9-81a6bf8f58ba"
	if status, r := hostChildInputHTTPV1(t, h, cancelInput, true); status != 200 || r.Receipt.Admission != "queued" {
		t.Fatal("cancel fixture not queued", status, r)
	}
	kill := h.runtimeTaskJobHTTPService().KillTaskJob(source.ParentThreadID, subagentapp.TaskJobKillRequest{JobID: record.ID})
	if kill.IsError {
		t.Fatal("Host stop failed", kill.ErrorCode, kill.Err)
	}
	waitScheduleOperationsV1(t, h)
	if status, r := hostChildInputHTTPV1(t, h, cancelInput, true); status != 409 || r.Code != "input_cancelled" || r.Receipt != nil || len(h.jobs.AllRecords()) != 2 {
		t.Fatal("stopped queue revived", status, r)
	}
	if err = runtimeStartupRecoveryForTest(h).RecoverPendingDeliveries(); err != nil {
		t.Fatal(err)
	}
	ended, err := h.jobs.LoadChildRun(record.ID)
	if err != nil || ended.DeadLetterReason != "" || ended.CompletionDeliveryID != "" {
		t.Fatal("Host completion borrowed old parent settlement", err, ended.DeadLetterReason)
	}
	retained, err := h.jobs.LoadChildRun(source.ID)
	if err != nil || !reflect.DeepEqual(source, retained) {
		t.Fatal("new input changed old result/receipt", err)
	}
	parentAfter, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), source.ParentThreadID)
	if err != nil || !reflect.DeepEqual(parentBefore.Thread, parentAfter.Thread) {
		t.Fatal("Host input altered old parent tool/history", err)
	}
	oldPendingAfter, err := h.pendingWork.TrustedInventoryV1(context.Background())
	if err != nil || !reflect.DeepEqual(oldPendingBefore, oldPendingAfter) {
		t.Fatal("Host input settled old parent pending call", err)
	}
	resumed, err := jobs.NewManager(filepath.Join(h.dataDir, "child-runs"))
	if err != nil {
		t.Fatal(err)
	}
	restored, err := resumed.LoadChildRun(record.ID)
	if err != nil || restored.HostChildInput == nil || *restored.HostChildInput != *ended.HostChildInput || len(resumed.AllRecords()) != 2 {
		t.Fatal("restart lost exact claimed input", err)
	}
	if status, r := hostChildInputHTTPV1(t, h, input, true); status != 200 || !reflect.DeepEqual(start.Receipt, r.Receipt) || len(h.jobs.AllRecords()) != 2 {
		t.Fatal("terminal replay created a second run", status, r)
	}
}

func TestHostChildInputTailFailureCannotClaimOrRedispatchV1(t *testing.T) {
	h, _, input := hostChildInputFixtureV1(t)
	p := &asyncTurnTerminalGuardFailureProvider{started: make(chan struct{}), release: make(chan struct{}), failure: errors.New("synthetic")}
	h.provider = p
	h.store.beforeRecordEventHook = func(map[string]any) error { return errors.New("synthetic start tail failure") }
	status, r := hostChildInputHTTPV1(t, h, input, true)
	h.store.beforeRecordEventHook = nil
	if status != 503 || r.Receipt != nil || len(h.jobs.AllRecords()) != 2 {
		t.Fatal("tail failure invented admitted ownership", status, r)
	}
	for _, record := range h.jobs.AllRecords() {
		if record.HostChildInput != nil && record.HostChildInput.ClaimedAt != "" {
			t.Fatal("tail failure retained a pre-worker claim")
		}
	}
	select {
	case <-p.started:
		t.Fatal("unclaimed worker reached provider")
	default:
	}
	if status, r = hostChildInputHTTPV1(t, h, input, true); status != 503 || r.Receipt != nil || len(h.jobs.AllRecords()) != 2 {
		t.Fatal("unknown retry redispatched", status, r)
	}
}

type hostChildInputACKCutV1 struct {
	recoveryport.PrimaryThreadReaderV1
	clientID string
	cut      bool
}

func (c *hostChildInputACKCutV1) ReadPrimaryThreadSnapshotV1(ctx context.Context, id string) (recoveryport.PrimaryThreadSnapshotV1, error) {
	snapshot, err := c.PrimaryThreadReaderV1.ReadPrimaryThreadSnapshotV1(ctx, id)
	if err != nil {
		return snapshot, err
	}
	if !c.cut {
		for _, raw := range listAny(snapshot.Thread["turns"]) {
			turn, _ := raw.(map[string]any)
			for _, marker := range listAny(turn[domainjob.HostChildInputMarkerFieldV1]) {
				value, _ := marker.(map[string]any)
				if stringField(value, "clientMessageId") == c.clientID {
					c.cut = true
					return recoveryport.PrimaryThreadSnapshotV1{}, errors.New("synthetic committed ACK read cut")
				}
			}
		}
	}
	return snapshot, nil
}

func TestHostChildInputUnknownACKQueriesSameReceiptV1(t *testing.T) {
	h, source, input := hostChildInputFixtureV1(t)
	p := &asyncTurnTerminalGuardFailureProvider{started: make(chan struct{}), release: make(chan struct{}), failure: errors.New("synthetic")}
	h.provider = p
	t.Cleanup(func() {
		select {
		case <-p.release:
		default:
			close(p.release)
		}
	})
	status, start := hostChildInputHTTPV1(t, h, input, true)
	if status != 200 || start.Receipt == nil {
		t.Fatal("start fixture failed", status, start)
	}
	steer := cloneMap(input)
	steer["expectedRunId"] = start.Receipt.JobID
	steer["expectedChildTurnId"] = start.Receipt.ChildTurnID
	steer["clientMessageId"] = "f3e2c8bb-aa5b-49ef-ae69-a6a38ca13667"
	steer["message"] = "queued once across lost ACK"
	reader := h.store.primaryReader
	cut := &hostChildInputACKCutV1{PrimaryThreadReaderV1: reader, clientID: stringField(steer, "clientMessageId")}
	h.store.primaryReader = cut
	status, unknown := hostChildInputHTTPV1(t, h, steer, true)
	h.store.primaryReader = reader
	if !cut.cut || status != 503 || unknown.Receipt != nil {
		t.Fatal("lost ACK invented certainty", status, unknown, cut.cut)
	}
	status, confirmed := hostChildInputHTTPV1(t, h, steer, true)
	if status != 200 || confirmed.Receipt == nil || confirmed.Receipt.Admission != "queued" || len(h.jobs.AllRecords()) != 2 {
		t.Fatal("same-key query did not recover original queue", status, confirmed)
	}
	child, err := h.store.GetThread(source.ChildThreadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, ok := subagentapp.FindTurn(child, start.Receipt.ChildTurnID)
	if !ok || len(listAny(turn["steering"])) != 1 {
		t.Fatal("lost ACK duplicated steering")
	}
	close(p.release)
	if stopped, err := h.runtimeSubagentState().WaitBackgroundJobSettled(start.Receipt.JobID, 15*time.Second); !stopped || err != nil {
		t.Fatal("natural Host completion did not unregister", err)
	}

}

func TestHostChildInputColdPrimaryUsesExactTerminalOwnerV1(t *testing.T) {
	h, source, input := hostChildInputFixtureV1(t)
	p := &asyncTurnTerminalGuardFailureProvider{started: make(chan struct{}), release: make(chan struct{}), failure: errors.New("synthetic cold run")}
	h.provider = p
	status, first := hostChildInputHTTPV1(t, h, input, true)
	if status != 200 || first.Receipt == nil {
		t.Fatal("cold fixture start failed", status, first)
	}
	h.store.beforeTerminalWrite = func() error { return errors.New("synthetic terminal primary failure") }
	close(p.release)
	waitScheduleOperationsV1(t, h)
	if stopped, err := h.runtimeSubagentState().WaitBackgroundJobSettled(first.Receipt.JobID, 15*time.Second); !stopped || err != nil {
		t.Fatal("cold terminal owner did not unregister", err)
	}
	h.store.beforeTerminalWrite = nil
	child, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), source.ChildThreadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, ok := subagentapp.FindTurn(child.Thread, first.Receipt.ChildTurnID)
	if !ok || stringField(turn, "status") != "running" {
		t.Fatal("cold fixture primary was already terminal")
	}
	interrupted, err := h.jobs.CleanupStaleRunningRecords()
	if err != nil || len(interrupted) != 1 {
		t.Fatal("cold manager lifecycle did not close abandoned owner", err, len(interrupted))
	}
	if err = runtimeStartupRecoveryForTest(h).RecoverInterrupted(interrupted); err != nil {
		t.Fatal(err)
	}
	old, err := h.jobs.LoadChildRun(first.Receipt.JobID)
	if err != nil || old.Status != "interrupted" {
		t.Fatal("old cold record not interrupted", err)
	}
	next := cloneMap(input)
	next["expectedRunId"] = old.ID
	next["expectedChildTurnId"] = old.ChildTurnID
	next["clientMessageId"] = "c1ac110d-462d-4591-89cc-b16a4f5892d9"
	next["message"] = "continue after cold recovery"
	p2 := &asyncTurnTerminalGuardFailureProvider{started: make(chan struct{}), release: make(chan struct{}), failure: errors.New("synthetic new owner")}
	h.provider = p2
	t.Cleanup(func() {
		select {
		case <-p2.release:
		default:
			close(p2.release)
		}
	})
	status, started := hostChildInputHTTPV1(t, h, next, true)
	if status != 200 || started.Receipt == nil || started.Receipt.JobID == old.ID || started.Receipt.ChildTurnID == old.ChildTurnID || started.Receipt.ChildThreadID != old.ChildThreadID {
		t.Fatal("cold continuation did not obtain a fresh run", status, started)
	}
	preserved, err := h.jobs.LoadChildRun(old.ID)
	if err != nil || !reflect.DeepEqual(old, preserved) {
		t.Fatal("cold new run rewrote prior record", err)
	}
	close(p2.release)
	if stopped, err := h.runtimeSubagentState().WaitBackgroundJobSettled(started.Receipt.JobID, 15*time.Second); !stopped || err != nil {
		t.Fatal("fresh cold run did not unregister", err)
	}
}

func TestHostChildInputTerminalTailCapabilitiesRemainClosedV1(t *testing.T) {
	h, source, _ := hostChildInputFixtureV1(t)
	_, registered := h.runtimeSubagentState().RegisterBackgroundJobWithStartBarrier(source.ID, func() {})
	if !registered {
		t.Fatal("terminal tail control fixture unavailable")
	}
	defer h.runtimeSubagentState().UnregisterBackgroundJob(source.ID)
	status, response := hostChildInputHTTPV1(t, h, map[string]any{"mode": "inspect", "threadId": source.ParentThreadID, "jobId": source.ID}, true)
	if status != 200 || response.Capabilities.Send != "disabled" || !response.Capabilities.Stop || response.Capabilities.ReasonCode != "turn_terminalizing" || response.Receipt != nil {
		t.Fatal("terminal tail exposed an invalid start/stop combination", status, response)
	}
}

// Only the request's final readback carries this test key. Async claim and
// worker reads use their normal detached context, so this cuts ACK after handoff.
type hostChildInputStartACKKeyV1 struct{}
type hostChildInputStartACKCutV1 struct {
	recoveryport.PrimaryThreadReaderV1
	clientID string
	cut      bool
}

func (c *hostChildInputStartACKCutV1) ReadPrimaryThreadSnapshotV1(ctx context.Context, id string) (recoveryport.PrimaryThreadSnapshotV1, error) {
	snapshot, err := c.PrimaryThreadReaderV1.ReadPrimaryThreadSnapshotV1(ctx, id)
	if err != nil || ctx.Value(hostChildInputStartACKKeyV1{}) != true || c.cut {
		return snapshot, err
	}
	for _, raw := range listAny(snapshot.Thread["turns"]) {
		turn, _ := raw.(map[string]any)
		for _, rawMarker := range listAny(turn[domainjob.HostChildInputMarkerFieldV1]) {
			marker, _ := rawMarker.(map[string]any)
			if stringField(marker, "clientMessageId") == c.clientID {
				c.cut = true
				return recoveryport.PrimaryThreadSnapshotV1{}, errors.New("synthetic start ACK read cut")
			}
		}
	}
	return snapshot, nil
}

func TestHostChildInputStartACKLossQueriesOneActualRunV1(t *testing.T) {
	h, source, input := hostChildInputFixtureV1(t)
	p := &asyncTurnTerminalGuardFailureProvider{started: make(chan struct{}), release: make(chan struct{}), failure: errors.New("synthetic")}
	capture := &scheduleModelCaptureV1{p, make(chan []domainmodel.Message, 2)}
	h.provider = capture
	t.Cleanup(func() {
		select {
		case <-p.release:
		default:
			close(p.release)
		}
	})
	reader := h.store.primaryReader
	cut := &hostChildInputStartACKCutV1{PrimaryThreadReaderV1: reader, clientID: stringField(input, "clientMessageId")}
	h.store.primaryReader = cut
	status, unknown := hostChildInputHTTPContextV1(t, context.WithValue(context.Background(), hostChildInputStartACKKeyV1{}, true), h, input, true)
	h.store.primaryReader = reader
	if !cut.cut || status != 503 || unknown.Receipt != nil {
		t.Fatal("lost start ACK invented certainty", status, unknown, cut.cut)
	}
	status, confirmed := hostChildInputHTTPV1(t, h, input, true)
	if status != 200 || confirmed.Receipt == nil || confirmed.Receipt.Action != "start" || confirmed.Receipt.Admission != "admitted" || confirmed.Receipt.JobID == source.ID || len(h.jobs.AllRecords()) != 2 {
		t.Fatal("same-key lookup did not recover actual start", status, confirmed)
	}
	if status, repeated := hostChildInputHTTPV1(t, h, input, true); status != 200 || !reflect.DeepEqual(confirmed.Receipt, repeated.Receipt) {
		t.Fatal("start lookup changed receipt", status, repeated)
	}
	select {
	case <-capture.messages:
	case <-time.After(5 * time.Second):
		t.Fatal("actual worker did not reach provider")
	}
	select {
	case <-capture.messages:
		t.Fatal("ACK replay launched a second provider run")
	default:
	}
	child, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), source.ChildThreadID)
	if err != nil || len(listAny(child.Thread["turns"])) != 2 {
		t.Fatal("ACK replay appended another turn", err)
	}
	old, err := h.jobs.LoadChildRun(source.ID)
	if err != nil || !reflect.DeepEqual(source, old) {
		t.Fatal("ACK replay changed old result", err)
	}
	close(p.release)
	if settled, err := h.runtimeSubagentState().WaitBackgroundJobSettled(confirmed.Receipt.JobID, 15*time.Second); !settled || err != nil {
		t.Fatal("ACK fixture owner did not settle", err)
	}
}

func TestHostChildInputSidecarFailureKeepsOneActualOwnerV1(t *testing.T) {
	h, source, input := hostChildInputFixtureV1(t)
	metadata := h.store.metadataPath(source.ChildThreadID)
	backup := metadata + ".host-input-fixture-backup"
	if err := os.Rename(metadata, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(metadata, 0700); err != nil {
		t.Fatal(err)
	}
	restored := false
	restore := func() {
		if restored {
			return
		}
		if err := os.Remove(metadata); err != nil {
			t.Error(err)
			return
		}
		if err := os.Rename(backup, metadata); err != nil {
			t.Error(err)
			return
		}
		restored = true
	}
	t.Cleanup(restore)
	p := &asyncTurnTerminalGuardFailureProvider{started: make(chan struct{}), release: make(chan struct{}), failure: errors.New("synthetic")}
	h.provider = p
	t.Cleanup(func() {
		select {
		case <-p.release:
		default:
			close(p.release)
		}
	})
	status, admitted := hostChildInputHTTPV1(t, h, input, true)
	restore()
	if status != 200 || admitted.Receipt == nil || admitted.Receipt.Admission != "admitted" {
		t.Fatal("sidecar failure did not finish existing owner", status, admitted)
	}
	record, err := h.jobs.LoadChildRun(admitted.Receipt.JobID)
	if err != nil || record.HostChildInput == nil || record.HostChildInput.ClaimedAt == "" || !h.runtimeSubagentState().BackgroundJobStartedV1(record.ID) {
		t.Fatal("primary alone was reported as actual ownership", err)
	}
	child, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), source.ChildThreadID)
	if err != nil {
		t.Fatal(err)
	}
	request, err := subagentapp.ChildInputRequestFromArgsV1(input)
	if err != nil {
		t.Fatal(err)
	}
	receipt, _, err := h.store.hostChildInputReceiptFromPrimaryV1(child.Thread, request)
	if err != nil || !reflect.DeepEqual(receipt, admitted.Receipt) {
		t.Fatal("sidecar failure lost exact primary marker", err)
	}
	if status, repeated := hostChildInputHTTPV1(t, h, input, true); status != 200 || !reflect.DeepEqual(admitted.Receipt, repeated.Receipt) || len(h.jobs.AllRecords()) != 2 {
		t.Fatal("sidecar retry duplicated run", status, repeated)
	}
	old, err := h.jobs.LoadChildRun(source.ID)
	if err != nil || !reflect.DeepEqual(source, old) {
		t.Fatal("sidecar failure changed old result", err)
	}
	close(p.release)
	if settled, err := h.runtimeSubagentState().WaitBackgroundJobSettled(record.ID, 15*time.Second); !settled || err != nil {
		t.Fatal("sidecar fixture owner did not settle", err)
	}
}

// Owner-scope evidence: a durable claim plus a consumed start barrier is not
// proof of a launch. The HTTP tests above cover the actual fresh Host carrier.
func TestHostChildInputClaimWithoutLaunchRemainsUnknownOwnerV1(t *testing.T) {
	h, source, input := hostChildInputFixtureV1(t)
	request, err := subagentapp.ChildInputRequestFromArgsV1(input)
	if err != nil {
		t.Fatal(err)
	}
	identity := &domainjob.HostChildInputIdentityV1{SchemaVersion: 1, ClientMessageID: request.ClientMessageID, InputDigest: subagentapp.ChildInputDigestV1(request)}
	reservation, err := h.jobs.ReserveHostChildInputRunV1(context.Background(), source.ParentThreadID, source.ChildThreadID, "turn_owner_claim_cut", identity)
	if err != nil {
		t.Fatal(err)
	}
	record, err := h.jobs.StartReservedHostChildInputRunV1(context.Background(), domainjob.StartRequest{
		HostChildInput: identity, ParentGoalID: source.ParentGoalID, ParentThreadID: source.ParentThreadID, ParentTurnID: source.ParentTurnID,
		ChildThreadID: source.ChildThreadID, ChildTurnID: "turn_owner_claim_cut", Kind: "subagent", Name: "Host claim cut fixture", Prompt: request.Message,
		Status: "queued", Background: true, ContinueFrom: source.ID, SourceRef: source.ID,
		Workspace: source.Workspace, ProviderID: source.ProviderID, Model: source.Model,
		ToolScope: source.ToolScope, ToolSchemaHash: source.ToolSchemaHash, DelegatedToolManifest: source.DelegatedToolManifest,
	}, reservation)
	if err != nil {
		t.Fatal(err)
	}
	record, err = h.jobs.UpdateChildRun(record.ID, domainjob.UpdateRequest{Status: "running"})
	if err != nil {
		t.Fatal(err)
	}
	barrier, registered := h.runtimeSubagentState().RegisterBackgroundJobWithStartBarrier(record.ID, func() {})
	if !registered {
		t.Fatal("existing start owner registration failed")
	}
	defer h.runtimeSubagentState().UnregisterBackgroundJob(record.ID)
	cut := errors.New("synthetic callback failure after durable claim before launch")
	callbacks := 0
	err = barrier.StartIfActive(context.Background(), func() error {
		callbacks++
		claimed, claimErr := h.jobs.ClaimChildRunStart(record)
		if claimErr != nil {
			return claimErr
		}
		persisted, readErr := h.jobs.LoadChildRun(record.ID)
		if readErr != nil || claimed.HostChildInput.ClaimedAt == "" || !reflect.DeepEqual(claimed, persisted) {
			t.Fatal("cut did not follow durable claim", readErr)
		}
		return cut
	})
	if !errors.Is(err, cut) || callbacks != 1 || barrier.StartedV1() || h.runtimeSubagentState().BackgroundJobStartedV1(record.ID) {
		t.Fatal("failed launch reported an execution owner", err, callbacks)
	}
	if err := barrier.StartIfActive(context.Background(), func() error { callbacks++; return nil }); err == nil || callbacks != 1 {
		t.Fatal("failed callback consumed authority was reused", err, callbacks)
	}
	caps := subagentapp.ChildInputCapabilitiesV1{Send: "disabled", RunID: record.ID, ChildThreadID: record.ChildThreadID, ChildTurnID: record.ChildTurnID, ReasonCode: "child_input_unknown"}
	receipt := &subagentapp.ChildInputReceiptV1{Action: "start", Admission: "admitted", ClientMessageID: request.ClientMessageID, JobID: record.ID, ChildThreadID: record.ChildThreadID, ChildTurnID: record.ChildTurnID, MessageID: "item_owner_claim_cut_user"}
	result := h.confirmHostChildInputReceiptV1(context.Background(), request, caps, receipt)
	decoded, err := subagentapp.DecodeChildInputResponseV1(result.Response)
	if err != nil || !result.IsError || decoded.Receipt != nil || decoded.Code != "child_input_unknown" {
		t.Fatal("claim without launch invented an admitted receipt", err, decoded)
	}
	resumed, err := jobs.NewManager(filepath.Join(h.dataDir, "child-runs"))
	if err != nil {
		t.Fatal(err)
	}
	interrupted, err := resumed.CleanupStaleRunningRecords()
	if err != nil || len(interrupted) != 1 || interrupted[0].ID != record.ID || !interrupted[0].Orphaned || interrupted[0].HostChildInput.ClaimedAt == "" {
		t.Fatal("restart did not retain uncertain exact claim", err, interrupted)
	}
	old, err := resumed.LoadChildRun(source.ID)
	if err != nil || !reflect.DeepEqual(source, old) {
		t.Fatal("claim-cut recovery changed old result", err)
	}
}

// Owner-scope evidence: fault the real atomic writer inside its existing
// authorization callback. A directory collision causes rename failure, and
// restoring the exact fixture primary lets us inspect whether anything escaped.
func TestHostChildInputPrimaryWriteFailurePreservesOwnerSnapshotV1(t *testing.T) {
	h, source, input := hostChildInputFixtureV1(t)
	before, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), source.ChildThreadID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := cloneMap(before.Thread)
	candidate["title"] = "HOST_PRIMARY_WRITE_MUST_NOT_PERSIST"
	path := h.store.threadPath(source.ChildThreadID)
	backup := path + ".host-input-fixture-backup"
	writeReached := false
	h.store.mu.Lock()
	err = h.store.upsertThreadNoLockWithAtomicWriteAuthority(candidate, false, func(write func() error) error {
		if err := os.Rename(path, backup); err != nil {
			return err
		}
		if err := os.Mkdir(path, 0700); err != nil {
			_ = os.Rename(backup, path)
			return err
		}
		writeReached = true
		writeErr := write()
		restoreErr := os.Remove(path)
		if restoreErr == nil {
			restoreErr = os.Rename(backup, path)
		}
		return errors.Join(writeErr, restoreErr)
	})
	h.store.mu.Unlock()
	if !writeReached || err == nil {
		t.Fatal("primary failure fixture did not reach atomic write", err)
	}
	after, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), source.ChildThreadID)
	if err != nil || !reflect.DeepEqual(before.Thread, after.Thread) {
		t.Fatal("failed primary write altered source", err)
	}
	request, err := subagentapp.ChildInputRequestFromArgsV1(input)
	if err != nil {
		t.Fatal(err)
	}
	receipt, _, err := h.store.hostChildInputReceiptFromPrimaryV1(after.Thread, request)
	if err != nil || receipt != nil || len(h.jobs.AllRecords()) != 1 {
		t.Fatal("primary failure invented an input admission", err, receipt)
	}
	old, err := h.jobs.LoadChildRun(source.ID)
	if err != nil || !reflect.DeepEqual(source, old) {
		t.Fatal("primary failure changed old result", err)
	}
}
