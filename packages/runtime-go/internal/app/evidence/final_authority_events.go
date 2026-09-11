package evidence

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	appturn "analytix.local/runtime-go/internal/app/turn"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// ValidateAcceptedFinalEventReplay makes the committed thread authority the
// source of truth for replay. Missing, duplicated, or tampered case-final
// events fail closed until a later durable outbox repair can reconcile them.
func ValidateAcceptedFinalEventReplay(thread map[string]any, events []map[string]any) error {
	return validateAcceptedFinalEventReplayWithAuthority(thread, events, acceptedFinalReplayAuthorityV1{})
}

type acceptedFinalReplayTurnKeyV1 struct {
	threadID string
	turnID   string
}

type acceptedFinalReplayAuthorityV1 struct {
	explicit                   bool
	committed                  map[acceptedFinalReplayTurnKeyV1]string
	quarantined                map[acceptedFinalReplayTurnKeyV1]string
	validateCaseCompactionTurn func(string, map[string]any, map[string]any) error
}

func newAcceptedFinalReplayAuthorityV1(
	committed []domainevidence.PrivateAcceptedFinalRecord,
	legacyQuarantined []domainevidence.PrivateAcceptedFinalRecord,
	auditOnlyRecords []domainevidence.PrivateAcceptedFinalRecord,
) (acceptedFinalReplayAuthorityV1, error) {
	authority := acceptedFinalReplayAuthorityV1{
		explicit: true, committed: map[acceptedFinalReplayTurnKeyV1]string{},
		quarantined: map[acceptedFinalReplayTurnKeyV1]string{},
	}
	add := func(record domainevidence.PrivateAcceptedFinalRecord, target map[acceptedFinalReplayTurnKeyV1]string, quarantine, auditOnly bool) error {
		validate := domainevidence.ValidatePrivateAcceptedFinalPublicationAuthority
		if auditOnly {
			validate = domainevidence.ValidatePrivateAcceptedFinalAuditAuthority
		}
		if err := validate(record); err != nil {
			return errors.New("accepted final replay authority contains an invalid private final")
		}
		key := acceptedFinalReplayTurnKeyV1{threadID: record.SecurityContext.ThreadID, turnID: record.SecurityContext.TurnID}
		digest := record.AcceptedFinal.RecordDigest
		if existing, duplicate := target[key]; duplicate && existing != digest {
			return errors.New("accepted final replay authority contains conflicting turn records")
		}
		if existing := authority.committed[key]; existing != "" && quarantine {
			return errors.New("accepted final replay authority overlaps committed and quarantined turns")
		}
		if existing := authority.quarantined[key]; existing != "" && !quarantine {
			return errors.New("accepted final replay authority overlaps committed and quarantined turns")
		}
		target[key] = digest
		return nil
	}
	for _, record := range committed {
		if err := add(record, authority.committed, false, false); err != nil {
			return acceptedFinalReplayAuthorityV1{}, err
		}
	}
	for _, record := range legacyQuarantined {
		if err := add(record, authority.quarantined, true, true); err != nil {
			return acceptedFinalReplayAuthorityV1{}, err
		}
	}
	for _, record := range auditOnlyRecords {
		if err := add(record, authority.quarantined, true, true); err != nil {
			return acceptedFinalReplayAuthorityV1{}, err
		}
	}
	return authority, nil
}

func validateAcceptedFinalEventReplayWithAuthority(
	thread map[string]any,
	events []map[string]any,
	authority acceptedFinalReplayAuthorityV1,
) error {
	required := map[string]domainevidence.AcceptedFinalRecord{}
	quarantined := map[string]bool{}
	seenAuthority := map[acceptedFinalReplayTurnKeyV1]bool{}
	caseThread := false
	threadID := strings.TrimSpace(authorityString(thread, "id"))
	for _, turn := range authorityTurns(thread) {
		securityContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
		if turn["securityContext"] == nil {
			continue
		}
		if err != nil {
			return errors.New("event replay turn has an invalid frozen security context")
		}
		if !domainsecurity.TurnSecurityContextIsCaseSensitive(securityContext) {
			continue
		}
		caseThread = true
		if !authorityTerminalStatus(authorityString(turn, "status")) {
			continue
		}
		version, present := authoritySchemaVersion(turn["acceptedFinal"])
		if present && version == domainevidence.LegacyAcceptedFinalRecordVersion {
			return errors.New("legacy accepted final event replay requires a trusted signed cutover inventory")
		}
		if !present {
			if authority.validateCaseCompactionTurn != nil &&
				authority.validateCaseCompactionTurn(threadID, thread, turn) == nil {
				continue
			}
			return errors.New("terminal case turn has no replayable accepted-final authority")
		}
		record, err := domainevidence.ParseAcceptedFinalRecord(turn["acceptedFinal"])
		if err != nil {
			return errors.New("terminal case turn has no replayable accepted-final authority")
		}
		if authority.explicit {
			key := acceptedFinalReplayTurnKeyV1{threadID: threadID, turnID: securityContext.TurnID}
			switch {
			case authority.committed[key] == record.RecordDigest:
				seenAuthority[key] = true
			case authority.quarantined[key] == record.RecordDigest:
				seenAuthority[key] = true
				quarantined[securityContext.TurnID] = true
				continue
			default:
				return errors.New("terminal case turn lacks terminal-complete or quarantine replay authority")
			}
		}
		required[securityContext.TurnID] = record
	}
	if authority.explicit {
		for key := range authority.committed {
			if key.threadID == threadID && !seenAuthority[key] {
				return errors.New("accepted final replay authority references a missing committed turn")
			}
		}
		for key := range authority.quarantined {
			if key.threadID == threadID && !seenAuthority[key] {
				return errors.New("accepted final replay authority references a missing quarantined turn")
			}
		}
	}
	itemCounts := map[string]int{}
	terminalCounts := map[string]int{}
	for _, event := range events {
		turnID := strings.TrimSpace(authorityString(event, "turnId"))
		kind := strings.TrimSpace(authorityString(event, "kind"))
		if quarantined[turnID] {
			continue
		}
		if err := appturn.ValidateCaseEventPublication(thread, event); err != nil {
			return err
		}
		record, requiredTurn := required[turnID]
		if !requiredTurn {
			if caseThread && kind == "item_completed" {
				item, _ := event["item"].(map[string]any)
				if authorityString(item, "kind") == "assistant_text" && !authorityTurnExists(thread, turnID) {
					return errors.New("case thread event replay contains assistant text for an unknown turn")
				}
			}
			continue
		}
		switch kind {
		case "item_completed":
			item, _ := event["item"].(map[string]any)
			if authorityString(item, "kind") != "assistant_text" {
				continue
			}
			view, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(item["acceptedFinalView"])
			expected, expectedErr := domainevidence.NewAcceptedFinalPublicViewFromRecordV3(record)
			if err != nil || expectedErr != nil || item["acceptedFinal"] != nil || !reflect.DeepEqual(view, expected) {
				return errors.New("case accepted-final item event is inconsistent")
			}
			itemCounts[turnID]++
		case "turn_completed", "turn_failed", "turn_aborted":
			if authorityString(event, "acceptedFinalDigest") != record.RecordDigest {
				return errors.New("case accepted-final terminal event is inconsistent")
			}
			terminalCounts[turnID]++
		}
	}
	for turnID := range required {
		if itemCounts[turnID] != 1 || terminalCounts[turnID] != 1 {
			return fmt.Errorf("case accepted-final event replay is incomplete for turn %s", turnID)
		}
	}
	return nil
}

func authorityTurnExists(thread map[string]any, turnID string) bool {
	for _, turn := range authorityTurns(thread) {
		if authorityString(turn, "id") == strings.TrimSpace(turnID) {
			return true
		}
	}
	return false
}
