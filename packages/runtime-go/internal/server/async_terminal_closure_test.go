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
	"sync"
	"testing"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type asyncTerminalClosureProviderV1 struct {
	entered      chan struct{}
	release      chan struct{}
	failure      error
	enteredOnce  sync.Once
	observations chan AsyncTurnObservationV1
}

func (*asyncTerminalClosureProviderV1) RequiresDurablePipelineStagesV1() {}

func (p *asyncTerminalClosureProviderV1) Stream(ctx context.Context, request domainmodel.Request) (domainmodel.Result, error) {
	if err := emitTestDurableProviderPipelinePairV1(request); err != nil {
		return domainmodel.Result{}, err
	}
	p.enteredOnce.Do(func() { close(p.entered) })
	select {
	case <-p.release:
	case <-ctx.Done():
		return domainmodel.Result{}, ctx.Err()
	}
	if p.failure != nil {
		return domainmodel.Result{}, p.failure
	}
	chunk := domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: "The ordinary task is complete."}
	if request.OnChunk != nil {
		if err := request.OnChunk(chunk); err != nil {
			return domainmodel.Result{}, err
		}
	}
	return domainmodel.Result{ProviderID: request.ProviderID, EndpointFormat: request.EndpointFormat, Chunks: []domainmodel.Chunk{chunk}, StreamCompleted: true}, nil
}

func newAsyncTerminalClosureFixtureV1(t *testing.T, failure error) (*runtimeServerHandler, *asyncTerminalClosureProviderV1, string, string) {
	t.Helper()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "closure-provider", BaseURL: "https://provider.invalid", APIKey: "test-key", Model: "closure-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	workspace := t.TempDir()
	thread, err := handler.store.CreateThread(map[string]any{"workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	provider := &asyncTerminalClosureProviderV1{entered: make(chan struct{}), release: make(chan struct{}), failure: failure, observations: make(chan AsyncTurnObservationV1, 8)}
	handler.provider = provider
	handler.asyncTurnObserver = func(o AsyncTurnObservationV1) { provider.observations <- o }
	t.Cleanup(func() {
		select {
		case <-provider.release:
		default:
			close(provider.release)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := handler.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	response, err := handler.startRuntimeTurn(context.Background(), stringField(thread, "id"), startRuntimeTurnRequest{Prompt: "complete ordinary work", Async: true})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.entered:
	case <-time.After(15 * time.Second):
		t.Fatal("provider did not reach controlled completion")
	}
	return handler, provider, stringField(thread, "id"), stringField(response, "turnId")
}

func waitAsyncTerminalClosureV1(t *testing.T, handler *runtimeServerHandler) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := handler.runtimeControl().WaitForTurnOperations(ctx); err != nil {
		t.Fatal(err)
	}
}

// Each cut occurs after real start/pipeline persistence. The next start must
// neither append a second turn nor change the first outcome while settlement
// fails. A drained process retains the authentic CAS/outbox for recovery.
func TestAsyncTerminalClosurePreservesUnpublishedOutcomeAcrossNextTurn(t *testing.T) {
	for _, outcome := range []string{"success", "failure", "cancel"} {
		for _, cut := range []string{"archive", "events", "usage"} {
			t.Run(outcome+"/"+cut, func(t *testing.T) {
				var failure error
				if outcome == "failure" {
					failure = errors.New("producer /private/closure-sentinel")
				}
				if outcome == "cancel" {
					failure = context.Canceled
				}
				handler, provider, threadID, turnID := newAsyncTerminalClosureFixtureV1(t, failure)
				var restore func()
				switch cut {
				case "archive":
					handler.store.beforeTerminalWrite = func() error { return errors.New("archive /private/closure-sentinel") }
					restore = func() { handler.store.beforeTerminalWrite = nil }
				case "events":
					handler.store.generalTerminalAtomicAppend = func(string, []map[string]any) error { return errors.New("append /private/closure-sentinel") }
					restore = func() { handler.store.generalTerminalAtomicAppend = nil }
				case "usage":
					path := handler.store.usageIndex.ThreadPath(threadID)
					if err := os.MkdirAll(path, 0700); err != nil {
						t.Fatal(err)
					}
					restore = func() {
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
					}
				}
				defer func() { restore() }()
				close(provider.release)
				waitAsyncTerminalClosureV1(t, handler)
				before, err := handler.store.GetThread(threadID)
				if err != nil {
					t.Fatal(err)
				}
				turns := listAny(before["turns"])
				if len(turns) != 1 {
					t.Fatal("unexpected terminal inventory")
				}
				old := turns[0].(map[string]any)
				wantStatus := map[string]string{"success": "completed", "failure": "failed", "cancel": "aborted"}[outcome]
				if cut == "archive" {
					wantStatus = "running"
				}
				if stringField(old, "status") != wantStatus {
					t.Fatalf("cut did not reach expected durable state: got=%s want=%s", stringField(old, "status"), wantStatus)
				}
				started, finished := <-provider.observations, <-provider.observations
				if started.Stage != "started" || finished.Stage != "finished" || finished.CompletionErrorClass == "none" || finished.FailureRecordErrorClass == "none" || finished.TerminalStatus != wantStatus {
					t.Fatal("whole-operation observation lost the completion or fallback persistence failure")
				}
				if _, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{Prompt: "continue ordinary work"}); err == nil {
					t.Error("next turn overtook unresolved persistence")
				}
				request := httptest.NewRequest(http.MethodPost, "/v1/threads/"+threadID+"/turns", strings.NewReader(`{"prompt":"continue ordinary work","async":true}`))
				request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
				request.Header.Set("Content-Type", "application/json")
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, request)
				if recorder.Code < 400 || strings.Contains(recorder.Body.String(), "closure-sentinel") || strings.Contains(recorder.Body.String(), "/private/") {
					t.Fatal("public start accepted an unresolved terminal or exposed its private cause")
				}
				after, err := handler.store.GetThread(threadID)
				if err != nil || !reflect.DeepEqual(before["turns"], after["turns"]) {
					t.Fatal("next turn changed unresolved terminal inventory")
				}
				// Internal durable replay may retain the legitimate security context,
				// but must not retain the raw failure or private persistence path.
				replay, err := handler.store.LoadEventsSince(threadID, 0)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := json.Marshal(replay.Events)
				if strings.Contains(string(body), "closure-sentinel") || strings.Contains(string(body), handler.store.root) {
					t.Fatal("durable replay retained private failure input")
				}
				// Exercise the actual production projector separately: internal
				// workspaceRealPath is not evidence of public disclosure.
				publicStarted := false
				for _, event := range replay.Events {
					projected, visible, projectionErr := handler.publicProjector.ProjectEvent(threadID, after, event)
					if projectionErr != nil {
						t.Fatal(projectionErr)
					}
					if !visible {
						continue
					}
					publicBody, err := json.Marshal(projected)
					if err != nil || strings.Contains(string(publicBody), "closure-sentinel") ||
						strings.Contains(string(publicBody), handler.store.root) || strings.Contains(string(publicBody), "/private/") ||
						strings.Contains(string(publicBody), stringField(after, "workspace")) || projected["securityContext"] != nil {
						t.Fatal("public replay exposed failure input or private execution context")
					}
					publicStarted = publicStarted || stringField(projected, "kind") == "turn_started"
				}
				if !publicStarted {
					t.Fatal("privacy assertion did not exercise a visible public turn event")
				}
				terminalCount := 0
				for _, event := range replay.Events {
					if stringField(event, "turnId") == turnID && strings.HasPrefix(stringField(event, "kind"), "turn_") && stringField(event, "kind") != "turn_started" {
						terminalCount++
					}
				}
				if cut != "usage" && terminalCount != 0 {
					t.Fatal("uncommitted event bundle exposed a terminal")
				}
				interrupt := httptest.NewRequest(http.MethodPost, "/v1/threads/"+threadID+"/turns/"+turnID+"/interrupt", strings.NewReader(`{}`))
				interrupt.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
				interrupt.Header.Set("Content-Type", "application/json")
				interrupted := httptest.NewRecorder()
				handler.ServeHTTP(interrupted, interrupt)
				if interrupted.Code < 400 || strings.Contains(interrupted.Body.String(), "closure-sentinel") || strings.Contains(interrupted.Body.String(), handler.store.root) {
					t.Fatal("failed interrupt exposed success or private persistence input")
				}
				restore()
				restore = func() {}
				// Explicit interrupt reconciles the existing CAS winner. It must not
				// convert a committed success/failure into a conflicting cancellation.
				result, err := handler.interruptRuntimeTurn(context.Background(), controlapp.InterruptTurnRequest{ThreadID: threadID, TurnID: turnID})
				if err != nil || result.StatusCode != 200 {
					t.Fatalf("interrupt did not reconcile terminal: status=%d err=%v", result.StatusCode, err)
				}
				if cut == "archive" {
					wantStatus = "aborted"
				}
				current, err := handler.store.TurnStatus(threadID, turnID)
				if err != nil || current != wantStatus {
					t.Fatalf("interrupt changed canonical outcome: status=%s err=%v", current, err)
				}
				if err := handler.Shutdown(context.Background()); err != nil {
					t.Fatal(err)
				}
				reopened, err := NewTempDurableEventSessionStore(handler.store.root)
				if err != nil {
					t.Fatal(err)
				}
				if err := appturn.RecoverGeneralTerminalPublicationsAtStartupV1(context.Background(), reopened); err != nil {
					t.Fatal(err)
				}
				events, err := reopened.LoadEventsSince(threadID, 0)
				if err != nil {
					t.Fatal(err)
				}
				assertGeneralTerminalEventIDsExactlyOnce(t, terminalBundleEventsForClosureV1(events.Events))
				assertGeneralTerminalUsageIndexExactlyOnce(t, reopened, threadID, terminalBundleEventsForClosureV1(events.Events))
			})
		}
	}
}

func terminalBundleEventsForClosureV1(events []map[string]any) []map[string]any {
	var result []map[string]any
	for _, event := range events {
		if stringField(event, "generalTerminalEventId") != "" {
			result = append(result, event)
		}
	}
	return result
}

// The precommit hook holds the real store mutex and candidate permit. The
// bounded waits below only observe that controlled barrier; no timing race is
// used to decide which terminal owner wins.
func TestAsyncTerminalClosureOrdersCompletionCancelAndNextTurn(t *testing.T) {
	for _, failAppend := range []bool{false, true} {
		name := "committed"
		if failAppend {
			name = "append_failed"
		}
		t.Run(name, func(t *testing.T) {
			handler, provider, threadID, turnID := newAsyncTerminalClosureFixtureV1(t, nil)
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			handler.store.beforeTerminalWrite = func() error {
				once.Do(func() { close(entered) })
				<-release
				return nil
			}
			if failAppend {
				handler.store.generalTerminalAtomicAppend = func(string, []map[string]any) error { return errors.New("controlled terminal append failure") }
			}
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			close(provider.release)
			select {
			case <-entered:
			case <-time.After(15 * time.Second):
				t.Fatal("terminal commitment did not reach barrier")
			}
			if handler.runtimeControl().CancelRegisteredTurn(threadID, turnID) {
				t.Fatal("late cancellation stole the acquired candidate terminal")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
			if err := handler.runtimeControl().WaitForTurnOperations(ctx); !errors.Is(err, context.DeadlineExceeded) {
				cancel()
				t.Fatal("drain overtook terminal publication")
			}
			cancel()
			ctx, cancel = context.WithTimeout(context.Background(), 25*time.Millisecond)
			_, releaseInterrupt, err := handler.runtimeControl().ReserveInterruptTerminalAndWait(ctx, threadID, turnID)
			cancel()
			if releaseInterrupt != nil {
				releaseInterrupt()
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("interrupt overtook acquired terminal")
			}
			nextStarted := make(chan struct{})
			next := make(chan error, 1)
			go func() {
				close(nextStarted)
				_, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{Prompt: "continue ordinary work"})
				next <- err
			}()
			<-nextStarted
			close(release)
			select {
			case err := <-next:
				if (err != nil) != failAppend {
					t.Fatalf("next turn settlement result: failed=%t err=%v", failAppend, err)
				}
			case <-time.After(15 * time.Second):
				t.Fatal("next turn did not leave terminal barrier")
			}
			waitAsyncTerminalClosureV1(t, handler)
			thread, err := handler.store.GetThread(threadID)
			if err != nil {
				t.Fatal(err)
			}
			turns := listAny(thread["turns"])
			wantCount := 2
			if failAppend {
				wantCount = 1
			}
			if len(turns) != wantCount || stringField(turns[0].(map[string]any), "status") != "completed" {
				t.Fatal("completion/cancel ordering lost authentic winner")
			}
			replay, err := handler.store.LoadEventsSince(threadID, 0)
			if err != nil {
				t.Fatal(err)
			}
			terminalSeq, nextStart := 0, 0
			for _, event := range replay.Events {
				if stringField(event, "turnId") == turnID && stringField(event, "kind") == "turn_completed" {
					terminalSeq, _ = numericSeq(event["seq"])
				}
				if stringField(event, "turnId") != turnID && stringField(event, "kind") == "turn_started" {
					nextStart, _ = numericSeq(event["seq"])
				}
			}
			if !failAppend && (terminalSeq == 0 || nextStart <= terminalSeq) {
				t.Fatal("next turn started before the committed terminal event")
			}
			if failAppend && (terminalSeq != 0 || nextStart != 0) {
				t.Fatal("failed publication exposed success or next start")
			}
		})
	}
}
