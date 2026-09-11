package loop

import (
	"errors"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const executionGrantEntityReferenceUnavailableV1 = "execution_grant_entity_reference_unavailable"

// validateFundsSubjectReferenceProvenanceV1 runs before an execution grant is
// issued. It accepts a subject only when the same final-projection sidecars
// prove that the reference came from current host ingress, a bound funds
// semantic, or the host-compiled accepted-final context for this exact turn.
func validateFundsSubjectReferenceProvenanceV1(
	input ToolStepInput,
	call domainmodel.ToolCall,
) error {
	if call.Name != providerFundsAccountFlowToolNameV1 {
		return nil
	}
	arguments, err := domainsecurity.DecodeCanonicalJSONObject(call.Arguments)
	if err != nil {
		return err
	}
	subjectText, ok := arguments["subject_alias"].(string)
	if !ok || domaincaseentity.ValidateModelEntityAliasV1(subjectText) != nil {
		return errors.New("funds account-flow subject alias is invalid")
	}
	if !input.HostEntitySelection.AllowsModelAliasV1(
		input.SecurityContext,
		domaincaseentity.ModelEntityAliasV1(subjectText),
	) {
		return fundsSubjectReferenceUnavailableV1(call.Name)
	}
	return nil
}

func fundsSubjectReferenceUnavailableV1(toolName string) error {
	return TurnFailureError{
		Message: "funds account-flow subject is unavailable under current host provenance",
		Code:    "execution_grant_rejected",
		Details: map[string]any{
			"code":     executionGrantEntityReferenceUnavailableV1,
			"toolName": toolName,
		},
		Severity: "error",
	}
}
