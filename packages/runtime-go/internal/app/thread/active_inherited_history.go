package thread

import (
	"context"
	"encoding/json"
	"errors"

	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

// ValidateActiveInheritedHistoryV1 proves the complete private prefix against
// the current strict primary and immutable installation-signed lineage. It
// returns only inert identities, never a public text or execution grant.
func observeActiveInheritedHistoryV1(ctx context.Context, view map[string]any, authority casethreadapp.ActiveInheritedHistoryReaderV1, primary recoveryport.PrimaryThreadReaderV1) (*domainsecurity.CaseThreadAuthorityRecord, error) {
	threadID, _ := view["id"].(string)
	record, found, err := authority.ActiveInheritedHistoryRecordV1(ctx, threadID)
	if err != nil {
		return nil, err
	}
	if !found {
		if _, present := view["activeInheritedHistoryReceipt"]; present {
			return nil, errors.New("active inherited history receipt is untrusted")
		}
		return nil, nil
	}
	if primary == nil {
		return nil, errors.New("active inherited history strict primary reader is unavailable")
	}
	snapshot, err := primary.ReadPrimaryThreadSnapshotV1(ctx, threadID)
	if err != nil {
		return nil, err
	}
	if _, err := validateActiveInheritedHistoryViewsV1(record, snapshot.Thread, view); err != nil {
		return nil, err
	}
	return cloneActiveInheritedHistoryRecordV1(&record), nil
}

func validateActiveInheritedHistoryViewsV1(record domainsecurity.CaseThreadAuthorityRecord, targets ...map[string]any) (map[string]bool, error) {
	threadID := record.ThreadID
	binding := record.ActiveInheritedHistory
	if binding == nil || domainsecurity.ValidateCaseThreadAuthorityRecord(record) != nil {
		return nil, errors.New("active inherited history record is invalid")
	}
	ids := map[string]bool{}
	for _, target := range targets {
		if target["id"] != binding.TargetThreadID || target["activeInheritedHistoryReceipt"] != record.RecordDigest ||
			target["forkedFromThreadId"] != binding.SourceThreadID || target["relation"] != binding.TargetRelation ||
			target["createdAt"] != binding.TargetCreatedAt || target["forkedAt"] != binding.TargetCreatedAt ||
			(binding.Derivation == "fork" && target["parentThreadId"] != binding.SourceThreadID) {
			return nil, errors.New("active inherited history target binding differs from signed lineage")
		}
		turns, ok := target["turns"].([]any)
		if !ok || len(turns) < len(binding.Turns) {
			return nil, errors.New("active inherited history prefix is incomplete")
		}
		seen := map[string]bool{}
		for index, raw := range turns {
			turn, ok := raw.(map[string]any)
			id, _ := turn["id"].(string)
			if !ok || id == "" || seen[id] {
				return nil, errors.New("active inherited history contains duplicate or invalid identity")
			}
			seen[id] = true
			if index >= len(binding.Turns) {
				continue
			}
			wanted := binding.Turns[index]
			if id != wanted.TurnID || ValidateCaseDerivedHistoryTurnV1(threadID, turn) != nil {
				return nil, errors.New("active inherited history ordered shape is invalid")
			}
			if _, present := turn["caseHistoryProjection"]; present {
				return nil, errors.New("active inherited history cannot carry compaction authority")
			}
			if _, present := turn["acceptedFinalView"]; present {
				return nil, errors.New("active inherited history cannot carry accepted final authority")
			}
			body, err := json.Marshal(turn)
			if err != nil || domainsecurity.SHA256Hex(body) != wanted.ContentSHA256 {
				return nil, errors.New("active inherited history content differs from signed inventory")
			}
			ids[id] = true
		}
	}
	return ids, nil
}

func (validator *currentCaseThreadAuthorityValidator) ActiveInheritedHistoryRecordV1(ctx context.Context, threadID string) (domainsecurity.CaseThreadAuthorityRecord, bool, error) {
	reader, ok := validator.authority.(casethreadapp.ActiveInheritedHistoryReaderV1)
	if !ok {
		return domainsecurity.CaseThreadAuthorityRecord{}, false, nil
	}
	return reader.ActiveInheritedHistoryRecordV1(ctx, threadID)
}

func ValidateActiveInheritedHistoryV1(ctx context.Context, view map[string]any, authority casethreadapp.ActiveInheritedHistoryReaderV1, primary recoveryport.PrimaryThreadReaderV1) (map[string]bool, error) {
	record, err := observeActiveInheritedHistoryV1(ctx, view, authority, primary)
	if err != nil || record == nil {
		return nil, err
	}
	ids := map[string]bool{}
	for _, turn := range record.ActiveInheritedHistory.Turns {
		ids[turn.TurnID] = true
	}
	return ids, nil
}

// ActiveInheritedCompactionRecordV1 observes existing provenance before a
// compaction transition. It cannot mint, replace or extend inherited history.
func ActiveInheritedCompactionRecordV1(ctx context.Context, view map[string]any, authority any, primary any) (*domainsecurity.CaseThreadAuthorityRecord, error) {
	reader, ok := authority.(casethreadapp.ActiveInheritedHistoryReaderV1)
	if !ok {
		if _, present := view["activeInheritedHistoryReceipt"]; present {
			return nil, errors.New("active inherited compaction authority is unavailable")
		}
		return nil, nil
	}
	strict, _ := primary.(recoveryport.PrimaryThreadReaderV1)
	return observeActiveInheritedHistoryV1(ctx, view, reader, strict)
}

func cloneActiveInheritedHistoryRecordV1(record *domainsecurity.CaseThreadAuthorityRecord) *domainsecurity.CaseThreadAuthorityRecord {
	if record == nil {
		return nil
	}
	cloned := *record
	cloned.ActiveInheritedHistory = domainsecurity.CloneActiveInheritedHistoryBindingV1(record.ActiveInheritedHistory)
	return &cloned
}
