package subagent

import (
	"context"
	"errors"
	"strings"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

type ChildCompletionContextResolver func(string, string) (domainsecurity.TurnSecurityContext, bool)

type ChildCompletionFinalAuthority interface {
	ResolveCommitted(string, string) (domainevidence.PrivateAcceptedFinalRecord, domainevidence.AcceptedFinalDispositionRecord, bool)
}

type ChildCompletionCurrentValidator func(context.Context, domainsecurity.TurnSecurityContext) error

type ChildCompletionAuthority struct {
	contexts        ChildCompletionContextResolver
	finals          ChildCompletionFinalAuthority
	authority       finalauthorityport.Authority
	threads         JobSecurityThreadReader
	validateCurrent ChildCompletionCurrentValidator
	now             func() time.Time
}

type VerifiedChildCompletion struct {
	receipt domainjob.ChildCompletionReceiptV1
	trusted bool
}

type storedChildCompletionVerifierV1 struct{ authority *ChildCompletionAuthority }

// NewStoredChildCompletionVerifierV1 exposes only historical validation of an
// existing receipt. Contexts must come from a complete verified committed
// inventory; finals must carry the same terminal-complete admission as the
// live trusted projection index. Both resolvers are immutable observations,
// not raw primary fields or private final pairs. The caller revalidates their
// physical snapshots around the complete operation.
func NewStoredChildCompletionVerifierV1(contexts ChildCompletionContextResolver, finals ChildCompletionFinalAuthority, authority finalauthorityport.Authority, threads JobSecurityThreadReader) *storedChildCompletionVerifierV1 {
	return &storedChildCompletionVerifierV1{authority: &ChildCompletionAuthority{
		contexts: contexts, finals: finals, authority: authority, threads: threads,
	}}
}

func (verifier *storedChildCompletionVerifierV1) VerifyStoredChildCompletion(ctx context.Context, record domainjob.Record) error {
	if verifier == nil || verifier.authority == nil {
		return errors.New("stored child completion verifier is unavailable")
	}
	return verifier.authority.VerifyStoredChildCompletion(ctx, record)
}

func NewChildCompletionAuthority(
	contexts ChildCompletionContextResolver,
	finals ChildCompletionFinalAuthority,
	authority finalauthorityport.Authority,
	threads JobSecurityThreadReader,
	validateCurrent ChildCompletionCurrentValidator,
) *ChildCompletionAuthority {
	return &ChildCompletionAuthority{
		contexts: contexts, finals: finals, authority: authority, threads: threads, validateCurrent: validateCurrent,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (authority *ChildCompletionAuthority) Issue(ctx context.Context, record domainjob.Record, childTurnID string) (VerifiedChildCompletion, error) {
	childTurnID = strings.TrimSpace(childTurnID)
	if authority == nil || authority.contexts == nil || authority.finals == nil || authority.authority == nil ||
		authority.threads == nil || authority.validateCurrent == nil || ctx == nil || childTurnID == "" || record.SecurityBinding == nil ||
		domainjob.ValidateSecurityBinding(record.SecurityBinding) != nil || record.ID == "" ||
		strings.TrimSpace(record.ChildThreadID) == "" || record.SecurityBinding.ParentCaseID == domainsecurity.UnboundCaseID {
		return VerifiedChildCompletion{}, errors.New("child completion authority is unavailable")
	}
	issuedAt := time.Now().UTC()
	if authority.now != nil {
		issuedAt = authority.now().UTC()
	}
	if blocker := (JobSecurityAuthorizer{Threads: authority.threads}).BlockerAt(record, issuedAt); blocker != "" {
		return VerifiedChildCompletion{}, errors.New("parent execution grant is not current for child completion: " + blocker)
	}
	parent, parentOK := authority.contexts(record.ParentThreadID, record.ParentTurnID)
	child, childOK := authority.contexts(record.ChildThreadID, childTurnID)
	if !parentOK || !childOK || !domainjob.SecurityBindingMatchesContext(record.SecurityBinding, parent) ||
		domainsecurity.ValidateTurnSecurityContextForExecution(parent) != nil ||
		domainsecurity.ValidateTurnSecurityContextForCasePublication(child) != nil {
		return VerifiedChildCompletion{}, errors.New("child completion contexts are not committed")
	}
	if err := authority.validateCurrent(ctx, parent); err != nil {
		return VerifiedChildCompletion{}, errors.Join(errors.New("parent child-completion context is stale"), err)
	}
	if err := authority.validateCurrent(ctx, child); err != nil {
		return VerifiedChildCompletion{}, errors.Join(errors.New("child completion context is stale"), err)
	}
	privateFinal, _, ok := authority.finals.ResolveCommitted(record.ChildThreadID, childTurnID)
	if !ok || privateFinal.SecurityContext != child {
		return VerifiedChildCompletion{}, errors.New("child accepted final is not committed")
	}
	canContinue := childCompletionVariantCanContinue(privateFinal.AcceptedFinal.Variant)
	receipt, err := domainjob.NewChildCompletionReceiptV1(domainjob.ChildCompletionReceiptInputV1{
		ChildRunID: record.ID, SecurityBinding: record.SecurityBinding,
		ParentContext: parent, ChildContext: child,
		AcceptedFinal: privateFinal.AcceptedFinal, CanContinueParent: canContinue, IssuedAt: issuedAt,
		AuthorityKeyID: authority.authority.KeyID(), AuthorityPublicKey: authority.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.authority.Sign(ctx, message) })
	if err != nil {
		return VerifiedChildCompletion{}, err
	}
	candidate := record
	candidate.Status = string(domainjob.StatusCompleted)
	candidate.ChildTurnID = childTurnID
	candidate.ChildCompletionReceipt = domainjob.CloneChildCompletionReceiptV1(&receipt)
	verified, err := authority.verify(ctx, candidate, true, issuedAt)
	if err != nil {
		return VerifiedChildCompletion{}, err
	}
	return verified, nil
}

// VerifyStoredChildCompletion implements jobs.ChildCompletionReceiptVerifier.
// It proves the receipt was issued by the configured host authority and binds
// an exact committed final. Historical verification evaluates grant validity
// at receipt issuance; it does not make the record current again.
func (authority *ChildCompletionAuthority) VerifyStoredChildCompletion(ctx context.Context, record domainjob.Record) error {
	if ctx == nil {
		return errors.New("stored child completion context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if record.ChildCompletionReceipt == nil {
		return errors.New("child completion receipt is missing")
	}
	issuedAt, err := time.Parse(time.RFC3339Nano, record.ChildCompletionReceipt.IssuedAt)
	if err != nil {
		return errors.New("child completion receipt issuedAt is invalid")
	}
	_, err = authority.verify(ctx, record, false, issuedAt)
	return err
}

// Rehydrate re-establishes an in-process capability from durable audit data.
// It is the only supported consumption path after persistence or restart.
func (authority *ChildCompletionAuthority) Rehydrate(ctx context.Context, record domainjob.Record) (VerifiedChildCompletion, error) {
	checkAt := time.Now().UTC()
	if authority != nil && authority.now != nil {
		checkAt = authority.now().UTC()
	}
	return authority.verify(ctx, record, true, checkAt)
}

func (authority *ChildCompletionAuthority) verify(ctx context.Context, record domainjob.Record, requireCurrent bool, grantCheckAt time.Time) (VerifiedChildCompletion, error) {
	if authority == nil || authority.contexts == nil || authority.finals == nil || authority.authority == nil || authority.threads == nil ||
		(requireCurrent && authority.validateCurrent == nil) || record.ChildCompletionReceipt == nil || record.SecurityBinding == nil || ctx == nil ||
		strings.TrimSpace(record.Kind) != "subagent" || strings.TrimSpace(record.Status) != string(domainjob.StatusCompleted) ||
		record.Output != "" || record.SecurityBinding.ParentCaseID == domainsecurity.UnboundCaseID {
		return VerifiedChildCompletion{}, errors.New("child completion verification authority is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return VerifiedChildCompletion{}, err
	}
	receipt := *record.ChildCompletionReceipt
	if domainjob.ValidateChildCompletionReceiptV1(receipt) != nil || receipt.ChildRunID != strings.TrimSpace(record.ID) ||
		receipt.ParentContext.ThreadID != strings.TrimSpace(record.ParentThreadID) ||
		receipt.ParentContext.TurnID != strings.TrimSpace(record.ParentTurnID) ||
		receipt.ChildContext.ThreadID != strings.TrimSpace(record.ChildThreadID) ||
		receipt.ChildContext.TurnID != strings.TrimSpace(record.ChildTurnID) {
		return VerifiedChildCompletion{}, errors.New("child completion receipt does not match the durable run")
	}
	parent, parentOK := authority.contexts(receipt.ParentContext.ThreadID, receipt.ParentContext.TurnID)
	child, childOK := authority.contexts(receipt.ChildContext.ThreadID, receipt.ChildContext.TurnID)
	if !parentOK || !childOK || !domainjob.SecurityBindingMatchesContext(record.SecurityBinding, parent) ||
		!domainjob.ChildCompletionContextRefV1MatchesContext(receipt.ParentContext, parent) ||
		!domainjob.ChildCompletionContextRefV1MatchesContext(receipt.ChildContext, child) {
		return VerifiedChildCompletion{}, errors.New("child completion contexts are not committed")
	}
	if requireCurrent {
		if blocker := (JobSecurityAuthorizer{Threads: authority.threads}).BlockerAt(record, grantCheckAt); blocker != "" {
			return VerifiedChildCompletion{}, errors.New("parent execution grant is not current for child completion: " + blocker)
		}
		if err := authority.validateCurrent(ctx, parent); err != nil {
			return VerifiedChildCompletion{}, errors.Join(errors.New("parent child-completion context is stale"), err)
		}
		if err := authority.validateCurrent(ctx, child); err != nil {
			return VerifiedChildCompletion{}, errors.Join(errors.New("child completion context is stale"), err)
		}
	} else {
		blocker, err := authority.historicalGrantBlocker(record, grantCheckAt)
		if err != nil {
			return VerifiedChildCompletion{}, err
		}
		if blocker != "" {
			return VerifiedChildCompletion{}, errors.New("parent execution grant did not authorize child completion: " + blocker)
		}
	}
	privateFinal, disposition, ok := authority.finals.ResolveCommitted(receipt.ChildContext.ThreadID, receipt.ChildContext.TurnID)
	if !ok || privateFinal.SecurityContext != child || domainevidence.ValidatePrivateAcceptedFinalRecord(privateFinal) != nil ||
		domainevidence.ValidateAcceptedFinalDispositionRecord(disposition) != nil ||
		disposition.State != domainevidence.AcceptedFinalCommitted ||
		disposition.AcceptedFinalDigest != privateFinal.AcceptedFinal.RecordDigest ||
		disposition.PrivateRecordDigest != privateFinal.PrivateRecordDigest ||
		disposition.ThreadID != child.ThreadID || disposition.TurnID != child.TurnID ||
		domainjob.ValidateChildCompletionReceiptForBindingsV1(receipt, parent, child, record.SecurityBinding, privateFinal.AcceptedFinal) != nil {
		return VerifiedChildCompletion{}, errors.New("child accepted final is not trusted")
	}
	receiptIssuedAt, receiptTimeErr := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
	dispositionDecidedAt, dispositionTimeErr := time.Parse(time.RFC3339Nano, disposition.DecidedAt)
	if receiptTimeErr != nil || dispositionTimeErr != nil || receiptIssuedAt.Before(dispositionDecidedAt) {
		return VerifiedChildCompletion{}, errors.New("child completion receipt predates committed final disposition")
	}
	if err := authority.verifyAcceptedFinalAuthority(ctx, privateFinal, disposition); err != nil {
		return VerifiedChildCompletion{}, err
	}
	keyID, publicKey, signature, err := domainjob.ChildCompletionReceiptV1AuthorityMaterial(receipt)
	if err != nil {
		return VerifiedChildCompletion{}, err
	}
	if err := authority.authority.VerifyTrusted(ctx, keyID, publicKey, domainjob.ChildCompletionReceiptV1SigningBytes(receipt), signature); err != nil {
		return VerifiedChildCompletion{}, errors.Join(errors.New("child completion receipt is not host-trusted"), err)
	}
	if err := ctx.Err(); err != nil {
		return VerifiedChildCompletion{}, err
	}
	return VerifiedChildCompletion{receipt: receipt, trusted: true}, nil
}

func (authority *ChildCompletionAuthority) historicalGrantBlocker(record domainjob.Record, at time.Time) (string, error) {
	thread, err := authority.threads.GetThread(record.ParentThreadID)
	if err != nil {
		return "", errors.Join(errors.New("original child parent observation failed"), err)
	}
	if thread == nil {
		return "parent_thread_missing", nil
	}
	if !securityThreadContainsTurn(thread, record.ParentTurnID) {
		return "parent_turn_missing", nil
	}
	return durableParentGrantBlocker(record, thread, at), nil
}

func (authority *ChildCompletionAuthority) verifyAcceptedFinalAuthority(ctx context.Context, privateFinal domainevidence.PrivateAcceptedFinalRecord, disposition domainevidence.AcceptedFinalDispositionRecord) error {
	keyID, publicKey, signature, err := domainevidence.AcceptedFinalAuthorityMaterial(privateFinal.AcceptedFinal)
	if err != nil {
		return err
	}
	if err := authority.authority.VerifyTrusted(ctx, keyID, publicKey, domainevidence.AcceptedFinalSigningBytes(privateFinal.AcceptedFinal), signature); err != nil {
		return errors.Join(errors.New("child accepted final authority is not trusted"), err)
	}
	keyID, publicKey, signature, err = domainevidence.AcceptedFinalDispositionAuthorityMaterial(disposition)
	if err != nil {
		return err
	}
	if err := authority.authority.VerifyTrusted(ctx, keyID, publicKey, domainevidence.AcceptedFinalDispositionSigningBytes(disposition), signature); err != nil {
		return errors.Join(errors.New("child accepted final disposition is not trusted"), err)
	}
	return nil
}

func (completion VerifiedChildCompletion) ReceiptForPersistence() (*domainjob.ChildCompletionReceiptV1, error) {
	if !completion.trusted || domainjob.ValidateChildCompletionReceiptV1(completion.receipt) != nil {
		return nil, errors.New("verified child completion is unavailable")
	}
	return domainjob.CloneChildCompletionReceiptV1(&completion.receipt), nil
}

func (completion VerifiedChildCompletion) OutputProjection(record domainjob.Record) map[string]any {
	projection := ChildOutputMetadataProjection(record)
	if !completion.trusted || domainjob.ValidateChildCompletionReceiptV1(completion.receipt) != nil || completion.receipt.ChildRunID != strings.TrimSpace(record.ID) ||
		record.ChildCompletionReceipt == nil || *record.ChildCompletionReceipt != completion.receipt {
		return projection
	}
	projection["childCompletionReceiptDigest"] = completion.receipt.ReceiptDigest
	projection["canContinueParent"] = completion.receipt.CanContinueParent
	return projection
}

// VerifyParentContinuation implements loop.ParentContinuationCapability
// without importing the loop package. Only a rehydrated, host-trusted receipt
// can return a positive decision; public output maps cannot implement this
// in-process authority accidentally.
func (completion VerifiedChildCompletion) VerifyParentContinuation(parent domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant, toolCallID string) (string, bool) {
	if !completion.trusted || !completion.receipt.CanContinueParent ||
		domainjob.ValidateChildCompletionReceiptV1(completion.receipt) != nil ||
		!domainjob.ChildCompletionContextRefV1MatchesContext(completion.receipt.ParentContext, parent) ||
		domainsecurity.ValidateExecutionGrantForContext(grant, parent) != nil ||
		completion.receipt.ParentExecutionGrantID != grant.GrantID || grant.ToolCallID != strings.TrimSpace(toolCallID) ||
		completion.receipt.ParentToolCallID != strings.TrimSpace(toolCallID) {
		return "", false
	}
	return completion.receipt.ReceiptDigest, true
}

func childCompletionVariantCanContinue(variant domainevidence.FinalAnswerVariant) bool {
	switch variant {
	case domainevidence.SourceUnavailableAnswer, domainevidence.NeedsEvidenceAnswer, domainevidence.GeneralGuidanceAnswer:
		return true
	default:
		return false
	}
}

func caseChildCompletionRequired(record domainjob.Record) bool {
	return record.SecurityBinding != nil && record.SecurityBinding.ParentCaseID != domainsecurity.UnboundCaseID
}
