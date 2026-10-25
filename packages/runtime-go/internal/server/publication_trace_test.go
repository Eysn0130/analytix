package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	apploop "analytix.local/runtime-go/internal/app/loop"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	codecport "analytix.local/runtime-go/internal/ports/documentgeneration"
	provider "analytix.local/runtime-go/internal/provider"
)

func TestRuntimePublicationTraceMeasuresFinalDependencyWithoutCandidateContent(t *testing.T) {
	start := time.Now()
	result := apploop.RuntimeAgentLoopResult{AssistantText: "private candidate sentinel", LastDependencyAt: start.Add(30 * time.Millisecond), LastResult: domainmodel.Result{HostTiming: domainmodel.ProviderTiming{StartedAt: start, FirstContentAt: start.Add(time.Millisecond), LastContentAt: start.Add(10 * time.Millisecond), FinishedAt: start.Add(20 * time.Millisecond)}}}
	body := runtimePublicationTrace("thread-private-id", "turn-private-id", result, start.Add(35*time.Millisecond), appturn.PublicationTiming{ProjectionReadyAt: start.Add(40 * time.Millisecond), CommittedAt: start.Add(50 * time.Millisecond), DeliverableAt: start.Add(55 * time.Millisecond)})
	if strings.Contains(string(body), "private candidate sentinel") || strings.Contains(string(body), "thread-private-id") || strings.Contains(string(body), "turn-private-id") {
		t.Fatal("trace disclosed identity or candidate bytes")
	}
	var record struct {
		Data map[string]float64 `json:"data"`
	}
	if json.Unmarshal(body, &record) != nil || record.Data["publicationGapMs"] != 20 {
		t.Fatal("gap included model or previous tool wait", string(body))
	}
	if record.Data["sseDeliverableMs"] <= record.Data["publicationCommittedMs"] || record.Data["publicationCommittedMs"] <= record.Data["publicationProjectionReadyMs"] {
		t.Fatal("publication stages lost their order")
	}
	result.LastDependencyAt = time.Time{}
	body = runtimePublicationTrace("t", "u", result, start, appturn.PublicationTiming{CommittedAt: start})
	if strings.Contains(string(body), "publicationGapMs") {
		t.Fatal("missing dependency clock became zero duration")
	}
}

type runtimeTraceOpaqueError struct{}

func (runtimeTraceOpaqueError) Error() string {
	panic("trace must not read error text")
}

func waitForRuntimeTraceDrain(t *testing.T) {
	t.Helper()
	if !runtimeTraceDrainedWithin(2 * time.Second) {
		t.Fatal("runtime diagnostic queue did not drain")
	}
}

func runtimeTraceDrainedWithin(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		publicationTraceMu.Lock()
		idle := !publicationTraceDraining && len(publicationTracePending) == 0
		publicationTraceMu.Unlock()
		if idle {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Millisecond)
	}
}

func TestRuntimeProviderFailureTraceIsOptInAndContainsOnlyClosedFields(t *testing.T) {
	waitForRuntimeTraceDrain(t)
	output, err := os.CreateTemp(t.TempDir(), "core-stderr")
	if err != nil {
		t.Fatal("create synthetic Core stderr capture")
	}
	previous := os.Stderr
	os.Stderr = output
	t.Cleanup(func() {
		defer func() {
			os.Stderr = previous
			_ = output.Close()
		}()
		waitForRuntimeTraceDrain(t)
	})
	t.Setenv("ANALYTIX_THREAD_TRACE", "0")
	recordRuntimeProviderFailureTrace("synthetic-private-thread", "synthetic-private-turn", runtimeTraceOpaqueError{})
	waitForRuntimeTraceDrain(t)
	info, err := output.Stat()
	if err != nil || info.Size() != 0 {
		t.Fatal("disabled diagnostic emitted stderr")
	}
	t.Setenv("ANALYTIX_THREAD_TRACE", "1")
	const secret = "synthetic-secret-diagnostic-sentinel"
	recordRuntimeProviderFailureTrace("synthetic-private-thread", "synthetic-private-turn", errors.Join(
		runtimeTraceOpaqueError{}, privacyprojectionapp.ErrProviderSchemaContainsPII, errors.New(secret),
	))
	recordRuntimeProviderFailureTrace("synthetic-private-thread", "synthetic-private-turn", apploop.TurnFailureError{
		Code: secret, Message: secret, Details: map[string]any{"code": "source_probe_unavailable", "secret": secret, "path": "/synthetic/private/path"},
	})
	recordRuntimeProviderFailureTrace("synthetic-private-thread", "synthetic-private-turn", nil)
	waitForRuntimeTraceDrain(t)
	body, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal("read synthetic Core stderr capture")
	}
	for _, forbidden := range []string{secret, "synthetic-private-thread", "synthetic-private-turn", "/synthetic/private/path", "Details"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatal("diagnostic disclosed untrusted text or identity")
		}
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 2 {
		t.Fatalf("diagnostic records = %d, want 2", len(lines))
	}
	for i, line := range lines {
		const prefix = "[analytix-thread-trace] "
		if !strings.HasPrefix(line, prefix) {
			t.Fatal("diagnostic bypassed existing Core stderr trace route")
		}
		var record map[string]any
		if json.Unmarshal([]byte(strings.TrimPrefix(line, prefix)), &record) != nil || len(record) != 4 || record["name"] != "thread.provider.loop_failure" {
			t.Fatal("unexpected diagnostic envelope")
		}
		for _, key := range []string{"threadId", "turnId"} {
			identity, _ := record[key].(string)
			if !strings.HasPrefix(identity, "ref-") || len(identity) != 68 {
				t.Fatal("diagnostic identity is not hashed")
			}
		}
		data, ok := record["data"].(map[string]any)
		if !ok || len(data) != 3 || data["pid"] != float64(os.Getpid()) {
			t.Fatal("unexpected diagnostic fields")
		}
		wantStage, wantCause := "privacy_projection", "provider_schema_contains_pii"
		if i == 1 {
			wantStage, wantCause = "unknown", "unknown"
		}
		if data["stage"] != wantStage || data["cause"] != wantCause {
			t.Fatal("diagnostic labels escaped closed classification")
		}
	}
}

type runtimeTraceIntentFailureResolver struct {
	handlerProviderExecutionResolverForTest
	err error
}

func (resolver runtimeTraceIntentFailureResolver) ResolveTurnIntent(context.Context, provider.TurnExecutionInput) (provider.TurnExecutionResult, error) {
	return provider.TurnExecutionResult{}, resolver.err
}

func TestRuntimeProviderFailureTracePreservesLoopErrorAndZeroProviderCalls(t *testing.T) {
	for _, enabled := range []string{"0", "1"} {
		t.Run("trace="+enabled, func(t *testing.T) {
			t.Setenv("ANALYTIX_THREAD_TRACE", enabled)
			fixture := newProviderStepIntegrationFixture(t, "diagnostic-error-return")
			const secret = "synthetic-secret-loop-error-sentinel"
			sourceErr := errors.New(secret)
			fixture.handler.providerExecution = runtimeTraceIntentFailureResolver{
				handlerProviderExecutionResolverForTest: handlerProviderExecutionResolverForTest{handler: fixture.handler},
				err:                                     sourceErr,
			}
			transport := &providerStepRecordingProvider{}
			fixture.handler.provider = transport
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if !fixture.handler.runtimeControl().RegisterTurnCancel(fixture.input.ThreadID, fixture.input.TurnID, cancel) {
				t.Fatal("synthetic loop ownership registration failed")
			}
			defer fixture.handler.runtimeControl().UnregisterTurnCancel(fixture.input.ThreadID, fixture.input.TurnID)
			var result runtimeAgentLoopResult
			var returnedErr error
			var drained bool
			stderr := captureRuntimeStderrForTest(t, func() {
				result, returnedErr = fixture.handler.runRuntimeAgentLoopWithMessages(ctx, fixture.input, []provider.Message{
					{Role: "system", Content: "synthetic diagnostic error return"},
					{Role: "user", Content: "ordinary synthetic task"},
				})
				drained = runtimeTraceDrainedWithin(2 * time.Second)
			})
			if !drained {
				t.Fatal("runtime diagnostic queue did not drain")
			}
			// The runner joins its error with logical-step closure. The server
			// must preserve that exact one-level tree and its original cause.
			joined, ok := returnedErr.(interface{ Unwrap() []error })
			if !ok || len(joined.Unwrap()) != 1 || joined.Unwrap()[0] != sourceErr || !errors.Is(returnedErr, sourceErr) {
				t.Fatal("diagnostic changed the original loop error tree")
			}
			if len(transport.Requests()) != 0 || result.AssistantText != "" || len(result.LastResult.Chunks) != 0 {
				t.Fatal("diagnostic changed the original error, result or Provider call count")
			}
			if strings.Contains(stderr, secret) {
				t.Fatal("loop diagnostic disclosed error text")
			}
			if enabled == "0" {
				if stderr != "" {
					t.Fatal("trace-off loop emitted stderr")
				}
				return
			}
			failures := 0
			for _, line := range strings.Split(strings.TrimSpace(stderr), "\n") {
				var record struct {
					Name string         `json:"name"`
					Data map[string]any `json:"data"`
				}
				if json.Unmarshal([]byte(strings.TrimPrefix(line, "[analytix-thread-trace] ")), &record) != nil || record.Name != "thread.provider.loop_failure" {
					continue
				}
				failures++
				if record.Data["stage"] != "unknown" || record.Data["cause"] != "unknown" {
					t.Fatal("ordinary resolver failure received an inferred diagnostic label")
				}
			}
			if failures != 1 {
				t.Fatalf("loop failure diagnostic count = %d, want 1", failures)
			}
		})
	}
}

func TestRuntimeDocumentGenerationFailureTraceIsClosedAndOptIn(t *testing.T) {
	waitForRuntimeTraceDrain(t)
	output, err := os.CreateTemp(t.TempDir(), "generation-stderr")
	if err != nil {
		t.Fatal("create synthetic trace capture")
	}
	previous := os.Stderr
	os.Stderr = output
	t.Cleanup(func() {
		defer func() { os.Stderr = previous; _ = output.Close() }()
		waitForRuntimeTraceDrain(t)
	})
	t.Setenv("ANALYTIX_THREAD_TRACE", "0")
	recordRuntimeDocumentGenerationFailureTrace("private-thread", "private-turn", "checkpoint_record", "unavailable")
	waitForRuntimeTraceDrain(t)
	if info, err := output.Stat(); err != nil || info.Size() != 0 {
		t.Fatal("disabled trace emitted output")
	}
	t.Setenv("ANALYTIX_THREAD_TRACE", "1")
	pairs := [][2]string{
		{"checkpoint_settlement", "unavailable"}, {"checkpoint_settlement", "incomplete"}, {"checkpoint_record", "unavailable"},
		{"host_preconsume", "snapshot_unavailable"}, {"host_postconsume", "identity_invalid"}, {"host_postconsume", "context_canceled"},
		{"generation_output", "checkpoint_unavailable"},
	}
	for _, pair := range pairs {
		recordRuntimeDocumentGenerationFailureTrace("private-thread", "private-turn", pair[0], pair[1])
	}
	for _, pair := range [][2]string{{"checkpoint_record", "incomplete"}, {"/private/path-sentinel", "unavailable"}, {"host_postconsume", "secret-error-sentinel"}} {
		recordRuntimeDocumentGenerationFailureTrace("private-thread", "private-turn", pair[0], pair[1])
	}
	waitForRuntimeTraceDrain(t)
	body, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal("read synthetic trace capture")
	}
	for _, text := range []string{"private-thread", "private-turn", "/private/path-sentinel", "secret-error-sentinel"} {
		if strings.Contains(string(body), text) {
			t.Fatal("trace disclosed untrusted identity or text")
		}
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != len(pairs) {
		t.Fatalf("trace count = %d, want %d", len(lines), len(pairs))
	}
	for i, line := range lines {
		var event struct {
			Name     string         `json:"name"`
			ThreadID string         `json:"threadId"`
			TurnID   string         `json:"turnId"`
			Data     map[string]any `json:"data"`
		}
		if !strings.HasPrefix(line, "[analytix-thread-trace] ") || json.Unmarshal([]byte(strings.TrimPrefix(line, "[analytix-thread-trace] ")), &event) != nil ||
			event.Name != "thread.tool.generation_failure" || len(event.Data) != 3 || event.Data["stage"] != pairs[i][0] || event.Data["cause"] != pairs[i][1] ||
			!strings.HasPrefix(event.ThreadID, "ref-") || !strings.HasPrefix(event.TurnID, "ref-") {
			t.Fatal("generation trace lost closed fields")
		}
	}
}

func TestRuntimeDocumentGenerationOutputCauseIgnoresPrivateFields(t *testing.T) {
	for _, test := range []struct{ code, want string }{
		{"validation_error", "validation_invalid"}, {"generation_target_unavailable", "target_unavailable"},
		{"document_generation_failed", "codec_invalid"}, {"checkpoint_unavailable", "checkpoint_unavailable"},
		{"document_generation_cancelled", "context_canceled"}, {"private-code-sentinel", "other_unavailable"},
	} {
		if got := runtimeDocumentGenerationOutputCause(map[string]any{"code": test.code, "error": runtimeTraceOpaqueError{}, "path": "/private/path"}); got != test.want {
			t.Fatal("unexpected closed cause", got)
		}
	}
	if runtimeDocumentGenerationOutputCause(runtimeTraceOpaqueError{}) != "other_unavailable" {
		t.Fatal("arbitrary output was inspected")
	}
}

func TestGenerationSettlementBoundaryEmitsOwnedFailureOnlyForGeneration(t *testing.T) {
	waitForRuntimeTraceDrain(t)
	output, err := os.CreateTemp(t.TempDir(), "settlement-stderr")
	if err != nil {
		t.Fatal("create synthetic trace capture")
	}
	previous := os.Stderr
	os.Stderr = output
	t.Cleanup(func() {
		defer func() { os.Stderr = previous; _ = output.Close() }()
		waitForRuntimeTraceDrain(t)
	})
	t.Setenv("ANALYTIX_THREAD_TRACE", "1")
	h := &runtimeServerHandler{}
	pending := runtimePendingToolCall{ThreadID: "synthetic-private-thread", TurnID: "synthetic-private-turn",
		Call: provider.ToolCall{ID: "synthetic-call", Name: "generate_office_document"}}
	input := h.mutationToolInput(context.Background(), pending, nil)
	status, err := input.Checkpoint.SettleOperation(filestore.MutationOperationDraft{Enabled: true}, true)
	if err == nil || status != "" {
		t.Fatal("invalid real settlement unexpectedly succeeded")
	}
	pending.Call.Name = "write"
	input = h.mutationToolInput(context.Background(), pending, nil)
	if _, err := input.Checkpoint.SettleOperation(filestore.MutationOperationDraft{Enabled: true}, true); err == nil {
		t.Fatal("invalid write settlement unexpectedly succeeded")
	}
	waitForRuntimeTraceDrain(t)
	body, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal("read synthetic trace capture")
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], `"stage":"checkpoint_settlement"`) || !strings.Contains(lines[0], `"cause":"unavailable"`) ||
		strings.Contains(lines[0], "synthetic-private") {
		t.Fatal("real generation hook lost closed diagnostic or traced another tool")
	}
}

type diagnosticGenerationCodec func(context.Context, codecport.Input) ([]byte, error)

func (codec diagnosticGenerationCodec) Encode(ctx context.Context, input codecport.Input) ([]byte, error) {
	return codec(ctx, input)
}

func TestGenerationHostPostconsumeFailureRemainsClosed(t *testing.T) {
	for _, scenario := range []string{"snapshot", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			projector, _, scope := newSelectionProjectorFixture(t)
			office, _, installedSkill := officeRuntimeFixtureWithInstalledSkill(t, "docx")
			h := projector.handler
			h.officePackageHost = office.officePackageHost
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			skills := h.officePackageHost.Skills(ctx)
			if len(skills) != 1 {
				t.Fatal("synthetic hosted skill unavailable")
			}
			called := 0
			h.documentCodec = diagnosticGenerationCodec(func(context.Context, codecport.Input) ([]byte, error) {
				called++
				if scenario == "cancel" {
					cancel()
				} else if err := os.WriteFile(installedSkill, []byte("PRIVATE_INSTALLED_BODY_SENTINEL"), 0600); err != nil {
					t.Fatal("tamper synthetic installed fixture")
				}
				return nil, codecport.ErrCodecUnavailable
			})
			waitForRuntimeTraceDrain(t)
			capture, err := os.CreateTemp(t.TempDir(), "host-stderr")
			if err != nil {
				t.Fatal("create synthetic trace capture")
			}
			previous := os.Stderr
			os.Stderr = capture
			t.Cleanup(func() {
				defer func() { os.Stderr = previous; _ = capture.Close() }()
				waitForRuntimeTraceDrain(t)
			})
			t.Setenv("ANALYTIX_THREAD_TRACE", "1")
			pending := runtimePendingToolCall{ThreadID: scope.ThreadID, TurnID: "synthetic-generation-turn", Workspace: scope.Workspace,
				SandboxMode: "workspace-write", Call: provider.ToolCall{Name: "generate_office_document"}}
			bound := context.WithValue(ctx, preparedHostedSkillKey{}, skills[0])
			output, failed := h.executeGenerateDocumentRuntimeTool(bound, pending, map[string]any{"kind": "docx", "path": "report.docx", "markdown": "Synthetic"})
			if called != 1 || !failed || runtimeDocumentGenerationOutputCause(output) != "other_unavailable" {
				t.Fatal("fixture did not reach the real Host post-consume failure")
			}
			if _, err := os.Stat(filepath.Join(scope.Workspace, "report.docx")); !os.IsNotExist(err) {
				t.Fatal("failed synthetic codec created a target")
			}
			waitForRuntimeTraceDrain(t)
			body, err := os.ReadFile(capture.Name())
			if err != nil {
				t.Fatal("read synthetic trace capture")
			}
			cause := "snapshot_unavailable"
			if scenario == "cancel" {
				cause = "context_canceled"
			}
			if !strings.Contains(string(body), `"stage":"host_postconsume"`) || !strings.Contains(string(body), `"cause":"`+cause+`"`) {
				t.Fatal("real Host boundary lost fixed stage/cause")
			}
			for _, forbidden := range []string{"PRIVATE_INSTALLED_BODY_SENTINEL", installedSkill, scope.Workspace, scope.ThreadID, pending.TurnID} {
				if strings.Contains(string(body), forbidden) {
					t.Fatal("Host diagnostic disclosed private content or identity")
				}
			}
		})
	}
}
