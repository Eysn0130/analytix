package nativecomponent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	OperationFundsAnalyzeAccountFlows = "funds.analyze_account_flows"

	AccountFlowQueryContractV1 = "analytix.account-flow-query/v1"

	AccountFlowMaximumRequestBytesV1 = 16 * 1024
	AccountFlowMaximumResultBytesV1  = 1024 * 1024
	// One complete account-flow result must fit one EvidenceReceiptV2 source
	// lineage set; the receipt contract permits at most 512 source records.
	AccountFlowMaximumEvidenceRowsV1 = uint32(512)
	AccountFlowMaximumScanRowsV1     = uint32(100_000)
	AccountFlowMinorUnitScaleV1      = uint8(2)

	accountFlowMaximumCaseIDBytesV1             = 512
	accountFlowMaximumResolvedAccountKeyBytesV1 = 4_096
	accountFlowMaximumTimestampBytesV1          = 64
	accountFlowMinimumUTCOffsetMinutesV1        = int16(-840)
	accountFlowMaximumUTCOffsetMinutesV1        = int16(840)
	accountFlowMaximumSourceRowNumberV1         = uint64(9_007_199_254_740_991)
	accountFlowMaximumPrivateTextBytesV1        = 4_096

	accountFlowQuerySQLHashV1 = "d06b98ac697a24f6cca52d33ead6839ad9d2eed6ce9147f967d1a0a4e79a5b51"
)

var analyzeAccountFlowsRequiredFieldsV1 = []string{
	"caseId",
	"datasetSnapshotId",
	"contextEpoch",
	"contextDigest",
	"caseBindingHash",
	"expectedProducerContentId",
	"expectedProducerManifestSha256",
	"subjectRef",
	"resolvedAccountKey",
	"subjectResolutionDigest",
	"startInclusive",
	"endInclusive",
	"evidenceRowLimit",
	"datasetUtcOffsetMinutes",
	"expectedCurrency",
	"minorUnitScale",
	"scanCap",
}

// AnalyzeAccountFlowsArgumentsInputV1 is assembled only by the trusted host
// after current case, entity and DSV2 resolution. It is not a provider, MCP,
// renderer, history or durable tool-call schema.
type AnalyzeAccountFlowsArgumentsInputV1 struct {
	CaseID                               string
	DatasetSnapshotID                    string
	ContextEpoch                         uint64
	ContextDigest                        string
	CaseBindingHash                      string
	ExpectedProducerContentID            string
	ExpectedProducerManifestSHA256       string
	ExpectedDuckDBContentSnapshotDigest  string
	ExpectedDuckDBSnapshotManifestSHA256 string
	ExpectedMaterializationIdentity      string
	SubjectAlias                         string
	SubjectRef                           string
	resolvedAccountKey                   accountFlowPrivateStringV1
	SubjectResolutionDigest              string
	StartInclusive                       string
	EndInclusive                         string
	EvidenceRowLimit                     uint32
	DatasetUTCOffsetMinutes              int16
	ExpectedCurrency                     string
	MinorUnitScale                       uint8
	ScanCap                              uint32
}

// accountFlowPrivateStringV1 keeps host-private text behind a synchronous
// callback. The exported values that contain it therefore hold only a
// function value: removing their methods through a defined-type conversion
// cannot expose the captured text through encoding/json or fmt.
type accountFlowPrivateStringV1 struct {
	use func(func(string) error) error
}

func newAccountFlowPrivateStringV1(value string) accountFlowPrivateStringV1 {
	return accountFlowPrivateStringV1{
		use: func(consume func(string) error) error {
			if consume == nil {
				return ErrRequestInvalid
			}
			return consume(value)
		},
	}
}

func (private accountFlowPrivateStringV1) useExact(consume func(string) error) error {
	if private.use == nil || consume == nil {
		return ErrRequestInvalid
	}
	calls := 0
	err := private.use(func(value string) error {
		calls++
		if calls != 1 {
			return ErrRequestInvalid
		}
		return consume(value)
	})
	if err != nil || calls != 1 {
		return ErrRequestInvalid
	}
	return nil
}

func NewAnalyzeAccountFlowsArgumentsInputV1(
	input AnalyzeAccountFlowsArgumentsInputV1,
	resolvedAccountKey string,
) AnalyzeAccountFlowsArgumentsInputV1 {
	input.resolvedAccountKey = newAccountFlowPrivateStringV1(resolvedAccountKey)
	return input
}

func (input AnalyzeAccountFlowsArgumentsInputV1) MarshalJSON() ([]byte, error) {
	return nil, ErrRequestInvalid
}

func (input AnalyzeAccountFlowsArgumentsInputV1) String() string {
	return fmt.Sprintf(
		"AnalyzeAccountFlowsArgumentsInputV1{caseId:%q,datasetSnapshotId:%q,contextEpoch:%d,subjectAlias:[REDACTED],subjectRef:[REDACTED],resolvedAccountKey:[REDACTED],startInclusive:%q,endInclusive:%q,evidenceRowLimit:%d,scanCap:%d}",
		input.CaseID,
		input.DatasetSnapshotID,
		input.ContextEpoch,
		input.StartInclusive,
		input.EndInclusive,
		input.EvidenceRowLimit,
		input.ScanCap,
	)
}

func (input AnalyzeAccountFlowsArgumentsInputV1) GoString() string {
	return input.String()
}

// AnalyzeAccountFlowsArgumentsV1 is an opaque host-private value. Its fields
// are deliberately unexported so ordinary JSON serialization cannot disclose
// the resolved account key or invent a path/SQL extension. Only the fixed
// native contract validator may derive its private Rust payload and grant
// argument hash.
type AnalyzeAccountFlowsArgumentsV1 struct {
	caseID                               string
	datasetSnapshotID                    string
	contextEpoch                         uint64
	contextDigest                        string
	caseBindingHash                      string
	expectedProducerContentID            string
	expectedProducerManifestSHA256       string
	expectedDuckDBContentSnapshotDigest  string
	expectedDuckDBSnapshotManifestSHA256 string
	expectedMaterializationIdentity      string
	subjectAlias                         string
	subjectRef                           string
	resolvedAccountKey                   accountFlowPrivateStringV1
	subjectResolutionDigest              string
	startInclusive                       string
	endInclusive                         string
	evidenceRowLimit                     uint32
	datasetUTCOffsetMinutes              int16
	expectedCurrency                     string
	minorUnitScale                       uint8
	scanCap                              uint32
}

type analyzeAccountFlowsArgumentsWireV1 struct {
	CaseID                         string `json:"caseId"`
	DatasetSnapshotID              string `json:"datasetSnapshotId"`
	ContextEpoch                   uint64 `json:"contextEpoch"`
	ContextDigest                  string `json:"contextDigest"`
	CaseBindingHash                string `json:"caseBindingHash"`
	ExpectedProducerContentID      string `json:"expectedProducerContentId"`
	ExpectedProducerManifestSHA256 string `json:"expectedProducerManifestSha256"`
	SubjectRef                     string `json:"subjectRef"`
	ResolvedAccountKey             string `json:"resolvedAccountKey"`
	SubjectResolutionDigest        string `json:"subjectResolutionDigest"`
	StartInclusive                 string `json:"startInclusive"`
	EndInclusive                   string `json:"endInclusive"`
	EvidenceRowLimit               uint32 `json:"evidenceRowLimit"`
	DatasetUTCOffsetMinutes        int16  `json:"datasetUtcOffsetMinutes"`
	ExpectedCurrency               string `json:"expectedCurrency"`
	MinorUnitScale                 uint8  `json:"minorUnitScale"`
	ScanCap                        uint32 `json:"scanCap"`
}

// analyzeAccountFlowsGrantArgumentsWireV1 extends the fixed Rust payload only
// for the host-owned execution-grant hash. The expected provenance fields are
// deliberately not serialized into the deny-unknown-fields Rust request.
type analyzeAccountFlowsGrantArgumentsWireV1 struct {
	analyzeAccountFlowsArgumentsWireV1
	ExpectedDuckDBContentSnapshotDigest  string `json:"expectedDuckdbContentSnapshotDigest"`
	ExpectedDuckDBSnapshotManifestSHA256 string `json:"expectedDuckdbSnapshotManifestSha256"`
	ExpectedMaterializationIdentity      string `json:"expectedMaterializationIdentity"`
}

func NewAnalyzeAccountFlowsArgumentsV1(input AnalyzeAccountFlowsArgumentsInputV1) (AnalyzeAccountFlowsArgumentsV1, error) {
	startInclusive, startOK := canonicalAccountFlowTimestampV1(input.StartInclusive)
	endInclusive, endOK := canonicalAccountFlowTimestampV1(input.EndInclusive)
	arguments := AnalyzeAccountFlowsArgumentsV1{
		caseID:                               input.CaseID,
		datasetSnapshotID:                    input.DatasetSnapshotID,
		contextEpoch:                         input.ContextEpoch,
		contextDigest:                        input.ContextDigest,
		caseBindingHash:                      input.CaseBindingHash,
		expectedProducerContentID:            input.ExpectedProducerContentID,
		expectedProducerManifestSHA256:       input.ExpectedProducerManifestSHA256,
		expectedDuckDBContentSnapshotDigest:  input.ExpectedDuckDBContentSnapshotDigest,
		expectedDuckDBSnapshotManifestSHA256: input.ExpectedDuckDBSnapshotManifestSHA256,
		expectedMaterializationIdentity:      input.ExpectedMaterializationIdentity,
		subjectAlias:                         input.SubjectAlias,
		subjectRef:                           input.SubjectRef,
		resolvedAccountKey:                   input.resolvedAccountKey,
		subjectResolutionDigest:              input.SubjectResolutionDigest,
		startInclusive:                       startInclusive,
		endInclusive:                         endInclusive,
		evidenceRowLimit:                     input.EvidenceRowLimit,
		datasetUTCOffsetMinutes:              input.DatasetUTCOffsetMinutes,
		expectedCurrency:                     input.ExpectedCurrency,
		minorUnitScale:                       input.MinorUnitScale,
		scanCap:                              input.ScanCap,
	}
	if !startOK || !endOK || validateAnalyzeAccountFlowsArgumentsV1(arguments) != nil {
		return AnalyzeAccountFlowsArgumentsV1{}, ErrRequestInvalid
	}
	return arguments, nil
}

// AnalyzeAccountFlowsArgumentsHashV1 binds a host grant to the fixed private
// Rust request and the exact host-expected snapshot/materialization provenance
// without making either value JSON-serializable to ordinary callers.
func AnalyzeAccountFlowsArgumentsHashV1(arguments AnalyzeAccountFlowsArgumentsV1) string {
	if validateAnalyzeAccountFlowsArgumentsV1(arguments) != nil {
		return ""
	}
	var body []byte
	err := withAnalyzeAccountFlowsArgumentsWireV1(arguments, func(wire analyzeAccountFlowsArgumentsWireV1) error {
		var marshalErr error
		body, marshalErr = json.Marshal(analyzeAccountFlowsGrantArgumentsWireV1{
			analyzeAccountFlowsArgumentsWireV1:   wire,
			ExpectedDuckDBContentSnapshotDigest:  arguments.expectedDuckDBContentSnapshotDigest,
			ExpectedDuckDBSnapshotManifestSHA256: arguments.expectedDuckDBSnapshotManifestSHA256,
			ExpectedMaterializationIdentity:      arguments.expectedMaterializationIdentity,
		})
		return marshalErr
	})
	if err != nil || len(body) == 0 || len(body) > AccountFlowMaximumRequestBytesV1 {
		return ""
	}
	return domainsecurity.CanonicalJSONHash(body)
}

func (arguments AnalyzeAccountFlowsArgumentsV1) MarshalJSON() ([]byte, error) {
	return nil, ErrRequestInvalid
}

func (arguments AnalyzeAccountFlowsArgumentsV1) String() string {
	return fmt.Sprintf(
		"AnalyzeAccountFlowsArgumentsV1{caseId:%q,datasetSnapshotId:%q,contextEpoch:%d,subjectAlias:[REDACTED],subjectRef:[REDACTED],resolvedAccountKey:[REDACTED],startInclusive:%q,endInclusive:%q,evidenceRowLimit:%d,scanCap:%d}",
		arguments.caseID,
		arguments.datasetSnapshotID,
		arguments.contextEpoch,
		arguments.startInclusive,
		arguments.endInclusive,
		arguments.evidenceRowLimit,
		arguments.scanCap,
	)
}

func (arguments AnalyzeAccountFlowsArgumentsV1) GoString() string {
	return arguments.String()
}

func (request Request) String() string {
	arguments := "<nil>"
	if request.AccountFlowArguments != nil {
		arguments = request.AccountFlowArguments.String()
	}
	return fmt.Sprintf(
		"Request{componentId:%q,operation:%q,accountFlowArguments:%s,contextDigest:%q,grantId:%q,deadline:%q}",
		request.ComponentID,
		request.Operation,
		arguments,
		request.Context.ContextDigest,
		request.Grant.GrantID,
		request.Deadline.UTC().Format(time.RFC3339Nano),
	)
}

func (request Request) GoString() string {
	return request.String()
}

func validateAnalyzeAccountFlowsArgumentsV1(arguments AnalyzeAccountFlowsArgumentsV1) error {
	start, startOK := canonicalAccountFlowTimestampV1(arguments.startInclusive)
	end, endOK := canonicalAccountFlowTimestampV1(arguments.endInclusive)
	if !canonicalBoundedAccountFlowTextV1(arguments.caseID, accountFlowMaximumCaseIDBytesV1) ||
		arguments.caseID == domainsecurity.UnboundCaseID ||
		!domainsecurity.IsDatasetSnapshotIDV2Syntax(arguments.datasetSnapshotID) ||
		arguments.contextEpoch == 0 ||
		!canonicalAccountFlowDigestV1(arguments.contextDigest) ||
		!canonicalAccountFlowDigestV1(arguments.caseBindingHash) ||
		!domainsecurity.IsFundsProducerContentIDV1Syntax(arguments.expectedProducerContentID) ||
		!canonicalAccountFlowDigestV1(arguments.expectedProducerManifestSHA256) ||
		!canonicalAccountFlowDigestV1(arguments.expectedDuckDBContentSnapshotDigest) ||
		!canonicalAccountFlowDigestV1(arguments.expectedDuckDBSnapshotManifestSHA256) ||
		!canonicalAccountFlowMaterializationIdentityV1(arguments.expectedMaterializationIdentity) ||
		domaincaseentity.ValidateModelEntityAliasV1(arguments.subjectAlias) != nil ||
		domaincaseentity.ValidateReferenceV1(arguments.subjectRef) != nil ||
		!canonicalAccountFlowDigestV1(arguments.subjectResolutionDigest) ||
		!startOK || !endOK || start != arguments.startInclusive || end != arguments.endInclusive || start > end ||
		arguments.evidenceRowLimit == 0 || arguments.evidenceRowLimit > AccountFlowMaximumEvidenceRowsV1 ||
		arguments.datasetUTCOffsetMinutes < accountFlowMinimumUTCOffsetMinutesV1 ||
		arguments.datasetUTCOffsetMinutes > accountFlowMaximumUTCOffsetMinutesV1 ||
		!canonicalAccountFlowCurrencyV1(arguments.expectedCurrency) ||
		arguments.minorUnitScale != AccountFlowMinorUnitScaleV1 ||
		arguments.scanCap == 0 || arguments.scanCap > AccountFlowMaximumScanRowsV1 {
		return ErrRequestInvalid
	}
	var body []byte
	err := withAnalyzeAccountFlowsArgumentsWireV1(arguments, func(wire analyzeAccountFlowsArgumentsWireV1) error {
		if !canonicalBoundedAccountFlowTextV1(wire.ResolvedAccountKey, accountFlowMaximumResolvedAccountKeyBytesV1) {
			return ErrRequestInvalid
		}
		var marshalErr error
		body, marshalErr = json.Marshal(wire)
		return marshalErr
	})
	if err != nil || len(body) == 0 || len(body) > AccountFlowMaximumRequestBytesV1 {
		return ErrRequestInvalid
	}
	return nil
}

func analyzeAccountFlowsArgumentsMatchContextV1(
	arguments AnalyzeAccountFlowsArgumentsV1,
	securityContext domainsecurity.TurnSecurityContext,
) bool {
	return validateAnalyzeAccountFlowsArgumentsV1(arguments) == nil &&
		arguments.caseID == securityContext.CaseID &&
		arguments.datasetSnapshotID == securityContext.DatasetSnapshotID &&
		arguments.contextEpoch == securityContext.ContextEpoch &&
		arguments.contextDigest == securityContext.ContextDigest &&
		arguments.caseBindingHash == securityContext.CaseBindingHash
}

func analyzeAccountFlowsPrivatePayloadV1(arguments AnalyzeAccountFlowsArgumentsV1) ([]byte, error) {
	if validateAnalyzeAccountFlowsArgumentsV1(arguments) != nil {
		return nil, ErrRequestInvalid
	}
	var body []byte
	err := withAnalyzeAccountFlowsArgumentsWireV1(arguments, func(wire analyzeAccountFlowsArgumentsWireV1) error {
		var marshalErr error
		body, marshalErr = json.Marshal(wire)
		return marshalErr
	})
	if err != nil || len(body) == 0 || len(body) > AccountFlowMaximumRequestBytesV1 {
		return nil, ErrRequestInvalid
	}
	return body, nil
}

func withAnalyzeAccountFlowsArgumentsWireV1(
	arguments AnalyzeAccountFlowsArgumentsV1,
	consume func(analyzeAccountFlowsArgumentsWireV1) error,
) error {
	if consume == nil {
		return ErrRequestInvalid
	}
	return arguments.resolvedAccountKey.useExact(func(resolvedAccountKey string) error {
		return consume(analyzeAccountFlowsArgumentsWireV1{
			CaseID:                         arguments.caseID,
			DatasetSnapshotID:              arguments.datasetSnapshotID,
			ContextEpoch:                   arguments.contextEpoch,
			ContextDigest:                  arguments.contextDigest,
			CaseBindingHash:                arguments.caseBindingHash,
			ExpectedProducerContentID:      arguments.expectedProducerContentID,
			ExpectedProducerManifestSHA256: arguments.expectedProducerManifestSHA256,
			SubjectRef:                     arguments.subjectRef,
			ResolvedAccountKey:             resolvedAccountKey,
			SubjectResolutionDigest:        arguments.subjectResolutionDigest,
			StartInclusive:                 arguments.startInclusive,
			EndInclusive:                   arguments.endInclusive,
			EvidenceRowLimit:               arguments.evidenceRowLimit,
			DatasetUTCOffsetMinutes:        arguments.datasetUTCOffsetMinutes,
			ExpectedCurrency:               arguments.expectedCurrency,
			MinorUnitScale:                 arguments.minorUnitScale,
			ScanCap:                        arguments.scanCap,
		})
	})
}

func canonicalAccountFlowTimestampV1(value string) (string, bool) {
	if !canonicalBoundedAccountFlowTextV1(value, accountFlowMaximumTimestampBytesV1) {
		return "", false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.Nanosecond()%1_000 != 0 {
		return "", false
	}
	return parsed.UTC().Format("2006-01-02T15:04:05.000000Z"), true
}

func accountFlowTimezoneV1(offsetMinutes int16) string {
	if offsetMinutes == 0 {
		return "Z"
	}
	absolute := int(offsetMinutes)
	sign := "+"
	if absolute < 0 {
		sign = "-"
		absolute = -absolute
	}
	return fmt.Sprintf("%s%02d:%02d", sign, absolute/60, absolute%60)
}

func canonicalBoundedAccountFlowTextV1(value string, maximumBytes int) bool {
	if value == "" || len(value) > maximumBytes || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, current := range value {
		// Go's encoding/json escapes these two separators while serde_json emits
		// them verbatim. Rejecting them keeps the cross-language hash grammar
		// single-valued without weakening any useful account-flow semantic.
		if current == '\u2028' || current == '\u2029' || unicode.IsControl(current) {
			return false
		}
	}
	return true
}

func canonicalAccountFlowDigestV1(value string) bool {
	return value == strings.ToLower(value) && domainsecurity.IsSHA256Hex(value)
}

func canonicalAccountFlowMaterializationIdentityV1(value string) bool {
	return strings.HasPrefix(value, accountFlowMaterializationIdentityPrefixV1) &&
		canonicalAccountFlowDigestV1(strings.TrimPrefix(value, accountFlowMaterializationIdentityPrefixV1))
}

func canonicalAccountFlowCurrencyV1(value string) bool {
	if len(value) != 3 {
		return false
	}
	for index := range value {
		if value[index] < 'A' || value[index] > 'Z' {
			return false
		}
	}
	return true
}
