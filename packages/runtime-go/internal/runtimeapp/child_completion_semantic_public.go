package runtimeapp

import (
	"context"
	"errors"

	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	appturn "analytix.local/runtime-go/internal/app/turn"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

// Read only exact final After bytes for the requested primary/event identity.
// A second family cannot silently replace an existing primary owner.
func (projection *runtimeChildCompletionProjectionV1) publicAfterV1(inventory *runtimeOriginalPrimaryInventoryV1, threadID, leaf string) ([]byte, bool, error) {
	entry, original := inventory.entries[threadID]
	expected := ""
	if original {
		expected = entry.primary.Path[:len(entry.primary.Path)-len("thread.json")] + leaf
	}
	var body []byte
	found := false
	for _, operation := range projection.activeOperationsV1() {
		if operation.Path != "durable/threads/"+threadID+"/"+leaf && operation.Path != "durable/runtime-go/threads/"+threadID+"/"+leaf {
			continue
		}
		if original && operation.Path != expected || found {
			return nil, false, errors.New("candidate child public identity has conflicting families")
		}
		switch operation.Kind {
		case domainstartup.SemanticOperationSetMode:
			continue
		case domainstartup.SemanticOperationRemoveFile:
			return nil, false, errors.New("candidate child public proof was removed")
		case domainstartup.SemanticOperationInstallFile:
			if projection.readAfter == nil {
				return nil, false, errors.New("candidate child public After bytes are unavailable")
			}
			var err error
			body, err = projection.readAfter(operation)
			if err != nil {
				return nil, false, err
			}
			if int64(len(body)) != operation.After.Size || domainsecurity.SHA256Hex(body) != operation.After.SHA256 {
				return nil, false, errors.New("candidate child public After integrity changed")
			}
			found = true
		default:
			return nil, false, errors.New("candidate child public transition is invalid")
		}
	}
	return body, found, nil
}

func validateRuntimeChildPublicationEventsV1(record domainevidence.PrivateAcceptedFinalRecord, events []map[string]any) error {
	plan, err := appturn.BuildAcceptedFinalPublicationPlan(record.AcceptedFinal, record.RenderedText, record.PublicationIntent)
	if err != nil {
		return err
	}
	start := 0
	for _, event := range events {
		if contracts.StringField(event, "publicationCommitId") == record.AcceptedFinal.RecordDigest {
			start, _ = contracts.NumericSeq(event["seq"])
			break
		}
	}
	if start <= 0 {
		return errors.New("stored child publication group is missing")
	}
	expected := make([]map[string]any, 0, len(plan.Events))
	for index, event := range plan.Events {
		draft := contracts.CloneMap(event.Draft)
		draft["seq"] = start + index
		expected = append(expected, draft)
	}
	return appturn.ValidateAcceptedFinalDurableReadbackV1(events, expected, false)
}

func (projection *runtimeChildCompletionProjectionV1) observeFinalV1(ctx context.Context, inventory *runtimeOriginalPrimaryInventoryV1, record domainevidence.PrivateAcceptedFinalRecord) (_ domainevidence.AcceptedFinalCASObservationV1, resultErr error) {
	empty := domainevidence.AcceptedFinalCASObservationV1{}
	threadID, turnID := record.SecurityContext.ThreadID, record.SecurityContext.TurnID
	primary, changed, err := projection.publicAfterV1(inventory, threadID, "thread.json")
	if err != nil {
		return empty, err
	}
	entry, original := inventory.entries[threadID]
	var observations map[string]domainevidence.AcceptedFinalCASObservationV1
	if changed {
		observations, err = finalauthority.ParseAcceptedFinalCASObservationsV1(ctx, threadID, []string{turnID}, primary)
	} else if original {
		observations, err = entry.reader.ReadAcceptedFinalCASObservations(ctx, threadID, []string{turnID})
		if err == nil && observations[turnID].ThreadFileSHA256 != entry.digest {
			return empty, errors.New("candidate child original primary changed")
		}
	} else {
		return empty, errors.New("candidate child primary is unavailable")
	}
	if err != nil {
		return empty, err
	}
	body, changed, err := projection.publicAfterV1(inventory, threadID, "events.jsonl")
	if err != nil {
		return empty, err
	}
	var events []map[string]any
	if changed {
		events, err = eventlog.ParseCommittedEventLogBytesV1(ctx, threadID, body)
	} else if original && entry.events.Type == "file" {
		check := func() error {
			digest, err := entry.reader.ReadCommittedEventLogSHA256V1(ctx, threadID)
			if err != nil || digest != entry.events.SHA256 {
				return errors.Join(errors.New("candidate child original event log changed"), err)
			}
			return ctx.Err()
		}
		if err := check(); err != nil {
			return empty, err
		}
		defer func() { resultErr = errors.Join(resultErr, check()) }()
		loaded, frontier, readErr := eventlog.NewStore(entry.eventRoot).ObserveSinceWithFrontier(ctx, threadID, 0)
		if readErr != nil || len(loaded.Diagnostics) != 0 || !frontier.Exists || frontier.SHA256 != entry.events.SHA256 || frontier.Size != entry.events.Size {
			return empty, errors.Join(errors.New("candidate child original event log is not exact"), readErr)
		}
		events = loaded.Events
	} else {
		return empty, errors.New("candidate child public events are unavailable")
	}
	if err != nil {
		return empty, err
	}
	if err := validateRuntimeChildPublicationEventsV1(record, events); err != nil {
		return empty, err
	}
	return observations[turnID], ctx.Err()
}
