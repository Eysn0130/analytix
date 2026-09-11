package subagent

import (
	"context"
	"errors"
	"strings"

	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	apploop "analytix.local/runtime-go/internal/app/loop"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type CurrentChildCaseDelegationReaderV1 interface {
	LoadChildRun(string) (domainjob.Record, error)
}

type ResolveCurrentChildCaseDelegationInputV1 struct {
	ChildRunID      string
	ChildThreadID   string
	Prompt          string
	SecurityContext domainsecurity.TurnSecurityContext
	Jobs            CurrentChildCaseDelegationReaderV1
	Blocker         func(domainjob.Record) string
	CaseEntities    *caseentityapp.Service
}

type CurrentChildCaseDelegationV1 struct {
	Prompt        string
	HostSelection apploop.HostCaseEntitySelectionV1
	Aliases       []domaincaseentity.ModelEntityAliasV1
	CurrentRecord domainjob.Record
}

// ResolveCurrentChildCaseDelegationV1 re-resolves the durable delegated alias
// set under the newly frozen child TSCV2. Persisted prompt or provider history
// cannot substitute for the current private caseentity owner.
func ResolveCurrentChildCaseDelegationV1(
	ctx context.Context,
	input ResolveCurrentChildCaseDelegationInputV1,
) (CurrentChildCaseDelegationV1, error) {
	childRunID := strings.TrimSpace(input.ChildRunID)
	if childRunID == "" || input.Jobs == nil || input.Blocker == nil {
		return CurrentChildCaseDelegationV1{}, errors.New("case delegation replay authority is unavailable")
	}
	record, err := input.Jobs.LoadChildRun(childRunID)
	if err != nil || record.ID != childRunID || record.ChildThreadID != input.ChildThreadID ||
		record.SecurityBinding == nil || input.Blocker(record) != "" {
		return CurrentChildCaseDelegationV1{}, errors.New("durable child case delegation is unavailable")
	}
	if record.SecurityBinding.ParentCaseID == domainsecurity.UnboundCaseID {
		if record.CaseDelegation != nil {
			return CurrentChildCaseDelegationV1{}, errors.New("unbound child contains case delegation")
		}
		return CurrentChildCaseDelegationV1{Prompt: input.Prompt}, nil
	}
	if input.CaseEntities == nil || input.Prompt != record.Prompt ||
		domainjob.ValidateExecutableCaseDelegationV1(
			record.Kind, record.Name, record.Label, record.Prompt, record.SecurityBinding, record.CaseDelegation,
		) != nil {
		return CurrentChildCaseDelegationV1{}, errors.New("durable child case delegation changed")
	}
	bound, err := BindCaseDelegationV1(ctx, BindCaseDelegationInputV1{
		SecurityContext: input.SecurityContext, SecurityBinding: record.SecurityBinding,
		CaseEntities:   input.CaseEntities,
		Request:        TaskRequest{Prompt: record.Prompt, CaseDelegation: domainjob.CloneCaseDelegationContextV1(record.CaseDelegation)},
		SourceThreadID: record.SecurityBinding.ParentThreadID, Expected: record.CaseDelegation,
	})
	if err != nil || !domainjob.CaseDelegationContextsEqualV1(bound.Context, record.CaseDelegation) {
		return CurrentChildCaseDelegationV1{}, errors.New("current child case delegation resolution failed closed")
	}
	var selection apploop.HostCaseEntitySelectionV1
	if err := bound.UseHostSelectionV1(input.SecurityContext, func(
		recordReference caseentityapp.PrivateRecordReferenceV1,
		references []domaincaseentity.ReferenceV1,
		aliases []domaincaseentity.ModelEntityAliasV1,
	) error {
		var selectionErr error
		selection, selectionErr = apploop.NewHostCaseEntitySelectionFromPersistedIngressAliasesV1(
			input.SecurityContext, recordReference, references, aliases,
		)
		return selectionErr
	}); err != nil || !selection.AvailableV1() {
		return CurrentChildCaseDelegationV1{}, errors.New("current child case delegation host selection is unavailable")
	}
	return CurrentChildCaseDelegationV1{
		Prompt: record.Prompt, HostSelection: selection, Aliases: domainjob.CaseDelegationAliasesV1(record.CaseDelegation),
		CurrentRecord: record,
	}, nil
}
