package controlledaccount

import (
	"reflect"
	"testing"
)

func TestHostControlledAccountFieldMappingPolicyV1IsClosedAndCanonical(t *testing.T) {
	t.Parallel()

	policy := CurrentHostControlledAccountFieldMappingPolicyV1()
	if err := ValidateHostControlledAccountFieldMappingPolicyV1(policy); err != nil {
		t.Fatal(err)
	}
	wantEntityHeaders := []string{
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
	}
	wantFinancialMappings := []HostControlledAccountFinancialHeaderMappingV1{
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
	}
	if !reflect.DeepEqual(policy.EntityHeaders, wantEntityHeaders) ||
		!reflect.DeepEqual(policy.FinancialAccountHeaderMappings, wantFinancialMappings) {
		t.Fatalf("closed first-stage header vocabulary drifted: %#v", policy)
	}
	if policy.HeaderComparison != ControlledAccountHeaderComparisonExactUTF8V1 ||
		policy.EntityHeaderResolution != ControlledAccountHeaderResolutionExactOneV1 ||
		policy.FinancialHeaderResolution != ControlledAccountHeaderResolutionExactOneV1 ||
		policy.ScalarKind != ControlledAccountFieldScalarTextV1 ||
		policy.ValuePreservation != ControlledAccountValuePreservationSourceExactV1 ||
		policy.FinancialValueCanonicalization != ControlledAccountFinancialCanonicalizationV1 {
		t.Fatalf("mapping semantics drifted: %#v", policy)
	}

	body, err := HostControlledAccountFieldMappingPolicyV1Bytes(policy)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseHostControlledAccountFieldMappingPolicyV1(body)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := HostControlledAccountFieldMappingPolicyV1Bytes(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if string(roundTrip) != string(body) {
		t.Fatal("mapping policy canonical bytes changed on round trip")
	}

	extended := policy
	extended.EntityHeaders = append(append([]string(nil), policy.EntityHeaders...), "name")
	extended.PolicyDigest = hostControlledAccountFieldMappingPolicyDigestV1(extended)
	if ValidateHostControlledAccountFieldMappingPolicyV1(extended) == nil {
		t.Fatal("caller-added header alias was accepted")
	}
}

func TestResolveHostControlledAccountHeadersV1RequiresExactOneTextMapping(t *testing.T) {
	t.Parallel()

	policy := CurrentHostControlledAccountFieldMappingPolicyV1()
	fields := []HostControlledAccountSourceFieldV1{
		{Header: "transaction_time", ScalarKind: ControlledAccountFieldScalarTimestampV1},
		{Header: "amount", ScalarKind: ControlledAccountFieldScalarNumberV1},
		{Header: "account_open_name", ScalarKind: ControlledAccountFieldScalarTextV1},
		{Header: "acct_no", ScalarKind: ControlledAccountFieldScalarTextV1},
		{Header: "card_no", ScalarKind: ControlledAccountFieldScalarTextV1},
	}
	resolved, err := ResolveHostControlledAccountHeadersV1(
		policy,
		fields,
		ControlledAccountFinancialFieldBankAccountNumberV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.EntityHeader != "account_open_name" ||
		resolved.FinancialHeader != "acct_no" ||
		resolved.FinancialAccountField != ControlledAccountFinancialFieldBankAccountNumberV1 ||
		resolved.ScalarKind != ControlledAccountFieldScalarTextV1 ||
		resolved.ValuePreservation != ControlledAccountValuePreservationSourceExactV1 ||
		resolved.PolicyDigest != policy.PolicyDigest {
		t.Fatalf("unexpected resolved mapping: %#v", resolved)
	}

	card, err := ResolveHostControlledAccountHeadersV1(
		policy,
		fields,
		ControlledAccountFinancialFieldBankCardNumberV1,
	)
	if err != nil || card.FinancialHeader != "card_no" {
		t.Fatalf("exact card mapping failed: mapping=%#v err=%v", card, err)
	}

	tests := []struct {
		name   string
		fields []HostControlledAccountSourceFieldV1
		field  string
	}{
		{
			name: "missing entity",
			fields: []HostControlledAccountSourceFieldV1{
				{Header: "acct_no", ScalarKind: ControlledAccountFieldScalarTextV1},
			},
			field: ControlledAccountFinancialFieldBankAccountNumberV1,
		},
		{
			name: "ambiguous entity",
			fields: []HostControlledAccountSourceFieldV1{
				{Header: "account_open_name", ScalarKind: ControlledAccountFieldScalarTextV1},
				{Header: "holder_name", ScalarKind: ControlledAccountFieldScalarTextV1},
				{Header: "acct_no", ScalarKind: ControlledAccountFieldScalarTextV1},
			},
			field: ControlledAccountFinancialFieldBankAccountNumberV1,
		},
		{
			name: "missing exact case",
			fields: []HostControlledAccountSourceFieldV1{
				{Header: "Account_Open_Name", ScalarKind: ControlledAccountFieldScalarTextV1},
				{Header: "Account_No", ScalarKind: ControlledAccountFieldScalarTextV1},
			},
			field: ControlledAccountFinancialFieldBankAccountNumberV1,
		},
		{
			name: "ambiguous account",
			fields: []HostControlledAccountSourceFieldV1{
				{Header: "account_open_name", ScalarKind: ControlledAccountFieldScalarTextV1},
				{Header: "account_no", ScalarKind: ControlledAccountFieldScalarTextV1},
				{Header: "acct_no", ScalarKind: ControlledAccountFieldScalarTextV1},
			},
			field: ControlledAccountFinancialFieldBankAccountNumberV1,
		},
		{
			name: "duplicate",
			fields: []HostControlledAccountSourceFieldV1{
				{Header: "account_open_name", ScalarKind: ControlledAccountFieldScalarTextV1},
				{Header: "acct_no", ScalarKind: ControlledAccountFieldScalarTextV1},
				{Header: "acct_no", ScalarKind: ControlledAccountFieldScalarTextV1},
			},
			field: ControlledAccountFinancialFieldBankAccountNumberV1,
		},
		{
			name: "selected account non text",
			fields: []HostControlledAccountSourceFieldV1{
				{Header: "account_open_name", ScalarKind: ControlledAccountFieldScalarTextV1},
				{Header: "acct_no", ScalarKind: ControlledAccountFieldScalarNumberV1},
			},
			field: ControlledAccountFinancialFieldBankAccountNumberV1,
		},
		{
			name: "selected entity non text",
			fields: []HostControlledAccountSourceFieldV1{
				{Header: "account_open_name", ScalarKind: ControlledAccountFieldScalarNumberV1},
				{Header: "acct_no", ScalarKind: ControlledAccountFieldScalarTextV1},
			},
			field: ControlledAccountFinancialFieldBankAccountNumberV1,
		},
		{
			name: "unsupported unrelated scalar kind",
			fields: []HostControlledAccountSourceFieldV1{
				{Header: "transaction_time", ScalarKind: "date"},
				{Header: "account_open_name", ScalarKind: ControlledAccountFieldScalarTextV1},
				{Header: "acct_no", ScalarKind: ControlledAccountFieldScalarTextV1},
			},
			field: ControlledAccountFinancialFieldBankAccountNumberV1,
		},
		{
			name: "unsupported field",
			fields: []HostControlledAccountSourceFieldV1{
				{Header: "account_open_name", ScalarKind: ControlledAccountFieldScalarTextV1},
				{Header: "acct_no", ScalarKind: ControlledAccountFieldScalarTextV1},
			},
			field: "routing_number",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := ResolveHostControlledAccountHeadersV1(policy, test.fields, test.field); err == nil {
				t.Fatal("hostile or ambiguous mapping was accepted")
			}
		})
	}
}
