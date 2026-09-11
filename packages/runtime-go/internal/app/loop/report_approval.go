package loop

import (
	"context"
	"errors"

	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type ReportApprovalPreparationInputV1 struct {
	Prompt              string
	Call                domainmodel.ToolCall
	ToolSchemas         []domainmodel.ToolSchema
	ToolScope           []string
	SecurityContext     domainsecurity.TurnSecurityContext
	HostEntitySelection HostCaseEntitySelectionV1
}

type ReportApprovalPreparationV1 struct {
	Call        domainmodel.ToolCall
	ToolSchemas []domainmodel.ToolSchema
	Controlled  bool
}

// ReportDeliveryApprovalHostV1 is an optional private capability implemented
// only by a report host that can retain one exact process-local controlled
// approval intent. The provider-visible report schema is never changed.
type ReportDeliveryApprovalHostV1 interface {
	PrepareReportApprovalWithinHeldAuthorityV1(
		context.Context,
		ReportApprovalPreparationInputV1,
	) (ReportApprovalPreparationV1, error)
	ValidatePreparedReportPendingCatalogV1(
		appmodel.PendingToolCall,
		[]domainmodel.ToolSchema,
		bool,
	) error
}

func PrepareReportApprovalV1(
	ctx context.Context,
	host ReportDeliveryHostV1,
	input ReportApprovalPreparationInputV1,
) (ReportApprovalPreparationV1, error) {
	unchanged := ReportApprovalPreparationV1{
		Call: input.Call,
		ToolSchemas: append(
			[]domainmodel.ToolSchema(nil),
			input.ToolSchemas...,
		),
	}
	if input.Call.Name != toolcatalogapp.ReportDeliveryToolName {
		return unchanged, nil
	}
	probe := appmodel.PendingToolCall{
		Prompt: input.Prompt,
		Call:   input.Call,
	}
	if len(toolcatalogapp.CanonicalControlledAccessActionsForPendingToolCallV1(probe)) == 0 {
		return unchanged, nil
	}
	approvalHost, ok := host.(ReportDeliveryApprovalHostV1)
	if !ok || approvalHost == nil || ctx == nil {
		return ReportApprovalPreparationV1{},
			errors.New("controlled report approval authority is unavailable")
	}
	prepared, err := approvalHost.PrepareReportApprovalWithinHeldAuthorityV1(
		ctx, input,
	)
	if err != nil || !prepared.Controlled ||
		prepared.Call.ID != input.Call.ID ||
		prepared.Call.Name != input.Call.Name ||
		len(prepared.ToolSchemas) != len(input.ToolSchemas) {
		return ReportApprovalPreparationV1{},
			errors.Join(
				errors.New("controlled report approval preparation failed"),
				err,
			)
	}
	return prepared, nil
}

// ValidateReportPendingCatalogV1 lets the private report owner replace
// only its report schema during catalog revalidation. The server remains a
// thin dispatcher and never learns the controlled approval contract.
func ValidateReportPendingCatalogV1(
	host ReportDeliveryHostV1,
	pending appmodel.PendingToolCall,
	currentSchemas []domainmodel.ToolSchema,
	currentReadOnly bool,
	fallback func(appmodel.PendingToolCall, []domainmodel.ToolSchema, bool) error,
) error {
	if pending.Call.Name != toolcatalogapp.ReportDeliveryToolName ||
		len(toolcatalogapp.CanonicalControlledAccessActionsForPendingToolCallV1(pending)) == 0 {
		if fallback == nil {
			return errors.New("tool catalog validation authority is unavailable")
		}
		return fallback(pending, currentSchemas, currentReadOnly)
	}
	approvalHost, ok := host.(ReportDeliveryApprovalHostV1)
	if !ok || approvalHost == nil {
		return errors.New("controlled report approval authority is unavailable")
	}
	return approvalHost.ValidatePreparedReportPendingCatalogV1(
		pending, currentSchemas, currentReadOnly,
	)
}
