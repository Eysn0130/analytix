package pendingwork

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	grantregistryport "analytix.local/runtime-go/internal/ports/grantregistry"
	pendingworkstoreport "analytix.local/runtime-go/internal/ports/pendingworkstore"
)

const (
	payloadKeyDerivationDomain = "analytix/pending-work-payload-key/v1\x00"
	payloadMACDomain           = "analytix/pending-work-payload-mac/v1\x00"
	maxCanonicalPayloadBytes   = 16 * 1024 * 1024
)

var (
	ErrAuthorityUnavailable = errors.New("pending work authority is unavailable")
	ErrWorkClosed           = errors.New("pending work is already closed")
	ErrWorkAlreadyOpen      = errors.New("pending work is already open")
	ErrWorkExpired          = errors.New("pending work is expired")
	ErrCurrentContext       = errors.New("pending work current security context is invalid")
	ErrGrantAuthority       = errors.New("pending work grant authority is invalid")
	ErrOperationMismatch    = errors.New("pending work operation identity is mismatched")
	ErrRestartPreserved     = errors.New("pending work is preserved for unresolved report recovery")
)

type GrantReference struct {
	GrantID      string
	ResultItemID string
}

// IssueInput accepts only host identities and hashes. Provider text, prompts,
// tool-result bytes, PII, and reasoning have no field through which they can
// enter the durable pending-work receipt.
type IssueInput struct {
	Kind                   string
	SecurityContext        domainsecurity.TurnSecurityContext
	GrantReferences        []GrantReference
	ExpectedRegistryDigest string
	PayloadHash            string
	RouteHash              string
	IssuedAt               time.Time
	ExpiresAt              time.Time
	ChildProducer          *domainpendingwork.ChildProducerV1
}

type Service struct {
	authority             authorityport.Authority
	store                 pendingworkstoreport.Store
	grants                grantregistryport.Reader
	now                   func() time.Time
	reportStageTerminalMu sync.Mutex
	restartMu             sync.RWMutex
	restartPreserved      ReportRestartScopeV1
	closedReportRestart   map[string]executiongrantapp.RestartGrantOutcomeAuthorityV1
}

func NewService(authority authorityport.Authority, store pendingworkstoreport.Store, grants grantregistryport.Reader) *Service {
	return &Service{authority: authority, store: store, grants: grants, now: time.Now}
}

func (service *Service) boundaryTime() time.Time {
	if service == nil || service.now == nil {
		return time.Now().UTC()
	}
	return service.now().UTC()
}

func (service *Service) Available() bool {
	return service != nil && service.authority != nil && service.store != nil && service.grants != nil &&
		strings.TrimSpace(service.authority.KeyID()) != "" && len(service.authority.PublicKey()) > 0
}

// KeyedPayloadHash binds exact canonical semantic request bytes without
// persisting a publicly guessable hash of low-entropy case data. The host
// authority signs only a fixed, domain-separated KDF message; payload bytes
// never cross the signer boundary and only the HMAC is durable.
func (service *Service) KeyedPayloadHash(ctx context.Context, purpose string, canonicalBytes []byte) (string, error) {
	if service == nil || service.authority == nil || strings.TrimSpace(service.authority.KeyID()) == "" || len(service.authority.PublicKey()) == 0 {
		return "", ErrAuthorityUnavailable
	}
	purpose = strings.TrimSpace(purpose)
	if !validPayloadPurpose(purpose) {
		return "", errors.New("pending work payload purpose is invalid")
	}
	value, err := domainjsonstrict.DecodeValue(canonicalBytes, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxCanonicalPayloadBytes, MaxDepth: 128, MaxTokens: 1_000_000, MaxStringBytes: maxCanonicalPayloadBytes,
	})
	if err != nil {
		return "", errors.New("pending work payload is not strict canonical JSON")
	}
	canonical, err := json.Marshal(value)
	if err != nil || !bytes.Equal(canonical, canonicalBytes) {
		return "", errors.New("pending work payload is not strict canonical JSON")
	}
	key, err := service.authority.Sign(ctx, []byte(payloadKeyDerivationDomain+purpose))
	if err != nil || len(key) < sha256.Size {
		return "", errors.New("pending work payload key derivation failed")
	}
	defer func() {
		for index := range key {
			key[index] = 0
		}
	}()
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payloadMACDomain))
	_, _ = mac.Write([]byte(purpose))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(canonicalBytes)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func (service *Service) Issue(ctx context.Context, input IssueInput) (domainpendingwork.PendingWorkReceiptV1, error) {
	if kind := strings.TrimSpace(input.Kind); kind == domainpendingwork.KindApprovedToolDispatch || kind == domainpendingwork.KindSideEffectIntent {
		return domainpendingwork.PendingWorkReceiptV1{}, errors.New("side effect dispatch requires the exclusive lease API")
	}
	receipt, err := service.prepareIssue(ctx, input)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	unlock, err := service.lockRestartWrite(receipt.Context.ThreadID)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	defer unlock()
	if err := service.store.PutReceiptIfAbsent(ctx, receipt); err != nil {
		existing, readErr := service.store.ReadReceipt(ctx, receipt.WorkID)
		// WorkID deliberately excludes receipt timing so the exact same semantic
		// operation can retry at its real effect boundary. A trusted existing
		// receipt is authoritative; VerifyOpenFor below still rejects closed,
		// expired, future-issued, stale-context, or changed-grant authority.
		if readErr != nil || existing.WorkID != receipt.WorkID || service.verifyReceipt(ctx, existing) != nil {
			return domainpendingwork.PendingWorkReceiptV1{}, err
		}
		return existing, nil
	}
	persisted, err := service.store.ReadReceipt(ctx, receipt.WorkID)
	if err != nil || persisted.ReceiptID != receipt.ReceiptID || service.verifyReceipt(ctx, persisted) != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, errors.New("persisted pending work receipt verification failed")
	}
	return persisted, nil
}

// issueExclusive is the write-effect admission boundary. Only the caller that
// receives an unambiguous successful create may obtain an executable lease.
// An existing open receipt, including a commit followed by lost acknowledgement,
// is conservatively non-resumable and must never be returned as new authority.
func (service *Service) issueExclusive(ctx context.Context, input IssueInput) (domainpendingwork.PendingWorkReceiptV1, error) {
	receipt, err := service.prepareIssue(ctx, input)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	return service.issuePreparedExclusive(ctx, receipt)
}

func (service *Service) issuePreparedExclusive(ctx context.Context, receipt domainpendingwork.PendingWorkReceiptV1) (domainpendingwork.PendingWorkReceiptV1, error) {
	unlock, err := service.lockRestartWrite(receipt.Context.ThreadID)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	defer unlock()
	created, err := service.store.CreateReceiptExclusive(ctx, receipt)
	if err != nil {
		existing, readErr := service.store.ReadReceipt(ctx, receipt.WorkID)
		if readErr == nil && existing.WorkID == receipt.WorkID && service.verifyReceipt(ctx, existing) == nil {
			if disposition, dispositionErr := service.store.ReadDisposition(ctx, existing.WorkID); dispositionErr == nil {
				if service.verifyDisposition(ctx, disposition, existing) != nil {
					return domainpendingwork.PendingWorkReceiptV1{}, ErrOperationMismatch
				}
				return domainpendingwork.PendingWorkReceiptV1{}, ErrWorkClosed
			} else if !errors.Is(dispositionErr, pendingworkstoreport.ErrNotFound) {
				return domainpendingwork.PendingWorkReceiptV1{}, dispositionErr
			}
			return domainpendingwork.PendingWorkReceiptV1{}, ErrWorkAlreadyOpen
		}
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	if !created {
		existing, readErr := service.store.ReadReceipt(ctx, receipt.WorkID)
		if readErr != nil || existing.WorkID != receipt.WorkID || service.verifyReceipt(ctx, existing) != nil {
			return domainpendingwork.PendingWorkReceiptV1{}, ErrOperationMismatch
		}
		if disposition, dispositionErr := service.store.ReadDisposition(ctx, existing.WorkID); dispositionErr == nil {
			if service.verifyDisposition(ctx, disposition, existing) != nil {
				return domainpendingwork.PendingWorkReceiptV1{}, ErrOperationMismatch
			}
			return domainpendingwork.PendingWorkReceiptV1{}, ErrWorkClosed
		} else if !errors.Is(dispositionErr, pendingworkstoreport.ErrNotFound) {
			return domainpendingwork.PendingWorkReceiptV1{}, dispositionErr
		}
		return domainpendingwork.PendingWorkReceiptV1{}, ErrWorkAlreadyOpen
	}
	persisted, err := service.store.ReadReceipt(ctx, receipt.WorkID)
	if err != nil || persisted.ReceiptID != receipt.ReceiptID || service.verifyReceipt(ctx, persisted) != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, errors.New("persisted exclusive pending work receipt verification failed")
	}
	if _, err := service.store.ReadDisposition(ctx, persisted.WorkID); err == nil {
		return domainpendingwork.PendingWorkReceiptV1{}, ErrWorkClosed
	} else if !errors.Is(err, pendingworkstoreport.ErrNotFound) {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	return persisted, nil
}

func (service *Service) prepareIssue(ctx context.Context, input IssueInput) (domainpendingwork.PendingWorkReceiptV1, error) {
	if !service.Available() {
		return domainpendingwork.PendingWorkReceiptV1{}, ErrAuthorityUnavailable
	}
	thread, registry, err := service.currentAuthority(input.SecurityContext)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	if expected := strings.TrimSpace(input.ExpectedRegistryDigest); expected != "" && expected != registry.StateDigest {
		return domainpendingwork.PendingWorkReceiptV1{}, fmt.Errorf("%w: registry changed before issue", ErrGrantAuthority)
	}
	issuedAt := input.IssuedAt.UTC()
	if issuedAt.IsZero() {
		issuedAt = time.Now().UTC()
	}
	members, err := grantMembers(input.Kind, input.SecurityContext, thread, registry, input.GrantReferences, issuedAt, input.ExpiresAt)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	receipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
		Kind: input.Kind, SecurityContext: input.SecurityContext,
		GrantRegistrySequence: registry.Sequence, GrantRegistryDigest: registry.StateDigest, GrantMembers: members,
		PayloadHash: input.PayloadHash, RouteHash: input.RouteHash, IssuedAt: issuedAt, ExpiresAt: input.ExpiresAt,
		AuthorityKeyID: service.authority.KeyID(), AuthorityPublicKey: service.authority.PublicKey(),
		ChildProducer: input.ChildProducer,
	}, func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) })
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	return receipt, nil
}

func (service *Service) VerifyOpenFor(ctx context.Context, workID string, current domainsecurity.TurnSecurityContext, kind, payloadHash, routeHash string, now time.Time) (domainpendingwork.PendingWorkReceiptV1, error) {
	if !service.Available() {
		return domainpendingwork.PendingWorkReceiptV1{}, ErrAuthorityUnavailable
	}
	receipt, err := service.openReceipt(ctx, workID)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	if receipt.Kind != strings.TrimSpace(kind) || receipt.PayloadHash != strings.TrimSpace(payloadHash) || receipt.RouteHash != strings.TrimSpace(routeHash) {
		return domainpendingwork.PendingWorkReceiptV1{}, ErrOperationMismatch
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	issuedAt, issuedErr := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
	expiresAt, err := time.Parse(time.RFC3339Nano, receipt.ExpiresAt)
	now = now.UTC()
	if issuedErr != nil || err != nil || now.Before(issuedAt) || !now.Before(expiresAt) {
		return domainpendingwork.PendingWorkReceiptV1{}, ErrWorkExpired
	}
	thread, registry, err := service.currentAuthority(current)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	if !receiptMatchesContext(receipt, current) {
		return domainpendingwork.PendingWorkReceiptV1{}, ErrCurrentContext
	}
	if err := verifyReceiptGrantAuthority(receipt, current, thread, registry, now); err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	return receipt, nil
}

// Close commits completion only after a current-context VerifyOpen. Downgrade
// statuses still require the exact current context, except stale_context which
// requires a different authoritative context on the same thread.
func (service *Service) Close(ctx context.Context, workID string, current domainsecurity.TurnSecurityContext, status, reasonCode string, disposedAt time.Time) (domainpendingwork.PendingWorkDispositionV1, error) {
	if !service.Available() {
		return domainpendingwork.PendingWorkDispositionV1{}, ErrAuthorityUnavailable
	}
	// One production process owns the pending-work roots. Serializing terminal
	// disposition with report settlement reservation prevents a close from
	// landing between the final open-work check and the settlement CAS.
	service.reportStageTerminalMu.Lock()
	defer service.reportStageTerminalMu.Unlock()
	if disposedAt.IsZero() {
		disposedAt = time.Now().UTC()
	}
	receipt, err := service.openReceipt(ctx, workID)
	if err != nil {
		return domainpendingwork.PendingWorkDispositionV1{}, err
	}
	if _, _, err := service.currentAuthority(current); err != nil {
		return domainpendingwork.PendingWorkDispositionV1{}, err
	}
	matches := receiptMatchesContext(receipt, current)
	switch strings.TrimSpace(status) {
	case domainpendingwork.StatusCompleted:
		if !matches {
			return domainpendingwork.PendingWorkDispositionV1{}, ErrCurrentContext
		}
		thread, registry, err := service.currentAuthority(current)
		if err != nil {
			return domainpendingwork.PendingWorkDispositionV1{}, err
		}
		if err := verifyCompletedGrantAuthority(receipt, current, thread, registry, disposedAt.UTC()); err != nil {
			return domainpendingwork.PendingWorkDispositionV1{}, err
		}
	case domainpendingwork.StatusStaleContext:
		if receipt.Context.ThreadID != current.ThreadID || matches {
			return domainpendingwork.PendingWorkDispositionV1{}, ErrCurrentContext
		}
	case domainpendingwork.StatusRestartInvalid:
		return domainpendingwork.PendingWorkDispositionV1{}, errors.New("restart-invalid pending work must be closed by restart reconciliation")
	case domainpendingwork.StatusOutcomeUnknown:
		return domainpendingwork.PendingWorkDispositionV1{}, errors.New("outcome-unknown pending work must be closed by restart reconciliation")
	default:
		if !matches {
			return domainpendingwork.PendingWorkDispositionV1{}, ErrCurrentContext
		}
	}
	return service.dispose(ctx, receipt, status, reasonCode, disposedAt)
}

// CloseAllOpenOnRestart is deliberately non-resumable. It validates the full
// signed inventory before the first mutation, then closes every open receipt
// in deterministic work-id order. A rerun safely observes prior dispositions.
func (service *Service) CloseAllOpenOnRestart(ctx context.Context, disposedAt time.Time) ([]domainpendingwork.PendingWorkDispositionV1, error) {
	if !service.Available() {
		return nil, ErrAuthorityUnavailable
	}
	service.reportStageTerminalMu.Lock()
	defer service.reportStageTerminalMu.Unlock()
	if disposedAt.IsZero() {
		disposedAt = time.Now().UTC()
	}
	inventory, err := service.TrustedInventoryV1(ctx)
	if err != nil {
		return nil, err
	}
	receiptByWork := make(map[string]domainpendingwork.PendingWorkReceiptV1, len(inventory.Receipts))
	for _, receipt := range inventory.Receipts {
		issuedAt, issuedErr := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
		if issuedErr != nil || disposedAt.UTC().Before(issuedAt) {
			return nil, errors.New("pending work restart disposition time is invalid")
		}
		receiptByWork[receipt.WorkID] = receipt
	}
	closed := make(map[string]bool, len(inventory.Dispositions))
	for workID := range inventory.Dispositions {
		closed[workID] = true
	}
	workIDs := make([]string, 0, len(receiptByWork))
	restartStatuses := make(map[string]string, len(receiptByWork))
	restartReasons := make(map[string]string, len(receiptByWork))
	for workID := range receiptByWork {
		if service.restartOwnsThread(receiptByWork[workID].Context.ThreadID) {
			continue
		}
		if !closed[workID] {
			workIDs = append(workIDs, workID)
			receipt := receiptByWork[workID]
			status, reason := domainpendingwork.StatusRestartInvalid, "restart_invalid"
			switch receipt.Kind {
			case domainpendingwork.KindApprovedToolDispatch:
				status, reason, err = service.approvedDispatchRestartDisposition(receipt)
				if err != nil {
					return nil, err
				}
			case domainpendingwork.KindSideEffectIntent:
				status, reason, err = service.sideEffectIntentRestartDisposition(receipt)
				if err != nil {
					return nil, err
				}
			case domainpendingwork.KindReportStage:
				status, reason, err = service.reportStageRestartDisposition(receipt)
				if err != nil {
					return nil, err
				}
			}
			restartStatuses[workID] = status
			restartReasons[workID] = reason
		}
	}
	sort.Strings(workIDs)
	result := make([]domainpendingwork.PendingWorkDispositionV1, 0, len(workIDs))
	for _, workID := range workIDs {
		receipt := receiptByWork[workID]
		status, reason := restartStatuses[workID], restartReasons[workID]
		disposition, err := service.dispose(ctx, receipt, status, reason, disposedAt)
		if errors.Is(err, ErrWorkClosed) {
			existing, readErr := service.store.ReadDisposition(ctx, workID)
			if readErr != nil || service.verifyDisposition(ctx, existing, receipt) != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		result = append(result, disposition)
	}
	return result, nil
}

func (service *Service) approvedDispatchRestartDisposition(receipt domainpendingwork.PendingWorkReceiptV1) (string, string, error) {
	if len(receipt.GrantMembers) != 1 {
		return "", "", errors.New("approved dispatch restart membership is invalid")
	}
	thread, err := service.grants.GetThread(receipt.Context.ThreadID)
	if err != nil || thread == nil {
		return "", "", errors.New("approved dispatch restart thread authority is unavailable")
	}
	turn, found := appmodel.TurnByID(thread, receipt.Context.TurnID)
	if !found {
		return "", "", errors.New("approved dispatch restart turn authority is unavailable")
	}
	current, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || !receiptMatchesContext(receipt, current) {
		return "", "", errors.New("approved dispatch restart context authority is invalid")
	}
	registry, err := executiongrantapp.RegistryFromThread(receipt.Context.ThreadID, thread, receipt.Context.TurnID)
	if err != nil {
		return "", "", errors.New("approved dispatch restart registry authority is invalid")
	}
	member := receipt.GrantMembers[0]
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, member.GrantID)
	if !found || entry.Grant.ReadOnly || entry.Grant.ApprovalState != "approved" ||
		entry.Grant.ToolName == ReportStageToolName {
		return "", "", errors.New("approved dispatch restart grant authority is invalid")
	}
	switch entry.Status {
	case domainsecurity.GrantRegistryActive:
		if !writeEffectReceiptMatchesRegistry(receipt, thread, entry.Grant) {
			return "", "", errors.New("approved dispatch restart registry prefix changed")
		}
		return domainpendingwork.StatusOutcomeUnknown, "tool_outcome_unknown_after_restart", nil
	case domainsecurity.GrantRegistrySettled:
		resultItemID, resultItem, found := exactDurableResultItem(thread, current, entry.Grant)
		if !found {
			return "", "", errors.New("approved dispatch settled grant lacks an exact durable result")
		}
		durable, err := executiongrantapp.DurableSettlementFromThread(
			receipt.Context.ThreadID, thread, receipt.Context.TurnID, resultItemID, entry.Grant,
		)
		if err != nil || canonicalRecordHash(durable.ResultItem) != canonicalRecordHash(resultItem) ||
			!writeEffectReceiptMatchesRegistry(receipt, thread, entry.Grant) {
			return "", "", errors.New("approved dispatch durable result authority is invalid")
		}
		return domainpendingwork.StatusCompleted, "tool_outcome_durable", nil
	default:
		return "", "", errors.New("approved dispatch restart grant state is invalid")
	}
}

func (service *Service) sideEffectIntentRestartDisposition(receipt domainpendingwork.PendingWorkReceiptV1) (string, string, error) {
	if receipt.Kind != domainpendingwork.KindSideEffectIntent || len(receipt.GrantMembers) != 1 {
		return "", "", errors.New("side effect intent restart membership is invalid")
	}
	thread, err := service.grants.GetThread(receipt.Context.ThreadID)
	if err != nil || thread == nil {
		return "", "", errors.New("side effect intent restart thread authority is unavailable")
	}
	turn, found := appmodel.TurnByID(thread, receipt.Context.TurnID)
	if !found {
		return "", "", errors.New("side effect intent restart turn authority is unavailable")
	}
	current, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || !receiptMatchesContext(receipt, current) {
		return "", "", errors.New("side effect intent restart context authority is invalid")
	}
	registry, err := executiongrantapp.RegistryFromThread(receipt.Context.ThreadID, thread, receipt.Context.TurnID)
	if err != nil {
		return "", "", errors.New("side effect intent restart registry authority is invalid")
	}
	member := receipt.GrantMembers[0]
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, member.GrantID)
	if !found || entry.Grant.ReadOnly ||
		(entry.Grant.ApprovalState != "approved" && entry.Grant.ApprovalState != "not_required") ||
		entry.Grant.ToolName == ReportStageToolName {
		return "", "", errors.New("side effect intent restart grant authority is invalid")
	}
	switch entry.Status {
	case domainsecurity.GrantRegistryActive:
		if !writeEffectReceiptMatchesRegistry(receipt, thread, entry.Grant) {
			return "", "", errors.New("side effect intent restart registry prefix changed")
		}
		return domainpendingwork.StatusOutcomeUnknown, "tool_outcome_unknown_after_restart", nil
	case domainsecurity.GrantRegistrySettled:
		resultItemID, resultItem, found := exactDurableResultItem(thread, current, entry.Grant)
		if !found {
			return "", "", errors.New("side effect intent settled grant lacks an exact durable result")
		}
		durable, err := executiongrantapp.DurableSettlementFromThread(
			receipt.Context.ThreadID, thread, receipt.Context.TurnID, resultItemID, entry.Grant,
		)
		if err != nil || canonicalRecordHash(durable.ResultItem) != canonicalRecordHash(resultItem) ||
			!writeEffectReceiptMatchesRegistry(receipt, thread, entry.Grant) {
			return "", "", errors.New("side effect intent durable result authority is invalid")
		}
		return domainpendingwork.StatusCompleted, "tool_outcome_durable", nil
	default:
		return "", "", errors.New("side effect intent restart grant state is invalid")
	}
}

func (service *Service) reportStageRestartDisposition(receipt domainpendingwork.PendingWorkReceiptV1) (string, string, error) {
	thread, err := service.grants.GetThread(receipt.Context.ThreadID)
	if err != nil || thread == nil {
		return "", "", errors.New("report stage restart thread authority is unavailable")
	}
	return reportStageRestartDispositionFromThread(receipt, thread)
}

func reportStageRestartDispositionFromThread(receipt domainpendingwork.PendingWorkReceiptV1, thread map[string]any) (string, string, error) {
	if len(receipt.GrantMembers) != 1 {
		return "", "", errors.New("report stage restart membership is invalid")
	}
	turn, found := appmodel.TurnByID(thread, receipt.Context.TurnID)
	if !found {
		return "", "", errors.New("report stage restart turn authority is unavailable")
	}
	current, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || !receiptMatchesContext(receipt, current) {
		return "", "", errors.New("report stage restart context authority is invalid")
	}
	registry, err := executiongrantapp.RegistryFromThread(receipt.Context.ThreadID, thread, receipt.Context.TurnID)
	if err != nil {
		return "", "", errors.New("report stage restart registry authority is invalid")
	}
	member := receipt.GrantMembers[0]
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, member.GrantID)
	if !found || entry.Grant.ReadOnly || entry.Grant.ApprovalState != "approved" || entry.Grant.ToolName != ReportStageToolName ||
		!writeEffectReceiptMatchesRegistry(receipt, thread, entry.Grant) {
		return "", "", errors.New("report stage restart grant authority is invalid")
	}
	switch entry.Status {
	case domainsecurity.GrantRegistryActive:
		return domainpendingwork.StatusOutcomeUnknown, "report_stage_outcome_unknown_after_restart", nil
	case domainsecurity.GrantRegistrySettled:
		resultItemID, resultItem, found := exactDurableResultItem(thread, current, entry.Grant)
		if !found {
			return "", "", errors.New("report stage settled grant lacks an exact durable result")
		}
		durable, err := executiongrantapp.DurableSettlementFromThread(
			receipt.Context.ThreadID, thread, receipt.Context.TurnID, resultItemID, entry.Grant,
		)
		if err != nil || canonicalRecordHash(durable.ResultItem) != canonicalRecordHash(resultItem) {
			return "", "", errors.New("report stage durable result authority is invalid")
		}
		if mapString(resultItem, "status") == "completed" && !mapBool(resultItem, "isError") {
			return domainpendingwork.StatusCompleted, "report_stage_completed", nil
		}
		return domainpendingwork.StatusFailed, "report_stage_failed", nil
	default:
		return "", "", errors.New("report stage restart grant state is invalid")
	}
}

func writeEffectReceiptMatchesRegistry(
	receipt domainpendingwork.PendingWorkReceiptV1,
	thread map[string]any,
	grant domainsecurity.ExecutionGrant,
) bool {
	if len(receipt.GrantMembers) != 1 {
		return false
	}
	prefix, err := executiongrantapp.RegistrySnapshotFromThread(
		receipt.Context.ThreadID, thread, receipt.Context.TurnID,
		receipt.GrantRegistrySequence, receipt.GrantRegistryDigest,
	)
	if err != nil {
		return false
	}
	member := receipt.GrantMembers[0]
	if member.RegistrySequence == 0 || member.RegistrySequence > uint64(len(prefix.Entries)) {
		return false
	}
	issuedEntry := prefix.Entries[member.RegistrySequence-1]
	return issuedEntry.Grant == grant && issuedEntry.Grant.GrantID == member.GrantID &&
		issuedEntry.EntryDigest == member.RegistryEntryDigest && issuedEntry.Status == domainsecurity.GrantRegistryActive
}

func (service *Service) currentAuthority(current domainsecurity.TurnSecurityContext) (map[string]any, domainsecurity.ExecutionGrantRegistry, error) {
	if service.restartOwnsThread(current.ThreadID) {
		return nil, domainsecurity.ExecutionGrantRegistry{}, ErrRestartPreserved
	}
	if domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(current) != nil {
		return nil, domainsecurity.ExecutionGrantRegistry{}, ErrCurrentContext
	}
	thread, err := service.grants.GetThread(current.ThreadID)
	if err != nil || thread == nil {
		return nil, domainsecurity.ExecutionGrantRegistry{}, ErrCurrentContext
	}
	if threadID := mapString(thread, "id"); threadID != "" && threadID != current.ThreadID {
		return nil, domainsecurity.ExecutionGrantRegistry{}, ErrCurrentContext
	}
	latest, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil || latest != current {
		return nil, domainsecurity.ExecutionGrantRegistry{}, ErrCurrentContext
	}
	turn, found := appmodel.TurnByID(thread, current.TurnID)
	if !found {
		return nil, domainsecurity.ExecutionGrantRegistry{}, ErrCurrentContext
	}
	frozen, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || frozen != current {
		return nil, domainsecurity.ExecutionGrantRegistry{}, ErrCurrentContext
	}
	registry, err := executiongrantapp.RegistryFromThread(current.ThreadID, thread, current.TurnID)
	if err != nil {
		return nil, domainsecurity.ExecutionGrantRegistry{}, fmt.Errorf("%w: registry replay failed", ErrGrantAuthority)
	}
	return thread, registry, nil
}

func validatePendingGrantForCall(
	current domainsecurity.TurnSecurityContext,
	grant domainsecurity.ExecutionGrant,
	call domainmodel.ToolCall,
) error {
	if executiongrantapp.ValidateExecutionGrantForCall(current, grant, call) != nil {
		return ErrGrantAuthority
	}
	return nil
}

func grantMembers(kind string, current domainsecurity.TurnSecurityContext, thread map[string]any, registry domainsecurity.ExecutionGrantRegistry, references []GrantReference, issuedAt, expiresAt time.Time) ([]domainpendingwork.GrantMemberV1, error) {
	if len(references) == 0 {
		return nil, fmt.Errorf("%w: no registered members", ErrGrantAuthority)
	}
	type memberAuthority struct {
		entry        domainsecurity.ExecutionGrantRegistryEntry
		resultItemID string
		resultDigest string
	}
	authorities := make([]memberAuthority, 0, len(references))
	seenGrants := map[string]bool{}
	seenResults := map[string]bool{}
	for _, reference := range references {
		grantID := strings.TrimSpace(reference.GrantID)
		resultItemID := strings.TrimSpace(reference.ResultItemID)
		entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, grantID)
		if !found || seenGrants[grantID] || entry.TurnID != current.TurnID || entry.ContextDigest != current.ContextDigest {
			return nil, fmt.Errorf("%w: member is not in the current registry", ErrGrantAuthority)
		}
		if err := executiongrantapp.ValidateExecutionGrantForToolName(
			current,
			entry.Grant,
			entry.Grant.ToolName,
		); err != nil {
			return nil, fmt.Errorf("%w: member call authority is invalid", ErrGrantAuthority)
		}
		seenGrants[grantID] = true
		resultDigest := ""
		if err := verifyKindAuthority(kind, current, thread, registry, entry, resultItemID, issuedAt); err != nil {
			return nil, err
		}
		if kind == domainpendingwork.KindProviderContinuation {
			if seenResults[resultItemID] {
				return nil, fmt.Errorf("%w: duplicate durable result", ErrGrantAuthority)
			}
			seenResults[resultItemID] = true
			durable, err := executiongrantapp.DurableSettlementFromThread(current.ThreadID, thread, current.TurnID, resultItemID, entry.Grant)
			if err != nil || !validDurableResultItem(durable.ResultItem, current, entry.Grant, resultItemID) {
				return nil, fmt.Errorf("%w: durable result settlement is invalid", ErrGrantAuthority)
			}
			resultDigest = canonicalRecordHash(durable.ResultItem)
			if resultDigest == "" {
				return nil, fmt.Errorf("%w: durable result is not canonical", ErrGrantAuthority)
			}
		}
		grantExpiresAt, err := time.Parse(time.RFC3339Nano, entry.Grant.ExpiresAt)
		memberUpdatedAt, updatedErr := time.Parse(time.RFC3339Nano, entry.UpdatedAt)
		if err != nil || updatedErr != nil || issuedAt.Before(memberUpdatedAt) || !issuedAt.Before(grantExpiresAt) || expiresAt.IsZero() || expiresAt.UTC().After(grantExpiresAt) {
			return nil, fmt.Errorf("%w: member expiry does not cover pending work", ErrGrantAuthority)
		}
		authorities = append(authorities, memberAuthority{entry: entry, resultItemID: resultItemID, resultDigest: resultDigest})
	}
	sort.Slice(authorities, func(i, j int) bool { return authorities[i].entry.Sequence < authorities[j].entry.Sequence })
	members := make([]domainpendingwork.GrantMemberV1, len(authorities))
	for index, authority := range authorities {
		members[index] = domainpendingwork.GrantMemberV1{
			Ordinal: uint32(index + 1), GrantID: authority.entry.Grant.GrantID, RegistrySequence: authority.entry.Sequence,
			RegistryEntryDigest: authority.entry.EntryDigest, ResultItemID: authority.resultItemID, ResultItemDigest: authority.resultDigest,
		}
	}
	return members, nil
}

func verifyReceiptGrantAuthority(receipt domainpendingwork.PendingWorkReceiptV1, current domainsecurity.TurnSecurityContext, thread map[string]any, registry domainsecurity.ExecutionGrantRegistry, now time.Time) error {
	prefix, err := executiongrantapp.RegistrySnapshotFromThread(
		current.ThreadID, thread, current.TurnID,
		receipt.GrantRegistrySequence, receipt.GrantRegistryDigest,
	)
	if err != nil {
		return fmt.Errorf("%w: registry prefix changed", ErrGrantAuthority)
	}
	for _, member := range receipt.GrantMembers {
		if member.RegistrySequence == 0 || member.RegistrySequence > uint64(len(prefix.Entries)) {
			return fmt.Errorf("%w: member sequence is unavailable", ErrGrantAuthority)
		}
		issuedEntry := prefix.Entries[member.RegistrySequence-1]
		entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, member.GrantID)
		if !found || issuedEntry.Grant.GrantID != member.GrantID || issuedEntry.EntryDigest != member.RegistryEntryDigest ||
			issuedEntry.Grant != entry.Grant {
			return fmt.Errorf("%w: member registry binding changed", ErrGrantAuthority)
		}
		if err := executiongrantapp.ValidateExecutionGrantForToolName(current, entry.Grant, entry.Grant.ToolName); err != nil {
			return fmt.Errorf("%w: member call authority is invalid", ErrGrantAuthority)
		}
		if err := verifyKindAuthority(receipt.Kind, current, thread, registry, entry, member.ResultItemID, now); err != nil {
			return err
		}
		if receipt.Kind == domainpendingwork.KindProviderContinuation {
			durable, err := executiongrantapp.DurableSettlementFromThread(current.ThreadID, thread, current.TurnID, member.ResultItemID, entry.Grant)
			if err != nil || !validDurableResultItem(durable.ResultItem, current, entry.Grant, member.ResultItemID) || canonicalRecordHash(durable.ResultItem) != member.ResultItemDigest {
				return fmt.Errorf("%w: durable result binding changed", ErrGrantAuthority)
			}
		}
		grantExpiresAt, err := time.Parse(time.RFC3339Nano, entry.Grant.ExpiresAt)
		if err != nil || !now.Before(grantExpiresAt) {
			return fmt.Errorf("%w: member grant expired", ErrGrantAuthority)
		}
	}
	return nil
}

func verifyCompletedGrantAuthority(receipt domainpendingwork.PendingWorkReceiptV1, current domainsecurity.TurnSecurityContext, thread map[string]any, registry domainsecurity.ExecutionGrantRegistry, now time.Time) error {
	expiresAt, err := time.Parse(time.RFC3339Nano, receipt.ExpiresAt)
	if err != nil || !now.Before(expiresAt) {
		return ErrWorkExpired
	}
	if receipt.Kind == domainpendingwork.KindProviderContinuation {
		return verifyReceiptGrantAuthority(receipt, current, thread, registry, now)
	}
	prefix, err := executiongrantapp.RegistrySnapshotFromThread(
		current.ThreadID, thread, current.TurnID,
		receipt.GrantRegistrySequence, receipt.GrantRegistryDigest,
	)
	if err != nil {
		return errors.Join(ErrGrantAuthority, err)
	}
	for _, member := range receipt.GrantMembers {
		entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, member.GrantID)
		if !found || entry.Sequence != member.RegistrySequence || entry.TurnID != current.TurnID ||
			entry.ContextDigest != current.ContextDigest || entry.Status != domainsecurity.GrantRegistrySettled {
			return fmt.Errorf("%w: completed pending work member is not durably settled", ErrGrantAuthority)
		}
		if member.RegistrySequence == 0 || member.RegistrySequence > uint64(len(prefix.Entries)) {
			return fmt.Errorf("%w: completed pending work issued member is absent", ErrGrantAuthority)
		}
		issued := prefix.Entries[member.RegistrySequence-1]
		if issued.Grant != entry.Grant || issued.EntryDigest != member.RegistryEntryDigest || issued.Status != domainsecurity.GrantRegistryActive {
			return fmt.Errorf("%w: completed pending work lost its issued active registry prefix", ErrGrantAuthority)
		}
		if err := executiongrantapp.ValidateExecutionGrantForToolName(current, entry.Grant, entry.Grant.ToolName); err != nil {
			return fmt.Errorf("%w: completed member call authority is invalid", ErrGrantAuthority)
		}
		if receipt.Kind == domainpendingwork.KindToolBatch && !entry.Grant.ReadOnly {
			return fmt.Errorf("%w: completed tool batch member is not read-only", ErrGrantAuthority)
		}
		if receipt.Kind == domainpendingwork.KindApprovedToolDispatch &&
			(entry.Grant.ReadOnly || entry.Grant.ApprovalState != "approved" || entry.Grant.ToolName == ReportStageToolName) {
			return fmt.Errorf("%w: completed approved dispatch lacks writable approval", ErrGrantAuthority)
		}
		if receipt.Kind == domainpendingwork.KindSideEffectIntent &&
			(entry.Grant.ReadOnly || (entry.Grant.ApprovalState != "approved" && entry.Grant.ApprovalState != "not_required") || entry.Grant.ToolName == ReportStageToolName) {
			return fmt.Errorf("%w: completed side effect intent lacks writable authority", ErrGrantAuthority)
		}
		if receipt.Kind == domainpendingwork.KindReportStage &&
			(entry.Grant.ToolName != ReportStageToolName || entry.Grant.ReadOnly || entry.Grant.ApprovalState != "approved") {
			return fmt.Errorf("%w: completed report stage member lacks write approval", ErrGrantAuthority)
		}
		resultItemID, resultItem, found := exactDurableResultItem(thread, current, entry.Grant)
		if !found {
			return fmt.Errorf("%w: completed pending work member lacks its durable result", ErrGrantAuthority)
		}
		durable, err := executiongrantapp.DurableSettlementFromThread(current.ThreadID, thread, current.TurnID, resultItemID, entry.Grant)
		if err != nil || !validDurableResultItem(durable.ResultItem, current, entry.Grant, resultItemID) || canonicalRecordHash(durable.ResultItem) != canonicalRecordHash(resultItem) {
			return fmt.Errorf("%w: completed pending work settlement is invalid", ErrGrantAuthority)
		}
		if receipt.Kind == domainpendingwork.KindReportStage &&
			(mapString(durable.ResultItem, "status") != "completed" || mapBool(durable.ResultItem, "isError")) {
			return fmt.Errorf("%w: completed report stage requires a successful durable result", ErrGrantAuthority)
		}
		activeEntry, activeFound := domainsecurity.ExecutionGrantRegistryEntryByID(durable.ActiveRegistry, member.GrantID)
		if !activeFound || activeEntry.Sequence != member.RegistrySequence || activeEntry.EntryDigest != member.RegistryEntryDigest ||
			domainsecurity.VerifyExecutionGrantMembership(durable.ActiveRegistry, current.ThreadID, current.TurnID, entry.Grant, domainsecurity.GrantRegistryActive) != nil ||
			domainsecurity.VerifyExecutionGrantMembership(durable.SettledRegistry, current.ThreadID, current.TurnID, entry.Grant, domainsecurity.GrantRegistrySettled) != nil {
			return fmt.Errorf("%w: completed pending work does not close its issued active member", ErrGrantAuthority)
		}
	}
	return nil
}

func exactDurableResultItem(thread map[string]any, current domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant) (string, map[string]any, bool) {
	turn, found := appmodel.TurnByID(thread, current.TurnID)
	if !found {
		return "", nil, false
	}
	var result map[string]any
	items, _ := turn["items"].([]any)
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok || mapString(item, "kind") != "tool_result" || mapString(item, "executionGrantId") != grant.GrantID {
			continue
		}
		if !validDurableResultItem(item, current, grant, mapString(item, "id")) {
			return "", nil, false
		}
		if result != nil {
			return "", nil, false
		}
		result = item
	}
	if result == nil {
		return "", nil, false
	}
	return mapString(result, "id"), result, true
}

func validDurableResultItem(item map[string]any, current domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant, resultItemID string) bool {
	if mapString(item, "id") != strings.TrimSpace(resultItemID) || mapString(item, "kind") != "tool_result" ||
		mapString(item, "role") != "tool" || (mapString(item, "status") != "completed" && mapString(item, "status") != "failed") ||
		mapString(item, "threadId") != current.ThreadID || mapString(item, "turnId") != current.TurnID ||
		mapString(item, "contextDigest") != current.ContextDigest || mapString(item, "executionGrantId") != grant.GrantID ||
		mapString(item, "toolName") != grant.ToolName || mapString(item, "callId") != grant.ToolCallID || canonicalRecordHash(item) == "" {
		return false
	}
	epoch, epochOK := mapUint64(item["contextEpoch"])
	createdAt, createdErr := time.Parse(time.RFC3339Nano, mapString(item, "createdAt"))
	finishedAt, finishedErr := time.Parse(time.RFC3339Nano, mapString(item, "finishedAt"))
	return epochOK && epoch == current.ContextEpoch && createdErr == nil && finishedErr == nil && !createdAt.After(finishedAt)
}

func verifyKindAuthority(kind string, current domainsecurity.TurnSecurityContext, thread map[string]any, registry domainsecurity.ExecutionGrantRegistry, entry domainsecurity.ExecutionGrantRegistryEntry, resultItemID string, _ time.Time) error {
	switch strings.TrimSpace(kind) {
	case domainpendingwork.KindToolBatch:
		if entry.Status != domainsecurity.GrantRegistryActive || !entry.Grant.ReadOnly || resultItemID != "" {
			return fmt.Errorf("%w: tool batch requires an active read-only grant", ErrGrantAuthority)
		}
	case domainpendingwork.KindApprovedToolDispatch:
		if entry.Status != domainsecurity.GrantRegistryActive || entry.Grant.ReadOnly ||
			entry.Grant.ApprovalState != "approved" || entry.Grant.ToolName == ReportStageToolName || resultItemID != "" {
			return fmt.Errorf("%w: approved dispatch requires one active writable approved grant", ErrGrantAuthority)
		}
	case domainpendingwork.KindSideEffectIntent:
		if entry.Status != domainsecurity.GrantRegistryActive || entry.Grant.ReadOnly ||
			(entry.Grant.ApprovalState != "approved" && entry.Grant.ApprovalState != "not_required") ||
			entry.Grant.ToolName == ReportStageToolName || resultItemID != "" {
			return fmt.Errorf("%w: side effect intent requires one active writable grant", ErrGrantAuthority)
		}
	case domainpendingwork.KindProviderContinuation:
		if entry.Status != domainsecurity.GrantRegistrySettled || resultItemID == "" {
			return fmt.Errorf("%w: provider continuation requires a settled durable result", ErrGrantAuthority)
		}
		if _, err := executiongrantapp.DurableSettlementFromThread(current.ThreadID, thread, current.TurnID, resultItemID, entry.Grant); err != nil {
			return fmt.Errorf("%w: provider continuation settlement is invalid", ErrGrantAuthority)
		}
	case domainpendingwork.KindReportStage:
		if entry.Status != domainsecurity.GrantRegistryActive || entry.Grant.ToolName != ReportStageToolName ||
			entry.Grant.ApprovalState != "approved" || entry.Grant.ReadOnly || resultItemID != "" {
			return fmt.Errorf("%w: report stage requires an active approved writable grant", ErrGrantAuthority)
		}
	default:
		return fmt.Errorf("%w: pending work kind is invalid", ErrGrantAuthority)
	}
	return nil
}

func (service *Service) openReceipt(ctx context.Context, workID string) (domainpendingwork.PendingWorkReceiptV1, error) {
	receipt, err := service.store.ReadReceipt(ctx, strings.TrimSpace(workID))
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	if err := service.verifyReceipt(ctx, receipt); err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	if service.restartOwnsThread(receipt.Context.ThreadID) {
		return domainpendingwork.PendingWorkReceiptV1{}, ErrRestartPreserved
	}
	if _, err := service.store.ReadDisposition(ctx, receipt.WorkID); err == nil {
		return domainpendingwork.PendingWorkReceiptV1{}, ErrWorkClosed
	} else if !errors.Is(err, pendingworkstoreport.ErrNotFound) {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	return receipt, nil
}

func (service *Service) dispose(ctx context.Context, receipt domainpendingwork.PendingWorkReceiptV1, status, reasonCode string, disposedAt time.Time) (domainpendingwork.PendingWorkDispositionV1, error) {
	unlock, err := service.lockRestartWrite(receipt.Context.ThreadID)
	if err != nil {
		return domainpendingwork.PendingWorkDispositionV1{}, err
	}
	defer unlock()
	disposition, err := domainpendingwork.NewPendingWorkDispositionV1(receipt, status, reasonCode, disposedAt, service.authority.KeyID(), service.authority.PublicKey(), func(message []byte) ([]byte, error) {
		return service.authority.Sign(ctx, message)
	})
	if err != nil {
		return domainpendingwork.PendingWorkDispositionV1{}, err
	}
	if err := service.store.PutDispositionIfAbsent(ctx, disposition); err != nil {
		if existing, readErr := service.store.ReadDisposition(ctx, receipt.WorkID); readErr == nil && service.verifyDisposition(ctx, existing, receipt) == nil {
			return domainpendingwork.PendingWorkDispositionV1{}, ErrWorkClosed
		}
		return domainpendingwork.PendingWorkDispositionV1{}, err
	}
	persisted, err := service.store.ReadDisposition(ctx, receipt.WorkID)
	if err != nil || persisted.DispositionID != disposition.DispositionID || service.verifyDisposition(ctx, persisted, receipt) != nil {
		return domainpendingwork.PendingWorkDispositionV1{}, errors.New("persisted pending work disposition verification failed")
	}
	return persisted, nil
}

func (service *Service) verifyReceipt(ctx context.Context, receipt domainpendingwork.PendingWorkReceiptV1) error {
	keyID, publicKey, signature, err := domainpendingwork.PendingWorkReceiptV1AuthorityMaterial(receipt)
	if err != nil {
		return err
	}
	if keyID != service.authority.KeyID() || !bytes.Equal(publicKey, service.authority.PublicKey()) {
		return errors.New("pending work receipt installation authority is mismatched")
	}
	if err := service.authority.VerifyTrusted(ctx, keyID, publicKey, domainpendingwork.PendingWorkReceiptV1SigningBytes(receipt), signature); err != nil {
		return errors.Join(errors.New("pending work receipt signature is not trusted"), err)
	}
	return nil
}

func (service *Service) verifyDisposition(ctx context.Context, disposition domainpendingwork.PendingWorkDispositionV1, receipt domainpendingwork.PendingWorkReceiptV1) error {
	if err := domainpendingwork.ValidatePendingWorkDispositionForReceiptV1(disposition, receipt); err != nil {
		return err
	}
	keyID, publicKey, signature, err := domainpendingwork.PendingWorkDispositionV1AuthorityMaterial(disposition)
	if err != nil {
		return err
	}
	if keyID != service.authority.KeyID() || !bytes.Equal(publicKey, service.authority.PublicKey()) {
		return errors.New("pending work disposition installation authority is mismatched")
	}
	if err := service.authority.VerifyTrusted(ctx, keyID, publicKey, domainpendingwork.PendingWorkDispositionV1SigningBytes(disposition), signature); err != nil {
		return errors.Join(errors.New("pending work disposition signature is not trusted"), err)
	}
	return nil
}

func receiptMatchesContext(receipt domainpendingwork.PendingWorkReceiptV1, current domainsecurity.TurnSecurityContext) bool {
	binding, err := domainpendingwork.ContextBindingFromSecurityContextV1(current)
	return err == nil && binding == receipt.Context
}

func canonicalRecordHash(record map[string]any) string {
	body, err := json.Marshal(record)
	if err != nil {
		return ""
	}
	return domainsecurity.CanonicalJSONHash(body)
}

func mapString(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}

func mapBool(record map[string]any, key string) bool {
	value, _ := record[key].(bool)
	return value
}

func mapUint64(value any) (uint64, bool) {
	switch number := value.(type) {
	case uint64:
		return number, true
	case uint32:
		return uint64(number), true
	case int:
		if number >= 0 {
			return uint64(number), true
		}
	case float64:
		converted := uint64(number)
		if number >= 0 && float64(converted) == number {
			return converted, true
		}
	case json.Number:
		converted, err := number.Int64()
		if err == nil && converted >= 0 {
			return uint64(converted), true
		}
	}
	return 0, false
}

func validPayloadPurpose(value string) bool {
	if value == "" || len(value) > 96 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' && char != '-' && char != '.' && char != '/' && char != ':' {
			return false
		}
	}
	return true
}
