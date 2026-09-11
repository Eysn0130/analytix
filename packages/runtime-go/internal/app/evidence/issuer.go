package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmcpname "analytix.local/runtime-go/internal/domain/mcpname"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
)

type VerifiedEvidenceMaterial struct {
	CanonicalEvidence      json.RawMessage
	SourceType             string
	ServerVersion          string
	QueryHash              string
	QueryRange             domainevidence.EvidenceQueryRange
	Granularity            string
	Currency               string
	Timezone               string
	PaginationCompleteness domainevidence.PaginationCompleteness
	SourceRecordIDs        []string
	TransformationLineage  []domainevidence.TransformationLineageStep
	PIIClassification      domainevidence.PIIClassification
}

type IssueEvidenceInput struct {
	Context        domainsecurity.TurnSecurityContext
	Grant          domainsecurity.ExecutionGrant
	GrantRegistry  domainsecurity.ExecutionGrantRegistry
	Outcome        domainevidence.ToolOutcome
	SourceProbe    domainsecurity.VerifiedSourceProbe
	RawResult      domainmcp.LosslessToolResult
	Material       VerifiedEvidenceMaterial
	ResultItemID   string
	IssuedAt       time.Time
	HostAuthority  *domainevidence.PreparedEvidenceHostAuthorityV1
	HostCapability sourceprobeport.HostEvidenceCapability
}

func ResolveCurrentEvidence(ctx context.Context, registry registryport.Registry, securityContext domainsecurity.TurnSecurityContext, receiptIDs []string) ([]domainevidence.RegisteredEvidence, error) {
	if registry == nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil || len(receiptIDs) == 0 {
		return nil, errors.New("current evidence registry authority is unavailable")
	}
	seen := map[string]bool{}
	resolved := make([]domainevidence.RegisteredEvidence, 0, len(receiptIDs))
	for _, receiptID := range receiptIDs {
		receiptID = strings.TrimSpace(receiptID)
		if receiptID == "" || seen[receiptID] {
			return nil, errors.New("evidence receipt reference is invalid")
		}
		seen[receiptID] = true
		registered, err := registry.Resolve(ctx, registryport.MembershipQuery{Context: securityContext, ReceiptID: receiptID})
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, registered)
	}
	return resolved, nil
}

func validateEvidenceIssueAuthority(input IssueEvidenceInput, issuedAt time.Time) error {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil ||
		domainsecurity.ValidateExecutionGrantForContext(input.Grant, input.Context) != nil ||
		!domainmodel.IsHostToolCallIDV1(input.Grant.ToolCallID) ||
		domainevidence.ValidateToolOutcome(input.Outcome) != nil || domainsecurity.ValidateVerifiedSourceProbe(input.SourceProbe) != nil ||
		domainsecurity.ValidateExecutionGrantRegistry(input.GrantRegistry) != nil {
		return errors.New("evidence issuance authority is invalid")
	}
	if input.Grant.ToolName == fundsAccountFlowCanonicalTool {
		if input.HostAuthority == nil || input.HostCapability == nil ||
			!domainsecurity.SourceProbeEligibleForHostAuthorityV2(input.SourceProbe) ||
			validateHostAuthorityBinding(input.HostAuthority.Binding, input.Context) != nil ||
			validateHostAuthoritySelection(input.Context, input.SourceProbe, input.HostAuthority.SelectionDigest, input.HostCapability) != nil ||
			validateNewHostAuthorityContent(input.HostAuthority.SelectionContentDigest, input.HostCapability) != nil {
			return errors.New("account-flow host evidence authority is invalid")
		}
	} else if !domainsecurity.SourceProbeCanAuthorizeFacts(input.SourceProbe) {
		return errors.New("evidence issuance source probe cannot authorize facts")
	}
	if err := domainsecurity.VerifyExecutionGrantMembership(input.GrantRegistry, input.Context.ThreadID, input.Context.TurnID, input.Grant, domainsecurity.GrantRegistryActive); err != nil {
		return errors.New("evidence issuance grant is not active in the host registry")
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, input.Grant.ExpiresAt)
	if err != nil || !issuedAt.Before(expiresAt) {
		return errors.New("evidence issuance grant is expired")
	}
	if input.Grant.ContextDigest != input.Context.ContextDigest || input.Grant.TurnID != input.Context.TurnID ||
		!input.Grant.ReadOnly ||
		input.ResultItemID != domaintoolresult.ToolResultItemIDV1(input.Context.TurnID, input.Grant.ToolCallID) ||
		input.Outcome.ContextDigest != input.Context.ContextDigest || input.Outcome.ExecutionGrantID != input.Grant.GrantID ||
		input.Outcome.ToolName != input.Grant.ToolName || input.Outcome.ToolCallID != input.Grant.ToolCallID ||
		input.Outcome.CaseID != input.Context.CaseID || input.Outcome.ContextEpoch != input.Context.ContextEpoch ||
		input.Outcome.DatasetSnapshotID != input.Context.DatasetSnapshotID || input.Outcome.ServerIdentity != input.Grant.ServerIdentity {
		return errors.New("evidence issuance context or outcome is mismatched")
	}
	if input.Outcome.TransportStatus != domainevidence.TransportSuccess || input.Outcome.IsError ||
		strings.TrimSpace(input.Outcome.Blocker) != "" ||
		(input.Outcome.SemanticStatus != domainevidence.SemanticSuccess && input.Outcome.SemanticStatus != domainevidence.SemanticPartial) {
		return errors.New("evidence issuance requires successful transport and usable semantic status")
	}
	if input.Material.PaginationCompleteness == domainevidence.PaginationPartial && input.Outcome.SemanticStatus != domainevidence.SemanticPartial {
		return errors.New("partial pagination requires a partial tool outcome")
	}
	probe := input.SourceProbe
	if probe.ThreadID != input.Context.ThreadID || probe.TurnID != input.Context.TurnID || probe.ContextEpoch != input.Context.ContextEpoch ||
		probe.ProbeContextDigest != input.Context.ContextDigest || probe.CaseID != input.Context.CaseID ||
		probe.CaseBindingHash != input.Context.CaseBindingHash || probe.DatasetSnapshotID != input.Context.DatasetSnapshotID ||
		probe.ConnectionEpoch != input.Grant.ConnectionEpoch || !probe.ReadOnly {
		return errors.New("evidence issuance source probe is not bound to the frozen turn")
	}
	serverID := mcpServerID(input.Grant.ToolName)
	identity, identityErr := domainsecurity.ParseVerifiedMCPServerIdentity(input.Grant.ServerIdentity)
	if serverID == "" || identityErr != nil || !domainsecurity.VerifiedMCPServerIdentityCanAuthorizeFacts(identity) || identity.ServerID != serverID || identity.ConnectionEpoch != input.Grant.ConnectionEpoch ||
		serverID != probe.ServerID || input.Grant.ServerIdentity != probe.ServerIdentity || input.Outcome.ServerIdentity != probe.ServerIdentity {
		return errors.New("evidence issuance server identity is mismatched")
	}
	version := strings.TrimSpace(input.Material.ServerVersion)
	if version == "" || identity.ObservedVersion != version {
		return errors.New("evidence issuance server version is unverified")
	}
	if !domainmcp.ValidLosslessToolResult(input.RawResult) || domainsecurity.SHA256Hex(input.RawResult.RawResult) != input.RawResult.RawSHA256 {
		return errors.New("evidence issuance raw result integrity is invalid")
	}
	if err := domainjsonstrict.Validate(input.RawResult.RawResult, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 4 * 1024 * 1024, MaxTokens: 200_000, MaxStringBytes: 1024 * 1024,
	}); err != nil {
		return errors.New("evidence issuance raw result is not strict JSON")
	}
	if err := domainjsonstrict.Validate(input.Material.CanonicalEvidence, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 4 * 1024 * 1024, MaxTokens: 200_000, MaxStringBytes: 1024 * 1024,
	}); err != nil {
		return errors.New("evidence issuance canonical material is not strict JSON")
	}
	if len(input.Material.CanonicalEvidence) == 0 || strings.TrimSpace(input.Material.SourceType) == "" || strings.TrimSpace(input.Material.QueryHash) == "" {
		return errors.New("evidence issuance material is incomplete")
	}
	return nil
}

func validateHostAuthorityBinding(binding domainsecurity.CaseBindingObservationV1, context domainsecurity.TurnSecurityContext) error {
	if domainsecurity.ValidateCaseBindingObservationV1(binding) != nil || binding.State != domainsecurity.CaseBindingStateValid ||
		binding.WorkspaceRealPath != context.WorkspaceRealPath || binding.CaseID != context.CaseID ||
		binding.CaseBindingHash != context.CaseBindingHash {
		return errors.New("host evidence case binding is invalid")
	}
	return nil
}

func validateHostAuthoritySelection(
	context domainsecurity.TurnSecurityContext,
	probe domainsecurity.VerifiedSourceProbe,
	selectionDigest string,
	capability sourceprobeport.HostEvidenceCapability,
) error {
	if capability == nil || !domainsecurity.SourceProbeEligibleForHostAuthorityV2(probe) ||
		probe.DatasetSnapshotID != context.DatasetSnapshotID || !domainsecurity.IsSHA256Hex(selectionDigest) {
		return errors.New("host evidence selection authority is invalid")
	}
	selection, err := capability.DatasetSelection()
	if err != nil || datasetsnapshotport.ValidateCurrentSelectionDigestV2(selection) != nil || selection.SelectionDigest != selectionDigest ||
		selection.Snapshot.Record.DatasetSnapshotID != context.DatasetSnapshotID ||
		selection.Snapshot.Record.Binding.TenantID != context.TenantID ||
		selection.Snapshot.Record.Binding.UserID != context.UserID ||
		selection.Snapshot.Record.Binding.WorkspaceRealPath != context.WorkspaceRealPath ||
		selection.Snapshot.Record.Binding.CaseID != context.CaseID ||
		selection.Snapshot.Record.Binding.CaseBindingHash != context.CaseBindingHash ||
		selection.Snapshot.Manifest.Binding.TenantID != context.TenantID ||
		selection.Snapshot.Manifest.Binding.UserID != context.UserID ||
		selection.Snapshot.Manifest.Binding.WorkspaceRealPath != context.WorkspaceRealPath ||
		selection.Snapshot.Manifest.Binding.CaseID != context.CaseID ||
		selection.Snapshot.Manifest.Binding.CaseBindingHash != context.CaseBindingHash ||
		selection.Snapshot.Manifest.SourceManifestHash != context.SourceManifestHash {
		return errors.New("host evidence selection authority is stale")
	}
	return nil
}

func validateNewHostAuthorityContent(contentDigest string, capability sourceprobeport.HostEvidenceCapability) error {
	if capability == nil || !domainsecurity.IsSHA256Hex(contentDigest) {
		return errors.New("new host evidence content binding is unavailable")
	}
	selection, err := capability.DatasetSelection()
	if err != nil {
		return err
	}
	expected, err := datasetsnapshotport.CanonicalCurrentSelectionContentDigestV2(selection)
	if err != nil || expected != contentDigest {
		return errors.New("new host evidence content binding is invalid")
	}
	return nil
}

func validatePreparedHostAuthoritySelection(
	prepared domainevidence.PreparedEvidenceSettlement,
	context domainsecurity.TurnSecurityContext,
	probe domainsecurity.VerifiedSourceProbe,
	capability sourceprobeport.HostEvidenceCapability,
) (datasetsnapshotport.CurrentSelectionV2, error) {
	if capability == nil {
		return datasetsnapshotport.CurrentSelectionV2{}, errors.New("host evidence current capability is unavailable")
	}
	selection, err := capability.DatasetSelection()
	if err != nil || validateHostAuthoritySelection(context, probe, selection.SelectionDigest, capability) != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, errors.New("host evidence current selection is unavailable")
	}
	contentDigest, err := datasetsnapshotport.CanonicalCurrentSelectionContentDigestV2(selection)
	if err != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, err
	}
	if err := domainevidence.ValidatePreparedEvidenceSettlementForCurrentHostAuthorityV2(
		prepared, context, probe, selection.SelectionDigest, contentDigest,
	); err != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, errors.Join(errors.New("host evidence prepared content or context has changed"), err)
	}
	return selection, nil
}

func mcpServerID(toolName string) string {
	serverID, _, ok := domainmcpname.Parse(toolName)
	if !ok {
		return ""
	}
	return serverID
}

func validateEvidenceTransformation(raw domainmcp.LosslessToolResult, canonicalEvidence []byte, lineage []domainevidence.TransformationLineageStep) error {
	resultHash := domainsecurity.CanonicalJSONHash(canonicalEvidence)
	if domainsecurity.CanonicalJSONHash(raw.RawResult) == resultHash && len(lineage) == 0 {
		return nil
	}
	if len(lineage) == 0 || lineage[0].InputHash != raw.RawSHA256 || lineage[len(lineage)-1].OutputHash != resultHash {
		return errors.New("evidence transformation lineage does not bind raw and canonical material")
	}
	for index := 1; index < len(lineage); index++ {
		if lineage[index].InputHash != lineage[index-1].OutputHash {
			return errors.New("evidence transformation lineage chain is broken")
		}
	}
	return nil
}
