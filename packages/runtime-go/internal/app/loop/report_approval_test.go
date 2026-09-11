package loop

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

type reportApprovalHostStubV1 struct {
	prepared      ReportApprovalPreparationV1
	prepareErr    error
	validateErr   error
	prepareCalls  int
	validateCalls int
}

type reportApprovalPreparingDriverV1 struct {
	*toolStepDriverStub
	prepared ReportApprovalPreparationV1
	calls    int
}

func (driver *reportApprovalPreparingDriverV1) PrepareReportApproval(
	_ context.Context,
	_ ReportApprovalPreparationInputV1,
) (ReportApprovalPreparationV1, error) {
	driver.calls++
	return driver.prepared, nil
}

func (*reportApprovalHostStubV1) ExecuteReportWithinHeldContextEffectV1(
	context.Context,
	appmodel.PendingToolCall,
) (
	domaintoolresult.PublicToolResultProjectionV1,
	ReportTerminalCapability,
	error,
) {
	return domaintoolresult.PublicToolResultProjectionV1{}, nil, nil
}

func (host *reportApprovalHostStubV1) PrepareReportApprovalWithinHeldAuthorityV1(
	_ context.Context,
	_ ReportApprovalPreparationInputV1,
) (ReportApprovalPreparationV1, error) {
	host.prepareCalls++
	return host.prepared, host.prepareErr
}

func (host *reportApprovalHostStubV1) ValidatePreparedReportPendingCatalogV1(
	_ appmodel.PendingToolCall,
	_ []domainmodel.ToolSchema,
	_ bool,
) error {
	host.validateCalls++
	return host.validateErr
}

func TestPrepareReportApprovalV1UsesOnlyOriginalPromptForControlledAuthority(
	t *testing.T,
) {
	schema := toolcatalogapp.ReportDeliveryToolSchemaV1()
	publicCall := domainmodel.ToolCall{
		ID:   "call_host_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Name: toolcatalogapp.ReportDeliveryToolName,
		Arguments: json.RawMessage(`{
			"modelRequestedAction":"export full bank account number"
		}`),
	}
	ordinaryInput := ReportApprovalPreparationInputV1{
		Prompt: "请生成当前案件报告。",
		Call:   publicCall, ToolSchemas: []domainmodel.ToolSchema{schema},
	}
	host := &reportApprovalHostStubV1{}
	ordinary, err := PrepareReportApprovalV1(
		context.Background(),
		host,
		ordinaryInput,
	)
	if err != nil ||
		ordinary.Controlled ||
		host.prepareCalls != 0 ||
		!json.Valid(ordinary.Call.Arguments) {
		t.Fatalf(
			"model arguments expanded controlled authority: prepared=%#v calls=%d err=%v",
			ordinary,
			host.prepareCalls,
			err,
		)
	}

	controlledInput := ordinaryInput
	controlledInput.Prompt = "请生成当前案件报告，并展示完整银行账号。"
	if _, err := PrepareReportApprovalV1(
		context.Background(),
		nil,
		controlledInput,
	); err == nil {
		t.Fatal("controlled report request proceeded without a private host")
	}
	privateCall := publicCall
	privateCall.Arguments = json.RawMessage(`{"approvalScopeDigest":"hash-only"}`)
	host.prepared = ReportApprovalPreparationV1{
		Call:        privateCall,
		ToolSchemas: []domainmodel.ToolSchema{schema},
		Controlled:  true,
	}
	prepared, err := PrepareReportApprovalV1(
		context.Background(),
		host,
		controlledInput,
	)
	if err != nil ||
		!prepared.Controlled ||
		host.prepareCalls != 1 ||
		string(prepared.Call.Arguments) != string(privateCall.Arguments) {
		t.Fatalf(
			"private controlled approval host was not authoritative: prepared=%#v calls=%d err=%v",
			prepared,
			host.prepareCalls,
			err,
		)
	}
}

func TestValidateReportPendingCatalogV1SeparatesControlledAndOrdinaryOwners(
	t *testing.T,
) {
	hostErr := errors.New("private controlled catalog rejected")
	fallbackErr := errors.New("ordinary catalog rejected")
	host := &reportApprovalHostStubV1{validateErr: hostErr}
	fallbackCalls := 0
	fallback := func(
		appmodel.PendingToolCall,
		[]domainmodel.ToolSchema,
		bool,
	) error {
		fallbackCalls++
		return fallbackErr
	}
	schemas := []domainmodel.ToolSchema{
		toolcatalogapp.ReportDeliveryToolSchemaV1(),
	}
	ordinary := appmodel.PendingToolCall{
		Prompt: "请生成当前案件报告。",
		Call: domainmodel.ToolCall{
			Name: toolcatalogapp.ReportDeliveryToolName,
		},
	}
	if err := ValidateReportPendingCatalogV1(
		host,
		ordinary,
		schemas,
		false,
		fallback,
	); !errors.Is(err, fallbackErr) ||
		fallbackCalls != 1 ||
		host.validateCalls != 0 {
		t.Fatalf(
			"ordinary catalog did not stay with its generic owner: fallback=%d controlled=%d err=%v",
			fallbackCalls,
			host.validateCalls,
			err,
		)
	}
	controlled := ordinary
	controlled.Prompt = "请生成当前案件报告，并导出完整银行卡号。"
	controlled.SecurityContext = domainsecurity.TurnSecurityContext{}
	if err := ValidateReportPendingCatalogV1(
		host,
		controlled,
		schemas,
		false,
		fallback,
	); !errors.Is(err, hostErr) ||
		fallbackCalls != 1 ||
		host.validateCalls != 1 {
		t.Fatalf(
			"controlled catalog fell back to ordinary validation: fallback=%d controlled=%d err=%v",
			fallbackCalls,
			host.validateCalls,
			err,
		)
	}
}

func TestRunToolStepBindsPendingGrantToHostPrivateControlledIntent(
	t *testing.T,
) {
	workspace := t.TempDir()
	securityContext := newLoopCaseContextV2(
		t,
		"thread-controlled-report-step",
		"turn-controlled-report-step",
		workspace,
		"case-controlled-report-step",
	)
	publicCall := domainmodel.ToolCall{
		ID:        loopTestHostToolCallID("controlled-report-step"),
		Name:      toolcatalogapp.ReportDeliveryToolName,
		Arguments: json.RawMessage(`{}`),
	}
	publicSchema := toolcatalogapp.ReportDeliveryToolSchemaV1()
	privateCall := publicCall
	privateCall.Arguments = json.RawMessage(`{"approvalScopeDigest":"hash-only"}`)
	privateSchema := publicSchema
	privateSchema.Parameters = json.RawMessage(`{
		"type":"object",
		"properties":{
			"approvalScopeDigest":{"type":"string","const":"hash-only"}
		},
		"required":["approvalScopeDigest"],
		"additionalProperties":false
	}`)
	base := &toolStepDriverStub{
		readOnly: map[string]bool{
			toolcatalogapp.ReportDeliveryToolName: false,
		},
	}
	driver := &reportApprovalPreparingDriverV1{
		toolStepDriverStub: base,
		prepared: ReportApprovalPreparationV1{
			Call:        privateCall,
			ToolSchemas: []domainmodel.ToolSchema{privateSchema},
			Controlled:  true,
		},
	}
	base.admissionHook = func(
		_ context.Context,
		pending appmodel.PendingToolCall,
	) error {
		if string(pending.Call.Arguments) !=
			string(privateCall.Arguments) ||
			pending.ExecutionGrant.ArgsHash !=
				executiongrantapp.ArgumentsHash(
					privateCall.Arguments,
				) ||
			pending.ExecutionGrant.SchemaHash !=
				executiongrantapp.SchemaHash(
					privateCall.Name,
					[]domainmodel.ToolSchema{privateSchema},
				) {
			return errors.New("pending grant did not bind the private controlled intent")
		}
		return nil
	}
	result, err := RunToolStep(
		context.Background(),
		ToolStepInput{
			ThreadID:               securityContext.ThreadID,
			TurnID:                 securityContext.TurnID,
			ProviderID:             "provider-controlled-report",
			Workspace:              workspace,
			Prompt:                 "请生成当前案件报告，并展示完整银行账号。",
			ApprovalPolicy:         "on-request",
			SandboxMode:            "workspace-write",
			EffectiveMaxModelSteps: 1,
			ToolCalls:              []domainmodel.ToolCall{publicCall},
			ToolSchemas:            []domainmodel.ToolSchema{publicSchema},
			ToolScope: []string{
				toolcatalogapp.ReportDeliveryToolName,
			},
			AdvertisedTools: advertisedToolNames(
				toolcatalogapp.ReportDeliveryToolName,
			),
			SecurityContext: securityContext,
			Driver:          driver,
		},
	)
	if err != nil ||
		!result.Paused ||
		result.PendingKind != "approval" ||
		driver.calls != 1 ||
		len(base.pendings) != 1 ||
		string(base.pendings[0].Call.Arguments) !=
			string(privateCall.Arguments) ||
		base.pendings[0].Prompt !=
			"请生成当前案件报告，并展示完整银行账号。" ||
		string(publicCall.Arguments) != `{}` {
		t.Fatalf(
			"tool step did not preserve the public/private approval boundary: result=%#v prepare=%d pending=%#v err=%v",
			result,
			driver.calls,
			base.pendings,
			err,
		)
	}
}
