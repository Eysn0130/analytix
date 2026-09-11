package thread

import (
	"context"
	"errors"
	"strings"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
)

type AutoCompactionInputV1 struct {
	ThreadID            string
	Prompt              string
	ContextWindowTokens int
	MainThread          bool
}

type AutoCompactionResultV1 struct {
	Compacted    bool
	BeforeTokens int
	AfterTokens  int
	Thresholds   appmodel.ContextThresholdsV1
}

func (s *Service) AutoCompactBeforeTurnV1(ctx context.Context, input AutoCompactionInputV1) (AutoCompactionResultV1, error) {
	result := AutoCompactionResultV1{Thresholds: appmodel.ResolveContextThresholdsV1(input.ContextWindowTokens)}
	if err := s.requireRestartWritableV1(input.ThreadID); err != nil {
		return result, err
	}
	if !input.MainThread {
		return result, nil
	}
	if ctx == nil || s == nil || s.repository == nil {
		return result, errors.New("automatic compaction authority is unavailable")
	}
	threadID := strings.TrimSpace(input.ThreadID)
	thread, err := s.repository.GetThread(threadID)
	if err != nil {
		return result, err
	}
	if thread == nil {
		return result, ErrThreadNotFound
	}
	result.BeforeTokens = s.automaticCompactionPreflightTokensV1(
		thread, input.Prompt, result.Thresholds.ContextWindowTokens,
	)
	result.AfterTokens = result.BeforeTokens
	if result.BeforeTokens < result.Thresholds.SoftThresholdTokens {
		return result, nil
	}
	response, err := s.compact(ctx, threadID, "automatic_context_threshold", true)
	if err != nil {
		// A protected case archive is an additive capability, not a global
		// turn-start prerequisite. The raw preflight runs before the host has
		// partitioned protected and ordinary lanes, so it may not turn a large
		// case history into a universal Agent outage. The later per-effect final
		// request guard enforces the exact hard limit before either lane reaches a
		// provider; the case compiler independently withholds unavailable case
		// context.
		if errors.Is(err, ErrCaseCompactionRequiresTrustedArchive) {
			return result, nil
		}
		return result, err
	}
	result.Compacted = compactionResponseReplacedTokensV1(response) > 0
	thread, err = s.repository.GetThread(threadID)
	if err != nil || thread == nil {
		return result, errors.Join(errors.New("automatic compaction readback is unavailable"), err)
	}
	result.AfterTokens = s.automaticCompactionPreflightTokensV1(
		thread, input.Prompt, result.Thresholds.ContextWindowTokens,
	)
	// This observation is intentionally non-terminal: compaction precedes
	// effect classification, while the final provider guard measures the exact
	// privacy-projected request and guarantees zero transport at the hard limit.
	return result, nil
}

func (s *Service) automaticCompactionPreflightTokensV1(
	thread map[string]any,
	prompt string,
	contextWindowTokens int,
) int {
	estimate := appmodel.EstimateThreadPreflightTokensV1(thread, prompt, contextWindowTokens)
	turns, turnsOK := thread["turns"].([]any)
	if turnsOK && len(turns) == 0 {
		return estimate
	}
	caseSlots := map[string]domainordinaryresult.ResultSlotV1{}
	if resolver, ok := s.publicProjector.(interface {
		CommittedCaseOrdinaryResultsV1(map[string]any) (map[string]domainordinaryresult.ResultSlotV1, error)
	}); ok {
		resolved, err := resolver.CommittedCaseOrdinaryResultsV1(thread)
		if err == nil {
			caseSlots = resolved
		}
		// A failed case-slot projection withholds only those slots. General
		// terminal authority and a signed compaction continuation are verified
		// independently below and must remain aligned with the provider path.
	}
	trustedCaseCompactions := TrustedCaseCompactionTurnIDsV1(thread, s.caseThreads, s.repository)
	typedHistory, err := appmodel.TypedOrdinaryProviderHistoryWithCaseCompactionsV1(
		thread, caseSlots, trustedCaseCompactions,
	)
	if err != nil {
		return estimate
	}
	typedEstimate := appmodel.EstimateMessagesPreflightTokensV1(
		typedHistory, prompt, contextWindowTokens,
	)
	if typedEstimate > estimate {
		return typedEstimate
	}
	return estimate
}

func compactionResponseReplacedTokensV1(response map[string]any) int {
	switch value := response["replacedTokens"].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}
