package loop

import (
	"context"
	"errors"
	"testing"
	"time"

	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
)

func TestRuntimeTerminalRecoveryKindIsStickyAndStepLimitDominates(t *testing.T) {
	if got := MergeRuntimeTerminalRecoveryKind("", ""); got != RuntimeTerminalRecoveryNone {
		t.Fatalf("zero-value recovery state was not normalized to none: %q", got)
	}
	if got := MergeRuntimeTerminalRecoveryKind(RuntimeTerminalRecoveryApplied, RuntimeTerminalRecoveryNone); got != RuntimeTerminalRecoveryApplied {
		t.Fatalf("recovery was cleared: %q", got)
	}
	if got := MergeRuntimeTerminalRecoveryKind(RuntimeTerminalRecoveryApplied, RuntimeTerminalRecoveryStepLimit); got != RuntimeTerminalRecoveryStepLimit {
		t.Fatalf("step-limit recovery did not dominate: %q", got)
	}
	if got := MergeRuntimeTerminalRecoveryKind(RuntimeTerminalRecoveryStepLimit, RuntimeTerminalRecoveryApplied); got != RuntimeTerminalRecoveryStepLimit {
		t.Fatalf("step-limit recovery was downgraded: %q", got)
	}
	result := RuntimeAgentLoopResult{TerminalRecoveryKind: RuntimeTerminalRecoveryApplied}
	sticky := RuntimeTerminalRecoveryStepLimit
	CommitRuntimeTerminalRecovery(&result, &sticky)
	if result.TerminalRecoveryKind != RuntimeTerminalRecoveryStepLimit {
		t.Fatalf("deferred terminal recovery commit lost the sticky state: %q", result.TerminalRecoveryKind)
	}
}

func TestRejectCancelledRuntimeResultDropsDraftAndPreservesProviderDiagnostics(t *testing.T) {
	for _, terminalErr := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(terminalErr.Error(), func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(context.Background())
			cancel(terminalErr)
			if errors.Is(terminalErr, context.DeadlineExceeded) {
				deadlineCtx, deadlineCancel := context.WithDeadline(context.Background(), time.Unix(1, 0))
				defer deadlineCancel()
				ctx = deadlineCtx
			}
			input := RuntimeAgentLoopResult{
				AssistantText: "UNPUBLISHED_ASSISTANT_DRAFT", Paused: true, PendingKind: "approval", PendingID: "pending-secret",
				TerminalRecoveryKind:  RuntimeTerminalRecoveryApplied,
				CandidateUsesCaseData: true,
				CandidateOrdinaryWork: true,
				CandidateInputClass:   RuntimeCandidateInputClassOrdinaryOnly,
				OrdinaryResult:        &domainordinaryresult.ResultSlotV1{},
				LastResult: domainmodel.Result{
					ProviderID: "deepseek", Chunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "draft"}, {Kind: domainmodel.ChunkReasoning, Text: "reasoning"}},
					Usage: domainmodel.Usage{PromptTokens: 10, TotalTokens: 10}, PrefixShape: domainmodel.PrefixShape{PrefixHash: "prefix"},
					CacheObservations: []domaincache.ProviderCallObservationV1{{}},
				},
			}
			output, err := RejectCancelledRuntimeResult(ctx, input, nil)
			if !errors.Is(err, terminalErr) || output.AssistantText != "" || len(output.LastResult.Chunks) != 0 || output.Paused || output.PendingKind != "" || output.PendingID != "" ||
				output.OrdinaryResult != nil || output.CandidateUsesCaseData || output.CandidateOrdinaryWork || output.CandidateInputClass != "" {
				t.Fatalf("cancelled runtime result retained draft state: output=%#v err=%v", output, err)
			}
			if output.LastResult.ProviderID != "deepseek" || output.LastResult.Usage.TotalTokens != 10 || output.LastResult.PrefixShape.PrefixHash != "prefix" ||
				len(output.LastResult.CacheObservations) != 1 || output.TerminalRecoveryKind != RuntimeTerminalRecoveryApplied {
				t.Fatalf("cancelled runtime result discarded diagnostics: %#v", output)
			}
		})
	}
}

func TestSealRuntimeAgentLoopResultForReturnDropsPrivateChunksOnEveryTerminal(t *testing.T) {
	for _, recovery := range []RuntimeTerminalRecoveryKind{
		RuntimeTerminalRecoveryNone,
		RuntimeTerminalRecoveryApplied,
		RuntimeTerminalRecoveryStepLimit,
	} {
		result := RuntimeAgentLoopResult{
			AssistantText: "public candidate",
			LastResult: domainmodel.Result{
				ProviderID: "deepseek",
				Chunks: []domainmodel.Chunk{
					{Kind: domainmodel.ChunkReasoning, Text: "PRIVATE_REASONING_SENTINEL"},
					{Kind: domainmodel.ChunkText, Text: "private attempt draft"},
				},
				Usage: domainmodel.Usage{PromptTokens: 7, TotalTokens: 11},
			},
		}
		sticky := recovery
		SealRuntimeAgentLoopResultForReturn(&result, &sticky)
		if len(result.LastResult.Chunks) != 0 || result.LastResult.ProviderID != "deepseek" ||
			result.LastResult.Usage.TotalTokens != 11 || result.TerminalRecoveryKind != recovery {
			t.Fatalf("runtime result return boundary retained private chunks or lost diagnostics: %#v", result)
		}
	}
}

func TestRuntimeResultCarriesOrdinaryCandidateRequiresProcessLocalProvenance(t *testing.T) {
	slot, err := domainordinaryresult.NewResultSlotV1("Updated the ordinary source file and ran its focused test.")
	if err != nil {
		t.Fatal(err)
	}
	valid := RuntimeAgentLoopResult{
		CandidateOrdinaryWork: true,
		CandidateInputClass:   RuntimeCandidateInputClassOrdinaryOnly,
		OrdinaryResult:        &slot,
	}
	if !RuntimeResultCarriesOrdinaryCandidate(valid) {
		t.Fatal("valid typed ordinary candidate was rejected")
	}
	withoutWork := valid
	withoutWork.CandidateOrdinaryWork = false
	if RuntimeResultCarriesOrdinaryCandidate(withoutWork) {
		t.Fatal("slot asserted ordinary work without process-local provenance")
	}
	withoutInputClass := valid
	withoutInputClass.CandidateInputClass = ""
	if RuntimeResultCarriesOrdinaryCandidate(withoutInputClass) {
		t.Fatal("slot asserted ordinary-only provider input without process-local provenance")
	}
	caseCandidate := valid
	caseCandidate.CandidateUsesCaseData = true
	if RuntimeResultCarriesOrdinaryCandidate(caseCandidate) {
		t.Fatal("case-data candidate was relabeled as an ordinary result")
	}
	tampered := valid
	tamperedSlot := slot
	tamperedSlot.Text = "tampered"
	tampered.OrdinaryResult = &tamperedSlot
	if RuntimeResultCarriesOrdinaryCandidate(tampered) {
		t.Fatal("tampered ordinary result slot was accepted")
	}
}
