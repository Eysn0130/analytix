package turnterminal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	GeneralTerminalPublicationV1SchemaVersion = "general-terminal-publication.v1"
	GeneralTerminalPublicationV1Purpose       = "analytix.general-terminal-publication/v1"
	GeneralTerminalAuthorityKindCASV1         = "general_terminal_cas"
)

var (
	generalTerminalCommitSeedDomain = []byte("analytix/general-terminal-publication-seed/v1\x00")
	generalTerminalCommitDomain     = []byte("analytix/general-terminal-publication/v1\x00")
	generalTerminalEventDomain      = []byte("analytix/general-terminal-event/v1\x00")
)

var generalTerminalPublicationMarkerFields = []string{
	"generalTerminalCommitId",
	"generalTerminalEventId",
	"generalTerminalSlot",
	"generalTerminalPayloadDigest",
	"generalTerminalAuthorityKind",
	"generalTerminalAuthorityDigest",
}

var generalTerminalStoredUsageFieldsV1 = map[string]bool{
	"kind": true, "threadId": true, "turnId": true, "model": true,
	"usage": true, "cacheDiagnostics": true, "usageSource": true,
	"childRunId": true, "timestamp": true, "usageFinalStatus": true,
}

type GeneralTerminalPublicationEventV1 struct {
	Slot          string `json:"slot"`
	EventID       string `json:"eventId"`
	PayloadDigest string `json:"payloadDigest"`
}

// GeneralTerminalPublicationCommitV1 is an internal durability outbox. It is
// not evidence, a citation, an AcceptedFinal, or case-fact authority. The
// terminal item body remains solely in the canonical turn item; the outbox
// stores only its identity plus host-authored usage and lifecycle projections
// needed to rebuild one deterministic terminal event bundle after a crash.
type GeneralTerminalPublicationCommitV1 struct {
	SchemaVersion     string                              `json:"schemaVersion"`
	Purpose           string                              `json:"purpose"`
	CommitID          string                              `json:"commitId"`
	ThreadID          string                              `json:"threadId"`
	TurnID            string                              `json:"turnId"`
	ContextDigest     string                              `json:"contextDigest"`
	ContextEpoch      uint64                              `json:"contextEpoch"`
	DatasetSnapshotID string                              `json:"datasetSnapshotId"`
	TerminalStatus    string                              `json:"terminalStatus"`
	TerminalReason    string                              `json:"terminalReason"`
	AuthorityKind     string                              `json:"authorityKind"`
	AuthorityDigest   string                              `json:"authorityDigest"`
	CommittedAt       string                              `json:"committedAt"`
	TerminalItemID    string                              `json:"terminalItemId"`
	UsageEvent        map[string]any                      `json:"usageEvent"`
	TerminalEvent     map[string]any                      `json:"terminalEvent"`
	Events            []GeneralTerminalPublicationEventV1 `json:"events"`
	ManifestDigest    string                              `json:"manifestDigest"`
	CommitDigest      string                              `json:"commitDigest"`
}

// GeneralTerminalPublicationDraftV1 is accepted only while creating a
// commit. Draft bodies are hashed into the manifest and are never retained in
// the durable outbox.
type GeneralTerminalPublicationDraftV1 struct {
	Slot  string         `json:"slot"`
	Draft map[string]any `json:"draft"`
}

func NewGeneralTerminalPublicationCommitV1(
	context domainsecurity.TurnSecurityContext,
	binding GeneralTerminalCASBindingV1,
	committedAt string,
	drafts []GeneralTerminalPublicationDraftV1,
) (GeneralTerminalPublicationCommitV1, error) {
	committedAt = strings.TrimSpace(committedAt)
	if domainsecurity.ValidateTurnSecurityContextForExecution(context) != nil ||
		!domainsecurity.TurnSecurityContextIsGeneral(context) ||
		ValidateGeneralTerminalCASBindingForOutcomeV1(
			binding, context, bindingTextForDrafts(drafts), binding.TerminalReason, binding.TerminalStatus,
		) != nil ||
		validateGeneralTerminalPublicationDrafts(context, binding, committedAt, drafts) != nil {
		return GeneralTerminalPublicationCommitV1{}, errors.New("general terminal publication input is invalid")
	}

	terminalItemID := ""
	usageIndex := 0
	if len(drafts) == 3 {
		item, _ := drafts[0].Draft["item"].(map[string]any)
		terminalItemID = generalTerminalString(item, "id")
		usageIndex = 1
	}
	commit := GeneralTerminalPublicationCommitV1{
		SchemaVersion:     GeneralTerminalPublicationV1SchemaVersion,
		Purpose:           GeneralTerminalPublicationV1Purpose,
		ThreadID:          context.ThreadID,
		TurnID:            context.TurnID,
		ContextDigest:     context.ContextDigest,
		ContextEpoch:      context.ContextEpoch,
		DatasetSnapshotID: context.DatasetSnapshotID,
		TerminalStatus:    binding.TerminalStatus,
		TerminalReason:    binding.TerminalReason,
		AuthorityKind:     GeneralTerminalAuthorityKindCASV1,
		AuthorityDigest:   binding.BindingDigest,
		CommittedAt:       committedAt,
		TerminalItemID:    terminalItemID,
		UsageEvent:        cloneGeneralTerminalMap(drafts[usageIndex].Draft),
		TerminalEvent:     cloneGeneralTerminalMap(drafts[usageIndex+1].Draft),
	}
	commit.CommitID = generalTerminalPublicationCommitIDV1(commit)

	commit.Events = make([]GeneralTerminalPublicationEventV1, 0, len(drafts))
	manifest := make([]map[string]any, 0, len(drafts))
	for _, candidate := range drafts {
		draft := cloneGeneralTerminalMap(candidate.Draft)
		eventID := generalTerminalPublicationEventIDV1(commit.CommitID, candidate.Slot)
		addGeneralTerminalPublicationMarkers(draft, commit, candidate.Slot, eventID)
		payloadDigest := GeneralTerminalPublicationPayloadDigestV1(draft)
		if payloadDigest == "" {
			return GeneralTerminalPublicationCommitV1{}, errors.New("general terminal publication payload is invalid")
		}
		commit.Events = append(commit.Events, GeneralTerminalPublicationEventV1{
			Slot: candidate.Slot, EventID: eventID, PayloadDigest: payloadDigest,
		})
		manifest = append(manifest, map[string]any{
			"slot": candidate.Slot, "eventId": eventID, "payloadDigest": payloadDigest,
		})
	}
	manifestBody, err := json.Marshal(manifest)
	if err != nil {
		return GeneralTerminalPublicationCommitV1{}, err
	}
	commit.ManifestDigest = domainsecurity.SHA256Hex(manifestBody)
	commit.CommitDigest = generalTerminalPublicationCommitDigestV1(commit)
	if err := ValidateGeneralTerminalPublicationCommitV1(commit); err != nil {
		return GeneralTerminalPublicationCommitV1{}, err
	}
	return cloneGeneralTerminalPublicationCommitV1(commit), nil
}

func ValidateGeneralTerminalPublicationCommitV1(commit GeneralTerminalPublicationCommitV1) error {
	if commit.SchemaVersion != GeneralTerminalPublicationV1SchemaVersion ||
		commit.Purpose != GeneralTerminalPublicationV1Purpose ||
		!domainsecurity.IsSHA256Hex(commit.CommitID) || commit.CommitID != generalTerminalPublicationCommitIDV1(commit) ||
		!generalTerminalExactNonEmpty(commit.ThreadID) || !generalTerminalExactNonEmpty(commit.TurnID) ||
		!domainsecurity.IsSHA256Hex(commit.ContextDigest) || commit.ContextEpoch == 0 ||
		!generalTerminalExactNonEmpty(commit.DatasetSnapshotID) ||
		!validGeneralTerminalOutcomeV1(commit.TerminalReason, commit.TerminalStatus) ||
		commit.AuthorityKind != GeneralTerminalAuthorityKindCASV1 || !domainsecurity.IsSHA256Hex(commit.AuthorityDigest) ||
		!generalTerminalTimestampIsCanonical(commit.CommittedAt) ||
		!validCommittedGeneralTerminalUsageEvent(commit) || !validCommittedGeneralTerminalLifecycleEvent(commit) ||
		!domainsecurity.IsSHA256Hex(commit.ManifestDigest) ||
		!domainsecurity.IsSHA256Hex(commit.CommitDigest) || commit.CommitDigest != generalTerminalPublicationCommitDigestV1(commit) ||
		!validGeneralTerminalEvents(commit) {
		return errors.New("general terminal publication commit is invalid")
	}
	return nil
}

func ParseGeneralTerminalPublicationCommitV1(value any) (GeneralTerminalPublicationCommitV1, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return GeneralTerminalPublicationCommitV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var commit GeneralTerminalPublicationCommitV1
	if err := decoder.Decode(&commit); err != nil {
		return GeneralTerminalPublicationCommitV1{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return GeneralTerminalPublicationCommitV1{}, errors.New("general terminal publication commit contains trailing JSON")
	}
	if err := ValidateGeneralTerminalPublicationCommitV1(commit); err != nil {
		return GeneralTerminalPublicationCommitV1{}, err
	}
	return cloneGeneralTerminalPublicationCommitV1(commit), nil
}

func GeneralTerminalPublicationCommitV1Map(commit GeneralTerminalPublicationCommitV1) map[string]any {
	body, _ := json.Marshal(commit)
	value := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	_ = decoder.Decode(&value)
	return value
}

func GeneralTerminalPublicationPayloadDigestV1(event map[string]any) string {
	canonical := cloneGeneralTerminalMap(event)
	delete(canonical, "seq")
	delete(canonical, "generalTerminalPayloadDigest")
	body, err := json.Marshal(canonical)
	if err != nil {
		return ""
	}
	return domainsecurity.SHA256Hex(body)
}

func GeneralTerminalPublicationEventDraftV1(
	commit GeneralTerminalPublicationCommitV1,
	slot string,
	baseDraft map[string]any,
) (map[string]any, error) {
	if ValidateGeneralTerminalPublicationCommitV1(commit) != nil || baseDraft == nil ||
		hasGeneralTerminalPublicationMarker(baseDraft) ||
		domainevent.ContainsAcceptedFinalPublicationAuthority(baseDraft) || baseDraft["seq"] != nil {
		return nil, errors.New("general terminal publication event input is invalid")
	}
	slot = strings.TrimSpace(slot)
	var expected *GeneralTerminalPublicationEventV1
	for index := range commit.Events {
		if commit.Events[index].Slot == slot {
			expected = &commit.Events[index]
			break
		}
	}
	if expected == nil {
		return nil, errors.New("general terminal publication slot is not committed")
	}
	draft := cloneGeneralTerminalMap(baseDraft)
	addGeneralTerminalPublicationMarkers(draft, commit, expected.Slot, expected.EventID)
	payloadDigest := GeneralTerminalPublicationPayloadDigestV1(draft)
	if payloadDigest != expected.PayloadDigest {
		return nil, errors.New("general terminal publication payload is not committed")
	}
	draft["generalTerminalPayloadDigest"] = payloadDigest
	return draft, nil
}

func GeneralTerminalPublicationMarkerPresentV1(value map[string]any) bool {
	return hasGeneralTerminalPublicationMarker(value)
}

func validateGeneralTerminalPublicationDrafts(
	context domainsecurity.TurnSecurityContext,
	binding GeneralTerminalCASBindingV1,
	committedAt string,
	drafts []GeneralTerminalPublicationDraftV1,
) error {
	if !generalTerminalTimestampIsCanonical(committedAt) || !validGeneralTerminalDraftSlots(drafts) {
		return errors.New("general terminal publication draft manifest is invalid")
	}
	for _, candidate := range drafts {
		if candidate.Draft == nil || candidate.Draft["seq"] != nil || hasGeneralTerminalPublicationMarker(candidate.Draft) ||
			domainevent.ContainsAcceptedFinalPublicationAuthority(candidate.Draft) ||
			generalTerminalString(candidate.Draft, "threadId") != context.ThreadID ||
			generalTerminalString(candidate.Draft, "turnId") != context.TurnID ||
			generalTerminalString(candidate.Draft, "timestamp") != committedAt {
			return errors.New("general terminal publication draft identity is invalid")
		}
		switch candidate.Slot {
		case "terminal-item":
			item, _ := candidate.Draft["item"].(map[string]any)
			itemBinding, err := ParseGeneralTerminalCASBindingV1(item["generalTerminalCASBinding"])
			text, textOK := GeneralTerminalItemTextV1(item)
			if generalTerminalString(candidate.Draft, "kind") != "item_completed" ||
				item == nil || !textOK || !validGeneralTerminalItemShapeV1(item, binding.TerminalStatus) ||
				generalTerminalString(item, "threadId") != context.ThreadID || generalTerminalString(item, "turnId") != context.TurnID ||
				generalTerminalString(item, "id") == "" || generalTerminalString(candidate.Draft, "itemId") != generalTerminalString(item, "id") ||
				generalTerminalString(item, "createdAt") == "" || generalTerminalString(item, "finishedAt") != committedAt ||
				err != nil || itemBinding.BindingDigest != binding.BindingDigest ||
				ValidateGeneralTerminalCASBindingForOutcomeV1(
					binding, context, text, binding.TerminalReason, binding.TerminalStatus,
				) != nil {
				return errors.New("general terminal item draft is invalid")
			}
		case "usage":
			if generalTerminalString(candidate.Draft, "kind") != "usage" {
				return errors.New("general terminal usage draft is invalid")
			}
		case "terminal":
			expectedKind, _ := GeneralTerminalLifecycleEventKindV1(binding.TerminalStatus)
			if generalTerminalString(candidate.Draft, "kind") != expectedKind ||
				generalTerminalString(candidate.Draft, "status") != binding.TerminalStatus ||
				generalTerminalString(candidate.Draft, "terminalReason") != binding.TerminalReason ||
				generalTerminalString(candidate.Draft, "generalTerminalCASBindingDigest") != binding.BindingDigest ||
				!validGeneralTerminalInterruptMetadataV1(candidate.Draft, binding.TerminalReason) {
				return errors.New("general terminal lifecycle draft is invalid")
			}
		default:
			return errors.New("general terminal publication slot is unknown")
		}
	}
	return nil
}

func validGeneralTerminalDraftSlots(drafts []GeneralTerminalPublicationDraftV1) bool {
	if len(drafts) != 2 && len(drafts) != 3 {
		return false
	}
	expected := []string{"usage", "terminal"}
	if len(drafts) == 3 {
		expected = []string{"terminal-item", "usage", "terminal"}
	}
	for index, candidate := range drafts {
		if candidate.Slot != expected[index] || candidate.Draft == nil {
			return false
		}
	}
	return true
}

func validGeneralTerminalEvents(commit GeneralTerminalPublicationCommitV1) bool {
	expected := []string{"usage", "terminal"}
	if strings.TrimSpace(commit.TerminalItemID) != "" {
		expected = []string{"terminal-item", "usage", "terminal"}
	}
	if len(commit.Events) != len(expected) {
		return false
	}
	manifest := make([]map[string]any, 0, len(commit.Events))
	seenEventIDs := map[string]bool{}
	for index, event := range commit.Events {
		if event.Slot != expected[index] || seenEventIDs[event.EventID] ||
			!domainsecurity.IsSHA256Hex(event.EventID) || !domainsecurity.IsSHA256Hex(event.PayloadDigest) ||
			event.EventID != generalTerminalPublicationEventIDV1(commit.CommitID, event.Slot) {
			return false
		}
		seenEventIDs[event.EventID] = true
		manifest = append(manifest, map[string]any{
			"slot": event.Slot, "eventId": event.EventID, "payloadDigest": event.PayloadDigest,
		})
	}
	manifestBody, err := json.Marshal(manifest)
	return err == nil && domainsecurity.SHA256Hex(manifestBody) == commit.ManifestDigest
}

func validCommittedGeneralTerminalUsageEvent(commit GeneralTerminalPublicationCommitV1) bool {
	usage := commit.UsageEvent
	if usage == nil || usage["seq"] != nil || hasGeneralTerminalPublicationMarker(usage) ||
		domainevent.ContainsAcceptedFinalPublicationAuthority(usage) {
		return false
	}
	for key := range usage {
		if !generalTerminalStoredUsageFieldsV1[key] {
			return false
		}
	}
	return generalTerminalString(usage, "kind") == "usage" &&
		generalTerminalString(usage, "threadId") == commit.ThreadID &&
		generalTerminalString(usage, "turnId") == commit.TurnID &&
		generalTerminalString(usage, "timestamp") == commit.CommittedAt &&
		(generalTerminalString(usage, "usageFinalStatus") == "" ||
			generalTerminalString(usage, "usageFinalStatus") == commit.TerminalStatus)
}

func validCommittedGeneralTerminalLifecycleEvent(commit GeneralTerminalPublicationCommitV1) bool {
	event := commit.TerminalEvent
	expectedKind, ok := GeneralTerminalLifecycleEventKindV1(commit.TerminalStatus)
	if !ok || event == nil || event["seq"] != nil || hasGeneralTerminalPublicationMarker(event) ||
		domainevent.ContainsAcceptedFinalPublicationAuthority(event) ||
		domainevent.ValidatePublicRecord(event) != nil {
		return false
	}
	return generalTerminalString(event, "kind") == expectedKind &&
		generalTerminalString(event, "threadId") == commit.ThreadID &&
		generalTerminalString(event, "turnId") == commit.TurnID &&
		generalTerminalString(event, "status") == commit.TerminalStatus &&
		generalTerminalString(event, "timestamp") == commit.CommittedAt &&
		generalTerminalString(event, "terminalReason") == commit.TerminalReason &&
		generalTerminalString(event, "generalTerminalCASBindingDigest") == commit.AuthorityDigest &&
		validGeneralTerminalInterruptMetadataV1(event, commit.TerminalReason)
}

func validGeneralTerminalInterruptMetadataV1(event map[string]any, terminalReason string) bool {
	_, hasDiscard := event["discard"]
	_, hasCancelled := event["cancelled"]
	_, hasPendingGates := event["cancelledPendingGates"]
	hasAny := hasDiscard || hasCancelled || hasPendingGates
	if strings.TrimSpace(terminalReason) != "cancel" {
		return !hasAny
	}
	if !hasDiscard || !hasCancelled || !hasPendingGates {
		return false
	}
	if _, ok := event["discard"].(bool); !ok {
		return false
	}
	cancelled, ok := event["cancelled"].(bool)
	if !ok {
		return false
	}
	count, ok := exactGeneralTerminalPendingGateCountV1(event["cancelledPendingGates"])
	return ok && (count == 0 || cancelled)
}

func exactGeneralTerminalPendingGateCountV1(value any) (int64, bool) {
	const maxSafeInteger = int64(1<<53 - 1)
	var count int64
	switch typed := value.(type) {
	case int:
		count = int64(typed)
		if int(count) != typed {
			return 0, false
		}
	case int64:
		count = typed
	case json.Number:
		parsed, err := strconv.ParseInt(string(typed), 10, 64)
		if err != nil {
			return 0, false
		}
		count = parsed
	case float64:
		if math.Signbit(typed) || math.IsNaN(typed) || math.IsInf(typed, 0) || typed != math.Trunc(typed) || typed > float64(maxSafeInteger) {
			return 0, false
		}
		count = int64(typed)
	default:
		return 0, false
	}
	if count < 0 || count > maxSafeInteger {
		return 0, false
	}
	return count, true
}

func bindingTextForDrafts(drafts []GeneralTerminalPublicationDraftV1) string {
	if len(drafts) == 3 {
		item, _ := drafts[0].Draft["item"].(map[string]any)
		text, _ := GeneralTerminalItemTextV1(item)
		return text
	}
	return ""
}

func generalTerminalPublicationCommitIDV1(commit GeneralTerminalPublicationCommitV1) string {
	seed := struct {
		ThreadID          string         `json:"threadId"`
		TurnID            string         `json:"turnId"`
		ContextDigest     string         `json:"contextDigest"`
		ContextEpoch      uint64         `json:"contextEpoch"`
		DatasetSnapshotID string         `json:"datasetSnapshotId"`
		AuthorityDigest   string         `json:"authorityDigest"`
		CommittedAt       string         `json:"committedAt"`
		TerminalStatus    string         `json:"terminalStatus"`
		TerminalReason    string         `json:"terminalReason"`
		TerminalItemID    string         `json:"terminalItemId"`
		UsageEvent        map[string]any `json:"usageEvent"`
		TerminalEvent     map[string]any `json:"terminalEvent"`
	}{
		ThreadID: commit.ThreadID, TurnID: commit.TurnID, ContextDigest: commit.ContextDigest,
		ContextEpoch: commit.ContextEpoch, DatasetSnapshotID: commit.DatasetSnapshotID,
		AuthorityDigest: commit.AuthorityDigest, CommittedAt: commit.CommittedAt,
		TerminalStatus: commit.TerminalStatus, TerminalReason: commit.TerminalReason,
		TerminalItemID: commit.TerminalItemID, UsageEvent: commit.UsageEvent, TerminalEvent: commit.TerminalEvent,
	}
	body, _ := json.Marshal(seed)
	return domainSeparatedSHA256Hex(generalTerminalCommitSeedDomain, body)
}

func GeneralTerminalItemTextV1(item map[string]any) (string, bool) {
	if item == nil {
		return "", false
	}
	switch generalTerminalString(item, "kind") {
	case "assistant_text":
		text, ok := item["text"].(string)
		return text, ok
	case "error":
		message, ok := item["message"].(string)
		return message, ok && strings.TrimSpace(message) != ""
	default:
		return "", false
	}
}

func GeneralTerminalLifecycleEventKindV1(status string) (string, bool) {
	switch strings.TrimSpace(status) {
	case "completed":
		return "turn_completed", true
	case "failed":
		return "turn_failed", true
	case "aborted":
		return "turn_aborted", true
	default:
		return "", false
	}
}

func validGeneralTerminalItemShapeV1(item map[string]any, terminalStatus string) bool {
	switch generalTerminalString(item, "kind") {
	case "assistant_text":
		baseFields := map[string]bool{
			"id": true, "turnId": true, "threadId": true, "role": true, "status": true,
			"createdAt": true, "finishedAt": true, "kind": true, "text": true,
			"generalTerminalCASBinding": true,
		}
		text, textOK := item["text"].(string)
		if !textOK || terminalStatus != "completed" || generalTerminalString(item, "role") != "assistant" ||
			generalTerminalString(item, "status") != "completed" {
			return false
		}
		if value, present := item["ordinaryResult"]; present {
			slot, err := domainordinaryresult.ParseResultSlotV1(value)
			baseFields["ordinaryResult"] = true
			return err == nil && text == slot.Text && generalTerminalExactFieldSetV1(item, baseFields)
		}
		return text == domainevent.GeneralTerminalCompletedBoundaryTextV1 &&
			generalTerminalExactFieldSetV1(item, baseFields)
	case "error":
		required := map[string]bool{
			"id": true, "turnId": true, "threadId": true, "role": true, "status": true,
			"createdAt": true, "finishedAt": true, "kind": true, "code": true,
			"message": true, "severity": true, "generalTerminalCASBinding": true,
		}
		if _, present := item["details"]; present {
			required["details"] = true
		}
		_, messageOK := item["message"].(string)
		return messageOK && terminalStatus != "completed" && generalTerminalString(item, "role") == "system" &&
			generalTerminalString(item, "status") == terminalStatus && generalTerminalString(item, "code") != "" &&
			generalTerminalString(item, "severity") != "" && generalTerminalExactFieldSetV1(item, required)
	default:
		return false
	}
}

func generalTerminalExactFieldSetV1(value map[string]any, fields map[string]bool) bool {
	if value == nil || len(value) != len(fields) {
		return false
	}
	for field := range value {
		if !fields[field] {
			return false
		}
	}
	return true
}

func generalTerminalPublicationCommitDigestV1(commit GeneralTerminalPublicationCommitV1) string {
	commit.CommitDigest = ""
	body, _ := json.Marshal(commit)
	return domainSeparatedSHA256Hex(generalTerminalCommitDomain, body)
}

func generalTerminalPublicationEventIDV1(commitID, slot string) string {
	body := []byte(strings.TrimSpace(commitID) + "\x00" + strings.TrimSpace(slot))
	return domainSeparatedSHA256Hex(generalTerminalEventDomain, body)
}

func addGeneralTerminalPublicationMarkers(draft map[string]any, commit GeneralTerminalPublicationCommitV1, slot, eventID string) {
	draft["generalTerminalCommitId"] = commit.CommitID
	draft["generalTerminalEventId"] = eventID
	draft["generalTerminalSlot"] = slot
	draft["generalTerminalAuthorityKind"] = commit.AuthorityKind
	draft["generalTerminalAuthorityDigest"] = commit.AuthorityDigest
}

func hasGeneralTerminalPublicationMarker(value map[string]any) bool {
	for _, field := range generalTerminalPublicationMarkerFields {
		if _, ok := value[field]; ok {
			return true
		}
	}
	return false
}

func generalTerminalTimestampIsCanonical(value string) bool {
	if value == "" || value != strings.TrimSpace(value) {
		return false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && parsed.UTC().Format(time.RFC3339Nano) == value
}

func generalTerminalExactNonEmpty(value string) bool {
	return value != "" && value == strings.TrimSpace(value)
}

func domainSeparatedSHA256Hex(domain, body []byte) string {
	hash := sha256.New()
	_, _ = hash.Write(domain)
	_, _ = hash.Write(body)
	return hex.EncodeToString(hash.Sum(nil))
}

func cloneGeneralTerminalMap(value map[string]any) map[string]any {
	body, _ := json.Marshal(value)
	out := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	_ = decoder.Decode(&out)
	return out
}

func cloneGeneralTerminalPublicationCommitV1(commit GeneralTerminalPublicationCommitV1) GeneralTerminalPublicationCommitV1 {
	body, _ := json.Marshal(commit)
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	out := GeneralTerminalPublicationCommitV1{}
	_ = decoder.Decode(&out)
	return out
}

func generalTerminalString(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}
