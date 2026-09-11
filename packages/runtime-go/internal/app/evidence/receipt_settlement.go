package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	settlementport "analytix.local/runtime-go/internal/ports/evidencesettlement"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
)

type PreparedEvidenceAuthority struct {
	Record domainevidence.PreparedEvidenceSettlement
	Marker domainevidence.HostEvidenceSettlementMarker
}

type CommitEvidenceInput struct {
	Context         domainsecurity.TurnSecurityContext
	Marker          domainevidence.HostEvidenceSettlementMarker
	Authority       executiongrantapp.DurableSettlementAuthority
	CurrentProbe    domainsecurity.VerifiedSourceProbe
	SelectionDigest string
	HostAuthority   *domainevidence.PreparedEvidenceHostAuthorityV1
	HostCapability  sourceprobeport.HostEvidenceCapability
}

type Issuer struct {
	Registry                 registryport.Registry
	SettlementStore          settlementport.Store
	Authority                finalauthorityport.Authority
	Now                      func() time.Time
	beforeSettlementCommitV1 func(context.Context) error
}

func (issuer Issuer) Prepare(ctx context.Context, input IssueEvidenceInput) (PreparedEvidenceAuthority, error) {
	if issuer.Registry == nil || issuer.SettlementStore == nil || issuer.Authority == nil {
		return PreparedEvidenceAuthority{}, errors.New("evidence settlement authority is unavailable")
	}
	preparedAt := time.Now().UTC()
	if issuer.Now != nil {
		preparedAt = issuer.Now().UTC()
	}
	if err := validateEvidenceIssueAuthority(input, preparedAt); err != nil {
		return PreparedEvidenceAuthority{}, err
	}
	if input.ResultItemID != domaintoolresult.ToolResultItemIDV1(input.Context.TurnID, input.Grant.ToolCallID) {
		return PreparedEvidenceAuthority{}, errors.New("evidence settlement durable result identity is invalid")
	}
	canonicalEvidence, err := domainevidence.CanonicalEvidenceBytes(input.Material.CanonicalEvidence)
	if err != nil {
		return PreparedEvidenceAuthority{}, errors.New("canonical evidence material is invalid")
	}
	material, err := domainevidence.ParseCanonicalEvidenceMaterial(canonicalEvidence)
	if err != nil || (len(material.Facts) > 0) != (len(input.Material.SourceRecordIDs) > 0) {
		return PreparedEvidenceAuthority{}, errors.New("canonical evidence facts require exact source record lineage")
	}
	if err := validateEvidenceTransformation(input.RawResult, canonicalEvidence, input.Material.TransformationLineage); err != nil {
		return PreparedEvidenceAuthority{}, err
	}
	if material.SchemaVersion == domainevidence.CanonicalEvidenceVersionV2 {
		if err := domainevidence.ValidateSourceFieldBindingsAgainstRawResultV2(
			input.RawResult.RawResult, input.RawResult.RawSHA256, material,
		); err != nil {
			return PreparedEvidenceAuthority{}, err
		}
	}
	draftInput := evidenceReceiptDraftInput(input, canonicalEvidence, "pending", preparedAt)
	provisional, err := domainevidence.NewEvidenceReceiptDraft(draftInput)
	if err != nil {
		return PreparedEvidenceAuthority{}, err
	}
	if err := domainevidence.ValidateCanonicalEvidenceAgainstReceipt(provisional, canonicalEvidence); err != nil {
		return PreparedEvidenceAuthority{}, err
	}
	preparedInput := domainevidence.PreparedEvidenceSettlementInput{
		Context: input.Context, Grant: input.Grant, ActiveGrantRegistrySequence: input.GrantRegistry.Sequence,
		ActiveGrantRegistryDigest: input.GrantRegistry.StateDigest, SourceProbe: input.SourceProbe, ToolOutcome: input.Outcome,
		RawResult: input.RawResult.RawResult, CanonicalEvidence: canonicalEvidence, ReceiptDraft: provisional,
		QueryHash: input.Material.QueryHash, ResultItemID: input.ResultItemID, PreparedAt: preparedAt,
		AuthorityKeyID: issuer.Authority.KeyID(), AuthorityPublicKey: issuer.Authority.PublicKey(),
		HostAuthority: input.HostAuthority,
	}
	settlementID := domainevidence.ComputeEvidenceSettlementID(preparedInput)
	draftInput.ReceiptID = domainevidence.EvidenceSettlementReceiptID(settlementID)
	preparedInput.ReceiptDraft, err = domainevidence.NewEvidenceReceiptDraft(draftInput)
	if err != nil {
		return PreparedEvidenceAuthority{}, err
	}
	record, err := domainevidence.NewPreparedEvidenceSettlement(preparedInput, func(message []byte) ([]byte, error) {
		return issuer.Authority.Sign(ctx, message)
	})
	if err != nil {
		return PreparedEvidenceAuthority{}, err
	}
	if input.HostAuthority != nil {
		hostStore, ok := issuer.SettlementStore.(settlementport.HostAuthorityStore)
		if !ok || input.HostCapability == nil {
			return PreparedEvidenceAuthority{}, errors.New("host evidence settlement store authority is unavailable")
		}
		if err := hostStore.PutPreparedIfAbsentWithHostAuthority(ctx, record, settlementport.HostAuthorityInput{
			Context: input.Context, Binding: input.HostAuthority.Binding, CurrentProbe: input.SourceProbe,
			SelectionDigest: input.HostAuthority.SelectionDigest, Capability: input.HostCapability,
		}); err != nil {
			return PreparedEvidenceAuthority{}, err
		}
	} else if err := issuer.SettlementStore.PutPreparedIfAbsent(ctx, record); err != nil {
		return PreparedEvidenceAuthority{}, err
	}
	written, err := issuer.SettlementStore.ResolvePrepared(ctx, record.SettlementID)
	if err != nil || written.RecordDigest != record.RecordDigest {
		return PreparedEvidenceAuthority{}, errors.New("prepared evidence settlement private readback failed")
	}
	keyID, publicKey, signature, err := domainevidence.EvidenceSettlementAuthorityMaterial(written)
	if err != nil || issuer.Authority.VerifyTrusted(ctx, keyID, publicKey, domainevidence.EvidenceSettlementSigningBytes(written), signature) != nil {
		return PreparedEvidenceAuthority{}, errors.New("prepared evidence settlement private authority is untrusted")
	}
	marker, err := domainevidence.NewHostEvidenceSettlementMarker(written)
	if err != nil {
		return PreparedEvidenceAuthority{}, err
	}
	return PreparedEvidenceAuthority{Record: written, Marker: marker}, nil
}

func (issuer Issuer) Commit(ctx context.Context, input CommitEvidenceInput) (domainevidence.EvidenceReceipt, error) {
	prepared, err := issuer.validateCommitAuthority(ctx, input)
	if err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	registry, err := issuer.Registry.Replay(ctx, input.Context)
	if err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	if existing, found, resolveErr := domainevidence.ResolvePreparedSettlementIssue(registry, prepared); resolveErr != nil {
		return domainevidence.EvidenceReceipt{}, resolveErr
	} else if found {
		return existing, nil
	}
	if issuer.beforeSettlementCommitV1 != nil {
		if err := issuer.beforeSettlementCommitV1(ctx); err != nil {
			return domainevidence.EvidenceReceipt{}, err
		}
	}
	receipt, err := issuer.Registry.CommitPrepared(ctx, registryport.CommitPreparedInput{
		Context: input.Context, Draft: prepared.ReceiptDraft, CanonicalEvidence: prepared.CanonicalEvidence,
		SettlementProof: domainevidence.EvidenceSettlementProof{
			SettlementID: prepared.SettlementID, PreparedRecordDigest: prepared.RecordDigest,
		}, RegisteredAt: input.Authority.SettledAt,
	})
	if err != nil {
		// A committed append may report a post-write sync failure. Exact replay
		// readback is authoritative and prevents duplicate recovery issuance.
		registry, replayErr := issuer.Registry.Replay(ctx, input.Context)
		if replayErr != nil {
			return domainevidence.EvidenceReceipt{}, err
		}
		existing, found, resolveErr := domainevidence.ResolvePreparedSettlementIssue(registry, prepared)
		if resolveErr != nil || !found {
			return domainevidence.EvidenceReceipt{}, err
		}
		return existing, nil
	}
	registry, err = issuer.Registry.Replay(ctx, input.Context)
	if err != nil {
		return domainevidence.EvidenceReceipt{}, errors.New("evidence settlement registry readback failed")
	}
	resolved, found, err := domainevidence.ResolvePreparedSettlementIssue(registry, prepared)
	if err != nil || !found || resolved.ReceiptID != receipt.ReceiptID {
		return domainevidence.EvidenceReceipt{}, errors.New("evidence settlement registry readback failed")
	}
	return resolved, nil
}

func (issuer Issuer) validateCommitAuthority(ctx context.Context, input CommitEvidenceInput) (domainevidence.PreparedEvidenceSettlement, error) {
	if issuer.Registry == nil || issuer.SettlementStore == nil || issuer.Authority == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil ||
		domainevidence.ValidateHostEvidenceSettlementMarker(input.Marker) != nil {
		return domainevidence.PreparedEvidenceSettlement{}, errors.New("evidence settlement commit authority is unavailable")
	}
	prepared, err := issuer.SettlementStore.ResolvePrepared(ctx, input.Marker.SettlementID)
	if err != nil || prepared.RecordDigest != input.Marker.PreparedRecordDigest || prepared.ReceiptID != input.Marker.ReceiptID {
		return domainevidence.PreparedEvidenceSettlement{}, errors.New("evidence settlement prepared authority is missing")
	}
	if prepared.HostAuthority != nil {
		selection, selectionErr := validatePreparedHostAuthoritySelection(prepared, input.Context, input.CurrentProbe, input.HostCapability)
		if input.HostCapability == nil || input.HostAuthority == nil ||
			*input.HostAuthority != *prepared.HostAuthority ||
			selectionErr != nil || selection.SelectionDigest != input.SelectionDigest ||
			validateHostAuthorityBinding(prepared.HostAuthority.Binding, input.Context) != nil ||
			validateHostAuthoritySelection(input.Context, input.CurrentProbe, input.SelectionDigest, input.HostCapability) != nil {
			return domainevidence.PreparedEvidenceSettlement{}, errors.New("evidence settlement host authority is unavailable")
		}
	} else {
		if !domainsecurity.SourceProbeCanAuthorizeFacts(prepared.SourceProbe) {
			return domainevidence.PreparedEvidenceSettlement{}, errors.New("evidence settlement snapshot authority is unavailable")
		}
		if domainevidence.ValidatePreparedEvidenceSettlementForExecution(prepared) != nil {
			return domainevidence.PreparedEvidenceSettlement{}, errors.New("evidence settlement current execution authority is unavailable")
		}
	}
	keyID, publicKey, signature, err := domainevidence.EvidenceSettlementAuthorityMaterial(prepared)
	if err != nil || issuer.Authority.VerifyTrusted(ctx, keyID, publicKey, domainevidence.EvidenceSettlementSigningBytes(prepared), signature) != nil {
		return domainevidence.PreparedEvidenceSettlement{}, errors.New("evidence settlement prepared authority is untrusted")
	}
	if err := validateEvidenceSettlementDurableAuthorityV1(prepared, input); err != nil {
		return domainevidence.PreparedEvidenceSettlement{}, err
	}
	return prepared, nil
}

// This proves a stored prefix and result, without granting current issuance.
func validateEvidenceSettlementDurableAuthorityV1(prepared domainevidence.PreparedEvidenceSettlement, input CommitEvidenceInput) error {
	expectedMarker, err := domainevidence.NewHostEvidenceSettlementMarker(prepared)
	if err != nil || expectedMarker != input.Marker || prepared.SecurityContext.ContextDigest != input.Context.ContextDigest {
		return errors.New("evidence settlement marker or context is mismatched")
	}
	authority := input.Authority
	if domainsecurity.ValidateExecutionGrantRegistry(authority.ActiveRegistry) != nil ||
		domainsecurity.ValidateExecutionGrantRegistry(authority.SettledRegistry) != nil ||
		authority.ActiveRegistry.Sequence != prepared.ActiveGrantRegistrySequence ||
		authority.ActiveRegistry.StateDigest != prepared.ActiveGrantRegistryDigest ||
		domainsecurity.VerifyExecutionGrantMembership(authority.ActiveRegistry, input.Context.ThreadID, input.Context.TurnID, prepared.ExecutionGrant, domainsecurity.GrantRegistryActive) != nil ||
		domainsecurity.VerifyExecutionGrantMembership(authority.SettledRegistry, input.Context.ThreadID, input.Context.TurnID, prepared.ExecutionGrant, domainsecurity.GrantRegistrySettled) != nil {
		return errors.New("evidence settlement grant prefix is invalid")
	}
	return validateEvidenceSettlementResultV1(prepared, input.Context, input.Marker, authority.ResultItem, authority.SettledAt)
}

func validateEvidenceSettlementResultV1(prepared domainevidence.PreparedEvidenceSettlement, securityContext domainsecurity.TurnSecurityContext, expectedMarker domainevidence.HostEvidenceSettlementMarker, item map[string]any, settledAt time.Time) error {
	for _, key := range []string{"id", "kind", "status", "role", "threadId", "turnId", "contextDigest", "executionGrantId", "toolName", "callId", "createdAt", "finishedAt"} {
		value, ok := item[key].(string)
		if !ok || value == "" || value != strings.TrimSpace(value) {
			return errors.New("evidence settlement durable result field is noncanonical")
		}
	}
	if isError, ok := item["isError"].(bool); !ok || isError {
		return errors.New("evidence settlement durable result error flag is invalid")
	}
	contextEpoch, epochOK := settlementUint64Field(item, "contextEpoch")
	marker, markerErr := domainevidence.ParseHostEvidenceSettlementMarker(item["hostEvidenceSettlement"])
	if markerErr != nil || marker != expectedMarker || settlementStringField(item, "id") != prepared.ResultItemID ||
		settlementStringField(item, "kind") != "tool_result" || settlementStringField(item, "status") != "completed" ||
		settlementStringField(item, "role") != "tool" || !epochOK || contextEpoch != securityContext.ContextEpoch ||
		settlementStringField(item, "threadId") != securityContext.ThreadID || settlementStringField(item, "turnId") != securityContext.TurnID ||
		settlementStringField(item, "contextDigest") != securityContext.ContextDigest ||
		settlementStringField(item, "executionGrantId") != prepared.ExecutionGrant.GrantID ||
		settlementStringField(item, "toolName") != prepared.ExecutionGrant.ToolName ||
		settlementStringField(item, "callId") != prepared.ExecutionGrant.ToolCallID || settlementBoolField(item, "isError") {
		return errors.New("evidence settlement durable result marker is invalid")
	}
	createdAt, createdErr := time.Parse(time.RFC3339Nano, settlementStringField(item, "createdAt"))
	finishedAt, err := time.Parse(time.RFC3339Nano, settlementStringField(item, "finishedAt"))
	preparedAt, preparedErr := time.Parse(time.RFC3339Nano, prepared.PreparedAt)
	if err != nil || createdErr != nil || preparedErr != nil || createdAt.After(finishedAt) ||
		!finishedAt.Equal(settledAt) || finishedAt.Before(preparedAt) {
		return errors.New("evidence settlement durable result time is invalid")
	}
	return nil
}

func evidenceReceiptDraftInput(input IssueEvidenceInput, canonicalEvidence []byte, receiptID string, issuedAt time.Time) domainevidence.EvidenceReceiptInput {
	return domainevidence.EvidenceReceiptInput{
		ReceiptID: receiptID, Context: input.Context, ExecutionGrantID: input.Grant.GrantID, ToolCallID: input.Grant.ToolCallID,
		ServerIdentity: input.SourceProbe.ServerIdentity, ServerVersion: input.Material.ServerVersion,
		ConnectionEpoch: input.SourceProbe.ConnectionEpoch, ToolName: input.Grant.ToolName, ArgsHash: input.Grant.ArgsHash,
		ResultHash: domainsecurity.CanonicalJSONHash(canonicalEvidence), SourceType: input.Material.SourceType,
		DatasetSnapshotID: input.Context.DatasetSnapshotID, QueryHash: input.Material.QueryHash, QueryRange: input.Material.QueryRange,
		Granularity: input.Material.Granularity, Currency: input.Material.Currency, Timezone: input.Material.Timezone,
		PaginationCompleteness: input.Material.PaginationCompleteness, SourceRecordIDs: input.Material.SourceRecordIDs,
		RawSHA256: input.RawResult.RawSHA256, TransformationLineage: input.Material.TransformationLineage,
		PIIClassification: input.Material.PIIClassification, IssuedAt: issuedAt,
	}
}

func settlementStringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}

func settlementBoolField(record map[string]any, key string) bool {
	value, _ := record[key].(bool)
	return value
}

func settlementUint64Field(record map[string]any, key string) (uint64, bool) {
	switch value := record[key].(type) {
	case uint64:
		return value, true
	case uint:
		return uint64(value), true
	case int:
		return uint64(value), value >= 0
	case int64:
		return uint64(value), value >= 0
	case float64:
		parsed := uint64(value)
		return parsed, value >= 0 && value == float64(parsed)
	case json.Number:
		parsed, err := value.Int64()
		return uint64(parsed), err == nil && parsed >= 0
	default:
		return 0, false
	}
}

func settlementMarkerRecord(marker domainevidence.HostEvidenceSettlementMarker) map[string]any {
	body, _ := json.Marshal(marker)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}
