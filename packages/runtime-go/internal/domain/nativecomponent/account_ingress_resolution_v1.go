package nativecomponent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	OperationFundsResolveAccountIngress = "funds.resolve_account_ingress"

	AccountIngressResolutionQueryContractV1  = "analytix.account-ingress-resolution-query/v1"
	AccountIngressResolutionResultContractV1 = "analytix.account-ingress-resolution-result/v1"
	AccountIngressResolutionQuerySQLHashV1   = "5fef7c06fe02fd67de5699e1a826f02103fe7c872f586b1f902ff967cf33f18d"

	AccountIngressResolutionMaximumCandidatesV1       = 64
	AccountIngressResolutionMaximumNativeFrameBytesV1 = 32 * 1024
	AccountIngressResolutionMaximumResultBytesV1      = 256 * 1024

	AccountIngressResolutionDispositionResolvedV1         = "resolved"
	AccountIngressResolutionDispositionNotFoundV1         = "not_found"
	AccountIngressResolutionDispositionAmbiguousV1        = "ambiguous"
	AccountIngressResolutionDispositionIntegrityFailureV1 = "integrity_failure"

	accountIngressResolutionMinimumCandidateDigitsV1 = 8
	accountIngressResolutionMaximumCandidateDigitsV1 = 32
	accountIngressResolutionMaximumSemanticBytesV1   = 256
	accountIngressResolutionMaximumCaseIDBytesV1     = 512
)

var resolveAccountIngressRequiredFieldsV1 = []string{
	"caseId",
	"datasetSnapshotId",
	"contextEpoch",
	"contextDigest",
	"caseBindingHash",
	"expectedProducerContentId",
	"expectedProducerManifestSha256",
	"candidates",
}

// ResolveAccountIngressCandidateInputV1 keeps a normalized complete account
// behind a synchronous private callback. Ordinal is the only safe directly
// stored field and binds the response to the host's original candidate order.
type ResolveAccountIngressCandidateInputV1 struct {
	Ordinal             uint32
	normalizedCandidate accountFlowPrivateStringV1
}

func NewResolveAccountIngressCandidateInputV1(
	ordinal uint32,
	normalizedCandidate string,
) (ResolveAccountIngressCandidateInputV1, error) {
	candidate := ResolveAccountIngressCandidateInputV1{
		Ordinal:             ordinal,
		normalizedCandidate: newAccountFlowPrivateStringV1(normalizedCandidate),
	}
	if validateResolveAccountIngressCandidateInputV1(candidate) != nil {
		return ResolveAccountIngressCandidateInputV1{}, ErrRequestInvalid
	}
	return candidate, nil
}

func (ResolveAccountIngressCandidateInputV1) MarshalJSON() ([]byte, error) {
	return nil, ErrRequestInvalid
}

func (*ResolveAccountIngressCandidateInputV1) UnmarshalJSON([]byte) error {
	return ErrRequestInvalid
}

func (candidate ResolveAccountIngressCandidateInputV1) String() string {
	return fmt.Sprintf(
		"ResolveAccountIngressCandidateInputV1{ordinal:%d,normalizedCandidate:[REDACTED]}",
		candidate.Ordinal,
	)
}

func (candidate ResolveAccountIngressCandidateInputV1) GoString() string {
	return candidate.String()
}

type ResolveAccountIngressArgumentsInputV1 struct {
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
	Candidates                           []ResolveAccountIngressCandidateInputV1
}

// ResolveAccountIngressArgumentsV1 is the opaque host-private native request.
// It has no path, SQL, provider, MCP, renderer, history, or durable encoding.
type ResolveAccountIngressArgumentsV1 struct {
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
	candidates                           []ResolveAccountIngressCandidateInputV1
}

type resolveAccountIngressCandidateWireV1 struct {
	Ordinal             uint32 `json:"ordinal"`
	NormalizedCandidate string `json:"normalizedCandidate"`
}

type resolveAccountIngressArgumentsWireV1 struct {
	CaseID                         string                                 `json:"caseId"`
	DatasetSnapshotID              string                                 `json:"datasetSnapshotId"`
	ContextEpoch                   uint64                                 `json:"contextEpoch"`
	ContextDigest                  string                                 `json:"contextDigest"`
	CaseBindingHash                string                                 `json:"caseBindingHash"`
	ExpectedProducerContentID      string                                 `json:"expectedProducerContentId"`
	ExpectedProducerManifestSHA256 string                                 `json:"expectedProducerManifestSha256"`
	Candidates                     []resolveAccountIngressCandidateWireV1 `json:"candidates"`
}

func NewResolveAccountIngressArgumentsV1(
	input ResolveAccountIngressArgumentsInputV1,
) (ResolveAccountIngressArgumentsV1, error) {
	arguments := ResolveAccountIngressArgumentsV1{
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
		candidates:                           append([]ResolveAccountIngressCandidateInputV1(nil), input.Candidates...),
	}
	if validateResolveAccountIngressArgumentsV1(arguments) != nil {
		return ResolveAccountIngressArgumentsV1{}, ErrRequestInvalid
	}
	return arguments, nil
}

func (ResolveAccountIngressArgumentsV1) MarshalJSON() ([]byte, error) {
	return nil, ErrRequestInvalid
}

func (*ResolveAccountIngressArgumentsV1) UnmarshalJSON([]byte) error {
	return ErrRequestInvalid
}

func (arguments ResolveAccountIngressArgumentsV1) String() string {
	return fmt.Sprintf(
		"ResolveAccountIngressArgumentsV1{caseId:%q,datasetSnapshotId:%q,contextEpoch:%d,candidateCount:%d,candidates:[REDACTED]}",
		arguments.caseID,
		arguments.datasetSnapshotID,
		arguments.contextEpoch,
		len(arguments.candidates),
	)
}

func (arguments ResolveAccountIngressArgumentsV1) GoString() string {
	return arguments.String()
}

func validateResolveAccountIngressCandidateInputV1(
	candidate ResolveAccountIngressCandidateInputV1,
) error {
	var value string
	if candidate.normalizedCandidate.useExact(func(current string) error {
		value = current
		return nil
	}) != nil || len(value) < accountIngressResolutionMinimumCandidateDigitsV1 ||
		len(value) > accountIngressResolutionMaximumCandidateDigitsV1 {
		return ErrRequestInvalid
	}
	for index := range value {
		if value[index] < '0' || value[index] > '9' {
			return ErrRequestInvalid
		}
	}
	return nil
}

func validateResolveAccountIngressArgumentsV1(
	arguments ResolveAccountIngressArgumentsV1,
) error {
	if arguments.caseID == "" || arguments.caseID != strings.TrimSpace(arguments.caseID) ||
		len(arguments.caseID) > accountIngressResolutionMaximumCaseIDBytesV1 ||
		strings.IndexFunc(arguments.caseID, unicode.IsControl) >= 0 ||
		!domainsecurity.IsDatasetSnapshotIDV2Syntax(arguments.datasetSnapshotID) ||
		arguments.contextEpoch == 0 ||
		!canonicalLowerDigestV1(arguments.contextDigest) ||
		!canonicalLowerDigestV1(arguments.caseBindingHash) ||
		!domainsecurity.IsFundsProducerContentIDV1Syntax(arguments.expectedProducerContentID) ||
		!canonicalLowerDigestV1(arguments.expectedProducerManifestSHA256) ||
		!canonicalLowerDigestV1(arguments.expectedDuckDBContentSnapshotDigest) ||
		!canonicalLowerDigestV1(arguments.expectedDuckDBSnapshotManifestSHA256) ||
		!strings.HasPrefix(arguments.expectedMaterializationIdentity, accountFlowMaterializationIdentityPrefixV1) ||
		!canonicalLowerDigestV1(strings.TrimPrefix(arguments.expectedMaterializationIdentity, accountFlowMaterializationIdentityPrefixV1)) ||
		len(arguments.candidates) == 0 || len(arguments.candidates) > AccountIngressResolutionMaximumCandidatesV1 {
		return ErrRequestInvalid
	}
	seen := make(map[string]struct{}, len(arguments.candidates))
	var previousOrdinal uint32
	for index, candidate := range arguments.candidates {
		if validateResolveAccountIngressCandidateInputV1(candidate) != nil ||
			index > 0 && candidate.Ordinal <= previousOrdinal {
			return ErrRequestInvalid
		}
		var value string
		if candidate.normalizedCandidate.useExact(func(current string) error {
			value = current
			return nil
		}) != nil {
			return ErrRequestInvalid
		}
		if _, found := seen[value]; found {
			return ErrRequestInvalid
		}
		seen[value] = struct{}{}
		previousOrdinal = candidate.Ordinal
	}
	return nil
}

func resolveAccountIngressPrivatePayloadV1(
	arguments ResolveAccountIngressArgumentsV1,
) ([]byte, error) {
	if validateResolveAccountIngressArgumentsV1(arguments) != nil {
		return nil, ErrRequestInvalid
	}
	candidates := make([]resolveAccountIngressCandidateWireV1, 0, len(arguments.candidates))
	for _, candidate := range arguments.candidates {
		var value string
		if candidate.normalizedCandidate.useExact(func(current string) error {
			value = current
			return nil
		}) != nil {
			return nil, ErrRequestInvalid
		}
		candidates = append(candidates, resolveAccountIngressCandidateWireV1{
			Ordinal: candidate.Ordinal, NormalizedCandidate: value,
		})
	}
	body, err := json.Marshal(resolveAccountIngressArgumentsWireV1{
		CaseID: arguments.caseID, DatasetSnapshotID: arguments.datasetSnapshotID,
		ContextEpoch: arguments.contextEpoch, ContextDigest: arguments.contextDigest,
		CaseBindingHash:                arguments.caseBindingHash,
		ExpectedProducerContentID:      arguments.expectedProducerContentID,
		ExpectedProducerManifestSHA256: arguments.expectedProducerManifestSHA256,
		Candidates:                     candidates,
	})
	if err != nil || len(body) == 0 || len(body) > AccountIngressResolutionMaximumNativeFrameBytesV1 {
		return nil, ErrRequestInvalid
	}
	return body, nil
}

func ValidateResolveAccountIngressArgumentsAuthorityV1(
	arguments ResolveAccountIngressArgumentsV1,
	securityContext domainsecurity.TurnSecurityContext,
	descriptor domainfundsquerysource.DescriptorV1,
) error {
	if validateResolveAccountIngressArgumentsV1(arguments) != nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		domainfundsquerysource.ValidateDescriptorV1(descriptor) != nil ||
		arguments.caseID != securityContext.CaseID ||
		arguments.datasetSnapshotID != securityContext.DatasetSnapshotID ||
		arguments.contextEpoch != securityContext.ContextEpoch ||
		arguments.contextDigest != securityContext.ContextDigest ||
		arguments.caseBindingHash != securityContext.CaseBindingHash ||
		descriptor.CaseID != securityContext.CaseID ||
		descriptor.DatasetSnapshotID != securityContext.DatasetSnapshotID ||
		descriptor.SourceManifestHash != securityContext.SourceManifestHash ||
		descriptor.CaseBindingHash != securityContext.CaseBindingHash ||
		descriptor.BindingObservationDigest != securityContext.PublicationPolicy.BindingObservationDigest ||
		arguments.expectedProducerContentID != descriptor.FundsProducerContentID ||
		arguments.expectedProducerManifestSHA256 != descriptor.FundsProducerContentManifestSHA256 ||
		arguments.expectedDuckDBContentSnapshotDigest != descriptor.DuckDBContentSnapshotDigest ||
		arguments.expectedDuckDBSnapshotManifestSHA256 != descriptor.DuckDBSnapshotManifestSHA256 ||
		arguments.expectedMaterializationIdentity != descriptor.MaterializationIdentity ||
		!domainfundsquerysource.QueryProfileSupportsAccountIngressResolutionV1(
			descriptor.QueryProfileDigest,
		) {
		return ErrContextMismatch
	}
	return nil
}

type resolveAccountIngressNativeRequestFrameWireV1 struct {
	RequestID string          `json:"request_id"`
	Command   string          `json:"command"`
	CaseID    string          `json:"case_id"`
	Payload   json.RawMessage `json:"payload"`
}

func EncodeResolveAccountIngressNativeRequestFrameV1(
	requestID string,
	arguments ResolveAccountIngressArgumentsV1,
) ([]byte, error) {
	if !canonicalLowerDigestV1(requestID) {
		return nil, ErrRequestInvalid
	}
	payload, err := resolveAccountIngressPrivatePayloadV1(arguments)
	if err != nil {
		return nil, ErrRequestInvalid
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(resolveAccountIngressNativeRequestFrameWireV1{
		RequestID: requestID,
		Command:   OperationFundsResolveAccountIngress,
		CaseID:    arguments.caseID,
		Payload:   payload,
	}); err != nil || output.Len() == 0 || output.Len() > AccountIngressResolutionMaximumNativeFrameBytesV1 {
		return nil, ErrRequestInvalid
	}
	frame := output.Bytes()
	if frame[len(frame)-1] != '\n' || bytes.IndexByte(frame[:len(frame)-1], '\n') >= 0 ||
		bytes.IndexByte(frame, '\r') >= 0 || bytes.IndexByte(frame, 0) >= 0 {
		return nil, ErrRequestInvalid
	}
	return append([]byte(nil), frame...), nil
}

type AccountIngressResolutionV1 struct {
	Ordinal         uint32 `json:"ordinal"`
	Disposition     string `json:"disposition"`
	EntityType      string `json:"entityType"`
	BankInstitution string `json:"bankInstitution"`
	AccountType     string `json:"accountType"`
}

type AccountIngressResolutionProvenanceV1 struct {
	DatasetSnapshotID              string `json:"datasetSnapshotId"`
	ContextEpoch                   uint64 `json:"contextEpoch"`
	ContextDigest                  string `json:"contextDigest"`
	CaseBindingHash                string `json:"caseBindingHash"`
	ExpectedProducerContentID      string `json:"expectedProducerContentId"`
	ExpectedProducerManifestSHA256 string `json:"expectedProducerManifestSha256"`
	DuckDBContentSnapshotDigest    string `json:"duckdbContentSnapshotDigest"`
	DuckDBSnapshotManifestSHA256   string `json:"duckdbSnapshotManifestSha256"`
	MaterializationIdentity        string `json:"materializationIdentity"`
	SourceSignature                string `json:"sourceSignature"`
	ResultSignature                string `json:"resultSignature"`
	ProducerContentID              string `json:"producerContentId"`
	ProducerManifestSHA256         string `json:"producerManifestSha256"`
	QueryContract                  string `json:"queryContract"`
	QuerySQLHash                   string `json:"querySqlHash"`
}

// ResolveAccountIngressResultV1 is safe host output: it contains neither the
// complete candidate nor a stable digest derived from it. Ordinals are useful
// only to the synchronous host caller that already owns the private inputs.
type ResolveAccountIngressResultV1 struct {
	SchemaVersion uint32                               `json:"schemaVersion"`
	Contract      string                               `json:"contract"`
	Resolutions   []AccountIngressResolutionV1         `json:"resolutions"`
	Provenance    AccountIngressResolutionProvenanceV1 `json:"provenance"`
}

func ParseResolveAccountIngressResultV1(
	raw []byte,
	arguments ResolveAccountIngressArgumentsV1,
) (ResolveAccountIngressResultV1, error) {
	if len(raw) == 0 || len(raw) > AccountIngressResolutionMaximumResultBytesV1 ||
		validateResolveAccountIngressArgumentsV1(arguments) != nil ||
		domainjsonstrict.Validate(raw, domainjsonstrict.Options{
			RequireObject: true, MaxBytes: AccountIngressResolutionMaximumResultBytesV1,
			MaxDepth: 8, MaxTokens: 2_048, MaxStringBytes: 4_096,
			MaxNumberBytes: 32, MaxAbsExponent: 1,
		}) != nil {
		return ResolveAccountIngressResultV1{}, ErrResultInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var result ResolveAccountIngressResultV1
	if decoder.Decode(&result) != nil {
		return ResolveAccountIngressResultV1{}, ErrResultInvalid
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) ||
		ValidateResolveAccountIngressResultV1(result, arguments) != nil {
		return ResolveAccountIngressResultV1{}, ErrResultInvalid
	}
	result.Resolutions = append([]AccountIngressResolutionV1(nil), result.Resolutions...)
	return result, nil
}

func ValidateResolveAccountIngressResultV1(
	result ResolveAccountIngressResultV1,
	arguments ResolveAccountIngressArgumentsV1,
) error {
	if validateResolveAccountIngressArgumentsV1(arguments) != nil ||
		result.SchemaVersion != 1 || result.Contract != AccountIngressResolutionResultContractV1 ||
		len(result.Resolutions) != len(arguments.candidates) ||
		result.Provenance.DatasetSnapshotID != arguments.datasetSnapshotID ||
		result.Provenance.ContextEpoch != arguments.contextEpoch ||
		result.Provenance.ContextDigest != arguments.contextDigest ||
		result.Provenance.CaseBindingHash != arguments.caseBindingHash ||
		result.Provenance.ExpectedProducerContentID != arguments.expectedProducerContentID ||
		result.Provenance.ExpectedProducerManifestSHA256 != arguments.expectedProducerManifestSHA256 ||
		result.Provenance.DuckDBContentSnapshotDigest != arguments.expectedDuckDBContentSnapshotDigest ||
		result.Provenance.DuckDBSnapshotManifestSHA256 != arguments.expectedDuckDBSnapshotManifestSHA256 ||
		result.Provenance.MaterializationIdentity != arguments.expectedMaterializationIdentity ||
		!canonicalLowerDigestV1(result.Provenance.SourceSignature) ||
		!canonicalLowerDigestV1(result.Provenance.ResultSignature) ||
		result.Provenance.MaterializationIdentity !=
			accountFlowMaterializationIdentityPrefixV1+result.Provenance.SourceSignature ||
		result.Provenance.ProducerContentID != arguments.expectedProducerContentID ||
		result.Provenance.ProducerManifestSHA256 != arguments.expectedProducerManifestSHA256 ||
		result.Provenance.QueryContract != AccountIngressResolutionQueryContractV1 ||
		result.Provenance.QuerySQLHash != AccountIngressResolutionQuerySQLHashV1 {
		return ErrResultInvalid
	}
	for index, resolution := range result.Resolutions {
		if resolution.Ordinal != arguments.candidates[index].Ordinal {
			return ErrResultInvalid
		}
		switch resolution.Disposition {
		case AccountIngressResolutionDispositionResolvedV1:
			if !validAccountIngressEntityTypeV1(resolution.EntityType) ||
				!safeAccountIngressSemanticV1(resolution.BankInstitution) ||
				!safeAccountIngressSemanticV1(resolution.AccountType) {
				return ErrResultInvalid
			}
		case AccountIngressResolutionDispositionNotFoundV1,
			AccountIngressResolutionDispositionAmbiguousV1,
			AccountIngressResolutionDispositionIntegrityFailureV1:
			if resolution.EntityType != "" || resolution.BankInstitution != "" || resolution.AccountType != "" {
				return ErrResultInvalid
			}
		default:
			return ErrResultInvalid
		}
	}
	return nil
}

// ValidateResolveAccountIngressResultAuthorityV1 binds the safe native result
// returned after the first exact-source lease to the complete path-free source
// descriptor and current case turn. It does not re-authorize execution; it
// lets a later private compiler reject provenance loss or cross-selection
// recombination before writing any case-entity state.
func ValidateResolveAccountIngressResultAuthorityV1(
	result ResolveAccountIngressResultV1,
	securityContext domainsecurity.TurnSecurityContext,
	descriptor domainfundsquerysource.DescriptorV1,
) error {
	provenance := result.Provenance
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		domainfundsquerysource.ValidateDescriptorV1(descriptor) != nil ||
		!domainfundsquerysource.QueryProfileSupportsAccountIngressResolutionV1(
			descriptor.QueryProfileDigest,
		) ||
		result.SchemaVersion != 1 ||
		result.Contract != AccountIngressResolutionResultContractV1 ||
		len(result.Resolutions) == 0 ||
		len(result.Resolutions) > AccountIngressResolutionMaximumCandidatesV1 ||
		descriptor.DatasetSnapshotID != securityContext.DatasetSnapshotID ||
		descriptor.SourceManifestHash != securityContext.SourceManifestHash ||
		descriptor.CaseID != securityContext.CaseID ||
		descriptor.CaseBindingHash != securityContext.CaseBindingHash ||
		descriptor.BindingObservationDigest !=
			securityContext.PublicationPolicy.BindingObservationDigest ||
		provenance.DatasetSnapshotID != securityContext.DatasetSnapshotID ||
		provenance.ContextEpoch != securityContext.ContextEpoch ||
		provenance.ContextDigest != securityContext.ContextDigest ||
		provenance.CaseBindingHash != securityContext.CaseBindingHash ||
		provenance.ExpectedProducerContentID != descriptor.FundsProducerContentID ||
		provenance.ExpectedProducerManifestSHA256 !=
			descriptor.FundsProducerContentManifestSHA256 ||
		provenance.ProducerContentID != descriptor.FundsProducerContentID ||
		provenance.ProducerManifestSHA256 !=
			descriptor.FundsProducerContentManifestSHA256 ||
		provenance.DuckDBContentSnapshotDigest != descriptor.DuckDBContentSnapshotDigest ||
		provenance.DuckDBSnapshotManifestSHA256 != descriptor.DuckDBSnapshotManifestSHA256 ||
		provenance.MaterializationIdentity != descriptor.MaterializationIdentity ||
		!canonicalLowerDigestV1(provenance.SourceSignature) ||
		!canonicalLowerDigestV1(provenance.ResultSignature) ||
		provenance.MaterializationIdentity !=
			accountFlowMaterializationIdentityPrefixV1+provenance.SourceSignature ||
		provenance.QueryContract != AccountIngressResolutionQueryContractV1 ||
		provenance.QuerySQLHash != AccountIngressResolutionQuerySQLHashV1 {
		return ErrContextMismatch
	}
	for _, resolution := range result.Resolutions {
		switch resolution.Disposition {
		case AccountIngressResolutionDispositionResolvedV1:
			if !validAccountIngressEntityTypeV1(resolution.EntityType) ||
				!safeAccountIngressSemanticV1(resolution.BankInstitution) ||
				!safeAccountIngressSemanticV1(resolution.AccountType) {
				return ErrContextMismatch
			}
		case AccountIngressResolutionDispositionNotFoundV1,
			AccountIngressResolutionDispositionAmbiguousV1,
			AccountIngressResolutionDispositionIntegrityFailureV1:
			if resolution.EntityType != "" || resolution.BankInstitution != "" ||
				resolution.AccountType != "" {
				return ErrContextMismatch
			}
		default:
			return ErrContextMismatch
		}
	}
	return nil
}

func validAccountIngressEntityTypeV1(value string) bool {
	return value == domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1 ||
		value == domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1
}

func ResolveAccountIngressResultHasFailClosedDispositionV1(
	result ResolveAccountIngressResultV1,
) bool {
	for _, resolution := range result.Resolutions {
		if resolution.Disposition == AccountIngressResolutionDispositionAmbiguousV1 ||
			resolution.Disposition == AccountIngressResolutionDispositionIntegrityFailureV1 {
			return true
		}
	}
	return false
}

func safeAccountIngressSemanticV1(value string) bool {
	if len(value) > accountIngressResolutionMaximumSemanticBytesV1 || !utf8.ValidString(value) ||
		value != strings.TrimSpace(value) {
		return false
	}
	digits := 0
	for _, character := range value {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return false
		}
		switch {
		case unicode.IsDigit(character):
			digits++
			if digits >= accountIngressResolutionMinimumCandidateDigitsV1 {
				return false
			}
		case unicode.IsSpace(character) || unicode.IsPunct(character) || unicode.IsSymbol(character):
		default:
			digits = 0
		}
	}
	return true
}

func canonicalLowerDigestV1(value string) bool {
	return value == strings.ToLower(value) && domainsecurity.IsSHA256Hex(value)
}
