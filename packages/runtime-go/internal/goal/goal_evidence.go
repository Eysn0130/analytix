package goal

import domaingoal "analytix.local/runtime-go/internal/domain/goal"

type GoalEvidenceAuditResult = domaingoal.GoalEvidenceAuditResult

const (
	GoalEvidenceAllowed = domaingoal.GoalEvidenceAllowed
	GoalEvidenceBlocked = domaingoal.GoalEvidenceBlocked
	GoalEvidenceErrored = domaingoal.GoalEvidenceErrored
)

type GoalEvidenceReceipt = domaingoal.GoalEvidenceReceipt
type GoalEvidenceProjectCheck = domaingoal.GoalEvidenceProjectCheck
type GoalEvidenceTodoItem = domaingoal.GoalEvidenceTodoItem
type GoalEvidenceInput = domaingoal.GoalEvidenceInput
type GoalEvidenceAudit = domaingoal.GoalEvidenceAudit

func EvaluateGoalEvidence(input GoalEvidenceInput) GoalEvidenceAudit {
	return domaingoal.EvaluateGoalEvidence(input)
}

func GoalEvidenceAuditEvent(audit GoalEvidenceAudit) map[string]any {
	return domaingoal.GoalEvidenceAuditEvent(audit)
}

func RuntimeTurnGoalEvidenceAudits(threadID, turnID string) []GoalEvidenceAudit {
	return domaingoal.RuntimeTurnGoalEvidenceAudits(threadID, turnID)
}

func GoalEvidenceCommandMatches(cited, ran string) bool {
	return domaingoal.GoalEvidenceCommandMatches(cited, ran)
}
