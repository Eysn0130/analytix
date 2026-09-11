package fundsquerysource

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
)

const (
	accountFlowResolverSourceFileIDV1  = "0123456789abcdefabcd"
	accountFlowCountCanaryPolicyIDV1   = "funds.go-strict-csv-count-source-row-ledger/v1"
	accountFlowCountCanarySourceTypeV1 = "transaction_dataset_inventory_source_rows"
)

func TestAccountFlowLedgerLocatorMapsCanonicalPrivateImportIdentity(t *testing.T) {
	canonical, ok := domainevidence.ResolveSourceRowProducerPolicyV1(
		domainevidence.FundsCanonicalTransactionSourceRowPolicyIDV1,
	)
	if !ok {
		t.Fatal("canonical transaction source-row policy is unavailable")
	}
	input := domainevidence.SourceRowLocatorInputV1{
		SourceFileID: accountFlowResolverSourceFileIDV1, SourceRowNumber: 41,
	}
	got, err := accountFlowLedgerLocatorInputV1(canonical, "case-account-flow", input)
	want, wantErr := domainevidence.DeriveFundsCanonicalCSVSourceFileIDV1(
		"case-account-flow", accountFlowResolverSourceFileIDV1,
	)
	if err != nil || wantErr != nil || got.SourceFileID != want || got.SourceFileID == input.SourceFileID ||
		got.SourceRowNumber != input.SourceRowNumber {
		t.Fatalf("canonical source locator mapping = err:%v wantErr:%v mapped:%t row:%d",
			err != nil, wantErr != nil, got.SourceFileID == want, got.SourceRowNumber)
	}

	legacy, ok := domainevidence.ResolveSourceRowProducerPolicyV1(
		domainevidence.FundsTransactionSourceRowPolicyIDV1,
	)
	if !ok {
		t.Fatal("legacy transaction source-row policy is unavailable")
	}
	unchanged, err := accountFlowLedgerLocatorInputV1(legacy, "case-account-flow", input)
	if err != nil || unchanged != input {
		t.Fatalf("legacy source locator changed = err:%v equal:%t", err != nil, unchanged == input)
	}

	invalid := input
	invalid.SourceFileID = "not-a-private-import-id"
	if _, err := accountFlowLedgerLocatorInputV1(canonical, "case-account-flow", invalid); !errors.Is(
		err, fundsquerysourceport.ErrMismatch,
	) {
		t.Fatalf("invalid canonical private identity error class = mismatch:%t", errors.Is(err, fundsquerysourceport.ErrMismatch))
	}
}

func TestControlledAccountFlowParsedRowMustMatchEveryClaimedTransactionField(t *testing.T) {
	sourceRecordID := "srow1_" + domainsecurity.SHA256Hex([]byte("controlled-source-row"))
	account := "6222021234567890123"
	fields := []domainevidence.ParsedTypedFieldV1{
		{Name: "norm.clean_acct_no", Scalar: domainevidence.ParsedTypedScalarV1{Kind: "text", Value: account}},
		{Name: "norm.clean_amount", Scalar: domainevidence.ParsedTypedScalarV1{Kind: "decimal", Value: "-32800.5"}},
		{Name: "norm.clean_card_no", Scalar: domainevidence.ParsedTypedScalarV1{Kind: "null", Value: ""}},
		{Name: "norm.clean_dc_flag", Scalar: domainevidence.ParsedTypedScalarV1{Kind: "text", Value: "进"}},
		{Name: "norm.txn_ts", Scalar: domainevidence.ParsedTypedScalarV1{Kind: "text", Value: "2026-01-02 03:04:05"}},
		{Name: "raw.acct_no", Scalar: domainevidence.ParsedTypedScalarV1{Kind: "text", Value: account}},
		{Name: "raw.card_no", Scalar: domainevidence.ParsedTypedScalarV1{Kind: "null", Value: ""}},
		{Name: "raw.currency", Scalar: domainevidence.ParsedTypedScalarV1{Kind: "text", Value: "CNY"}},
	}
	expected := domainnative.AccountFlowProviderSemanticTransactionV1{
		EvidenceRef: sourceRecordID, OccurredAt: "2026-01-01T19:04:05.000000Z",
		Direction: domainnative.AccountFlowDirectionInflowV1, AmountMinor: "3280050",
		Currency: "CNY", MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
	}
	if err := validateControlledAccountFlowParsedFieldsV1(
		sourceRecordID, fields, account, "+08:00", expected,
	); err != nil {
		t.Fatalf("valid controlled source transaction was rejected: %v", err)
	}

	for name, mutate := range map[string]func(*string, *[]domainevidence.ParsedTypedFieldV1, *domainnative.AccountFlowProviderSemanticTransactionV1){
		"forged account": func(_ *string, values *[]domainevidence.ParsedTypedFieldV1, _ *domainnative.AccountFlowProviderSemanticTransactionV1) {
			for index := range *values {
				if (*values)[index].Name == "norm.clean_acct_no" || (*values)[index].Name == "raw.acct_no" {
					(*values)[index].Scalar.Value = "6222021234567890124"
				}
			}
		},
		"forged amount": func(_ *string, _ *[]domainevidence.ParsedTypedFieldV1, row *domainnative.AccountFlowProviderSemanticTransactionV1) {
			row.AmountMinor = "3280051"
		},
		"forged currency": func(_ *string, _ *[]domainevidence.ParsedTypedFieldV1, row *domainnative.AccountFlowProviderSemanticTransactionV1) {
			row.Currency = "USD"
		},
		"forged direction": func(_ *string, _ *[]domainevidence.ParsedTypedFieldV1, row *domainnative.AccountFlowProviderSemanticTransactionV1) {
			row.Direction = domainnative.AccountFlowDirectionOutflowV1
		},
		"forged time": func(_ *string, _ *[]domainevidence.ParsedTypedFieldV1, row *domainnative.AccountFlowProviderSemanticTransactionV1) {
			row.OccurredAt = "2026-01-01T19:04:06.000000Z"
		},
		"wrong source row": func(record *string, _ *[]domainevidence.ParsedTypedFieldV1, _ *domainnative.AccountFlowProviderSemanticTransactionV1) {
			*record = "srow1_" + domainsecurity.SHA256Hex([]byte("different-source-row"))
		},
		"ambiguous duplicate field": func(_ *string, values *[]domainevidence.ParsedTypedFieldV1, _ *domainnative.AccountFlowProviderSemanticTransactionV1) {
			*values = append(*values, (*values)[0])
		},
	} {
		t.Run(name, func(t *testing.T) {
			record := sourceRecordID
			tamperedFields := append([]domainevidence.ParsedTypedFieldV1(nil), fields...)
			tamperedExpected := expected
			mutate(&record, &tamperedFields, &tamperedExpected)
			if err := validateControlledAccountFlowParsedFieldsV1(
				record, tamperedFields, account, "+08:00", tamperedExpected,
			); !errors.Is(err, datasetsnapshotport.ErrMismatch) {
				t.Fatalf("tampered controlled transaction was not rejected: %v", err)
			}
		})
	}
	if err := validateControlledAccountFlowParsedFieldsV1(
		sourceRecordID, fields, account, "+15:00", expected,
	); !errors.Is(err, datasetsnapshotport.ErrMismatch) {
		t.Fatalf("invalid source timezone was not rejected: %v", err)
	}

	cardFields := append([]domainevidence.ParsedTypedFieldV1(nil), fields...)
	for index := range cardFields {
		switch cardFields[index].Name {
		case "norm.clean_acct_no", "raw.acct_no":
			cardFields[index].Scalar = domainevidence.ParsedTypedScalarV1{Kind: "null", Value: ""}
		case "norm.clean_card_no", "raw.card_no":
			cardFields[index].Scalar = domainevidence.ParsedTypedScalarV1{Kind: "text", Value: account}
		}
	}
	if err := validateControlledAccountFlowParsedFieldsV1(
		sourceRecordID, cardFields, account, "+08:00", expected,
	); err != nil {
		t.Fatalf("valid card-only controlled source transaction was rejected: %v", err)
	}

	formattedRawFields := append([]domainevidence.ParsedTypedFieldV1(nil), fields...)
	for index := range formattedRawFields {
		if formattedRawFields[index].Name == "raw.acct_no" {
			formattedRawFields[index].Scalar.Value = "6222 0212-3456 7890-123"
		}
	}
	if err := validateControlledAccountFlowParsedFieldsV1(
		sourceRecordID, formattedRawFields, account, "+08:00", expected,
	); err != nil {
		t.Fatalf("canonically equivalent raw account formatting was rejected: %v", err)
	}

	rawFallbackFields := append([]domainevidence.ParsedTypedFieldV1(nil), fields...)
	for index := range rawFallbackFields {
		if rawFallbackFields[index].Name == "norm.clean_acct_no" {
			rawFallbackFields[index].Scalar = domainevidence.ParsedTypedScalarV1{Kind: "null", Value: ""}
		}
	}
	if err := validateControlledAccountFlowParsedFieldsV1(
		sourceRecordID, rawFallbackFields, account, "+08:00", expected,
	); err != nil {
		t.Fatalf("raw account fallback used by the materializer was rejected: %v", err)
	}

	conflictingAccountFields := append([]domainevidence.ParsedTypedFieldV1(nil), fields...)
	for index := range conflictingAccountFields {
		if conflictingAccountFields[index].Name == "raw.acct_no" {
			conflictingAccountFields[index].Scalar.Value = "6222021234567890124"
		}
	}
	if err := validateControlledAccountFlowParsedFieldsV1(
		sourceRecordID, conflictingAccountFields, account, "+08:00", expected,
	); !errors.Is(err, datasetsnapshotport.ErrMismatch) {
		t.Fatalf("conflicting clean and raw account identities were not rejected: %v", err)
	}
}

func TestAcceptedSlotSourceResolverReadsOnlyOriginalRetainedField(t *testing.T) {
	const (
		originalExact = "6222 0212-3456 7890"
		canonical     = "6222021234567890"
	)
	for _, field := range []string{
		domainevidence.AcceptedSlotSourceFieldAccountV1,
		domainevidence.AcceptedSlotSourceFieldCardV1,
	} {
		t.Run(field, func(t *testing.T) {
			fixture := newAcceptedSlotSourceResolverFixtureForFieldV1(t, field, originalExact, canonical)
			uses := 0
			err := fixture.resolver.useExactAcceptedSlotEntityV1(
				[]domainevidence.AcceptedSlotSourceBindingV1{fixture.binding}, canonical,
				func(value string) error {
					uses++
					if value != originalExact || value == canonical {
						t.Fatalf("retained source projection = %q", value)
					}
					return nil
				},
			)
			if err != nil || uses != 1 {
				t.Fatalf("retained source exact use = calls=%d err=%v", uses, err)
			}
			if !fixture.materials.allIssuedBytesCleared() {
				t.Fatal("retained resolver kept exact parsed or lineage material bytes")
			}
		})
	}

	for name, mutate := range map[string]func(*acceptedSlotSourceResolverFixtureV1){
		"current canonical sentinel": func(candidate *acceptedSlotSourceResolverFixtureV1) {
			candidate.expectedCanonical = "9999000011112222"
		},
		"wrong source file": func(candidate *acceptedSlotSourceResolverFixtureV1) {
			input := acceptedSlotSourceBindingInputV1(candidate.binding)
			input.SourceFileID = "fedcba9876543210abcd"
			candidate.binding = mustAcceptedSlotSourceBindingV1(t, input)
		},
		"wrong row": func(candidate *acceptedSlotSourceResolverFixtureV1) {
			input := acceptedSlotSourceBindingInputV1(candidate.binding)
			input.SourceRowNumber++
			candidate.binding = mustAcceptedSlotSourceBindingV1(t, input)
		},
		"wrong record": func(candidate *acceptedSlotSourceResolverFixtureV1) {
			input := acceptedSlotSourceBindingInputV1(candidate.binding)
			input.SourceRecordID = domainevidence.SourceRowRecordIDPrefixV1 + digestV1("wrong-accepted-slot-record")
			candidate.binding = mustAcceptedSlotSourceBindingV1(t, input)
		},
		"missing lineage": func(candidate *acceptedSlotSourceResolverFixtureV1) {
			candidate.materials.values[datasetsnapshotport.MaterialSourceRowLineageV1] = map[string][]byte{}
		},
		"corrupt parsed page": func(candidate *acceptedSlotSourceResolverFixtureV1) {
			candidate.materials.tamperSecondKind = datasetsnapshotport.MaterialParsedPageV1
		},
		"account binding cannot read card row": func(candidate *acceptedSlotSourceResolverFixtureV1) {
			card := newAcceptedSlotSourceResolverFixtureForFieldV1(t, domainevidence.AcceptedSlotSourceFieldCardV1, originalExact, canonical)
			candidate.resolver = card.resolver
			candidate.materials = card.materials
			input := acceptedSlotSourceBindingInputV1(card.binding)
			input.Field = domainevidence.AcceptedSlotSourceFieldAccountV1
			candidate.binding = mustAcceptedSlotSourceBindingV1(t, input)
		},
		"card binding cannot read account row": func(candidate *acceptedSlotSourceResolverFixtureV1) {
			input := acceptedSlotSourceBindingInputV1(candidate.binding)
			input.Field = domainevidence.AcceptedSlotSourceFieldCardV1
			candidate.binding = mustAcceptedSlotSourceBindingV1(t, input)
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := newAcceptedSlotSourceResolverFixtureV1(t, originalExact, canonical)
			candidate.expectedCanonical = canonical
			mutate(candidate)
			called := false
			err := candidate.resolver.useExactAcceptedSlotEntityV1(
				[]domainevidence.AcceptedSlotSourceBindingV1{candidate.binding}, candidate.expectedCanonical,
				func(string) error { called = true; return nil },
			)
			if err == nil || called {
				t.Fatalf("invalid retained source authority projected bytes: called=%t err=%v", called, err)
			}
		})
	}
}

func TestAcceptedSlotSourceFieldPairFailsClosedWithoutCrossTypeFallback(t *testing.T) {
	const (
		exact     = "6222 0212-3456 7890"
		canonical = "6222021234567890"
	)
	fields := func(accountNorm, accountRaw, cardNorm, cardRaw domainevidence.ParsedTypedScalarV1) []domainevidence.ParsedTypedFieldV1 {
		return []domainevidence.ParsedTypedFieldV1{
			{Name: "norm.clean_acct_no", Scalar: accountNorm},
			{Name: "raw.acct_no", Scalar: accountRaw},
			{Name: "norm.clean_card_no", Scalar: cardNorm},
			{Name: "raw.card_no", Scalar: cardRaw},
		}
	}
	text := func(value string) domainevidence.ParsedTypedScalarV1 {
		return domainevidence.ParsedTypedScalarV1{Kind: "text", Value: value}
	}
	null := domainevidence.ParsedTypedScalarV1{Kind: "null"}
	for _, test := range []struct {
		name  string
		field string
		rows  []domainevidence.ParsedTypedFieldV1
	}{
		{
			name: "account binding cannot borrow card raw", field: domainevidence.AcceptedSlotSourceFieldAccountV1,
			rows: fields(null, null, text(canonical), text(exact)),
		},
		{
			name: "card binding cannot borrow account raw", field: domainevidence.AcceptedSlotSourceFieldCardV1,
			rows: fields(text(canonical), text(exact), null, null),
		},
		{
			name: "declared normalized and raw conflict", field: domainevidence.AcceptedSlotSourceFieldAccountV1,
			rows: fields(text("6222021234567899"), text(exact), null, null),
		},
		{
			name: "declared raw is missing", field: domainevidence.AcceptedSlotSourceFieldCardV1,
			rows: fields(null, null, text(canonical), null),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, err := exactControlledFinancialEntitySourceFieldV1(test.rows, test.field, canonical)
			if err == nil || value != "" {
				t.Fatalf("invalid declared source pair projected %q err=%v", value, err)
			}
		})
	}
}

type acceptedSlotSourceResolverFixtureV1 struct {
	resolver          *accountFlowSourceRowResolverV1
	materials         *fakeAdmissionMaterialReaderV2
	binding           domainevidence.AcceptedSlotSourceBindingV1
	expectedCanonical string
	snapshot          datasetsnapshotport.ResolvedSnapshotV2
	securityContext   domainsecurity.TurnSecurityContext
}

func newAcceptedSlotSourceResolverFixtureV1(
	t *testing.T,
	originalExact string,
	canonical string,
) *acceptedSlotSourceResolverFixtureV1 {
	return newAcceptedSlotSourceResolverFixtureForFieldV1(
		t, domainevidence.AcceptedSlotSourceFieldAccountV1, originalExact, canonical,
	)
}

func newAcceptedSlotSourceResolverFixtureForFieldV1(
	t *testing.T,
	sourceField string,
	originalExact string,
	canonical string,
) *acceptedSlotSourceResolverFixtureV1 {
	t.Helper()
	base := newServiceFixtureV1(t, snapshotKindAnalyticalV1, "accepted-slot-retained-source")
	source := []byte("account,amount\r\n" + originalExact + ",1.00\r\n")
	values := map[datasetsnapshotport.MaterialKindV2]map[string][]byte{}
	var manifestBody []byte
	var producerBody []byte
	var sourceFileID string
	_, err := domainevidence.ComposeFundsCanonicalCSVAdmissionV1(
		context.Background(),
		domainevidence.FundsCanonicalCSVAdmissionInputV1{
			Binding:                  base.selection.Snapshot.Record.Binding,
			AcquiredAt:               time.Date(2026, 8, 21, 1, 2, 3, 0, time.UTC),
			AcquisitionActorDigest:   digestV1("accepted-slot-acquisition-actor"),
			IntentNonceDigest:        digestV1("accepted-slot-intent"),
			SourceArtifactSHA256:     domainsecurity.SHA256Hex(source),
			SourceArtifactByteLength: uint64(len(source)), SourceRowCount: 1,
		},
		bytes.NewReader(source),
		func(
			_ context.Context,
			build domainevidence.FundsCanonicalCSVNativeBuildContextV1,
		) (domainevidence.FundsCanonicalCSVNativeBuildResultV1, error) {
			var deriveErr error
			sourceFileID, deriveErr = domainevidence.DeriveFundsCanonicalCSVSourceFileIDV1(
				build.Binding.CaseID, build.PrivateImportFileID,
			)
			if deriveErr != nil {
				return domainevidence.FundsCanonicalCSVNativeBuildResultV1{}, deriveErr
			}
			accountNorm := domainevidence.ParsedTypedScalarV1{Kind: "null"}
			accountRaw := domainevidence.ParsedTypedScalarV1{Kind: "null"}
			cardNorm := domainevidence.ParsedTypedScalarV1{Kind: "null"}
			cardRaw := domainevidence.ParsedTypedScalarV1{Kind: "null"}
			if sourceField == domainevidence.AcceptedSlotSourceFieldAccountV1 {
				accountNorm = domainevidence.ParsedTypedScalarV1{Kind: "text", Value: canonical}
				accountRaw = domainevidence.ParsedTypedScalarV1{Kind: "text", Value: originalExact}
			} else if sourceField == domainevidence.AcceptedSlotSourceFieldCardV1 {
				cardNorm = domainevidence.ParsedTypedScalarV1{Kind: "text", Value: canonical}
				cardRaw = domainevidence.ParsedTypedScalarV1{Kind: "text", Value: originalExact}
			} else {
				t.Fatalf("unsupported accepted-slot fixture field %q", sourceField)
			}
			fields := []domainevidence.ParsedTypedFieldV1{
				{Name: "norm.clean_acct_no", Scalar: accountNorm},
				{Name: "norm.clean_card_no", Scalar: cardNorm},
				{Name: "raw.acct_no", Scalar: accountRaw},
				{Name: "raw.card_no", Scalar: cardRaw},
			}
			row, rowErr := domainevidence.NewFundsCanonicalCSVHostRowV1(
				sourceFileID, 1, build.SourceArtifactSHA256,
				acceptedSlotParsedRowDigestV1(fields),
				[]domainevidence.FundsCanonicalCSVHostTypedFieldV1{
					{Name: "norm.clean_acct_no", Kind: accountNorm.Kind, Value: accountNorm.Value},
					{Name: "norm.clean_card_no", Kind: cardNorm.Kind, Value: cardNorm.Value},
					{Name: "raw.acct_no", Kind: accountRaw.Kind, Value: accountRaw.Value},
					{Name: "raw.card_no", Kind: cardRaw.Kind, Value: cardRaw.Value},
				},
			)
			if rowErr != nil {
				return domainevidence.FundsCanonicalCSVNativeBuildResultV1{}, rowErr
			}
			return domainevidence.NewFundsCanonicalCSVNativeBuildResultV1(
				acceptedSlotMaterializationV1(t, build), digestV1("accepted-slot-duckdb"), 4096,
				[]domainevidence.FundsCanonicalCSVHostRowV1{row},
			)
		},
		func(material domainevidence.FundsCanonicalCSVAdmissionMaterialV1) error {
			return material.UseExactV1(func(
				kind domainevidence.FundsCanonicalCSVAdmissionMaterialKindV1,
				address string,
				body []byte,
			) error {
				materialKind := datasetsnapshotport.MaterialKindV2(kind)
				if values[materialKind] == nil {
					values[materialKind] = map[string][]byte{}
				}
				values[materialKind][address] = append([]byte(nil), body...)
				if materialKind == datasetsnapshotport.MaterialSnapshotManifestV2 {
					manifestBody = append([]byte(nil), body...)
				} else if materialKind == datasetsnapshotport.MaterialFundsProducerContentV1 {
					producerBody = append([]byte(nil), body...)
				}
				return nil
			})
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := domainsecurity.ParseDatasetSnapshotManifestV2(manifestBody)
	if err != nil {
		t.Fatal(err)
	}
	producer, err := domainsecurity.ParseFundsProducerContentManifestV1(producerBody)
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x63}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	snapshot := datasetsnapshotport.ResolvedSnapshotV2{
		Manifest: manifest, FundsProducerContent: producer,
	}
	snapshot.Record = issueSnapshotRecordV1(
		t, manifest, producer, domainsecurity.FundsProducerContentManifestV2{}, privateKey, publicKey,
	)
	historicalContext := securityContextV1(t, base.observation, snapshot, nil)
	var record domainevidence.SourceRowRecordV1
	for _, body := range values[datasetsnapshotport.MaterialSourceRowPageV1] {
		page, parseErr := domainevidence.ParseSourceRowLedgerPageV1(body)
		err = parseErr
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Entries) != 1 {
			t.Fatal("accepted source fixture did not contain exactly one ledger record")
		}
		record = page.Entries[0].Record
	}
	binding := mustAcceptedSlotSourceBindingV1(t, domainevidence.AcceptedSlotSourceBindingInputV1{
		FactIDs:         []string{"accepted-slot-fact"},
		EntityReference: "cer1_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SourceRecordID:  record.SourceRecordID, SourceFileID: sourceFileID,
		SourceRowNumber: record.Locator.SourceRowNumber,
		Field:           sourceField,
	})
	reader := &fakeAdmissionMaterialReaderV2{
		values: values, reads: map[datasetsnapshotport.MaterialKindV2]int{},
		issued: map[datasetsnapshotport.MaterialKindV2][][]byte{},
	}
	fixture := &acceptedSlotSourceResolverFixtureV1{
		resolver: newAccountFlowSourceRowResolverV1(
			context.Background(), manifest, historicalContext, reader,
		),
		materials: reader, binding: binding, expectedCanonical: canonical,
		snapshot: snapshot, securityContext: historicalContext,
	}
	t.Cleanup(fixture.resolver.close)
	return fixture
}

func acceptedSlotSourceBindingInputV1(
	binding domainevidence.AcceptedSlotSourceBindingV1,
) domainevidence.AcceptedSlotSourceBindingInputV1 {
	return domainevidence.AcceptedSlotSourceBindingInputV1{
		FactIDs: append([]string(nil), binding.FactIDs...), EntityReference: binding.EntityReference,
		SourceRecordID: binding.SourceRecordID, SourceFileID: binding.SourceFileID,
		SourceRowNumber: binding.SourceRowNumber, Field: binding.Field,
	}
}

func mustAcceptedSlotSourceBindingV1(
	t *testing.T,
	input domainevidence.AcceptedSlotSourceBindingInputV1,
) domainevidence.AcceptedSlotSourceBindingV1 {
	t.Helper()
	binding, err := domainevidence.NewAcceptedSlotSourceBindingV1(input)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func acceptedSlotParsedRowDigestV1(fields []domainevidence.ParsedTypedFieldV1) string {
	body, _ := json.Marshal(fields)
	return domainsecurity.SHA256Hex(append([]byte("analytix.parsed-canonical-row/digest/v1\x00"), body...))
}

func acceptedSlotMaterializationV1(
	t *testing.T,
	build domainevidence.FundsCanonicalCSVNativeBuildContextV1,
) domainsecurity.FundsMaterializationResultV1 {
	t.Helper()
	producer, err := domainsecurity.NewFundsProducerContentManifestV1(
		domainsecurity.FundsProducerContentManifestInputV1{
			CaseID: build.Binding.CaseID, SourceRevision: 1,
			RawManifestSHA256:       build.RawArtifactManifestSHA256,
			NormalizedContentSHA256: digestV1("accepted-slot-normalized"),
			DetailContentSHA256:     digestV1("accepted-slot-detail"),
			AggregateContentSHA256:  digestV1("accepted-slot-aggregate"),
			KeywordContentSHA256:    digestV1("accepted-slot-keyword"),
			AccountContentSHA256:    digestV1("accepted-slot-account"),
			NormalizedRowCount:      1, AcceptedRowCount: 1, DetailRowCount: 1,
			AggregateRowCount: 1, KeywordRowCount: 1, AccountRowCount: 1,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	producerBody, err := domainsecurity.FundsProducerContentManifestV1Bytes(producer)
	if err != nil {
		t.Fatal(err)
	}
	rawSources := domainsecurity.FundsRawSourceManifestV1{{
		CleanedStatus: "done", FileID: build.PrivateImportFileID, RowsImportedNorm: 1,
		SHA256: build.SourceArtifactSHA256, Status: "已完成",
	}}
	rawBody, err := json.Marshal(rawSources)
	if err != nil {
		t.Fatal(err)
	}
	value := domainsecurity.FundsMaterializationResultV1{
		CaseID: build.Binding.CaseID, RowCount: 1,
		AggregateName:                        domainsecurity.FundsMaterializationIdentityPrefixV1 + strings.Repeat("0", 64),
		AggregateVersion:                     domainsecurity.FundsMaterializationAggregateVersionV1,
		MaterializationIdentity:              domainsecurity.FundsMaterializationIdentityPrefixV1 + strings.Repeat("0", 64),
		MaterializationIdentitySchemaVersion: domainsecurity.FundsMaterializationIdentitySchemaVersionV1,
		ProducerContentID:                    domainsecurity.DeriveFundsProducerContentIDV1(producer),
		ProducerContentManifestBase64:        base64.StdEncoding.EncodeToString(producerBody),
		ProducerContentManifestSHA256:        domainsecurity.SHA256Hex(producerBody),
		ProducerContentManifestByteLength:    uint64(len(producerBody)),
		RawArtifactManifestSHA256:            build.RawArtifactManifestSHA256,
		RawSourceManifestBase64:              base64.StdEncoding.EncodeToString(rawBody),
		RawSourceManifestSHA256:              domainsecurity.SHA256Hex(rawBody),
		RawSourceManifestByteLength:          uint64(len(rawBody)),
		DuckDBContentSnapshotDigest:          digestV1("accepted-slot-duckdb-content"),
		DuckDBSnapshotManifestSHA256:         digestV1("accepted-slot-duckdb-manifest"),
		SchemaDigest:                         domainsecurity.FixedFundsAnalyticalSchemaDigestV1(),
	}
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := domainsecurity.ParseFundsMaterializationResultV1(body)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestControlledAccountFlowExpectedRowsRejectDuplicateSourceIdentity(t *testing.T) {
	row := domainnative.AccountFlowProviderSemanticTransactionV1{
		EvidenceRef: "srow1_" + domainsecurity.SHA256Hex([]byte("duplicate-controlled-source-row")),
		OccurredAt:  "2026-01-01T00:00:00.000000Z", Direction: domainnative.AccountFlowDirectionInflowV1,
		AmountMinor: "1", Currency: "CNY", MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
	}
	if _, err := controlledAccountFlowExpectedRowsV1([]domainnative.AccountFlowProviderSemanticTransactionV1{row, row}); !errors.Is(err, datasetsnapshotport.ErrMismatch) {
		t.Fatalf("duplicate controlled source row was not rejected: %v", err)
	}
}

func TestControlledAccountFlowRequiresOnePrivateLocatorPerEvidenceRow(t *testing.T) {
	row := domainnative.AccountFlowProviderSemanticTransactionV1{
		EvidenceRef: "srow1_" + domainsecurity.SHA256Hex([]byte("controlled-locator-row")),
		OccurredAt:  "2026-01-01T00:00:00.000000Z",
		Direction:   domainnative.AccountFlowDirectionInflowV1,
		AmountMinor: "1", Currency: "CNY",
		MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
	}
	consumeCalls := 0
	err := (&accountFlowSourceRowResolverV1{}).useExactControlledAccountV1(
		"6222021234567890123",
		"Z",
		[]domainnative.AccountFlowProviderSemanticTransactionV1{row},
		nil,
		func(string) error {
			consumeCalls++
			return nil
		},
	)
	if !errors.Is(err, fundsquerysourceport.ErrUnavailable) || consumeCalls != 0 {
		t.Fatalf("controlled account flow accepted evidence without its private locator: calls=%d err=%v", consumeCalls, err)
	}
}

func TestAccountFlowSourceRowResolverUsesExactBoundLedgerRecordOnce(t *testing.T) {
	fixture, rowNumber, wantRecordID := newAccountFlowResolverServiceFixtureV1(t)
	consumeCalls := 0
	err := fixture.service.UseCurrent(
		context.Background(),
		fixture.securityContext,
		func(
			_ context.Context,
			_ domainfundsquerysource.DescriptorV1,
			_ fundsquerysourceport.ExactReadLease,
			resolve domainnative.AccountFlowSourceRowResolverV1,
		) error {
			return resolve(accountFlowResolverSourceFileIDV1, rowNumber, func(sourceRecordID string) error {
				consumeCalls++
				if sourceRecordID != wantRecordID {
					t.Fatalf("source record id = %q, want %q", sourceRecordID, wantRecordID)
				}
				return nil
			})
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if consumeCalls != 1 {
		t.Fatalf("consume calls = %d", consumeCalls)
	}
	for _, kind := range []datasetsnapshotport.MaterialKindV2{
		datasetsnapshotport.MaterialSourceRowLedgerRootV1,
		datasetsnapshotport.MaterialSourceRowIndexPageV1,
		datasetsnapshotport.MaterialSourceRowPageV1,
		datasetsnapshotport.MaterialSourceRowRecordV1,
	} {
		if got := fixture.materials.readCount(kind); got != 2 {
			t.Fatalf("%s reads = %d, want two independent reads", kind, got)
		}
	}
	if !fixture.materials.allIssuedBytesCleared() {
		t.Fatal("resolver retained exact material bytes returned by the private reader")
	}
}

func TestServiceRejectsCountCanaryPolicyBeforeHostSourceWithoutResolverUse(t *testing.T) {
	fixture, _, _ := newAccountFlowResolverServiceFixtureV1(t)
	manifestInput := manifestInputV1(t, fixture.selection.Snapshot)
	manifestInput.SourceType = accountFlowCountCanarySourceTypeV1
	manifestInput.ProducerPolicyID = accountFlowCountCanaryPolicyIDV1
	manifestInput.ProducerPolicyDigest = digestV1("count-canary-policy")
	manifestInput.ProducerComponentID = "runtime-go"
	manifestInput.ProducerComponentVersion = "1.0.0"
	manifestInput.ProducerOperation = "build_count_case_rows_source_row_page_v1"
	manifestInput.ProducerOperationSchemaHash = digestV1("count-canary-operation-schema")
	manifestInput.ParserID = "analytix.strict-utf8-csv"
	manifestInput.ParserVersion = "1"
	manifestInput.AnalyticalDuckDB = fixture.selection.Snapshot.Manifest.AnalyticalDuckDB
	manifest, err := domainsecurity.NewDatasetSnapshotManifestV2(manifestInput)
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x52}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	snapshot := datasetsnapshotport.ResolvedSnapshotV2{
		Manifest:             manifest,
		FundsProducerContent: fixture.selection.Snapshot.FundsProducerContent,
	}
	snapshot.Record = issueSnapshotRecordV1(
		t,
		manifest,
		snapshot.FundsProducerContent,
		domainsecurity.FundsProducerContentManifestV2{},
		privateKey,
		publicKey,
	)
	index, err := domainsecurity.NewDatasetSnapshotIndexV1(
		domainsecurity.DatasetSnapshotIndexInputV1{
			InstallationID:       snapshot.Record.InstallationID,
			EnrollmentID:         digestV1("count-policy-enrollment"),
			Generation:           1,
			PreviousIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
			MutationID:           digestV1("count-policy-index-mutation"),
			Binding:              snapshot.Record.Binding,
			SnapshotRecordDigest: snapshot.Record.RecordDigest,
			AuthorityKeyID:       domainsecurity.SHA256Hex(publicKey),
			AuthorityPublicKey:   publicKey,
		},
		func(message []byte) ([]byte, error) {
			return ed25519.Sign(privateKey, message), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	selection := datasetsnapshotport.CurrentSelectionV2{
		DatasetIndexPath: []domainsecurity.DatasetSnapshotIndexV1{index},
		SelectedIndex:    index,
		Snapshot:         snapshot,
		SelectionDigest:  digestV1("count-policy-selection"),
	}
	securityContext := securityContextV1(t, fixture.observation, snapshot, nil)
	fixture.authority.selection = selection
	fixture.authority.expectedSecurityContext = securityContext
	fixture.securityContext = securityContext

	consumerCalled := false
	err = fixture.service.UseCurrent(
		context.Background(),
		securityContext,
		func(
			context.Context,
			domainfundsquerysource.DescriptorV1,
			fundsquerysourceport.ExactReadLease,
			domainnative.AccountFlowSourceRowResolverV1,
		) error {
			consumerCalled = true
			return nil
		},
	)
	if !errors.Is(err, fundsquerysourceport.ErrMismatch) || consumerCalled ||
		fixture.source.calls != 0 || fixture.materials.totalReads() != 0 {
		t.Fatalf(
			"count canary policy entered account-flow source: err=%v consumer=%t source=%d materials=%d",
			err,
			consumerCalled,
			fixture.source.calls,
			fixture.materials.totalReads(),
		)
	}
}

func TestAccountFlowSourceRowResolverRejectsConcurrentAndReentrantUse(t *testing.T) {
	fixture, rowNumber, _ := newAccountFlowResolverServiceFixtureV1(t)
	var reentrantErr error
	var concurrentErr error
	innerConsumeCalls := 0
	err := fixture.service.UseCurrent(
		context.Background(),
		fixture.securityContext,
		func(
			_ context.Context,
			_ domainfundsquerysource.DescriptorV1,
			_ fundsquerysourceport.ExactReadLease,
			resolve domainnative.AccountFlowSourceRowResolverV1,
		) error {
			return resolve(accountFlowResolverSourceFileIDV1, rowNumber, func(string) error {
				reentrantErr = resolve(accountFlowResolverSourceFileIDV1, rowNumber, func(string) error {
					innerConsumeCalls++
					return nil
				})
				concurrentDone := make(chan error, 1)
				go func() {
					concurrentDone <- resolve(accountFlowResolverSourceFileIDV1, rowNumber, func(string) error {
						innerConsumeCalls++
						return nil
					})
				}()
				concurrentErr = <-concurrentDone
				return nil
			})
		},
	)
	if err != nil || !errors.Is(reentrantErr, datasetsnapshotport.ErrMismatch) ||
		!errors.Is(concurrentErr, datasetsnapshotport.ErrMismatch) || innerConsumeCalls != 0 {
		t.Fatalf("resolver concurrent/reentrant admission: outer=%v reentrant=%v concurrent=%v consumes=%d", err, reentrantErr, concurrentErr, innerConsumeCalls)
	}
}

func TestAccountFlowSourceRowResolverRejectsEveryDoubleReadDrift(t *testing.T) {
	for _, kind := range []datasetsnapshotport.MaterialKindV2{
		datasetsnapshotport.MaterialSourceRowLedgerRootV1,
		datasetsnapshotport.MaterialSourceRowIndexPageV1,
		datasetsnapshotport.MaterialSourceRowPageV1,
		datasetsnapshotport.MaterialSourceRowRecordV1,
	} {
		t.Run(string(kind), func(t *testing.T) {
			fixture, rowNumber, _ := newAccountFlowResolverServiceFixtureV1(t)
			fixture.materials.tamperSecondKind = kind
			consumeCalls := 0
			err := fixture.service.UseCurrent(
				context.Background(),
				fixture.securityContext,
				func(
					_ context.Context,
					_ domainfundsquerysource.DescriptorV1,
					_ fundsquerysourceport.ExactReadLease,
					resolve domainnative.AccountFlowSourceRowResolverV1,
				) error {
					return resolve(accountFlowResolverSourceFileIDV1, rowNumber, func(string) error {
						consumeCalls++
						return nil
					})
				},
			)
			if !errors.Is(err, datasetsnapshotport.ErrCorrupt) || consumeCalls != 0 ||
				fixture.materials.readCount(kind) != 2 {
				t.Fatalf("double-read drift = err=%v consume=%d reads=%d", err, consumeCalls, fixture.materials.readCount(kind))
			}
		})
	}
}

func TestAccountFlowSourceRowResolverRejectsWrongTurnBinding(t *testing.T) {
	fixture, rowNumber, _ := newAccountFlowResolverServiceFixtureV1(t)
	otherObservation, err := domainsecurity.NewCaseBindingObservationV1(
		domainsecurity.CaseBindingObservationInputV1{
			WorkspaceRealPath: "/private/cases/other-funds-query-source",
			State:             domainsecurity.CaseBindingStateValid,
			CaseID:            "case-other-funds-002",
			BindingSHA256:     digestV1("other-binding-file"),
			CaseBindingHash:   digestV1("other-binding"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	wrongContext := securityContextV1(t, otherObservation, fixture.selection.Snapshot, nil)
	resolver := newAccountFlowSourceRowResolverV1(
		context.Background(),
		fixture.selection.Snapshot.Manifest,
		wrongContext,
		fixture.materials,
	)
	defer resolver.close()
	consumeCalls := 0
	err = resolver.resolve(accountFlowResolverSourceFileIDV1, rowNumber, func(string) error {
		consumeCalls++
		return nil
	})
	if !errors.Is(err, datasetsnapshotport.ErrMismatch) || consumeCalls != 0 ||
		fixture.materials.readCount(datasetsnapshotport.MaterialSourceRowLedgerRootV1) != 2 {
		t.Fatalf("wrong binding = err=%v consume=%d", err, consumeCalls)
	}
}

func TestAccountFlowSourceRowResolverRejectsMissingAndRepeatedLocators(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		fixture, _, _ := newAccountFlowResolverServiceFixtureV1(t)
		consumeCalls := 0
		err := fixture.service.UseCurrent(
			context.Background(),
			fixture.securityContext,
			func(
				_ context.Context,
				_ domainfundsquerysource.DescriptorV1,
				_ fundsquerysourceport.ExactReadLease,
				resolve domainnative.AccountFlowSourceRowResolverV1,
			) error {
				return resolve(accountFlowResolverSourceFileIDV1, 999, func(string) error {
					consumeCalls++
					return nil
				})
			},
		)
		if !errors.Is(err, datasetsnapshotport.ErrMismatch) || consumeCalls != 0 {
			t.Fatalf("missing locator = err=%v consume=%d", err, consumeCalls)
		}
	})

	t.Run("repeated", func(t *testing.T) {
		fixture, rowNumber, _ := newAccountFlowResolverServiceFixtureV1(t)
		consumeCalls := 0
		err := fixture.service.UseCurrent(
			context.Background(),
			fixture.securityContext,
			func(
				_ context.Context,
				_ domainfundsquerysource.DescriptorV1,
				_ fundsquerysourceport.ExactReadLease,
				resolve domainnative.AccountFlowSourceRowResolverV1,
			) error {
				consume := func(string) error {
					consumeCalls++
					return nil
				}
				if resolveErr := resolve(accountFlowResolverSourceFileIDV1, rowNumber, consume); resolveErr != nil {
					return resolveErr
				}
				return resolve(accountFlowResolverSourceFileIDV1, rowNumber, consume)
			},
		)
		if !errors.Is(err, datasetsnapshotport.ErrMismatch) || consumeCalls != 1 {
			t.Fatalf("repeated locator = err=%v consume=%d", err, consumeCalls)
		}
	})
}

func TestAccountFlowSourceRowResolverRetainedCallbackIsInactive(t *testing.T) {
	fixture, rowNumber, _ := newAccountFlowResolverServiceFixtureV1(t)
	var retained domainnative.AccountFlowSourceRowResolverV1
	err := fixture.service.UseCurrent(
		context.Background(),
		fixture.securityContext,
		func(
			_ context.Context,
			_ domainfundsquerysource.DescriptorV1,
			_ fundsquerysourceport.ExactReadLease,
			resolve domainnative.AccountFlowSourceRowResolverV1,
		) error {
			retained = resolve
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	consumeCalls := 0
	err = retained(accountFlowResolverSourceFileIDV1, rowNumber, func(string) error {
		consumeCalls++
		return nil
	})
	if !errors.Is(err, fundsquerysourceport.ErrUnavailable) || consumeCalls != 0 ||
		fixture.materials.totalReads() != 0 {
		t.Fatalf("retained resolver = err=%v consume=%d reads=%d", err, consumeCalls, fixture.materials.totalReads())
	}
}

func newAccountFlowResolverServiceFixtureV1(
	t *testing.T,
) (*serviceFixtureV1, uint64, string) {
	t.Helper()
	base := newServiceFixtureV1(t, snapshotKindAnalyticalV1, "account-flow-resolver-base")
	graphManifest := base.selection.Snapshot.Manifest
	materials := map[datasetsnapshotport.MaterialKindV2]map[string][]byte{}
	transactionPolicy, ok := domainevidence.ResolveSourceRowProducerPolicyV1(
		domainevidence.FundsTransactionSourceRowPolicyIDV1,
	)
	if !ok {
		t.Fatal("transaction source-row policy is unavailable")
	}
	transactionRoot, rowNumber, recordID := installAccountFlowSourceRowLedgerV1(
		t,
		transactionPolicy,
		graphManifest.Binding,
		materials,
	)
	transactionRootBody, err := domainevidence.SourceRowLedgerRootV1Bytes(transactionRoot)
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x51}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	duckDB := append([]byte(nil), base.duckDB...)
	producer, err := domainsecurity.NewFundsProducerContentManifestV1(
		domainsecurity.FundsProducerContentManifestInputV1{
			CaseID:                  graphManifest.Binding.CaseID,
			SourceRevision:          1,
			RawManifestSHA256:       graphManifest.RawArtifactManifestSHA256,
			NormalizedContentSHA256: digestV1("resolver-normalized"),
			DetailContentSHA256:     digestV1("resolver-detail"),
			AggregateContentSHA256:  digestV1("resolver-aggregate"),
			KeywordContentSHA256:    digestV1("resolver-keyword"),
			AccountContentSHA256:    digestV1("resolver-account"),
			NormalizedRowCount:      transactionRoot.RecordCount,
			AcceptedRowCount:        transactionRoot.RecordCount,
			RejectedRowCount:        0,
			DuplicateRowCount:       0,
			DetailRowCount:          transactionRoot.RecordCount,
			AggregateRowCount:       1,
			KeywordRowCount:         1,
			AccountRowCount:         1,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	analytical, err := domainsecurity.NewDatasetSnapshotAnalyticalDuckDBBindingV2(
		domainsecurity.DatasetSnapshotAnalyticalDuckDBBindingInputV2{
			DuckDBSHA256:                 domainsecurity.SHA256Hex(duckDB),
			DuckDBByteLength:             uint64(len(duckDB)),
			DuckDBContentSnapshotDigest:  digestV1("resolver-duckdb-content"),
			DuckDBSnapshotManifestSHA256: digestV1("resolver-duckdb-manifest"),
			MaterializationIdentity: domainsecurity.FundsMaterializationIdentityPrefixV1 +
				digestV1("resolver-materialization"),
			SchemaDigest:            domainfundsquerysource.FixedFundsAnalyticalSchemaDigestV1(),
			QueryProfileDigest:      domainfundsquerysource.FixedAccountFlowQueryProfileDigestV1(),
			DatasetUTCOffsetMinutes: 480,
			ExpectedCurrency:        "CNY",
			MinorUnitScale:          domainfundsquerysource.AccountFlowMinorUnitScaleV1,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	acquiredAt, err := time.Parse(time.RFC3339Nano, graphManifest.AcquiredAt)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := domainsecurity.NewDatasetSnapshotManifestV2(
		domainsecurity.DatasetSnapshotManifestInputV2{
			Binding:                           graphManifest.Binding,
			AcquisitionMethod:                 domainevidence.SourceRowSnapshotAcquisitionMethodFundsV2,
			AcquiredAt:                        acquiredAt,
			AcquisitionActorDigest:            graphManifest.AcquisitionActorDigest,
			RawArtifactManifestDigest:         graphManifest.RawArtifactManifestDigest,
			RawArtifactManifestSHA256:         graphManifest.RawArtifactManifestSHA256,
			RawArtifactManifestByteLength:     graphManifest.RawArtifactManifestByteLength,
			RawArtifactCount:                  graphManifest.RawArtifactCount,
			FundsProducerContentManifest:      producer,
			SourceType:                        transactionPolicy.SourceType,
			ProducerPolicyID:                  transactionPolicy.PolicyID,
			ProducerPolicyDigest:              transactionPolicy.PolicyDigest,
			ProducerComponentID:               transactionPolicy.ProducerComponentID,
			ProducerComponentVersion:          transactionPolicy.ProducerComponentVersion,
			ProducerOperation:                 transactionPolicy.Operation,
			ProducerOperationSchemaHash:       transactionPolicy.OperationSchemaHash,
			ParserID:                          transactionPolicy.ParserID,
			ParserVersion:                     transactionPolicy.ParserVersion,
			ParsedGenerationReceiptDigest:     digestV1("resolver-transaction-generation"),
			ParsedGenerationReceiptSHA256:     digestV1("resolver-transaction-generation-body"),
			ParsedGenerationReceiptByteLength: 640,
			ClassificationLedgerDigest:        digestV1("resolver-transaction-classification"),
			ClassificationLedgerSHA256:        digestV1("resolver-transaction-classification-body"),
			ClassificationLedgerByteLength:    384,
			TimezoneSemantics:                 domainevidence.SourceRowSnapshotTimezoneUnresolvedV2,
			CurrencySemantics:                 domainevidence.SourceRowSnapshotCurrencyUnresolvedV2,
			AnalyticalDuckDB:                  analytical,
			SourceRowLedgerRootDigest:         transactionRoot.RootDigest,
			SourceRowLedgerRootSHA256:         domainsecurity.SHA256Hex(transactionRootBody),
			SourceRowLedgerRootByteLength:     uint64(len(transactionRootBody)),
			SourceRowLedgerPageCount:          transactionRoot.PageCount,
			SourceRecordCount:                 transactionRoot.RecordCount,
			AcceptedRecordCount:               transactionRoot.RecordCount,
			RejectedRecordCount:               0,
			DuplicateRecordCount:              0,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := datasetsnapshotport.ResolvedSnapshotV2{
		Manifest:             manifest,
		FundsProducerContent: producer,
	}
	snapshot.Record = issueSnapshotRecordV1(t, manifest, producer, domainsecurity.FundsProducerContentManifestV2{}, privateKey, publicKey)
	index, err := domainsecurity.NewDatasetSnapshotIndexV1(
		domainsecurity.DatasetSnapshotIndexInputV1{
			InstallationID:       snapshot.Record.InstallationID,
			EnrollmentID:         digestV1("resolver-enrollment"),
			Generation:           1,
			PreviousIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
			MutationID:           digestV1("resolver-index-mutation"),
			Binding:              snapshot.Record.Binding,
			SnapshotRecordDigest: snapshot.Record.RecordDigest,
			AuthorityKeyID:       domainsecurity.SHA256Hex(publicKey),
			AuthorityPublicKey:   publicKey,
		},
		func(message []byte) ([]byte, error) {
			return ed25519.Sign(privateKey, message), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	selection := datasetsnapshotport.CurrentSelectionV2{
		DatasetIndexPath: []domainsecurity.DatasetSnapshotIndexV1{index},
		SelectedIndex:    index,
		Snapshot:         snapshot,
		SelectionDigest:  digestV1("resolver-selection"),
	}
	securityContext := securityContextV1(t, base.observation, snapshot, nil)
	authority := &fakeCurrentAuthorityV2{
		selection:                selection,
		expectedSecurityContext:  securityContext,
		callbackInvocations:      1,
		exactCallbackInvocations: 1,
	}
	observer := &fakeBindingObserverV1{observation: base.observation}
	reader := &fakeAdmissionMaterialReaderV2{
		values: materials,
		reads:  map[datasetsnapshotport.MaterialKindV2]int{},
		issued: map[datasetsnapshotport.MaterialKindV2][][]byte{},
	}
	source := &fakeHostExactSourceV1{body: duckDB, callbackInvocations: 1}
	service, err := NewService(authority, observer, reader, source)
	if err != nil {
		t.Fatal(err)
	}
	verifiedRowNumber, verifiedRecordID := exactResolverFixtureRecordV1(t, manifest, reader.values)
	if verifiedRowNumber != rowNumber || verifiedRecordID != recordID {
		t.Fatalf("transaction ledger fixture drifted: row=%d/%d record=%s/%s", verifiedRowNumber, rowNumber, verifiedRecordID, recordID)
	}
	return &serviceFixtureV1{
		service: service, authority: authority, observer: observer, materials: reader, source: source,
		observation: base.observation, selection: selection, securityContext: securityContext, duckDB: duckDB,
	}, rowNumber, recordID
}

func installAccountFlowSourceRowLedgerV1(
	t *testing.T,
	policy domainevidence.SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	materials map[datasetsnapshotport.MaterialKindV2]map[string][]byte,
) (domainevidence.SourceRowLedgerRootV1, uint64, string) {
	t.Helper()
	const rowNumber = uint64(41)
	locator, err := domainevidence.NewSourceRowLocatorV1(
		policy,
		domainevidence.SourceRowLocatorInputV1{
			SourceFileID:    accountFlowResolverSourceFileIDV1,
			SourceRowNumber: rowNumber,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	sourceArtifactSHA256 := digestV1("resolver-transaction-source-artifact")
	recordID, err := domainevidence.DeriveSourceRowRecordIDV1(
		policy,
		binding,
		sourceArtifactSHA256,
		locator,
	)
	if err != nil {
		t.Fatal(err)
	}
	record := domainevidence.SourceRowRecordV1{
		SchemaVersion:        domainevidence.SourceRowRecordSchemaVersionV1,
		Purpose:              domainevidence.SourceRowRecordPurposeV1,
		PolicyID:             policy.PolicyID,
		PolicyDigest:         policy.PolicyDigest,
		BindingKeyDigest:     binding.BindingKeyDigest,
		SourceRecordID:       recordID,
		Locator:              locator,
		SourceArtifactSHA256: sourceArtifactSHA256,
		CanonicalRowSHA256:   digestV1("resolver-transaction-canonical-row"),
		ParserID:             policy.ParserID,
		ParserVersion:        policy.ParserVersion,
		LineageDigest:        digestV1("resolver-transaction-lineage"),
		LineageSHA256:        digestV1("resolver-transaction-lineage-body"),
		LineageByteLength:    512,
	}
	record.RecordDigest = accountFlowResolverRecordDigestV1(record)
	if err := domainevidence.ValidateSourceRowRecordV1(policy, record); err != nil {
		t.Fatal(err)
	}
	page, err := domainevidence.NewSourceRowLedgerPageV1(
		policy,
		binding,
		1,
		[]domainevidence.SourceRowRecordV1{record},
	)
	if err != nil {
		t.Fatal(err)
	}
	pageDescriptor, err := domainevidence.NewSourceRowLedgerPageDescriptorV1(policy, binding, page)
	if err != nil {
		t.Fatal(err)
	}
	indexPage, err := domainevidence.NewSourceRowLedgerIndexPageV1(
		policy,
		binding,
		1,
		[]domainevidence.SourceRowLedgerPageDescriptorV1{pageDescriptor},
	)
	if err != nil {
		t.Fatal(err)
	}
	indexDescriptor, err := domainevidence.NewSourceRowLedgerIndexPageDescriptorV1(policy, binding, indexPage)
	if err != nil {
		t.Fatal(err)
	}
	root, err := domainevidence.NewSourceRowLedgerRootFromIndexDescriptorsV1(
		policy,
		binding,
		[]domainevidence.SourceRowLedgerIndexPageDescriptorV1{indexDescriptor},
	)
	if err != nil {
		t.Fatal(err)
	}
	recordBody, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	pageBody, err := domainevidence.SourceRowLedgerPageV1Bytes(page)
	if err != nil {
		t.Fatal(err)
	}
	indexBody, err := domainevidence.SourceRowLedgerIndexPageV1Bytes(indexPage)
	if err != nil {
		t.Fatal(err)
	}
	rootBody, err := domainevidence.SourceRowLedgerRootV1Bytes(root)
	if err != nil {
		t.Fatal(err)
	}
	for kind, body := range map[datasetsnapshotport.MaterialKindV2][]byte{
		datasetsnapshotport.MaterialSourceRowRecordV1:     recordBody,
		datasetsnapshotport.MaterialSourceRowPageV1:       pageBody,
		datasetsnapshotport.MaterialSourceRowIndexPageV1:  indexBody,
		datasetsnapshotport.MaterialSourceRowLedgerRootV1: rootBody,
	} {
		digest := domainsecurity.SHA256Hex(body)
		materials[kind] = map[string][]byte{digest: append([]byte(nil), body...)}
	}
	return root, rowNumber, recordID
}

func accountFlowResolverRecordDigestV1(record domainevidence.SourceRowRecordV1) string {
	record.RecordDigest = ""
	body, _ := json.Marshal(record)
	return domainsecurity.SHA256Hex(append(
		[]byte("analytix.source-row-record/digest/v1\x00"),
		body...,
	))
}

func exactResolverFixtureRecordV1(
	t *testing.T,
	manifest domainsecurity.DatasetSnapshotManifestV2,
	materials map[datasetsnapshotport.MaterialKindV2]map[string][]byte,
) (uint64, string) {
	t.Helper()
	root, err := domainevidence.ParseSourceRowLedgerRootV1(
		materials[datasetsnapshotport.MaterialSourceRowLedgerRootV1][manifest.SourceRowLedgerRootSHA256],
	)
	if err != nil || len(root.IndexPageDescriptors) != 1 {
		t.Fatalf("resolver root fixture is invalid: root=%#v err=%v", root, err)
	}
	indexDescriptor := root.IndexPageDescriptors[0]
	index, err := domainevidence.ParseSourceRowLedgerIndexPageV1(
		materials[datasetsnapshotport.MaterialSourceRowIndexPageV1][indexDescriptor.IndexPageSHA256],
	)
	if err != nil || len(index.PageDescriptors) != 1 {
		t.Fatalf("resolver index fixture is invalid: index=%#v err=%v", index, err)
	}
	pageDescriptor := index.PageDescriptors[0]
	page, err := domainevidence.ParseSourceRowLedgerPageV1(
		materials[datasetsnapshotport.MaterialSourceRowPageV1][pageDescriptor.PageSHA256],
	)
	if err != nil || len(page.Entries) == 0 {
		t.Fatalf("resolver page fixture is invalid: page=%#v err=%v", page, err)
	}
	record := page.Entries[0].Record
	policy, ok := domainevidence.ResolveSourceRowProducerPolicyV1(root.PolicyID)
	if !ok {
		t.Fatal("resolver fixture policy is unavailable")
	}
	locator, err := domainevidence.NewSourceRowLocatorV1(
		policy,
		domainevidence.SourceRowLocatorInputV1{
			SourceFileID:    accountFlowResolverSourceFileIDV1,
			SourceRowNumber: record.Locator.SourceRowNumber,
		},
	)
	if err != nil || locator != record.Locator {
		t.Fatalf("resolver fixture source locator is unexpected: locator=%#v record=%#v err=%v", locator, record.Locator, err)
	}
	return record.Locator.SourceRowNumber, record.SourceRecordID
}

type fakeAdmissionMaterialReaderV2 struct {
	mu               sync.Mutex
	values           map[datasetsnapshotport.MaterialKindV2]map[string][]byte
	reads            map[datasetsnapshotport.MaterialKindV2]int
	issued           map[datasetsnapshotport.MaterialKindV2][][]byte
	tamperSecondKind datasetsnapshotport.MaterialKindV2
}

func (reader *fakeAdmissionMaterialReaderV2) ResolveExact(
	ctx context.Context,
	kind datasetsnapshotport.MaterialKindV2,
	reference datasetsnapshotport.ExactMaterialReferenceV2,
) ([]byte, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	if ctx == nil || ctx.Err() != nil || reference.Address != reference.SHA256 ||
		!domainsecurity.IsSHA256Hex(reference.SHA256) || reference.ByteLength == 0 {
		return nil, datasetsnapshotport.ErrMismatch
	}
	if reader.reads == nil {
		reader.reads = map[datasetsnapshotport.MaterialKindV2]int{}
	}
	reader.reads[kind]++
	body, ok := reader.values[kind][reference.Address]
	if !ok {
		return nil, datasetsnapshotport.ErrUnavailable
	}
	result := append([]byte(nil), body...)
	if reader.tamperSecondKind == kind && reader.reads[kind] == 2 && len(result) > 0 {
		result[len(result)-1] ^= 0x01
	}
	if reader.issued == nil {
		reader.issued = map[datasetsnapshotport.MaterialKindV2][][]byte{}
	}
	reader.issued[kind] = append(reader.issued[kind], result)
	return result, nil
}

func (reader *fakeAdmissionMaterialReaderV2) readCount(kind datasetsnapshotport.MaterialKindV2) int {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	return reader.reads[kind]
}

func (reader *fakeAdmissionMaterialReaderV2) totalReads() int {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	total := 0
	for _, reads := range reader.reads {
		total += reads
	}
	return total
}

func (reader *fakeAdmissionMaterialReaderV2) allIssuedBytesCleared() bool {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	issued := 0
	for _, bodies := range reader.issued {
		for _, body := range bodies {
			issued++
			for _, value := range body {
				if value != 0 {
					return false
				}
			}
		}
	}
	return issued > 0
}
