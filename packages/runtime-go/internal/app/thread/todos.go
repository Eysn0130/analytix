package thread

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
	domaintodo "analytix.local/runtime-go/internal/domain/todo"
)

const (
	MaxTodoItems      = 200
	MaxTodoOps        = 50
	MaxTodoNoteLength = 2000
)

var ErrTodoTerminalAuditRetention = errors.New("failed or canceled todos must be retained for audit")

func TodoReplacementMutation(threadID string, items []any) func(map[string]any, string) error {
	return func(thread map[string]any, now string) error {
		current, _ := thread["todos"].(map[string]any)
		next, err := NormalizeTodos(threadID, items, now)
		if err != nil {
			return err
		}
		if err := ValidateTodoReplacement(current, next); err != nil {
			return err
		}
		thread["todos"] = next
		return nil
	}
}

func TodoClearMutation(cleared *bool) func(map[string]any, string) error {
	return func(thread map[string]any, _ string) error {
		current, _ := thread["todos"].(map[string]any)
		if HasRetainedTerminalTodos(current) {
			return ErrTodoTerminalAuditRetention
		}
		_, *cleared = thread["todos"]
		delete(thread, "todos")
		return nil
	}
}

func NormalizeTodos(threadID string, items []any, now string) (map[string]any, error) {
	if len(items) > MaxTodoItems {
		return nil, fmt.Errorf("too many todos (max %d)", MaxTodoItems)
	}
	normalized := make([]any, 0, len(items))
	activeSeen := false
	seenIDs := map[string]bool{}
	reservedIDs := map[string]bool{}
	for index, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok || item == nil {
			continue
		}
		rawID, exists := item["id"]
		if !exists {
			continue
		}
		id, ok := rawID.(string)
		id = strings.TrimSpace(id)
		if !ok || id == "" {
			return nil, fmt.Errorf("todo %d id must be a non-empty string", index+1)
		}
		if reservedIDs[id] {
			return nil, fmt.Errorf("duplicate todo id %q", id)
		}
		reservedIDs[id] = true
	}
	for index, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok || item == nil {
			return nil, fmt.Errorf("todo %d must be an object", index+1)
		}
		if err := validateAllowedTodoKeys(item); err != nil {
			return nil, fmt.Errorf("todo %d: %w", index+1, err)
		}
		if _, ok := item["content"].(string); !ok {
			return nil, fmt.Errorf("todo %d content must be a string", index+1)
		}
		content := strings.TrimSpace(projectOrdinaryThreadText(stringField(item, "content")))
		if content == "" {
			return nil, fmt.Errorf("todo %d content is required", index+1)
		}
		status := strings.TrimSpace(stringField(item, "status"))
		if status == "" {
			if _, exists := item["status"]; exists {
				return nil, fmt.Errorf("todo %d status must be a non-empty string", index+1)
			}
			status = "pending"
		}
		parsedStatus, err := domaintodo.ParseStatus(status)
		if err != nil {
			return nil, fmt.Errorf("todo %d: %w", index+1, err)
		}
		reason, reasonExists, err := todoStatusReason(item)
		if err != nil {
			return nil, fmt.Errorf("todo %d: %w", index+1, err)
		}
		if !domaintodo.IsRetainedTerminal(parsedStatus) && reasonExists {
			return nil, fmt.Errorf("todo %d: todo status %q prohibits statusReasonCode", index+1, status)
		}
		if err := domaintodo.ValidateStatusReason(parsedStatus, reason); err != nil {
			return nil, fmt.Errorf("todo %d: %w", index+1, err)
		}
		if status == "in_progress" {
			if activeSeen {
				return nil, fmt.Errorf("at most one todo can be in_progress")
			}
			activeSeen = true
		}
		id := strings.TrimSpace(stringField(item, "id"))
		if id == "" {
			id = availableTodoID(seenIDs, reservedIDs, index+1)
		}
		if seenIDs[id] {
			return nil, fmt.Errorf("duplicate todo id %q", id)
		}
		seenIDs[id] = true
		createdAt := strings.TrimSpace(stringField(item, "createdAt"))
		if _, exists := item["createdAt"]; exists && createdAt == "" {
			return nil, fmt.Errorf("todo %d createdAt must be a non-empty string", index+1)
		}
		if createdAt == "" {
			createdAt = now
		}
		if rawUpdatedAt, exists := item["updatedAt"]; exists {
			value, ok := rawUpdatedAt.(string)
			if !ok || strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("todo %d updatedAt must be a non-empty string", index+1)
			}
		}
		next := map[string]any{
			"id":        id,
			"content":   content,
			"status":    status,
			"createdAt": createdAt,
			"updatedAt": now,
		}
		if reasonExists {
			next["statusReasonCode"] = reason
		}
		if rawSource, exists := item["source"]; exists {
			if rawSource == nil {
				return nil, fmt.Errorf("todo %d source must be an object", index+1)
			}
			source, err := normalizeTodoSource(rawSource)
			if err != nil {
				return nil, fmt.Errorf("todo %d source: %w", index+1, err)
			}
			if source != nil {
				next["source"] = source
			}
		}
		if rawNote, exists := item["note"]; exists {
			note, ok := rawNote.(string)
			if !ok {
				return nil, fmt.Errorf("todo %d note must be a string", index+1)
			}
			if note = truncateTodoText(projectOrdinaryThreadText(note), MaxTodoNoteLength); note != "" {
				next["note"] = note
			}
		}
		if evidenceIDs, exists := item["evidenceIds"]; exists {
			normalizedIDs, err := normalizeTodoEvidenceIDs(evidenceIDs)
			if err != nil {
				return nil, fmt.Errorf("todo %d: %w", index+1, err)
			}
			if len(normalizedIDs) > 0 {
				next["evidenceIds"] = normalizedIDs
			}
		}
		if parentTodoRef, exists := item["parentTodoRef"]; exists {
			value, ok := parentTodoRef.(string)
			value = strings.TrimSpace(value)
			if !ok || value == "" {
				return nil, fmt.Errorf("todo %d parentTodoRef must be a non-empty string", index+1)
			}
			next["parentTodoRef"] = value
		}
		normalized = append(normalized, next)
	}
	return map[string]any{
		"threadId":  threadID,
		"items":     normalized,
		"updatedAt": now,
	}, nil
}

func ValidateTodoReplacement(current map[string]any, next map[string]any) error {
	currentItems := cloneTodoItems(listAny(current["items"]))
	nextItems := cloneTodoItems(listAny(next["items"]))
	nextByID := make(map[string]map[string]any, len(nextItems))
	for _, item := range nextItems {
		nextByID[stringField(item, "id")] = item
	}
	currentByID := make(map[string]map[string]any, len(currentItems))
	for _, item := range currentItems {
		id := strings.TrimSpace(stringField(item, "id"))
		status, err := domaintodo.ParseStatus(stringField(item, "status"))
		if err != nil {
			return fmt.Errorf("current todo %q has invalid durable status", id)
		}
		currentByID[id] = item
		nextItem, exists := nextByID[id]
		if !exists {
			if domaintodo.IsRetainedTerminal(status) {
				return fmt.Errorf("%w: todo %q", ErrTodoTerminalAuditRetention, id)
			}
			continue
		}
		nextStatus, err := domaintodo.ParseStatus(stringField(nextItem, "status"))
		if err != nil {
			return err
		}
		switch status {
		case domaintodo.StatusCompleted:
			if nextStatus != domaintodo.StatusCompleted {
				return fmt.Errorf("completed todo %q cannot transition to %s", id, nextStatus)
			}
		case domaintodo.StatusFailed, domaintodo.StatusCanceled:
			if nextStatus != status {
				return fmt.Errorf("todo %q requires explicit retry before transition to %s", id, nextStatus)
			}
			if strings.TrimSpace(stringField(item, "statusReasonCode")) != strings.TrimSpace(stringField(nextItem, "statusReasonCode")) {
				return fmt.Errorf("todo %q terminal statusReasonCode is immutable until explicit retry", id)
			}
		case domaintodo.StatusPending, domaintodo.StatusInProgress:
			if domaintodo.IsRetainedTerminal(nextStatus) {
				return fmt.Errorf("todo %q requires explicit %s operation", id, map[domaintodo.Status]string{domaintodo.StatusFailed: "fail", domaintodo.StatusCanceled: "cancel"}[nextStatus])
			}
		}
	}
	for id, item := range nextByID {
		if _, exists := currentByID[id]; exists {
			continue
		}
		status, err := domaintodo.ParseStatus(stringField(item, "status"))
		if err != nil {
			return err
		}
		if domaintodo.IsRetainedTerminal(status) {
			return fmt.Errorf("new todo %q cannot start in terminal status %s", id, status)
		}
	}
	return nil
}

func HasRetainedTerminalTodos(todos map[string]any) bool {
	for _, raw := range listAny(todos["items"]) {
		item, _ := raw.(map[string]any)
		status, err := domaintodo.ParseStatus(stringField(item, "status"))
		if err == nil && domaintodo.IsRetainedTerminal(status) {
			return true
		}
	}
	return false
}

func IncompleteTodoCount(todos map[string]any) int {
	count := 0
	for _, raw := range listAny(todos["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "status") != "completed" {
			count++
		}
	}
	return count
}

func ApplyTodoOps(todos map[string]any, ops []any) ([]any, []any, error) {
	prepared, err := PrepareTodoOpsV1(todos, ops)
	if err != nil {
		return nil, nil, err
	}
	return prepared.NextItems, prepared.Applied, nil
}

// PreparedTodoOpsV1 is the owner-resolved operation set shared by semantic
// side-effect admission and persistence. Selectors are reduced to durable todo
// IDs against the same evolving list that execution will persist.
type PreparedTodoOpsV1 struct {
	NextItems  []any
	Applied    []any
	Operations []any
}

func PrepareTodoOpsV1(todos map[string]any, ops []any) (PreparedTodoOpsV1, error) {
	normalizedOps, err := NormalizeTodoOpsForSemanticIdentityV1(ops)
	if err != nil {
		return PreparedTodoOpsV1{}, err
	}
	items := cloneTodoItems(listAny(todos["items"]))
	if len(items) > MaxTodoItems {
		return PreparedTodoOpsV1{}, fmt.Errorf("too many existing todos (max %d)", MaxTodoItems)
	}
	applied := make([]any, 0, len(ops))
	resolvedOps := make([]any, 0, len(ops))
	for index, raw := range normalizedOps {
		op, _ := raw.(map[string]any)
		if op == nil {
			return PreparedTodoOpsV1{}, fmt.Errorf("op %d must be an object", index+1)
		}
		name := strings.TrimSpace(stringField(op, "op"))
		switch name {
		case "append":
			var result map[string]any
			var err error
			items, result, err = applyTodoAppend(items, op)
			if err != nil {
				return PreparedTodoOpsV1{}, fmt.Errorf("op %d append: %w", index+1, err)
			}
			resolved := contracts.CloneMap(op)
			delete(resolved, "id")
			target := "new:" + strconv.Itoa(index)
			if explicitID := strings.TrimSpace(stringField(op, "id")); explicitID != "" {
				target = "todo:" + explicitID
			}
			resolved["target"] = target
			resolvedOps = append(resolvedOps, resolved)
			applied = append(applied, result)
		case "start", "done", "fail", "cancel", "retry", "drop", "note":
			targetIndex, matchErr := matchTodoIndex(items, op)
			if matchErr != nil {
				return PreparedTodoOpsV1{}, fmt.Errorf("op %d %s: %w", index+1, name, matchErr)
			}
			targetID := stringField(items[targetIndex], "id")
			executionOp := contracts.CloneMap(op)
			executionOp["id"] = targetID
			delete(executionOp, "content")
			resolved := contracts.CloneMap(executionOp)
			resolved["target"] = "todo:" + targetID
			delete(resolved, "id")
			var result map[string]any
			var err error
			items, result, err = applyTodoMutation(items, executionOp, name)
			if err != nil {
				return PreparedTodoOpsV1{}, fmt.Errorf("op %d %s: %w", index+1, name, err)
			}
			resolvedOps = append(resolvedOps, resolved)
			applied = append(applied, result)
		default:
			return PreparedTodoOpsV1{}, fmt.Errorf("op %d has invalid operation %q", index+1, name)
		}
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return PreparedTodoOpsV1{NextItems: out, Applied: applied, Operations: resolvedOps}, nil
}

// NormalizeTodoOpsForSemanticIdentityV1 is the owner canonicalizer shared by
// execution and same-turn side-effect admission. It applies the same privacy,
// default, selector, provenance, and truncation rules before either layer can
// observe an operation.
func NormalizeTodoOpsForSemanticIdentityV1(ops []any) ([]any, error) {
	if len(ops) == 0 {
		return nil, fmt.Errorf("ops must include at least one operation")
	}
	if len(ops) > MaxTodoOps {
		return nil, fmt.Errorf("too many todo ops (max %d)", MaxTodoOps)
	}
	normalized := make([]any, 0, len(ops))
	for index, raw := range ops {
		op, _ := raw.(map[string]any)
		if op == nil {
			return nil, fmt.Errorf("op %d must be an object", index+1)
		}
		name := strings.TrimSpace(stringField(op, "op"))
		next := map[string]any{"op": name}
		switch name {
		case "append":
			if err := validateAllowedKeys(op, map[string]bool{
				"op": true, "id": true, "content": true, "status": true, "source": true, "note": true,
			}); err != nil {
				return nil, fmt.Errorf("op %d append: %w", index+1, err)
			}
			content := strings.TrimSpace(projectOrdinaryThreadText(stringField(op, "content")))
			if content == "" {
				return nil, fmt.Errorf("op %d append: content is required", index+1)
			}
			next["content"] = content
			if id := strings.TrimSpace(stringField(op, "id")); id != "" {
				next["id"] = id
			}
			status := strings.TrimSpace(stringField(op, "status"))
			if status == "" {
				status = "pending"
			}
			if status != "pending" && status != "in_progress" && status != "completed" {
				return nil, fmt.Errorf("op %d append: invalid status %q", index+1, status)
			}
			next["status"] = status
			source, sourceErr := normalizeTodoSource(op["source"])
			if sourceErr != nil {
				return nil, fmt.Errorf("op %d append: %w", index+1, sourceErr)
			}
			if source != nil {
				next["source"] = source
			}
			if note := truncateTodoText(projectOrdinaryThreadText(stringField(op, "note")), MaxTodoNoteLength); note != "" {
				next["note"] = note
			}
		case "start", "done", "fail", "cancel", "retry", "drop", "note":
			allowed := map[string]bool{"op": true, "id": true, "content": true}
			if name == "note" {
				allowed["note"] = true
			}
			if name == "fail" || name == "cancel" {
				allowed["statusReasonCode"] = true
			}
			if err := validateAllowedKeys(op, allowed); err != nil {
				return nil, fmt.Errorf("op %d %s: %w", index+1, name, err)
			}
			if id := strings.TrimSpace(stringField(op, "id")); id != "" {
				next["id"] = id
			} else {
				content := strings.TrimSpace(projectOrdinaryThreadText(stringField(op, "content")))
				if content == "" {
					return nil, fmt.Errorf("op %d %s: id or content is required", index+1, name)
				}
				next["content"] = content
			}
			if name == "note" {
				note := truncateTodoText(projectOrdinaryThreadText(stringField(op, "note")), MaxTodoNoteLength)
				if note == "" {
					return nil, fmt.Errorf("op %d note: note is required", index+1)
				}
				next["note"] = note
			}
			if name == "fail" || name == "cancel" {
				reason, exists, err := todoStatusReason(op)
				if err != nil || !exists {
					if err == nil {
						err = fmt.Errorf("statusReasonCode is required")
					}
					return nil, fmt.Errorf("op %d %s: %w", index+1, name, err)
				}
				status := domaintodo.StatusFailed
				if name == "cancel" {
					status = domaintodo.StatusCanceled
				}
				if err := domaintodo.ValidateStatusReason(status, reason); err != nil {
					return nil, fmt.Errorf("op %d %s: %w", index+1, name, err)
				}
				next["statusReasonCode"] = reason
			}
		default:
			return nil, fmt.Errorf("op %d has invalid operation %q", index+1, name)
		}
		normalized = append(normalized, next)
	}
	return normalized, nil
}

func applyTodoAppend(items []map[string]any, op map[string]any) ([]map[string]any, map[string]any, error) {
	if len(items) >= MaxTodoItems {
		return nil, nil, fmt.Errorf("too many todos (max %d)", MaxTodoItems)
	}
	content := strings.TrimSpace(projectOrdinaryThreadText(stringField(op, "content")))
	if content == "" {
		return nil, nil, fmt.Errorf("content is required")
	}
	id := strings.TrimSpace(stringField(op, "id"))
	if id == "" {
		id = nextTodoID(items, len(items)+1)
	} else if todoIndexByID(items, id) >= 0 {
		return nil, nil, fmt.Errorf("id %q already exists", id)
	}
	status := strings.TrimSpace(stringField(op, "status"))
	if status == "" {
		status = "pending"
	}
	if status != "pending" && status != "in_progress" && status != "completed" {
		return nil, nil, fmt.Errorf("invalid status %q", status)
	}
	if status == "in_progress" {
		clearInProgress(items, -1)
	}
	next := map[string]any{
		"id":      id,
		"content": content,
		"status":  status,
	}
	if source, err := normalizeTodoSource(op["source"]); err != nil {
		return nil, nil, err
	} else if source != nil {
		next["source"] = source
	}
	if note := truncateTodoText(projectOrdinaryThreadText(stringField(op, "note")), MaxTodoNoteLength); note != "" {
		next["note"] = note
	}
	items = append(items, next)
	return items, map[string]any{
		"op":     "append",
		"id":     id,
		"status": "applied",
	}, nil
}

func applyTodoMutation(items []map[string]any, op map[string]any, name string) ([]map[string]any, map[string]any, error) {
	index, err := matchTodoIndex(items, op)
	if err != nil {
		return nil, nil, err
	}
	item := items[index]
	currentStatus, err := domaintodo.ParseStatus(stringField(item, "status"))
	if err != nil {
		return nil, nil, err
	}
	previousStatus := currentStatus
	switch name {
	case "start":
		if err := domaintodo.ValidateOperationTransition(name, currentStatus, domaintodo.StatusInProgress); err != nil {
			return nil, nil, err
		}
		clearInProgress(items, index)
		item["status"] = "in_progress"
	case "done":
		if err := domaintodo.ValidateOperationTransition(name, currentStatus, domaintodo.StatusCompleted); err != nil {
			return nil, nil, err
		}
		item["status"] = "completed"
		delete(item, "statusReasonCode")
		ensureOneActiveAfter(items, index)
	case "fail", "cancel":
		nextStatus := domaintodo.StatusFailed
		if name == "cancel" {
			nextStatus = domaintodo.StatusCanceled
		}
		if err := domaintodo.ValidateOperationTransition(name, currentStatus, nextStatus); err != nil {
			return nil, nil, err
		}
		reason := strings.TrimSpace(stringField(op, "statusReasonCode"))
		if err := domaintodo.ValidateStatusReason(nextStatus, reason); err != nil {
			return nil, nil, err
		}
		wasActive := currentStatus == domaintodo.StatusInProgress
		item["status"] = string(nextStatus)
		item["statusReasonCode"] = reason
		if wasActive {
			ensureOneActiveAfter(items, index)
		}
	case "retry":
		if err := domaintodo.ValidateOperationTransition(name, currentStatus, domaintodo.StatusInProgress); err != nil {
			return nil, nil, err
		}
		clearInProgress(items, index)
		item["status"] = "in_progress"
		delete(item, "statusReasonCode")
	case "drop":
		if domaintodo.IsRetainedTerminal(currentStatus) {
			return nil, nil, fmt.Errorf("%w: todo %q", ErrTodoTerminalAuditRetention, stringField(item, "id"))
		}
		wasActive := stringField(item, "status") == "in_progress"
		items = append(items[:index], items[index+1:]...)
		if wasActive {
			ensureOneActiveAfter(items, index-1)
		}
	case "note":
		note := truncateTodoText(projectOrdinaryThreadText(stringField(op, "note")), MaxTodoNoteLength)
		if note == "" {
			return nil, nil, fmt.Errorf("note is required")
		}
		item["note"] = note
	}
	result := map[string]any{
		"op":             name,
		"id":             stringField(item, "id"),
		"status":         "applied",
		"previousStatus": string(previousStatus),
	}
	if name != "drop" {
		result["nextStatus"] = stringField(item, "status")
		if reason := strings.TrimSpace(stringField(item, "statusReasonCode")); reason != "" {
			result["statusReasonCode"] = reason
		}
	}
	return items, result, nil
}

func matchTodoIndex(items []map[string]any, op map[string]any) (int, error) {
	id := strings.TrimSpace(stringField(op, "id"))
	if id != "" {
		index := todoIndexByID(items, id)
		if index < 0 {
			return -1, fmt.Errorf("todo id %q not found", id)
		}
		return index, nil
	}
	content := strings.TrimSpace(projectOrdinaryThreadText(stringField(op, "content")))
	if content == "" {
		return -1, fmt.Errorf("id or content is required")
	}
	match := -1
	for index, item := range items {
		if strings.TrimSpace(stringField(item, "content")) != content {
			continue
		}
		if match >= 0 {
			return -1, fmt.Errorf("content %q matches multiple todos; use id", content)
		}
		match = index
	}
	if match < 0 {
		return -1, fmt.Errorf("todo content %q not found", content)
	}
	return match, nil
}

func clearInProgress(items []map[string]any, keepIndex int) {
	for index, item := range items {
		if index != keepIndex && stringField(item, "status") == "in_progress" {
			item["status"] = "pending"
		}
	}
}

func ensureOneActiveAfter(items []map[string]any, afterIndex int) {
	for _, item := range items {
		if stringField(item, "status") == "in_progress" {
			return
		}
	}
	for index := afterIndex + 1; index < len(items); index++ {
		if stringField(items[index], "status") == "pending" {
			items[index]["status"] = "in_progress"
			return
		}
	}
	for index := 0; index <= afterIndex && index < len(items); index++ {
		if stringField(items[index], "status") == "pending" {
			items[index]["status"] = "in_progress"
			return
		}
	}
}

func nextTodoID(items []map[string]any, seed int) string {
	used := map[string]bool{}
	for _, item := range items {
		used[stringField(item, "id")] = true
	}
	for index := seed; ; index++ {
		id := fmt.Sprintf("todo_%d", index)
		if !used[id] {
			return id
		}
	}
}

func availableTodoID(used map[string]bool, reserved map[string]bool, seed int) string {
	for index := seed; ; index++ {
		id := fmt.Sprintf("todo_%d", index)
		if !used[id] && !reserved[id] {
			return id
		}
	}
}

func todoIndexByID(items []map[string]any, id string) int {
	for index, item := range items {
		if stringField(item, "id") == id {
			return index
		}
	}
	return -1
}

func cloneTodoItems(raw []any) []map[string]any {
	out := make([]map[string]any, 0, len(raw))
	for _, value := range raw {
		item, _ := value.(map[string]any)
		if item == nil {
			continue
		}
		out = append(out, contracts.CloneMap(item))
	}
	return out
}

func normalizeTodoSource(value any) (map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	source, _ := value.(map[string]any)
	if source == nil {
		return nil, fmt.Errorf("source must be an object")
	}
	kind := strings.TrimSpace(stringField(source, "kind"))
	if kind == "" {
		return nil, fmt.Errorf("source.kind is required")
	}
	if kind != "manual" && kind != "plan" && kind != "child" {
		return nil, fmt.Errorf("source.kind must be manual, plan, or child")
	}
	allowed := map[string]bool{"kind": true}
	result := map[string]any{"kind": kind}
	copyOptionalString := func(key string) error {
		allowed[key] = true
		raw, exists := source[key]
		if !exists {
			return nil
		}
		value, ok := raw.(string)
		value = strings.TrimSpace(value)
		if !ok || value == "" {
			return fmt.Errorf("source.%s must be a non-empty string", key)
		}
		result[key] = value
		return nil
	}
	if kind == "plan" {
		for _, key := range []string{"planId", "relativePath", "contentHash"} {
			if err := copyOptionalString(key); err != nil {
				return nil, err
			}
		}
		allowed["ordinal"] = true
		if raw, exists := source["ordinal"]; exists {
			ordinal, ok := todoSourceOrdinal(raw)
			if !ok {
				return nil, fmt.Errorf("source.ordinal must be a non-negative integer")
			}
			result["ordinal"] = ordinal
		}
	}
	if kind == "child" {
		for _, key := range []string{"parentThreadId", "childThreadId", "childRunId", "jobId", "projectionId"} {
			if err := copyOptionalString(key); err != nil {
				return nil, err
			}
		}
	}
	for key := range source {
		if !allowed[key] {
			return nil, fmt.Errorf("source.%s is not allowed", key)
		}
	}
	return result, nil
}

func todoSourceOrdinal(value any) (any, bool) {
	switch typed := value.(type) {
	case int:
		return typed, typed >= 0
	case int32:
		return typed, typed >= 0
	case int64:
		return typed, typed >= 0
	case float64:
		return typed, typed >= 0 && math.Trunc(typed) == typed && typed <= 1<<53
	case json.Number:
		parsed, err := strconv.ParseInt(typed.String(), 10, 64)
		return parsed, err == nil && parsed >= 0
	default:
		return nil, false
	}
}

func validateAllowedTodoKeys(item map[string]any) error {
	return validateAllowedKeys(item, map[string]bool{
		"id": true, "content": true, "status": true, "statusReasonCode": true,
		"source": true, "note": true, "createdAt": true, "updatedAt": true,
		"evidenceIds": true, "parentTodoRef": true,
	})
}

func validateAllowedKeys(value map[string]any, allowed map[string]bool) error {
	for key := range value {
		if !allowed[key] {
			return fmt.Errorf("property %q is not allowed", key)
		}
	}
	return nil
}

func todoStatusReason(item map[string]any) (string, bool, error) {
	raw, exists := item["statusReasonCode"]
	if !exists {
		return "", false, nil
	}
	value, ok := raw.(string)
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return "", true, fmt.Errorf("statusReasonCode must be a non-empty string")
	}
	return value, true, nil
}

func normalizeTodoEvidenceIDs(value any) ([]any, error) {
	var raw []any
	switch typed := value.(type) {
	case []any:
		raw = typed
	case []string:
		raw = make([]any, 0, len(typed))
		for _, id := range typed {
			raw = append(raw, id)
		}
	default:
		return nil, fmt.Errorf("evidenceIds must be an array")
	}
	out := make([]any, 0, len(raw))
	seen := map[string]bool{}
	for _, item := range raw {
		id, ok := item.(string)
		id = strings.TrimSpace(id)
		if !ok || id == "" {
			return nil, fmt.Errorf("evidenceIds must contain non-empty strings")
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, nil
}

func truncateTodoText(value string, limit int) string {
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
