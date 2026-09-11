package httpapi

import (
	"errors"

	threadapp "analytix.local/runtime-go/internal/app/thread"
)

func isThreadMutationConflict(err error) bool {
	return errors.Is(err, threadapp.ErrThreadMutationBaselineConflict) || errors.Is(err, threadapp.ErrThreadMutationTransition) ||
		errors.Is(err, threadapp.ErrCaseWorkspaceSignedRebind) || errors.Is(err, threadapp.ErrCaseRewindSignedArchive) ||
		errors.Is(err, threadapp.ErrCaseDeleteSignedTombstone)
}

func threadMutationError(err error) map[string]any {
	code := "thread_mutation_conflict"
	switch {
	case errors.Is(err, threadapp.ErrCaseWorkspaceSignedRebind):
		code = "case_workspace_rebind_authority_required"
	case errors.Is(err, threadapp.ErrCaseRewindSignedArchive):
		code = "case_rewind_archive_required"
	case errors.Is(err, threadapp.ErrCaseDeleteSignedTombstone):
		code = "case_delete_tombstone_required"
	case errors.Is(err, threadapp.ErrThreadMutationTransition):
		code = "thread_mutation_transition_unavailable"
	}
	return map[string]any{"code": code, "message": err.Error()}
}
