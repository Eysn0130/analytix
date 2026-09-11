package event

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
	domainterminaltelemetry "analytix.local/runtime-go/internal/domain/terminaltelemetry"
)

const (
	GeneralTerminalDeliveryBatchVersion = 1
	GeneralTerminalDeliveryBatchPurpose = "analytix.general-terminal-delivery-batch/v1"
	GeneralTerminalDeliveryBatchKind    = "general_terminal_batch"
	GeneralTerminalTransportAuthority   = "host_batch_digest_v1"
	GeneralTerminalCASAuthorityKind     = "general_terminal_cas"
	generalTerminalMaxSafeIntegerV1     = int64(1<<53 - 1)
	// GeneralTerminalCompletedBoundaryTextV1 is the legacy fallback for a
	// completed turn that has no typed ordinary result.
	GeneralTerminalCompletedBoundaryTextV1 = "本轮模型自由文本尚无宿主可验证的发布权限，草稿未进入消息、历史或导出。请使用受验证的工具或结构化成果物完成任务。"
)

var (
	generalTerminalDeliveryBatchDigestDomain      = []byte("analytix/general-terminal-delivery-batch/v1\x00")
	generalTerminalDeliveryEventIDDomain          = []byte("analytix/general-terminal-event/v1\x00")
	generalTerminalDeliveryManifestDigestDomain   = []byte("analytix/general-terminal-delivery-manifest/v1\x00")
	generalTerminalProjectedEventsDigestDomain    = []byte("analytix/general-terminal-projected-events/v1\x00")
	generalTerminalDeliverySlots                  = []string{"usage", "terminal"}
	generalTerminalDeliveryWithItemSlots          = []string{"terminal-item", "usage", "terminal"}
	generalTerminalDeliveryTimestampPatternV1     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-](\d{2}):(\d{2}))$`)
	generalTerminalPublicIDPatternV1              = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,255}$`)
	generalTerminalDeliveryPublicationMarkerNames = []string{
		"generalTerminalCommitId",
		"generalTerminalEventId",
		"generalTerminalSlot",
		"generalTerminalPayloadDigest",
		"generalTerminalAuthorityKind",
		"generalTerminalAuthorityDigest",
	}
)

// GeneralTerminalDeliveryManifestEventV1 binds a public projection to the
// exact durable CAS/outbox event it was derived from. These values establish
// transport integrity only; they do not establish evidence or case authority.
type GeneralTerminalDeliveryManifestEventV1 struct {
	Slot          string `json:"slot"`
	EventID       string `json:"eventId"`
	PayloadDigest string `json:"payloadDigest"`
}

// GeneralTerminalDeliveryBatchV1 is an indivisible public transport unit for
// one ordinary, non-evidence terminal CAS/outbox commit. The nested events are
// already-sanitized public projections. The batch cannot issue evidence,
// citations, accepted-final authority, or permission to answer case facts.
type GeneralTerminalDeliveryBatchV1 struct {
	SchemaVersion                  int                                      `json:"schemaVersion"`
	Purpose                        string                                   `json:"purpose"`
	Kind                           string                                   `json:"kind"`
	BatchDigest                    string                                   `json:"batchDigest"`
	ThreadID                       string                                   `json:"threadId"`
	TurnID                         string                                   `json:"turnId"`
	Seq                            int                                      `json:"seq"`
	FirstSeq                       int                                      `json:"firstSeq"`
	LastSeq                        int                                      `json:"lastSeq"`
	Timestamp                      string                                   `json:"timestamp"`
	GeneralTerminalCommitID        string                                   `json:"generalTerminalCommitId"`
	GeneralTerminalAuthorityKind   string                                   `json:"generalTerminalAuthorityKind"`
	GeneralTerminalAuthorityDigest string                                   `json:"generalTerminalAuthorityDigest"`
	EventManifestDigest            string                                   `json:"eventManifestDigest"`
	ProjectedEventsDigest          string                                   `json:"projectedEventsDigest"`
	TransportAuthority             string                                   `json:"transportAuthority"`
	EvidenceAuthority              bool                                     `json:"evidenceAuthority"`
	CitationAuthority              bool                                     `json:"citationAuthority"`
	FactAnswerAllowed              bool                                     `json:"factAnswerAllowed"`
	Events                         []map[string]any                         `json:"events"`
	EventManifest                  []GeneralTerminalDeliveryManifestEventV1 `json:"eventManifest"`
}

// NewGeneralTerminalDeliveryBatchV1 validates the complete durable marker
// group before binding it to the exact sanitized public projections.
func NewGeneralTerminalDeliveryBatchV1(
	rawEvents []map[string]any,
	projectedEvents []map[string]any,
) (GeneralTerminalDeliveryBatchV1, error) {
	content, err := validateGeneralTerminalDeliveryEventsV1(rawEvents)
	if err != nil {
		return GeneralTerminalDeliveryBatchV1{}, err
	}
	projected := cloneAcceptedFinalDeliveryEvents(projectedEvents)
	if err := validateGeneralTerminalProjectedEventsV1(content, projected); err != nil {
		return GeneralTerminalDeliveryBatchV1{}, err
	}
	batch := GeneralTerminalDeliveryBatchV1{
		SchemaVersion:                  GeneralTerminalDeliveryBatchVersion,
		Purpose:                        GeneralTerminalDeliveryBatchPurpose,
		Kind:                           GeneralTerminalDeliveryBatchKind,
		ThreadID:                       content.threadID,
		TurnID:                         content.turnID,
		Seq:                            content.lastSeq,
		FirstSeq:                       content.firstSeq,
		LastSeq:                        content.lastSeq,
		Timestamp:                      content.timestamp,
		GeneralTerminalCommitID:        content.commitID,
		GeneralTerminalAuthorityKind:   content.authorityKind,
		GeneralTerminalAuthorityDigest: content.authorityDigest,
		EventManifestDigest:            generalTerminalDeliveryManifestDigestV1(content.manifest),
		ProjectedEventsDigest:          generalTerminalProjectedEventsDigestV1(projected),
		TransportAuthority:             GeneralTerminalTransportAuthority,
		EvidenceAuthority:              false,
		CitationAuthority:              false,
		FactAnswerAllowed:              false,
		Events:                         projected,
		EventManifest:                  append([]GeneralTerminalDeliveryManifestEventV1(nil), content.manifest...),
	}
	batch.BatchDigest = generalTerminalDeliveryBatchDigestV1(batch)
	if err := ValidateGeneralTerminalDeliveryBatchV1(batch); err != nil {
		return GeneralTerminalDeliveryBatchV1{}, err
	}
	return cloneGeneralTerminalDeliveryBatchV1(batch), nil
}

func ValidateGeneralTerminalDeliveryBatchV1(batch GeneralTerminalDeliveryBatchV1) error {
	if batch.SchemaVersion != GeneralTerminalDeliveryBatchVersion ||
		batch.Purpose != GeneralTerminalDeliveryBatchPurpose || batch.Kind != GeneralTerminalDeliveryBatchKind ||
		!domainsecurity.IsSHA256Hex(batch.BatchDigest) ||
		!generalTerminalDeliveryPublicIDV1(batch.ThreadID) || !generalTerminalDeliveryPublicIDV1(batch.TurnID) ||
		batch.Seq <= 0 || batch.FirstSeq <= 0 || batch.LastSeq < batch.FirstSeq || batch.Seq != batch.LastSeq ||
		int64(batch.Seq) > generalTerminalMaxSafeIntegerV1 || int64(batch.FirstSeq) > generalTerminalMaxSafeIntegerV1 ||
		int64(batch.LastSeq) > generalTerminalMaxSafeIntegerV1 ||
		batch.LastSeq-batch.FirstSeq+1 != len(batch.Events) ||
		!generalTerminalDeliveryCanonicalTimestamp(batch.Timestamp) ||
		!domainsecurity.IsSHA256Hex(batch.GeneralTerminalCommitID) ||
		batch.GeneralTerminalAuthorityKind != GeneralTerminalCASAuthorityKind ||
		!domainsecurity.IsSHA256Hex(batch.GeneralTerminalAuthorityDigest) ||
		!domainsecurity.IsSHA256Hex(batch.EventManifestDigest) ||
		!domainsecurity.IsSHA256Hex(batch.ProjectedEventsDigest) ||
		batch.TransportAuthority != GeneralTerminalTransportAuthority ||
		batch.EvidenceAuthority || batch.CitationAuthority || batch.FactAnswerAllowed ||
		(len(batch.Events) != 2 && len(batch.Events) != 3) || len(batch.EventManifest) != len(batch.Events) {
		return errors.New("general terminal delivery batch is incomplete")
	}
	expectedSlots := generalTerminalDeliverySlots
	if len(batch.Events) == 3 {
		expectedSlots = generalTerminalDeliveryWithItemSlots
	}
	seenEventIDs := map[string]bool{}
	for index, manifest := range batch.EventManifest {
		if manifest.Slot != expectedSlots[index] || seenEventIDs[manifest.EventID] ||
			!domainsecurity.IsSHA256Hex(manifest.EventID) ||
			manifest.EventID != GeneralTerminalDeliveryEventIDV1(batch.GeneralTerminalCommitID, manifest.Slot) ||
			!domainsecurity.IsSHA256Hex(manifest.PayloadDigest) {
			return errors.New("general terminal delivery manifest is invalid")
		}
		seenEventIDs[manifest.EventID] = true
	}
	content := generalTerminalDeliveryContentV1{
		threadID:  batch.ThreadID,
		turnID:    batch.TurnID,
		firstSeq:  batch.FirstSeq,
		lastSeq:   batch.LastSeq,
		timestamp: batch.Timestamp,
		manifest:  batch.EventManifest,
	}
	if err := validateGeneralTerminalProjectedEventsV1(content, batch.Events); err != nil ||
		generalTerminalDeliveryManifestDigestV1(batch.EventManifest) != batch.EventManifestDigest ||
		generalTerminalProjectedEventsDigestV1(batch.Events) != batch.ProjectedEventsDigest ||
		generalTerminalDeliveryBatchDigestV1(batch) != batch.BatchDigest {
		return errors.New("general terminal delivery batch integrity is invalid")
	}
	return nil
}

func ParseGeneralTerminalDeliveryBatchV1(value map[string]any) (GeneralTerminalDeliveryBatchV1, error) {
	if value == nil {
		return GeneralTerminalDeliveryBatchV1{}, errors.New("general terminal delivery batch is unavailable")
	}
	body, err := json.Marshal(value)
	if err != nil {
		return GeneralTerminalDeliveryBatchV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var batch GeneralTerminalDeliveryBatchV1
	if err := decoder.Decode(&batch); err != nil {
		return GeneralTerminalDeliveryBatchV1{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return GeneralTerminalDeliveryBatchV1{}, errors.New("general terminal delivery batch contains trailing JSON")
	}
	if err := ValidateGeneralTerminalDeliveryBatchV1(batch); err != nil {
		return GeneralTerminalDeliveryBatchV1{}, err
	}
	canonical := GeneralTerminalDeliveryBatchV1Map(batch)
	if canonical == nil || !reflect.DeepEqual(canonical, value) {
		return GeneralTerminalDeliveryBatchV1{}, errors.New("general terminal delivery batch is not canonical")
	}
	return batch, nil
}

func GeneralTerminalDeliveryBatchV1Map(batch GeneralTerminalDeliveryBatchV1) map[string]any {
	if ValidateGeneralTerminalDeliveryBatchV1(batch) != nil {
		return nil
	}
	body, _ := json.Marshal(batch)
	out := map[string]any{}
	if json.Unmarshal(body, &out) != nil {
		return nil
	}
	return out
}

// GeneralTerminalReplayEventsAfter rewinds a cursor that lands inside a
// complete ordinary terminal outbox group. A malformed, split, duplicate, or
// incomplete marker group fails closed instead of returning a prefix/suffix.
func GeneralTerminalReplayEventsAfter(events []map[string]any, afterSeq int) ([]map[string]any, error) {
	effectiveAfter := afterSeq
	seenCommits := map[string]bool{}
	for start := 0; start < len(events); {
		commitID := strings.TrimSpace(contracts.StringField(events[start], "generalTerminalCommitId"))
		if commitID == "" {
			if GeneralTerminalDeliveryMarkerPresentV1(events[start]) {
				return nil, errors.New("general terminal delivery marker group is malformed")
			}
			start++
			continue
		}
		end := start + 1
		for end < len(events) && strings.TrimSpace(contracts.StringField(events[end], "generalTerminalCommitId")) == commitID {
			end++
		}
		if seenCommits[commitID] {
			return nil, errors.New("general terminal delivery commit is split or duplicated")
		}
		seenCommits[commitID] = true
		content, err := validateGeneralTerminalDeliveryEventsV1(events[start:end])
		if err != nil {
			return nil, err
		}
		if content.firstSeq <= afterSeq && afterSeq < content.lastSeq {
			effectiveAfter = content.firstSeq - 1
		}
		start = end
	}
	filtered := make([]map[string]any, 0, len(events))
	for _, event := range events {
		seq, ok := contracts.NumericSeq(event["seq"])
		if ok && seq > effectiveAfter {
			filtered = append(filtered, contracts.CloneMap(event))
		}
	}
	return filtered, nil
}

// AtomicTerminalReplayEventsAfter applies one cursor after considering both
// accepted-final and ordinary-general atomic manifests. Applying the two
// rewind helpers sequentially would let the second filter discard the first
// manifest's rewound prefix.
func AtomicTerminalReplayEventsAfter(events []map[string]any, afterSeq int) ([]map[string]any, error) {
	effectiveAfter := afterSeq
	seenAccepted := map[string]bool{}
	seenGeneral := map[string]bool{}
	for start := 0; start < len(events); {
		acceptedCommitID := strings.TrimSpace(contracts.StringField(events[start], "publicationCommitId"))
		if acceptedCommitID != "" {
			end := start + 1
			for end < len(events) && strings.TrimSpace(contracts.StringField(events[end], "publicationCommitId")) == acceptedCommitID {
				end++
			}
			for _, event := range events[start:end] {
				if GeneralTerminalDeliveryMarkerPresentV1(event) || generalTerminalDeliveryNestedMarkerPresentV1(event, true) {
					return nil, errors.New("accepted final delivery is mixed with general terminal authority")
				}
			}
			if seenAccepted[acceptedCommitID] {
				return nil, errors.New("accepted final delivery commit is split or duplicated")
			}
			seenAccepted[acceptedCommitID] = true
			batch, err := newAcceptedFinalDeliveryBatchContentV2(events[start:end])
			if err != nil {
				return nil, err
			}
			if batch.FirstSeq <= afterSeq && afterSeq < batch.LastSeq {
				effectiveAfter = min(effectiveAfter, batch.FirstSeq-1)
			}
			start = end
			continue
		}

		generalCommitID := strings.TrimSpace(contracts.StringField(events[start], "generalTerminalCommitId"))
		if generalCommitID != "" {
			end := start + 1
			for end < len(events) && strings.TrimSpace(contracts.StringField(events[end], "generalTerminalCommitId")) == generalCommitID {
				end++
			}
			if seenGeneral[generalCommitID] {
				return nil, errors.New("general terminal delivery commit is split or duplicated")
			}
			seenGeneral[generalCommitID] = true
			content, err := validateGeneralTerminalDeliveryEventsV1(events[start:end])
			if err != nil {
				return nil, err
			}
			if content.firstSeq <= afterSeq && afterSeq < content.lastSeq {
				effectiveAfter = min(effectiveAfter, content.firstSeq-1)
			}
			start = end
			continue
		}
		if GeneralTerminalDeliveryMarkerPresentV1(events[start]) {
			return nil, errors.New("general terminal delivery marker group is malformed")
		}
		start++
	}
	filtered := make([]map[string]any, 0, len(events))
	for _, event := range events {
		seq, ok := contracts.NumericSeq(event["seq"])
		if ok && seq > effectiveAfter {
			filtered = append(filtered, contracts.CloneMap(event))
		}
	}
	return filtered, nil
}

func ValidateGeneralTerminalDeliveryEventsV1(events []map[string]any) error {
	_, err := validateGeneralTerminalDeliveryEventsV1(events)
	return err
}

func GeneralTerminalDeliveryMarkerPresentV1(event map[string]any) bool {
	for _, name := range generalTerminalDeliveryPublicationMarkerNames {
		if _, present := event[name]; present {
			return true
		}
	}
	return false
}

func GeneralTerminalDeliveryExpectedEventCountV1(first map[string]any) (int, bool) {
	switch strings.TrimSpace(contracts.StringField(first, "generalTerminalSlot")) {
	case "usage":
		return 2, true
	case "terminal-item":
		return 3, true
	default:
		return 0, false
	}
}

func GeneralTerminalDeliveryEventIDV1(commitID, slot string) string {
	body := []byte(strings.TrimSpace(commitID) + "\x00" + strings.TrimSpace(slot))
	return generalTerminalDeliveryDomainSHA256(generalTerminalDeliveryEventIDDomain, body)
}

func GeneralTerminalDeliveryPayloadDigestV1(event map[string]any) string {
	canonical := contracts.CloneMap(event)
	delete(canonical, "seq")
	delete(canonical, "generalTerminalPayloadDigest")
	body, err := json.Marshal(canonical)
	if err != nil {
		return ""
	}
	return domainsecurity.SHA256Hex(body)
}

type generalTerminalDeliveryContentV1 struct {
	threadID        string
	turnID          string
	firstSeq        int
	lastSeq         int
	timestamp       string
	commitID        string
	authorityKind   string
	authorityDigest string
	manifest        []GeneralTerminalDeliveryManifestEventV1
}

func validateGeneralTerminalDeliveryEventsV1(events []map[string]any) (generalTerminalDeliveryContentV1, error) {
	if len(events) != 2 && len(events) != 3 {
		return generalTerminalDeliveryContentV1{}, errors.New("general terminal delivery group must contain exactly two or three events")
	}
	expectedSlots := generalTerminalDeliverySlots
	if len(events) == 3 {
		expectedSlots = generalTerminalDeliveryWithItemSlots
	}
	first := events[0]
	content := generalTerminalDeliveryContentV1{
		threadID:        strings.TrimSpace(contracts.StringField(first, "threadId")),
		turnID:          strings.TrimSpace(contracts.StringField(first, "turnId")),
		timestamp:       strings.TrimSpace(contracts.StringField(first, "timestamp")),
		commitID:        strings.TrimSpace(contracts.StringField(first, "generalTerminalCommitId")),
		authorityKind:   strings.TrimSpace(contracts.StringField(first, "generalTerminalAuthorityKind")),
		authorityDigest: strings.TrimSpace(contracts.StringField(first, "generalTerminalAuthorityDigest")),
		manifest:        make([]GeneralTerminalDeliveryManifestEventV1, 0, len(events)),
	}
	if !generalTerminalDeliveryPublicIDV1(content.threadID) || !generalTerminalDeliveryPublicIDV1(content.turnID) ||
		!generalTerminalDeliveryCanonicalTimestamp(content.timestamp) || !domainsecurity.IsSHA256Hex(content.commitID) ||
		content.authorityKind != GeneralTerminalCASAuthorityKind || !domainsecurity.IsSHA256Hex(content.authorityDigest) {
		return generalTerminalDeliveryContentV1{}, errors.New("general terminal delivery identity is invalid")
	}
	seenEventIDs := map[string]bool{}
	for index, event := range events {
		if event == nil || generalTerminalDeliveryMarkerCountV1(event) != len(generalTerminalDeliveryPublicationMarkerNames) ||
			generalTerminalDeliveryNestedMarkerPresentV1(event, true) || ContainsAcceptedFinalPublicationAuthority(event) ||
			ValidatePublicRecord(event) != nil || contracts.StringField(event, "threadId") != content.threadID ||
			contracts.StringField(event, "turnId") != content.turnID || contracts.StringField(event, "timestamp") != content.timestamp ||
			contracts.StringField(event, "generalTerminalCommitId") != content.commitID ||
			contracts.StringField(event, "generalTerminalAuthorityKind") != content.authorityKind ||
			contracts.StringField(event, "generalTerminalAuthorityDigest") != content.authorityDigest ||
			contracts.StringField(event, "generalTerminalSlot") != expectedSlots[index] {
			return generalTerminalDeliveryContentV1{}, errors.New("general terminal delivery event is outside the committed group")
		}
		seq, ok := contracts.NumericSeq(event["seq"])
		if !ok || seq <= 0 || (index > 0 && seq != content.firstSeq+index) {
			return generalTerminalDeliveryContentV1{}, errors.New("general terminal delivery sequence is not contiguous")
		}
		if index == 0 {
			content.firstSeq = seq
		}
		content.lastSeq = seq
		eventID := strings.TrimSpace(contracts.StringField(event, "generalTerminalEventId"))
		payloadDigest := strings.TrimSpace(contracts.StringField(event, "generalTerminalPayloadDigest"))
		if seenEventIDs[eventID] || !domainsecurity.IsSHA256Hex(eventID) ||
			eventID != GeneralTerminalDeliveryEventIDV1(content.commitID, expectedSlots[index]) ||
			!domainsecurity.IsSHA256Hex(payloadDigest) || payloadDigest != GeneralTerminalDeliveryPayloadDigestV1(event) ||
			!generalTerminalDeliverySlotKindValidV1(expectedSlots[index], event) {
			return generalTerminalDeliveryContentV1{}, errors.New("general terminal delivery event integrity is invalid")
		}
		seenEventIDs[eventID] = true
		content.manifest = append(content.manifest, GeneralTerminalDeliveryManifestEventV1{
			Slot: expectedSlots[index], EventID: eventID, PayloadDigest: payloadDigest,
		})
	}
	if !generalTerminalDeliveryProfileValidV1(events) {
		return generalTerminalDeliveryContentV1{}, errors.New("general terminal durable event profile is invalid")
	}
	return content, nil
}

func validateGeneralTerminalProjectedEventsV1(content generalTerminalDeliveryContentV1, events []map[string]any) error {
	if len(events) != len(content.manifest) {
		return errors.New("general terminal public projection is incomplete")
	}
	for index, event := range events {
		seq, ok := contracts.NumericSeq(event["seq"])
		if !ok || seq <= 0 || int64(seq) > generalTerminalMaxSafeIntegerV1 || seq != content.firstSeq+index ||
			contracts.StringField(event, "threadId") != content.threadID ||
			contracts.StringField(event, "turnId") != content.turnID || contracts.StringField(event, "timestamp") != content.timestamp ||
			GeneralTerminalDeliveryMarkerPresentV1(event) || generalTerminalDeliveryNestedMarkerPresentV1(event, true) ||
			ContainsAcceptedFinalPublicationAuthority(event) || ValidatePublicRecord(event) != nil ||
			!generalTerminalDeliveryProjectedEventShapeV1(content.manifest[index].Slot, event) {
			return errors.New("general terminal public projection does not match its durable event")
		}
	}
	if !generalTerminalDeliveryProfileValidV1(events) {
		return errors.New("general terminal public projection profile is invalid")
	}
	return nil
}

func generalTerminalDeliveryProjectedEventShapeV1(slot string, event map[string]any) bool {
	if !generalTerminalDeliverySlotKindValidV1(slot, event) {
		return false
	}
	switch slot {
	case "terminal-item":
		if !generalTerminalDeliveryExactKeysV1(event, []string{
			"kind", "seq", "timestamp", "threadId", "turnId", "itemId", "item",
		}) {
			return false
		}
		item, _ := event["item"].(map[string]any)
		return generalTerminalDeliveryItemShapeV1(event, item)
	case "usage":
		return generalTerminalDeliveryAllowedKeysV1(event, []string{
			"kind", "seq", "timestamp", "threadId", "turnId", "model", "usage", "cacheDiagnostics",
		}, []string{"usageSource", "childRunId", "usageFinalStatus"})
	case "terminal":
		return generalTerminalDeliveryAllowedKeysV1(event, []string{
			"kind", "seq", "timestamp", "threadId", "turnId", "status", "terminalReason",
		}, []string{
			"itemId", "message", "error", "code", "details", "severity", "discard", "cancelled", "cancelledPendingGates",
		}) && generalTerminalDeliveryLifecycleFieldsV1(event)
	default:
		return false
	}
}

func generalTerminalDeliveryItemShapeV1(event, item map[string]any) bool {
	createdAt := contracts.StringField(item, "createdAt")
	finishedAt := contracts.StringField(item, "finishedAt")
	createdInstant, createdErr := time.Parse(time.RFC3339Nano, createdAt)
	finishedInstant, finishedErr := time.Parse(time.RFC3339Nano, finishedAt)
	if item == nil || !generalTerminalDeliveryPublicIDV1(contracts.StringField(event, "itemId")) ||
		!generalTerminalDeliveryPublicIDV1(contracts.StringField(item, "id")) ||
		contracts.StringField(item, "id") != contracts.StringField(event, "itemId") ||
		contracts.StringField(item, "threadId") != contracts.StringField(event, "threadId") ||
		contracts.StringField(item, "turnId") != contracts.StringField(event, "turnId") ||
		finishedAt != contracts.StringField(event, "timestamp") ||
		!generalTerminalDeliveryTimestampV1(createdAt) || !generalTerminalDeliveryTimestampV1(finishedAt) ||
		createdErr != nil || finishedErr != nil || createdInstant.After(finishedInstant) {
		return false
	}
	switch contracts.StringField(item, "kind") {
	case "assistant_text":
		baseKeys := []string{
			"id", "turnId", "threadId", "role", "status", "createdAt", "finishedAt", "kind", "text",
		}
		if contracts.StringField(item, "role") != "assistant" || contracts.StringField(item, "status") != "completed" {
			return false
		}
		if value, present := item["ordinaryResult"]; present {
			slot, err := domainordinaryresult.ParseResultSlotV1(value)
			return err == nil && generalTerminalDeliveryExactKeysV1(item, append(baseKeys, "ordinaryResult")) &&
				contracts.StringField(item, "text") == slot.Text
		}
		return generalTerminalDeliveryExactKeysV1(item, baseKeys) &&
			contracts.StringField(item, "text") == GeneralTerminalCompletedBoundaryTextV1
	case "error":
		return generalTerminalDeliveryAllowedKeysV1(item, []string{
			"id", "turnId", "threadId", "role", "status", "createdAt", "finishedAt", "kind", "code", "message", "severity",
		}, []string{"details"}) && contracts.StringField(item, "role") == "system" &&
			(contracts.StringField(item, "status") == "failed" || contracts.StringField(item, "status") == "aborted") &&
			isClosedFailureRecordV1(item)
	default:
		return false
	}
}

func generalTerminalDeliveryProfileValidV1(events []map[string]any) bool {
	if len(events) != 2 && len(events) != 3 {
		return false
	}
	usage := events[len(events)-2]
	terminal := events[len(events)-1]
	usageMap, usageOK := usage["usage"].(map[string]any)
	diagnosticsMap, diagnosticsOK := usage["cacheDiagnostics"].(map[string]any)
	_, modelOK := usage["model"].(string)
	if !usageOK || !diagnosticsOK || !modelOK ||
		!domainterminaltelemetry.ValidateTerminalTelemetryPublicMapsV1(usageMap, diagnosticsMap) {
		return false
	}
	for _, key := range []string{"usageSource", "childRunId"} {
		if value, present := usage[key]; present {
			text, ok := value.(string)
			if !ok || (key == "childRunId" && !generalTerminalDeliveryPublicIDV1(text)) ||
				(key == "usageSource" && !generalTerminalDeliveryExactText(text)) {
				return false
			}
		}
	}
	reason := contracts.StringField(terminal, "terminalReason")
	expectedStatus, ok := domainterminal.StatusForReasonV1(reason)
	status := contracts.StringField(terminal, "status")
	if !ok || status != expectedStatus || contracts.StringField(terminal, "kind") != "turn_"+expectedStatus {
		return false
	}
	if domainterminal.CandidateAllowedV1(reason) {
		if generalTerminalDeliveryHasFailureProjectionV1(terminal) {
			return false
		}
	} else {
		projection, fixed := domainterminal.GeneralFailureProjectionForCodeV1(
			reason, contracts.StringField(terminal, "code"),
		)
		durable := GeneralTerminalDeliveryMarkerPresentV1(events[0])
		if !fixed || !generalTerminalFailureProjectionMatchesV1(terminal, projection, durable) {
			return false
		}
	}
	if usageStatus, present := usage["usageFinalStatus"]; present {
		if usageStatus != status {
			return false
		}
	}
	hasInterrupt := false
	for _, key := range []string{"discard", "cancelled", "cancelledPendingGates"} {
		if _, present := terminal[key]; present {
			hasInterrupt = true
		}
	}
	if reason == "cancel" {
		if _, ok := terminal["discard"].(bool); !ok {
			return false
		}
		cancelled, ok := terminal["cancelled"].(bool)
		if !ok {
			return false
		}
		if count, ok := contracts.NumericSeq(terminal["cancelledPendingGates"]); !ok || count < 0 ||
			int64(count) > generalTerminalMaxSafeIntegerV1 || (count > 0 && !cancelled) {
			return false
		}
	} else if hasInterrupt {
		return false
	}
	if len(events) == 2 {
		_, hasItemID := terminal["itemId"]
		return !hasItemID
	}
	itemEvent := events[0]
	item, _ := itemEvent["item"].(map[string]any)
	if item == nil || (terminal["itemId"] != nil && terminal["itemId"] != itemEvent["itemId"]) {
		return false
	}
	if contracts.StringField(item, "kind") == "assistant_text" {
		return status == "completed"
	}
	return contracts.StringField(item, "kind") == "error" && contracts.StringField(item, "status") == status &&
		terminal["itemId"] == itemEvent["itemId"] && generalTerminalDeliveryFailureProjectionMatchesItemV1(item, terminal)
}

func generalTerminalFailureProjectionMatchesV1(
	record map[string]any,
	projection domainterminal.PublicFailureProjectionV1,
	durable bool,
) bool {
	if contracts.StringField(record, "status") != projection.Status ||
		contracts.StringField(record, "code") != projection.Code ||
		contracts.StringField(record, "message") != projection.Message ||
		contracts.StringField(record, "severity") != projection.Severity {
		return false
	}
	errorValue, hasError := record["error"]
	if durable {
		if !hasError || errorValue != projection.Message {
			return false
		}
	} else if hasError {
		// The durable outbox keeps the closed host failure in both message and
		// error for restart diagnostics. Public ordinary projections expose the
		// canonical message only, so arbitrary error bytes can never hitchhike
		// on the atomic SSE batch.
		return false
	}
	rawDetails, hasDetails := record["details"]
	if !hasDetails {
		return true
	}
	details, ok := rawDetails.(map[string]any)
	return ok && projection.Code == "tool_not_advertised" &&
		domainfailure.ValidateToolNotAdvertisedDetails(details)
}

func generalTerminalDeliveryHasFailureProjectionV1(record map[string]any) bool {
	for _, key := range []string{"message", "error", "code", "details", "severity"} {
		if _, present := record[key]; present {
			return true
		}
	}
	return false
}

func generalTerminalDeliveryFailureProjectionMatchesItemV1(item, terminal map[string]any) bool {
	for _, key := range []string{"code", "message", "severity"} {
		itemValue, itemPresent := item[key]
		terminalValue, terminalPresent := terminal[key]
		if !itemPresent || !terminalPresent || itemValue != terminalValue {
			return false
		}
	}
	itemDetails, itemHasDetails := item["details"]
	terminalDetails, terminalHasDetails := terminal["details"]
	if itemHasDetails != terminalHasDetails || (itemHasDetails && !reflect.DeepEqual(itemDetails, terminalDetails)) {
		return false
	}
	if failureText, present := terminal["error"]; present && failureText != item["message"] {
		return false
	}
	return true
}

func generalTerminalDeliveryLifecycleFieldsV1(event map[string]any) bool {
	if itemID, present := event["itemId"]; present {
		text, ok := itemID.(string)
		if !ok || !generalTerminalDeliveryPublicIDV1(text) {
			return false
		}
	}
	hasFailureProjection := false
	for _, key := range []string{"message", "error", "code"} {
		if value, present := event[key]; present {
			text, ok := value.(string)
			if !ok || !generalTerminalDeliveryExactText(text) {
				return false
			}
			hasFailureProjection = true
		}
	}
	if value, present := event["severity"]; present {
		severity, ok := value.(string)
		if !ok || (severity != "info" && severity != "warning" && severity != "error") {
			return false
		}
		hasFailureProjection = true
	}
	if value, present := event["details"]; present {
		details, ok := value.(map[string]any)
		if !ok || !domainfailure.ValidatePublicDetails(details) {
			return false
		}
		hasFailureProjection = true
	}
	for _, key := range []string{"discard", "cancelled"} {
		if value, present := event[key]; present {
			if _, ok := value.(bool); !ok {
				return false
			}
		}
	}
	if value, present := event["cancelledPendingGates"]; present {
		count, ok := contracts.NumericSeq(value)
		if !ok || count < 0 || int64(count) > generalTerminalMaxSafeIntegerV1 {
			return false
		}
	}
	return !hasFailureProjection || isClosedFailureRecordV1(event)
}

func generalTerminalDeliveryExactKeysV1(record map[string]any, keys []string) bool {
	return generalTerminalDeliveryAllowedKeysV1(record, keys, nil)
}

func generalTerminalDeliveryAllowedKeysV1(record map[string]any, required, optional []string) bool {
	if record == nil || len(record) < len(required) || len(record) > len(required)+len(optional) {
		return false
	}
	allowed := make(map[string]bool, len(required)+len(optional))
	for _, key := range required {
		allowed[key] = true
		if _, present := record[key]; !present {
			return false
		}
	}
	for _, key := range optional {
		allowed[key] = true
	}
	for key := range record {
		if !allowed[key] {
			return false
		}
	}
	return true
}

func generalTerminalDeliveryTimestampV1(value string) bool {
	matches := generalTerminalDeliveryTimestampPatternV1.FindStringSubmatch(value)
	if matches == nil {
		return false
	}
	if matches[1] != "" {
		hour, hourErr := strconv.Atoi(matches[1])
		minute, minuteErr := strconv.Atoi(matches[2])
		if hourErr != nil || minuteErr != nil || hour >= 24 || minute >= 60 {
			return false
		}
	}
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

func generalTerminalDeliverySlotKindValidV1(slot string, event map[string]any) bool {
	switch slot {
	case "terminal-item":
		item, _ := event["item"].(map[string]any)
		return contracts.StringField(event, "kind") == "item_completed" &&
			generalTerminalDeliveryPublicIDV1(contracts.StringField(event, "itemId")) &&
			contracts.StringField(item, "id") == contracts.StringField(event, "itemId")
	case "usage":
		return contracts.StringField(event, "kind") == "usage"
	case "terminal":
		switch contracts.StringField(event, "kind") {
		case "turn_completed", "turn_failed", "turn_aborted":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func generalTerminalDeliveryMarkerCountV1(event map[string]any) int {
	count := 0
	for _, name := range generalTerminalDeliveryPublicationMarkerNames {
		if _, present := event[name]; present {
			count++
		}
	}
	return count
}

func generalTerminalDeliveryNestedMarkerPresentV1(value any, root bool) bool {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if !root {
				for _, marker := range generalTerminalDeliveryPublicationMarkerNames {
					if key == marker {
						return true
					}
				}
			}
			if generalTerminalDeliveryNestedMarkerPresentV1(child, false) {
				return true
			}
		}
	case []any:
		for _, child := range current {
			if generalTerminalDeliveryNestedMarkerPresentV1(child, false) {
				return true
			}
		}
	}
	return false
}

func generalTerminalDeliveryManifestDigestV1(manifest []GeneralTerminalDeliveryManifestEventV1) string {
	body, err := json.Marshal(manifest)
	if err != nil {
		return ""
	}
	return generalTerminalDeliveryDomainSHA256(generalTerminalDeliveryManifestDigestDomain, body)
}

func generalTerminalProjectedEventsDigestV1(events []map[string]any) string {
	body, err := json.Marshal(events)
	if err != nil {
		return ""
	}
	return generalTerminalDeliveryDomainSHA256(generalTerminalProjectedEventsDigestDomain, body)
}

func generalTerminalDeliveryBatchDigestV1(batch GeneralTerminalDeliveryBatchV1) string {
	batch.BatchDigest = ""
	body, err := json.Marshal(batch)
	if err != nil {
		return ""
	}
	return generalTerminalDeliveryDomainSHA256(generalTerminalDeliveryBatchDigestDomain, body)
}

func generalTerminalDeliveryDomainSHA256(domain, body []byte) string {
	hash := sha256.New()
	_, _ = hash.Write(domain)
	_, _ = hash.Write(body)
	return hex.EncodeToString(hash.Sum(nil))
}

func generalTerminalDeliveryExactText(value string) bool {
	return value != "" && value == strings.TrimSpace(value) &&
		!strings.HasPrefix(value, "\ufeff") && !strings.HasSuffix(value, "\ufeff")
}

func generalTerminalDeliveryPublicIDV1(value string) bool {
	return generalTerminalPublicIDPatternV1.MatchString(value)
}

func generalTerminalDeliveryCanonicalTimestamp(value string) bool {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && value == parsed.UTC().Format(time.RFC3339Nano)
}

func cloneGeneralTerminalDeliveryBatchV1(batch GeneralTerminalDeliveryBatchV1) GeneralTerminalDeliveryBatchV1 {
	body, _ := json.Marshal(batch)
	var out GeneralTerminalDeliveryBatchV1
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	_ = decoder.Decode(&out)
	return out
}
