package casethread

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	appturn "analytix.local/runtime-go/internal/app/turn"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	storeport "analytix.local/runtime-go/internal/ports/casethreadauthority"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

type Authority interface {
	Register(context.Context, domainsecurity.TurnSecurityContext) error
	Derive(context.Context, string, string, string) error
	IsCaseThread(string) bool
	// ContainsContext reports publication/execution authority only. Prepared
	// records remain invisible; a read-only migration overlay is the sole
	// compatibility exception and is rebuilt from verified final inventory.
	ContainsContext(domainsecurity.TurnSecurityContext) bool
	ContextTurnIDs(string) []string
	CanExecute(string) bool
	RestartPreservesThreadV1(string) bool
	ReplaceQuarantine(map[string]string)
}

// LineageReceiptAuthority exposes the exact installation-signed lineage
// record needed by crash-recoverable thread derivation. Legacy Authority
// callers may continue to use Derive when they do not persist a transaction.
type LineageReceiptAuthority interface {
	Authority
	DeriveWithReceipt(context.Context, string, string, string) (domainsecurity.CaseThreadAuthorityRecord, error)
	VerifyDerivedLineageReceipt(context.Context, string, string, string, string) (domainsecurity.CaseThreadAuthorityRecord, error)
}

type CommittedAuthority interface {
	Authority
	WithRestartRecoveryV1(string, func() error) error
	Commit(context.Context, domainsecurity.TurnSecurityContext, domaincontextepoch.State, time.Time) error
	CommittedContext(string, string) (CommittedContext, bool)
	CommittedContexts() []CommittedContext
}

type CommittedContext struct {
	SecurityContext domainsecurity.TurnSecurityContext
	EpochState      domaincontextepoch.State
}

type Registry struct {
	authority               authorityport.Authority
	store                   storeport.Store
	mu                      sync.RWMutex
	byRecord                map[string]domainsecurity.CaseThreadAuthorityRecord
	byTurn                  map[string]domainsecurity.CaseThreadAuthorityRecord
	byContext               map[string]domainsecurity.CaseThreadAuthorityRecord
	committed               map[string]domainsecurity.CaseThreadAuthorityRecord
	migration               map[string]domainsecurity.TurnSecurityContext
	lineage                 map[string]domainsecurity.CaseThreadAuthorityRecord
	threadIndex             map[string]map[string]bool
	quarantine              map[string]string
	restartPreserved        *restartPreservationV1
	activeHistoryIdentities func(context.Context, string, []string) error
}

type MigrationPlan struct {
	Records  []domainsecurity.CaseThreadAuthorityRecord
	Contexts []domainsecurity.TurnSecurityContext
}

type RestartInventory struct {
	Quarantined map[string]string
}

type ThreadReader interface {
	AllThreadIDs() ([]string, error)
	GetThread(string) (map[string]any, error)
}

func RegisterRequired(ctx context.Context, authority Authority, securityContext domainsecurity.TurnSecurityContext) error {
	if authority != nil && authority.RestartPreservesThreadV1(securityContext.ThreadID) {
		return ErrRestartPreserved
	}
	required, err := currentAuthorityRequired(securityContext)
	if err != nil {
		return err
	}
	if !required {
		return nil
	}
	if authority == nil {
		return errors.New("case thread authority is unavailable")
	}
	return authority.Register(ctx, securityContext)
}

func CommitRequired(ctx context.Context, authority Authority, securityContext domainsecurity.TurnSecurityContext, state domaincontextepoch.State, committedAt time.Time) error {
	if authority != nil && authority.RestartPreservesThreadV1(securityContext.ThreadID) {
		return ErrRestartPreserved
	}
	required, err := currentAuthorityRequired(securityContext)
	if err != nil {
		return err
	}
	if !required {
		return nil
	}
	committer, ok := authority.(CommittedAuthority)
	if !ok {
		return errors.New("case thread authority is unavailable")
	}
	return committer.Commit(ctx, securityContext, state, committedAt)
}

type CommittedContextRepairStore interface {
	GetThreadForAuthorityRepair(string) (map[string]any, error)
	ReplaceThreadForAuthorityRepair(string, map[string]any) error
}

func RepairCommittedContexts(authority CommittedAuthority, store CommittedContextRepairStore) error {
	if authority == nil || store == nil {
		return errors.New("committed case turn context repair dependencies are unavailable")
	}
	for _, committed := range authority.CommittedContexts() {
		if !isCurrentCaseAuthorityContext(committed.SecurityContext) {
			return errors.New("legacy case turn context is audit-only")
		}
		if authority.RestartPreservesThreadV1(committed.SecurityContext.ThreadID) {
			continue
		}
		if authority.IsCaseThread(committed.SecurityContext.ThreadID) &&
			!authority.CanExecute(committed.SecurityContext.ThreadID) {
			continue
		}
		if err := authority.WithRestartRecoveryV1(committed.SecurityContext.ThreadID, func() error {
			return repairCommittedContext(committed, store)
		}); err != nil && !errors.Is(err, ErrRestartPreserved) {
			return err
		}
	}
	return nil
}

func repairCommittedContext(committed CommittedContext, store CommittedContextRepairStore) error {
	thread, err := store.GetThreadForAuthorityRepair(committed.SecurityContext.ThreadID)
	if err != nil {
		return err
	}
	repaired, err := appturn.RepairCommittedTurnContext(appturn.CommittedContextRepairInput{
		Thread: thread, SecurityContext: committed.SecurityContext, EpochState: committed.EpochState,
	})
	if err != nil {
		return err
	}
	if sameCommittedContextJSONV1(thread, repaired) {
		return nil
	}
	if err := store.ReplaceThreadForAuthorityRepair(committed.SecurityContext.ThreadID, repaired); err != nil {
		return err
	}
	reloaded, err := store.GetThreadForAuthorityRepair(committed.SecurityContext.ThreadID)
	if err != nil {
		return err
	}
	verified, err := appturn.RepairCommittedTurnContext(appturn.CommittedContextRepairInput{
		Thread: reloaded, SecurityContext: committed.SecurityContext, EpochState: committed.EpochState,
	})
	if err != nil || !sameCommittedContextJSONV1(reloaded, verified) {
		return errors.New("committed case turn context repair readback failed")
	}
	return nil
}

func sameCommittedContextJSONV1(left, right map[string]any) bool {
	leftBody, leftErr := json.Marshal(left)
	rightBody, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func RegistrationHook(authority Authority) func(context.Context, domainsecurity.TurnSecurityContext) error {
	return func(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
		if authority != nil && authority.IsCaseThread(securityContext.ThreadID) && !isCurrentCaseAuthorityContext(securityContext) {
			return errors.New("case thread lost its host publication policy")
		}
		return RegisterRequired(ctx, authority, securityContext)
	}
}

func currentAuthorityRequired(securityContext domainsecurity.TurnSecurityContext) (bool, error) {
	if domainsecurity.ValidateTurnSecurityContext(securityContext) != nil ||
		securityContext.Version != domainsecurity.TurnSecurityContextVersionV2 {
		return false, errors.New("case thread authority requires a V2 turn security context")
	}
	if domainsecurity.TurnSecurityContextRequiresFinalEvidenceGate(securityContext) {
		return true, nil
	}
	if securityContext.PublicationPolicy.Disposition == domainsecurity.PublicationDispositionGeneralOutput {
		return false, nil
	}
	return false, errors.New("case thread authority publication policy is invalid")
}

func isCurrentCaseAuthorityContext(securityContext domainsecurity.TurnSecurityContext) bool {
	return securityContext.Version == domainsecurity.TurnSecurityContextVersionV2 &&
		domainsecurity.TurnSecurityContextRequiresFinalEvidenceGate(securityContext)
}

func RequireExecutable(authority Authority, threadID string) error {
	if authority != nil && authority.RestartPreservesThreadV1(threadID) {
		return ErrRestartPreserved
	}
	if authority != nil && authority.IsCaseThread(threadID) && !authority.CanExecute(threadID) {
		return errors.New("case thread is quarantined by host authority")
	}
	return nil
}

func ValidateRestartState(authority Authority, threadID string, thread map[string]any) error {
	if authority != nil && authority.RestartPreservesThreadV1(threadID) {
		return ErrRestartPreserved
	}
	if authority == nil || !authority.IsCaseThread(threadID) {
		return nil
	}
	quarantine, reason := inspectRestartState(authority, threadID, thread)
	if quarantine {
		return errors.New(reason)
	}
	return nil
}

func PreflightRestartInventory(authority Authority, reader ThreadReader) (RestartInventory, error) {
	if authority == nil || reader == nil {
		return RestartInventory{}, errors.New("case thread restart preflight dependencies are unavailable")
	}
	threadIDs, err := reader.AllThreadIDs()
	if err != nil {
		return RestartInventory{}, err
	}
	inventory := RestartInventory{Quarantined: map[string]string{}}
	for _, threadID := range threadIDs {
		if authority.RestartPreservesThreadV1(threadID) {
			continue
		}
		if !authority.IsCaseThread(threadID) {
			continue
		}
		if !authority.CanExecute(threadID) {
			inventory.Quarantined[strings.TrimSpace(threadID)] = "case thread is quarantined by host authority"
			continue
		}
		thread, err := reader.GetThread(threadID)
		if err != nil {
			inventory.Quarantined[strings.TrimSpace(threadID)] = "case thread durable state is unavailable"
			continue
		}
		if quarantined, reason := inspectRestartState(authority, threadID, thread); quarantined {
			inventory.Quarantined[strings.TrimSpace(threadID)] = reason
		}
	}
	return inventory, nil
}

func ApplyRestartInventory(authority Authority, inventory RestartInventory) error {
	if authority == nil || inventory.Quarantined == nil {
		return errors.New("case thread restart inventory is invalid")
	}
	authority.ReplaceQuarantine(inventory.Quarantined)
	return nil
}

func NewRegistry(ctx context.Context, authority authorityport.Authority, store storeport.Store) (*Registry, error) {
	if authority == nil || store == nil {
		return nil, errors.New("case thread authority dependencies are unavailable")
	}
	registry := newRegistry(authority, store)
	records, err := store.List(ctx)
	if err != nil {
		return nil, err
	}
	if err := registry.addInventory(ctx, records); err != nil {
		return nil, err
	}
	return registry, nil
}

func newRegistry(authority authorityport.Authority, store storeport.Store) *Registry {
	return &Registry{
		authority: authority, store: store, byRecord: map[string]domainsecurity.CaseThreadAuthorityRecord{},
		byTurn: map[string]domainsecurity.CaseThreadAuthorityRecord{}, byContext: map[string]domainsecurity.CaseThreadAuthorityRecord{},
		lineage: map[string]domainsecurity.CaseThreadAuthorityRecord{}, threadIndex: map[string]map[string]bool{}, quarantine: map[string]string{},
		committed: map[string]domainsecurity.CaseThreadAuthorityRecord{}, migration: map[string]domainsecurity.TurnSecurityContext{},
		restartPreserved: &restartPreservationV1{},
	}
}

func (registry *Registry) Register(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	unlock, err := registry.lockRestartWriteV1(securityContext.ThreadID)
	if err != nil {
		return err
	}
	defer unlock()
	if registry == nil || registry.authority == nil || registry.store == nil ||
		!isCurrentCaseAuthorityContext(securityContext) {
		return errors.New("case thread authority context is invalid")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	key := caseThreadContextKey(securityContext)
	if current, found := registry.byTurn[key]; found {
		if current.SecurityContext != nil && reflect.DeepEqual(*current.SecurityContext, securityContext) {
			return nil
		}
		return errors.New("case thread authority conflicts with the frozen context")
	}
	record, err := registry.newContextRecord(ctx, securityContext)
	if err != nil {
		return err
	}
	if err := registry.store.PutIfAbsent(ctx, record); err != nil {
		return err
	}
	return registry.addRecord(record)
}

func (registry *Registry) Commit(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, state domaincontextepoch.State, committedAt time.Time) error {
	unlock, err := registry.lockRestartWriteV1(securityContext.ThreadID)
	if err != nil {
		return err
	}
	defer unlock()
	if registry == nil || registry.authority == nil || registry.store == nil || !isCurrentCaseAuthorityContext(securityContext) ||
		domaincontextepoch.ValidateState(state) != nil ||
		state.ThreadID != securityContext.ThreadID || state.AcceptedSnapshot.Epoch != securityContext.ContextEpoch {
		return errors.New("committed case turn context is invalid")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	staged, stagedOK := registry.byContext[securityContext.ContextDigest]
	if !stagedOK || staged.SecurityContext == nil || !reflect.DeepEqual(*staged.SecurityContext, securityContext) {
		return errors.New("committed case turn context lacks staged authority")
	}
	key := committedTurnKey(securityContext.ThreadID, securityContext.TurnID)
	if current, found := registry.committed[key]; found {
		if current.SecurityContext != nil && current.CommittedTurnState != nil && reflect.DeepEqual(*current.SecurityContext, securityContext) &&
			reflect.DeepEqual(current.CommittedTurnState.ContextEpochState, state) {
			return nil
		}
		return errors.New("committed case turn context conflicts with existing authority")
	}
	record, err := domainsecurity.NewCommittedTurnContextAuthorityRecord(
		securityContext, state, committedAt, registry.authority.KeyID(), registry.authority.PublicKey(),
		func(message []byte) ([]byte, error) { return registry.authority.Sign(ctx, message) },
	)
	if err != nil || registry.verifyTrusted(ctx, record) != nil {
		return errors.New("committed case turn context signing failed")
	}
	if err := registry.store.PutIfAbsent(ctx, record); err != nil {
		return err
	}
	return registry.addRecord(record)
}

func (registry *Registry) Derive(ctx context.Context, parentThreadID, threadID, derivation string) error {
	_, err := registry.DeriveWithReceipt(ctx, parentThreadID, threadID, derivation)
	return err
}

func (registry *Registry) DeriveWithReceipt(
	ctx context.Context,
	parentThreadID,
	threadID,
	derivation string,
) (domainsecurity.CaseThreadAuthorityRecord, error) {
	parentThreadID = strings.TrimSpace(parentThreadID)
	threadID = strings.TrimSpace(threadID)
	derivation = strings.TrimSpace(derivation)
	unlock, err := registry.lockRestartWriteV1(parentThreadID, threadID)
	if err != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, err
	}
	defer unlock()
	if registry == nil || parentThreadID == "" || threadID == "" || parentThreadID == threadID ||
		(derivation != "fork" && derivation != "resume") {
		return domainsecurity.CaseThreadAuthorityRecord{}, errors.New("case thread lineage input is invalid")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if len(registry.threadIndex[parentThreadID]) == 0 || strings.TrimSpace(registry.quarantine[parentThreadID]) != "" {
		return domainsecurity.CaseThreadAuthorityRecord{}, errors.New("case thread lineage parent authority is unavailable")
	}
	if existing, found := registry.lineage[threadID]; found {
		if existing.ParentThreadID == parentThreadID && existing.Derivation == derivation {
			existing.ActiveInheritedHistory = domainsecurity.CloneActiveInheritedHistoryBindingV1(existing.ActiveInheritedHistory)
			return existing, ValidateDerivedLineageReceipt(existing, parentThreadID, threadID, derivation)
		}
		return domainsecurity.CaseThreadAuthorityRecord{}, errors.New("case thread lineage conflicts with existing authority")
	}
	if len(registry.threadIndex[threadID]) != 0 {
		return domainsecurity.CaseThreadAuthorityRecord{}, errors.New("case thread lineage target already has authority")
	}
	parentDigest := firstRecordDigest(registry.threadIndex[parentThreadID])
	record, err := domainsecurity.NewCaseThreadLineageAuthorityRecord(
		threadID, parentThreadID, parentDigest, derivation, registry.authority.KeyID(), registry.authority.PublicKey(),
		func(message []byte) ([]byte, error) { return registry.authority.Sign(ctx, message) },
	)
	if err != nil || registry.verifyTrusted(ctx, record) != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, errors.New("case thread lineage signing failed")
	}
	if err := registry.store.PutIfAbsent(ctx, record); err != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, err
	}
	if err := registry.addRecord(record); err != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, err
	}
	return record, ValidateDerivedLineageReceipt(record, parentThreadID, threadID, derivation)
}

// VerifyDerivedLineageReceipt is deliberately read-only. Recovery calls it
// after a durable derivation journal claims lineage_committed so a restored or
// deleted case-lineage store cannot be silently reconstructed from the later
// journal and treated as continuous authority.
func (registry *Registry) VerifyDerivedLineageReceipt(
	ctx context.Context,
	parentThreadID,
	threadID,
	derivation,
	expectedRecordDigest string,
) (domainsecurity.CaseThreadAuthorityRecord, error) {
	if registry == nil || ctx == nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, errors.New("case thread lineage authority is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, err
	}
	parentThreadID = strings.TrimSpace(parentThreadID)
	threadID = strings.TrimSpace(threadID)
	derivation = strings.TrimSpace(derivation)
	if !domainsecurity.IsSHA256Hex(expectedRecordDigest) {
		return domainsecurity.CaseThreadAuthorityRecord{}, errors.New("case thread lineage receipt digest is invalid")
	}
	unlock, err := registry.lockRestartWriteV1(parentThreadID, threadID)
	if err != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, err
	}
	defer unlock()
	registry.mu.RLock()
	record, found := registry.lineage[threadID]
	parentAvailable := len(registry.threadIndex[parentThreadID]) != 0 &&
		strings.TrimSpace(registry.quarantine[parentThreadID]) == ""
	registry.mu.RUnlock()
	if !found || !parentAvailable ||
		ValidateDerivedLineageReceipt(record, parentThreadID, threadID, derivation) != nil ||
		record.RecordDigest != expectedRecordDigest {
		return domainsecurity.CaseThreadAuthorityRecord{}, errors.New("case thread lineage receipt is unavailable or stale")
	}
	record.ActiveInheritedHistory = domainsecurity.CloneActiveInheritedHistoryBindingV1(record.ActiveInheritedHistory)
	return record, nil
}

func ValidateDerivedLineageReceipt(
	record domainsecurity.CaseThreadAuthorityRecord,
	parentThreadID,
	threadID,
	derivation string,
) error {
	if domainsecurity.ValidateCaseThreadAuthorityRecord(record) != nil ||
		!domainsecurity.CaseThreadAuthorityIsLineage(record) ||
		record.ParentThreadID != strings.TrimSpace(parentThreadID) ||
		record.ThreadID != strings.TrimSpace(threadID) ||
		record.Derivation != strings.TrimSpace(derivation) {
		return errors.New("case thread lineage receipt is invalid")
	}
	return nil
}

func (registry *Registry) PlanContexts(ctx context.Context, contexts []domainsecurity.TurnSecurityContext) (MigrationPlan, error) {
	if registry == nil || registry.restartPreserved == nil {
		return MigrationPlan{}, errors.New("case thread authority registry is unavailable")
	}
	registry.restartPreserved.mu.RLock()
	defer registry.restartPreserved.mu.RUnlock()
	registry.mu.RLock()
	existingTurns := cloneRecords(registry.byTurn)
	existingContexts := cloneRecords(registry.byContext)
	existingCommitted := cloneRecords(registry.committed)
	registry.mu.RUnlock()
	// Validate the complete input before signing any active migration member.
	// A hold never excuses a malformed or conflicting original Core context.
	for _, frozen := range contexts {
		if !isCurrentCaseAuthorityContext(frozen) || domainsecurity.ValidateTurnSecurityContext(frozen) != nil {
			return MigrationPlan{}, errors.New("case thread migration context is invalid")
		}
		if current, found := existingCommitted[committedTurnKey(frozen.ThreadID, frozen.TurnID)]; found && (current.SecurityContext == nil || !reflect.DeepEqual(*current.SecurityContext, frozen)) {
			return MigrationPlan{}, errors.New("case thread migration conflicts with committed turn authority")
		}
		if current, found := existingTurns[caseThreadContextKey(frozen)]; found && (current.SecurityContext == nil || !reflect.DeepEqual(*current.SecurityContext, frozen)) {
			return MigrationPlan{}, errors.New("case thread migration conflicts with existing authority")
		}
		if current, found := existingContexts[frozen.ContextDigest]; found && (current.SecurityContext == nil || !reflect.DeepEqual(*current.SecurityContext, frozen)) {
			return MigrationPlan{}, errors.New("case thread migration context digest conflicts with existing authority")
		}
	}
	plan := MigrationPlan{Records: []domainsecurity.CaseThreadAuthorityRecord{}, Contexts: []domainsecurity.TurnSecurityContext{}}
	plannedContexts := map[string]bool{}
	for _, frozen := range contexts {
		if registry.restartPreserved.threads[frozen.ThreadID] {
			continue
		}
		if !plannedContexts[frozen.ContextDigest] {
			plan.Contexts = append(plan.Contexts, frozen)
			plannedContexts[frozen.ContextDigest] = true
		}
		key := caseThreadContextKey(frozen)
		if _, found := existingTurns[key]; found {
			continue
		}
		record, err := registry.newContextRecord(ctx, frozen)
		if err != nil {
			return MigrationPlan{}, err
		}
		plan.Records = append(plan.Records, record)
		existingTurns[key] = record
		existingContexts[frozen.ContextDigest] = record
	}
	sort.Slice(plan.Records, func(i, j int) bool { return plan.Records[i].RecordDigest < plan.Records[j].RecordDigest })
	sort.Slice(plan.Contexts, func(i, j int) bool {
		return caseThreadContextKey(plan.Contexts[i]) < caseThreadContextKey(plan.Contexts[j])
	})
	return plan, nil
}

func (registry *Registry) WithPlan(ctx context.Context, plan MigrationPlan) (*Registry, error) {
	unlock, err := registry.lockRestartPlanV1(plan)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if registry == nil {
		return nil, errors.New("case thread authority registry is unavailable")
	}
	registry.mu.RLock()
	records := make([]domainsecurity.CaseThreadAuthorityRecord, 0, len(registry.byRecord)+len(plan.Records))
	for _, record := range registry.byRecord {
		records = append(records, record)
	}
	quarantine := cloneStrings(registry.quarantine)
	migration := cloneContexts(registry.migration)
	activeHistoryIdentities := registry.activeHistoryIdentities
	registry.mu.RUnlock()
	if err := validateMigrationPlan(plan); err != nil {
		return nil, err
	}
	records = append(records, plan.Records...)
	overlay := newRegistry(registry.authority, registry.store)
	overlay.restartPreserved = registry.restartPreserved
	overlay.activeHistoryIdentities = activeHistoryIdentities
	if err := overlay.addInventory(ctx, records); err != nil {
		return nil, err
	}
	for _, securityContext := range plan.Contexts {
		if !isCurrentCaseAuthorityContext(securityContext) || !overlay.stagedContainsContext(securityContext) {
			return nil, errors.New("case thread migration context lacks signed authority")
		}
		if current, found := overlay.committed[committedTurnKey(securityContext.ThreadID, securityContext.TurnID)]; found &&
			(current.SecurityContext == nil || !reflect.DeepEqual(*current.SecurityContext, securityContext)) {
			return nil, errors.New("case thread migration conflicts with committed turn authority")
		}
		migration[securityContext.ContextDigest] = securityContext
	}
	overlay.migration = migration
	overlay.quarantine = quarantine
	return overlay, nil
}

func (registry *Registry) ApplyPlan(ctx context.Context, plan MigrationPlan) error {
	unlock, err := registry.lockRestartPlanV1(plan)
	if err != nil {
		return err
	}
	defer unlock()
	if registry == nil {
		return errors.New("case thread authority registry is unavailable")
	}
	if err := validateMigrationPlan(plan); err != nil {
		return err
	}
	for _, record := range plan.Records {
		if err := registry.verifyTrusted(ctx, record); err != nil {
			return err
		}
		if err := registry.store.PutIfAbsent(ctx, record); err != nil {
			return err
		}
	}
	stored, err := registry.store.List(ctx)
	if err != nil {
		return err
	}
	storedByDigest := make(map[string]domainsecurity.CaseThreadAuthorityRecord, len(stored))
	for _, record := range stored {
		storedByDigest[record.RecordDigest] = record
	}
	for _, record := range plan.Records {
		if !reflect.DeepEqual(storedByDigest[record.RecordDigest], record) {
			return errors.New("case thread authority migration readback failed")
		}
	}
	return nil
}

func validateMigrationPlan(plan MigrationPlan) error {
	contexts := make(map[string]domainsecurity.TurnSecurityContext, len(plan.Contexts))
	for _, securityContext := range plan.Contexts {
		if !isCurrentCaseAuthorityContext(securityContext) {
			return errors.New("case thread migration requires a V2 final-gated context")
		}
		if current, found := contexts[securityContext.ContextDigest]; found && !reflect.DeepEqual(current, securityContext) {
			return errors.New("case thread migration contains a conflicting context digest")
		}
		contexts[securityContext.ContextDigest] = securityContext
	}
	for _, record := range plan.Records {
		if record.SecurityContext == nil || record.CommittedTurnState != nil ||
			!isCurrentCaseAuthorityContext(*record.SecurityContext) {
			return errors.New("case thread migration record is not current staged authority")
		}
		securityContext, found := contexts[record.SecurityContext.ContextDigest]
		if !found || !reflect.DeepEqual(securityContext, *record.SecurityContext) {
			return errors.New("case thread migration record lacks its exact planned context")
		}
	}
	return nil
}

func (registry *Registry) IsCaseThread(threadID string) bool {
	if registry == nil {
		return false
	}
	registry.mu.RLock()
	trusted := len(registry.threadIndex[strings.TrimSpace(threadID)]) != 0
	registry.mu.RUnlock()
	return trusted
}

func (registry *Registry) ContainsContext(securityContext domainsecurity.TurnSecurityContext) bool {
	if registry.RestartPreservesThreadV1(securityContext.ThreadID) {
		return false
	}
	if registry == nil || !isCurrentCaseAuthorityContext(securityContext) {
		return false
	}
	registry.mu.RLock()
	record, committed := registry.committed[committedTurnKey(securityContext.ThreadID, securityContext.TurnID)]
	migrated, migration := registry.migration[securityContext.ContextDigest]
	registry.mu.RUnlock()
	return (committed && record.SecurityContext != nil && reflect.DeepEqual(*record.SecurityContext, securityContext)) ||
		(migration && reflect.DeepEqual(migrated, securityContext))
}

// stagedContainsContext is deliberately private: a prepared authority record
// is sufficient to construct a read-only migration overlay, but it is never
// publication or execution authority for a live/restarted turn.
func (registry *Registry) stagedContainsContext(securityContext domainsecurity.TurnSecurityContext) bool {
	if registry == nil {
		return false
	}
	record, found := registry.byContext[securityContext.ContextDigest]
	return found && record.SecurityContext != nil && reflect.DeepEqual(*record.SecurityContext, securityContext)
}

func (registry *Registry) CommittedContext(threadID, turnID string) (CommittedContext, bool) {
	if registry == nil {
		return CommittedContext{}, false
	}
	registry.mu.RLock()
	record, found := registry.committed[committedTurnKey(threadID, turnID)]
	registry.mu.RUnlock()
	if !found || record.SecurityContext == nil || record.CommittedTurnState == nil {
		return CommittedContext{}, false
	}
	return CommittedContext{SecurityContext: *record.SecurityContext, EpochState: record.CommittedTurnState.ContextEpochState}, true
}

func (registry *Registry) CommittedContexts() []CommittedContext {
	if registry == nil {
		return nil
	}
	registry.mu.RLock()
	contexts := make([]CommittedContext, 0, len(registry.committed))
	for _, record := range registry.committed {
		if record.SecurityContext != nil && record.CommittedTurnState != nil {
			contexts = append(contexts, CommittedContext{SecurityContext: *record.SecurityContext, EpochState: record.CommittedTurnState.ContextEpochState})
		}
	}
	registry.mu.RUnlock()
	sort.Slice(contexts, func(left, right int) bool {
		return committedTurnKey(contexts[left].SecurityContext.ThreadID, contexts[left].SecurityContext.TurnID) <
			committedTurnKey(contexts[right].SecurityContext.ThreadID, contexts[right].SecurityContext.TurnID)
	})
	return contexts
}

// CommittedTurnIDs returns the publication/execution inventory for a thread.
// ContextTurnIDs intentionally also contains durable prepared records for
// audit and migration, so destructive history operations and current-context
// resolution must use this committed-only view.
func CommittedTurnIDs(authority CommittedAuthority, threadID string) []string {
	if authority == nil {
		return nil
	}
	threadID = strings.TrimSpace(threadID)
	seen := map[string]bool{}
	for _, committed := range authority.CommittedContexts() {
		turnID := strings.TrimSpace(committed.SecurityContext.TurnID)
		if committed.SecurityContext.ThreadID == threadID && turnID != "" {
			seen[turnID] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for turnID := range seen {
		ids = append(ids, turnID)
	}
	sort.Strings(ids)
	return ids
}

func (registry *Registry) ContextTurnIDs(threadID string) []string {
	if registry == nil {
		return nil
	}
	threadID = strings.TrimSpace(threadID)
	registry.mu.RLock()
	turns := map[string]bool{}
	for _, record := range registry.byContext {
		if record.SecurityContext != nil && record.SecurityContext.ThreadID == threadID {
			turns[record.SecurityContext.TurnID] = true
		}
	}
	registry.mu.RUnlock()
	ids := make([]string, 0, len(turns))
	for turnID := range turns {
		ids = append(ids, turnID)
	}
	sort.Strings(ids)
	return ids
}

func (registry *Registry) CanExecute(threadID string) bool {
	if registry.RestartPreservesThreadV1(threadID) {
		return false
	}
	if registry == nil {
		return false
	}
	registry.mu.RLock()
	allowed := strings.TrimSpace(registry.quarantine[strings.TrimSpace(threadID)]) == ""
	registry.mu.RUnlock()
	return allowed
}

func (registry *Registry) ReplaceQuarantine(quarantine map[string]string) {
	if registry == nil {
		return
	}
	registry.mu.Lock()
	registry.quarantine = cloneStrings(quarantine)
	registry.mu.Unlock()
}

func (registry *Registry) newContextRecord(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) (domainsecurity.CaseThreadAuthorityRecord, error) {
	if lineage := registry.lineage[securityContext.ThreadID]; lineage.ActiveInheritedHistory != nil {
		for _, turn := range lineage.ActiveInheritedHistory.Turns {
			if turn.TurnID == securityContext.TurnID {
				return domainsecurity.CaseThreadAuthorityRecord{}, errors.New("inherited history identity cannot authorize target execution")
			}
		}
	}
	if !isCurrentCaseAuthorityContext(securityContext) {
		return domainsecurity.CaseThreadAuthorityRecord{}, errors.New("case thread authority requires a V2 final-gated context")
	}
	record, err := domainsecurity.NewCaseThreadAuthorityRecord(
		securityContext, registry.authority.KeyID(), registry.authority.PublicKey(),
		func(message []byte) ([]byte, error) { return registry.authority.Sign(ctx, message) },
	)
	if err != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, err
	}
	if err := registry.verifyTrusted(ctx, record); err != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, err
	}
	return record, nil
}

func (registry *Registry) addInventory(ctx context.Context, records []domainsecurity.CaseThreadAuthorityRecord) error {
	for _, record := range records {
		if err := registry.verifyTrusted(ctx, record); err != nil {
			return err
		}
		if current, found := registry.byRecord[record.RecordDigest]; found && !reflect.DeepEqual(current, record) {
			return errors.New("case thread authority inventory contains a duplicate digest")
		}
		registry.byRecord[record.RecordDigest] = record
	}
	for _, record := range records {
		if domainsecurity.CaseThreadAuthorityIsLineage(record) {
			parent, found := registry.byRecord[record.ParentRecordDigest]
			if !found || domainsecurity.CaseThreadAuthorityThreadID(parent) != strings.TrimSpace(record.ParentThreadID) {
				return errors.New("case thread lineage is detached from parent authority")
			}
		}
		if err := registry.addRecord(record); err != nil {
			return err
		}
	}
	return nil
}

func (registry *Registry) addRecord(record domainsecurity.CaseThreadAuthorityRecord) error {
	record.ActiveInheritedHistory = domainsecurity.CloneActiveInheritedHistoryBindingV1(record.ActiveInheritedHistory)
	if record.SecurityContext != nil {
		if lineage := registry.lineage[record.SecurityContext.ThreadID]; lineage.ActiveInheritedHistory != nil {
			for _, turn := range lineage.ActiveInheritedHistory.Turns {
				if turn.TurnID == record.SecurityContext.TurnID {
					return errors.New("inherited history identity cannot authorize target execution")
				}
			}
		}
	} else if record.ActiveInheritedHistory != nil {
		for _, existing := range registry.byRecord {
			if existing.SecurityContext != nil && existing.SecurityContext.ThreadID == record.ThreadID {
				for _, turn := range record.ActiveInheritedHistory.Turns {
					if turn.TurnID == existing.SecurityContext.TurnID {
						return errors.New("inherited history collides with target execution")
					}
				}
			}
		}
	}
	if current, found := registry.byRecord[record.RecordDigest]; found && !reflect.DeepEqual(current, record) {
		return errors.New("case thread authority inventory contains a conflicting digest")
	}
	threadID := domainsecurity.CaseThreadAuthorityThreadID(record)
	if record.SecurityContext != nil {
		context := *record.SecurityContext
		if context.Version != domainsecurity.TurnSecurityContextVersionV2 {
			registry.quarantine[threadID] = "legacy case thread authority is audit-only"
		}
		key := caseThreadContextKey(context)
		if current, found := registry.byTurn[key]; found && !reflect.DeepEqual(current, record) {
			if current.SecurityContext == nil || !reflect.DeepEqual(*current.SecurityContext, context) ||
				(domainsecurity.CaseThreadAuthorityIsCommittedContext(current) && domainsecurity.CaseThreadAuthorityIsCommittedContext(record)) {
				return errors.New("case thread authority inventory contains a conflicting context key")
			}
		}
		if current, found := registry.byContext[context.ContextDigest]; found && !reflect.DeepEqual(current, record) {
			if current.SecurityContext == nil || !reflect.DeepEqual(*current.SecurityContext, context) ||
				(domainsecurity.CaseThreadAuthorityIsCommittedContext(current) && domainsecurity.CaseThreadAuthorityIsCommittedContext(record)) {
				return errors.New("case thread authority inventory contains a conflicting context")
			}
		}
		if domainsecurity.CaseThreadAuthorityIsCommittedContext(record) {
			committedKey := committedTurnKey(context.ThreadID, context.TurnID)
			if current, found := registry.committed[committedKey]; found && !reflect.DeepEqual(current, record) {
				return errors.New("case thread authority inventory contains conflicting committed turn contexts")
			}
			registry.committed[committedKey] = record
			registry.byTurn[key] = record
			registry.byContext[context.ContextDigest] = record
		} else {
			if !domainsecurity.CaseThreadAuthorityIsCommittedContext(registry.byTurn[key]) {
				registry.byTurn[key] = record
			}
			if !domainsecurity.CaseThreadAuthorityIsCommittedContext(registry.byContext[context.ContextDigest]) {
				registry.byContext[context.ContextDigest] = record
			}
		}
	} else if current, found := registry.lineage[threadID]; found && !reflect.DeepEqual(current, record) {
		return errors.New("case thread authority inventory contains conflicting lineage")
	} else {
		registry.lineage[threadID] = record
	}
	registry.byRecord[record.RecordDigest] = record
	if registry.threadIndex[threadID] == nil {
		registry.threadIndex[threadID] = map[string]bool{}
	}
	registry.threadIndex[threadID][record.RecordDigest] = true
	return nil
}

func (registry *Registry) verifyTrusted(ctx context.Context, record domainsecurity.CaseThreadAuthorityRecord) error {
	keyID, publicKey, signature, err := domainsecurity.CaseThreadAuthorityMaterial(record)
	if err != nil {
		return err
	}
	if err := registry.authority.VerifyTrusted(ctx, keyID, publicKey, domainsecurity.CaseThreadAuthoritySigningBytes(record), signature); err != nil {
		return errors.Join(errors.New("case thread authority is not signed by the installation authority"), err)
	}
	return nil
}

func inspectRestartState(authority Authority, threadID string, thread map[string]any) (bool, string) {
	turnIDs := authority.ContextTurnIDs(threadID)
	if len(turnIDs) == 0 {
		if caseThreadContainsAssistantDraft(thread) {
			return true, "derived case thread contains unauthorised assistant history"
		}
		return false, ""
	}
	turns := map[string]map[string]any{}
	for _, value := range listValues(thread["turns"]) {
		turn, _ := value.(map[string]any)
		if turnID := stringValue(turn, "id"); turnID != "" {
			turns[turnID] = turn
		}
	}
	for _, turnID := range turnIDs {
		turn := turns[turnID]
		if turn == nil {
			// A prepared record can survive a signing failure or a crash before
			// the committed authority record is created. When the source thread
			// and its committed current context are still present, that record is
			// inert audit material and must not quarantine the whole Agent. A
			// missing durably committed context remains unsafe.
			if committed, ok := authority.(CommittedAuthority); ok {
				if _, found := committed.CommittedContext(threadID, turnID); !found {
					continue
				}
			}
			return true, "case thread has committed authority without a durable turn"
		}
		securityContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
		if err != nil || securityContext.ThreadID != strings.TrimSpace(threadID) || !authority.ContainsContext(securityContext) {
			return true, "case thread turn security authority is missing at restart"
		}
	}
	securityContext, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil || securityContext.ThreadID != strings.TrimSpace(threadID) || !authority.ContainsContext(securityContext) || thread["contextEpochState"] == nil {
		return true, "case thread current epoch authority is missing at restart"
	}
	return false, ""
}

func caseThreadContainsAssistantDraft(thread map[string]any) bool {
	for _, turnValue := range listValues(thread["turns"]) {
		turn, _ := turnValue.(map[string]any)
		for _, itemValue := range listValues(turn["items"]) {
			item, _ := itemValue.(map[string]any)
			switch stringValue(item, "kind") {
			case "assistant_text", "assistant_reasoning":
				return true
			}
		}
	}
	return false
}

func caseThreadContextKey(securityContext domainsecurity.TurnSecurityContext) string {
	return strings.TrimSpace(securityContext.ThreadID) + "\x00" + strings.TrimSpace(securityContext.TurnID) + "\x00" + securityContext.ContextDigest
}

func committedTurnKey(threadID, turnID string) string {
	return strings.TrimSpace(threadID) + "\x00" + strings.TrimSpace(turnID)
}

func firstRecordDigest(records map[string]bool) string {
	digests := make([]string, 0, len(records))
	for digest := range records {
		digests = append(digests, digest)
	}
	sort.Strings(digests)
	if len(digests) == 0 {
		return ""
	}
	return digests[0]
}

func cloneRecords(input map[string]domainsecurity.CaseThreadAuthorityRecord) map[string]domainsecurity.CaseThreadAuthorityRecord {
	cloned := make(map[string]domainsecurity.CaseThreadAuthorityRecord, len(input))
	for key, value := range input {
		value.ActiveInheritedHistory = domainsecurity.CloneActiveInheritedHistoryBindingV1(value.ActiveInheritedHistory)
		cloned[key] = value
	}
	return cloned
}

func cloneStrings(input map[string]string) map[string]string {
	cloned := make(map[string]string, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func cloneContexts(input map[string]domainsecurity.TurnSecurityContext) map[string]domainsecurity.TurnSecurityContext {
	cloned := make(map[string]domainsecurity.TurnSecurityContext, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func listValues(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []map[string]any:
		out := make([]any, len(typed))
		for index := range typed {
			out[index] = typed[index]
		}
		return out
	default:
		return nil
	}
}

func stringValue(value map[string]any, key string) string {
	text, _ := value[key].(string)
	return strings.TrimSpace(text)
}
