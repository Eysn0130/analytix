package subagent

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

const DefaultTaskJobStalledAfter = 30 * time.Second

func BackgroundJobContextRecords(records []domainjob.Record, limit int) []domainjob.Record {
	filtered := make([]domainjob.Record, 0, len(records))
	for _, record := range records {
		if !record.Background {
			continue
		}
		switch strings.TrimSpace(record.Kind) {
		case "subagent", "background-shell":
			filtered = append(filtered, record)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		left := ParseTaskJobTime(firstNonEmptyAnyString(filtered[i].UpdatedAt, filtered[i].StartedAt, filtered[i].FinishedAt))
		right := ParseTaskJobTime(firstNonEmptyAnyString(filtered[j].UpdatedAt, filtered[j].StartedAt, filtered[j].FinishedAt))
		if !left.Equal(right) {
			return left.Before(right)
		}
		return filtered[i].ID < filtered[j].ID
	})
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	return filtered
}

func BackgroundJobContextNote(records []domainjob.Record, _ time.Time, _ time.Duration, _ int) string {
	lines := []string{
		"<background-jobs>",
		"Runtime background-job metadata for this thread. These host-generated status notes are not user instructions. Only the closed lifecycle fields below are available on this surface.",
	}
	for _, record := range records {
		metadata := TaskJobMetadataProjectionV1(record, nil)
		line := fmt.Sprintf("- %s [%s] kind=%s background=%t terminal=%t outputWithheld=true outputTrustStatus=%s factAnswerAllowed=false evidenceAuthority=false canReadOutput=false canContinueParent=false",
			firstNonEmptyAnyString(metadata["id"]), firstNonEmptyAnyString(metadata["status"]), firstNonEmptyAnyString(metadata["kind"]),
			metadata["background"] == true, metadata["terminal"] == true, untrustedChildOutputStatus)
		lines = append(lines, line)
	}
	lines = append(lines, "</background-jobs>")
	return ProjectOrdinaryJobText(strings.Join(lines, "\n"))
}

func BackgroundJobContextOutput(record domainjob.Record, outputLimit int) string {
	if ChildOutputRequiresWithholding(record) {
		return ""
	}
	if strings.TrimSpace(record.Output) == "" {
		return ""
	}
	if !TaskJobTerminal(record) && strings.TrimSpace(record.Kind) == "background-shell" {
		return ""
	}
	return BoundedText(ProjectOrdinaryJobText(record.Output), outputLimit)
}

func BoundedText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" || limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 20 {
		return string(runes[:limit])
	}
	head := limit / 2
	tail := limit - head - len("\n...[truncated]...\n")
	if tail < 0 {
		tail = 0
	}
	return string(runes[:head]) + "\n...[truncated]...\n" + string(runes[len(runes)-tail:])
}

func TaskJobRecordsAny(records []domainjob.Record, now time.Time, stalledAfter time.Duration) []any {
	out := make([]any, 0, len(records))
	for _, record := range records {
		out = append(out, TaskJobRecordView(record, now, stalledAfter))
	}
	return out
}

func TaskJobRecordsAnyWithState(records []domainjob.Record, now time.Time, stalledAfter time.Duration, state *RuntimeState) []any {
	active := map[string]bool{}
	if state != nil {
		for _, id := range state.ActiveBackgroundJobIDs() {
			active[id] = true
		}
	}
	out := make([]any, 0, len(records))
	for _, record := range records {
		isActive := active[record.ID]
		out = append(out, taskJobRecordView(record, now, stalledAfter, &isActive))
	}
	return out
}

func TaskJobRecordView(record domainjob.Record, now time.Time, stalledAfter time.Duration) map[string]any {
	return taskJobRecordView(record, now, stalledAfter, nil)
}

func taskJobRecordView(record domainjob.Record, now time.Time, stalledAfter time.Duration, active *bool) map[string]any {
	return TaskJobSummaryResponseV1(record, now, stalledAfter, active)
}

func TaskJobDiagnostics(record domainjob.Record, now time.Time, stalledAfter time.Duration) map[string]any {
	return TaskJobSummaryResponseV1(record, now, stalledAfter, nil)
}

// taskJobControlDiagnostics is host-internal control state. Callers must not
// return it directly for a security-bound child; public projection remains the
// fixed metadata-only view above.
func taskJobControlDiagnostics(record domainjob.Record, now time.Time, stalledAfter time.Duration) map[string]any {
	return taskJobControlDiagnosticsWithState(record, now, stalledAfter, nil)
}

func taskJobControlDiagnosticsWithState(record domainjob.Record, now time.Time, stalledAfter time.Duration, active *bool) map[string]any {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	updatedAt := ParseTaskJobTime(record.UpdatedAt)
	startedAt := ParseTaskJobTime(record.StartedAt)
	finishedAt := ParseTaskJobTime(record.FinishedAt)
	lastHeartbeatAt := ParseTaskJobTime(record.LastHeartbeatAt)
	lastActivityAt := updatedAt
	if lastActivityAt.IsZero() {
		lastActivityAt = startedAt
	}
	if lastActivityAt.IsZero() {
		lastActivityAt = now
	}
	if lastHeartbeatAt.IsZero() {
		lastHeartbeatAt = lastActivityAt
	}
	ageMs := int64(0)
	if !startedAt.IsZero() && now.After(startedAt) {
		ageMs = now.Sub(startedAt).Milliseconds()
	}
	idleMs := int64(0)
	if now.After(lastActivityAt) {
		idleMs = now.Sub(lastActivityAt).Milliseconds()
	}
	heartbeatAgeMs := int64(0)
	if now.After(lastHeartbeatAt) {
		heartbeatAgeMs = now.Sub(lastHeartbeatAt).Milliseconds()
	}
	terminal := TaskJobTerminal(record)
	paused := strings.TrimSpace(record.Status) == string(domainjob.StatusPaused)
	staleAfterMs := record.StaleAfterMs
	if staleAfterMs <= 0 && stalledAfter > 0 {
		staleAfterMs = stalledAfter.Milliseconds()
	}
	stalled := !terminal && !paused && staleAfterMs > 0 && heartbeatAgeMs >= staleAfterMs
	leaseExpiresAt := ParseTaskJobTime(record.LeaseExpiresAt)
	heartbeatStatus := TaskJobHeartbeatStatus(record, now, heartbeatAgeMs, staleAfterMs, leaseExpiresAt, terminal, paused)
	out := map[string]any{
		"status":          domainjob.PublicStatusV1(record.Status),
		"heartbeatStatus": heartbeatStatus,
		"terminal":        terminal,
		"background":      record.Background,
		"paused":          paused,
		"outputBytes":     float64(len([]byte(record.Output))),
		"nextOffset":      float64(len([]byte(record.Output))),
		"ageMs":           float64(ageMs),
		"idleMs":          float64(idleMs),
		"stalled":         stalled,
		"staleAfterMs":    float64(staleAfterMs),
		"stalledAfterMs":  float64(staleAfterMs),
		"lastActivityAt":  lastActivityAt.Format(time.RFC3339Nano),
		"lastHeartbeatAt": lastHeartbeatAt.Format(time.RFC3339Nano),
		"heartbeatAt":     lastHeartbeatAt.Format(time.RFC3339Nano),
		"heartbeatAgeMs":  float64(heartbeatAgeMs),
	}
	if active != nil {
		out["active"] = *active
	}
	if leaseOwner := publicTaskJobLeaseOwnerV1(record.LeaseOwner); leaseOwner != "" {
		out["leaseOwner"] = leaseOwner
	}
	if !leaseExpiresAt.IsZero() {
		out["leaseExpiresAt"] = leaseExpiresAt.Format(time.RFC3339Nano)
		if now.After(leaseExpiresAt) {
			out["leaseExpired"] = true
		}
	}
	if record.Orphaned {
		out["orphaned"] = true
	}
	if strings.TrimSpace(record.RecoveryStatus) != "" {
		out["recoveryStatus"] = domainjob.PublicRecoveryStatusV1(record.RecoveryStatus)
	}
	if record.RecoveryAttempt > 0 {
		out["recoveryAttempt"] = float64(record.RecoveryAttempt)
	}
	if strings.TrimSpace(record.RecoveryReason) != "" {
		out["recoveryReason"] = domainjob.NormalizeOperationalReasonV1(record.RecoveryReason)
	}
	addTaskJobPublicTimestampV1(out, "recoveryUpdatedAt", record.RecoveryUpdatedAt)
	if strings.TrimSpace(record.DeadLetterReason) != "" {
		out["deadLetterReason"] = domainjob.NormalizeOperationalReasonV1(record.DeadLetterReason)
	}
	if !startedAt.IsZero() {
		out["startedAt"] = startedAt.Format(time.RFC3339Nano)
	}
	if !updatedAt.IsZero() {
		out["updatedAt"] = updatedAt.Format(time.RFC3339Nano)
	}
	if !finishedAt.IsZero() {
		out["finishedAt"] = finishedAt.Format(time.RFC3339Nano)
	}
	if record.Output != "" {
		out["lastOutputAt"] = lastActivityAt.Format(time.RFC3339Nano)
	}
	if strings.TrimSpace(record.PauseState.Status) != "" {
		out["pauseStatus"] = domainjob.PublicPauseStatusV1(record.PauseState.Status)
	}
	addTaskJobPublicTimestampV1(out, "lastPausedAt", record.PauseState.LastPausedAt)
	addTaskJobPublicTimestampV1(out, "lastResumedAt", record.PauseState.LastResumedAt)
	if strings.TrimSpace(record.AutoContinueStatus) != "" {
		out["autoContinueParent"] = record.AutoContinueParent
		out["autoContinueStatus"] = domainjob.PublicAutoContinueStatusV1(record.AutoContinueStatus)
	}
	if strings.TrimSpace(record.AutoContinueReason) != "" {
		out["autoContinueReason"] = domainjob.NormalizeOperationalReasonV1(record.AutoContinueReason)
	}
	if record.LateCompletionSuppressed {
		out["lateCompletionSuppressed"] = true
	}
	if strings.TrimSpace(record.LateCompletionReason) != "" {
		out["lateCompletionReason"] = domainjob.NormalizeOperationalReasonV1(record.LateCompletionReason)
	}
	if strings.TrimSpace(record.CompletionDeliveryStatus) != "" {
		out["deliveryStatus"] = domainjob.PublicCompletionDeliveryStatusV1(record.CompletionDeliveryStatus)
	}
	if strings.TrimSpace(record.CompletionDeliveryReason) != "" {
		out["deliveryReason"] = domainjob.NormalizeOperationalReasonV1(record.CompletionDeliveryReason)
	}
	addTaskJobPublicTimestampV1(out, "completionDeliveryAt", record.CompletionDeliveryAt)
	addTaskJobPublicTimestampV1(out, "completionDeadLetterAt", record.CompletionDeadLetterAt)
	if record.CompletionDeliveryAttempts > 0 {
		out["completionDeliveryAttempt"] = float64(record.CompletionDeliveryAttempts)
	}
	if failureCode := domainjob.NormalizeFailureCode(record.FailureCode, record.Status, false); failureCode != "" {
		out["failureCode"] = failureCode
	}
	if warningCode := TaskJobHeartbeatWarningCode(heartbeatStatus); warningCode != "" {
		out["warningCode"] = warningCode
		out["warning"] = TaskJobHeartbeatWarning(record, heartbeatStatus, heartbeatAgeMs)
		suggestedTools := []any{"bash_output", "wait", "kill_shell"}
		suggestion := ""
		jobID := publicTaskJobReferenceIDV1(record.ID)
		if jobID != "" {
			suggestion = fmt.Sprintf("Use bash_output(jobId=%q) to inspect recent output, wait(jobIds=[%q]) to poll completion, or kill_shell(jobId=%q) to cancel it.", jobID, jobID, jobID)
		}
		if JobIsSubagent(record) {
			suggestedTools = []any{"bash_output", "wait", "steer_job", "pause_job", "kill_shell"}
			if jobID != "" {
				suggestion = fmt.Sprintf("Use bash_output(jobId=%q) to inspect recent output, steer_job(jobId=%q) to append guidance, wait(jobIds=[%q]) to poll completion, pause_job(jobId=%q) to request a pause, or kill_shell(jobId=%q) to cancel it.", jobID, jobID, jobID, jobID, jobID)
			}
		}
		if heartbeatStatus == "lease_expired" || heartbeatStatus == "orphaned" || heartbeatStatus == "dead_lettered" {
			suggestedTools = []any{"bash_output", "wait", "restart", "kill_shell"}
			if jobID != "" {
				suggestion = fmt.Sprintf("Inspect output for job %q, then restart it or kill it if the old run should not continue.", jobID)
			}
		}
		out["suggestedTools"] = suggestedTools
		if suggestion != "" {
			out["suggestion"] = suggestion
		}
	}
	return out
}

func addTaskJobPublicTimestampV1(out map[string]any, key string, value string) {
	if parsed := ParseTaskJobTime(value); !parsed.IsZero() {
		out[key] = parsed.Format(time.RFC3339Nano)
	}
}

func publicTaskJobLeaseOwnerV1(value string) string {
	value = strings.TrimSpace(value)
	if value == "analytix-runtime" {
		return value
	}
	const prefix = "analytix-runtime:"
	if !strings.HasPrefix(value, prefix) || len(value) == len(prefix) {
		return ""
	}
	for _, char := range value[len(prefix):] {
		if char < '0' || char > '9' {
			return ""
		}
	}
	return "analytix-runtime"
}

func publicTaskJobReferenceIDV1(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') ||
			char == '-' || char == '_' || char == '.' || char == ':' {
			continue
		}
		return ""
	}
	return value
}

func TaskJobHeartbeatStatus(record domainjob.Record, now time.Time, heartbeatAgeMs int64, staleAfterMs int64, leaseExpiresAt time.Time, terminal bool, paused bool) string {
	if domainjob.PublicCompletionDeliveryStatusV1(record.CompletionDeliveryStatus) == "dead_letter" ||
		domainjob.PublicRecoveryStatusV1(record.RecoveryStatus) == "dead_lettered" {
		return "dead_lettered"
	}
	switch domainjob.PublicRecoveryStatusV1(record.RecoveryStatus) {
	case "recovering":
		return "recovering"
	case "recovered":
		return "recovered"
	}
	if record.Orphaned {
		return "orphaned"
	}
	if terminal {
		return domainjob.PublicStatusV1(record.Status)
	}
	if paused {
		return "paused"
	}
	switch strings.TrimSpace(record.Status) {
	case string(domainjob.StatusQueued), string(domainjob.StatusRunning), string(domainjob.StatusPauseRequested), string(domainjob.StatusResumeRequested), string(domainjob.StatusResuming):
		if !leaseExpiresAt.IsZero() && now.After(leaseExpiresAt) {
			return "lease_expired"
		}
		if staleAfterMs > 0 && heartbeatAgeMs >= staleAfterMs {
			return "stale"
		}
		return "running"
	default:
		return domainjob.PublicStatusV1(record.Status)
	}
}

func TaskJobHeartbeatWarningCode(status string) string {
	switch strings.TrimSpace(status) {
	case "stale":
		return "task_job_stale"
	case "lease_expired":
		return "task_job_lease_expired"
	case "orphaned":
		return "task_job_orphaned"
	case "recovering":
		return "task_job_recovering"
	case "dead_lettered":
		return "task_job_dead_lettered"
	default:
		return ""
	}
}

func TaskJobHeartbeatWarning(record domainjob.Record, status string, heartbeatAgeMs int64) string {
	jobID := publicTaskJobReferenceIDV1(record.ID)
	if jobID == "" {
		jobID = "task-job"
	}
	switch strings.TrimSpace(status) {
	case "stale":
		return fmt.Sprintf("background job %s has no heartbeat for %dms; inspect output or stop it if it is stuck", jobID, heartbeatAgeMs)
	case "lease_expired":
		return fmt.Sprintf("background job %s lease expired; restart or kill it before trusting late output", jobID)
	case "orphaned":
		return fmt.Sprintf("background job %s has no active runtime owner after recovery scan", jobID)
	case "recovering":
		return fmt.Sprintf("background job %s is being recovered", jobID)
	case "dead_lettered":
		return fmt.Sprintf("background job %s could not deliver its completion notification", jobID)
	default:
		return ""
	}
}

func FilterTaskJobRecords(records []domainjob.Record, request TaskJobListRequest) []domainjob.Record {
	out := make([]domainjob.Record, 0, len(records))
	status := strings.TrimSpace(request.Status)
	for _, record := range records {
		if status != "" && strings.TrimSpace(record.Status) != status {
			continue
		}
		if request.Background != nil && record.Background != *request.Background {
			continue
		}
		out = append(out, record)
	}
	if request.Limit > 0 && len(out) > request.Limit {
		out = out[len(out)-request.Limit:]
	}
	return out
}

func TaskJobOutputView(record domainjob.Record, offset int, limit int, now time.Time, stalledAfter time.Duration) map[string]any {
	view, err := TaskJobOutputViewWithFilter(record, offset, limit, "", now, stalledAfter)
	if err != nil {
		fallback := map[string]any{
			"id":          record.ID,
			"status":      domainjob.PublicStatusV1(record.Status),
			"output":      "",
			"offset":      float64(0),
			"nextOffset":  float64(0),
			"outputBytes": float64(len([]byte(record.Output))),
			"truncated":   false,
			"diagnostics": TaskJobDiagnostics(record, now, stalledAfter),
		}
		AddIsolationMetadata(fallback, record)
		AddChildTodoMetadata(fallback, record)
		return ProjectOrdinaryJobMap(fallback)
	}
	return view
}

func TaskJobOutputViewWithFilter(record domainjob.Record, offset int, limit int, filter string, now time.Time, stalledAfter time.Duration) (map[string]any, error) {
	if ChildOutputRequiresWithholding(record) {
		return TaskJobOutputWithheldResponseV1(record), nil
	}
	outputBytes := []byte(record.Output)
	if offset < 0 {
		offset = 0
	}
	if offset > len(outputBytes) {
		offset = len(outputBytes)
	}
	end := len(outputBytes)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	output := string(outputBytes[offset:end])
	filter = strings.TrimSpace(filter)
	filterApplied := false
	if filter != "" {
		filtered, err := FilterLines(output, filter)
		if err != nil {
			return nil, err
		}
		output = filtered
		filterApplied = true
	}
	view := map[string]any{
		"id":          record.ID,
		"status":      domainjob.PublicStatusV1(record.Status),
		"output":      output,
		"offset":      float64(offset),
		"nextOffset":  float64(end),
		"outputBytes": float64(len(outputBytes)),
		"rawBytes":    float64(end - offset),
		"truncated":   end < len(outputBytes),
		"diagnostics": TaskJobDiagnostics(record, now, stalledAfter),
	}
	AddIsolationMetadata(view, record)
	AddChildTodoMetadata(view, record)
	if filterApplied {
		view["filter"] = filter
		view["filterApplied"] = true
		view["filteredBytes"] = float64(len([]byte(output)))
	}
	return ProjectOrdinaryJobMap(view), nil
}

func FilterLines(value string, pattern string) (string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return value, nil
	}
	rx, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid filter regexp: %w", err)
	}
	lines := strings.Split(value, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if rx.MatchString(line) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n"), nil
}

func ParseTaskJobTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed.UTC()
	}
	return time.Time{}
}

func TaskJobsTerminal(records []domainjob.Record) bool {
	if len(records) == 0 {
		return true
	}
	for _, record := range records {
		if !TaskJobTerminal(record) {
			return false
		}
	}
	return true
}

func TaskJobTerminal(record domainjob.Record) bool {
	return domainjob.TerminalStatusV1(record.Status)
}
