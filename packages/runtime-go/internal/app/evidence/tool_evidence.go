package evidence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	runtimeports "analytix.local/runtime-go/internal/ports"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	grantregistryport "analytix.local/runtime-go/internal/ports/grantregistry"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
)

const (
	fundsCountEvidenceCanonicalTool = "mcp__analytix_funds__count_case_rows"
	fundsAccountFlowCanonicalTool   = "mcp__analytix_funds__analyze_account_flows"
	fundsCountEvidenceServerVersion = "0.16.16"
	fundsCountEvidencePurpose       = "analytix.funds.count-case-rows-evidence-candidate/v1"
	fundsCountEvidenceTable         = "analysis_txn_detail_idx"
	accountFlowReceiptGranularity   = "aggregate"
	accountFlowAggregateFactPrefix  = "fact_aggregate_"
	accountFlowNativeBindStepIDV1   = "bind-account-flow-native-result-v1"
	accountFlowNativeBinderV1       = "analytix-host-account-flow-oracle-binder"
	accountFlowNormalizeStepIDV1    = "normalize-account-flow-v1"
	accountFlowNormalizerV1         = "analytix-host-account-flow-normalizer"
)

type PrepareToolEvidenceInput struct {
	Context       domainsecurity.TurnSecurityContext
	Grant         domainsecurity.ExecutionGrant
	GrantRegistry domainsecurity.ExecutionGrantRegistry
	Call          domainmodel.ToolCall
	Outcome       domainevidence.ToolOutcome
	ResultItemID  string
	Binding       domainsecurity.CaseBindingObservationV1
	RawResult     domainmcp.LosslessToolResult
}

type PreparedToolEvidence struct {
	Authority PreparedEvidenceAuthority
	Output    map[string]any
}

type CommitToolEvidenceInput struct {
	Context domainsecurity.TurnSecurityContext
	Marker  domainevidence.HostEvidenceSettlementMarker
	Thread  map[string]any
}

type ToolEvidenceAuthority interface {
	PrepareCurrentToolEvidence(context.Context, PrepareToolEvidenceInput) (PreparedToolEvidence, bool, error)
	CommitCurrentToolEvidence(context.Context, CommitToolEvidenceInput) (domainevidence.EvidenceReceipt, error)
}

type ToolEvidenceService struct {
	Issuer Issuer
	Reader sourceprobeport.LockedEvidenceReader
	Now    func() time.Time
}

type accountFlowHostSource interface {
	sourceprobeport.LockedCurrentAuthority
	ConsumeHostFundsAccountFlowEvidenceV1(domainmcp.LosslessToolResult, domainnative.AccountFlowHostEvidenceSummaryConsumerV1, domainnative.AccountFlowHostEvidenceRowConsumerV1) error
	HostFundsAccountFlowProviderSemanticV1(domainmcp.LosslessToolResult) (domainnative.AccountFlowProviderSemanticResultV1, bool)
}

type accountFlowHostDisposer interface {
	DiscardHostFundsAccountFlowEvidenceV1(domainmcp.LosslessToolResult)
}

type PreparedToolSettlement struct {
	Output  any
	IsError bool
	Marker  *domainevidence.HostEvidenceSettlementMarker
}

func PrepareToolSettlement(ctx context.Context, authority any, securityAuthority turnsecurityapp.WorkspaceSecurityAuthority, workspaceReader turnsecurityapp.WorkspaceReader, grantStore grantregistryport.Reader, source runtimeports.MCPToolAdvertisementSource, pending appmodel.PendingToolCall, output any, isError bool) (PreparedToolSettlement, error) {
	expectedApprovalState := ""
	if pending.ExecutionGrant.ApprovalState == "pending" && executiongrantapp.PendingBoundaryOutput(output, isError) {
		expectedApprovalState = "pending"
	}
	grantRegistry := domainsecurity.ExecutionGrantRegistry{}
	output, isError = executiongrantapp.SettlementOutput(output, isError, pending.ExecutionGrant, pending.Call.Name, expectedApprovalState, func() error {
		var err error
		grantRegistry, err = executiongrantapp.AuthorizePendingRegistry(ctx, securityAuthority, workspaceReader, grantStore, source, pending, expectedApprovalState, time.Now().UTC())
		return err
	})
	settlement := PreparedToolSettlement{Output: output, IsError: isError}
	toolAuthority, ok := authority.(ToolEvidenceAuthority)
	if !ok || expectedApprovalState != "" || grantRegistry.Version == 0 {
		return settlement, nil
	}
	originalOutcome, eligible, outcomeErr := originalOutcomeForEvidence(output, isError, pending)
	if outcomeErr != nil {
		settlement.Output = map[string]any{"executed": false, "isError": true, "code": "evidence_original_outcome_invalid"}
		settlement.IsError = true
		return settlement, nil
	}
	if !eligible {
		return settlement, nil
	}
	prepareInput := PrepareToolEvidenceInput{
		Context: pending.SecurityContext, Grant: pending.ExecutionGrant, GrantRegistry: grantRegistry, Call: pending.Call,
		Outcome: originalOutcome, ResultItemID: toolcatalogapp.ToolResultItemID(pending.TurnID, pending.Call.ID),
	}
	if pending.Call.Name == fundsAccountFlowCanonicalTool {
		if securityAuthority.Observer == nil {
			settlement.Output = map[string]any{"executed": false, "isError": true, "code": "evidence_preparation_failed"}
			settlement.IsError = true
			return settlement, nil
		}
		binding, bindingErr := securityAuthority.Observer.Observe(pending.SecurityContext.WorkspaceRealPath)
		if bindingErr != nil {
			settlement.Output = map[string]any{"executed": false, "isError": true, "code": "evidence_preparation_failed"}
			settlement.IsError = true
			return settlement, nil
		}
		record, rawOK := output.(map[string]any)
		raw, present := record[domainmcp.HostRawToolResultKey].(domainmcp.LosslessToolResult)
		if !rawOK || !present {
			settlement.Output = map[string]any{"executed": false, "isError": true, "code": "evidence_preparation_failed"}
			settlement.IsError = true
			return settlement, nil
		}
		prepareInput.Binding = binding
		prepareInput.RawResult = raw
	}
	prepared, eligible, err := toolAuthority.PrepareCurrentToolEvidence(ctx, prepareInput)
	if !eligible {
		return settlement, nil
	}
	if err != nil {
		settlement.Output = map[string]any{"executed": false, "isError": true, "code": "evidence_preparation_failed"}
		settlement.IsError = true
		return settlement, nil
	}
	marker, present := domainmcp.ExtractHostEvidenceSettlementCarrier(prepared.Output)
	if !present {
		return PreparedToolSettlement{}, errors.New("prepared tool evidence lost its opaque settlement carrier")
	}
	settlement.Output = prepared.Output
	settlement.IsError = false
	settlement.Marker = &marker
	return settlement, nil
}

func originalOutcomeForEvidence(output any, isError bool, pending appmodel.PendingToolCall) (domainevidence.ToolOutcome, bool, error) {
	if (pending.Call.Name != fundsCountEvidenceCanonicalTool && pending.Call.Name != fundsAccountFlowCanonicalTool) || isError {
		return domainevidence.ToolOutcome{}, false, nil
	}
	record, ok := output.(map[string]any)
	if !ok || record["executed"] != true {
		return domainevidence.ToolOutcome{}, false, errors.New("successful evidence tool output is missing execution authority")
	}
	outcome, err := domainevidence.ParseToolOutcome(record["toolOutcome"])
	if err != nil || outcome.ToolName != pending.Call.Name || outcome.ToolCallID != pending.Call.ID ||
		outcome.ContextDigest != pending.SecurityContext.ContextDigest || outcome.ExecutionGrantID != pending.ExecutionGrant.GrantID ||
		outcome.CaseID != pending.SecurityContext.CaseID || outcome.ContextEpoch != pending.SecurityContext.ContextEpoch ||
		outcome.DatasetSnapshotID != pending.SecurityContext.DatasetSnapshotID || outcome.ServerIdentity != pending.ExecutionGrant.ServerIdentity {
		return domainevidence.ToolOutcome{}, false, errors.New("successful evidence tool output has no matching host outcome")
	}
	if outcome.TransportStatus != domainevidence.TransportSuccess || outcome.IsError || strings.TrimSpace(outcome.Blocker) != "" {
		return domainevidence.ToolOutcome{}, false, errors.New("failed tool outcome cannot prepare evidence")
	}
	if outcome.SemanticStatus != domainevidence.SemanticSuccess &&
		!(pending.Call.Name == fundsAccountFlowCanonicalTool && outcome.SemanticStatus == domainevidence.SemanticPartial) {
		return outcome, false, nil
	}
	if outcome.ReportedSafeToAnswer != nil && !*outcome.ReportedSafeToAnswer {
		return outcome, false, nil
	}
	return outcome, true, nil
}

type toolEvidenceThreadReader interface {
	GetThread(string) (map[string]any, error)
}

func CommitPersistedToolEvidence(ctx context.Context, authority any, reader toolEvidenceThreadReader, securityContext domainsecurity.TurnSecurityContext, settlement PreparedToolSettlement) (domainevidence.EvidenceReceipt, bool, error) {
	if settlement.Marker == nil {
		return domainevidence.EvidenceReceipt{}, false, nil
	}
	toolAuthority, ok := authority.(ToolEvidenceAuthority)
	if !ok || reader == nil {
		return domainevidence.EvidenceReceipt{}, false, errors.New("durable tool evidence commit authority is unavailable")
	}
	thread, err := reader.GetThread(securityContext.ThreadID)
	if err != nil {
		return domainevidence.EvidenceReceipt{}, false, err
	}
	receipt, err := toolAuthority.CommitCurrentToolEvidence(ctx, CommitToolEvidenceInput{Context: securityContext, Marker: *settlement.Marker, Thread: thread})
	if err != nil || domainevidence.ValidateEvidenceReceipt(receipt) != nil {
		if err == nil {
			err = errors.New("durable tool evidence receipt is invalid")
		}
		return domainevidence.EvidenceReceipt{}, false, err
	}
	return receipt, true, nil
}

func (service ToolEvidenceService) PrepareCurrentToolEvidence(ctx context.Context, input PrepareToolEvidenceInput) (PreparedToolEvidence, bool, error) {
	if strings.TrimSpace(input.Call.Name) == fundsAccountFlowCanonicalTool {
		return service.prepareAccountFlowEvidence(ctx, input)
	}
	if strings.TrimSpace(input.Call.Name) != fundsCountEvidenceCanonicalTool {
		return PreparedToolEvidence{}, false, nil
	}
	if blocker := domainsecurity.DatasetSnapshotFactAuthorityBlocker(input.Context.DatasetSnapshotID); blocker != "" {
		return PreparedToolEvidence{}, true, errors.New(blocker)
	}
	if service.Reader == nil || service.Issuer.Registry == nil || service.Issuer.SettlementStore == nil || service.Issuer.Authority == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil ||
		domainsecurity.ValidateExecutionGrantForContext(input.Grant, input.Context) != nil ||
		domainsecurity.ValidateExecutionGrantRegistry(input.GrantRegistry) != nil || domainevidence.ValidateToolOutcome(input.Outcome) != nil || input.Call.ID != input.Grant.ToolCallID ||
		!domainmodel.IsHostToolCallIDV1(input.Call.ID) ||
		input.Call.Name != input.Grant.ToolName || input.Grant.ContextDigest != input.Context.ContextDigest ||
		input.Grant.ArgsHash != domainsecurity.CanonicalJSONHash(input.Call.Arguments) ||
		input.ResultItemID != domaintoolresult.ToolResultItemIDV1(input.Context.TurnID, input.Call.ID) ||
		input.Outcome.ContextDigest != input.Context.ContextDigest || input.Outcome.ExecutionGrantID != input.Grant.GrantID ||
		input.Outcome.ToolName != input.Call.Name || input.Outcome.ToolCallID != input.Call.ID || input.Outcome.CaseID != input.Context.CaseID ||
		input.Outcome.ContextEpoch != input.Context.ContextEpoch || input.Outcome.DatasetSnapshotID != input.Context.DatasetSnapshotID ||
		input.Outcome.ServerIdentity != input.Grant.ServerIdentity || input.Outcome.TransportStatus != domainevidence.TransportSuccess ||
		input.Outcome.SemanticStatus != domainevidence.SemanticSuccess || input.Outcome.IsError || strings.TrimSpace(input.Outcome.Blocker) != "" ||
		(input.Outcome.ReportedSafeToAnswer != nil && !*input.Outcome.ReportedSafeToAnswer) {
		return PreparedToolEvidence{}, true, errors.New("tool evidence preparation authority is invalid")
	}
	var prepared PreparedEvidenceAuthority
	err := service.Reader.WithCurrentEvidenceRead(ctx, sourceprobeport.EvidenceReadInput{
		Context: input.Context, Grant: input.Grant, Arguments: input.Call.Arguments,
	}, func(probe domainsecurity.VerifiedSourceProbe, raw domainmcp.LosslessToolResult) error {
		material, err := normalizeFundsCountEvidenceCandidate(input, probe, raw)
		if err != nil {
			return err
		}
		prepared, err = service.Issuer.Prepare(ctx, IssueEvidenceInput{
			Context: input.Context, Grant: input.Grant, GrantRegistry: input.GrantRegistry, Outcome: input.Outcome,
			SourceProbe: probe, RawResult: raw, Material: material, ResultItemID: input.ResultItemID,
		})
		return err
	})
	if err != nil {
		return PreparedToolEvidence{}, true, err
	}
	if domainevidence.ValidateHostEvidenceSettlementMarker(prepared.Marker) != nil {
		return PreparedToolEvidence{}, true, errors.New("tool evidence preparation returned no settlement marker")
	}
	return PreparedToolEvidence{
		Authority: prepared,
		Output: map[string]any{
			"executed": true, "isError": false, "code": "evidence_captured_pending_final_gate",
			"toolOutcome": domainevidence.ToolOutcomeRecord(input.Outcome),
			domainmcp.HostEvidenceSettlementCarrierKey: domainmcp.HostEvidenceSettlementCarrier{Marker: prepared.Marker},
		},
	}, true, nil
}

type accountFlowCapturedSummary struct {
	subjectRef, snapshotID, contextDigest, bindingHash string
	contextEpoch                                       uint64
	startInclusive, endInclusive, timezone, currency   string
	minorUnitScale                                     uint8
	inflowMinor, outflowMinor, netMinor                string
	transactionCount, evidenceRowCount                 uint64
	aggregateComplete, evidenceRowsComplete            bool
	coverage                                           domainnative.AccountFlowProviderSemanticCoverageV1
	queryHash, resultHash                              string
	set                                                bool
}

type accountFlowCapturedRow struct {
	index                                    int
	subjectRef, sourceRecordID, sourceFileID string
	sourceRowNumber                          uint64
	occurredAt, direction, amountMinor       string
	currency                                 string
	minorUnitScale                           uint8
}

type accountFlowEvidenceCapture struct {
	summary accountFlowCapturedSummary
	rows    []accountFlowCapturedRow
}

func (capture *accountFlowEvidenceCapture) consumeSummary(
	subjectRef string, snapshotID string, contextEpoch uint64, contextDigest string, bindingHash string,
	startInclusive string, endInclusive string, timezone string, currency string, minorUnitScale uint8,
	inflowMinor string, outflowMinor string, netMinor string, transactionCount uint64,
	aggregateComplete bool, evidenceRowsComplete bool, evidenceRowCount uint64,
	coverage domainnative.AccountFlowProviderSemanticCoverageV1, queryHash string, resultHash string,
) error {
	if capture == nil || capture.summary.set {
		return errors.New("account-flow evidence summary was consumed more than once")
	}
	capture.summary = accountFlowCapturedSummary{
		subjectRef: subjectRef, snapshotID: snapshotID, contextDigest: contextDigest, bindingHash: bindingHash,
		contextEpoch: contextEpoch, startInclusive: startInclusive, endInclusive: endInclusive, timezone: timezone,
		currency: currency, minorUnitScale: minorUnitScale, inflowMinor: inflowMinor, outflowMinor: outflowMinor,
		netMinor: netMinor, transactionCount: transactionCount, evidenceRowCount: evidenceRowCount,
		aggregateComplete: aggregateComplete, evidenceRowsComplete: evidenceRowsComplete, coverage: coverage,
		queryHash: queryHash, resultHash: resultHash, set: true,
	}
	return nil
}

func (capture *accountFlowEvidenceCapture) consumeRow(
	index int, subjectRef string, sourceRecordID string, sourceFileID string, sourceRowNumber uint64,
	occurredAt string, direction string, amountMinor string, currency string, minorUnitScale uint8,
) error {
	if capture == nil || !capture.summary.set || index != len(capture.rows) {
		return errors.New("account-flow evidence row order is invalid")
	}
	capture.rows = append(capture.rows, accountFlowCapturedRow{
		index: index, subjectRef: subjectRef, sourceRecordID: sourceRecordID, sourceFileID: sourceFileID,
		sourceRowNumber: sourceRowNumber, occurredAt: occurredAt, direction: direction, amountMinor: amountMinor,
		currency: currency, minorUnitScale: minorUnitScale,
	})
	return nil
}

func (service ToolEvidenceService) prepareAccountFlowEvidence(ctx context.Context, input PrepareToolEvidenceInput) (prepared PreparedToolEvidence, eligible bool, err error) {
	eligible = true
	defer func() {
		if recover() != nil {
			prepared = PreparedToolEvidence{}
			eligible = true
			err = errors.New("account-flow host evidence preparation panicked")
		}
	}()
	if service.Reader == nil || service.Issuer.Registry == nil || service.Issuer.SettlementStore == nil || service.Issuer.Authority == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil ||
		domainsecurity.ValidateExecutionGrantForContext(input.Grant, input.Context) != nil ||
		domainsecurity.ValidateExecutionGrantRegistry(input.GrantRegistry) != nil || domainevidence.ValidateToolOutcome(input.Outcome) != nil ||
		domainsecurity.ValidateCaseBindingObservationV1(input.Binding) != nil || input.Binding.State != domainsecurity.CaseBindingStateValid ||
		input.Binding.WorkspaceRealPath != input.Context.WorkspaceRealPath || input.Binding.CaseID != input.Context.CaseID ||
		input.Binding.CaseBindingHash != input.Context.CaseBindingHash || input.Call.ID != input.Grant.ToolCallID ||
		!domainmodel.IsHostToolCallIDV1(input.Call.ID) || input.Call.Name != input.Grant.ToolName ||
		input.Grant.ContextDigest != input.Context.ContextDigest || input.Grant.ArgsHash != domainsecurity.CanonicalJSONHash(input.Call.Arguments) ||
		input.ResultItemID != domaintoolresult.ToolResultItemIDV1(input.Context.TurnID, input.Call.ID) ||
		input.Outcome.ContextDigest != input.Context.ContextDigest || input.Outcome.ExecutionGrantID != input.Grant.GrantID ||
		input.Outcome.ToolName != input.Call.Name || input.Outcome.ToolCallID != input.Call.ID || input.Outcome.CaseID != input.Context.CaseID ||
		input.Outcome.ContextEpoch != input.Context.ContextEpoch || input.Outcome.DatasetSnapshotID != input.Context.DatasetSnapshotID ||
		input.Outcome.ServerIdentity != input.Grant.ServerIdentity || input.Outcome.TransportStatus != domainevidence.TransportSuccess ||
		(input.Outcome.SemanticStatus != domainevidence.SemanticSuccess && input.Outcome.SemanticStatus != domainevidence.SemanticPartial) ||
		input.Outcome.IsError || strings.TrimSpace(input.Outcome.Blocker) != "" ||
		(input.Outcome.ReportedSafeToAnswer != nil && !*input.Outcome.ReportedSafeToAnswer) ||
		!domainmcp.ValidLosslessToolResult(input.RawResult) || domainsecurity.SHA256Hex(input.RawResult.RawResult) != input.RawResult.RawSHA256 {
		return PreparedToolEvidence{}, true, errors.New("account-flow tool evidence preparation authority is invalid")
	}
	source, sourceOK := service.Reader.(accountFlowHostSource)
	disposer, disposerOK := service.Reader.(accountFlowHostDisposer)
	if !sourceOK || !disposerOK {
		return PreparedToolEvidence{}, true, errors.New("account-flow host evidence capability is unavailable")
	}
	carrierConsumed := false
	defer func() {
		if !carrierConsumed {
			disposer.DiscardHostFundsAccountFlowEvidenceV1(input.RawResult)
		}
	}()
	var preparedAuthority PreparedEvidenceAuthority
	err = source.WithCurrentProbeAuthority(ctx, sourceprobeport.CurrentInput{
		ServerID: mcpServerID(input.Call.Name), Context: input.Context, Binding: input.Binding,
		ConnectionEpoch: input.Grant.ConnectionEpoch,
	}, func(probe domainsecurity.VerifiedSourceProbe, capability sourceprobeport.HostEvidenceCapability) error {
		if !domainsecurity.SourceProbeEligibleForHostAuthorityV2(probe) || capability == nil {
			return errors.New("account-flow current host source authority is unavailable")
		}
		selection, selectionErr := capability.DatasetSelection()
		if selectionErr != nil || datasetsnapshotport.ValidateCurrentSelectionDigestV2(selection) != nil ||
			validateHostAuthoritySelection(input.Context, probe, selection.SelectionDigest, capability) != nil {
			return errors.New("account-flow current dataset selection is unavailable")
		}
		capture := &accountFlowEvidenceCapture{}
		return capability.UseExact(input.Context, probe, selection, func(leaseContext context.Context) error {
			contentDigest, contentErr := datasetsnapshotport.CanonicalCurrentSelectionContentDigestV2(selection)
			if contentErr != nil {
				return contentErr
			}
			semantic, semanticOK := source.HostFundsAccountFlowProviderSemanticV1(input.RawResult)
			consumeErr := source.ConsumeHostFundsAccountFlowEvidenceV1(input.RawResult, capture.consumeSummary, capture.consumeRow)
			carrierConsumed = true
			if consumeErr != nil || !semanticOK {
				return errors.New("account-flow host-private evidence carrier is invalid")
			}
			// Native CSV locators use the private import-file identity. Persist
			// the same case-bound ledger locator that the live source resolver
			// already witnessed, so retained reads address that exact record.
			if selection.Snapshot.Manifest.ProducerPolicyID == domainevidence.FundsCanonicalTransactionSourceRowPolicyIDV1 {
				for index := range capture.rows {
					fileID, err := domainevidence.DeriveFundsCanonicalCSVSourceFileIDV1(input.Context.CaseID, capture.rows[index].sourceFileID)
					if err != nil {
						return errors.New("account-flow accepted slot source file identity is invalid")
					}
					capture.rows[index].sourceFileID = fileID
				}
			}
			material, materialErr := normalizeAccountFlowEvidence(input, probe, input.RawResult, semantic, capture)
			if materialErr != nil {
				return materialErr
			}
			preparedAuthority, materialErr = service.Issuer.Prepare(leaseContext, IssueEvidenceInput{
				Context: input.Context, Grant: input.Grant, GrantRegistry: input.GrantRegistry, Outcome: input.Outcome,
				SourceProbe: probe, RawResult: input.RawResult, Material: material, ResultItemID: input.ResultItemID,
				HostAuthority: &domainevidence.PreparedEvidenceHostAuthorityV1{
					Binding: input.Binding, SelectionDigest: selection.SelectionDigest, SelectionContentDigest: contentDigest,
				},
				HostCapability: capability,
			})
			return materialErr
		})
	})
	if err != nil {
		return PreparedToolEvidence{}, true, err
	}
	return PreparedToolEvidence{Authority: preparedAuthority, Output: map[string]any{
		"executed": true, "isError": false, "code": "evidence_captured_pending_final_gate",
		"toolOutcome": domainevidence.ToolOutcomeRecord(input.Outcome),
		domainmcp.HostEvidenceSettlementCarrierKey: domainmcp.HostEvidenceSettlementCarrier{Marker: preparedAuthority.Marker},
	}}, true, nil
}

func normalizeAccountFlowEvidence(
	input PrepareToolEvidenceInput,
	probe domainsecurity.VerifiedSourceProbe,
	raw domainmcp.LosslessToolResult,
	semantic domainnative.AccountFlowProviderSemanticResultV1,
	capture *accountFlowEvidenceCapture,
) (VerifiedEvidenceMaterial, error) {
	status := input.Outcome.SemanticStatus
	if status != domainevidence.SemanticSuccess && status != domainevidence.SemanticPartial ||
		domainnative.ValidateAccountFlowProviderSemanticResultV1(semantic, semantic.EvidenceRowLimit, status) != nil ||
		capture == nil || !capture.summary.set ||
		capture.summary.snapshotID != input.Context.DatasetSnapshotID || capture.summary.contextEpoch != input.Context.ContextEpoch ||
		capture.summary.contextDigest != input.Context.ContextDigest || capture.summary.bindingHash != input.Context.CaseBindingHash ||
		capture.summary.startInclusive != semantic.StartInclusive || capture.summary.endInclusive != semantic.EndInclusive ||
		capture.summary.timezone != semantic.Timezone || capture.summary.currency != semantic.Currency ||
		capture.summary.minorUnitScale != semantic.MinorUnitScale || capture.summary.inflowMinor != semantic.InflowMinor ||
		capture.summary.outflowMinor != semantic.OutflowMinor || capture.summary.netMinor != semantic.NetMinor ||
		capture.summary.transactionCount != semantic.TransactionCount || capture.summary.evidenceRowCount != semantic.EvidenceTransactionCount ||
		capture.summary.evidenceRowCount != uint64(len(capture.rows)) ||
		capture.summary.aggregateComplete != semantic.AggregateComplete || capture.summary.evidenceRowsComplete != semantic.EvidenceRowsComplete ||
		semantic.Currentness != domainnative.AccountFlowProviderCurrentnessCurrentV1 ||
		!reflect.DeepEqual(capture.summary.coverage, semantic.Coverage) ||
		capture.summary.queryHash != semantic.QueryHash || capture.summary.resultHash != semantic.ResultHash ||
		!domainmcp.ValidLosslessToolResult(raw) || domainsecurity.SHA256Hex(raw.RawResult) != raw.RawSHA256 ||
		!domainsecurity.IsSHA256Hex(semantic.QueryHash) || !domainsecurity.IsSHA256Hex(semantic.ResultHash) ||
		domaincaseentity.ValidateReferenceV1(capture.summary.subjectRef) != nil {
		return VerifiedEvidenceMaterial{}, errors.New("account-flow semantic and private evidence do not match")
	}
	subjectRef := capture.summary.subjectRef
	identity, err := domainsecurity.ParseVerifiedMCPServerIdentity(probe.ServerIdentity)
	if err != nil || identity.ServerID != probe.ServerID || identity.ConnectionEpoch != probe.ConnectionEpoch || strings.TrimSpace(identity.ObservedVersion) == "" {
		return VerifiedEvidenceMaterial{}, errors.New("account-flow source identity is invalid")
	}
	start, startErr := canonicalAccountFlowEvidenceTime(semantic.StartInclusive)
	end, endErr := canonicalAccountFlowEvidenceTime(semantic.EndInclusive)
	if startErr != nil || endErr != nil {
		return VerifiedEvidenceMaterial{}, errors.New("account-flow semantic date range is invalid")
	}
	facts := make([]domainevidence.CanonicalEvidenceFact, 0, 3)
	sourceIDs := make([]string, 0, len(capture.rows))
	directions := []string{"in", "out"}
	rowInflow := new(big.Int)
	rowOutflow := new(big.Int)
	for index, row := range capture.rows {
		if index >= len(semantic.Transactions) || row.subjectRef != subjectRef || row.sourceRecordID != semantic.Transactions[index].EvidenceRef ||
			strings.TrimSpace(row.sourceFileID) == "" || row.sourceRowNumber == 0 ||
			row.occurredAt != semantic.Transactions[index].OccurredAt || row.direction != semantic.Transactions[index].Direction ||
			row.amountMinor != semantic.Transactions[index].AmountMinor || row.currency != semantic.Transactions[index].Currency ||
			row.minorUnitScale != semantic.Transactions[index].MinorUnitScale ||
			row.currency != semantic.Currency || row.minorUnitScale != semantic.MinorUnitScale ||
			(row.direction != domainnative.AccountFlowDirectionInflowV1 && row.direction != domainnative.AccountFlowDirectionOutflowV1) ||
			row.amountMinor == "" || !canonicalUnsignedAccountFlowAmount(row.amountMinor) {
			return VerifiedEvidenceMaterial{}, errors.New("account-flow private row authority is invalid")
		}
		occurredAt, timeErr := canonicalAccountFlowEvidenceTime(row.occurredAt)
		if timeErr != nil || occurredAt < start || occurredAt > end {
			return VerifiedEvidenceMaterial{}, errors.New("account-flow private row date is invalid")
		}
		direction := "in"
		if row.direction == domainnative.AccountFlowDirectionOutflowV1 {
			direction = "out"
		}
		amount, amountOK := new(big.Int).SetString(row.amountMinor, 10)
		if !amountOK {
			return VerifiedEvidenceMaterial{}, errors.New("account-flow private row amount is not canonical")
		}
		if direction == "in" {
			rowInflow.Add(rowInflow, amount)
		} else {
			rowOutflow.Add(rowOutflow, amount)
		}
		sourceIDs = append(sourceIDs, row.sourceRecordID)
	}
	expectedInflow, expectedInflowOK := new(big.Int).SetString(capture.summary.inflowMinor, 10)
	expectedOutflow, expectedOutflowOK := new(big.Int).SetString(capture.summary.outflowMinor, 10)
	if capture.summary.evidenceRowsComplete &&
		(!expectedInflowOK || !expectedOutflowOK || uint64(len(capture.rows)) != semantic.TransactionCount || rowInflow.Cmp(expectedInflow) != 0 || rowOutflow.Cmp(expectedOutflow) != 0) {
		return VerifiedEvidenceMaterial{}, errors.New("account-flow row and aggregate values do not reconcile")
	}
	if semantic.TransactionCount > 0 {
		if len(capture.rows) == 0 {
			return VerifiedEvidenceMaterial{}, errors.New("account-flow aggregate lacks source-row lineage")
		}
		inflow, inflowOK := new(big.Int).SetString(capture.summary.inflowMinor, 10)
		outflow, outflowOK := new(big.Int).SetString(capture.summary.outflowMinor, 10)
		if !inflowOK || !outflowOK || inflow.Sign() < 0 || outflow.Sign() < 0 {
			return VerifiedEvidenceMaterial{}, errors.New("account-flow aggregate amount is not canonical")
		}
		facts = append(facts,
			domainevidence.CanonicalEvidenceFact{
				FactID:    accountFlowAggregateFactPrefix + domainsecurity.SHA256Hex([]byte(input.Context.ContextDigest+"\x00"+semantic.QueryHash+"\x00in\x00"+capture.summary.inflowMinor)),
				ClaimType: domainevidence.ClaimAmount,
				NormalizedPayload: domainevidence.NormalizedClaimPayload{
					SubjectID: subjectRef, EntityID: subjectRef, AccountID: subjectRef,
					AmountMinor: capture.summary.inflowMinor, Currency: semantic.Currency, Direction: "in",
					StartAt: start, EndAt: end, Granularity: accountFlowReceiptGranularity,
				},
			},
			domainevidence.CanonicalEvidenceFact{
				FactID:    accountFlowAggregateFactPrefix + domainsecurity.SHA256Hex([]byte(input.Context.ContextDigest+"\x00"+semantic.QueryHash+"\x00out\x00"+capture.summary.outflowMinor)),
				ClaimType: domainevidence.ClaimAmount,
				NormalizedPayload: domainevidence.NormalizedClaimPayload{
					SubjectID: subjectRef, EntityID: subjectRef, AccountID: subjectRef,
					AmountMinor: capture.summary.outflowMinor, Currency: semantic.Currency, Direction: "out",
					StartAt: start, EndAt: end, Granularity: accountFlowReceiptGranularity,
				},
			},
			domainevidence.CanonicalEvidenceFact{
				FactID:    accountFlowAggregateFactPrefix + domainsecurity.SHA256Hex([]byte(input.Context.ContextDigest+"\x00"+semantic.QueryHash+"\x00count\x00"+strconv.FormatUint(capture.summary.transactionCount, 10))),
				ClaimType: domainevidence.ClaimCount,
				NormalizedPayload: domainevidence.NormalizedClaimPayload{
					SubjectID: subjectRef, EntityID: subjectRef,
					Count: strconv.FormatUint(capture.summary.transactionCount, 10), StartAt: start, EndAt: end,
					Granularity: accountFlowReceiptGranularity,
				},
			},
		)
	}
	sourceField, err := domainevidence.AcceptedSlotSourceFieldForModelEntityAliasV1(semantic.SubjectAlias)
	if err != nil || semantic.Outcome.SourceFieldReference.Field != sourceField ||
		domainevidence.ValidateAccountFlowTypedSourceFieldReferenceV1(
			semantic.Outcome.SourceFieldReference,
			semantic.QueryHash,
			semantic.ResultHash,
		) != nil {
		return VerifiedEvidenceMaterial{}, errors.New("account-flow accepted slot source alias is invalid")
	}
	canonicalMaterial := domainevidence.CanonicalEvidenceMaterial{
		SchemaVersion: domainevidence.CanonicalEvidenceVersion,
		Facts:         facts,
	}
	if len(facts) > 0 {
		factIDs := make([]string, len(facts))
		for index, fact := range facts {
			factIDs[index] = fact.FactID
		}
		sourceBindings := make([]domainevidence.AcceptedSlotSourceBindingV1, len(capture.rows))
		for index, row := range capture.rows {
			binding, bindingErr := domainevidence.NewAcceptedSlotSourceBindingV1(
				domainevidence.AcceptedSlotSourceBindingInputV1{
					FactIDs: factIDs, EntityReference: subjectRef,
					SourceRecordID: row.sourceRecordID, SourceFileID: row.sourceFileID,
					SourceRowNumber: row.sourceRowNumber,
					Field:           sourceField,
				},
			)
			if bindingErr != nil {
				return VerifiedEvidenceMaterial{}, errors.New("account-flow accepted slot source lineage is invalid")
			}
			sourceBindings[index] = binding
		}
		canonicalBindings, bindingErr := domainevidence.CanonicalAcceptedSlotSourceBindingsV1(sourceBindings)
		if bindingErr != nil {
			return VerifiedEvidenceMaterial{}, errors.New("account-flow accepted slot source lineage is invalid")
		}
		bindingDigest, bindingErr := domainevidence.AcceptedSlotSourceBindingSetDigestV1(canonicalBindings)
		if bindingErr != nil {
			return VerifiedEvidenceMaterial{}, errors.New("account-flow accepted slot source lineage is invalid")
		}
		canonicalMaterial.SchemaVersion = domainevidence.CanonicalEvidenceVersionV3
		canonicalMaterial.Purpose = domainevidence.CanonicalEvidencePurposeV3
		canonicalMaterial.AcceptedSlotSourceBindings = canonicalBindings
		canonicalMaterial.AcceptedSlotSourceBindingSetDigest = bindingDigest
	}
	canonicalBody, err := json.Marshal(canonicalMaterial)
	if err != nil {
		return VerifiedEvidenceMaterial{}, err
	}
	canonical, err := domainevidence.CanonicalEvidenceBytes(canonicalBody)
	if err != nil {
		return VerifiedEvidenceMaterial{}, err
	}
	filtersHash, err := accountFlowFiltersHashV1(subjectRef, semantic.StartInclusive, semantic.EndInclusive)
	if err != nil {
		return VerifiedEvidenceMaterial{}, err
	}
	queryScope, err := domainevidence.NewAccountFlowQueryScopeRefV1(input.Context.ContextDigest, semantic.QueryHash)
	if err != nil {
		return VerifiedEvidenceMaterial{}, errors.New("account-flow query scope binding is invalid")
	}
	completeness := domainevidence.PaginationPartial
	if semantic.AggregateComplete && semantic.EvidenceRowsComplete {
		completeness = domainevidence.PaginationComplete
	}
	return VerifiedEvidenceMaterial{
		CanonicalEvidence: canonical, SourceType: "transactions", ServerVersion: identity.ObservedVersion, QueryHash: semantic.QueryHash,
		QueryRange: domainevidence.EvidenceQueryRange{EntityIDs: []string{subjectRef}, AccountIDs: []string{subjectRef}, Directions: directions,
			StartAt: start, EndAt: end, SourceIDs: []string{queryScope}, FiltersHash: filtersHash},
		Granularity: accountFlowReceiptGranularity, Currency: semantic.Currency, Timezone: semantic.Timezone, PaginationCompleteness: completeness,
		SourceRecordIDs: sourceIDs, TransformationLineage: []domainevidence.TransformationLineageStep{
			{
				StepID: accountFlowNativeBindStepIDV1, Transformer: accountFlowNativeBinderV1, TransformerVersion: "1",
				InputHash: raw.RawSHA256, OutputHash: semantic.ResultHash,
			},
			{
				StepID: accountFlowNormalizeStepIDV1, Transformer: accountFlowNormalizerV1, TransformerVersion: "1",
				InputHash: semantic.ResultHash, OutputHash: domainsecurity.CanonicalJSONHash(canonical),
			},
		}, PIIClassification: domainevidence.PIINone,
	}, nil
}

func accountFlowFiltersHashV1(subject, start, end string) (string, error) {
	body, err := json.Marshal(struct {
		SchemaVersion  int    `json:"schemaVersion"`
		Purpose        string `json:"purpose"`
		SubjectRef     string `json:"subjectRef"`
		StartInclusive string `json:"startInclusive"`
		EndInclusive   string `json:"endInclusive"`
	}{1, "analytix.account-flow-filters/v1", subject, start, end})
	if err != nil {
		return "", err
	}
	return domainsecurity.CanonicalJSONHash(body), nil
}

func canonicalAccountFlowEvidenceTime(value string) (string, error) {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	return parsed.UTC().Format(time.RFC3339Nano), nil
}

func canonicalUnsignedAccountFlowAmount(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || (len(value) > 1 && value[0] == '0') {
		return false
	}
	parsed, ok := new(big.Int).SetString(value, 10)
	return ok && parsed.Sign() >= 0 && parsed.String() == value
}

func (service ToolEvidenceService) CommitCurrentToolEvidence(ctx context.Context, input CommitToolEvidenceInput) (domainevidence.EvidenceReceipt, error) {
	if input.Thread == nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil ||
		domainevidence.ValidateHostEvidenceSettlementMarker(input.Marker) != nil {
		return domainevidence.EvidenceReceipt{}, errors.New("tool evidence durable commit input is invalid")
	}
	prepared, err := service.Issuer.SettlementStore.ResolvePrepared(ctx, input.Marker.SettlementID)
	if err != nil || !reflect.DeepEqual(prepared.SecurityContext, input.Context) {
		return domainevidence.EvidenceReceipt{}, errors.New("tool evidence prepared context is unavailable")
	}
	authority, err := executiongrantapp.DurableSettlementFromThread(
		input.Context.ThreadID, input.Thread, input.Context.TurnID, prepared.ResultItemID, prepared.ExecutionGrant,
	)
	if err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	if prepared.HostAuthority != nil {
		source, sourceOK := service.Reader.(accountFlowHostSource)
		if !sourceOK {
			return domainevidence.EvidenceReceipt{}, errors.New("account-flow durable host authority is unavailable")
		}
		returnReceipt := domainevidence.EvidenceReceipt{}
		err = source.WithCurrentProbeAuthority(ctx, sourceprobeport.CurrentInput{
			ServerID: mcpServerID(prepared.ExecutionGrant.ToolName), Context: input.Context, Binding: prepared.HostAuthority.Binding,
			ConnectionEpoch: prepared.ExecutionGrant.ConnectionEpoch,
		}, func(probe domainsecurity.VerifiedSourceProbe, capability sourceprobeport.HostEvidenceCapability) error {
			selection, selectionErr := validatePreparedHostAuthoritySelection(prepared, input.Context, probe, capability)
			if selectionErr != nil {
				return errors.Join(errors.New("account-flow host source changed before durable commit"), selectionErr)
			}
			effect, ok := capability.(sourceprobeport.HostEvidenceRegistryCommitCapabilityV1)
			if !ok {
				return errors.New("account-flow host registry effect capability is unavailable")
			}
			return effect.UseExactRegistryCommit(input.Context, probe, selection, prepared, input.Marker, func(leaseContext context.Context) error {
				var commitErr error
				returnReceipt, commitErr = service.Issuer.Commit(leaseContext, CommitEvidenceInput{
					Context: input.Context, Marker: input.Marker, Authority: authority, CurrentProbe: probe,
					SelectionDigest: selection.SelectionDigest, HostAuthority: prepared.HostAuthority, HostCapability: capability,
				})
				return commitErr
			})
		})
		if err != nil {
			return domainevidence.EvidenceReceipt{}, err
		}
		return returnReceipt, nil
	}
	return service.Issuer.Commit(ctx, CommitEvidenceInput{Context: input.Context, Marker: input.Marker, Authority: authority})
}

type fundsCountEvidenceCandidate struct {
	SchemaVersion      int    `json:"schemaVersion"`
	Purpose            string `json:"purpose"`
	ServerName         string `json:"serverName"`
	ServerVersion      string `json:"serverVersion"`
	ToolName           string `json:"toolName"`
	CaseID             string `json:"caseId"`
	ContextDigest      string `json:"contextDigest"`
	ContextEpoch       uint64 `json:"contextEpoch"`
	DatasetSnapshotID  string `json:"datasetSnapshotId"`
	SnapshotContract   string `json:"snapshotContract"`
	TableName          string `json:"tableName"`
	NoFilter           bool   `json:"noFilter"`
	RowCount           string `json:"rowCount"`
	PaginationComplete bool   `json:"paginationComplete"`
	ReadOnly           bool   `json:"readOnly"`
}

func normalizeFundsCountEvidenceCandidate(input PrepareToolEvidenceInput, probe domainsecurity.VerifiedSourceProbe, raw domainmcp.LosslessToolResult) (VerifiedEvidenceMaterial, error) {
	if !domainsecurity.SourceProbeCanAuthorizeFacts(probe) ||
		domainsecurity.DatasetSnapshotFactAuthorityBlocker(input.Context.DatasetSnapshotID) != "" {
		return VerifiedEvidenceMaterial{}, errors.New("funds evidence snapshot authority is unavailable")
	}
	if !domainmcp.ValidLosslessToolResult(raw) || domainsecurity.SHA256Hex(raw.RawResult) != raw.RawSHA256 {
		return VerifiedEvidenceMaterial{}, errors.New("funds count evidence raw authority is invalid")
	}
	if err := domainjsonstrict.Validate(raw.RawResult, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 4 * 1024 * 1024, MaxTokens: 200_000, MaxStringBytes: 1024 * 1024,
	}); err != nil {
		return VerifiedEvidenceMaterial{}, errors.New("funds count evidence raw JSON is ambiguous")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw.RawResult))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var candidate fundsCountEvidenceCandidate
	if err := decoder.Decode(&candidate); err != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		return VerifiedEvidenceMaterial{}, errors.New("funds count evidence candidate schema is invalid")
	}
	identity, identityErr := domainsecurity.ParseVerifiedMCPServerIdentity(probe.ServerIdentity)
	if candidate.SchemaVersion != 1 || candidate.Purpose != fundsCountEvidencePurpose || candidate.ServerName != "analytix_funds" ||
		candidate.ServerVersion != fundsCountEvidenceServerVersion || candidate.ToolName != "count_case_rows" ||
		candidate.CaseID != input.Context.CaseID || candidate.ContextDigest != input.Context.ContextDigest ||
		candidate.ContextEpoch != input.Context.ContextEpoch || candidate.DatasetSnapshotID != input.Context.DatasetSnapshotID ||
		candidate.DatasetSnapshotID != probe.DatasetSnapshotID || candidate.SnapshotContract != "analytix_duckdb_dataset_snapshot_v1" ||
		candidate.TableName != fundsCountEvidenceTable || !candidate.NoFilter || !candidate.PaginationComplete || !candidate.ReadOnly ||
		probe.ServerID != "analytix_funds" || identityErr != nil || identity.ServerID != probe.ServerID ||
		identity.ObservedName != candidate.ServerName || identity.ObservedVersion != fundsCountEvidenceServerVersion || identity.ConnectionEpoch != probe.ConnectionEpoch {
		return VerifiedEvidenceMaterial{}, errors.New("funds count evidence candidate contradicts frozen source authority")
	}
	count, err := strconv.ParseUint(candidate.RowCount, 10, 63)
	if err != nil || strconv.FormatUint(count, 10) != candidate.RowCount {
		return VerifiedEvidenceMaterial{}, errors.New("funds count evidence candidate count is not canonical")
	}
	entityID := "dataset:" + fundsCountEvidenceTable
	queryContract := map[string]any{
		"schemaVersion": 1, "purpose": "analytix.host.dataset-table-row-count-query/v1",
		"contextDigest": input.Context.ContextDigest, "datasetSnapshotId": input.Context.DatasetSnapshotID,
		"toolName": input.Grant.ToolName, "tableName": fundsCountEvidenceTable, "noFilter": true,
	}
	queryBody, _ := json.Marshal(queryContract)
	queryHash := domainsecurity.CanonicalJSONHash(queryBody)
	filterBody, _ := json.Marshal(map[string]any{
		"schemaVersion": 1, "purpose": "analytix.host.no-filter/v1", "tableName": fundsCountEvidenceTable,
	})
	filtersHash := domainsecurity.CanonicalJSONHash(filterBody)
	factID := "fact_" + domainsecurity.SHA256Hex([]byte(input.Context.ContextDigest+"\x00"+queryHash+"\x00"+candidate.RowCount))
	canonicalBody, err := json.Marshal(domainevidence.CanonicalEvidenceMaterial{
		SchemaVersion: domainevidence.CanonicalEvidenceVersion,
		Facts: []domainevidence.CanonicalEvidenceFact{{
			FactID: factID, ClaimType: domainevidence.ClaimCount,
			NormalizedPayload: domainevidence.NormalizedClaimPayload{
				SubjectID: entityID, EntityID: entityID, Count: candidate.RowCount, Granularity: "dataset_table_rows",
			},
		}},
	})
	if err != nil {
		return VerifiedEvidenceMaterial{}, err
	}
	canonical, err := domainevidence.CanonicalEvidenceBytes(canonicalBody)
	if err != nil {
		return VerifiedEvidenceMaterial{}, err
	}
	material := VerifiedEvidenceMaterial{
		CanonicalEvidence: canonical, SourceType: domainevidence.SourceTypeTransactionDatasetInventory,
		ServerVersion: fundsCountEvidenceServerVersion, QueryHash: queryHash,
		QueryRange: domainevidence.EvidenceQueryRange{
			EntityIDs: []string{entityID}, AccountIDs: []string{}, Directions: []string{}, StartAt: "", EndAt: "",
			SourceIDs: []string{fundsCountEvidenceTable + "@" + input.Context.DatasetSnapshotID}, FiltersHash: filtersHash,
		},
		Granularity: "dataset_table_rows", Currency: "", Timezone: "UTC", PaginationCompleteness: domainevidence.PaginationComplete,
		SourceRecordIDs: []string{fundsCountEvidenceTable + "@" + input.Context.DatasetSnapshotID},
		TransformationLineage: []domainevidence.TransformationLineageStep{{
			StepID: "normalize-funds-count-v1", Transformer: "analytix-host-funds-count-normalizer",
			TransformerVersion: "1", InputHash: raw.RawSHA256, OutputHash: domainsecurity.CanonicalJSONHash(canonical),
		}},
		PIIClassification: domainevidence.PIINone,
	}
	return material, nil
}

func (service ToolEvidenceService) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

type toolEvidenceFinalizer struct {
	CasePublicationFinalizer
	toolEvidence ToolEvidenceAuthority
}

func WithToolEvidenceAuthority(finalizer CasePublicationFinalizer, authority ToolEvidenceAuthority) CasePublicationFinalizer {
	if finalizer == nil || authority == nil {
		return finalizer
	}
	return &toolEvidenceFinalizer{CasePublicationFinalizer: finalizer, toolEvidence: authority}
}

func (finalizer *toolEvidenceFinalizer) PrepareCurrentToolEvidence(ctx context.Context, input PrepareToolEvidenceInput) (PreparedToolEvidence, bool, error) {
	return finalizer.toolEvidence.PrepareCurrentToolEvidence(ctx, input)
}

func (finalizer *toolEvidenceFinalizer) CommitCurrentToolEvidence(ctx context.Context, input CommitToolEvidenceInput) (domainevidence.EvidenceReceipt, error) {
	return finalizer.toolEvidence.CommitCurrentToolEvidence(ctx, input)
}
