package turnstart

import (
	"context"
	"errors"
	"strings"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type ChildRunAuthorityStore interface {
	LoadChildRun(string) (domainjob.Record, error)
	ValidateChildRunStart(domainjob.Record) (domainjob.Record, error)
}

type ChildRunBlocker func(domainjob.Record) string

type ChildTransitionResolveInput struct {
	ChildRunID    string
	ChildDepth    int
	ChildThreadID string
	Store         ChildRunAuthorityStore
	Blocker       ChildRunBlocker
}

func ResolveChildTransitionAuthority(input ChildTransitionResolveInput) (*ChildTransitionAuthority, error) {
	childRunID := strings.TrimSpace(input.ChildRunID)
	if childRunID == "" {
		if input.ChildDepth != 0 {
			return nil, errors.New("child turn depth is not bound to a durable child run")
		}
		return nil, nil
	}
	if input.ChildDepth <= 0 || input.Store == nil || input.Blocker == nil {
		return nil, errors.New("durable child turn authority is unavailable")
	}
	record, err := input.Store.LoadChildRun(childRunID)
	binding := record.SecurityBinding
	if err != nil || record.ID != childRunID || strings.TrimSpace(record.ChildThreadID) != strings.TrimSpace(input.ChildThreadID) ||
		strings.TrimSpace(record.Status) != string(domainjob.StatusRunning) || domainjob.ValidateSecurityBinding(binding) != nil ||
		record.ParentThreadID != binding.ParentThreadID || record.ParentTurnID != binding.ParentTurnID ||
		record.ParentToolCallID != binding.ParentToolCallID || strings.TrimSpace(record.ParentToolCallID) == "" ||
		input.Blocker(record) != "" {
		return nil, errors.New("durable child turn authority is invalid")
	}
	return &ChildTransitionAuthority{
		ChildRunID: childRunID, ChildThreadID: strings.TrimSpace(record.ChildThreadID),
		Background: record.Background, Binding: domainjob.CloneSecurityBinding(binding), ExpectedRecord: record,
		frozenWitness: &childTransitionFrozenWitnessStateV1{},
	}, nil
}

func ResolveDelegatedToolAuthority(
	authority *ChildTransitionAuthority,
	requestedScope []string,
) ([]string, *domainjob.DelegatedToolManifestV1, error) {
	if authority == nil {
		return append([]string(nil), requestedScope...), nil, nil
	}
	record := authority.ExpectedRecord
	requestScopeHash, requestScopeErr := domainjob.DelegatedToolScopeHashV1(requestedScope)
	recordScopeHash, recordScopeErr := domainjob.DelegatedToolScopeHashV1(record.ToolScope)
	if requestScopeErr != nil || recordScopeErr != nil || requestScopeHash != recordScopeHash ||
		domainjob.ValidateDelegatedToolManifestV1(record.DelegatedToolManifest, record.ToolScope, record.ToolSchemaHash) != nil {
		return nil, nil, errors.New("durable child delegated tool authority is invalid")
	}
	return append([]string(nil), record.ToolScope...), domainjob.CloneDelegatedToolManifestV1(record.DelegatedToolManifest), nil
}

func ValidateChildTransitionFrozen(
	ctx context.Context,
	authority *ChildTransitionAuthority,
	store ChildRunAuthorityStore,
	blocker ChildRunBlocker,
	frozen domainsecurity.TurnSecurityContext,
) error {
	if authority == nil {
		return nil
	}
	if ctx == nil || ctx.Err() != nil || store == nil || blocker == nil ||
		domainsecurity.ValidateTurnSecurityContextForExecution(frozen) != nil || frozen.ThreadID != authority.ChildThreadID ||
		domainjob.ValidateSecurityBinding(authority.Binding) != nil {
		return errors.New("frozen durable child turn authority is invalid")
	}
	record, err := store.ValidateChildRunStart(authority.ExpectedRecord)
	if err != nil || record.ID != authority.ChildRunID || record.ChildThreadID != authority.ChildThreadID ||
		record.ChildTurnID == "" || record.ChildTurnID != authority.ExpectedRecord.ChildTurnID || frozen.TurnID != record.ChildTurnID ||
		record.SecurityBinding == nil || record.SecurityBinding.BindingDigest != authority.Binding.BindingDigest ||
		record.Background != authority.Background || blocker(record) != "" ||
		!domainjob.SecurityBindingMatchesWorkspaceScope(record.SecurityBinding, frozen) {
		return errors.New("durable child turn authority changed while waiting for the transition writer")
	}
	return authority.frozenWitness.validateOnceV1(frozen, authority.ExpectedRecord)
}

// HostChildCasePromptCandidateV1 recognizes only the canonical prompt that the
// host committed into the exact durable child record. It is deliberately
// independent of provider arguments and ordinary prompt text.
func HostChildCasePromptCandidateV1(authority *ChildTransitionAuthority, prompt string) string {
	if authority == nil {
		return ""
	}
	record := authority.ExpectedRecord
	prompt = strings.TrimSpace(prompt)
	canonical, err := domainjob.CaseDelegationProviderPromptV1(record.CaseDelegation, record.SecurityBinding)
	if err != nil || record.CaseDelegation == nil || prompt == "" || prompt != record.Prompt || prompt != canonical ||
		domainjob.ValidateExecutableCaseDelegationV1(
			record.Kind, record.Name, record.Label, canonical, record.SecurityBinding, record.CaseDelegation,
		) != nil {
		return ""
	}
	return canonical
}

// ValidateHostChildCasePromptFrozenV1 binds a canonical host-created prompt to
// the child TSC frozen under the same transition writer that validated the
// current durable ChildTransitionAuthority. It does not replace that durable
// validation and must be called only after ValidateChildTransitionFrozen.
func ValidateHostChildCasePromptFrozenV1(
	authority *ChildTransitionAuthority,
	frozen domainsecurity.TurnSecurityContext,
	expectedTurnID string,
	candidate string,
) (string, error) {
	if authority == nil || authority.Binding == nil || authority.ExpectedRecord.SecurityBinding == nil {
		return "", errors.New("host child case prompt authority is invalid")
	}
	record := authority.ExpectedRecord
	canonical := HostChildCasePromptCandidateV1(authority, candidate)
	if domainjob.ValidateSecurityBinding(authority.Binding) != nil ||
		authority.ChildRunID != record.ID || authority.ChildThreadID != record.ChildThreadID ||
		authority.Binding.BindingDigest != record.SecurityBinding.BindingDigest ||
		frozen.ThreadID != authority.ChildThreadID || frozen.TurnID != expectedTurnID || record.ChildTurnID != expectedTurnID ||
		!domainjob.SecurityBindingMatchesCaseEpochScope(record.SecurityBinding, frozen) ||
		canonical == "" || canonical != candidate {
		return "", errors.New("host child case prompt authority is invalid")
	}
	return canonical, nil
}
