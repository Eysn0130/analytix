package pendingwork

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	toolBatchPayloadPurpose            = "tool_batch.semantic_request"
	providerContinuationPayloadPurpose = "provider_continuation.semantic_request"
	toolBatchRouteIdentity             = "analytix/pending-work-route/tool-batch/v1"
)

type ProviderContinuationIssueInput struct {
	SecurityContext  domainsecurity.TurnSecurityContext
	GrantReferences  []GrantReference
	RegistryDigest   string
	CanonicalPayload []byte
	RouteHash        string
	IssuedAt         time.Time
	ExpiresAt        time.Time
}

type ToolBatchEffectGate interface {
	AcquireEffect(context.Context, domainsecurity.TurnSecurityContext) (context.Context, func(), error)
	AcquireOrdinaryEffect(context.Context, domainsecurity.TurnSecurityContext) (context.Context, func(), error)
	AcquireTransition(context.Context, domainsecurity.TurnSecurityContext) (func(), error)
}

// WithOpenToolBatch holds the same host effect lease used by context
// acceptance and batch disposition from exact-open through the executor's
// return. A transition/close either wins before exact-open or waits for the
// already-current batch; it cannot make a stale batch start in between.
func (service *Service) WithOpenToolBatch(ctx context.Context, gate ToolBatchEffectGate, calls []appmodel.PendingToolCall, execute func(context.Context) error) (domainpendingwork.PendingWorkReceiptV1, error) {
	securityContext, usesCaseDataAuthority, _, _, _, err := toolBatchAuthority(calls)
	if err != nil || gate == nil || execute == nil {
		return domainpendingwork.PendingWorkReceiptV1{}, ErrGrantAuthority
	}
	acquire := gate.AcquireOrdinaryEffect
	if usesCaseDataAuthority {
		acquire = gate.AcquireEffect
	}
	effectCtx, release, err := acquire(ctx, securityContext)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	defer release()
	issuedAt := service.boundaryTime()
	receipt, err := service.IssueToolBatch(effectCtx, calls, issuedAt)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	return receipt, execute(effectCtx)
}

func (service *Service) CompleteToolBatchWithGate(ctx context.Context, gate ToolBatchEffectGate, workID string, securityContext domainsecurity.TurnSecurityContext, calls []appmodel.PendingToolCall, status, reasonCode string) (domainpendingwork.PendingWorkDispositionV1, error) {
	derivedContext, _, _, _, _, authorityErr := toolBatchAuthority(calls)
	if gate == nil || authorityErr != nil || derivedContext != securityContext {
		return domainpendingwork.PendingWorkDispositionV1{}, ErrAuthorityUnavailable
	}
	release, err := gate.AcquireTransition(ctx, securityContext)
	if err != nil {
		return domainpendingwork.PendingWorkDispositionV1{}, err
	}
	defer release()
	disposedAt := service.boundaryTime()
	return service.CompleteToolBatch(ctx, workID, securityContext, calls, status, reasonCode, disposedAt)
}

// IssueToolBatch is the server-gravity boundary immediately before parallel
// read-only calls start. It persists the authority and then exact-opens the
// just-persisted receipt against the current context, registry, grants,
// operation payload, and expiry as its final action before returning.
func (service *Service) IssueToolBatch(ctx context.Context, calls []appmodel.PendingToolCall, issuedAt time.Time) (domainpendingwork.PendingWorkReceiptV1, error) {
	securityContext, _, references, canonicalPayload, expiresAt, err := toolBatchAuthority(calls)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	payloadHash, err := service.KeyedPayloadHash(ctx, toolBatchPayloadPurpose, canonicalPayload)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	receipt, err := service.Issue(ctx, IssueInput{
		Kind: domainpendingwork.KindToolBatch, SecurityContext: securityContext, GrantReferences: references,
		PayloadHash: payloadHash, RouteHash: domainsecurity.SHA256Hex([]byte(toolBatchRouteIdentity)), IssuedAt: issuedAt, ExpiresAt: expiresAt,
	})
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	return service.VerifyToolBatchAtStart(ctx, receipt.WorkID, securityContext, calls, issuedAt)
}

// VerifyToolBatchAtStart is the exact-open effect boundary for an already
// issued batch. A closed, expired, stale-context, changed-registry, changed
// grant, reordered-call, or changed-argument receipt fails before execution.
func (service *Service) VerifyToolBatchAtStart(ctx context.Context, workID string, securityContext domainsecurity.TurnSecurityContext, calls []appmodel.PendingToolCall, now time.Time) (domainpendingwork.PendingWorkReceiptV1, error) {
	derivedContext, _, references, canonicalPayload, _, err := toolBatchAuthority(calls)
	if err != nil || derivedContext != securityContext {
		return domainpendingwork.PendingWorkReceiptV1{}, ErrCurrentContext
	}
	payloadHash, err := service.KeyedPayloadHash(ctx, toolBatchPayloadPurpose, canonicalPayload)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	receipt, err := service.VerifyOpenFor(
		ctx, workID, securityContext, domainpendingwork.KindToolBatch, payloadHash,
		domainsecurity.SHA256Hex([]byte(toolBatchRouteIdentity)), now,
	)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	if !providerReferencesMatch(receipt.GrantMembers, references) {
		return domainpendingwork.PendingWorkReceiptV1{}, ErrOperationMismatch
	}
	return receipt, nil
}

// CompleteToolBatch recomputes the exact private batch identity before close;
// passing a different set/order/argument payload or another work id fails.
func (service *Service) CompleteToolBatch(ctx context.Context, workID string, securityContext domainsecurity.TurnSecurityContext, calls []appmodel.PendingToolCall, status, reasonCode string, disposedAt time.Time) (domainpendingwork.PendingWorkDispositionV1, error) {
	derivedContext, _, _, canonicalPayload, _, err := toolBatchAuthority(calls)
	if err != nil || derivedContext != securityContext {
		return domainpendingwork.PendingWorkDispositionV1{}, ErrCurrentContext
	}
	payloadHash, err := service.KeyedPayloadHash(ctx, toolBatchPayloadPurpose, canonicalPayload)
	if err != nil {
		return domainpendingwork.PendingWorkDispositionV1{}, err
	}
	receipt, err := service.openReceipt(ctx, workID)
	if err != nil {
		return domainpendingwork.PendingWorkDispositionV1{}, err
	}
	if receipt.Kind != domainpendingwork.KindToolBatch || receipt.PayloadHash != payloadHash || receipt.RouteHash != domainsecurity.SHA256Hex([]byte(toolBatchRouteIdentity)) {
		return domainpendingwork.PendingWorkDispositionV1{}, ErrOperationMismatch
	}
	return service.Close(ctx, workID, securityContext, status, reasonCode, disposedAt)
}

func (service *Service) IssueProviderContinuation(ctx context.Context, input ProviderContinuationIssueInput) (domainpendingwork.PendingWorkReceiptV1, error) {
	if _, _, err := service.currentAuthority(input.SecurityContext); err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	payloadHash, err := service.KeyedPayloadHash(ctx, providerContinuationPayloadPurpose, input.CanonicalPayload)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	return service.Issue(ctx, IssueInput{
		Kind: domainpendingwork.KindProviderContinuation, SecurityContext: input.SecurityContext, GrantReferences: input.GrantReferences,
		ExpectedRegistryDigest: input.RegistryDigest,
		PayloadHash:            payloadHash, RouteHash: input.RouteHash, IssuedAt: input.IssuedAt, ExpiresAt: input.ExpiresAt,
	})
}

func (service *Service) VerifyProviderContinuation(ctx context.Context, workID string, input ProviderContinuationIssueInput, now time.Time) (domainpendingwork.PendingWorkReceiptV1, error) {
	if _, _, err := service.currentAuthority(input.SecurityContext); err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	payloadHash, err := service.KeyedPayloadHash(ctx, providerContinuationPayloadPurpose, input.CanonicalPayload)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	receipt, err := service.VerifyOpenFor(ctx, workID, input.SecurityContext, domainpendingwork.KindProviderContinuation, payloadHash, input.RouteHash, now)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	if !providerReferencesMatch(receipt.GrantMembers, input.GrantReferences) {
		return domainpendingwork.PendingWorkReceiptV1{}, ErrOperationMismatch
	}
	return receipt, nil
}

func providerReferencesMatch(members []domainpendingwork.GrantMemberV1, references []GrantReference) bool {
	if len(members) != len(references) {
		return false
	}
	type pair struct{ grantID, resultItemID string }
	expected := make([]pair, len(members))
	actual := make([]pair, len(references))
	for index, member := range members {
		expected[index] = pair{member.GrantID, member.ResultItemID}
	}
	for index, reference := range references {
		actual[index] = pair{strings.TrimSpace(reference.GrantID), strings.TrimSpace(reference.ResultItemID)}
	}
	sortPairs := func(values []pair) {
		sort.Slice(values, func(i, j int) bool {
			if values[i].grantID == values[j].grantID {
				return values[i].resultItemID < values[j].resultItemID
			}
			return values[i].grantID < values[j].grantID
		})
	}
	sortPairs(expected)
	sortPairs(actual)
	for index := range expected {
		if expected[index] != actual[index] {
			return false
		}
	}
	return true
}

func toolBatchAuthority(calls []appmodel.PendingToolCall) (domainsecurity.TurnSecurityContext, bool, []GrantReference, []byte, time.Time, error) {
	if len(calls) == 0 || len(calls) > 512 {
		return domainsecurity.TurnSecurityContext{}, false, nil, nil, time.Time{}, errors.New("pending tool batch member count is invalid")
	}
	type payloadCall struct {
		GrantID    string `json:"grantId"`
		ToolCallID string `json:"toolCallId"`
		ToolName   string `json:"toolName"`
		Arguments  any    `json:"arguments"`
	}
	payload := struct {
		SchemaVersion int           `json:"schemaVersion"`
		Kind          string        `json:"kind"`
		Calls         []payloadCall `json:"calls"`
	}{SchemaVersion: 1, Kind: domainpendingwork.KindToolBatch, Calls: make([]payloadCall, 0, len(calls))}
	securityContext := calls[0].SecurityContext
	if domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(securityContext) != nil {
		return domainsecurity.TurnSecurityContext{}, false, nil, nil, time.Time{}, ErrCurrentContext
	}
	references := make([]GrantReference, 0, len(calls))
	seen := map[string]bool{}
	var expiresAt time.Time
	usesCaseDataAuthority := false
	effectClassified := false
	for _, pending := range calls {
		grant := pending.ExecutionGrant
		if pending.SecurityContext != securityContext || pending.ThreadID != securityContext.ThreadID || pending.TurnID != securityContext.TurnID ||
			executiongrantapp.ValidateExecutionGrantForCall(securityContext, grant, pending.Call) != nil || grant.ContextDigest != securityContext.ContextDigest || grant.TurnID != securityContext.TurnID ||
			grant.ToolCallID != strings.TrimSpace(pending.Call.ID) || grant.ToolName != strings.TrimSpace(pending.Call.Name) ||
			grant.ArgsHash != domainsecurity.CanonicalJSONHash(pending.Call.Arguments) || !grant.ReadOnly || seen[grant.GrantID] {
			return domainsecurity.TurnSecurityContext{}, false, nil, nil, time.Time{}, ErrGrantAuthority
		}
		callUsesCaseDataAuthority := executiongrantapp.CallUsesCaseDataAuthority(pending.Call)
		if effectClassified && callUsesCaseDataAuthority != usesCaseDataAuthority {
			return domainsecurity.TurnSecurityContext{}, false, nil, nil, time.Time{}, ErrGrantAuthority
		}
		usesCaseDataAuthority = callUsesCaseDataAuthority
		effectClassified = true
		arguments, err := domainjsonstrict.DecodeValue(pending.Call.Arguments, domainjsonstrict.Options{
			RequireObject: true, MaxBytes: 4 * 1024 * 1024, MaxDepth: 128, MaxTokens: 500_000, MaxStringBytes: 4 * 1024 * 1024,
		})
		if err != nil {
			return domainsecurity.TurnSecurityContext{}, false, nil, nil, time.Time{}, ErrGrantAuthority
		}
		memberExpiresAt, err := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
		if err != nil {
			return domainsecurity.TurnSecurityContext{}, false, nil, nil, time.Time{}, ErrGrantAuthority
		}
		if expiresAt.IsZero() || memberExpiresAt.Before(expiresAt) {
			expiresAt = memberExpiresAt
		}
		seen[grant.GrantID] = true
		references = append(references, GrantReference{GrantID: grant.GrantID})
		payload.Calls = append(payload.Calls, payloadCall{GrantID: grant.GrantID, ToolCallID: grant.ToolCallID, ToolName: grant.ToolName, Arguments: arguments})
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, false, nil, nil, time.Time{}, err
	}
	canonicalValue, err := domainjsonstrict.DecodeValue(rawPayload, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxCanonicalPayloadBytes, MaxDepth: 128, MaxTokens: 1_000_000, MaxStringBytes: maxCanonicalPayloadBytes,
	})
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, false, nil, nil, time.Time{}, err
	}
	canonicalPayload, err := json.Marshal(canonicalValue)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, false, nil, nil, time.Time{}, err
	}
	return securityContext, usesCaseDataAuthority, references, canonicalPayload, expiresAt, nil
}
