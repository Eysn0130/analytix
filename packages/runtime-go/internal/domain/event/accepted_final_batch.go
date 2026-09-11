package event

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
)

const (
	AcceptedFinalDeliveryBatchV1Version = 1
	AcceptedFinalDeliveryBatchV1Purpose = "analytix.accepted-final-delivery-batch/v1"
	AcceptedFinalDeliveryBatchV2Version = 2
	AcceptedFinalDeliveryBatchV2Purpose = "analytix.accepted-final-delivery-batch/v2"
	AcceptedFinalDeliveryBatchKind      = "accepted_final_batch"
	AcceptedFinalDeliveryGroupLimitV1   = 256
)

var acceptedFinalDeliverySlots = []string{"assistant-final", "usage", "terminal"}
var acceptedFinalFailureDeliverySlots = []string{"assistant-final", "terminal-error-item", "usage", "terminal"}

// AcceptedFinalDeliveryBatchV2 is the current transport-only, indivisible projection of
// one already durable accepted-final manifest. The nested events remain the
// canonical persisted records; the batch prevents live/SSE consumers from
// advancing a cursor through only a prefix.
type AcceptedFinalDeliveryBatchV2 struct {
	SchemaVersion        int                         `json:"schemaVersion"`
	Purpose              string                      `json:"purpose"`
	Kind                 string                      `json:"kind"`
	BatchID              string                      `json:"batchId"`
	ThreadID             string                      `json:"threadId"`
	TurnID               string                      `json:"turnId"`
	Seq                  int                         `json:"seq"`
	FirstSeq             int                         `json:"firstSeq"`
	LastSeq              int                         `json:"lastSeq"`
	Timestamp            string                      `json:"timestamp"`
	PublicationCommitID  string                      `json:"publicationCommitId"`
	EventManifestDigest  string                      `json:"eventManifestDigest"`
	PublicationAuthority AcceptedFinalDeliverySealV1 `json:"publicationAuthority"`
	Events               []map[string]any            `json:"events"`
}

// AcceptedFinalDeliveryBatchV1 is the frozen historical transport shape. Its
// assistant event contains the private signed accepted-final record and is
// therefore audit-only; it must never be routed to a generic public consumer.
type AcceptedFinalDeliveryBatchV1 struct {
	SchemaVersion        int                         `json:"schemaVersion"`
	Purpose              string                      `json:"purpose"`
	Kind                 string                      `json:"kind"`
	BatchID              string                      `json:"batchId"`
	ThreadID             string                      `json:"threadId"`
	TurnID               string                      `json:"turnId"`
	Seq                  int                         `json:"seq"`
	FirstSeq             int                         `json:"firstSeq"`
	LastSeq              int                         `json:"lastSeq"`
	Timestamp            string                      `json:"timestamp"`
	PublicationCommitID  string                      `json:"publicationCommitId"`
	EventManifestDigest  string                      `json:"eventManifestDigest"`
	PublicationAuthority AcceptedFinalDeliverySealV1 `json:"publicationAuthority"`
	Events               []map[string]any            `json:"events"`
}

func NewAcceptedFinalDeliveryBatchV1(events []map[string]any, authority AcceptedFinalDeliverySealV1) (AcceptedFinalDeliveryBatchV1, error) {
	batch, err := newAcceptedFinalDeliveryBatchContentV1(events)
	if err != nil {
		return AcceptedFinalDeliveryBatchV1{}, err
	}
	batch.PublicationAuthority = authority
	if err := ValidateAcceptedFinalDeliveryBatchV1(batch); err != nil {
		return AcceptedFinalDeliveryBatchV1{}, err
	}
	return batch, nil
}

func NewAcceptedFinalDeliverySealForEventsV1(
	events []map[string]any,
	acceptedFinalDispositionDigest string,
	terminalDispositionID string,
	authorityKeyID string,
	authorityPublicKey []byte,
	sign AcceptedFinalDeliverySignFuncV1,
) (AcceptedFinalDeliverySealV1, error) {
	batch, err := newAcceptedFinalDeliveryBatchContentV1(events)
	if err != nil {
		return AcceptedFinalDeliverySealV1{}, err
	}
	return NewAcceptedFinalDeliverySealV1(AcceptedFinalDeliverySealInputV1{
		ThreadID: batch.ThreadID, TurnID: batch.TurnID, PublicationCommitID: batch.PublicationCommitID,
		AcceptedFinalDispositionDigest: acceptedFinalDispositionDigest, TerminalDispositionID: terminalDispositionID,
		EventManifestDigest: batch.EventManifestDigest, SequencedEventsDigest: acceptedFinalDeliverySequencedEventsDigest(batch.Events),
		BatchID: batch.BatchID, FirstSeq: batch.FirstSeq, LastSeq: batch.LastSeq, Timestamp: batch.Timestamp,
		AuthorityKeyID: authorityKeyID, AuthorityPublicKey: authorityPublicKey,
	}, sign)
}

// ValidateAcceptedFinalDeliveryEventsV1 validates only the frozen delivery
// framing owned by this package. Because Batch V1 embeds private evidence
// authority, every migration/hydration admission must additionally call
// evidence.ValidateHistoricalAcceptedFinalDeliveryEventsV1.
func ValidateAcceptedFinalDeliveryEventsV1(events []map[string]any) error {
	_, err := newAcceptedFinalDeliveryBatchContentV1(events)
	return err
}

func newAcceptedFinalDeliveryBatchContentV1(events []map[string]any) (AcceptedFinalDeliveryBatchV1, error) {
	cloned := cloneAcceptedFinalDeliveryEvents(events)
	if len(cloned) == 0 {
		return AcceptedFinalDeliveryBatchV1{}, errors.New("accepted final delivery events are required")
	}
	firstSeq, _ := contracts.NumericSeq(cloned[0]["seq"])
	lastSeq, _ := contracts.NumericSeq(cloned[len(cloned)-1]["seq"])
	batch := AcceptedFinalDeliveryBatchV1{
		SchemaVersion: AcceptedFinalDeliveryBatchV1Version, Purpose: AcceptedFinalDeliveryBatchV1Purpose,
		Kind: AcceptedFinalDeliveryBatchKind, ThreadID: strings.TrimSpace(contracts.StringField(cloned[0], "threadId")),
		TurnID: strings.TrimSpace(contracts.StringField(cloned[0], "turnId")), Seq: lastSeq,
		FirstSeq: firstSeq, LastSeq: lastSeq, Timestamp: strings.TrimSpace(contracts.StringField(cloned[len(cloned)-1], "timestamp")),
		PublicationCommitID: strings.TrimSpace(contracts.StringField(cloned[0], "publicationCommitId")),
		EventManifestDigest: acceptedFinalDeliveryManifestDigest(cloned), Events: cloned,
	}
	batch.BatchID = acceptedFinalDeliveryBatchIDV1(batch)
	if err := validateAcceptedFinalDeliveryBatchContentV1(batch); err != nil {
		return AcceptedFinalDeliveryBatchV1{}, err
	}
	return batch, nil
}

func ValidateAcceptedFinalDeliveryBatchV1(batch AcceptedFinalDeliveryBatchV1) error {
	if err := validateAcceptedFinalDeliveryBatchContentV1(batch); err != nil {
		return err
	}
	if ValidateAcceptedFinalDeliverySealV1(batch.PublicationAuthority) != nil ||
		batch.PublicationAuthority.ThreadID != batch.ThreadID || batch.PublicationAuthority.TurnID != batch.TurnID ||
		batch.PublicationAuthority.PublicationCommitID != batch.PublicationCommitID ||
		batch.PublicationAuthority.EventManifestDigest != batch.EventManifestDigest ||
		batch.PublicationAuthority.SequencedEventsDigest != acceptedFinalDeliverySequencedEventsDigest(batch.Events) ||
		batch.PublicationAuthority.BatchID != batch.BatchID || batch.PublicationAuthority.FirstSeq != batch.FirstSeq ||
		batch.PublicationAuthority.LastSeq != batch.LastSeq || batch.PublicationAuthority.Timestamp != batch.Timestamp {
		return errors.New("historical accepted final delivery batch authority is invalid")
	}
	return nil
}

func validateAcceptedFinalDeliveryBatchContentV1(batch AcceptedFinalDeliveryBatchV1) error {
	if batch.SchemaVersion != AcceptedFinalDeliveryBatchV1Version || batch.Purpose != AcceptedFinalDeliveryBatchV1Purpose ||
		batch.Kind != AcceptedFinalDeliveryBatchKind || !domainsecurity.IsSHA256Hex(batch.BatchID) ||
		strings.TrimSpace(batch.ThreadID) == "" || strings.TrimSpace(batch.TurnID) == "" ||
		batch.Seq <= 0 || batch.FirstSeq <= 0 || batch.LastSeq < batch.FirstSeq || batch.Seq != batch.LastSeq ||
		strings.TrimSpace(batch.Timestamp) == "" || !domainsecurity.IsSHA256Hex(batch.PublicationCommitID) ||
		!domainsecurity.IsSHA256Hex(batch.EventManifestDigest) || (len(batch.Events) != 3 && len(batch.Events) != 4) ||
		batch.LastSeq-batch.FirstSeq+1 != len(batch.Events) {
		return errors.New("historical accepted final delivery batch is incomplete")
	}
	expectedSlots := acceptedFinalDeliverySlots
	if len(batch.Events) == len(acceptedFinalFailureDeliverySlots) {
		expectedSlots = acceptedFinalFailureDeliverySlots
	}
	for index, event := range batch.Events {
		seq, ok := contracts.NumericSeq(event["seq"])
		slot := strings.TrimSpace(contracts.StringField(event, "publicationSlot"))
		if !ok || seq != batch.FirstSeq+index || slot != expectedSlots[index] ||
			contracts.StringField(event, "threadId") != batch.ThreadID || contracts.StringField(event, "turnId") != batch.TurnID ||
			contracts.StringField(event, "timestamp") != batch.Timestamp ||
			contracts.StringField(event, "publicationCommitId") != batch.PublicationCommitID ||
			contracts.StringField(event, "acceptedFinalDigest") != batch.PublicationCommitID ||
			contracts.StringField(event, "publicationEventId") != acceptedFinalDeliveryEventID(batch.PublicationCommitID, slot) ||
			contracts.StringField(event, "publicationPayloadDigest") != acceptedFinalDeliveryPayloadDigest(event) ||
			(slot == "usage" && !validAcceptedFinalUsageEffortV1(event)) {
			return errors.New("historical accepted final delivery event is outside the committed manifest")
		}
		if index == 0 {
			item, itemOK := event["item"].(map[string]any)
			record, recordOK := item["acceptedFinal"].(map[string]any)
			view, viewOK := item["acceptedFinalView"].(map[string]any)
			if !itemOK || !recordOK || !viewOK ||
				!historicalAcceptedFinalV1HasExactKeys(event, historicalAcceptedFinalItemEventV1Keys) ||
				!historicalAcceptedFinalV1HasExactKeys(item, historicalAcceptedFinalAssistantItemV1Keys) ||
				!historicalAcceptedFinalRecordV5HasExactKeys(record) ||
				!validHistoricalAcceptedFinalPublicViewV2Wire(view, record) ||
				!ContainsPrivateAcceptedFinalAuthority(event) ||
				contracts.StringField(record, "threadId") != batch.ThreadID || contracts.StringField(record, "turnId") != batch.TurnID ||
				contracts.StringField(record, "recordDigest") != batch.PublicationCommitID ||
				contracts.StringField(record, "acceptedAt") != batch.Timestamp ||
				contracts.StringField(record, "terminalReason") != contracts.StringField(batch.Events[len(batch.Events)-1], "terminalReason") ||
				contracts.StringField(item, "threadId") != batch.ThreadID || contracts.StringField(item, "turnId") != batch.TurnID ||
				contracts.StringField(event, "itemId") != contracts.StringField(item, "id") ||
				contracts.StringField(item, "role") != "assistant" || contracts.StringField(item, "kind") != "assistant_text" ||
				contracts.StringField(item, "status") != "completed" ||
				contracts.StringField(item, "createdAt") != batch.Timestamp || contracts.StringField(item, "finishedAt") != batch.Timestamp {
				return errors.New("historical accepted final delivery assistant authority is invalid")
			}
		} else if ContainsPrivateAcceptedFinalAuthority(event) || ValidatePublicRecord(event) != nil {
			return errors.New("historical accepted final delivery non-assistant event is not public")
		}
	}
	if contracts.StringField(batch.Events[0], "kind") != "item_completed" ||
		contracts.StringField(batch.Events[len(batch.Events)-2], "kind") != "usage" ||
		!strings.HasPrefix(contracts.StringField(batch.Events[len(batch.Events)-1], "kind"), "turn_") ||
		(len(batch.Events) == 4 && contracts.StringField(batch.Events[1], "kind") != "item_completed") {
		return errors.New("historical accepted final delivery slots have invalid event kinds")
	}
	if err := validateAcceptedFinalDeliveryTerminalSemanticsV2(AcceptedFinalDeliveryBatchV2{Events: batch.Events}); err != nil {
		return err
	}
	if acceptedFinalDeliveryManifestDigest(batch.Events) != batch.EventManifestDigest || acceptedFinalDeliveryBatchIDV1(batch) != batch.BatchID {
		return errors.New("historical accepted final delivery batch integrity is invalid")
	}
	return nil
}

var historicalAcceptedFinalItemEventV1Keys = map[string]struct{}{
	"kind": {}, "seq": {}, "timestamp": {}, "threadId": {}, "turnId": {}, "itemId": {}, "item": {},
	"acceptedFinalDigest": {}, "publicationCommitId": {}, "publicationEventId": {}, "publicationSlot": {},
	"publicationPayloadDigest": {},
}

var historicalAcceptedFinalAssistantItemV1Keys = map[string]struct{}{
	"id": {}, "turnId": {}, "threadId": {}, "role": {}, "status": {}, "createdAt": {}, "finishedAt": {},
	"kind": {}, "text": {}, "acceptedFinal": {}, "acceptedFinalView": {},
}

var historicalAcceptedFinalRecordV5Keys = map[string]struct{}{
	"schemaVersion": {}, "authorityPurpose": {}, "authorityAlgorithm": {}, "authorityKeyId": {},
	"authorityPublicKey": {}, "threadId": {}, "turnId": {}, "envelopeDigest": {}, "contextDigest": {},
	"contextEpoch": {}, "datasetSnapshotId": {}, "variant": {}, "terminalReason": {}, "renderedTextSha256": {},
	"registrySequence": {}, "registryStateDigest": {}, "rendererVersion": {}, "finalGateVersion": {},
	"verifierVersion": {}, "publicView": {}, "publicViewDigest": {}, "publicationSnapshotProofDigest": {},
	"factFinalWitnessAdmission": {}, "privateRecordDigest": {}, "acceptedAt": {}, "authoritySignature": {},
	"recordDigest": {},
}

func historicalAcceptedFinalV1HasExactKeys(record map[string]any, allowed map[string]struct{}) bool {
	if len(record) != len(allowed) {
		return false
	}
	for key := range record {
		if _, present := allowed[key]; !present {
			return false
		}
	}
	return true
}

func historicalAcceptedFinalRecordV5HasExactKeys(record map[string]any) bool {
	version, versionOK := contracts.NumericSeq(record["schemaVersion"])
	_, hasProof := record["publicationSnapshotProofDigest"]
	_, hasWitness := record["factFinalWitnessAdmission"]
	if !versionOK || version != 5 || hasProof != hasWitness {
		return false
	}
	expectedCount := len(historicalAcceptedFinalRecordV5Keys)
	if !hasProof {
		expectedCount -= 2
	}
	if len(record) != expectedCount {
		return false
	}
	for key := range record {
		if _, allowed := historicalAcceptedFinalRecordV5Keys[key]; !allowed ||
			(!hasProof && (key == "publicationSnapshotProofDigest" || key == "factFinalWitnessAdmission")) {
			return false
		}
	}
	return true
}

var historicalAcceptedFinalPublicViewCoreV2Keys = map[string]struct{}{
	"schemaVersion": {}, "publicationState": {}, "envelopeDigest": {}, "contextDigest": {},
	"contextEpoch": {}, "datasetSnapshotId": {}, "variant": {}, "terminalReason": {},
	"blockerCode": {}, "coverageStatus": {}, "checkedScopeDigest": {}, "missingScopeCount": {},
	"claimCount": {}, "claimTypes": {}, "receiptMetadata": {}, "noHitWording": {},
	"envelopeIssuedAt": {}, "acceptedAt": {},
}

// validHistoricalAcceptedFinalPublicViewV2Wire preserves the original private
// Batch V1 assistant tuple: the full signed V5 record plus the exact expanded
// V2 view derived from its embedded signed core. Generic consumers never call
// this path; current delivery is the closed V3-in-Batch-V2 family.
func validHistoricalAcceptedFinalPublicViewV2Wire(view, record map[string]any) bool {
	core, coreOK := record["publicView"].(map[string]any)
	if !coreOK || len(core) != len(historicalAcceptedFinalPublicViewCoreV2Keys) ||
		len(view) != len(historicalAcceptedFinalPublicViewCoreV2Keys)+2 {
		return false
	}
	for key := range core {
		if _, allowed := historicalAcceptedFinalPublicViewCoreV2Keys[key]; !allowed {
			return false
		}
	}
	for key := range view {
		if key == "acceptedFinalDigest" || key == "publicViewDigest" {
			continue
		}
		if _, allowed := historicalAcceptedFinalPublicViewCoreV2Keys[key]; !allowed {
			return false
		}
	}
	coreVersion, coreVersionOK := contracts.NumericSeq(core["schemaVersion"])
	viewVersion, viewVersionOK := contracts.NumericSeq(view["schemaVersion"])
	if !coreVersionOK || coreVersion != 2 || !viewVersionOK || viewVersion != 2 ||
		contracts.StringField(core, "publicationState") != "accepted" ||
		contracts.StringField(view, "publicationState") != "accepted" ||
		!domainsecurity.IsSHA256Hex(contracts.StringField(record, "recordDigest")) ||
		!domainsecurity.IsSHA256Hex(contracts.StringField(record, "publicViewDigest")) ||
		historicalAcceptedFinalPublicViewCoreV2Digest(core) != contracts.StringField(record, "publicViewDigest") ||
		contracts.StringField(view, "acceptedFinalDigest") != contracts.StringField(record, "recordDigest") ||
		contracts.StringField(view, "publicViewDigest") != contracts.StringField(record, "publicViewDigest") {
		return false
	}
	expected := contracts.CloneMap(core)
	expected["acceptedFinalDigest"] = contracts.StringField(record, "recordDigest")
	expected["publicViewDigest"] = contracts.StringField(record, "publicViewDigest")
	return reflect.DeepEqual(view, expected)
}

func historicalAcceptedFinalPublicViewCoreV2Digest(core map[string]any) string {
	receipt, receiptOK := core["receiptMetadata"].(map[string]any)
	citations, citationsOK := receipt["citations"].([]any)
	if !receiptOK || !citationsOK || len(receipt) != 4 {
		return ""
	}
	for _, key := range []string{"projection", "count", "setDigest", "citations"} {
		if _, present := receipt[key]; !present {
			return ""
		}
	}
	type citationV1 struct {
		Handle any `json:"handle"`
		Label  any `json:"label"`
	}
	canonicalCitations := make([]citationV1, 0, len(citations))
	for _, value := range citations {
		citation, ok := value.(map[string]any)
		if !ok || len(citation) != 2 || citation["handle"] == nil || citation["label"] == nil {
			return ""
		}
		canonicalCitations = append(canonicalCitations, citationV1{
			Handle: citation["handle"], Label: citation["label"],
		})
	}
	type receiptV1 struct {
		Projection any          `json:"projection"`
		Count      any          `json:"count"`
		SetDigest  any          `json:"setDigest"`
		Citations  []citationV1 `json:"citations"`
	}
	canonical := struct {
		SchemaVersion      any       `json:"schemaVersion"`
		PublicationState   any       `json:"publicationState"`
		EnvelopeDigest     any       `json:"envelopeDigest"`
		ContextDigest      any       `json:"contextDigest"`
		ContextEpoch       any       `json:"contextEpoch"`
		DatasetSnapshotID  any       `json:"datasetSnapshotId"`
		Variant            any       `json:"variant"`
		TerminalReason     any       `json:"terminalReason"`
		BlockerCode        any       `json:"blockerCode"`
		CoverageStatus     any       `json:"coverageStatus"`
		CheckedScopeDigest any       `json:"checkedScopeDigest"`
		MissingScopeCount  any       `json:"missingScopeCount"`
		ClaimCount         any       `json:"claimCount"`
		ClaimTypes         any       `json:"claimTypes"`
		ReceiptMetadata    receiptV1 `json:"receiptMetadata"`
		NoHitWording       any       `json:"noHitWording"`
		EnvelopeIssuedAt   any       `json:"envelopeIssuedAt"`
		AcceptedAt         any       `json:"acceptedAt"`
	}{
		SchemaVersion: core["schemaVersion"], PublicationState: core["publicationState"],
		EnvelopeDigest: core["envelopeDigest"], ContextDigest: core["contextDigest"],
		ContextEpoch: core["contextEpoch"], DatasetSnapshotID: core["datasetSnapshotId"],
		Variant: core["variant"], TerminalReason: core["terminalReason"], BlockerCode: core["blockerCode"],
		CoverageStatus: core["coverageStatus"], CheckedScopeDigest: core["checkedScopeDigest"],
		MissingScopeCount: core["missingScopeCount"], ClaimCount: core["claimCount"], ClaimTypes: core["claimTypes"],
		ReceiptMetadata: receiptV1{
			Projection: receipt["projection"], Count: receipt["count"], SetDigest: receipt["setDigest"],
			Citations: canonicalCitations,
		},
		NoHitWording: core["noHitWording"], EnvelopeIssuedAt: core["envelopeIssuedAt"], AcceptedAt: core["acceptedAt"],
	}
	body, err := json.Marshal(canonical)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(append([]byte("analytix.accepted-final-public-view/v2\x00"), body...))
	return hex.EncodeToString(digest[:])
}

func NewAcceptedFinalDeliveryBatchV2(events []map[string]any, authority AcceptedFinalDeliverySealV1) (AcceptedFinalDeliveryBatchV2, error) {
	batch, err := newAcceptedFinalDeliveryBatchContentV2(events)
	if err != nil {
		return AcceptedFinalDeliveryBatchV2{}, err
	}
	batch.PublicationAuthority = authority
	if err := ValidateAcceptedFinalDeliveryBatchV2(batch); err != nil {
		return AcceptedFinalDeliveryBatchV2{}, err
	}
	return batch, nil
}

func NewAcceptedFinalDeliverySealForEventsV2(
	events []map[string]any,
	acceptedFinalDispositionDigest string,
	terminalDispositionID string,
	authorityKeyID string,
	authorityPublicKey []byte,
	sign AcceptedFinalDeliverySignFuncV1,
) (AcceptedFinalDeliverySealV1, error) {
	batch, err := newAcceptedFinalDeliveryBatchContentV2(events)
	if err != nil {
		return AcceptedFinalDeliverySealV1{}, err
	}
	return NewAcceptedFinalDeliverySealV1(AcceptedFinalDeliverySealInputV1{
		ThreadID: batch.ThreadID, TurnID: batch.TurnID, PublicationCommitID: batch.PublicationCommitID,
		AcceptedFinalDispositionDigest: acceptedFinalDispositionDigest, TerminalDispositionID: terminalDispositionID,
		EventManifestDigest: batch.EventManifestDigest, SequencedEventsDigest: acceptedFinalDeliverySequencedEventsDigest(batch.Events),
		BatchID: batch.BatchID, FirstSeq: batch.FirstSeq, LastSeq: batch.LastSeq, Timestamp: batch.Timestamp,
		AuthorityKeyID: authorityKeyID, AuthorityPublicKey: authorityPublicKey,
	}, sign)
}

// ValidateAcceptedFinalDeliveryEventsV2 validates the complete sequenced
// event manifest without treating an unsigned transport wrapper as
// publication authority. Durable readback adapters use this before comparing
// the exact persisted objects that may later be sealed.
func ValidateAcceptedFinalDeliveryEventsV2(events []map[string]any) error {
	_, err := newAcceptedFinalDeliveryBatchContentV2(events)
	return err
}

func newAcceptedFinalDeliveryBatchContentV2(events []map[string]any) (AcceptedFinalDeliveryBatchV2, error) {
	cloned := cloneAcceptedFinalDeliveryEvents(events)
	if len(cloned) == 0 {
		return AcceptedFinalDeliveryBatchV2{}, errors.New("accepted final delivery events are required")
	}
	firstSeq, _ := contracts.NumericSeq(cloned[0]["seq"])
	lastSeq, _ := contracts.NumericSeq(cloned[len(cloned)-1]["seq"])
	manifestDigest := acceptedFinalDeliveryManifestDigest(cloned)
	commitID := strings.TrimSpace(contracts.StringField(cloned[0], "publicationCommitId"))
	batch := AcceptedFinalDeliveryBatchV2{
		SchemaVersion: AcceptedFinalDeliveryBatchV2Version, Purpose: AcceptedFinalDeliveryBatchV2Purpose,
		Kind: AcceptedFinalDeliveryBatchKind, ThreadID: strings.TrimSpace(contracts.StringField(cloned[0], "threadId")),
		TurnID: strings.TrimSpace(contracts.StringField(cloned[0], "turnId")), Seq: lastSeq,
		FirstSeq: firstSeq, LastSeq: lastSeq, Timestamp: strings.TrimSpace(contracts.StringField(cloned[len(cloned)-1], "timestamp")),
		PublicationCommitID: commitID, EventManifestDigest: manifestDigest, Events: cloned,
	}
	batch.BatchID = acceptedFinalDeliveryBatchIDV2(batch)
	if err := validateAcceptedFinalDeliveryBatchContentV2(batch); err != nil {
		return AcceptedFinalDeliveryBatchV2{}, err
	}
	return batch, nil
}

func ValidateAcceptedFinalDeliveryBatchV2(batch AcceptedFinalDeliveryBatchV2) error {
	if err := validateAcceptedFinalDeliveryBatchContentV2(batch); err != nil {
		return err
	}
	if ValidateAcceptedFinalDeliverySealV1(batch.PublicationAuthority) != nil ||
		batch.PublicationAuthority.ThreadID != batch.ThreadID || batch.PublicationAuthority.TurnID != batch.TurnID ||
		batch.PublicationAuthority.PublicationCommitID != batch.PublicationCommitID ||
		batch.PublicationAuthority.EventManifestDigest != batch.EventManifestDigest ||
		batch.PublicationAuthority.SequencedEventsDigest != acceptedFinalDeliverySequencedEventsDigest(batch.Events) ||
		batch.PublicationAuthority.BatchID != batch.BatchID || batch.PublicationAuthority.FirstSeq != batch.FirstSeq ||
		batch.PublicationAuthority.LastSeq != batch.LastSeq || batch.PublicationAuthority.Timestamp != batch.Timestamp {
		return errors.New("accepted final delivery batch authority is invalid")
	}
	return nil
}

func validateAcceptedFinalDeliveryBatchContentV2(batch AcceptedFinalDeliveryBatchV2) error {
	if batch.SchemaVersion != AcceptedFinalDeliveryBatchV2Version || batch.Purpose != AcceptedFinalDeliveryBatchV2Purpose ||
		batch.Kind != AcceptedFinalDeliveryBatchKind || !domainsecurity.IsSHA256Hex(batch.BatchID) ||
		strings.TrimSpace(batch.ThreadID) == "" || strings.TrimSpace(batch.TurnID) == "" ||
		batch.Seq <= 0 || batch.FirstSeq <= 0 || batch.LastSeq < batch.FirstSeq || batch.Seq != batch.LastSeq ||
		strings.TrimSpace(batch.Timestamp) == "" || !domainsecurity.IsSHA256Hex(batch.PublicationCommitID) ||
		!domainsecurity.IsSHA256Hex(batch.EventManifestDigest) || len(batch.Events) < 3 || len(batch.Events) > 4 {
		return errors.New("accepted final delivery batch is incomplete")
	}
	expectedSlots := acceptedFinalDeliverySlots
	if len(batch.Events) == len(acceptedFinalFailureDeliverySlots) {
		expectedSlots = acceptedFinalFailureDeliverySlots
	}
	if batch.LastSeq-batch.FirstSeq+1 != len(batch.Events) {
		return errors.New("accepted final delivery batch sequence range is not contiguous")
	}
	for index, event := range batch.Events {
		seq, ok := contracts.NumericSeq(event["seq"])
		slot := strings.TrimSpace(contracts.StringField(event, "publicationSlot"))
		if !ok || seq != batch.FirstSeq+index || slot != expectedSlots[index] ||
			contracts.StringField(event, "threadId") != batch.ThreadID || contracts.StringField(event, "turnId") != batch.TurnID ||
			contracts.StringField(event, "timestamp") != batch.Timestamp ||
			contracts.StringField(event, "publicationCommitId") != batch.PublicationCommitID ||
			contracts.StringField(event, "acceptedFinalDigest") != batch.PublicationCommitID ||
			contracts.StringField(event, "publicationEventId") != acceptedFinalDeliveryEventID(batch.PublicationCommitID, slot) ||
			contracts.StringField(event, "publicationPayloadDigest") != acceptedFinalDeliveryPayloadDigest(event) ||
			(slot == "usage" && !validAcceptedFinalUsageEffortV1(event)) ||
			ContainsPrivateAcceptedFinalAuthority(event) ||
			ValidatePublicRecord(event) != nil {
			return errors.New("accepted final delivery event is outside the committed manifest")
		}
		if index == 0 && !validAcceptedFinalPublicAssistantEventV3(event, batch) {
			return errors.New("accepted final delivery assistant event is not the closed V3 public projection")
		}
	}
	if contracts.StringField(batch.Events[0], "kind") != "item_completed" ||
		contracts.StringField(batch.Events[len(batch.Events)-2], "kind") != "usage" ||
		!strings.HasPrefix(contracts.StringField(batch.Events[len(batch.Events)-1], "kind"), "turn_") {
		return errors.New("accepted final delivery slots have invalid event kinds")
	}
	if len(batch.Events) == 4 && contracts.StringField(batch.Events[1], "kind") != "item_completed" {
		return errors.New("accepted final terminal error slot is invalid")
	}
	if err := validateAcceptedFinalDeliveryTerminalSemanticsV2(batch); err != nil {
		return err
	}
	if acceptedFinalDeliveryManifestDigest(batch.Events) != batch.EventManifestDigest ||
		acceptedFinalDeliveryBatchIDV2(batch) != batch.BatchID {
		return errors.New("accepted final delivery batch integrity is invalid")
	}
	return nil
}

func validAcceptedFinalPublicAssistantEventV3(event map[string]any, batch AcceptedFinalDeliveryBatchV2) bool {
	item, itemOK := event["item"].(map[string]any)
	view, viewOK := item["acceptedFinalView"].(map[string]any)
	_, hasPrivateRecord := item["acceptedFinal"]
	return itemOK && viewOK && !hasPrivateRecord && validAcceptedFinalPublicViewV3Wire(view) &&
		contracts.StringField(event, "itemId") != "" && contracts.StringField(event, "itemId") == contracts.StringField(item, "id") &&
		contracts.StringField(item, "threadId") == batch.ThreadID && contracts.StringField(item, "turnId") == batch.TurnID &&
		contracts.StringField(view, "acceptedFinalDigest") == batch.PublicationCommitID &&
		contracts.StringField(view, "acceptedAt") == batch.Timestamp &&
		contracts.StringField(view, "terminalReason") == contracts.StringField(batch.Events[len(batch.Events)-1], "terminalReason")
}

var acceptedFinalPublicViewV3WireKeys = map[string]struct{}{
	"schemaVersion": {}, "acceptedFinalDigest": {}, "publicationState": {}, "variant": {},
	"terminalReason": {}, "blockerCode": {}, "coverageStatus": {}, "checkedScopeDigest": {},
	"missingScopeCount": {}, "claimCount": {}, "claimTypes": {}, "receiptMetadata": {},
	"noHitWording": {}, "acceptedAt": {},
}

var acceptedFinalPublicClaimTypesV3 = map[string]struct{}{
	"amount": {}, "count": {}, "account": {}, "entity": {}, "direction": {}, "date_range": {},
	"relationship": {}, "quote": {}, "device_identifier": {}, "ownership": {}, "control": {},
	"address": {}, "change": {}, "bid_certificate": {}, "bid_edit_metadata": {}, "legal_characterization": {},
}

// validAcceptedFinalPublicViewV3Wire is the closed transport validator owned by
// the Batch V2 boundary. The evidence package separately derives this view from
// the private signed V5 record; this check prevents a durable or projected event
// from adding private V2/V5 fields before the existing delivery authority seals
// it for generic HTTP/SSE.
func validAcceptedFinalPublicViewV3Wire(view map[string]any) bool {
	if len(view) != len(acceptedFinalPublicViewV3WireKeys) {
		return false
	}
	for key := range view {
		if _, allowed := acceptedFinalPublicViewV3WireKeys[key]; !allowed {
			return false
		}
	}
	for _, key := range []string{
		"acceptedFinalDigest", "publicationState", "variant", "terminalReason", "blockerCode",
		"coverageStatus", "checkedScopeDigest", "noHitWording", "acceptedAt",
	} {
		if _, ok := view[key].(string); !ok {
			return false
		}
	}
	schemaVersion, schemaOK := contracts.NumericSeq(view["schemaVersion"])
	missingScopeCount, missingOK := acceptedFinalPublicSafeCountV3(view["missingScopeCount"])
	claimCount, claimCountOK := acceptedFinalPublicSafeCountV3(view["claimCount"])
	acceptedAt := contracts.StringField(view, "acceptedAt")
	parsedAcceptedAt, acceptedAtErr := time.Parse(time.RFC3339Nano, acceptedAt)
	if !schemaOK || schemaVersion != 3 || !missingOK || !claimCountOK ||
		contracts.StringField(view, "publicationState") != "accepted" ||
		!domainsecurity.IsSHA256Hex(contracts.StringField(view, "acceptedFinalDigest")) ||
		acceptedAtErr != nil || parsedAcceptedAt.Location() != time.UTC ||
		parsedAcceptedAt.UTC().Format(time.RFC3339Nano) != acceptedAt ||
		!acceptedFinalPublicBlockerCodeV3(contracts.StringField(view, "blockerCode")) {
		return false
	}
	checkedScopeDigest := contracts.StringField(view, "checkedScopeDigest")
	if checkedScopeDigest != "" && !domainsecurity.IsSHA256Hex(checkedScopeDigest) {
		return false
	}
	claimTypes, ok := acceptedFinalPublicStringSliceV3(view["claimTypes"])
	if !ok || !acceptedFinalPublicClaimTypesAreCanonicalV3(claimTypes, claimCount) {
		return false
	}
	receiptCount, ok := acceptedFinalPublicReceiptMetadataIsCanonicalV3(view["receiptMetadata"])
	if !ok {
		return false
	}
	variant := contracts.StringField(view, "variant")
	coverageStatus := contracts.StringField(view, "coverageStatus")
	blockerCode := contracts.StringField(view, "blockerCode")
	noHitWording := contracts.StringField(view, "noHitWording")
	hasCheckedScope := checkedScopeDigest != ""
	switch variant {
	case "EvidenceBackedAnswer":
		return coverageStatus == "complete" && claimCount > 0 && receiptCount > 0 && hasCheckedScope &&
			missingScopeCount == 0 && blockerCode == "" && noHitWording == ""
	case "PartialEvidenceAnswer":
		return coverageStatus == "partial" && claimCount > 0 && receiptCount > 0 && hasCheckedScope &&
			missingScopeCount > 0 && blockerCode == "" && noHitWording == ""
	case "VerifiedNoHitAnswer":
		return coverageStatus == "complete" && claimCount == 0 && receiptCount > 0 && hasCheckedScope &&
			missingScopeCount == 0 && blockerCode == "" && noHitWording == "not_found_in_checked_scope"
	case "SourceUnavailableAnswer":
		return coverageStatus == "unavailable" && claimCount == 0 && receiptCount == 0 &&
			blockerCode != "" && noHitWording == ""
	case "NeedsEvidenceAnswer":
		return coverageStatus == "unverified" && claimCount == 0 && receiptCount == 0 &&
			missingScopeCount > 0 && noHitWording == ""
	case "GeneralGuidanceAnswer":
		return coverageStatus == "guidance_only" && claimCount == 0 && receiptCount == 0 &&
			!hasCheckedScope && missingScopeCount == 0 && blockerCode == "" && noHitWording == ""
	default:
		return false
	}
}

func acceptedFinalPublicSafeCountV3(value any) (int, bool) {
	count, ok := contracts.NumericSeq(value)
	return count, ok && count >= 0 && uint64(count) <= uint64(1<<53-1)
}

func acceptedFinalPublicBlockerCodeV3(value string) bool {
	if len(value) > 96 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') &&
			character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func acceptedFinalPublicStringSliceV3(value any) ([]string, bool) {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...), true
	case []any:
		out := make([]string, 0, len(typed))
		for _, entry := range typed {
			text, ok := entry.(string)
			if !ok {
				return nil, false
			}
			out = append(out, text)
		}
		return out, true
	default:
		return nil, false
	}
}

func acceptedFinalPublicClaimTypesAreCanonicalV3(claimTypes []string, claimCount int) bool {
	if claimCount == 0 && len(claimTypes) != 0 || claimCount > 0 && (len(claimTypes) == 0 || len(claimTypes) > claimCount) {
		return false
	}
	previous := ""
	for _, claimType := range claimTypes {
		if claimType <= previous {
			return false
		}
		if _, allowed := acceptedFinalPublicClaimTypesV3[claimType]; !allowed {
			return false
		}
		previous = claimType
	}
	return true
}

func acceptedFinalPublicReceiptMetadataIsCanonicalV3(value any) (int, bool) {
	metadata, ok := value.(map[string]any)
	if !ok || len(metadata) != 4 || contracts.StringField(metadata, "projection") != "masked_metadata_only" ||
		!domainsecurity.IsSHA256Hex(contracts.StringField(metadata, "setDigest")) {
		return 0, false
	}
	for _, key := range []string{"projection", "count", "setDigest", "citations"} {
		if _, present := metadata[key]; !present {
			return 0, false
		}
	}
	count, countOK := acceptedFinalPublicSafeCountV3(metadata["count"])
	citations, citationsOK := metadata["citations"].([]any)
	if !citationsOK {
		if typed, ok := metadata["citations"].([]map[string]any); ok {
			citations = make([]any, 0, len(typed))
			for _, citation := range typed {
				citations = append(citations, citation)
			}
			citationsOK = true
		}
	}
	if !countOK || !citationsOK || len(citations) != count {
		return 0, false
	}
	seen := make(map[string]struct{}, len(citations))
	for index, value := range citations {
		citation, ok := value.(map[string]any)
		handle := contracts.StringField(citation, "handle")
		if !ok || len(citation) != 2 || citation["handle"] == nil || citation["label"] == nil ||
			len(handle) != len("cite_")+64 || !strings.HasPrefix(handle, "cite_") ||
			!domainsecurity.IsSHA256Hex(strings.TrimPrefix(handle, "cite_")) ||
			contracts.StringField(citation, "label") != "evidence-"+strconv.Itoa(index+1) {
			return 0, false
		}
		if _, duplicate := seen[handle]; duplicate {
			return 0, false
		}
		seen[handle] = struct{}{}
	}
	return count, true
}

func validAcceptedFinalUsageEffortV1(event map[string]any) bool {
	raw, present := event["effort"]
	if !present {
		return true
	}
	effort, ok := raw.(string)
	canonical, valid := domainmodel.ProjectReasoningEffortV1(effort)
	return ok && valid && canonical != "" && canonical == effort
}

func validateAcceptedFinalDeliveryTerminalSemanticsV2(batch AcceptedFinalDeliveryBatchV2) error {
	terminalEvent := batch.Events[len(batch.Events)-1]
	terminalReason := strings.TrimSpace(contracts.StringField(terminalEvent, "terminalReason"))
	expectedStatus, ok := domainterminal.StatusForReasonV1(terminalReason)
	requiresErrorItem, profileOK := domainterminal.AcceptedFinalDeliveryRequiresErrorItemV1(terminalReason)
	if !ok || !profileOK || contracts.StringField(terminalEvent, "kind") != "turn_"+expectedStatus ||
		contracts.StringField(terminalEvent, "status") != expectedStatus ||
		contracts.StringField(batch.Events[len(batch.Events)-2], "usageFinalStatus") != expectedStatus ||
		(len(batch.Events) == 4) != requiresErrorItem {
		return errors.New("accepted final delivery terminal profile is invalid")
	}
	if expectedStatus == "aborted" {
		_, discardOK := terminalEvent["discard"].(bool)
		cancelled, cancelledOK := terminalEvent["cancelled"].(bool)
		cancelledPendingGates, countOK := contracts.NumericSeq(terminalEvent["cancelledPendingGates"])
		if !discardOK || !cancelledOK || !countOK || cancelledPendingGates < 0 ||
			(cancelledPendingGates > 0 && !cancelled) {
			return errors.New("accepted final aborted terminal disposition is incomplete")
		}
	} else if hasAnyAcceptedFinalDeliveryField(terminalEvent, "discard", "cancelled", "cancelledPendingGates") {
		return errors.New("accepted final non-aborted terminal carries abort disposition")
	}
	if expectedStatus != "failed" && hasAnyAcceptedFinalDeliveryField(terminalEvent, "error", "message") {
		return errors.New("accepted final non-failed terminal carries failure text")
	}
	if !requiresErrorItem {
		if _, ok := terminalEvent["itemId"]; ok {
			return errors.New("accepted final three-slot terminal carries an error item identity")
		}
		return nil
	}

	errorEvent := batch.Events[1]
	errorItem, itemOK := errorEvent["item"].(map[string]any)
	errorItemID := strings.TrimSpace(contracts.StringField(errorItem, "id"))
	terminalCode := strings.TrimSpace(contracts.StringField(terminalEvent, "code"))
	itemMessage := strings.TrimSpace(contracts.StringField(errorItem, "message"))
	expectedSeverity := "warning"
	if expectedStatus == "failed" {
		expectedSeverity = "error"
	}
	if !itemOK || errorItemID == "" || contracts.StringField(errorEvent, "itemId") != errorItemID ||
		contracts.StringField(terminalEvent, "itemId") != errorItemID ||
		contracts.StringField(errorItem, "threadId") != batch.ThreadID ||
		contracts.StringField(errorItem, "turnId") != batch.TurnID ||
		contracts.StringField(errorItem, "role") != "system" || contracts.StringField(errorItem, "kind") != "error" ||
		contracts.StringField(errorItem, "status") != expectedStatus ||
		contracts.StringField(errorItem, "acceptedFinalDigest") != batch.PublicationCommitID ||
		contracts.StringField(errorItem, "createdAt") != batch.Timestamp ||
		contracts.StringField(errorItem, "finishedAt") != batch.Timestamp ||
		terminalCode == "" || contracts.StringField(errorItem, "code") != terminalCode || itemMessage == "" ||
		contracts.StringField(errorItem, "severity") != expectedSeverity {
		return errors.New("accepted final terminal error item binding is invalid")
	}
	if expectedStatus == "failed" && (contracts.StringField(terminalEvent, "error") != itemMessage ||
		contracts.StringField(terminalEvent, "message") != itemMessage) {
		return errors.New("accepted final failed terminal text is detached from its error item")
	}
	return nil
}

func hasAnyAcceptedFinalDeliveryField(record map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := record[key]; ok {
			return true
		}
	}
	return false
}

func AcceptedFinalDeliverySequencedEventsDigest(events []map[string]any) string {
	return acceptedFinalDeliverySequencedEventsDigest(events)
}

// AcceptedFinalReplayEventsAfter rewinds a cursor that lands inside a valid
// accepted-final manifest. It never returns a suffix of that manifest.
func AcceptedFinalReplayEventsAfter(events []map[string]any, afterSeq int) ([]map[string]any, error) {
	effectiveAfter := afterSeq
	for start := 0; start < len(events); {
		commitID := strings.TrimSpace(contracts.StringField(events[start], "publicationCommitId"))
		if commitID == "" {
			start++
			continue
		}
		end := start + 1
		for end < len(events) && strings.TrimSpace(contracts.StringField(events[end], "publicationCommitId")) == commitID {
			end++
		}
		batch, err := newAcceptedFinalDeliveryBatchContentV2(events[start:end])
		if err != nil {
			return nil, err
		}
		if batch.FirstSeq <= afterSeq && afterSeq < batch.LastSeq {
			effectiveAfter = batch.FirstSeq - 1
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

func acceptedFinalDeliverySequencedEventsDigest(events []map[string]any) string {
	body, err := json.Marshal(events)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func AcceptedFinalDeliveryBatchV1Map(batch AcceptedFinalDeliveryBatchV1) map[string]any {
	if ValidateAcceptedFinalDeliveryBatchV1(batch) != nil {
		return nil
	}
	body, _ := json.Marshal(batch)
	var out map[string]any
	if json.Unmarshal(body, &out) != nil {
		return nil
	}
	return out
}

func ParseAcceptedFinalDeliveryBatchV1(value map[string]any) (AcceptedFinalDeliveryBatchV1, error) {
	if value == nil {
		return AcceptedFinalDeliveryBatchV1{}, errors.New("historical accepted final delivery batch is unavailable")
	}
	body, err := json.Marshal(value)
	if err != nil {
		return AcceptedFinalDeliveryBatchV1{}, err
	}
	var batch AcceptedFinalDeliveryBatchV1
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&batch); err != nil {
		return AcceptedFinalDeliveryBatchV1{}, err
	}
	if err := ValidateAcceptedFinalDeliveryBatchV1(batch); err != nil {
		return AcceptedFinalDeliveryBatchV1{}, err
	}
	canonical := AcceptedFinalDeliveryBatchV1Map(batch)
	if canonical == nil || !reflect.DeepEqual(canonical, value) {
		return AcceptedFinalDeliveryBatchV1{}, errors.New("historical accepted final delivery batch is not canonical")
	}
	return batch, nil
}

func AcceptedFinalDeliveryBatchV2Map(batch AcceptedFinalDeliveryBatchV2) map[string]any {
	if ValidateAcceptedFinalDeliveryBatchV2(batch) != nil {
		return nil
	}
	body, _ := json.Marshal(batch)
	var out map[string]any
	if json.Unmarshal(body, &out) != nil {
		return nil
	}
	return out
}

func ParseAcceptedFinalDeliveryBatchV2(value map[string]any) (AcceptedFinalDeliveryBatchV2, error) {
	if value == nil {
		return AcceptedFinalDeliveryBatchV2{}, errors.New("accepted final delivery batch is unavailable")
	}
	body, err := json.Marshal(value)
	if err != nil {
		return AcceptedFinalDeliveryBatchV2{}, err
	}
	var batch AcceptedFinalDeliveryBatchV2
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&batch); err != nil {
		return AcceptedFinalDeliveryBatchV2{}, err
	}
	if err := ValidateAcceptedFinalDeliveryBatchV2(batch); err != nil {
		return AcceptedFinalDeliveryBatchV2{}, err
	}
	canonical := AcceptedFinalDeliveryBatchV2Map(batch)
	if canonical == nil || !reflect.DeepEqual(canonical, value) {
		return AcceptedFinalDeliveryBatchV2{}, errors.New("accepted final delivery batch is not canonical")
	}
	return batch, nil
}

func cloneAcceptedFinalDeliveryEvents(events []map[string]any) []map[string]any {
	cloned := make([]map[string]any, 0, len(events))
	for _, event := range events {
		cloned = append(cloned, contracts.CloneMap(event))
	}
	return cloned
}

func acceptedFinalDeliveryPayloadDigest(event map[string]any) string {
	canonical := contracts.CloneMap(event)
	delete(canonical, "seq")
	delete(canonical, "publicationPayloadDigest")
	body, err := json.Marshal(canonical)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func acceptedFinalDeliveryEventID(commitID, slot string) string {
	digest := sha256.Sum256([]byte("analytix.accepted-final-event/v1\x00" + commitID + "\x00" + slot))
	return hex.EncodeToString(digest[:])
}

func acceptedFinalDeliveryManifestDigest(events []map[string]any) string {
	manifest := make([]map[string]any, 0, len(events))
	for _, event := range events {
		manifest = append(manifest, map[string]any{
			"slot":          strings.TrimSpace(contracts.StringField(event, "publicationSlot")),
			"eventId":       strings.TrimSpace(contracts.StringField(event, "publicationEventId")),
			"payloadDigest": strings.TrimSpace(contracts.StringField(event, "publicationPayloadDigest")),
		})
	}
	body, _ := json.Marshal(manifest)
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func acceptedFinalDeliveryBatchIDV2(batch AcceptedFinalDeliveryBatchV2) string {
	body, _ := json.Marshal(struct {
		Purpose             string `json:"purpose"`
		ThreadID            string `json:"threadId"`
		TurnID              string `json:"turnId"`
		FirstSeq            int    `json:"firstSeq"`
		LastSeq             int    `json:"lastSeq"`
		PublicationCommitID string `json:"publicationCommitId"`
		EventManifestDigest string `json:"eventManifestDigest"`
	}{batch.Purpose, batch.ThreadID, batch.TurnID, batch.FirstSeq, batch.LastSeq, batch.PublicationCommitID, batch.EventManifestDigest})
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func acceptedFinalDeliveryBatchIDV1(batch AcceptedFinalDeliveryBatchV1) string {
	body, _ := json.Marshal(struct {
		Purpose             string `json:"purpose"`
		ThreadID            string `json:"threadId"`
		TurnID              string `json:"turnId"`
		FirstSeq            int    `json:"firstSeq"`
		LastSeq             int    `json:"lastSeq"`
		PublicationCommitID string `json:"publicationCommitId"`
		EventManifestDigest string `json:"eventManifestDigest"`
	}{batch.Purpose, batch.ThreadID, batch.TurnID, batch.FirstSeq, batch.LastSeq, batch.PublicationCommitID, batch.EventManifestDigest})
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
