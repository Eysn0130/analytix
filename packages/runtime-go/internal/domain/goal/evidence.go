package goal

import (
	"sort"
	"strings"
	"unicode"
)

type GoalEvidenceAuditResult string

const (
	GoalEvidenceAllowed GoalEvidenceAuditResult = "allowed"
	GoalEvidenceBlocked GoalEvidenceAuditResult = "blocked"
	GoalEvidenceErrored GoalEvidenceAuditResult = "errored"
)

type GoalEvidenceReceipt struct {
	Index           int      `json:"index"`
	Command         string   `json:"command"`
	Success         bool     `json:"success"`
	WritesWorkspace bool     `json:"writesWorkspace"`
	TouchedPaths    []string `json:"touchedPaths,omitempty"`
}

type GoalEvidenceProjectCheck struct {
	ID                 string `json:"id"`
	Command            string `json:"command"`
	RequiredAfterWrite bool   `json:"requiredAfterWrite"`
}

type GoalEvidenceTodoItem struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type GoalEvidenceInput struct {
	ThreadID              string                     `json:"threadId"`
	TurnID                string                     `json:"turnId"`
	GoalID                string                     `json:"goalId"`
	Receipts              []GoalEvidenceReceipt      `json:"receipts"`
	ProjectChecks         []GoalEvidenceProjectCheck `json:"projectChecks"`
	Todos                 []GoalEvidenceTodoItem     `json:"todos"`
	PreviousBlockedAudits []GoalEvidenceAudit        `json:"previousBlockedAudits,omitempty"`
}

type GoalEvidenceAudit struct {
	SchemaVersion              int                     `json:"schemaVersion"`
	ChangeID                   string                  `json:"changeId"`
	RuntimeContract            string                  `json:"runtimeContract"`
	UpstreamSource             string                  `json:"upstreamSource"`
	ThreadID                   string                  `json:"threadId"`
	TurnID                     string                  `json:"turnId"`
	GoalID                     string                  `json:"goalId"`
	Result                     GoalEvidenceAuditResult `json:"result"`
	Recovered                  bool                    `json:"recovered"`
	MissingProjectChecks       int                     `json:"missingProjectChecks"`
	IncompleteTodos            int                     `json:"incompleteTodos"`
	CommandMismatchMissing     int                     `json:"commandMismatchMissing"`
	LatestWriterReceiptIndex   int                     `json:"latestWriterReceiptIndex"`
	BlockedStateKey            string                  `json:"blockedStateKey,omitempty"`
	Reason                     string                  `json:"reason,omitempty"`
	MissingCheckIDs            []string                `json:"missingCheckIds,omitempty"`
	MissingCommands            []string                `json:"missingCommands,omitempty"`
	UsesReasonixPublicProtocol bool                    `json:"usesReasonixPublicProtocol"`
	UsesReasonixConfigRoot     bool                    `json:"usesReasonixConfigRoot"`
	ChangesRendererContract    bool                    `json:"changesRendererContract"`
	ChangesProductIdentity     bool                    `json:"changesProductIdentity"`
}

func EvaluateGoalEvidence(input GoalEvidenceInput) GoalEvidenceAudit {
	latestWriterIndex := 0
	for _, receipt := range input.Receipts {
		if receipt.Success && receipt.WritesWorkspace && receipt.Index > latestWriterIndex {
			latestWriterIndex = receipt.Index
		}
	}

	missingCheckIDs := []string{}
	missingCommands := []string{}
	for _, check := range input.ProjectChecks {
		if strings.TrimSpace(check.ID) == "" || strings.TrimSpace(check.Command) == "" {
			continue
		}
		if check.RequiredAfterWrite && latestWriterIndex == 0 {
			continue
		}
		if !goalEvidenceHasSuccessfulCommandAfter(input.Receipts, check.Command, latestWriterIndex) {
			missingCheckIDs = append(missingCheckIDs, check.ID)
			missingCommands = append(missingCommands, check.Command)
		}
	}
	sort.Strings(missingCheckIDs)
	sort.Strings(missingCommands)

	incompleteTodos := 0
	for _, todo := range input.Todos {
		if !goalEvidenceTodoComplete(todo.Status) {
			incompleteTodos++
		}
	}

	audit := GoalEvidenceAudit{
		SchemaVersion:              1,
		ChangeID:                   "goal-evidence-audit",
		RuntimeContract:            "analytix-go-runtime",
		UpstreamSource:             "reasonix-absorbed",
		ThreadID:                   input.ThreadID,
		TurnID:                     input.TurnID,
		GoalID:                     input.GoalID,
		Result:                     GoalEvidenceAllowed,
		MissingProjectChecks:       len(missingCheckIDs),
		IncompleteTodos:            incompleteTodos,
		CommandMismatchMissing:     len(missingCommands),
		LatestWriterReceiptIndex:   latestWriterIndex,
		MissingCheckIDs:            missingCheckIDs,
		MissingCommands:            missingCommands,
		UsesReasonixPublicProtocol: false,
		UsesReasonixConfigRoot:     false,
		ChangesRendererContract:    false,
		ChangesProductIdentity:     false,
	}

	if audit.MissingProjectChecks > 0 || audit.IncompleteTodos > 0 {
		audit.Result = GoalEvidenceBlocked
		audit.Reason = goalEvidenceReason(audit)
		audit.BlockedStateKey = goalEvidenceBlockedStateKey(audit)
		if goalEvidenceRepeatedBlockedState(input.PreviousBlockedAudits, audit.BlockedStateKey) >= 2 {
			audit.Result = GoalEvidenceErrored
		}
		return audit
	}

	for _, previous := range input.PreviousBlockedAudits {
		if previous.Result == GoalEvidenceBlocked || previous.Result == GoalEvidenceErrored {
			audit.Recovered = true
			break
		}
	}
	audit.MissingCheckIDs = nil
	audit.MissingCommands = nil
	return audit
}

func GoalEvidenceAuditEvent(audit GoalEvidenceAudit) map[string]any {
	event := map[string]any{
		"kind":                       "goal_evidence_audit",
		"schemaVersion":              audit.SchemaVersion,
		"changeId":                   audit.ChangeID,
		"runtimeContract":            audit.RuntimeContract,
		"upstreamSource":             audit.UpstreamSource,
		"threadId":                   audit.ThreadID,
		"turnId":                     audit.TurnID,
		"goalId":                     audit.GoalID,
		"result":                     string(audit.Result),
		"recovered":                  audit.Recovered,
		"missingProjectChecks":       audit.MissingProjectChecks,
		"incompleteTodos":            audit.IncompleteTodos,
		"commandMismatchMissing":     audit.CommandMismatchMissing,
		"latestWriterReceiptIndex":   audit.LatestWriterReceiptIndex,
		"usesReasonixPublicProtocol": audit.UsesReasonixPublicProtocol,
		"usesReasonixConfigRoot":     audit.UsesReasonixConfigRoot,
		"changesRendererContract":    audit.ChangesRendererContract,
		"changesProductIdentity":     audit.ChangesProductIdentity,
	}
	if audit.BlockedStateKey != "" {
		event["blockedStateKey"] = audit.BlockedStateKey
	}
	if audit.Reason != "" {
		event["reason"] = audit.Reason
	}
	if len(audit.MissingCheckIDs) > 0 {
		event["missingCheckIds"] = audit.MissingCheckIDs
	}
	if len(audit.MissingCommands) > 0 {
		event["missingCommands"] = audit.MissingCommands
	}
	return event
}

func RuntimeTurnGoalEvidenceAudits(threadID, turnID string) []GoalEvidenceAudit {
	base := GoalEvidenceInput{
		ThreadID: threadID,
		TurnID:   turnID,
		GoalID:   "goal_" + threadID,
		Receipts: []GoalEvidenceReceipt{{
			Index:           1,
			Command:         "apply_patch packages/runtime-go/runtime_server.go",
			Success:         true,
			WritesWorkspace: true,
			TouchedPaths:    []string{"packages/runtime-go/runtime_server.go"},
		}},
		ProjectChecks: []GoalEvidenceProjectCheck{{
			ID:                 "runtime-go-contract-tests",
			Command:            "go test ./...",
			RequiredAfterWrite: true,
		}},
		Todos: []GoalEvidenceTodoItem{{
			ID:     "goal-evidence-contract",
			Status: "completed",
		}},
	}
	blocked := EvaluateGoalEvidence(base)
	recoveredInput := base
	recoveredInput.Receipts = append([]GoalEvidenceReceipt{}, base.Receipts...)
	recoveredInput.Receipts = append(recoveredInput.Receipts, GoalEvidenceReceipt{
		Index:   2,
		Command: "cd packages/runtime-go && go test ./...",
		Success: true,
	})
	recoveredInput.PreviousBlockedAudits = []GoalEvidenceAudit{blocked}
	recovered := EvaluateGoalEvidence(recoveredInput)
	return []GoalEvidenceAudit{blocked, recovered}
}

func GoalEvidenceCommandMatches(cited, ran string) bool {
	citedSegments := goalEvidenceShellSegments(cited)
	ranSegments := goalEvidenceShellSegments(ran)
	for _, citedSegment := range citedSegments {
		for _, ranSegment := range ranSegments {
			if goalEvidenceShellCommandSegmentMatches(citedSegment, ranSegment) {
				return true
			}
		}
	}
	return false
}

func goalEvidenceHasSuccessfulCommandAfter(receipts []GoalEvidenceReceipt, cited string, afterIndex int) bool {
	for _, receipt := range receipts {
		if !receipt.Success || receipt.Index <= afterIndex {
			continue
		}
		if GoalEvidenceCommandMatches(cited, receipt.Command) {
			return true
		}
	}
	return false
}

func goalEvidenceShellCommandSegmentMatches(cited, ran string) bool {
	citedTokens := goalEvidenceShellTokens(cited)
	ranTokens := goalEvidenceShellTokens(ran)
	if len(citedTokens) == 0 || len(ranTokens) == 0 {
		return false
	}
	citedNormalized := strings.Join(citedTokens, " ")
	ranNormalized := strings.Join(ranTokens, " ")
	if citedNormalized == ranNormalized {
		return true
	}
	if len(citedTokens) < 2 || citedTokens[0] != ranTokens[0] {
		return false
	}
	ranSet := map[string]int{}
	for _, token := range ranTokens {
		ranSet[token]++
	}
	for _, token := range citedTokens {
		if ranSet[token] == 0 {
			return false
		}
		ranSet[token]--
	}
	return true
}

func goalEvidenceShellSegments(command string) []string {
	segments := []string{}
	var builder strings.Builder
	var quote rune
	flush := func() {
		segment := strings.TrimSpace(builder.String())
		builder.Reset()
		if segment != "" {
			segments = append(segments, segment)
		}
	}
	for index := 0; index < len(command); index++ {
		r := rune(command[index])
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			builder.WriteRune(r)
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			builder.WriteRune(r)
		case '\n', ';', '|':
			flush()
		case '&':
			if index+1 < len(command) && command[index+1] == '&' {
				flush()
				index++
				continue
			}
			builder.WriteRune(r)
		default:
			builder.WriteRune(r)
		}
	}
	flush()
	return segments
}

func goalEvidenceShellTokens(command string) []string {
	tokens := []string{}
	var builder strings.Builder
	var quote rune
	flush := func() {
		token := strings.TrimSpace(builder.String())
		builder.Reset()
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	for _, r := range command {
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			builder.WriteRune(r)
			continue
		}
		switch {
		case r == '\'' || r == '"':
			quote = r
		case r == '#':
			flush()
			return tokens
		case unicode.IsSpace(r):
			flush()
		default:
			builder.WriteRune(r)
		}
	}
	flush()
	return tokens
}

func goalEvidenceTodoComplete(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "complete", "done":
		return true
	default:
		return false
	}
}

func goalEvidenceReason(audit GoalEvidenceAudit) string {
	reasons := []string{}
	if audit.MissingProjectChecks > 0 {
		reasons = append(reasons, "project checks missing after latest workspace write")
	}
	if audit.IncompleteTodos > 0 {
		reasons = append(reasons, "todos still incomplete")
	}
	return strings.Join(reasons, "; ")
}

func goalEvidenceBlockedStateKey(audit GoalEvidenceAudit) string {
	parts := []string{
		"missingProjectChecks",
		strings.Join(audit.MissingCheckIDs, ","),
		"incompleteTodos",
	}
	if audit.IncompleteTodos > 0 {
		parts = append(parts, "present")
	} else {
		parts = append(parts, "none")
	}
	return goalEvidenceNormalizeBlockedState(strings.Join(parts, ":"))
}

func goalEvidenceRepeatedBlockedState(audits []GoalEvidenceAudit, key string) int {
	repeated := 0
	for _, audit := range audits {
		if (audit.Result == GoalEvidenceBlocked || audit.Result == GoalEvidenceErrored) && audit.BlockedStateKey == key {
			repeated++
		}
	}
	return repeated
}

func goalEvidenceNormalizeBlockedState(value string) string {
	fields := strings.Fields(strings.ToLower(value))
	return strings.Join(fields, " ")
}
