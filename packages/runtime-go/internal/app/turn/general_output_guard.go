package turn

import (
	"errors"
	"strings"

	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const GeneralCaseFactCandidateBlockedText = domainordinaryresult.HostFixedProtectedFactBlockedTextV1

const OrdinaryProviderResultWithheldText = domainordinaryresult.HostFixedProviderResultWithheldTextV1

// GeneralProviderFinalQuarantinedText is the legacy host-fixed boundary used
// when a general terminal has no admitted typed ordinary result. A successful
// provider candidate may cross the terminal CAS only through ResultSlotV1;
// raw provider prose never falls back through this boundary.
const GeneralProviderFinalQuarantinedText = domainevent.GeneralTerminalCompletedBoundaryTextV1

type GeneralOutputDecision struct {
	Text           string
	Blocked        bool
	OrdinaryResult domainordinaryresult.ResultSlotV1
}

// CompileOrdinaryResultSlot admits a provider candidate only as a typed,
// non-evidentiary ordinary result. It is valid in both general and
// case-sensitive turns because the ordinary Agent base is permanent; the
// caller must still hold the exact ordinary candidate terminal lease before
// persisting it. Unsafe prose is replaced by a deterministic host fallback.
func CompileOrdinaryResultSlot(
	securityContext domainsecurity.TurnSecurityContext,
	candidate string,
) (domainordinaryresult.ResultSlotV1, bool, error) {
	if domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(securityContext) != nil {
		return domainordinaryresult.ResultSlotV1{}, false, errors.New("ordinary output publication authority is invalid")
	}
	if domainsecurity.TurnSecurityContextIsCaseSensitive(securityContext) &&
		domainsecurity.ContainsProtectedCaseFactCandidate(candidate) {
		fallback, fallbackErr := domainordinaryresult.NewHostFixedResultSlotV1(GeneralCaseFactCandidateBlockedText)
		if fallbackErr != nil {
			return domainordinaryresult.ResultSlotV1{}, false, fallbackErr
		}
		return fallback, true, nil
	}
	slot, err := domainordinaryresult.NewResultSlotV1(candidate)
	if err == nil {
		return slot, false, nil
	}
	if errors.Is(err, domainordinaryresult.ErrResultSlotEmptyV1) {
		return domainordinaryresult.ResultSlotV1{}, false, err
	}
	boundary := OrdinaryProviderResultWithheldText
	if errors.Is(err, domainordinaryresult.ErrResultSlotProtectedFactV1) {
		boundary = GeneralCaseFactCandidateBlockedText
	}
	fallback, fallbackErr := domainordinaryresult.NewHostFixedResultSlotV1(boundary)
	if fallbackErr != nil {
		return domainordinaryresult.ResultSlotV1{}, false, fallbackErr
	}
	return fallback, true, nil
}

// GuardGeneralOutput admits a complete provider candidate only under the
// exact frozen general publication policy. Provider text remains a private
// draft until this function returns. Detection can only remove authority: it
// never upgrades a draft into a fact or evidence-backed answer.
func GuardGeneralOutput(securityContext domainsecurity.TurnSecurityContext, candidate string) (GeneralOutputDecision, error) {
	if domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil ||
		!domainsecurity.TurnSecurityContextIsGeneral(securityContext) {
		return GeneralOutputDecision{}, errors.New("general output publication authority is invalid")
	}
	candidate = strings.TrimSpace(candidate)
	slot, err := domainordinaryresult.NewResultSlotV1(candidate)
	if err == nil {
		return GeneralOutputDecision{Text: slot.Text, OrdinaryResult: slot}, nil
	}
	boundary := OrdinaryProviderResultWithheldText
	if errors.Is(err, domainordinaryresult.ErrResultSlotProtectedFactV1) {
		boundary = GeneralCaseFactCandidateBlockedText
	}
	fallback, fallbackErr := domainordinaryresult.NewHostFixedResultSlotV1(boundary)
	if fallbackErr != nil {
		return GeneralOutputDecision{}, fallbackErr
	}
	return GeneralOutputDecision{Text: fallback.Text, Blocked: true, OrdinaryResult: fallback}, nil
}
