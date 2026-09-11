// Package securitycontext contains deterministic fixtures for Go tests. It is
// not linked by production composition and must only be imported from *_test.go
// files.
package securitycontext

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// GeneralExecutionContextV2 returns a structurally valid witnessed V2 context
// for tests that exercise a live execution boundary. It deliberately keeps
// the witness construction in test support so production code cannot acquire
// authority from a local convenience constructor.
func GeneralExecutionContextV2(input domainsecurity.TurnSecurityContextInput) (domainsecurity.TurnSecurityContext, error) {
	input.CaseID = domainsecurity.UnboundCaseID
	input.CaseBindingHash = domainsecurity.UnboundCaseBindingHash(input.WorkspaceRealPath)
	input.DatasetSnapshotID = domainsecurity.NoDatasetSnapshotID
	input.SourceManifestHash = domainsecurity.EmptySourceManifestHash
	return executionContextV2(input, domainsecurity.RiskClassGeneral, domainsecurity.PublicationDispositionGeneralOutput, domainsecurity.CaseBindingStateMissing)
}

// HostGeneralOnlyExecutionContextV2 constructs the deterministic production
// fallback shape for tests. Unlike GeneralExecutionContextV2, it carries no
// local or witnessed signing authority and can authorize only general output
// plus host-classified read-only tools.
func HostGeneralOnlyExecutionContextV2(input domainsecurity.TurnSecurityContextInput) (domainsecurity.TurnSecurityContext, error) {
	if input.ThreadID == "" || input.TurnID == "" || input.WorkspaceRealPath == "" {
		return domainsecurity.TurnSecurityContext{}, errors.New("test host general-only context identity is incomplete")
	}
	if input.TenantID == "" {
		input.TenantID = domainsecurity.LocalTenantID
	}
	if input.UserID == "" {
		input.UserID = domainsecurity.LocalUserID
	}
	if input.ContextEpoch == 0 {
		input.ContextEpoch = 1
	}
	if input.IssuedAt.IsZero() {
		input.IssuedAt = time.Unix(1_700_000_000, 0).UTC()
	}
	input.CaseID = domainsecurity.UnboundCaseID
	input.CaseBindingHash = domainsecurity.UnboundCaseBindingHash(input.WorkspaceRealPath)
	input.DatasetSnapshotID = domainsecurity.NoDatasetSnapshotID
	input.SourceManifestHash = domainsecurity.EmptySourceManifestHash
	observationDigest := domainsecurity.SHA256Hex([]byte("test-host-general-only-observation:\x00" + input.WorkspaceRealPath))
	policy, err := domainsecurity.NewGeneralOnlyRiskPolicyV1(input.ThreadID, input.WorkspaceRealPath, observationDigest)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: policy.PolicyDigest, RiskClass: domainsecurity.RiskClassGeneral,
		Disposition: domainsecurity.PublicationDispositionGeneralOutput, CaseBindingState: domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: observationDigest, BlockerCode: domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	binding, err := domainsecurity.NewHostGeneralOnlyRiskAuthorityBindingV1(policy)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	input.PublicationPolicy = publication
	input.RiskAuthorityBinding = binding
	return domainsecurity.NewTurnSecurityContextV2(input)
}

// CaseExecutionContextV2 returns a structurally valid witnessed V2 case
// context for tests. Callers may provide their own opaque case, binding,
// snapshot, and manifest values; omitted values are deterministic test-only
// defaults, never inferred by production issuance.
func CaseExecutionContextV2(input domainsecurity.TurnSecurityContextInput) (domainsecurity.TurnSecurityContext, error) {
	if input.CaseID == "" {
		input.CaseID = "case-test"
	}
	if input.CaseBindingHash == "" {
		input.CaseBindingHash = domainsecurity.SHA256Hex([]byte("test-case-binding:\x00" + input.ThreadID + "\x00" + input.CaseID))
	}
	if input.DatasetSnapshotID == "" {
		input.DatasetSnapshotID = DatasetSnapshotID(input.ThreadID + "\x00" + input.TurnID + "\x00" + input.CaseBindingHash)
	}
	if input.SourceManifestHash == "" {
		input.SourceManifestHash = domainsecurity.SHA256Hex([]byte("test-source-manifest:\x00" + input.ThreadID + "\x00" + input.TurnID))
	}
	return executionContextV2(input, domainsecurity.RiskClassCase, domainsecurity.PublicationDispositionCaseEvidenceGate, domainsecurity.CaseBindingStateValid)
}

// BoundaryOnlyContextV2 returns a valid V2 case-protected context that cannot
// authorize provider, tool, evidence, or report effects. It models host
// failures before a concrete case binding or dataset snapshot can be frozen.
func BoundaryOnlyContextV2(input domainsecurity.TurnSecurityContextInput) (domainsecurity.TurnSecurityContext, error) {
	if input.ThreadID == "" || input.TurnID == "" || input.WorkspaceRealPath == "" {
		return domainsecurity.TurnSecurityContext{}, errors.New("test boundary context identity is incomplete")
	}
	if input.TenantID == "" {
		input.TenantID = domainsecurity.LocalTenantID
	}
	if input.UserID == "" {
		input.UserID = domainsecurity.LocalUserID
	}
	if input.ContextEpoch == 0 {
		input.ContextEpoch = 1
	}
	if input.IssuedAt.IsZero() {
		input.IssuedAt = time.Unix(1_700_000_000, 0).UTC()
	}
	input.CaseID = domainsecurity.UnboundCaseID
	input.CaseBindingHash = domainsecurity.UnboundCaseBindingHash(input.WorkspaceRealPath)
	input.DatasetSnapshotID = domainsecurity.NoDatasetSnapshotID
	input.SourceManifestHash = domainsecurity.EmptySourceManifestHash
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: domainsecurity.SHA256Hex([]byte("test-boundary-risk-policy:\x00" + input.ThreadID + "\x00" + input.WorkspaceRealPath)),
		RiskClass:              domainsecurity.RiskClassCase,
		Disposition:            domainsecurity.PublicationDispositionCaseBoundaryOnly,
		CaseBindingState:       domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte(
			"test-boundary-observation:\x00" + input.ThreadID + "\x00" + input.TurnID,
		)),
		BlockerCode: domainsecurity.PublicationBlockerCaseBindingMissing,
	})
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	input.PublicationPolicy = policy
	input.RiskAuthorityBinding = domainsecurity.NewQuarantinedRiskAuthorityBindingV1()
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(input)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	if err := domainsecurity.ValidateTurnSecurityContextForCasePublication(securityContext); err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	return securityContext, nil
}

func executionContextV2(input domainsecurity.TurnSecurityContextInput, riskClass, disposition, bindingState string) (domainsecurity.TurnSecurityContext, error) {
	if input.ThreadID == "" || input.TurnID == "" || input.WorkspaceRealPath == "" {
		return domainsecurity.TurnSecurityContext{}, errors.New("test execution context identity is incomplete")
	}
	if input.TenantID == "" {
		input.TenantID = domainsecurity.LocalTenantID
	}
	if input.UserID == "" {
		input.UserID = domainsecurity.LocalUserID
	}
	if input.ContextEpoch == 0 {
		input.ContextEpoch = 1
	}
	if input.IssuedAt.IsZero() {
		input.IssuedAt = time.Unix(1_700_000_000, 0).UTC()
	}
	policyDigest := domainsecurity.SHA256Hex([]byte("test-risk-policy:\x00" + input.ThreadID + "\x00" + input.WorkspaceRealPath + "\x00" + riskClass))
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: policyDigest,
		RiskClass:              riskClass,
		Disposition:            disposition,
		CaseBindingState:       bindingState,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte(
			"test-binding-observation:\x00" + input.ThreadID + "\x00" + input.TurnID + "\x00" + input.CaseBindingHash,
		)),
		BlockerCode: domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	binding, err := WitnessedRiskBinding(input.ThreadID, input.WorkspaceRealPath, riskClass, policyDigest)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	input.PublicationPolicy = policy
	input.RiskAuthorityBinding = binding
	context, err := domainsecurity.NewTurnSecurityContextV2(input)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	if err := domainsecurity.ValidateTurnSecurityContextForExecution(context); err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	return context, nil
}

func DatasetSnapshotID(material string) string {
	return domainsecurity.DatasetSnapshotIDPrefixV2 + domainsecurity.SHA256Hex([]byte("test-dataset-snapshot-v2:\x00"+material))
}

func WitnessedRiskBinding(threadID, workspace, riskClass, policyDigest string) (domainsecurity.RiskAuthorityBindingV1, error) {
	contracts, err := WitnessedRiskAuthorityContracts(threadID, workspace, riskClass, policyDigest)
	if err != nil {
		return domainsecurity.RiskAuthorityBindingV1{}, err
	}
	return contracts.Binding, nil
}

type RiskAuthorityContracts struct {
	Index       domainsecurity.ThreadRiskAuthorityIndexV1
	Request     domainsecurity.MonotonicHeadObserveRequestV1
	Observation domainsecurity.MonotonicHeadObservationV1
	Binding     domainsecurity.RiskAuthorityBindingV1
}

// WitnessedRiskAuthorityContracts returns a complete internally consistent
// issuance tuple for tests that exercise app ports. Production code must
// obtain the tuple from a fresh enrolled monotonic witness.
func WitnessedRiskAuthorityContracts(threadID, workspace, riskClass, policyDigest string) (RiskAuthorityContracts, error) {
	if threadID == "" || workspace == "" ||
		(riskClass != domainsecurity.RiskClassGeneral && riskClass != domainsecurity.RiskClassCase) ||
		!domainsecurity.IsSHA256Hex(policyDigest) {
		return RiskAuthorityContracts{}, errors.New("test risk binding input is invalid")
	}
	authorityPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x41}, ed25519.SeedSize))
	authorityPublic := authorityPrivate.Public().(ed25519.PublicKey)
	witnessPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x57}, ed25519.SeedSize))
	witnessPublic := witnessPrivate.Public().(ed25519.PublicKey)
	installationID := domainsecurity.SHA256Hex([]byte("analytix-test-installation"))
	enrollmentID := domainsecurity.SHA256Hex([]byte("analytix-test-risk-enrollment"))
	mutationID := domainsecurity.SHA256Hex([]byte("test-risk-mutation:\x00" + threadID + "\x00" + policyDigest))
	index, err := domainsecurity.NewThreadRiskAuthorityIndexV1(domainsecurity.ThreadRiskAuthorityIndexInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Namespace: domainsecurity.ThreadRiskAuthorityNamespaceV1,
		Generation: 1, Entries: []domainsecurity.ThreadRiskAuthorityEntryV1{{
			ThreadID: threadID, WorkspaceRealPath: workspace, RiskClass: riskClass, CurrentPolicyDigest: policyDigest,
		}}, MutationID: mutationID, AuthorityKeyID: domainsecurity.SHA256Hex(authorityPublic), AuthorityPublicKey: authorityPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(authorityPrivate, message), nil })
	if err != nil {
		return RiskAuthorityContracts{}, err
	}
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Namespace: domainsecurity.ThreadRiskAuthorityNamespaceV1,
		Generation: 1, CurrentStateDigest: index.IndexDigest,
		PreviousStateDigest:      domainsecurity.SHA256Hex([]byte("test-risk-genesis-state")),
		PreviousCheckpointDigest: domainsecurity.SHA256Hex([]byte("test-risk-genesis-checkpoint")),
		FenceNonce:               domainsecurity.SHA256Hex([]byte("test-risk-fence:\x00" + threadID)), MutationID: mutationID,
		WitnessKeyID: domainsecurity.SHA256Hex(witnessPublic), WitnessPublicKey: witnessPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(witnessPrivate, message), nil })
	if err != nil {
		return RiskAuthorityContracts{}, err
	}
	request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Namespace: domainsecurity.ThreadRiskAuthorityNamespaceV1,
		ChallengeNonce: domainsecurity.SHA256Hex([]byte("test-risk-challenge:\x00" + threadID + "\x00" + policyDigest)),
		AuthorityKeyID: domainsecurity.SHA256Hex(authorityPublic), AuthorityPublicKey: authorityPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(authorityPrivate, message), nil })
	if err != nil {
		return RiskAuthorityContracts{}, err
	}
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(
		request, checkpoint, func(message []byte) ([]byte, error) { return ed25519.Sign(witnessPrivate, message), nil },
	)
	if err != nil {
		return RiskAuthorityContracts{}, err
	}
	binding, err := domainsecurity.NewWitnessedRiskAuthorityBindingV1(index, request, observation)
	if err != nil {
		return RiskAuthorityContracts{}, err
	}
	return RiskAuthorityContracts{Index: index, Request: request, Observation: observation, Binding: binding}, nil
}
