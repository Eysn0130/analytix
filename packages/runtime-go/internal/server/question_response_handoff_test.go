package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gatecontinuationapp "analytix.local/runtime-go/internal/app/gatecontinuation"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	continuationstoreport "analytix.local/runtime-go/internal/ports/continuationstore"
	displayport "analytix.local/runtime-go/internal/ports/userinputdisplay"
	runtimeprovider "analytix.local/runtime-go/internal/provider"
)

// Real channel barriers hold either the signed request outbox before activation,
// or the original async producer before its deferred registration release.
func TestQuestionResponseHandoffV1(t *testing.T) {
	for _, window := range []string{"reserved", "active_producer"} {
		t.Run(window+"_same_post", func(t *testing.T) {
			h, selector, release, calls := questionResponseHandoffFixtureV1(t, window)
			initial := calls.Load()
			view := readTimedQuestionPrivateDisplay(t, h, selector)
			if view.Status != "pending" || view.Submission != nil {
				t.Fatal("the real private display did not expose the unanswered Question")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			response := questionResponseHandoffStartPOSTV1(h, ctx, selector, "One synthetic answer")
			questionResponseHandoffAssertWaitingV1(t, response)
			questionResponseHandoffAssertUnansweredV1(t, h, selector)
			if calls.Load() != initial {
				t.Fatal("waiting answer entered the Provider")
			}
			release()
			questionResponseHandoffAssertStatusV1(t, questionResponseHandoffAwaitV1(t, response), http.StatusOK)
			questionResponseHandoffAssertSubmittedV1(t, h, selector, "One synthetic answer", calls, initial+1)
		})
	}
	for _, test := range []struct {
		name, window string
		deadline     bool
	}{
		{"reserved_cancelled", "reserved", false},
		{"active_deadline", "active_producer", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			h, selector, release, calls := questionResponseHandoffFixtureV1(t, test.window)
			initial := calls.Load()
			var ctx context.Context
			var cancel context.CancelFunc
			if test.deadline {
				deadline := &questionResponseHandoffDeadlineContextV1{Context: context.Background(), done: make(chan struct{})}
				ctx, cancel = deadline, deadline.expire
			} else {
				ctx, cancel = context.WithCancel(context.Background())
			}
			defer cancel()
			response := questionResponseHandoffStartPOSTV1(h, ctx, selector, "Cancelled synthetic answer")
			questionResponseHandoffAssertWaitingV1(t, response)
			cancel()
			if test.deadline && ctx.Err() != context.DeadlineExceeded {
				t.Fatal("controlled deadline did not expire the waiting request")
			}
			actual := questionResponseHandoffAwaitV1(t, response)
			if actual.Code < 400 {
				t.Fatalf("cancelled wait acknowledged an answer: status=%d", actual.Code)
			}
			questionResponseHandoffAssertUnansweredV1(t, h, selector)
			release()
			waitCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
			defer stop()
			if err := h.runtimeControl().WaitForThreadTurns(waitCtx, selector.ThreadID); err != nil {
				t.Fatal(err)
			}
			questionResponseHandoffAssertUnansweredV1(t, h, selector)
			if calls.Load() != initial {
				t.Fatal("cancelled request continued after producer release")
			}
		})
	}
	t.Run("duplicate_single_winner", func(t *testing.T) {
		h, selector, release, calls := questionResponseHandoffFixtureV1(t, "active_producer")
		initial := calls.Load()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		first := questionResponseHandoffStartPOSTV1(h, ctx, selector, "One synthetic answer")
		second := questionResponseHandoffStartPOSTV1(h, ctx, selector, "One synthetic answer")
		questionResponseHandoffAssertWaitingV1(t, first)
		questionResponseHandoffAssertWaitingV1(t, second)
		questionResponseHandoffAssertUnansweredV1(t, h, selector)
		release()
		a, b := questionResponseHandoffAwaitV1(t, first), questionResponseHandoffAwaitV1(t, second)
		if !((a.Code == http.StatusOK && b.Code == http.StatusConflict) || (a.Code == http.StatusConflict && b.Code == http.StatusOK)) {
			t.Fatalf("duplicate POSTs did not have one winner: statuses=%d,%d", a.Code, b.Code)
		}
		questionResponseHandoffAssertSubmittedV1(t, h, selector, "One synthetic answer", calls, initial+1)
	})
	t.Run("settled_rejected", func(t *testing.T) {
		h, selector, release, calls := questionResponseHandoffFixtureV1(t, "active_producer")
		initial := calls.Load()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		first := questionResponseHandoffStartPOSTV1(h, ctx, selector, "One synthetic answer")
		questionResponseHandoffAssertWaitingV1(t, first)
		release()
		questionResponseHandoffAssertStatusV1(t, questionResponseHandoffAwaitV1(t, first), http.StatusOK)
		second := questionResponseHandoffPOSTV1(h, ctx, selector, "Another synthetic answer")
		questionResponseHandoffAssertStatusV1(t, second, http.StatusConflict)
		questionResponseHandoffAssertSubmittedV1(t, h, selector, "One synthetic answer", calls, initial+1)
	})
	t.Run("authority_stale_during_handoff", func(t *testing.T) {
		h, selector, release, calls := questionResponseHandoffFixtureV1(t, "reserved")
		initial := calls.Load()
		base, ok := h.turnSecurity.RiskAuthority.(*serverTestRiskAuthority)
		if !ok {
			t.Fatal("reserved fixture lost its witnessed current risk authority")
		}
		authority := &questionResponseHandoffStaleRiskV1{finalizationStaleRiskAuthority: &finalizationStaleRiskAuthority{base: base}}
		h.turnSecurity.RiskAuthority = authority
		defer authority.stale.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		// Keep the original selector and signed pending/grant unchanged. Only
		// the current authority source becomes stale after the POST is waiting.
		response := questionResponseHandoffStartPOSTV1(h, ctx, selector, "Stale authority synthetic answer")
		questionResponseHandoffAssertWaitingV1(t, response)
		questionResponseHandoffAssertUnansweredV1(t, h, selector)
		authority.stale.Store(true)
		release()
		actual := questionResponseHandoffAwaitV1(t, response)
		if actual.Code < 400 || authority.staleChecks.Load() == 0 {
			t.Fatalf("handoff did not reject through fresh authority validation: status=%d staleChecks=%d", actual.Code, authority.staleChecks.Load())
		}
		_, disposition, err := h.continuations.ResolveTrustedDispositionForAudit(context.Background(), selector.RequestID)
		if err != nil && !errors.Is(err, continuationstoreport.ErrNotFound) {
			t.Fatal("could not verify the rejected answer's signed state", err)
		}
		// Security closure may legitimately persist a rejected disposition.
		// It must never persist this submission or acknowledge it as submitted.
		if err == nil && (disposition.Status == domaincontinuation.StatusSubmitted || disposition.UserInputSubmission != nil) {
			t.Fatal("stale authority wrote a signed answer")
		}
		replay, err := h.store.LoadEventsSince(selector.ThreadID, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range replay.Events {
			if stringField(event, "kind") == "user_input_resolved" && stringField(event, "inputId") == selector.RequestID && stringField(event, "status") == domaincontinuation.StatusSubmitted {
				t.Fatal("stale authority published a submitted answer")
			}
		}
		if calls.Load() != initial {
			t.Fatal("stale authority entered the Provider continuation")
		}
		var body map[string]any
		_ = json.Unmarshal(actual.Body.Bytes(), &body)
		t.Logf("stale_authority_status=%d code=%s signed_disposition=%s stale_checks=%d provider_calls=%d", actual.Code, stringField(body, "code"), disposition.Status, authority.staleChecks.Load(), calls.Load()-initial)
	})
	t.Run("stale_selector_rejected", func(t *testing.T) {
		h, selector, _, calls := questionResponseHandoffFixtureV1(t, "active_producer")
		initial := calls.Load()
		stale := selector
		stale.TurnID = "turn_unrelated_synthetic"
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		response := questionResponseHandoffPOSTV1(h, ctx, stale, "Stale synthetic answer")
		questionResponseHandoffAssertStatusV1(t, response, http.StatusConflict)
		if ctx.Err() != nil {
			t.Fatal("unrelated selector waited on another producer")
		}
		questionResponseHandoffAssertUnansweredV1(t, h, selector)
		if calls.Load() != initial {
			t.Fatal("stale selector entered the Provider")
		}
	})
}

// Reuse the existing atomic stale-authority port and witness that the real
// post-handoff authorization consulted it, rather than rejecting another guard.
type questionResponseHandoffStaleRiskV1 struct {
	*finalizationStaleRiskAuthority
	staleChecks atomic.Int32
}

func (authority *questionResponseHandoffStaleRiskV1) ValidateCurrent(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	if authority.stale.Load() {
		authority.staleChecks.Add(1)
	}
	return authority.finalizationStaleRiskAuthority.ValidateCurrent(ctx, securityContext)
}

func questionResponseHandoffFixtureV1(t *testing.T, window string) (*runtimeServerHandler, displayport.SelectorV1, func(), *atomic.Int32) {
	t.Helper()
	if window == "reserved" {
		// This is the production RequestUserInput use case with its real signed
		// receipt, primary store and outbox. Only RecordRequest's return is held.
		var calls atomic.Int32
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			calls.Add(1)
			writeInlineCompletionFixtureReply(w)
		}))
		t.Cleanup(upstream.Close)
		h, pending, _, _ := protectedSeamFixtureV1(t, "request_user_input", map[string]any{
			"prompt": "One synthetic question", "questions": []any{map[string]any{"question": "Choose one answer?"}},
		})
		pending.ProviderConfig = domainmodel.TurnConfig{ProviderID: "synthetic", Model: "synthetic", APIKey: "synthetic-only", BaseURL: upstream.URL + "/v1", EndpointFormat: "chat_completions"}
		pending.LogicalEffect = domainsecurity.LogicalEffectOrdinary
		pending.OrdinaryWork, pending.ProviderStepExact, pending.OrdinaryResultInputIsolated = true, true, true
		pending.ProviderNamespace = domaincontinuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil)
		h.providerConfig = runtimeprovider.NewRuntimeProviderConfigSet(domainmodel.RuntimeProviderConfigInput{DefaultProviderID: "synthetic", DefaultModel: "synthetic", DefaultBaseURL: upstream.URL + "/v1", DefaultAPIKey: "synthetic-only", DefaultEndpointFormat: "chat_completions"})
		installHandlerProviderExecutionResolverForTest(h)
		pending.Messages = []domainmodel.Message{{Role: "user", Content: pending.Prompt}, {Role: "assistant", ToolCalls: []domainmodel.ToolCall{pending.Call}}}
		producerCtx, cancelProducer := context.WithCancel(context.Background())
		ownedCtx, owner, err := h.runtimeControl().RegisterTurnCancelOwnedV1(producerCtx, pending.ThreadID, pending.TurnID, cancelProducer)
		if err != nil {
			t.Fatal(err)
		}
		deps := h.runtimeGateContinuationDependencies()
		record := deps.RecordRequest
		published, unblock := make(chan map[string]any, 1), make(chan struct{})
		deps.RecordRequest = func(event map[string]any) error {
			if err := record(event); err != nil {
				return err
			}
			published <- event
			<-unblock
			return nil
		}
		done := make(chan error, 1)
		go func() {
			defer cancelProducer()
			defer h.runtimeControl().ReleaseTurnExecutionOwnerV1(owner)
			_, err := gatecontinuationapp.RequestUserInput(ownedCtx, pending, deps)
			done <- err
		}()
		var once sync.Once
		release := func() { once.Do(func() { close(unblock) }) }
		t.Cleanup(func() {
			release()
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("real Question issuance failed: %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Error("Question issuer did not leave RecordRequest barrier")
			}
			questionResponseHandoffShutdownV1(t, h)
		})
		var event map[string]any
		select {
		case event = <-published:
		case <-time.After(10 * time.Second):
			t.Fatal("real request outbox did not reach its published barrier")
		}
		selector := displayport.SelectorV1{ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: stringField(event, "itemId"), RequestID: stringField(event, "inputId")}
		_, _, exists, active := h.gates.PeekUserInput(selector.RequestID)
		if !exists || active {
			t.Fatal("published request was not reserved and non-executable")
		}
		return h, selector, release, &calls
	}
	var calls atomic.Int32
	f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			writeSyntheticQuestionCallV1(w, "request_user_input", `{"prompt":"One synthetic question","questions":[{"question":"Choose one answer?"}]}`)
			return
		}
		writeInlineCompletionFixtureReply(w)
	})
	h := f.handler
	configureTimedQuestionHistoryAuthority(t, h)
	entered, unblock := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(unblock) }) }
	h.asyncTurnObserver = func(observation AsyncTurnObservationV1) {
		if observation.Stage == "finished" {
			close(entered)
			<-unblock
		}
	}
	t.Cleanup(func() {
		release()
		questionResponseHandoffShutdownV1(t, h)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	started, err := h.startRuntimeTurn(ctx, f.request.ThreadID, startRuntimeTurnRequest{Prompt: "Ask one synthetic Question.", Async: true})
	if err != nil {
		t.Fatal(err)
	}
	selector := productionQuestionSelectorV1(t, h, f.request.ThreadID, stringField(started, "turnId"))
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("original async producer did not reach its finished observer")
	}
	_, _, exists, active := h.gates.PeekUserInput(selector.RequestID)
	if !exists || !active {
		t.Fatal("original producer did not activate its Question before its owner barrier")
	}
	return h, selector, release, &calls
}

func questionResponseHandoffPOSTV1(h *runtimeServerHandler, ctx context.Context, selector displayport.SelectorV1, answer string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]any{"selector": selector, "answers": []any{map[string]any{"id": selector.RequestID + "_1", "kind": "answer", "optionIds": []string{}, "customText": answer}}})
	request := httptest.NewRequest(http.MethodPost, "/v1/user-inputs/"+selector.RequestID, bytes.NewReader(body)).WithContext(ctx)
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	return response
}

type questionResponseHandoffPOSTResultV1 struct {
	response    <-chan *httptest.ResponseRecorder
	waitEntered <-chan struct{}
}

// The normal HTTP/response prefix checks Err without observing Done. The
// producer's bounded wait selects directly on this context, so this signal
// proves the POST reached that wait while the producer barrier remains held.
type questionResponseHandoffWaitContextV1 struct {
	context.Context
	entered chan struct{}
	once    sync.Once
}

func (ctx *questionResponseHandoffWaitContextV1) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.entered) })
	return ctx.Context.Done()
}

// A controlled deadline lets the test enter the real wait before delivering
// DeadlineExceeded, independently of machine scheduling or wall-clock speed.
type questionResponseHandoffDeadlineContextV1 struct {
	context.Context
	done chan struct{}
	once sync.Once
	mu   sync.Mutex
	err  error
}

func (ctx *questionResponseHandoffDeadlineContextV1) Done() <-chan struct{} { return ctx.done }
func (ctx *questionResponseHandoffDeadlineContextV1) Err() error {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.err
}
func (ctx *questionResponseHandoffDeadlineContextV1) expire() {
	ctx.once.Do(func() {
		ctx.mu.Lock()
		ctx.err = context.DeadlineExceeded
		ctx.mu.Unlock()
		close(ctx.done)
	})
}

func questionResponseHandoffStartPOSTV1(h *runtimeServerHandler, ctx context.Context, selector displayport.SelectorV1, answer string) questionResponseHandoffPOSTResultV1 {
	done := make(chan *httptest.ResponseRecorder, 1)
	observed := &questionResponseHandoffWaitContextV1{Context: ctx, entered: make(chan struct{})}
	go func() { done <- questionResponseHandoffPOSTV1(h, observed, selector, answer) }()
	return questionResponseHandoffPOSTResultV1{response: done, waitEntered: observed.entered}
}

func questionResponseHandoffAssertWaitingV1(t *testing.T, result questionResponseHandoffPOSTResultV1) {
	t.Helper()
	early := func(response *httptest.ResponseRecorder) {
		var body map[string]any
		_ = json.Unmarshal(response.Body.Bytes(), &body)
		t.Fatalf("the same POST returned before producer handoff: status=%d code=%s", response.Code, stringField(body, "code"))
	}
	select {
	case <-result.waitEntered:
	case response := <-result.response:
		early(response)
	case <-time.After(10 * time.Second):
		t.Fatal("Question POST did not enter its producer wait")
	}
	select {
	case response := <-result.response:
		early(response)
	default:
	}
}

func questionResponseHandoffAwaitV1(t *testing.T, result questionResponseHandoffPOSTResultV1) *httptest.ResponseRecorder {
	t.Helper()
	select {
	case response := <-result.response:
		return response
	case <-time.After(15 * time.Second):
		t.Fatal("Question POST did not finish after its handoff/context boundary")
		return nil
	}
}

func questionResponseHandoffAssertStatusV1(t *testing.T, response *httptest.ResponseRecorder, want int) {
	t.Helper()
	var body map[string]any
	if json.Unmarshal(response.Body.Bytes(), &body) != nil || response.Code != want {
		t.Fatalf("Question POST status=%d want=%d code=%s", response.Code, want, stringField(body, "code"))
	}
	if want == http.StatusOK && stringField(body, "status") != "submitted" {
		t.Fatal("Question response did not acknowledge a submitted answer")
	}
}

func questionResponseHandoffAssertSubmittedV1(t *testing.T, h *runtimeServerHandler, selector displayport.SelectorV1, answer string, calls *atomic.Int32, wantCalls int32) {
	t.Helper()
	_, disposition, err := h.continuations.ResolveTrustedDispositionForAudit(context.Background(), selector.RequestID)
	if err != nil || disposition.Status != domaincontinuation.StatusSubmitted || disposition.UserInputSubmission == nil {
		t.Fatal("the same POST did not retain the signed original answer", err)
	}
	var answers []map[string]any
	if json.Unmarshal(disposition.UserInputSubmission.Answers, &answers) != nil || len(answers) != 1 || stringField(answers[0], "customText") != answer {
		t.Fatal("the signed submission lost or replaced the original answer")
	}
	if calls.Load() != wantCalls {
		t.Fatalf("continuation Provider calls=%d want=%d", calls.Load(), wantCalls)
	}
	replay, err := h.store.LoadEventsSince(selector.ThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	resolved, completed := 0, 0
	for _, event := range replay.Events {
		if stringField(event, "turnId") != selector.TurnID {
			continue
		}
		if stringField(event, "kind") == "user_input_resolved" && stringField(event, "inputId") == selector.RequestID {
			resolved++
		}
		if stringField(event, "kind") == "turn_completed" {
			completed++
		}
	}
	if resolved != 1 || completed != 1 {
		t.Fatalf("Question continuation effects: resolutions=%d completions=%d", resolved, completed)
	}
	view := readTimedQuestionPrivateDisplay(t, h, selector)
	if view.Status != "submitted" || view.Submission == nil {
		t.Fatal("private display disagreed with the signed submitted answer")
	}
}

func questionResponseHandoffAssertUnansweredV1(t *testing.T, h *runtimeServerHandler, selector displayport.SelectorV1) {
	t.Helper()
	if _, _, err := h.continuations.ResolveTrustedDispositionForAudit(context.Background(), selector.RequestID); err == nil {
		t.Fatal("waiting/rejected answer acquired a signed disposition")
	}
	replay, err := h.store.LoadEventsSince(selector.ThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range replay.Events {
		if stringField(event, "kind") == "user_input_resolved" && stringField(event, "inputId") == selector.RequestID {
			t.Fatal("waiting/rejected answer published a resolution")
		}
	}
}

func questionResponseHandoffShutdownV1(t *testing.T, h *runtimeServerHandler) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.Shutdown(ctx); err != nil {
		t.Errorf("handoff fixture shutdown: %v", err)
	}
}
