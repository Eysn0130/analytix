package server

import (
	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	controlapp "analytix.local/runtime-go/internal/app/control"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	admission "analytix.local/runtime-go/internal/domain/scheduleadmission"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
	"analytix.local/runtime-go/internal/testsupport/toolidentity"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func scheduleSeamFixtureV1(t *testing.T) (*runtimeServerHandler, admission.RequestV1) {
	t.Helper()
	h := NewRuntimeServerHandler(RuntimeServerConfig{RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(), ProviderID: "schedule-fixture", BaseURL: "https://provider.invalid", APIKey: "test-key", Model: "original-model", EndpointFormat: "chat_completions"}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, h)
	workspace := workspacetest.New(t)
	thread, e := h.store.CreateThread(map[string]any{"title": "Schedule fixture", "workspace": workspace, "providerId": "schedule-fixture", "model": "original-model"}, workspace)
	if e != nil {
		t.Fatal(e)
	}
	p := &asyncTurnTerminalGuardFailureProvider{started: make(chan struct{}), release: make(chan struct{}), failure: errors.New("synthetic completed origin")}
	h.provider = p
	response, e := h.runtimeControl().SendTurn(context.Background(), controlapp.StartTurnRequest{ThreadID: stringField(thread, "id"), Prompt: "create a scheduled message", Async: true})
	if e != nil {
		t.Fatal(e)
	}
	close(p.release)
	waitScheduleOperationsV1(t, h)
	r := admission.RequestV1{SchemaVersion: 1, Kind: "submit", TaskID: "task-private", Origin: admission.OriginV1{ThreadID: stringField(thread, "id"), TurnID: stringField(response, "turnId"), ToolCallID: toolidentity.MustHostToolCallIDV1("trusted-main-create"), GrantID: "historical-grant"}, AttemptID: "a8b6850e-10e3-4a54-a80c-140899708b67", ScheduledAt: "2026-10-12T01:00:00Z", Prompt: "scheduled ordinary delivery", Execution: admission.ExecutionV1{}}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = h.Shutdown(ctx)
	})
	return h, r
}
func waitScheduleOperationsV1(t *testing.T, h *runtimeServerHandler) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if e := h.runtimeControl().WaitForTurnOperations(ctx); e != nil {
		t.Fatal(e)
	}
}
func scheduleHTTPV1(t *testing.T, h *runtimeServerHandler, r admission.RequestV1) admission.ResponseV1 {
	t.Helper()
	b, _ := json.Marshal(r)
	req := httptest.NewRequest(http.MethodPost, httpapi.ScheduleAdmissionPathV1, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	req.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
	w := httptest.NewRecorder()
	httpapi.LocalDisplayMuxV1{RuntimeToken: DefaultRuntimeToken, ScheduleAdmissions: h.ScheduleAdmissionV1()}.ServeHTTP(w, req)
	var response admission.ResponseV1
	if json.Unmarshal(w.Body.Bytes(), &response) != nil {
		t.Fatalf("invalid private response: %d", w.Code)
	}
	if response.HTTPStatusV1() != w.Code {
		t.Fatalf("wire mismatch: %d %#v", w.Code, response)
	}
	return response
}

type scheduleModelCaptureV1 struct {
	*asyncTurnTerminalGuardFailureProvider
	messages chan []domainmodel.Message
}

func (p *scheduleModelCaptureV1) Stream(ctx context.Context, r domainmodel.Request) (domainmodel.Result, error) {
	p.messages <- r.Messages
	return p.asyncTurnTerminalGuardFailureProvider.Stream(ctx, r)
}
func TestScheduleAdmissionActualStartBusyReplayAndPrivateProjectionV1(t *testing.T) {
	h, r := scheduleSeamFixtureV1(t)
	p := &asyncTurnTerminalGuardFailureProvider{started: make(chan struct{}), release: make(chan struct{}), failure: errors.New("synthetic schedule execution failure")}
	capture := &scheduleModelCaptureV1{p, make(chan []domainmodel.Message, 1)}
	h.provider = capture
	t.Cleanup(func() {
		select {
		case <-p.release:
		default:
			close(p.release)
		}
	})
	override := "override-model"
	r.Execution.Model = &override
	start := scheduleHTTPV1(t, h, r)
	if start.Status != "accepted" || start.Receipt == nil || start.Receipt.Delivery != "start" || start.Receipt.ThreadID != r.Origin.ThreadID || start.Receipt.MessageID == r.AttemptID || !strings.HasPrefix(start.Receipt.MessageID, "item_") {
		t.Fatalf("actual start admission %#v", start)
	}
	select {
	case messages := <-capture.messages:
		visible, _ := json.Marshal(messages)
		if bytes.Contains(visible, []byte(admission.MarkerFieldV1)) || bytes.Contains(visible, []byte(r.Origin.GrantID)) || bytes.Contains(visible, []byte(r.TaskID)) {
			t.Fatal("private receipt leaked to model")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("synthetic provider did not receive admitted messages")
	}
	thread, e := h.store.GetThread(r.Origin.ThreadID)
	if e != nil || stringField(thread, "model") != "original-model" || stringField(thread, "providerId") != "schedule-fixture" {
		t.Fatal("Schedule changed persistent selections", e)
	}
	again := scheduleHTTPV1(t, h, r)
	if !reflect.DeepEqual(start, again) {
		t.Fatal("repeat lost immutable receipt")
	}
	busy := r
	busy.AttemptID = "834b2739-07d7-4bb9-87b6-58cfe24b547c"
	busy.Execution = admission.ExecutionV1{}
	busy.Prompt = "busy scheduled delivery"
	steer := scheduleHTTPV1(t, h, busy)
	if steer.Status != "accepted" || steer.Receipt.Delivery != "steer" || steer.Receipt.TurnID != start.Receipt.TurnID || !strings.HasPrefix(steer.Receipt.MessageID, "item_steer_") {
		h.store.mu.Lock()
		_, receipt, readErr := h.store.readScheduleAdmissionNoLockV1(context.Background(), busy)
		h.store.mu.Unlock()
		t.Fatalf("actual steering admission %#v; primaryReceipt=%t read=%v", steer, receipt != nil, readErr)
	}
	if repeat := scheduleHTTPV1(t, h, busy); !reflect.DeepEqual(repeat, steer) {
		t.Fatal("busy repeat changed receipt")
	}
	turn := lastScheduleTurnV1(t, h, r.Origin.ThreadID)
	frozen, parseErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if _, _, e = h.store.PromotePendingSteeringEntriesForContext(r.Origin.ThreadID, steer.Receipt.TurnID, frozen.ContextDigest); e != nil {
		t.Fatal(e)
	}
	if repeated := scheduleHTTPV1(t, h, busy); !reflect.DeepEqual(repeated, steer) {
		t.Fatal("promoted receipt changed")
	}
	cancelled := busy
	cancelled.AttemptID = "b1a81647-63fd-48c4-8b18-ff156c60b423"
	cancelled.Prompt = "queued until terminal"
	cancelledReceipt := scheduleHTTPV1(t, h, cancelled)
	if cancelledReceipt.Status != "accepted" {
		t.Fatal(cancelledReceipt.Status)
	}
	conflict := busy
	conflict.Prompt = "changed frozen prompt"
	if got := scheduleHTTPV1(t, h, conflict); got.Status != "conflict" {
		t.Fatalf("identity reused: %#v", got)
	}
	mismatch := busy
	mismatch.AttemptID = "fd797878-4050-4204-a93b-fb73a06ba7c2"
	mismatch.Execution.Model = new(string)
	*mismatch.Execution.Model = "different-model"
	if got := scheduleHTTPV1(t, h, mismatch); got.Status != "rejected" {
		t.Fatalf("busy override ignored: %#v", got)
	}
	snap, e := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), r.Origin.ThreadID)
	if e != nil {
		t.Fatal(e)
	}
	public, e := threadapp.ProjectPublicThread(snap.Thread)
	if e != nil {
		t.Fatal(e)
	}
	encoded, _ := json.Marshal(public)
	if bytes.Contains(encoded, []byte(admission.MarkerFieldV1)) || bytes.Contains(encoded, []byte(r.TaskID)) || bytes.Contains(encoded, []byte(r.Origin.GrantID)) {
		t.Fatal("private receipt leaked to GET projection")
	}
	events, e := h.store.LoadEventsSince(r.Origin.ThreadID, 0)
	if e != nil {
		t.Fatal(e)
	}
	encoded, _ = json.Marshal(events.Events)
	if bytes.Contains(encoded, []byte(admission.MarkerFieldV1)) || bytes.Contains(encoded, []byte(r.Origin.GrantID)) {
		t.Fatal("private receipt leaked to SSE")
	}
	close(p.release)
	waitScheduleOperationsV1(t, h)
	// Execution failure/terminal cancellation does not erase admission truth.
	busy.Kind = "query"
	after := scheduleHTTPV1(t, h, busy)
	if !reflect.DeepEqual(after, steer) {
		t.Fatalf("cancelled admission lost: %#v", after)
	}
	cancelled.Kind = "query"
	if got := scheduleHTTPV1(t, h, cancelled); !reflect.DeepEqual(got, cancelledReceipt) {
		t.Fatal("terminal cancellation erased acceptance")
	}
	// Reopened primary and the original installation-backed signed steering owner
	// suffice; no process-local dedup map, sidecar or task database is consulted.
	reopened, e := NewTempDurableEventSessionStore(h.store.root)
	if e != nil {
		t.Fatal(e)
	}
	reader, e := finalauthority.NewAcceptedFinalCASReader(h.store.root)
	if e != nil {
		t.Fatal(e)
	}
	if e = reopened.BindPrimaryThreadReaderV1(reader); e != nil {
		t.Fatal(e)
	}
	reopened.SetSteeringAuthority(h.store.steeringAuthority)
	reopened.mu.Lock()
	_, receipt, e := reopened.readScheduleAdmissionNoLockV1(context.Background(), busy)
	reopened.mu.Unlock()
	if e != nil || !reflect.DeepEqual(receipt, steer.Receipt) {
		t.Fatal("restart receipt changed", e)
	}
}

type scheduleReadCutV1 struct {
	recoveryport.PrimaryThreadReaderV1
	reads int
	cutAt int
}

func (c *scheduleReadCutV1) ReadPrimaryThreadSnapshotV1(ctx context.Context, id string) (recoveryport.PrimaryThreadSnapshotV1, error) {
	c.reads++
	if c.reads == c.cutAt {
		return recoveryport.PrimaryThreadSnapshotV1{}, errors.New("synthetic ACK read cut")
	}
	return c.PrimaryThreadReaderV1.ReadPrimaryThreadSnapshotV1(ctx, id)
}
func TestScheduleAdmissionUnknownACKQueriesOldIdentityAndStrictPrimaryV1(t *testing.T) {
	h, r := scheduleSeamFixtureV1(t)
	p := &asyncTurnTerminalGuardFailureProvider{started: make(chan struct{}), release: make(chan struct{}), failure: errors.New("synthetic")}
	h.provider = p
	t.Cleanup(func() {
		select {
		case <-p.release:
		default:
			close(p.release)
		}
	})
	first := scheduleHTTPV1(t, h, r)
	if first.Status != "accepted" {
		t.Fatal(first.Status)
	}
	r.AttemptID = "834b2739-07d7-4bb9-87b6-58cfe24b547c"
	original := h.store.primaryReader
	cut := &scheduleReadCutV1{PrimaryThreadReaderV1: original, cutAt: 4}
	h.store.primaryReader = cut
	unknown := scheduleHTTPV1(t, h, r)
	h.store.primaryReader = original
	if unknown.Status != "unknown" || unknown.Receipt != nil {
		t.Fatalf("uncertain read invented result: %#v reads=%d", unknown, cut.reads)
	}
	r.Kind = "query"
	receipt := scheduleHTTPV1(t, h, r)
	if receipt.Status != "accepted" || receipt.Receipt.Delivery != "steer" {
		t.Fatalf("old exact query lost primary receipt: %#v", receipt)
	}
	snap, e := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), r.Origin.ThreadID)
	if e != nil {
		t.Fatal(e)
	}
	turns, _ := snap.Thread["turns"].([]any)
	last, _ := turns[len(turns)-1].(map[string]any)
	if len(listAny(last["steering"])) != 1 {
		t.Fatal("unknown dispatch repeated")
	}
	close(p.release)
	waitScheduleOperationsV1(t, h)
	path := h.store.threadPath(r.Origin.ThreadID)
	body, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(path); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = os.WriteFile(path, body, 0600) })
	missing := scheduleHTTPV1(t, h, r)
	if missing.Status != "unknown" {
		t.Fatalf("sidecar became receipt authority: %#v", missing)
	}
	if e = os.WriteFile(path, body, 0600); e != nil {
		t.Fatal(e)
	}
}

func lastScheduleTurnV1(t *testing.T, h *runtimeServerHandler, id string) map[string]any {
	t.Helper()
	snap, e := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), id)
	if e != nil {
		t.Fatal(e)
	}
	turns, _ := snap.Thread["turns"].([]any)
	turn, _ := turns[len(turns)-1].(map[string]any)
	return turn
}

// The ordinary durable/public history contract already accepts terminal legacy
// string steering. Schedule must neither discard it nor treat it as a receipt.
func TestScheduleAdmissionLegacyTerminalSteeringPreservedWithoutMarkerV1(t *testing.T) {
	h, r := scheduleSeamFixtureV1(t)
	const legacy = "legacy terminal steering text"
	setScheduleFixtureSteeringV1(t, h, r.Origin.ThreadID, r.Origin.TurnID, []any{legacy})
	query := r
	query.Kind = "query"
	if got := scheduleHTTPV1(t, h, query); got.Status != "not_found" {
		t.Fatalf("legacy history blocked new occurrence query: %#v", got)
	}
	accepted := scheduleHTTPV1(t, h, r)
	if accepted.Status != "accepted" {
		t.Fatalf("legacy history blocked actual admission: %#v", accepted)
	}
	waitScheduleOperationsV1(t, h)
	if got := scheduleHTTPV1(t, h, query); !reflect.DeepEqual(got, accepted) {
		t.Fatalf("legacy history blocked exact receipt replay: %#v", got)
	}
	snap, e := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), r.Origin.ThreadID)
	if e != nil {
		t.Fatal(e)
	}
	turns, _ := snap.Thread["turns"].([]any)
	origin, _ := turns[0].(map[string]any)
	if !reflect.DeepEqual(origin["steering"], []any{legacy}) {
		t.Fatal("legacy source history was rewritten")
	}
	// Compatibility cannot legitimize a string in a Schedule-marked turn,
	// even after execution has become terminal.
	setScheduleFixtureSteeringV1(t, h, r.Origin.ThreadID, accepted.Receipt.TurnID, []any{legacy})
	if got := scheduleHTTPV1(t, h, query); got.Status != "unknown" || got.Receipt != nil {
		t.Fatalf("marked string acquired receipt authority: %#v", got)
	}
}
func setScheduleFixtureSteeringV1(t *testing.T, h *runtimeServerHandler, threadID, turnID string, entries []any) {
	t.Helper()
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	thread, e := h.store.readThreadNoLock(threadID)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, raw := range listAny(thread["turns"]) {
		turn, _ := raw.(map[string]any)
		if stringField(turn, "id") == turnID {
			turn["steering"] = entries
			found = true
			break
		}
	}
	if !found {
		t.Fatal("isolated fixture turn not found")
	}
	if e = h.store.upsertThreadNoLock(thread, false); e != nil {
		t.Fatal(e)
	}
}
func TestScheduleAdmissionRejectsRoundedNonV1SteeringVersionV1(t *testing.T) {
	h, r := scheduleSeamFixtureV1(t)
	p := &asyncTurnTerminalGuardFailureProvider{started: make(chan struct{}), release: make(chan struct{}), failure: errors.New("synthetic")}
	h.provider = p
	t.Cleanup(func() {
		select {
		case <-p.release:
		default:
			close(p.release)
		}
	})
	if got := scheduleHTTPV1(t, h, r); got.Status != "accepted" {
		t.Fatal(got.Status)
	}
	// Provider is quiescent before the fixture-only primary mutation.
	select {
	case <-p.started:
	case <-time.After(15 * time.Second):
		t.Fatal("synthetic provider did not block")
	}
	r.AttemptID = "834b2739-07d7-4bb9-87b6-58cfe24b547c"
	admitted := scheduleHTTPV1(t, h, r)
	if admitted.Status != "accepted" {
		t.Fatal(admitted.Status)
	}
	r.Kind = "query"
	path := h.store.threadPath(r.Origin.ThreadID)
	original, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	restored := false
	t.Cleanup(func() {
		if !restored {
			_ = os.WriteFile(path, original, 0600)
		}
	})
	snap, e := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), r.Origin.ThreadID)
	if e != nil {
		t.Fatal(e)
	}
	turns, _ := snap.Thread["turns"].([]any)
	turn, _ := turns[len(turns)-1].(map[string]any)
	entries, _ := turn["steering"].([]any)
	if len(entries) != 1 {
		t.Fatal("isolated signed entry missing")
	}
	entry, _ := entries[0].(map[string]any)
	if version, ok := entry["projectionVersion"].(json.Number); !ok || version.String() != "1" {
		t.Fatal("fixture is not a strict native V1 primary")
	}
	// Keep the authentic admission signature. The old lossy clone would round
	// this non-integer to 1 and accept the existing signature for the V1 payload.
	entry["projectionVersion"] = json.Number("1.0000000000000000001")
	injected, e := json.Marshal(snap.Thread)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, injected, 0600); e != nil {
		t.Fatal(e)
	}
	got := scheduleHTTPV1(t, h, r)
	if got.Status != "unknown" || got.Receipt != nil {
		t.Fatalf("non-V1 version acquired receipt authority: %#v", got)
	}
	after, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(after, injected) {
		t.Fatal("query rewrote malformed primary", e)
	}
	if e = os.WriteFile(path, original, 0600); e != nil {
		t.Fatal(e)
	}
	restored = true
	if got := scheduleHTTPV1(t, h, r); !reflect.DeepEqual(got, admitted) {
		t.Fatal("normal signed pending replay changed")
	}
	close(p.release)
	waitScheduleOperationsV1(t, h)
}
