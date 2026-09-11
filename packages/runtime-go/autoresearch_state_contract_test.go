//go:build !analytix_prod

package runtimego

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	research "analytix.local/runtime-go/internal/research"
)

func TestAutoResearchProjectStateCreatesOnlyProjectLocalFiles(t *testing.T) {
	workspace := t.TempDir()
	store := research.NewAutoResearchProjectStore(func() string { return "2026-06-23T00:00:00.000Z" })

	snapshot, err := store.CreateOrResume(workspace, "../../AGENTS.md", "Map provider cache behavior", []string{
		"Compare providers",
		"Record unsupported fallback",
	})
	if err != nil {
		t.Fatalf("create autoresearch state: %v", err)
	}
	if !strings.HasPrefix(snapshot.Descriptor.StateRelativePath, ".analytix/autoresearch/") ||
		!strings.HasSuffix(snapshot.Descriptor.StateRelativePath, "AGENTS.md") {
		t.Fatalf("thread id must be sanitized into project-local state path: %#v", snapshot.Descriptor)
	}
	if strings.Contains(snapshot.Descriptor.StateRelativePath, "..") {
		t.Fatalf("state path must not contain path escape segments: %#v", snapshot.Descriptor.StateRelativePath)
	}
	for _, fileName := range research.AutoResearchRequiredFiles() {
		path := filepath.Join(workspace, snapshot.Descriptor.StateRelativePath, fileName)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("required AutoResearch file missing %s: %v", fileName, err)
		}
	}
	for _, forbidden := range []string{"REASONIX.md", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(workspace, forbidden)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("AutoResearch store must not write %s: %v", forbidden, err)
		}
	}
	taskSpec, err := os.ReadFile(filepath.Join(workspace, snapshot.Descriptor.TaskSpecPath))
	if err != nil {
		t.Fatalf("read task spec: %v", err)
	}
	if !strings.Contains(string(taskSpec), "Map provider cache behavior") ||
		!strings.Contains(string(taskSpec), "req_1") ||
		!strings.Contains(string(taskSpec), "req_2") {
		t.Fatalf("task_spec.md does not contain objective and requirements:\n%s", string(taskSpec))
	}
	var progress research.AutoResearchProgress
	progressData, err := os.ReadFile(filepath.Join(workspace, snapshot.Descriptor.ProgressPath))
	if err != nil {
		t.Fatalf("read progress: %v", err)
	}
	if err := json.Unmarshal(progressData, &progress); err != nil {
		t.Fatalf("parse progress: %v", err)
	}
	if progress.ThreadID != "../../AGENTS.md" ||
		progress.Status != "active" ||
		len(progress.Requirements) != 2 {
		t.Fatalf("progress.json mismatch: %#v", progress)
	}
}

func TestAutoResearchRejectsUnsafeInputsAndUnknownRequirement(t *testing.T) {
	workspace := t.TempDir()
	store := research.NewAutoResearchProjectStore(func() string { return "2026-06-23T00:00:00.000Z" })
	if _, err := store.CreateOrResume("relative/workspace", "thr_research", "Research", nil); err == nil {
		t.Fatalf("relative workspaces must be rejected")
	}
	snapshot, err := store.CreateOrResume(workspace, "thr_research", "Research long task recovery", []string{"Find restart path"})
	if err != nil {
		t.Fatalf("create autoresearch state: %v", err)
	}
	if _, err := store.RecordEvidence(workspace, "thr_research", "req_missing", "Wrong requirement", []string{"must not write"}, ""); err == nil ||
		!strings.Contains(err.Error(), "unknown research requirement") {
		t.Fatalf("unknown requirement should be rejected, got %v", err)
	}
	findings, err := os.ReadFile(filepath.Join(workspace, snapshot.Descriptor.FindingsPath))
	if err != nil {
		t.Fatalf("read findings: %v", err)
	}
	if string(findings) != "" {
		t.Fatalf("unknown requirement must not write findings: %s", string(findings))
	}
	audit, err := store.AuditRequirements(workspace, "thr_research")
	if err != nil {
		t.Fatalf("audit requirements: %v", err)
	}
	if audit.Complete || len(audit.Requirements) != 1 || audit.Requirements[0].HasEvidence {
		t.Fatalf("audit should show missing evidence: %#v", audit)
	}
}

func TestAutoResearchDirectionEvidenceAuditAndResume(t *testing.T) {
	workspace := t.TempDir()
	store := research.NewAutoResearchProjectStore(func() string { return "2026-06-23T00:00:00.000Z" })
	snapshot, err := store.CreateOrResume(workspace, "thr_research", "Research cache behavior", []string{
		"Compare providers",
		"Audit fallback",
	})
	if err != nil {
		t.Fatalf("create autoresearch state: %v", err)
	}
	direction, err := store.RecordDirection(workspace, "thr_research", "Compare provider cache telemetry", "dead_end", "No fallback evidence yet.")
	if err != nil {
		t.Fatalf("record direction: %v", err)
	}
	if direction.Outcome != "dead_end" {
		t.Fatalf("direction outcome mismatch: %#v", direction)
	}
	event := research.AutoResearchStateAuditEvent(snapshot, 1, "thr_research", "turn_research")
	if event["kind"] != "autoresearch_state_audit" ||
		event["result"] != "pivot_required" ||
		event["pivotRequired"] != true ||
		event["stablePrefixContainsState"] != false ||
		event["toolSchemaContainsState"] != false ||
		event["topLevelAutoResearchRouteExposed"] != false {
		t.Fatalf("stale/pivot audit mismatch: %#v", event)
	}
	if _, err := store.RecordEvidence(workspace, "thr_research", "req_1", "Provider comparison", []string{"DeepSeek-only cache fields stayed scoped"}, ""); err != nil {
		t.Fatalf("record first evidence: %v", err)
	}
	audit, err := store.AuditRequirements(workspace, "thr_research")
	if err != nil {
		t.Fatalf("audit after first evidence: %v", err)
	}
	if audit.Complete || audit.Requirements[0].EvidenceCount != 1 || audit.Requirements[1].EvidenceCount != 0 {
		t.Fatalf("first evidence audit mismatch: %#v", audit)
	}
	if _, err := store.RecordEvidence(workspace, "thr_research", "req_2", "Fallback audit", []string{"OpenAI-compatible fallback remained available"}, ""); err != nil {
		t.Fatalf("record second evidence: %v", err)
	}
	audit, err = store.AuditRequirements(workspace, "thr_research")
	if err != nil {
		t.Fatalf("audit after second evidence: %v", err)
	}
	if !audit.Complete {
		t.Fatalf("all requirements should be complete: %#v", audit)
	}
	resumedStore := research.NewAutoResearchProjectStore(func() string { return "2026-06-23T00:00:01.000Z" })
	resumed, err := resumedStore.CreateOrResume(workspace, "thr_research", "Research cache behavior", nil)
	if err != nil {
		t.Fatalf("resume autoresearch state: %v", err)
	}
	if !resumed.Resumed || resumed.Progress.Status != "complete" {
		t.Fatalf("resume should preserve progress: %#v", resumed)
	}
	directions, err := os.ReadFile(filepath.Join(workspace, resumed.Descriptor.DirectionsTriedPath))
	if err != nil {
		t.Fatalf("read directions: %v", err)
	}
	if !strings.Contains(string(directions), "Compare provider cache telemetry") {
		t.Fatalf("directions_tried.json did not persist direction: %s", string(directions))
	}
	iterationLog, err := os.ReadFile(filepath.Join(workspace, resumed.Descriptor.IterationLogPath))
	if err != nil {
		t.Fatalf("read iteration log: %v", err)
	}
	if !strings.Contains(string(iterationLog), "direction_recorded") ||
		!strings.Contains(string(iterationLog), "evidence_recorded") {
		t.Fatalf("iteration_log.jsonl missing durable records: %s", string(iterationLog))
	}
}
