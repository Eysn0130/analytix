package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cachetelemetrystore "analytix.local/runtime-go/internal/adapters/outbound/cachetelemetrystore"
	"analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	providerclient "analytix.local/runtime-go/internal/adapters/outbound/provider/client"
	turnterminalstore "analytix.local/runtime-go/internal/adapters/outbound/turnterminalstore"
	cachetelemetryapp "analytix.local/runtime-go/internal/app/cachetelemetry"
	apploop "analytix.local/runtime-go/internal/app/loop"
	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	riskapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	turnterminalapp "analytix.local/runtime-go/internal/app/turnterminal"
	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

type inlineCompletionFixture struct {
	handler                                        *runtimeServerHandler
	service                                        *InlineCompletionService
	request                                        apploop.InlineCompletionRequest
	workspace, durableRoot, telemetryRoot, keyPath string
	before                                         map[string]any
	calls                                          *atomic.Int32
}

func newInlineCompletionFixture(t *testing.T, reply http.HandlerFunc) inlineCompletionFixture {
	t.Helper()
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); reply(w, r) }))
	t.Cleanup(upstream.Close)
	workspace := workspacetest.New(t)
	if err := os.WriteFile(filepath.Join(workspace, "draft.md"), []byte("saved draft"), 0600); err != nil {
		t.Fatal(err)
	}
	durableRoot, dataDir := t.TempDir(), t.TempDir()
	h := NewRuntimeServerHandler(RuntimeServerConfig{RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: dataDir,
		ProviderID: "openai", BaseURL: upstream.URL + "/v1", APIKey: "synthetic-fixture-key", Model: "fixture-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, h)
	risk, err := riskapp.NewGeneralOnlyAuthority(filestore.CaseBindingReader{})
	if err != nil {
		t.Fatal(err)
	}
	h.turnSecurity.RiskAuthority = risk
	installHandlerProviderExecutionResolverForTest(h)
	files, err := filestore.NewObjectEditingFiles(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	h.objectEditing = editingapp.NewWithProjector(h.turnSecurity.Identity, files, runtimeObjectProjector{h})
	keyPath := filepath.Join(t.TempDir(), "key", "authority.json")
	signer, err := finalauthority.OpenOrCreateFileAuthority(keyPath, false)
	if err != nil {
		t.Fatal(err)
	}
	telemetryRoot := t.TempDir()
	access, err := privatecastest.NewAccessAuthority(telemetryRoot)
	if err != nil {
		t.Fatal(err)
	}
	telemetryStore, err := cachetelemetrystore.NewStore(telemetryRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	telemetry, err := cachetelemetryapp.NewDurableService(signer, telemetryStore)
	if err != nil {
		t.Fatal(err)
	}
	h.provider, err = providerclient.NewProductionHTTPProviderClient(upstream.Client(), telemetry)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewInlineCompletionService(h, telemetry)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := h.store.CreateThread(map[string]any{"workspace": workspace, "providerId": "openai", "model": "fixture-model"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	before, err := h.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	return inlineCompletionFixture{h, service, apploop.InlineCompletionRequest{ThreadID: threadID, RequestID: "00000000-0000-4000-8000-000000000001", Document: apploop.InlineCompletionDocument{Path: "draft.md"}, Prompt: "Continue this draft: AUX_ALLOWED_INPUT_CANARY", Model: "fixture-model"}, workspace, durableRoot, telemetryRoot, keyPath, before, &calls}
}

func writeInlineCompletionFixtureReply(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"AUX_ALLOWED_OUTPUT_CANARY\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
}

func TestInlineCompletionRealProviderAuditHasNoChatTurnAndReopensAuditOnly(t *testing.T) {
	bodies := make(chan []byte, 2)
	f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies <- body
		writeInlineCompletionFixtureReply(w)
	})
	for _, withSession := range []bool{false, true} {
		request := f.request
		if withSession {
			opened, err := f.handler.objectEditing.Open(context.Background(), f.workspace, "draft.md")
			if err != nil {
				t.Fatal(err)
			}
			request.Document = apploop.InlineCompletionDocument{SessionID: opened.SessionID, ObjectID: opened.ObjectID, BaseRevision: opened.Revision}
		}
		result, err := f.service.Complete(context.Background(), request)
		if err != nil {
			t.Fatalf("auxiliary completion failed: %v", err)
		}
		if result.ThreadID != request.ThreadID || result.RequestID != request.RequestID || result.Text != "AUX_ALLOWED_OUTPUT_CANARY" || len(result.ObjectID) != 64 || len(result.BaseRevision) != 64 {
			t.Fatal("response lost document/request ownership")
		}
		body := <-bodies
		if !bytes.Contains(body, []byte("AUX_ALLOWED_INPUT_CANARY")) {
			t.Fatal("projected prompt did not reach production transport")
		}
		var sent map[string]any
		if json.Unmarshal(body, &sent) != nil {
			t.Fatal("invalid provider request")
		}
		if _, has := sent["tools"]; has {
			t.Fatal("auxiliary advertised tools")
		}
	}
	after, err := f.handler.store.GetThread(f.request.ThreadID)
	if err != nil || !reflect.DeepEqual(f.before, after) {
		t.Fatal("auxiliary changed primary history/security/title")
	}
	for _, root := range []string{f.durableRoot, f.telemetryRoot} {
		persisted := readCaseIngressPublicTreeV1(t, root)
		if strings.Contains(persisted, "AUX_ALLOWED_INPUT_CANARY") || strings.Contains(persisted, "AUX_ALLOWED_OUTPUT_CANARY") {
			t.Fatal("auxiliary persisted prompt or output")
		}
	}
	// Reopen the actual signed telemetry store. With no final/terminal record,
	// production recovery classifies these closures as provider audit only.
	signer, err := finalauthority.OpenOrCreateFileAuthority(f.keyPath, true)
	if err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(f.telemetryRoot)
	if err != nil {
		t.Fatal(err)
	}
	store, err := cachetelemetrystore.NewStore(f.telemetryRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	telemetry, err := cachetelemetryapp.NewDurableService(signer, store)
	if err != nil {
		t.Fatal(err)
	}
	closures := 0
	if err := telemetry.VisitTurnClosuresV1(context.Background(), func(closure domaincache.ProviderTurnClosureV1) error {
		closures++
		if closure.TerminalReasonCode != domaincache.ProviderTurnTerminalSuccessV1 {
			t.Fatal("completed producer lacked successful closure")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if closures != 2 || f.calls.Load() != 2 {
		t.Fatalf("closure/provider count=%d/%d", closures, f.calls.Load())
	}
	finals, err := newServerTestPrivateFinalStore(t, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	terminalRoot := t.TempDir()
	terminalAccess, err := privatecastest.NewAccessAuthority(terminalRoot)
	if err != nil {
		t.Fatal(err)
	}
	terminals, err := turnterminalstore.NewStore(terminalRoot, terminalAccess)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := turnterminalapp.NewCoordinator(signer, finals, terminals, telemetry)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := NewTempDurableEventSessionStore(f.durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	casReader, err := finalauthority.NewAcceptedFinalCASReader(f.durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := coordinator.RecoverV1(context.Background(), turnterminalapp.RestartRecoveryInputV1{CompletionStore: reopened, CASReader: casReader})
	if err != nil || len(recovery.ProviderAuditOnly) != 2 || len(recovery.Complete) != 0 {
		t.Fatalf("auxiliary audit recovery failed: %v", err)
	}
	recovered, err := reopened.GetThread(f.request.ThreadID)
	if err != nil || !reflect.DeepEqual(f.before, recovered) {
		t.Fatal("audit recovery manufactured chat history")
	}
}

func TestInlineCompletionCancelsPhysicalProviderAndRejectsLateDocument(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		entered, aborted := make(chan struct{}), make(chan struct{})
		f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "text/event-stream")
			w.(http.Flusher).Flush()
			close(entered)
			select {
			case <-r.Context().Done():
				close(aborted)
			case <-time.After(10 * time.Second):
			}
		})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result := make(chan error, 1)
		go func() { _, err := f.service.Complete(ctx, f.request); result <- err }()
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("provider did not start")
		}
		cancel()
		select {
		case err := <-result:
			if !errors.Is(err, apploop.ErrInlineCompletionCanceled) {
				t.Fatalf("cancellation not returned: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("auxiliary did not settle cancellation")
		}
		select {
		case <-aborted:
		case <-time.After(time.Second):
			t.Fatal("provider HTTP context did not cancel")
		}
	})
	t.Run("changed revision", func(t *testing.T) {
		var path string
		f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
			if err := os.WriteFile(path, []byte("external revision"), 0600); err != nil {
				t.Error(err)
			}
			writeInlineCompletionFixtureReply(w)
		})
		path = filepath.Join(f.workspace, "draft.md")
		result, err := f.service.Complete(context.Background(), f.request)
		if err == nil || result.Text != "" {
			t.Fatal("late response survived document revision change")
		}
	})
}

func TestInlineCompletionRejectsBusyRevokedAndSensitiveSourcesBeforeSend(t *testing.T) {
	f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) { writeInlineCompletionFixtureReply(w) })
	if err := f.handler.control.RegisterTurnCancelWithError(f.request.ThreadID, "foreground", func() {}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Complete(context.Background(), f.request); !errors.Is(err, apploop.ErrInlineCompletionBusy) {
		t.Fatal("busy thread admitted")
	}
	f.handler.control.UnregisterTurnCancel(f.request.ThreadID, "foreground")
	opened, err := f.handler.objectEditing.Open(context.Background(), f.workspace, "draft.md")
	if err != nil {
		t.Fatal(err)
	}
	closed := f.request
	closed.Document = apploop.InlineCompletionDocument{SessionID: opened.SessionID, ObjectID: opened.ObjectID, BaseRevision: opened.Revision}
	if err := f.handler.objectEditing.Close(context.Background(), opened.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Complete(context.Background(), closed); err == nil {
		t.Fatal("revoked session fell back to path")
	}
	protected := f.request
	protected.Prompt = "Continue this draft using the local reference:\nprivate-person@example.invalid"
	if _, err := f.service.Complete(context.Background(), protected); err == nil {
		t.Fatal("sensitive source bypassed case-fund policy")
	}
	if f.calls.Load() != 0 {
		t.Fatal("rejected request reached physical provider")
	}
}

func TestInlineCompletionResumedPrimaryWithCurrentAuthority(t *testing.T) {
	f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) { writeInlineCompletionFixtureReply(w) })
	frozen, err := (runtimeObjectProjector{f.handler}).selectionSecurityContext(context.Background(), f.before, editingapp.ScopeAuthority{ThreadID: f.request.ThreadID, Workspace: f.workspace, Principal: testIdentityPrincipal()})
	if err != nil {
		t.Fatal(err)
	}
	record := turnsecurityapp.PublicRecord(frozen)
	if err := f.handler.store.AppendTurnToThread(f.request.ThreadID, map[string]any{"id": frozen.TurnID, "threadId": f.request.ThreadID, "status": "completed", "items": []any{}, "securityContext": record}, "", map[string]any{"securityState": record, "forkedFromThreadId": "prior-session"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.handler.store.PatchThread(f.request.ThreadID, map[string]any{"status": "idle"}); err != nil {
		t.Fatal(err)
	}
	if result, err := f.service.Complete(context.Background(), f.request); err != nil || result.Text == "" {
		t.Fatalf("resumed primary with current authority was rejected: %v", err)
	}
}

func TestInlineCompletionForegroundPreparationCancelsAndWaitsForAudit(t *testing.T) {
	entered, aborted := make(chan struct{}), make(chan struct{})
	f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		close(entered)
		select {
		case <-r.Context().Done():
			close(aborted)
		case <-time.After(10 * time.Second):
		}
	})
	result := make(chan error, 1)
	go func() { _, err := f.service.Complete(context.Background(), f.request); result <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	release, err := f.handler.control.PrepareForeground(ctx, f.request.ThreadID)
	if err != nil {
		t.Fatalf("foreground could not await audit closure: %v", err)
	}
	defer release()
	if err := <-result; !errors.Is(err, apploop.ErrInlineCompletionCanceled) {
		t.Fatalf("foreground did not cancel auxiliary: %v", err)
	}
	select {
	case <-aborted:
	case <-time.After(time.Second):
		t.Fatal("foreground did not cancel physical provider")
	}
	if _, err := f.service.Complete(context.Background(), f.request); !errors.Is(err, apploop.ErrInlineCompletionBusy) {
		t.Fatal("new auxiliary entered foreground preparation")
	}
}
