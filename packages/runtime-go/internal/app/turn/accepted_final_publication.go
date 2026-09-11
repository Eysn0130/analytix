package turn

import (
	"encoding/json"
	"errors"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const acceptedFinalPublicationEventDomain = "analytix.accepted-final-event/v1\x00"

type AcceptedFinalPublicationEvent struct {
	Slot          string
	EventID       string
	PayloadDigest string
	Draft         map[string]any
}

type AcceptedFinalPublicationPlan struct {
	Completion          CompletionRecord
	TurnItems           []map[string]any
	TurnFields          map[string]any
	Events              []AcceptedFinalPublicationEvent
	EventManifestDigest string
}

func BuildAcceptedFinalPublicationPlan(record domainevidence.AcceptedFinalRecord, renderedText string, intent domainevidence.TerminalPublicationIntent) (AcceptedFinalPublicationPlan, error) {
	if domainevidence.ValidateAcceptedFinalForCurrentWriteV1(record) != nil ||
		domainevidence.ValidateTerminalPublicationIntent(intent, record.TerminalReason) != nil ||
		domainsecurity.SHA256Hex([]byte(renderedText)) != record.RenderedTextSHA256 {
		return AcceptedFinalPublicationPlan{}, errors.New("accepted final publication plan input is invalid")
	}
	privateView, err := domainevidence.NewAcceptedFinalPublicViewFromRecordV2(record)
	if err != nil {
		return AcceptedFinalPublicationPlan{}, errors.New("accepted final publication plan public view is invalid")
	}
	genericView, err := domainevidence.NewAcceptedFinalPublicViewFromRecordV3(record)
	if err != nil {
		return AcceptedFinalPublicationPlan{}, errors.New("accepted final publication plan generic view is invalid")
	}
	return buildAcceptedFinalPublicationPlan(
		record,
		renderedText,
		intent,
		domainevidence.AcceptedFinalPublicViewRecordV2(privateView),
		domainevidence.AcceptedFinalPublicViewRecordV3(genericView),
	)
}

// BuildAcceptedFinalAuditPublicationPlan reconstructs only the immutable
// manifest bytes used by historical V2/V3/V4 records. It must never be used
// for current writes, repair, projection admission, or resume.
func BuildAcceptedFinalAuditPublicationPlan(record domainevidence.AcceptedFinalRecord, renderedText string, intent domainevidence.TerminalPublicationIntent) (AcceptedFinalPublicationPlan, error) {
	if record.SchemaVersion == domainevidence.AcceptedFinalRecordVersion ||
		domainevidence.ValidateAcceptedFinalRecord(record) != nil ||
		domainevidence.ValidateTerminalPublicationIntent(intent, record.TerminalReason) != nil ||
		domainsecurity.SHA256Hex([]byte(renderedText)) != record.RenderedTextSHA256 {
		return AcceptedFinalPublicationPlan{}, errors.New("historical accepted final audit plan input is invalid")
	}
	return buildAcceptedFinalPublicationPlan(record, renderedText, intent, nil, nil)
}

func buildAcceptedFinalPublicationPlan(
	record domainevidence.AcceptedFinalRecord,
	renderedText string,
	intent domainevidence.TerminalPublicationIntent,
	privateViewMap map[string]any,
	genericViewMap map[string]any,
) (AcceptedFinalPublicationPlan, error) {
	completion := BuildCompletionRecord(CompletionRecordInput{
		ThreadID: record.ThreadID, TurnID: record.TurnID, Model: intent.Model, AssistantText: renderedText,
		CreatedAt: intent.CreatedAt, FinishedAt: record.AcceptedAt, Usage: intent.Usage,
		CacheDiagnostics: intent.CacheDiagnostics, UsageSource: intent.UsageSource, ChildRunID: intent.ChildRunID,
	})
	acceptedFinalMap := domainevidence.AcceptedFinalRecordMap(record)
	completion.AssistantItem["acceptedFinal"] = acceptedFinalMap
	if privateViewMap != nil {
		completion.AssistantItem["acceptedFinalView"] = contracts.CloneMap(privateViewMap)
	}
	publicAssistantItem := completion.AssistantItem
	if genericViewMap != nil {
		publicAssistantItem = contracts.CloneMap(completion.AssistantItem)
		delete(publicAssistantItem, "acceptedFinal")
		publicAssistantItem["acceptedFinalView"] = contracts.CloneMap(genericViewMap)
	}
	completion.ItemCompletedEvent = AssistantItemCompletedEvent(publicAssistantItem)
	completion.UsageEvent["usageFinalStatus"] = intent.TerminalStatus
	completion.UsageEvent["acceptedFinalDigest"] = record.RecordDigest
	items := []map[string]any{completion.AssistantItem}
	type eventDraft struct {
		slot  string
		draft map[string]any
	}
	eventDrafts := []eventDraft{{slot: "assistant-final", draft: completion.ItemCompletedEvent}}
	terminalItem := acceptedFinalTerminalErrorItem(intent, record)
	if terminalItem != nil {
		items = append(items, terminalItem)
		eventDrafts = append(eventDrafts, eventDraft{slot: "terminal-error-item", draft: AssistantItemCompletedEvent(terminalItem)})
	}
	eventDrafts = append(eventDrafts,
		eventDraft{slot: "usage", draft: completion.UsageEvent},
		eventDraft{slot: "terminal", draft: acceptedFinalTerminalEvent(intent, record, terminalItem)},
	)
	fields := map[string]any{"acceptedFinal": acceptedFinalMap}
	if privateViewMap != nil {
		fields["acceptedFinalView"] = contracts.CloneMap(privateViewMap)
	}
	if intent.TerminalStatus == "aborted" {
		fields["discard"] = intent.Discard
		fields["cancelled"] = intent.Cancelled
		fields["cancelledPendingGates"] = intent.CancelledPendingGates
	}
	events := make([]AcceptedFinalPublicationEvent, 0, len(eventDrafts))
	manifest := make([]map[string]any, 0, len(eventDrafts))
	for _, candidate := range eventDrafts {
		event, err := buildAcceptedFinalPublicationEvent(record, candidate.slot, candidate.draft)
		if err != nil {
			return AcceptedFinalPublicationPlan{}, err
		}
		events = append(events, event)
		manifest = append(manifest, map[string]any{"slot": event.Slot, "eventId": event.EventID, "payloadDigest": event.PayloadDigest})
	}
	manifestBody, _ := json.Marshal(manifest)
	return AcceptedFinalPublicationPlan{
		Completion: completion, TurnItems: items, TurnFields: fields, Events: events,
		EventManifestDigest: domainsecurity.SHA256Hex(manifestBody),
	}, nil
}

func AcceptedFinalPublicationPayloadDigest(event map[string]any) string {
	canonical := contracts.CloneMap(event)
	delete(canonical, "seq")
	delete(canonical, "publicationPayloadDigest")
	body, err := json.Marshal(canonical)
	if err != nil {
		return ""
	}
	return domainsecurity.SHA256Hex(body)
}

func buildAcceptedFinalPublicationEvent(record domainevidence.AcceptedFinalRecord, slot string, draft map[string]any) (AcceptedFinalPublicationEvent, error) {
	slot = strings.TrimSpace(slot)
	if slot == "" || draft == nil {
		return AcceptedFinalPublicationEvent{}, errors.New("accepted final publication event is invalid")
	}
	event := contracts.CloneMap(draft)
	eventID := domainsecurity.SHA256Hex([]byte(acceptedFinalPublicationEventDomain + record.RecordDigest + "\x00" + slot))
	event["publicationCommitId"] = record.RecordDigest
	event["publicationEventId"] = eventID
	event["publicationSlot"] = slot
	event["acceptedFinalDigest"] = record.RecordDigest
	event["timestamp"] = record.AcceptedAt
	payloadDigest := AcceptedFinalPublicationPayloadDigest(event)
	if payloadDigest == "" {
		return AcceptedFinalPublicationEvent{}, errors.New("accepted final publication payload is invalid")
	}
	event["publicationPayloadDigest"] = payloadDigest
	if domainevent.ValidatePublicRecord(event) != nil {
		return AcceptedFinalPublicationEvent{}, errors.New("accepted final publication event is not public-safe")
	}
	return AcceptedFinalPublicationEvent{Slot: slot, EventID: eventID, PayloadDigest: payloadDigest, Draft: event}, nil
}

func acceptedFinalTerminalEvent(intent domainevidence.TerminalPublicationIntent, record domainevidence.AcceptedFinalRecord, terminalItem map[string]any) map[string]any {
	event := map[string]any{
		"kind": "turn_" + intent.TerminalStatus, "threadId": record.ThreadID, "turnId": record.TurnID,
		"status": intent.TerminalStatus, "acceptedFinalDigest": record.RecordDigest, "terminalReason": record.TerminalReason,
	}
	if intent.TerminalStatus == "failed" {
		event["error"] = intent.TerminalMessage
		event["message"] = intent.TerminalMessage
	}
	if intent.TerminalStatus == "aborted" {
		event["discard"] = intent.Discard
		event["cancelled"] = intent.Cancelled
		event["cancelledPendingGates"] = intent.CancelledPendingGates
	}
	if intent.TerminalCode != "" {
		event["code"] = intent.TerminalCode
	}
	if terminalItem != nil {
		event["itemId"] = terminalItem["id"]
	}
	return event
}

func acceptedFinalTerminalErrorItem(intent domainevidence.TerminalPublicationIntent, record domainevidence.AcceptedFinalRecord) map[string]any {
	if intent.TerminalStatus == "completed" && record.TerminalReason != "approval_denied" && record.TerminalReason != "input_cancelled" {
		return nil
	}
	if intent.TerminalMessage == "" || intent.TerminalCode == "" {
		return nil
	}
	severity := intent.TerminalSeverity
	if severity == "" {
		severity = "warning"
	}
	return map[string]any{
		"id": "item_" + record.TurnID + "_case_terminal", "turnId": record.TurnID,
		"threadId": record.ThreadID, "role": "system", "status": intent.TerminalStatus, "kind": "error",
		"createdAt": record.AcceptedAt, "finishedAt": record.AcceptedAt, "code": intent.TerminalCode,
		"message": intent.TerminalMessage, "severity": severity, "acceptedFinalDigest": record.RecordDigest,
	}
}
