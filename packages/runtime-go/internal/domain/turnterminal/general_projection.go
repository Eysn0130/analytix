package turnterminal

import (
	"encoding/json"
	"errors"
	"strings"

	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const GeneralTerminalPublicationArchiveFieldV1 = "generalTerminalPublicationArchive"

var generalTerminalProjectionMarkerFieldsV1 = []string{
	"generalTerminalCommitId",
	"generalTerminalEventId",
	"generalTerminalSlot",
	"generalTerminalPayloadDigest",
	"generalTerminalAuthorityKind",
	"generalTerminalAuthorityDigest",
}

// GeneralTerminalProjectionAuthorityV1 is the pure host decision consumed by
// public history, SSE, provider history, compaction, fork, and sidecar paths.
// Governed means a valid frozen general context exists. Terminal is true only
// for an exact CAS/outbox/root-archive tuple.
type GeneralTerminalProjectionAuthorityV1 struct {
	Commit   GeneralTerminalPublicationCommitV1
	Governed bool
	Terminal bool
}

func GeneralTerminalProjectionAuthoritiesV1(
	thread map[string]any,
) (map[string]GeneralTerminalProjectionAuthorityV1, error) {
	archive, err := generalTerminalArchiveCommitsForProjectionV1(thread)
	if err != nil {
		return nil, err
	}
	authorities := map[string]GeneralTerminalProjectionAuthorityV1{}
	seen := map[string]bool{}
	for _, rawTurn := range generalTerminalList(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if turn == nil {
			continue
		}
		turnID := generalTerminalString(turn, "id")
		authority, governed, authorityErr := generalTerminalProjectionAuthorityForTurnV1(thread, turn, archive)
		if authorityErr != nil {
			return nil, authorityErr
		}
		if governed {
			if turnID == "" || seen[turnID] {
				return nil, errors.New("ordinary context-bound projection contains an invalid or duplicate turn identity")
			}
			seen[turnID] = true
			authorities[turnID] = authority
		}
	}
	return authorities, nil
}

func generalTerminalArchiveCommitsForProjectionV1(
	thread map[string]any,
) (map[string]GeneralTerminalPublicationCommitV1, error) {
	commits := map[string]GeneralTerminalPublicationCommitV1{}
	archiveValue, present := thread[GeneralTerminalPublicationArchiveFieldV1]
	if !present {
		return commits, nil
	}
	archive, err := ParseGeneralTerminalPublicationArchiveV1(archiveValue)
	threadID := generalTerminalString(thread, "id")
	if err != nil || threadID == "" {
		return nil, errors.New("ordinary projection terminal archive is invalid")
	}
	for _, commit := range archive.Commits {
		if commit.ThreadID != threadID {
			return nil, errors.New("ordinary projection terminal archive contains a foreign thread")
		}
		commits[commit.TurnID] = commit
	}
	return commits, nil
}

func generalTerminalProjectionAuthorityForTurnV1(
	thread map[string]any,
	turn map[string]any,
	archive map[string]GeneralTerminalPublicationCommitV1,
) (GeneralTerminalProjectionAuthorityV1, bool, error) {
	turnID := generalTerminalString(turn, "id")
	archived, archivedPresent := archive[turnID]
	_, hasCommit := turn["generalTerminalPublication"]
	_, hasBinding := turn["generalTerminalCASBinding"]
	contextValue, hasContext := turn["securityContext"]
	if !hasContext {
		if archivedPresent || hasCommit || hasBinding {
			return GeneralTerminalProjectionAuthorityV1{}, true, errors.New("ordinary terminal authority has no frozen security context")
		}
		return GeneralTerminalProjectionAuthorityV1{}, false, nil
	}
	context, err := domainsecurity.ParseTurnSecurityContext(contextValue)
	if err != nil {
		return GeneralTerminalProjectionAuthorityV1{}, true, errors.New("ordinary terminal authority has an invalid frozen security context")
	}
	if !domainsecurity.TurnSecurityContextIsGeneral(context) {
		if archivedPresent || hasCommit || hasBinding {
			return GeneralTerminalProjectionAuthorityV1{}, true, errors.New("non-general turn carries ordinary terminal authority")
		}
		return GeneralTerminalProjectionAuthorityV1{}, false, nil
	}
	if domainsecurity.ValidateTurnSecurityContextForExecution(context) != nil ||
		context.ThreadID != generalTerminalString(thread, "id") || context.TurnID != turnID {
		return GeneralTerminalProjectionAuthorityV1{}, true, errors.New("ordinary terminal authority is detached from its frozen security context")
	}
	authority := GeneralTerminalProjectionAuthorityV1{Governed: true}
	if !generalTerminalProjectionStatusIsTerminalV1(generalTerminalString(turn, "status")) {
		if archivedPresent || hasCommit || hasBinding {
			return authority, true, errors.New("active general turn carries terminal authority")
		}
		return authority, true, nil
	}
	if !archivedPresent || !hasCommit || !hasBinding {
		if archivedPresent || hasCommit || hasBinding {
			return authority, true, errors.New("terminal general turn carries a partial atomic outbox authority")
		}
		return authority, true, nil
	}
	commit, commitErr := ParseGeneralTerminalPublicationCommitV1(turn["generalTerminalPublication"])
	binding, bindingErr := ParseGeneralTerminalCASBindingV1(turn["generalTerminalCASBinding"])
	if commitErr != nil || bindingErr != nil || archived.CommitID != commit.CommitID || archived.CommitDigest != commit.CommitDigest ||
		validateGeneralTerminalProjectionWinnerV1(thread, turn, context, binding, commit) != nil {
		return authority, true, errors.New("terminal general turn does not match its canonical archived outbox")
	}
	authority.Commit = commit
	authority.Terminal = true
	return authority, true, nil
}

func validateGeneralTerminalProjectionWinnerV1(
	thread map[string]any,
	turn map[string]any,
	context domainsecurity.TurnSecurityContext,
	binding GeneralTerminalCASBindingV1,
	commit GeneralTerminalPublicationCommitV1,
) error {
	if ValidateGeneralTerminalPublicationCommitV1(commit) != nil || ValidateGeneralTerminalCASBindingV1(binding) != nil ||
		commit.ThreadID != generalTerminalString(thread, "id") || commit.TurnID != generalTerminalString(turn, "id") ||
		commit.TerminalStatus != generalTerminalString(turn, "status") || commit.CommittedAt != generalTerminalString(turn, "finishedAt") ||
		commit.ContextDigest != context.ContextDigest || commit.ContextEpoch != context.ContextEpoch ||
		commit.DatasetSnapshotID != context.DatasetSnapshotID || commit.AuthorityDigest != binding.BindingDigest ||
		binding.ThreadID != context.ThreadID || binding.TurnID != context.TurnID || binding.ContextDigest != context.ContextDigest ||
		binding.ContextEpoch != context.ContextEpoch || binding.DatasetSnapshotID != context.DatasetSnapshotID ||
		binding.TerminalReason != commit.TerminalReason || binding.TerminalStatus != commit.TerminalStatus {
		return errors.New("general terminal projection winner is detached")
	}
	if commit.TerminalItemID == "" {
		if ValidateGeneralTerminalCASBindingForOutcomeV1(binding, context, "", commit.TerminalReason, commit.TerminalStatus) != nil {
			return errors.New("empty general terminal projection binding is invalid")
		}
		return nil
	}
	matchCount := 0
	for _, rawItem := range generalTerminalList(turn["items"]) {
		item, _ := rawItem.(map[string]any)
		if generalTerminalString(item, "id") != commit.TerminalItemID {
			continue
		}
		matchCount++
		text, textOK := GeneralTerminalItemTextV1(item)
		itemBinding, itemBindingErr := ParseGeneralTerminalCASBindingV1(item["generalTerminalCASBinding"])
		if !textOK || !validGeneralTerminalItemShapeV1(item, commit.TerminalStatus) || itemBindingErr != nil ||
			itemBinding.BindingDigest != binding.BindingDigest || generalTerminalString(item, "threadId") != commit.ThreadID ||
			generalTerminalString(item, "turnId") != commit.TurnID || generalTerminalString(item, "finishedAt") != commit.CommittedAt ||
			ValidateGeneralTerminalCASBindingForOutcomeV1(binding, context, text, commit.TerminalReason, commit.TerminalStatus) != nil {
			return errors.New("general terminal projection item is detached")
		}
	}
	if matchCount != 1 {
		return errors.New("general terminal projection item is missing or duplicated")
	}
	return nil
}

// ValidateGeneralTerminalPublicEventV1 validates one raw terminal event before
// public sanitation removes the generalTerminal* tuple.
func ValidateGeneralTerminalPublicEventV1(thread, event map[string]any) (bool, bool, error) {
	archive, err := generalTerminalArchiveCommitsForProjectionV1(thread)
	if err != nil {
		return true, false, err
	}
	turnID := generalTerminalString(event, "turnId")
	commit, archived := archive[turnID]
	turn, present := rawGeneralTerminalTurnByIDV1(thread, turnID)
	if present {
		authority, governed, authorityErr := generalTerminalProjectionAuthorityForTurnV1(thread, turn, archive)
		if authorityErr != nil {
			return true, false, authorityErr
		}
		if authority.Terminal {
			commit = authority.Commit
			archived = true
		} else if governed && generalTerminalProjectionEventNeedsAuthorityV1(event, "") {
			return true, false, nil
		}
	}
	marked := generalTerminalProjectionMarkerCountV1(event) != 0 || generalTerminalProjectionNestedMarkerV1(event, true)
	if !archived {
		if marked || generalTerminalProjectionEventNeedsAuthorityV1(event, "") {
			return true, false, nil
		}
		return false, true, nil
	}
	if !generalTerminalProjectionEventNeedsAuthorityV1(event, commit.TerminalItemID) && !marked {
		return false, true, nil
	}
	if !marked {
		return true, false, nil
	}
	return true, generalTerminalProjectionEventMatchesCommitV1(event, commit), nil
}

func generalTerminalProjectionEventMatchesCommitV1(event map[string]any, commit GeneralTerminalPublicationCommitV1) bool {
	if event == nil || generalTerminalProjectionMarkerCountV1(event) != len(generalTerminalProjectionMarkerFieldsV1) ||
		generalTerminalProjectionNestedMarkerV1(event, true) || domainevent.ValidatePublicRecord(event) != nil ||
		generalTerminalString(event, "threadId") != commit.ThreadID || generalTerminalString(event, "turnId") != commit.TurnID ||
		generalTerminalString(event, "timestamp") != commit.CommittedAt || generalTerminalString(event, "generalTerminalCommitId") != commit.CommitID ||
		generalTerminalString(event, "generalTerminalAuthorityKind") != GeneralTerminalAuthorityKindCASV1 ||
		generalTerminalString(event, "generalTerminalAuthorityDigest") != commit.AuthorityDigest {
		return false
	}
	seq, seqOK := generalTerminalNumericSeq(event["seq"])
	if !seqOK || seq <= 0 {
		return false
	}
	eventID := generalTerminalString(event, "generalTerminalEventId")
	for _, manifest := range commit.Events {
		if manifest.EventID != eventID {
			continue
		}
		if generalTerminalString(event, "generalTerminalSlot") != manifest.Slot ||
			generalTerminalString(event, "generalTerminalPayloadDigest") != manifest.PayloadDigest ||
			GeneralTerminalPublicationPayloadDigestV1(event) != manifest.PayloadDigest {
			return false
		}
		switch manifest.Slot {
		case "terminal-item":
			item, _ := event["item"].(map[string]any)
			return generalTerminalString(event, "kind") == "item_completed" && generalTerminalString(event, "itemId") == commit.TerminalItemID &&
				generalTerminalString(item, "id") == commit.TerminalItemID
		case "usage":
			return generalTerminalString(event, "kind") == "usage"
		case "terminal":
			expectedKind, ok := GeneralTerminalLifecycleEventKindV1(commit.TerminalStatus)
			return ok && generalTerminalString(event, "kind") == expectedKind &&
				generalTerminalString(event, "status") == commit.TerminalStatus && generalTerminalString(event, "terminalReason") == commit.TerminalReason
		default:
			return false
		}
	}
	return false
}

func generalTerminalProjectionEventNeedsAuthorityV1(event map[string]any, terminalItemID string) bool {
	switch generalTerminalString(event, "kind") {
	case "usage", "turn_completed", "turn_failed", "turn_aborted":
		return true
	case "item_created", "item_updated", "item_completed", "assistant_text_delta":
		item, _ := event["item"].(map[string]any)
		itemID := generalTerminalString(event, "itemId")
		itemKind := generalTerminalString(item, "kind")
		return terminalItemID != "" && itemID == terminalItemID || itemKind == "assistant_text" || itemKind == "error"
	default:
		return false
	}
}

func rawGeneralTerminalTurnByIDV1(thread map[string]any, turnID string) (map[string]any, bool) {
	for _, rawTurn := range generalTerminalList(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if generalTerminalString(turn, "id") == turnID {
			return turn, true
		}
	}
	return nil, false
}

func generalTerminalProjectionMarkerCountV1(value map[string]any) int {
	count := 0
	for _, field := range generalTerminalProjectionMarkerFieldsV1 {
		if _, present := value[field]; present {
			count++
		}
	}
	return count
}

func generalTerminalProjectionNestedMarkerV1(value any, root bool) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			isMarker := false
			for _, field := range generalTerminalProjectionMarkerFieldsV1 {
				if key == field {
					isMarker = true
					break
				}
			}
			if isMarker {
				if !root {
					return true
				}
				continue
			}
			if generalTerminalProjectionNestedMarkerV1(child, false) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if generalTerminalProjectionNestedMarkerV1(child, false) {
				return true
			}
		}
	}
	return false
}

func generalTerminalProjectionStatusIsTerminalV1(status string) bool {
	switch strings.TrimSpace(status) {
	case "completed", "failed", "aborted":
		return true
	default:
		return false
	}
}

func generalTerminalList(value any) []any {
	items, _ := value.([]any)
	return items
}

func generalTerminalNumericSeq(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), typed == float64(int(typed))
	case json.Number:
		parsed, err := typed.Int64()
		return int(parsed), err == nil
	default:
		return 0, false
	}
}
