package turn

import (
	"errors"
	"strings"

	appusage "analytix.local/runtime-go/internal/app/usage"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

var generalTerminalUsageFieldsV1 = map[string]bool{
	"kind": true, "threadId": true, "turnId": true, "model": true,
	"usage": true, "cacheDiagnostics": true, "usageSource": true,
	"childRunId": true, "timestamp": true, "seq": true, "usageFinalStatus": true,
	"generalTerminalCommitId": true, "generalTerminalEventId": true,
	"generalTerminalSlot": true, "generalTerminalPayloadDigest": true,
	"generalTerminalAuthorityKind": true, "generalTerminalAuthorityDigest": true,
}

const GeneralTerminalPublicationArchiveFieldV1 = domainturnterminal.GeneralTerminalPublicationArchiveFieldV1

// NextGeneralTerminalPublicationArchiveV1 returns the exact thread-root
// archive value that must be written in the same durable CAS as the terminal
// turn. The archive carries no assistant prose and is durability metadata, not
// evidence or case-fact publication authority.
func NextGeneralTerminalPublicationArchiveV1(
	thread map[string]any,
	commitValue any,
) (map[string]any, error) {
	commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(commitValue)
	if err != nil || strings.TrimSpace(stringField(thread, "id")) != commit.ThreadID {
		return nil, errors.New("general terminal publication archive commit identity is invalid")
	}
	var archive domainturnterminal.GeneralTerminalPublicationArchiveV1
	if current, present := thread[GeneralTerminalPublicationArchiveFieldV1]; present {
		if current == nil {
			return nil, errors.New("general terminal publication archive is explicitly null")
		}
		archive, err = domainturnterminal.ParseGeneralTerminalPublicationArchiveV1(current)
	} else {
		archive, err = domainturnterminal.NewGeneralTerminalPublicationArchiveV1(nil)
	}
	if err != nil {
		return nil, err
	}
	for _, existing := range archive.Commits {
		if existing.ThreadID != commit.ThreadID {
			return nil, errors.New("general terminal publication archive contains a foreign thread commit")
		}
	}
	archive, err = domainturnterminal.AppendGeneralTerminalPublicationArchiveV1(archive, commit)
	if err != nil {
		return nil, err
	}
	return domainturnterminal.GeneralTerminalPublicationArchiveV1Map(archive), nil
}

func BuildGeneralTerminalPublicationCommitV1(
	securityContext domainsecurity.TurnSecurityContext,
	binding domainturnterminal.GeneralTerminalCASBindingV1,
	record CompletionRecord,
	committedAt string,
	includeAssistant bool,
) (domainturnterminal.GeneralTerminalPublicationCommitV1, error) {
	return BuildGeneralTerminalPublicationCommitForEventsV1(
		securityContext, binding, record.ItemCompletedEvent, record.UsageEvent,
		record.TurnCompletedEvent, committedAt, includeAssistant,
	)
}

func BuildGeneralTerminalPublicationCommitForEventsV1(
	securityContext domainsecurity.TurnSecurityContext,
	binding domainturnterminal.GeneralTerminalCASBindingV1,
	itemCompletedEvent map[string]any,
	usageEvent map[string]any,
	terminalEvent map[string]any,
	committedAt string,
	includeTerminalItem bool,
) (domainturnterminal.GeneralTerminalPublicationCommitV1, error) {
	drafts := make([]domainturnterminal.GeneralTerminalPublicationDraftV1, 0, 3)
	if includeTerminalItem {
		drafts = append(drafts, domainturnterminal.GeneralTerminalPublicationDraftV1{
			Slot: "terminal-item", Draft: contracts.CloneMap(itemCompletedEvent),
		})
	}
	drafts = append(drafts,
		domainturnterminal.GeneralTerminalPublicationDraftV1{Slot: "usage", Draft: contracts.CloneMap(usageEvent)},
		domainturnterminal.GeneralTerminalPublicationDraftV1{Slot: "terminal", Draft: contracts.CloneMap(terminalEvent)},
	)
	for _, candidate := range drafts {
		if err := domainevent.ValidatePublicRecord(candidate.Draft); err != nil {
			return domainturnterminal.GeneralTerminalPublicationCommitV1{}, err
		}
		if candidate.Slot == "usage" {
			if err := validateGeneralTerminalTelemetryMapsV1(candidate.Draft); err != nil {
				return domainturnterminal.GeneralTerminalPublicationCommitV1{}, err
			}
		}
	}
	return domainturnterminal.NewGeneralTerminalPublicationCommitV1(securityContext, binding, committedAt, drafts)
}

// ValidateGeneralTerminalPublicationCommitForThreadV1 is the canonical CAS
// readback check for an ordinary terminal outbox. The commit is an internal
// durability authority only; it is never evidence or case-fact authority.
func ValidateGeneralTerminalPublicationCommitForThreadV1(
	thread map[string]any,
	turnID string,
	commit domainturnterminal.GeneralTerminalPublicationCommitV1,
) error {
	if domainturnterminal.ValidateGeneralTerminalPublicationCommitV1(commit) != nil ||
		strings.TrimSpace(stringField(thread, "id")) != commit.ThreadID || strings.TrimSpace(turnID) != commit.TurnID {
		return errors.New("general terminal publication commit identity is invalid")
	}
	turn, found := securityTurnByID(thread, commit.TurnID)
	if !found || stringField(turn, "status") != commit.TerminalStatus || turn["acceptedFinal"] != nil ||
		strings.TrimSpace(stringField(turn, "finishedAt")) != commit.CommittedAt {
		return errors.New("general terminal publication commit has no matching terminal turn")
	}
	turnContext, turnErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	binding, bindingErr := domainturnterminal.ParseGeneralTerminalCASBindingV1(turn["generalTerminalCASBinding"])
	if turnErr != nil || bindingErr != nil || !domainsecurity.TurnSecurityContextIsGeneral(turnContext) ||
		turnContext.ThreadID != commit.ThreadID || turnContext.TurnID != commit.TurnID ||
		turnContext.ContextDigest != commit.ContextDigest || turnContext.ContextEpoch != commit.ContextEpoch ||
		turnContext.DatasetSnapshotID != commit.DatasetSnapshotID || binding.BindingDigest != commit.AuthorityDigest {
		return errors.New("general terminal publication commit is stale or detached")
	}
	storedCommit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(turn["generalTerminalPublication"])
	if err != nil || storedCommit.CommitDigest != commit.CommitDigest {
		return errors.New("general terminal publication commit is not the canonical turn winner")
	}
	if err := validateGeneralTerminalItemWinnerV1(turn, turnContext, binding, commit); err != nil {
		return err
	}
	drafts, err := generalTerminalPublicationEventDraftsV1(turn, commit)
	if err != nil || len(drafts) != len(commit.Events) {
		return errors.New("general terminal publication event reconstruction failed")
	}
	for index, draft := range drafts {
		publicationEvent := commit.Events[index]
		if err := domainevent.ValidatePublicRecord(draft); err != nil ||
			stringField(draft, "threadId") != commit.ThreadID || stringField(draft, "turnId") != commit.TurnID ||
			stringField(draft, "timestamp") != commit.CommittedAt ||
			stringField(draft, "generalTerminalCommitId") != commit.CommitID ||
			stringField(draft, "generalTerminalEventId") != publicationEvent.EventID ||
			stringField(draft, "generalTerminalSlot") != publicationEvent.Slot ||
			stringField(draft, "generalTerminalPayloadDigest") != publicationEvent.PayloadDigest ||
			domainturnterminal.GeneralTerminalPublicationPayloadDigestV1(draft) != publicationEvent.PayloadDigest {
			return errors.New("general terminal publication event identity is invalid")
		}
		switch publicationEvent.Slot {
		case "terminal-item":
			if err := validateGeneralTerminalItemPublicationV1(turn, draft, commit); err != nil {
				return errors.New("general terminal item publication is detached")
			}
		case "usage":
			if err := validateGeneralTerminalUsageEventV1(draft, commit); err != nil {
				return err
			}
		case "terminal":
			expectedKind, _ := domainturnterminal.GeneralTerminalLifecycleEventKindV1(commit.TerminalStatus)
			if stringField(draft, "kind") != expectedKind || stringField(draft, "status") != commit.TerminalStatus ||
				stringField(draft, "terminalReason") != commit.TerminalReason ||
				stringField(draft, "generalTerminalCASBindingDigest") != binding.BindingDigest {
				return errors.New("general terminal lifecycle publication is invalid")
			}
		default:
			return errors.New("general terminal publication slot is unknown")
		}
	}
	return nil
}

// ValidateGeneralTerminalPublicationUpdateV1 validates the exact prospective
// CAS winner before it is written. This closes the terminal-without-outbox
// crash gap: a general completed turn cannot commit only its binding or only
// its event manifest.
func ValidateGeneralTerminalPublicationUpdateV1(
	thread map[string]any,
	turnID string,
	status string,
	items []map[string]any,
	fields map[string]any,
) error {
	bindingValue, hasBinding := fields["generalTerminalCASBinding"]
	commitValue, hasCommit := fields["generalTerminalPublication"]
	if !hasBinding || !hasCommit {
		return errors.New("general terminal update requires binding and publication outbox")
	}
	if _, err := domainturnterminal.ParseGeneralTerminalCASBindingV1(bindingValue); err != nil {
		return errors.New("general terminal update binding is invalid")
	}
	commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(commitValue)
	if err != nil || commit.TurnID != strings.TrimSpace(turnID) || commit.TerminalStatus != strings.TrimSpace(status) {
		return errors.New("general terminal update outbox is invalid")
	}
	turn, found := securityTurnByID(thread, strings.TrimSpace(turnID))
	frozen, frozenErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	current, currentErr := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if !found || frozenErr != nil || currentErr != nil || current != frozen ||
		!domainsecurity.TurnSecurityContextIsGeneral(current) || current.ThreadID != strings.TrimSpace(stringField(thread, "id")) ||
		current.TurnID != strings.TrimSpace(turnID) {
		return errors.New("general terminal update is stale")
	}
	prospective := ApplyTerminalUpdate(TerminalUpdateInput{
		Thread: thread, TurnID: turnID, Status: status, ProtectTerminal: true,
		AppendItems: items, Fields: fields, FinishedAt: commit.CommittedAt,
	})
	if !prospective.Found || !prospective.Applied {
		return errors.New("general terminal update has no active CAS target")
	}
	return ValidateGeneralTerminalPublicationCommitForThreadV1(prospective.Thread, turnID, commit)
}

func PrepareGeneralTerminalEventBundleV1(
	thread map[string]any,
	turnID string,
	commit domainturnterminal.GeneralTerminalPublicationCommitV1,
	nextSeq int,
) ([]map[string]any, error) {
	if nextSeq <= 0 || ValidateGeneralTerminalPublicationCommitForThreadV1(thread, turnID, commit) != nil {
		return nil, errors.New("general terminal event bundle input is invalid")
	}
	drafts, err := generalTerminalPublicationEventDraftsV1(mustSecurityTurnByID(thread, turnID), commit)
	if err != nil {
		return nil, err
	}
	events := make([]map[string]any, 0, len(drafts))
	for offset, draft := range drafts {
		if err := domainevent.ValidatePublicRecord(draft); err != nil {
			return nil, err
		}
		projectedValue, _ := domainevent.ProjectPublicValuePreservingClosedPlanDigestV1(draft)
		projected, ok := projectedValue.(map[string]any)
		if !ok || projected == nil {
			return nil, errors.New("general terminal event projection is unavailable")
		}
		projected, err = validateCaseEventOrdinaryProjection(projected)
		if err != nil {
			return nil, err
		}
		event := contracts.CloneMap(projected)
		event["seq"] = float64(nextSeq + offset)
		publicationEvent := commit.Events[offset]
		if stringField(event, "generalTerminalCommitId") != commit.CommitID ||
			stringField(event, "generalTerminalEventId") != publicationEvent.EventID ||
			stringField(event, "generalTerminalPayloadDigest") != publicationEvent.PayloadDigest ||
			domainturnterminal.GeneralTerminalPublicationPayloadDigestV1(event) != publicationEvent.PayloadDigest {
			return nil, errors.New("general terminal event changed during projection")
		}
		events = append(events, event)
	}
	return events, nil
}

func generalTerminalPublicationEventDraftsV1(
	turn map[string]any,
	commit domainturnterminal.GeneralTerminalPublicationCommitV1,
) ([]map[string]any, error) {
	baseDrafts := make([]map[string]any, 0, len(commit.Events))
	if commit.TerminalItemID != "" {
		item, found := turnItemByID(turn, commit.TerminalItemID)
		if !found {
			return nil, errors.New("general terminal item is missing")
		}
		event := itemCompletedEvent(item)
		event["timestamp"] = commit.CommittedAt
		baseDrafts = append(baseDrafts, event)
	}
	baseDrafts = append(baseDrafts, contracts.CloneMap(commit.UsageEvent), contracts.CloneMap(commit.TerminalEvent))
	if len(baseDrafts) != len(commit.Events) {
		return nil, errors.New("general terminal publication manifest shape is invalid")
	}
	drafts := make([]map[string]any, 0, len(baseDrafts))
	for index, base := range baseDrafts {
		draft, err := domainturnterminal.GeneralTerminalPublicationEventDraftV1(commit, commit.Events[index].Slot, base)
		if err != nil {
			return nil, err
		}
		drafts = append(drafts, draft)
	}
	return drafts, nil
}

func validateGeneralTerminalItemWinnerV1(
	turn map[string]any,
	context domainsecurity.TurnSecurityContext,
	binding domainturnterminal.GeneralTerminalCASBindingV1,
	commit domainturnterminal.GeneralTerminalPublicationCommitV1,
) error {
	terminalItems := make([]map[string]any, 0, 1)
	assistantItems := 0
	items, _ := turn["items"].([]any)
	for _, value := range items {
		item, _ := value.(map[string]any)
		if stringField(item, "kind") == "assistant_text" {
			assistantItems++
		}
		if item["generalTerminalCASBinding"] != nil {
			terminalItems = append(terminalItems, item)
		}
	}
	if commit.TerminalItemID == "" {
		if assistantItems != 0 || len(terminalItems) != 0 ||
			domainturnterminal.ValidateGeneralTerminalCASBindingForOutcomeV1(
				binding, context, "", commit.TerminalReason, commit.TerminalStatus,
			) != nil {
			return errors.New("general terminal empty item winner is invalid")
		}
		return nil
	}
	if len(terminalItems) != 1 || stringField(terminalItems[0], "id") != commit.TerminalItemID {
		return errors.New("general terminal item winner is not unique")
	}
	item := terminalItems[0]
	itemBinding, err := domainturnterminal.ParseGeneralTerminalCASBindingV1(item["generalTerminalCASBinding"])
	text, textOK := domainturnterminal.GeneralTerminalItemTextV1(item)
	if err != nil || itemBinding.BindingDigest != binding.BindingDigest ||
		!textOK || strings.TrimSpace(stringField(item, "finishedAt")) != commit.CommittedAt ||
		domainturnterminal.ValidateGeneralTerminalCASBindingForOutcomeV1(
			binding, context, text, commit.TerminalReason, commit.TerminalStatus,
		) != nil {
		return errors.New("general terminal item winner is detached from its binding")
	}
	if stringField(item, "kind") == "assistant_text" && assistantItems != 1 ||
		stringField(item, "kind") != "assistant_text" && assistantItems != 0 {
		return errors.New("general terminal assistant item set contradicts the terminal outcome")
	}
	return nil
}

func validateGeneralTerminalItemPublicationV1(
	turn map[string]any,
	event map[string]any,
	commit domainturnterminal.GeneralTerminalPublicationCommitV1,
) error {
	item, ok := event["item"].(map[string]any)
	canonicalItem, found := turnItemByID(turn, commit.TerminalItemID)
	if stringField(event, "kind") != "item_completed" || !ok || !found ||
		stringField(event, "itemId") != commit.TerminalItemID ||
		stringField(item, "id") != commit.TerminalItemID ||
		!canonicalJSONEqual(item, canonicalItem) {
		return errors.New("general terminal item publication is not canonical")
	}
	return nil
}

func validateGeneralTerminalUsageEventV1(event map[string]any, commit domainturnterminal.GeneralTerminalPublicationCommitV1) error {
	for key := range event {
		if !generalTerminalUsageFieldsV1[key] {
			return errors.New("general terminal usage publication has an unknown field")
		}
	}
	if stringField(event, "kind") != "usage" || stringField(event, "threadId") != commit.ThreadID ||
		stringField(event, "turnId") != commit.TurnID || stringField(event, "timestamp") != commit.CommittedAt {
		return errors.New("general terminal usage publication is invalid")
	}
	return validateGeneralTerminalTelemetryMapsV1(event)
}

func validateGeneralTerminalTelemetryMapsV1(event map[string]any) error {
	usageMap, usageOK := event["usage"].(map[string]any)
	diagnosticsMap, diagnosticsOK := event["cacheDiagnostics"].(map[string]any)
	if !usageOK || !diagnosticsOK ||
		!appusage.ValidateTerminalTelemetryPublicMapsV1(usageMap, diagnosticsMap) {
		return errors.New("general terminal usage telemetry is not a closed host projection")
	}
	return nil
}

func mustSecurityTurnByID(thread map[string]any, turnID string) map[string]any {
	turn, _ := securityTurnByID(thread, turnID)
	return turn
}
