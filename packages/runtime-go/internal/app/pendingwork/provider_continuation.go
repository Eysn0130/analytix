package pendingwork

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	pendingworkstoreport "analytix.local/runtime-go/internal/ports/pendingworkstore"
)

type ProviderContinuationRequest struct {
	SecurityContext  domainsecurity.TurnSecurityContext
	References       []domainsecurity.SettledToolReference
	Request          domainmodel.Request
	ProviderConfig   domainmodel.TurnConfig
	PromptRoute      string
	ToolManifestHash string
	Sequence         uint64
	IssuedAt         time.Time
}

// ProviderContinuationLease holds private canonical payload bytes only in the
// active process. Only its keyed digest and signed receipt are durable.
type ProviderContinuationLease struct {
	workID string
	input  ProviderContinuationIssueInput
}

func (lease ProviderContinuationLease) WorkID() string { return lease.workID }

func (service *Service) BeginProviderContinuation(ctx context.Context, request ProviderContinuationRequest) (ProviderContinuationLease, error) {
	if !service.Available() || domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(request.SecurityContext) != nil ||
		domainsecurity.ValidateSettledToolReferences(request.References) != nil || len(request.References) == 0 || request.Sequence == 0 {
		return ProviderContinuationLease{}, ErrAuthorityUnavailable
	}
	toolManifestHash := strings.TrimSpace(request.ToolManifestHash)
	if err := validateProviderContinuationRequestRoute(request.Request, request.ProviderConfig, request.PromptRoute, toolManifestHash); err != nil {
		return ProviderContinuationLease{}, err
	}
	_, registry, err := service.currentAuthority(request.SecurityContext)
	if err != nil {
		return ProviderContinuationLease{}, err
	}
	routeHash := appmodel.ProviderContinuationCallRouteHash(request.ProviderConfig, request.PromptRoute, toolManifestHash)
	if routeHash == "" {
		return ProviderContinuationLease{}, ErrOperationMismatch
	}
	payload, err := appmodel.ProviderContinuationPayloadBytes(
		request.Request.SystemPrompt, appmodel.SanitizeToolPairing(request.Request.Messages), request.Request.PrivateAttachmentPlanDigest,
		toolManifestHash, registry.StateDigest, request.Sequence,
	)
	if err != nil {
		return ProviderContinuationLease{}, err
	}
	references := make([]GrantReference, len(request.References))
	expiresAt := time.Time{}
	for index, reference := range request.References {
		entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, reference.GrantID)
		if !found {
			return ProviderContinuationLease{}, ErrGrantAuthority
		}
		memberExpiry, parseErr := time.Parse(time.RFC3339Nano, entry.Grant.ExpiresAt)
		if parseErr != nil {
			return ProviderContinuationLease{}, ErrGrantAuthority
		}
		if expiresAt.IsZero() || memberExpiry.Before(expiresAt) {
			expiresAt = memberExpiry
		}
		references[index] = GrantReference{GrantID: reference.GrantID, ResultItemID: reference.ResultItemID}
	}
	issuedAt := request.IssuedAt.UTC()
	if issuedAt.IsZero() {
		issuedAt = time.Now().UTC()
	}
	input := ProviderContinuationIssueInput{
		SecurityContext: request.SecurityContext, GrantReferences: references, RegistryDigest: registry.StateDigest,
		CanonicalPayload: payload, RouteHash: routeHash, IssuedAt: issuedAt, ExpiresAt: expiresAt,
	}
	receipt, err := service.IssueProviderContinuation(ctx, input)
	if err != nil {
		return ProviderContinuationLease{}, err
	}
	return ProviderContinuationLease{workID: receipt.WorkID, input: input}, nil
}

func (service *Service) VerifyProviderContinuationLease(ctx context.Context, lease ProviderContinuationLease, now time.Time) error {
	if strings.TrimSpace(lease.workID) == "" || len(lease.input.CanonicalPayload) == 0 {
		return ErrOperationMismatch
	}
	_, err := service.VerifyProviderContinuation(ctx, lease.workID, lease.input, now)
	return err
}

// VerifyProviderContinuationRequest re-derives the private semantic identity
// at the physical send boundary. A caller cannot reuse a valid lease for a
// changed message set, route, tool manifest, sequence, or settled result set.
func (service *Service) VerifyProviderContinuationRequest(ctx context.Context, lease ProviderContinuationLease, request ProviderContinuationRequest, now time.Time) error {
	if strings.TrimSpace(lease.workID) == "" || len(lease.input.CanonicalPayload) == 0 ||
		domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(request.SecurityContext) != nil ||
		domainsecurity.ValidateSettledToolReferences(request.References) != nil || len(request.References) == 0 || request.Sequence == 0 ||
		request.SecurityContext.ContextDigest != lease.input.SecurityContext.ContextDigest {
		return ErrOperationMismatch
	}
	toolManifestHash := strings.TrimSpace(request.ToolManifestHash)
	if err := validateProviderContinuationRequestRoute(request.Request, request.ProviderConfig, request.PromptRoute, toolManifestHash); err != nil {
		return err
	}
	_, registry, err := service.currentAuthority(request.SecurityContext)
	if err != nil {
		return err
	}
	routeHash := appmodel.ProviderContinuationCallRouteHash(request.ProviderConfig, request.PromptRoute, toolManifestHash)
	payload, err := appmodel.ProviderContinuationPayloadBytes(
		request.Request.SystemPrompt, appmodel.SanitizeToolPairing(request.Request.Messages), request.Request.PrivateAttachmentPlanDigest,
		toolManifestHash, registry.StateDigest, request.Sequence,
	)
	if err != nil {
		return err
	}
	if registry.StateDigest != lease.input.RegistryDigest || routeHash == "" || routeHash != lease.input.RouteHash ||
		!bytes.Equal(payload, lease.input.CanonicalPayload) || len(request.References) != len(lease.input.GrantReferences) {
		return ErrOperationMismatch
	}
	for index, reference := range request.References {
		if reference.GrantID != lease.input.GrantReferences[index].GrantID || reference.ResultItemID != lease.input.GrantReferences[index].ResultItemID {
			return ErrOperationMismatch
		}
	}
	return service.VerifyProviderContinuationLease(ctx, lease, now)
}

func validateProviderContinuationRequestRoute(actual domainmodel.Request, config domainmodel.TurnConfig, promptRoute, toolManifestHash string) error {
	if err := domainmodel.ValidateReasoningEffortV1(actual.ReasoningEffort); err != nil {
		return err
	}
	if err := domainmodel.ValidateReasoningEffortV1(config.ReasoningEffort); err != nil {
		return err
	}
	promptRoute = strings.TrimSpace(promptRoute)
	if strings.TrimSpace(actual.ProviderID) != strings.TrimSpace(config.ProviderID) ||
		strings.TrimSpace(actual.Family) != strings.TrimSpace(config.Family) ||
		strings.TrimSpace(actual.EndpointFormat) != strings.TrimSpace(config.EndpointFormat) ||
		strings.TrimSpace(actual.BaseURL) != strings.TrimSpace(config.BaseURL) ||
		strings.TrimSpace(actual.Model) != strings.TrimSpace(config.Model) ||
		actual.ReasoningEffort != config.ReasoningEffort ||
		strings.TrimSpace(actual.ReasoningProtocol) != strings.TrimSpace(config.ReasoningProtocol) ||
		strings.TrimSpace(actual.Route) != promptRoute || promptRoute == "" ||
		toolcatalogapp.ToolSchemaHash(actual.Tools) != strings.TrimSpace(toolManifestHash) {
		return ErrOperationMismatch
	}
	return nil
}

func (service *Service) CloseProviderContinuationLease(ctx context.Context, lease ProviderContinuationLease, status, reasonCode string, disposedAt time.Time) (domainpendingwork.PendingWorkDispositionV1, error) {
	if disposedAt.IsZero() {
		disposedAt = time.Now().UTC()
	}
	receipt, existing, err := service.providerContinuationLeaseRecord(ctx, lease)
	if err != nil {
		return domainpendingwork.PendingWorkDispositionV1{}, err
	}
	status = strings.TrimSpace(status)
	reasonCode = strings.TrimSpace(reasonCode)
	if existing != nil {
		if existing.Status == status || (status != domainpendingwork.StatusCompleted && existing.Status != domainpendingwork.StatusCompleted) {
			return *existing, nil
		}
		return domainpendingwork.PendingWorkDispositionV1{}, ErrWorkClosed
	}
	if status == domainpendingwork.StatusCompleted {
		if err := service.VerifyProviderContinuationLease(ctx, lease, disposedAt); err != nil {
			return domainpendingwork.PendingWorkDispositionV1{}, err
		}
		return service.Close(ctx, lease.workID, lease.input.SecurityContext, status, reasonCode, disposedAt)
	}
	switch status {
	case domainpendingwork.StatusFailed, domainpendingwork.StatusCancelled, domainpendingwork.StatusExpired,
		domainpendingwork.StatusRejected, domainpendingwork.StatusStaleContext:
	default:
		return domainpendingwork.PendingWorkDispositionV1{}, ErrOperationMismatch
	}
	status, reasonCode = service.providerTerminalDisposition(lease.input.SecurityContext, receipt, status, reasonCode, disposedAt)
	return service.dispose(ctx, receipt, status, reasonCode, disposedAt)
}

func (service *Service) providerContinuationLeaseRecord(ctx context.Context, lease ProviderContinuationLease) (domainpendingwork.PendingWorkReceiptV1, *domainpendingwork.PendingWorkDispositionV1, error) {
	if !service.Available() || strings.TrimSpace(lease.workID) == "" || len(lease.input.CanonicalPayload) == 0 {
		return domainpendingwork.PendingWorkReceiptV1{}, nil, ErrAuthorityUnavailable
	}
	receipt, err := service.store.ReadReceipt(ctx, lease.workID)
	if err != nil || service.verifyReceipt(ctx, receipt) != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, nil, firstNonNilPendingWorkError(err, ErrOperationMismatch)
	}
	payloadHash, err := service.KeyedPayloadHash(ctx, providerContinuationPayloadPurpose, lease.input.CanonicalPayload)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, nil, err
	}
	binding, err := domainpendingwork.ContextBindingFromSecurityContextV1(lease.input.SecurityContext)
	if err != nil || receipt.Kind != domainpendingwork.KindProviderContinuation || receipt.Context != binding ||
		receipt.GrantRegistryDigest != lease.input.RegistryDigest || receipt.PayloadHash != payloadHash || receipt.RouteHash != lease.input.RouteHash ||
		receipt.IssuedAt != lease.input.IssuedAt.UTC().Format(time.RFC3339Nano) || receipt.ExpiresAt != lease.input.ExpiresAt.UTC().Format(time.RFC3339Nano) ||
		!providerReferencesMatch(receipt.GrantMembers, lease.input.GrantReferences) {
		return domainpendingwork.PendingWorkReceiptV1{}, nil, ErrOperationMismatch
	}
	disposition, err := service.store.ReadDisposition(ctx, receipt.WorkID)
	if err == nil {
		if service.verifyDisposition(ctx, disposition, receipt) != nil {
			return domainpendingwork.PendingWorkReceiptV1{}, nil, ErrOperationMismatch
		}
		return receipt, &disposition, nil
	}
	if !errors.Is(err, pendingworkstoreport.ErrNotFound) {
		return domainpendingwork.PendingWorkReceiptV1{}, nil, err
	}
	return receipt, nil, nil
}

func (service *Service) providerTerminalDisposition(current domainsecurity.TurnSecurityContext, receipt domainpendingwork.PendingWorkReceiptV1, status, reasonCode string, disposedAt time.Time) (string, string) {
	expiresAt, err := time.Parse(time.RFC3339Nano, receipt.ExpiresAt)
	if err == nil && !disposedAt.UTC().Before(expiresAt) {
		return domainpendingwork.StatusExpired, "provider_expired"
	}
	thread, err := service.grants.GetThread(current.ThreadID)
	if err == nil {
		latest, parseErr := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
		if parseErr == nil && latest.ThreadID == current.ThreadID && latest.ContextDigest != current.ContextDigest {
			return domainpendingwork.StatusStaleContext, "provider_stale_context"
		}
	}
	return status, reasonCode
}

func firstNonNilPendingWorkError(err, fallback error) error {
	if err != nil {
		return err
	}
	return fallback
}
