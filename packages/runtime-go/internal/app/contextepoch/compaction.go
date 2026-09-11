package contextepoch

import (
	"errors"
	"strings"
	"time"

	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type CompactionAuthority struct {
	State           domaincontextepoch.State
	SecurityContext domainsecurity.TurnSecurityContext
}

const (
	CompactionModeSourceID     = "compaction-operation-mode"
	compactionModeSourceKind   = "compaction-operation"
	compactionModeAutomatic    = "automatic"
	compactionModeManual       = "manual"
	compactionModeDigestPrefix = "analytix-compaction-operation-mode:"
)

// PrepareAndAttachCompactionAuthority advances the one shared Context Epoch
// and installs a synthetic compaction-turn security context in the same
// candidate thread. Callers must pass a disposable candidate: this function
// never writes durable state, so a later baseline CAS can fail without
// changing the accepted in-memory authority.
func PrepareAndAttachCompactionAuthority(
	thread map[string]any,
	nextTurns []any,
	current domainsecurity.TurnSecurityContext,
	threadID string,
	compactionTurnID string,
	recoveryDigest string,
	at time.Time,
	caseCompactionAuthorized bool,
) (CompactionAuthority, error) {
	threadID = strings.TrimSpace(threadID)
	compactionTurnID = strings.TrimSpace(compactionTurnID)
	if thread == nil || threadID == "" || compactionTurnID == "" || len(nextTurns) == 0 ||
		domainsecurity.ValidateTurnSecurityContext(current) != nil || current.ThreadID != threadID ||
		strings.TrimSpace(compactionStringField(thread, "id")) != threadID {
		return CompactionAuthority{}, errors.New("compaction security authority input is invalid")
	}
	if current.Version != domainsecurity.TurnSecurityContextVersionV2 {
		return CompactionAuthority{}, errors.New("turn security context V1 is audit-only")
	}
	if domainsecurity.TurnSecurityContextIsBoundaryOnly(current) && !caseCompactionAuthorized {
		return CompactionAuthority{}, errors.New("case boundary-only context cannot be compacted")
	}
	persisted, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil || persisted != current {
		return CompactionAuthority{}, errors.New("compaction current security authority is invalid")
	}
	state, hasState, err := StateFromThread(thread)
	if err != nil || !hasState || state.AcceptedSnapshot.Epoch != current.ContextEpoch {
		return CompactionAuthority{}, errors.New("compaction current context epoch authority is invalid")
	}
	// PrepareTurn is used here as a pure consistency verifier. A changed result
	// means the persisted state is missing or contradicts the binding entry and
	// must not be silently repaired by compaction.
	verified, err := PrepareTurn(PrepareTurnInput{Thread: thread, SecurityContext: current, At: at})
	if err != nil || verified.Initialized || verified.Changed || verified.SecurityContext != current {
		return CompactionAuthority{}, errors.New("compaction security binding is inconsistent with the accepted epoch")
	}
	if err := ReconcileAndAttachCompaction(thread, nextTurns, threadID, compactionTurnID, recoveryDigest, at); err != nil {
		return CompactionAuthority{}, err
	}
	state, hasState, err = StateFromThread(thread)
	if err != nil || !hasState || state.AcceptedSnapshot.Epoch <= current.ContextEpoch {
		return CompactionAuthority{}, errors.New("compaction did not advance the accepted context epoch")
	}
	issuedAt := at.UTC()
	if issuedAt.IsZero() {
		issuedAt = time.Now().UTC()
	}
	next, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: compactionTurnID, WorkspaceRealPath: current.WorkspaceRealPath,
		TenantID: current.TenantID, UserID: current.UserID, CaseID: current.CaseID,
		CaseBindingHash: current.CaseBindingHash, DatasetSnapshotID: current.DatasetSnapshotID,
		SourceManifestHash: current.SourceManifestHash, ContextEpoch: state.AcceptedSnapshot.Epoch, IssuedAt: issuedAt,
		PublicationPolicy: current.PublicationPolicy, RiskAuthorityBinding: current.RiskAuthorityBinding,
	})
	if err != nil {
		return CompactionAuthority{}, err
	}
	compactTurn, ok := compactionTurnByID(nextTurns, compactionTurnID)
	if !ok {
		return CompactionAuthority{}, errors.New("compaction turn authority target is invalid")
	}
	securityRecord := turnsecurityapp.PublicRecord(next)
	compactTurn["securityContext"] = cloneMap(securityRecord)
	thread["securityState"] = cloneMap(securityRecord)
	thread["turns"] = nextTurns

	// Verify the new state/context pair without allowing this operation to
	// create another epoch. This catches a binding-state drift before CAS.
	verified, err = PrepareTurn(PrepareTurnInput{Thread: thread, SecurityContext: next, At: issuedAt})
	if err != nil || verified.Initialized || verified.Changed || verified.SecurityContext != next || verified.State.StateDigest != state.StateDigest {
		return CompactionAuthority{}, errors.New("compaction target authority is internally inconsistent")
	}
	return CompactionAuthority{State: state, SecurityContext: next}, nil
}

func compactionStringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}

func ReconcileAndAttachCompaction(thread map[string]any, nextTurns []any, threadID string, compactionTurnID string, recoveryDigest string, at time.Time) error {
	state, hasState, err := StateFromThread(thread)
	if err != nil {
		return err
	}
	if !hasState {
		baseEpoch := uint64(1)
		if securityContext, securityErr := domainsecurity.ParseTurnSecurityContext(thread["securityState"]); securityErr == nil {
			baseEpoch = securityContext.ContextEpoch
		}
		state, err = DefaultState(threadID, baseEpoch, at)
		if err != nil {
			return err
		}
	}
	compactTurn, ok := compactionTurnByID(nextTurns, compactionTurnID)
	if !ok {
		return errors.New("compaction turn epoch snapshot target is invalid")
	}
	auto, err := compactionTurnAutoMode(compactTurn)
	if err != nil {
		return err
	}
	state, _, err = Upsert(state, compactionModeSourceEntry(auto), false)
	if err != nil {
		return err
	}
	reconciled, err := Reconcile(ReconcileInput{State: state, Cause: CauseCompaction, RecoveryDigest: recoveryDigest, At: at})
	if err != nil {
		return err
	}
	thread["contextEpochState"] = PublicState(reconciled.State)
	compactTurn["contextEpochSnapshot"] = PublicSnapshot(reconciled.State.AcceptedSnapshot)
	return nil
}

// CompactionAutoMode returns the exact mode sealed into the committed context
// epoch. Crash recovery must not guess this informational distinction: the
// same signed source digest can otherwise replay to either auto=true or
// auto=false while still matching the case authority record.
func CompactionAutoMode(state domaincontextepoch.State) (bool, error) {
	if domaincontextepoch.ValidateState(state) != nil ||
		state.AcceptedSnapshot.RegistryDigest != domaincontextepoch.RegistryDigest(state.Registry) {
		return false, errors.New("compaction operation mode authority is invalid")
	}
	var found *domaincontextepoch.SourceEntry
	for index := range state.Registry {
		if state.Registry[index].SourceID != CompactionModeSourceID {
			continue
		}
		if found != nil {
			return false, errors.New("compaction operation mode authority is ambiguous")
		}
		candidate := state.Registry[index]
		found = &candidate
	}
	if found == nil {
		return false, errors.New("compaction operation mode authority is unavailable")
	}
	expectedSnapshot := domaincontextepoch.SourceSnapshotFromEntry(*found)
	accepted := false
	for _, source := range state.AcceptedSnapshot.Sources {
		if source.SourceID != CompactionModeSourceID {
			continue
		}
		if accepted || source != expectedSnapshot {
			return false, errors.New("compaction operation mode snapshot is invalid")
		}
		accepted = true
	}
	if !accepted {
		return false, errors.New("compaction operation mode snapshot is unavailable")
	}
	for _, auto := range []bool{false, true} {
		if sameSourceConfiguration(*found, compactionModeSourceEntry(auto)) {
			return auto, nil
		}
	}
	return false, errors.New("compaction operation mode authority does not match a supported mode")
}

func compactionModeSourceEntry(auto bool) domaincontextepoch.SourceEntry {
	mode := compactionModeManual
	if auto {
		mode = compactionModeAutomatic
	}
	return domaincontextepoch.SourceEntry{
		Version: domaincontextepoch.ContractVersion, SourceID: CompactionModeSourceID,
		Kind: compactionModeSourceKind, Reference: mode,
		Digest:           domaincontextepoch.SHA256Hex([]byte(compactionModeDigestPrefix + mode)),
		Sequence:         1,
		TrustState:       domaincontextepoch.TrustTrusted,
		PromptBoundary:   domaincontextepoch.BoundaryStoreOnly,
		ActivationState:  domaincontextepoch.ActivationInactive,
		ActivationReason: mode,
	}
}

func compactionTurnAutoMode(turn map[string]any) (bool, error) {
	items, ok := turn["items"].([]any)
	if !ok || len(items) != 1 {
		return false, errors.New("compaction operation mode target is invalid")
	}
	item, ok := items[0].(map[string]any)
	if !ok || strings.TrimSpace(compactionStringField(item, "kind")) != "compaction" {
		return false, errors.New("compaction operation mode item is invalid")
	}
	auto, ok := item["auto"].(bool)
	if !ok {
		return false, errors.New("compaction operation mode is unavailable")
	}
	return auto, nil
}

func compactionTurnByID(turns []any, turnID string) (map[string]any, bool) {
	turnID = strings.TrimSpace(turnID)
	var found map[string]any
	for _, raw := range turns {
		turn, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(compactionStringField(turn, "id")) != turnID {
			continue
		}
		if found != nil {
			return nil, false
		}
		found = turn
	}
	return found, found != nil
}
