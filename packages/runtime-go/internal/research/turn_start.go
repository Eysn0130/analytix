package research

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type TurnEventRecorder interface {
	RecordEvent(map[string]any) (map[string]any, []string, error)
}

func WorkspacePreparationBeforeFreezeV1(prompt string) func(context.Context, string) error {
	if !IsResearchGoalPrompt(prompt) {
		return nil
	}
	return func(ctx context.Context, workspaceRealPath string) error {
		return PrepareWorkspaceBeforeFreeze(ctx, prompt, workspaceRealPath)
	}
}

// PrepareWorkspaceBeforeFreeze materializes only the canonical workspace root
// required by an AutoResearch turn. It deliberately does not create research
// state or case authority material. The caller must hold the turn security
// transition writer and must pass the exact canonical transition identity.
func PrepareWorkspaceBeforeFreeze(ctx context.Context, prompt, workspaceRealPath string) error {
	if !IsResearchGoalPrompt(prompt) {
		return nil
	}
	if ctx == nil {
		return errors.New("autoresearch workspace preparation context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	workspaceRealPath = strings.TrimSpace(workspaceRealPath)
	if workspaceRealPath == "" || !filepath.IsAbs(workspaceRealPath) {
		return errors.New("autoresearch state requires an absolute thread workspace")
	}
	workspaceRealPath = filepath.Clean(workspaceRealPath)
	if err := os.MkdirAll(workspaceRealPath, 0o700); err != nil {
		return fmt.Errorf("prepare autoresearch workspace: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(workspaceRealPath)
	if err != nil || filepath.Clean(resolved) != workspaceRealPath {
		return errors.New("autoresearch workspace identity changed during preparation")
	}
	info, err := os.Stat(workspaceRealPath)
	if err != nil || !info.IsDir() {
		return errors.New("autoresearch workspace is not a directory")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func PrepareTurnState(ctx context.Context, store AutoResearchProjectStore, events TurnEventRecorder, prompt, workspace, threadID, turnID string) error {
	if !IsResearchGoalPrompt(prompt) {
		return nil
	}
	if ctx == nil {
		return errors.New("autoresearch state context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" || events == nil {
		return errors.New("autoresearch state requires an absolute thread workspace")
	}
	objective := ResearchObjectiveFromPrompt(prompt)
	snapshot, err := store.CreateOrResume(workspace, threadID, objective, []string{objective})
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := store.RecordDirection(
		workspace, threadID, "Initial /goal --research runtime turn audit", "tried",
		"Go runtime recorded project-local AutoResearch state before model output.",
	); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, _, err = events.RecordEvent(AutoResearchStateAuditEvent(snapshot, 1, threadID, turnID))
	return err
}
