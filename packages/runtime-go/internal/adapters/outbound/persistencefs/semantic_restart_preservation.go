package persistencefs

import (
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	"context"
	"errors"
	"path/filepath"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

// SemanticRestartPreservationV1 is a denial-only check over the complete
// remaining authenticated program. It cannot replace, filter or apply its
// operations. The reader returns only verified install-file after bytes.
// The final string identifies at most the exact Applying cursor already
// proved physically After. Execution must recheck it and cannot write it.
type SemanticRestartPreservationV1 interface {
	ValidateSemanticOperationsV1(context.Context, []domainstartup.SemanticStartupOperationV1, func(domainstartup.SemanticStartupOperationV1) ([]byte, error), string) error
}

func NewSemanticPlanBuilderWithRestartPreservationV1(roots RootSet, authority *RootAuthority, journalAuthority *JournalNamespaceAuthority, preserved SemanticRestartPreservationV1,
	originals ...privatecasport.OriginalCreateResiduesV1,
) *SemanticPlanBuilder {
	builder := NewSemanticPlanBuilderWithAuthorities(roots, authority, journalAuthority)
	builder.restartPreserved = preserved
	if len(originals) > 1 {
		builder.authorityErr = errors.New("semantic original creation proof is ambiguous")
	} else if len(originals) == 1 {
		builder.originalCreates = originals[0]
	}
	return builder
}

func validateSemanticRestartOperationsV1(ctx context.Context, preserved SemanticRestartPreservationV1, plan domainstartup.SemanticStartupPlanV1, next int, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string) error {
	if preserved == nil {
		return nil
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	if domainstartup.ValidateSemanticStartupPlanV1(plan) != nil || next < 0 || next > len(plan.Operations) {
		return errors.New("semantic preserved operation program is invalid")
	}
	operations := append([]domainstartup.SemanticStartupOperationV1(nil), plan.Operations[next:]...)
	byID := make(map[string]domainstartup.SemanticStartupOperationV1, len(operations))
	for _, operation := range operations {
		byID[operation.OperationID] = operation
	}
	if noWriteOperationID != "" && (next == len(plan.Operations) || plan.Operations[next].OperationID != noWriteOperationID) {
		return errors.New("semantic no-write observation is outside the exact cursor")
	}
	return preserved.ValidateSemanticOperationsV1(ctx, operations, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
		if known, found := byID[operation.OperationID]; !found || known != operation || operation.Kind != domainstartup.SemanticOperationInstallFile || readAfter == nil {
			return nil, errors.New("semantic preserved after-image request is outside the authenticated program")
		}
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		body, err := readAfter(operation)
		if err != nil {
			return nil, err
		}
		if int64(len(body)) != operation.After.Size || domainsecurity.SHA256Hex(body) != operation.After.SHA256 {
			return nil, errors.New("semantic preserved after-image lost integrity")
		}
		return body, nil
	}, noWriteOperationID)
}

// Full journal, stage and current-state validation precede this check. The
// cursor selects the remaining program without removing a physically finished
// cursor operation. Its explicit no-write observation is rechecked on execution.
func validateSemanticJournalRestartPreservationV1(ctx context.Context, preserved SemanticRestartPreservationV1, journalRoot string, namespace *JournalNamespaceAuthority, roots RootSet, rootAuthority *RootAuthority, journal semanticJournalV1) (string, error) {
	if preserved == nil {
		return "", nil
	}
	noWriteOperationID := ""
	if journal.State == semanticJournalApplying && journal.NextOperation < len(journal.Plan.Operations) {
		operation := journal.Plan.Operations[journal.NextOperation]
		root, relative, err := managedRootRelative(roots, operation.Path)
		if err != nil {
			return "", err
		}
		after, err := secureManagedTargetMatches(rootAuthority, root, relative, operation.After)
		if err != nil {
			return "", err
		}
		if after {
			noWriteOperationID = operation.OperationID
		}
	}
	err := validateSemanticRestartOperationsV1(ctx, preserved, journal.Plan, journal.NextOperation, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
		directory, err := secureStartupOpenDirectory(namespace.root, filepath.Base(journalRoot), journal.JournalRootIdentity)
		if err != nil {
			return nil, err
		}
		defer directory.Close()
		stage, err := directory.OpenDirectory("stage", journal.JournalStageIdentity)
		if err != nil {
			return nil, err
		}
		defer stage.Close()
		body, _, err := stage.ReadFile(operation.OperationID, semanticFileSizeLimit(operation.After.Size), true)
		return body, err
	}, noWriteOperationID)
	return noWriteOperationID, err
}
