package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	threadriskauthorityapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	visionbridgeapp "analytix.local/runtime-go/internal/app/visionbridge"
	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	provider "analytix.local/runtime-go/internal/provider"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

type recordingVisionBridgeProvider struct {
	requests []provider.Request
	result   provider.Result
	err      error
}

type imageQuarantineProvider struct {
	mu       sync.Mutex
	requests []provider.Request
}

func (*imageQuarantineProvider) RequiresDurablePipelineStagesV1() {}

func (fake *imageQuarantineProvider) Stream(_ context.Context, request provider.Request) (provider.Result, error) {
	if request.BeforeSend == nil {
		return provider.Result{}, errors.New("image quarantine physical-send authority is unavailable")
	}
	if err := request.BeforeSend(1); err != nil {
		return provider.Result{}, err
	}
	if err := emitTestDurableProviderPipelinePairV1(request); err != nil {
		return provider.Result{}, err
	}
	fake.mu.Lock()
	fake.requests = append(fake.requests, request)
	fake.mu.Unlock()
	chunk := provider.Chunk{Kind: provider.ChunkText, Text: "ordinary text work continued"}
	if request.OnChunk != nil {
		if err := request.OnChunk(chunk); err != nil {
			return provider.Result{}, err
		}
	}
	return provider.Result{Chunks: []provider.Chunk{chunk}, StreamCompleted: true}, nil
}

func (fake *imageQuarantineProvider) snapshot() []provider.Request {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return append([]provider.Request(nil), fake.requests...)
}

func serverVisionTelemetryBindingV1(t *testing.T) *domainmodel.ProviderTelemetryBindingV1 {
	t.Helper()
	securityContext := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-vision-telemetry", TurnID: "turn-vision-telemetry", WorkspaceRealPath: "/workspace",
		CaseID: "case-vision-telemetry", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	return &domainmodel.ProviderTelemetryBindingV1{
		SecurityContext: securityContext, UsageSource: domaincache.ProviderUsageSourceTurn,
		Channel: domaincache.ProviderChannelPrimary, LogicalSequence: 1, OuterAttempt: 1,
	}
}

type visionPostCommitRiskAuthority struct {
	mu        sync.Mutex
	private   ed25519.PrivateKey
	public    ed25519.PublicKey
	policy    domainsecurity.ThreadRiskPolicyV1
	contracts securitycontexttest.RiskAuthorityContracts
}

func newVisionPostCommitRiskAuthority() *visionPostCommitRiskAuthority {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x76}, ed25519.SeedSize))
	return &visionPostCommitRiskAuthority{
		private: privateKey,
		public:  privateKey.Public().(ed25519.PublicKey),
	}
}

func (authority *visionPostCommitRiskAuthority) ResolveOrRaise(_ context.Context, input threadriskauthorityapp.ResolveOrRaiseInput) (threadriskauthorityapp.Head, error) {
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if input.RequestedRisk != domainsecurity.RiskClassGeneral {
		return threadriskauthorityapp.Head{}, errors.New("vision post-commit fixture only authorizes general turns")
	}
	if authority.policy.PolicyDigest == "" {
		policy, err := domainsecurity.NewThreadRiskPolicyV1(domainsecurity.ThreadRiskPolicyInputV1{
			ThreadID: input.ThreadID, WorkspaceRealPath: input.WorkspaceRealPath, RiskClass: input.RequestedRisk,
			Origin: input.Origin, SignalsDigest: input.SignalsDigest, IssuedAt: input.IssuedAt,
			AuthorityKeyID: domainsecurity.SHA256Hex(authority.public), AuthorityPublicKey: authority.public,
		}, func(message []byte) ([]byte, error) { return ed25519.Sign(authority.private, message), nil })
		if err != nil {
			return threadriskauthorityapp.Head{}, err
		}
		contracts, err := securitycontexttest.WitnessedRiskAuthorityContracts(
			policy.ThreadID, policy.WorkspaceRealPath, policy.RiskClass, policy.PolicyDigest,
		)
		if err != nil {
			return threadriskauthorityapp.Head{}, err
		}
		authority.policy = policy
		authority.contracts = contracts
	}
	if authority.policy.ThreadID != input.ThreadID || authority.policy.WorkspaceRealPath != input.WorkspaceRealPath {
		return threadriskauthorityapp.Head{}, errors.New("vision post-commit risk authority request changed identity")
	}
	return authority.headLocked(), nil
}

func (authority *visionPostCommitRiskAuthority) ValidateCurrent(_ context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if authority.policy.PolicyDigest == "" || domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil ||
		securityContext.ThreadID != authority.policy.ThreadID || securityContext.WorkspaceRealPath != authority.policy.WorkspaceRealPath ||
		domainsecurity.ValidateTurnPublicationPolicyForThreadRiskPolicyV1(securityContext.PublicationPolicy, authority.policy) != nil ||
		domainsecurity.ValidateWitnessedRiskAuthorityBindingV1(
			securityContext.RiskAuthorityBinding,
			authority.contracts.Index,
			authority.contracts.Request,
			authority.contracts.Observation,
		) != nil {
		return errors.New("vision post-commit current risk authority mismatch")
	}
	return nil
}

func (authority *visionPostCommitRiskAuthority) headLocked() threadriskauthorityapp.Head {
	return threadriskauthorityapp.Head{
		HasIndex: true, Index: authority.contracts.Index, Request: authority.contracts.Request,
		Observation: authority.contracts.Observation, RiskAuthorityBinding: authority.contracts.Binding,
		Policy: authority.policy, Found: true,
	}
}

type postCommitVisionProvider struct {
	mu      sync.Mutex
	calls   int
	inspect func(provider.Request) error
}

func (fake *postCommitVisionProvider) Stream(_ context.Context, request provider.Request) (provider.Result, error) {
	fake.mu.Lock()
	fake.calls++
	call := fake.calls
	fake.mu.Unlock()
	if call == 1 {
		if request.ProviderID != "vision-order-bridge" {
			return provider.Result{}, errors.New("vision bridge was not the first provider effect")
		}
		if fake.inspect == nil {
			return provider.Result{}, errors.New("vision bridge post-commit inspection is unavailable")
		}
		if err := fake.inspect(request); err != nil {
			return provider.Result{}, err
		}
		return provider.Result{
			ProviderID: request.ProviderID, EndpointFormat: request.EndpointFormat, StreamCompleted: true,
			Chunks: []provider.Chunk{{
				Kind: provider.ChunkText,
				Text: `{"source_kind":"user_attachment","summary":"A generic chart is visible","visible_text":["Quarterly chart"],"objects":["chart"],"layout_or_spatial_notes":[],"task_relevant_details":["title is visible"],"uncertainties":[],"confidence":0.99}`,
			}},
		}, nil
	}
	chunk := provider.Chunk{Kind: provider.ChunkText, Text: "general vision turn completed"}
	if request.OnChunk != nil {
		if err := request.OnChunk(chunk); err != nil {
			return provider.Result{}, err
		}
	}
	return provider.Result{
		ProviderID: request.ProviderID, EndpointFormat: request.EndpointFormat, StreamCompleted: true,
		Chunks: []provider.Chunk{chunk},
	}, nil
}

func (fake *postCommitVisionProvider) CallCount() int {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fake.calls
}

func TestCaseToolRawPIINeverReentersProviderHistoryOrPublicPersistence(t *testing.T) {
	handler := &runtimeServerHandler{}
	securityContext := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-a", TurnID: "turn-a", WorkspaceRealPath: "/workspace", CaseID: "case-a",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("binding")),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 3, IssuedAt: time.Unix(1, 0),
	})
	call := provider.ToolCall{ID: serverTestHostToolCallID("case_tool_raw_pii"), Name: "mcp__analytix-fund-analysis__query_transactions", Arguments: json.RawMessage(`{}`)}
	serverIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity("analytix-fund-analysis", "analytix-fund-analysis", "1.0.0", domainsecurity.SHA256Hex([]byte("vision-test-instance")), 3)
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-a", ServerIdentity: serverIdentity, ToolName: call.Name,
		ToolCallID: call.ID, ConnectionEpoch: 3, ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("scope")),
		ReadOnly: true, ApprovalState: "not_required", IssuedAt: time.Unix(1, 0), ExpiresAt: time.Unix(3, 0),
	})
	pending := runtimePendingToolCall{Call: call, SecurityContext: securityContext, ExecutionGrant: grant}
	outcome := domainevidence.NewToolOutcome(domainevidence.ToolOutcomeInput{
		ToolName: pending.Call.Name, ToolCallID: pending.Call.ID, ContextDigest: securityContext.ContextDigest,
		ExecutionGrantID: grant.GrantID, CaseID: securityContext.CaseID, ContextEpoch: securityContext.ContextEpoch,
		DatasetSnapshotID: securityContext.DatasetSnapshotID, ServerIdentity: grant.ServerIdentity, TransportStatus: domainevidence.TransportSuccess,
		SemanticStatus: domainevidence.SemanticSuccess, IssuedAt: time.Unix(2, 0),
	})
	output := map[string]any{
		"executed": true,
		"result": map[string]any{
			"account": "6222020202020202020",
			"amount":  "123.45",
		},
		"toolOutcome": domainevidence.ToolOutcomeRecord(outcome),
	}

	persistOutput, modelContent := handler.prepareRuntimeToolResultForModel(context.Background(), pending, output, false)
	persistJSON, err := json.Marshal(persistOutput)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(persistJSON), "6222020202020202020") || strings.Contains(string(persistJSON), "123.45") {
		t.Fatalf("case fact reached durable projection: %s", persistJSON)
	}
	if strings.Contains(modelContent, "6222020202020202020") || strings.Contains(modelContent, "123.45") {
		t.Fatalf("raw case fact re-entered provider context: %s", modelContent)
	}
	for _, expected := range []string{"case_source_result_private", `"executed":true`, `"isError":false`} {
		if !strings.Contains(modelContent, expected) {
			t.Fatalf("model projection missing %q: %s", expected, modelContent)
		}
	}
	for _, forbidden := range []string{
		"toolOutcome", pending.Call.Name, pending.Call.ID, securityContext.ContextDigest, grant.GrantID,
		securityContext.DatasetSnapshotID, grant.ServerIdentity,
	} {
		if strings.Contains(modelContent, forbidden) {
			t.Fatalf("private settlement authority %q re-entered provider context: %s", forbidden, modelContent)
		}
	}
}

func (p *recordingVisionBridgeProvider) Stream(_ context.Context, request provider.Request) (provider.Result, error) {
	p.requests = append(p.requests, request)
	return p.result, p.err
}

func prepareAuthorizedVisionBridgePending(
	t *testing.T,
	handler *runtimeServerHandler,
	primary provider.TurnConfig,
) runtimePendingToolCall {
	t.Helper()
	if handler == nil || handler.store == nil {
		t.Fatal("vision bridge handler authority is unavailable")
	}
	configureServerGeneralExecution(t, handler)
	workspace := workspacetest.New(t)
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "vision tool result", "workspace": workspace, "providerId": "deepseek", "model": "deepseek-v4-pro",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_vision_tool_result"
	now := time.Now().UTC().Truncate(time.Second)
	securityContext, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: context.Background(), Authority: handler.turnSecurity, Thread: thread,
		ThreadID: threadID, TurnID: turnID, Workspace: workspace,
		Principal: testIdentityPrincipal(), IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	call := provider.ToolCall{ID: serverTestHostToolCallID("call_read_screenshot"), Name: "read", Arguments: json.RawMessage(`{"path":"screenshot.png"}`)}
	prompt := "inspect screenshot output"
	schemas := handler.runtimeToolSchemasForPromptWithGoalToolsAndMCPNames(false, nil, false, false, prompt, false, nil)
	advertisedToolScope := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		if name := strings.TrimSpace(schema.Name); name != "" {
			advertisedToolScope = append(advertisedToolScope, name)
		}
	}
	grant, err := executiongrantapp.IssueProvider(
		securityContext, "deepseek", call, schemas, advertisedToolScope, true, "not_required", 0, "", now,
	)
	if err != nil {
		t.Fatal(err)
	}
	grantRecord := map[string]any{}
	grantBody, _ := json.Marshal(grant)
	_ = json.Unmarshal(grantBody, &grantRecord)
	epochState, err := contextepochapp.BootstrapState(
		threadID, securityContext.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(securityContext)}, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	securityContextRecord := turnsecurityapp.PublicRecord(securityContext)
	toolCallItemID := domaintoolcall.ToolCallItemIDV1(turnID, call.ID)
	if err := handler.store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "createdAt": now.Format(time.RFC3339Nano),
		"securityContext": securityContextRecord,
		"items": []any{map[string]any{
			"id": toolCallItemID, "threadId": threadID, "turnId": turnID, "kind": "tool_call",
			"toolName": call.Name, "callId": call.ID, "status": "completed", "createdAt": now.Format(time.RFC3339Nano),
			"arguments": map[string]any{"path": "screenshot.png"}, "contextDigest": securityContext.ContextDigest,
			"contextEpoch": float64(securityContext.ContextEpoch), "executionGrantId": grant.GrantID, "executionGrant": grantRecord,
		}},
	}, "deepseek", map[string]any{
		"securityState":     securityContextRecord,
		"contextEpochState": contextepochapp.PublicState(epochState),
	}); err != nil {
		t.Fatal(err)
	}
	pending := runtimePendingToolCall{
		ThreadID: threadID, TurnID: turnID, ProviderID: "deepseek", Model: primary.Model,
		Workspace: workspace, Prompt: prompt, ProviderConfig: primary, Call: call,
		ToolCallItemID: toolCallItemID, ToolScope: advertisedToolScope,
		SecurityContext: securityContext, ExecutionGrant: grant,
	}
	if err := handler.authorizeRuntimePending(context.Background(), pending, "", now.Add(time.Second)); err != nil {
		t.Fatalf("vision bridge pending fixture is not executable: %v", err)
	}
	return pending
}

func TestCaseBoundBankImageNeverInvokesVisionBridgeWithoutSnapshotAuthority(t *testing.T) {
	dataDir := t.TempDir()
	workspace := writeThreadMutationCaseBinding(t)
	durableRoot := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: dataDir,
		ProviderID: "vision-order-primary", BaseURL: "https://primary.invalid/v1", APIKey: "primary-key",
		Model: "text-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	handler.turnSecurity.RiskAuthority = newVisionPostCommitRiskAuthority()
	configureSteerTestCaseAuthorities(t, handler, durableRoot)
	handler.providerConfig = provider.NewRuntimeProviderConfigSet(provider.RuntimeProviderConfigInput{
		DefaultProviderID: "vision-order-primary", DefaultBaseURL: "https://primary.invalid/v1",
		DefaultAPIKey: "primary-key", DefaultEndpointFormat: "chat_completions", DefaultModel: "text-model",
	})
	handler.visionBridge = runtimeVisionBridgeConfig{
		Enabled: true, Mode: "always", ProviderID: "vision-order-bridge", BaseURL: "https://bridge.invalid/v1",
		APIKey: "bridge-key", EndpointFormat: "chat_completions", Model: "vision-model",
		SemanticProbeStatus: "supported", FallbackWhenPrimaryImageUnsupported: true,
		MaxImageBytes: 1500000, MaxScreenshotsPerTurn: 4,
	}
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "Vision post-commit order", "workspace": workspace,
		"providerId": "vision-order-primary", "model": "text-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	attachment, err := handler.attachments.Create(map[string]any{
		"name": "bank-ledger.png", "mimeType": "image/png", "dataBase64": "iVBORw0KGgo=",
		"threadId": threadID, "workspace": workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.attachmentAccess.CommitUpload(context.Background(), attachment); err != nil {
		t.Fatal(err)
	}
	attachmentID := stringField(attachment, "id")
	if attachmentID == "" {
		t.Fatalf("image attachment missing id: %#v", attachment)
	}

	fake := &postCommitVisionProvider{}
	handler.provider = fake

	response, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt: "Summarize the attachment.", ProviderID: "vision-order-primary",
		Model: "text-model", AttachmentIDs: []string{attachmentID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fake.CallCount() != 0 {
		t.Fatalf("case-bound attachment invoked bridge or primary provider before snapshot authority: calls=%d", fake.CallCount())
	}
	durable, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, ok := appmodel.TurnByID(durable, stringField(response, "turnId"))
	if !ok {
		t.Fatalf("boundary attachment turn is missing: %#v", durable["turns"])
	}
	frozen, err := appturn.FrozenSecurityContextForTurn(durable, stringField(turn, "id"))
	if err != nil || !domainsecurity.TurnSecurityContextIsBoundaryOnly(frozen) ||
		!domainsecurity.TurnSecurityContextRequiresFinalEvidenceGate(frozen) {
		t.Fatalf("case-bound attachment did not freeze a case boundary before provider access: context=%#v err=%v", frozen, err)
	}
}

func TestRuntimeToolResultMediaBlockedUntilAttemptLocalAuthority(t *testing.T) {
	imageBase64 := "iVBORw0KGgo="
	fakeProvider := &recordingVisionBridgeProvider{
		result: provider.Result{
			Chunks: []provider.Chunk{{
				Kind: provider.ChunkText,
				Text: `{"summary":"TextEdit is visible","visible_text":["AX-731"],"ui_elements":["text area"],"selected_text":"","warnings":[]}`,
			}},
			StreamCompleted: true,
		},
	}
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "deepseek", BaseURL: "https://api.deepseek.com", APIKey: "deepseek-key",
		EndpointFormat: "chat_completions", Model: "deepseek-v4-pro",
	}).(*runtimeServerHandler)
	handler.provider = fakeProvider
	handler.providerConfig = provider.NewRuntimeProviderConfigSet(provider.RuntimeProviderConfigInput{
		DefaultProviderID:     "deepseek",
		DefaultBaseURL:        "https://api.deepseek.com",
		DefaultAPIKey:         "deepseek-key",
		DefaultEndpointFormat: "chat_completions",
		DefaultModel:          "deepseek-v4-pro",
	})
	handler.visionBridge = runtimeVisionBridgeConfig{
		Enabled:               true,
		Mode:                  "auto",
		ProviderID:            "xiaomi",
		BaseURL:               "https://api.xiaomimimo.com/v1",
		APIKey:                "xiaomi-key",
		EndpointFormat:        "chat_completions",
		Model:                 "mimo-v2.5",
		MaxImageBytes:         1500000,
		MaxScreenshotsPerTurn: 4,
		SemanticProbeStatus:   "supported",
	}
	pending := prepareAuthorizedVisionBridgePending(t, handler, provider.TurnConfig{SupportsImageInput: false, Model: "deepseek-v4-pro"})
	output := map[string]any{
		"kind": "computer_app_state",
		"images": []any{map[string]any{
			"mime_type":   "image/png",
			"data_base64": imageBase64,
		}},
	}

	persistOutput, modelContent := handler.prepareRuntimeToolResultForModel(context.Background(), pending, output, false)
	if len(fakeProvider.requests) != 0 {
		t.Fatalf("tool media escaped without attempt-local private authority: %#v", fakeProvider.requests)
	}
	if persistOutput.ProjectionKind != "host_status" || persistOutput.MessageKey != "tool_completed" || !persistOutput.PrivatePayloadWithheld {
		t.Fatalf("persist output should be a closed metadata-only projection: %#v", persistOutput)
	}
	persistJSON, _ := json.Marshal(persistOutput)
	persistContent := string(persistJSON)
	if strings.Contains(persistContent, imageBase64) ||
		strings.Contains(persistContent, "data_base64") ||
		strings.Contains(persistContent, "data:image/") {
		t.Fatalf("persisted tool result must not contain raw image bytes or image payload keys: %s", persistContent)
	}
	if strings.Contains(modelContent, imageBase64) || strings.Contains(modelContent, "data:image/") {
		t.Fatalf("model-facing tool result must not contain raw image bytes: %s", modelContent)
	}
	for _, expected := range []string{"vision_bridge", "unavailable", "attempt-local private authority", "[redacted image bytes:"} {
		if !strings.Contains(modelContent, expected) {
			t.Fatalf("model content missing %q: %s", expected, modelContent)
		}
	}
}

func TestRuntimeVisionBridgeUserAttachmentFailsClosedWithoutTrustedProjector(t *testing.T) {
	imageBase64 := base64.StdEncoding.EncodeToString([]byte("attachment-png"))
	fakeProvider := &recordingVisionBridgeProvider{
		result: provider.Result{
			Chunks: []provider.Chunk{{
				Kind: provider.ChunkText,
				Text: `{"source_kind":"user_attachment","summary":"A chart is visible","visible_text":["Revenue"],"objects":["bar chart"],"layout_or_spatial_notes":[],"task_relevant_details":["Q4 is highest"],"uncertainties":[],"confidence":0.91}`,
			}},
			StreamCompleted: true,
		},
	}
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "deepseek", BaseURL: "https://api.deepseek.com", APIKey: "deepseek-key",
		EndpointFormat: "chat_completions", Model: "deepseek-v4-pro",
	}).(*runtimeServerHandler)
	handler.provider = fakeProvider
	handler.providerConfig = provider.NewRuntimeProviderConfigSet(provider.RuntimeProviderConfigInput{
		DefaultProviderID:     "deepseek",
		DefaultBaseURL:        "https://api.deepseek.com",
		DefaultAPIKey:         "deepseek-key",
		DefaultEndpointFormat: "chat_completions",
		DefaultModel:          "deepseek-v4-pro",
	})
	handler.visionBridge = runtimeVisionBridgeConfig{
		Enabled:               true,
		Mode:                  "auto",
		ProviderID:            "aliyun",
		BaseURL:               "https://dashscope.aliyuncs.com/compatible-mode/v1",
		APIKey:                "aliyun-key",
		EndpointFormat:        "chat_completions",
		Model:                 "qwen3-vl-plus",
		MaxImageBytes:         1500000,
		MaxScreenshotsPerTurn: 4,
		SemanticProbeStatus:   "supported",
	}
	attachments := appturn.ResolvedAttachments{
		IDs: []string{"att_1"},
		Parts: []appturn.ResolvedAttachmentPart{{
			ID:              "att_1",
			MIMEType:        "image/png",
			Source:          "text_fallback",
			IsImageFallback: true,
			MessagePart: provider.MessagePart{
				Type: "text",
				Text: "Base64:\n```base64\n" + imageBase64 + "\n```",
			},
		}},
		MessageParts: []provider.MessagePart{{
			Type: "text",
			Text: "Base64:\n```base64\n" + imageBase64 + "\n```",
		}},
		ImageCandidates: []appturn.ResolvedAttachmentImage{{
			ID:         "att_1",
			Name:       "chart.png",
			MIMEType:   "image/png",
			DataBase64: imageBase64,
		}},
		TextFallbackCount: 1,
		TextMIMETypes:     []string{"image/png"},
	}

	prepared, err := handler.runtimeVisionBridgeService().PrepareAttachments(context.Background(), visionbridgeapp.PrepareAttachmentsInput{Attachments: attachments, Primary: provider.TurnConfig{
		ProviderID:         "deepseek",
		Model:              "deepseek-v4-pro",
		SupportsImageInput: false,
		InputModalities:    []string{"text"},
		MessageParts:       []string{"text"},
		EndpointFormat:     "chat_completions",
		ReasoningProtocol:  "deepseek",
		ReasoningEffort:    "high",
	}, PrimaryProviderID: "deepseek", PrimaryModel: "deepseek-v4-pro", ProviderTelemetry: serverVisionTelemetryBindingV1(t), Authorize: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}

	if len(fakeProvider.requests) != 0 {
		t.Fatalf("missing trusted projector reached bridge provider: %#v", fakeProvider.requests)
	}
	if len(prepared.MessageParts) != 1 || prepared.MessageParts[0].Type != "text" {
		t.Fatalf("text-only primary should receive one text observation part: %#v", prepared.MessageParts)
	}
	primaryText := prepared.MessageParts[0].Text
	if strings.Contains(primaryText, imageBase64) || strings.Contains(primaryText, "image_url") || strings.Contains(primaryText, "input_image") {
		t.Fatalf("primary attachment observation must not contain raw image payload: %s", primaryText)
	}
	for _, expected := range []string{"Vision Bridge observation", "untrusted observation", "trusted local image privacy projection is unavailable"} {
		if !strings.Contains(primaryText, expected) {
			t.Fatalf("primary attachment observation missing %q: %s", expected, primaryText)
		}
	}
	if prepared.VisionBridgeUsed || prepared.VisionBridgeStatus != "unavailable" || prepared.VisionBridgeObservedCount != 0 {
		t.Fatalf("pipeline should record unavailable image effect: %#v", prepared.PipelineDetails())
	}
}

func TestRuntimeTurnQuarantinesImageAndContinuesIndependentPrimaryText(t *testing.T) {
	workspace := workspacetest.New(t)
	durableRoot := t.TempDir()
	dataDir := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: dataDir,
		ProviderID: "primary-text", BaseURL: "https://primary.invalid/v1", APIKey: "primary-key",
		EndpointFormat: "chat_completions", Model: "text-model",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	handler.providerConfig = provider.NewRuntimeProviderConfigSet(provider.RuntimeProviderConfigInput{
		DefaultProviderID: "primary-text", DefaultBaseURL: "https://primary.invalid/v1",
		DefaultAPIKey: "primary-key", DefaultEndpointFormat: "chat_completions", DefaultModel: "text-model",
	})
	handler.visionBridge = runtimeVisionBridgeConfig{
		Enabled: true, Mode: "auto", ProviderID: "bridge-image", BaseURL: "https://bridge.invalid/v1",
		APIKey: "bridge-key", EndpointFormat: "chat_completions", Model: "vision-model",
		SemanticProbeStatus: "supported", FallbackWhenPrimaryImageUnsupported: true,
		MaxImageBytes: 1500000, MaxScreenshotsPerTurn: 4,
	}
	fake := &imageQuarantineProvider{}
	handler.provider = fake

	thread, err := handler.store.CreateThread(map[string]any{
		"title": "image quarantine", "workspace": workspace,
		"providerId": "primary-text", "model": "text-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	const sourceName = "PRIVATE_IMAGE_SOURCE_NAME_731.png"
	rawImage, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	rawImage = append(rawImage, []byte("PRIVATE_IMAGE_BYTES_SENTINEL_731")...)
	rawBase64 := base64.StdEncoding.EncodeToString(rawImage)
	attachment, err := handler.attachments.Create(map[string]any{
		"name": sourceName, "mimeType": "image/png", "dataBase64": rawBase64,
		"threadId": threadID, "workspace": workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.attachmentAccess.CommitUpload(context.Background(), attachment); err != nil {
		t.Fatal(err)
	}
	response, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt:     "Continue the independent text task and report image availability.",
		ProviderID: "primary-text", Model: "text-model", AttachmentIDs: []string{stringField(attachment, "id")},
	})
	if err != nil {
		t.Fatal(err)
	}
	requests := fake.snapshot()
	if len(requests) != 1 || requests[0].ProviderID != "primary-text" {
		t.Fatalf("image quarantine did not isolate bridge while continuing primary text: %#v", requests)
	}
	providerBody, err := json.Marshal(requests[0].Messages)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{rawBase64, "PRIVATE_IMAGE_BYTES_SENTINEL_731", sourceName, "data:image/", "image_url"} {
		if strings.Contains(string(providerBody), forbidden) {
			t.Fatalf("primary text provider received quarantined image source %q: %s", forbidden, providerBody)
		}
	}
	if !strings.Contains(string(providerBody), "trusted local image privacy projection is unavailable") {
		t.Fatalf("primary text continuation omitted image-effect unavailable context: %s", providerBody)
	}

	reloaded, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	durableBody, err := json.Marshal(reloaded)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{rawBase64, "PRIVATE_IMAGE_BYTES_SENTINEL_731", "data:image/", "image_url"} {
		if strings.Contains(string(durableBody), forbidden) {
			t.Fatalf("durable/public turn state received quarantined image source %q: %s", forbidden, durableBody)
		}
	}
	turn, ok := appmodel.TurnByID(reloaded, stringField(response, "turnId"))
	if !ok {
		t.Fatalf("image quarantine turn is missing: %#v", reloaded["turns"])
	}
	securityContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil {
		t.Fatal(err)
	}
	restartPlan, err := handler.attachmentUses.PlanRestartDispositions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, receipt := range restartPlan.OpenReceipts {
		if receipt.Context.ContextDigest == securityContext.ContextDigest &&
			receipt.Context.ThreadID == securityContext.ThreadID && receipt.Context.TurnID == securityContext.TurnID {
			t.Fatalf("terminal image quarantine turn retained an open attachment use: %#v", receipt)
		}
	}
}

func TestRuntimeVisionBridgeUserAttachmentNativeVisionPrimaryBypassesBridge(t *testing.T) {
	imageBase64 := base64.StdEncoding.EncodeToString([]byte("attachment-png"))
	fakeProvider := &recordingVisionBridgeProvider{}
	handler := &runtimeServerHandler{
		provider: fakeProvider,
		visionBridge: runtimeVisionBridgeConfig{
			Enabled:             true,
			Mode:                "auto",
			ProviderID:          "aliyun",
			BaseURL:             "https://dashscope.aliyuncs.com/compatible-mode/v1",
			APIKey:              "aliyun-key",
			Model:               "qwen3-vl-plus",
			SemanticProbeStatus: "supported",
		},
	}
	attachments := appturn.ResolvedAttachments{
		IDs: []string{"att_1"},
		Parts: []appturn.ResolvedAttachmentPart{{
			ID:       "att_1",
			MIMEType: "image/png",
			Source:   "image",
			MessagePart: provider.MessagePart{
				Type:      "image",
				MediaType: "image/png",
				Data:      imageBase64,
			},
		}},
		MessageParts: []provider.MessagePart{{
			Type:      "image",
			MediaType: "image/png",
			Data:      imageBase64,
		}},
		ImageCandidates: []appturn.ResolvedAttachmentImage{{ID: "att_1", MIMEType: "image/png", DataBase64: imageBase64}},
		ImageCount:      1,
		ImageMIMETypes:  []string{"image/png"},
	}

	prepared, err := handler.runtimeVisionBridgeService().PrepareAttachments(context.Background(), visionbridgeapp.PrepareAttachmentsInput{Attachments: attachments, Primary: provider.TurnConfig{
		SupportsImageInput: true,
		InputModalities:    []string{"text", "image"},
		MessageParts:       []string{"text", "image_url"},
	}, PrimaryProviderID: "aliyun", PrimaryModel: "qwen3-vl-plus", ProviderTelemetry: serverVisionTelemetryBindingV1(t), Authorize: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}

	if len(fakeProvider.requests) != 0 {
		t.Fatalf("native vision primary should bypass bridge in auto mode")
	}
	if len(prepared.MessageParts) != 1 || prepared.MessageParts[0].Type != "image" || prepared.MessageParts[0].Data != imageBase64 {
		t.Fatalf("native primary image part should be preserved: %#v", prepared.MessageParts)
	}
	if prepared.VisionBridgeStatus != "skipped" {
		t.Fatalf("pipeline should mark bridge skipped for native vision primary: %#v", prepared.PipelineDetails())
	}
}

func TestRuntimeVisionBridgeUnavailableDoesNotReflectProviderBody(t *testing.T) {
	imageBase64 := base64.StdEncoding.EncodeToString([]byte("attachment-png"))
	const rawProviderBody = "RAW_PROVIDER_BODY_SENTINEL_731"
	fakeProvider := &recordingVisionBridgeProvider{err: errors.New(rawProviderBody)}
	handler := &runtimeServerHandler{
		provider: fakeProvider,
		providerConfig: provider.NewRuntimeProviderConfigSet(provider.RuntimeProviderConfigInput{
			DefaultProviderID:     "deepseek",
			DefaultBaseURL:        "https://api.deepseek.com",
			DefaultAPIKey:         "deepseek-key",
			DefaultEndpointFormat: "chat_completions",
			DefaultModel:          "deepseek-v4-pro",
		}),
		visionBridge: runtimeVisionBridgeConfig{
			Enabled:               true,
			Mode:                  "auto",
			ProviderID:            "aliyun",
			BaseURL:               "https://dashscope.aliyuncs.com/compatible-mode/v1",
			APIKey:                "aliyun-key",
			EndpointFormat:        "chat_completions",
			Model:                 "qwen3-vl-plus",
			MaxImageBytes:         1500000,
			MaxScreenshotsPerTurn: 4,
			SemanticProbeStatus:   "supported",
		},
	}
	attachments := appturn.ResolvedAttachments{
		IDs: []string{"att_1"},
		Parts: []appturn.ResolvedAttachmentPart{{
			ID:              "att_1",
			MIMEType:        "image/png",
			Source:          "text_fallback",
			IsImageFallback: true,
			MessagePart:     provider.MessagePart{Type: "text", Text: imageBase64},
		}},
		MessageParts:      []provider.MessagePart{{Type: "text", Text: imageBase64}},
		ImageCandidates:   []appturn.ResolvedAttachmentImage{{ID: "att_1", MIMEType: "image/png", DataBase64: imageBase64}},
		TextFallbackCount: 1,
		TextMIMETypes:     []string{"image/png"},
	}

	prepared, err := handler.runtimeVisionBridgeService().PrepareAttachments(context.Background(), visionbridgeapp.PrepareAttachmentsInput{Attachments: attachments, Primary: provider.TurnConfig{
		SupportsImageInput: false,
		InputModalities:    []string{"text"},
		MessageParts:       []string{"text"},
	}, PrimaryProviderID: "deepseek", PrimaryModel: "deepseek-v4-pro", ProviderTelemetry: serverVisionTelemetryBindingV1(t), Authorize: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}

	if len(fakeProvider.requests) != 0 {
		t.Fatalf("unavailable image effect reached bridge provider: %#v", fakeProvider.requests)
	}
	if prepared.VisionBridgeStatus != "unavailable" || prepared.VisionBridgeUsed ||
		prepared.VisionBridgeReason != "trusted local image privacy projection is unavailable" {
		t.Fatalf("pipeline should record fixed image-effect unavailable reason: %#v", prepared.PipelineDetails())
	}
	primaryText := prepared.MessageParts[0].Text
	if strings.Contains(primaryText, imageBase64) {
		t.Fatalf("unavailable bridge observation must not leak image fallback bytes: %s", primaryText)
	}
	if strings.Contains(primaryText, rawProviderBody) || !strings.Contains(primaryText, "trusted local image privacy projection is unavailable") {
		t.Fatalf("image-effect unavailable result reflected provider body or omitted fixed reason: %s", primaryText)
	}
}

func TestRuntimeVisionBridgeProbeMustPassBeforeProviderCall(t *testing.T) {
	imageBase64 := "iVBORw0KGgo="
	fakeProvider := &recordingVisionBridgeProvider{}
	handler := &runtimeServerHandler{
		provider: fakeProvider,
		visionBridge: runtimeVisionBridgeConfig{
			Enabled:               true,
			Mode:                  "auto",
			ProviderID:            "xiaomi",
			BaseURL:               "https://api.xiaomimimo.com/v1",
			APIKey:                "xiaomi-key",
			EndpointFormat:        "chat_completions",
			Model:                 "mimo-v2.5",
			MaxImageBytes:         1500000,
			MaxScreenshotsPerTurn: 4,
			SemanticProbeStatus:   "semantic_failed",
		},
	}
	pending := runtimePendingToolCall{
		ProviderConfig: provider.TurnConfig{SupportsImageInput: false},
		Call:           provider.ToolCall{ID: "call_get_state", Name: "mcp__analytix-computer-use__get_app_state"},
	}
	output := map[string]any{
		"images": []any{map[string]any{
			"mime_type":   "image/png",
			"data_base64": imageBase64,
		}},
	}

	persistOutput, modelContent := handler.prepareRuntimeToolResultForModel(context.Background(), pending, output, false)
	if len(fakeProvider.requests) != 0 {
		t.Fatalf("vision bridge provider must not be called before semantic probe passes")
	}
	persistJSON, _ := json.Marshal(persistOutput)
	if strings.Contains(string(persistJSON), imageBase64) || strings.Contains(string(persistJSON), "data_base64") {
		t.Fatalf("persisted tool result must redact image bytes even when bridge is unavailable: %s", string(persistJSON))
	}
	if strings.Contains(modelContent, imageBase64) {
		t.Fatalf("model-facing tool result must redact image bytes even when bridge is unavailable: %s", modelContent)
	}
	if !strings.Contains(modelContent, "tool result media requires attempt-local private authority") {
		t.Fatalf("model-facing result should explain bridge unavailability: %s", modelContent)
	}
}

func TestRuntimeVisionBridgeToolResultObservesMultipleImagesWithinBudget(t *testing.T) {
	firstImage := "iVBORw0KGgox"
	secondImage := "iVBORw0KGgoy"
	thirdImage := "iVBORw0KGgoz"
	fakeProvider := &recordingVisionBridgeProvider{
		result: provider.Result{
			Chunks: []provider.Chunk{{
				Kind: provider.ChunkText,
				Text: `{"source_kind":"tool_screenshot","summary":"screen","visible_text":["ok"],"ui_elements":[],"selected_text":"","warnings":[]}`,
			}},
			StreamCompleted: true,
		},
	}
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "deepseek", BaseURL: "https://api.deepseek.com", APIKey: "deepseek-key",
		EndpointFormat: "chat_completions", Model: "deepseek-v4-pro",
	}).(*runtimeServerHandler)
	handler.provider = fakeProvider
	handler.providerConfig = provider.NewRuntimeProviderConfigSet(provider.RuntimeProviderConfigInput{
		DefaultProviderID:     "deepseek",
		DefaultBaseURL:        "https://api.deepseek.com",
		DefaultAPIKey:         "deepseek-key",
		DefaultEndpointFormat: "chat_completions",
		DefaultModel:          "deepseek-v4-pro",
	})
	handler.visionBridge = runtimeVisionBridgeConfig{
		Enabled:               true,
		Mode:                  "auto",
		ProviderID:            "aliyun",
		BaseURL:               "https://dashscope.aliyuncs.com/compatible-mode/v1",
		APIKey:                "aliyun-key",
		EndpointFormat:        "chat_completions",
		Model:                 "qwen3-vl-plus",
		MaxImageBytes:         1500000,
		MaxScreenshotsPerTurn: 2,
		SemanticProbeStatus:   "supported",
	}
	pending := prepareAuthorizedVisionBridgePending(t, handler, provider.TurnConfig{SupportsImageInput: false, Model: "deepseek-v4-pro"})
	output := map[string]any{
		"images": []any{
			map[string]any{"mime_type": "image/png", "data_base64": firstImage},
			map[string]any{"mime_type": "image/png", "data_base64": secondImage},
			map[string]any{"mime_type": "image/png", "data_base64": thirdImage},
		},
	}

	persistOutput, modelContent := handler.prepareRuntimeToolResultForModel(context.Background(), pending, output, false)
	if len(fakeProvider.requests) != 0 {
		t.Fatalf("budgeted tool media escaped without attempt-local private authority: %#v", fakeProvider.requests)
	}
	persistJSON, _ := json.Marshal(persistOutput)
	persistContent := string(persistJSON)
	if strings.Contains(persistContent, firstImage) || strings.Contains(persistContent, secondImage) ||
		strings.Contains(persistContent, thirdImage) || strings.Contains(persistContent, "data_base64") {
		t.Fatalf("persisted tool result must redact all image bytes, including omitted images: %s", persistContent)
	}
	if strings.Contains(modelContent, firstImage) || strings.Contains(modelContent, secondImage) || strings.Contains(modelContent, thirdImage) {
		t.Fatalf("model-facing tool result must redact all image bytes: %s", modelContent)
	}
	for _, expected := range []string{`"imageCount":2`, `"omittedCount":1`, "attempt-local private authority"} {
		if !strings.Contains(modelContent, expected) {
			t.Fatalf("model content missing %q: %s", expected, modelContent)
		}
	}
}

func TestRuntimeVisionBridgeCapabilityRequiresProbeAndAPIKey(t *testing.T) {
	handler := &runtimeServerHandler{
		visionBridge: runtimeVisionBridgeConfig{
			Enabled:             true,
			Mode:                "auto",
			ProviderID:          "xiaomi",
			BaseURL:             "https://api.xiaomimimo.com/v1",
			Model:               "mimo-v2.5",
			SemanticProbeStatus: "supported",
		},
	}
	state := handler.runtimeVisionBridgeCapabilityState()
	if boolField(state, "available") || stringField(state, "status") != "unavailable" {
		t.Fatalf("bridge without API key must be unavailable: %#v", state)
	}
	if !strings.Contains(stringField(state, "reason"), "API key") {
		t.Fatalf("bridge should explain missing API key: %#v", state)
	}

	handler.visionBridge.APIKey = "xiaomi-key"
	handler.visionBridge.SemanticProbeStatus = "semantic_failed"
	state = handler.runtimeVisionBridgeCapabilityState()
	if boolField(state, "available") {
		t.Fatalf("bridge with failed semantic probe must be unavailable: %#v", state)
	}
	if !strings.Contains(stringField(state, "reason"), "semantic probe") {
		t.Fatalf("bridge should explain missing semantic probe pass: %#v", state)
	}

	handler.visionBridge.SemanticProbeStatus = "supported"
	state = handler.runtimeVisionBridgeCapabilityState()
	if !boolField(state, "available") || stringField(state, "status") != "available" {
		t.Fatalf("bridge with key and supported probe should be available: %#v", state)
	}
	if strings.Contains(stringField(state, "reason"), "disabled") {
		t.Fatalf("available bridge must not keep a stale disabled reason: %#v", state)
	}
}
