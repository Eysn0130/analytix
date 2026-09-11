package turn

import (
	"errors"
	"sort"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

const (
	GeneralTerminalPublicationMissingV1  = "missing"
	GeneralTerminalPublicationCompleteV1 = "complete"
)

var generalTerminalPublicationMarkerNamesV1 = map[string]bool{
	"generalTerminalCommitId":        true,
	"generalTerminalEventId":         true,
	"generalTerminalSlot":            true,
	"generalTerminalPayloadDigest":   true,
	"generalTerminalAuthorityKind":   true,
	"generalTerminalAuthorityDigest": true,
}

type GeneralTerminalPublicationInventoryEntryV1 struct {
	TurnID       string
	Commit       domainturnterminal.GeneralTerminalPublicationCommitV1
	State        string
	Events       []map[string]any
	ArchivedOnly bool
}

// PreflightGeneralTerminalPublicationInventoryV1 validates every ordinary
// terminal marker in one thread before any recovery write. A committed outbox
// is either wholly absent or present as one exact contiguous bundle; partial,
// duplicated, unknown, or cross-commit markers fail closed.
func PreflightGeneralTerminalPublicationInventoryV1(
	thread map[string]any,
	events []map[string]any,
) ([]GeneralTerminalPublicationInventoryEntryV1, error) {
	threadID := strings.TrimSpace(stringField(thread, "id"))
	if threadID == "" {
		return nil, errors.New("general terminal inventory thread identity is invalid")
	}
	entries := make([]GeneralTerminalPublicationInventoryEntryV1, 0)
	byCommit := map[string]int{}
	byTurn := map[string]int{}
	byEventID := map[string]string{}
	if archiveValue, present := thread[GeneralTerminalPublicationArchiveFieldV1]; present {
		if archiveValue == nil {
			return nil, errors.New("general terminal publication archive is explicitly null")
		}
		archive, err := domainturnterminal.ParseGeneralTerminalPublicationArchiveV1(archiveValue)
		if err != nil {
			return nil, errors.New("general terminal publication archive is invalid")
		}
		for _, commit := range archive.Commits {
			if commit.ThreadID != threadID || byCommit[commit.CommitID] != 0 || byTurn[commit.TurnID] != 0 {
				return nil, errors.New("general terminal publication archive contains a foreign or duplicate commit")
			}
			entryIndex := len(entries)
			byCommit[commit.CommitID] = entryIndex + 1
			byTurn[commit.TurnID] = entryIndex + 1
			entries = append(entries, GeneralTerminalPublicationInventoryEntryV1{
				TurnID: commit.TurnID, Commit: commit, State: GeneralTerminalPublicationMissingV1,
				Events: []map[string]any{}, ArchivedOnly: true,
			})
			for _, event := range commit.Events {
				if owner := byEventID[event.EventID]; owner != "" {
					return nil, errors.New("general terminal inventory reuses a publication event identity")
				}
				byEventID[event.EventID] = commit.CommitID
			}
		}
	}

	allTurnIDs := map[string]bool{}
	turns, _ := thread["turns"].([]any)
	for _, value := range turns {
		turn, _ := value.(map[string]any)
		if turn == nil {
			continue
		}
		turnID := strings.TrimSpace(stringField(turn, "id"))
		if turnID == "" || allTurnIDs[turnID] {
			return nil, errors.New("general terminal inventory contains an invalid or duplicate turn identity")
		}
		allTurnIDs[turnID] = true
		if turn["generalTerminalPublication"] == nil {
			continue
		}
		commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(turn["generalTerminalPublication"])
		if err != nil || ValidateGeneralTerminalPublicationCommitForThreadV1(thread, turnID, commit) != nil ||
			commit.ThreadID != threadID {
			return nil, errors.New("general terminal inventory contains an invalid canonical outbox")
		}
		entryPosition := byTurn[turnID]
		if entryPosition == 0 {
			return nil, errors.New("general terminal canonical outbox is missing its atomic thread archive entry")
		}
		entry := &entries[entryPosition-1]
		if !entry.ArchivedOnly || entry.Commit.CommitID != commit.CommitID ||
			!canonicalJSONEqual(
				domainturnterminal.GeneralTerminalPublicationCommitV1Map(entry.Commit),
				domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit),
			) {
			return nil, errors.New("general terminal turn and thread archive commits diverge")
		}
		entry.ArchivedOnly = false
	}
	for _, entry := range entries {
		if entry.ArchivedOnly && allTurnIDs[entry.TurnID] {
			return nil, errors.New("general terminal archive refers to a present turn without its canonical outbox")
		}
	}

	seenSeq := map[int]bool{}
	for _, event := range events {
		seq, seqOK := contracts.NumericSeq(event["seq"])
		if event == nil || !seqOK || seq <= 0 || seenSeq[seq] || stringField(event, "threadId") != threadID {
			return nil, errors.New("general terminal event inventory has an invalid, duplicate, or foreign sequence")
		}
		seenSeq[seq] = true
		topLevelMarkers, nestedMarker := generalTerminalPublicationMarkerShapeV1(event)
		if nestedMarker || topLevelMarkers != 0 && topLevelMarkers != len(generalTerminalPublicationMarkerNamesV1) {
			return nil, errors.New("general terminal event contains a partial or nested publication marker")
		}
		if topLevelMarkers == 0 {
			continue
		}
		commitID := strings.TrimSpace(stringField(event, "generalTerminalCommitId"))
		eventID := strings.TrimSpace(stringField(event, "generalTerminalEventId"))
		entryPosition := byCommit[commitID]
		if entryPosition == 0 || byEventID[eventID] != commitID ||
			stringField(event, "generalTerminalAuthorityKind") != domainturnterminal.GeneralTerminalAuthorityKindCASV1 {
			return nil, errors.New("general terminal event refers to unknown publication authority")
		}
		entry := &entries[entryPosition-1]
		manifestEvent, found := generalTerminalManifestEventByIDV1(entry.Commit, eventID)
		if !found ||
			stringField(event, "threadId") != entry.Commit.ThreadID || stringField(event, "turnId") != entry.Commit.TurnID ||
			stringField(event, "generalTerminalSlot") != manifestEvent.Slot ||
			stringField(event, "generalTerminalPayloadDigest") != manifestEvent.PayloadDigest ||
			stringField(event, "generalTerminalAuthorityDigest") != entry.Commit.AuthorityDigest ||
			domainturnterminal.GeneralTerminalPublicationPayloadDigestV1(event) != manifestEvent.PayloadDigest {
			return nil, errors.New("general terminal event diverges from its canonical manifest")
		}
		entry.Events = append(entry.Events, contracts.CloneMap(event))
	}

	for index := range entries {
		entry := &entries[index]
		if len(entry.Events) == 0 {
			if entry.ArchivedOnly {
				return nil, errors.New("compacted general terminal publication bundle is missing and cannot be reconstructed")
			}
			continue
		}
		if len(entry.Events) != len(entry.Commit.Events) {
			return nil, errors.New("general terminal event bundle is partial")
		}
		sort.SliceStable(entry.Events, func(i, j int) bool {
			left, _ := contracts.NumericSeq(entry.Events[i]["seq"])
			right, _ := contracts.NumericSeq(entry.Events[j]["seq"])
			return left < right
		})
		firstSeq, _ := contracts.NumericSeq(entry.Events[0]["seq"])
		if entry.ArchivedOnly {
			for eventIndex, manifestEvent := range entry.Commit.Events {
				seq, ok := contracts.NumericSeq(entry.Events[eventIndex]["seq"])
				if !ok || seq != firstSeq+eventIndex ||
					stringField(entry.Events[eventIndex], "generalTerminalEventId") != manifestEvent.EventID ||
					stringField(entry.Events[eventIndex], "generalTerminalSlot") != manifestEvent.Slot ||
					validateArchivedGeneralTerminalEventV1(entry.Commit, manifestEvent, entry.Events[eventIndex]) != nil {
					return nil, errors.New("compacted general terminal event bundle is duplicated, non-contiguous, or conflicting")
				}
			}
		} else {
			expected, err := PrepareGeneralTerminalEventBundleV1(thread, entry.TurnID, entry.Commit, firstSeq)
			if err != nil || len(expected) != len(entry.Events) {
				return nil, errors.New("general terminal event bundle cannot be reconstructed")
			}
			for eventIndex := range expected {
				seq, ok := contracts.NumericSeq(entry.Events[eventIndex]["seq"])
				if !ok || seq != firstSeq+eventIndex || !canonicalJSONEqual(expected[eventIndex], entry.Events[eventIndex]) {
					return nil, errors.New("general terminal event bundle is duplicated, non-contiguous, or conflicting")
				}
			}
		}
		entry.State = GeneralTerminalPublicationCompleteV1
	}
	lastEndSeq := 0
	missingSeen := false
	for _, entry := range entries {
		if entry.State == GeneralTerminalPublicationMissingV1 {
			missingSeen = true
			continue
		}
		if missingSeen || len(entry.Events) == 0 {
			return nil, errors.New("general terminal publication bundles skip an earlier archive commit")
		}
		firstSeq, _ := contracts.NumericSeq(entry.Events[0]["seq"])
		lastSeq, _ := contracts.NumericSeq(entry.Events[len(entry.Events)-1]["seq"])
		if firstSeq <= lastEndSeq {
			return nil, errors.New("general terminal publication bundle sequence ranges overlap or reverse archive order")
		}
		lastEndSeq = lastSeq
	}
	return entries, nil
}

func GeneralTerminalPublicationEntryForTurnV1(
	entries []GeneralTerminalPublicationInventoryEntryV1,
	turnID string,
) (GeneralTerminalPublicationInventoryEntryV1, bool) {
	turnID = strings.TrimSpace(turnID)
	for _, entry := range entries {
		if entry.TurnID == turnID {
			return entry, true
		}
	}
	return GeneralTerminalPublicationInventoryEntryV1{}, false
}

// ValidateGenericTerminalEventPathV1 prevents single-event and raw JSON APIs
// from impersonating the dedicated ordinary terminal bundle sink.
func ValidateGenericTerminalEventPathV1(event map[string]any) error {
	markers, nestedMarker := generalTerminalPublicationMarkerShapeV1(event)
	if markers != 0 || nestedMarker || domainevent.ContainsAcceptedFinalPublicationAuthority(event) || containsAssistantTextRecord(event) {
		return ErrAssistantPublicationUnbound
	}
	return nil
}

func ValidateGenericTerminalEventPathForThreadV1(thread map[string]any, event map[string]any) error {
	if err := ValidateGenericTerminalEventPathV1(event); err != nil {
		return err
	}
	turnID := strings.TrimSpace(stringField(event, "turnId"))
	turn, found := securityTurnByID(thread, turnID)
	hasCanonicalPublication := found && turn["generalTerminalPublication"] != nil
	if archiveValue, present := thread[GeneralTerminalPublicationArchiveFieldV1]; present {
		archive, err := domainturnterminal.ParseGeneralTerminalPublicationArchiveV1(archiveValue)
		if err != nil {
			return ErrAssistantPublicationUnbound
		}
		for _, commit := range archive.Commits {
			if commit.ThreadID != strings.TrimSpace(stringField(thread, "id")) {
				return ErrAssistantPublicationUnbound
			}
			if commit.TurnID == turnID {
				hasCanonicalPublication = true
			}
		}
	}
	if !hasCanonicalPublication {
		return nil
	}
	switch strings.TrimSpace(stringField(event, "kind")) {
	case "item_completed", "usage", "turn_completed", "turn_failed", "turn_aborted":
		return ErrAssistantPublicationUnbound
	default:
		return nil
	}
}

func generalTerminalManifestEventByIDV1(
	commit domainturnterminal.GeneralTerminalPublicationCommitV1,
	eventID string,
) (domainturnterminal.GeneralTerminalPublicationEventV1, bool) {
	for _, event := range commit.Events {
		if event.EventID == eventID {
			return event, true
		}
	}
	return domainturnterminal.GeneralTerminalPublicationEventV1{}, false
}

func validateArchivedGeneralTerminalEventV1(
	commit domainturnterminal.GeneralTerminalPublicationCommitV1,
	manifest domainturnterminal.GeneralTerminalPublicationEventV1,
	event map[string]any,
) error {
	if domainevent.ValidatePublicRecord(event) != nil || stringField(event, "timestamp") != commit.CommittedAt {
		return errors.New("compacted general terminal event contains a forbidden or stale payload")
	}
	actual := contracts.CloneMap(event)
	delete(actual, "seq")
	switch manifest.Slot {
	case "usage":
		expected, err := domainturnterminal.GeneralTerminalPublicationEventDraftV1(commit, manifest.Slot, commit.UsageEvent)
		if err != nil || !canonicalJSONEqual(expected, actual) {
			return errors.New("compacted general terminal usage event is not canonical")
		}
	case "terminal":
		expected, err := domainturnterminal.GeneralTerminalPublicationEventDraftV1(commit, manifest.Slot, commit.TerminalEvent)
		if err != nil || !canonicalJSONEqual(expected, actual) {
			return errors.New("compacted general terminal lifecycle event is not canonical")
		}
	case "terminal-item":
		if !generalTerminalExactFieldSetV1(event, map[string]bool{
			"kind": true, "threadId": true, "turnId": true, "itemId": true, "item": true, "timestamp": true, "seq": true,
			"generalTerminalCommitId": true, "generalTerminalEventId": true, "generalTerminalSlot": true,
			"generalTerminalPayloadDigest": true, "generalTerminalAuthorityKind": true, "generalTerminalAuthorityDigest": true,
		}) || stringField(event, "kind") != "item_completed" {
			return errors.New("compacted general terminal item event schema is invalid")
		}
		item, _ := event["item"].(map[string]any)
		if item == nil || !validArchivedGeneralTerminalItemFieldSetV1(item) {
			return errors.New("compacted general terminal item schema is invalid")
		}
		text, textOK := domainturnterminal.GeneralTerminalItemTextV1(item)
		binding, bindingErr := domainturnterminal.ParseGeneralTerminalCASBindingV1(item["generalTerminalCASBinding"])
		if !textOK || bindingErr != nil || stringField(item, "id") != commit.TerminalItemID ||
			stringField(event, "itemId") != commit.TerminalItemID || stringField(item, "threadId") != commit.ThreadID ||
			stringField(item, "turnId") != commit.TurnID || stringField(item, "status") != commit.TerminalStatus ||
			stringField(item, "createdAt") == "" || stringField(item, "finishedAt") != commit.CommittedAt ||
			binding.ThreadID != commit.ThreadID || binding.TurnID != commit.TurnID ||
			binding.ContextDigest != commit.ContextDigest || binding.ContextEpoch != commit.ContextEpoch ||
			binding.DatasetSnapshotID != commit.DatasetSnapshotID || binding.BindingDigest != commit.AuthorityDigest ||
			binding.TerminalReason != commit.TerminalReason || binding.TerminalStatus != commit.TerminalStatus ||
			binding.RenderedTextSHA256 != domainsecurity.SHA256Hex([]byte(text)) {
			return errors.New("compacted general terminal item is detached from its binding")
		}
		base := map[string]any{
			"kind": "item_completed", "threadId": commit.ThreadID, "turnId": commit.TurnID,
			"itemId": commit.TerminalItemID, "item": contracts.CloneMap(item), "timestamp": commit.CommittedAt,
		}
		expected, err := domainturnterminal.GeneralTerminalPublicationEventDraftV1(commit, manifest.Slot, base)
		if err != nil || !canonicalJSONEqual(expected, actual) {
			return errors.New("compacted general terminal item event is not canonical")
		}
	default:
		return errors.New("compacted general terminal slot is unknown")
	}
	return nil
}

func validArchivedGeneralTerminalItemFieldSetV1(item map[string]any) bool {
	common := map[string]bool{
		"id": true, "turnId": true, "threadId": true, "role": true, "status": true,
		"createdAt": true, "finishedAt": true, "kind": true, "generalTerminalCASBinding": true,
	}
	allowed := map[string]bool{}
	for key := range common {
		allowed[key] = true
	}
	required := map[string]bool{}
	for key := range common {
		required[key] = true
	}
	switch stringField(item, "kind") {
	case "assistant_text":
		allowed["text"] = true
		required["text"] = true
		if value, present := item["ordinaryResult"]; present {
			slot, err := domainordinaryresult.ParseResultSlotV1(value)
			if err != nil || slot.Text != stringField(item, "text") {
				return false
			}
			allowed["ordinaryResult"] = true
			required["ordinaryResult"] = true
		} else if stringField(item, "text") != domainevent.GeneralTerminalCompletedBoundaryTextV1 {
			return false
		}
		if stringField(item, "role") != "assistant" || stringField(item, "status") != "completed" {
			return false
		}
	case "error":
		for _, key := range []string{"code", "message", "severity"} {
			allowed[key] = true
			required[key] = true
		}
		allowed["details"] = true
		if stringField(item, "role") != "system" || stringField(item, "status") == "completed" {
			return false
		}
	default:
		return false
	}
	for key := range item {
		if !allowed[key] {
			return false
		}
	}
	for key := range required {
		if _, present := item[key]; !present {
			return false
		}
	}
	return true
}

func generalTerminalExactFieldSetV1(value map[string]any, allowed map[string]bool) bool {
	if value == nil || len(value) != len(allowed) {
		return false
	}
	for key := range value {
		if !allowed[key] {
			return false
		}
	}
	return true
}

func generalTerminalPublicationMarkerShapeV1(event map[string]any) (int, bool) {
	topLevel := 0
	for key := range event {
		if generalTerminalPublicationMarkerNamesV1[key] {
			topLevel++
		}
	}
	nested := false
	for key, value := range event {
		if generalTerminalPublicationMarkerNamesV1[key] {
			continue
		}
		if containsGeneralTerminalPublicationMarkerV1(value) {
			nested = true
			break
		}
	}
	return topLevel, nested
}

func containsGeneralTerminalPublicationMarkerV1(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if generalTerminalPublicationMarkerNamesV1[key] || containsGeneralTerminalPublicationMarkerV1(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsGeneralTerminalPublicationMarkerV1(child) {
				return true
			}
		}
	case []map[string]any:
		for _, child := range typed {
			if containsGeneralTerminalPublicationMarkerV1(child) {
				return true
			}
		}
	}
	return false
}
