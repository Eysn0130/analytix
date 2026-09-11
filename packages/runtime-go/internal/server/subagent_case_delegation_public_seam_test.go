package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	evidenceregistryadapter "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	evidencesettlementadapter "analytix.local/runtime-go/internal/adapters/outbound/evidencesettlement"
	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	mcpprotocol "analytix.local/runtime-go/internal/adapters/outbound/mcp/protocol"
	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	gateprojectionapp "analytix.local/runtime-go/internal/app/gateprojection"
	apploop "analytix.local/runtime-go/internal/app/loop"
	appmodel "analytix.local/runtime-go/internal/app/model"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	turnstartapp "analytix.local/runtime-go/internal/app/turnstart"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	jobs "analytix.local/runtime-go/internal/jobs"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
	runtimeprovider "analytix.local/runtime-go/internal/provider"
)

const caseDelegationDeepSeekReasoningCanaryV1 = "PRIVATE_DEEPSEEK_REASONING_SLICE21_TEST_ONLY"

type caseDelegationChildCaptureProviderV1 struct {
	rawTaskPrompt string
	mu            sync.Mutex
	requests      []domainmodel.Request
	parentCalls   int
	childCalls    int
}

func (*caseDelegationChildCaptureProviderV1) RequiresDurablePipelineStagesV1() {}

func (providerClient *caseDelegationChildCaptureProviderV1) Stream(
	_ context.Context,
	request domainmodel.Request,
) (domainmodel.Result, error) {
	if err := emitTestDurableProviderPipelinePairV1(request); err != nil {
		return domainmodel.Result{}, err
	}
	providerClient.mu.Lock()
	providerClient.requests = append(providerClient.requests, cloneProviderStepRequest(request))
	child := request.PrivateProviderTelemetry != nil && strings.TrimSpace(request.PrivateProviderTelemetry.ChildRunID) != ""
	if child {
		providerClient.childCalls++
	} else {
		providerClient.parentCalls++
	}
	parentCall := providerClient.parentCalls
	childCall := providerClient.childCalls
	providerClient.mu.Unlock()

	if child {
		switch childCall {
		case 1:
			selection, err := caseForegroundSelectionFromProviderRequestV1(request)
			if err != nil {
				return domainmodel.Result{}, err
			}
			arguments, _ := json.Marshal(map[string]any{"caseResult": selection})
			return caseDelegationProviderToolCallV1(request, domainmodel.ToolCall{
				ID: "provider-case-child-submit", Name: "submit_child_result", Arguments: arguments,
			})
		case 2:
			return caseDelegationProviderTextV1(request, "closed selector submitted to host")
		default:
			return domainmodel.Result{}, errors.New("unexpected physical child provider call")
		}
	}

	switch parentCall {
	case 1:
		arguments := json.RawMessage(`{"subject_alias":"acct:1","start_inclusive":"2026-01-01T00:00:00.000000Z","end_inclusive":"2026-01-31T23:59:59.000000Z","evidence_row_limit":1}`)
		return caseDelegationProviderToolCallV1(request, domainmodel.ToolCall{
			ID: "provider-case-parent-account-flow", Name: providerStepFundsTool, Arguments: arguments,
		})
	case 2:
		arguments, _ := json.Marshal(map[string]any{
			"prompt": providerClient.rawTaskPrompt, "max_steps": 2, "token_budget": 512, "time_budget_ms": 30000,
		})
		return caseDelegationProviderToolCallV1(request, domainmodel.ToolCall{
			ID: "provider-case-delegation-task", Name: "task", Arguments: arguments,
		})
	case 3:
		if !providerRequestContainsPrivateCaseResultV1(request) {
			return domainmodel.Result{}, errors.New("parent provider did not receive the exact host-private typed child result")
		}
		return caseDelegationProviderTextV1(request, "请按宿主 Final Evidence Gate 发布已核验的账户资金流结论。")
	default:
		return domainmodel.Result{}, errors.New("unexpected physical parent provider call")
	}
}

func caseDelegationProviderToolCallV1(
	request domainmodel.Request,
	call domainmodel.ToolCall,
) (domainmodel.Result, error) {
	chunks := make([]domainmodel.Chunk, 0, 2)
	if request.ReasoningProtocol == "deepseek-chat-completions" {
		reasoning := domainmodel.Chunk{Kind: domainmodel.ChunkReasoning, Text: caseDelegationDeepSeekReasoningCanaryV1}
		if request.OnChunk != nil {
			if err := request.OnChunk(reasoning); err != nil {
				return domainmodel.Result{}, err
			}
		}
		chunks = append(chunks, reasoning)
	}
	if request.OnChunk != nil {
		if err := request.OnChunk(domainmodel.Chunk{Kind: domainmodel.ChunkToolCallStart, ToolCall: domainmodel.ToolCall{ID: call.ID, Name: call.Name}}); err != nil {
			return domainmodel.Result{}, err
		}
	}
	chunks = append(chunks, domainmodel.Chunk{Kind: domainmodel.ChunkToolCall, ToolCall: call})
	return domainmodel.Result{Chunks: chunks, StreamCompleted: true}, nil
}

func caseDelegationProviderTextV1(
	request domainmodel.Request,
	text string,
) (domainmodel.Result, error) {
	chunks := make([]domainmodel.Chunk, 0, 2)
	if request.ReasoningProtocol == "deepseek-chat-completions" {
		reasoning := domainmodel.Chunk{Kind: domainmodel.ChunkReasoning, Text: caseDelegationDeepSeekReasoningCanaryV1}
		if request.OnChunk != nil {
			if err := request.OnChunk(reasoning); err != nil {
				return domainmodel.Result{}, err
			}
		}
		chunks = append(chunks, reasoning)
	}
	chunk := domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: text}
	if request.OnChunk != nil {
		if err := request.OnChunk(chunk); err != nil {
			return domainmodel.Result{}, err
		}
	}
	chunks = append(chunks, chunk)
	return domainmodel.Result{Chunks: chunks, StreamCompleted: true}, nil
}

func caseForegroundSelectionFromProviderRequestV1(
	request domainmodel.Request,
) (domainjob.CaseForegroundChildSelectionV1, error) {
	const startTag = "<analytix_host_case_delegation_v1>"
	const endTag = "</analytix_host_case_delegation_v1>"
	for _, candidate := range append([]string{request.SystemPrompt}, providerMessageContentsV1(request.Messages)...) {
		start := strings.Index(candidate, startTag)
		end := strings.Index(candidate, endTag)
		if start < 0 || end <= start {
			continue
		}
		body := strings.TrimSpace(candidate[start+len(startTag) : end])
		var delegation struct {
			TaskKind    string `json:"taskKind"`
			Currentness string `json:"currentness"`
			Entities    []struct {
				Alias                domaincaseentity.ModelEntityAliasV1 `json:"alias"`
				EntityType           string                              `json:"entityType"`
				FinancialAccountType string                              `json:"financialAccountType"`
			} `json:"entities"`
			Claims           []domainjob.CaseDelegatedClaimReferenceV1       `json:"claims"`
			Evidence         []domainjob.CaseDelegatedEvidenceReferenceV1    `json:"evidence"`
			DelegationDigest string                                          `json:"delegationDigest"`
			AnswerSlots      []domainjob.CaseDelegatedAnswerSlotCommitmentV1 `json:"answerSlots"`
		}
		if json.Unmarshal([]byte(body), &delegation) != nil ||
			delegation.TaskKind != domainjob.CaseDelegationTaskKindV1 ||
			delegation.Currentness != domaincaseentity.SnapshotCurrentV1 ||
			len(delegation.Entities) != 1 || delegation.Entities[0].Alias != "acct:1" ||
			strings.TrimSpace(delegation.Entities[0].EntityType) == "" ||
			strings.TrimSpace(delegation.Entities[0].FinancialAccountType) == "" ||
			len(delegation.Claims) != 0 || len(delegation.Evidence) != 0 ||
			len(delegation.AnswerSlots) != 1 || delegation.AnswerSlots[0].ClaimCount != 3 ||
			delegation.AnswerSlots[0].EvidenceCount != 1 || delegation.AnswerSlots[0].GapCount != 0 {
			continue
		}
		selection := domainjob.CaseForegroundChildSelectionV1{
			SchemaVersion:    domainjob.CaseForegroundChildResultSchemaVersionV1,
			Purpose:          domainjob.CaseForegroundChildSelectionPurposeV1,
			DelegationDigest: delegation.DelegationDigest,
			AnswerSlotDigest: delegation.AnswerSlots[0].Digest,
		}
		if selection.Purpose == domainjob.CaseForegroundChildSelectionPurposeV1 &&
			domainjob.ValidateCaseDelegationProviderCommitmentTokenV1(selection.DelegationDigest) == nil &&
			domainjob.ValidateCaseDelegationProviderCommitmentTokenV1(selection.AnswerSlotDigest) == nil {
			return selection, nil
		}
	}
	return domainjob.CaseForegroundChildSelectionV1{}, errors.New("child provider request omitted the closed case selector commitment")
}

func providerMessageContentsV1(messages []domainmodel.Message) []string {
	contents := make([]string, 0, len(messages))
	for _, message := range messages {
		contents = append(contents, message.Content)
	}
	return contents
}

func providerRequestContainsPrivateCaseResultV1(request domainmodel.Request) bool {
	for _, message := range request.Messages {
		content := message.Content
		if message.Role == "user" && message.Name == "" && message.ToolCallID == "" {
			var opened bool
			content, opened = appmodel.CompletedPrivateProtocolSafeHistoryContentV1(content, "task")
			if !opened {
				continue
			}
		} else if message.Role != "tool" || message.Name != "task" {
			continue
		}
		var output map[string]any
		if json.Unmarshal([]byte(content), &output) == nil {
			if _, ok := output["caseResult"].(map[string]any); ok && len(output) == 17 {
				return true
			}
		}
	}
	return false
}

type caseForegroundPublicSeamEvidenceCapabilityV1 struct {
	mu        sync.Mutex
	active    bool
	ctx       context.Context
	context   domainsecurity.TurnSecurityContext
	probe     domainsecurity.VerifiedSourceProbe
	selection datasetsnapshotport.CurrentSelectionV2
}

func (capability *caseForegroundPublicSeamEvidenceCapabilityV1) DatasetSelection() (datasetsnapshotport.CurrentSelectionV2, error) {
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if !capability.active || capability.ctx == nil || capability.ctx.Err() != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, errors.New("case foreground evidence capability is inactive")
	}
	return cloneServerCurrentSelectionV1(capability.selection), nil
}

func (capability *caseForegroundPublicSeamEvidenceCapabilityV1) UseExact(
	securityContext domainsecurity.TurnSecurityContext,
	probe domainsecurity.VerifiedSourceProbe,
	selection datasetsnapshotport.CurrentSelectionV2,
	callback func(context.Context) error,
) error {
	capability.mu.Lock()
	active := capability.active && capability.ctx != nil && capability.ctx.Err() == nil &&
		securityContext == capability.context && probe == capability.probe &&
		selection.SelectionDigest == capability.selection.SelectionDigest && callback != nil
	leaseContext := capability.ctx
	capability.mu.Unlock()
	if !active {
		return errors.New("case foreground evidence capability exact binding mismatch")
	}
	return callback(leaseContext)
}

type caseForegroundPublicSeamMCPV1 struct {
	*providerStepIntegrationMCP
	dataset *serverSignedCurrentDatasetV1

	mu                sync.Mutex
	probes            map[string]domainsecurity.VerifiedSourceProbe
	semantic          domainnative.AccountFlowProviderSemanticResultV1
	raw               domainmcp.LosslessToolResult
	subjectRef        string
	toolContextDigest string
	consumed          bool
	discarded         bool
	toolCalls         int
	lastAuthorityErr  string
}

type caseForegroundPublicSeamFinalizerV1 struct {
	evidenceapp.CasePublicationFinalizer
	tool   evidenceapp.ToolEvidenceAuthority
	source *caseForegroundPublicSeamMCPV1
}

func (finalizer *caseForegroundPublicSeamFinalizerV1) PrepareCurrentToolEvidence(
	ctx context.Context,
	input evidenceapp.PrepareToolEvidenceInput,
) (evidenceapp.PreparedToolEvidence, bool, error) {
	prepared, eligible, err := finalizer.tool.PrepareCurrentToolEvidence(ctx, input)
	finalizer.record(err)
	return prepared, eligible, err
}

func (finalizer *caseForegroundPublicSeamFinalizerV1) CommitCurrentToolEvidence(
	ctx context.Context,
	input evidenceapp.CommitToolEvidenceInput,
) (domainevidence.EvidenceReceipt, error) {
	receipt, err := finalizer.tool.CommitCurrentToolEvidence(ctx, input)
	finalizer.record(err)
	return receipt, err
}

func (finalizer *caseForegroundPublicSeamFinalizerV1) record(err error) {
	if err == nil || finalizer.source == nil {
		return
	}
	finalizer.source.mu.Lock()
	finalizer.source.lastAuthorityErr = err.Error()
	finalizer.source.mu.Unlock()
}

func newCaseForegroundPublicSeamMCPV1(
	t *testing.T,
	dataset *serverSignedCurrentDatasetV1,
) *caseForegroundPublicSeamMCPV1 {
	t.Helper()
	serverIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"analytix_funds", "analytix_funds", "0.16.15",
		domainsecurity.SHA256Hex([]byte("turn-start-admission-failure-source-instance")), 11,
	)
	if err != nil {
		t.Fatal(err)
	}
	advertisement := domainmcp.ToolAdvertisementV1{
		Name: providerStepFundsTool, Description: "Analyze one bounded current-case account-flow scope",
		InputSchema: json.RawMessage(`{
  "type":"object",
  "properties":{
    "subject_alias":{"type":"string","pattern":"^(acct|card):[1-9][0-9]{0,8}$","minLength":6,"maxLength":15},
    "start_inclusive":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\.[0-9]{6}Z$","minLength":27,"maxLength":27},
    "end_inclusive":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\.[0-9]{6}Z$","minLength":27,"maxLength":27},
    "evidence_row_limit":{"type":"integer","minimum":1,"maximum":512}
  },
  "required":["subject_alias","start_inclusive","end_inclusive","evidence_row_limit"],
  "additionalProperties":false
}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"schemaVersion":{"type":"integer"},"purpose":{"type":"string"}},"required":["schemaVersion","purpose"],"additionalProperties":false}`),
		TaskSupport:  domainmcp.ToolTaskSupportForbidden, ReadOnly: true,
		ConnectionEpoch: 11, ServerIdentity: serverIdentity,
	}
	semantic := domainnative.AccountFlowProviderSemanticResultV1{
		SubjectAlias:   "acct:1",
		StartInclusive: "2026-01-01T00:00:00.000000Z",
		EndInclusive:   "2026-01-31T23:59:59.000000Z",
		Timezone:       "Z", Currency: "CNY", MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
		InflowMinor: "100", OutflowMinor: "0", NetMinor: "100",
		TransactionCount: 1, EvidenceTransactionCount: 1, EvidenceRowLimit: 1,
		AggregateComplete: true, EvidenceRowsComplete: true, CounterpartySemanticsComplete: true,
		Currentness: domainnative.AccountFlowProviderCurrentnessCurrentV1,
		Coverage: domainnative.AccountFlowProviderSemanticCoverageV1{
			State: domainnative.AccountFlowCoverageCompleteV1, Gaps: []string{},
			NormalizedSnapshotRows: 1, AcceptedSnapshotRows: 1, ObservedMatchingRows: 1,
		},
		Transactions: []domainnative.AccountFlowProviderSemanticTransactionV1{{
			EvidenceRef: "srow1_" + domainsecurity.SHA256Hex([]byte("case-foreground-public-seam-row")),
			Counterparty: domainnative.AccountFlowProviderCounterpartyV1{
				Status: domainnative.AccountFlowCounterpartyResolvedV1, Alias: "acct:1",
				EntityType:  domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
				AccountType: domainnative.AccountFlowCounterpartyAccountTypeV1,
			},
			OccurredAt: "2026-01-05T10:30:00.000000Z", Direction: domainnative.AccountFlowDirectionInflowV1,
			AmountMinor: "100", Currency: "CNY", MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
		}},
		QueryHash:  domainsecurity.SHA256Hex([]byte("case-foreground-public-seam-query")),
		ResultHash: domainsecurity.SHA256Hex([]byte("case-foreground-public-seam-result")),
	}
	outcome, err := domainnative.NewAccountFlowProviderOutcomeV1(
		semantic.SubjectAlias, semantic.AggregateComplete, semantic.EvidenceRowsComplete,
		semantic.QueryHash, semantic.ResultHash,
	)
	if err != nil {
		t.Fatal(err)
	}
	semantic.Outcome = outcome
	if err := domainnative.ValidateAccountFlowProviderSemanticResultV1(semantic, 1, domainevidence.SemanticSuccess); err != nil {
		t.Fatalf("construct complete account-flow semantic: %v", err)
	}
	return &caseForegroundPublicSeamMCPV1{
		providerStepIntegrationMCP: &providerStepIntegrationMCP{
			admissionFailureMCP: &admissionFailureMCP{}, advertisements: []domainmcp.ToolAdvertisementV1{advertisement},
		},
		dataset: dataset, probes: map[string]domainsecurity.VerifiedSourceProbe{}, semantic: semantic,
	}
}

func (source *caseForegroundPublicSeamMCPV1) ProbeCaseSource(
	ctx context.Context,
	input sourceprobeport.Input,
) (domainsecurity.VerifiedSourceProbe, error) {
	probe, err := source.admissionFailureMCP.ProbeCaseSource(ctx, input)
	if err != nil {
		return domainsecurity.VerifiedSourceProbe{}, err
	}
	source.mu.Lock()
	source.probes[probe.ProbeContextDigest] = probe
	source.mu.Unlock()
	return probe, nil
}

func (source *caseForegroundPublicSeamMCPV1) ValidateCurrentProbe(
	ctx context.Context,
	serverID string,
	securityContext domainsecurity.TurnSecurityContext,
) error {
	if ctx == nil || ctx.Err() != nil {
		return errors.New("case foreground current probe context is unavailable")
	}
	source.mu.Lock()
	probe, ok := source.probes[securityContext.ContextDigest]
	source.mu.Unlock()
	if !ok || probe.ServerID != serverID || !turnsecurityapp.SourceDiscoveryMatchesContext(probe, securityContext) {
		return errors.New("case foreground current source probe is unavailable")
	}
	return nil
}

func (source *caseForegroundPublicSeamMCPV1) CallToolSecurityBoundContext(
	ctx context.Context,
	toolName string,
	approved bool,
	envelope domainmcp.HostContextEnvelope,
	arguments ...map[string]any,
) map[string]any {
	securityContext, grant, err := envelope.Authority()
	canonicalArguments, argumentsErr := json.Marshal(firstCaseForegroundMCPArgumentsV1(arguments))
	if err != nil || ctx == nil || ctx.Err() != nil || !approved || toolName != providerStepFundsTool ||
		len(arguments) != 1 || argumentsErr != nil || domainsecurity.ValidateExecutionGrantForContext(grant, securityContext) != nil ||
		grant.ToolName != toolName || grant.ArgsHash != domainsecurity.CanonicalJSONHash(canonicalArguments) ||
		source.ValidateCurrentProbe(ctx, "analytix_funds", securityContext) != nil {
		return map[string]any{"executed": false, "isError": true, "code": "case_foreground_account_flow_authority_invalid"}
	}
	raw := mcpprotocol.ExtractLosslessToolResult([]byte(`{"content":[],"structuredContent":{"schemaVersion":1,"purpose":"analytix.funds-account-flow-analysis/v1","semanticStatus":"success","data":{}}}`))
	if !domainmcp.ValidLosslessToolResult(raw) {
		return map[string]any{"executed": false, "isError": true, "code": "case_foreground_account_flow_raw_invalid"}
	}
	source.mu.Lock()
	source.raw = raw
	source.toolContextDigest = securityContext.ContextDigest
	source.consumed = false
	source.discarded = false
	source.toolCalls++
	source.mu.Unlock()
	return map[string]any{
		"executed": true, "isError": false, "transportStatus": "success", "semanticStatus": "success",
		"result":                       map[string]any{"schemaVersion": 1, "purpose": "analytix.funds-account-flow-analysis/v1"},
		domainmcp.HostRawToolResultKey: raw,
	}
}

func firstCaseForegroundMCPArgumentsV1(arguments []map[string]any) map[string]any {
	if len(arguments) != 1 {
		return nil
	}
	return arguments[0]
}

func (*caseForegroundPublicSeamMCPV1) WithCurrentEvidenceRead(
	context.Context,
	sourceprobeport.EvidenceReadInput,
	func(domainsecurity.VerifiedSourceProbe, domainmcp.LosslessToolResult) error,
) error {
	return errors.New("legacy evidence read path is unavailable")
}

func (source *caseForegroundPublicSeamMCPV1) WithCurrentProbeAuthority(
	ctx context.Context,
	input sourceprobeport.CurrentInput,
	callback func(domainsecurity.VerifiedSourceProbe, sourceprobeport.HostEvidenceCapability) error,
) error {
	if callback == nil || source.ValidateCurrentProbe(ctx, input.ServerID, input.Context) != nil ||
		input.Binding.WorkspaceRealPath != input.Context.WorkspaceRealPath ||
		input.Binding.CaseID != input.Context.CaseID || input.Binding.CaseBindingHash != input.Context.CaseBindingHash {
		return errors.New("case foreground current host evidence authority is unavailable")
	}
	source.mu.Lock()
	probe := source.probes[input.Context.ContextDigest]
	source.mu.Unlock()
	capability := &caseForegroundPublicSeamEvidenceCapabilityV1{
		active: true, ctx: ctx, context: input.Context, probe: probe,
		selection: cloneServerCurrentSelectionV1(source.dataset.selection),
	}
	defer func() {
		capability.mu.Lock()
		capability.active = false
		capability.mu.Unlock()
	}()
	callbackErr := callback(probe, capability)
	source.mu.Lock()
	if callbackErr != nil {
		source.lastAuthorityErr = callbackErr.Error()
	}
	source.mu.Unlock()
	return callbackErr
}

func (source *caseForegroundPublicSeamMCPV1) WithFreshPublicationSnapshot(
	context.Context,
	sourceprobeport.PublicationInput,
	func([]domainsecurity.VerifiedSourceProbe) error,
) error {
	return errors.New("legacy publication snapshot path is unavailable")
}

func (source *caseForegroundPublicSeamMCPV1) WithFreshPublicationSnapshotAuthority(
	ctx context.Context,
	input sourceprobeport.PublicationInput,
	callback func([]domainsecurity.VerifiedSourceProbe, sourceprobeport.HostEvidenceCapability) error,
) error {
	if callback == nil || len(input.Requirements) != 1 ||
		source.ValidateCurrentProbe(ctx, input.Requirements[0].ServerID, input.Context) != nil {
		return errors.New("case foreground publication source authority is unavailable")
	}
	source.mu.Lock()
	probe := source.probes[input.Context.ContextDigest]
	source.mu.Unlock()
	requirement := input.Requirements[0]
	if requirement.ServerIdentity != probe.ServerIdentity || requirement.ServerVersion != "0.16.15" ||
		requirement.ConnectionEpoch != probe.ConnectionEpoch || requirement.ToolName != providerStepFundsTool ||
		requirement.DatasetSnapshotID != input.Context.DatasetSnapshotID {
		return errors.New("case foreground publication source requirement is mismatched")
	}
	capability := &caseForegroundPublicSeamEvidenceCapabilityV1{
		active: true, ctx: ctx, context: input.Context, probe: probe,
		selection: cloneServerCurrentSelectionV1(source.dataset.selection),
	}
	defer func() {
		capability.mu.Lock()
		capability.active = false
		capability.mu.Unlock()
	}()
	callbackErr := callback([]domainsecurity.VerifiedSourceProbe{probe}, capability)
	source.mu.Lock()
	if callbackErr != nil {
		source.lastAuthorityErr = callbackErr.Error()
	}
	source.mu.Unlock()
	return callbackErr
}

func (source *caseForegroundPublicSeamMCPV1) HostFundsAccountFlowProviderSemanticV1(
	result domainmcp.LosslessToolResult,
) (domainnative.AccountFlowProviderSemanticResultV1, bool) {
	source.mu.Lock()
	defer source.mu.Unlock()
	return source.semantic, domainmcp.ValidLosslessToolResult(result) && result.RawSHA256 == source.raw.RawSHA256 && !source.discarded
}

func (source *caseForegroundPublicSeamMCPV1) ConsumeHostFundsAccountFlowEvidenceV1(
	result domainmcp.LosslessToolResult,
	consumeSummary domainnative.AccountFlowHostEvidenceSummaryConsumerV1,
	consumeRow domainnative.AccountFlowHostEvidenceRowConsumerV1,
) error {
	source.mu.Lock()
	if source.consumed || source.discarded || result.RawSHA256 != source.raw.RawSHA256 ||
		strings.TrimSpace(source.subjectRef) == "" || consumeSummary == nil || consumeRow == nil {
		source.mu.Unlock()
		return errors.New("case foreground account-flow evidence carrier is unavailable")
	}
	source.consumed = true
	semantic := source.semantic
	subjectRef := source.subjectRef
	probe := source.probes[source.toolContextDigest]
	source.mu.Unlock()
	if probe.ProbeContextDigest == "" {
		return errors.New("case foreground account-flow evidence probe is unavailable")
	}
	if err := consumeSummary(
		subjectRef, probe.DatasetSnapshotID, probe.ContextEpoch, probe.ProbeContextDigest, probe.CaseBindingHash,
		semantic.StartInclusive, semantic.EndInclusive, semantic.Timezone, semantic.Currency, semantic.MinorUnitScale,
		semantic.InflowMinor, semantic.OutflowMinor, semantic.NetMinor, semantic.TransactionCount,
		semantic.AggregateComplete, semantic.EvidenceRowsComplete, semantic.EvidenceTransactionCount,
		semantic.Coverage, semantic.QueryHash, semantic.ResultHash,
	); err != nil {
		return err
	}
	for index, row := range semantic.Transactions {
		if err := consumeRow(
			index, subjectRef, row.EvidenceRef, "0123456789abcdefabcd", uint64(index+1),
			row.OccurredAt, row.Direction, row.AmountMinor, row.Currency, row.MinorUnitScale,
		); err != nil {
			return err
		}
	}
	return nil
}

func (source *caseForegroundPublicSeamMCPV1) DiscardHostFundsAccountFlowEvidenceV1(
	result domainmcp.LosslessToolResult,
) {
	source.mu.Lock()
	if result.RawSHA256 == source.raw.RawSHA256 {
		source.discarded = true
	}
	source.mu.Unlock()
}

func configureCaseForegroundPublicSeamAuthoritiesV1(
	t *testing.T,
	handler *runtimeServerHandler,
	durableRoot string,
	dataset *serverSignedCurrentDatasetV1,
	source *caseForegroundPublicSeamMCPV1,
) func() *subagentapp.ForegroundHandoffAuthority {
	t.Helper()
	privateRoot := t.TempDir()
	if err := os.Chmod(privateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	caseKeyRoot := filepath.Join(privateRoot, "case-key")
	finalKeyRoot := filepath.Join(privateRoot, "final-key")
	for _, root := range []string{caseKeyRoot, finalKeyRoot} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	caseSigner, err := finalauthorityadapter.OpenOrCreateFileAuthority(
		filepath.Join(caseKeyRoot, "authority.json"), false,
	)
	if err != nil {
		t.Fatal(err)
	}
	caseStore, err := newServerTestCaseThreadStore(t, filepath.Join(privateRoot, "case-authority-records"))
	if err != nil {
		t.Fatal(err)
	}
	caseAuthority, err := casethreadapp.NewRegistry(context.Background(), caseSigner, caseStore)
	if err != nil {
		t.Fatal(err)
	}
	handler.caseThreads = caseAuthority
	handler.store.SetCaseThreadAuthority(caseAuthority)

	finalSigner, err := finalauthorityadapter.OpenOrCreateFileAuthority(
		filepath.Join(finalKeyRoot, "authority.json"), false,
	)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := evidenceregistryadapter.NewStore(filepath.Join(privateRoot, "evidence-registry"), finalSigner)
	if err != nil {
		t.Fatal(err)
	}
	settlements, err := evidencesettlementadapter.NewStore(filepath.Join(privateRoot, "evidence-settlements"))
	if err != nil {
		t.Fatal(err)
	}
	privateFinals, err := newServerTestPrivateFinalStore(t, filepath.Join(privateRoot, "accepted-finals"))
	if err != nil {
		t.Fatal(err)
	}
	casReader, err := finalauthorityadapter.NewAcceptedFinalCASReader(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	eventIO := durableAcceptedFinalEventIO(handler.store, casReader)
	trustedFinals := gateprojectionapp.NewTrustedFinalProjectionIndexWithReadback(finalSigner, eventIO.Readback)
	validateCurrent := func(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
		return turnsecurityapp.ValidateCurrent(turnsecurityapp.CurrentValidationInput{
			OperationContext: ctx, Identity: handler.turnSecurity.Identity,
			Observer: handler.turnSecurity.Observer, RiskAuthority: handler.turnSecurity.RiskAuthority,
			SnapshotAuthority:   handler.turnSecurity.SnapshotAuthority,
			SnapshotAuthorityV2: handler.turnSecurity.SnapshotAuthorityV2,
			Context:             securityContext, Workspace: securityContext.WorkspaceRealPath,
		})
	}
	finalizer := evidenceapp.NewCasePublicationFinalizerWithHostEvidenceAuthority(
		registry, registry, finalSigner, privateFinals, eventIO,
		newServerTestTurnTerminalCoordinator(t, finalSigner, privateFinals), source, dataset, trustedFinals,
	)
	finalizer = evidenceapp.WithToolEvidenceAuthority(finalizer, evidenceapp.ToolEvidenceService{
		Issuer: evidenceapp.Issuer{Registry: registry, SettlementStore: settlements, Authority: finalSigner},
		Reader: source,
	})
	finalizer = evidenceapp.WithCurrentPublicationAuthority(
		finalizer,
		evidenceapp.CurrentPublicationAuthorityFunc(validateCurrent),
	)
	toolAuthority, ok := finalizer.(evidenceapp.ToolEvidenceAuthority)
	if !ok {
		t.Fatal("case foreground public seam tool evidence authority is unavailable")
	}
	finalizer = &caseForegroundPublicSeamFinalizerV1{
		CasePublicationFinalizer: finalizer, tool: toolAuthority, source: source,
	}
	handler.caseFinalizer = finalizer
	currentCaseAuthority := threadapp.NewCurrentCaseThreadAuthorityValidator(caseAuthority, dataset, nil)
	handler.publicProjector = threadapp.NewTrustedPublicProjectorWithPrimaryCAS(
		trustedFinals, caseAuthority, currentCaseAuthority, casReader,
	)
	handler.threads = nil

	contexts := func(threadID string, turnID string) (domainsecurity.TurnSecurityContext, bool) {
		committed, ok := caseAuthority.CommittedContext(threadID, turnID)
		return committed.SecurityContext, ok
	}
	childCompletions := subagentapp.NewChildCompletionAuthority(
		contexts, trustedFinals, finalSigner, handler.store, validateCurrent,
	)
	handler.childCompletions = childCompletions
	jobManager, err := jobs.NewManagerWithChildCompletionVerifier(
		filepath.Join(handler.dataDir, "child-runs"), childCompletions,
	)
	if err != nil {
		t.Fatal(err)
	}
	handler.jobs = jobManager
	handler.caseAnswerSlots = func(
		ctx context.Context,
		parent domainsecurity.TurnSecurityContext,
		outputs []domainnative.AccountFlowProviderModelOutputV1,
	) ([]domainjob.CaseDelegatedAnswerSlotBindingV1, error) {
		return evidenceapp.ResolveCurrentCaseForegroundAnswerSlotsV1(ctx, registry, parent, outputs)
	}
	primaryReader, err := finalauthorityadapter.NewAcceptedFinalCASReader(handler.store.root)
	if err != nil {
		t.Fatal(err)
	}
	newForegroundAuthority := func() *subagentapp.ForegroundHandoffAuthority {
		return subagentapp.NewForegroundHandoffAuthorityWithCaseTyped(
			subagentapp.NewForegroundSubmissionRegistry(), handler.store, contexts, validateCurrent,
			func(threadID string, turnID string) (subagentapp.ForegroundChildTerminalV1, bool) {
				resolved, terminalErr := appturn.ResolveCommittedGeneralTerminalV1(
					context.Background(), handler.store, primaryReader, threadID, turnID,
				)
				if terminalErr != nil {
					return subagentapp.ForegroundChildTerminalV1{}, false
				}
				return subagentapp.ForegroundChildTerminalV1{
					SecurityContext: resolved.SecurityContext, TerminalDigest: resolved.Commit.CommitDigest,
					TerminalStatus: resolved.Commit.TerminalStatus, TerminalReason: resolved.Commit.TerminalReason,
				}, true
			},
			childCompletions,
			func(
				ctx context.Context,
				parent domainsecurity.TurnSecurityContext,
				result domainjob.CaseForegroundChildResultV1,
			) error {
				return evidenceapp.ValidateCurrentCaseForegroundChildResultV1(ctx, registry, parent, result)
			},
			func(
				ctx context.Context,
				record domainjob.Record,
				parent domainsecurity.TurnSecurityContext,
			) error {
				_, bindErr := subagentapp.BindCaseDelegationV1(ctx, subagentapp.BindCaseDelegationInputV1{
					SecurityContext: parent, SecurityBinding: record.SecurityBinding,
					CaseEntities: handler.caseEntities,
					Request: subagentapp.TaskRequest{
						Prompt:         record.Prompt,
						CaseDelegation: domainjob.CloneCaseDelegationContextV1(record.CaseDelegation),
					},
					Expected: record.CaseDelegation,
				})
				return bindErr
			},
		)
	}
	handler.foregroundHandoffs = newForegroundAuthority()
	return newForegroundAuthority
}

func waitForCaseForegroundPublicSeamTurnsV1(
	t *testing.T,
	handler *runtimeServerHandler,
	threadID string,
	want int,
) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		thread, err := handler.store.GetThread(threadID)
		turns := listAny(thread["turns"])
		if err == nil && stringField(thread, "status") == "idle" && len(turns) >= want {
			last, _ := turns[want-1].(map[string]any)
			if stringField(last, "status") == "completed" {
				return thread
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d case foreground public-seam turns: thread=%#v err=%v", want, thread, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForCaseForegroundPublicSeamCompletionV1(
	t *testing.T,
	handler *runtimeServerHandler,
	providerClient *caseDelegationChildCaptureProviderV1,
	threadID string,
) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		records := handler.jobs.AllRecords()
		providerClient.mu.Lock()
		parentCalls, childCalls := providerClient.parentCalls, providerClient.childCalls
		providerClient.mu.Unlock()
		thread, err := handler.store.GetThread(threadID)
		turns := listAny(thread["turns"])
		parentComplete := err == nil && stringField(thread, "status") == "idle" && len(turns) >= 2
		if parentComplete {
			last, _ := turns[1].(map[string]any)
			parentComplete = stringField(last, "status") == "completed"
		}
		if len(records) == 1 && records[0].Status == string(domainjob.StatusCompleted) &&
			records[0].ChildCompletionReceipt != nil && records[0].ForegroundChildHandoffReceipt != nil &&
			parentCalls == 3 && childCalls == 1 && parentComplete {
			return
		}
		if time.Now().After(deadline) {
			var childThread map[string]any
			var childThreadErr error
			var childEvents []map[string]any
			var childEventsErr error
			if len(records) == 1 && strings.TrimSpace(records[0].ChildThreadID) != "" {
				childThread, childThreadErr = handler.store.GetThread(records[0].ChildThreadID)
				loaded, loadErr := handler.store.LoadEventsSince(records[0].ChildThreadID, 0)
				childEventsErr = loadErr
				for _, event := range loaded.Events {
					childEvents = append(childEvents, map[string]any{
						"kind": stringField(event, "kind"), "status": stringField(event, "status"),
						"stage": stringField(event, "stage"), "code": stringField(event, "code"),
						"failureCode": stringField(event, "failureCode"), "terminalReason": stringField(event, "terminalReason"),
					})
				}
			}
			t.Fatalf("timed out waiting for foreground case completion: recordStatus=%q failureCode=%q parentCalls=%d childCalls=%d threadStatus=%q err=%v childThread=%#v childThreadErr=%v childEvents=%#v childEventsErr=%v", firstCaseForegroundRecordFieldV1(records, func(record domainjob.Record) string { return record.Status }), firstCaseForegroundRecordFieldV1(records, func(record domainjob.Record) string { return record.FailureCode }), parentCalls, childCalls, stringField(thread, "status"), err, childThread, childThreadErr, childEvents, childEventsErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func firstCaseForegroundRecordFieldV1(records []domainjob.Record, project func(domainjob.Record) string) string {
	if len(records) != 1 {
		return ""
	}
	return project(records[0])
}

type casePromptFrozenStartStoreV1 struct{ record domainjob.Record }

func (store casePromptFrozenStartStoreV1) LoadChildRun(id string) (domainjob.Record, error) {
	if id != store.record.ID {
		return domainjob.Record{}, errors.New("synthetic child identity mismatch")
	}
	return store.record, nil
}

func (store casePromptFrozenStartStoreV1) ValidateChildRunStart(expected domainjob.Record) (domainjob.Record, error) {
	if !reflect.DeepEqual(expected, store.record) {
		return domainjob.Record{}, errors.New("synthetic child start claim mismatch")
	}
	return store.record, nil
}

func TestRuntimeHostChildCasePromptBypassRequiresExactFrozenAuthority(t *testing.T) {
	const workspace = "/workspace/case-prompt-bypass"
	parent := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "parent-thread", TurnID: "parent-turn", WorkspaceRealPath: workspace,
		CaseID: "case-a", CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-a-binding")),
		DatasetSnapshotID:  "dsv2_" + domainsecurity.SHA256Hex([]byte("snapshot-a")),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-a")), ContextEpoch: 7,
	})
	callID := serverTestHostToolCallID("case-prompt-bypass")
	issuedAt := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	var binding *domainjob.SecurityBinding
	var delegation *domainjob.CaseDelegationContextV1
	for nonce := 0; nonce < 200_000; nonce++ {
		grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
			Context: parent, Provider: "provider", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: callID,
			ArgsHash: domainsecurity.SHA256Hex([]byte("args-" + strconv.Itoa(nonce))), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
			ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: issuedAt,
		})
		candidateBinding, err := domainjob.NewSecurityBinding(parent, grant, callID)
		if err != nil {
			t.Fatal(err)
		}
		candidateDelegation, err := domainjob.NewCaseDelegationContextV1(candidateBinding, domainjob.CaseDelegationSemanticContextV1{
			TaskKind: domainjob.CaseDelegationTaskKindV1, Currentness: domaincaseentity.SnapshotCurrentV1,
			Entities: []domainjob.CaseDelegatedEntitySemanticV1{{
				Alias: "acct:1", EntityType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
				FinancialAccountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
				ResolutionDigest:     domainsecurity.SHA256Hex([]byte("private-resolution")),
			}},
			Claims: []domainjob.CaseDelegatedClaimReferenceV1{}, Evidence: []domainjob.CaseDelegatedEvidenceReferenceV1{},
			Continuations: []domainjob.CaseDelegatedContinuationReferenceV1{},
			AnswerSlots: []domainjob.CaseDelegatedAnswerSlotCommitmentV1{{
				Digest: strings.Repeat("a1", 32), ClaimCount: 3, EvidenceCount: 1,
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		projectedRawDigest := privacyprojectionapp.ProjectOrdinaryText(candidateDelegation.DelegationDigest)
		if strings.Contains(projectedRawDigest, "[ACCOUNT]") {
			binding = candidateBinding
			delegation = candidateDelegation
			break
		}
	}
	if binding == nil || delegation == nil {
		t.Fatal("could not construct a valid delegation digest that deterministically exercises the account sanitizer")
	}
	prompt, err := domainjob.CaseDelegationProviderPromptV1(delegation, binding)
	if err != nil {
		t.Fatal(err)
	}
	record := domainjob.Record{
		ID: "child-run", ChildThreadID: "child-thread", ChildTurnID: "child-turn", Kind: "subagent",
		ParentThreadID: binding.ParentThreadID, ParentTurnID: binding.ParentTurnID, ParentToolCallID: binding.ParentToolCallID,
		Name: domainjob.CaseDelegationNameV1, Label: domainjob.CaseDelegationLabelV1, Prompt: prompt,
		SecurityBinding: binding, CaseDelegation: delegation, Status: string(domainjob.StatusRunning),
		LeaseOwner: "worker:case-prompt", LastHeartbeatAt: issuedAt.Format(time.RFC3339Nano),
		LeaseExpiresAt: issuedAt.Add(time.Minute).Format(time.RFC3339Nano), UpdatedAt: issuedAt.Format(time.RFC3339Nano),
	}
	authority := &turnstartapp.ChildTransitionAuthority{
		ChildRunID: record.ID, ChildThreadID: record.ChildThreadID,
		Binding: domainjob.CloneSecurityBinding(binding), ExpectedRecord: record,
	}
	newFrozen := func(threadID, caseID, caseBinding, snapshot string, epoch uint64) domainsecurity.TurnSecurityContext {
		t.Helper()
		return newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
			ThreadID: threadID, TurnID: "child-turn", WorkspaceRealPath: workspace,
			TenantID: binding.TenantID, UserID: binding.UserID, CaseID: caseID, CaseBindingHash: caseBinding,
			DatasetSnapshotID: snapshot, SourceManifestHash: binding.ParentSourceManifest, ContextEpoch: epoch,
		})
	}
	frozen := newFrozen(record.ChildThreadID, binding.ParentCaseID, binding.ParentCaseBindingHash, binding.ParentDatasetSnapshot, binding.ParentContextEpoch)
	validatedPrompt, validateErr := turnstartapp.ValidateHostChildCasePromptFrozenV1(authority, frozen, frozen.TurnID, prompt)
	if turnstartapp.HostChildCasePromptCandidateV1(authority, prompt) != prompt ||
		validateErr != nil || validatedPrompt != prompt {
		t.Fatal("exact current child authority did not admit the canonical host prompt")
	}
	providerToken := domainjob.CaseDelegationProviderCommitmentTokenV1(delegation.DelegationDigest)
	sanitizedRawDigest := privacyprojectionapp.ProjectOrdinaryText(delegation.DelegationDigest)
	sanitized, _, _, _ := privacyprojectionapp.ProjectTurnContent(prompt, "", nil, nil)
	if !strings.Contains(sanitizedRawDigest, "[ACCOUNT]") ||
		domainjob.ValidateCaseDelegationProviderCommitmentTokenV1(providerToken) != nil ||
		!strings.Contains(prompt, providerToken) || strings.Contains(prompt, delegation.DelegationDigest) || sanitized != prompt {
		t.Fatalf("provider-safe commitment did not isolate the account-shaped raw digest: raw=%q token=%q prompt=%q", sanitizedRawDigest, providerToken, sanitized)
	}
	if turnstartapp.HostChildCasePromptCandidateV1(authority, prompt+" tampered") != "" {
		t.Fatal("tampered prompt acquired the host prompt bypass")
	}
	tamperedDelegation := domainjob.CloneCaseDelegationContextV1(delegation)
	tamperedDelegation.Semantic.AnswerSlots[0].Digest = strings.Repeat("2", 64)
	tamperedAuthority := *authority
	tamperedRecord := record
	tamperedRecord.CaseDelegation = tamperedDelegation
	tamperedAuthority.ExpectedRecord = tamperedRecord
	if turnstartapp.HostChildCasePromptCandidateV1(&tamperedAuthority, prompt) != "" {
		t.Fatal("tampered commitment acquired the host prompt bypass")
	}
	for name, hostile := range map[string]domainsecurity.TurnSecurityContext{
		"child":    newFrozen("cross-child", binding.ParentCaseID, binding.ParentCaseBindingHash, binding.ParentDatasetSnapshot, binding.ParentContextEpoch),
		"case":     newFrozen(record.ChildThreadID, "case-b", domainsecurity.SHA256Hex([]byte("case-b-binding")), binding.ParentDatasetSnapshot, binding.ParentContextEpoch),
		"snapshot": newFrozen(record.ChildThreadID, binding.ParentCaseID, binding.ParentCaseBindingHash, "dsv2_"+domainsecurity.SHA256Hex([]byte("snapshot-b")), binding.ParentContextEpoch),
		"epoch":    newFrozen(record.ChildThreadID, binding.ParentCaseID, binding.ParentCaseBindingHash, binding.ParentDatasetSnapshot, binding.ParentContextEpoch+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := turnstartapp.ValidateHostChildCasePromptFrozenV1(authority, hostile, hostile.TurnID, prompt); err == nil {
				t.Fatal("cross-scope child context acquired the host prompt bypass")
			}
		})
	}
	wrongBinding := domainjob.CloneSecurityBinding(binding)
	wrongBinding.ParentToolCallID = serverTestHostToolCallID("wrong-call")
	wrongAuthority := *authority
	wrongAuthority.Binding = wrongBinding
	if _, err := turnstartapp.ValidateHostChildCasePromptFrozenV1(&wrongAuthority, frozen, frozen.TurnID, prompt); err == nil {
		t.Fatal("wrong grant/call binding acquired the host prompt bypass")
	}
	newWitness := func(t *testing.T) *turnstartapp.HostChildCasePromptFrozenWitnessV1 {
		t.Helper()
		store := casePromptFrozenStartStoreV1{record: record}
		noBlocker := func(domainjob.Record) string { return "" }
		currentAuthority, err := turnstartapp.ResolveChildTransitionAuthority(turnstartapp.ChildTransitionResolveInput{ChildRunID: record.ID, ChildThreadID: record.ChildThreadID, ChildDepth: 1, Store: store, Blocker: noBlocker})
		if err != nil {
			t.Fatal(err)
		}
		if err := turnstartapp.ValidateChildTransitionFrozen(context.Background(), currentAuthority, store, noBlocker, frozen); err != nil {
			t.Fatal(err)
		}
		validated, witness, witnessErr := turnstartapp.NewHostChildCasePromptFrozenWitnessV1(currentAuthority, frozen, frozen.TurnID, prompt)
		if witnessErr != nil || validated != prompt || witness == nil {
			t.Fatalf("current-attempt witness was not issued: validated=%q err=%v", validated, witnessErr)
		}
		return witness
	}
	currentRecord := record
	currentRecord.ChildTurnID = frozen.TurnID
	currentRecord.LastHeartbeatAt = issuedAt.Add(time.Second).Format(time.RFC3339Nano)
	currentRecord.UpdatedAt = currentRecord.LastHeartbeatAt
	currentRecord.LeaseExpiresAt = issuedAt.Add(time.Minute).Format(time.RFC3339Nano)
	const userItemID = "item-child-user"
	projectedPrompt := privacyprojectionapp.ProjectOrdinaryText(prompt)
	newThread := func() map[string]any {
		return map[string]any{"id": record.ChildThreadID, "turns": []any{map[string]any{
			"id": frozen.TurnID, "threadId": record.ChildThreadID, "prompt": projectedPrompt,
			"items": []any{map[string]any{
				"id": userItemID, "threadId": record.ChildThreadID, "turnId": frozen.TurnID,
				"kind": "user_message", "role": "user", "status": "completed", "text": projectedPrompt,
			}},
		}}}
	}
	durableThread := newThread()
	durableTurn := durableThread["turns"].([]any)[0].(map[string]any)
	durableItem := durableTurn["items"].([]any)[0].(map[string]any)
	durableThreadBytes, _ := json.Marshal(durableThread)
	if durableTurn["prompt"] != projectedPrompt || durableItem["text"] != projectedPrompt ||
		durableTurn["prompt"] != prompt || durableItem["text"] != prompt ||
		bytes.Contains(durableThreadBytes, []byte(delegation.DelegationDigest)) {
		t.Fatalf("durable current user item did not retain only provider-safe commitments: %s", durableThreadBytes)
	}
	newInput := func(witness *turnstartapp.HostChildCasePromptFrozenWitnessV1) (privacyprojectionapp.BindHostChildCasePromptCurrentAttemptInputV1, *[]domainmodel.Message) {
		messages := []domainmodel.Message{{Role: "user", Content: projectedPrompt}}
		return privacyprojectionapp.BindHostChildCasePromptCurrentAttemptInputV1{
			Witness: witness, SecurityContext: frozen, CurrentRecord: currentRecord, Thread: newThread(),
			ThreadID: record.ChildThreadID, TurnID: frozen.TurnID, UserItemID: userItemID,
			Aliases: domainjob.CaseDelegationAliasesV1(delegation), Messages: &messages,
		}, &messages
	}
	positive, positiveMessages := newInput(newWitness(t))
	if err := privacyprojectionapp.BindHostChildCasePromptCurrentAttemptV1(positive); err != nil {
		t.Fatalf("exact current user item did not bind the private provider attempt: %v", err)
	}
	providerRequest, err := privacyprojectionapp.ProjectProviderRequestForEffect(
		frozen, domainmodel.Request{Messages: *positiveMessages}, false,
	)
	if err != nil || len(providerRequest.Messages) != 1 || providerRequest.Messages[0].Content != prompt ||
		providerRequest.Messages[0].PrivateProviderSemanticBinding != nil {
		t.Fatalf("private current-attempt prompt was not projected exactly once: request=%#v err=%v", providerRequest, err)
	}
	if replay, err := privacyprojectionapp.ProjectProviderRequestForEffect(
		frozen, domainmodel.Request{Messages: *positiveMessages}, false,
	); !errors.Is(err, privacyprojectionapp.ErrProviderPrivacyAuthorityUnavailable) || !reflect.DeepEqual(replay, domainmodel.Request{}) {
		t.Fatalf("private child prompt was projected into a second provider attempt: request=%#v err=%v", replay, err)
	}
	if err := privacyprojectionapp.BindHostChildCasePromptCurrentAttemptV1(positive); err == nil {
		t.Fatal("current-attempt witness was consumed twice")
	}
	safePositive, safeMessages := newInput(newWitness(t))
	if err := privacyprojectionapp.BindHostChildCasePromptCurrentAttemptV1(safePositive); err != nil {
		t.Fatalf("safe-history current user item did not bind: %v", err)
	}
	history, err := privacyprojectionapp.PrivateProtocolSafeHistoryV1(frozen, *safeMessages)
	if err != nil || len(history) != 1 || history[0].Role != "user" || history[0].Content != prompt {
		t.Fatalf("private-protocol history lost the current host child prompt: history=%#v err=%v", history, err)
	}
	recompiledHistory, err := privacyprojectionapp.PrivateProtocolSafeHistoryV1(frozen, history)
	if err != nil || len(recompiledHistory) != 1 || recompiledHistory[0].Content != prompt {
		t.Fatalf("private-protocol recompilation lost the current host child prompt: history=%#v err=%v", recompiledHistory, err)
	}
	privateProtocolRequest, err := privacyprojectionapp.ProjectProviderRequestForEffect(
		frozen, domainmodel.Request{Messages: recompiledHistory}, false,
	)
	if err != nil || len(privateProtocolRequest.Messages) != 1 || privateProtocolRequest.Messages[0].Content != prompt ||
		privateProtocolRequest.Messages[0].PrivateProviderSemanticBinding != nil {
		t.Fatalf("private-protocol child prompt was not projected exactly once: request=%#v err=%v", privateProtocolRequest, err)
	}
	if replay, err := privacyprojectionapp.ProjectProviderRequestForEffect(
		frozen, domainmodel.Request{Messages: history}, false,
	); !errors.Is(err, privacyprojectionapp.ErrProviderPrivacyAuthorityUnavailable) || !reflect.DeepEqual(replay, domainmodel.Request{}) {
		t.Fatalf("private-protocol child prompt was projected twice: request=%#v err=%v", replay, err)
	}
	serializedHistory, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	var restartedHistory []domainmodel.Message
	if err := json.Unmarshal(serializedHistory, &restartedHistory); err != nil {
		t.Fatal(err)
	}
	restartedProviderRequest, err := privacyprojectionapp.ProjectProviderRequestForEffect(
		frozen, domainmodel.Request{Messages: restartedHistory}, false,
	)
	if err != nil || len(restartedProviderRequest.Messages) != 1 || restartedProviderRequest.Messages[0].Content != prompt ||
		restartedProviderRequest.Messages[0].PrivateProviderSemanticBinding != nil {
		t.Fatalf("restart did not retain only the non-authoritative provider-safe commitment metadata: request=%#v err=%v", restartedProviderRequest, err)
	}
	restartAuthority := *authority
	restartAuthority.ExpectedRecord = currentRecord
	if _, witness, err := turnstartapp.NewHostChildCasePromptFrozenWitnessV1(&restartAuthority, frozen, frozen.TurnID, prompt); err == nil || witness != nil {
		t.Fatal("restart/replay reconstructed a consumed current-attempt witness")
	}
	for name, mutate := range map[string]func(*privacyprojectionapp.BindHostChildCasePromptCurrentAttemptInputV1){
		"missing witness": func(input *privacyprojectionapp.BindHostChildCasePromptCurrentAttemptInputV1) { input.Witness = nil },
		"wrong item id": func(input *privacyprojectionapp.BindHostChildCasePromptCurrentAttemptInputV1) {
			input.UserItemID = "item-other"
		},
		"cross turn": func(input *privacyprojectionapp.BindHostChildCasePromptCurrentAttemptInputV1) {
			input.TurnID = "turn-other"
		},
		"fork thread": func(input *privacyprojectionapp.BindHostChildCasePromptCurrentAttemptInputV1) {
			input.ThreadID = "thread-fork"
		},
		"wrong durable role": func(input *privacyprojectionapp.BindHostChildCasePromptCurrentAttemptInputV1) {
			turn := input.Thread["turns"].([]any)[0].(map[string]any)
			turn["items"].([]any)[0].(map[string]any)["role"] = "assistant"
		},
		"wrong provider role": func(input *privacyprojectionapp.BindHostChildCasePromptCurrentAttemptInputV1) {
			(*input.Messages)[0].Role = "assistant"
		},
		"multiple current user items": func(input *privacyprojectionapp.BindHostChildCasePromptCurrentAttemptInputV1) {
			turn := input.Thread["turns"].([]any)[0].(map[string]any)
			turn["items"] = append(turn["items"].([]any), map[string]any{
				"id": "item-second", "threadId": record.ChildThreadID, "turnId": frozen.TurnID,
				"kind": "user_message", "role": "user", "status": "completed", "text": projectedPrompt,
			})
		},
		"tampered durable prompt": func(input *privacyprojectionapp.BindHostChildCasePromptCurrentAttemptInputV1) {
			turn := input.Thread["turns"].([]any)[0].(map[string]any)
			turn["prompt"] = projectedPrompt + " tampered"
		},
	} {
		t.Run("current attempt "+name, func(t *testing.T) {
			input, _ := newInput(newWitness(t))
			mutate(&input)
			if err := privacyprojectionapp.BindHostChildCasePromptCurrentAttemptV1(input); err == nil {
				t.Fatal("hostile current-attempt message acquired the private prompt authority")
			}
		})
	}
}

func TestCaseBoundProviderChildDelegationUsesCurrentAliasResolutionAndPersistsNoRawBody(t *testing.T) {
	runCaseBoundProviderChildDelegationV1(t, "")
}

func TestCaseBoundDeepSeekProviderChildDelegationPreservesPrivateTypedHandoff(t *testing.T) {
	runCaseBoundProviderChildDelegationV1(t, "deepseek-chat-completions")
}

func runCaseBoundProviderChildDelegationV1(t *testing.T, reasoningProtocol string) {
	t.Helper()
	workspace := writeThreadMutationCaseBinding(t)
	durableRoot := t.TempDir()
	dataRoot := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: dataRoot,
		ProviderID: "case-delegation-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "case-delegation-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	t.Cleanup(func() { _ = handler.Shutdown(context.Background()) })
	dataset := newServerSignedCurrentDatasetV1(t, workspace)
	handler.turnSecurity = turnsecurityapp.WorkspaceSecurityAuthority{
		Identity: testIdentityAuthority(), Observer: dataset, RiskAuthority: newServerTestRiskAuthority(), SnapshotAuthorityV2: dataset,
	}
	handler.threads = nil
	caseSource := newCaseForegroundPublicSeamMCPV1(t, dataset)
	handler.mcp = caseSource
	newRestartedForegroundAuthority := configureCaseForegroundPublicSeamAuthoritiesV1(t, handler, durableRoot, dataset, caseSource)
	configureCaseIngressEntityServiceV1(t, handler, dataset)
	configureCaseIngressNativeAuthorityV1(t, handler)
	recordingProvider := &providerStepRecordingProvider{}
	handler.provider = recordingProvider
	server := httptest.NewServer(handler)
	defer server.Close()
	thread := requestThreadSummaryJSON(t, server.URL, http.MethodPost, "/v1/threads", bytes.NewReader(caseIngressJSONV1(t, map[string]any{
		"title": "case delegation parent", "workspace": workspace,
		"providerId": "case-delegation-provider", "model": "case-delegation-model",
	})), http.StatusCreated)
	threadID := stringField(thread, "id")
	const rawAccount = "6222021234567890123"
	requestThreadSummaryJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", bytes.NewReader(caseIngressJSONV1(t, map[string]any{
		"prompt": "请核查银行账号 " + rawAccount + " 的当前案件数据。", "riskIntent": "case",
	})), http.StatusAccepted)
	currentThread := waitForCaseForegroundPublicSeamTurnsV1(t, handler, threadID, 1)
	securityContext, err := domainsecurity.ParseTurnSecurityContext(currentThread["securityState"])
	if err != nil {
		t.Fatal(err)
	}
	longReference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	const pii = "13800138000"
	const rawPath = "/Users/provider/private-case.txt"
	rawPrompt := "Analyze acct:1 only; raw=" + rawAccount + " phone=" + pii + " path=" + rawPath +
		" reverseMap={selected=" + rawAccount + "} authority=" + string(longReference)
	arguments, _ := json.Marshal(map[string]any{"prompt": rawPrompt, "name": rawPrompt, "label": rawPrompt})
	callID := serverTestHostToolCallID("case-delegation-public-seam")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "case-delegation-provider", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: callID,
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("case-delegation-schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("case-delegation-scope")), ReadOnly: true, ApprovalState: "not_required",
		IssuedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	binding, err := domainjob.NewSecurityBinding(securityContext, grant, callID)
	if err != nil {
		t.Fatal(err)
	}
	for _, unprovenPrompt := range []string{"Analyze acct:999 only.", "Analyze card:1 only.", "Analyze the current case without an alias."} {
		unprovenRequest, requestErr := subagentapp.TaskRequestFromArgs("task", map[string]any{"prompt": unprovenPrompt})
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		if _, bindErr := subagentapp.BindCaseDelegationV1(context.Background(), subagentapp.BindCaseDelegationInputV1{
			SecurityContext: securityContext, SecurityBinding: binding, CaseEntities: handler.caseEntities, Request: unprovenRequest,
		}); bindErr == nil {
			t.Fatalf("prompt-shaped but unproven delegated alias was accepted: %q", unprovenPrompt)
		}
	}
	request, err := subagentapp.TaskRequestFromArgs("task", map[string]any{"prompt": rawPrompt, "name": rawPrompt, "label": rawPrompt})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := subagentapp.BindCaseDelegationV1(context.Background(), subagentapp.BindCaseDelegationInputV1{
		SecurityContext: securityContext, SecurityBinding: binding, CaseEntities: handler.caseEntities, Request: request,
	})
	if err != nil {
		t.Fatal(err)
	}
	if bound.Request.Name != domainjob.CaseDelegationNameV1 || bound.Request.Label != domainjob.CaseDelegationLabelV1 ||
		!bound.Request.NameExplicit || !bound.Request.LabelExplicit ||
		bound.Context == nil || len(bound.Context.Semantic.Entities) != 1 || bound.Context.Semantic.Entities[0].Alias != "acct:1" ||
		!domainsecurity.IsSHA256Hex(bound.Context.Semantic.Entities[0].ResolutionDigest) {
		t.Fatalf("host-private alias resolution did not produce typed delegation: %#v", bound)
	}
	variant, err := subagentapp.BindCaseDelegationV1(context.Background(), subagentapp.BindCaseDelegationInputV1{
		SecurityContext: securityContext, SecurityBinding: binding, CaseEntities: handler.caseEntities,
		Request: subagentapp.TaskRequest{Prompt: "Different raw provider prose for acct:1 with phone=13900139000 and reverseMap=forbidden"},
	})
	if err != nil || !domainjob.CaseDelegationContextsEqualV1(bound.Context, variant.Context) || bound.Request.Prompt != variant.Request.Prompt {
		t.Fatalf("raw provider prose changed the host-authoritative delegated identity: err=%v", err)
	}
	var hostSelection apploop.HostCaseEntitySelectionV1
	if err := bound.UseHostSelectionV1(securityContext, func(
		record caseentityapp.PrivateRecordReferenceV1,
		references []domaincaseentity.ReferenceV1,
		aliases []domaincaseentity.ModelEntityAliasV1,
	) error {
		if len(references) != 1 {
			return errors.New("case foreground public seam expected one private subject reference")
		}
		caseSource.mu.Lock()
		caseSource.subjectRef = string(references[0])
		caseSource.mu.Unlock()
		var selectionErr error
		hostSelection, selectionErr = apploop.NewHostCaseEntitySelectionFromPersistedIngressAliasesV1(securityContext, record, references, aliases)
		return selectionErr
	}); err != nil || !hostSelection.AllowsModelAliasV1(securityContext, "acct:1") {
		t.Fatalf("host-private typed selection was not usable: selection=%#v err=%v", hostSelection, err)
	}
	modelExecution := map[string]any{
		"providerId": "case-delegation-provider", "modelId": "case-delegation-model", "source": "runtime-default",
		"resolvedAt": time.Now().UTC().Format(time.RFC3339Nano), "endpointFormat": "chat_completions",
		"baseUrlFingerprint": domainsecurity.SHA256Hex([]byte("base")), "capabilityFingerprint": domainsecurity.SHA256Hex([]byte("caps")),
	}
	schemaHash := domainsecurity.SHA256Hex([]byte("case-delegation-tool-schema"))
	manifest, err := domainjob.NewDelegatedToolManifestV1(nil, schemaHash, domainsecurity.SHA256Hex([]byte("case-delegation-mcp")))
	if err != nil {
		t.Fatal(err)
	}
	managerRoot := t.TempDir()
	manager, err := jobs.NewManager(managerRoot)
	if err != nil {
		t.Fatal(err)
	}
	expectedBackgroundPrompt, err := domainjob.CaseDelegationProviderPromptV1(bound.Context, binding)
	if err != nil || bound.Request.Prompt != expectedBackgroundPrompt {
		t.Fatalf("background delegation prompt changed before persistence: err=%v got=%q want=%q", err, bound.Request.Prompt, expectedBackgroundPrompt)
	}
	if err := domainjob.ValidateExecutableCaseDelegationV1(
		"subagent", bound.Request.Name, bound.Request.Label, bound.Request.Prompt, binding, bound.Context,
	); err != nil {
		t.Fatalf("background delegation was not executable before persistence: %v", err)
	}
	record, err := manager.StartChildRun(domainjob.StartRequest{
		ParentGoalID: "goal-case-delegation", ParentThreadID: securityContext.ThreadID, ParentTurnID: securityContext.TurnID,
		ParentToolCallID: callID, SecurityBinding: binding, CaseDelegation: bound.Context,
		Kind: "subagent", Name: bound.Request.Name, Label: bound.Request.Label, Prompt: bound.Request.Prompt,
		Status: string(domainjob.StatusRunning), Background: true, ModelExecution: modelExecution,
		ToolSchemaHash: schemaHash, DelegatedToolManifest: manifest,
	})
	if err != nil {
		t.Fatal(err)
	}
	durableBytes, err := os.ReadFile(filepath.Join(managerRoot, record.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	publicBytes, _ := json.Marshal(subagentapp.SecurityBoundChildOutputProjection(record))
	providerBytes := []byte(bound.Request.Prompt)
	for channel, body := range map[string][]byte{"provider": providerBytes, "durable": durableBytes, "public": publicBytes} {
		for _, forbidden := range []string{rawPrompt, rawAccount, pii, rawPath, string(longReference), "reverseMap"} {
			if bytes.Contains(body, []byte(forbidden)) {
				t.Fatalf("%s bytes leaked %q: %s", channel, forbidden, body)
			}
		}
	}
	if !bytes.Contains(providerBytes, []byte(`"alias":"acct:1"`)) ||
		!bytes.Contains(durableBytes, []byte(`"caseDelegation"`)) ||
		bytes.Contains(publicBytes, []byte(`"caseDelegation"`)) {
		t.Fatalf("typed/private/public delegation projection mismatch: provider=%s durable=%s public=%s", providerBytes, durableBytes, publicBytes)
	}

	const capturedRawProviderBody = "RAW_PROVIDER_CHILD_BODY_1FD8 analyze only acct:1 in this bounded task"
	if reasoningProtocol != "" {
		modelProviders, err := json.Marshal(map[string]any{
			"defaultProviderId": "case-delegation-provider",
			"providers": []map[string]any{{
				"id": "case-delegation-provider", "apiKey": "test-key", "baseUrl": "https://provider.invalid",
				"endpointFormat": "chat_completions", "models": []string{"case-delegation-model"},
				"modelProfiles": map[string]any{"case-delegation-model": map[string]any{
					"reasoning": map[string]any{
						"supportedEfforts": []string{"off", "high"}, "defaultEffort": "high", "requestProtocol": reasoningProtocol,
					},
				}},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		handler.providerConfig = runtimeprovider.NewRuntimeProviderConfigSet(domainmodel.RuntimeProviderConfigInput{
			ModelProvidersJSON: string(modelProviders),
		})
		if err := handler.providerConfig.ConfigurationError(); err != nil {
			t.Fatalf("private-protocol provider configuration failed: %v", err)
		}
	}
	capture := &caseDelegationChildCaptureProviderV1{rawTaskPrompt: capturedRawProviderBody}
	handler.provider = capture
	requestThreadSummaryJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", bytes.NewReader(caseIngressJSONV1(t, map[string]any{
		"prompt": "请委托子代理分析当前案件账户别名 acct:1 的资金流入、流出和净额。", "riskIntent": "case",
		"approvalPolicy": "auto", "sandboxMode": "workspace-write",
	})), http.StatusAccepted)
	waitForCaseForegroundPublicSeamCompletionV1(t, handler, capture, threadID)
	if reasoningProtocol != "" {
		accountFlowSeen := false
		childPromptSafeSeen := false
		parentTypedSeen := false
		for _, captured := range capture.requests {
			if captured.ReasoningProtocol != reasoningProtocol {
				t.Fatalf("physical private-protocol request used protocol %q want %q", captured.ReasoningProtocol, reasoningProtocol)
			}
			capturedBody, _ := json.Marshal(captured.Messages)
			if bytes.Contains(capturedBody, []byte(caseDelegationDeepSeekReasoningCanaryV1)) {
				t.Fatalf("private response reasoning re-entered a provider request: %s", capturedBody)
			}
			child := captured.PrivateProviderTelemetry != nil &&
				strings.TrimSpace(captured.PrivateProviderTelemetry.ChildRunID) != ""
			hasToolWire := false
			for _, message := range captured.Messages {
				if message.Role == "tool" || len(message.ToolCalls) != 0 {
					hasToolWire = true
				}
				if !child && message.Role == "tool" && message.Name == providerStepFundsTool &&
					strings.Contains(message.Content, caseSource.semantic.QueryHash) {
					accountFlowSeen = true
				}
				if message.Role != "user" || message.Name != "" || message.ToolCallID != "" {
					continue
				}
				if child && strings.Contains(message.Content, "<analytix_host_case_delegation_v1>") {
					childPromptSafeSeen = true
				}
				if content, opened := appmodel.CompletedPrivateProtocolSafeHistoryContentV1(
					message.Content, providerStepFundsTool,
				); !child && opened && strings.Contains(content, caseSource.semantic.QueryHash) {
					accountFlowSeen = true
				}
				if content, opened := appmodel.CompletedPrivateProtocolSafeHistoryContentV1(
					message.Content, "task",
				); !child && opened && strings.Contains(content, `"caseResult"`) {
					parentTypedSeen = true
				}
			}
			if !child && providerRequestContainsPrivateCaseResultV1(captured) {
				parentTypedSeen = true
			}
			if hasToolWire && len(captured.DeepSeekReasoningReplays) == 0 {
				t.Fatalf("private provider request retained tool wire without an exact process-local replay: %#v", captured.Messages)
			}
		}
		if !accountFlowSeen || !childPromptSafeSeen || !parentTypedSeen {
			t.Fatalf("DeepSeek typed flow seams accountFlow=%t childPrompt=%t parentTyped=%t", accountFlowSeen, childPromptSafeSeen, parentTypedSeen)
		}
	}
	childProviderCalls := 0
	for _, captured := range capture.requests {
		if captured.PrivateProviderTelemetry == nil || strings.TrimSpace(captured.PrivateProviderTelemetry.ChildRunID) == "" {
			continue
		}
		childProviderCalls++
		capturedBytes, _ := json.Marshal(struct {
			SystemPrompt string                   `json:"systemPrompt"`
			Messages     []domainmodel.Message    `json:"messages"`
			Tools        []domainmodel.ToolSchema `json:"tools"`
		}{SystemPrompt: captured.SystemPrompt, Messages: captured.Messages, Tools: captured.Tools})
		for _, forbidden := range []string{
			capturedRawProviderBody, rawPrompt, rawAccount, pii, rawPath, string(longReference), "reverseMap",
			`"claims"`, `"evidence"`, `"gaps"`, `"inflowMinor"`, `"outflowMinor"`, `"netMinor"`,
			caseSource.semantic.QueryHash, caseSource.semantic.ResultHash, caseSource.semantic.Transactions[0].EvidenceRef,
			caseDelegationDeepSeekReasoningCanaryV1,
		} {
			if bytes.Contains(capturedBytes, []byte(forbidden)) {
				t.Fatalf("captured child provider request leaked %q: %s", forbidden, capturedBytes)
			}
		}
		if !bytes.Contains(capturedBytes, []byte("acct:1")) {
			t.Fatalf("captured child provider request omitted delegated alias: %s", capturedBytes)
		}
	}
	if childProviderCalls != 1 || capture.parentCalls != 3 {
		capturedMessages := make([][]domainmodel.Message, len(capture.requests))
		for index := range capture.requests {
			capturedMessages[index] = capture.requests[index].Messages
		}
		diagnosticMessages, _ := json.Marshal(capturedMessages)
		t.Fatalf("physical provider calls child=%d parent=%d total=%d records=%d toolCalls=%d messages=%s", childProviderCalls, capture.parentCalls, len(capture.requests), len(handler.jobs.AllRecords()), caseSource.toolCalls, diagnosticMessages)
	}
	actualRecords := handler.jobs.AllRecords()
	if len(actualRecords) != 1 {
		t.Fatalf("provider child durable records=%d want=1", len(actualRecords))
	}
	actualRecord := actualRecords[0]
	if actualRecord.CaseDelegation == nil || actualRecord.Prompt == capturedRawProviderBody ||
		actualRecord.Name != domainjob.CaseDelegationNameV1 || actualRecord.Label != domainjob.CaseDelegationLabelV1 ||
		strings.TrimSpace(actualRecord.ChildThreadID) == "" || strings.TrimSpace(actualRecord.ChildTurnID) == "" ||
		len(actualRecord.CaseDelegation.Semantic.Claims) != 0 || len(actualRecord.CaseDelegation.Semantic.Evidence) != 0 ||
		len(actualRecord.CaseDelegation.Semantic.Continuations) != 0 || len(actualRecord.CaseDelegation.Semantic.AnswerSlots) != 1 ||
		actualRecord.Output != "" || actualRecord.Usage != nil {
		t.Fatalf("provider child did not retain only typed durable delegation: %#v", actualRecord)
	}
	actualDurableBytes, _ := json.Marshal(actualRecord)
	actualPublicBytes, _ := json.Marshal(subagentapp.SecurityBoundChildOutputProjection(actualRecord))
	for channel, body := range map[string][]byte{"actual-durable": actualDurableBytes, "actual-public": actualPublicBytes} {
		for _, forbidden := range []string{
			capturedRawProviderBody, rawPrompt, rawAccount, pii, rawPath, string(longReference), "reverseMap",
			`"caseResult"`, `"inflowMinor"`, `"outflowMinor"`, `"netMinor"`,
			caseSource.semantic.QueryHash, caseSource.semantic.ResultHash, caseSource.semantic.Transactions[0].EvidenceRef,
			caseDelegationDeepSeekReasoningCanaryV1,
		} {
			if bytes.Contains(body, []byte(forbidden)) {
				t.Fatalf("%s bytes leaked %q: %s", channel, forbidden, body)
			}
		}
	}
	completionVerifier, ok := handler.childCompletions.(jobs.ChildCompletionReceiptVerifier)
	if !ok {
		t.Fatal("case foreground child completion authority cannot verify durable restart")
	}
	restartedJobs, err := jobs.NewManagerWithChildCompletionVerifier(filepath.Join(handler.dataDir, "child-runs"), completionVerifier)
	if err != nil {
		t.Fatal(err)
	}
	restartedRecord, err := restartedJobs.LoadChildRun(actualRecord.ID)
	if err != nil || restartedRecord.Status != string(domainjob.StatusCompleted) || restartedRecord.Output != "" || restartedRecord.Usage != nil ||
		!domainjob.ForegroundChildHandoffReceiptsEqualV1(restartedRecord.ForegroundChildHandoffReceipt, actualRecord.ForegroundChildHandoffReceipt) ||
		restartedRecord.ChildCompletionReceipt == nil || actualRecord.ChildCompletionReceipt == nil ||
		restartedRecord.ChildCompletionReceipt.ReceiptDigest != actualRecord.ChildCompletionReceipt.ReceiptDigest {
		t.Fatalf("durable restart changed or reconstructed the completed child carrier: record=%#v err=%v", restartedRecord, err)
	}
	replayRecord := restartedRecord
	replayRecord.Status = string(domainjob.StatusRunning)
	if prepared, err := newRestartedForegroundAuthority().Prepare(context.Background(), replayRecord, replayRecord.ChildTurnID); err == nil {
		t.Fatalf("restart reconstructed a process-local case handoff: %#v", prepared)
	}
	threadAfterChild, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turns, _ := threadAfterChild["turns"].([]any)
	lastTurn, _ := turns[len(turns)-1].(map[string]any)
	acceptedFinalView, _ := lastTurn["acceptedFinalView"].(map[string]any)
	acceptedFinal, _ := lastTurn["acceptedFinal"].(map[string]any)
	if acceptedFinal["finalGateVersion"] != "analytix.final-evidence-gate/v4" ||
		acceptedFinalView["variant"] != string(domainevidence.NeedsEvidenceAnswer) ||
		acceptedFinalView["blockerCode"] != "publication_snapshot_not_fresh" || acceptedFinalView["claimCount"] != float64(0) {
		t.Fatalf("parent typed handoff escaped or skipped the Final Gate: final=%#v view=%#v", acceptedFinal, acceptedFinalView)
	}
	publicThreadBytes, _ := json.Marshal(threadAfterChild)
	publicHTTPBytes, _ := json.Marshal(requestThreadSummaryJSON(
		t, server.URL, http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK,
	))
	publicSSEBytes := []byte(requestRuntimeSSEText(t, server.URL+"/v1/threads/"+threadID+"/events?since_seq=0"))
	for _, forbidden := range []string{
		capturedRawProviderBody, rawPrompt, rawAccount, pii, rawPath, string(longReference), "reverseMap",
		`"caseResult"`, caseSource.semantic.QueryHash, caseSource.semantic.ResultHash,
		caseSource.semantic.Transactions[0].EvidenceRef, caseDelegationDeepSeekReasoningCanaryV1,
	} {
		if bytes.Contains(publicThreadBytes, []byte(forbidden)) {
			t.Fatalf("public parent thread leaked %q: %s", forbidden, publicThreadBytes)
		}
		if bytes.Contains(publicHTTPBytes, []byte(forbidden)) {
			t.Fatalf("public HTTP thread leaked %q: %s", forbidden, publicHTTPBytes)
		}
		if bytes.Contains(publicSSEBytes, []byte(forbidden)) {
			t.Fatalf("public SSE replay leaked %q: %s", forbidden, publicSSEBytes)
		}
	}
	for _, root := range []string{dataRoot, durableRoot} {
		if err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				return nil
			}
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			for _, forbidden := range []string{
				capturedRawProviderBody, `"caseResult"`, `"inflowMinor"`, `"outflowMinor"`, `"netMinor"`,
				caseSource.semantic.QueryHash, caseSource.semantic.ResultHash, caseSource.semantic.Transactions[0].EvidenceRef,
				caseDelegationDeepSeekReasoningCanaryV1,
			} {
				if bytes.Contains(body, []byte(forbidden)) {
					t.Fatalf("private typed child value %q reached durable file %s", forbidden, path)
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}
