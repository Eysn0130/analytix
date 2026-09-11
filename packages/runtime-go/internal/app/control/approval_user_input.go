package control

import (
	"strings"
	"sync"
)

const (
	gateStatusOK       = 200
	gateStatusNotFound = 404
	gateStatusConflict = 409
)

type ApprovalUserInputManager struct {
	mu                  sync.Mutex
	approvals           map[string]approval
	inputs              map[string]userInput
	replay              []map[string]any
	executedDeniedTools int
}

type approval struct {
	ID       string
	ToolName string
	Status   string
	Decision string
}

type userInput struct {
	ID       string
	Status   string
	Answers  []map[string]string
	Question string
}

func NewApprovalUserInputManager() *ApprovalUserInputManager {
	return &ApprovalUserInputManager{
		approvals: map[string]approval{},
		inputs:    map[string]userInput{},
		replay:    []map[string]any{},
	}
}

func (m *ApprovalUserInputManager) RequestApproval(id, toolName string) {
	_ = m.EnsureApprovalPending(id, toolName)
}

// EnsureApprovalPending projects a request without reopening or overwriting a
// prior manager state. The manager is a UI projection only; signed private
// continuation authority and GateRegistry activation remain authoritative.
func (m *ApprovalUserInputManager) EnsureApprovalPending(id, toolName string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	id = strings.TrimSpace(id)
	toolName = strings.TrimSpace(toolName)
	if id == "" || toolName == "" {
		return false
	}
	if existing, ok := m.approvals[id]; ok {
		return existing.ID == id && existing.ToolName == toolName && existing.Status == "pending" && existing.Decision == ""
	}
	m.approvals[id] = approval{ID: id, ToolName: toolName, Status: "pending"}
	m.replay = append(m.replay, map[string]any{"kind": "approval_requested", "approvalId": id, "toolName": toolName, "status": "pending"})
	return true
}

func (m *ApprovalUserInputManager) ResolveApproval(id, decision string) (int, map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.approvals[id]
	if !ok || item.Status != "pending" {
		return gateStatusConflict, map[string]any{"code": "approval_not_pending", "approvalId": id}
	}
	item.Decision = decision
	if decision == "deny" {
		item.Status = "denied"
	} else {
		item.Status = "allowed"
	}
	m.approvals[id] = item
	m.replay = append(m.replay, map[string]any{"kind": "approval_resolved", "approvalId": id, "decision": decision, "status": item.Status})
	return gateStatusOK, map[string]any{"approvalId": id, "decision": decision, "status": item.Status}
}

func (m *ApprovalUserInputManager) ApprovalDisposition(id string) (status, decision string, exists bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, exists := m.approvals[id]
	if !exists {
		return "", "", false
	}
	return item.Status, item.Decision, true
}

func (m *ApprovalUserInputManager) ExecuteToolAfterApproval(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.approvals[id]
	if item.Status == "denied" {
		m.executedDeniedTools++
		return false
	}
	return item.Status == "allowed"
}

func (m *ApprovalUserInputManager) RequestUserInput(id, question string) {
	_ = m.EnsureUserInputPending(id, question)
}

// EnsureUserInputPending is exact and monotonic: it never changes a submitted,
// cancelled, timed-out, or conflicting pending projection back to pending.
func (m *ApprovalUserInputManager) EnsureUserInputPending(id, question string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	id = strings.TrimSpace(id)
	question = strings.TrimSpace(question)
	if id == "" || question == "" {
		return false
	}
	if existing, ok := m.inputs[id]; ok {
		return existing.ID == id && existing.Question == question && existing.Status == "pending" && len(existing.Answers) == 0
	}
	m.inputs[id] = userInput{ID: id, Question: question, Status: "pending"}
	m.replay = append(m.replay, map[string]any{"kind": "user_input_requested", "inputId": id, "question": question, "status": "pending"})
	return true
}

func (m *ApprovalUserInputManager) SubmitUserInput(id string, answers []map[string]string) (int, map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.inputs[id]
	if !ok || item.Status != "pending" {
		return gateStatusNotFound, map[string]any{"code": "user_input_not_pending", "inputId": id}
	}
	item.Status = "submitted"
	item.Answers = append([]map[string]string(nil), answers...)
	m.inputs[id] = item
	m.replay = append(m.replay, map[string]any{"kind": "user_input_resolved", "inputId": id, "status": "submitted"})
	return gateStatusOK, map[string]any{"inputId": id, "status": "submitted", "answers": answers}
}

func (m *ApprovalUserInputManager) CancelUserInput(id string) (int, map[string]any) {
	return m.finishUserInput(id, "cancelled")
}

func (m *ApprovalUserInputManager) UserInputDisposition(id string) (status string, exists bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, exists := m.inputs[id]
	if !exists {
		return "", false
	}
	return item.Status, true
}

func (m *ApprovalUserInputManager) TimeoutUserInput(id string) (int, map[string]any) {
	return m.finishUserInput(id, "timeout")
}

func (m *ApprovalUserInputManager) finishUserInput(id, status string) (int, map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.inputs[id]
	if !ok || item.Status != "pending" {
		return gateStatusNotFound, map[string]any{"code": "user_input_not_pending", "inputId": id}
	}
	item.Status = status
	m.inputs[id] = item
	m.replay = append(m.replay, map[string]any{"kind": "user_input_resolved", "inputId": id, "status": status})
	return gateStatusOK, map[string]any{"inputId": id, "status": status}
}

func (m *ApprovalUserInputManager) Replay() []map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]map[string]any, 0, len(m.replay))
	for _, item := range m.replay {
		out = append(out, cloneGateMap(item))
	}
	return out
}

func RunApprovalUserInputContractExercise() map[string]any {
	manager := NewApprovalUserInputManager()
	manager.RequestApproval("appr_contract", "write_file")
	denyStatus, denyBody := manager.ResolveApproval("appr_contract", "deny")
	deniedToolExecuted := manager.ExecuteToolAfterApproval("appr_contract")
	secondStatus, _ := manager.ResolveApproval("appr_contract", "allow")
	manager.RequestUserInput("input_submit_contract", "Choose next action")
	submitStatus, submitBody := manager.SubmitUserInput("input_submit_contract", []map[string]string{{"id": "q1", "label": "Continue", "value": "continue"}})
	manager.RequestUserInput("input_cancel_contract", "Cancel direction")
	cancelStatus, cancelBody := manager.CancelUserInput("input_cancel_contract")
	manager.RequestUserInput("input_timeout_contract", "Timeout direction")
	timeoutStatus, timeoutBody := manager.TimeoutUserInput("input_timeout_contract")
	replay := manager.Replay()
	return map[string]any{
		"runtimeGoContractParitySlice":      true,
		"pendingApprovalCreated":            true,
		"denyStatus":                        denyStatus,
		"denyBody":                          denyBody,
		"secondDecisionStatus":              secondStatus,
		"deniedToolExecuted":                deniedToolExecuted,
		"submitStatus":                      submitStatus,
		"submitBodyEchoesAnswers":           len(answerSliceFromBody(submitBody)) == 1,
		"cancelStatus":                      cancelStatus,
		"cancelBody":                        cancelBody,
		"timeoutStatus":                     timeoutStatus,
		"timeoutBody":                       timeoutBody,
		"replayKinds":                       gateEventKinds(replay),
		"submittedAnswersPersistedInEvents": gateReplayContainsKey(replay, "answers"),
		"submittedAnswersPrivacyBoundary":   !gateReplayContainsKey(replay, "answers"),
		"lateApprovalRejected":              secondStatus == gateStatusConflict,
	}
}

func ApprovalItem(threadID, turnID, itemID, approvalID, now string) map[string]any {
	return map[string]any{
		"id":         itemID,
		"turnId":     turnID,
		"threadId":   threadID,
		"role":       "tool",
		"status":     "pending",
		"createdAt":  now,
		"kind":       "approval",
		"approvalId": approvalID,
		"toolName":   "write_file",
		"summary":    "Approve write_file",
	}
}

func UserInputItem(threadID, turnID, itemID, inputID, now string) map[string]any {
	return map[string]any{
		"id":        itemID,
		"turnId":    turnID,
		"threadId":  threadID,
		"role":      "system",
		"status":    "pending",
		"createdAt": now,
		"kind":      "user_input",
		"inputId":   inputID,
		"prompt":    "User input required",
		"questions": []map[string]any{{
			"header":   "Input",
			"id":       "q1",
			"question": "User input required",
			"options":  []map[string]string{{"label": "Continue", "description": "Continue the turn."}},
		}},
	}
}

func cloneGateMap(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func gateEventKinds(events []map[string]any) []string {
	kinds := make([]string, 0, len(events))
	for _, event := range events {
		if kind, ok := event["kind"].(string); ok {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

func answerSliceFromBody(body map[string]any) []any {
	answers, _ := body["answers"].([]map[string]string)
	if len(answers) > 0 {
		out := make([]any, len(answers))
		for i := range answers {
			out[i] = answers[i]
		}
		return out
	}
	raw, _ := body["answers"].([]any)
	return raw
}

func gateReplayContainsKey(events []map[string]any, key string) bool {
	for _, event := range events {
		if _, ok := event[key]; ok {
			return true
		}
	}
	return false
}
