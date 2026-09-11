package runtimeapp

import (
	"errors"
	"reflect"

	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	turnterminalapp "analytix.local/runtime-go/internal/app/turnterminal"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

func terminalCompleteFinalAuthorityV1(inventory evidenceapp.FinalAuthorityInventory, recovery turnterminalapp.RestartRecoveryResultV1) ([]domainevidence.PrivateAcceptedFinalRecord, []domainevidence.AcceptedFinalDispositionRecord, error) {
	completeByDigest := make(map[string]turnterminalapp.CommitResultV1, len(recovery.Complete))
	for _, committed := range recovery.Complete {
		digest := committed.AcceptedFinalDisposition.AcceptedFinalDigest
		if digest == "" || committed.Persistence.AcceptedFinal.RecordDigest != digest ||
			domainturnterminal.ValidateTurnTerminalDispositionForAuthoritiesV1(
				committed.TerminalDisposition, committed.Intent, committed.ProviderClosure, committed.AcceptedFinalDisposition,
			) != nil {
			return nil, nil, errors.New("terminal recovery complete result is invalid")
		}
		if _, duplicate := completeByDigest[digest]; duplicate {
			return nil, nil, errors.New("terminal recovery complete result is duplicated")
		}
		completeByDigest[digest] = committed
	}
	legacyByDigest := make(map[string]bool, len(recovery.LegacyQuarantined))
	for _, record := range recovery.LegacyQuarantined {
		digest := record.AcceptedFinal.RecordDigest
		if digest == "" || legacyByDigest[digest] || completeByDigest[digest].Intent.IntentID != "" {
			return nil, nil, errors.New("terminal recovery legacy quarantine is invalid")
		}
		legacyByDigest[digest] = true
	}
	auditByDigest := make(map[string]domainevidence.PrivateAcceptedFinalRecord, len(recovery.AuditOnly))
	for _, record := range recovery.AuditOnly {
		digest := record.AcceptedFinal.RecordDigest
		if _, duplicate := auditByDigest[digest]; digest == "" || duplicate || legacyByDigest[digest] || completeByDigest[digest].Intent.IntentID != "" {
			return nil, nil, errors.New("terminal recovery audit-only inventory is invalid")
		}
		auditByDigest[digest] = record
	}
	nonExecutableAuditByDigest := make(map[string]domainevidence.PrivateAcceptedFinalRecord, len(recovery.NonExecutableAuditOnly))
	for _, record := range recovery.NonExecutableAuditOnly {
		digest := record.AcceptedFinal.RecordDigest
		if _, duplicate := nonExecutableAuditByDigest[digest]; digest == "" || duplicate || legacyByDigest[digest] ||
			completeByDigest[digest].Intent.IntentID != "" || auditByDigest[digest].AcceptedFinal.RecordDigest != "" {
			return nil, nil, errors.New("terminal recovery non-executable audit inventory is invalid")
		}
		nonExecutableAuditByDigest[digest] = record
	}
	preservedByDigest := make(map[string]domainevidence.PrivateAcceptedFinalRecord, len(recovery.Preserved))
	for _, record := range recovery.Preserved {
		digest := record.AcceptedFinal.RecordDigest
		if _, duplicate := preservedByDigest[digest]; digest == "" || duplicate || legacyByDigest[digest] ||
			completeByDigest[digest].Intent.IntentID != "" || auditByDigest[digest].AcceptedFinal.RecordDigest != "" ||
			nonExecutableAuditByDigest[digest].AcceptedFinal.RecordDigest != "" {
			return nil, nil, errors.New("terminal recovery preserved inventory is invalid")
		}
		preservedByDigest[digest] = record
	}
	seenPreserved := map[string]bool{}
	observePreserved := func(record domainevidence.PrivateAcceptedFinalRecord) (bool, error) {
		digest := record.AcceptedFinal.RecordDigest
		original, held := preservedByDigest[digest]
		if !held {
			return false, nil
		}
		if seenPreserved[digest] || !reflect.DeepEqual(original, record) {
			return false, errors.New("terminal recovery preserved original changed or is duplicated")
		}
		seenPreserved[digest] = true
		return true, nil
	}
	for _, repair := range inventory.PublicCommitRepairs {
		held, err := observePreserved(repair.PrivateRecord)
		if err != nil || !held {
			return nil, nil, errors.Join(errors.New("terminal recovery left an unresolved public commit"), err)
		}
	}
	seenAudit := map[string]bool{}
	for _, record := range inventory.NotCommitted {
		if held, err := observePreserved(record); err != nil {
			return nil, nil, err
		} else if held {
			continue
		}
		digest := record.AcceptedFinal.RecordDigest
		auditRecord, found := auditByDigest[digest]
		if !found || !reflect.DeepEqual(auditRecord, record) {
			return nil, nil, errors.New("not-committed accepted final lacks audit-only terminal classification")
		}
		seenAudit[digest] = true
	}
	if len(seenAudit) != len(auditByDigest) {
		return nil, nil, errors.New("terminal recovery audit-only classification exceeds final authority inventory")
	}
	seenNonExecutableAudit := map[string]bool{}
	for _, record := range append(
		append([]domainevidence.PrivateAcceptedFinalRecord{}, inventory.AuditOnlyPublicWinners...),
		inventory.AuditOnlyNotCommitted...,
	) {
		if held, err := observePreserved(record); err != nil {
			return nil, nil, err
		} else if held {
			continue
		}
		digest := record.AcceptedFinal.RecordDigest
		auditRecord, found := nonExecutableAuditByDigest[digest]
		if !found || seenNonExecutableAudit[digest] || !reflect.DeepEqual(auditRecord, record) {
			return nil, nil, errors.New("final authority audit-only record lacks non-executable terminal classification")
		}
		seenNonExecutableAudit[digest] = true
	}
	if len(seenNonExecutableAudit) != len(nonExecutableAuditByDigest) {
		return nil, nil, errors.New("terminal non-executable audit classification exceeds final authority inventory")
	}
	dispositionByDigest := make(map[string]domainevidence.AcceptedFinalDispositionRecord, len(inventory.CommittedDispositions))
	for _, disposition := range inventory.CommittedDispositions {
		if _, duplicate := dispositionByDigest[disposition.AcceptedFinalDigest]; duplicate {
			return nil, nil, errors.New("terminal-complete accepted-final disposition is duplicated")
		}
		dispositionByDigest[disposition.AcceptedFinalDigest] = disposition
	}
	records := make([]domainevidence.PrivateAcceptedFinalRecord, 0, len(completeByDigest))
	dispositions := make([]domainevidence.AcceptedFinalDispositionRecord, 0, len(completeByDigest))
	seenComplete := map[string]bool{}
	seenLegacy := map[string]bool{}
	for _, record := range inventory.Committed {
		if held, err := observePreserved(record); err != nil {
			return nil, nil, err
		} else if held {
			continue
		}
		digest := record.AcceptedFinal.RecordDigest
		if committed, ok := completeByDigest[digest]; ok {
			disposition, found := dispositionByDigest[digest]
			if !found || !reflect.DeepEqual(disposition, committed.AcceptedFinalDisposition) {
				return nil, nil, errors.New("terminal-complete final authority disposition changed after recovery")
			}
			records = append(records, record)
			dispositions = append(dispositions, disposition)
			seenComplete[digest] = true
			continue
		}
		if legacyByDigest[digest] {
			seenLegacy[digest] = true
			continue
		}
		return nil, nil, errors.New("committed accepted final lacks terminal-complete authority")
	}
	if len(seenComplete) != len(completeByDigest) || len(seenLegacy) != len(legacyByDigest) || len(seenPreserved) != len(preservedByDigest) {
		return nil, nil, errors.New("terminal recovery inventory is not closed over final authority")
	}
	return records, dispositions, nil
}

func terminalCompleteProjectionAuthoritiesV1(
	records []domainevidence.PrivateAcceptedFinalRecord,
	recovery turnterminalapp.RestartRecoveryResultV1,
) ([]gateprojection.TerminalCompleteFinalAuthorityV1, error) {
	completeByDigest := make(map[string]turnterminalapp.CommitResultV1, len(recovery.Complete))
	for _, complete := range recovery.Complete {
		digest := complete.AcceptedFinalDisposition.AcceptedFinalDigest
		if digest == "" || complete.Persistence.AcceptedFinal.RecordDigest != digest {
			return nil, errors.New("terminal projection recovery result is invalid")
		}
		if _, duplicate := completeByDigest[digest]; duplicate {
			return nil, errors.New("terminal projection recovery result is duplicated")
		}
		completeByDigest[digest] = complete
	}
	authorities := make([]gateprojection.TerminalCompleteFinalAuthorityV1, 0, len(records))
	for _, record := range records {
		digest := record.AcceptedFinal.RecordDigest
		complete, found := completeByDigest[digest]
		if !found {
			return nil, errors.New("terminal projection record lacks complete authority")
		}
		authorities = append(authorities, gateprojection.TerminalCompleteFinalAuthorityV1{
			PrivateFinal: record, Intent: complete.Intent, ProviderClosure: complete.ProviderClosure,
			PublicObservation:        complete.PublicObservation,
			AcceptedFinalDisposition: complete.AcceptedFinalDisposition,
			TerminalDisposition:      complete.TerminalDisposition,
		})
		delete(completeByDigest, digest)
	}
	if len(completeByDigest) != 0 {
		return nil, errors.New("terminal projection authority is not closed over committed records")
	}
	return authorities, nil
}
