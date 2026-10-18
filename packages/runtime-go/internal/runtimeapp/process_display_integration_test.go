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
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	loopapp "analytix.local/runtime-go/internal/app/loop"
	processapp "analytix.local/runtime-go/internal/app/processdisplay"
	"analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainprocess "analytix.local/runtime-go/internal/domain/processdisplay"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
	compositionfixture "analytix.local/runtime-go/internal/formalauthority"
	"analytix.local/runtime-go/internal/server"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

// Production assembly, protected Registry and real asynchronous turn runner.
// HTTP Provider bytes come only from this test's loopback fixture; each root,
// authority and credential is newly created under an isolated qualified root.
func TestRuntimeAppProcessDisplayNativeProducerToProtectedMuxAndPublicIsolation(t *testing.T) {
	// Trace-enabled transport must preserve closed logical metadata before the terminal batch.
	t.Setenv("ANALYTIX_THREAD_TRACE", "1")
	const thinking = "PRIVATE_ASSEMBLED_NATIVE_PROCESS_9471"
	const signature = "PRIVATE_ASSEMBLED_SIGNATURE_7419"
	const secret = "synthetic-runtime-process-key-2948"
	var providerRequests atomic.Int64
	var requestLeak atomic.Bool
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerRequests.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			requestLeak.Store(true)
		}
		for _, forbidden := range []string{thinking, signature, "processDisplay", "displayCapture", "captureDigest", "expandedBlockIds"} {
			if strings.Contains(string(body), forbidden) {
				requestLeak.Store(true)
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, strings.Join([]string{
			`data: {"type":"message_start","message":{"id":"msg_assembled_process","model":"process-native-model","usage":{"input_tokens":4,"output_tokens":0}}}`,
			`data: {"type":"content_block_start","index":2,"content_block":{"type":"thinking","thinking":""}}`,
			`data: {"type":"content_block_delta","index":2,"delta":{"type":"thinking_delta","thinking":"` + thinking + ` api_key=` + secret + `"}}`,
			`data: {"type":"content_block_delta","index":2,"delta":{"type":"signature_delta","signature":"` + signature + `"}}`,
			`data: {"type":"content_block_stop","index":2}`,
			`data: {"type":"content_block_start","index":4,"content_block":{"type":"text","text":""}}`,
			`data: {"type":"content_block_delta","index":4,"delta":{"type":"text_delta","text":"assembled visible answer"}}`,
			`data: {"type":"content_block_stop","index":4}`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`,
			`data: {"type":"message_stop"}`,
		}, "\n\n")+"\n\n")
	}))
	defer fake.Close()
	fixture, err := compositionfixture.New(processAssemblyFixtureRootV2(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fixture.Close)
	if blocker := compositionfixture.SecureConfigurationFilesystemBlocker(fixture.ManifestRoot); blocker != "" {
		t.Skipf("BLOCKED: %s", blocker)
	}
	config := Config{RuntimeToken: DefaultRuntimeToken, ProductionDurableRoot: t.TempDir(), DataDir: fixture.DataDir,
		ProviderID: "process-native", BaseURL: fake.URL + "/v1", APIKey: secret, Model: "process-native-model", EndpointFormat: "messages",
		AuthorityAnchorV1: fixture.AnchorEnvelope, AuthorityManifestRoot: fixture.ManifestRoot,
		AuthorityCredentialProfileRoot: fixture.CredentialProfileRoot, AuthorityCredentialBundleRoot: fixture.CredentialBundleRoot}
	seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, config.ProviderID, config.BaseURL, []string{config.Model}, config.Model, secret, "anthropic-compatible")
	observations := make(chan server.AsyncTurnObservationV1, 32)
	lease, err := AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), asyncTurnObservationContextKeyV1{}, func(observation server.AsyncTurnObservationV1) { observations <- observation })
	inner, err := newRuntimeServerHandlerWithPersistenceLeaseContextE(ctx, config, lease)
	if err != nil {
		_ = lease.Close()
		t.Fatal(err)
	}
	handler := &ownedPersistenceLeaseHandler{Handler: inner, lease: lease}
	defer shutdownOwnedRuntimeHandler(t, handler)
	request := func(method, path string, payload map[string]any, typed bool) (int, []byte) {
		t.Helper()
		body, _ := json.Marshal(payload)
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		r.Header.Set("Content-Type", "application/json")
		if typed {
			r.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if typed && w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("assembled display permitted caching")
		}
		return w.Code, append([]byte(nil), w.Body.Bytes()...)
	}
	status, body := request(http.MethodPost, "/v1/threads", map[string]any{"workspace": workspacetest.New(t), "providerId": config.ProviderID, "model": config.Model}, false)
	var created map[string]any
	if status != http.StatusCreated || json.Unmarshal(body, &created) != nil {
		t.Fatalf("assembled thread status=%d", status)
	}
	threadID := contracts.StringField(created, "id")
	status, body = request(http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{"prompt": "Answer a synthetic ordinary question", "async": true, "providerId": config.ProviderID, "model": config.Model, "approvalPolicy": "never", "sandboxMode": "read-only", "disableUserInput": true}, false)
	var started map[string]any
	if status != http.StatusAccepted || json.Unmarshal(body, &started) != nil {
		t.Fatalf("assembled turn status=%d", status)
	}
	turnID := contracts.StringField(started, "turnId")
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	finished := false
	for !finished {
		select {
		case observation := <-observations:
			if observation.TurnID == turnID && observation.Stage == "finished" {
				if observation.CompletionErrorClass != "none" {
					t.Fatalf("assembled turn completion=%s", observation.CompletionErrorClass)
				}
				finished = true
			}
		case <-deadline.C:
			t.Fatal("assembled process turn observer timeout")
		}
	}
	if providerRequests.Load() != 1 || requestLeak.Load() {
		t.Fatal("display sidecar changed model request or repeated actual Provider send")
	}
	const token = "f01c43bb-35a6-4198-8c1c-6edc8f7b2293"
	const document = "b434da8d-23fd-4ed3-a674-21e88e869f58"
	status, body = request(http.MethodPost, httpapi.ProcessLocalDisplayReadPathV1, map[string]any{"schemaVersion": 2, "action": "authorize", "viewerToken": token, "documentNonce": document, "threadId": threadID, "turnId": turnID}, true)
	var authorization processapp.ResponseV1
	if status != http.StatusOK || json.Unmarshal(body, &authorization) != nil || authorization.Status != "authorized" {
		t.Fatalf("assembled own display authorization status=%d disposition=%s", status, authorization.Status)
	}
	// The text producer also leaves one retired draft control. Neither collapsed
	// nor expanded reads may resurrect its bytes after terminal publication.
	nativeBlock := func(snapshot processapp.ResponseV1) domainprocess.BlockV1 {
		t.Helper()
		if len(snapshot.Blocks) != 2 {
			t.Fatalf("expected native reasoning and one retired draft, got %d blocks", len(snapshot.Blocks))
		}
		var native domainprocess.BlockV1
		nativeCount, draftCount := 0, 0
		for _, block := range snapshot.Blocks {
			switch {
			case block.Kind == "native_reasoning" && block.Source.Kind == "native":
				native, nativeCount = block, nativeCount+1
			case block.Kind == "assistant_draft" && block.Source.Kind == "assistant_draft":
				draftCount++
				if block.State != "unknown" || block.Classification != "withheld" || block.Body != nil || len(block.DraftSegments) != 0 || block.DraftBytes != "" {
					t.Fatal("terminal snapshot retained live assistant draft bytes")
				}
			}
		}
		if nativeCount != 1 || draftCount != 1 {
			t.Fatal("production snapshot changed native/draft source identity")
		}
		return native
	}
	status, body = request(http.MethodPost, httpapi.ProcessLocalDisplayReadPathV1, map[string]any{"schemaVersion": 2, "action": "read", "viewerToken": token}, true)
	var collapsed processapp.ResponseV1
	if status != http.StatusOK || json.Unmarshal(body, &collapsed) != nil || collapsed.Status != "snapshot" {
		t.Fatalf("production mux did not retain collapsed native source block: HTTP=%d status=%s count=%d", status, collapsed.Status, len(collapsed.Blocks))
	}
	if collapsed.SchemaVersion != 2 || collapsed.Scope.ContextEpoch == "" || collapsed.Scope.ProcessIncarnation == "" || len(collapsed.Units) == 0 {
		t.Fatal("production assembly omitted V2 host unit or scope generation")
	}
	collapsedNative := nativeBlock(collapsed)
	if collapsedNative.Body != nil {
		t.Fatal("collapsed native source released its body")
	}
	blockID := collapsedNative.BlockID
	status, body = request(http.MethodPost, httpapi.ProcessLocalDisplayReadPathV1, map[string]any{"schemaVersion": 2, "action": "read", "viewerToken": token, "expandedBlockIds": []string{blockID}}, true)
	var expanded processapp.ResponseV1
	if status != http.StatusOK || json.Unmarshal(body, &expanded) != nil || expanded.Status != "snapshot" {
		t.Fatal("production native expansion unavailable")
	}
	block := nativeBlock(expanded)
	if block.Source.Kind != "native" || block.Source.SourceOrdinal != "1" || block.Source.ProviderSequence != "1" || len(expanded.Units) != 1 || expanded.Units[0].UnitID != block.Source.UnitID || expanded.Units[0].ProducerID != block.Source.ProducerID || expanded.Units[0].Placement != "unit_only" {
		t.Fatal("real native producer did not bind its logical unit and stable source ordinal")
	}
	if block.Kind != "native_reasoning" || block.State != "completed" || block.NativeIndex == nil || *block.NativeIndex != 2 || block.Body == nil || !strings.Contains(block.Body.Text, thinking) || strings.Contains(block.Body.Text, secret) || strings.Contains(string(body), signature) || block.Classification != "credential_redacted" {
		t.Fatal("production pipeline lost native bytes/source identity or released credentials/opaque continuation")
	}
	for _, path := range []string{"/v1/threads/" + threadID, "/v1/threads/" + threadID + "/events?since_seq=0"} {
		status, public := request(http.MethodGet, path, nil, false)
		if status != http.StatusOK {
			t.Fatalf("assembled public read status=%d", status)
		}
		if strings.Contains(path, "/events?") {
			assertTraceEnabledProcessTerminalReplay(t, public, threadID, turnID)
		}
		for _, forbidden := range []string{thinking, signature, secret, "captureDigest", "processDisplay", "displayCapture", blockID, block.CaptureDigest} {
			if forbidden != "" && strings.Contains(string(public), forbidden) {
				t.Fatal("assembled process/private protocol entered history or real SSE replay")
			}
		}
	}
	// A separate real turn rebuilds model history. Observe its actual request,
	// rather than assuming the first pre-capture request proves non-replay.
	status, body = request(http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{"prompt": "Continue with another synthetic ordinary answer", "async": true, "providerId": config.ProviderID, "model": config.Model, "approvalPolicy": "never", "sandboxMode": "read-only", "disableUserInput": true}, false)
	var continued map[string]any
	if status != http.StatusAccepted || json.Unmarshal(body, &continued) != nil {
		t.Fatalf("assembled continuation status=%d", status)
	}
	continuedID := contracts.StringField(continued, "turnId")
	if continuedID == "" || continuedID == turnID {
		t.Fatal("continuation did not create a separate real turn")
	}
	finished = false
	for !finished {
		select {
		case observation := <-observations:
			if observation.TurnID == continuedID && observation.Stage == "finished" {
				if observation.CompletionErrorClass != "none" {
					t.Fatalf("assembled continuation completion=%s", observation.CompletionErrorClass)
				}
				finished = true
			}
		case <-deadline.C:
			t.Fatal("assembled continuation observer timeout")
		}
	}
	if providerRequests.Load() != 2 || requestLeak.Load() {
		t.Fatal("native display or opaque protocol replayed into a fresh real model request")
	}
	// Typed local display authority is required in the production assembled mux.
	status, _ = request(http.MethodPost, httpapi.ProcessLocalDisplayReadPathV1, map[string]any{"schemaVersion": 2, "action": "read", "viewerToken": token}, false)
	if status != http.StatusForbidden {
		t.Fatal("assembled process route accepted generic HTTP authority")
	}
	status, body = request(http.MethodPost, httpapi.ProcessLocalDisplayReadPathV1, map[string]any{"schemaVersion": 2, "action": "revoke", "viewerToken": token}, true)
	if status != http.StatusOK || !bytes.Contains(body, []byte(`"status":"revoked"`)) {
		t.Fatal("assembled native document revocation failed")
	}
	status, body = request(http.MethodPost, httpapi.ProcessLocalDisplayReadPathV1, map[string]any{"schemaVersion": 2, "action": "read", "viewerToken": token, "expandedBlockIds": []string{blockID}}, true)
	if status != http.StatusOK || string(bytes.TrimSpace(body)) != `{"status":"unavailable"}` {
		t.Fatal("revoked native document retained process authority")
	}
}

func assertTraceEnabledProcessTerminalReplay(t *testing.T, replay []byte, threadID, turnID string) {
	t.Helper()
	logical := map[string]string{}
	ordinaryTrace, terminalAnswer := false, false
	for _, line := range strings.Split(string(replay), "\n") {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event) != nil {
			t.Fatal("invalid production SSE replay event")
		}
		kind, stage := contracts.StringField(event, "kind"), contracts.StringField(event, "stage")
		if kind == "pipeline_stage" && (stage == "logical_step_started" || stage == "logical_step_closed") {
			if len(event) != 8 || event["trace"] != nil || event["threadId"] != threadID || event["turnId"] != turnID {
				t.Fatal("trace-enabled production SSE violated the closed logical-step contract")
			}
			details, _ := event["details"].(map[string]any)
			logical[stage] = contracts.StringField(details, "logicalStepId")
		} else if kind == domainevent.GeneralTerminalDeliveryBatchKind {
			if _, err := domainevent.ParseGeneralTerminalDeliveryBatchV1(event); err != nil {
				t.Fatal("trace-enabled production SSE invalidated terminal batch authority")
			}
			if event["trace"] != nil || event["threadId"] != threadID || event["turnId"] != turnID {
				t.Fatal("trace-enabled production SSE changed terminal batch identity")
			}
			nested, _ := event["events"].([]any)
			for _, raw := range nested {
				member, _ := raw.(map[string]any)
				item, _ := member["item"].(map[string]any)
				terminalAnswer = terminalAnswer || member["kind"] == "item_completed" && item["kind"] == "assistant_text" &&
					item["threadId"] == threadID && item["turnId"] == turnID && item["status"] == "completed" &&
					item["text"] == "assembled visible answer"
			}
		} else if trace, ok := event["trace"].(map[string]any); ok {
			ordinaryTrace = ordinaryTrace || trace["sse_sent_at"] != nil && trace["sse_live_emitted_at"] != nil
		}
	}
	if logical["logical_step_started"] == "" || logical["logical_step_started"] != logical["logical_step_closed"] || !ordinaryTrace || !terminalAnswer {
		t.Fatalf("production replay incomplete: logicalPair=%t ordinaryTrace=%t terminalAnswer=%t",
			logical["logical_step_started"] != "" && logical["logical_step_started"] == logical["logical_step_closed"], ordinaryTrace, terminalAnswer)
	}
}

// Initial configuration carries a different opaque key. The protected Registry
// materializes the actual key only during successful attempt preparation. A
// Provider with no native reasoning emits a real Bash call printing that key.
// Retired live seats retain producer identity, but never body or input authority.
func assertRuntimeRetiredProcessSeatV2(t *testing.T, b domainprocess.BlockV1, units []domainprocess.UnitV2, kind, sequence, callID string) {
	t.Helper()
	var source *domainprocess.UnitV2
	for index := range units {
		if units[index].ProviderSequence == sequence {
			if source != nil {
				t.Fatal("duplicate retired-seat provider unit")
			}
			source = &units[index]
		}
	}
	if source == nil || b.BlockID == "" || b.Kind != kind || b.State != "unknown" || b.Source.Kind != kind || b.Source.UnitID != source.UnitID || b.Source.ProducerID != source.ProducerID || b.Source.ProviderSequence != sequence || b.Source.Attempt == 0 || b.Source.CallID != callID {
		t.Fatal("retired live seat lost its exact producer identity")
	}
	if b.Body != nil || b.Input != nil || b.InputUtf16CodeUnits != nil || b.ToolName != "" || b.Title != "" || b.Preview != "" || b.Path != "" || b.CaptureDigest != "" || b.InputDigest != "" || b.RequestURL != "" || len(b.DraftSegments) != 0 || b.DraftBytes != "" || b.Classification != "withheld" {
		t.Fatal("retired live seat retained private body or input")
	}
	if kind == "tool_preparation" {
		ordinal, err := strconv.ParseUint(b.Source.SourceOrdinal, 10, 64)
		if err != nil || ordinal == 0 || len(source.Calls) != 1 || source.Calls[0].CallID != callID || source.Calls[0].ToolCallItemID == "" || b.Source.ToolCallItemID != "" || b.Source.ToolResultItemID != "" || b.Source.StartedEventSeq != "" {
			t.Fatal("retired preparation was detached from its real call or acquired tool authority")
		}
	} else if kind != "assistant_draft" || callID != "" || len(source.Calls) != 0 {
		t.Fatal("retired assistant draft acquired a tool identity")
	}
}

func TestRuntimeAppProcessDisplayPreparedRegistryCredentialReachesShellRedaction(t *testing.T) {
	const preparedKey = "r7Tb9W1x3Y5z8A0c2D4f6G9h1J3k5M7n0P2q4R6t8"
	const initialKey = "d9Fg1H3j5K7m0N2p4Q6r8T1v3W5x7Y9z2A4c6D8f0"
	var sends atomic.Int64
	var invalidAuthorization atomic.Bool
	var displayCopyInRequest atomic.Bool
	arguments, _ := json.Marshal(map[string]any{"command": "printf '%s\\n' '" + preparedKey + "'"})
	encodedArguments, _ := json.Marshal(string(arguments))
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := sends.Add(1)
		if r.Header.Get("x-api-key") != preparedKey {
			invalidAuthorization.Store(true)
		}
		requestBody, err := io.ReadAll(r.Body)
		if err != nil {
			displayCopyInRequest.Store(true)
		}
		for _, forbidden := range []string{"processDisplay", "displayCapture", "captureDigest", "expandedBlockIds"} {
			if strings.Contains(string(requestBody), forbidden) {
				displayCopyInRequest.Store(true)
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		frames := []string{`data: {"type":"message_start","message":{"id":"msg_prepared_shell","model":"process-prepared-model","usage":{"input_tokens":4,"output_tokens":0}}}`}
		if call == 1 {
			frames = append(frames,
				`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"wire_prepared_shell","name":"bash","input":{}}}`,
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":`+string(encodedArguments)+`}}`,
				`data: {"type":"content_block_stop","index":0}`,
				`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":3}}`)
		} else {
			frames = append(frames,
				`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"prepared shell completed"}}`,
				`data: {"type":"content_block_stop","index":0}`,
				`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`)
		}
		frames = append(frames, `data: {"type":"message_stop"}`)
		_, _ = io.WriteString(w, strings.Join(frames, "\n\n")+"\n\n")
	}))
	defer fake.Close()
	fixture, err := compositionfixture.New(processAssemblyFixtureRootV2(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fixture.Close)
	if blocker := compositionfixture.SecureConfigurationFilesystemBlocker(fixture.ManifestRoot); blocker != "" {
		t.Skipf("BLOCKED: %s", blocker)
	}
	config := Config{RuntimeToken: DefaultRuntimeToken, ProductionDurableRoot: t.TempDir(), DataDir: fixture.DataDir,
		ProviderID: "process-prepared", BaseURL: fake.URL + "/v1", APIKey: initialKey, Model: "process-prepared-model", EndpointFormat: "messages",
		AuthorityAnchorV1: fixture.AnchorEnvelope, AuthorityManifestRoot: fixture.ManifestRoot,
		AuthorityCredentialProfileRoot: fixture.CredentialProfileRoot, AuthorityCredentialBundleRoot: fixture.CredentialBundleRoot}
	seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, config.ProviderID, config.BaseURL, []string{config.Model}, config.Model, preparedKey, "anthropic-compatible")
	observations := make(chan server.AsyncTurnObservationV1, 32)
	lease, err := AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), asyncTurnObservationContextKeyV1{}, func(observation server.AsyncTurnObservationV1) { observations <- observation })
	inner, err := newRuntimeServerHandlerWithPersistenceLeaseContextE(ctx, config, lease)
	if err != nil {
		_ = lease.Close()
		t.Fatal(err)
	}
	handler := &ownedPersistenceLeaseHandler{Handler: inner, lease: lease}
	defer shutdownOwnedRuntimeHandler(t, handler)
	request := func(method, path string, payload map[string]any, typed bool) (int, []byte) {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		r.Header.Set("Content-Type", "application/json")
		if typed {
			r.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if typed && w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("prepared-key display permitted caching")
		}
		return w.Code, append([]byte(nil), w.Body.Bytes()...)
	}
	status, body := request(http.MethodPost, "/v1/threads", map[string]any{"workspace": workspacetest.New(t), "providerId": config.ProviderID, "model": config.Model}, false)
	var created map[string]any
	if status != http.StatusCreated || json.Unmarshal(body, &created) != nil {
		t.Fatalf("prepared-key thread status=%d", status)
	}
	threadID := contracts.StringField(created, "id")
	status, body = request(http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{"prompt": "Run a synthetic shell command and report completion", "async": true, "providerId": config.ProviderID, "model": config.Model, "approvalPolicy": "auto", "sandboxMode": "danger-full-access", "disableUserInput": true, "maxModelSteps": 3}, false)
	var started map[string]any
	if status != http.StatusAccepted || json.Unmarshal(body, &started) != nil {
		t.Fatalf("prepared-key turn status=%d", status)
	}
	turnID := contracts.StringField(started, "turnId")
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	finished := false
	for !finished {
		select {
		case observation := <-observations:
			if observation.TurnID == turnID && observation.Stage == "finished" {
				if observation.CompletionErrorClass != "none" {
					t.Fatalf("prepared-key completion=%s", observation.CompletionErrorClass)
				}
				finished = true
			}
		case <-deadline.C:
			t.Fatal("prepared-key actual turn observer timeout")
		}
	}
	if sends.Load() != 2 || invalidAuthorization.Load() || displayCopyInRequest.Load() {
		t.Fatal("protected Registry did not materialize exact prepared HTTP authority or display altered model requests")
	}
	const token = "ad388cff-aa0b-437c-a0ab-24a0cf3f486c"
	const document = "6b73c331-e4d0-4d7a-81fa-a638d3b4aaec"
	status, body = request(http.MethodPost, httpapi.ProcessLocalDisplayReadPathV1, map[string]any{"schemaVersion": 2, "action": "authorize", "viewerToken": token, "documentNonce": document, "threadId": threadID, "turnId": turnID}, true)
	var authorized processapp.ResponseV1
	if status != http.StatusOK || json.Unmarshal(body, &authorized) != nil || authorized.Status != "authorized" {
		t.Fatal("actual prepared-key tool display authorization failed")
	}
	status, body = request(http.MethodPost, httpapi.ProcessLocalDisplayReadPathV1, map[string]any{"schemaVersion": 2, "action": "read", "viewerToken": token}, true)
	var collapsed processapp.ResponseV1
	if status != http.StatusOK || json.Unmarshal(body, &collapsed) != nil || collapsed.Status != "snapshot" || len(collapsed.Blocks) != 3 || collapsed.Blocks[1].Body != nil || collapsed.Blocks[1].Kind != "shell" {
		t.Fatal("no-native Registry sequence did not preserve its actual Shell block")
	}
	if collapsed.SchemaVersion != 2 || collapsed.Scope.ContextEpoch == "" || collapsed.Scope.ProcessIncarnation == "" || len(collapsed.Units) == 0 {
		t.Fatal("production assembly omitted V2 host unit or scope generation")
	}
	if len(collapsed.Units) != 2 {
		t.Fatal("Registry sequence duplicated actual provider units")
	}
	assertRuntimeRetiredProcessSeatV2(t, collapsed.Blocks[0], collapsed.Units, "tool_preparation", "1", collapsed.Blocks[1].Source.CallID)
	assertRuntimeRetiredProcessSeatV2(t, collapsed.Blocks[2], collapsed.Units, "assistant_draft", "2", "")
	blockID := collapsed.Blocks[1].BlockID
	status, body = request(http.MethodPost, httpapi.ProcessLocalDisplayReadPathV1, map[string]any{"schemaVersion": 2, "action": "read", "viewerToken": token, "expandedBlockIds": []string{blockID}}, true)
	var expanded processapp.ResponseV1
	if status != http.StatusOK || json.Unmarshal(body, &expanded) != nil || expanded.Status != "snapshot" || len(expanded.Blocks) != 3 {
		t.Fatal("prepared Registry Shell expansion failed")
	}
	for index := range expanded.Blocks {
		if expanded.Blocks[index].BlockID != collapsed.Blocks[index].BlockID || expanded.Blocks[index].Source != collapsed.Blocks[index].Source {
			t.Fatal("expansion replaced real process seats")
		}
	}
	assertRuntimeRetiredProcessSeatV2(t, expanded.Blocks[0], expanded.Units, "tool_preparation", "1", expanded.Blocks[1].Source.CallID)
	assertRuntimeRetiredProcessSeatV2(t, expanded.Blocks[2], expanded.Units, "assistant_draft", "2", "")
	block := expanded.Blocks[1]
	var sourceUnit *domainprocess.UnitV2
	for i := range expanded.Units {
		if expanded.Units[i].UnitID == block.Source.UnitID {
			sourceUnit = &expanded.Units[i]
		}
	}
	if block.Source.Kind != "tool" || block.Source.CallID == "" || block.Source.ToolCallItemID == "" || block.Source.ToolResultItemID == "" || block.Source.StartedEventSeq == "" || sourceUnit == nil || sourceUnit.ProviderSequence != "1" || sourceUnit.Placement != "anchored" || len(sourceUnit.Calls) != 1 || sourceUnit.Calls[0].CallID != block.Source.CallID || sourceUnit.Calls[0].ToolCallItemID != block.Source.ToolCallItemID || sourceUnit.Calls[0].ToolResultItemID != block.Source.ToolResultItemID || sourceUnit.Calls[0].StartedEventSeq != block.Source.StartedEventSeq {
		t.Fatal("actual Bash settlement did not preserve its real persisted call/result and committed started anchor")
	}
	if block.State != "completed" || block.Body == nil || block.Body.Text != "<redacted>\n" || block.Classification != "credential_redacted" || strings.Contains(string(body), preparedKey) || strings.Contains(string(body), initialKey) {
		t.Fatal("bare actual prepared Registry key survived Shell process projection")
	}
	for _, path := range []string{"/v1/threads/" + threadID, "/v1/threads/" + threadID + "/events?since_seq=0"} {
		status, public := request(http.MethodGet, path, nil, false)
		if status != http.StatusOK {
			t.Fatalf("prepared-key public status=%d", status)
		}
		for _, forbidden := range []string{preparedKey, initialKey, "displayCapture", "processDisplay", blockID, block.CaptureDigest} {
			if forbidden != "" && strings.Contains(string(public), forbidden) {
				t.Fatal("prepared Registry tool/display entered generic history or SSE")
			}
		}
	}
	shutdownOwnedRuntimeHandler(t, handler)
	status, body = request(http.MethodPost, httpapi.ProcessLocalDisplayReadPathV1, map[string]any{"schemaVersion": 2, "action": "read", "viewerToken": token, "expandedBlockIds": []string{blockID}}, true)
	if status != http.StatusOK || strings.TrimSpace(string(body)) != `{"status":"unavailable"}` {
		t.Fatal("owned runtime shutdown left a process preview lease readable")
	}
	status, body = request(http.MethodPost, httpapi.ProcessLocalDisplayTargetPathV1, map[string]any{"schemaVersion": 2, "action": "open_capture", "viewerToken": token, "blockId": blockID, "captureDigest": block.CaptureDigest}, true)
	if status != http.StatusOK || strings.TrimSpace(string(body)) != `{"status":"unavailable"}` {
		t.Fatal("owned runtime shutdown left private process capture readable")
	}
}

// Only synthetic secure configuration/state lives here. Build/compiler caches
// retain the official helper paths; the production filesystem gate is unchanged.
func processAssemblyFixtureRootV2(t *testing.T) string {
	t.Helper()
	parent := os.TempDir()
	if runtime.GOOS == "darwin" {
		parent = "/private/tmp"
	}
	root, err := os.MkdirTemp(parent, "analytix-process-correlation-v2-")
	if err != nil {
		t.Fatal("isolated production fixture root unavailable")
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error("isolated production fixture cleanup failed")
		}
	})
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal("isolated production fixture permissions unavailable")
	}
	if blocker := compositionfixture.SecureConfigurationFilesystemBlocker(root); blocker != "" {
		t.Skipf("BLOCKED: %s", blocker)
	}
	return root
}

func TestRuntimeAppProcessDisplayReadApprovedWriteRealSettlementAnchorsV2(t *testing.T) {
	const before = "synthetic anchor input\n"
	const after = "synthetic anchor approved output\n"
	const thinking = "synthetic complete native anchor reasoning"
	const secret = "synthetic-anchor-registry-key"
	secondEntered, releaseSecond := make(chan struct{}), make(chan struct{})
	thirdEntered, releaseThird := make(chan struct{}), make(chan struct{})
	var releaseSecondOnce, releaseThirdOnce sync.Once
	release := func() {
		releaseSecondOnce.Do(func() { close(releaseSecond) })
		releaseThirdOnce.Do(func() { close(releaseThird) })
	}
	var sends atomic.Int64
	var requestViolation atomic.Bool
	readArguments, _ := json.Marshal(map[string]any{"path": "anchor.txt"})
	writeArguments, _ := json.Marshal(map[string]any{"path": "anchor.txt", "content": after})
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := sends.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil || r.Header.Get("x-api-key") != secret {
			requestViolation.Store(true)
		}
		for _, forbidden := range []string{"processDisplay", "displayCapture", "captureDigest", "expandedBlockIds", "processIncarnation"} {
			if strings.Contains(string(body), forbidden) {
				requestViolation.Store(true)
			}
		}
		if call == 2 {
			close(secondEntered)
			select {
			case <-releaseSecond:
			case <-r.Context().Done():
				return
			}
		} else if call == 3 {
			close(thirdEntered)
			select {
			case <-releaseThird:
			case <-r.Context().Done():
				return
			}
		} else if call != 1 {
			requestViolation.Store(true)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		frames := []string{`data: {"type":"message_start","message":{"id":"msg_actual_anchor","model":"anchor-model","usage":{"input_tokens":4,"output_tokens":0}}}`}
		if call <= 2 {
			tool, wireID, arguments := "read", "wire_anchor_read", readArguments
			if call == 2 {
				tool, wireID, arguments = "write", "wire_anchor_write", writeArguments
				frames = append(frames,
					`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
					`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"`+thinking+`"}}`,
					`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"synthetic-anchor-signature"}}`,
					`data: {"type":"content_block_stop","index":0}`)
			}
			encodedArguments, _ := json.Marshal(string(arguments))
			frames = append(frames,
				`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"`+wireID+`","name":"`+tool+`","input":{}}}`,
				`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":`+string(encodedArguments)+`}}`,
				`data: {"type":"content_block_stop","index":1}`,
				`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":3}}`)
		} else {
			frames = append(frames,
				`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"synthetic approved completion"}}`,
				`data: {"type":"content_block_stop","index":0}`,
				`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`)
		}
		frames = append(frames, `data: {"type":"message_stop"}`)
		_, _ = io.WriteString(w, strings.Join(frames, "\n\n")+"\n\n")
	}))
	defer func() { release(); fake.Close() }()
	fixture, err := compositionfixture.New(processAssemblyFixtureRootV2(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fixture.Close)
	config := Config{RuntimeToken: DefaultRuntimeToken, ProductionDurableRoot: t.TempDir(), DataDir: fixture.DataDir,
		ProviderID: "anchor-provider", BaseURL: fake.URL + "/v1", APIKey: secret, Model: "anchor-model", EndpointFormat: "messages",
		AuthorityAnchorV1: fixture.AnchorEnvelope, AuthorityManifestRoot: fixture.ManifestRoot,
		AuthorityCredentialProfileRoot: fixture.CredentialProfileRoot, AuthorityCredentialBundleRoot: fixture.CredentialBundleRoot}
	seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, config.ProviderID, config.BaseURL, []string{config.Model}, config.Model, secret, "anthropic-compatible")
	observations := make(chan server.AsyncTurnObservationV1, 32)
	lease, err := AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), asyncTurnObservationContextKeyV1{}, func(o server.AsyncTurnObservationV1) { observations <- o })
	inner, err := newRuntimeServerHandlerWithPersistenceLeaseContextE(ctx, config, lease)
	if err != nil {
		_ = lease.Close()
		t.Fatal(err)
	}
	handler := &ownedPersistenceLeaseHandler{Handler: inner, lease: lease}
	defer func() { release(); shutdownOwnedRuntimeHandler(t, handler) }()
	request := func(method, path string, payload map[string]any, typed bool) (int, []byte) {
		encoded, _ := json.Marshal(payload)
		r := httptest.NewRequest(method, path, bytes.NewReader(encoded))
		r.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		r.Header.Set("Content-Type", "application/json")
		if typed {
			r.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if typed && w.Header().Get("Cache-Control") != "no-store" {
			t.Error("typed anchor response permitted caching")
		}
		return w.Code, append([]byte(nil), w.Body.Bytes()...)
	}
	wait := func(ch <-chan struct{}, phase string) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(20 * time.Second):
			t.Fatalf("actual %s barrier timeout", phase)
		}
	}
	workspace := workspacetest.New(t)
	target := filepath.Join(workspace, "anchor.txt")
	if err := os.WriteFile(target, []byte(before), 0600); err != nil {
		t.Fatal(err)
	}
	status, body := request(http.MethodPost, "/v1/threads", map[string]any{"workspace": workspace, "providerId": config.ProviderID, "model": config.Model}, false)
	var created map[string]any
	if status != http.StatusCreated || json.Unmarshal(body, &created) != nil {
		t.Fatalf("anchor thread status=%d", status)
	}
	threadID := contracts.StringField(created, "id")
	status, body = request(http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{"prompt": "Read the synthetic file, then write its approved replacement and finish", "async": true, "providerId": config.ProviderID, "model": config.Model, "approvalPolicy": "on-request", "sandboxMode": "workspace-write", "disableUserInput": false, "maxModelSteps": 4}, false)
	var started map[string]any
	if status != http.StatusAccepted || json.Unmarshal(body, &started) != nil {
		t.Fatalf("anchor turn status=%d", status)
	}
	turnID := contracts.StringField(started, "turnId")
	wait(secondEntered, "Read-settled")
	const token = "2f2d9174-779b-4b8f-a405-c2757b5446fd"
	const document = "a13dce14-d2b0-4d0c-812b-753029aa3de7"
	status, body = request(http.MethodPost, httpapi.ProcessLocalDisplayReadPathV1, map[string]any{"schemaVersion": 2, "action": "authorize", "viewerToken": token, "documentNonce": document, "threadId": threadID, "turnId": turnID}, true)
	var authorized processapp.ResponseV1
	if status != http.StatusOK || json.Unmarshal(body, &authorized) != nil || authorized.Status != "authorized" {
		t.Fatal("actual current anchor pane denied")
	}
	snapshot := func(phase string) processapp.ResponseV1 {
		t.Helper()
		status, body := request(http.MethodPost, httpapi.ProcessLocalDisplayReadPathV1, map[string]any{"schemaVersion": 2, "action": "read", "viewerToken": token, "expandedBlockIds": []string{}}, true)
		var value processapp.ResponseV1
		if status != http.StatusOK || json.Unmarshal(body, &value) != nil || value.Status != "snapshot" {
			t.Fatalf("actual %s snapshot unavailable", phase)
		}
		for _, b := range value.Blocks {
			if b.Body != nil {
				t.Fatal("empty expansion delivered a body")
			}
		}
		t.Logf("phase=%s revision=%s units=%d blocks=%d bodies=0", phase, value.Revision, len(value.Units), len(value.Blocks))
		return value
	}
	unit := func(value processapp.ResponseV1, seq string) domainprocess.UnitV2 {
		t.Helper()
		for _, u := range value.Units {
			if u.ProviderSequence == seq {
				return u
			}
		}
		t.Fatal("actual provider unit missing")
		return domainprocess.UnitV2{}
	}
	readPhase := snapshot("Read-settled")
	readUnit := unit(readPhase, "1")
	if len(readPhase.Blocks) != 1 || len(readUnit.Calls) != 1 || readUnit.Calls[0].ToolResultItemID == "" || readUnit.Calls[0].StartedEventSeq == "" {
		t.Fatal("capture-less actual Read settlement anchor missing")
	}
	assertRuntimeRetiredProcessSeatV2(t, readPhase.Blocks[0], readPhase.Units, "tool_preparation", "1", readUnit.Calls[0].CallID)
	releaseSecondOnce.Do(func() { close(releaseSecond) })
	select {
	case o := <-observations:
		for o.TurnID != turnID || o.Stage != "finished" {
			select {
			case o = <-observations:
			case <-time.After(20 * time.Second):
				t.Fatal("actual approval pause timeout")
			}
		}
		if o.CompletionErrorClass != "none" {
			t.Fatalf("actual approval pause completion=%s", o.CompletionErrorClass)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("actual approval pause observer timeout")
	}
	status, body = request(http.MethodGet, "/v1/threads/"+threadID, nil, false)
	var pausedThread map[string]any
	if status != http.StatusOK || json.Unmarshal(body, &pausedThread) != nil {
		t.Fatal("actual paused primary projection unavailable")
	}
	var approvalID string
	turns, _ := pausedThread["turns"].([]any)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if contracts.StringField(turn, "id") != turnID {
			continue
		}
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if contracts.StringField(item, "kind") == "approval" && contracts.StringField(item, "status") == "pending" {
				if approvalID != "" {
					t.Fatal("duplicate actual pending approval")
				}
				approvalID = contracts.StringField(item, "approvalId")
			}
		}
	}
	if approvalID == "" || sends.Load() != 2 {
		t.Fatal("actual Write did not pause for normal approval")
	}
	paused := snapshot("Write-awaiting-Allow")
	second := unit(paused, "2")
	if len(paused.Blocks) != 3 || paused.Blocks[1].Source.Kind != "native" || paused.Blocks[1].Source.UnitID != second.UnitID || len(second.PrecedingCalls) != 1 || second.PrecedingCalls[0] != readUnit.Calls[0] || len(second.Calls) != 1 || second.Calls[0].ToolResultItemID != "" {
		t.Fatal("actual paused native unit does not preserve Read boundary or prematurely settles Write")
	}
	if paused.Blocks[0].BlockID != readPhase.Blocks[0].BlockID || paused.Blocks[0].Source != readPhase.Blocks[0].Source {
		t.Fatal("approval pause replaced the real Read preparation seat")
	}
	assertRuntimeRetiredProcessSeatV2(t, paused.Blocks[0], paused.Units, "tool_preparation", "1", readUnit.Calls[0].CallID)
	assertRuntimeRetiredProcessSeatV2(t, paused.Blocks[2], paused.Units, "tool_preparation", "2", second.Calls[0].CallID)
	if actual, err := os.ReadFile(target); err != nil || string(actual) != before {
		t.Fatal("Write effected before actual Allow")
	}
	approvalDone := make(chan int, 1)
	go func() {
		status, _ := request(http.MethodPost, "/v1/approvals/"+approvalID, map[string]any{"decision": "allow"}, false)
		approvalDone <- status
	}()
	wait(thirdEntered, "approved-Write-settled")
	writePhase := snapshot("Write-settled-before-final")
	var writeBlock domainprocess.BlockV1
	for _, b := range writePhase.Blocks {
		if b.Kind == "write" {
			if writeBlock.BlockID != "" {
				t.Fatal("duplicate actual Write capture")
			}
			writeBlock = b
		}
	}
	writeUnit := unit(writePhase, "2")
	if writePhase.Revision == paused.Revision || len(writePhase.Blocks) != 4 || writeBlock.BlockID == "" || writeBlock.State != "completed" || writeBlock.EffectApplied == nil || !*writeBlock.EffectApplied || writeBlock.Source.Kind != "tool" || len(writeUnit.Calls) != 1 || writeBlock.Source.CallID != writeUnit.Calls[0].CallID || writeBlock.Source.ToolCallItemID != writeUnit.Calls[0].ToolCallItemID || writeBlock.Source.ToolResultItemID == "" || writeBlock.Source.ToolResultItemID != writeUnit.Calls[0].ToolResultItemID || writeBlock.Source.StartedEventSeq == "" || writeBlock.Source.StartedEventSeq != writeUnit.Calls[0].StartedEventSeq {
		t.Fatal("actual approved Write capture lacks settled exact identity")
	}
	assertRuntimeRetiredProcessSeatV2(t, writePhase.Blocks[0], writePhase.Units, "tool_preparation", "1", readUnit.Calls[0].CallID)
	assertRuntimeRetiredProcessSeatV2(t, writePhase.Blocks[2], writePhase.Units, "tool_preparation", "2", writeUnit.Calls[0].CallID)
	for index := range paused.Blocks {
		if writePhase.Blocks[index].BlockID != paused.Blocks[index].BlockID || writePhase.Blocks[index].Source != paused.Blocks[index].Source {
			t.Fatal("Write settlement replaced an existing process seat")
		}
	}
	if actual, err := os.ReadFile(target); err != nil || string(actual) != after {
		t.Fatal("approved actual Write effect missing")
	}
	releaseThirdOnce.Do(func() { close(releaseThird) })
	select {
	case status := <-approvalDone:
		if status != http.StatusOK {
			t.Fatalf("actual Allow completion status=%d", status)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("actual approved continuation timeout")
	}
	final := snapshot("final")
	finalUnit := unit(final, "2")
	if len(final.Blocks) != 5 || len(finalUnit.Calls) != 1 || len(finalUnit.PrecedingCalls) != 1 || finalUnit.Calls[0] != writeUnit.Calls[0] || finalUnit.PrecedingCalls[0] != readUnit.Calls[0] {
		t.Fatal("final continuation replaced or duplicated real anchors")
	}
	for index := range writePhase.Blocks {
		if final.Blocks[index].BlockID != writePhase.Blocks[index].BlockID || final.Blocks[index].Source != writePhase.Blocks[index].Source {
			t.Fatal("final continuation replaced or reordered the settled process prefix")
		}
	}
	assertRuntimeRetiredProcessSeatV2(t, final.Blocks[0], final.Units, "tool_preparation", "1", readUnit.Calls[0].CallID)
	assertRuntimeRetiredProcessSeatV2(t, final.Blocks[2], final.Units, "tool_preparation", "2", writeUnit.Calls[0].CallID)
	assertRuntimeRetiredProcessSeatV2(t, final.Blocks[4], final.Units, "assistant_draft", "3", "")
	for _, b := range final.Blocks {
		if b.Kind == "write" && (b.BlockID != writeBlock.BlockID || b.Source != writeBlock.Source) {
			t.Fatal("final continuation moved settled Write source")
		}
	}
	status, body = request(http.MethodGet, "/v1/threads/"+threadID, nil, false)
	var finalThread map[string]any
	if status != http.StatusOK || json.Unmarshal(body, &finalThread) != nil {
		t.Fatal("actual final public primary unavailable")
	}
	actualCalls, actualResults := map[string]string{}, map[string]string{}
	turns, _ = finalThread["turns"].([]any)
	terminal := false
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if contracts.StringField(turn, "id") != turnID {
			continue
		}
		terminal = contracts.StringField(turn, "status") == "completed"
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			callID := contracts.StringField(item, "callId")
			switch contracts.StringField(item, "kind") {
			case "tool_call":
				if actualCalls[callID] != "" {
					t.Fatal("duplicate public call identity")
				}
				actualCalls[callID] = contracts.StringField(item, "id")
			case "tool_result":
				if actualResults[callID] != "" {
					t.Fatal("duplicate public result identity")
				}
				actualResults[callID] = contracts.StringField(item, "id")
			}
		}
	}
	if !terminal || len(actualCalls) != 2 || len(actualResults) != 2 || sends.Load() != 3 || requestViolation.Load() {
		t.Fatal("actual approved turn/Provider sequence did not complete exactly once")
	}
	status, events := request(http.MethodGet, "/v1/threads/"+threadID+"/events?since_seq=0", nil, false)
	if status != http.StatusOK {
		t.Fatal("actual started event replay unavailable")
	}
	startedSeqs := map[string]string{}
	for _, line := range strings.Split(string(events), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event map[string]any
		if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) != nil || contracts.StringField(event, "kind") != "tool_call_started" {
			continue
		}
		item, _ := event["item"].(map[string]any)
		seq, ok := contracts.NumericSeq(event["seq"])
		if ok && seq > 0 {
			startedSeqs[contracts.StringField(item, "callId")] = strconv.Itoa(seq)
		}
	}
	for _, anchor := range []domainprocess.CallAnchorV2{readUnit.Calls[0], writeUnit.Calls[0]} {
		if actualCalls[anchor.CallID] != anchor.ToolCallItemID || actualResults[anchor.CallID] != anchor.ToolResultItemID || startedSeqs[anchor.CallID] != anchor.StartedEventSeq {
			t.Fatal("anchor was not bound to real persisted primary and committed started event")
		}
	}
	for _, public := range []string{string(body), string(events)} {
		for _, forbidden := range []string{thinking, "synthetic-anchor-signature", before, after, "processDisplay", "displayCapture", writeBlock.BlockID, writeBlock.CaptureDigest, paused.Blocks[1].BlockID, paused.Blocks[1].CaptureDigest} {
			if forbidden != "" && strings.Contains(public, forbidden) {
				t.Fatal("private process copy entered public history/SSE")
			}
		}
	}
}

// Uses the existing production assembly and real SSE replay, with a synthetic
// loopback Provider only. Replaying observations cannot create another effect.
func TestRuntimeAppRetryAndOutputCapLifecycleThroughProductionSSEReplay(t *testing.T) {
	for _, scenario := range []string{"provider_retry", "transport_reconnect", "output_cap"} {
		t.Run(scenario, func(t *testing.T) {
			const secret = "synthetic-retry-registry-key-5612"
			const private = "PRIVATE_RETRY_NATIVE_THOUGHT_9517"
			const cappedText = "DISCARDED_CAP_TEXT_3816"
			expectedCalls := int64(2)
			if scenario == "output_cap" {
				expectedCalls = 1
			}
			var calls atomic.Int64
			fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				if r.URL.Path != "/v1/responses" {
					t.Error("Registry selected a different fixture protocol")
				}
				body, _ := io.ReadAll(r.Body)
				if bytes.Contains(body, []byte(private)) || bytes.Contains(body, []byte(cappedText)) {
					t.Error("failed candidate reentered Provider request")
				}
				if call == 1 && scenario == "provider_retry" {
					w.Header().Set("Retry-After", "1")
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = io.WriteString(w, `{"error":"PRIVATE_RETRY_RESPONSE_BODY"}`)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if call == 1 && scenario == "transport_reconnect" {
					_, _ = io.WriteString(w, "data: {}\n\n")
					return
				}
				if call == 1 && scenario == "output_cap" {
					_, _ = io.WriteString(w, `data: {"type":"response.reasoning_text.delta","delta":"`+private+`"}`+"\n\n"+`data: {"type":"response.output_text.delta","delta":"`+cappedText+`"}`+"\n\n"+`data: {"type":"response.incomplete","response":{"incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":7,"output_tokens":9,"total_tokens":16}}}`+"\n\n")
					return
				}
				_, _ = io.WriteString(w, `data: {"type":"response.output_text.delta","delta":"complete synthetic answer"}`+"\n\n"+`data: {"type":"response.completed","response":{"usage":{"input_tokens":4,"output_tokens":3,"total_tokens":7}}}`+"\n\n")
			}))
			defer fake.Close()
			fixture, err := compositionfixture.New(processAssemblyFixtureRootV2(t))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(fixture.Close)
			if blocker := compositionfixture.SecureConfigurationFilesystemBlocker(fixture.ManifestRoot); blocker != "" {
				t.Skipf("BLOCKED: %s", blocker)
			}
			config := Config{RuntimeToken: DefaultRuntimeToken, ProductionDurableRoot: t.TempDir(), DataDir: fixture.DataDir, ProviderID: "retry-fixture", BaseURL: fake.URL + "/v1", APIKey: secret, Model: "retry-fixture-model", EndpointFormat: "responses", AuthorityAnchorV1: fixture.AnchorEnvelope, AuthorityManifestRoot: fixture.ManifestRoot, AuthorityCredentialProfileRoot: fixture.CredentialProfileRoot, AuthorityCredentialBundleRoot: fixture.CredentialBundleRoot}
			seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, config.ProviderID, config.BaseURL, []string{config.Model}, config.Model, secret, "openai-responses")
			observations := make(chan server.AsyncTurnObservationV1, 32)
			lease, err := AcquireRuntimePersistenceLease(config)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.WithValue(context.Background(), asyncTurnObservationContextKeyV1{}, func(o server.AsyncTurnObservationV1) { observations <- o })
			inner, err := newRuntimeServerHandlerWithPersistenceLeaseContextE(ctx, config, lease)
			if err != nil {
				_ = lease.Close()
				t.Fatal(err)
			}
			handler := &ownedPersistenceLeaseHandler{Handler: inner, lease: lease}
			defer shutdownOwnedRuntimeHandler(t, handler)
			request := func(method, path string, payload map[string]any) (int, []byte) {
				t.Helper()
				data, _ := json.Marshal(payload)
				req := httptest.NewRequest(method, path, bytes.NewReader(data))
				req.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				return rec.Code, append([]byte(nil), rec.Body.Bytes()...)
			}
			status, body := request(http.MethodPost, "/v1/threads", map[string]any{"workspace": workspacetest.New(t), "providerId": config.ProviderID, "model": config.Model})
			var created map[string]any
			if status != http.StatusCreated || json.Unmarshal(body, &created) != nil {
				t.Fatalf("create=%d", status)
			}
			threadID := contracts.StringField(created, "id")
			status, body = request(http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{"prompt": "synthetic ordinary retry question", "async": true, "providerId": config.ProviderID, "model": config.Model, "approvalPolicy": "never", "sandboxMode": "read-only", "disableUserInput": true})
			var started map[string]any
			if status != http.StatusAccepted || json.Unmarshal(body, &started) != nil {
				t.Fatalf("start=%d", status)
			}
			turnID := contracts.StringField(started, "turnId")
			deadline := time.NewTimer(20 * time.Second)
			defer deadline.Stop()
			finished := false
			for !finished {
				select {
				case o := <-observations:
					if o.TurnID == turnID && o.Stage == "finished" {
						if o.CompletionErrorClass != "none" {
							t.Fatalf("completion=%s", o.CompletionErrorClass)
						}
						finished = true
					}
				case <-deadline.C:
					t.Fatal("production retry observer timed out")
				}
			}
			if calls.Load() != expectedCalls {
				t.Fatalf("unexpected actual Provider calls=%d", calls.Load())
			}
			var replay []byte
			for i := 0; i < 2; i++ {
				status, body = request(http.MethodGet, "/v1/threads/"+threadID+"/events?since_seq=0", nil)
				if status != http.StatusOK {
					t.Fatalf("replay=%d", status)
				}
				if i == 0 {
					replay = body
				} else if !bytes.Equal(replay, body) {
					t.Fatal("SSE replay changed settled observations")
				}
				if calls.Load() != expectedCalls {
					t.Fatal("replay restarted Provider/timer")
				}
			}
			for _, forbidden := range []string{private, secret, "PRIVATE_RETRY_RESPONSE_BODY"} {
				if bytes.Contains(replay, []byte(forbidden)) {
					t.Fatal("public replay leaked private candidate or extra tool effect")
				}
			}
			if scenario != "output_cap" && bytes.Contains(replay, []byte(cappedText)) {
				t.Fatal("ordinary retry replay acquired capped candidate text")
			}
			phases := []string{}
			pairs := 0
			caps := 0
			capSeq := float64(0)
			recoverySeen := false
			for _, line := range strings.Split(string(replay), "\n") {
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				var event map[string]any
				if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) != nil {
					continue
				}
				if kind, _ := event["kind"].(string); strings.HasPrefix(kind, "tool_call_") {
					t.Fatal("retry or cap created an extra tool effect")
				}
				details, _ := event["details"].(map[string]any)
				if event["stage"] == "pre_send" {
					pairs++
				}
				if event["stage"] == "provider_retrying" {
					if details["visibleRecovery"] == true {
						recoverySeen = true
						if caps != 1 {
							t.Fatal("recovery preceded cap observation")
						}
						if details["retry"] != nil {
							t.Fatal("visible recovery invented phase")
						}
					} else {
						retry, ok := details["retry"].(map[string]any)
						if !ok {
							t.Fatal("production retry lacks structured lifecycle")
						}
						if retry["source"] != scenario || retry["logicalSequence"] != float64(1) {
							t.Fatal("retry source/sequence was fabricated")
						}
						phases = append(phases, retry["phase"].(string))
						if event["timestamp"] == nil {
							t.Fatal("retry lacks owner timestamp")
						}
					}
				}
				if event["stage"] == "response_received" && details["stopReason"] == "length" {
					caps++
					capSeq, _ = details["logicalSequence"].(float64)
					if details["streamCompleted"] != false || details["toolCallCount"] != float64(0) {
						t.Fatal("cap authorized completion/tool output")
					}
				}
			}
			if int64(pairs) != expectedCalls {
				t.Fatalf("durable sends=%d", pairs)
			}
			if scenario == "output_cap" {
				if caps != 1 || capSeq != 1 || recoverySeen || len(phases) != 0 {
					t.Fatalf("cap/recovery facts=%d seq=%v recovery=%v phases=%v", caps, capSeq, recoverySeen, phases)
				}
			} else if strings.Join(phases, ",") != "scheduled,started" || caps != 0 {
				t.Fatalf("lifecycle=%v caps=%d", phases, caps)
			}
			if scenario == "output_cap" {
				status, public := request(http.MethodGet, "/v1/threads/"+threadID, nil)
				var snapshot map[string]any
				if status != http.StatusOK || json.Unmarshal(public, &snapshot) != nil {
					t.Fatal("capped public Primary unavailable")
				}
				turnCount, answerCount := 0, 0
				turns, _ := snapshot["turns"].([]any)
				for _, raw := range turns {
					turn, _ := raw.(map[string]any)
					if turn["id"] != turnID {
						continue
					}
					turnCount++
					reason, err := domainterminal.ParseEndReason(turn["endReason"])
					if turn["status"] != "completed" || err != nil || reason == nil || reason.Kind != domainterminal.EndReasonMaxTokens {
						t.Fatal("capped Primary lost its honest max-tokens terminal")
					}
					items, _ := turn["items"].([]any)
					for _, rawItem := range items {
						item, _ := rawItem.(map[string]any)
						if item["kind"] == "assistant_text" {
							answerCount++
							if item["text"] != cappedText {
								t.Fatal("capped Primary lost truncated public text")
							}
						}
					}
				}
				if turnCount != 1 || answerCount != 1 {
					t.Fatal("capped Primary duplicated or omitted its terminal answer")
				}
				for _, forbidden := range []string{private, secret, "PRIVATE_RETRY_RESPONSE_BODY"} {
					if bytes.Contains(public, []byte(forbidden)) {
						t.Fatal("capped Primary leaked private material")
					}
				}
				batches := 0
				for _, line := range strings.Split(string(replay), "\n") {
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					var event map[string]any
					if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) != nil {
						t.Fatal("invalid capped replay JSON")
					}
					if event["kind"] != domainevent.GeneralTerminalDeliveryBatchKind {
						continue
					}
					batch, err := domainevent.ParseGeneralTerminalDeliveryBatchV1(event)
					if err != nil || batch.ThreadID != threadID || batch.TurnID != turnID {
						t.Fatal("capped replay lost exact terminal batch authority")
					}
					batches++
					answers, usages, terminals := 0, 0, 0
					for _, member := range batch.Events {
						switch member["kind"] {
						case "item_completed":
							item, _ := member["item"].(map[string]any)
							if item["kind"] != "assistant_text" || item["text"] != cappedText || item["status"] != "completed" {
								t.Fatal("capped replay changed truncated answer")
							}
							answers++
						case "usage":
							usage, _ := member["usage"].(map[string]any)
							for field, want := range map[string]int{"promptTokens": 7, "completionTokens": 9, "reasoningTokens": 0, "totalTokens": 16} {
								value, ok := contracts.NumericSeq(usage[field])
								if !ok || value != want {
									t.Fatal("capped replay lost actual Provider usage")
								}
							}
							usages++
						case "turn_completed":
							reason, err := domainterminal.ParseEndReason(member["endReason"])
							if member["status"] != "completed" || err != nil || reason == nil || reason.Kind != domainterminal.EndReasonMaxTokens {
								t.Fatal("capped replay misrepresented its terminal")
							}
							terminals++
						default:
							t.Fatal("capped terminal batch acquired an extra effect")
						}
					}
					if answers != 1 || usages != 1 || terminals != 1 {
						t.Fatal("capped replay duplicated or omitted terminal members")
					}
				}
				if batches != 1 {
					t.Fatal("capped replay duplicated or omitted terminal batch")
				}
			}
			// Persist a real legacy observation only after the composition has
			// quiesced. Reopen it through the same production public replay route.
			shutdownOwnedRuntimeHandler(t, handler)
			durable, err := server.NewProductionDurableEventSessionStore(config.ProductionDurableRoot)
			if err != nil {
				t.Fatal(err)
			}
			// The rejected record advances the public cursor with an owner-time
			// observation. Put it first so the legacy-only suffix stays durable.
			bad := loopapp.BuildProviderRetryingEvent(loopapp.ProviderRetryingEventInput{ThreadID: threadID, TurnID: turnID, Attempt: 2, MaxAttempts: 3, Retry: map[string]any{"schemaVersion": float64(1), "source": "provider_retry", "phase": "scheduled", "logicalSequence": float64(1), "outerAttempt": float64(2), "delayMs": float64(0), "deadlineUnixMs": float64(1700000000000), "unknownProtocol": true}})
			badRecord, _, err := durable.RecordEvent(bad)
			if err != nil {
				t.Fatal(err)
			}
			badSeq, seqOK := contracts.NumericSeq(badRecord["seq"])
			if !seqOK || badSeq < 1 {
				t.Fatal("negative event has no durable sequence")
			}
			if err := domainevent.ValidatePublicRecord(badRecord); err != nil {
				t.Fatalf("test-owned negative observation failed generic public-record validation: %v", err)
			}
			legacy := loopapp.BuildProviderRetryingEvent(loopapp.ProviderRetryingEventInput{ThreadID: threadID, TurnID: turnID, Attempt: 2, MaxAttempts: 3})
			recorded, _, err := durable.RecordEvent(legacy)
			if err != nil {
				t.Fatal(err)
			}
			legacySeq, seqOK := contracts.NumericSeq(recorded["seq"])
			if !seqOK || legacySeq != badSeq+1 {
				t.Fatal("legacy event does not follow the negative durable sequence")
			}
			if err := domainevent.ValidatePublicRecord(recorded); err != nil {
				t.Fatalf("test-owned legacy observation failed public-record validation: %v", err)
			}
			loaded, loadErr := durable.LoadEventsSince(threadID, 0)
			if loadErr != nil {
				t.Fatalf("test-owned quiescent replay failed before restart: %v", loadErr)
			}
			if len(loaded.Diagnostics) != 0 {
				codes := make([]string, 0, len(loaded.Diagnostics))
				for _, d := range loaded.Diagnostics {
					codes = append(codes, d.Error)
				}
				t.Fatalf("test-owned replay diagnostics before restart: %v", codes)
			}
			lease, err = AcquireRuntimePersistenceLease(config)
			if err != nil {
				t.Fatal(err)
			}
			inner, err = newRuntimeServerHandlerWithPersistenceLeaseContextE(ctx, config, lease)
			if err != nil {
				_ = lease.Close()
				t.Fatal(err)
			}
			handler = &ownedPersistenceLeaseHandler{Handler: inner, lease: lease}
			defer shutdownOwnedRuntimeHandler(t, handler)
			var legacyReplay []byte
			for i := 0; i < 2; i++ {
				status, body = request(http.MethodGet, "/v1/threads/"+threadID+"/events?since_seq="+strconv.Itoa(legacySeq-1), nil)
				if status != http.StatusOK || calls.Load() != expectedCalls {
					t.Fatal("legacy replay started a Provider effect")
				}
				for _, line := range strings.Split(string(body), "\n") {
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					var event map[string]any
					if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) != nil {
						t.Fatal("invalid legacy replay JSON")
					}
					if event["kind"] == "error" || event["kind"] == "stream_error" {
						t.Fatalf("legacy replay setup failed: kind=%v code=%v phase=%v", event["kind"], event["code"], event["phase"])
					}
				}
				if i == 0 {
					legacyReplay = body
				} else if !bytes.Equal(legacyReplay, body) {
					t.Fatal("legacy replay changed observation")
				}
			}
			legacyCount := 0
			legacyData := ""
			for _, line := range strings.Split(string(legacyReplay), "\n") {
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				var event map[string]any
				if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) != nil {
					t.Fatal("invalid legacy SSE")
				}
				if event["stage"] != "provider_retrying" {
					continue
				}
				legacyCount++
				legacyData = strings.TrimPrefix(line, "data: ")
				details, _ := event["details"].(map[string]any)
				if details["retry"] != nil || event["attempt"] != float64(2) || event["maxAttempt"] != float64(3) {
					t.Fatal("legacy schedule upgraded or malformed retry published")
				}
			}
			if legacyCount != 1 || bytes.Contains(legacyReplay, []byte("unknownProtocol")) {
				t.Fatal("closed public replay failed legacy/negative observations")
			}
			for i := 0; i < 2; i++ {
				status, body = request(http.MethodGet, "/v1/threads/"+threadID+"/events?since_seq="+strconv.Itoa(badSeq-1), nil)
				if status != http.StatusOK || calls.Load() != expectedCalls {
					t.Fatal("negative replay started a Provider effect")
				}
				for _, forbidden := range []string{private, secret, cappedText, "PRIVATE_RETRY_RESPONSE_BODY", "unknownProtocol"} {
					if bytes.Contains(body, []byte(forbidden)) {
						t.Fatal("negative public replay leaked rejected or private content")
					}
				}
				publicCount := 0
				for _, line := range strings.Split(string(body), "\n") {
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					data := strings.TrimPrefix(line, "data: ")
					var event map[string]any
					if json.Unmarshal([]byte(data), &event) != nil {
						t.Fatal("invalid negative replay JSON")
					}
					publicCount++
					switch publicCount {
					case 1:
						if len(event) != 5 || event["kind"] != "cursor_advanced" || event["threadId"] != threadID || event["seq"] != float64(badSeq) || event["reason"] != "restricted_content_removed" {
							t.Fatal("rejected retry did not produce the exact closed cursor observation")
						}
						stamp, ok := event["timestamp"].(string)
						if _, err := time.Parse(time.RFC3339Nano, stamp); !ok || err != nil {
							t.Fatal("cursor observation has no real owner timestamp")
						}
					case 2:
						if data != legacyData {
							t.Fatal("negative replay changed the persisted legacy observation")
						}
					default:
						t.Fatal("negative replay added an unexpected public effect")
					}
				}
				if publicCount != 2 {
					t.Fatal("negative replay lost the cursor or legacy observation")
				}
			}
		})
	}
}
