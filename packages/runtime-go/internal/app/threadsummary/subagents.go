package threadsummary

import (
	"strings"
	"time"

	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type SubagentContext struct {
	ParentThreadID    string
	ChildThreadTitles map[string]string
}

func SubagentChildThreadIDs(byKey map[string]map[string]any) map[string]bool {
	ids := map[string]bool{}
	for _, subagent := range byKey {
		if id := strings.TrimSpace(stringField(subagent, "childThreadId")); id != "" {
			ids[id] = true
		}
	}
	return ids
}

// ApplySubagentJobThreadAuthority restores immutable durable job authority
// after newer public lifecycle events are merged. Event projections may omit
// lineage, profile, policy, or execution bounds, but they cannot revoke or
// contradict those job-owned values for the same child run.
func ApplySubagentJobThreadAuthority(byKey map[string]map[string]any, records []domainjob.Record) {
	for _, record := range records {
		projected := byKey["run:"+strings.TrimSpace(record.ID)]
		if projected == nil {
			continue
		}
		SetString(projected, "parentThreadId", record.ParentThreadID)
		SetString(projected, "parentTurnId", record.ParentTurnID)
		SetString(projected, "parentToolCallId", record.ParentToolCallID)
		SetString(projected, "childId", record.ID)
		SetString(projected, "childRunId", record.ID)
		SetString(projected, "childThreadId", record.ChildThreadID)
		SetString(projected, "childTurnId", record.ChildTurnID)
		SetString(projected, "profile", record.ProfileName)
		SetString(projected, "toolPolicy", record.ToolPolicy)
		if record.MaxModelSteps != nil && *record.MaxModelSteps >= 0 {
			projected["maxModelSteps"] = float64(*record.MaxModelSteps)
		}
		if record.TimeBudgetMs > 0 {
			projected["timeBudgetMs"] = float64(record.TimeBudgetMs)
		}
		projected["canOpenThread"] = strings.TrimSpace(record.ChildThreadID) != ""
	}
}

func ChildThreadIDs(records []domainjob.Record, events []map[string]any) map[string]bool {
	ids := map[string]bool{}
	for _, record := range records {
		if id := strings.TrimSpace(record.ChildThreadID); id != "" {
			ids[id] = true
		}
	}
	for _, event := range events {
		child, _ := event["child"].(map[string]any)
		if child == nil {
			continue
		}
		if id := firstNonEmptyAnyString(child["childThreadId"]); id != "" {
			ids[id] = true
		}
	}
	return ids
}

func SubagentFromJob(context SubagentContext, record domainjob.Record, now time.Time) map[string]any {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	key := "run:" + record.ID
	updatedAt := firstNonEmptyString(record.UpdatedAt, record.FinishedAt, record.StartedAt, record.QueuedAt, now.Format(time.RFC3339Nano))
	displayName := SubagentDisplayName(
		context,
		record.ChildThreadID,
		nil,
		nil,
		[]string{
			ParallelIndexSeed(record.ParallelIndex),
			record.ID,
			record.ChildThreadID,
			record.ParentToolCallID,
			DistinctSubagentNameCandidate(record.Name, record.Label, record.ProfileName),
		},
		[]string{record.ProfileName},
	)
	out := map[string]any{
		"schemaVersion":  1,
		"id":             key,
		"key":            key,
		"parentThreadId": record.ParentThreadID,
		"taskJobId":      record.ID,
		"taskKind":       domainjob.PublicKindV1(record.Kind),
		"status":         NormalizeActiveStatus(record.Status),
		"rawStatus":      PublicSummaryJobStatusV1(record.Status),
		"canOpenThread":  strings.TrimSpace(record.ChildThreadID) != "",
		"canKill":        false,
		"canRestart":     false,
		"canReadOutput":  false,
		"updatedAt":      updatedAt,
	}
	applyChildOutputWithholding(out, record)
	AddSubagentJobDiagnostics(out, record, now)
	SetString(out, "parentTurnId", record.ParentTurnID)
	SetString(out, "parentToolCallId", record.ParentToolCallID)
	SetString(out, "childId", record.ID)
	SetString(out, "childRunId", record.ID)
	SetString(out, "childThreadId", record.ChildThreadID)
	SetString(out, "childTurnId", record.ChildTurnID)
	SetString(out, "displayName", displayName)
	SetString(out, "agentNickname", displayName)
	SetString(out, "title", displayName)
	SetString(out, "label", displayName)
	SetString(out, "model", record.Model)
	SetString(out, "providerId", record.ProviderID)
	SetString(out, "endpointFormat", domainmodel.OptionalEndpointFormat(record.EndpointFormat))
	SetString(out, "variant", record.Variant)
	SetModelSource(out, record.ModelSource)
	SetReasoningEffort(out, record.Effort)
	SetString(out, "profile", record.ProfileName)
	SetString(out, "toolPolicy", record.ToolPolicy)
	if record.MaxModelSteps != nil && *record.MaxModelSteps >= 0 {
		out["maxModelSteps"] = float64(*record.MaxModelSteps)
	}
	if record.TimeBudgetMs > 0 {
		out["timeBudgetMs"] = float64(record.TimeBudgetMs)
	}
	if record.Background {
		out["background"] = true
	}
	SetString(out, "parallelGroupId", record.ParallelGroupID)
	if record.ParallelIndex > 0 {
		out["parallelIndex"] = float64(record.ParallelIndex)
	}
	if record.ToolInvocations > 0 {
		out["toolInvocations"] = float64(record.ToolInvocations)
	}
	if durationMs := DurationMs(record); durationMs > 0 {
		out["durationMs"] = durationMs
	}
	if record.QueuedMs > 0 {
		out["queuedMs"] = float64(record.QueuedMs)
	}
	if record.Usage != nil {
		for _, key := range []string{"totalTokens", "cacheHitRate", "costUsd", "costCny"} {
			if value := floatFromAny(record.Usage[key]); value > 0 {
				out[key] = value
			}
		}
	}
	SetString(out, "createdAt", firstNonEmptyString(record.QueuedAt, record.StartedAt))
	return out
}

func SubagentFromTaskJob(context SubagentContext, record domainjob.Record, now time.Time) map[string]any {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	key := "run:" + record.ID
	updatedAt := firstNonEmptyString(record.UpdatedAt, record.FinishedAt, record.StartedAt, record.QueuedAt, now.Format(time.RFC3339Nano))
	displayName := SubagentDisplayName(
		context,
		record.ChildThreadID,
		nil,
		nil,
		[]string{ParallelIndexSeed(record.ParallelIndex), record.ID, record.ChildThreadID, record.ParentToolCallID},
		[]string{record.ProfileName, record.Label},
	)
	out := map[string]any{
		"schemaVersion":  1,
		"id":             key,
		"key":            key,
		"parentThreadId": record.ParentThreadID,
		"taskJobId":      record.ID,
		"taskKind":       domainjob.PublicKindV1(record.Kind),
		"status":         NormalizeTaskStatus(record.Status),
		"rawStatus":      PublicSummaryJobStatusV1(record.Status),
		"canOpenThread":  strings.TrimSpace(record.ChildThreadID) != "",
		"canKill":        false,
		"canRestart":     false,
		"canReadOutput":  false,
		"updatedAt":      updatedAt,
	}
	applyChildOutputWithholding(out, record)
	AddSubagentJobDiagnostics(out, record, now)
	SetString(out, "parentTurnId", record.ParentTurnID)
	SetString(out, "parentToolCallId", record.ParentToolCallID)
	SetString(out, "childId", record.ID)
	SetString(out, "childRunId", record.ID)
	SetString(out, "childThreadId", record.ChildThreadID)
	SetString(out, "childTurnId", record.ChildTurnID)
	SetString(out, "displayName", displayName)
	SetString(out, "agentNickname", displayName)
	SetString(out, "title", displayName)
	SetString(out, "label", displayName)
	if record.Background {
		out["background"] = true
	}
	SetString(out, "parallelGroupId", record.ParallelGroupID)
	if record.ParallelIndex > 0 {
		out["parallelIndex"] = float64(record.ParallelIndex)
	}
	SetString(out, "createdAt", firstNonEmptyString(record.QueuedAt, record.StartedAt))
	return out
}

func AddSubagentJobDiagnostics(out map[string]any, record domainjob.Record, now time.Time) {
	if out == nil {
		return
	}
	diagnostics := summaryJobDiagnostics(record)
	out["diagnostics"] = diagnostics
}

func summaryJobDiagnostics(record domainjob.Record) map[string]any {
	status := PublicSummaryJobStatusV1(record.Status)
	normalized := NormalizeTaskStatus(status)
	diagnostics := map[string]any{
		"status":     status,
		"terminal":   normalized == "done" || normalized == "terminal",
		"background": record.Background,
		"paused":     status == "paused" || status == "pause_requested",
	}
	SetString(diagnostics, "startedAt", record.StartedAt)
	SetString(diagnostics, "updatedAt", record.UpdatedAt)
	SetString(diagnostics, "finishedAt", record.FinishedAt)
	return diagnostics
}

func applyChildOutputWithholding(out map[string]any, record domainjob.Record) {
	if out == nil {
		return
	}
	projection := subagentapp.ChildOutputMetadataProjection(record)
	for _, key := range []string{"outputWithheld", "outputTrustStatus", "factAnswerAllowed", "evidenceAuthority", "canReadOutput", "canContinueParent"} {
		out[key] = projection[key]
	}
}

func applyChildOutputMapWithholding(out map[string]any, child map[string]any) {
	applyChildOutputWithholding(out, domainjob.Record{
		ID:         firstNonEmptyAnyString(child["childRunId"], child["childId"], child["jobId"], child["id"]),
		Status:     firstNonEmptyAnyString(child["childStatus"], child["status"]),
		Background: boolField(child, "background"),
	})
}

func SubagentFromEventChild(context SubagentContext, child map[string]any, now time.Time) map[string]any {
	parentThreadID := firstNonEmptyAnyString(child["parentThreadId"], context.ParentThreadID)
	if parentThreadID != context.ParentThreadID {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	childRunID := firstNonEmptyAnyString(child["childRunId"], child["childId"])
	childThreadID := firstNonEmptyAnyString(child["childThreadId"])
	updatedAt := firstNonEmptyAnyString(
		child["updatedAt"],
		child["timestamp"],
		child["finishedAt"],
		child["completedAt"],
		child["startedAt"],
		child["queuedAt"],
		child["createdAt"],
	)
	if updatedAt == "" {
		updatedAt = now.Format(time.RFC3339Nano)
	}
	key := "call:" + firstNonEmptyAnyString(child["parentToolCallId"], childRunID, childThreadID)
	if childRunID != "" {
		key = "run:" + childRunID
	} else if childThreadID != "" {
		key = "thread:" + childThreadID
	}
	out := map[string]any{
		"schemaVersion":  1,
		"id":             key,
		"key":            key,
		"parentThreadId": parentThreadID,
		"status":         NormalizeActiveStatus(firstNonEmptyAnyString(child["childStatus"], child["status"])),
		"rawStatus":      PublicSummaryJobStatusV1(firstNonEmptyAnyString(child["childStatus"], child["status"])),
		"canOpenThread":  childThreadID != "",
		"canKill":        false,
		"canRestart":     false,
		"updatedAt":      updatedAt,
	}
	applyChildOutputMapWithholding(out, child)
	displayName := SubagentDisplayName(
		context,
		childThreadID,
		nil,
		nil,
		[]string{
			FloatParallelIndexSeed(floatFromAny(child["parallelIndex"])),
			childRunID,
			firstNonEmptyAnyString(child["childId"]),
			firstNonEmptyAnyString(child["parentToolCallId"]),
			childThreadID,
			DistinctSubagentNameCandidate(
				firstNonEmptyAnyString(child["childName"]),
				firstNonEmptyAnyString(child["childLabel"]),
				firstNonEmptyAnyString(child["childProfile"], child["profile"]),
			),
		},
		[]string{
			firstNonEmptyAnyString(child["childProfile"], child["profile"]),
		},
	)
	SetString(out, "parentTurnId", firstNonEmptyAnyString(child["parentTurnId"]))
	SetString(out, "parentToolCallId", firstNonEmptyAnyString(child["parentToolCallId"]))
	SetString(out, "childId", firstNonEmptyAnyString(child["childId"]))
	SetString(out, "childRunId", childRunID)
	SetString(out, "childThreadId", childThreadID)
	SetString(out, "childTurnId", firstNonEmptyAnyString(child["childTurnId"]))
	SetString(out, "displayName", displayName)
	SetString(out, "agentNickname", displayName)
	SetString(out, "title", displayName)
	SetString(out, "label", displayName)
	SetString(out, "model", firstNonEmptyAnyString(child["childModel"], child["model"]))
	SetString(out, "providerId", firstNonEmptyAnyString(child["childProviderId"], child["providerId"]))
	SetString(out, "endpointFormat", domainmodel.OptionalEndpointFormat(firstNonEmptyAnyString(child["childEndpointFormat"], child["endpointFormat"])))
	SetString(out, "variant", firstNonEmptyAnyString(child["childModelVariant"], child["variant"]))
	SetModelSource(out, firstNonEmptyAnyString(child["childModelSource"], child["modelSource"]))
	if rawEffort, ok := child["effort"].(string); ok {
		SetReasoningEffort(out, rawEffort)
	}
	SetString(out, "profile", firstNonEmptyAnyString(child["childProfile"], child["profile"]))
	SetString(out, "toolPolicy", firstNonEmptyAnyString(child["childToolPolicy"], child["toolPolicy"]))
	if raw, exists := child["maxModelSteps"]; exists && nonNegativeIntegerV1(raw) {
		out["maxModelSteps"] = floatFromAny(raw)
	}
	if raw, exists := child["timeBudgetMs"]; exists && nonNegativeIntegerV1(raw) {
		out["timeBudgetMs"] = floatFromAny(raw)
	}
	if value := floatFromAny(child["durationMs"]); value > 0 {
		out["durationMs"] = value
	}
	if value := floatFromAny(child["queuedMs"]); value > 0 {
		out["queuedMs"] = value
	}
	if value := floatFromAny(child["toolInvocations"]); value > 0 {
		out["toolInvocations"] = value
	}
	if value := floatFromAny(child["parallelIndex"]); value > 0 {
		out["parallelIndex"] = value
	}
	if _, ok := child["evidenceLedgered"]; ok {
		out["evidenceLedgered"] = boolField(child, "evidenceLedgered")
	}
	if value := floatFromAny(child["totalTokens"]); value > 0 {
		out["totalTokens"] = value
	}
	if value := floatFromAny(child["cacheHitRate"]); value > 0 {
		out["cacheHitRate"] = value
	}
	if value := floatFromAny(child["costUsd"]); value > 0 {
		out["costUsd"] = value
	}
	if value := floatFromAny(child["costCny"]); value > 0 {
		out["costCny"] = value
	}
	if boolField(child, "background") {
		out["background"] = true
	}
	return out
}

func SetModelSource(target map[string]any, value string) {
	value = strings.TrimSpace(value)
	switch value {
	case "thread", "subagent-profile", "explicit-input", "session", "runtime-default":
		target["modelSource"] = value
	}
}

func SetReasoningEffort(target map[string]any, value string) {
	projected, valid := domainmodel.ProjectReasoningEffortV1(value)
	if !valid {
		return
	}
	SetString(target, "effort", projected)
}

func SubagentDisplayName(context SubagentContext, childThreadID string, primaryCandidates []string, fallbackCandidates []string, seedCandidates []string, disallowedCandidates []string) string {
	for _, candidate := range primaryCandidates {
		if display := DisplayNameCandidate(candidate, disallowedCandidates); display != "" {
			return display
		}
	}
	if title := NormalizeChildThreadTitle(context.ChildThreadTitles[childThreadID], disallowedCandidates...); title != "" {
		return title
	}
	for _, candidate := range fallbackCandidates {
		if display := DisplayNameCandidate(candidate, disallowedCandidates); display != "" {
			return display
		}
	}
	candidates := []string{}
	candidates = append(candidates, seedCandidates...)
	candidates = append(candidates, childThreadID)
	candidates = append(candidates, primaryCandidates...)
	candidates = append(candidates, fallbackCandidates...)
	return subagentapp.GeneratedNickname(candidates...)
}

func DurationMs(record domainjob.Record) float64 {
	started := strings.TrimSpace(record.StartedAt)
	if started == "" {
		return 0
	}
	ended := firstNonEmptyString(record.FinishedAt, record.UpdatedAt)
	if strings.TrimSpace(ended) == "" {
		return 0
	}
	startTime, err := time.Parse(time.RFC3339Nano, started)
	if err != nil {
		return 0
	}
	endTime, err := time.Parse(time.RFC3339Nano, ended)
	if err != nil || endTime.Before(startTime) {
		return 0
	}
	return float64(endTime.Sub(startTime).Milliseconds())
}
