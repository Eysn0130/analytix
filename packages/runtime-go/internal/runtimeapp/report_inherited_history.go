package runtimeapp

import (
	"context"
	"errors"
	"reflect"

	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	pendingapp "analytix.local/runtime-go/internal/app/pendingwork"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type runtimeInheritedFinalReaderV1 struct {
	threadapp.AuthorityThreadReader
	ctx   context.Context
	scope *pendingapp.ReportRestartScopeV1
}

func runtimeRestartFinalPublicReaderV1(ctx context.Context, reader threadapp.AuthorityThreadReader, scope *pendingapp.ReportRestartScopeV1) threadapp.AuthorityThreadReader {
	if scope == nil {
		return reader
	}
	return runtimeInheritedFinalReaderV1{AuthorityThreadReader: reader, ctx: ctx, scope: scope}
}

func (reader runtimeInheritedFinalReaderV1) CommittedContext(threadID, turnID string) (casethreadapp.CommittedContext, bool) {
	current, ok := reader.AuthorityThreadReader.(interface {
		CommittedContext(string, string) (casethreadapp.CommittedContext, bool)
	})
	if !ok {
		return casethreadapp.CommittedContext{}, false
	}
	return current.CommittedContext(threadID, turnID)
}

// Keep every thread and final in the inventory. Only an exact inherited
// compaction turn in an already sealed hold is observed as inert history;
// current or unheld compaction still requires the signed execution proof.
func (reader runtimeInheritedFinalReaderV1) ValidateCaseCompactionAuthorityTurnV1(threadID string, thread, turn map[string]any) error {
	if _, present := turn["securityContext"]; !present && reader.scope != nil && reader.scope.OwnsThread(threadID) {
		if thread["id"] != threadID || turn["threadId"] != threadID || turn["caseHistoryProjection"] != "compaction_authority_v1" {
			return errors.New("inherited compaction observation binding is invalid")
		}
		return reader.scope.ValidateInheritedTurnV1(reader.ctx, thread, turn)
	}
	current, ok := reader.AuthorityThreadReader.(interface {
		ValidateCaseCompactionAuthorityTurnV1(string, map[string]any, map[string]any) error
	})
	if !ok {
		return errors.New("current compaction observation is unavailable")
	}
	return current.ValidateCaseCompactionAuthorityTurnV1(threadID, thread, turn)
}

// This proof is consumed only while constructing a startup hold. Subsequent
// live checks use the held primary digest and the existing signed-record hold;
// they do not freeze unrelated future case records against this startup scan.
type runtimeReportInheritedHistoryV1 struct {
	core        *runtimeChildIdentityStartupV1
	observation *runtimeCaseThreadSemanticObservationV1
}

func (proof *runtimeReportInheritedHistoryV1) Revalidate(ctx context.Context) error {
	if proof == nil || proof.core == nil || proof.core.revalidateKey == nil || proof.observation == nil {
		return errors.New("original derived history observation is unavailable")
	}
	return errors.Join(proof.observation.Revalidate(ctx), proof.core.revalidateKey(ctx), ctx.Err())
}

func (proof *runtimeReportInheritedHistoryV1) ValidateInheritedTurnV1(ctx context.Context, thread, turn map[string]any) (resultErr error) {
	if ctx == nil || proof == nil || proof.core == nil {
		return errors.New("original derived history observer is unavailable")
	}
	threadID, _ := thread["id"].(string)
	turnID, _ := turn["id"].(string)
	if err := threadapp.ValidateCaseDerivedHistoryTurnV1(threadID, turn); err != nil {
		return err
	}
	if proof.observation == nil {
		observed, err := observeRuntimeCaseThreadSemanticInventoryV1(ctx, proof.core)
		if err != nil {
			return err
		}
		proof.observation = observed
	}
	defer func() { resultErr = errors.Join(resultErr, proof.Revalidate(ctx)) }()
	var lineage domainsecurity.CaseThreadAuthorityRecord
	for _, record := range proof.observation.original {
		if domainsecurity.CaseThreadAuthorityIsLineage(record) && record.ThreadID == threadID {
			if lineage.RecordDigest != "" {
				return errors.New("original derived history lineage is ambiguous")
			}
			lineage = record
		}
	}
	if lineage.RecordDigest == "" || thread["forkedFromThreadId"] != lineage.ParentThreadID ||
		(lineage.Derivation == "fork" && (thread["relation"] != "fork" || thread["parentThreadId"] != lineage.ParentThreadID)) ||
		(lineage.Derivation == "resume" && thread["relation"] != "primary") {
		return errors.New("original derived history lacks exact signed lineage")
	}
	// Verify every staged and committed context, including legacy contexts.
	// A prospective Final record cannot fill an Original hole or erase it.
	for _, inventory := range []runtimeCaseThreadSemanticInventoryV1{proof.observation.original, proof.observation.final} {
		for _, record := range inventory {
			if record.SecurityContext != nil && record.SecurityContext.ThreadID == threadID && record.SecurityContext.TurnID == turnID {
				return errors.New("original derived history is an executed target turn")
			}
			if domainsecurity.CaseThreadAuthorityThreadID(record) == threadID || record.ParentThreadID == threadID {
				original, exists := proof.observation.original[record.RecordDigest]
				final, remains := proof.observation.final[record.RecordDigest]
				if !exists || !remains || !reflect.DeepEqual(original, final) {
					return errors.New("semantic candidate changed derived history authority")
				}
			}
		}
	}
	inventory := proof.core.pendingInventory
	if proof.core.pendingSemantic != nil {
		inventory = proof.core.pendingSemantic.allocation
	}
	for _, receipt := range inventory.Receipts {
		if receipt.Context.ThreadID == threadID && receipt.Context.TurnID == turnID {
			return errors.New("derived history has a pending execution identity")
		}
		if receipt.ChildProducer != nil {
			for _, target := range receipt.ChildProducer.Children {
				if target.ChildThreadID == threadID && target.ChildTurnID == turnID {
					return errors.New("derived history has a reserved child execution identity")
				}
			}
		}
	}
	for _, record := range proof.core.jobRecords {
		if record.ParentThreadID == threadID && (record.ParentTurnID == turnID || record.AutoContinueTurnID == turnID) || record.ChildThreadID == threadID && record.ChildTurnID == turnID {
			return errors.New("derived history has a child job execution identity")
		}
	}
	return nil
}
