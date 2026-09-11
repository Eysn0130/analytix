package controlledaccount

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	HostControlledAccountFieldMappingPolicySchemaVersionV1 = 1
	HostControlledAccountFieldMappingPolicyPurposeV1       = "analytix.host-controlled-account-field-mapping-policy/v1"
	HostControlledAccountFieldMappingPolicyIDV1            = "analytix.first-stage-controlled-account-csv/v1"

	ControlledAccountFinancialFieldBankAccountNumberV1 = "bank_account_number"
	ControlledAccountFinancialFieldBankCardNumberV1    = "bank_card_number"

	ControlledAccountFieldScalarTextV1              = "text"
	ControlledAccountFieldScalarNumberV1            = "number"
	ControlledAccountFieldScalarTimestampV1         = "timestamp"
	ControlledAccountHeaderComparisonExactUTF8V1    = "exact_utf8_bytes"
	ControlledAccountHeaderResolutionExactOneV1     = "exactly_one_header"
	ControlledAccountValuePreservationSourceExactV1 = "source_exact_utf8_bytes"
	ControlledAccountEntityCanonicalizationV1       = "dataset_selection_bound_source_exact_utf8_sha256/v1"
	ControlledAccountFinancialCanonicalizationV1    = "analytix.bank-account-text/v2"
	ControlledAccountMappingRejectionMissingV1      = "reject_missing"
	ControlledAccountMappingRejectionAmbiguousV1    = "reject_ambiguous"
	ControlledAccountMappingRejectionDuplicateV1    = "reject_duplicate"
	ControlledAccountMappingRejectionNonTextV1      = "reject_non_text"
	maxControlledAccountSourceHeaderCountV1         = 256
	maxControlledAccountSourceHeaderBytesV1         = 1024
)

var hostControlledAccountFieldMappingPolicyDigestDomainV1 = []byte(
	"analytix.host-controlled-account-field-mapping-policy/digest/v1\x00",
)

type HostControlledAccountFinancialHeaderMappingV1 struct {
	FinancialAccountField string   `json:"financialAccountField"`
	Headers               []string `json:"headers"`
}

// HostControlledAccountFieldMappingPolicyV1 is the complete first-stage
// mapping vocabulary. Matching is byte-exact and case-sensitive; no alias,
// normalization, numeric conversion, or caller-supplied fallback is allowed.
// ScalarKind names the only kind permitted for selected entity and financial
// fields. Unrelated descriptors may use another closed scalar kind.
type HostControlledAccountFieldMappingPolicyV1 struct {
	SchemaVersion                  int                                             `json:"schemaVersion"`
	Purpose                        string                                          `json:"purpose"`
	PolicyID                       string                                          `json:"policyId"`
	HeaderComparison               string                                          `json:"headerComparison"`
	EntityHeaderResolution         string                                          `json:"entityHeaderResolution"`
	FinancialHeaderResolution      string                                          `json:"financialHeaderResolution"`
	ScalarKind                     string                                          `json:"scalarKind"`
	ValuePreservation              string                                          `json:"valuePreservation"`
	EntityCanonicalization         string                                          `json:"entityCanonicalization"`
	FinancialValueCanonicalization string                                          `json:"financialValueCanonicalization"`
	MissingDisposition             string                                          `json:"missingDisposition"`
	AmbiguousDisposition           string                                          `json:"ambiguousDisposition"`
	DuplicateDisposition           string                                          `json:"duplicateDisposition"`
	NonTextDisposition             string                                          `json:"nonTextDisposition"`
	EntityHeaders                  []string                                        `json:"entityHeaders"`
	FinancialAccountHeaderMappings []HostControlledAccountFinancialHeaderMappingV1 `json:"financialAccountHeaderMappings"`
	PolicyDigest                   string                                          `json:"policyDigest"`
}

type HostControlledAccountSourceFieldV1 struct {
	Header     string `json:"header"`
	ScalarKind string `json:"scalarKind"`
}

type HostControlledAccountResolvedFieldMappingV1 struct {
	PolicyDigest          string `json:"policyDigest"`
	EntityHeader          string `json:"entityHeader"`
	FinancialAccountField string `json:"financialAccountField"`
	FinancialHeader       string `json:"financialHeader"`
	ScalarKind            string `json:"scalarKind"`
	ValuePreservation     string `json:"valuePreservation"`
}

func CurrentHostControlledAccountFieldMappingPolicyV1() HostControlledAccountFieldMappingPolicyV1 {
	policy := HostControlledAccountFieldMappingPolicyV1{
		SchemaVersion:                  HostControlledAccountFieldMappingPolicySchemaVersionV1,
		Purpose:                        HostControlledAccountFieldMappingPolicyPurposeV1,
		PolicyID:                       HostControlledAccountFieldMappingPolicyIDV1,
		HeaderComparison:               ControlledAccountHeaderComparisonExactUTF8V1,
		EntityHeaderResolution:         ControlledAccountHeaderResolutionExactOneV1,
		FinancialHeaderResolution:      ControlledAccountHeaderResolutionExactOneV1,
		ScalarKind:                     ControlledAccountFieldScalarTextV1,
		ValuePreservation:              ControlledAccountValuePreservationSourceExactV1,
		EntityCanonicalization:         ControlledAccountEntityCanonicalizationV1,
		FinancialValueCanonicalization: ControlledAccountFinancialCanonicalizationV1,
		MissingDisposition:             ControlledAccountMappingRejectionMissingV1,
		AmbiguousDisposition:           ControlledAccountMappingRejectionAmbiguousV1,
		DuplicateDisposition:           ControlledAccountMappingRejectionDuplicateV1,
		NonTextDisposition:             ControlledAccountMappingRejectionNonTextV1,
		EntityHeaders: []string{
			"account_name",
			"account_open_name",
			"customer_name",
			"entity_id",
			"entity_name",
			"holder_name",
			"open_name",
			"主体",
			"主体标识",
			"开户名称",
			"开户户名",
			"户名",
		},
		FinancialAccountHeaderMappings: []HostControlledAccountFinancialHeaderMappingV1{
			{
				FinancialAccountField: ControlledAccountFinancialFieldBankAccountNumberV1,
				Headers: []string{
					"account_no",
					"account_number",
					"acct_no",
					"bank_account_no",
					"bank_account_number",
					"账号",
					"银行账号",
				},
			},
			{
				FinancialAccountField: ControlledAccountFinancialFieldBankCardNumberV1,
				Headers: []string{
					"bank_card_no",
					"bank_card_number",
					"card_no",
					"card_number",
					"卡号",
					"银行卡号",
				},
			},
		},
	}
	policy.PolicyDigest = hostControlledAccountFieldMappingPolicyDigestV1(policy)
	return policy
}

func ValidateHostControlledAccountFieldMappingPolicyV1(
	policy HostControlledAccountFieldMappingPolicyV1,
) error {
	expected := CurrentHostControlledAccountFieldMappingPolicyV1()
	if !reflect.DeepEqual(policy, expected) ||
		!canonicalControlledAccountDigestV1(policy.PolicyDigest) ||
		policy.PolicyDigest != hostControlledAccountFieldMappingPolicyDigestV1(policy) {
		return errors.New("host controlled account field mapping policy is invalid")
	}
	return nil
}

func ParseHostControlledAccountFieldMappingPolicyV1(
	raw []byte,
) (HostControlledAccountFieldMappingPolicyV1, error) {
	var policy HostControlledAccountFieldMappingPolicyV1
	if err := parseCanonicalControlledAccountContractV1(raw, &policy); err != nil {
		return HostControlledAccountFieldMappingPolicyV1{}, err
	}
	return policy, ValidateHostControlledAccountFieldMappingPolicyV1(policy)
}

func HostControlledAccountFieldMappingPolicyV1Bytes(
	policy HostControlledAccountFieldMappingPolicyV1,
) ([]byte, error) {
	if err := ValidateHostControlledAccountFieldMappingPolicyV1(policy); err != nil {
		return nil, err
	}
	return controlledAccountContractBytesV1(policy)
}

// ResolveHostControlledAccountHeadersV1 selects exactly one entity header and
// exactly one header for the explicitly allowed account field. It observes
// descriptors only; source values never cross this mapping boundary.
func ResolveHostControlledAccountHeadersV1(
	policy HostControlledAccountFieldMappingPolicyV1,
	fields []HostControlledAccountSourceFieldV1,
	allowedFinancialAccountField string,
) (HostControlledAccountResolvedFieldMappingV1, error) {
	if ValidateHostControlledAccountFieldMappingPolicyV1(policy) != nil ||
		!validControlledAccountFinancialFieldV1(allowedFinancialAccountField) ||
		len(fields) == 0 || len(fields) > maxControlledAccountSourceHeaderCountV1 {
		return HostControlledAccountResolvedFieldMappingV1{}, errors.New(
			"host controlled account field mapping input is invalid",
		)
	}

	mapping, ok := financialHeaderMappingForFieldV1(policy, allowedFinancialAccountField)
	if !ok {
		return HostControlledAccountResolvedFieldMappingV1{}, errors.New(
			"host controlled account financial field is unsupported",
		)
	}
	seen := make(map[string]struct{}, len(fields))
	entityMatches := make([]string, 0, 2)
	financialMatches := make([]string, 0, 2)
	for _, field := range fields {
		if !canonicalControlledAccountOpaqueTextV1(field.Header, maxControlledAccountSourceHeaderBytesV1) ||
			!validControlledAccountSourceScalarKindV1(field.ScalarKind) {
			return HostControlledAccountResolvedFieldMappingV1{}, errors.New(
				"host controlled account source field descriptor is invalid",
			)
		}
		if _, duplicate := seen[field.Header]; duplicate {
			return HostControlledAccountResolvedFieldMappingV1{}, errors.New(
				"host controlled account source header is duplicated",
			)
		}
		seen[field.Header] = struct{}{}
		entityMatch := slices.Contains(policy.EntityHeaders, field.Header)
		financialMatch := slices.Contains(mapping.Headers, field.Header)
		if (entityMatch || financialMatch) && field.ScalarKind != policy.ScalarKind {
			return HostControlledAccountResolvedFieldMappingV1{}, errors.New(
				"host controlled account selected source field is non-text",
			)
		}
		if entityMatch {
			entityMatches = append(entityMatches, field.Header)
		}
		if financialMatch {
			financialMatches = append(financialMatches, field.Header)
		}
	}
	if len(entityMatches) != 1 || len(financialMatches) != 1 {
		return HostControlledAccountResolvedFieldMappingV1{}, errors.New(
			"host controlled account source header mapping is missing or ambiguous",
		)
	}
	return HostControlledAccountResolvedFieldMappingV1{
		PolicyDigest:          policy.PolicyDigest,
		EntityHeader:          entityMatches[0],
		FinancialAccountField: allowedFinancialAccountField,
		FinancialHeader:       financialMatches[0],
		ScalarKind:            policy.ScalarKind,
		ValuePreservation:     policy.ValuePreservation,
	}, nil
}

func validControlledAccountSourceScalarKindV1(value string) bool {
	return value == ControlledAccountFieldScalarTextV1 ||
		value == ControlledAccountFieldScalarNumberV1 ||
		value == ControlledAccountFieldScalarTimestampV1
}

func validControlledAccountFinancialFieldV1(value string) bool {
	return value == ControlledAccountFinancialFieldBankAccountNumberV1 ||
		value == ControlledAccountFinancialFieldBankCardNumberV1
}

func financialHeaderMappingForFieldV1(
	policy HostControlledAccountFieldMappingPolicyV1,
	field string,
) (HostControlledAccountFinancialHeaderMappingV1, bool) {
	for _, mapping := range policy.FinancialAccountHeaderMappings {
		if mapping.FinancialAccountField == field {
			return mapping, true
		}
	}
	return HostControlledAccountFinancialHeaderMappingV1{}, false
}

func hostControlledAccountFieldMappingPolicyDigestV1(
	policy HostControlledAccountFieldMappingPolicyV1,
) string {
	policy.PolicyDigest = ""
	body, _ := json.Marshal(policy)
	return domainsecurity.SHA256Hex(append(
		append([]byte(nil), hostControlledAccountFieldMappingPolicyDigestDomainV1...),
		body...,
	))
}
