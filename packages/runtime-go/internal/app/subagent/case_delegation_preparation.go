package subagent

import (
	"context"
	"errors"

	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type preparedCaseDelegationV1 struct {
	service         *caseentityapp.Service
	aliases         caseentityapp.PreparedCaseLongitudinalAliasesV1
	securityContext domainsecurity.TurnSecurityContext
	expected        *domainjob.CaseDelegationContextV1
}

// PrepareCaseDelegationV1 uses the same typed semantic owner without persisting
// alias continuity. It returns no BoundCaseDelegationV1 or host-selection proof
// for an unwritten record. The host retains only an opaque process preparation.
func PrepareCaseDelegationV1(ctx context.Context, input BindCaseDelegationInputV1) (TaskRequest, error) {
	var aliases caseentityapp.PreparedCaseLongitudinalAliasesV1
	bound, err := bindCaseDelegationWithAliasesV1(ctx, input, func(ctx context.Context, selection caseentityapp.UseCaseLongitudinalAliasesInputV1, use func(caseentityapp.CaseLongitudinalAliasSelectionV1) error) error {
		var err error
		aliases, err = input.CaseEntities.PrepareCaseLongitudinalAliasesV1(ctx, selection)
		if err != nil {
			return err
		}
		return aliases.InspectSelectionV1(use)
	})
	if err != nil {
		return TaskRequest{}, err
	}
	request := bound.Request
	request.caseDelegationPreparation = nil
	if bound.Context != nil {
		request.caseDelegationPreparation = &preparedCaseDelegationV1{service: input.CaseEntities, aliases: aliases, securityContext: input.SecurityContext, expected: domainjob.CloneCaseDelegationContextV1(bound.Context)}
	}
	return request, nil
}

// ApplyPreparedCaseDelegationV1 is a mandatory predecessor to the matching
// first queued-job CAS. The host calls it only inside the signed child-producer
// lease; repeated or ambiguous applications cannot be retried by copying it.
func (request TaskRequest) ApplyPreparedCaseDelegationV1(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	prepared := request.caseDelegationPreparation
	if request.CaseDelegation == nil && prepared == nil {
		return nil
	}
	if prepared == nil || prepared.service == nil || prepared.securityContext != securityContext || !domainjob.CaseDelegationContextsEqualV1(request.CaseDelegation, prepared.expected) {
		return errors.New("case delegation preparation differs from admitted parent")
	}
	return prepared.service.ApplyPreparedCaseLongitudinalAliasesV1(ctx, prepared.aliases, func(selection caseentityapp.CaseLongitudinalAliasSelectionV1) error {
		if !domainsecurity.IsSHA256Hex(selection.Record.RecordID) || !domainsecurity.IsSHA256Hex(selection.Record.RecordDigest) {
			return caseentityapp.ErrPrivateStateIntegrity
		}
		return nil
	})
}
