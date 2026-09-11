package goal

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appthread "analytix.local/runtime-go/internal/app/thread"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

type ToolStore interface {
	GetGoal(threadID string) (map[string]any, error)
	SetGoal(threadID string, patch map[string]any) (map[string]any, error)
	AppendGoalEvidence(threadID string, entry map[string]any) (map[string]any, map[string]any, error)
	GetTodos(threadID string) (map[string]any, error)
	SetTodos(threadID string, items []any) (map[string]any, error)
	GetThread(threadID string) (map[string]any, error)
	CommitPreparedCompleteStep(PreparedCompleteStepCommit) (PreparedCompleteStepCommitResult, error)
	CommitPreparedUpdateGoal(PreparedUpdateGoalCommit) (map[string]any, error)
	CommitPreparedTodoOps(PreparedTodoOpsCommit) (map[string]any, error)
}

var ErrPreparedStateStale = errors.New("prepared owner state changed before persistence")

type PreparedCompleteStepCommit struct {
	ThreadID         string
	ExpectedGoalHash string
	ExpectedTodoHash string
	Entry            map[string]any
	NextTodoItems    []any
	GoalPatch        map[string]any
}

type PreparedCompleteStepCommitResult struct {
	Goal  map[string]any
	Entry map[string]any
	Todos map[string]any
}

type PreparedUpdateGoalCommit struct {
	ThreadID         string
	ExpectedGoalHash string
	ExpectedTodoHash string
	Patch            map[string]any
}

type PreparedTodoOpsCommit struct {
	ThreadID         string
	ExpectedTodoHash string
	NextTodoItems    []any
}

type PendingToolContext struct {
	ThreadID   string
	TurnID     string
	ToolCallID string
}

type ToolResult struct {
	Output  any
	IsError bool
}

type ToolService struct {
	Store              ToolStore
	RecordGoalUpdated  func(threadID string, goal map[string]any)
	RecordTodosUpdated func(threadID string, todos map[string]any)
}

// NormalizedEvidenceItemV1 is the owner-authoritative semantic shape used by
// both complete_step execution and its durable side-effect identity. Evidence
// retains the exact text that execution persists, so the shorthand string
// form cannot alias the object form when their persisted effects differ.
type NormalizedEvidenceItemV1 struct {
	Evidence string   `json:"evidence"`
	Kind     string   `json:"kind"`
	Summary  string   `json:"summary"`
	Command  string   `json:"command,omitempty"`
	Paths    []string `json:"paths,omitempty"`
}

type toolPreparationError struct {
	code    string
	message string
}

func (err toolPreparationError) Error() string { return err.message }

func newToolPreparationError(code string, err error) error {
	if err == nil {
		return nil
	}
	return toolPreparationError{code: code, message: err.Error()}
}

func preparationError(code string, message string) error {
	return toolPreparationError{code: code, message: message}
}

func preparationErrorResult(err error) ToolResult {
	var typed toolPreparationError
	if errors.As(err, &typed) {
		return errorResult(typed.code, typed.message)
	}
	return errorResult("validation_error", err.Error())
}

// PreparationFailure exposes the closed host tool outcome for a rejected
// owner preparation. Callers may settle this outcome without issuing a
// side-effect intent; arbitrary infrastructure errors still map to the
// generic validation boundary.
func PreparationFailure(err error) ToolResult {
	return preparationErrorResult(err)
}

// PreparedCompleteStep is the host-owned value shared by semantic admission
// and execution. It freezes the persisted evidence shape and the durable todo
// selected by step/step_index before either layer can observe the request.
type PreparedCompleteStep struct {
	Step             string
	Evidence         []any
	EvidenceDetails  []any
	SemanticEvidence []NormalizedEvidenceItemV1
	Summary          string
	RequirementID    string
	SelfCheck        bool
	TodoTargetID     string
	NextTodoItems    []any
	ExpectedGoalHash string
	ExpectedTodoHash string
	ownerVersion     int
}

func (prepared PreparedCompleteStep) SemanticProjectionV1() map[string]any {
	return map[string]any{
		"step": prepared.Step, "evidence": prepared.SemanticEvidence,
		"summary": prepared.Summary, "requirementId": prepared.RequirementID,
		"selfCheck": prepared.SelfCheck, "todoTargetId": prepared.TodoTargetID,
	}
}

// PreparedUpdateGoal captures the state-dependent owner phase. In strict
// completion, requesting a self-check and final completion are intentionally
// distinct effects even when the provider sends identical arguments.
type PreparedUpdateGoal struct {
	Status           string
	Phase            string
	Reason           string
	NormalizedReason string
	Patch            map[string]any
	FailureCode      string
	FailureMessage   string
	ExpectedGoalHash string
	ExpectedTodoHash string
	ownerVersion     int
}

type PreparedTodoOps struct {
	Owner            appthread.PreparedTodoOpsV1
	ExpectedTodoHash string
	ownerVersion     int
}

func (prepared PreparedTodoOps) SemanticProjectionV1() map[string]any {
	return map[string]any{"ops": prepared.Owner.Operations}
}

func (prepared PreparedUpdateGoal) SemanticProjectionV1() map[string]any {
	projection := map[string]any{"status": prepared.Status, "phase": prepared.Phase}
	if prepared.Status == "blocked" {
		projection["reason"] = prepared.NormalizedReason
	}
	return projection
}

func (s ToolService) GetGoal(threadID string) ToolResult {
	goal, err := s.store().GetGoal(threadID)
	if err != nil {
		return errorResult("goal_read_failed", err.Error())
	}
	return okResult(ToolResponse(goal, ""))
}

func (s ToolService) CreateGoal(pending PendingToolContext, args map[string]any) ToolResult {
	objective := strings.TrimSpace(firstNonEmptyAnyString(args["objective"]))
	if objective == "" {
		return errorResult("validation_error", "objective is required")
	}
	if existing, err := s.store().GetGoal(pending.ThreadID); err == nil && existing != nil {
		return errorResult("goal_exists", "cannot create a new goal because this thread already has a goal; use update_goal only when the existing goal is complete")
	} else if err != nil {
		return errorResult("goal_read_failed", err.Error())
	}
	patch := map[string]any{
		"objective": objective,
		"status":    "active",
	}
	if tokenBudget, ok, err := NormalizeTokenBudget(firstNonEmptyAny(args["token_budget"], args["tokenBudget"])); err != nil {
		return errorResult("validation_error", err.Error())
	} else if ok {
		patch["tokenBudget"] = float64(tokenBudget)
	}
	if boolField(args, "strict_completion") || boolField(args, "strictCompletion") {
		patch["strictCompletion"] = true
	}
	goal, err := s.store().SetGoal(pending.ThreadID, patch)
	if err != nil {
		return errorResult("goal_create_failed", err.Error())
	}
	s.recordGoalUpdated(pending.ThreadID, goal)
	return okResult(ToolResponse(goal, ""))
}

func (s ToolService) CompleteStep(pending PendingToolContext, args map[string]any) ToolResult {
	prepared, err := s.PrepareCompleteStep(pending, args)
	if err != nil {
		return preparationErrorResult(err)
	}
	return s.ExecutePreparedCompleteStep(pending, prepared)
}

func (s ToolService) PrepareCompleteStep(pending PendingToolContext, args map[string]any) (PreparedCompleteStep, error) {
	step := strings.TrimSpace(firstNonEmptyAnyString(args["step"]))
	stepProvided := step != ""
	stepIndex := 0
	if value, ok := numericAny(firstNonEmptyAny(args["step_index"], args["stepIndex"])); ok {
		stepIndex = value
	}
	if step == "" && stepIndex > 0 {
		step = strconv.Itoa(stepIndex)
	}
	if step == "" {
		return PreparedCompleteStep{}, preparationError("validation_error", "step or step_index is required")
	}
	if stepIndex < 0 {
		return PreparedCompleteStep{}, preparationError("validation_error", "step_index must be a positive 1-based task-list number")
	}
	semanticEvidence, err := NormalizeEvidenceItemsV1(args["evidence"])
	if err != nil {
		return PreparedCompleteStep{}, newToolPreparationError("goal_evidence_rejected", err)
	}
	evidence, details, err := s.verifyNormalizedGoalEvidence(pending.ThreadID, semanticEvidence)
	if err != nil {
		return PreparedCompleteStep{}, newToolPreparationError("goal_evidence_rejected", err)
	}
	goal, err := s.store().GetGoal(pending.ThreadID)
	if err != nil {
		return PreparedCompleteStep{}, newToolPreparationError("goal_read_failed", err)
	}
	if goal == nil {
		return PreparedCompleteStep{}, preparationError("goal_missing", "cannot record goal evidence because this thread does not have a goal")
	}
	if stringField(goal, "status") != "active" {
		return PreparedCompleteStep{}, preparationError("goal_not_active", "cannot record goal evidence because the current goal is "+stringField(goal, "status")+", not active")
	}
	selfCheck := boolField(args, "self_check") || boolField(args, "selfCheck")
	if selfCheck && (goal["strictCompletion"] != true || goal["selfCheckRequired"] != true) {
		return PreparedCompleteStep{}, preparationError("self_check_not_requested", "self_check evidence can only be recorded after update_goal requests a strict completion self-check")
	}
	todos, err := s.store().GetTodos(pending.ThreadID)
	if err != nil {
		return PreparedCompleteStep{}, newToolPreparationError("todo_step_mismatch", err)
	}
	summary := strings.TrimSpace(firstNonEmptyAnyString(args["summary"], args["result"], args["notes"]))
	prepared := PreparedCompleteStep{
		Step: step, Evidence: evidence, EvidenceDetails: details, SemanticEvidence: semanticEvidence,
		Summary: summary, RequirementID: strings.TrimSpace(firstNonEmptyAnyString(args["requirement_id"], args["requirementId"])),
		SelfCheck: selfCheck, ExpectedGoalHash: SemanticStateHashV1(goal), ExpectedTodoHash: SemanticStateHashV1(todos), ownerVersion: 1,
	}
	items := listAny(todos["items"])
	if len(items) > 0 {
		matchIndex := FindTodoStepIndex(items, step, stepIndex, stepProvided)
		if matchIndex < 0 {
			return PreparedCompleteStep{}, preparationError("todo_step_mismatch", fmt.Sprintf("step %q has no matching todo_write item", step))
		}
		target, _ := items[matchIndex].(map[string]any)
		if status := stringField(target, "status"); status != "pending" && status != "in_progress" {
			return PreparedCompleteStep{}, preparationError("todo_step_mismatch", fmt.Sprintf("todo %q in status %q cannot be completed", stringField(target, "id"), status))
		}
		prepared.TodoTargetID = stringField(target, "id")
		if prepared.TodoTargetID == "" {
			return PreparedCompleteStep{}, preparationError("todo_step_mismatch", "matched todo does not have a durable id")
		}
		prepared.Step = strings.TrimSpace(stringField(target, "content"))
		if prepared.Step == "" {
			prepared.Step = prepared.TodoTargetID
		}
		prepared.NextTodoItems = completedTodoItems(items, matchIndex)
	}
	return prepared, nil
}

func (s ToolService) ExecutePreparedCompleteStep(pending PendingToolContext, prepared PreparedCompleteStep) ToolResult {
	if prepared.ownerVersion != 1 {
		return errorResult("goal_prepared_state_invalid", "complete_step prepared owner state is invalid")
	}
	entryPatch := map[string]any{
		"step":            prepared.Step,
		"evidence":        prepared.Evidence,
		"evidenceDetails": prepared.EvidenceDetails,
		"turnId":          pending.TurnID,
		"toolCallId":      pending.ToolCallID,
	}
	if prepared.Summary != "" {
		entryPatch["summary"] = prepared.Summary
	}
	if prepared.RequirementID != "" {
		entryPatch["requirementId"] = prepared.RequirementID
	}
	advanced := len(prepared.NextTodoItems) > 0
	goalPatch := map[string]any{}
	if prepared.SelfCheck {
		goalPatch = map[string]any{
			"selfCheckRequired": true, "selfCheckCompleted": true, "selfCheckTurnId": pending.TurnID,
		}
	}
	committed, err := s.store().CommitPreparedCompleteStep(PreparedCompleteStepCommit{
		ThreadID: pending.ThreadID, ExpectedGoalHash: prepared.ExpectedGoalHash, ExpectedTodoHash: prepared.ExpectedTodoHash,
		Entry: entryPatch, NextTodoItems: prepared.NextTodoItems, GoalPatch: goalPatch,
	})
	if err != nil {
		if errors.Is(err, ErrPreparedStateStale) {
			return errorResult("goal_prepared_state_stale", "complete_step owner state changed before persistence")
		}
		return errorResult("goal_evidence_failed", err.Error())
	}
	goal, entry, todos := committed.Goal, committed.Entry, committed.Todos
	if advanced {
		s.recordTodosUpdated(pending.ThreadID, todos)
	}
	s.recordGoalUpdated(pending.ThreadID, goal)
	out := map[string]any{
		"goal":                goal,
		"entry":               entry,
		"evidenceLedgerCount": float64(len(listAny(goal["evidenceLedger"]))),
		"hostVerified":        EvidenceDetailsHostVerified(prepared.EvidenceDetails),
	}
	if advanced {
		out["todoAdvanced"] = true
		out["todos"] = todos
	}
	if prepared.SelfCheck {
		out["selfCheckCompleted"] = true
	}
	return okResult(out)
}

func (s ToolService) UpdateGoal(pending PendingToolContext, args map[string]any) ToolResult {
	prepared, err := s.PrepareUpdateGoal(pending, args)
	if err != nil {
		return preparationErrorResult(err)
	}
	return s.ExecutePreparedUpdateGoal(pending, prepared)
}

func (s ToolService) PrepareUpdateGoal(pending PendingToolContext, args map[string]any) (PreparedUpdateGoal, error) {
	status := strings.TrimSpace(firstNonEmptyAnyString(args["status"]))
	if status != "complete" && status != "blocked" {
		return PreparedUpdateGoal{}, preparationError("validation_error", "update_goal can only mark the existing goal complete or blocked")
	}
	goal, err := s.store().GetGoal(pending.ThreadID)
	if err != nil {
		return PreparedUpdateGoal{}, newToolPreparationError("goal_read_failed", err)
	}
	if goal == nil {
		return PreparedUpdateGoal{}, preparationError("goal_missing", "cannot update goal because this thread does not have a goal")
	}
	if stringField(goal, "status") != "active" {
		return PreparedUpdateGoal{}, preparationError("goal_not_active", "cannot update goal because the current goal is "+stringField(goal, "status")+", not active")
	}
	if status == "blocked" {
		reason, normalizedReason, reasonErr := CanonicalBlockedReasonV1(firstNonEmptyAnyString(args["reason"]))
		if reasonErr != nil {
			return PreparedUpdateGoal{}, newToolPreparationError("validation_error", reasonErr)
		}
		sameReason := NormalizeBlockedReason(stringField(goal, "blockedReason")) == normalizedReason
		sameTurn := stringField(goal, "blockedTurnId") == pending.TurnID
		prior := int(floatFromAny(goal["blockedCount"]))
		if !sameReason {
			prior = 0
		}
		count := prior + 1
		if sameReason && sameTurn && prior > 0 {
			count = prior
		}
		nextStatus := "active"
		if count >= 3 {
			nextStatus = "blocked"
		}
		return PreparedUpdateGoal{
			Status: status, Phase: "record_blocker", Reason: reason, NormalizedReason: normalizedReason,
			ExpectedGoalHash: SemanticStateHashV1(goal), ownerVersion: 1,
			Patch: map[string]any{
				"status": nextStatus, "blockedReason": reason, "blockedCount": float64(count), "blockedTurnId": pending.TurnID,
			},
		}, nil
	}
	if len(listAny(goal["evidenceLedger"])) == 0 {
		return PreparedUpdateGoal{}, preparationError("goal_evidence_required", "cannot mark goal complete without evidence; call complete_step first")
	}
	todos, err := s.store().GetTodos(pending.ThreadID)
	if err != nil {
		return PreparedUpdateGoal{}, newToolPreparationError("todo_read_failed", err)
	}
	if todos != nil && incompleteTodos(todos) > 0 {
		return PreparedUpdateGoal{}, preparationError("goal_todos_incomplete", "cannot mark goal complete while todos are still incomplete")
	}
	if goal["strictCompletion"] == true && goal["selfCheckCompleted"] != true {
		if goal["selfCheckRequired"] != true {
			return PreparedUpdateGoal{Status: status, Phase: "request_self_check", ExpectedGoalHash: SemanticStateHashV1(goal), ExpectedTodoHash: SemanticStateHashV1(todos), ownerVersion: 1, Patch: map[string]any{
				"selfCheckRequired": true, "selfCheckTurnId": pending.TurnID,
			}}, nil
		}
		return PreparedUpdateGoal{
			Status: status, Phase: "await_self_check", FailureCode: "goal_self_check_required",
			FailureMessage:   "cannot mark strict goal complete until a completion self-check has been recorded; call complete_step with self_check: true",
			ExpectedGoalHash: SemanticStateHashV1(goal), ExpectedTodoHash: SemanticStateHashV1(todos), ownerVersion: 1,
		}, nil
	}
	return PreparedUpdateGoal{Status: status, Phase: "finalize_complete", ExpectedGoalHash: SemanticStateHashV1(goal), ExpectedTodoHash: SemanticStateHashV1(todos), ownerVersion: 1, Patch: map[string]any{"status": "complete"}}, nil
}

func (s ToolService) ExecutePreparedUpdateGoal(pending PendingToolContext, prepared PreparedUpdateGoal) ToolResult {
	if prepared.ownerVersion != 1 {
		return errorResult("goal_prepared_state_invalid", "update_goal prepared owner state is invalid")
	}
	if prepared.FailureCode != "" {
		return errorResult(prepared.FailureCode, prepared.FailureMessage)
	}
	updated, err := s.store().CommitPreparedUpdateGoal(PreparedUpdateGoalCommit{
		ThreadID: pending.ThreadID, ExpectedGoalHash: prepared.ExpectedGoalHash,
		ExpectedTodoHash: prepared.ExpectedTodoHash, Patch: prepared.Patch,
	})
	if err != nil {
		if errors.Is(err, ErrPreparedStateStale) {
			return errorResult("goal_prepared_state_stale", "update_goal owner state changed before persistence")
		}
		return errorResult("goal_update_failed", err.Error())
	}
	s.recordGoalUpdated(pending.ThreadID, updated)
	switch prepared.Phase {
	case "record_blocker":
		count := floatFromAny(prepared.Patch["blockedCount"])
		nextStatus := stringField(prepared.Patch, "status")
		message := "Goal remains active until the same blocking condition repeats across three goal turns."
		if nextStatus == "blocked" {
			message = "Goal marked blocked after repeated same-condition reports."
		}
		return okResult(ToolResponseWithAudits(updated, "", map[string]any{
			"reason": prepared.Reason, "count": count, "requiredCount": float64(3), "status": nextStatus, "message": message,
		}, nil))
	case "request_self_check":
		return okResult(ToolResponseWithAudits(updated, "", nil, map[string]any{
			"selfCheckRequired": true, "instructions": StrictCompletionSelfCheckInstructions(),
		}))
	default:
		return okResult(ToolResponse(updated, "Goal achieved. Report final usage from this tool result if relevant."))
	}
}

func (s ToolService) TodoList(threadID string) ToolResult {
	todos, err := s.store().GetTodos(threadID)
	if err != nil {
		return errorResult("todo_read_failed", err.Error())
	}
	return okResult(map[string]any{"todos": todos})
}

func (s ToolService) TodoWrite(threadID string, args map[string]any) ToolResult {
	rawTodos, ok := args["todos"].([]any)
	if !ok {
		return errorResult("validation_error", "todos must be an array")
	}
	current, err := s.store().GetTodos(threadID)
	if err != nil {
		return errorResult("todo_read_failed", err.Error())
	}
	normalized, err := appthread.NormalizeTodos(threadID, rawTodos, "validation")
	if err != nil {
		return errorResult("todo_write_failed", err.Error())
	}
	if err := appthread.ValidateTodoReplacement(current, normalized); err != nil {
		return errorResult("todo_write_failed", err.Error())
	}
	todos, err := s.store().SetTodos(threadID, rawTodos)
	if err != nil {
		return errorResult("todo_write_failed", err.Error())
	}
	s.recordTodosUpdated(threadID, todos)
	return okResult(map[string]any{"todos": todos})
}

func (s ToolService) TodoOps(threadID string, args map[string]any) ToolResult {
	prepared, err := s.PrepareTodoOps(threadID, args)
	if err != nil {
		return preparationErrorResult(err)
	}
	return s.ExecutePreparedTodoOps(threadID, prepared)
}

func (s ToolService) PrepareTodoOps(threadID string, args map[string]any) (PreparedTodoOps, error) {
	rawOps, ok := firstNonEmptyAny(args["ops"], args["operations"]).([]any)
	if !ok {
		return PreparedTodoOps{}, preparationError("validation_error", "ops must be an array")
	}
	current, err := s.store().GetTodos(threadID)
	if err != nil {
		return PreparedTodoOps{}, newToolPreparationError("todo_read_failed", err)
	}
	owner, err := appthread.PrepareTodoOpsV1(current, rawOps)
	if err != nil {
		return PreparedTodoOps{}, newToolPreparationError("todo_ops_failed", err)
	}
	return PreparedTodoOps{Owner: owner, ExpectedTodoHash: SemanticStateHashV1(current), ownerVersion: 1}, nil
}

func (s ToolService) ExecutePreparedTodoOps(threadID string, prepared PreparedTodoOps) ToolResult {
	if prepared.ownerVersion != 1 {
		return errorResult("todo_prepared_state_invalid", "todo_ops prepared owner state is invalid")
	}
	todos, err := s.store().CommitPreparedTodoOps(PreparedTodoOpsCommit{
		ThreadID: threadID, ExpectedTodoHash: prepared.ExpectedTodoHash, NextTodoItems: prepared.Owner.NextItems,
	})
	if err != nil {
		if errors.Is(err, ErrPreparedStateStale) {
			return errorResult("todo_prepared_state_stale", "todo_ops owner state changed before persistence")
		}
		return errorResult("todo_ops_failed", err.Error())
	}
	s.recordTodosUpdated(threadID, todos)
	return okResult(map[string]any{
		"todos":      todos,
		"operations": prepared.Owner.Applied,
	})
}

func (s ToolService) NormalizeGoalEvidence(threadID string, value any) ([]any, []any, error) {
	items, err := NormalizeEvidenceItemsV1(value)
	if err != nil {
		return nil, nil, err
	}
	return s.verifyNormalizedGoalEvidence(threadID, items)
}

func (s ToolService) verifyNormalizedGoalEvidence(threadID string, items []NormalizedEvidenceItemV1) ([]any, []any, error) {
	evidence := []any{}
	details := []any{}
	for index, item := range items {
		detail := map[string]any{
			"kind":    item.Kind,
			"summary": item.Summary,
		}
		hostVerified := false
		switch item.Kind {
		case "verification":
			if !s.GoalHasSuccessfulCommand(threadID, item.Command) {
				return nil, nil, fmt.Errorf("evidence %d: verification command %q has no matching successful bash receipt", index+1, item.Command)
			}
			detail["command"] = item.Command
			hostVerified = true
		case "diff", "files":
			wantWrite := item.Kind == "diff"
			if !s.GoalHasSuccessfulPathEvidence(threadID, item.Paths, wantWrite) {
				return nil, nil, fmt.Errorf("evidence %d: %s paths have no matching successful host receipt", index+1, item.Kind)
			}
			detail["paths"] = stringListAny(item.Paths)
			hostVerified = true
		case "manual":
			hostVerified = false
		}
		detail["hostVerified"] = hostVerified
		evidence = append(evidence, item.Evidence)
		details = append(details, detail)
	}
	return evidence, details, nil
}

// NormalizeEvidenceItemsV1 parses complete_step evidence exactly once without
// consulting mutable receipt state. Execution subsequently verifies the
// required host receipts before persisting this shape.
func NormalizeEvidenceItemsV1(value any) ([]NormalizedEvidenceItemV1, error) {
	rawItems := listAny(value)
	if len(rawItems) == 0 {
		return nil, errors.New("at least one evidence item is required")
	}
	items := make([]NormalizedEvidenceItemV1, 0, min(len(rawItems), 20))
	for index, raw := range rawItems {
		if len(items) >= 20 {
			break
		}
		if text, ok := raw.(string); ok {
			summary := truncateText(text, 2000)
			if summary == "" {
				continue
			}
			items = append(items, NormalizedEvidenceItemV1{
				Evidence: summary,
				Kind:     "manual",
				Summary:  summary,
			})
			continue
		}
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("evidence %d must be a string or object", index+1)
		}
		kind := strings.TrimSpace(stringField(item, "kind"))
		if kind == "" {
			kind = "manual"
		}
		summary := truncateText(stringField(item, "summary"), 2000)
		if summary == "" {
			return nil, fmt.Errorf("evidence %d: summary is required", index+1)
		}
		normalized := NormalizedEvidenceItemV1{
			Evidence: truncateText(kind+": "+summary, 2000),
			Kind:     kind,
			Summary:  summary,
		}
		switch kind {
		case "verification":
			normalized.Command = strings.TrimSpace(stringField(item, "command"))
			if normalized.Command == "" {
				return nil, fmt.Errorf("evidence %d: verification command is required", index+1)
			}
		case "diff", "files":
			normalized.Paths = stringList(item["paths"])
			if len(normalized.Paths) == 0 {
				return nil, fmt.Errorf("evidence %d: %s evidence requires paths", index+1, kind)
			}
		case "manual":
		default:
			return nil, fmt.Errorf("evidence %d: invalid kind %q (want verification|diff|files|manual)", index+1, kind)
		}
		items = append(items, normalized)
	}
	if len(items) == 0 {
		return nil, errors.New("evidence must include at least one concrete non-empty item")
	}
	return items, nil
}

func (s ToolService) GoalHasSuccessfulCommand(threadID string, command string) bool {
	command = strings.TrimSpace(command)
	if command == "" || s.Store == nil {
		return false
	}
	arguments, err := json.Marshal(map[string]any{"command": command})
	if err != nil {
		return false
	}
	wantedArgsHash := domainsecurity.CanonicalJSONHash(arguments)
	thread, err := s.Store.GetThread(threadID)
	if err != nil || thread == nil || wantedArgsHash == "" {
		return false
	}
	for _, rawTurn := range listAny(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		turnID := stringField(turn, "id")
		securityContext, contextErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
		if turnID == "" || contextErr != nil || domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil ||
			securityContext.ThreadID != strings.TrimSpace(threadID) || securityContext.TurnID != turnID {
			continue
		}
		registry, registryErr := executiongrantapp.RegistryFromThread(threadID, thread, turnID)
		if registryErr != nil {
			continue
		}
		for _, entry := range registry.Entries {
			grant := entry.Grant
			if entry.Status != domainsecurity.GrantRegistrySettled || grant.ToolName != "bash" || grant.ServerIdentity != "host:builtin" ||
				grant.ReadOnly || (grant.ApprovalState != "not_required" && grant.ApprovalState != "approved") || grant.ArgsHash != wantedArgsHash ||
				domainsecurity.ValidateExecutionGrantForContext(grant, securityContext) != nil {
				continue
			}
			result, unique := uniqueToolResultForGrant(turn, grant)
			if !unique || !successfulSettledToolResult(threadID, thread, turn, result, grant, securityContext) {
				continue
			}
			return true
		}
	}
	return false
}

// successfulSettledToolResult verifies goal evidence against the host-private
// execution grant and the closed durable result. Public history deliberately
// withholds exact arguments and output, so neither model-visible text nor a
// non-empty result field can act as a goal-completion receipt.
func successfulSettledToolResult(threadID string, thread map[string]any, turn map[string]any, result map[string]any, grant domainsecurity.ExecutionGrant, securityContext domainsecurity.TurnSecurityContext) bool {
	turnID := stringField(turn, "id")
	if stringField(result, "kind") != "tool_result" || stringField(result, "id") == "" ||
		stringField(result, "threadId") != strings.TrimSpace(threadID) || stringField(result, "turnId") != turnID ||
		stringField(result, "role") != "tool" || stringField(result, "toolName") != grant.ToolName ||
		stringField(result, "callId") != grant.ToolCallID || stringField(result, "status") != "completed" ||
		stringField(result, "contextDigest") != securityContext.ContextDigest ||
		stringField(result, "executionGrantId") != grant.GrantID || uint64Field(result["contextEpoch"]) != securityContext.ContextEpoch {
		return false
	}
	isError, errorFieldPresent := result["isError"].(bool)
	projection, projectionErr := domaintoolresult.ParsePublicToolResultProjectionV1(result["output"])
	if !errorFieldPresent || isError || projectionErr != nil ||
		projection.ProjectionKind != domaintoolresult.ProjectionHostStatus || projection.Status != "completed" ||
		projection.MessageKey != "tool_completed" || projection.Code != "tool_completed" {
		return false
	}
	authority, err := executiongrantapp.DurableSettlementFromThread(threadID, thread, turnID, stringField(result, "id"), grant)
	if err != nil {
		return false
	}
	issuedAt, issuedErr := time.Parse(time.RFC3339Nano, grant.IssuedAt)
	expiresAt, expiresErr := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	if issuedErr != nil || expiresErr != nil || authority.SettledAt.Before(issuedAt) || authority.SettledAt.After(expiresAt) {
		return false
	}
	return true
}

func uniqueToolResultForGrant(turn map[string]any, grant domainsecurity.ExecutionGrant) (map[string]any, bool) {
	var result map[string]any
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "kind") != "tool_result" || stringField(item, "executionGrantId") != grant.GrantID {
			continue
		}
		if result != nil {
			return nil, false
		}
		result = item
	}
	return result, result != nil
}

func uint64Field(value any) uint64 {
	switch typed := value.(type) {
	case uint64:
		return typed
	case uint:
		return uint64(typed)
	case int:
		if typed > 0 {
			return uint64(typed)
		}
	case int64:
		if typed > 0 {
			return uint64(typed)
		}
	case float64:
		if typed > 0 && typed == float64(uint64(typed)) {
			return uint64(typed)
		}
	}
	return 0
}

func (s ToolService) GoalHasSuccessfulPathEvidence(threadID string, paths []string, wantWrite bool) bool {
	if len(paths) == 0 {
		return false
	}
	for _, item := range s.toolResultItems(threadID) {
		if item["isError"] == true || stringField(item, "status") != "completed" {
			continue
		}
		toolName := stringField(item, "toolName")
		if wantWrite && !ToolWritesPath(toolName) {
			continue
		}
		if !wantWrite && !ToolReadsOrWritesPath(toolName) {
			continue
		}
		output, _ := item["output"].(map[string]any)
		if output == nil {
			continue
		}
		allMatched := true
		for _, path := range paths {
			if !OutputMentionsPath(output, path) {
				allMatched = false
				break
			}
		}
		if allMatched {
			return true
		}
	}
	return false
}

func (s ToolService) AdvanceTodoForCompletedStep(threadID string, step string, stepIndex int, stepProvided bool) (map[string]any, bool, error) {
	todos, err := s.store().GetTodos(threadID)
	if err != nil || todos == nil {
		return nil, false, err
	}
	items := listAny(todos["items"])
	if len(items) == 0 {
		return todos, false, nil
	}
	matchIndex := FindTodoStepIndex(items, step, stepIndex, stepProvided)
	if matchIndex < 0 {
		return nil, false, fmt.Errorf("step %q has no matching todo_write item", step)
	}
	target, _ := items[matchIndex].(map[string]any)
	if status := stringField(target, "status"); status != "pending" && status != "in_progress" {
		return nil, false, fmt.Errorf("todo %q in status %q cannot be completed", stringField(target, "id"), status)
	}
	nextItems := completedTodoItems(items, matchIndex)
	updated, err := s.store().SetTodos(threadID, nextItems)
	if err != nil {
		return nil, false, err
	}
	s.recordTodosUpdated(threadID, updated)
	return updated, true, nil
}

func completedTodoItems(items []any, matchIndex int) []any {
	nextItems := make([]any, 0, len(items))
	activeSeen := false
	for index, raw := range items {
		item, _ := raw.(map[string]any)
		next := cloneMap(item)
		if index == matchIndex {
			next["status"] = "completed"
			delete(next, "statusReasonCode")
		}
		if stringField(next, "status") == "in_progress" {
			if activeSeen {
				next["status"] = "pending"
			} else {
				activeSeen = true
			}
		}
		nextItems = append(nextItems, next)
	}
	if !activeSeen {
		for index := matchIndex + 1; index < len(nextItems); index++ {
			item, _ := nextItems[index].(map[string]any)
			if stringField(item, "status") == "pending" {
				item["status"] = "in_progress"
				break
			}
		}
	}
	return nextItems
}

func (s ToolService) ValidateTodoStepMatch(threadID string, step string, stepIndex int, stepProvided bool) error {
	todos, err := s.store().GetTodos(threadID)
	if err != nil || todos == nil {
		return err
	}
	items := listAny(todos["items"])
	if len(items) == 0 {
		return nil
	}
	if FindTodoStepIndex(items, step, stepIndex, stepProvided) < 0 {
		return fmt.Errorf("step %q has no matching todo_write item", step)
	}
	return nil
}

func (s ToolService) blockGoal(pending PendingToolContext, goal map[string]any, args map[string]any) ToolResult {
	reason, normalizedReason, err := CanonicalBlockedReasonV1(firstNonEmptyAnyString(args["reason"]))
	if err != nil {
		return errorResult("validation_error", err.Error())
	}
	sameReason := NormalizeBlockedReason(stringField(goal, "blockedReason")) == normalizedReason
	sameTurn := stringField(goal, "blockedTurnId") == pending.TurnID
	prior := int(floatFromAny(goal["blockedCount"]))
	if !sameReason {
		prior = 0
	}
	count := prior + 1
	if sameReason && sameTurn && prior > 0 {
		count = prior
	}
	nextStatus := "active"
	if count >= 3 {
		nextStatus = "blocked"
	}
	updated, err := s.store().SetGoal(pending.ThreadID, map[string]any{
		"status":        nextStatus,
		"blockedReason": reason,
		"blockedCount":  float64(count),
		"blockedTurnId": pending.TurnID,
	})
	if err != nil {
		return errorResult("goal_update_failed", err.Error())
	}
	s.recordGoalUpdated(pending.ThreadID, updated)
	message := "Goal remains active until the same blocking condition repeats across three goal turns."
	if nextStatus == "blocked" {
		message = "Goal marked blocked after repeated same-condition reports."
	}
	return okResult(ToolResponseWithAudits(updated, "", map[string]any{
		"reason":        reason,
		"count":         float64(count),
		"requiredCount": float64(3),
		"status":        nextStatus,
		"message":       message,
	}, nil))
}

func (s ToolService) toolResultItems(threadID string) []map[string]any {
	if s.Store == nil {
		return nil
	}
	thread, err := s.Store.GetThread(threadID)
	if err != nil || thread == nil {
		return nil
	}
	items := []map[string]any{}
	for _, rawTurn := range listAny(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		for _, rawItem := range listAny(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "kind") == "tool_result" {
				items = append(items, item)
			}
		}
	}
	return items
}

func (s ToolService) store() ToolStore {
	if s.Store != nil {
		return s.Store
	}
	return missingStore{}
}

func (s ToolService) recordGoalUpdated(threadID string, goal map[string]any) {
	if s.RecordGoalUpdated != nil {
		s.RecordGoalUpdated(threadID, goal)
	}
}

func (s ToolService) recordTodosUpdated(threadID string, todos map[string]any) {
	if s.RecordTodosUpdated != nil {
		s.RecordTodosUpdated(threadID, todos)
	}
}

func SemanticStateHashV1(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return domainsecurity.CanonicalJSONHash(body)
}

func okResult(output any) ToolResult {
	return ToolResult{Output: output}
}

func errorResult(code string, message string) ToolResult {
	return ToolResult{Output: map[string]any{"code": code, "error": message}, IsError: true}
}

type missingStore struct{}

func (missingStore) GetGoal(string) (map[string]any, error) {
	return nil, errors.New("goal store is not configured")
}

func (missingStore) SetGoal(string, map[string]any) (map[string]any, error) {
	return nil, errors.New("goal store is not configured")
}

func (missingStore) AppendGoalEvidence(string, map[string]any) (map[string]any, map[string]any, error) {
	return nil, nil, errors.New("goal store is not configured")
}

func (missingStore) GetTodos(string) (map[string]any, error) {
	return nil, errors.New("goal store is not configured")
}

func (missingStore) SetTodos(string, []any) (map[string]any, error) {
	return nil, errors.New("goal store is not configured")
}

func (missingStore) GetThread(string) (map[string]any, error) {
	return nil, errors.New("goal store is not configured")
}

func (missingStore) CommitPreparedCompleteStep(PreparedCompleteStepCommit) (PreparedCompleteStepCommitResult, error) {
	return PreparedCompleteStepCommitResult{}, errors.New("goal store is not configured")
}

func (missingStore) CommitPreparedUpdateGoal(PreparedUpdateGoalCommit) (map[string]any, error) {
	return nil, errors.New("goal store is not configured")
}

func (missingStore) CommitPreparedTodoOps(PreparedTodoOpsCommit) (map[string]any, error) {
	return nil, errors.New("goal store is not configured")
}

func firstNonEmptyAny(values ...any) any {
	for _, value := range values {
		if value == nil {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		return value
	}
	return nil
}

func listAny(value any) []any {
	if items, ok := value.([]any); ok {
		return items
	}
	return []any{}
}

func stringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, raw := range typed {
			item := strings.TrimSpace(firstNonEmptyAnyString(raw))
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	default:
		item := strings.TrimSpace(firstNonEmptyAnyString(value))
		if item == "" {
			return nil
		}
		return []string{item}
	}
}

func stringListAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func truncateText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func incompleteTodos(todos map[string]any) int {
	count := 0
	for _, raw := range listAny(todos["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "status") != "completed" {
			count++
		}
	}
	return count
}

func boolField(record map[string]any, key string) bool {
	if record == nil {
		return false
	}
	value, _ := record[key].(bool)
	return value
}
