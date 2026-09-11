package subagent

import (
	"errors"
	"strings"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type JobSecurityThreadReader interface {
	GetThread(string) (map[string]any, error)
}

type JobSecurityUpdater interface {
	UpdateChildRun(string, domainjob.UpdateRequest) (domainjob.Record, error)
}

type JobSecurityAuthorizer struct {
	Threads                 JobSecurityThreadReader
	Jobs                    JobSecurityUpdater
	WorkspaceHasCaseBinding func(string) bool
}

func RecordMatchesSecurityContext(record domainjob.Record, context domainsecurity.TurnSecurityContext) bool {
	return strings.TrimSpace(record.ParentThreadID) == context.ThreadID && strings.TrimSpace(record.ParentTurnID) == context.TurnID &&
		strings.TrimSpace(record.ParentToolCallID) == SecurityBindingParentToolCallID(record) &&
		domainjob.SecurityBindingMatchesContext(record.SecurityBinding, context)
}

func SecurityBindingParentToolCallID(record domainjob.Record) string {
	if record.SecurityBinding == nil {
		return ""
	}
	return strings.TrimSpace(record.SecurityBinding.ParentToolCallID)
}

func RecordMatchesThreadSecurity(record domainjob.Record, thread map[string]any) bool {
	if thread == nil || record.SecurityBinding == nil {
		return false
	}
	context, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	return err == nil && RecordMatchesSecurityContext(record, context)
}

func SecurityBoundRecords(records []domainjob.Record, context domainsecurity.TurnSecurityContext) []domainjob.Record {
	out := make([]domainjob.Record, 0, len(records))
	for _, record := range records {
		if RecordMatchesSecurityContext(record, context) {
			out = append(out, record)
		}
	}
	return out
}

func (authorizer JobSecurityAuthorizer) Blocker(record domainjob.Record) string {
	return authorizer.BlockerAt(record, time.Now().UTC())
}

func (authorizer JobSecurityAuthorizer) BlockerAt(record domainjob.Record, now time.Time) string {
	if strings.TrimSpace(record.ParentThreadID) == "" || strings.TrimSpace(record.ParentTurnID) == "" {
		return "job_security_binding_missing"
	}
	status := strings.TrimSpace(record.Status)
	if record.SecurityBinding != nil && strings.TrimSpace(record.Kind) == "subagent" &&
		(status == string(domainjob.StatusQueued) || status == string(domainjob.StatusRunning)) &&
		domainjob.ValidateDelegatedToolManifestV1(record.DelegatedToolManifest, record.ToolScope, record.ToolSchemaHash) != nil {
		return "job_delegated_tool_manifest_invalid"
	}
	if err := domainjob.ValidateExecutableCaseDelegationV1(
		record.Kind, record.Name, record.Label, record.Prompt, record.SecurityBinding, record.CaseDelegation,
	); err != nil {
		return "job_case_delegation_invalid"
	}
	if authorizer.Threads == nil {
		return "job_security_authority_unavailable"
	}
	thread, err := authorizer.Threads.GetThread(record.ParentThreadID)
	if err != nil || thread == nil {
		return "parent_thread_missing"
	}
	if thread["securityState"] == nil {
		return "parent_security_context_mismatch"
	}
	if !RecordMatchesThreadSecurity(record, thread) {
		return "parent_security_context_mismatch"
	}
	if blocker := durableParentGrantBlocker(record, thread, now); blocker != "" {
		return blocker
	}
	return ""
}

// HistoricalBlockerAt authorizes only an exact typed projection back onto the
// already-terminal parent. It validates the frozen parent turn and its grant
// at the durable record time without treating a later admitted turn as the
// parent execution context.
func (authorizer JobSecurityAuthorizer) HistoricalBlockerAt(record domainjob.Record, now time.Time) string {
	if strings.TrimSpace(record.ParentThreadID) == "" || strings.TrimSpace(record.ParentTurnID) == "" || record.SecurityBinding == nil {
		return "job_security_binding_missing"
	}
	if err := domainjob.ValidateExecutableCaseDelegationV1(
		record.Kind, record.Name, record.Label, record.Prompt, record.SecurityBinding, record.CaseDelegation,
	); err != nil {
		return "job_case_delegation_invalid"
	}
	if authorizer.Threads == nil {
		return "job_security_authority_unavailable"
	}
	thread, err := authorizer.Threads.GetThread(record.ParentThreadID)
	if err != nil || thread == nil {
		return "parent_thread_missing"
	}
	parentTurn, ok := FindTurn(thread, record.ParentTurnID)
	if !ok {
		return "parent_turn_missing"
	}
	parent, err := domainsecurity.ParseTurnSecurityContext(parentTurn["securityContext"])
	if err != nil || !RecordMatchesSecurityContext(record, parent) {
		return "parent_security_context_mismatch"
	}
	return durableParentGrantBlocker(record, thread, now)
}

func durableParentGrantBlocker(record domainjob.Record, thread map[string]any, now time.Time) string {
	if record.SecurityBinding == nil || !jobRequiresParentGrantMembership(record) {
		return ""
	}
	registry, err := executiongrantapp.RegistryFromThread(record.ParentThreadID, thread, record.ParentTurnID)
	if err != nil {
		return "parent_execution_grant_registry_invalid"
	}
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, record.SecurityBinding.ParentExecutionGrantID)
	if !found {
		return "parent_execution_grant_missing"
	}
	grant := entry.Grant
	if entry.ContextDigest != record.SecurityBinding.ParentContextDigest || grant.ContextDigest != record.SecurityBinding.ParentContextDigest ||
		grant.TurnID != record.ParentTurnID || grant.ToolCallID != record.ParentToolCallID ||
		grant.ToolCallID != record.SecurityBinding.ParentToolCallID || !jobGrantToolMatches(record.Kind, grant.ToolName) {
		return "parent_execution_grant_mismatch"
	}
	if entry.Status != domainsecurity.GrantRegistryActive && entry.Status != domainsecurity.GrantRegistrySettled {
		return "parent_execution_grant_inactive"
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	if err != nil {
		return "parent_execution_grant_expiry_invalid"
	}
	if !now.UTC().Before(expiresAt) {
		return "parent_execution_grant_expired"
	}
	return ""
}

func jobRequiresParentGrantMembership(record domainjob.Record) bool {
	switch strings.TrimSpace(record.Kind) {
	case "subagent", "background-shell":
		return true
	default:
		return false
	}
}

func jobGrantToolMatches(kind string, toolName string) bool {
	switch strings.TrimSpace(kind) {
	case "subagent":
		switch strings.TrimSpace(toolName) {
		case "task", "delegate_task", "parallel_tasks", "run_skill":
			return true
		}
	case "background-shell":
		return strings.TrimSpace(toolName) == "bash"
	}
	return false
}

func securityThreadContainsTurn(thread map[string]any, turnID string) bool {
	turnID = strings.TrimSpace(turnID)
	turns, _ := thread["turns"].([]any)
	for _, value := range turns {
		turn, _ := value.(map[string]any)
		if securityStringField(turn, "id") == turnID {
			return true
		}
	}
	return false
}

func (authorizer JobSecurityAuthorizer) Allows(parentThreadID string, record domainjob.Record) bool {
	return authorizer.Blocker(record) == "" && strings.TrimSpace(record.ParentThreadID) == strings.TrimSpace(parentThreadID)
}

func (authorizer JobSecurityAuthorizer) MarkDeadLetter(record domainjob.Record, reason string) error {
	if authorizer.Jobs == nil || strings.TrimSpace(record.ID) == "" {
		return errors.New("job dead-letter authority is unavailable")
	}
	reason = domainjob.NormalizeOperationalReasonV1(reason)
	if reason == "" {
		return errors.New("job dead-letter reason is required")
	}
	suppressed := true
	_, err := authorizer.Jobs.UpdateChildRun(record.ID, domainjob.UpdateRequest{
		LateCompletionSuppressed: &suppressed, LateCompletionReason: reason,
		CompletionDeliveryStatus: "dead_letter", CompletionDeliveryReason: reason,
		DeadLetterReason: reason, RecoveryStatus: "dead_lettered", RecoveryReason: reason,
	})
	return err
}

func securityStringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}
