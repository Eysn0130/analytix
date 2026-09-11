//go:build darwin || linux

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	apploop "analytix.local/runtime-go/internal/app/loop"
	turnapp "analytix.local/runtime-go/internal/app/turn"
)

func TestGeneralTerminalDiagnosticsPreserveErrorChainAndClosedProjection(t *testing.T) {
	cause := &os.PathError{Op: "append", Path: "/private/diagnostic-secret-sentinel", Err: os.ErrPermission}
	err := turnapp.WithGeneralTerminalDetailV1(cause, turnapp.GeneralTerminalDetailAppendV1)
	err = turnapp.WithGeneralTerminalDetailV1(err, turnapp.GeneralTerminalDetailOutboxV1)
	public := apploop.WrapHostBoundaryFailure(apploop.HostCandidatePublicationFailure, err)
	var pathError *os.PathError
	if !errors.Is(public, cause) || !errors.Is(public, os.ErrPermission) || !errors.As(public, &pathError) || pathError != cause {
		t.Fatal("diagnostic wrapper lost errors.Is/errors.As identity")
	}
	if turnapp.GeneralTerminalDetailClassV1(public) != "outbox_append" || asyncTurnErrorClassV1(public) != "host_candidate_publication_failed" || asyncTurnErrorClassV1(err) != "unclassified" {
		t.Fatal("private detail changed the public error class or lost the inner stage")
	}
	for _, text := range []string{err.Error(), public.Error(), turnapp.GeneralTerminalDetailClassV1(public)} {
		if strings.Contains(text, "diagnostic-secret-sentinel") || strings.Contains(text, "/private/") {
			t.Fatal("diagnostic projection exposed a private cause")
		}
	}
	unknown := errors.New("outbox_append /private/diagnostic-secret-sentinel")
	if turnapp.GeneralTerminalDetailClassV1(unknown) != "unknown" || turnapp.GeneralTerminalDetailClassV1(nil) != "none" || turnapp.WithGeneralTerminalDetailV1(nil, "unsafe") != nil {
		t.Fatal("untyped errors or success were misclassified")
	}
	unsafe := turnapp.WithGeneralTerminalDetailV1(unknown, unknown.Error())
	if unsafe.Error() != "general terminal failure: unknown" || !errors.Is(unsafe, unknown) {
		t.Fatal("unrecognized detail was not closed while preserving its cause")
	}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		if asyncTurnErrorClassV1(turnapp.WithGeneralTerminalDetailV1(cause, "terminal_finish")) != asyncTurnErrorClassV1(cause) {
			t.Fatal("wrapper changed cancellation/deadline classification")
		}
	}
}

func TestGeneralTerminalDiagnosticsFinishStage(t *testing.T) {
	store, threadID, turnID, security := newGeneralTerminalOutboxFixture(t)
	cause := errors.New("/private/terminal-secret-sentinel")
	store.beforeTerminalWrite = func() error { return cause }
	result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: store, SecurityContext: security, ThreadID: threadID, TurnID: turnID,
		CreatedAt: "2026-07-15T00:00:00Z", FinishedAt: "2026-07-15T00:00:00Z",
	})
	if result.Changed || !errors.Is(err, cause) || turnapp.GeneralTerminalDetailClassV1(err) != "terminal_finish" || strings.Contains(err.Error(), "secret-sentinel") {
		t.Fatal("terminal finish failure lost its closed stage or original cause")
	}
	store.beforeTerminalWrite = nil
}

func TestGeneralTerminalDiagnosticsAsyncCutsAndRecovery(t *testing.T) {
	for _, cut := range []string{"none", "append", "usage", "append_then_usage"} {
		t.Run(cut, func(t *testing.T) {
			handler, provider, threadID, turnID := newAsyncTerminalClosureFixtureV1(t, nil)
			cause := errors.New("append /private/diagnostic-secret-sentinel")
			usagePath := handler.store.usageIndex.ThreadPath(threadID)
			blockedUsage := cut == "usage" || cut == "append_then_usage"
			if blockedUsage {
				if err := os.MkdirAll(usagePath, 0700); err != nil {
					t.Fatal("could not install synthetic usage fault")
				}
			}
			attempts := 0
			if cut == "append" || cut == "append_then_usage" {
				handler.store.generalTerminalAtomicAppend = func(id string, events []map[string]any) error {
					attempts++
					if cut == "append" || attempts == 1 {
						return cause
					}
					return handler.store.eventLog.AppendEventsAtomic(id, events)
				}
			}
			restore := func() {
				handler.store.generalTerminalAtomicAppend = nil
				if blockedUsage {
					if err := os.Remove(usagePath); err != nil {
						t.Fatal("could not remove synthetic usage fault")
					}
					blockedUsage = false
				}
			}
			defer restore()
			stderr := captureRuntimeStderrForTest(t, func() {
				close(provider.release)
				waitAsyncTerminalClosureV1(t, handler)
			})
			started, finished := <-provider.observations, <-provider.observations
			completion, fallback := "none", "none"
			if cut == "append" || cut == "append_then_usage" {
				completion = "outbox_append"
			}
			if cut == "usage" {
				completion = "outbox_usage_settle"
			}
			if cut != "none" {
				// The fallback requests failed after the completed CAS winner.
				// Replay validation rejects that status before another outbox call.
				fallback = "terminal_finish"
			}
			if started.Stage != "started" || finished.Stage != "finished" || finished.CompletionDetailClass != completion || finished.FailureRecordDetailClass != fallback || finished.TerminalStatus != "completed" {
				t.Fatalf("closed async detail mismatch: completion=%s fallback=%s status=%s", finished.CompletionDetailClass, finished.FailureRecordDetailClass, finished.TerminalStatus)
			}
			if cut != "none" {
				if finished.CompletionErrorClass != "host_candidate_publication_failed" || finished.FailureRecordErrorClass != "unclassified" || stderr != "[analytix] event=ANALYTIX_RUNTIME_ASYNC_TURN_FAILURE_RECORD_FAILED\n" {
					t.Fatal("diagnostic changed existing public classes or fixed stderr")
				}
			} else if finished.CompletionErrorClass != "none" || finished.FailureRecordErrorClass != "none" || stderr != "" {
				t.Fatal("success acquired an error or diagnostic marker")
			}
			observed, _ := json.Marshal(finished)
			if strings.Contains(string(observed)+stderr, "diagnostic-secret-sentinel") || strings.Contains(string(observed)+stderr, usagePath) {
				t.Fatal("observer or stderr exposed a private cause")
			}
			if cut == "append" || cut == "append_then_usage" {
				if attempts != 1 {
					t.Fatal("diagnostic changed completion/fallback append attempt count")
				}
			}
			if cut == "append_then_usage" {
				// An explicit settlement retry now gets past append, but must
				// expose the still-blocked usage stage without changing the CAS.
				_, err := handler.store.RecordGeneralTerminalEventBundle(threadID, turnID)
				var pathError *os.PathError
				if turnapp.GeneralTerminalDetailClassV1(err) != "outbox_usage_settle" || !errors.As(err, &pathError) || strings.Contains(err.Error(), usagePath) {
					t.Fatal("settlement retry lost its distinct usage failure or leaked its cause")
				}
			}
			before, err := handler.store.GetThread(threadID)
			if err != nil {
				t.Fatal("committed terminal is unreadable")
			}
			if cut != "none" {
				request := httptest.NewRequest(http.MethodPost, "/v1/threads/"+threadID+"/turns", strings.NewReader(`{"prompt":"continue ordinary work","async":true}`))
				request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if response.Code < 400 || strings.Contains(response.Body.String(), "secret-sentinel") || strings.Contains(response.Body.String(), usagePath) || strings.Contains(response.Body.String(), "outbox_") {
					t.Fatal("public projection admitted unresolved terminal or exposed private diagnostics")
				}
			}
			restore()
			for range 2 {
				if _, err := handler.store.RecordGeneralTerminalEventBundle(threadID, turnID); err != nil {
					t.Fatal("restored exact-once settlement failed")
				}
			}
			after, err := handler.store.GetThread(threadID)
			if err != nil || !reflect.DeepEqual(before["turns"], after["turns"]) {
				t.Fatal("recovery changed the existing terminal CAS")
			}
			if err := handler.Shutdown(context.Background()); err != nil {
				t.Fatal("fixture shutdown failed")
			}
			reopened, err := NewTempDurableEventSessionStore(handler.store.root)
			if err != nil {
				t.Fatal("fixture reopen failed")
			}
			if err := turnapp.RecoverGeneralTerminalPublicationsAtStartupV1(context.Background(), reopened); err != nil {
				t.Fatal("startup recovery failed")
			}
			replay, err := reopened.LoadEventsSince(threadID, 0)
			if err != nil {
				t.Fatal("recovered replay failed")
			}
			bundle := terminalBundleEventsForClosureV1(replay.Events)
			if len(bundle) != 3 {
				t.Fatal("recovery lost exact terminal bundle")
			}
			assertGeneralTerminalEventIDsExactlyOnce(t, bundle)
			assertGeneralTerminalUsageIndexExactlyOnce(t, reopened, threadID, bundle)
			t.Logf("cut=%s completion_detail=%s failure_record_detail=%s terminal=completed recovery=exact_once privacy=pass", cut, completion, fallback)
		})
	}
}

func TestGeneralTerminalSettlementDuringPublicUsageRead(t *testing.T) {
	handler, provider, threadID, turnID := newAsyncTerminalClosureFixtureV1(t, nil)
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var readerTicket atomic.Bool
	handler.store.beforeUsageIndexThreadHook = func(id string) {
		if id == threadID && readerTicket.CompareAndSwap(false, true) {
			close(entered)
			select {
			case <-release:
			case <-time.After(10 * time.Second):
				t.Error("public usage reader barrier expired")
			}
		}
	}
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
		select {
		case <-finished:
		case <-time.After(10 * time.Second):
			t.Error("public usage reader did not drain")
		}
	}()
	request := httptest.NewRequest(http.MethodGet, "/v1/threads/"+threadID, nil)
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	response := httptest.NewRecorder()
	go func() { defer close(finished); handler.ServeHTTP(response, request) }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("public thread read did not enter usage rebuild")
	}
	stderr := captureRuntimeStderrForTest(t, func() {
		close(provider.release)
		waitAsyncTerminalClosureV1(t, handler)
	})
	for _, stage := range []string{"started", "finished"} {
		select {
		case observation := <-provider.observations:
			if observation.Stage != stage {
				t.Errorf("unexpected asynchronous observation stage: %s", observation.Stage)
			}
			if stage == "finished" && (observation.CompletionErrorClass != "none" || observation.FailureRecordErrorClass != "none" || observation.TerminalStatus != "completed") {
				t.Errorf("benign public usage rebuild failed completion: completion=%s detail=%s fallback=%s fallbackDetail=%s status=%s", observation.CompletionErrorClass, observation.CompletionDetailClass, observation.FailureRecordErrorClass, observation.FailureRecordDetailClass, observation.TerminalStatus)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("asynchronous observation did not complete")
		}
	}
	if stderr != "" {
		t.Error("benign public usage rebuild emitted a failure marker")
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("public usage read did not finish")
	}
	// This GET captured the old public cursor before entering usage rebuild.
	// Its later hydration must reject the now-torn frontier, not serve it as
	// current authority. A new GET after both operations drain must succeed.
	var refusal map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &refusal); err != nil || response.Code != http.StatusServiceUnavailable ||
		refusal["code"] != "accepted_final_hydration_unavailable" || len(refusal) != 2 {
		t.Fatalf("old public snapshot did not fail closed: status=%d", response.Code)
	}
	freshRequest := httptest.NewRequest(http.MethodGet, "/v1/threads/"+threadID, nil)
	freshRequest.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	freshResponse := httptest.NewRecorder()
	handler.ServeHTTP(freshResponse, freshRequest)
	var fresh map[string]any
	if err := json.Unmarshal(freshResponse.Body.Bytes(), &fresh); err != nil || freshResponse.Code != http.StatusOK {
		t.Fatalf("fresh public usage read failed: status=%d", freshResponse.Code)
	}
	turns := listAny(fresh["turns"])
	if len(turns) != 1 {
		t.Fatal("fresh public read changed the turn inventory")
	}
	turn, _ := turns[0].(map[string]any)
	if stringField(turn, "id") != turnID || stringField(turn, "status") != "completed" {
		t.Fatal("fresh public read omitted the completed turn")
	}
	for range 2 {
		if _, err := handler.store.RecordGeneralTerminalEventBundle(threadID, turnID); err != nil {
			t.Fatal("repeated terminal settlement failed after reader writeback")
		}
	}
	replay, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal("settled terminal replay failed")
	}
	bundle := terminalBundleEventsForClosureV1(replay.Events)
	if len(bundle) != 3 {
		t.Fatal("reader writeback changed terminal bundle cardinality")
	}
	assertGeneralTerminalEventIDsExactlyOnce(t, bundle)
	assertGeneralTerminalUsageIndexExactlyOnce(t, handler.store, threadID, bundle)
}
