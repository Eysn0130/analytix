package nativecomponent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestResolveAccountIngressArgumentsAndFrameKeepCandidatesPrivate(t *testing.T) {
	const exactAccount = "6222021234567890123"
	arguments := nativeComponentTestAccountIngressArguments(
		t,
		nativeComponentTestContext(
			t,
			"case-account-ingress",
			time.Date(2026, 7, 28, 1, 0, 0, 0, time.UTC),
		),
		[]string{exactAccount, "6217009876543210987"},
	)
	for _, value := range []any{arguments, arguments.candidates[0]} {
		if encoded, err := json.Marshal(value); err == nil || encoded != nil {
			t.Fatalf("private value serialized: %T %q err=%v", value, encoded, err)
		}
		if strings.Contains(fmt.Sprintf("%v", value), exactAccount) ||
			strings.Contains(fmt.Sprintf("%#v", value), exactAccount) {
			t.Fatalf("private value formatted exact account: %T", value)
		}
	}

	requestID := strings.Repeat("9", 64)
	frame, err := EncodeResolveAccountIngressNativeRequestFrameV1(requestID, arguments)
	if err != nil {
		t.Fatalf("encode native account-ingress frame: %v", err)
	}
	if len(frame) == 0 || len(frame) > AccountIngressResolutionMaximumNativeFrameBytesV1 ||
		frame[len(frame)-1] != '\n' {
		t.Fatalf("invalid native frame size=%d", len(frame))
	}
	var wire resolveAccountIngressNativeRequestFrameWireV1
	decoder := json.NewDecoder(bytes.NewReader(frame[:len(frame)-1]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil || wire.RequestID != requestID ||
		wire.Command != OperationFundsResolveAccountIngress || wire.CaseID != arguments.caseID {
		t.Fatalf("native frame mismatch: wire=%#v err=%v", wire, err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(wire.Payload, &payload); err != nil ||
		len(payload) != len(resolveAccountIngressRequiredFieldsV1) {
		t.Fatalf("native payload mismatch: keys=%v err=%v", payload, err)
	}
	for _, forbidden := range []string{"dbPath", "db_path", "sql", "query", "authority"} {
		if _, found := payload[forbidden]; found {
			t.Fatalf("native payload exposed forbidden field %q", forbidden)
		}
	}
	if !bytes.Contains(wire.Payload, []byte(exactAccount)) {
		t.Fatal("private process frame omitted the exact bound candidate")
	}
}

func TestResolveAccountIngressResultRequiresCoverageOrderProvenanceAndSafeSemantics(t *testing.T) {
	const exactAccount = "6222021234567890123"
	arguments := nativeComponentTestAccountIngressArguments(
		t,
		nativeComponentTestContext(
			t,
			"case-account-ingress-result",
			time.Date(2026, 7, 28, 1, 0, 0, 0, time.UTC),
		),
		[]string{exactAccount, "6217009876543210987"},
	)
	result := nativeComponentTestAccountIngressResult(arguments)
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseResolveAccountIngressResultV1(body, arguments)
	if err != nil || len(parsed.Resolutions) != 2 ||
		parsed.Resolutions[0].Disposition != AccountIngressResolutionDispositionResolvedV1 ||
		parsed.Resolutions[1].Disposition != AccountIngressResolutionDispositionNotFoundV1 {
		t.Fatalf("parse valid result: result=%#v err=%v", parsed, err)
	}
	if bytes.Contains(body, []byte(exactAccount)) {
		t.Fatal("safe result reflected the complete candidate")
	}
	mixed := result
	mixed.Resolutions = append([]AccountIngressResolutionV1(nil), result.Resolutions...)
	mixed.Resolutions[1] = AccountIngressResolutionV1{
		Ordinal: mixed.Resolutions[1].Ordinal, Disposition: AccountIngressResolutionDispositionResolvedV1,
		EntityType:      domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1,
		BankInstitution: "中国银行", AccountType: "借记卡",
	}
	if ValidateResolveAccountIngressResultV1(mixed, arguments) != nil {
		t.Fatal("closed account/card result rejected a supported card resolution")
	}

	mutations := []func(*ResolveAccountIngressResultV1){
		func(value *ResolveAccountIngressResultV1) { value.Resolutions = value.Resolutions[:1] },
		func(value *ResolveAccountIngressResultV1) { value.Resolutions[0].Ordinal++ },
		func(value *ResolveAccountIngressResultV1) {
			value.Resolutions[0].BankInstitution = "银行" + exactAccount
		},
		func(value *ResolveAccountIngressResultV1) {
			value.Provenance.DatasetSnapshotID = "dsv2_" + strings.Repeat("0", 64)
		},
		func(value *ResolveAccountIngressResultV1) {
			value.Provenance.QuerySQLHash = strings.Repeat("0", 64)
		},
		func(value *ResolveAccountIngressResultV1) { value.Resolutions[0].EntityType = "person" },
	}
	for index, mutate := range mutations {
		candidate := result
		candidate.Resolutions = append([]AccountIngressResolutionV1(nil), result.Resolutions...)
		mutate(&candidate)
		body, err := json.Marshal(candidate)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseResolveAccountIngressResultV1(body, arguments); err == nil {
			t.Fatalf("mutation %d survived", index)
		}
	}

	ambiguous := result
	ambiguous.Resolutions = append([]AccountIngressResolutionV1(nil), result.Resolutions...)
	ambiguous.Resolutions[0] = AccountIngressResolutionV1{
		Ordinal:     ambiguous.Resolutions[0].Ordinal,
		Disposition: AccountIngressResolutionDispositionAmbiguousV1,
	}
	if ValidateResolveAccountIngressResultV1(ambiguous, arguments) != nil ||
		!ResolveAccountIngressResultHasFailClosedDispositionV1(ambiguous) {
		t.Fatal("closed ambiguous disposition was not represented for host fail-closed handling")
	}
}

func TestResolveAccountIngressResultAuthorityBindsCompleteDescriptorProvenance(t *testing.T) {
	securityContext := nativeComponentTestContext(
		t,
		"case-account-ingress-authority",
		time.Date(2026, 7, 28, 1, 0, 0, 0, time.UTC),
	)
	arguments := nativeComponentTestAccountIngressArguments(
		t,
		securityContext,
		[]string{"6222021234567890123"},
	)
	result := nativeComponentTestAccountIngressResult(arguments)
	descriptorInput := domainfundsquerysource.DescriptorInputV1{
		SnapshotRecordDigest:     strings.Repeat("1", 64),
		DatasetSnapshotID:        securityContext.DatasetSnapshotID,
		SourceManifestHash:       securityContext.SourceManifestHash,
		CaseID:                   securityContext.CaseID,
		CaseBindingHash:          securityContext.CaseBindingHash,
		DatasetBindingDigest:     strings.Repeat("2", 64),
		BindingObservationDigest: securityContext.PublicationPolicy.BindingObservationDigest,

		FundsProducerContentID:                 arguments.expectedProducerContentID,
		FundsProducerContentManifestSHA256:     arguments.expectedProducerManifestSHA256,
		FundsProducerContentManifestByteLength: 2048,

		DuckDBSHA256:                 strings.Repeat("3", 64),
		DuckDBByteLength:             8192,
		DuckDBContentSnapshotDigest:  arguments.expectedDuckDBContentSnapshotDigest,
		DuckDBSnapshotManifestSHA256: arguments.expectedDuckDBSnapshotManifestSHA256,
		MaterializationIdentity:      arguments.expectedMaterializationIdentity,
		SchemaDigest:                 domainfundsquerysource.FixedFundsAnalyticalSchemaDigestV1(),
		DatasetUTCOffsetMinutes:      0,
		ExpectedCurrency:             "CNY",
		MinorUnitScale:               domainfundsquerysource.AccountFlowMinorUnitScaleV1,
		QueryProfileDigest:           domainfundsquerysource.FixedFundsQueryProfileDigestV1(),
	}
	descriptor, err := domainfundsquerysource.NewDescriptorV1(descriptorInput)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateResolveAccountIngressResultAuthorityV1(
		result,
		securityContext,
		descriptor,
	); err != nil {
		t.Fatalf("valid native provenance did not bind the source descriptor: %v", err)
	}

	mutations := map[string]func(*ResolveAccountIngressResultV1){
		"context digest": func(value *ResolveAccountIngressResultV1) {
			value.Provenance.ContextDigest = strings.Repeat("0", 64)
		},
		"producer": func(value *ResolveAccountIngressResultV1) {
			value.Provenance.ProducerContentID = "fpc1_" + strings.Repeat("0", 64)
		},
		"duckdb snapshot": func(value *ResolveAccountIngressResultV1) {
			value.Provenance.DuckDBContentSnapshotDigest = strings.Repeat("0", 64)
		},
		"materialization": func(value *ResolveAccountIngressResultV1) {
			value.Provenance.MaterializationIdentity =
				domainsecurity.FundsMaterializationIdentityPrefixV1 + strings.Repeat("0", 64)
		},
		"source signature": func(value *ResolveAccountIngressResultV1) {
			value.Provenance.SourceSignature = strings.Repeat("0", 64)
		},
		"result signature": func(value *ResolveAccountIngressResultV1) {
			value.Provenance.ResultSignature = strings.Repeat("A", 64)
		},
		"query contract": func(value *ResolveAccountIngressResultV1) {
			value.Provenance.QueryContract = "analytix.account-ingress-resolution-query/v0"
		},
		"query hash": func(value *ResolveAccountIngressResultV1) {
			value.Provenance.QuerySQLHash = strings.Repeat("0", 64)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			candidate := result
			candidate.Resolutions = append(
				[]AccountIngressResolutionV1(nil),
				result.Resolutions...,
			)
			mutate(&candidate)
			if ValidateResolveAccountIngressResultAuthorityV1(
				candidate,
				securityContext,
				descriptor,
			) == nil {
				t.Fatal("mutated provenance remained bound")
			}
		})
	}

	legacyInput := descriptorInput
	legacyInput.QueryProfileDigest = domainfundsquerysource.FixedAccountFlowQueryProfileDigestV1()
	legacy, err := domainfundsquerysource.NewDescriptorV1(legacyInput)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateResolveAccountIngressResultAuthorityV1(
		result,
		securityContext,
		legacy,
	) == nil {
		t.Fatal("account-flow-only profile authorized private ingress")
	}
}

func nativeComponentTestAccountIngressArguments(
	t *testing.T,
	securityContextTestContext domainsecurity.TurnSecurityContext,
	candidateValues []string,
) ResolveAccountIngressArgumentsV1 {
	t.Helper()
	candidates := make([]ResolveAccountIngressCandidateInputV1, 0, len(candidateValues))
	for index, value := range candidateValues {
		candidate, err := NewResolveAccountIngressCandidateInputV1(uint32(index*3+1), value)
		if err != nil {
			t.Fatal(err)
		}
		candidates = append(candidates, candidate)
	}
	arguments, err := NewResolveAccountIngressArgumentsV1(ResolveAccountIngressArgumentsInputV1{
		CaseID:                               securityContextTestContext.CaseID,
		DatasetSnapshotID:                    securityContextTestContext.DatasetSnapshotID,
		ContextEpoch:                         securityContextTestContext.ContextEpoch,
		ContextDigest:                        securityContextTestContext.ContextDigest,
		CaseBindingHash:                      securityContextTestContext.CaseBindingHash,
		ExpectedProducerContentID:            "fpc1_" + strings.Repeat("4", 64),
		ExpectedProducerManifestSHA256:       strings.Repeat("5", 64),
		ExpectedDuckDBContentSnapshotDigest:  strings.Repeat("b", 64),
		ExpectedDuckDBSnapshotManifestSHA256: strings.Repeat("c", 64),
		ExpectedMaterializationIdentity:      "txn_daily_snapshot:v12:" + strings.Repeat("a", 64),
		Candidates:                           candidates,
	})
	if err != nil {
		t.Fatal(err)
	}
	return arguments
}

func nativeComponentTestAccountIngressResult(
	arguments ResolveAccountIngressArgumentsV1,
) ResolveAccountIngressResultV1 {
	resolutions := make([]AccountIngressResolutionV1, len(arguments.candidates))
	for index, candidate := range arguments.candidates {
		resolutions[index] = AccountIngressResolutionV1{
			Ordinal:     candidate.Ordinal,
			Disposition: AccountIngressResolutionDispositionNotFoundV1,
		}
	}
	resolutions[0] = AccountIngressResolutionV1{
		Ordinal:         resolutions[0].Ordinal,
		Disposition:     AccountIngressResolutionDispositionResolvedV1,
		EntityType:      "bank_account_number",
		BankInstitution: "中国银行",
		AccountType:     "储蓄账户",
	}
	return ResolveAccountIngressResultV1{
		SchemaVersion: 1,
		Contract:      AccountIngressResolutionResultContractV1,
		Resolutions:   resolutions,
		Provenance: AccountIngressResolutionProvenanceV1{
			DatasetSnapshotID:              arguments.datasetSnapshotID,
			ContextEpoch:                   arguments.contextEpoch,
			ContextDigest:                  arguments.contextDigest,
			CaseBindingHash:                arguments.caseBindingHash,
			ExpectedProducerContentID:      arguments.expectedProducerContentID,
			ExpectedProducerManifestSHA256: arguments.expectedProducerManifestSHA256,
			DuckDBContentSnapshotDigest:    arguments.expectedDuckDBContentSnapshotDigest,
			DuckDBSnapshotManifestSHA256:   arguments.expectedDuckDBSnapshotManifestSHA256,
			MaterializationIdentity:        arguments.expectedMaterializationIdentity,
			SourceSignature: strings.TrimPrefix(
				arguments.expectedMaterializationIdentity,
				accountFlowMaterializationIdentityPrefixV1,
			),
			ResultSignature:        strings.Repeat("9", 64),
			ProducerContentID:      arguments.expectedProducerContentID,
			ProducerManifestSHA256: arguments.expectedProducerManifestSHA256,
			QueryContract:          AccountIngressResolutionQueryContractV1,
			QuerySQLHash:           AccountIngressResolutionQuerySQLHashV1,
		},
	}
}
