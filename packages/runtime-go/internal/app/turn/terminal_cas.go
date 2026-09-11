package turn

import (
	"errors"
	"reflect"
	"strings"
	"time"

	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
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
		if err := domainevent.ValidatePublicRecord(item); err != nil {
			return PreparedTerminalCAS{}, err
		}
	}
	if err := domainevent.ValidatePublicRecord(input.Fields); err != nil {
		return PreparedTerminalCAS{}, err
	}
	hasAcceptedFinal, err := ValidateAcceptedFinalCASAuthority(
		input.ThreadID, input.TurnID, input.Status, input.AppendItems, input.Fields,
		input.PrivateFinal, input.FactAuthority,
	)
	if err != nil {
		return PreparedTerminalCAS{}, err
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
	if !found || strings.TrimSpace(currentStatus) != strings.TrimSpace(input.Status) {
		return errors.New("terminal CAS replay status or turn identity differs")
	}
	value, hasWinner := turn["acceptedFinal"]
	if !hasAcceptedFinal {
		if hasWinner && value != nil {
			return errors.New("generic terminal replay cannot claim an accepted-final winner")
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
