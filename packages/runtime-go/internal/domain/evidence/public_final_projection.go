package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	AcceptedFinalPublicViewVersion     = 1
	AcceptedFinalPublicViewCoreVersion = 2
	AcceptedFinalPublicViewV2Version   = 2
	AcceptedFinalPublicViewV3Version   = 3
	AcceptedFinalPublicationAccepted   = "accepted"
	AcceptedFinalReceiptProjection     = "masked_metadata_only"
	AcceptedFinalBlockerCodeRedacted   = "redacted_blocker"
	AcceptedFinalCoverageComplete      = "complete"
	AcceptedFinalCoveragePartial       = "partial"
	AcceptedFinalCoverageUnavailable   = "unavailable"
	AcceptedFinalCoverageUnverified    = "unverified"
	AcceptedFinalCoverageGuidanceOnly  = "guidance_only"
	acceptedFinalMaxSafeIntegerV2      = uint64(1<<53 - 1)
)

var acceptedFinalPublicViewV2DigestDomain = []byte("analytix.accepted-final-public-view/v2\x00")

type AcceptedFinalPublicCitationV1 struct {
	Handle string `json:"handle"`
	Label  string `json:"label"`
}

type AcceptedFinalPublicReceiptMetadataV1 struct {
	Projection string                          `json:"projection"`
	Count      int                             `json:"count"`
	SetDigest  string                          `json:"setDigest"`
	Citations  []AcceptedFinalPublicCitationV1 `json:"citations"`
}

// AcceptedFinalPublicViewV1 is non-authoritative display metadata rebuilt
// only from a trusted private accepted final. Raw claims, receipt ids, query
// fields, source records, complete PII, provider assertions, and reasoning are
// deliberately absent. AcceptedFinalRecord remains the signed authority.
type AcceptedFinalPublicViewV1 struct {
	SchemaVersion       int                                  `json:"schemaVersion"`
	PublicationState    string                               `json:"publicationState"`
	AcceptedFinalDigest string                               `json:"acceptedFinalDigest"`
	EnvelopeDigest      string                               `json:"envelopeDigest"`
	ContextDigest       string                               `json:"contextDigest"`
	ContextEpoch        uint64                               `json:"contextEpoch"`
	DatasetSnapshotID   string                               `json:"datasetSnapshotId"`
	Variant             FinalAnswerVariant                   `json:"variant"`
	TerminalReason      string                               `json:"terminalReason"`
	BlockerCode         string                               `json:"blockerCode"`
	CoverageStatus      string                               `json:"coverageStatus"`
	CheckedScopeDigest  string                               `json:"checkedScopeDigest"`
	MissingScopeCount   int                                  `json:"missingScopeCount"`
	ClaimCount          int                                  `json:"claimCount"`
	ClaimTypes          []string                             `json:"claimTypes"`
	ReceiptMetadata     AcceptedFinalPublicReceiptMetadataV1 `json:"receiptMetadata"`
	NoHitWording        string                               `json:"noHitWording"`
	EnvelopeIssuedAt    string                               `json:"envelopeIssuedAt"`
	AcceptedAt          string                               `json:"acceptedAt"`
}

// AcceptedFinalPublicViewCoreV2 is embedded in AcceptedFinalRecord V5 and is
// therefore covered by the installation-authority signature. It excludes the
// accepted-final record digest to avoid a signing cycle.
type AcceptedFinalPublicViewCoreV2 struct {
	SchemaVersion      int                                  `json:"schemaVersion"`
	PublicationState   string                               `json:"publicationState"`
	EnvelopeDigest     string                               `json:"envelopeDigest"`
	ContextDigest      string                               `json:"contextDigest"`
	ContextEpoch       uint64                               `json:"contextEpoch"`
	DatasetSnapshotID  string                               `json:"datasetSnapshotId"`
	Variant            FinalAnswerVariant                   `json:"variant"`
	TerminalReason     string                               `json:"terminalReason"`
	BlockerCode        string                               `json:"blockerCode"`
	CoverageStatus     string                               `json:"coverageStatus"`
	CheckedScopeDigest string                               `json:"checkedScopeDigest"`
	MissingScopeCount  int                                  `json:"missingScopeCount"`
	ClaimCount         int                                  `json:"claimCount"`
	ClaimTypes         []string                             `json:"claimTypes"`
	ReceiptMetadata    AcceptedFinalPublicReceiptMetadataV1 `json:"receiptMetadata"`
	NoHitWording       string                               `json:"noHitWording"`
	EnvelopeIssuedAt   string                               `json:"envelopeIssuedAt"`
	AcceptedAt         string                               `json:"acceptedAt"`
}

// AcceptedFinalPublicViewV2 is the exact private/legacy expansion of the signed
// V5 core. Its wire shape remains frozen; generic HTTP/SSE uses the closed V3
// projection below.
type AcceptedFinalPublicViewV2 struct {
	SchemaVersion       int                                  `json:"schemaVersion"`
	PublicationState    string                               `json:"publicationState"`
	AcceptedFinalDigest string                               `json:"acceptedFinalDigest"`
	PublicViewDigest    string                               `json:"publicViewDigest"`
	EnvelopeDigest      string                               `json:"envelopeDigest"`
	ContextDigest       string                               `json:"contextDigest"`
	ContextEpoch        uint64                               `json:"contextEpoch"`
	DatasetSnapshotID   string                               `json:"datasetSnapshotId"`
	Variant             FinalAnswerVariant                   `json:"variant"`
	TerminalReason      string                               `json:"terminalReason"`
	BlockerCode         string                               `json:"blockerCode"`
	CoverageStatus      string                               `json:"coverageStatus"`
	CheckedScopeDigest  string                               `json:"checkedScopeDigest"`
	MissingScopeCount   int                                  `json:"missingScopeCount"`
	ClaimCount          int                                  `json:"claimCount"`
	ClaimTypes          []string                             `json:"claimTypes"`
	ReceiptMetadata     AcceptedFinalPublicReceiptMetadataV1 `json:"receiptMetadata"`
	NoHitWording        string                               `json:"noHitWording"`
	EnvelopeIssuedAt    string                               `json:"envelopeIssuedAt"`
	AcceptedAt          string                               `json:"acceptedAt"`
}

// AcceptedFinalPublicViewV3 is the closed generic HTTP/SSE projection. It
// retains the accepted PII-free semantic and masked-citation fields while
// excluding the signed private record, context/case binding, dataset/source
// identity, receipt values, and publication/witness authority material.
// AcceptedFinalRecord remains private durable/CAS authority.
type AcceptedFinalPublicViewV3 struct {
	SchemaVersion       int                                  `json:"schemaVersion"`
	AcceptedFinalDigest string                               `json:"acceptedFinalDigest"`
	PublicationState    string                               `json:"publicationState"`
	Variant             FinalAnswerVariant                   `json:"variant"`
	TerminalReason      string                               `json:"terminalReason"`
	BlockerCode         string                               `json:"blockerCode"`
	CoverageStatus      string                               `json:"coverageStatus"`
	CheckedScopeDigest  string                               `json:"checkedScopeDigest"`
	MissingScopeCount   int                                  `json:"missingScopeCount"`
	ClaimCount          int                                  `json:"claimCount"`
	ClaimTypes          []string                             `json:"claimTypes"`
	ReceiptMetadata     AcceptedFinalPublicReceiptMetadataV1 `json:"receiptMetadata"`
	NoHitWording        string                               `json:"noHitWording"`
	AcceptedAt          string                               `json:"acceptedAt"`
}

func NewAcceptedFinalPublicViewV2(record PrivateAcceptedFinalRecord) (AcceptedFinalPublicViewV2, error) {
	if ValidatePrivateAcceptedFinalPublicationAuthority(record) != nil ||
		ValidateAcceptedFinalPublicViewCoreV2(record.AcceptedFinal) != nil {
		return AcceptedFinalPublicViewV2{}, errors.New("accepted final public view V2 authority is invalid")
	}
	return NewAcceptedFinalPublicViewFromRecordV2(record.AcceptedFinal)
}

// NewAcceptedFinalPublicViewV2WithWitness emits a fact-bearing display view
// only after the caller has performed the operation's fresh witness replay.
func NewAcceptedFinalPublicViewV2WithWitness(record PrivateAcceptedFinalRecord, verify FactFinalWitnessReplayVerifierV1) (AcceptedFinalPublicViewV2, error) {
	if ValidatePrivateAcceptedFinalPublicationAuthorityWithWitnessV1(record, verify) != nil {
		return AcceptedFinalPublicViewV2{}, errors.New("accepted final public view V2 witness authority is invalid")
	}
	return NewAcceptedFinalPublicViewFromRecordV2(record.AcceptedFinal)
}

// NewAcceptedFinalPublicViewFromRecordV2 derives the display object from the
// signed V5 core. Installation-key trust and fresh witness replay remain
// application-layer admission duties; this function only accepts the current
// cryptographic wire contract.
func NewAcceptedFinalPublicViewFromRecordV2(record AcceptedFinalRecord) (AcceptedFinalPublicViewV2, error) {
	if ValidateAcceptedFinalForCurrentWriteV1(record) != nil || ValidateAcceptedFinalPublicViewCoreV2(record) != nil {
		return AcceptedFinalPublicViewV2{}, errors.New("accepted final public view V2 record authority is invalid")
	}
	return acceptedFinalPublicViewV2FromRecord(record), nil
}

func ValidateAcceptedFinalPublicViewV2(view AcceptedFinalPublicViewV2, authority PrivateAcceptedFinalRecord) error {
	if ValidatePrivateAcceptedFinalPublicationAuthority(authority) != nil ||
		ValidateAcceptedFinalPublicViewCoreV2(authority.AcceptedFinal) != nil {
		return errors.New("accepted final public view V2 authority is invalid")
	}
	expected := acceptedFinalPublicViewV2FromRecord(authority.AcceptedFinal)
	if !reflect.DeepEqual(view, expected) {
		return errors.New("accepted final public view V2 does not match signed authority")
	}
	return nil
}

// ValidateAcceptedFinalPublicViewV2ForRecord is the strict historical-wire
// counterpart to ValidateAcceptedFinalPublicViewV2. Batch V1 carried the
// signed AcceptedFinalRecord rather than the enclosing private CAS record, so
// its audit parser must validate the signature, the complete V5 public core,
// and the exact expanded V2 view without granting current publication
// authority.
func ValidateAcceptedFinalPublicViewV2ForRecord(view AcceptedFinalPublicViewV2, authority AcceptedFinalRecord) error {
	if ValidateAcceptedFinalRecord(authority) != nil ||
		ValidateAcceptedFinalPublicViewCoreV2(authority) != nil {
		return errors.New("historical accepted final public view V2 authority is invalid")
	}
	if !reflect.DeepEqual(view, acceptedFinalPublicViewV2FromRecord(authority)) {
		return errors.New("historical accepted final public view V2 does not match signed authority")
	}
	return nil
}

func ParseAcceptedFinalPublicViewV2(value any, authority PrivateAcceptedFinalRecord) (AcceptedFinalPublicViewV2, error) {
	view, err := parseAcceptedFinalPublicViewV2Value(value)
	if err != nil {
		return AcceptedFinalPublicViewV2{}, err
	}
	if err := ValidateAcceptedFinalPublicViewV2(view, authority); err != nil {
		return AcceptedFinalPublicViewV2{}, err
	}
	return view, nil
}

// ParseAcceptedFinalPublicViewV2ForRecord is shape-only nowhere: callers must
// supply the signed V5 record embedded in the same historical Batch V1 item.
func ParseAcceptedFinalPublicViewV2ForRecord(value any, authority AcceptedFinalRecord) (AcceptedFinalPublicViewV2, error) {
	view, err := parseAcceptedFinalPublicViewV2Value(value)
	if err != nil {
		return AcceptedFinalPublicViewV2{}, err
	}
	if err := ValidateAcceptedFinalPublicViewV2ForRecord(view, authority); err != nil {
		return AcceptedFinalPublicViewV2{}, err
	}
	return view, nil
}

func parseAcceptedFinalPublicViewV2Value(value any) (AcceptedFinalPublicViewV2, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return AcceptedFinalPublicViewV2{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var view AcceptedFinalPublicViewV2
	if err := decoder.Decode(&view); err != nil {
		return AcceptedFinalPublicViewV2{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return AcceptedFinalPublicViewV2{}, errors.New("accepted final public view V2 contains trailing JSON")
	}
	return view, nil
}

func AcceptedFinalPublicViewRecordV2(view AcceptedFinalPublicViewV2) map[string]any {
	body, _ := json.Marshal(view)
	record := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	_ = decoder.Decode(&record)
	return record
}

func NewAcceptedFinalPublicViewV3(record PrivateAcceptedFinalRecord) (AcceptedFinalPublicViewV3, error) {
	if ValidatePrivateAcceptedFinalPublicationAuthority(record) != nil ||
		ValidateAcceptedFinalPublicViewCoreV2(record.AcceptedFinal) != nil {
		return AcceptedFinalPublicViewV3{}, errors.New("accepted final public view V3 authority is invalid")
	}
	return NewAcceptedFinalPublicViewFromRecordV3(record.AcceptedFinal)
}

// NewAcceptedFinalPublicViewV3WithWitness emits a fact-bearing generic view
// only after the caller has replayed the current witness authority.
func NewAcceptedFinalPublicViewV3WithWitness(record PrivateAcceptedFinalRecord, verify FactFinalWitnessReplayVerifierV1) (AcceptedFinalPublicViewV3, error) {
	if ValidatePrivateAcceptedFinalPublicationAuthorityWithWitnessV1(record, verify) != nil {
		return AcceptedFinalPublicViewV3{}, errors.New("accepted final public view V3 witness authority is invalid")
	}
	return NewAcceptedFinalPublicViewFromRecordV3(record.AcceptedFinal)
}

// NewAcceptedFinalPublicViewFromRecordV3 is used only after the application
// layer has admitted the private signed V5 and its durable CAS/event authority.
func NewAcceptedFinalPublicViewFromRecordV3(record AcceptedFinalRecord) (AcceptedFinalPublicViewV3, error) {
	if ValidateAcceptedFinalForCurrentWriteV1(record) != nil || ValidateAcceptedFinalPublicViewCoreV2(record) != nil {
		return AcceptedFinalPublicViewV3{}, errors.New("accepted final public view V3 record authority is invalid")
	}
	view := acceptedFinalPublicViewV3FromRecord(record)
	if err := ValidateAcceptedFinalPublicViewV3Shape(view); err != nil {
		return AcceptedFinalPublicViewV3{}, err
	}
	return view, nil
}

func ValidateAcceptedFinalPublicViewV3(view AcceptedFinalPublicViewV3, authority PrivateAcceptedFinalRecord) error {
	if ValidatePrivateAcceptedFinalPublicationAuthority(authority) != nil ||
		ValidateAcceptedFinalPublicViewCoreV2(authority.AcceptedFinal) != nil ||
		ValidateAcceptedFinalPublicViewV3Shape(view) != nil {
		return errors.New("accepted final public view V3 authority is invalid")
	}
	if !reflect.DeepEqual(view, acceptedFinalPublicViewV3FromRecord(authority.AcceptedFinal)) {
		return errors.New("accepted final public view V3 does not match signed authority")
	}
	return nil
}

func ParseAcceptedFinalPublicViewV3(value any, authority PrivateAcceptedFinalRecord) (AcceptedFinalPublicViewV3, error) {
	view, err := ParseAcceptedFinalPublicViewV3Value(value)
	if err != nil {
		return AcceptedFinalPublicViewV3{}, err
	}
	if err := ValidateAcceptedFinalPublicViewV3(view, authority); err != nil {
		return AcceptedFinalPublicViewV3{}, err
	}
	return view, nil
}

func ParseAcceptedFinalPublicViewV3Value(value any) (AcceptedFinalPublicViewV3, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return AcceptedFinalPublicViewV3{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var view AcceptedFinalPublicViewV3
	if err := decoder.Decode(&view); err != nil {
		return AcceptedFinalPublicViewV3{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return AcceptedFinalPublicViewV3{}, errors.New("accepted final public view V3 contains trailing JSON")
	}
	if err := ValidateAcceptedFinalPublicViewV3Shape(view); err != nil {
		return AcceptedFinalPublicViewV3{}, err
	}
	return view, nil
}

func AcceptedFinalPublicViewRecordV3(view AcceptedFinalPublicViewV3) map[string]any {
	if ValidateAcceptedFinalPublicViewV3Shape(view) != nil {
		return nil
	}
	body, _ := json.Marshal(view)
	record := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	_ = decoder.Decode(&record)
	return record
}

func acceptedFinalPublicViewV3FromRecord(record AcceptedFinalRecord) AcceptedFinalPublicViewV3 {
	core := *record.PublicView
	return AcceptedFinalPublicViewV3{
		SchemaVersion: AcceptedFinalPublicViewV3Version, AcceptedFinalDigest: record.RecordDigest,
		PublicationState: core.PublicationState,
		Variant:          core.Variant, TerminalReason: core.TerminalReason, BlockerCode: core.BlockerCode,
		CoverageStatus: core.CoverageStatus, CheckedScopeDigest: core.CheckedScopeDigest,
		MissingScopeCount: core.MissingScopeCount, ClaimCount: core.ClaimCount,
		ClaimTypes: append([]string{}, core.ClaimTypes...), ReceiptMetadata: cloneAcceptedFinalReceiptMetadataV1(core.ReceiptMetadata),
		NoHitWording: core.NoHitWording, AcceptedAt: core.AcceptedAt,
	}
}

func ValidateAcceptedFinalPublicViewV3Shape(view AcceptedFinalPublicViewV3) error {
	if view.SchemaVersion != AcceptedFinalPublicViewV3Version ||
		view.PublicationState != AcceptedFinalPublicationAccepted ||
		!validSHA256(view.AcceptedFinalDigest) ||
		view.ClaimTypes == nil || view.ReceiptMetadata.Citations == nil ||
		view.MissingScopeCount < 0 || uint64(view.MissingScopeCount) > acceptedFinalMaxSafeIntegerV2 ||
		view.ClaimCount < 0 || uint64(view.ClaimCount) > acceptedFinalMaxSafeIntegerV2 ||
		view.ReceiptMetadata.Count < 0 || uint64(view.ReceiptMetadata.Count) > acceptedFinalMaxSafeIntegerV2 ||
		view.ReceiptMetadata.Projection != AcceptedFinalReceiptProjection ||
		!validSHA256(view.ReceiptMetadata.SetDigest) ||
		len(view.ReceiptMetadata.Citations) != view.ReceiptMetadata.Count ||
		(view.CheckedScopeDigest != "" && !validSHA256(view.CheckedScopeDigest)) ||
		boundedAcceptedFinalBlockerCode(view.BlockerCode) != view.BlockerCode {
		return errors.New("accepted final public view V3 is invalid")
	}
	acceptedAt, acceptedErr := time.Parse(time.RFC3339Nano, view.AcceptedAt)
	if acceptedErr != nil || acceptedAt.Location() != time.UTC ||
		acceptedAt.UTC().Format(time.RFC3339Nano) != view.AcceptedAt {
		return errors.New("accepted final public view V3 timestamp is invalid")
	}
	if err := validateAcceptedFinalPublicClaimTypesV2(view.ClaimTypes, view.ClaimCount); err != nil {
		return err
	}
	if err := validateAcceptedFinalPublicCitationsV2(view.ReceiptMetadata.Citations); err != nil {
		return err
	}
	core := AcceptedFinalPublicViewCoreV2{
		Variant: view.Variant, BlockerCode: view.BlockerCode, CoverageStatus: view.CoverageStatus,
		CheckedScopeDigest: view.CheckedScopeDigest, MissingScopeCount: view.MissingScopeCount,
		ClaimCount: view.ClaimCount, ClaimTypes: view.ClaimTypes, ReceiptMetadata: view.ReceiptMetadata,
		NoHitWording: view.NoHitWording,
	}
	return validateAcceptedFinalPublicViewShapeV2(core)
}

func acceptedFinalPublicViewV2FromRecord(record AcceptedFinalRecord) AcceptedFinalPublicViewV2 {
	core := *record.PublicView
	return AcceptedFinalPublicViewV2{
		SchemaVersion: AcceptedFinalPublicViewV2Version, PublicationState: core.PublicationState,
		AcceptedFinalDigest: record.RecordDigest, PublicViewDigest: record.PublicViewDigest,
		EnvelopeDigest: core.EnvelopeDigest, ContextDigest: core.ContextDigest, ContextEpoch: core.ContextEpoch,
		DatasetSnapshotID: core.DatasetSnapshotID, Variant: core.Variant, TerminalReason: core.TerminalReason,
		BlockerCode: core.BlockerCode, CoverageStatus: core.CoverageStatus, CheckedScopeDigest: core.CheckedScopeDigest,
		MissingScopeCount: core.MissingScopeCount, ClaimCount: core.ClaimCount,
		ClaimTypes: append([]string{}, core.ClaimTypes...), ReceiptMetadata: cloneAcceptedFinalReceiptMetadataV1(core.ReceiptMetadata),
		NoHitWording: core.NoHitWording, EnvelopeIssuedAt: core.EnvelopeIssuedAt, AcceptedAt: core.AcceptedAt,
	}
}

func buildAcceptedFinalPublicViewCoreV2(envelope FinalAnswerEnvelope, record AcceptedFinalRecord) AcceptedFinalPublicViewCoreV2 {
	legacy := buildAcceptedFinalPublicViewFields(envelope, record)
	return AcceptedFinalPublicViewCoreV2{
		SchemaVersion: AcceptedFinalPublicViewCoreVersion, PublicationState: AcceptedFinalPublicationAccepted,
		EnvelopeDigest: record.EnvelopeDigest, ContextDigest: record.ContextDigest, ContextEpoch: record.ContextEpoch,
		DatasetSnapshotID: record.DatasetSnapshotID, Variant: record.Variant, TerminalReason: record.TerminalReason,
		BlockerCode: legacy.BlockerCode, CoverageStatus: legacy.CoverageStatus, CheckedScopeDigest: legacy.CheckedScopeDigest,
		MissingScopeCount: legacy.MissingScopeCount, ClaimCount: legacy.ClaimCount,
		ClaimTypes: append([]string{}, legacy.ClaimTypes...), ReceiptMetadata: cloneAcceptedFinalReceiptMetadataV1(legacy.ReceiptMetadata),
		NoHitWording: legacy.NoHitWording, EnvelopeIssuedAt: legacy.EnvelopeIssuedAt, AcceptedAt: record.AcceptedAt,
	}
}

func acceptedFinalPublicViewCoreV2Digest(core AcceptedFinalPublicViewCoreV2) string {
	body, _ := json.Marshal(core)
	payload := make([]byte, 0, len(acceptedFinalPublicViewV2DigestDomain)+len(body))
	payload = append(payload, acceptedFinalPublicViewV2DigestDomain...)
	payload = append(payload, body...)
	return domainsecurity.SHA256Hex(payload)
}

// ValidateAcceptedFinalPublicViewCoreV2 validates only the PII-free material
// carried by the signed V5 record. Exact reconstruction from the private
// envelope is a separate private-authority check; this validator ensures the
// public record cannot smuggle a non-canonical or cross-runtime-ambiguous view.
func ValidateAcceptedFinalPublicViewCoreV2(record AcceptedFinalRecord) error {
	if record.SchemaVersion != AcceptedFinalRecordVersion || record.PublicView == nil ||
		!validSHA256(record.PublicViewDigest) {
		return errors.New("accepted final V5 public view is incomplete")
	}
	core := *record.PublicView
	if core.SchemaVersion != AcceptedFinalPublicViewCoreVersion ||
		core.PublicationState != AcceptedFinalPublicationAccepted ||
		core.EnvelopeDigest != record.EnvelopeDigest || core.ContextDigest != record.ContextDigest ||
		core.ContextEpoch != record.ContextEpoch || core.ContextEpoch == 0 || core.ContextEpoch > acceptedFinalMaxSafeIntegerV2 ||
		core.DatasetSnapshotID != record.DatasetSnapshotID || core.Variant != record.Variant ||
		core.TerminalReason != record.TerminalReason || core.AcceptedAt != record.AcceptedAt ||
		core.ClaimTypes == nil || core.ReceiptMetadata.Citations == nil ||
		core.MissingScopeCount < 0 || uint64(core.MissingScopeCount) > acceptedFinalMaxSafeIntegerV2 ||
		core.ClaimCount < 0 || uint64(core.ClaimCount) > acceptedFinalMaxSafeIntegerV2 ||
		core.ReceiptMetadata.Count < 0 || uint64(core.ReceiptMetadata.Count) > acceptedFinalMaxSafeIntegerV2 ||
		core.ReceiptMetadata.Projection != AcceptedFinalReceiptProjection ||
		!validSHA256(core.ReceiptMetadata.SetDigest) ||
		len(core.ReceiptMetadata.Citations) != core.ReceiptMetadata.Count ||
		(core.CheckedScopeDigest != "" && !validSHA256(core.CheckedScopeDigest)) ||
		boundedAcceptedFinalBlockerCode(core.BlockerCode) != core.BlockerCode ||
		acceptedFinalCoverageStatus(core.Variant) != core.CoverageStatus ||
		acceptedFinalPublicViewCoreV2Digest(core) != record.PublicViewDigest {
		return errors.New("accepted final V5 public view binding is invalid")
	}
	issuedAt, issuedErr := time.Parse(time.RFC3339Nano, core.EnvelopeIssuedAt)
	acceptedAt, acceptedErr := time.Parse(time.RFC3339Nano, core.AcceptedAt)
	if issuedErr != nil || acceptedErr != nil ||
		issuedAt.Location() != time.UTC || issuedAt.UTC().Format(time.RFC3339Nano) != core.EnvelopeIssuedAt ||
		acceptedAt.Location() != time.UTC || acceptedAt.UTC().Format(time.RFC3339Nano) != core.AcceptedAt ||
		acceptedAt.Before(issuedAt) {
		return errors.New("accepted final V5 public view timestamp is invalid")
	}
	if err := validateAcceptedFinalPublicClaimTypesV2(core.ClaimTypes, core.ClaimCount); err != nil {
		return err
	}
	if err := validateAcceptedFinalPublicCitationsV2(core.ReceiptMetadata.Citations); err != nil {
		return err
	}
	if err := validateAcceptedFinalPublicViewShapeV2(core); err != nil {
		return err
	}
	if record.FactFinalWitnessAdmission != nil &&
		(record.FactFinalWitnessAdmission.EvidenceReceiptCount != uint64(core.ReceiptMetadata.Count) ||
			core.ReceiptMetadata.Count == 0) {
		return errors.New("accepted final V5 public receipt count does not match witness authority")
	}
	return nil
}

func validateAcceptedFinalPublicClaimTypesV2(claimTypes []string, claimCount int) error {
	if (claimCount == 0 && len(claimTypes) != 0) || (claimCount > 0 && (len(claimTypes) == 0 || len(claimTypes) > claimCount)) {
		return errors.New("accepted final V5 public claim metadata is invalid")
	}
	previous := ""
	for _, value := range claimTypes {
		if value == "" || value <= previous || !validClaimType(ClaimType(value)) {
			return errors.New("accepted final V5 public claim types are not canonical")
		}
		previous = value
	}
	return nil
}

func validateAcceptedFinalPublicCitationsV2(citations []AcceptedFinalPublicCitationV1) error {
	seen := make(map[string]struct{}, len(citations))
	for index, citation := range citations {
		expectedLabel := "evidence-" + strconv.Itoa(index+1)
		if citation.Label != expectedLabel || len(citation.Handle) != len("cite_")+64 ||
			!strings.HasPrefix(citation.Handle, "cite_") || !validSHA256(strings.TrimPrefix(citation.Handle, "cite_")) {
			return errors.New("accepted final V5 public citation metadata is invalid")
		}
		if _, exists := seen[citation.Handle]; exists {
			return errors.New("accepted final V5 public citation metadata is duplicated")
		}
		seen[citation.Handle] = struct{}{}
	}
	return nil
}

func validateAcceptedFinalPublicViewShapeV2(core AcceptedFinalPublicViewCoreV2) error {
	if acceptedFinalCoverageStatus(core.Variant) != core.CoverageStatus {
		return errors.New("accepted final V5 public coverage binding is invalid")
	}
	claimCount := core.ClaimCount
	receiptCount := core.ReceiptMetadata.Count
	hasCheckedScope := core.CheckedScopeDigest != ""
	switch core.Variant {
	case EvidenceBackedAnswer:
		if claimCount == 0 || receiptCount == 0 || !hasCheckedScope || core.MissingScopeCount != 0 || core.BlockerCode != "" || core.NoHitWording != "" {
			return errors.New("accepted final V5 evidence-backed public view shape is invalid")
		}
	case PartialEvidenceAnswer:
		if claimCount == 0 || receiptCount == 0 || !hasCheckedScope || core.MissingScopeCount == 0 || core.BlockerCode != "" || core.NoHitWording != "" {
			return errors.New("accepted final V5 partial public view shape is invalid")
		}
	case VerifiedNoHitAnswer:
		if claimCount != 0 || receiptCount == 0 || !hasCheckedScope || core.MissingScopeCount != 0 || core.BlockerCode != "" || core.NoHitWording != VerifiedNoHitWording {
			return errors.New("accepted final V5 no-hit public view shape is invalid")
		}
	case SourceUnavailableAnswer:
		if claimCount != 0 || receiptCount != 0 || core.BlockerCode == "" || core.NoHitWording != "" {
			return errors.New("accepted final V5 source-unavailable public view shape is invalid")
		}
	case NeedsEvidenceAnswer:
		if claimCount != 0 || receiptCount != 0 || core.MissingScopeCount == 0 || core.NoHitWording != "" {
			return errors.New("accepted final V5 needs-evidence public view shape is invalid")
		}
	case GeneralGuidanceAnswer:
		if claimCount != 0 || receiptCount != 0 || hasCheckedScope || core.MissingScopeCount != 0 || core.BlockerCode != "" || core.NoHitWording != "" {
			return errors.New("accepted final V5 guidance public view shape is invalid")
		}
	default:
		return errors.New("accepted final V5 public view variant is invalid")
	}
	return nil
}

func cloneAcceptedFinalReceiptMetadataV1(input AcceptedFinalPublicReceiptMetadataV1) AcceptedFinalPublicReceiptMetadataV1 {
	return AcceptedFinalPublicReceiptMetadataV1{
		Projection: input.Projection, Count: input.Count, SetDigest: input.SetDigest,
		Citations: append([]AcceptedFinalPublicCitationV1{}, input.Citations...),
	}
}

func NewAcceptedFinalPublicViewV1(record PrivateAcceptedFinalRecord) (AcceptedFinalPublicViewV1, error) {
	_ = record
	return AcceptedFinalPublicViewV1{}, errors.New("accepted final public view V1 is audit-only")
}

func ParseAcceptedFinalPublicViewV1(value any, authority PrivateAcceptedFinalRecord) (AcceptedFinalPublicViewV1, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return AcceptedFinalPublicViewV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var view AcceptedFinalPublicViewV1
	if err := decoder.Decode(&view); err != nil {
		return AcceptedFinalPublicViewV1{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return AcceptedFinalPublicViewV1{}, errors.New("accepted final public view contains trailing JSON")
	}
	if err := ValidateAcceptedFinalPublicViewV1(view, authority); err != nil {
		return AcceptedFinalPublicViewV1{}, err
	}
	return view, nil
}

func ValidateAcceptedFinalPublicViewV1(view AcceptedFinalPublicViewV1, authority PrivateAcceptedFinalRecord) error {
	_, _ = view, authority
	return errors.New("accepted final public view V1 is audit-only")
}

func AcceptedFinalPublicViewRecordV1(view AcceptedFinalPublicViewV1) map[string]any {
	body, _ := json.Marshal(view)
	record := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	_ = decoder.Decode(&record)
	return record
}

func buildAcceptedFinalPublicViewV1(record PrivateAcceptedFinalRecord) AcceptedFinalPublicViewV1 {
	return buildAcceptedFinalPublicViewFields(record.Envelope, record.AcceptedFinal)
}

func buildAcceptedFinalPublicViewFields(envelope FinalAnswerEnvelope, acceptedFinal AcceptedFinalRecord) AcceptedFinalPublicViewV1 {
	receiptIDs := append([]string(nil), envelope.EvidenceReceiptIDs...)
	sort.Strings(receiptIDs)
	receiptBody, _ := json.Marshal(receiptIDs)
	citations := make([]AcceptedFinalPublicCitationV1, 0, len(receiptIDs))
	for index, receiptID := range receiptIDs {
		citations = append(citations, AcceptedFinalPublicCitationV1{
			Handle: "cite_" + domainsecurity.SHA256Hex([]byte("analytix.public-citation/v1\x00"+receiptID)),
			Label:  "evidence-" + strconv.Itoa(index+1),
		})
	}
	claimTypes := make([]string, 0, len(envelope.Claims))
	seenClaimTypes := map[string]bool{}
	for _, claim := range envelope.Claims {
		claimType := strings.TrimSpace(string(claim.ClaimType))
		if claimType != "" && !seenClaimTypes[claimType] {
			seenClaimTypes[claimType] = true
			claimTypes = append(claimTypes, claimType)
		}
	}
	sort.Strings(claimTypes)
	checkedScopeDigest := ""
	if envelope.CheckedScope != nil {
		body, _ := json.Marshal(envelope.CheckedScope)
		checkedScopeDigest = domainsecurity.SHA256Hex(body)
	}
	return AcceptedFinalPublicViewV1{
		SchemaVersion:       AcceptedFinalPublicViewVersion,
		PublicationState:    AcceptedFinalPublicationAccepted,
		AcceptedFinalDigest: acceptedFinal.RecordDigest,
		EnvelopeDigest:      acceptedFinal.EnvelopeDigest,
		ContextDigest:       acceptedFinal.ContextDigest,
		ContextEpoch:        acceptedFinal.ContextEpoch,
		DatasetSnapshotID:   acceptedFinal.DatasetSnapshotID,
		Variant:             acceptedFinal.Variant,
		TerminalReason:      acceptedFinal.TerminalReason,
		BlockerCode:         boundedAcceptedFinalBlockerCode(envelope.Blocker),
		CoverageStatus:      acceptedFinalCoverageStatus(envelope.Variant),
		CheckedScopeDigest:  checkedScopeDigest,
		MissingScopeCount:   len(envelope.MissingScope),
		ClaimCount:          len(envelope.Claims),
		ClaimTypes:          claimTypes,
		ReceiptMetadata: AcceptedFinalPublicReceiptMetadataV1{
			Projection: AcceptedFinalReceiptProjection, Count: len(receiptIDs),
			SetDigest: domainsecurity.SHA256Hex(receiptBody), Citations: citations,
		},
		NoHitWording:     strings.TrimSpace(envelope.NoHitWording),
		EnvelopeIssuedAt: envelope.IssuedAt,
		AcceptedAt:       acceptedFinal.AcceptedAt,
	}
}

func boundedAcceptedFinalBlockerCode(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) > 96 {
		return AcceptedFinalBlockerCodeRedacted
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' && character != '-' {
			return AcceptedFinalBlockerCodeRedacted
		}
	}
	return value
}

func acceptedFinalCoverageStatus(variant FinalAnswerVariant) string {
	switch variant {
	case EvidenceBackedAnswer, VerifiedNoHitAnswer:
		return AcceptedFinalCoverageComplete
	case PartialEvidenceAnswer:
		return AcceptedFinalCoveragePartial
	case SourceUnavailableAnswer:
		return AcceptedFinalCoverageUnavailable
	case NeedsEvidenceAnswer:
		return AcceptedFinalCoverageUnverified
	case GeneralGuidanceAnswer:
		return AcceptedFinalCoverageGuidanceOnly
	default:
		return AcceptedFinalCoverageUnverified
	}
}
