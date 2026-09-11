package turn

import (
	"errors"
	"reflect"
	"strings"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

type AcceptedFinalCompletionStore interface {
	FinishTurnIfActiveWithItemsAndFields(threadID, turnID, status string, items []map[string]any, fields map[string]any) (bool, string, error)
	RecordEvent(map[string]any) (map[string]any, []string, error)
}

// FactFinalMutationAuthority is an opaque, callback-scoped capability for one
// exact fact-bearing private final. Implementations must fail after the
// authority callback returns.
type FactFinalMutationAuthority interface {
	UseExact(domainevidence.PrivateAcceptedFinalRecord, func() error) error
}

// AcceptedFinalAuthorizedCompletionStore is the only public-turn CAS that may
// receive accepted-final fields. The implementation must verify factAuthority
// while holding the durable turn lock immediately before mutation.
type AcceptedFinalAuthorizedCompletionStore interface {
	FinishTurnIfActiveWithAcceptedFinalAuthority(
		threadID, turnID, status string,
		items []map[string]any,
		fields map[string]any,
		privateFinal domainevidence.PrivateAcceptedFinalRecord,
		factAuthority FactFinalMutationAuthority,
	) (bool, string, error)
}

type PersistAcceptedFinalInput struct {
	Store             AcceptedFinalCompletionStore
	ThreadID          string
	TurnID            string
	RenderedText      string
	AcceptedFinal     domainevidence.AcceptedFinalRecord
	PublicationIntent domainevidence.TerminalPublicationIntent
	PrivateFinal      domainevidence.PrivateAcceptedFinalRecord
	FactAuthority     FactFinalMutationAuthority
}

type PersistAcceptedFinalResult struct {
	CompletionRecord CompletionRecord
	AcceptedFinal    domainevidence.AcceptedFinalRecord
	Publication      AcceptedFinalPublicationPlan
	Changed          bool
	Status           string
}

func AcceptedFinalFinishedAt(fields map[string]any, fallback time.Time) (string, error) {
	if fields["acceptedFinal"] == nil {
		if fields["generalTerminalPublication"] == nil {
			return fallback.UTC().Format(time.RFC3339Nano), nil
		}
		commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(fields["generalTerminalPublication"])
		if err != nil {
			return "", err
		}
		return commit.CommittedAt, nil
	}
	record, err := domainevidence.ParseAcceptedFinalRecord(fields["acceptedFinal"])
	if err != nil {
		return "", err
	}
	return record.AcceptedAt, nil
}

func PersistAcceptedFinalTerminal(input PersistAcceptedFinalInput) (PersistAcceptedFinalResult, error) {
	if input.Store == nil || strings.TrimSpace(input.ThreadID) == "" || strings.TrimSpace(input.TurnID) == "" {
		return PersistAcceptedFinalResult{}, errors.New("accepted final completion input is invalid")
	}
	terminalStatus := strings.TrimSpace(input.PublicationIntent.TerminalStatus)
	if terminalStatus != "completed" && terminalStatus != "failed" && terminalStatus != "aborted" {
		return PersistAcceptedFinalResult{}, errors.New("accepted final terminal status is invalid")
	}
	acceptedFinal := input.AcceptedFinal
	privateFinal := input.PrivateFinal
	if err := domainevidence.ValidateAcceptedFinalForCurrentWriteV1(acceptedFinal); err != nil ||
		ValidatePrivateAcceptedFinalMutationAuthority(privateFinal, input.FactAuthority) != nil ||
		privateFinal.AcceptedFinal.RecordDigest != acceptedFinal.RecordDigest ||
		privateFinal.RenderedText != input.RenderedText ||
		!reflect.DeepEqual(privateFinal.PublicationIntent, input.PublicationIntent) ||
		acceptedFinal.ThreadID != strings.TrimSpace(input.ThreadID) || acceptedFinal.TurnID != strings.TrimSpace(input.TurnID) ||
		domainsecurity.SHA256Hex([]byte(input.RenderedText)) != acceptedFinal.RenderedTextSHA256 ||
		domainevidence.ValidateTerminalPublicationIntent(input.PublicationIntent, acceptedFinal.TerminalReason) != nil {
		return PersistAcceptedFinalResult{}, errors.New("accepted final authority or rendered text is invalid")
	}
	publication, err := BuildAcceptedFinalPublicationPlan(acceptedFinal, input.RenderedText, input.PublicationIntent)
	if err != nil {
		return PersistAcceptedFinalResult{}, err
	}
	authorizedStore, ok := input.Store.(AcceptedFinalAuthorizedCompletionStore)
	if !ok {
		return PersistAcceptedFinalResult{}, errors.New("accepted final completion store lacks authorized CAS")
	}
	changed, status, err := authorizedStore.FinishTurnIfActiveWithAcceptedFinalAuthority(
		input.ThreadID, input.TurnID, terminalStatus, publication.TurnItems, publication.TurnFields,
		privateFinal, input.FactAuthority,
	)
	if err != nil || !changed {
		return PersistAcceptedFinalResult{CompletionRecord: publication.Completion, AcceptedFinal: acceptedFinal, Publication: publication, Changed: changed, Status: status}, err
	}
	return PersistAcceptedFinalResult{CompletionRecord: publication.Completion, AcceptedFinal: acceptedFinal, Publication: publication, Changed: true, Status: status}, nil
}

func ValidatePrivateAcceptedFinalMutationAuthority(
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	factAuthority FactFinalMutationAuthority,
) error {
	if domainevidence.FinalAnswerRequiresPublicationSnapshotProof(privateFinal.Envelope) {
		if factAuthority == nil {
			return errors.New("fact final mutation authority is unavailable")
		}
		return domainevidence.ValidatePrivateAcceptedFinalPublicationAuthorityWithWitnessV1(
			privateFinal,
			func(record domainevidence.PrivateAcceptedFinalRecord) error {
				return factAuthority.UseExact(record, func() error { return nil })
			},
		)
	}
	if factAuthority != nil {
		return errors.New("boundary final must not carry fact mutation authority")
	}
	return domainevidence.ValidatePrivateAcceptedFinalPublicationAuthority(privateFinal)
}

func UsePrivateAcceptedFinalMutationAuthority(
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	factAuthority FactFinalMutationAuthority,
	mutation func() error,
) error {
	if mutation == nil {
		return errors.New("private accepted-final mutation is unavailable")
	}
	if domainevidence.FinalAnswerRequiresPublicationSnapshotProof(privateFinal.Envelope) {
		if factAuthority == nil {
			return errors.New("fact final mutation authority is unavailable")
		}
		return factAuthority.UseExact(privateFinal, mutation)
	}
	if factAuthority != nil {
		return errors.New("boundary final must not carry fact mutation authority")
	}
	if err := domainevidence.ValidatePrivateAcceptedFinalPublicationAuthority(privateFinal); err != nil {
		return err
	}
	return mutation()
}

// ValidateAcceptedFinalCASAuthority binds the entire public CAS payload to the
// exact private final. It is side-effect free; durable adapters must call it
// again in their atomic-write prewrite callback.
func ValidateAcceptedFinalCASAuthority(
	threadID, turnID, status string,
	appendItems []map[string]any,
	fields map[string]any,
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	factAuthority FactFinalMutationAuthority,
) (bool, error) {
	hasAcceptedFinal, err := validateAcceptedFinalCASPayload(
		threadID, turnID, status, appendItems, fields, privateFinal,
	)
	if err != nil {
		return hasAcceptedFinal, err
	}
	if !hasAcceptedFinal {
		if factAuthority != nil {
			return false, errors.New("terminal CAS authority has no accepted final")
		}
		return false, nil
	}
	if err := ValidatePrivateAcceptedFinalMutationAuthority(privateFinal, factAuthority); err != nil {
		return true, errors.Join(errors.New("accepted final CAS lacks live mutation authority"), err)
	}
	return true, nil
}

// UseAcceptedFinalCASAuthority holds the fact-witness lease across the exact
// atomic mutation. The mutation must be the adapter's immediate durable write,
// not a preflight that returns before persistence.
func UseAcceptedFinalCASAuthority(
	threadID, turnID, status string,
	appendItems []map[string]any,
	fields map[string]any,
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	factAuthority FactFinalMutationAuthority,
	mutation func() error,
) error {
	if mutation == nil {
		return errors.New("accepted final CAS mutation is unavailable")
	}
	hasAcceptedFinal, err := validateAcceptedFinalCASPayload(
		threadID, turnID, status, appendItems, fields, privateFinal,
	)
	if err != nil || !hasAcceptedFinal {
		return errors.Join(errors.New("accepted final CAS payload is invalid at mutation"), err)
	}
	if domainevidence.FinalAnswerRequiresPublicationSnapshotProof(privateFinal.Envelope) {
		return UsePrivateAcceptedFinalMutationAuthority(privateFinal, factAuthority, func() error {
			if _, err := validateAcceptedFinalCASPayload(
				threadID, turnID, status, appendItems, fields, privateFinal,
			); err != nil {
				return err
			}
			return mutation()
		})
	}
	return UsePrivateAcceptedFinalMutationAuthority(privateFinal, factAuthority, mutation)
}

func validateAcceptedFinalCASPayload(
	threadID, turnID, status string,
	appendItems []map[string]any,
	fields map[string]any,
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
) (bool, error) {
	hasAcceptedFinal := fields["acceptedFinal"] != nil
	for _, item := range appendItems {
		hasAcceptedFinal = hasAcceptedFinal || item["acceptedFinal"] != nil
	}
	if !hasAcceptedFinal {
		if privateFinal.StoreDigest != "" {
			return false, errors.New("terminal CAS authority has no accepted final")
		}
		return false, nil
	}
	acceptedFinal, parseErr := domainevidence.ParseAcceptedFinalRecord(fields["acceptedFinal"])
	if parseErr != nil || domainevidence.ValidatePrivateAcceptedFinalRecord(privateFinal) != nil ||
		privateFinal.SchemaVersion != domainevidence.PrivateAcceptedFinalRecordVersion ||
		privateFinal.AcceptedFinal.SchemaVersion != domainevidence.AcceptedFinalRecordVersion ||
		privateFinal.SecurityContext.ThreadID != threadID || privateFinal.SecurityContext.TurnID != turnID ||
		!reflect.DeepEqual(privateFinal.AcceptedFinal, acceptedFinal) {
		return true, errors.Join(errors.New("accepted final CAS lacks exact private authority"), parseErr)
	}
	publication, err := BuildAcceptedFinalPublicationPlan(
		privateFinal.AcceptedFinal,
		privateFinal.RenderedText,
		privateFinal.PublicationIntent,
	)
	if err != nil || status != privateFinal.PublicationIntent.TerminalStatus ||
		!reflect.DeepEqual(appendItems, publication.TurnItems) || !reflect.DeepEqual(fields, publication.TurnFields) {
		return true, errors.Join(errors.New("accepted final CAS payload differs from the exact publication plan"), err)
	}
	return true, nil
}

// ValidateAcceptedFinalTerminalUpdate runs under the durable thread lock. It
// closes the gap between gate evaluation and persistence by rejecting a final
// produced for an older case/epoch or a different turn.
func ValidateAcceptedFinalTerminalUpdate(thread map[string]any, turnID, status string, items []map[string]any, fields map[string]any) error {
	turn, found := securityTurnByID(thread, strings.TrimSpace(turnID))
	if !found {
		return errors.New("accepted final turn does not exist")
	}
	turnContext, contextErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if contextErr != nil {
		return errors.New("terminal update requires a valid frozen security context")
	}
	caseRequired := domainsecurity.TurnSecurityContextIsCaseSensitive(turnContext)
	value, hasAcceptedFinal := fields["acceptedFinal"]
	if !hasAcceptedFinal {
		if caseRequired {
			return errors.New("case-bound terminal update requires an accepted final")
		}
		_, hasBinding := fields["generalTerminalCASBinding"]
		_, hasPublication := fields["generalTerminalPublication"]
		if domainsecurity.TurnSecurityContextIsGeneral(turnContext) {
			if err := ValidateGeneralTerminalPublicationUpdateV1(thread, turnID, status, items, fields); err != nil {
				return err
			}
		} else if hasBinding || hasPublication {
			return errors.New("general terminal authority is invalid for this terminal path")
		} else {
			for _, item := range items {
				if stringField(item, "kind") == "assistant_text" {
					return errors.New("general assistant terminal update requires a context-bound publication outbox")
				}
			}
		}
		for _, item := range items {
			if item["acceptedFinal"] != nil {
				return errors.New("accepted final item is missing turn authority")
			}
		}
		return nil
	}
	record, err := domainevidence.ParseAcceptedFinalRecord(value)
	if err != nil {
		return err
	}
	if err := domainevidence.ValidateAcceptedFinalForCurrentWriteV1(record); err != nil {
		return errors.New("accepted final lacks current-write authority")
	}
	expectedStatus, ok := domainevidence.FinalAnswerTerminalStatus(record.TerminalReason)
	if !ok || strings.TrimSpace(status) != expectedStatus {
		return errors.New("accepted final terminal status contradicts terminal reason")
	}
	current, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil || domainsecurity.ValidateTurnSecurityContextForCasePublication(current) != nil ||
		domainsecurity.ValidateTurnSecurityContextForCasePublication(turnContext) != nil ||
		(domainevidence.FinalAnswerVariantRequiresPublicationSnapshotProof(record.Variant) &&
			(domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(current) != nil ||
				domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(turnContext) != nil)) ||
		current.ThreadID != strings.TrimSpace(stringField(thread, "id")) || current.TurnID != strings.TrimSpace(turnID) ||
		current.ContextDigest != record.ContextDigest || current.ContextEpoch != record.ContextEpoch || current.DatasetSnapshotID != record.DatasetSnapshotID {
		return errors.New("accepted final does not match the current thread context")
	}
	if contextErr != nil || turnContext.ContextDigest != current.ContextDigest {
		return errors.New("accepted final does not match the frozen turn context")
	}
	matchedItem := false
	for _, item := range items {
		itemRecordValue := item["acceptedFinal"]
		if stringField(item, "kind") == "assistant_text" && itemRecordValue == nil {
			return errors.New("case terminal update contains an unaccepted assistant item")
		}
		if itemRecordValue == nil {
			continue
		}
		itemRecord, err := domainevidence.ParseAcceptedFinalRecord(itemRecordValue)
		text, _ := item["text"].(string)
		if err != nil || itemRecord.RecordDigest != record.RecordDigest ||
			domainsecurity.SHA256Hex([]byte(text)) != record.RenderedTextSHA256 || strings.TrimSpace(stringField(item, "turnId")) != strings.TrimSpace(turnID) {
			return errors.New("accepted final item integrity is invalid")
		}
		if matchedItem {
			return errors.New("accepted final update contains duplicate final items")
		}
		matchedItem = true
	}
	if !matchedItem {
		return errors.New("accepted final update is missing its rendered item")
	}
	return nil
}
