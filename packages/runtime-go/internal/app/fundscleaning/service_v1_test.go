package fundscleaning

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	fundscsvadmissionapp "analytix.local/runtime-go/internal/app/fundscsvadmission"
	localdisplayapp "analytix.local/runtime-go/internal/app/localdisplay"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainlocaldisplay "analytix.local/runtime-go/internal/domain/localdisplay"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	fundsquerysourcefixture "analytix.local/runtime-go/internal/testsupport/fundsquerysourcefixture"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type cleaningIdentityStubV1 struct {
	mu        sync.Mutex
	principal domainidentity.PrincipalV1
	current   bool
}

func (stub *cleaningIdentityStubV1) ResolveCurrent(context.Context) (domainidentity.PrincipalV1, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if !stub.current {
		return domainidentity.PrincipalV1{}, errors.New("identity unavailable")
	}
	return stub.principal, nil
}

func (stub *cleaningIdentityStubV1) ValidateCurrent(_ context.Context, principal domainidentity.PrincipalV1) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if !stub.current || !domainidentity.SamePrincipalV1(stub.principal, principal) {
		return errors.New("identity changed")
	}
	return nil
}

func (stub *cleaningIdentityStubV1) setCurrent(current bool) {
	stub.mu.Lock()
	stub.current = current
	stub.mu.Unlock()
}

type cleaningSourceStubV1 struct {
	mu      sync.Mutex
	input   domainfundsquerysource.DescriptorV1
	current domainfundsquerysource.DescriptorV1
}

func (stub *cleaningSourceStubV1) UseCurrentLocalDisplay(
	ctx context.Context,
	_ string,
	_ string,
	_ string,
	use func(context.Context, domainfundsquerysource.DescriptorV1, fundsquerysourceport.ExactReadLease) error,
) error {
	stub.mu.Lock()
	input := stub.input
	stub.mu.Unlock()
	lease := fundsquerysourcefixture.NewExactReadLeaseV1([]byte("synthetic immutable DuckDB"))
	defer lease.Close()
	return use(ctx, input, lease)
}

func (stub *cleaningSourceStubV1) ResolveCurrentLocalDisplay(
	context.Context,
	string,
	string,
	string,
) (domainfundsquerysource.DescriptorV1, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	return stub.current, nil
}

func (stub *cleaningSourceStubV1) setCurrent(descriptor domainfundsquerysource.DescriptorV1) {
	stub.mu.Lock()
	stub.current = descriptor
	stub.mu.Unlock()
}

type cleaningNativeStubV1 struct {
	result domainnative.DeterministicCleaningResultV1
	body   []byte
	err    error
}

func (stub *cleaningNativeStubV1) DeterministicCleaning(
	context.Context,
	domainnative.DeterministicCleaningArgumentsV1,
	domainfundsquerysource.DescriptorV1,
	fundsquerysourceport.ExactReadLease,
) (domainnative.DeterministicCleaningResultV1, []byte, error) {
	return stub.result, append([]byte(nil), stub.body...), stub.err
}

type cleaningAdmissionStubV1 struct {
	source          *cleaningSourceStubV1
	output          domainfundsquerysource.DescriptorV1
	status          string
	inputSnapshotID string
	err             error
	calls           int
}

func (stub *cleaningAdmissionStubV1) CommitCleaningV1(
	ctx context.Context,
	input fundscsvadmissionapp.CleaningCommitInputV1,
) (fundscsvadmissionapp.CleaningCommitResultV1, error) {
	stub.calls++
	if ctx.Err() != nil {
		return fundscsvadmissionapp.CleaningCommitResultV1{}, ctx.Err()
	}
	if stub.err != nil {
		return fundscsvadmissionapp.CleaningCommitResultV1{}, stub.err
	}
	if input.ExpectedInputSnapshotID != stub.source.input.DatasetSnapshotID ||
		input.SourceSHA256 != domainsecurity.SHA256Hex(input.SourceBody) || input.SourceRowCount != 2 {
		return fundscsvadmissionapp.CleaningCommitResultV1{}, errors.New("cleaning admission input changed")
	}
	if stub.status == fundscsvadmissionapp.CleaningCommitStatusCommittedV1 {
		stub.source.setCurrent(stub.output)
	}
	inputSnapshotID := input.ExpectedInputSnapshotID
	if stub.inputSnapshotID != "" {
		inputSnapshotID = stub.inputSnapshotID
	}
	return fundscsvadmissionapp.CleaningCommitResultV1{
		Status: stub.status, InputDatasetSnapshotID: inputSnapshotID,
		OutputDatasetSnapshotID: stub.output.DatasetSnapshotID,
	}, nil
}

func TestDeterministicCleaningProducesCurrentDSV2AndPostOperationTypedDiff(t *testing.T) {
	now := time.Date(2026, 8, 22, 3, 0, 0, 0, time.UTC)
	identity := cleaningIdentityV1(t, "principal-a")
	input := cleaningDescriptorV1(t, "input", "case-cleaning")
	output := cleaningDescriptorV1(t, "output", "case-cleaning")
	body, nativeResult := cleaningNativeResultV1(t, input)
	source := &cleaningSourceStubV1{input: input, current: input}
	admission := &cleaningAdmissionStubV1{
		source: source, output: output, status: fundscsvadmissionapp.CleaningCommitStatusCommittedV1,
	}
	service, err := NewServiceV1(ConfigV1{
		Identity: identity, Source: source,
		Native:    &cleaningNativeStubV1{result: nativeResult, body: body},
		Admission: admission, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.RunV1(context.Background(), RunInputV1{WorkspaceRoot: "/synthetic/case"})
	if err != nil || result.Status != ResultStatusCommittedV1 ||
		result.InputSnapshot == result.OutputSnapshot || result.RuleDigest != nativeResult.RuleDigest ||
		result.RowCount != 2 || result.ChangedRowCount != 1 || admission.calls != 1 {
		t.Fatalf("deterministic cleaning did not commit one current DSV2: result=%#v err=%v", result, err)
	}
	replayedAuthority := cleaningAuthorityV1(identity.principal, input, output.DatasetSnapshotID, nativeResult)
	if replayedAuthority.Selector != result.Selector || replayedAuthority.InputSnapshot != result.InputSnapshot ||
		replayedAuthority.OutputSnapshot != result.OutputSnapshot || replayedAuthority.TransformLineage != result.TransformLineage {
		t.Fatalf("identical input and rule did not replay stable lineage: got=%#v want=%#v", replayedAuthority, result)
	}

	local := localdisplayapp.NewServiceWithTypedLocalDataSurface(
		nil, nil,
		localdisplayapp.ImportMappingPreviewDependenciesV1{},
		localdisplayapp.CleaningDiffPreviewDependenciesV1{Identity: identity, Reader: service},
		localdisplayapp.DirectSourcePreviewDependenciesV1{},
		localdisplayapp.AcceptedSlotDisplayDependenciesV1{},
	)
	full, err := local.CleaningDiffPreview(context.Background(), localdisplayapp.CleaningDiffPreviewInputV1{
		Selector: result.Selector, Fields: []string{"counterpartyAccount", "amountText"}, RowLimit: 2,
		DisplayMode: localdisplayapp.DisplayModeFull,
	})
	if err != nil || len(full.Rows) != 2 || full.Rows[0].Status != "changed" ||
		full.Rows[0].Cells[0].BeforeDisplayValue != "CP-001_ 23" ||
		full.Rows[0].Cells[0].AfterDisplayValue != "CP00123" ||
		full.Rows[0].Cells[1].BeforeDisplayValue != "1.2" ||
		full.Rows[0].Cells[1].AfterDisplayValue != "1.2" || full.Rows[1].Status != "unchanged" {
		t.Fatalf("full cleaning diff was not exact: response=%#v err=%v", full, err)
	}
	masked, err := local.CleaningDiffPreview(context.Background(), localdisplayapp.CleaningDiffPreviewInputV1{
		Selector: result.Selector, Fields: []string{"counterpartyAccount", "amountText"}, RowLimit: 2,
		DisplayMode: localdisplayapp.DisplayModeMasked,
	})
	if err != nil || masked.Lineage != full.Lineage || masked.Rows[0].Cells[0].BeforeDisplayValue != "****_ 23" ||
		masked.Rows[0].Cells[0].AfterDisplayValue != "****0123" ||
		masked.Rows[0].Cells[1].BeforeDisplayValue != "1.2" {
		t.Fatalf("masked projection changed authority or safe exact money: response=%#v err=%v", masked, err)
	}
	empty, err := local.CleaningDiffPreview(context.Background(), localdisplayapp.CleaningDiffPreviewInputV1{
		Selector: result.Selector, Fields: []string{"account"}, RowOffset: 99, RowLimit: 1,
		DisplayMode: localdisplayapp.DisplayModeFull,
	})
	if err != nil || len(empty.Rows) != 0 || empty.HasMore {
		t.Fatalf("bounded offset beyond the result did not return an empty page: response=%#v err=%v", empty, err)
	}
	identity.setCurrent(false)
	if err := service.RevokeCleaningDiffPreviewV1(context.Background(), result.Selector); err != nil {
		t.Fatal(err)
	}
	identity.setCurrent(true)
	if _, err := local.CleaningDiffPreview(context.Background(), localdisplayapp.CleaningDiffPreviewInputV1{
		Selector: result.Selector, Fields: []string{"account"}, RowLimit: 1,
		DisplayMode: localdisplayapp.DisplayModeFull,
	}); !errors.Is(err, localdisplayapp.ErrUnavailable) {
		t.Fatalf("revoked exact diff remained readable: %v", err)
	}
}

func TestDeterministicCleaningFailureOutcomeUnknownExpiryAndDriftFailClosed(t *testing.T) {
	now := time.Date(2026, 8, 22, 4, 0, 0, 0, time.UTC)
	identity := cleaningIdentityV1(t, "principal-b")
	input := cleaningDescriptorV1(t, "input-b", "case-cleaning")
	output := cleaningDescriptorV1(t, "output-b", "case-cleaning")
	body, nativeResult := cleaningNativeResultV1(t, input)

	t.Run("pre-CAS failure preserves input", func(t *testing.T) {
		source := &cleaningSourceStubV1{input: input, current: input}
		service, err := NewServiceV1(ConfigV1{
			Identity: identity, Source: source,
			Native: &cleaningNativeStubV1{result: nativeResult, body: body},
			Admission: &cleaningAdmissionStubV1{
				source: source, output: output, status: fundscsvadmissionapp.CleaningCommitStatusPreCASFailedV1,
			},
			Now: func() time.Time { return now },
		})
		if err != nil {
			t.Fatal(err)
		}
		if result, err := service.RunV1(context.Background(), RunInputV1{WorkspaceRoot: "/synthetic/case"}); !errors.Is(err, ErrPreCASFailed) || result.Status != "" || source.current.DatasetSnapshotID != input.DatasetSnapshotID {
			t.Fatalf("failed cleaning changed current input: result=%#v current=%q err=%v", result, source.current.DatasetSnapshotID, err)
		}
	})

	t.Run("mismatched pre-CAS status remains outcome unknown", func(t *testing.T) {
		source := &cleaningSourceStubV1{input: input, current: input}
		service, err := NewServiceV1(ConfigV1{
			Identity: identity, Source: source,
			Native: &cleaningNativeStubV1{result: nativeResult, body: body},
			Admission: &cleaningAdmissionStubV1{
				source: source, output: output, status: fundscsvadmissionapp.CleaningCommitStatusPreCASFailedV1,
				inputSnapshotID: securitycontexttest.DatasetSnapshotID("wrong-pre-cas-input"),
			},
			Now: func() time.Time { return now },
		})
		if err != nil {
			t.Fatal(err)
		}
		result, runErr := service.RunV1(context.Background(), RunInputV1{WorkspaceRoot: "/synthetic/case"})
		if runErr != nil || result.Status != ResultStatusOutcomeUnknownV1 || result.Selector != "" {
			t.Fatalf("mismatched pre-CAS status was trusted: result=%#v err=%v", result, runErr)
		}
	})

	t.Run("unclassified admission error remains outcome unknown", func(t *testing.T) {
		source := &cleaningSourceStubV1{input: input, current: input}
		service, err := NewServiceV1(ConfigV1{
			Identity: identity, Source: source,
			Native:    &cleaningNativeStubV1{result: nativeResult, body: body},
			Admission: &cleaningAdmissionStubV1{source: source, output: output, err: errors.New("ambiguous CAS failure")},
			Now:       func() time.Time { return now },
		})
		if err != nil {
			t.Fatal(err)
		}
		result, runErr := service.RunV1(context.Background(), RunInputV1{WorkspaceRoot: "/synthetic/case"})
		if runErr != nil || result.Status != ResultStatusOutcomeUnknownV1 || result.Selector != "" {
			t.Fatalf("ambiguous admission error was downgraded: result=%#v err=%v", result, runErr)
		}
	})

	t.Run("invalid native result fails before CAS and preserves input", func(t *testing.T) {
		source := &cleaningSourceStubV1{input: input, current: input}
		admission := &cleaningAdmissionStubV1{
			source: source, output: output, status: fundscsvadmissionapp.CleaningCommitStatusCommittedV1,
		}
		service, err := NewServiceV1(ConfigV1{
			Identity: identity, Source: source,
			Native:    &cleaningNativeStubV1{err: domainnative.ErrResultInvalid},
			Admission: admission, Now: func() time.Time { return now },
		})
		if err != nil {
			t.Fatal(err)
		}
		result, runErr := service.RunV1(context.Background(), RunInputV1{WorkspaceRoot: "/synthetic/case"})
		if !errors.Is(runErr, domainnative.ErrResultInvalid) || !errors.Is(runErr, ErrPreCASFailed) ||
			result.Status != "" || result.Selector != "" ||
			admission.calls != 0 || source.current.DatasetSnapshotID != input.DatasetSnapshotID {
			t.Fatalf("invalid native result crossed CAS: result=%#v admissionCalls=%d current=%q err=%v", result, admission.calls, source.current.DatasetSnapshotID, runErr)
		}
	})

	t.Run("post-CAS outcome unknown exposes no selector", func(t *testing.T) {
		source := &cleaningSourceStubV1{input: input, current: input}
		service, err := NewServiceV1(ConfigV1{
			Identity: identity, Source: source,
			Native: &cleaningNativeStubV1{result: nativeResult, body: body},
			Admission: &cleaningAdmissionStubV1{
				source: source, output: output, status: fundscsvadmissionapp.CleaningCommitStatusOutcomeUnknownV1,
			},
			Now: func() time.Time { return now },
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := service.RunV1(context.Background(), RunInputV1{WorkspaceRoot: "/synthetic/case"})
		if err != nil || result.Status != ResultStatusOutcomeUnknownV1 || result.Selector != "" {
			t.Fatalf("post-CAS uncertainty minted preview authority: result=%#v err=%v", result, err)
		}
	})

	t.Run("expiry and output drift revoke exact rows", func(t *testing.T) {
		source := &cleaningSourceStubV1{input: input, current: input}
		admission := &cleaningAdmissionStubV1{
			source: source, output: output, status: fundscsvadmissionapp.CleaningCommitStatusCommittedV1,
		}
		service, err := NewServiceV1(ConfigV1{
			Identity: identity, Source: source,
			Native:    &cleaningNativeStubV1{result: nativeResult, body: body},
			Admission: admission, Now: func() time.Time { return now },
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := service.RunV1(context.Background(), RunInputV1{WorkspaceRoot: "/synthetic/case"})
		if err != nil {
			t.Fatal(err)
		}
		wrongSelector := "tlsel1_" + domainsecurity.SHA256Hex([]byte("old-cleaning-generation"))
		if err := service.UseCurrentCleaningDiffPreviewV1(
			context.Background(), identity.principal, wrongSelector, []string{"account"}, 0, 1,
			func(context.Context, domainlocaldisplay.CleaningDiffAuthorityV1, []domainlocaldisplay.CleaningDiffRowV1, bool) error {
				return nil
			},
		); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("wrong selector was accepted: %v", err)
		}
		if err := service.ValidateCurrentCleaningDiffPreviewV1(
			context.Background(), identity.principal, cleaningAuthorityV1(identity.principal, input, output.DatasetSnapshotID, nativeResult),
		); err != nil {
			t.Fatalf("old selector invalidated the newer active generation: %v", err)
		}
		now = now.Add(activeCleaningLifetimeV1 + time.Second)
		if err := service.UseCurrentCleaningDiffPreviewV1(
			context.Background(), identity.principal, result.Selector, []string{"account"}, 0, 1,
			func(context.Context, domainlocaldisplay.CleaningDiffAuthorityV1, []domainlocaldisplay.CleaningDiffRowV1, bool) error {
				return nil
			},
		); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("expired preview remained readable: %v", err)
		}

		now = time.Date(2026, 8, 22, 4, 0, 0, 0, time.UTC)
		source.setCurrent(input)
		result, err = service.RunV1(context.Background(), RunInputV1{WorkspaceRoot: "/synthetic/case"})
		if err != nil {
			t.Fatal(err)
		}
		source.setCurrent(cleaningDescriptorV1(t, "unexpected", "case-cleaning"))
		if err := service.UseCurrentCleaningDiffPreviewV1(
			context.Background(), identity.principal, result.Selector, []string{"account"}, 0, 1,
			func(context.Context, domainlocaldisplay.CleaningDiffAuthorityV1, []domainlocaldisplay.CleaningDiffRowV1, bool) error {
				return nil
			},
		); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("output drift remained readable: %v", err)
		}
	})

	t.Run("shutdown revokes exact rows", func(t *testing.T) {
		source := &cleaningSourceStubV1{input: input, current: input}
		service, err := NewServiceV1(ConfigV1{
			Identity: identity, Source: source,
			Native: &cleaningNativeStubV1{result: nativeResult, body: body},
			Admission: &cleaningAdmissionStubV1{
				source: source, output: output, status: fundscsvadmissionapp.CleaningCommitStatusCommittedV1,
			},
			Now: func() time.Time { return now },
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := service.RunV1(context.Background(), RunInputV1{WorkspaceRoot: "/synthetic/case"})
		if err != nil {
			t.Fatal(err)
		}
		if err := service.Close(); err != nil {
			t.Fatal(err)
		}
		if err := service.ValidateCurrentCleaningDiffPreviewV1(
			context.Background(), identity.principal,
			cleaningAuthorityV1(identity.principal, input, output.DatasetSnapshotID, nativeResult),
		); !errors.Is(err, ErrUnavailable) || result.Selector == "" {
			t.Fatalf("shutdown retained cleaning preview: selector=%q err=%v", result.Selector, err)
		}
	})
}

func cleaningIdentityV1(t *testing.T, material string) *cleaningIdentityStubV1 {
	t.Helper()
	principal, err := domainidentity.NewPrincipalV1(
		domainsecurity.SHA256Hex([]byte("installation:"+material)), "tenant-cleaning", "user-cleaning",
	)
	if err != nil {
		t.Fatal(err)
	}
	return &cleaningIdentityStubV1{principal: principal, current: true}
}

func cleaningDescriptorV1(t *testing.T, material, caseID string) domainfundsquerysource.DescriptorV1 {
	t.Helper()
	digest := func(field string) string { return domainsecurity.SHA256Hex([]byte(material + ":" + field)) }
	descriptor, err := domainfundsquerysource.NewDescriptorV1(domainfundsquerysource.DescriptorInputV1{
		SnapshotRecordDigest: digest("record"), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID(material),
		SourceManifestHash: digest("source"), CaseID: caseID,
		CaseBindingHash:      domainsecurity.SHA256Hex([]byte("case-binding:" + caseID)),
		DatasetBindingDigest: digest("dataset-binding"), BindingObservationDigest: digest("observation"),
		FundsProducerContentID:             domainsecurity.FundsProducerContentIDPrefixV1 + digest("producer"),
		FundsProducerContentManifestSHA256: digest("producer-manifest"), FundsProducerContentManifestByteLength: 2048,
		DuckDBSHA256: digest("duckdb"), DuckDBByteLength: 8192,
		DuckDBContentSnapshotDigest: digest("duckdb-content"), DuckDBSnapshotManifestSHA256: digest("duckdb-manifest"),
		MaterializationIdentity: "txn_daily_snapshot:v12:" + digest("materialization"),
		SchemaDigest:            domainfundsquerysource.FixedFundsAnalyticalSchemaDigestV1(),
		ExpectedCurrency:        "CNY", MinorUnitScale: 2,
		QueryProfileDigest: domainfundsquerysource.FixedFundsLocalDisplayQueryProfileDigestV1(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return descriptor
}

func cleaningNativeResultV1(
	t *testing.T,
	descriptor domainfundsquerysource.DescriptorV1,
) ([]byte, domainnative.DeterministicCleaningResultV1) {
	t.Helper()
	columns := domainevidence.FundsTransactionCSVColumnsV1()
	var body bytes.Buffer
	writer := csv.NewWriter(&body)
	writer.UseCRLF = true
	header := make([]string, len(columns))
	for index := range columns {
		header[index] = columns[index].Header
	}
	if err := writer.Write(header); err != nil {
		t.Fatal(err)
	}
	first := make([]string, len(columns))
	first[0], first[1], first[4], first[5], first[7], first[8], first[14] = "card-1", "62229999", "2026-08-22 01:02:03", "1.2", "进", "CP00123", "CNY"
	second := make([]string, len(columns))
	second[0], second[1], second[4], second[5], second[7], second[14] = "card-2", "account-2", "2026-08-22 01:02:04", "2.00", "出", "CNY"
	if err := writer.Write(first); err != nil {
		t.Fatal(err)
	}
	if err := writer.Write(second); err != nil {
		t.Fatal(err)
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		t.Fatal(err)
	}
	arguments, err := domainnative.NewDeterministicCleaningArgumentsV1(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	type cell struct {
		Field       domainnative.DirectSourcePreviewFieldV1 `json:"field"`
		BeforeValue string                                  `json:"beforeValue"`
		AfterValue  string                                  `json:"afterValue"`
		BeforeState string                                  `json:"beforeState"`
		AfterState  string                                  `json:"afterState"`
	}
	type row struct {
		RowIndex uint32 `json:"rowIndex"`
		Status   string `json:"status"`
		Cells    []cell `json:"cells"`
	}
	type material struct {
		SchemaVersion            uint8  `json:"schemaVersion"`
		Operation                string `json:"operation"`
		InputDatasetSnapshotID   string `json:"inputDatasetSnapshotId"`
		RuleGeneration           string `json:"ruleGeneration"`
		RuleDigest               string `json:"ruleDigest"`
		OutputArtifactSHA256     string `json:"outputArtifactSha256"`
		OutputArtifactByteLength uint64 `json:"outputArtifactByteLength"`
		RowCount                 uint64 `json:"rowCount"`
		ChangedRowCount          uint64 `json:"changedRowCount"`
		UnchangedRowCount        uint64 `json:"unchangedRowCount"`
		ChangedRows              []row  `json:"changedRows"`
	}
	changedRows := []row{{RowIndex: 0, Status: "changed", Cells: []cell{
		{Field: "counterpartyAccount", BeforeValue: "CP-001_ 23", AfterValue: "CP00123", BeforeState: "value", AfterState: "value"},
	}}}
	wire := material{
		SchemaVersion: 1, Operation: domainnative.OperationFundsDeterministicCleaningV1,
		InputDatasetSnapshotID: descriptor.DatasetSnapshotID,
		RuleGeneration:         arguments.RuleGeneration, RuleDigest: arguments.RuleDigest,
		OutputArtifactSHA256: domainsecurity.SHA256Hex(body.Bytes()), OutputArtifactByteLength: uint64(body.Len()),
		RowCount: 2, ChangedRowCount: 1, UnchangedRowCount: 1, ChangedRows: changedRows,
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("analytix.funds.deterministic-cleaning-result/v1\x00"))
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(encoded)))
	_, _ = hasher.Write(length[:])
	_, _ = hasher.Write(encoded)
	resultWire := struct {
		material
		ResultDigest string `json:"resultDigest"`
	}{material: wire, ResultDigest: hex.EncodeToString(hasher.Sum(nil))}
	resultBody, err := json.Marshal(resultWire)
	if err != nil {
		t.Fatal(err)
	}
	result, err := domainnative.ParseDeterministicCleaningResultV1(resultBody, arguments)
	if err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), body.Bytes()...), result
}
