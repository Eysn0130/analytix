//go:build darwin || linux

package runtimeapp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	casestore "analytix.local/runtime-go/internal/adapters/outbound/casethreadauthority"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	"analytix.local/runtime-go/internal/contracts"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	"analytix.local/runtime-go/internal/server"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func newAsyncCaseClosureConfigV1(t *testing.T) (Config, string) {
	t.Helper()
	// The runtime is opened after provisioning, so its registry needs an
	// authenticated nonempty inventory. An admitted Dataset alone, or an empty
	// registry activated only in the provisioning process, is not restart
	// authority. This synthetic receipt uses the actual snapshot effect,
	// enrolled witness and registry CAS; it is not native-analysis evidence.
	fixture := newPreparedRegistryEffectFixtureV1(t)
	config := fixture.config
	config.UserDataDir = t.TempDir()
	workspace := fixture.resolve.Observation.WorkspaceRealPath
	if err := os.WriteFile(filepath.Join(workspace, "ordinary.txt"), []byte("ordinary fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	durable, err := server.NewProductionDurableEventSessionStore(config.ProductionDurableRoot)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := durable.CreateThread(map[string]any{"title": "synthetic Registry provisioning"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID, _ := thread["id"].(string)
	previous := fixture.securityContext
	issuedAt, err := time.Parse(time.RFC3339Nano, previous.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	input := domainsecurity.TurnSecurityContextInput{ThreadID: threadID, TurnID: previous.TurnID, WorkspaceRealPath: workspace, TenantID: previous.TenantID, UserID: previous.UserID, CaseID: previous.CaseID, CaseBindingHash: previous.CaseBindingHash, DatasetSnapshotID: previous.DatasetSnapshotID, SourceManifestHash: previous.SourceManifestHash, ContextEpoch: previous.ContextEpoch, IssuedAt: issuedAt}
	frozen, err := securitycontexttest.CaseExecutionContextV2(input)
	if err != nil {
		t.Fatal(err)
	}
	input.RiskAuthorityBinding = frozen.RiskAuthorityBinding
	input.PublicationPolicy, err = domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{ThreadRiskPolicyDigest: frozen.PublicationPolicy.ThreadRiskPolicyDigest, RiskClass: domainsecurity.RiskClassCase, Disposition: domainsecurity.PublicationDispositionCaseEvidenceGate, CaseBindingState: domainsecurity.CaseBindingStateValid, BindingObservationDigest: fixture.resolve.Observation.ObservationDigest, BlockerCode: domainsecurity.PublicationBlockerNone})
	if err != nil {
		t.Fatal(err)
	}
	fixture.securityContext, err = domainsecurity.NewTurnSecurityContextV2(input)
	if err != nil {
		t.Fatal(err)
	}
	committed := false
	if err := fixture.composition.snapshot.WithCurrentSelectionV2(ctx, fixture.resolve, fixture.securityContext,
		func(selection datasetsnapshotport.CurrentSelectionV2, capability datasetsnapshotport.CurrentSelectionCapabilityV2) error {
			prepared := fixture.prepare(t, selection)
			seedAsyncCaseClosureRegistryHistoryV1(t, fixture, durable, prepared)
			marker, err := domainevidence.NewHostEvidenceSettlementMarker(prepared)
			if err != nil {
				return err
			}
			effect, ok := capability.(datasetsnapshotport.RegistryCommitCapabilityV1)
			if !ok {
				t.Fatal("CASE fixture lacks the actual registry commit effect")
			}
			return effect.UseExactRegistryCommit(selection, fixture.securityContext, prepared, marker, func(lease context.Context) error {
				receipt, err := fixture.composition.registryOwner.CommitPrepared(lease, preparedRegistryCommitInputV1(prepared))
				if err == nil {
					committed = receipt.ReceiptID == prepared.ReceiptID
				}
				return err
			})
		}); err != nil {
		t.Fatal(err)
	}
	if !committed {
		t.Fatal("CASE fixture lacks its actual committed registry receipt")
	}
	if err := fixture.composition.registryOwner.Close(); err != nil {
		t.Fatal(err)
	}
	return config, workspace
}

// Preserve the original authority graph for the real registration across
// restart. This is signed synthetic historical preparation, not a fabricated
// receipt, native execution, or accepted final. The receipt itself still comes
// exclusively from the callback-scoped registry effect above.
func seedAsyncCaseClosureRegistryHistoryV1(t *testing.T, fixture *preparedRegistryEffectFixtureV1, durable *server.DurableEventSessionStore, prepared domainevidence.PreparedEvidenceSettlement) {
	t.Helper()
	ctx := context.Background()
	frozen := fixture.securityContext
	at, err := time.Parse(time.RFC3339Nano, frozen.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	body, err := domainevidence.PreparedEvidenceSettlementBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	privateRoot := filepath.Join(fixture.config.DataDir, "private")
	preparedPath := filepath.Join(privateRoot, "evidence-settlements", "prepared", prepared.SettlementID[:2], prepared.SettlementID+".json")
	if err := os.MkdirAll(filepath.Dir(preparedPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(preparedPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	clear(body)
	marker, err := domainevidence.NewHostEvidenceSettlementMarker(prepared)
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := contextepochapp.PrepareTurn(contextepochapp.PrepareTurnInput{Thread: map[string]any{}, SecurityContext: frozen, At: at})
	if err != nil {
		t.Fatal(err)
	}
	grant := prepared.ExecutionGrant
	registeredAt := preparedRegistryCommitInputV1(prepared).RegisteredAt.Format(time.RFC3339Nano)
	call := map[string]any{"id": domaintoolcall.ToolCallItemIDV1(frozen.TurnID, grant.ToolCallID), "kind": "tool_call", "role": "assistant", "status": "completed", "threadId": frozen.ThreadID, "turnId": frozen.TurnID, "toolName": grant.ToolName, "callId": grant.ToolCallID, "contextDigest": frozen.ContextDigest, "contextEpoch": frozen.ContextEpoch, "executionGrantId": grant.GrantID, "executionGrant": grant, "arguments": domaintoolcall.WithheldArgumentsProjectionV1(), "createdAt": grant.IssuedAt}
	result := map[string]any{"id": prepared.ResultItemID, "kind": "tool_result", "role": "tool", "status": "completed", "threadId": frozen.ThreadID, "turnId": frozen.TurnID, "toolName": grant.ToolName, "callId": grant.ToolCallID, "contextDigest": frozen.ContextDigest, "contextEpoch": frozen.ContextEpoch, "executionGrantId": grant.GrantID, "createdAt": registeredAt, "finishedAt": registeredAt, "isError": false, "hostEvidenceSettlement": marker}
	if err := durable.AppendTurnToThread(frozen.ThreadID, map[string]any{"id": frozen.TurnID, "threadId": frozen.ThreadID, "status": "running", "securityContext": frozen, "contextEpochSnapshot": contextepochapp.PublicSnapshot(epoch.State.AcceptedSnapshot), "items": []any{call, result}}, "", map[string]any{"securityState": frozen, "contextEpochState": contextepochapp.PublicState(epoch.State)}); err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	store, err := casestore.NewStore(filepath.Join(privateRoot, "case-thread-authority"), access)
	if err != nil {
		t.Fatal(err)
	}
	cases, err := casethreadapp.NewRegistry(ctx, fixture.witness.Authority, store)
	if err != nil {
		t.Fatal(err)
	}
	if err := cases.Register(ctx, frozen); err != nil {
		t.Fatal(err)
	}
	if err := cases.Commit(ctx, frozen, epoch.State, at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
}

// The cuts occur at exact live CASE finalization phases. The archive failures
// retain one private preparation; the longitudinal failure follows a committed
// winner and degrades only the optional case-entity capability on restart.
func TestRuntimeAsyncCaseTerminalClosure(t *testing.T) {
	for _, branch := range []string{"candidate", "fixed", "longitudinal"} {
		t.Run(branch, func(t *testing.T) {
			config, workspace := newAsyncCaseClosureConfigV1(t)
			phase := "case_" + branch + "_persist"
			if branch == "longitudinal" {
				phase = "case_longitudinal_append"
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				frame := `{"choices":[{"delta":{"content":"An ordinary note is complete."},"finish_reason":"stop"}]}`
				_, _ = w.Write([]byte("data: " + frame + "\n\ndata: [DONE]\n\n"))
			}))
			defer provider.Close()
			config.BaseURL = provider.URL + "/v1"
			seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, config.ProviderID, config.BaseURL, []string{config.Model}, config.Model, "test-only")
			observations := make(chan server.AsyncTurnObservationV1, 8)
			lease, err := AcquireRuntimePersistenceLease(config)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.WithValue(context.Background(), asyncTurnObservationContextKeyV1{}, func(o server.AsyncTurnObservationV1) { observations <- o })
			ctx = context.WithValue(ctx, asyncTurnPhaseObservationContextKeyV1{}, func(current string) {
				if current == phase {
					once.Do(func() { close(entered) })
					<-release
				}
			})
			inner, err := newRuntimeServerHandlerWithPersistenceLeaseContextE(ctx, config, lease)
			if err != nil {
				_ = lease.Close()
				t.Fatal(err)
			}
			var handler http.Handler = &ownedPersistenceLeaseHandler{Handler: inner, lease: lease}
			var access finalauthority.SecurePrivateCASAccessAuthority = lease
			baselineAuthority := readAsyncCaseAuthorityLeavesV1(t, config, access)
			var restore func()
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
				if handler != nil {
					shutdownOwnedRuntimeHandler(t, handler)
				}
				if restore != nil {
					restore()
				}
			}()
			thread := asyncCaseClosureJSONV1(t, handler, http.MethodPost, "/v1/threads", map[string]any{"workspace": workspace, "providerId": config.ProviderID, "model": config.Model}, http.StatusCreated)
			threadID := contracts.StringField(thread, "id")
			prompt := "Read ordinary.txt and write a concise ordinary note."
			if branch == "fixed" {
				prompt = "请分析当前案件资金流向并生成资金报告。"
			}
			started := asyncCaseClosureJSONV1(t, handler, http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{
				"prompt": prompt, "riskIntent": "case", "async": true,
				"maxModelSteps": 1, "approvalPolicy": "never",
			}, http.StatusAccepted)
			turnID := contracts.StringField(started, "turnId")
			for waiting := true; waiting; {
				select {
				case <-entered:
					waiting = false
				case o := <-observations:
					if o.Stage == "finished" {
						t.Fatalf("CASE target phase not reached: phase=%s completion=%s fallback=%s", o.CompletionPhase, o.CompletionErrorClass, o.FailureRecordErrorClass)
					}
				case <-time.After(20 * time.Second):
					t.Fatal("CASE terminal phase was not entered")
				}
			}
			path := filepath.Join(config.ProductionDurableRoot, "threads", threadID, "thread.json")
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var raw map[string]any
			if json.Unmarshal(body, &raw) != nil {
				t.Fatal("invalid CASE archive")
			}
			turn := raw["turns"].([]any)[0].(map[string]any)
			frozen, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
			if err != nil || !domainsecurity.TurnSecurityContextIsCaseSensitive(frozen) {
				t.Fatal("fixture did not enter actual CASE authority")
			}
			var semanticBefore []finalauthority.SecurePrivateCASFile
			if branch == "longitudinal" {
				if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(frozen) != nil {
					t.Fatal("longitudinal fixture lacks actual fact-publication context")
				}
				if contracts.StringField(turn, "status") != "completed" {
					t.Fatal("longitudinal cut preceded the committed CASE final")
				}
				semanticBefore = injectAsyncCaseLongitudinalFaultV1(t, config, access)
			} else {
				backup := path + ".closure-fixture"
				if err := os.Rename(path, backup); err != nil {
					t.Fatal(err)
				}
				restore = func() {
					if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
						t.Fatal(err)
					}
					if err := os.Rename(backup, path); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			close(release)
			var finished server.AsyncTurnObservationV1
			for finished.Stage != "finished" {
				select {
				case finished = <-observations:
				case <-time.After(20 * time.Second):
					t.Fatal("CASE async operation did not finish")
				}
			}
			if finished.ThreadID != threadID || finished.TurnID != turnID || finished.CompletionPhase != phase || finished.CompletionErrorClass == "none" || finished.FailureRecordErrorClass == "none" {
				t.Fatalf("CASE phase=%s completion=%s fallback=%s status=%s", finished.CompletionPhase, finished.CompletionErrorClass, finished.FailureRecordErrorClass, finished.TerminalStatus)
			}
			t.Logf("CASE branch=%s completion=%s fallback=%s", branch, finished.CompletionErrorClass, finished.FailureRecordErrorClass)
			if branch == "longitudinal" && finished.CompletionErrorClass != "case_private_state_integrity" {
				t.Fatal("longitudinal append did not execute the actual persistent service error")
			}
			original := readAsyncCasePrivateFinalV1(t, config, access, threadID, turnID)
			beforeAuthority := readAsyncCaseAuthorityLeavesV1(t, config, access)
			if branch != "longitudinal" {
				req := httptest.NewRequest(http.MethodPost, "/v1/threads/"+threadID+"/turns", strings.NewReader(`{"prompt":"continue ordinary work","async":true}`))
				req.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				if rec.Code < 400 || strings.Contains(rec.Body.String(), config.DataDir) || strings.Contains(rec.Body.String(), config.ProductionDurableRoot) {
					t.Fatal("unresolved CASE start exposed success or private failure")
				}
			}
			interrupt := httptest.NewRequest(http.MethodPost, "/v1/threads/"+threadID+"/turns/"+turnID+"/interrupt", strings.NewReader(`{}`))
			interrupt.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
			interrupted := httptest.NewRecorder()
			handler.ServeHTTP(interrupted, interrupt)
			if (branch != "longitudinal" && interrupted.Code < 400) || strings.Contains(interrupted.Body.String(), config.DataDir) || strings.Contains(interrupted.Body.String(), config.ProductionDurableRoot) {
				t.Fatal("CASE cancel exposed a private error or crossed failed archive authority")
			}
			if !reflect.DeepEqual(beforeAuthority, readAsyncCaseAuthorityLeavesV1(t, config, access)) {
				t.Fatal("next-start or cancel changed the original CASE authority")
			}
			shutdownOwnedRuntimeHandler(t, handler)
			handler = nil
			if restore != nil {
				restore()
				restore = nil
			}
			handler, err = NewRuntimeServerHandlerE(config)
			if err != nil {
				t.Fatal(err)
			}
			access = handler.(*ownedPersistenceLeaseHandler).lease
			detail := asyncCaseClosureJSONV1(t, handler, http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK)
			publicDetail, _ := json.Marshal(detail)
			if bytes.Contains(publicDetail, []byte(config.DataDir)) || bytes.Contains(publicDetail, []byte(config.ProductionDurableRoot)) || bytes.Contains(publicDetail, []byte(`"acceptedFinal":`)) {
				t.Fatal("CASE HTTP exposed private authority")
			}
			turns := detail["turns"].([]any)
			if len(turns) != 1 || contracts.StringField(turns[0].(map[string]any), "id") != turnID {
				t.Fatal("CASE restart lost original inventory")
			}
			assertAsyncCaseCommittedAuthorityV1(t, config, access, original, baselineAuthority)
			assertAsyncCaseSSEV1(t, handler, config, threadID, turnID)
			recoveredArchive, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			eventPath := filepath.Join(filepath.Dir(path), "events.jsonl")
			recoveredEvents, err := os.ReadFile(eventPath)
			if err != nil {
				t.Fatal(err)
			}
			recoveredAuthority := readAsyncCaseAuthorityLeavesV1(t, config, access)
			if branch == "longitudinal" && (!bytes.Equal(body, recoveredArchive) || !reflect.DeepEqual(beforeAuthority, recoveredAuthority)) {
				t.Fatal("longitudinal failure or restart changed the already committed winner")
			}
			shutdownOwnedRuntimeHandler(t, handler)
			handler = nil
			handler, err = NewRuntimeServerHandlerE(config)
			if err != nil {
				t.Fatal(err)
			}
			access = handler.(*ownedPersistenceLeaseHandler).lease
			againArchive, archiveErr := os.ReadFile(path)
			againEvents, eventsErr := os.ReadFile(eventPath)
			if archiveErr != nil || eventsErr != nil || !bytes.Equal(recoveredArchive, againArchive) || !bytes.Equal(recoveredEvents, againEvents) || !reflect.DeepEqual(recoveredAuthority, readAsyncCaseAuthorityLeavesV1(t, config, access)) {
				t.Fatal("second production restart duplicated or changed CASE terminal authority")
			}
			if branch == "longitudinal" {
				if !reflect.DeepEqual(semanticBefore, readAsyncCaseCASV1(t, config, access, "case-entity/thread-context-v1")) {
					t.Fatal("semantic degradation rewrote or discarded authenticated longitudinal bytes")
				}
				tools := asyncCaseClosureJSONV1(t, handler, http.MethodGet, "/v1/runtime/tools?refresh=1", nil, http.StatusOK)
				if strings.Contains(runtimeStartupJSONBodyV1(t, tools), "analyze_account_flows") {
					t.Fatal("degraded optional case capability was advertised")
				}
			}
			// Once the original CASE winner is recovered, ordinary work on the
			// same thread can continue even when longitudinal capability degraded.
			asyncCaseClosureJSONV1(t, handler, http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{"prompt": "Write a concise ordinary note."}, http.StatusAccepted)
			ordinaryDetail := asyncCaseClosureJSONV1(t, handler, http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK)
			ordinaryTurns := ordinaryDetail["turns"].([]any)
			if len(ordinaryTurns) != 2 || contracts.StringField(ordinaryTurns[0].(map[string]any), "id") != turnID || contracts.StringField(ordinaryTurns[0].(map[string]any), "status") != original.PublicationIntent.TerminalStatus || contracts.StringField(ordinaryTurns[1].(map[string]any), "status") != "completed" {
				t.Fatal("CASE recovery blocked ordinary continuation or replaced the original turn")
			}
			// A new current epoch may legitimately suppress the old public
			// acceptedFinalView. Its canonical CAS and private authority remain
			// immutable; presentation equality would reject that privacy rule.
			var beforeContinuation, afterContinuation map[string]any
			continuedArchive, err := os.ReadFile(path)
			if err != nil || json.Unmarshal(recoveredArchive, &beforeContinuation) != nil || json.Unmarshal(continuedArchive, &afterContinuation) != nil {
				t.Fatal("CASE continuation archive is unavailable")
			}
			if !reflect.DeepEqual(beforeContinuation["turns"].([]any)[0], afterContinuation["turns"].([]any)[0]) {
				t.Fatal("ordinary next start changed the canonical original CASE turn")
			}
			afterAuthority := readAsyncCaseAuthorityLeavesV1(t, config, access)
			for leaf, files := range recoveredAuthority {
				for _, originalFile := range files {
					preserved := false
					for _, current := range afterAuthority[leaf] {
						if current.Digest == originalFile.Digest && bytes.Equal(current.Body, originalFile.Body) {
							preserved = true
						}
					}
					if !preserved {
						t.Fatal("ordinary next start lost or changed original CASE private authority")
					}
				}
			}
			beforeContext, beforeErr := domainsecurity.ParseTurnSecurityContext(beforeContinuation["securityState"])
			afterContext, afterErr := domainsecurity.ParseTurnSecurityContext(afterContinuation["securityState"])
			if beforeErr != nil || afterErr != nil {
				t.Fatal("CASE continuation context is invalid")
			}
			projectionChanged := !reflect.DeepEqual(ordinaryTurns[0], turns[0])
			if projectionChanged && beforeContext.ContextEpoch == afterContext.ContextEpoch {
				t.Fatal("CASE public projection changed without a new context epoch")
			}
			t.Logf("CASE ordinary continuation completed; original CAS/private bytes preserved; public_projection_changed=%t context_epoch_changed=%t", projectionChanged, beforeContext.ContextEpoch != afterContext.ContextEpoch)
		})
	}
}

func readAsyncCaseCASV1(t *testing.T, config Config, access finalauthority.SecurePrivateCASAccessAuthority, leaf string) []finalauthority.SecurePrivateCASFile {
	t.Helper()
	maxBytes := 16 << 20
	if leaf == "case-entity/thread-context-v1" {
		maxBytes = domaincaseentity.MaxThreadCaseContextRecordBytesV1
	} else if strings.HasPrefix(leaf, "turn-terminal-authority/") {
		maxBytes = 256 << 10
	}
	cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(config.DataDir, "private", leaf), maxBytes, access)
	if err != nil {
		t.Fatal(err)
	}
	defer cas.Close()
	files, err := cas.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func injectAsyncCaseLongitudinalFaultV1(t *testing.T, config Config, access finalauthority.SecurePrivateCASAccessAuthority) []finalauthority.SecurePrivateCASFile {
	t.Helper()
	cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(config.DataDir, "private", "case-entity", "thread-context-v1"), domaincaseentity.MaxThreadCaseContextRecordBytesV1, access)
	if err != nil {
		t.Fatal(err)
	}
	key := domainsecurity.SHA256Hex([]byte("async-case-longitudinal-domain-fault"))
	if err := cas.PutIfAbsent(context.Background(), key, []byte(`{}`)); err != nil {
		_ = cas.Close()
		t.Fatal(err)
	}
	if err := cas.Close(); err != nil {
		t.Fatal(err)
	}
	return readAsyncCaseCASV1(t, config, access, "case-entity/thread-context-v1")
}

func readAsyncCasePrivateFinalV1(t *testing.T, config Config, access finalauthority.SecurePrivateCASAccessAuthority, threadID, turnID string) domainevidence.PrivateAcceptedFinalRecord {
	t.Helper()
	files := readAsyncCaseCASV1(t, config, access, "accepted-finals/records")
	var original domainevidence.PrivateAcceptedFinalRecord
	count := 0
	for _, file := range files {
		record, err := domainevidence.ParsePrivateAcceptedFinalRecord(file.Body)
		if err != nil {
			t.Fatal("private CASE preparation is invalid")
		}
		if record.SecurityContext.ThreadID == threadID && record.SecurityContext.TurnID == turnID {
			original = record
			count++
		}
	}
	if count != 1 {
		t.Fatal("CASE fallback did not retain exactly one preparation for the original turn")
	}
	return original
}

func readAsyncCaseAuthorityLeavesV1(t *testing.T, config Config, access finalauthority.SecurePrivateCASAccessAuthority) map[string][]finalauthority.SecurePrivateCASFile {
	t.Helper()
	result := map[string][]finalauthority.SecurePrivateCASFile{}
	for _, leaf := range []string{"accepted-finals/records", "accepted-finals/dispositions", "turn-terminal-authority/intents", "turn-terminal-authority/dispositions"} {
		result[leaf] = readAsyncCaseCASV1(t, config, access, leaf)
	}
	return result
}

func assertAsyncCaseCommittedAuthorityV1(t *testing.T, config Config, access finalauthority.SecurePrivateCASAccessAuthority, original domainevidence.PrivateAcceptedFinalRecord, baseline map[string][]finalauthority.SecurePrivateCASFile) {
	t.Helper()
	current := readAsyncCasePrivateFinalV1(t, config, access, original.SecurityContext.ThreadID, original.SecurityContext.TurnID)
	if !reflect.DeepEqual(original, current) {
		t.Fatal("restart replaced the authentic private final")
	}
	leaves := readAsyncCaseAuthorityLeavesV1(t, config, access)
	for leaf, files := range leaves {
		if len(files) != len(baseline[leaf])+1 {
			t.Fatal("CASE recovery lacks one complete authority chain")
		}
		for _, previous := range baseline[leaf] {
			found := false
			for _, current := range files {
				found = found || (current.Digest == previous.Digest && bytes.Equal(current.Body, previous.Body))
			}
			if !found {
				t.Fatal("CASE recovery changed the independent historical authority")
			}
		}
		var added []finalauthority.SecurePrivateCASFile
		for _, current := range files {
			previous := false
			for _, before := range baseline[leaf] {
				previous = previous || before.Digest == current.Digest
			}
			if !previous {
				added = append(added, current)
			}
		}
		if len(added) != 1 {
			t.Fatal("CASE recovery added more than the exact original authority")
		}
		leaves[leaf] = added
	}
	disposition, err := domainevidence.ParseAcceptedFinalDispositionRecord(leaves["accepted-finals/dispositions"][0].Body)
	if err != nil || disposition.State != domainevidence.AcceptedFinalCommitted || disposition.DecisionBasis != domainevidence.AcceptedFinalDecisionSamePublicWinner || disposition.AcceptedFinalDigest != original.AcceptedFinal.RecordDigest || disposition.WinnerDigest != original.AcceptedFinal.RecordDigest {
		t.Fatal("CASE recovery disposition does not authenticate the original winner")
	}
	intent, err := domainturnterminal.ParseTurnTerminalIntentV1(leaves["turn-terminal-authority/intents"][0].Body)
	if err != nil || intent.AcceptedFinalDigest != original.AcceptedFinal.RecordDigest || intent.PrivateFinalStoreDigest != original.StoreDigest {
		t.Fatal("CASE intent changed private final authority")
	}
	terminal, err := domainturnterminal.ParseTurnTerminalDispositionV1(leaves["turn-terminal-authority/dispositions"][0].Body)
	if err != nil || terminal.IntentID != intent.IntentID || terminal.AcceptedFinalDigest != original.AcceptedFinal.RecordDigest || terminal.AcceptedFinalDispositionDigest != disposition.RecordDigest || terminal.ProviderClosureDigest == "" {
		t.Fatal("CASE terminal disposition lacks the exact private/provider closure")
	}
	body, err := os.ReadFile(filepath.Join(config.ProductionDurableRoot, "threads", original.SecurityContext.ThreadID, "thread.json"))
	if err != nil {
		t.Fatal(err)
	}
	var archive map[string]any
	if json.Unmarshal(body, &archive) != nil {
		t.Fatal("invalid recovered CASE archive")
	}
	turns := archive["turns"].([]any)
	if len(turns) != 1 {
		t.Fatal("recovered CASE turn inventory changed")
	}
	turn := turns[0].(map[string]any)
	final, err := domainevidence.ParseAcceptedFinalRecord(turn["acceptedFinal"])
	if err != nil || final.RecordDigest != original.AcceptedFinal.RecordDigest || contracts.StringField(turn, "status") != original.PublicationIntent.TerminalStatus {
		t.Fatal("recovered public CAS differs from authentic original intent")
	}
}

func assertAsyncCaseSSEV1(t *testing.T, handler http.Handler, config Config, threadID, turnID string) {
	t.Helper()
	live := httptest.NewServer(handler)
	defer live.Close()
	req, err := http.NewRequest(http.MethodGet, live.URL+"/v1/threads/"+threadID+"/events?since_seq=0", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatal("CASE SSE recovery is unavailable")
	}
	if bytes.Count(body, []byte("event: accepted_final_batch\n")) != 1 || !bytes.Contains(body, []byte(turnID)) || bytes.Contains(body, []byte(config.DataDir)) || bytes.Contains(body, []byte(config.ProductionDurableRoot)) || bytes.Contains(body, []byte(`"acceptedFinal":`)) {
		t.Fatal("CASE SSE lost or duplicated the public batch or exposed private authority")
	}
}

func asyncCaseClosureJSONV1(t *testing.T, handler http.Handler, method, path string, body map[string]any, want int) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal("invalid HTTP fixture input")
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	req.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("CASE HTTP status=%d want=%d", rec.Code, want)
	}
	var result map[string]any
	if json.Unmarshal(rec.Body.Bytes(), &result) != nil {
		t.Fatal("CASE HTTP did not return JSON")
	}
	return result
}
