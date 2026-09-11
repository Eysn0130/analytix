package caseentity

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
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainprivacyprojection "analytix.local/runtime-go/internal/domain/privacyprojection"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	PrivateStateSchemaVersionV1 = 1

	CaseEntityBindingPurposeV1      = "analytix.case-entity-binding/v1"
	CaseIngressPurposeV1            = "analytix.case-ingress/v1"
	ThreadCaseContextPurposeV1      = "analytix.thread-case-context/v1"
	caseLongitudinalIndexThreadIDV1 = "analytix.case-longitudinal-index/v1"

	CaseIngressKindTurnV1  = "turn"
	CaseIngressKindSteerV1 = "steer"

	SnapshotCurrentV1    = "current"
	SnapshotHistoricalV1 = "historical"
	SnapshotStaleV1      = "stale"
	SnapshotSupersededV1 = "superseded"

	InvestigationConfirmedV1 = "confirmed"
	InvestigationRejectedV1  = "rejected"
	InvestigationOpenV1      = "open"

	CaseClaimTypedStateSchemaVersionV1        = 1
	CaseAcceptedDisplayBindingSchemaVersionV1 = 1
	CaseAcceptedDisplayBindingPurposeV1       = "analytix.case-accepted-display-binding/v1"
	caseAcceptedDisplayFinalGateVersionV1     = "analytix.final-evidence-gate/v4"

	MaxCaseEntityBindingRecordBytesV1 = 16 * 1024
	MaxCaseIngressRecordBytesV1       = 512 * 1024
	MaxThreadCaseContextRecordBytesV1 = 2 * 1024 * 1024

	maxPrivateScopeTextBytesV1       = 512
	maxPrivateReferenceTextBytesV1   = 512
	maxCaseIngressTextBytesV1        = 256 * 1024
	maxCaseIngressSpansV1            = 1_024
	maxThreadCaseEntitiesV1          = 4_096
	maxThreadCaseSnapshotsV1         = 128
	maxThreadCaseClaimsV1            = 8_192
	maxThreadCaseEvidenceV1          = 16_384
	maxThreadCaseContinuationV1      = 4_096
	maxThreadCaseDisplayBindingsV1   = 4_096
	maxThreadCaseQuestionsAndGapsV1  = 4_096
	maxThreadCaseEvidencePerClaimV1  = 512
	maxThreadCaseContextGenerationV1 = 1<<53 - 1
)

// privateTextUseV1 keeps source-exact text in a closure environment. Unlike a
// plain unexported string, the captured bytes do not reappear when an external
// package converts an exported value to a new defined type and formats it.
type privateTextUseV1 func(func(string) error) error

func newPrivateTextUseV1(value string) privateTextUseV1 {
	return func(use func(string) error) error {
		if use == nil {
			return errors.New("private text use is invalid")
		}
		return use(value)
	}
}

func privateTextValueV1(privateUse privateTextUseV1) (string, error) {
	if privateUse == nil {
		return "", errors.New("private text use is invalid")
	}
	var value string
	if err := privateUse(func(privateValue string) error {
		value = privateValue
		return nil
	}); err != nil {
		return "", errors.New("private text use is invalid")
	}
	return value, nil
}

// CaseEntityBindingRecord is private host state. CanonicalValue and its
// reverse mapping must never be projected into provider, SSE, history, search,
// compaction, ordinary UI, or ordinary artifact contracts.
type CaseEntityBindingRecord struct {
	SchemaVersion     int         `json:"schemaVersion"`
	Purpose           string      `json:"purpose"`
	TenantID          string      `json:"tenantId"`
	UserID            string      `json:"userId"`
	CaseID            string      `json:"caseId"`
	CaseBindingHash   string      `json:"caseBindingHash"`
	EntityType        string      `json:"entityType"`
	Reference         ReferenceV1 `json:"reference"`
	StableOrdinal     uint32      `json:"stableOrdinal"`
	Canonicalizer     string      `json:"canonicalizer"`
	canonicalValueUse privateTextUseV1
	legacyOrdinal     bool
	BindingKey        string `json:"bindingKey"`
	RecordDigest      string `json:"recordDigest"`
}

// caseEntityBindingRecordWireV1 is the only JSON representation of a private
// binding. The public domain value deliberately rejects ordinary JSON
// marshaling so source-exact values cannot escape through generic logging,
// telemetry, or response encoding.
type caseEntityBindingRecordWireV1 struct {
	SchemaVersion   int         `json:"schemaVersion"`
	Purpose         string      `json:"purpose"`
	TenantID        string      `json:"tenantId"`
	UserID          string      `json:"userId"`
	CaseID          string      `json:"caseId"`
	CaseBindingHash string      `json:"caseBindingHash"`
	EntityType      string      `json:"entityType"`
	Reference       ReferenceV1 `json:"reference"`
	StableOrdinal   uint32      `json:"stableOrdinal"`
	Canonicalizer   string      `json:"canonicalizer"`
	CanonicalValue  string      `json:"canonicalValue"`
	BindingKey      string      `json:"bindingKey"`
	RecordDigest    string      `json:"recordDigest"`
}

// legacyCaseEntityBindingRecordWireV1 is the immutable V1 representation
// written before stableOrdinal became part of the same schema. Those records
// remain canonical V1 records and cannot be rewritten in the private CAS.
type legacyCaseEntityBindingRecordWireV1 struct {
	SchemaVersion   int         `json:"schemaVersion"`
	Purpose         string      `json:"purpose"`
	TenantID        string      `json:"tenantId"`
	UserID          string      `json:"userId"`
	CaseID          string      `json:"caseId"`
	CaseBindingHash string      `json:"caseBindingHash"`
	EntityType      string      `json:"entityType"`
	Reference       ReferenceV1 `json:"reference"`
	Canonicalizer   string      `json:"canonicalizer"`
	CanonicalValue  string      `json:"canonicalValue"`
	BindingKey      string      `json:"bindingKey"`
	RecordDigest    string      `json:"recordDigest"`
}

type CaseEntityBindingRecordInputV1 struct {
	SecurityContext     domainsecurity.TurnSecurityContext
	EntityType          string
	Reference           ReferenceV1
	sourceExactValueUse privateTextUseV1
	stableOrdinal       uint32
}

// WithCaseEntityBindingStableOrdinalV1 is used only by the private store while
// it atomically allocates the next case-scoped identity ordinal. Callers cannot
// make an ordinal authoritative without the store's uniqueness check.
func WithCaseEntityBindingStableOrdinalV1(
	input CaseEntityBindingRecordInputV1,
	stableOrdinal uint32,
) CaseEntityBindingRecordInputV1 {
	input.stableOrdinal = stableOrdinal
	return input
}

func NewCaseEntityBindingRecordInputV1(
	securityContext domainsecurity.TurnSecurityContext,
	entityType string,
	reference ReferenceV1,
	sourceExactValue string,
) CaseEntityBindingRecordInputV1 {
	return CaseEntityBindingRecordInputV1{
		SecurityContext: securityContext, EntityType: entityType,
		Reference: reference, sourceExactValueUse: newPrivateTextUseV1(sourceExactValue),
	}
}

func (CaseEntityBindingRecord) MarshalJSON() ([]byte, error) {
	return nil, errors.New("case entity private binding does not support ordinary JSON serialization")
}

func (*CaseEntityBindingRecord) UnmarshalJSON([]byte) error {
	return errors.New("case entity private binding does not support ordinary JSON deserialization")
}

func (CaseEntityBindingRecord) String() string {
	return "CaseEntityBindingRecord{private:[REDACTED]}"
}

func (record CaseEntityBindingRecord) GoString() string {
	return record.String()
}

func (CaseEntityBindingRecordInputV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("case entity private binding input does not support ordinary JSON serialization")
}

func (*CaseEntityBindingRecordInputV1) UnmarshalJSON([]byte) error {
	return errors.New("case entity private binding input does not support ordinary JSON deserialization")
}

func (CaseEntityBindingRecordInputV1) String() string {
	return "CaseEntityBindingRecordInputV1{private:[REDACTED]}"
}

func (input CaseEntityBindingRecordInputV1) GoString() string {
	return input.String()
}

func NewCaseEntityBindingRecordV1(input CaseEntityBindingRecordInputV1) (CaseEntityBindingRecord, error) {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.SecurityContext) != nil ||
		!IsFinancialEntityTypeV1(input.EntityType) ||
		ValidateReferenceV1(string(input.Reference)) != nil || input.stableOrdinal == 0 {
		return CaseEntityBindingRecord{}, errors.New("case entity binding input is invalid")
	}
	sourceExactValue, sourceErr := privateTextValueV1(input.sourceExactValueUse)
	canonicalValue, err := domaincontrolledaccount.CanonicalFinancialAccountTextV2(sourceExactValue)
	if sourceErr != nil || err != nil {
		return CaseEntityBindingRecord{}, errors.New("case entity binding input is invalid")
	}
	record := CaseEntityBindingRecord{
		SchemaVersion:     PrivateStateSchemaVersionV1,
		Purpose:           CaseEntityBindingPurposeV1,
		TenantID:          input.SecurityContext.TenantID,
		UserID:            input.SecurityContext.UserID,
		CaseID:            input.SecurityContext.CaseID,
		CaseBindingHash:   input.SecurityContext.CaseBindingHash,
		EntityType:        input.EntityType,
		Reference:         input.Reference,
		StableOrdinal:     input.stableOrdinal,
		Canonicalizer:     domaincontrolledaccount.ControlledAccountFinancialCanonicalizationV1,
		canonicalValueUse: newPrivateTextUseV1(canonicalValue),
	}
	record.BindingKey = caseEntityBindingKeyV1(
		record.TenantID,
		record.UserID,
		record.CaseID,
		record.CaseBindingHash,
		record.EntityType,
		record.Reference,
	)
	record.RecordDigest = caseEntityBindingRecordDigestV1(record)
	if ValidateCaseEntityBindingRecordV1(record) != nil {
		return CaseEntityBindingRecord{}, errors.New("case entity binding input is invalid")
	}
	return record, nil
}

func ValidateCaseEntityBindingRecordV1(record CaseEntityBindingRecord) error {
	storedCanonicalValue, storedErr := privateTextValueV1(record.canonicalValueUse)
	canonicalValue, canonicalErr := domaincontrolledaccount.CanonicalFinancialAccountTextV2(storedCanonicalValue)
	if record.SchemaVersion != PrivateStateSchemaVersionV1 ||
		record.Purpose != CaseEntityBindingPurposeV1 ||
		!canonicalPrivateScopeTextV1(record.TenantID) ||
		!canonicalPrivateScopeTextV1(record.UserID) ||
		!canonicalPrivateScopeTextV1(record.CaseID) ||
		!domainsecurity.IsSHA256Hex(record.CaseBindingHash) ||
		!IsFinancialEntityTypeV1(record.EntityType) ||
		ValidateReferenceV1(string(record.Reference)) != nil ||
		(!record.legacyOrdinal && record.StableOrdinal == 0) ||
		record.Canonicalizer != domaincontrolledaccount.ControlledAccountFinancialCanonicalizationV1 ||
		storedErr != nil || canonicalErr != nil ||
		canonicalValue != storedCanonicalValue ||
		!domainsecurity.IsSHA256Hex(record.BindingKey) ||
		record.BindingKey != caseEntityBindingKeyV1(
			record.TenantID,
			record.UserID,
			record.CaseID,
			record.CaseBindingHash,
			record.EntityType,
			record.Reference,
		) ||
		!domainsecurity.IsSHA256Hex(record.RecordDigest) ||
		record.RecordDigest != caseEntityBindingRecordDigestV1(record) {
		return errors.New("case entity binding record is invalid")
	}
	var (
		body []byte
		err  error
	)
	if record.legacyOrdinal {
		body, err = json.Marshal(legacyCaseEntityBindingRecordWireV1For(record))
	} else {
		body, err = json.Marshal(caseEntityBindingRecordWireV1For(record))
	}
	if err != nil || len(body) == 0 || len(body) > MaxCaseEntityBindingRecordBytesV1 {
		return errors.New("case entity binding record is invalid")
	}
	return nil
}

func ParseCaseEntityBindingRecordV1(body []byte) (CaseEntityBindingRecord, error) {
	var wire caseEntityBindingRecordWireV1
	if parseCanonicalPrivateStateV1(
		body,
		&wire,
		MaxCaseEntityBindingRecordBytesV1,
		16,
		512,
		maxCaseIngressTextBytesV1,
	) == nil {
		record := caseEntityBindingRecordFromWireV1(wire)
		if ValidateCaseEntityBindingRecordV1(record) == nil {
			return record, nil
		}
	}

	var legacyWire legacyCaseEntityBindingRecordWireV1
	if parseCanonicalPrivateStateV1(
		body,
		&legacyWire,
		MaxCaseEntityBindingRecordBytesV1,
		16,
		512,
		maxCaseIngressTextBytesV1,
	) != nil {
		return CaseEntityBindingRecord{}, errors.New("case entity binding record is invalid")
	}
	record := caseEntityBindingRecordFromLegacyWireV1(legacyWire)
	if ValidateCaseEntityBindingRecordV1(record) != nil {
		return CaseEntityBindingRecord{}, errors.New("case entity binding record is invalid")
	}
	return record, nil
}

func CaseEntityBindingRecordV1Bytes(record CaseEntityBindingRecord) ([]byte, error) {
	if ValidateCaseEntityBindingRecordV1(record) != nil {
		return nil, errors.New("case entity binding record is invalid")
	}
	var (
		body []byte
		err  error
	)
	if record.legacyOrdinal {
		body, err = json.Marshal(legacyCaseEntityBindingRecordWireV1For(record))
	} else {
		body, err = json.Marshal(caseEntityBindingRecordWireV1For(record))
	}
	if err != nil || len(body) == 0 || len(body) > MaxCaseEntityBindingRecordBytesV1 {
		return nil, errors.New("case entity binding record is invalid")
	}
	return body, nil
}

func CaseEntityBindingLookupKeyV1(
	securityContext domainsecurity.TurnSecurityContext,
	entityType string,
	reference ReferenceV1,
) (string, error) {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		!IsFinancialEntityTypeV1(entityType) ||
		ValidateReferenceV1(string(reference)) != nil {
		return "", errors.New("case entity binding lookup is invalid")
	}
	return caseEntityBindingKeyV1(
		securityContext.TenantID,
		securityContext.UserID,
		securityContext.CaseID,
		securityContext.CaseBindingHash,
		entityType,
		reference,
	), nil
}

func IsFinancialEntityTypeV1(value string) bool {
	return value == domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1 ||
		value == domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1
}

func caseEntityBindingKeyV1(
	tenantID string,
	userID string,
	caseID string,
	caseBindingHash string,
	entityType string,
	reference ReferenceV1,
) string {
	body, _ := json.Marshal(struct {
		CaseBindingHash string      `json:"caseBindingHash"`
		CaseID          string      `json:"caseId"`
		EntityType      string      `json:"entityType"`
		Reference       ReferenceV1 `json:"reference"`
		TenantID        string      `json:"tenantId"`
		UserID          string      `json:"userId"`
	}{
		CaseBindingHash: caseBindingHash,
		CaseID:          caseID,
		EntityType:      entityType,
		Reference:       reference,
		TenantID:        tenantID,
		UserID:          userID,
	})
	return domainsecurity.SHA256Hex(body)
}

func caseEntityBindingRecordDigestV1(record CaseEntityBindingRecord) string {
	record.RecordDigest = ""
	var body []byte
	if record.legacyOrdinal {
		body, _ = json.Marshal(legacyCaseEntityBindingRecordWireV1For(record))
	} else {
		body, _ = json.Marshal(caseEntityBindingRecordWireV1For(record))
	}
	return domainsecurity.SHA256Hex(body)
}

func caseEntityBindingRecordWireV1For(record CaseEntityBindingRecord) caseEntityBindingRecordWireV1 {
	canonicalValue, _ := privateTextValueV1(record.canonicalValueUse)
	return caseEntityBindingRecordWireV1{
		SchemaVersion: record.SchemaVersion, Purpose: record.Purpose,
		TenantID: record.TenantID, UserID: record.UserID, CaseID: record.CaseID,
		CaseBindingHash: record.CaseBindingHash, EntityType: record.EntityType,
		Reference: record.Reference, StableOrdinal: record.StableOrdinal, Canonicalizer: record.Canonicalizer,
		CanonicalValue: canonicalValue, BindingKey: record.BindingKey,
		RecordDigest: record.RecordDigest,
	}
}

func caseEntityBindingRecordFromWireV1(wire caseEntityBindingRecordWireV1) CaseEntityBindingRecord {
	return CaseEntityBindingRecord{
		SchemaVersion: wire.SchemaVersion, Purpose: wire.Purpose,
		TenantID: wire.TenantID, UserID: wire.UserID, CaseID: wire.CaseID,
		CaseBindingHash: wire.CaseBindingHash, EntityType: wire.EntityType,
		Reference: wire.Reference, StableOrdinal: wire.StableOrdinal, Canonicalizer: wire.Canonicalizer,
		canonicalValueUse: newPrivateTextUseV1(wire.CanonicalValue), BindingKey: wire.BindingKey,
		RecordDigest: wire.RecordDigest,
	}
}

func legacyCaseEntityBindingRecordWireV1For(record CaseEntityBindingRecord) legacyCaseEntityBindingRecordWireV1 {
	canonicalValue, _ := privateTextValueV1(record.canonicalValueUse)
	return legacyCaseEntityBindingRecordWireV1{
		SchemaVersion: record.SchemaVersion, Purpose: record.Purpose,
		TenantID: record.TenantID, UserID: record.UserID, CaseID: record.CaseID,
		CaseBindingHash: record.CaseBindingHash, EntityType: record.EntityType,
		Reference: record.Reference, Canonicalizer: record.Canonicalizer,
		CanonicalValue: canonicalValue, BindingKey: record.BindingKey,
		RecordDigest: record.RecordDigest,
	}
}

func caseEntityBindingRecordFromLegacyWireV1(wire legacyCaseEntityBindingRecordWireV1) CaseEntityBindingRecord {
	return CaseEntityBindingRecord{
		SchemaVersion: wire.SchemaVersion, Purpose: wire.Purpose,
		TenantID: wire.TenantID, UserID: wire.UserID, CaseID: wire.CaseID,
		CaseBindingHash: wire.CaseBindingHash, EntityType: wire.EntityType,
		Reference: wire.Reference, Canonicalizer: wire.Canonicalizer,
		canonicalValueUse: newPrivateTextUseV1(wire.CanonicalValue), legacyOrdinal: true,
		BindingKey: wire.BindingKey, RecordDigest: wire.RecordDigest,
	}
}

// CaseEntityBindingNeedsStableOrdinalRecoveryV1 identifies an immutable V1
// binding written before stableOrdinal was persisted. The private store must
// resolve all such records as one case-scoped inventory before exposing one.
func CaseEntityBindingNeedsStableOrdinalRecoveryV1(record CaseEntityBindingRecord) bool {
	return record.legacyOrdinal && record.StableOrdinal == 0 &&
		ValidateCaseEntityBindingRecordV1(record) == nil
}

// WithRecoveredCaseEntityBindingStableOrdinalV1 applies the private store's
// deterministic inventory result without changing the legacy canonical bytes
// or record digest. It cannot turn a current record into a legacy record.
func WithRecoveredCaseEntityBindingStableOrdinalV1(
	record CaseEntityBindingRecord,
	stableOrdinal uint32,
) (CaseEntityBindingRecord, error) {
	if !CaseEntityBindingNeedsStableOrdinalRecoveryV1(record) || stableOrdinal == 0 {
		return CaseEntityBindingRecord{}, errors.New("case entity binding ordinal recovery is invalid")
	}
	record.StableOrdinal = stableOrdinal
	if ValidateCaseEntityBindingRecordV1(record) != nil {
		return CaseEntityBindingRecord{}, errors.New("case entity binding ordinal recovery is invalid")
	}
	return record, nil
}

// UseCanonicalValueV1 keeps the source-exact value out of the record's
// exported representation. The app layer must additionally hold current DSV2
// exact-use authority while invoking this synchronous callback.
func (record CaseEntityBindingRecord) UseCanonicalValueV1(use func(string) error) error {
	if use == nil || record.canonicalValueUse == nil || ValidateCaseEntityBindingRecordV1(record) != nil {
		return errors.New("case entity private binding use is invalid")
	}
	return record.canonicalValueUse(use)
}

// CaseIngressSpanV1 binds one source-exact private span to the stable internal
// reference occupying the corresponding span in ProjectedText.
type CaseIngressSpanV1 struct {
	EntityType         string      `json:"entityType"`
	Reference          ReferenceV1 `json:"reference"`
	RawStartByte       int         `json:"rawStartByte"`
	RawEndByte         int         `json:"rawEndByte"`
	ProjectedStartByte int         `json:"projectedStartByte"`
	ProjectedEndByte   int         `json:"projectedEndByte"`
}

// CaseIngressRecord is private host-owned ingress. RawText is deliberately
// retained only here; public callers receive only IngressID/RecordDigest.
type CaseIngressRecord struct {
	SchemaVersion     int    `json:"schemaVersion"`
	Purpose           string `json:"purpose"`
	TenantID          string `json:"tenantId"`
	UserID            string `json:"userId"`
	CaseID            string `json:"caseId"`
	CaseBindingHash   string `json:"caseBindingHash"`
	ThreadID          string `json:"threadId"`
	TurnID            string `json:"turnId"`
	ContextDigest     string `json:"contextDigest"`
	ContextEpoch      uint64 `json:"contextEpoch"`
	DatasetSnapshotID string `json:"datasetSnapshotId"`
	IngressKind       string `json:"ingressKind"`
	IngressOrdinal    uint32 `json:"ingressOrdinal"`
	rawTextUse        privateTextUseV1
	ProjectedText     string              `json:"projectedText"`
	Spans             []CaseIngressSpanV1 `json:"spans"`
	IngressID         string              `json:"ingressId"`
	RecordDigest      string              `json:"recordDigest"`
}

type caseIngressRecordWireV1 struct {
	SchemaVersion     int                 `json:"schemaVersion"`
	Purpose           string              `json:"purpose"`
	TenantID          string              `json:"tenantId"`
	UserID            string              `json:"userId"`
	CaseID            string              `json:"caseId"`
	CaseBindingHash   string              `json:"caseBindingHash"`
	ThreadID          string              `json:"threadId"`
	TurnID            string              `json:"turnId"`
	ContextDigest     string              `json:"contextDigest"`
	ContextEpoch      uint64              `json:"contextEpoch"`
	DatasetSnapshotID string              `json:"datasetSnapshotId"`
	IngressKind       string              `json:"ingressKind"`
	IngressOrdinal    uint32              `json:"ingressOrdinal"`
	RawText           string              `json:"rawText"`
	ProjectedText     string              `json:"projectedText"`
	Spans             []CaseIngressSpanV1 `json:"spans"`
	IngressID         string              `json:"ingressId"`
	RecordDigest      string              `json:"recordDigest"`
}

type CaseIngressRecordInputV1 struct {
	SecurityContext domainsecurity.TurnSecurityContext
	IngressKind     string
	IngressOrdinal  uint32
	rawTextUse      privateTextUseV1
	ProjectedText   string
	Spans           []CaseIngressSpanV1
}

func NewCaseIngressRecordInputV1(
	securityContext domainsecurity.TurnSecurityContext,
	ingressKind string,
	ingressOrdinal uint32,
	rawText string,
	projectedText string,
	spans []CaseIngressSpanV1,
) CaseIngressRecordInputV1 {
	return CaseIngressRecordInputV1{
		SecurityContext: securityContext, IngressKind: ingressKind,
		IngressOrdinal: ingressOrdinal, rawTextUse: newPrivateTextUseV1(rawText),
		ProjectedText: projectedText, Spans: append([]CaseIngressSpanV1(nil), spans...),
	}
}

func (CaseIngressRecord) MarshalJSON() ([]byte, error) {
	return nil, errors.New("case ingress private record does not support ordinary JSON serialization")
}

func (*CaseIngressRecord) UnmarshalJSON([]byte) error {
	return errors.New("case ingress private record does not support ordinary JSON deserialization")
}

func (CaseIngressRecord) String() string {
	return "CaseIngressRecord{private:[REDACTED]}"
}

func (record CaseIngressRecord) GoString() string {
	return record.String()
}

func (CaseIngressRecordInputV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("case ingress private input does not support ordinary JSON serialization")
}

func (*CaseIngressRecordInputV1) UnmarshalJSON([]byte) error {
	return errors.New("case ingress private input does not support ordinary JSON deserialization")
}

func (CaseIngressRecordInputV1) String() string {
	return "CaseIngressRecordInputV1{private:[REDACTED]}"
}

func (input CaseIngressRecordInputV1) GoString() string {
	return input.String()
}

func NewCaseIngressRecordV1(input CaseIngressRecordInputV1) (CaseIngressRecord, error) {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.SecurityContext) != nil {
		return CaseIngressRecord{}, errors.New("case ingress input is invalid")
	}
	rawText, rawErr := privateTextValueV1(input.rawTextUse)
	if rawErr != nil {
		return CaseIngressRecord{}, errors.New("case ingress input is invalid")
	}
	record := CaseIngressRecord{
		SchemaVersion:     PrivateStateSchemaVersionV1,
		Purpose:           CaseIngressPurposeV1,
		TenantID:          input.SecurityContext.TenantID,
		UserID:            input.SecurityContext.UserID,
		CaseID:            input.SecurityContext.CaseID,
		CaseBindingHash:   input.SecurityContext.CaseBindingHash,
		ThreadID:          input.SecurityContext.ThreadID,
		TurnID:            input.SecurityContext.TurnID,
		ContextDigest:     input.SecurityContext.ContextDigest,
		ContextEpoch:      input.SecurityContext.ContextEpoch,
		DatasetSnapshotID: input.SecurityContext.DatasetSnapshotID,
		IngressKind:       input.IngressKind,
		IngressOrdinal:    input.IngressOrdinal,
		rawTextUse:        newPrivateTextUseV1(rawText),
		ProjectedText:     input.ProjectedText,
		Spans:             append([]CaseIngressSpanV1(nil), input.Spans...),
	}
	sort.Slice(record.Spans, func(i, j int) bool {
		if record.Spans[i].RawStartByte != record.Spans[j].RawStartByte {
			return record.Spans[i].RawStartByte < record.Spans[j].RawStartByte
		}
		return record.Spans[i].ProjectedStartByte < record.Spans[j].ProjectedStartByte
	})
	record.IngressID = caseIngressIDV1(record)
	record.RecordDigest = caseIngressRecordDigestV1(record)
	if ValidateCaseIngressRecordV1(record) != nil {
		return CaseIngressRecord{}, errors.New("case ingress input is invalid")
	}
	return record, nil
}

func ValidateCaseIngressRecordV1(record CaseIngressRecord) error {
	rawText, rawErr := privateTextValueV1(record.rawTextUse)
	if record.SchemaVersion != PrivateStateSchemaVersionV1 ||
		record.Purpose != CaseIngressPurposeV1 ||
		!canonicalPrivateScopeTextV1(record.TenantID) ||
		!canonicalPrivateScopeTextV1(record.UserID) ||
		!canonicalPrivateScopeTextV1(record.CaseID) ||
		!domainsecurity.IsSHA256Hex(record.CaseBindingHash) ||
		!canonicalPrivateScopeTextV1(record.ThreadID) ||
		!canonicalPrivateScopeTextV1(record.TurnID) ||
		!domainsecurity.IsSHA256Hex(record.ContextDigest) ||
		record.ContextEpoch == 0 ||
		!domainsecurity.IsDatasetSnapshotIDV2Syntax(record.DatasetSnapshotID) ||
		(record.IngressKind != CaseIngressKindTurnV1 && record.IngressKind != CaseIngressKindSteerV1) ||
		rawErr != nil || rawText == "" ||
		len(rawText) > maxCaseIngressTextBytesV1 ||
		!utf8.ValidString(rawText) ||
		record.ProjectedText == "" ||
		len(record.ProjectedText) > maxCaseIngressTextBytesV1 ||
		!utf8.ValidString(record.ProjectedText) ||
		len(record.Spans) == 0 ||
		len(record.Spans) > maxCaseIngressSpansV1 ||
		!domainsecurity.IsSHA256Hex(record.IngressID) ||
		record.IngressID != caseIngressIDV1(record) ||
		!domainsecurity.IsSHA256Hex(record.RecordDigest) ||
		record.RecordDigest != caseIngressRecordDigestV1(record) {
		return errors.New("case ingress record is invalid")
	}

	previousRawEnd := 0
	previousProjectedEnd := 0
	var expectedProjection strings.Builder
	expectedProjection.Grow(len(record.ProjectedText))
	for index, span := range record.Spans {
		if !IsFinancialEntityTypeV1(span.EntityType) ||
			ValidateReferenceV1(string(span.Reference)) != nil ||
			span.RawStartByte < 0 ||
			span.RawEndByte <= span.RawStartByte ||
			span.RawEndByte > len(rawText) ||
			span.ProjectedStartByte < 0 ||
			span.ProjectedEndByte <= span.ProjectedStartByte ||
			span.ProjectedEndByte > len(record.ProjectedText) ||
			!utf8ByteBoundaryV1(rawText, span.RawStartByte) ||
			!utf8ByteBoundaryV1(rawText, span.RawEndByte) ||
			!utf8ByteBoundaryV1(record.ProjectedText, span.ProjectedStartByte) ||
			!utf8ByteBoundaryV1(record.ProjectedText, span.ProjectedEndByte) ||
			(index > 0 && (span.RawStartByte < previousRawEnd ||
				span.ProjectedStartByte < previousProjectedEnd)) {
			return errors.New("case ingress record is invalid")
		}
		rawValue := rawText[span.RawStartByte:span.RawEndByte]
		projectedGap, projectionErr := ProjectCaseIngressPrivateGapV1(
			rawText[previousRawEnd:span.RawStartByte],
		)
		if _, err := domaincontrolledaccount.CanonicalFinancialAccountTextV2(rawValue); err != nil ||
			record.ProjectedText[span.ProjectedStartByte:span.ProjectedEndByte] != string(span.Reference) ||
			projectionErr != nil {
			return errors.New("case ingress record is invalid")
		}
		expectedProjection.WriteString(projectedGap)
		if span.ProjectedStartByte != expectedProjection.Len() {
			return errors.New("case ingress record is invalid")
		}
		expectedProjection.WriteString(string(span.Reference))
		if span.ProjectedEndByte != expectedProjection.Len() {
			return errors.New("case ingress record is invalid")
		}
		previousRawEnd = span.RawEndByte
		previousProjectedEnd = span.ProjectedEndByte
	}
	projectedGap, projectionErr := ProjectCaseIngressPrivateGapV1(rawText[previousRawEnd:])
	if projectionErr != nil {
		return errors.New("case ingress record is invalid")
	}
	expectedProjection.WriteString(projectedGap)
	if expectedProjection.String() != record.ProjectedText ||
		containsCompleteIdentifierShapeV1(record.ProjectedText) {
		return errors.New("case ingress record is invalid")
	}
	body, err := json.Marshal(caseIngressRecordWireV1For(record))
	if err != nil || len(body) == 0 || len(body) > MaxCaseIngressRecordBytesV1 {
		return errors.New("case ingress record is invalid")
	}
	return nil
}

func ParseCaseIngressRecordV1(body []byte) (CaseIngressRecord, error) {
	var wire caseIngressRecordWireV1
	if parseCanonicalPrivateStateV1(
		body,
		&wire,
		MaxCaseIngressRecordBytesV1,
		16,
		32_768,
		maxCaseIngressTextBytesV1,
	) != nil {
		return CaseIngressRecord{}, errors.New("case ingress record is invalid")
	}
	record := caseIngressRecordFromWireV1(wire)
	if ValidateCaseIngressRecordV1(record) != nil {
		return CaseIngressRecord{}, errors.New("case ingress record is invalid")
	}
	return record, nil
}

func CaseIngressRecordV1Bytes(record CaseIngressRecord) ([]byte, error) {
	if ValidateCaseIngressRecordV1(record) != nil {
		return nil, errors.New("case ingress record is invalid")
	}
	body, err := json.Marshal(caseIngressRecordWireV1For(record))
	if err != nil || len(body) == 0 || len(body) > MaxCaseIngressRecordBytesV1 {
		return nil, errors.New("case ingress record is invalid")
	}
	return body, nil
}

func caseIngressIDV1(record CaseIngressRecord) string {
	return caseIngressLookupIDFieldsV1(
		record.TenantID,
		record.UserID,
		record.CaseID,
		record.CaseBindingHash,
		record.ThreadID,
		record.TurnID,
		record.ContextDigest,
		record.DatasetSnapshotID,
		record.IngressKind,
		record.IngressOrdinal,
	)
}

// CaseIngressLookupIDV1 deterministically addresses one private ingress from
// public frozen-authority coordinates. It contains no raw text, entity
// reference, account value, or raw-dependent digest, so restart/resume can
// locate the exact private projection without persisting a public handle.
func CaseIngressLookupIDV1(
	securityContext domainsecurity.TurnSecurityContext,
	ingressKind string,
	ingressOrdinal uint32,
) (string, error) {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		(ingressKind != CaseIngressKindTurnV1 && ingressKind != CaseIngressKindSteerV1) {
		return "", errors.New("case ingress lookup input is invalid")
	}
	return caseIngressLookupIDFieldsV1(
		securityContext.TenantID,
		securityContext.UserID,
		securityContext.CaseID,
		securityContext.CaseBindingHash,
		securityContext.ThreadID,
		securityContext.TurnID,
		securityContext.ContextDigest,
		securityContext.DatasetSnapshotID,
		ingressKind,
		ingressOrdinal,
	), nil
}

func caseIngressLookupIDFieldsV1(
	tenantID string,
	userID string,
	caseID string,
	caseBindingHash string,
	threadID string,
	turnID string,
	contextDigest string,
	datasetSnapshotID string,
	ingressKind string,
	ingressOrdinal uint32,
) string {
	body, _ := json.Marshal(struct {
		CaseBindingHash   string `json:"caseBindingHash"`
		CaseID            string `json:"caseId"`
		ContextDigest     string `json:"contextDigest"`
		DatasetSnapshotID string `json:"datasetSnapshotId"`
		IngressKind       string `json:"ingressKind"`
		IngressOrdinal    uint32 `json:"ingressOrdinal"`
		TenantID          string `json:"tenantId"`
		ThreadID          string `json:"threadId"`
		TurnID            string `json:"turnId"`
		UserID            string `json:"userId"`
	}{
		CaseBindingHash:   caseBindingHash,
		CaseID:            caseID,
		ContextDigest:     contextDigest,
		DatasetSnapshotID: datasetSnapshotID,
		IngressKind:       ingressKind,
		IngressOrdinal:    ingressOrdinal,
		TenantID:          tenantID,
		ThreadID:          threadID,
		TurnID:            turnID,
		UserID:            userID,
	})
	return domainsecurity.SHA256Hex(body)
}

func caseIngressRecordDigestV1(record CaseIngressRecord) string {
	record.RecordDigest = ""
	body, _ := json.Marshal(caseIngressRecordWireV1For(record))
	return domainsecurity.SHA256Hex(body)
}

func caseIngressRecordWireV1For(record CaseIngressRecord) caseIngressRecordWireV1 {
	rawText, _ := privateTextValueV1(record.rawTextUse)
	return caseIngressRecordWireV1{
		SchemaVersion: record.SchemaVersion, Purpose: record.Purpose,
		TenantID: record.TenantID, UserID: record.UserID, CaseID: record.CaseID,
		CaseBindingHash: record.CaseBindingHash, ThreadID: record.ThreadID, TurnID: record.TurnID,
		ContextDigest: record.ContextDigest, ContextEpoch: record.ContextEpoch,
		DatasetSnapshotID: record.DatasetSnapshotID, IngressKind: record.IngressKind,
		IngressOrdinal: record.IngressOrdinal, RawText: rawText,
		ProjectedText: record.ProjectedText, Spans: append([]CaseIngressSpanV1(nil), record.Spans...),
		IngressID: record.IngressID, RecordDigest: record.RecordDigest,
	}
}

func caseIngressRecordFromWireV1(wire caseIngressRecordWireV1) CaseIngressRecord {
	return CaseIngressRecord{
		SchemaVersion: wire.SchemaVersion, Purpose: wire.Purpose,
		TenantID: wire.TenantID, UserID: wire.UserID, CaseID: wire.CaseID,
		CaseBindingHash: wire.CaseBindingHash, ThreadID: wire.ThreadID, TurnID: wire.TurnID,
		ContextDigest: wire.ContextDigest, ContextEpoch: wire.ContextEpoch,
		DatasetSnapshotID: wire.DatasetSnapshotID, IngressKind: wire.IngressKind,
		IngressOrdinal: wire.IngressOrdinal, rawTextUse: newPrivateTextUseV1(wire.RawText),
		ProjectedText: wire.ProjectedText, Spans: append([]CaseIngressSpanV1(nil), wire.Spans...),
		IngressID: wire.IngressID, RecordDigest: wire.RecordDigest,
	}
}

// UseRawTextV1 exposes ingress source text only to an immediate private
// callback. The app layer must additionally hold current DSV2 exact-use
// authority for the exact case/snapshot/epoch while invoking it.
func (record CaseIngressRecord) UseRawTextV1(use func(string) error) error {
	if use == nil || record.rawTextUse == nil || ValidateCaseIngressRecordV1(record) != nil {
		return errors.New("case ingress private text use is invalid")
	}
	return record.rawTextUse(use)
}

type CaseSnapshotStateV1 struct {
	DatasetSnapshotID string `json:"datasetSnapshotId"`
	ContextEpoch      uint64 `json:"contextEpoch"`
	Currentness       string `json:"currentness"`
}

type CaseEvidenceStateV1 struct {
	EvidenceReference string `json:"evidenceReference"`
	EvidenceDigest    string `json:"evidenceDigest,omitempty"`
	DatasetSnapshotID string `json:"datasetSnapshotId"`
	Currentness       string `json:"currentness"`
}

type CaseContinuationStateV1 struct {
	ContinuationDigest string `json:"continuationDigest"`
	DatasetSnapshotID  string `json:"datasetSnapshotId"`
	Currentness        string `json:"currentness"`
}

type CaseClaimStateV1 struct {
	ClaimReference            string                 `json:"claimReference"`
	ClaimDigest               string                 `json:"claimDigest,omitempty"`
	TypedState                *CaseClaimTypedStateV1 `json:"typedState,omitempty"`
	DatasetSnapshotID         string                 `json:"datasetSnapshotId"`
	Currentness               string                 `json:"currentness"`
	InvestigationState        string                 `json:"investigationState"`
	EvidenceReferences        []string               `json:"evidenceReferences"`
	CounterEvidenceReferences []string               `json:"counterEvidenceReferences"`
}

type CaseAcceptedDisplayClaimBindingV1 struct {
	ClaimReference string `json:"claimReference"`
	ClaimDigest    string `json:"claimDigest"`
}

type CaseAcceptedDisplayEvidenceBindingV1 struct {
	EvidenceReference string `json:"evidenceReference"`
	EvidenceDigest    string `json:"evidenceDigest"`
}

// CaseAcceptedDisplayBindingV1 is the value-free private bridge from one
// committed accepted-final slot to its immutable historical case snapshot.
// Source locators and source-exact values remain owned by the retained DSV2
// evidence chain and are deliberately absent here.
type CaseAcceptedDisplayBindingV1 struct {
	SchemaVersion           int                                    `json:"schemaVersion"`
	Purpose                 string                                 `json:"purpose"`
	CaseBindingHash         string                                 `json:"caseBindingHash"`
	OriginalThreadID        string                                 `json:"originalThreadId"`
	OriginalTurnID          string                                 `json:"originalTurnId"`
	AcceptedFinalDigest     string                                 `json:"acceptedFinalDigest"`
	DispositionDigest       string                                 `json:"dispositionDigest"`
	FinalGateVersion        string                                 `json:"finalGateVersion"`
	ContextDigest           string                                 `json:"contextDigest"`
	DatasetSnapshotID       string                                 `json:"datasetSnapshotId"`
	ContextEpoch            uint64                                 `json:"contextEpoch"`
	EntityReference         ReferenceV1                            `json:"entityReference"`
	EntityBindingDigest     string                                 `json:"entityBindingDigest"`
	SlotID                  string                                 `json:"slotId"`
	ClaimBindings           []CaseAcceptedDisplayClaimBindingV1    `json:"claimBindings"`
	EvidenceReceiptBindings []CaseAcceptedDisplayEvidenceBindingV1 `json:"evidenceReceiptBindings"`
	Currentness             string                                 `json:"currentness"`
	BindingDigest           string                                 `json:"bindingDigest"`
}

type CaseAcceptedDisplayBindingInputV1 struct {
	CaseBindingHash         string
	OriginalThreadID        string
	OriginalTurnID          string
	AcceptedFinalDigest     string
	DispositionDigest       string
	FinalGateVersion        string
	ContextDigest           string
	DatasetSnapshotID       string
	ContextEpoch            uint64
	EntityReference         ReferenceV1
	EntityBindingDigest     string
	SlotID                  string
	ClaimBindings           []CaseAcceptedDisplayClaimBindingV1
	EvidenceReceiptBindings []CaseAcceptedDisplayEvidenceBindingV1
	Currentness             string
}

func NewCaseAcceptedDisplayBindingV1(input CaseAcceptedDisplayBindingInputV1) (CaseAcceptedDisplayBindingV1, error) {
	binding := CaseAcceptedDisplayBindingV1{
		SchemaVersion:    CaseAcceptedDisplayBindingSchemaVersionV1,
		Purpose:          CaseAcceptedDisplayBindingPurposeV1,
		CaseBindingHash:  input.CaseBindingHash,
		OriginalThreadID: input.OriginalThreadID, OriginalTurnID: input.OriginalTurnID,
		AcceptedFinalDigest: input.AcceptedFinalDigest, DispositionDigest: input.DispositionDigest,
		FinalGateVersion: input.FinalGateVersion, ContextDigest: input.ContextDigest,
		DatasetSnapshotID: input.DatasetSnapshotID, ContextEpoch: input.ContextEpoch,
		EntityReference: input.EntityReference, EntityBindingDigest: input.EntityBindingDigest,
		SlotID:                  input.SlotID,
		ClaimBindings:           append([]CaseAcceptedDisplayClaimBindingV1(nil), input.ClaimBindings...),
		EvidenceReceiptBindings: append([]CaseAcceptedDisplayEvidenceBindingV1(nil), input.EvidenceReceiptBindings...),
		Currentness:             input.Currentness,
	}
	canonicalizeCaseAcceptedDisplayBindingV1(&binding)
	binding.BindingDigest = caseAcceptedDisplayBindingDigestV1(binding)
	if ValidateCaseAcceptedDisplayBindingV1(binding) != nil {
		return CaseAcceptedDisplayBindingV1{}, errors.New("case accepted display binding input is invalid")
	}
	return binding, nil
}

func ValidateCaseAcceptedDisplayBindingV1(binding CaseAcceptedDisplayBindingV1) error {
	if binding.SchemaVersion != CaseAcceptedDisplayBindingSchemaVersionV1 ||
		binding.Purpose != CaseAcceptedDisplayBindingPurposeV1 ||
		!domainsecurity.IsSHA256Hex(binding.CaseBindingHash) ||
		!canonicalPrivateScopeTextV1(binding.OriginalThreadID) ||
		!canonicalPrivateScopeTextV1(binding.OriginalTurnID) ||
		!domainsecurity.IsSHA256Hex(binding.AcceptedFinalDigest) ||
		!domainsecurity.IsSHA256Hex(binding.DispositionDigest) ||
		binding.FinalGateVersion != caseAcceptedDisplayFinalGateVersionV1 ||
		!domainsecurity.IsSHA256Hex(binding.ContextDigest) ||
		!domainsecurity.IsDatasetSnapshotIDV2Syntax(binding.DatasetSnapshotID) ||
		binding.ContextEpoch == 0 || ValidateReferenceV1(string(binding.EntityReference)) != nil ||
		!domainsecurity.IsSHA256Hex(binding.EntityBindingDigest) ||
		!validAcceptedDisplaySlotIDV1(binding.SlotID) ||
		binding.ClaimBindings == nil || binding.EvidenceReceiptBindings == nil ||
		len(binding.ClaimBindings) == 0 || len(binding.ClaimBindings) > maxThreadCaseEvidencePerClaimV1 ||
		len(binding.EvidenceReceiptBindings) == 0 || len(binding.EvidenceReceiptBindings) > maxThreadCaseEvidencePerClaimV1 ||
		!validSnapshotCurrentnessV1(binding.Currentness) ||
		!domainsecurity.IsSHA256Hex(binding.BindingDigest) ||
		binding.BindingDigest != caseAcceptedDisplayBindingDigestV1(binding) {
		return errors.New("case accepted display binding is invalid")
	}
	previousClaim := ""
	for _, claim := range binding.ClaimBindings {
		if !canonicalClaimOwnerReferenceV1(claim.ClaimReference) ||
			!domainsecurity.IsSHA256Hex(claim.ClaimDigest) ||
			(previousClaim != "" && claim.ClaimReference <= previousClaim) {
			return errors.New("case accepted display binding is invalid")
		}
		previousClaim = claim.ClaimReference
	}
	previousEvidence := ""
	for _, evidence := range binding.EvidenceReceiptBindings {
		if !canonicalEvidenceOwnerReferenceV1(evidence.EvidenceReference) ||
			!domainsecurity.IsSHA256Hex(evidence.EvidenceDigest) ||
			(previousEvidence != "" && evidence.EvidenceReference <= previousEvidence) {
			return errors.New("case accepted display binding is invalid")
		}
		previousEvidence = evidence.EvidenceReference
	}
	return nil
}

func canonicalizeCaseAcceptedDisplayBindingV1(binding *CaseAcceptedDisplayBindingV1) {
	if binding == nil {
		return
	}
	sort.Slice(binding.ClaimBindings, func(left, right int) bool {
		return binding.ClaimBindings[left].ClaimReference < binding.ClaimBindings[right].ClaimReference
	})
	sort.Slice(binding.EvidenceReceiptBindings, func(left, right int) bool {
		return binding.EvidenceReceiptBindings[left].EvidenceReference < binding.EvidenceReceiptBindings[right].EvidenceReference
	})
	if binding.ClaimBindings == nil {
		binding.ClaimBindings = []CaseAcceptedDisplayClaimBindingV1{}
	}
	if binding.EvidenceReceiptBindings == nil {
		binding.EvidenceReceiptBindings = []CaseAcceptedDisplayEvidenceBindingV1{}
	}
}

func validAcceptedDisplaySlotIDV1(value string) bool {
	const prefix = "account-slot-"
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	ordinalText := strings.TrimPrefix(value, prefix)
	ordinal, err := strconv.ParseUint(ordinalText, 10, 32)
	return err == nil && ordinal > 0 && strconv.FormatUint(ordinal, 10) == ordinalText
}

func caseAcceptedDisplayBindingDigestV1(binding CaseAcceptedDisplayBindingV1) string {
	binding.Currentness = ""
	binding.BindingDigest = ""
	body, _ := json.Marshal(binding)
	return domainsecurity.SHA256Hex(body)
}

// CaseClaimTypedStateV1 is populated by the existing finalized ClaimRecord
// owner on the production path. Legacy schemaVersion=1 records omit this
// versioned child entirely, so their canonical bytes and digests remain
// unchanged on readback.
type CaseClaimTypedStateV1 struct {
	SchemaVersion int    `json:"schemaVersion"`
	ClaimType     string `json:"claimType"`
}

func NewCaseClaimTypedStateV1(claimType string) (*CaseClaimTypedStateV1, error) {
	state := &CaseClaimTypedStateV1{
		SchemaVersion: CaseClaimTypedStateSchemaVersionV1,
		ClaimType:     claimType,
	}
	if ValidateCaseClaimTypedStateV1(state) != nil {
		return nil, errors.New("case claim typed state is invalid")
	}
	return state, nil
}

func ValidateCaseClaimTypedStateV1(state *CaseClaimTypedStateV1) error {
	if state == nil || state.SchemaVersion != CaseClaimTypedStateSchemaVersionV1 ||
		!validCaseClaimTypeV1(state.ClaimType) {
		return errors.New("case claim typed state is invalid")
	}
	return nil
}

type CaseEntityIdentityStateV1 struct {
	Reference     ReferenceV1 `json:"reference"`
	EntityType    string      `json:"entityType"`
	StableOrdinal uint32      `json:"stableOrdinal"`
}

// ThreadCaseContextRecord contains only stable opaque references and typed
// state. It intentionally has no source-exact value, reverse mapping, raw row,
// raw tool result, model prose, or provider-authored summary field.
type ThreadCaseContextRecord struct {
	SchemaVersion            int                            `json:"schemaVersion"`
	Purpose                  string                         `json:"purpose"`
	TenantID                 string                         `json:"tenantId"`
	UserID                   string                         `json:"userId"`
	CaseID                   string                         `json:"caseId"`
	CaseBindingHash          string                         `json:"caseBindingHash"`
	ThreadID                 string                         `json:"threadId"`
	Generation               uint64                         `json:"generation"`
	PreviousRecordDigest     string                         `json:"previousRecordDigest"`
	CurrentDatasetSnapshotID string                         `json:"currentDatasetSnapshotId"`
	CurrentContextEpoch      uint64                         `json:"currentContextEpoch"`
	EntityReferences         []ReferenceV1                  `json:"entityReferences"`
	EntityIdentities         []CaseEntityIdentityStateV1    `json:"entityIdentities,omitempty"`
	Snapshots                []CaseSnapshotStateV1          `json:"snapshots"`
	Claims                   []CaseClaimStateV1             `json:"claims"`
	Evidence                 []CaseEvidenceStateV1          `json:"evidence"`
	DisplayBindings          []CaseAcceptedDisplayBindingV1 `json:"displayBindings,omitempty"`
	OpenQuestionReferences   []string                       `json:"openQuestionReferences"`
	DataGapReferences        []string                       `json:"dataGapReferences"`
	Continuations            []CaseContinuationStateV1      `json:"continuations,omitempty"`
	StorageKey               string                         `json:"storageKey"`
	RecordDigest             string                         `json:"recordDigest"`
}

type ThreadCaseContextRecordInputV1 struct {
	SecurityContext        domainsecurity.TurnSecurityContext
	Generation             uint64
	PreviousRecordDigest   string
	EntityReferences       []ReferenceV1
	EntityIdentities       []CaseEntityIdentityStateV1
	Snapshots              []CaseSnapshotStateV1
	Claims                 []CaseClaimStateV1
	Evidence               []CaseEvidenceStateV1
	DisplayBindings        []CaseAcceptedDisplayBindingV1
	OpenQuestionReferences []string
	DataGapReferences      []string
	Continuations          []CaseContinuationStateV1
}

func NewThreadCaseContextRecordV1(input ThreadCaseContextRecordInputV1) (ThreadCaseContextRecord, error) {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.SecurityContext) != nil ||
		len(input.EntityIdentities) != 0 {
		return ThreadCaseContextRecord{}, errors.New("thread case context input is invalid")
	}
	record := ThreadCaseContextRecord{
		SchemaVersion:            PrivateStateSchemaVersionV1,
		Purpose:                  ThreadCaseContextPurposeV1,
		TenantID:                 input.SecurityContext.TenantID,
		UserID:                   input.SecurityContext.UserID,
		CaseID:                   input.SecurityContext.CaseID,
		CaseBindingHash:          input.SecurityContext.CaseBindingHash,
		ThreadID:                 input.SecurityContext.ThreadID,
		Generation:               input.Generation,
		PreviousRecordDigest:     input.PreviousRecordDigest,
		CurrentDatasetSnapshotID: input.SecurityContext.DatasetSnapshotID,
		CurrentContextEpoch:      input.SecurityContext.ContextEpoch,
		EntityReferences:         append([]ReferenceV1(nil), input.EntityReferences...),
		Snapshots:                append([]CaseSnapshotStateV1(nil), input.Snapshots...),
		Claims:                   cloneCaseClaimStatesV1(input.Claims),
		Evidence:                 append([]CaseEvidenceStateV1(nil), input.Evidence...),
		DisplayBindings:          cloneCaseAcceptedDisplayBindingsV1(input.DisplayBindings),
		OpenQuestionReferences:   append([]string(nil), input.OpenQuestionReferences...),
		DataGapReferences:        append([]string(nil), input.DataGapReferences...),
		Continuations:            append([]CaseContinuationStateV1(nil), input.Continuations...),
	}
	canonicalizeThreadCaseContextV1(&record)
	record.StorageKey = threadCaseContextStorageKeyV1(record)
	record.RecordDigest = threadCaseContextRecordDigestV1(record)
	if ValidateThreadCaseContextRecordV1(record) != nil {
		return ThreadCaseContextRecord{}, errors.New("thread case context input is invalid")
	}
	return record, nil
}

// NewCaseLongitudinalIndexRecordV1 reuses the existing thread-context record
// contract and CAS partition with one reserved, host-private thread identity.
// The real turn security context is validated before the reserved identity is
// applied; callers therefore cannot use this constructor to select a case.
func NewCaseLongitudinalIndexRecordV1(
	input ThreadCaseContextRecordInputV1,
) (ThreadCaseContextRecord, error) {
	identities := append([]CaseEntityIdentityStateV1(nil), input.EntityIdentities...)
	input.EntityIdentities = nil
	record, err := NewThreadCaseContextRecordV1(input)
	if err != nil {
		return ThreadCaseContextRecord{}, err
	}
	record.ThreadID = caseLongitudinalIndexThreadIDV1
	record.EntityIdentities = identities
	canonicalizeThreadCaseContextV1(&record)
	record.StorageKey = threadCaseContextStorageKeyV1(record)
	record.RecordDigest = threadCaseContextRecordDigestV1(record)
	if ValidateThreadCaseContextRecordV1(record) != nil {
		return ThreadCaseContextRecord{}, errors.New("case longitudinal index input is invalid")
	}
	return record, nil
}

func IsCaseLongitudinalIndexRecordV1(record ThreadCaseContextRecord) bool {
	return record.ThreadID == caseLongitudinalIndexThreadIDV1 &&
		ValidateThreadCaseContextRecordV1(record) == nil
}

func ValidateThreadCaseContextRecordV1(record ThreadCaseContextRecord) error {
	if record.SchemaVersion != PrivateStateSchemaVersionV1 ||
		record.Purpose != ThreadCaseContextPurposeV1 ||
		!canonicalPrivateScopeTextV1(record.TenantID) ||
		!canonicalPrivateScopeTextV1(record.UserID) ||
		!canonicalPrivateScopeTextV1(record.CaseID) ||
		!domainsecurity.IsSHA256Hex(record.CaseBindingHash) ||
		!canonicalPrivateScopeTextV1(record.ThreadID) ||
		record.Generation == 0 ||
		record.Generation > maxThreadCaseContextGenerationV1 ||
		(record.Generation == 1 && record.PreviousRecordDigest != "") ||
		(record.Generation > 1 && !domainsecurity.IsSHA256Hex(record.PreviousRecordDigest)) ||
		!domainsecurity.IsDatasetSnapshotIDV2Syntax(record.CurrentDatasetSnapshotID) ||
		record.CurrentContextEpoch == 0 ||
		record.EntityReferences == nil ||
		record.Snapshots == nil ||
		record.Claims == nil ||
		record.Evidence == nil ||
		record.OpenQuestionReferences == nil ||
		record.DataGapReferences == nil ||
		len(record.EntityReferences) > maxThreadCaseEntitiesV1 ||
		len(record.Snapshots) == 0 ||
		len(record.Snapshots) > maxThreadCaseSnapshotsV1 ||
		len(record.Claims) > maxThreadCaseClaimsV1 ||
		len(record.Evidence) > maxThreadCaseEvidenceV1 ||
		len(record.DisplayBindings) > maxThreadCaseDisplayBindingsV1 ||
		len(record.Continuations) > maxThreadCaseContinuationV1 ||
		len(record.OpenQuestionReferences) > maxThreadCaseQuestionsAndGapsV1 ||
		len(record.DataGapReferences) > maxThreadCaseQuestionsAndGapsV1 ||
		!domainsecurity.IsSHA256Hex(record.StorageKey) ||
		record.StorageKey != threadCaseContextStorageKeyV1(record) ||
		!domainsecurity.IsSHA256Hex(record.RecordDigest) ||
		record.RecordDigest != threadCaseContextRecordDigestV1(record) {
		return errors.New("thread case context record is invalid")
	}
	if !canonicalReferenceListV1(record.EntityReferences) ||
		!canonicalPrivateReferenceListV1(record.OpenQuestionReferences) ||
		!canonicalPrivateReferenceListV1(record.DataGapReferences) {
		return errors.New("thread case context record is invalid")
	}
	if validateCaseEntityIdentityStatesV1(record) != nil {
		return errors.New("thread case context record is invalid")
	}

	snapshotCurrentness := make(map[string]string, len(record.Snapshots))
	currentSnapshots := 0
	var previousEpoch uint64
	for index, snapshot := range record.Snapshots {
		if !domainsecurity.IsDatasetSnapshotIDV2Syntax(snapshot.DatasetSnapshotID) ||
			snapshot.ContextEpoch == 0 ||
			(index > 0 && snapshot.ContextEpoch <= previousEpoch) ||
			!validSnapshotCurrentnessV1(snapshot.Currentness) {
			return errors.New("thread case context record is invalid")
		}
		if _, duplicate := snapshotCurrentness[snapshot.DatasetSnapshotID]; duplicate {
			return errors.New("thread case context record is invalid")
		}
		snapshotCurrentness[snapshot.DatasetSnapshotID] = snapshot.Currentness
		if snapshot.Currentness == SnapshotCurrentV1 {
			currentSnapshots++
			if snapshot.DatasetSnapshotID != record.CurrentDatasetSnapshotID ||
				snapshot.ContextEpoch != record.CurrentContextEpoch {
				return errors.New("thread case context record is invalid")
			}
		}
		previousEpoch = snapshot.ContextEpoch
	}
	if currentSnapshots != 1 {
		return errors.New("thread case context record is invalid")
	}

	evidenceByIdentity := make(map[string]CaseEvidenceStateV1, len(record.Evidence))
	previousEvidenceIdentity := ""
	for _, evidence := range record.Evidence {
		identity := evidence.EvidenceReference + "\x00" + evidence.DatasetSnapshotID
		snapshotState, found := snapshotCurrentness[evidence.DatasetSnapshotID]
		if !canonicalEvidenceOwnerReferenceV1(evidence.EvidenceReference) ||
			(evidence.EvidenceDigest != "" && !domainsecurity.IsSHA256Hex(evidence.EvidenceDigest)) ||
			!found ||
			evidence.Currentness != snapshotState ||
			(previousEvidenceIdentity != "" && identity <= previousEvidenceIdentity) {
			return errors.New("thread case context record is invalid")
		}
		evidenceByIdentity[identity] = evidence
		previousEvidenceIdentity = identity
	}

	previousContinuationIdentity := ""
	for _, continuation := range record.Continuations {
		identity := continuation.ContinuationDigest + "\x00" + continuation.DatasetSnapshotID
		snapshotState, found := snapshotCurrentness[continuation.DatasetSnapshotID]
		if !domainsecurity.IsSHA256Hex(continuation.ContinuationDigest) || !found ||
			continuation.Currentness != snapshotState ||
			(previousContinuationIdentity != "" && identity <= previousContinuationIdentity) {
			return errors.New("thread case context record is invalid")
		}
		previousContinuationIdentity = identity
	}

	claimByIdentity := make(map[string]CaseClaimStateV1, len(record.Claims))
	previousClaimIdentity := ""
	for _, claim := range record.Claims {
		identity := claim.ClaimReference + "\x00" + claim.DatasetSnapshotID
		snapshotState, found := snapshotCurrentness[claim.DatasetSnapshotID]
		if !canonicalClaimOwnerReferenceV1(claim.ClaimReference) ||
			(claim.ClaimDigest != "" && !domainsecurity.IsSHA256Hex(claim.ClaimDigest)) ||
			(claim.TypedState != nil && ValidateCaseClaimTypedStateV1(claim.TypedState) != nil) ||
			!found ||
			claim.Currentness != snapshotState ||
			!validInvestigationStateV1(claim.InvestigationState) ||
			claim.EvidenceReferences == nil ||
			claim.CounterEvidenceReferences == nil ||
			len(claim.EvidenceReferences) > maxThreadCaseEvidencePerClaimV1 ||
			len(claim.CounterEvidenceReferences) > maxThreadCaseEvidencePerClaimV1 ||
			!canonicalEvidenceOwnerReferenceListV1(claim.EvidenceReferences) ||
			!canonicalEvidenceOwnerReferenceListV1(claim.CounterEvidenceReferences) ||
			(previousClaimIdentity != "" && identity <= previousClaimIdentity) {
			return errors.New("thread case context record is invalid")
		}
		if claim.InvestigationState == InvestigationConfirmedV1 && len(claim.EvidenceReferences) == 0 {
			return errors.New("thread case context record is invalid")
		}
		if claim.InvestigationState == InvestigationRejectedV1 && len(claim.CounterEvidenceReferences) == 0 {
			return errors.New("thread case context record is invalid")
		}
		for _, reference := range append(
			append([]string(nil), claim.EvidenceReferences...),
			claim.CounterEvidenceReferences...,
		) {
			if _, found := evidenceByIdentity[reference+"\x00"+claim.DatasetSnapshotID]; !found {
				return errors.New("thread case context record is invalid")
			}
		}
		claimByIdentity[identity] = claim
		previousClaimIdentity = identity
	}

	entityReferences := make(map[ReferenceV1]struct{}, len(record.EntityReferences))
	for _, reference := range record.EntityReferences {
		entityReferences[reference] = struct{}{}
	}
	previousBindingDigest := ""
	acceptedSlots := make(map[string]string, len(record.DisplayBindings))
	for _, binding := range record.DisplayBindings {
		snapshotState, found := snapshotCurrentness[binding.DatasetSnapshotID]
		if ValidateCaseAcceptedDisplayBindingV1(binding) != nil ||
			binding.CaseBindingHash != record.CaseBindingHash || !found ||
			binding.Currentness != snapshotState ||
			(previousBindingDigest != "" && binding.BindingDigest <= previousBindingDigest) {
			return errors.New("thread case context record is invalid")
		}
		if _, found := entityReferences[binding.EntityReference]; !found {
			return errors.New("thread case context record is invalid")
		}
		identity := binding.AcceptedFinalDigest + "\x00" + binding.SlotID
		if existing, duplicate := acceptedSlots[identity]; duplicate && existing != binding.BindingDigest {
			return errors.New("thread case context record is invalid")
		}
		acceptedSlots[identity] = binding.BindingDigest
		evidenceReferences := make(map[string]struct{}, len(binding.EvidenceReceiptBindings))
		for _, evidence := range binding.EvidenceReceiptBindings {
			state, found := evidenceByIdentity[evidence.EvidenceReference+"\x00"+binding.DatasetSnapshotID]
			if !found || state.EvidenceDigest != evidence.EvidenceDigest {
				return errors.New("thread case context record is invalid")
			}
			evidenceReferences[evidence.EvidenceReference] = struct{}{}
		}
		claimEvidenceReferences := make(map[string]struct{}, len(evidenceReferences))
		for _, claim := range binding.ClaimBindings {
			state, found := claimByIdentity[claim.ClaimReference+"\x00"+binding.DatasetSnapshotID]
			if !found || state.ClaimDigest != claim.ClaimDigest {
				return errors.New("thread case context record is invalid")
			}
			for _, reference := range state.EvidenceReferences {
				claimEvidenceReferences[reference] = struct{}{}
			}
		}
		if len(claimEvidenceReferences) != len(evidenceReferences) {
			return errors.New("thread case context record is invalid")
		}
		for reference := range claimEvidenceReferences {
			if _, found := evidenceReferences[reference]; !found {
				return errors.New("thread case context record is invalid")
			}
		}
		previousBindingDigest = binding.BindingDigest
	}
	body, err := json.Marshal(record)
	if err != nil || len(body) == 0 || len(body) > MaxThreadCaseContextRecordBytesV1 {
		return errors.New("thread case context record is invalid")
	}
	return nil
}

func ValidateThreadCaseContextEvolutionV1(previous, current ThreadCaseContextRecord) error {
	if ValidateThreadCaseContextRecordV1(previous) != nil ||
		ValidateThreadCaseContextRecordV1(current) != nil ||
		previous.TenantID != current.TenantID ||
		previous.UserID != current.UserID ||
		previous.CaseID != current.CaseID ||
		previous.CaseBindingHash != current.CaseBindingHash ||
		previous.ThreadID != current.ThreadID ||
		current.Generation != previous.Generation+1 ||
		current.PreviousRecordDigest != previous.RecordDigest ||
		!referenceSubsetV1(previous.EntityReferences, current.EntityReferences) ||
		!caseEntityIdentitySubsetV1(previous.EntityIdentities, current.EntityIdentities) {
		return errors.New("thread case context evolution is invalid")
	}

	currentSnapshots := make(map[string]CaseSnapshotStateV1, len(current.Snapshots))
	for _, snapshot := range current.Snapshots {
		currentSnapshots[snapshot.DatasetSnapshotID] = snapshot
	}
	for _, snapshot := range previous.Snapshots {
		next, found := currentSnapshots[snapshot.DatasetSnapshotID]
		epochValid := next.ContextEpoch == snapshot.ContextEpoch ||
			(snapshot.Currentness == SnapshotCurrentV1 && next.Currentness == SnapshotCurrentV1 &&
				next.ContextEpoch > snapshot.ContextEpoch)
		if !found || !epochValid ||
			!currentnessCanEvolveV1(snapshot.Currentness, next.Currentness) {
			return errors.New("thread case context evolution is invalid")
		}
	}

	currentEvidence := make(map[string]CaseEvidenceStateV1, len(current.Evidence))
	for _, evidence := range current.Evidence {
		currentEvidence[evidence.EvidenceReference+"\x00"+evidence.DatasetSnapshotID] = evidence
	}
	for _, evidence := range previous.Evidence {
		next, found := currentEvidence[evidence.EvidenceReference+"\x00"+evidence.DatasetSnapshotID]
		if !found || evidence.EvidenceDigest != next.EvidenceDigest ||
			!currentnessCanEvolveV1(evidence.Currentness, next.Currentness) {
			return errors.New("thread case context evolution is invalid")
		}
	}

	currentClaims := make(map[string]CaseClaimStateV1, len(current.Claims))
	for _, claim := range current.Claims {
		currentClaims[claim.ClaimReference+"\x00"+claim.DatasetSnapshotID] = claim
	}
	for _, claim := range previous.Claims {
		next, found := currentClaims[claim.ClaimReference+"\x00"+claim.DatasetSnapshotID]
		if !found ||
			claim.ClaimDigest != next.ClaimDigest ||
			!equalCaseClaimTypedStateV1(claim.TypedState, next.TypedState) ||
			!currentnessCanEvolveV1(claim.Currentness, next.Currentness) ||
			!investigationCanEvolveV1(claim.InvestigationState, next.InvestigationState) ||
			!stringSubsetV1(claim.EvidenceReferences, next.EvidenceReferences) ||
			!stringSubsetV1(claim.CounterEvidenceReferences, next.CounterEvidenceReferences) {
			return errors.New("thread case context evolution is invalid")
		}
	}
	currentContinuations := make(map[string]CaseContinuationStateV1, len(current.Continuations))
	for _, continuation := range current.Continuations {
		currentContinuations[continuation.ContinuationDigest+"\x00"+continuation.DatasetSnapshotID] = continuation
	}
	for _, continuation := range previous.Continuations {
		next, found := currentContinuations[continuation.ContinuationDigest+"\x00"+continuation.DatasetSnapshotID]
		if !found || !currentnessCanEvolveV1(continuation.Currentness, next.Currentness) {
			return errors.New("thread case context evolution is invalid")
		}
	}
	currentDisplayBindings := make(map[string]CaseAcceptedDisplayBindingV1, len(current.DisplayBindings))
	for _, binding := range current.DisplayBindings {
		currentDisplayBindings[binding.BindingDigest] = binding
	}
	for _, binding := range previous.DisplayBindings {
		next, found := currentDisplayBindings[binding.BindingDigest]
		previousCurrentness := binding.Currentness
		nextCurrentness := next.Currentness
		binding.Currentness = ""
		next.Currentness = ""
		if !found || !reflect.DeepEqual(binding, next) ||
			!currentnessCanEvolveV1(previousCurrentness, nextCurrentness) {
			return errors.New("thread case context evolution is invalid")
		}
	}
	return nil
}

func ParseThreadCaseContextRecordV1(body []byte) (ThreadCaseContextRecord, error) {
	var record ThreadCaseContextRecord
	if parseCanonicalPrivateStateV1(
		body,
		&record,
		MaxThreadCaseContextRecordBytesV1,
		24,
		1_000_000,
		maxPrivateReferenceTextBytesV1,
	) != nil ||
		ValidateThreadCaseContextRecordV1(record) != nil {
		return ThreadCaseContextRecord{}, errors.New("thread case context record is invalid")
	}
	return record, nil
}

func ThreadCaseContextRecordV1Bytes(record ThreadCaseContextRecord) ([]byte, error) {
	if ValidateThreadCaseContextRecordV1(record) != nil {
		return nil, errors.New("thread case context record is invalid")
	}
	body, err := json.Marshal(record)
	if err != nil || len(body) == 0 || len(body) > MaxThreadCaseContextRecordBytesV1 {
		return nil, errors.New("thread case context record is invalid")
	}
	return body, nil
}

func ThreadCaseContextStorageKeyV1(
	securityContext domainsecurity.TurnSecurityContext,
	generation uint64,
) (string, error) {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		generation == 0 ||
		generation > maxThreadCaseContextGenerationV1 {
		return "", errors.New("thread case context lookup is invalid")
	}
	return threadCaseContextStorageKeyV1(ThreadCaseContextRecord{
		TenantID:        securityContext.TenantID,
		UserID:          securityContext.UserID,
		CaseID:          securityContext.CaseID,
		CaseBindingHash: securityContext.CaseBindingHash,
		ThreadID:        securityContext.ThreadID,
		Generation:      generation,
	}), nil
}

func canonicalizeThreadCaseContextV1(record *ThreadCaseContextRecord) {
	if record == nil {
		return
	}
	sort.Slice(record.EntityReferences, func(i, j int) bool {
		return record.EntityReferences[i] < record.EntityReferences[j]
	})
	sort.Slice(record.EntityIdentities, func(i, j int) bool {
		return record.EntityIdentities[i].Reference < record.EntityIdentities[j].Reference
	})
	sort.Slice(record.Snapshots, func(i, j int) bool {
		if record.Snapshots[i].ContextEpoch != record.Snapshots[j].ContextEpoch {
			return record.Snapshots[i].ContextEpoch < record.Snapshots[j].ContextEpoch
		}
		return record.Snapshots[i].DatasetSnapshotID < record.Snapshots[j].DatasetSnapshotID
	})
	for index := range record.Claims {
		if record.Claims[index].EvidenceReferences == nil {
			record.Claims[index].EvidenceReferences = []string{}
		}
		if record.Claims[index].CounterEvidenceReferences == nil {
			record.Claims[index].CounterEvidenceReferences = []string{}
		}
		sort.Strings(record.Claims[index].EvidenceReferences)
		sort.Strings(record.Claims[index].CounterEvidenceReferences)
	}
	sort.Slice(record.Claims, func(i, j int) bool {
		if record.Claims[i].ClaimReference != record.Claims[j].ClaimReference {
			return record.Claims[i].ClaimReference < record.Claims[j].ClaimReference
		}
		return record.Claims[i].DatasetSnapshotID < record.Claims[j].DatasetSnapshotID
	})
	sort.Slice(record.Evidence, func(i, j int) bool {
		if record.Evidence[i].EvidenceReference != record.Evidence[j].EvidenceReference {
			return record.Evidence[i].EvidenceReference < record.Evidence[j].EvidenceReference
		}
		return record.Evidence[i].DatasetSnapshotID < record.Evidence[j].DatasetSnapshotID
	})
	sort.Slice(record.DisplayBindings, func(i, j int) bool {
		return record.DisplayBindings[i].BindingDigest < record.DisplayBindings[j].BindingDigest
	})
	sort.Strings(record.OpenQuestionReferences)
	sort.Strings(record.DataGapReferences)
	sort.Slice(record.Continuations, func(i, j int) bool {
		if record.Continuations[i].ContinuationDigest != record.Continuations[j].ContinuationDigest {
			return record.Continuations[i].ContinuationDigest < record.Continuations[j].ContinuationDigest
		}
		return record.Continuations[i].DatasetSnapshotID < record.Continuations[j].DatasetSnapshotID
	})
	if record.EntityReferences == nil {
		record.EntityReferences = []ReferenceV1{}
	}
	if record.Snapshots == nil {
		record.Snapshots = []CaseSnapshotStateV1{}
	}
	if record.Claims == nil {
		record.Claims = []CaseClaimStateV1{}
	}
	if record.Evidence == nil {
		record.Evidence = []CaseEvidenceStateV1{}
	}
	if record.Continuations == nil {
		record.Continuations = []CaseContinuationStateV1{}
	}
	if record.OpenQuestionReferences == nil {
		record.OpenQuestionReferences = []string{}
	}
	if record.DataGapReferences == nil {
		record.DataGapReferences = []string{}
	}
}

func validateCaseEntityIdentityStatesV1(record ThreadCaseContextRecord) error {
	if record.ThreadID != caseLongitudinalIndexThreadIDV1 {
		if len(record.EntityIdentities) != 0 {
			return errors.New("thread identity state is invalid")
		}
		return nil
	}
	if len(record.EntityIdentities) == 0 || len(record.EntityIdentities) != len(record.EntityReferences) {
		return errors.New("case identity index is invalid")
	}
	aliases := make(map[ModelEntityAliasV1]ReferenceV1, len(record.EntityIdentities))
	for index, identity := range record.EntityIdentities {
		if identity.Reference != record.EntityReferences[index] ||
			ValidateReferenceV1(string(identity.Reference)) != nil ||
			!IsFinancialEntityTypeV1(identity.EntityType) || identity.StableOrdinal == 0 {
			return errors.New("case identity index is invalid")
		}
		alias, err := NewModelEntityAliasV1(identity.EntityType, identity.StableOrdinal)
		if err != nil {
			return errors.New("case identity index is invalid")
		}
		if existing, duplicate := aliases[alias]; duplicate && existing != identity.Reference {
			return errors.New("case identity index is invalid")
		}
		aliases[alias] = identity.Reference
	}
	return nil
}

func caseEntityIdentitySubsetV1(previous, current []CaseEntityIdentityStateV1) bool {
	if len(previous) == 0 {
		return true
	}
	if len(current) < len(previous) {
		return false
	}
	byReference := make(map[ReferenceV1]CaseEntityIdentityStateV1, len(current))
	for _, identity := range current {
		byReference[identity.Reference] = identity
	}
	for _, identity := range previous {
		if next, found := byReference[identity.Reference]; !found || next != identity {
			return false
		}
	}
	return true
}

func cloneCaseClaimStatesV1(values []CaseClaimStateV1) []CaseClaimStateV1 {
	cloned := make([]CaseClaimStateV1, len(values))
	for index, value := range values {
		cloned[index] = value
		if value.TypedState != nil {
			typedState := *value.TypedState
			cloned[index].TypedState = &typedState
		}
		cloned[index].EvidenceReferences = append([]string(nil), value.EvidenceReferences...)
		cloned[index].CounterEvidenceReferences = append([]string(nil), value.CounterEvidenceReferences...)
	}
	return cloned
}

func cloneCaseAcceptedDisplayBindingsV1(values []CaseAcceptedDisplayBindingV1) []CaseAcceptedDisplayBindingV1 {
	if values == nil {
		return nil
	}
	cloned := make([]CaseAcceptedDisplayBindingV1, len(values))
	for index, value := range values {
		cloned[index] = value
		cloned[index].ClaimBindings = append([]CaseAcceptedDisplayClaimBindingV1(nil), value.ClaimBindings...)
		cloned[index].EvidenceReceiptBindings = append([]CaseAcceptedDisplayEvidenceBindingV1(nil), value.EvidenceReceiptBindings...)
	}
	return cloned
}

func equalCaseClaimTypedStateV1(left, right *CaseClaimTypedStateV1) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func threadCaseContextStorageKeyV1(record ThreadCaseContextRecord) string {
	body, _ := json.Marshal(struct {
		CaseBindingHash string `json:"caseBindingHash"`
		CaseID          string `json:"caseId"`
		Generation      uint64 `json:"generation"`
		TenantID        string `json:"tenantId"`
		ThreadID        string `json:"threadId"`
		UserID          string `json:"userId"`
	}{
		CaseBindingHash: record.CaseBindingHash,
		CaseID:          record.CaseID,
		Generation:      record.Generation,
		TenantID:        record.TenantID,
		ThreadID:        record.ThreadID,
		UserID:          record.UserID,
	})
	return domainsecurity.SHA256Hex(body)
}

func threadCaseContextRecordDigestV1(record ThreadCaseContextRecord) string {
	record.RecordDigest = ""
	body, _ := json.Marshal(record)
	return domainsecurity.SHA256Hex(body)
}

func canonicalPrivateScopeTextV1(value string) bool {
	if value == "" ||
		len(value) > maxPrivateScopeTextBytesV1 ||
		!utf8.ValidString(value) ||
		value != strings.TrimSpace(value) {
		return false
	}
	for _, current := range value {
		if unicode.IsControl(current) || unicode.In(current, unicode.Cf) {
			return false
		}
	}
	return true
}

func canonicalPrivateReferenceTextV1(value string) bool {
	return len(value) <= maxPrivateReferenceTextBytesV1 &&
		canonicalPrivateScopeTextV1(value) &&
		!containsCompleteIdentifierShapeV1(value) &&
		!ContainsReferenceCandidateV1(value)
}

func canonicalEvidenceOwnerReferenceV1(value string) bool {
	if canonicalPrivateReferenceTextV1(value) {
		return true
	}
	return strings.HasPrefix(value, "evr_") &&
		domainsecurity.IsSHA256Hex(strings.TrimPrefix(value, "evr_"))
}

func canonicalClaimOwnerReferenceV1(value string) bool {
	if canonicalPrivateReferenceTextV1(value) {
		return true
	}
	return strings.HasPrefix(value, "clm_") &&
		domainsecurity.IsSHA256Hex(strings.TrimPrefix(value, "clm_"))
}

func containsCompleteIdentifierShapeV1(value string) bool {
	if normalized := norm.NFKC.String(value); normalized != value &&
		containsCompleteIdentifierShapeV1(normalized) {
		return true
	}
	groups := make([]string, 0, 4)
	separators := make([]rune, 0, 4)
	pendingSeparators := make([]rune, 0, 2)
	var group strings.Builder
	totalDigits := 0
	candidateStart := -1
	candidateLastDigitEnd := -1
	flush := func(candidateEnd int) bool {
		if group.Len() > 0 {
			groups = append(groups, group.String())
			group.Reset()
		}
		if totalDigits < 8 {
			groups = groups[:0]
			separators = separators[:0]
			pendingSeparators = pendingSeparators[:0]
			totalDigits = 0
			candidateStart = -1
			candidateLastDigitEnd = -1
			return false
		}
		if candidateLastDigitEnd <= candidateStart || candidateLastDigitEnd > candidateEnd {
			return true
		}
		ordinary := canonicalISODateCandidateV1(groups, separators)
		protected := financialIdentifierCueNearV1(value, candidateStart, candidateLastDigitEnd) || !ordinary
		groups = groups[:0]
		separators = separators[:0]
		pendingSeparators = pendingSeparators[:0]
		totalDigits = 0
		candidateStart = -1
		candidateLastDigitEnd = -1
		return protected
	}
	for index, current := range value {
		switch {
		case unicode.IsNumber(current):
			if totalDigits == 0 {
				candidateStart = index
			}
			if group.Len() == 0 && totalDigits > 0 && len(pendingSeparators) > 0 {
				separators = append(separators, pendingSeparators...)
				pendingSeparators = pendingSeparators[:0]
			}
			group.WriteRune(current)
			totalDigits++
			candidateLastDigitEnd = index + utf8.RuneLen(current)
		case identifierVisualSeparatorV1(current):
			if group.Len() > 0 {
				groups = append(groups, group.String())
				group.Reset()
			}
			if totalDigits > 0 {
				pendingSeparators = append(pendingSeparators, current)
			}
		default:
			if flush(index) {
				return true
			}
		}
	}
	return flush(len(value))
}

func financialIdentifierCueNearV1(value string, start, end int) bool {
	if start < 0 || end < start || end > len(value) {
		return false
	}
	const cueWindowBytes = 16
	windowStart := max(0, start-cueWindowBytes)
	for windowStart < start && !utf8.RuneStart(value[windowStart]) {
		windowStart++
	}
	windowEnd := min(len(value), end+cueWindowBytes)
	for windowEnd < len(value) && !utf8.RuneStart(value[windowEnd]) {
		windowEnd++
	}
	window := strings.ToLower(value[windowStart:windowEnd])
	for _, cue := range []string{
		"账号", "帐号", "账户", "卡号", "银行卡", "bank account", "account", "acct", "card",
	} {
		if strings.Contains(window, cue) {
			return true
		}
	}
	return false
}

func identifierVisualSeparatorV1(value rune) bool {
	return unicode.IsSpace(value) || unicode.IsPunct(value) ||
		unicode.IsSymbol(value) || unicode.IsMark(value) ||
		unicode.IsControl(value) || unicode.In(value, unicode.Cf)
}

// ProjectCaseIngressPrivateGapV1 creates the durable audit projection for a
// non-account gap. Exact raw text remains in the record's private callback,
// while every complete numeric identifier shape is replaced deterministically
// so persisted ProjectedText is never a lexical declassification surface.
func ProjectCaseIngressPrivateGapV1(value string) (string, error) {
	if len(value) > maxCaseIngressTextBytesV1 || !utf8.ValidString(value) ||
		ContainsReferenceCandidateV1(value) {
		return "", errors.New("case ingress private gap is invalid")
	}
	value = domainprivacyprojection.ProjectText(value).Text
	type privateGapSpanV1 struct {
		startByte int
		endByte   int
	}
	spans := make([]privateGapSpanV1, 0, 4)
	startByte := -1
	lastDigitEnd := -1
	digits := 0
	flush := func() {
		if startByte >= 0 && lastDigitEnd > startByte && digits >= 8 {
			spans = append(spans, privateGapSpanV1{startByte: startByte, endByte: lastDigitEnd})
		}
		startByte = -1
		lastDigitEnd = -1
		digits = 0
	}
	for _, sourceRune := range referenceNormalizedSourceRunesV1(value) {
		offset := sourceRune.startByte
		current := sourceRune.value
		switch {
		case unicode.IsNumber(current):
			if startByte < 0 {
				startByte = offset
			}
			digits++
			lastDigitEnd = sourceRune.endByte
		case startByte >= 0 && identifierVisualSeparatorV1(current):
		default:
			flush()
		}
	}
	flush()
	if len(spans) == 0 {
		return value, nil
	}
	const placeholder = "[NUMBER]"
	var projected strings.Builder
	cursor := 0
	for _, span := range spans {
		if span.startByte < cursor || span.endByte <= span.startByte || span.endByte > len(value) {
			return "", errors.New("case ingress private gap is invalid")
		}
		projected.WriteString(value[cursor:span.startByte])
		projected.WriteString(placeholder)
		cursor = span.endByte
	}
	projected.WriteString(value[cursor:])
	return projected.String(), nil
}

func canonicalISODateCandidateV1(groups []string, separators []rune) bool {
	if len(groups) != 3 || len(separators) != 2 ||
		len(groups[0]) != 4 || len(groups[1]) != 2 || len(groups[2]) != 2 ||
		(separators[0] != '-' && separators[0] != '/') ||
		separators[1] != separators[0] {
		return false
	}
	for _, group := range groups {
		for index := range group {
			if group[index] < '0' || group[index] > '9' {
				return false
			}
		}
	}
	value := groups[0] + string(separators[0]) + groups[1] + string(separators[1]) + groups[2]
	layout := "2006-01-02"
	if separators[0] == '/' {
		layout = "2006/01/02"
	}
	parsed, err := time.Parse(layout, value)
	return err == nil && parsed.Format(layout) == value
}

func canonicalReferenceListV1(values []ReferenceV1) bool {
	for index, value := range values {
		if ValidateReferenceV1(string(value)) != nil ||
			(index > 0 && values[index-1] >= value) {
			return false
		}
	}
	return true
}

func canonicalPrivateReferenceListV1(values []string) bool {
	for index, value := range values {
		if !canonicalPrivateReferenceTextV1(value) ||
			(index > 0 && values[index-1] >= value) {
			return false
		}
	}
	return true
}

func canonicalEvidenceOwnerReferenceListV1(values []string) bool {
	for index, value := range values {
		if !canonicalEvidenceOwnerReferenceV1(value) ||
			(index > 0 && values[index-1] >= value) {
			return false
		}
	}
	return true
}

func validSnapshotCurrentnessV1(value string) bool {
	switch value {
	case SnapshotCurrentV1, SnapshotHistoricalV1, SnapshotStaleV1, SnapshotSupersededV1:
		return true
	default:
		return false
	}
}

func validInvestigationStateV1(value string) bool {
	switch value {
	case InvestigationConfirmedV1, InvestigationRejectedV1, InvestigationOpenV1:
		return true
	default:
		return false
	}
}

func validCaseClaimTypeV1(value string) bool {
	switch value {
	case "amount", "count", "account", "entity", "direction", "date_range", "relationship", "quote",
		"device_identifier", "ownership", "control", "address", "change", "bid_certificate",
		"bid_edit_metadata", "legal_characterization":
		return true
	default:
		return false
	}
}

func currentnessCanEvolveV1(previous, current string) bool {
	switch previous {
	case SnapshotCurrentV1:
		return current == SnapshotCurrentV1 ||
			current == SnapshotHistoricalV1 ||
			current == SnapshotStaleV1 ||
			current == SnapshotSupersededV1
	case SnapshotHistoricalV1:
		return current == SnapshotHistoricalV1 ||
			current == SnapshotStaleV1 ||
			current == SnapshotSupersededV1
	case SnapshotStaleV1:
		return current == SnapshotStaleV1 || current == SnapshotSupersededV1
	case SnapshotSupersededV1:
		return current == SnapshotSupersededV1
	default:
		return false
	}
}

func investigationCanEvolveV1(previous, current string) bool {
	switch previous {
	case InvestigationOpenV1:
		return validInvestigationStateV1(current)
	case InvestigationConfirmedV1:
		return current == InvestigationConfirmedV1 || current == InvestigationRejectedV1
	case InvestigationRejectedV1:
		return current == InvestigationRejectedV1
	default:
		return false
	}
}

func referenceSubsetV1(previous, current []ReferenceV1) bool {
	currentSet := make(map[ReferenceV1]struct{}, len(current))
	for _, reference := range current {
		currentSet[reference] = struct{}{}
	}
	for _, reference := range previous {
		if _, found := currentSet[reference]; !found {
			return false
		}
	}
	return true
}

func stringSubsetV1(previous, current []string) bool {
	currentSet := make(map[string]struct{}, len(current))
	for _, value := range current {
		currentSet[value] = struct{}{}
	}
	for _, value := range previous {
		if _, found := currentSet[value]; !found {
			return false
		}
	}
	return true
}

func utf8ByteBoundaryV1(value string, offset int) bool {
	return offset == len(value) || (offset >= 0 && offset < len(value) && utf8.RuneStart(value[offset]))
}

func parseCanonicalPrivateStateV1(
	body []byte,
	target any,
	maxBytes int,
	maxDepth int,
	maxTokens int,
	maxStringBytes int,
) error {
	if target == nil ||
		domainjsonstrict.Validate(body, domainjsonstrict.Options{
			RequireObject:  true,
			MaxBytes:       maxBytes,
			MaxDepth:       maxDepth,
			MaxTokens:      maxTokens,
			MaxStringBytes: maxStringBytes,
		}) != nil {
		return errors.New("case entity private state is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return errors.New("case entity private state is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("case entity private state is invalid")
	}
	canonical, err := json.Marshal(target)
	if err != nil || !bytes.Equal(body, canonical) {
		return errors.New("case entity private state is invalid")
	}
	return nil
}
