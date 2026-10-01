package turn

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	threaddomain "analytix.local/runtime-go/internal/domain/thread"
)

func continuationUserHistoryV1(thread map[string]any) (*threaddomain.ContinuationUserHistoryV1, error) {
	scope, ok := appmodel.ContinuationSourceScopeV1(thread)
	if !ok {
		return nil, nil
	}
	history := &threaddomain.ContinuationUserHistoryV1{Version: "continuation-user-history.v1", ScopeDigest: scope, Sources: []threaddomain.ContinuationUserSourceV1{}}
	turns := listAny(thread["turns"])
	// The last valid snapshot already owns all previous sources. Copy only that
	// one; repeated compaction must not resurrect an older overwritten snapshot.
	for i := len(turns) - 1; i >= 0; i-- {
		turn, _ := turns[i].(map[string]any)
		found := false
		for _, raw := range listAny(turn["items"]) {
			item, _ := raw.(map[string]any)
			if !domainevent.ValidGeneralCompactionProviderHistoryItemV4(item) {
				continue
			}
			previous, err := threaddomain.ParseTaskContinuationSnapshotV1(item["taskContinuation"])
			if err == nil && previous.UserHistory != nil && previous.UserHistory.ScopeDigest == scope {
				history.Sources = append(history.Sources, previous.UserHistory.Sources...)
			}
			found = true
			break
		}
		if found {
			break
		}
	}
	seen := map[string]string{}
	for _, source := range history.Sources {
		seen[source.Reference] = source.Text
	}
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if turn["discard"] == true {
			continue
		}
		if thread["securityState"] != nil {
			turnScopeThread := map[string]any{"id": stringField(thread, "id"), "workspace": stringField(thread, "workspace"), "securityState": turn["securityContext"]}
			turnScope, valid := appmodel.ContinuationSourceScopeV1(turnScopeThread)
			if turn["securityContext"] == nil || !valid || turnScope != scope {
				continue
			}
		}
		for _, rawItem := range listAny(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "kind") != "user_message" || stringField(item, "delivery") == "steer" {
				continue
			}
			rawText := stringField(item, "text")
			if len(rawText) > threaddomain.ContinuationSourceBudgetBytes {
				return nil, errors.New("continuation user source exceeds its storage budget")
			}
			text := continuationPublicTextV1(rawText, threaddomain.ContinuationSourceBudgetBytes+1)
			if text == "" {
				continue
			}
			if len(text) > threaddomain.ContinuationSourceBudgetBytes {
				return nil, errors.New("continuation user source exceeds its storage budget")
			}
			identity, _ := json.Marshal([]string{scope, stringField(turn, "id"), stringField(item, "id")})
			ref := domainsecurity.SHA256Hex(identity)
			if prior, found := seen[ref]; found {
				if prior != text {
					return nil, errors.New("continuation user source changed without a new revision")
				}
				continue
			}
			seen[ref] = text
			history.Sources = append(history.Sources, threaddomain.ContinuationUserSourceV1{Reference: ref, Text: text, Digest: threaddomain.ContinuationSourceDigestV1(scope, ref, text)})
		}
	}
	if err := history.Validate(); err != nil {
		return nil, err
	}
	return history, nil
}

// ReadTaskContinuationSourceV1 is a bounded read of an original user source
// from the current durable thread. It accepts no arbitrary path or thread ID.
// The server's normal tool grant/currentness gate runs before this function.
func ReadTaskContinuationSourceV1(thread map[string]any, current domainsecurity.TurnSecurityContext, reference string, offset, limit int) (map[string]any, error) {
	if domainsecurity.ValidateTurnSecurityContextForExecution(current) != nil || !domainsecurity.TurnSecurityContextIsGeneral(current) || current.ThreadID != stringField(thread, "id") || current.WorkspaceRealPath != stringField(thread, "workspace") {
		return nil, errors.New("task history source is unavailable in the current scope")
	}
	if (!domainsecurity.IsSHA256Hex(reference) && !threaddomain.ValidContinuationProviderReferenceV1(reference)) || offset < 0 || limit < 1 || limit > 16000 {
		return nil, errors.New("task history read bounds are invalid")
	}
	history, err := continuationUserHistoryV1(thread)
	if err != nil || history == nil {
		return nil, errors.New("task history source is unavailable")
	}
	// Bind the read itself to the current principal as well as the durable
	// thread. A copied valid context for another principal is not read authority.
	if thread["securityState"] != nil {
		currentThread := map[string]any{"id": current.ThreadID, "workspace": current.WorkspaceRealPath, "securityState": current}
		currentScope, valid := appmodel.ContinuationSourceScopeV1(currentThread)
		if !valid || currentScope != history.ScopeDigest {
			return nil, errors.New("task history principal is no longer current")
		}
	}
	for _, source := range history.Sources {
		if source.Reference != reference && threaddomain.ContinuationProviderReferenceV1(source.Reference) != reference {
			continue
		}
		runes := []rune(source.Text)
		if offset > len(runes) {
			return nil, errors.New("task history offset exceeds source")
		}
		end := offset + limit
		if end > len(runes) {
			end = len(runes)
		}
		return map[string]any{"version": history.Version, "reference": reference, "sourceCommitment": threaddomain.ContinuationProviderReferenceV1(source.Digest), "text": string(runes[offset:end]), "offset": offset, "nextOffset": end, "totalRunes": utf8.RuneCountInString(source.Text), "complete": end == len(runes), "authority": "original_user_history_not_execution_permission"}, nil
	}
	return nil, errors.New("task history reference is unavailable in the current scope")
}

// ContinuationSourceReadableInThreadV1 is used before injecting inline sources.
// Changing workspace, case or principal cannot replay a stale source snapshot.
func ContinuationSourceReadableInThreadV1(thread map[string]any, history *threaddomain.ContinuationUserHistoryV1) bool {
	if history == nil {
		return true
	}
	scope, ok := appmodel.ContinuationSourceScopeV1(thread)
	return ok && strings.TrimSpace(history.ScopeDigest) == scope && history.Validate() == nil
}
