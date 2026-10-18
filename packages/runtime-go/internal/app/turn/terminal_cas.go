package turn

import (
	"errors"
	"reflect"
	"strings"
	"time"

	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecret "analytix.local/runtime-go/internal/domain/secretprojection"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

var ErrTerminalCASTurnNotFound = errors.New("terminal CAS turn does not exist")

type PrepareTerminalCASInput struct {
	Thread                    map[string]any
	ThreadID                  string
	TurnID                    string
	Status                    string
	AppendItems               []map[string]any
	Fields                    map[string]any
	PrivateFinal              domainevidence.PrivateAcceptedFinalRecord
	FactAuthority             FactFinalMutationAuthority
	CaseThread                bool
	Now                       time.Time
	LoadGeneralTerminalEvents func() ([]map[string]any, error)
}

type PreparedTerminalCAS struct {
	Thread           map[string]any
	CurrentStatus    string
	Applied          bool
	HasAcceptedFinal bool
}

// PrepareTerminalCASMutation owns terminal-state policy while the outbound
// adapter holds its durable thread lock. It performs no persistence itself.
func PrepareTerminalCASMutation(input PrepareTerminalCASInput) (PreparedTerminalCAS, error) {
	for _, item := range input.AppendItems {
		if err := domainsecret.ValidateValueLimitsV1(item); err != nil {
			return PreparedTerminalCAS{}, errors.Join(domainevent.ErrCredentialProjection, err)
		}
	}
	if err := domainsecret.ValidateValueLimitsV1(input.Fields); err != nil {
		return PreparedTerminalCAS{}, errors.Join(domainevent.ErrCredentialProjection, err)
	}
	// Qualify original types/fields before any authority parser can normalize
	// them. All ordinary record budgets above have already been checked.
	for _, record := range append([]map[string]any{input.Fields}, input.AppendItems...) {
		if record["acceptedFinal"] != nil {
			if _, err := parseAcceptedFinalForPublicValidationV1(record["acceptedFinal"]); err != nil {
				return PreparedTerminalCAS{}, err
			}
		}
	}
	hasAcceptedFinal, err := ValidateAcceptedFinalCASAuthority(
		input.ThreadID, input.TurnID, input.Status, input.AppendItems, input.Fields,
		input.PrivateFinal, input.FactAuthority,
	)
	if err != nil {
		return PreparedTerminalCAS{}, err
	}
	fieldsView, err := terminalCASPublicFieldsValidationViewV1(input, hasAcceptedFinal)
	if err != nil {
		return PreparedTerminalCAS{}, err
	}
	for index, record := range append([]map[string]any{input.Fields}, input.AppendItems...) {
		view := record
		if index == 0 {
			view = fieldsView
		}
		if hasAcceptedFinal && record["acceptedFinal"] != nil {
			view = shallowAcceptedFinalMapV1(record)
			view["acceptedFinal"] = acceptedFinalBinaryValidationViewV1(record["acceptedFinal"].(map[string]any))
		}
		if err := domainevent.ValidatePublicRecord(view); err != nil {
			return PreparedTerminalCAS{}, err
		}
	}
	currentStatus, found, terminal, authorityErr := InspectTerminalAuthorityV1(input.Thread, input.TurnID)
	if !found {
		return PreparedTerminalCAS{}, ErrTerminalCASTurnNotFound
	}
	if authorityErr != nil {
		return PreparedTerminalCAS{CurrentStatus: currentStatus}, authorityErr
	}
	if terminal {
		if err := validateTerminalCASReplayWinner(input, hasAcceptedFinal, currentStatus); err != nil {
			return PreparedTerminalCAS{CurrentStatus: currentStatus}, err
		}
		return PreparedTerminalCAS{CurrentStatus: currentStatus, HasAcceptedFinal: hasAcceptedFinal}, nil
	}
	if input.CaseThread && !hasAcceptedFinal {
		return PreparedTerminalCAS{}, errors.New("case terminal update requires host accepted-final authority")
	}
	if err := ValidateAcceptedFinalTerminalUpdate(
		input.Thread, input.TurnID, input.Status, input.AppendItems, input.Fields,
	); err != nil {
		return PreparedTerminalCAS{}, err
	}
	threadFields, err := terminalCASThreadFields(input)
	if err != nil {
		return PreparedTerminalCAS{}, err
	}
	finishedAt, err := AcceptedFinalFinishedAt(input.Fields, input.Now)
	if err != nil {
		return PreparedTerminalCAS{}, err
	}
	result := ApplyTerminalUpdate(TerminalUpdateInput{
		Thread: input.Thread, TurnID: input.TurnID, Status: input.Status, ProtectTerminal: true,
		AppendItems: input.AppendItems, Fields: input.Fields, ThreadFields: threadFields, FinishedAt: finishedAt,
	})
	if !result.Found {
		return PreparedTerminalCAS{}, ErrTerminalCASTurnNotFound
	}
	return PreparedTerminalCAS{
		Thread: result.Thread, CurrentStatus: result.CurrentStatus,
		Applied: result.Applied, HasAcceptedFinal: hasAcceptedFinal,
	}, nil
}

// terminalCASPublicFieldsValidationViewV1 qualifies canonical outbox authority
// before substituting its content-scanning view in a copy of ordinary fields.
// The original outbox remains intact for CAS and persistence; no generic nested
// record receives the root usage event's validation treatment.
func terminalCASPublicFieldsValidationViewV1(input PrepareTerminalCASInput, hasAcceptedFinal bool) (map[string]any, error) {
	commitValue, present := input.Fields["generalTerminalPublication"]
	if hasAcceptedFinal || !present {
		return input.Fields, nil
	}
	if input.ThreadID != stringField(input.Thread, "id") {
		return nil, errors.New("general terminal CAS thread identity differs")
	}
	// Reject non-JSON original values before either authority path can marshal
	// the outbox. The view is used only after all canonical authority checks.
	commitView, err := GeneralTerminalPublicationCommitValidationViewV1(commitValue)
	if err != nil {
		return nil, err
	}
	_, found, terminal, err := InspectTerminalAuthorityV1(input.Thread, input.TurnID)
	if !found {
		return nil, ErrTerminalCASTurnNotFound
	}
	if err != nil {
		return nil, err
	}
	if terminal {
		turn, _ := securityTurnByID(input.Thread, strings.TrimSpace(input.TurnID))
		frozen, frozenErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
		commit, commitErr := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(commitValue)
		if frozenErr != nil || commitErr != nil || !domainsecurity.TurnSecurityContextIsGeneral(frozen) ||
			frozen.ThreadID != input.ThreadID || frozen.TurnID != strings.TrimSpace(input.TurnID) ||
			commit.ThreadID != frozen.ThreadID || commit.TurnID != frozen.TurnID ||
			commit.ContextDigest != frozen.ContextDigest || commit.ContextEpoch != frozen.ContextEpoch ||
			commit.DatasetSnapshotID != frozen.DatasetSnapshotID {
			return nil, errors.New("general terminal CAS loser context is detached")
		}
		// Validate the contender against its target's frozen context, even when
		// a newer turn owns the current context. This active, empty target exists
		// only for validation; the durable winner remains the first CAS result.
		candidate := map[string]any{
			"id": input.ThreadID, "securityState": turn["securityContext"],
			"turns": []any{map[string]any{
				"id": frozen.TurnID, "threadId": frozen.ThreadID, "status": "running",
				"securityContext": turn["securityContext"], "items": []any{},
			}},
		}
		if err := ValidateGeneralTerminalPublicationUpdateV1(candidate, input.TurnID, input.Status, input.AppendItems, input.Fields); err != nil {
			return nil, err
		}
	} else if err := ValidateGeneralTerminalPublicationUpdateV1(
		input.Thread, input.TurnID, input.Status, input.AppendItems, input.Fields,
	); err != nil {
		return nil, err
	}
	view := shallowAcceptedFinalMapV1(input.Fields)
	view["generalTerminalPublication"] = commitView
	return view, nil
}

func terminalCASThreadFields(input PrepareTerminalCASInput) (map[string]any, error) {
	fields := map[string]any{}
	commitValue, present := input.Fields["generalTerminalPublication"]
	if !present {
		return fields, nil
	}
	if input.LoadGeneralTerminalEvents == nil {
		return nil, errors.New("general terminal CAS event inventory is unavailable")
	}
	events, err := input.LoadGeneralTerminalEvents()
	if err != nil {
		return nil, err
	}
	entries, err := PreflightGeneralTerminalPublicationInventoryV1(input.Thread, events)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.State != GeneralTerminalPublicationCompleteV1 {
			return nil, errors.New("general terminal CAS cannot extend an unsettled archive")
		}
	}
	archive, err := NextGeneralTerminalPublicationArchiveV1(input.Thread, commitValue)
	if err != nil {
		return nil, err
	}
	fields[GeneralTerminalPublicationArchiveFieldV1] = archive
	return fields, nil
}

func validateTerminalCASReplayWinner(
	input PrepareTerminalCASInput,
	hasAcceptedFinal bool,
	currentStatus string,
) error {
	turn, found := securityTurnByID(input.Thread, strings.TrimSpace(input.TurnID))
	if !found || (hasAcceptedFinal && strings.TrimSpace(currentStatus) != strings.TrimSpace(input.Status)) {
		return errors.New("terminal CAS replay status or turn identity differs")
	}
	value, hasWinner := turn["acceptedFinal"]
	if !hasAcceptedFinal {
		if hasWinner && value != nil {
			return errors.New("generic terminal replay cannot claim an accepted-final winner")
		}
		if input.Fields["generalTerminalCASBinding"] == nil || input.Fields["generalTerminalPublication"] == nil {
			return errors.New("general terminal replay requires a context-bound publication outbox")
		}
		return nil
	}
	winner, err := domainevidence.ParseAcceptedFinalRecord(value)
	frozen, frozenErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	current, currentErr := domainsecurity.ParseTurnSecurityContext(input.Thread["securityState"])
	if err != nil || frozenErr != nil || currentErr != nil ||
		!reflect.DeepEqual(winner, input.PrivateFinal.AcceptedFinal) ||
		frozen.ContextDigest != input.PrivateFinal.SecurityContext.ContextDigest ||
		current.ContextDigest != input.PrivateFinal.SecurityContext.ContextDigest {
		return errors.Join(errors.New("terminal CAS replay winner differs from exact private authority"), err, frozenErr, currentErr)
	}
	return nil
}
