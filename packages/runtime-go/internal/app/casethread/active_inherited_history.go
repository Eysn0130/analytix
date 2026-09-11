package casethread

import (
	"context"
	"errors"
	"reflect"

	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// ActiveInheritedHistoryReaderV1 reads installation-authenticated provenance.
// It never reconstructs a record from the mutable target or parent.
type ActiveInheritedHistoryReaderV1 interface {
	ActiveInheritedHistoryRecordV1(context.Context, string) (domainsecurity.CaseThreadAuthorityRecord, bool, error)
}

type ActiveInheritedHistoryAuthorityV1 interface {
	ActiveInheritedHistoryReaderV1
	SourceAdmissionDigestV1(map[string]any) (string, error)
	DeriveWithInheritedHistoryV1(context.Context, domainsecurity.ActiveInheritedHistoryBindingV1) (domainsecurity.CaseThreadAuthorityRecord, error)
}

// SetActiveInheritedHistoryIdentityValidatorV1 is constructor-owned. The
// callback observes existing pending, child and private-final inventories.
func (registry *Registry) SetActiveInheritedHistoryIdentityValidatorV1(check func(context.Context, string, []string) error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.activeHistoryIdentities = check
}

func activeHistoryTurnIDsV1(binding *domainsecurity.ActiveInheritedHistoryBindingV1) []string {
	ids := make([]string, 0, len(binding.Turns))
	for _, turn := range binding.Turns {
		ids = append(ids, turn.TurnID)
	}
	return ids
}

func (registry *Registry) SourceAdmissionDigestV1(source map[string]any) (string, error) {
	threadID, _ := source["id"].(string)
	if registry == nil || registry.RestartPreservesThreadV1(threadID) {
		return "", ErrRestartPreserved
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	if registry.quarantine[threadID] != "" {
		return "", errors.New("inherited history source is quarantined")
	}
	if value, present := source["securityState"]; present {
		securityContext, err := domainsecurity.ParseTurnSecurityContext(value)
		record, found := registry.committed[committedTurnKey(threadID, securityContext.TurnID)]
		state, stateErr := domaincontextepoch.ParseState(source["contextEpochState"])
		if err != nil || stateErr != nil || !found || record.SecurityContext == nil || record.CommittedTurnState == nil ||
			!reflect.DeepEqual(*record.SecurityContext, securityContext) || !reflect.DeepEqual(record.CommittedTurnState.ContextEpochState, state) {
			return "", errors.New("inherited history source lacks its exact committed context")
		}
		return record.RecordDigest, nil
	}
	record, found := registry.lineage[threadID]
	if !found || record.ActiveInheritedHistory == nil || source["activeInheritedHistoryReceipt"] != record.RecordDigest {
		return "", errors.New("inherited history source admission is unavailable")
	}
	for _, value := range registry.byRecord {
		if value.SecurityContext != nil && value.SecurityContext.ThreadID == threadID {
			return "", errors.New("inherited history source omitted its execution context")
		}
	}
	return record.RecordDigest, nil
}

func (registry *Registry) DeriveWithInheritedHistoryV1(ctx context.Context, binding domainsecurity.ActiveInheritedHistoryBindingV1) (domainsecurity.CaseThreadAuthorityRecord, error) {
	if ctx == nil || registry == nil || domainsecurity.ValidateActiveInheritedHistoryBindingV1(binding) != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, errors.New("active inherited history input is invalid")
	}
	unlock, err := registry.lockRestartWriteV1(binding.SourceThreadID, binding.TargetThreadID)
	if err != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, err
	}
	defer unlock()
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, err
	}
	parent, found := registry.byRecord[binding.SourceAuthorityRecordDigest]
	if !found || domainsecurity.CaseThreadAuthorityThreadID(parent) != binding.SourceThreadID ||
		registry.quarantine[binding.SourceThreadID] != "" || registry.quarantine[binding.TargetThreadID] != "" ||
		(!domainsecurity.CaseThreadAuthorityIsCommittedContext(parent) && parent.ActiveInheritedHistory == nil) {
		return domainsecurity.CaseThreadAuthorityRecord{}, errors.New("active inherited history source authority is unavailable")
	}
	// A failed derivation consumes its target. Neither a legacy lineage nor an
	// orphan receipt may be upgraded by capturing a later mutable source.
	if len(registry.threadIndex[binding.TargetThreadID]) != 0 {
		return domainsecurity.CaseThreadAuthorityRecord{}, errors.New("active inherited history target already has authority")
	}
	records, err := registry.store.List(ctx)
	if err != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, err
	}
	if err := registry.verifyActiveHistoryAncestorsV1(ctx, parent, records); err != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, err
	}
	if registry.activeHistoryIdentities == nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, errors.New("active history identity observer is unavailable")
	}
	if err := registry.activeHistoryIdentities(ctx, binding.TargetThreadID, activeHistoryTurnIDsV1(&binding)); err != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, err
	}
	record, err := domainsecurity.NewCaseThreadLineageAuthorityRecordWithInheritedHistoryV1(
		binding.TargetThreadID, binding.SourceThreadID, binding.SourceAuthorityRecordDigest, binding.Derivation,
		binding, registry.authority.KeyID(), registry.authority.PublicKey(),
		func(body []byte) ([]byte, error) { return registry.authority.Sign(ctx, body) },
	)
	if err != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, err
	}
	if err := registry.verifyTrusted(ctx, record); err != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, err
	}
	if err := ctx.Err(); err != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, err
	}
	if err := registry.store.PutIfAbsent(ctx, record); err != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, err
	}
	if err := registry.addRecord(record); err != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, err
	}
	record.ActiveInheritedHistory = domainsecurity.CloneActiveInheritedHistoryBindingV1(record.ActiveInheritedHistory)
	return record, ctx.Err()
}

func (registry *Registry) ActiveInheritedHistoryRecordV1(ctx context.Context, threadID string) (domainsecurity.CaseThreadAuthorityRecord, bool, error) {
	if ctx == nil || registry == nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, false, errors.New("active history authority is unavailable")
	}
	if registry.RestartPreservesThreadV1(threadID) {
		return domainsecurity.CaseThreadAuthorityRecord{}, false, ErrRestartPreserved
	}
	registry.mu.RLock()
	expected, found := registry.lineage[threadID]
	quarantined := registry.quarantine[threadID] != ""
	checkIdentities := registry.activeHistoryIdentities
	registry.mu.RUnlock()
	if quarantined {
		return domainsecurity.CaseThreadAuthorityRecord{}, false, errors.New("active history target is quarantined")
	}
	if !found || expected.ActiveInheritedHistory == nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, false, nil
	}
	if checkIdentities == nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, false, errors.New("active history identity observer is unavailable")
	}
	if err := checkIdentities(ctx, threadID, activeHistoryTurnIDsV1(expected.ActiveInheritedHistory)); err != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, false, err
	}
	// Observe the existing immutable store on every admission: an in-memory
	// record alone cannot conceal a deleted or corrupt durable proof.
	records, err := registry.store.List(ctx)
	if err != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, false, err
	}
	matched := false
	for _, record := range records {
		if record.RecordDigest == expected.RecordDigest {
			if err := registry.verifyTrusted(ctx, record); err != nil {
				return domainsecurity.CaseThreadAuthorityRecord{}, false, err
			}
			matched = reflect.DeepEqual(record, expected)
		}
		if record.SecurityContext != nil && record.SecurityContext.ThreadID == threadID {
			for _, inherited := range expected.ActiveInheritedHistory.Turns {
				if inherited.TurnID == record.SecurityContext.TurnID {
					return domainsecurity.CaseThreadAuthorityRecord{}, false, errors.New("inherited turn has target execution authority")
				}
			}
		}
	}
	if !matched {
		return domainsecurity.CaseThreadAuthorityRecord{}, false, errors.New("active inherited history durable proof is missing")
	}
	if err := registry.verifyActiveHistoryAncestorsV1(ctx, expected, records); err != nil {
		return domainsecurity.CaseThreadAuthorityRecord{}, false, err
	}
	expected.ActiveInheritedHistory = domainsecurity.CloneActiveInheritedHistoryBindingV1(expected.ActiveInheritedHistory)
	return expected, true, ctx.Err()
}

func (registry *Registry) verifyActiveHistoryAncestorsV1(ctx context.Context, expected domainsecurity.CaseThreadAuthorityRecord, records []domainsecurity.CaseThreadAuthorityRecord) error {
	byDigest := make(map[string]domainsecurity.CaseThreadAuthorityRecord, len(records))
	for _, record := range records {
		if _, exists := byDigest[record.RecordDigest]; exists {
			return errors.New("active history lineage inventory is ambiguous")
		}
		byDigest[record.RecordDigest] = record
	}
	seen := map[string]bool{}
	for {
		record, found := byDigest[expected.RecordDigest]
		if !found || seen[record.RecordDigest] || !reflect.DeepEqual(record, expected) {
			return errors.New("active history lineage ancestor is missing or conflicting")
		}
		if err := registry.verifyTrusted(ctx, record); err != nil {
			return err
		}
		seen[record.RecordDigest] = true
		if record.SecurityContext != nil {
			if !domainsecurity.CaseThreadAuthorityIsCommittedContext(record) {
				return errors.New("active history lineage root is not committed")
			}
			return ctx.Err()
		}
		parent, found := byDigest[record.ParentRecordDigest]
		if !found || domainsecurity.CaseThreadAuthorityThreadID(parent) != record.ParentThreadID {
			return errors.New("active history lineage parent is missing or conflicting")
		}
		expected = parent
	}
}
