package threadsummary

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func IsSubagentJob(record domainjob.Record) bool {
	switch strings.TrimSpace(record.Kind) {
	case "subagent", "parallel-child-run", "child-run":
		return true
	default:
		return false
	}
}

func IsTaskJob(record domainjob.Record) bool {
	return !IsSubagentJob(record)
}

func IsChildTaskSubagentJob(record domainjob.Record) bool {
	if strings.TrimSpace(record.ChildThreadID) == "" {
		return false
	}
	switch strings.TrimSpace(record.Kind) {
	case "task", "parallel_task", "run_skill":
		return true
	default:
		return false
	}
}

func NormalizeActiveStatus(status string) string {
	switch PublicSummaryJobStatusV1(status) {
	case "queued", "running", "starting", "pause_requested", "paused", "resume_requested", "resuming":
		return "active"
	case "completed", "done":
		return "done"
	case "missing", "stopped", "archived", "idle":
		return "inactive"
	case "failed", "aborted", "interrupted", "killed", "canceled", "timeout":
		return "terminal"
	default:
		return "inactive"
	}
}

func PublicSummaryJobStatusV1(status string) string {
	status = strings.TrimSpace(status)
	if public := domainjob.PublicStatusV1(status); public != domainjob.PublicStatusUnknownV1 {
		return public
	}
	switch status {
	case "starting", "done", "missing", "stopped", "archived", "idle":
		return status
	default:
		return domainjob.PublicStatusUnknownV1
	}
}

func NormalizeTaskStatus(status string) string {
	return NormalizeActiveStatus(status)
}

func NormalizeCommandStatus(status string, isError bool) string {
	switch PublicCommandStatusV1(status, isError) {
	case "running", "pending":
		return "active"
	case "stopped":
		return "inactive"
	case "failed", "aborted", "killed", "canceled", "timeout", "error":
		return "terminal"
	case "completed", "done", "success":
		return "done"
	default:
		return "inactive"
	}
}

func PublicCommandStatusV1(status string, isError bool) string {
	status = strings.TrimSpace(status)
	switch status {
	case "running", "pending", "stopped", "failed", "aborted", "killed", "canceled", "timeout", "error", "completed", "done", "success":
		return status
	case "":
		if isError {
			return "error"
		}
		return "unknown"
	default:
		return "unknown"
	}
}

func CanKillJobStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "queued", "running", "starting":
		return true
	default:
		return false
	}
}

func CanRestartSubagentJob(record domainjob.Record) bool {
	if strings.TrimSpace(record.Prompt) == "" {
		return false
	}
	switch strings.TrimSpace(record.Status) {
	case "failed", "aborted", "interrupted", "killed", "timeout":
		return true
	default:
		return false
	}
}

func UpsertByUpdatedAt(target map[string]map[string]any, next map[string]any) {
	key := stringField(next, "key")
	if key == "" {
		return
	}
	existing := target[key]
	if existing == nil || stringField(next, "updatedAt") >= stringField(existing, "updatedAt") {
		merged := map[string]any{}
		for field, value := range existing {
			merged[field] = value
		}
		for field, value := range next {
			merged[field] = value
		}
		target[key] = merged
	}
}

func SortedValues(values map[string]map[string]any) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := out[i].(map[string]any)
		right := out[j].(map[string]any)
		if stringField(left, "updatedAt") == stringField(right, "updatedAt") {
			return stringField(left, "id") < stringField(right, "id")
		}
		return stringField(left, "updatedAt") > stringField(right, "updatedAt")
	})
	return out
}

func SetString(target map[string]any, key string, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		target[key] = value
	}
}

func ParallelIndexSeed(value int) string {
	if value > 0 {
		return strconv.Itoa(value)
	}
	return ""
}

func FloatParallelIndexSeed(value float64) string {
	if value > 0 {
		return strconv.Itoa(int(value))
	}
	return ""
}

func DisplayNameCandidate(candidate string, disallowedCandidates []string) string {
	display := subagentapp.DisplayNameCandidate(candidate)
	if display == "" || DisplayNameIsDisallowed(display, disallowedCandidates) {
		return ""
	}
	return display
}

func DistinctSubagentNameCandidate(name string, label string, profile string) string {
	name = strings.Join(strings.Fields(strings.TrimSpace(name)), " ")
	label = strings.Join(strings.Fields(strings.TrimSpace(label)), " ")
	profile = strings.Join(strings.Fields(strings.TrimSpace(profile)), " ")
	if name == "" || (label != "" && name == label) || (profile != "" && name == profile) {
		return ""
	}
	return name
}

func SubagentRecordDisplayLabel(record domainjob.Record) string {
	value := DistinctSubagentNameCandidate(record.Name, record.Label, record.ProfileName)
	if display := subagentapp.DisplayNameCandidate(value); display != "" {
		return display
	}
	return subagentapp.GeneratedNickname(subagentapp.ParallelIndexSeed(record.ParallelIndex), record.ID, record.ChildThreadID, record.ParentToolCallID)
}

func SubagentSourceDisplayLabel(record domainjob.Record, childThreadTitle string) string {
	value := DistinctSubagentNameCandidate(record.Name, record.Label, record.ProfileName)
	if display := subagentapp.DisplayNameCandidate(value); display != "" {
		return display
	}
	if title := NormalizeChildThreadTitle(childThreadTitle, record.ProfileName); title != "" {
		return title
	}
	return SubagentRecordDisplayLabel(record)
}

type ChildThreadTitleReader interface {
	GetThread(string) (map[string]any, error)
}

func ResolveSubagentSourceDisplayLabel(record domainjob.Record, reader ChildThreadTitleReader) string {
	childTitle := ""
	if reader != nil && strings.TrimSpace(record.ChildThreadID) != "" {
		if child, err := reader.GetThread(strings.TrimSpace(record.ChildThreadID)); err == nil && child != nil {
			childTitle = stringField(child, "title")
		}
	}
	return SubagentSourceDisplayLabel(record, childTitle)
}

func NormalizeChildThreadTitle(title string, disallowedCandidates ...string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(title), "child agent:") {
		return ""
	}
	title = strings.TrimSpace(strings.TrimSuffix(title, " fork"))
	return DisplayNameCandidate(title, disallowedCandidates)
}

func DisplayNameIsDisallowed(display string, disallowedCandidates []string) bool {
	normalized := CleanSubagentDisplayLabel(display)
	if normalized == "" {
		return false
	}
	for _, candidate := range disallowedCandidates {
		if normalized == CleanSubagentDisplayLabel(candidate) {
			return true
		}
	}
	return false
}

func CleanSubagentDisplayLabel(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if strings.HasPrefix(strings.ToLower(value), "child agent:") {
		value = strings.TrimSpace(value[len("Child agent:"):])
	}
	value = strings.TrimSpace(strings.TrimSuffix(value, " fork"))
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func TaskFromJob(record domainjob.Record, now time.Time) map[string]any {
	_ = now
	public := record
	public.ID = "taskjob:" + strings.TrimSpace(record.ID)
	return subagentapp.TaskJobMetadataProjectionV1(public, nil)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func stringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}

func numericAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return int(parsed), err == nil
	default:
		return 0, false
	}
}
