package loop

import (
	"errors"
	"sync"

	appmodel "analytix.local/runtime-go/internal/app/model"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	visionbridgeapp "analytix.local/runtime-go/internal/app/visionbridge"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

const accountFlowProviderToolNameV1 = "mcp__analytix_funds__analyze_account_flows"

type ToolResultProviderAttemptInputV1 struct {
	Pending            appmodel.PendingToolCall
	Output             any
	IsError            bool
	Config             runtimeinfoapp.VisionBridgeConfig
	Source             any
	ProjectExact       ToolResultExactProjectionV1
	ProjectExactPublic ToolResultExactPublicProjectionV1
}

type ToolResultExactProjectionV1 func(appmodel.PendingToolCall, any) (map[string]any, map[string]any, bool, bool)
type ToolResultExactPublicProjectionV1 func(appmodel.PendingToolCall, any) (map[string]any, bool, bool)

type PreparedToolResultProjectionV1 struct {
	Projection   domaintoolresult.PublicToolResultProjectionV1
	PublicOutput any
	Message      domainmodel.Message
}

type ToolResultProviderAttemptV1 struct {
	pending               appmodel.PendingToolCall
	config                runtimeinfoapp.VisionBridgeConfig
	projectExact          ToolResultExactProjectionV1
	projectExactPublic    ToolResultExactPublicProjectionV1
	accountFlowApplicable bool
	accountFlow           domainnative.AccountFlowProviderModelOutputV1
	captureErr            error
	discardPrivate        func()
	discardOnce           sync.Once
}

type accountFlowProviderSemanticReaderV1 interface {
	HostFundsAccountFlowProviderSemanticV1(domainmcp.LosslessToolResult) (domainnative.AccountFlowProviderSemanticResultV1, bool)
}

type accountFlowProviderPrivateDisposerV1 interface {
	DiscardHostFundsAccountFlowEvidenceV1(domainmcp.LosslessToolResult)
}

func CaptureToolResultProviderAttemptV1(input ToolResultProviderAttemptInputV1) *ToolResultProviderAttemptV1 {
	attempt := &ToolResultProviderAttemptV1{
		pending: input.Pending, config: input.Config, projectExact: input.ProjectExact,
		projectExactPublic: input.ProjectExactPublic,
	}
	attempt.captureAccountFlowProviderSemanticV1(input.Output, input.IsError, input.Source)
	return attempt
}

func (attempt *ToolResultProviderAttemptV1) captureAccountFlowProviderSemanticV1(output any, isError bool, source any) {
	if attempt == nil || attempt.pending.Call.Name != accountFlowProviderToolNameV1 {
		return
	}
	record, recordOK := output.(map[string]any)
	lossless, losslessOK := record[domainmcp.HostRawToolResultKey].(domainmcp.LosslessToolResult)
	if disposer, ok := source.(accountFlowProviderPrivateDisposerV1); ok && losslessOK {
		attempt.discardPrivate = func() {
			disposer.DiscardHostFundsAccountFlowEvidenceV1(lossless)
		}
	}
	if isError {
		return
	}
	attempt.accountFlowApplicable = true
	reader, readerOK := source.(accountFlowProviderSemanticReaderV1)
	if !recordOK || !losslessOK || !domainmcp.ValidLosslessToolResult(lossless) || !readerOK {
		attempt.captureErr = errors.New("account-flow provider semantic authority is unavailable")
		return
	}
	semantic, valid := reader.HostFundsAccountFlowProviderSemanticV1(lossless)
	if !valid {
		attempt.captureErr = errors.New("account-flow provider semantic authority is unavailable")
		return
	}
	status := domainevidence.SemanticPartial
	if semantic.AggregateComplete && semantic.EvidenceRowsComplete && semantic.CounterpartySemanticsComplete {
		status = domainevidence.SemanticSuccess
	}
	attempt.accountFlow = domainnative.AccountFlowProviderModelOutputV1{
		SchemaVersion: 3, Purpose: domainnative.AccountFlowProviderModelPurposeV1,
		SemanticStatus: status, Data: semantic,
	}
	if _, err := domainnative.CanonicalAccountFlowProviderModelOutputV1(attempt.accountFlow); err != nil {
		attempt.accountFlow = domainnative.AccountFlowProviderModelOutputV1{}
		attempt.captureErr = errors.New("account-flow provider semantic authority is unavailable")
	}
}

func (attempt *ToolResultProviderAttemptV1) PrepareSettlementV1(
	settlementOutput any,
	settlementIsError bool,
) (PreparedToolResultProjectionV1, error) {
	if attempt == nil {
		return PreparedToolResultProjectionV1{}, errors.New("tool-result provider attempt is unavailable")
	}
	if attempt.accountFlowApplicable && attempt.captureErr != nil {
		return PreparedToolResultProjectionV1{}, attempt.captureErr
	}
	persistOutput := PersistableToolOutputV1(attempt.pending, settlementOutput, attempt.projectExactPublic)
	var subagentPrivate, subagentPublic map[string]any
	valid := false
	if attempt.projectExact != nil {
		subagentPrivate, subagentPublic, _, valid = attempt.projectExact(attempt.pending, settlementOutput)
	}
	if settlementIsError && subagentPrivate != nil {
		// A process-local foreground carrier is burned by exact projection, but
		// an error settlement may never inject it into a parent provider attempt.
		subagentPrivate = nil
		subagentPublic = nil
		valid = false
	}
	var privateOutput any = subagentPrivate
	var publicOutput any = subagentPublic
	if settlementIsError {
		publicOutput = persistOutput
	}
	useAccountFlow := attempt.accountFlowApplicable && !settlementIsError
	if useAccountFlow {
		if _, err := domainnative.CanonicalAccountFlowProviderModelOutputV1(attempt.accountFlow); err != nil {
			return PreparedToolResultProjectionV1{}, errors.New("account-flow provider semantic authority is unavailable")
		}
		privateOutput = attempt.accountFlow
		publicOutput = persistOutput
		valid = true
	}
	projection, content := visionbridgeapp.PrepareToolResultForModel(visionbridgeapp.ToolResultProjectionInput{
		ToolName: attempt.pending.Call.Name, RawOutput: settlementOutput, PersistOutput: persistOutput,
		IsError: settlementIsError, Config: attempt.config, ExactPrivateModelOutput: privateOutput,
		ExactPublicOutput: publicOutput, HasExactPrivateOutput: valid,
	})
	message := toolcatalogapp.ToolResultMessage(attempt.pending.Call, content)
	if valid && subagentPrivate != nil {
		applicable, err := privacyprojectionapp.BindCaseForegroundProviderSemanticV1(
			attempt.pending.SecurityContext, attempt.pending.Call, privateOutput, &message,
		)
		if err != nil || applicable && message.PrivateProviderSemanticBinding == nil {
			return PreparedToolResultProjectionV1{}, errors.New("foreground case provider semantic binding failed")
		}
	}
	if useAccountFlow {
		bound, err := privacyprojectionapp.BindAccountFlowProviderSemanticV1(
			attempt.pending.SecurityContext,
			attempt.pending.Call,
			attempt.accountFlow,
			&message,
		)
		if err != nil || !bound {
			return PreparedToolResultProjectionV1{}, errors.New("account-flow provider semantic binding failed")
		}
	}
	return PreparedToolResultProjectionV1{
		Projection: projection, PublicOutput: domaintoolresult.PublicToolResultProjectionRecordV1(projection),
		Message: message,
	}, nil
}

func (attempt *ToolResultProviderAttemptV1) DiscardPrivate() {
	if attempt == nil {
		return
	}
	attempt.discardOnce.Do(func() {
		if attempt.discardPrivate != nil {
			attempt.discardPrivate()
		}
		attempt.discardPrivate = nil
		attempt.accountFlow = domainnative.AccountFlowProviderModelOutputV1{}
		attempt.captureErr = nil
	})
}

func PersistableToolOutputV1(pending appmodel.PendingToolCall, output any, projectExactPublic ToolResultExactPublicProjectionV1) any {
	foreground := toolcatalogapp.HostForegroundTaskCallForContextV1(
		pending.Call, pending.SubagentDepth, pending.SecurityContext,
	)
	if projectExactPublic != nil {
		public, applicable, _ := projectExactPublic(pending, output)
		if applicable {
			return public
		}
	}
	if foreground {
		return domaintoolresult.ForegroundHandoffInvalidPublicOutputV1()
	}
	return toolcatalogapp.PersistableToolOutputForExecution(
		pending.Call,
		pending.SecurityContext,
		pending.ExecutionGrant,
		output,
	)
}
