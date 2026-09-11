package research

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type AutoResearchRequirementStatus string

const (
	AutoResearchRequirementPending   AutoResearchRequirementStatus = "pending"
	AutoResearchRequirementCompleted AutoResearchRequirementStatus = "completed"
)

type AutoResearchRequirement struct {
	ID            string                        `json:"id"`
	Text          string                        `json:"text"`
	Status        AutoResearchRequirementStatus `json:"status"`
	EvidenceCount int                           `json:"evidenceCount"`
	UpdatedAt     string                        `json:"updatedAt"`
}

type AutoResearchProgress struct {
	SchemaVersion int                       `json:"schemaVersion"`
	ThreadID      string                    `json:"threadId"`
	Objective     string                    `json:"objective"`
	Status        string                    `json:"status"`
	Requirements  []AutoResearchRequirement `json:"requirements"`
	CreatedAt     string                    `json:"createdAt"`
	UpdatedAt     string                    `json:"updatedAt"`
}

type AutoResearchDescriptor struct {
	Enabled             bool   `json:"enabled"`
	StateRelativePath   string `json:"stateRelativePath"`
	TaskSpecPath        string `json:"taskSpecPath"`
	ProgressPath        string `json:"progressPath"`
	FindingsPath        string `json:"findingsPath"`
	DirectionsTriedPath string `json:"directionsTriedPath"`
	IterationLogPath    string `json:"iterationLogPath"`
	RequirementCount    int    `json:"requirementCount"`
}

type AutoResearchSnapshot struct {
	Descriptor AutoResearchDescriptor `json:"descriptor"`
	Progress   AutoResearchProgress   `json:"progress"`
	Resumed    bool                   `json:"resumed"`
}

type AutoResearchRequirementAudit struct {
	Complete     bool `json:"complete"`
	Requirements []struct {
		ID            string                        `json:"id"`
		Text          string                        `json:"text"`
		Status        AutoResearchRequirementStatus `json:"status"`
		HasEvidence   bool                          `json:"hasEvidence"`
		EvidenceCount int                           `json:"evidenceCount"`
	} `json:"requirements"`
}

type AutoResearchDirectionRecord struct {
	ID        string `json:"id"`
	Direction string `json:"direction"`
	Outcome   string `json:"outcome"`
	Summary   string `json:"summary,omitempty"`
	CreatedAt string `json:"createdAt"`
}

type autoResearchPaths struct {
	stateRelativePath        string
	absoluteStateDir         string
	taskSpecRelativePath     string
	progressRelativePath     string
	findingsRelativePath     string
	directionsRelativePath   string
	iterationLogRelativePath string
	taskSpecAbsolutePath     string
	progressAbsolutePath     string
	findingsAbsolutePath     string
	directionsAbsolutePath   string
	iterationLogAbsolutePath string
}

type AutoResearchProjectStore struct {
	nowIso func() string
}

func NewAutoResearchProjectStore(nowIso func() string) AutoResearchProjectStore {
	if nowIso == nil {
		nowIso = func() string { return time.Now().UTC().Format(time.RFC3339Nano) }
	}
	return AutoResearchProjectStore{nowIso: nowIso}
}

func (s AutoResearchProjectStore) CreateOrResume(workspace, threadID, objective string, requirements []string) (AutoResearchSnapshot, error) {
	paths, err := autoResearchPathsFor(workspace, threadID)
	if err != nil {
		return AutoResearchSnapshot{}, err
	}
	if err := os.MkdirAll(paths.absoluteStateDir, 0o700); err != nil {
		return AutoResearchSnapshot{}, err
	}
	progress, found, err := readAutoResearchProgress(paths.progressAbsolutePath)
	if err != nil {
		return AutoResearchSnapshot{}, err
	}
	if !found {
		now := s.nowIso()
		normalized := normalizeResearchRequirements(requirements, objective)
		items := make([]AutoResearchRequirement, 0, len(normalized))
		for index, text := range normalized {
			items = append(items, AutoResearchRequirement{
				ID:            fmt.Sprintf("req_%d", index+1),
				Text:          text,
				Status:        AutoResearchRequirementPending,
				EvidenceCount: 0,
				UpdatedAt:     now,
			})
		}
		progress = AutoResearchProgress{
			SchemaVersion: 1,
			ThreadID:      threadID,
			Objective:     strings.TrimSpace(objective),
			Status:        "active",
			Requirements:  items,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := writeAutoResearchJSONFile(paths.progressAbsolutePath, progress); err != nil {
			return AutoResearchSnapshot{}, err
		}
		if err := os.WriteFile(paths.taskSpecAbsolutePath, []byte(renderAutoResearchTaskSpec(progress)), 0o600); err != nil {
			return AutoResearchSnapshot{}, err
		}
		if err := ensureAutoResearchFile(paths.findingsAbsolutePath, ""); err != nil {
			return AutoResearchSnapshot{}, err
		}
		if err := ensureAutoResearchFile(paths.iterationLogAbsolutePath, ""); err != nil {
			return AutoResearchSnapshot{}, err
		}
		if err := ensureAutoResearchFile(paths.directionsAbsolutePath, autoResearchDirectionsJSON(threadID, nil)); err != nil {
			return AutoResearchSnapshot{}, err
		}
	} else {
		if err := ensureAutoResearchFile(paths.taskSpecAbsolutePath, renderAutoResearchTaskSpec(progress)); err != nil {
			return AutoResearchSnapshot{}, err
		}
		if err := ensureAutoResearchFile(paths.findingsAbsolutePath, ""); err != nil {
			return AutoResearchSnapshot{}, err
		}
		if err := ensureAutoResearchFile(paths.iterationLogAbsolutePath, ""); err != nil {
			return AutoResearchSnapshot{}, err
		}
		if err := ensureAutoResearchFile(paths.directionsAbsolutePath, autoResearchDirectionsJSON(threadID, nil)); err != nil {
			return AutoResearchSnapshot{}, err
		}
	}
	return AutoResearchSnapshot{
		Descriptor: autoResearchDescriptorFromPaths(paths, len(progress.Requirements)),
		Progress:   progress,
		Resumed:    found,
	}, nil
}

func (s AutoResearchProjectStore) RecordDirection(workspace, threadID, direction, outcome, summary string) (AutoResearchDirectionRecord, error) {
	paths, err := autoResearchPathsFor(workspace, threadID)
	if err != nil {
		return AutoResearchDirectionRecord{}, err
	}
	direction = strings.TrimSpace(direction)
	if direction == "" {
		return AutoResearchDirectionRecord{}, errors.New("research direction is required")
	}
	outcome = strings.TrimSpace(outcome)
	switch outcome {
	case "tried", "promising", "dead_end":
	default:
		return AutoResearchDirectionRecord{}, errors.New("research direction outcome must be one of tried, promising, or dead_end")
	}
	record := AutoResearchDirectionRecord{
		ID:        "dir_" + shortHexHash([]byte(direction + s.nowIso()))[:10],
		Direction: direction,
		Outcome:   outcome,
		CreatedAt: s.nowIso(),
	}
	if trimmed := strings.TrimSpace(summary); trimmed != "" {
		record.Summary = trimmed
	}
	directions, err := readAutoResearchDirections(paths.directionsAbsolutePath, threadID)
	if err != nil {
		return AutoResearchDirectionRecord{}, err
	}
	directions = append(directions, record)
	if err := writeAutoResearchDirections(paths.directionsAbsolutePath, threadID, directions); err != nil {
		return AutoResearchDirectionRecord{}, err
	}
	if err := appendAutoResearchJSONL(paths.iterationLogAbsolutePath, map[string]any{
		"kind":        "direction_recorded",
		"directionId": record.ID,
		"direction":   record.Direction,
		"outcome":     record.Outcome,
		"createdAt":   record.CreatedAt,
	}); err != nil {
		return AutoResearchDirectionRecord{}, err
	}
	return record, nil
}

func (s AutoResearchProjectStore) RecordEvidence(workspace, threadID, requirementID, step string, evidence []string, summary string) (AutoResearchSnapshot, error) {
	paths, err := autoResearchPathsFor(workspace, threadID)
	if err != nil {
		return AutoResearchSnapshot{}, err
	}
	progress, found, err := readAutoResearchProgress(paths.progressAbsolutePath)
	if err != nil {
		return AutoResearchSnapshot{}, err
	}
	if !found {
		return AutoResearchSnapshot{}, errors.New("autoresearch progress does not exist")
	}
	normalizedEvidence := normalizeResearchEvidence(evidence)
	if len(normalizedEvidence) == 0 {
		return AutoResearchSnapshot{}, errors.New("research evidence is required")
	}
	requirementID = strings.TrimSpace(requirementID)
	if requirementID != "" && !autoResearchHasRequirement(progress, requirementID) {
		return AutoResearchSnapshot{}, fmt.Errorf("unknown research requirement: %s", requirementID)
	}
	now := s.nowIso()
	if requirementID != "" {
		for index := range progress.Requirements {
			if progress.Requirements[index].ID == requirementID {
				progress.Requirements[index].Status = AutoResearchRequirementCompleted
				progress.Requirements[index].EvidenceCount += len(normalizedEvidence)
				progress.Requirements[index].UpdatedAt = now
			}
		}
	}
	progress.Status = "complete"
	for _, requirement := range progress.Requirements {
		if requirement.Status != AutoResearchRequirementCompleted {
			progress.Status = "active"
			break
		}
	}
	progress.UpdatedAt = now
	if err := writeAutoResearchJSONFile(paths.progressAbsolutePath, progress); err != nil {
		return AutoResearchSnapshot{}, err
	}
	if err := appendAutoResearchJSONL(paths.findingsAbsolutePath, map[string]any{
		"kind":          "finding",
		"requirementId": requirementID,
		"step":          strings.TrimSpace(step),
		"evidence":      normalizedEvidence,
		"summary":       strings.TrimSpace(summary),
		"createdAt":     now,
	}); err != nil {
		return AutoResearchSnapshot{}, err
	}
	if err := appendAutoResearchJSONL(paths.iterationLogAbsolutePath, map[string]any{
		"kind":          "evidence_recorded",
		"requirementId": requirementID,
		"evidenceCount": len(normalizedEvidence),
		"createdAt":     now,
	}); err != nil {
		return AutoResearchSnapshot{}, err
	}
	return AutoResearchSnapshot{
		Descriptor: autoResearchDescriptorFromPaths(paths, len(progress.Requirements)),
		Progress:   progress,
		Resumed:    true,
	}, nil
}

func (s AutoResearchProjectStore) AuditRequirements(workspace, threadID string) (AutoResearchRequirementAudit, error) {
	paths, err := autoResearchPathsFor(workspace, threadID)
	if err != nil {
		return AutoResearchRequirementAudit{}, err
	}
	progress, found, err := readAutoResearchProgress(paths.progressAbsolutePath)
	if err != nil {
		return AutoResearchRequirementAudit{}, err
	}
	if !found {
		return AutoResearchRequirementAudit{}, errors.New("autoresearch progress does not exist")
	}
	audit := AutoResearchRequirementAudit{Complete: len(progress.Requirements) > 0}
	for _, requirement := range progress.Requirements {
		row := struct {
			ID            string                        `json:"id"`
			Text          string                        `json:"text"`
			Status        AutoResearchRequirementStatus `json:"status"`
			HasEvidence   bool                          `json:"hasEvidence"`
			EvidenceCount int                           `json:"evidenceCount"`
		}{
			ID:            requirement.ID,
			Text:          requirement.Text,
			Status:        requirement.Status,
			HasEvidence:   requirement.EvidenceCount > 0,
			EvidenceCount: requirement.EvidenceCount,
		}
		if row.Status != AutoResearchRequirementCompleted || !row.HasEvidence {
			audit.Complete = false
		}
		audit.Requirements = append(audit.Requirements, row)
	}
	return audit, nil
}

func AutoResearchStateAuditEvent(snapshot AutoResearchSnapshot, directionCount int, threadID, turnID string) map[string]any {
	completed := 0
	for _, requirement := range snapshot.Progress.Requirements {
		if requirement.Status == AutoResearchRequirementCompleted && requirement.EvidenceCount > 0 {
			completed += 1
		}
	}
	stale := len(snapshot.Progress.Requirements) - completed
	result := "created"
	if snapshot.Resumed {
		result = "resumed"
	}
	if stale > 0 && directionCount > 0 {
		result = "pivot_required"
	}
	if stale == 0 && len(snapshot.Progress.Requirements) > 0 {
		result = "complete"
	}
	return map[string]any{
		"kind":                                 "autoresearch_state_audit",
		"threadId":                             threadID,
		"turnId":                               turnID,
		"schemaVersion":                        1,
		"changeId":                             "autoresearch-state-audit",
		"runtimeContract":                      "analytix-go-runtime",
		"upstreamSource":                       "reasonix-absorbed",
		"goalMode":                             "research",
		"fileCount":                            len(autoResearchRequiredFiles()),
		"requirementCount":                     len(snapshot.Progress.Requirements),
		"completedRequirementCount":            completed,
		"staleRequirementCount":                stale,
		"staleDirectionCount":                  directionCount,
		"complete":                             result == "complete",
		"pivotRequired":                        result == "pivot_required",
		"result":                               result,
		"unknownRequirementAccepted":           false,
		"findingsWrittenForUnknownRequirement": false,
		"writesReasonixFile":                   false,
		"writesAgentsFile":                     false,
		"stablePrefixContainsState":            false,
		"toolSchemaContainsState":              false,
		"topLevelAutoResearchRouteExposed":     false,
		"usesReasonixPublicProtocol":           false,
		"usesReasonixConfigRoot":               false,
		"changesRendererContract":              false,
		"changesProductIdentity":               false,
	}
}

func autoResearchPathsFor(workspace, threadID string) (autoResearchPaths, error) {
	if strings.TrimSpace(workspace) == "" {
		return autoResearchPaths{}, errors.New("workspace is required")
	}
	if !filepath.IsAbs(workspace) {
		return autoResearchPaths{}, fmt.Errorf("workspace must be absolute: %s", workspace)
	}
	workspaceRoot, err := filepath.Abs(workspace)
	if err != nil {
		return autoResearchPaths{}, err
	}
	workspaceRoot, err = filepath.EvalSymlinks(workspaceRoot)
	if err != nil {
		return autoResearchPaths{}, fmt.Errorf("autoresearch workspace is unavailable: %w", err)
	}
	workspaceInfo, err := os.Stat(workspaceRoot)
	if err != nil {
		return autoResearchPaths{}, fmt.Errorf("autoresearch workspace is unavailable: %w", err)
	}
	if !workspaceInfo.IsDir() {
		return autoResearchPaths{}, errors.New("autoresearch workspace is not a directory")
	}
	workspaceRoot = filepath.Clean(workspaceRoot)
	safeThreadID := autoResearchSafePathSegment(threadID)
	absoluteStateDir := filepath.Join(workspaceRoot, ".analytix", "autoresearch", safeThreadID)
	relativeStateDir, err := filepath.Rel(filepath.Clean(workspaceRoot), filepath.Clean(absoluteStateDir))
	if err != nil {
		return autoResearchPaths{}, err
	}
	if relativeStateDir == "." || strings.HasPrefix(relativeStateDir, ".."+string(filepath.Separator)) || relativeStateDir == ".." || filepath.IsAbs(relativeStateDir) {
		return autoResearchPaths{}, errors.New("autoresearch state path escaped workspace")
	}
	stateRelativePath := filepath.ToSlash(filepath.Join(".analytix", "autoresearch", safeThreadID))
	return autoResearchPaths{
		stateRelativePath:        stateRelativePath,
		absoluteStateDir:         absoluteStateDir,
		taskSpecRelativePath:     stateRelativePath + "/task_spec.md",
		progressRelativePath:     stateRelativePath + "/progress.json",
		findingsRelativePath:     stateRelativePath + "/findings.jsonl",
		directionsRelativePath:   stateRelativePath + "/directions_tried.json",
		iterationLogRelativePath: stateRelativePath + "/iteration_log.jsonl",
		taskSpecAbsolutePath:     filepath.Join(absoluteStateDir, "task_spec.md"),
		progressAbsolutePath:     filepath.Join(absoluteStateDir, "progress.json"),
		findingsAbsolutePath:     filepath.Join(absoluteStateDir, "findings.jsonl"),
		directionsAbsolutePath:   filepath.Join(absoluteStateDir, "directions_tried.json"),
		iterationLogAbsolutePath: filepath.Join(absoluteStateDir, "iteration_log.jsonl"),
	}, nil
}

func autoResearchSafePathSegment(value string) string {
	value = strings.TrimSpace(value)
	var out strings.Builder
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.' || r == '-' {
			out.WriteRune(r)
		} else {
			out.WriteByte('_')
		}
	}
	cleaned := out.String()
	for strings.HasPrefix(cleaned, ".") {
		cleaned = strings.TrimPrefix(cleaned, ".")
	}
	cleaned = strings.ReplaceAll(cleaned, "..", "__")
	if cleaned == "" {
		cleaned = "thread"
	}
	return cleaned
}

func autoResearchDescriptorFromPaths(paths autoResearchPaths, requirementCount int) AutoResearchDescriptor {
	return AutoResearchDescriptor{
		Enabled:             true,
		StateRelativePath:   paths.stateRelativePath,
		TaskSpecPath:        paths.taskSpecRelativePath,
		ProgressPath:        paths.progressRelativePath,
		FindingsPath:        paths.findingsRelativePath,
		DirectionsTriedPath: paths.directionsRelativePath,
		IterationLogPath:    paths.iterationLogRelativePath,
		RequirementCount:    requirementCount,
	}
}

func normalizeResearchRequirements(requirements []string, objective string) []string {
	out := []string{}
	for _, requirement := range requirements {
		if trimmed := strings.TrimSpace(requirement); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		if trimmed := strings.TrimSpace(objective); trimmed != "" {
			out = append(out, trimmed)
		} else {
			out = append(out, "Research objective")
		}
	}
	return out
}

func normalizeResearchEvidence(evidence []string) []string {
	out := []string{}
	for _, item := range evidence {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func autoResearchHasRequirement(progress AutoResearchProgress, requirementID string) bool {
	for _, requirement := range progress.Requirements {
		if requirement.ID == requirementID {
			return true
		}
	}
	return false
}

func renderAutoResearchTaskSpec(progress AutoResearchProgress) string {
	lines := []string{
		"# AutoResearch Task Spec",
		"",
		"Thread: " + progress.ThreadID,
		"Status: " + progress.Status,
		"",
		"## Objective",
		"",
		progress.Objective,
		"",
		"## Requirements",
		"",
	}
	for _, requirement := range progress.Requirements {
		marker := " "
		if requirement.Status == AutoResearchRequirementCompleted {
			marker = "x"
		}
		lines = append(lines, fmt.Sprintf("- [%s] %s: %s", marker, requirement.ID, requirement.Text))
	}
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

func readAutoResearchProgress(path string) (AutoResearchProgress, bool, error) {
	var progress AutoResearchProgress
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return progress, false, nil
	}
	if err != nil {
		return progress, false, err
	}
	if err := json.Unmarshal(data, &progress); err != nil {
		return progress, false, err
	}
	return progress, true, nil
}

func readAutoResearchDirections(path string, threadID string) ([]AutoResearchDirectionRecord, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []AutoResearchDirectionRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	var payload struct {
		SchemaVersion int                           `json:"schemaVersion"`
		ThreadID      string                        `json:"threadId"`
		Directions    []AutoResearchDirectionRecord `json:"directions"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	if payload.ThreadID != "" && payload.ThreadID != threadID {
		return nil, fmt.Errorf("directions thread mismatch: %s", payload.ThreadID)
	}
	return payload.Directions, nil
}

func writeAutoResearchDirections(path string, threadID string, directions []AutoResearchDirectionRecord) error {
	if directions == nil {
		directions = []AutoResearchDirectionRecord{}
	}
	return writeAutoResearchJSONFile(path, map[string]any{
		"schemaVersion": 1,
		"threadId":      threadID,
		"directions":    directions,
	})
}

func autoResearchDirectionsJSON(threadID string, directions []AutoResearchDirectionRecord) string {
	if directions == nil {
		directions = []AutoResearchDirectionRecord{}
	}
	data, _ := json.MarshalIndent(map[string]any{
		"schemaVersion": 1,
		"threadId":      threadID,
		"directions":    directions,
	}, "", "  ")
	return string(data) + "\n"
}

func writeAutoResearchJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func ensureAutoResearchFile(path string, content string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(content)
	return err
}

func appendAutoResearchJSONL(path string, value map[string]any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(data, '\n'))
	return err
}

func autoResearchRequiredFiles() []string {
	return AutoResearchRequiredFiles()
}

func AutoResearchRequiredFiles() []string {
	return []string{
		"task_spec.md",
		"progress.json",
		"findings.jsonl",
		"directions_tried.json",
		"iteration_log.jsonl",
	}
}

func IsResearchGoalPrompt(prompt string) bool {
	lower := strings.ToLower(prompt)
	return strings.Contains(lower, "/goal") && strings.Contains(lower, "--research")
}

func ResearchObjectiveFromPrompt(prompt string) string {
	objective := strings.TrimSpace(prompt)
	for _, token := range []string{"/goal", "--research"} {
		objective = strings.TrimSpace(strings.ReplaceAll(objective, token, ""))
	}
	if objective == "" {
		return "Research goal"
	}
	return objective
}

func shortHexHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
