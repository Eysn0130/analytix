package nativecomponent

import (
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
)

func TestTransactionSourceRowPageV1StrictHostPrivateRoundTrip(t *testing.T) {
	arguments, _, _, _ := transactionSourceRowTestArgumentsV1(t)
	requestID := strings.Repeat("9", 64)
	requestFrame, err := EncodeTransactionSourceRowPageNativeRequestFrameV1(requestID, arguments)
	if err != nil {
		t.Fatal(err)
	}
	requestText := string(requestFrame)
	for _, forbidden := range []string{
		"dbPath", "db_path", "sql", "privateStoredPath", "privateImportFileId",
		"parsedGenerationSha256", "/private/evidence/raw.csv", "6222021234567890123",
	} {
		if strings.Contains(requestText, forbidden) {
			t.Fatalf("pathless fixed request disclosed %q", forbidden)
		}
	}
	var request transactionSourceRowNativeRequestFrameWireV1
	if err := json.Unmarshal(requestFrame, &request); err != nil || request.RequestID != requestID ||
		request.Command != OperationFundsTransactionSourceRowPageV1 || request.CaseID != "case-source-row" ||
		request.Payload.Relation != TransactionSourceRowRelationV1 || request.Payload.MaxRows != 1 ||
		request.Payload.ParsedGenerationIdentitySHA256 != strings.Repeat("9", 64) || request.Payload.Cursor != nil {
		t.Fatalf("unexpected native request=%#v err=%v", request, err)
	}

	body := transactionSourceRowTestResultBodyV1(t, arguments)
	page, err := ParseTransactionSourceRowPageResultV1(body, arguments)
	if err != nil {
		t.Fatalf("parse valid Rust-shaped page: %v", err)
	}
	if page.RowCountV1() != 1 || page.CompleteV1() || !canonicalTransactionSourceRowDigestV1(page.PageDigestV1()) {
		t.Fatalf("unexpected safe page metadata: %s", page.String())
	}
	if _, err := json.Marshal(page); err == nil {
		t.Fatal("private source-row page became JSON serializable")
	}
	privateValues := []string{"import-private-a", "/private/evidence/raw.csv", "6222021234567890123"}
	for _, rendered := range []string{fmt.Sprint(page), fmt.Sprintf("%#v", page), fmt.Sprint(arguments), fmt.Sprintf("%#v", arguments)} {
		for _, private := range privateValues {
			if strings.Contains(rendered, private) {
				t.Fatalf("private value escaped diagnostic rendering: %q", rendered)
			}
		}
	}
	calls := 0
	if err := page.UseExact(func(view TransactionSourceRowPageHostV1) error {
		calls++
		if _, err := json.Marshal(view); err == nil {
			t.Fatal("host-private callback view became JSON serializable")
		}
		if len(view.Inventory) != 1 || view.Inventory[0].PrivateImportFileID != "import-private-a" ||
			len(view.Rows) != 1 || view.Operation != OperationFundsTransactionSourceRowPageV1 ||
			view.ParsedGenerationIdentitySHA256 != strings.Repeat("9", 64) {
			t.Fatalf("host callback lost private source observations: %s", view.String())
		}
		foundCompleteAccount := false
		for _, field := range view.Rows[0].CanonicalTypedRow {
			if field.Name == "raw.acct_no" && field.Scalar.Value == "6222021234567890123" {
				foundCompleteAccount = true
			}
		}
		if !foundCompleteAccount {
			t.Fatal("trusted host view lost complete typed replay semantics")
		}
		privateShapes := []any{
			view.Inventory[0], view.Rows[0], view.Rows[0].CanonicalTypedRow[0],
			view.Rows[0].CanonicalTypedRow[0].Scalar,
		}
		for _, shape := range privateShapes {
			if _, err := json.Marshal(shape); err == nil {
				t.Fatalf("nested host-private value became JSON serializable: %T", shape)
			}
			rendered := fmt.Sprintf("%#v", shape)
			for _, private := range privateValues {
				if strings.Contains(rendered, private) {
					t.Fatalf("nested host-private diagnostic leaked %q from %T", private, shape)
				}
			}
		}
		view.Inventory[0].PrivateImportFileID = "mutated"
		return nil
	}); err != nil || calls != 1 {
		t.Fatalf("use exact calls=%d err=%v", calls, err)
	}
	if err := page.UseExact(func(view TransactionSourceRowPageHostV1) error {
		if view.Inventory[0].PrivateImportFileID != "import-private-a" {
			t.Fatal("callback mutation altered retained private result")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	privateCallbackError := page.UseExact(func(TransactionSourceRowPageHostV1) error {
		return fmt.Errorf("private callback failed for %s", privateValues[2])
	})
	if !errors.Is(privateCallbackError, ErrResultInvalid) ||
		strings.Contains(privateCallbackError.Error(), privateValues[2]) {
		t.Fatalf("private callback error escaped public boundary: %v", privateCallbackError)
	}
}

func TestTransactionSourceRowPageV1RejectsDigestShapeAndAuthorityDrift(t *testing.T) {
	arguments, installedSnapshot, materialization, binding := transactionSourceRowTestArgumentsV1(t)
	if err := ValidateTransactionSourceRowPageArgumentsAuthorityV1(arguments, installedSnapshot); err != nil {
		t.Fatalf("pre-DSV2 installed snapshot did not bind: %v", err)
	}
	driftedObject, err := domainfundsquerysource.NewImmutableSnapshotObjectV1(
		installedSnapshot.CaseID,
		strings.Repeat("3", 64),
		installedSnapshot.DuckDBByteLength,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransactionSourceRowPageArgumentsAuthorityV1(arguments, driftedObject); !errors.Is(err, ErrContextMismatch) {
		t.Fatalf("different installed object was not rejected: %v", err)
	}
	validBody := transactionSourceRowTestResultBodyV1(t, arguments)
	var valid transactionSourceRowPageWireV1
	if err := json.Unmarshal(validBody, &valid); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*transactionSourceRowPageWireV1)
	}{
		{"inventory digest", func(value *transactionSourceRowPageWireV1) {
			value.SourceSnapshot.InventoryDigest = strings.Repeat("0", 64)
		}},
		{"snapshot digest", func(value *transactionSourceRowPageWireV1) {
			value.SourceSnapshot.SourceSnapshotDigest = strings.Repeat("0", 64)
		}},
		{"canonical row digest", func(value *transactionSourceRowPageWireV1) {
			value.Rows[0].CanonicalRowSHA256 = strings.Repeat("0", 64)
		}},
		{"page digest", func(value *transactionSourceRowPageWireV1) { value.PageDigest = strings.Repeat("0", 64) }},
		{"producer binding", func(value *transactionSourceRowPageWireV1) {
			value.ProducerContentID = domainsecurity.FundsProducerContentIDPrefixV1 + strings.Repeat("0", 64)
		}},
		{"private import binding", func(value *transactionSourceRowPageWireV1) { value.Inventory[0].PrivateImportFileID = "another-import" }},
		{"row pii changed", func(value *transactionSourceRowPageWireV1) {
			value.Rows[0].CanonicalTypedRow[0].Scalar.Value = "changed"
		}},
		{"unexpected completion", func(value *transactionSourceRowPageWireV1) { value.Complete = true }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			candidate.Inventory = append([]transactionSourceRowInventoryWireV1(nil), valid.Inventory...)
			candidate.Rows = cloneTransactionSourceRowWireForTestV1(valid.Rows)
			candidate.NextCursor = cloneTransactionSourceRowCursorV1(valid.NextCursor)
			test.mutate(&candidate)
			body, err := json.Marshal(candidate)
			if err != nil {
				t.Fatal(err)
			}
			if result, err := ParseTransactionSourceRowPageResultV1(body, arguments); err == nil || result.RowCountV1() != 0 {
				t.Fatalf("hostile result survived: result=%s err=%v", result.String(), err)
			}
		})
	}

	var reordered map[string]any
	if err := json.Unmarshal(validBody, &reordered); err != nil {
		t.Fatal(err)
	}
	reorderedBody, err := json.Marshal(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseTransactionSourceRowPageResultV1(reorderedBody, arguments); err == nil {
		t.Fatal("lexically reordered result survived the exact Rust field-order contract")
	}
	unknown := append(append([]byte(nil), validBody[:len(validBody)-1]...), []byte(`,"privateSQL":"SELECT *"}`)...)
	if _, err := ParseTransactionSourceRowPageResultV1(unknown, arguments); err == nil {
		t.Fatal("unknown result field survived")
	}
	retiredPrivatePath := bytes.Replace(
		validBody,
		[]byte(`"privateImportFileId":"import-private-a",`),
		[]byte(`"privateImportFileId":"import-private-a","privateStoredPath":"/retired",`),
		1,
	)
	if bytes.Equal(retiredPrivatePath, validBody) {
		t.Fatal("failed to inject retired private path field")
	}
	if _, err := ParseTransactionSourceRowPageResultV1(retiredPrivatePath, arguments); err == nil {
		t.Fatal("retired privateStoredPath field survived the exact result contract")
	}

	driftedMaterialization := materialization
	driftedMaterialization.RawArtifactManifestSHA256 = strings.Repeat("0", 64)
	if _, err := NewTransactionSourceRowPageArgumentsV1(TransactionSourceRowPageArgumentsInputV1{
		Binding: binding, ParsedGenerationIdentitySHA256: strings.Repeat("9", 64),
		Materialization: driftedMaterialization, InstalledSnapshot: installedSnapshot, MaxRows: 1,
	}); err == nil {
		t.Fatal("unverified materializer drift survived request construction")
	}

	driftedSnapshot, err := domainfundsquerysource.NewImmutableSnapshotObjectV1(
		"case-other",
		installedSnapshot.DuckDBSHA256,
		installedSnapshot.DuckDBByteLength,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTransactionSourceRowPageArgumentsV1(TransactionSourceRowPageArgumentsInputV1{
		Binding: binding, ParsedGenerationIdentitySHA256: strings.Repeat("9", 64),
		Materialization: materialization, InstalledSnapshot: driftedSnapshot, MaxRows: 1,
	}); err == nil {
		t.Fatal("installed snapshot for a different case survived request construction")
	}
}

func transactionSourceRowTestArgumentsV1(
	t *testing.T,
) (
	TransactionSourceRowPageArgumentsV1,
	domainfundsquerysource.ImmutableSnapshotObjectV1,
	domainsecurity.FundsMaterializationResultV1,
	domainsecurity.DatasetSnapshotBindingKeyV1,
) {
	t.Helper()
	binding := transactionSourceRowTestBindingV1(t)
	materialization := transactionSourceRowTestMaterializationV1(t, binding.CaseID)
	installedSnapshot, err := domainfundsquerysource.NewImmutableSnapshotObjectV1(
		binding.CaseID,
		strings.Repeat("4", 64),
		4096,
	)
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := NewTransactionSourceRowPageArgumentsV1(TransactionSourceRowPageArgumentsInputV1{
		Binding: binding, ParsedGenerationIdentitySHA256: strings.Repeat("9", 64),
		Materialization: materialization, InstalledSnapshot: installedSnapshot, MaxRows: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return arguments, installedSnapshot, materialization, binding
}

func transactionSourceRowTestBindingV1(t *testing.T) domainsecurity.DatasetSnapshotBindingKeyV1 {
	t.Helper()
	binding, err := domainsecurity.NewDatasetSnapshotBindingKeyV1(domainsecurity.DatasetSnapshotBindingKeyInputV1{
		TenantID:                 domainsecurity.LocalTenantID,
		UserID:                   domainsecurity.LocalUserID,
		WorkspaceRealPath:        "/cases/case-source-row",
		CaseID:                   "case-source-row",
		CaseBindingHash:          strings.Repeat("5", 64),
		BindingObservationDigest: strings.Repeat("6", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func transactionSourceRowTestMaterializationV1(
	t *testing.T,
	caseID string,
) domainsecurity.FundsMaterializationResultV1 {
	t.Helper()
	rawArtifactManifestSHA256 := strings.Repeat("a", 64)
	producer, err := domainsecurity.NewFundsProducerContentManifestV1(domainsecurity.FundsProducerContentManifestInputV1{
		CaseID:                  caseID,
		SourceRevision:          7,
		RawManifestSHA256:       rawArtifactManifestSHA256,
		NormalizedContentSHA256: strings.Repeat("b", 64),
		DetailContentSHA256:     strings.Repeat("c", 64),
		AggregateContentSHA256:  strings.Repeat("d", 64),
		KeywordContentSHA256:    strings.Repeat("e", 64),
		AccountContentSHA256:    strings.Repeat("f", 64),
		NormalizedRowCount:      2,
		AcceptedRowCount:        2,
		DetailRowCount:          2,
		AggregateRowCount:       1,
		KeywordRowCount:         1,
		AccountRowCount:         1,
	})
	if err != nil {
		t.Fatal(err)
	}
	producerBody, err := domainsecurity.FundsProducerContentManifestV1Bytes(producer)
	if err != nil {
		t.Fatal(err)
	}
	rawSources := domainsecurity.FundsRawSourceManifestV1{{
		CleanedStatus:    "done",
		FileID:           "import-private-a",
		RowsImportedNorm: 2,
		SHA256:           strings.Repeat("7", 64),
		Status:           "已完成",
	}}
	rawSourceBody, err := json.Marshal(rawSources)
	if err != nil {
		t.Fatal(err)
	}
	materialization := domainsecurity.FundsMaterializationResultV1{
		CaseID:                               caseID,
		RowCount:                             producer.AggregateRowCount,
		AggregateName:                        domainsecurity.FundsMaterializationIdentityPrefixV1 + strings.Repeat("0", 64),
		AggregateVersion:                     domainsecurity.FundsMaterializationAggregateVersionV1,
		MaterializationIdentity:              domainsecurity.FundsMaterializationIdentityPrefixV1 + strings.Repeat("0", 64),
		MaterializationIdentitySchemaVersion: domainsecurity.FundsMaterializationIdentitySchemaVersionV1,
		ProducerContentID:                    domainsecurity.DeriveFundsProducerContentIDV1(producer),
		ProducerContentManifestBase64:        base64.StdEncoding.EncodeToString(producerBody),
		ProducerContentManifestSHA256:        domainsecurity.SHA256Hex(producerBody),
		ProducerContentManifestByteLength:    uint64(len(producerBody)),
		RawArtifactManifestSHA256:            rawArtifactManifestSHA256,
		RawSourceManifestBase64:              base64.StdEncoding.EncodeToString(rawSourceBody),
		RawSourceManifestSHA256:              domainsecurity.SHA256Hex(rawSourceBody),
		RawSourceManifestByteLength:          uint64(len(rawSourceBody)),
		DuckDBContentSnapshotDigest:          strings.Repeat("1", 64),
		DuckDBSnapshotManifestSHA256:         strings.Repeat("2", 64),
		SchemaDigest:                         domainsecurity.FixedFundsAnalyticalSchemaDigestV1(),
	}
	body, err := json.Marshal(materialization)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := domainsecurity.ParseFundsMaterializationResultV1(body)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func transactionSourceRowTestResultBodyV1(
	t *testing.T,
	arguments TransactionSourceRowPageArgumentsV1,
) []byte {
	t.Helper()
	fields := make([]transactionSourceRowTypedFieldWireV1, 0, len(transactionSourceRowTypedFieldKindsV1))
	names := make([]string, 0, len(transactionSourceRowTypedFieldKindsV1))
	for name := range transactionSourceRowTypedFieldKindsV1 {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		scalar := transactionSourceRowScalarWireV1{Kind: "null", Value: ""}
		switch name {
		case "raw.acct_no", "norm.clean_acct_no":
			scalar = transactionSourceRowScalarWireV1{Kind: "text", Value: "6222021234567890123"}
		case "norm.clean_amount":
			scalar = transactionSourceRowScalarWireV1{Kind: "decimal", Value: "12.5"}
		case "norm.clean_invalid", "norm.clean_failed", "norm.clean_reversal":
			scalar = transactionSourceRowScalarWireV1{Kind: "integer", Value: "0"}
		}
		fields = append(fields, transactionSourceRowTypedFieldWireV1{Name: name, Scalar: scalar})
	}
	privateFileID := "import-private-a"
	sourceFileID := deriveTransactionSourceRowSourceFileIDV1(arguments.binding.CaseID, privateFileID)
	sourceFileDigest := deriveTransactionSourceRowSourceFileDigestV1(sourceFileID)
	inventory := []transactionSourceRowInventoryWireV1{{
		SourceFileID:         sourceFileID,
		SourceFileIDDigest:   sourceFileDigest,
		PrivateImportFileID:  privateFileID,
		SourceArtifactSHA256: strings.Repeat("7", 64),
		FileType:             "CSV",
		RowCount:             2,
		AcceptedRowCount:     2,
		RejectedRowCount:     0,
	}}
	inventoryDigest, err := transactionSourceRowDomainDigestV1(transactionSourceRowInventoryDomainV1, inventory)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := transactionSourceRowSnapshotWireV1{
		SourceRevision:    7,
		SourceRowCount:    2,
		SourceMaxTxnTS:    "2026-01-01 00:00:00",
		SourceMaxID:       2,
		AcceptedRowCount:  2,
		RejectedRowCount:  0,
		DuplicateRowCount: 0,
		InventoryDigest:   inventoryDigest,
	}
	snapshotDigest, err := transactionSourceRowDomainDigestV1(transactionSourceRowSnapshotDomainV1, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.SourceSnapshotDigest = snapshotDigest
	rowDigest, err := transactionSourceRowDomainDigestV1(transactionSourceRowCanonicalRowDomainV1, fields)
	if err != nil {
		t.Fatal(err)
	}
	row := transactionSourceRowWireV1{
		SourceFileID:         sourceFileID,
		SourceFileIDDigest:   sourceFileDigest,
		SourceRowNumber:      1,
		SourceArtifactSHA256: strings.Repeat("7", 64),
		CanonicalRowSHA256:   rowDigest,
		CanonicalTypedRow:    fields,
		Disposition:          "accepted",
	}
	page := transactionSourceRowPageWireV1{
		SchemaVersion:                  1,
		Operation:                      OperationFundsTransactionSourceRowPageV1,
		CaseID:                         arguments.binding.CaseID,
		BindingKeyDigest:               arguments.binding.BindingKeyDigest,
		ParsedGenerationIdentitySHA256: arguments.parsedGenerationIdentitySHA256,
		Relation:                       arguments.relation,
		MaterializationIdentity:        arguments.materializationIdentity,
		ProducerContentContract:        arguments.producerContentContract,
		ProducerContentID:              arguments.producerContentID,
		ProducerContentManifestSHA256:  arguments.producerManifestSHA256,
		RawArtifactManifestSHA256:      arguments.rawArtifactManifestSHA256,
		DuckDBContentSnapshotDigest:    arguments.duckDBContentSnapshotDigest,
		DuckDBSnapshotManifestSHA256:   arguments.duckDBSnapshotManifestSHA256,
		AnalyticalSchemaDigest:         arguments.analyticalSchemaDigest,
		ParserID:                       TransactionSourceRowParserIDV1,
		ParserVersion:                  TransactionSourceRowParserVersionV1,
		LocatorOrdering:                TransactionSourceRowLocatorOrderingV1,
		SourceProofStatus:              TransactionSourceRowProofStatusV1,
		RequiredHostRawReplayProfile:   TransactionSourceRowHostReplayProfileV1,
		HostRawReplayRequired:          true,
		SourceSnapshot:                 snapshot,
		Inventory:                      inventory,
		Rows:                           []transactionSourceRowWireV1{row},
		NextCursor:                     &TransactionSourceRowCursorV1{SourceFileIDDigest: sourceFileDigest, SourceRowNumber: 1},
		Complete:                       false,
		PageDigest:                     strings.Repeat("0", 64),
	}
	placeholder, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	page.PageDigest, err = transactionSourceRowPageDigestV1(placeholder)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func cloneTransactionSourceRowWireForTestV1(rows []transactionSourceRowWireV1) []transactionSourceRowWireV1 {
	clone := append([]transactionSourceRowWireV1(nil), rows...)
	for index := range clone {
		clone[index].CanonicalTypedRow = append([]transactionSourceRowTypedFieldWireV1(nil), rows[index].CanonicalTypedRow...)
	}
	return clone
}
