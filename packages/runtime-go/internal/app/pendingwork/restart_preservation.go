package pendingwork

import (
	"context"
	"errors"
	"sort"
	"strings"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

// ReportRestartScopeV1 is an immutable in-process preservation scope, derived
// from the complete signed Core inventory and original primary grant graph.
// It never authorizes execution, changes a disposition, or classifies a thread
// as ordinary. Thread-level preservation also protects shared epoch writes.
type ReportRestartScopeV1 struct {
	inheritedHistory  ReportRestartInheritedHistoryV1
	keyID             string
	reader            recoveryport.PrimaryThreadReaderV1
	primaryDigests    map[string]string
	absentPrimaries   map[string]bool
	childJobIDs       map[string]bool
	presenceReader    OriginalChildScopePrimaryReaderV1
	contexts          map[string]domainsecurity.TurnSecurityContext
	pendingDigests    map[string]string
	unresolvedDigests map[string]string
	turnIDs           []string
}

func (scope ReportRestartScopeV1) OwnsThread(threadID string) bool {
	_, found := scope.primaryDigests[threadID]
	return found || scope.absentPrimaries[threadID]
}

// ThreadIDs returns only observed original primaries. Reserved absent targets
// are carried separately so no consumer can fabricate an empty primary.
func (scope ReportRestartScopeV1) ThreadIDs() []string {
	ids := make([]string, 0, len(scope.primaryDigests))
	for id := range scope.primaryDigests {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (scope ReportRestartScopeV1) DeniedThreadIDsV1() []string {
	ids := scope.ThreadIDs()
	for id := range scope.absentPrimaries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (scope ReportRestartScopeV1) AbsentThreadIDsV1() []string {
	ids := make([]string, 0, len(scope.absentPrimaries))
	for id := range scope.absentPrimaries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (scope ReportRestartScopeV1) DeniedChildJobIDsV1() []string {
	ids := make([]string, 0, len(scope.childJobIDs))
	for id := range scope.childJobIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (scope ReportRestartScopeV1) Contexts() []domainsecurity.TurnSecurityContext {
	keys := make([]string, 0, len(scope.contexts))
	for key := range scope.contexts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	contexts := make([]domainsecurity.TurnSecurityContext, 0, len(keys))
	for _, key := range keys {
		contexts = append(contexts, scope.contexts[key])
	}
	return contexts
}

func (scope ReportRestartScopeV1) RevalidatePrimary(ctx context.Context) error {
	if ctx == nil {
		return errors.New("report restart preservation context is required")
	}
	for _, threadID := range scope.ThreadIDs() {
		if _, err := scope.ReadPrimaryThreadSnapshotV1(ctx, threadID); err != nil {
			return err
		}
	}
	for id := range scope.absentPrimaries {
		if scope.presenceReader == nil {
			return errors.New("original child primary absence authority is unavailable")
		}
		_, exists, err := scope.presenceReader.ObserveOriginalPrimaryPresenceV1(ctx, id)
		if err != nil {
			return err
		}
		if exists {
			return errors.New("reserved absent child primary appeared")
		}
	}
	return ctx.Err()
}

// ReadPrimaryThreadSnapshotV1 retains original-family authority for held
// consumers that cannot read a normalized or imported live thread view.
func (scope ReportRestartScopeV1) ReadPrimaryThreadSnapshotV1(ctx context.Context, threadID string) (recoveryport.PrimaryThreadSnapshotV1, error) {
	if ctx == nil || scope.reader == nil || scope.primaryDigests[threadID] == "" {
		return recoveryport.PrimaryThreadSnapshotV1{}, errors.New("report restart primary authority is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, err
	}
	snapshot, err := scope.reader.ReadPrimaryThreadSnapshotV1(ctx, threadID)
	if err != nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, err
	}
	if snapshot.ThreadID != threadID || snapshot.ThreadFileSHA256 != scope.primaryDigests[threadID] {
		return recoveryport.PrimaryThreadSnapshotV1{}, errors.New("report restart primary authority changed")
	}
	return snapshot, nil
}

func (service *Service) PlanReportRestartPreservationV1(ctx context.Context, reader recoveryport.PrimaryThreadReaderV1) (ReportRestartScopeV1, error) {
	if ctx == nil || reader == nil {
		return ReportRestartScopeV1{}, errors.New("report restart primary reader is required")
	}
	inventory, err := service.TrustedInventoryV1(ctx)
	if err != nil {
		return ReportRestartScopeV1{}, err
	}
	return planReportRestartPreservationV1(ctx, service.authority.KeyID(), inventory, reader, nil)
}

func planReportRestartPreservationV1(ctx context.Context, keyID string, inventory TrustedInventoryV1, reader recoveryport.PrimaryThreadReaderV1, inherited ReportRestartInheritedHistoryV1) (ReportRestartScopeV1, error) {
	scope := ReportRestartScopeV1{
		keyID: keyID, reader: reader, inheritedHistory: inherited,
		primaryDigests: map[string]string{}, contexts: map[string]domainsecurity.TurnSecurityContext{}, pendingDigests: map[string]string{},
		unresolvedDigests: map[string]string{},
	}
	primaries := map[string]map[string]any{}
	for _, receipt := range inventory.Receipts {
		if receipt.Kind != domainpendingwork.KindReportStage {
			continue
		}
		if disposition, found := inventory.Dispositions[receipt.WorkID]; found && disposition.Status != domainpendingwork.StatusOutcomeUnknown {
			continue
		}
		scope.unresolvedDigests[receipt.WorkID] = reportRestartPendingDigest(receipt, inventory.Dispositions[receipt.WorkID])
		threadID := receipt.Context.ThreadID
		thread := primaries[threadID]
		if thread == nil {
			snapshot, err := reader.ReadPrimaryThreadSnapshotV1(ctx, threadID)
			if err != nil {
				return ReportRestartScopeV1{}, err
			}
			if snapshot.ThreadID != threadID || !domainsecurity.IsSHA256Hex(snapshot.ThreadFileSHA256) {
				return ReportRestartScopeV1{}, errors.New("report restart primary binding is invalid")
			}
			if err := domainthread.ValidatePrimaryIdentityV1(threadID, snapshot.Thread); err != nil {
				return ReportRestartScopeV1{}, err
			}
			thread = snapshot.Thread
			primaries[threadID] = thread
			scope.primaryDigests[threadID] = snapshot.ThreadFileSHA256
			if err := scope.observeOriginalTurnsV1(ctx, threadID, thread, inventory); err != nil {
				return ReportRestartScopeV1{}, err
			}
		}
		turn, found := appmodel.TurnByID(thread, receipt.Context.TurnID)
		if !found {
			return ReportRestartScopeV1{}, errors.New("report restart primary turn is missing")
		}
		items, ok := turn["items"].([]any)
		if !ok {
			return ReportRestartScopeV1{}, errors.New("report restart grant inventory is invalid")
		}
		seen := map[string]bool{}
		for _, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok || item == nil {
				return ReportRestartScopeV1{}, errors.New("report restart grant inventory contains a non-object item")
			}
			id, ok := item["id"].(string)
			if !ok || id == "" || id != strings.TrimSpace(id) || seen[id] {
				return ReportRestartScopeV1{}, errors.New("report restart grant item identity is invalid")
			}
			seen[id] = true
		}
		if _, _, err := reportStageRestartDispositionFromThread(receipt, thread); err != nil {
			return ReportRestartScopeV1{}, err
		}
		frozen, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
		if err != nil || !receiptMatchesContext(receipt, frozen) {
			return ReportRestartScopeV1{}, errors.New("report restart frozen context is invalid")
		}
		scope.contexts[frozen.ContextDigest] = frozen
	}
	for _, receipt := range inventory.Receipts {
		if scope.OwnsThread(receipt.Context.ThreadID) {
			scope.pendingDigests[receipt.WorkID] = reportRestartPendingDigest(receipt, inventory.Dispositions[receipt.WorkID])
		}
	}
	if err := scope.RevalidatePrimary(ctx); err != nil {
		return ReportRestartScopeV1{}, err
	}
	return scope, nil
}

// PreserveReportRestartScopeV1 installs only a still-current sealed scope.
// Once installed it cannot be replaced to release a held thread in this process.
func (service *Service) PreserveReportRestartScopeV1(ctx context.Context, scope ReportRestartScopeV1) error {
	if service == nil || service.authority == nil || scope.keyID != service.authority.KeyID() {
		return ErrAuthorityUnavailable
	}
	service.reportStageTerminalMu.Lock()
	defer service.reportStageTerminalMu.Unlock()
	service.restartMu.Lock()
	defer service.restartMu.Unlock()
	if service.restartPreserved.keyID != "" {
		return errors.New("report restart preservation is already installed")
	}
	inventory, err := service.TrustedInventoryV1(ctx)
	if err != nil {
		return err
	}
	if err := scope.ValidateTrustedPendingInventoryV1(ctx, inventory); err != nil {
		return err
	}
	service.restartPreserved = scope
	return nil
}

// ValidateTrustedPendingInventoryV1 preserves the complete unresolved report
// inventory and every pending record on held threads. Callers first verify the
// entire inventory with the current installation key, including cross-leaf
// linkage. This does not authorize new work or mutate the immutable scope.
func (scope ReportRestartScopeV1) ValidateTrustedPendingInventoryV1(ctx context.Context, inventory TrustedInventoryV1) error {
	if ctx == nil || scope.keyID == "" {
		return errors.New("report restart pending observation is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	observed := map[string]string{}
	unresolved := map[string]string{}
	for _, receipt := range inventory.Receipts {
		if receipt.Kind == domainpendingwork.KindReportStage {
			disposition, found := inventory.Dispositions[receipt.WorkID]
			if !found || disposition.Status == domainpendingwork.StatusOutcomeUnknown {
				unresolved[receipt.WorkID] = reportRestartPendingDigest(receipt, disposition)
			}
		}
		if scope.OwnsThread(receipt.Context.ThreadID) {
			observed[receipt.WorkID] = reportRestartPendingDigest(receipt, inventory.Dispositions[receipt.WorkID])
		}
	}
	// The original scope is a complete unresolved inventory, including the
	// empty case. A new report on a previously unowned thread invalidates it.
	if len(unresolved) != len(scope.unresolvedDigests) {
		return errors.New("report restart unresolved inventory changed")
	}
	for id, digest := range scope.unresolvedDigests {
		if unresolved[id] != digest {
			return errors.New("report restart unresolved authority changed")
		}
	}
	if len(observed) != len(scope.pendingDigests) {
		return errors.New("report restart pending inventory changed")
	}
	for id, digest := range scope.pendingDigests {
		if observed[id] != digest {
			return errors.New("report restart pending authority changed")
		}
	}
	return scope.RevalidatePrimary(ctx)
}

func (service *Service) restartOwnsThread(threadID string) bool {
	if service == nil {
		return false
	}
	service.restartMu.RLock()
	defer service.restartMu.RUnlock()
	return service.restartPreserved.OwnsThread(threadID)
}

// OwnsRestartTurnV1 implements the existing restart gate ownership contract.
// The sealed scope preserves the entire thread, including shared epoch state.
func (service *Service) OwnsRestartTurnV1(threadID, _ string) bool {
	return service.restartOwnsThread(threadID)
}

func (service *Service) RestartPreservedTurnIDsV1() []string {
	if service == nil {
		return nil
	}
	service.restartMu.RLock()
	defer service.restartMu.RUnlock()
	return append([]string(nil), service.restartPreserved.turnIDs...)
}

func (service *Service) RevalidateRestartPreservationV1(ctx context.Context) error {
	if service == nil {
		return ErrAuthorityUnavailable
	}
	service.restartMu.RLock()
	defer service.restartMu.RUnlock()
	return service.restartPreserved.RevalidatePrimary(ctx)
}

func (service *Service) lockRestartWrite(threadID string) (func(), error) {
	service.restartMu.RLock()
	if service.restartPreserved.OwnsThread(threadID) {
		service.restartMu.RUnlock()
		return nil, ErrRestartPreserved
	}
	return service.restartMu.RUnlock, nil
}

func reportRestartPendingDigest(receipt domainpendingwork.PendingWorkReceiptV1, disposition domainpendingwork.PendingWorkDispositionV1) string {
	return canonicalRecordHash(map[string]any{"receipt": receipt, "disposition": disposition})
}
