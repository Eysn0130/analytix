package loop

import (
	"strings"

	appmodel "analytix.local/runtime-go/internal/app/model"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type InitialProviderMessagesInputV1 struct {
	SystemPrompt string
	ProviderID   string
	Model        string
	Policy       CaseFundAnalysisPolicy
	Boundary     string
	History      []domainmodel.Message
	// OrdinaryHistory contains only host-reconstructed typed ordinary finals.
	// It is independent from the case-capable execution transcript in History.
	OrdinaryHistory []domainmodel.Message
	// CaseTaskContinuationHistory contains only the original sealed task
	// continuation from the latest trusted case compaction cut.
	CaseTaskContinuationHistory []domainmodel.Message
	Background                  []domainmodel.Message
	UserPrompt                  string
	AttachmentPlanDigest        string
	ContextEpoch                domaincontextepoch.ProviderContext
	CaseSensitive               bool
}

type InitialProviderMessagesV1 struct {
	// BaseSystemPrompt is the immutable, effect-neutral prompt including the
	// host-selected stable context prefix. Per-step case instructions are always
	// compiled from this value so a prior effect cannot survive a transition.
	BaseSystemPrompt string
	SystemPrompt     string
	Messages         []domainmodel.Message
	// OrdinaryLaneMessages is the safe baseline for a later exact ordinary
	// steering transition. It never contains the raw case transcript, current
	// mixed prompt, case background, or protected tool material.
	OrdinaryLaneMessages []domainmodel.Message
	Boundary             string
	// OrdinaryResultInputIsolated is process-local provenance for the exact
	// initial provider input. It is false for an unpartitioned mixed/case
	// prompt even when the protected capability later downgrades to ordinary.
	OrdinaryResultInputIsolated bool
}

func PrepareInitialProviderMessagesV1(input InitialProviderMessagesInputV1) InitialProviderMessagesV1 {
	baseSystemPrompt := strings.TrimSpace(input.SystemPrompt)
	if baseSystemPrompt == "" {
		baseSystemPrompt = DefaultRuntimeSystemPrompt(input.ProviderID, input.Model)
	}
	if input.Boundary != "" {
		return InitialProviderMessagesV1{BaseSystemPrompt: baseSystemPrompt, Boundary: input.Boundary}
	}
	history := appmodel.CloneProviderMessages(input.History)
	ordinaryHistory := appmodel.CloneProviderMessages(input.OrdinaryHistory)
	background := appmodel.CloneProviderMessages(input.Background)
	if input.CaseSensitive && input.Policy.ProviderUsesCaseDataAuthority() {
		// The typed history owner emits at most one continuation, and only
		// after validating an installation-signed case compaction cut. Carry
		// that task state into the case-capable request without replaying any
		// ordinary result or raw pre-cut transcript beside it.
		continuation := typedCaseContinuationHistoryV1(input.CaseTaskContinuationHistory)
		history = append(continuation, history...)
	}
	if input.CaseSensitive && !input.Policy.ProviderUsesCaseDataAuthority() {
		// Untyped case-thread prose has no ordinary-result provenance. A pure
		// ordinary turn starts from the host-reconstructed typed ordinary finals
		// and its current prompt.
		history = ordinaryHistory
		background = nil
	}
	messages := []domainmodel.Message{{Role: "system", Content: baseSystemPrompt}}
	messages = append(messages, history...)
	messages = append(messages, background...)
	userMessage := domainmodel.Message{Role: "user", Content: input.UserPrompt}
	if strings.TrimSpace(input.AttachmentPlanDigest) != "" {
		userMessage.PrivateAttachmentPlanDigest = input.AttachmentPlanDigest
	}
	messages = append(messages, userMessage)
	baseSystemPrompt, messages = appmodel.ApplyContextEpoch(baseSystemPrompt, messages, input.ContextEpoch)
	ordinaryLaneMessages := []domainmodel.Message{{Role: "system", Content: baseSystemPrompt}}
	ordinaryLaneMessages = append(ordinaryLaneMessages, ordinaryHistory...)
	ordinaryPrompt := strings.TrimSpace(input.Policy.OrdinaryPrompt)
	if ordinaryPrompt != "" && IndependentOrdinaryPromptV1(ordinaryPrompt) == ordinaryPrompt {
		ordinaryLaneMessages = append(ordinaryLaneMessages, domainmodel.Message{Role: "user", Content: ordinaryPrompt})
	} else {
		ordinaryPrompt = ""
	}
	ordinaryLaneMessages = privacyprojectionapp.OrdinaryOnlyProviderMessagesV1(ordinaryLaneMessages)
	systemPrompt := RuntimeProviderSystemPromptV1(baseSystemPrompt, input.Policy)
	messages = replaceInitialProviderSystemPromptV1(messages, systemPrompt)
	ordinaryResultInputIsolated := false
	if input.Policy.Active && input.Policy.SourceUnavailable && input.Policy.OrdinaryWorkRequested && ordinaryPrompt != "" {
		// The protected clause is closed at the admitted cut. Run only the
		// independently compiled ordinary subrequest; the raw mixed prompt and
		// protected history never enter this provider lane.
		ordinaryLaneMessages = replaceInitialProviderSystemPromptV1(ordinaryLaneMessages, systemPrompt)
		messages = appmodel.CloneProviderMessages(ordinaryLaneMessages)
		ordinaryResultInputIsolated = true
	}
	if !input.Policy.ProviderUsesCaseDataAuthority() {
		messages = privacyprojectionapp.OrdinaryOnlyProviderMessagesV1(messages)
	}
	currentPromptOrdinaryOnly := !input.CaseSensitive ||
		(!PromptRequiresCaseRiskAdmission(input.UserPrompt) &&
			!domainsecurity.ContainsProtectedCaseFactCandidate(input.UserPrompt))
	return InitialProviderMessagesV1{
		BaseSystemPrompt:            baseSystemPrompt,
		SystemPrompt:                systemPrompt,
		Messages:                    messages,
		OrdinaryLaneMessages:        ordinaryLaneMessages,
		OrdinaryResultInputIsolated: ordinaryResultInputIsolated || (!input.Policy.Active && currentPromptOrdinaryOnly),
	}
}

func typedCaseContinuationHistoryV1(history []domainmodel.Message) []domainmodel.Message {
	var continuation *domainmodel.Message
	for _, message := range history {
		if message.Role != "user" || !strings.HasPrefix(message.Content, appmodel.TypedOrdinaryCaseContinuationPrefixV1) {
			continue
		}
		if continuation != nil {
			return nil
		}
		cloned := message
		continuation = &cloned
	}
	if continuation == nil {
		return nil
	}
	return []domainmodel.Message{*continuation}
}

func BindInitialProviderIngressAliasesV1(
	securityContext domainsecurity.TurnSecurityContext,
	consumed InitialProviderIngressConsumptionV1,
	initial *InitialProviderMessagesV1,
) error {
	if len(consumed.Aliases) == 0 {
		return nil
	}
	if initial == nil {
		return privacyprojectionapp.ErrProviderPrivacyAuthorityUnavailable
	}
	for messageIndex := len(initial.Messages) - 1; messageIndex >= 0; messageIndex-- {
		message := &initial.Messages[messageIndex]
		if message.Role != "user" || message.Content != consumed.Text {
			continue
		}
		return privacyprojectionapp.BindProviderCaseAliasesToMessageV1(
			securityContext,
			consumed.Aliases,
			message,
		)
	}
	return privacyprojectionapp.ErrProviderPrivacyAuthorityUnavailable
}

// RuntimeProviderSystemPromptV1 compiles an effect-specific instruction from
// an immutable base. An inactive policy deliberately returns the base exactly.
func RuntimeProviderSystemPromptV1(base string, policy CaseFundAnalysisPolicy) string {
	base = strings.TrimSpace(base)
	if !policy.Active || strings.TrimSpace(policy.SystemInstruction) == "" {
		return base
	}
	return strings.TrimSpace(base + "\n\n" + policy.SystemInstruction)
}

func replaceInitialProviderSystemPromptV1(messages []domainmodel.Message, systemPrompt string) []domainmodel.Message {
	next := appmodel.CloneProviderMessages(messages)
	for index := range next {
		if next[index].Role == "system" {
			next[index].Content = systemPrompt
			return next
		}
	}
	return append([]domainmodel.Message{{Role: "system", Content: systemPrompt}}, next...)
}
