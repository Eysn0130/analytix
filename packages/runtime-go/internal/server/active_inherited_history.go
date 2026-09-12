package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

// The production composition supplies public/current admission; capture runs
// while the existing durable mutation mutex owns both source and new target.
func (s *DurableEventSessionStore) SetActiveHistorySourceAdmissionV1(admit func(map[string]any) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeHistorySourceAdmission = admit
}

func (s *DurableEventSessionStore) activeHistorySourceNoLockV1(ctx context.Context, threadID string) (recoveryport.PrimaryThreadSnapshotV1, error) {
	if s.activeHistorySourceAdmission == nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, errors.New("active history source admission is unavailable")
	}
	snapshot, err := s.ReadPrimaryThreadSnapshotV1(ctx, threadID)
	if err != nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, err
	}
	if err := s.activeHistorySourceAdmission(snapshot.Thread); err != nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, err
	}
	return snapshot, nil
}

func (s *DurableEventSessionStore) bindActiveHistoryNoLockV1(ctx context.Context, source recoveryport.PrimaryThreadSnapshotV1, target map[string]any, derivation string) error {
	authority, ok := s.caseThreads.(casethreadapp.ActiveInheritedHistoryAuthorityV1)
	if !ok {
		return errors.New("active history lineage authority is unavailable")
	}
	targetID := stringField(target, "id")
	if _, err := os.Lstat(s.threadDir(targetID)); !errors.Is(err, os.ErrNotExist) {
		return errors.New("active history target identity already exists or is unavailable")
	}
	digest, err := authority.SourceAdmissionDigestV1(source.Thread)
	if err != nil {
		return err
	}
	turns, ok := target["turns"].([]any)
	sourceTurns, sourceOK := source.Thread["turns"].([]any)
	if !ok || !sourceOK || len(turns) > len(sourceTurns) {
		return errors.New("active history source/target turn inventory is invalid")
	}
	binding := domainsecurity.ActiveInheritedHistoryBindingV1{
		SchemaVersion: 1, Purpose: domainsecurity.ActiveInheritedHistoryPurposeV1,
		SourceThreadID: source.ThreadID, SourcePrimarySHA256: source.ThreadFileSHA256, SourceAuthorityRecordDigest: digest,
		TargetThreadID: targetID, Derivation: derivation, SourceTurnCount: len(sourceTurns),
		TargetCreatedAt: stringField(target, "createdAt"), TargetRelation: stringField(target, "relation"),
		Turns: make([]domainsecurity.ActiveInheritedTurnV1, 0, len(turns)),
	}
	for index, value := range turns {
		turn, ok := value.(map[string]any)
		original, originalOK := sourceTurns[index].(map[string]any)
		if !ok || !originalOK || stringField(turn, "id") != stringField(original, "id") || threadapp.ValidateCaseDerivedHistoryTurnV1(targetID, turn) != nil {
			return errors.New("active history derivation inventory is invalid")
		}
		if _, present := turn["caseHistoryProjection"]; present {
			return errors.New("active history retained source compaction authority")
		}
		if _, present := turn["acceptedFinalView"]; present {
			return errors.New("active history retained source accepted final authority")
		}
		body, err := json.Marshal(turn)
		if err != nil {
			return err
		}
		binding.Turns = append(binding.Turns, domainsecurity.ActiveInheritedTurnV1{TurnID: stringField(turn, "id"), ContentSHA256: domainsecurity.SHA256Hex(body)})
		binding.CutoffTurnID = stringField(original, "id")
	}
	binding.InventoryDigest = domainsecurity.ActiveInheritedHistoryInventoryDigestV1(binding.Turns)
	if target["forkedFromThreadId"] != source.ThreadID || (derivation == "fork" && target["parentThreadId"] != source.ThreadID) {
		return errors.New("active history source differs from requested parent")
	}
	// Ensure the regular durable writer will not alter authenticated history.
	normalized, err := threadapp.NormalizeForRead(targetID, target, binding.TargetCreatedAt)
	if err != nil {
		return err
	}
	before, err := json.Marshal(turns)
	if err != nil {
		return err
	}
	after, err := json.Marshal(normalized["turns"])
	if err != nil || string(before) != string(after) {
		return errors.New("active history persistence would change inherited bytes")
	}
	record, err := authority.DeriveWithInheritedHistoryV1(ctx, binding)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	target["activeInheritedHistoryReceipt"] = record.RecordDigest
	return nil
}

// BindPrimaryThreadReaderV1 binds the composition-owned anchored reader once,
// before the store is exposed. Reads may occur under the mutation mutex, so
// the immutable reader must not reenter that lock.
func (s *DurableEventSessionStore) BindPrimaryThreadReaderV1(reader recoveryport.PrimaryThreadReaderV1) error {
	if s == nil || reader == nil {
		return errors.New("primary thread reader is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.primaryReader != nil {
		return errors.New("primary thread reader is already bound")
	}
	s.primaryReader = reader
	return nil
}

func (s *DurableEventSessionStore) ReadPrimaryThreadSnapshotV1(ctx context.Context, threadID string) (recoveryport.PrimaryThreadSnapshotV1, error) {
	if s == nil || s.primaryReader == nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, errors.New("primary thread reader is unavailable")
	}
	return s.primaryReader.ReadPrimaryThreadSnapshotV1(ctx, threadID)
}

func (s *DurableEventSessionStore) ReadCommittedEventLogSHA256V1(ctx context.Context, threadID string) (string, error) {
	if s == nil || s.primaryReader == nil {
		return "", errors.New("primary thread reader is unavailable")
	}
	return s.primaryReader.ReadCommittedEventLogSHA256V1(ctx, threadID)
}

var _ recoveryport.PrimaryThreadReaderV1 = (*DurableEventSessionStore)(nil)
