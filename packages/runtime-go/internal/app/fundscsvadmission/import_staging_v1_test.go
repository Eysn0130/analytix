package fundscsvadmission

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	localdisplayapp "analytix.local/runtime-go/internal/app/localdisplay"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainlocaldisplay "analytix.local/runtime-go/internal/domain/localdisplay"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	nativecomponentport "analytix.local/runtime-go/internal/ports/nativecomponent"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

type privateNativeDiagnosticErrorV1 struct{}

func (privateNativeDiagnosticErrorV1) Error() string { panic("private error must never be formatted") }

func TestNativeBuildFailureDiagnosticPrivacyV1(t *testing.T) {
	private := privateNativeDiagnosticErrorV1{}
	for _, test := range []struct {
		err   error
		class string
	}{
		{nativecomponentport.ErrTerminationUnconfirmed, "TERMINATION"}, {context.Canceled, "CANCELLED"},
		{context.DeadlineExceeded, "DEADLINE"}, {nativecomponentport.ErrProtocolInvalid, "PROTOCOL"},
		{nativecomponentport.ErrRegistryInvalid, "REGISTRY"}, {nativecomponentport.ErrRequestInvalid, "REQUEST_INVALID"},
		{fundsquerysourceport.ErrNotFound, "SOURCE_NOT_FOUND"}, {fundsquerysourceport.ErrMismatch, "SOURCE_MISMATCH"},
		{fundsquerysourceport.ErrCorrupt, "SOURCE_CORRUPT"}, {ErrUnavailable, "UNAVAILABLE"}, {private, "UNKNOWN"},
	} {
		var output bytes.Buffer
		original := errors.Join(test.err, private)
		writeNativeBuildFailureV1(&output, "NATIVE_CALL", original)
		if output.String() != "[analytix] event=ANALYTIX_FUNDS_CSV_NATIVE_FAILURE_V1 layer=ADMISSION stage=NATIVE_CALL class="+test.class+"\n" ||
			!errors.Is(original, test.err) {
			t.Fatal("native diagnostic leaked or changed its sentinel class")
		}
	}
	var output bytes.Buffer
	writeNativeBuildFailureV1(&output, "private-stage-canary", &os.PathError{Op: "private-op", Path: "/private/canary", Err: private})
	if output.String() != "[analytix] event=ANALYTIX_FUNDS_CSV_NATIVE_FAILURE_V1 layer=ADMISSION stage=UNKNOWN class=UNKNOWN\n" {
		t.Fatal("unknown stage or PathError escaped the closed projection")
	}
	output.Reset()
	writeNativeBuildFailureV1(&output, "NATIVE_CALL", nil)
	if output.Len() != 0 {
		t.Fatal("successful native build produced a failure diagnostic")
	}
}

type nativeBuildDiagnosticStubV1 struct {
	NativeOwnerV1
	err   error
	calls int
}

func (stub *nativeBuildDiagnosticStubV1) BuildFundsCanonicalCSVSnapshot(context.Context, domainnative.FundsCanonicalCSVSnapshotBuildArgumentsV1, io.Reader, fundsquerysourceport.ImmutableSnapshotInstaller) (domainnative.FundsCanonicalCSVSnapshotBuildResultV1, domainfundsquerysource.ImmutableSnapshotObjectV1, fundsquerysourceport.ImmutableSnapshotInstallDispositionV1, error) {
	stub.calls++
	return domainnative.FundsCanonicalCSVSnapshotBuildResultV1{}, domainfundsquerysource.ImmutableSnapshotObjectV1{}, "", stub.err
}

func TestNativeBuildFailureRetainsOriginalErrorV1(t *testing.T) {
	binding, err := domainsecurity.NewDatasetSnapshotBindingKeyV1(domainsecurity.DatasetSnapshotBindingKeyInputV1{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, WorkspaceRealPath: "/cases/native-diagnostic", CaseID: "case-native-diagnostic",
		CaseBindingHash: strings.Repeat("a", 64), BindingObservationDigest: strings.Repeat("b", 64),
	})
	if err != nil {
		t.Fatal("synthetic binding invalid")
	}
	body := []byte("synthetic-source")
	build := domainevidence.FundsCanonicalCSVNativeBuildContextV1{Binding: binding, PrivateImportFileID: "0123456789abcdef0123",
		RawArtifactManifestSHA256: strings.Repeat("c", 64), SourceArtifactSHA256: domainsecurity.SHA256Hex(body),
		SourceArtifactByteLength: uint64(len(body)), SourceRowCount: 1}
	for _, test := range []struct {
		cause        error
		stage, class string
	}{
		{errors.Join(nativecomponentport.ErrRegistryInvalid, privateNativeDiagnosticErrorV1{}), "NATIVE_CALL", "REGISTRY"},
		{nil, "OBJECT_VALIDATION", "UNAVAILABLE"},
	} {
		func() {
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal("diagnostic capture unavailable")
			}
			previous := os.Stderr
			os.Stderr = writer
			defer func() { os.Stderr = previous; _ = writer.Close(); _ = reader.Close() }()
			stub := &nativeBuildDiagnosticStubV1{err: test.cause}
			service := &ServiceV1{native: stub}
			_, got := service.buildNativeV1(context.Background(), build, 1, body)
			os.Stderr = previous
			_ = writer.Close()
			output, readErr := io.ReadAll(reader)
			if readErr != nil || stub.calls != 1 || !errors.Is(got, ErrUnavailable) || errors.Is(got, ErrInvalidRequest) ||
				test.cause != nil && !errors.Is(got, test.cause) ||
				string(output) != "[analytix] event=ANALYTIX_FUNDS_CSV_NATIVE_FAILURE_V1 layer=ADMISSION stage="+test.stage+" class="+test.class+"\n" {
				t.Fatal("diagnostics changed native-call error semantics or lost its exact stage")
			}
		}()
	}
}

type importObserverStubV1 struct {
	observation domainsecurity.CaseBindingObservationV1
}

func (stub *importObserverStubV1) Observe(workspace string) (domainsecurity.CaseBindingObservationV1, error) {
	if stub == nil || stub.observation.WorkspaceRealPath != workspace {
		return domainsecurity.CaseBindingObservationV1{}, errors.New("unexpected workspace")
	}
	return stub.observation, nil
}

type importIdentityStubV1 struct {
	principal domainidentity.PrincipalV1
	valid     bool
}

func (stub *importIdentityStubV1) ResolveCurrent(context.Context) (domainidentity.PrincipalV1, error) {
	if stub == nil || !stub.valid {
		return domainidentity.PrincipalV1{}, errors.New("identity unavailable")
	}
	return stub.principal, nil
}

func (stub *importIdentityStubV1) ValidateCurrent(_ context.Context, principal domainidentity.PrincipalV1) error {
	if stub == nil || !stub.valid || !domainidentity.SamePrincipalV1(stub.principal, principal) {
		return errors.New("identity changed")
	}
	return nil
}

type mutableImportSourceV1 struct {
	format string
	body   []byte
}

type barrierImportSourceV1 struct {
	oldStarted chan struct{}
	releaseOld chan struct{}
	oldBody    []byte
	newBody    []byte
}

type cancelAfterRandomReadV1 struct {
	cancel context.CancelFunc
}

func (reader cancelAfterRandomReadV1) Read(target []byte) (int, error) {
	for index := range target {
		target[index] = 0x64
	}
	reader.cancel()
	return len(target), nil
}

func (source *mutableImportSourceV1) read(
	ctx context.Context,
	workspace string,
	_ string,
	use func(
		context.Context,
		string,
		string,
		[]byte,
		string,
		func(context.Context, func(context.Context, []byte, string) error) error,
	) error,
) error {
	if source == nil || ctx.Err() != nil {
		return ErrInvalidRequest
	}
	frozen := append([]byte(nil), source.body...)
	digest := domainsecurity.SHA256Hex(frozen)
	defer clear(frozen)
	revalidate := func(revalidateContext context.Context, consume func(context.Context, []byte, string) error) error {
		current := append([]byte(nil), source.body...)
		defer clear(current)
		return consume(revalidateContext, current, domainsecurity.SHA256Hex(current))
	}
	return use(ctx, workspace, source.format, frozen, digest, revalidate)
}

func (source *barrierImportSourceV1) read(
	_ context.Context,
	workspace string,
	sourcePath string,
	use func(
		context.Context,
		string,
		string,
		[]byte,
		string,
		func(context.Context, func(context.Context, []byte, string) error) error,
	) error,
) error {
	body := source.newBody
	if strings.Contains(sourcePath, "old") {
		close(source.oldStarted)
		<-source.releaseOld
		body = source.oldBody
	}
	frozen := append([]byte(nil), body...)
	defer clear(frozen)
	digest := domainsecurity.SHA256Hex(frozen)
	revalidate := func(ctx context.Context, consume func(context.Context, []byte, string) error) error {
		current := append([]byte(nil), body...)
		defer clear(current)
		return consume(ctx, current, domainsecurity.SHA256Hex(current))
	}
	// Intentionally ignore the acquisition context here. The staging owner must
	// still reject a superseded invocation whose host reader completes late.
	return use(context.Background(), workspace, "csv", frozen, digest, revalidate)
}

func TestHostImportCSVAndZIPStagePreviewConfirmLifecycle(t *testing.T) {
	workspace := "/synthetic/case-a"
	identity := importIdentityForTestV1(t, "principal-a")
	observer := &importObserverStubV1{observation: importObservationForTestV1(t, workspace, "case-a")}
	source := &mutableImportSourceV1{format: "csv", body: canonicalImportCSVForTestV1(t, 2, "IMPORT_SOURCE_EXACT_CANARY")}
	now := time.Date(2026, 8, 22, 1, 2, 3, 0, time.UTC)
	var committed []byte
	service := &ServiceV1{
		observer: observer, identity: identity, readImportSource: source.read,
		now: func() time.Time { return now }, random: bytes.NewReader(bytes.Repeat([]byte{0x41}, 512)),
	}
	service.commitExact = func(_ context.Context, gotWorkspace string, body []byte, digest string, rows uint64) (StageResultV1, error) {
		if gotWorkspace != workspace || digest != domainsecurity.SHA256Hex(body) || rows != 2 {
			t.Fatal("confirm did not bind the frozen canonical generation")
		}
		committed = append([]byte(nil), body...)
		return StageResultV1{SourceArtifactSHA256: digest, SourceArtifactByteLength: uint64(len(body)), SourceRowCount: rows}, nil
	}

	staged, err := service.StageMainSelectedImportV1(context.Background(), StageInputV1{WorkspaceRoot: workspace, SourcePath: "/main-private/source.csv"})
	if err != nil || staged.Status != importStagingReadyV1 || staged.TotalRowCount != 2 || len(staged.Items) != 1 ||
		!domainlocaldisplay.ValidSelectorV1(staged.Items[0].Selector) || staged.Items[0].SourceLabel != "Source 1 of 1" {
		t.Fatalf("CSV stage failed: result=%#v err=%v", staged, err)
	}
	selector := staged.Items[0].Selector
	previewCalls := 0
	err = service.UseCurrentImportMappingPreviewV1(
		context.Background(), identity.principal, selector,
		[]string{"sourceColumn", "sampleValue", "targetField", "parseStatus", "mappingStatus"},
		0, 34,
		func(_ context.Context, authority domainlocaldisplay.ImportMappingAuthorityV1, rows []domainlocaldisplay.ImportMappingRowV1, hasMore bool) error {
			previewCalls++
			if hasMore || len(rows) != 34 || domainlocaldisplay.ValidateImportMappingAuthorityV1(authority) != nil {
				t.Fatal("preview window or authority changed")
			}
			found := false
			for _, row := range rows {
				for _, cell := range row.Cells {
					_ = cell.Value.UseExactV1(func(value string) error {
						found = found || value == "IMPORT_SOURCE_EXACT_CANARY"
						return nil
					})
				}
			}
			if !found {
				t.Fatal("source-exact canary did not reach the typed callback")
			}
			return nil
		},
	)
	if err != nil || previewCalls != 1 {
		t.Fatalf("typed preview failed: calls=%d err=%v", previewCalls, err)
	}
	typedLocal := localdisplayapp.NewServiceWithTypedLocalDataSurface(
		nil, nil,
		localdisplayapp.ImportMappingPreviewDependenciesV1{Identity: identity, Reader: service},
		localdisplayapp.CleaningDiffPreviewDependenciesV1{},
		localdisplayapp.DirectSourcePreviewDependenciesV1{},
		localdisplayapp.AcceptedSlotDisplayDependenciesV1{},
	)
	full, err := typedLocal.ImportMappingPreview(context.Background(), localdisplayapp.ImportMappingPreviewInputV1{
		Selector: selector, Fields: []string{"sampleValue"}, RowOffset: 0, RowLimit: 1, DisplayMode: "full",
	})
	if err != nil || full.Rows[0].Cells[0].DisplayValue != "IMPORT_SOURCE_EXACT_CANARY" {
		t.Fatalf("full typed sink lost the exact canary: response=%#v err=%v", full, err)
	}
	masked, err := typedLocal.ImportMappingPreview(context.Background(), localdisplayapp.ImportMappingPreviewInputV1{
		Selector: selector, Fields: []string{"sampleValue"}, RowOffset: 0, RowLimit: 1, DisplayMode: "masked",
	})
	if err != nil || masked.Rows[0].Cells[0].DisplayValue == "IMPORT_SOURCE_EXACT_CANARY" {
		t.Fatalf("masked typed sink retained the exact canary: response=%#v err=%v", masked, err)
	}
	confirmed, err := service.ConfirmImportV1(context.Background(), selector)
	if err != nil || confirmed.SourceRowCount != 2 || len(committed) == 0 {
		t.Fatalf("confirm failed: result=%#v err=%v", confirmed, err)
	}
	defer clear(committed)
	if _, err := service.ImportStatusV1(context.Background(), selector); err == nil {
		t.Fatal("confirmed selector remained live")
	}

	zipBody := zipImportForTestV1(t, map[string][]byte{
		"z-last.csv":  canonicalImportCSVForTestV1(t, 1, "ZIP_Z"),
		"a-first.csv": canonicalImportCSVForTestV1(t, 1, "ZIP_A"),
	})
	source.format, source.body = "zip", zipBody
	staged, err = service.StageMainSelectedImportV1(context.Background(), StageInputV1{WorkspaceRoot: workspace, SourcePath: "/main-private/source.zip"})
	if err != nil || staged.TotalRowCount != 2 || len(staged.Items) != 2 ||
		staged.Items[0].SourceLabel != "Source 1 of 2" || staged.Items[1].SourceLabel != "Source 2 of 2" ||
		staged.Items[0].Selector == staged.Items[1].Selector {
		t.Fatalf("multi-member ZIP stage failed: result=%#v err=%v", staged, err)
	}
	for index, expected := range []string{"ZIP_A", "ZIP_Z"} {
		got := ""
		err := service.UseCurrentImportMappingPreviewV1(
			context.Background(), identity.principal, staged.Items[index].Selector,
			[]string{"sampleValue"}, 0, 1,
			func(_ context.Context, _ domainlocaldisplay.ImportMappingAuthorityV1, rows []domainlocaldisplay.ImportMappingRowV1, _ bool) error {
				return rows[0].Cells[0].Value.UseExactV1(func(value string) error { got = value; return nil })
			},
		)
		if err != nil || got != expected {
			t.Fatalf("stable member %d preview mismatch: got=%q err=%v", index, got, err)
		}
	}
}

func TestHostImportReselectCasePrincipalExpiryAndReplacementRevoke(t *testing.T) {
	workspace := "/synthetic/case-a"
	identity := importIdentityForTestV1(t, "principal-a")
	observer := &importObserverStubV1{observation: importObservationForTestV1(t, workspace, "case-a")}
	source := &mutableImportSourceV1{format: "csv", body: canonicalImportCSVForTestV1(t, 1, "CASE_A")}
	now := time.Date(2026, 8, 22, 1, 2, 3, 0, time.UTC)
	commits := 0
	randomBytes := make([]byte, 1024)
	for index := range randomBytes {
		randomBytes[index] = byte(index)
	}
	service := &ServiceV1{
		observer: observer, identity: identity, readImportSource: source.read,
		now: func() time.Time { return now }, random: bytes.NewReader(randomBytes),
		commitExact: func(_ context.Context, _ string, body []byte, digest string, rows uint64) (StageResultV1, error) {
			commits++
			return StageResultV1{SourceArtifactSHA256: digest, SourceArtifactByteLength: uint64(len(body)), SourceRowCount: rows}, nil
		},
	}
	stage := func() string {
		result, err := service.StageMainSelectedImportV1(context.Background(), StageInputV1{WorkspaceRoot: workspace, SourcePath: "/private/source.csv"})
		if err != nil {
			t.Fatal(err)
		}
		return result.Items[0].Selector
	}
	first := stage()
	second := stage()
	if first == second {
		t.Fatal("reselection reused a selector")
	}
	if _, err := service.ImportStatusV1(context.Background(), first); err == nil {
		t.Fatal("reselection did not revoke the old selector")
	}
	if _, err := service.CancelImportV1(context.Background(), first); err == nil {
		t.Fatal("stale selector canceled a newer generation")
	}
	if _, err := service.ImportStatusV1(context.Background(), second); err != nil {
		t.Fatalf("stale selector revoked the current generation: %v", err)
	}
	source.body = canonicalImportCSVForTestV1(t, 1, "REPLACED")
	if _, err := service.ConfirmImportV1(context.Background(), second); err == nil || commits != 0 {
		t.Fatalf("replacement reached commit: commits=%d err=%v", commits, err)
	}
	third := stage()
	identity.valid = false
	if _, err := service.ImportStatusV1(context.Background(), third); err == nil {
		t.Fatal("principal revocation left the selector live")
	}
	identity.valid = true
	fourth := stage()
	observer.observation = importObservationForTestV1(t, workspace, "case-b")
	if _, err := service.ImportStatusV1(context.Background(), fourth); err == nil {
		t.Fatal("case switch left the selector live")
	}
	caseBSelector := stage()
	if caseBSelector == fourth {
		t.Fatal("case A and B reused a selector")
	}
	restarted := &ServiceV1{
		observer: observer, identity: identity, readImportSource: source.read,
		now: func() time.Time { return now }, random: bytes.NewReader(bytes.Repeat([]byte{0x55}, 64)),
	}
	if _, err := restarted.ImportStatusV1(context.Background(), caseBSelector); err == nil {
		t.Fatal("unconfirmed generation survived process-local restart")
	}
	observer.observation = importObservationForTestV1(t, workspace, "case-a")
	fifth := stage()
	now = now.Add(ImportStagingLifetimeV1 + time.Second)
	if _, err := service.ImportStatusV1(context.Background(), fifth); err == nil {
		t.Fatal("expired selector remained live")
	}
}

func TestImportConfirmFailureRevokesGenerationWithoutReturningSuccess(t *testing.T) {
	workspace := "/synthetic/case-failure"
	identity := importIdentityForTestV1(t, "principal-failure")
	observer := &importObserverStubV1{observation: importObservationForTestV1(t, workspace, "case-failure")}
	source := &mutableImportSourceV1{format: "csv", body: canonicalImportCSVForTestV1(t, 1, "FAILURE")}
	service := &ServiceV1{
		observer: observer, identity: identity, readImportSource: source.read,
		now: time.Now, random: bytes.NewReader(bytes.Repeat([]byte{0x61}, 64)),
		commitExact: func(context.Context, string, []byte, string, uint64) (StageResultV1, error) {
			return StageResultV1{}, errors.New("synthetic native or CAS failure")
		},
	}
	staged, err := service.StageMainSelectedImportV1(context.Background(), StageInputV1{
		WorkspaceRoot: workspace, SourcePath: "/private/source.csv",
	})
	if err != nil {
		t.Fatal(err)
	}
	selector := staged.Items[0].Selector
	if _, err := service.ConfirmImportV1(context.Background(), selector); err == nil {
		t.Fatal("commit failure returned success")
	}
	if _, err := service.ImportStatusV1(context.Background(), selector); err == nil {
		t.Fatal("failed confirmation left a partial generation active")
	}
}

func TestImportCancelInterruptsInFlightConfirmationAndRevokesExactBytes(t *testing.T) {
	workspace := "/synthetic/case-cancel"
	identity := importIdentityForTestV1(t, "principal-cancel")
	observer := &importObserverStubV1{observation: importObservationForTestV1(t, workspace, "case-cancel")}
	source := &mutableImportSourceV1{format: "csv", body: canonicalImportCSVForTestV1(t, 1, "CANCEL")}
	commitStarted := make(chan struct{})
	commitReturned := make(chan struct{})
	var confirmationBytes []byte
	service := &ServiceV1{
		observer: observer, identity: identity, readImportSource: source.read,
		now: time.Now, random: bytes.NewReader(bytes.Repeat([]byte{0x62}, 64)),
		commitExact: func(ctx context.Context, _ string, body []byte, _ string, _ uint64) (StageResultV1, error) {
			confirmationBytes = body
			close(commitStarted)
			defer close(commitReturned)
			<-ctx.Done()
			return StageResultV1{}, ctx.Err()
		},
	}
	staged, err := service.StageMainSelectedImportV1(context.Background(), StageInputV1{
		WorkspaceRoot: workspace, SourcePath: "/private/source.csv",
	})
	if err != nil {
		t.Fatal(err)
	}
	selector := staged.Items[0].Selector
	service.stagingMu.Lock()
	activeCanonicalBytes := service.activeImport.canonicalBody
	activeSampleBytes := service.activeImport.items[0].columns[0].sampleValue
	service.stagingMu.Unlock()
	confirmResult := make(chan error, 1)
	go func() {
		_, confirmErr := service.ConfirmImportV1(context.Background(), selector)
		confirmResult <- confirmErr
	}()
	select {
	case <-commitStarted:
	case <-time.After(time.Second):
		t.Fatal("confirmation did not reach the cancellable commit cut point")
	}
	canceled, err := service.CancelImportV1(context.Background(), selector)
	if err != nil || !canceled.Canceled {
		t.Fatalf("cancel did not revoke in-flight confirmation: result=%#v err=%v", canceled, err)
	}
	select {
	case err := <-confirmResult:
		if err == nil {
			t.Fatal("canceled confirmation returned success")
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not interrupt confirmation")
	}
	select {
	case <-commitReturned:
	case <-time.After(time.Second):
		t.Fatal("canceled commit did not stop")
	}
	for name, exactBytes := range map[string][]byte{
		"active canonical": activeCanonicalBytes,
		"active sample":    activeSampleBytes,
		"confirm copy":     confirmationBytes,
	} {
		if len(exactBytes) == 0 {
			t.Fatalf("%s fixture did not retain an inspectable exact buffer", name)
		}
		for _, value := range exactBytes {
			if value != 0 {
				t.Fatalf("%s bytes were not zeroed after cancel", name)
			}
		}
	}
	if _, err := service.ImportStatusV1(context.Background(), selector); err == nil {
		t.Fatal("canceled confirmation left its selector or bytes active")
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStalePreviewConfirmAndCancelCannotRevokeNewGeneration(t *testing.T) {
	workspace := "/synthetic/case-stale-late"
	identity := importIdentityForTestV1(t, "principal-stale-late")
	observer := &importObserverStubV1{observation: importObservationForTestV1(t, workspace, "case-stale-late")}
	source := &mutableImportSourceV1{format: "csv", body: canonicalImportCSVForTestV1(t, 1, "OLD_GENERATION")}
	oldCommitStarted := make(chan struct{})
	service := &ServiceV1{
		observer: observer, identity: identity, readImportSource: source.read,
		now: time.Now, random: bytes.NewReader(bytes.Repeat([]byte{0x67}, 128)),
		commitExact: func(ctx context.Context, _ string, body []byte, digest string, rows uint64) (StageResultV1, error) {
			if bytes.Contains(body, []byte("OLD_GENERATION")) {
				close(oldCommitStarted)
				<-ctx.Done()
				return StageResultV1{}, ctx.Err()
			}
			if !bytes.Contains(body, []byte("NEW_GENERATION")) {
				return StageResultV1{}, errors.New("unexpected generation reached commit")
			}
			return StageResultV1{
				SourceArtifactSHA256: digest, SourceArtifactByteLength: uint64(len(body)), SourceRowCount: rows,
			}, nil
		},
	}
	oldStage, err := service.StageMainSelectedImportV1(context.Background(), StageInputV1{
		WorkspaceRoot: workspace, SourcePath: "/private/old.csv",
	})
	if err != nil {
		t.Fatal(err)
	}
	oldSelector := oldStage.Items[0].Selector
	var oldAuthority domainlocaldisplay.ImportMappingAuthorityV1
	err = service.UseCurrentImportMappingPreviewV1(
		context.Background(), identity.principal, oldSelector, []string{"sampleValue"}, 0, 1,
		func(_ context.Context, authority domainlocaldisplay.ImportMappingAuthorityV1, _ []domainlocaldisplay.ImportMappingRowV1, _ bool) error {
			oldAuthority = authority
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	oldConfirmResult := make(chan error, 1)
	go func() {
		_, confirmErr := service.ConfirmImportV1(context.Background(), oldSelector)
		oldConfirmResult <- confirmErr
	}()
	select {
	case <-oldCommitStarted:
	case <-time.After(time.Second):
		t.Fatal("old confirmation did not reach the concurrent commit cut point")
	}

	source.body = canonicalImportCSVForTestV1(t, 1, "NEW_GENERATION")
	newStage, err := service.StageMainSelectedImportV1(context.Background(), StageInputV1{
		WorkspaceRoot: workspace, SourcePath: "/private/new.csv",
	})
	if err != nil {
		t.Fatal(err)
	}
	newSelector := newStage.Items[0].Selector
	select {
	case confirmErr := <-oldConfirmResult:
		if confirmErr == nil {
			t.Fatal("reselected old confirmation returned success")
		}
	case <-time.After(time.Second):
		t.Fatal("reselection did not cancel the old in-flight confirmation")
	}

	if _, err := service.CancelImportV1(context.Background(), oldSelector); err == nil {
		t.Fatal("late old cancel returned success")
	}
	if _, err := service.ConfirmImportV1(context.Background(), oldSelector); err == nil {
		t.Fatal("late old confirm returned success")
	}
	if err := service.UseCurrentImportMappingPreviewV1(
		context.Background(), identity.principal, oldSelector, []string{"sampleValue"}, 0, 1,
		func(context.Context, domainlocaldisplay.ImportMappingAuthorityV1, []domainlocaldisplay.ImportMappingRowV1, bool) error {
			t.Fatal("late old preview exposed the new generation")
			return nil
		},
	); err == nil {
		t.Fatal("late old preview returned success")
	}
	if err := service.ValidateCurrentImportMappingPreviewV1(context.Background(), identity.principal, oldAuthority); err == nil {
		t.Fatal("late old preview authority remained valid")
	}
	if err := service.UseCurrentImportMappingPreviewV1(
		context.Background(), identity.principal, "malformed-selector", []string{"sampleValue"}, 0, 1,
		func(context.Context, domainlocaldisplay.ImportMappingAuthorityV1, []domainlocaldisplay.ImportMappingRowV1, bool) error {
			t.Fatal("malformed preview selector exposed the new generation")
			return nil
		},
	); err == nil {
		t.Fatal("malformed preview selector returned success")
	}

	newSample := ""
	err = service.UseCurrentImportMappingPreviewV1(
		context.Background(), identity.principal, newSelector, []string{"sampleValue"}, 0, 1,
		func(_ context.Context, _ domainlocaldisplay.ImportMappingAuthorityV1, rows []domainlocaldisplay.ImportMappingRowV1, _ bool) error {
			return rows[0].Cells[0].Value.UseExactV1(func(value string) error { newSample = value; return nil })
		},
	)
	if err != nil || newSample != "NEW_GENERATION" {
		t.Fatalf("late old actions damaged the new preview: sample=%q err=%v", newSample, err)
	}
	confirmed, err := service.ConfirmImportV1(context.Background(), newSelector)
	if err != nil || confirmed.SourceRowCount != 1 {
		t.Fatalf("late old actions damaged the new confirmation: result=%#v err=%v", confirmed, err)
	}
}

func TestLateStageCompletionCannotReplaceOrClearNewGeneration(t *testing.T) {
	workspace := "/synthetic/case-stage-interleave"
	identity := importIdentityForTestV1(t, "principal-stage-interleave")
	observer := &importObserverStubV1{observation: importObservationForTestV1(t, workspace, "case-stage-interleave")}
	source := &barrierImportSourceV1{
		oldStarted: make(chan struct{}),
		releaseOld: make(chan struct{}),
		oldBody:    canonicalImportCSVForTestV1(t, 1, "OLD_STAGE_LATE"),
		newBody:    canonicalImportCSVForTestV1(t, 1, "NEW_STAGE_CURRENT"),
	}
	service := &ServiceV1{
		observer: observer, identity: identity, readImportSource: source.read,
		now: time.Now, random: bytes.NewReader(bytes.Repeat([]byte{0x68}, 128)),
		commitExact: func(_ context.Context, _ string, body []byte, digest string, rows uint64) (StageResultV1, error) {
			if !bytes.Contains(body, []byte("NEW_STAGE_CURRENT")) || bytes.Contains(body, []byte("OLD_STAGE_LATE")) {
				return StageResultV1{}, errors.New("superseded stage reached confirmation")
			}
			return StageResultV1{
				SourceArtifactSHA256: digest, SourceArtifactByteLength: uint64(len(body)), SourceRowCount: rows,
			}, nil
		},
	}
	oldResult := make(chan error, 1)
	go func() {
		_, err := service.StageMainSelectedImportV1(context.Background(), StageInputV1{
			WorkspaceRoot: workspace, SourcePath: "/private/old.csv",
		})
		oldResult <- err
	}()
	select {
	case <-source.oldStarted:
	case <-time.After(time.Second):
		t.Fatal("old stage did not reach the acquisition barrier")
	}

	newStage, err := service.StageMainSelectedImportV1(context.Background(), StageInputV1{
		WorkspaceRoot: workspace, SourcePath: "/private/new.csv",
	})
	if err != nil || len(newStage.Items) != 1 {
		t.Fatalf("new stage did not become current: result=%#v err=%v", newStage, err)
	}
	newSelector := newStage.Items[0].Selector
	newSample := ""
	err = service.UseCurrentImportMappingPreviewV1(
		context.Background(), identity.principal, newSelector, []string{"sampleValue"}, 0, 1,
		func(_ context.Context, _ domainlocaldisplay.ImportMappingAuthorityV1, rows []domainlocaldisplay.ImportMappingRowV1, _ bool) error {
			return rows[0].Cells[0].Value.UseExactV1(func(value string) error { newSample = value; return nil })
		},
	)
	if err != nil || newSample != "NEW_STAGE_CURRENT" {
		t.Fatalf("new stage preview failed before old completion: sample=%q err=%v", newSample, err)
	}

	close(source.releaseOld)
	select {
	case oldErr := <-oldResult:
		if oldErr == nil {
			t.Fatal("superseded old stage returned success")
		}
	case <-time.After(time.Second):
		t.Fatal("superseded old stage did not return")
	}
	confirmed, err := service.ConfirmImportV1(context.Background(), newSelector)
	if err != nil || confirmed.SourceRowCount != 1 {
		t.Fatalf("late old stage damaged the new confirmation: result=%#v err=%v", confirmed, err)
	}
}

func TestCloseCancelsInFlightStageAndRejectsItsLateCompletion(t *testing.T) {
	workspace := "/synthetic/case-stage-close"
	identity := importIdentityForTestV1(t, "principal-stage-close")
	observer := &importObserverStubV1{observation: importObservationForTestV1(t, workspace, "case-stage-close")}
	source := &barrierImportSourceV1{
		oldStarted: make(chan struct{}),
		releaseOld: make(chan struct{}),
		oldBody:    canonicalImportCSVForTestV1(t, 1, "CLOSED_STAGE"),
		newBody:    canonicalImportCSVForTestV1(t, 1, "UNUSED"),
	}
	service := &ServiceV1{
		observer: observer, identity: identity, readImportSource: source.read,
		now: time.Now, random: bytes.NewReader(bytes.Repeat([]byte{0x69}, 64)),
	}
	stageResult := make(chan error, 1)
	go func() {
		_, err := service.StageMainSelectedImportV1(context.Background(), StageInputV1{
			WorkspaceRoot: workspace, SourcePath: "/private/old.csv",
		})
		stageResult <- err
	}()
	select {
	case <-source.oldStarted:
	case <-time.After(time.Second):
		t.Fatal("stage did not reach the acquisition barrier")
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	close(source.releaseOld)
	select {
	case err := <-stageResult:
		if err == nil {
			t.Fatal("stage completed successfully after service close")
		}
	case <-time.After(time.Second):
		t.Fatal("closed stage did not return")
	}
	service.stagingMu.Lock()
	defer service.stagingMu.Unlock()
	if service.stageInFlight != nil || service.activeImport != nil {
		t.Fatal("service close left a stage or generation active")
	}
}

func TestConfirmSuccessRemainsLinearizedWhenCallerCancelsAfterCommit(t *testing.T) {
	workspace := "/synthetic/case-confirm-linearization"
	identity := importIdentityForTestV1(t, "principal-confirm-linearization")
	observer := &importObserverStubV1{observation: importObservationForTestV1(t, workspace, "case-confirm-linearization")}
	source := &mutableImportSourceV1{format: "csv", body: canonicalImportCSVForTestV1(t, 1, "LINEARIZED")}
	confirmContext, cancelConfirm := context.WithCancel(context.Background())
	service := &ServiceV1{
		observer: observer, identity: identity, readImportSource: source.read,
		now: time.Now, random: bytes.NewReader(bytes.Repeat([]byte{0x6a}, 64)),
		commitExact: func(_ context.Context, _ string, body []byte, digest string, rows uint64) (StageResultV1, error) {
			cancelConfirm()
			return StageResultV1{
				SourceArtifactSHA256: digest, SourceArtifactByteLength: uint64(len(body)), SourceRowCount: rows,
			}, nil
		},
	}
	staged, err := service.StageMainSelectedImportV1(context.Background(), StageInputV1{
		WorkspaceRoot: workspace, SourcePath: "/private/source.csv",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.ConfirmImportV1(confirmContext, staged.Items[0].Selector)
	if err != nil || result.SourceRowCount != 1 || confirmContext.Err() == nil {
		t.Fatalf("linearized confirmation was reclassified after caller cancellation: result=%#v err=%v ctx=%v", result, err, confirmContext.Err())
	}
}

func TestImportCanceledConfirmRequestClearsStagedGeneration(t *testing.T) {
	workspace := "/synthetic/case-canceled-request"
	identity := importIdentityForTestV1(t, "principal-canceled-request")
	observer := &importObserverStubV1{observation: importObservationForTestV1(t, workspace, "case-canceled-request")}
	source := &mutableImportSourceV1{format: "csv", body: canonicalImportCSVForTestV1(t, 1, "CANCEL_REQUEST")}
	service := &ServiceV1{
		observer: observer, identity: identity, readImportSource: source.read,
		now: time.Now, random: bytes.NewReader(bytes.Repeat([]byte{0x63}, 64)),
		commitExact: func(context.Context, string, []byte, string, uint64) (StageResultV1, error) {
			t.Fatal("pre-canceled confirmation reached commit")
			return StageResultV1{}, nil
		},
	}
	staged, err := service.StageMainSelectedImportV1(context.Background(), StageInputV1{
		WorkspaceRoot: workspace, SourcePath: "/private/source.csv",
	})
	if err != nil {
		t.Fatal(err)
	}
	selector := staged.Items[0].Selector
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.ConfirmImportV1(canceled, selector); err == nil {
		t.Fatal("pre-canceled confirmation returned success")
	}
	if _, err := service.ImportStatusV1(context.Background(), selector); err == nil {
		t.Fatal("pre-canceled confirmation left its selector or bytes active")
	}
}

func TestImportStageCancellationBeforeGenerationActivationLeavesNoBytes(t *testing.T) {
	workspace := "/synthetic/case-stage-cancel"
	identity := importIdentityForTestV1(t, "principal-stage-cancel")
	observer := &importObserverStubV1{observation: importObservationForTestV1(t, workspace, "case-stage-cancel")}
	source := &mutableImportSourceV1{format: "csv", body: canonicalImportCSVForTestV1(t, 1, "STAGE_CANCEL")}
	ctx, cancel := context.WithCancel(context.Background())
	service := &ServiceV1{
		observer: observer, identity: identity, readImportSource: source.read,
		now: time.Now, random: cancelAfterRandomReadV1{cancel: cancel},
	}
	if _, err := service.StageMainSelectedImportV1(ctx, StageInputV1{
		WorkspaceRoot: workspace, SourcePath: "/private/source.csv",
	}); err == nil {
		t.Fatal("canceled staging returned success")
	}
	service.stagingMu.Lock()
	defer service.stagingMu.Unlock()
	if service.activeImport != nil {
		t.Fatal("canceled staging left an activatable generation")
	}
}

func TestImportMappingIsClosedAndConflictsFailConfirmation(t *testing.T) {
	columns := domainevidence.FundsTransactionCSVColumnsV1()
	body := canonicalImportCSVForTestV1(t, 1, "CANARY")
	reader := csv.NewReader(bytes.NewReader(body))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	records[0][1] = "card_number"
	var changed bytes.Buffer
	writer := csv.NewWriter(&changed)
	writer.UseCRLF = true
	_ = writer.WriteAll(records)
	parsed, err := parseImportSourceV1(context.Background(), "csv", changed.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	defer clearParsedImportV1(&parsed)
	if parsed.status != importStagingMappingInvalidV1 || parsed.items[0].columns[0].mappingStatus != "conflict" ||
		parsed.items[0].columns[1].mappingStatus != "conflict" {
		t.Fatalf("duplicate target was not typed as conflict: %#v", parsed.items[0].columns[:2])
	}
	records[0] = append(records[0], "unknown_column")
	records[1] = append(records[1], "value")
	changed.Reset()
	writer = csv.NewWriter(&changed)
	writer.UseCRLF = true
	_ = writer.WriteAll(records)
	parsedUnknown, err := parseImportSourceV1(context.Background(), "csv", changed.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	defer clearParsedImportV1(&parsedUnknown)
	if parsedUnknown.status != importStagingMappingInvalidV1 ||
		parsedUnknown.items[0].columns[len(columns)].mappingStatus != "unmapped" {
		t.Fatal("unknown source column was not typed as unmapped")
	}
}

func TestImportZIPSecurityMatrixFailsClosed(t *testing.T) {
	invalidNames := []string{
		"../escape.csv", "/absolute.csv", `C:/drive.csv`, `//server/share.csv`,
		`folder\member.csv`, "folder/./member.csv", "folder/../member.csv", ".", "..",
		"control\x00.csv", "control\n.csv", "e\u0301.csv",
	}
	for _, name := range invalidNames {
		if _, ok := validZIPMemberNameV1(name); ok {
			t.Fatalf("unsafe ZIP member name was accepted: %q", name)
		}
	}
	if _, ok := validZIPMemberNameV1("folder/member.csv"); !ok {
		t.Fatal("safe nested CSV member was rejected")
	}
	magicInOrdinaryMember := zipEntryForTestV1(t, "magic.csv", []byte("header\r\nPK\x06\x06PK\x06\x07\r\n"), 0)
	if containsZIP64StructureV1(magicInOrdinaryMember) {
		t.Fatal("ordinary member data was mistaken for an archive-level ZIP64 structure")
	}
	ordinaryMembers, err := readZIPMembersV1(context.Background(), magicInOrdinaryMember)
	if err != nil || len(ordinaryMembers) != 1 {
		t.Fatalf("ordinary member magic bytes did not reach the independent CSV policy: members=%d err=%v", len(ordinaryMembers), err)
	}
	clearImportMembersV1(ordinaryMembers)
	ordinaryCommentMagic := zipWithCentralCommentForTestV1(t, "PK\x06\x07"+strings.Repeat("A", 16))
	if containsZIP64StructureV1(ordinaryCommentMagic) {
		t.Fatal("ordinary central-file comment tail was mistaken for a ZIP64 locator")
	}
	commentMembers, err := readZIPMembersV1(context.Background(), ordinaryCommentMagic)
	if err != nil || len(commentMembers) != 1 {
		t.Fatalf("ordinary central-file comment magic was rejected: members=%d err=%v", len(commentMembers), err)
	}
	clearImportMembersV1(commentMembers)
	if _, err := readZIPMembersV1(context.Background(), zipNonUTF8NameForTestV1(t, "é.csv")); err == nil {
		t.Fatal("non-ASCII member without the UTF-8 flag was accepted")
	}
	asciiMembers, err := readZIPMembersV1(context.Background(), zipNonUTF8NameForTestV1(t, "ascii.csv"))
	if err != nil || len(asciiMembers) != 1 {
		t.Fatalf("ordinary ASCII member without bit 11 was rejected: members=%d err=%v", len(asciiMembers), err)
	}
	clearImportMembersV1(asciiMembers)

	memberOverflow := make(map[string][]byte, ImportStagingMaximumMembersV1+1)
	for index := 0; index < ImportStagingMaximumMembersV1+1; index++ {
		memberOverflow[fmt.Sprintf("member-%02d.csv", index)] = []byte("x")
	}
	memberOverflowZIP := zipImportForTestV1(t, memberOverflow)
	if err := preflightZIPArchiveV1(memberOverflowZIP); err == nil {
		t.Fatal("member count cap was not enforced by the pre-reader EOCD preflight")
	}
	baseZIP := zipEntryForTestV1(t, "one.csv", canonicalImportCSVForTestV1(t, 1, "A"), 0)
	if err := preflightZIPArchiveV1(mutateZIPLocalFlagsV1(t, baseZIP, 0x0040)); err == nil {
		t.Fatal("local and central flag mismatch passed structural preflight")
	}
	if !validZIPFlagsV1(0x0008, zip.Store) || !validZIPFlagsV1(0x080e, zip.Deflate) {
		t.Fatal("the fixed data-descriptor, UTF-8, or Deflate option envelope was rejected")
	}
	for _, test := range []struct {
		name string
		body []byte
	}{
		{name: "directory", body: zipEntryForTestV1(t, "folder/", nil, 0)},
		{name: "empty CSV member (Slice 5 non-empty policy)", body: zipEntryForTestV1(t, "empty.csv", nil, 0)},
		{name: "symlink", body: zipEntryForTestV1(t, "link.csv", []byte("target.csv"), zipUnixSymlinkModeV1|0o600)},
		{name: "special file", body: zipEntryForTestV1(t, "pipe.csv", []byte("value"), zipUnixNamedPipeModeV1|0o600)},
		{name: "nested archive", body: zipEntryForTestV1(t, "nested.zip", []byte("PK"), 0)},
		{name: "duplicate normalized member", body: zipDuplicateMemberForTestV1(t)},
		{name: "case collision", body: zipImportForTestV1(t, map[string][]byte{"A.csv": canonicalImportCSVForTestV1(t, 1, "A"), "a.csv": canonicalImportCSVForTestV1(t, 1, "B")})},
		{name: "Unicode case-fold collision", body: zipImportForTestV1(t, map[string][]byte{"STRASSE.csv": canonicalImportCSVForTestV1(t, 1, "A"), "Straße.csv": canonicalImportCSVForTestV1(t, 1, "B")})},
		{name: "member count", body: memberOverflowZIP},
		{name: "encrypted flag", body: mutateZIPFlagsV1(t, zipEntryForTestV1(t, "one.csv", canonicalImportCSVForTestV1(t, 1, "A"), 0), 1)},
		{name: "strong encryption flag", body: mutateZIPFlagsV1(t, baseZIP, 0x0040)},
		{name: "masked header flag", body: mutateZIPFlagsV1(t, baseZIP, 0x2000)},
		{name: "reserved flag", body: mutateZIPFlagsV1(t, baseZIP, 0x0010)},
		{name: "local central flag mismatch", body: mutateZIPLocalFlagsV1(t, baseZIP, 0x0040)},
		{name: "unsupported method", body: mutateZIPMethodV1(t, zipEntryForTestV1(t, "one.csv", canonicalImportCSVForTestV1(t, 1, "A"), 0), 99)},
		{name: "ZIP64 extra", body: zipWithExtraForTestV1(t, []byte{0x01, 0x00, 0x00, 0x00})},
		{name: "archive ZIP64 locator", body: mutateZIP64LocatorV1(t, zipEntryForTestV1(t, "one.csv", canonicalImportCSVForTestV1(t, 1, "A"), 0))},
		{name: "archive ZIP64 EOCD sentinel", body: mutateZIPEOCDSentinelV1(t, zipEntryForTestV1(t, "one.csv", canonicalImportCSVForTestV1(t, 1, "A"), 0))},
		{name: "local ZIP64 size sentinel", body: mutateZIPLocalSizeSentinelV1(t, baseZIP)},
		{name: "multi-disk EOCD", body: mutateZIPEOCDDiskV1(t, baseZIP, 1)},
		{name: "oversized central inventory", body: mutateZIPEOCDCentralSizeV1(t, baseZIP, uint32(len(baseZIP)))},
		{name: "member expanded bytes", body: mutateZIPUncompressedSizeV1(t, zipEntryForTestV1(t, "one.csv", canonicalImportCSVForTestV1(t, 1, "A"), 0), uint32(ImportStagingMaximumMemberBytesV1+1))},
		{name: "declared size mismatch", body: mutateZIPDeclaredSizeV1(t, zipEntryForTestV1(t, "one.csv", canonicalImportCSVForTestV1(t, 1, "A"), 0))},
		{name: "truncated central directory", body: func() []byte {
			value := zipImportForTestV1(t, map[string][]byte{"one.csv": canonicalImportCSVForTestV1(t, 1, "A")})
			return value[:len(value)-7]
		}()},
		{name: "CRC mismatch", body: corruptZIPStoredBodyV1(t)},
		{name: "per-member expansion ratio", body: zipImportForTestV1(t, map[string][]byte{
			"one.csv": bytes.Repeat([]byte("a"), int(ImportStagingRatioAllowanceBytesV1*2)),
		})},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := readZIPMembersV1(context.Background(), test.body); err == nil {
				t.Fatal("unsafe ZIP was accepted")
			}
		})
	}
	if !hasZIP64ExtraV1([]byte{0x01, 0x00, 0x00, 0x00}) ||
		!expansionRatioExceededV1(50*1024*1024, 1) ||
		expansionRatioExceededV1(1024, 1) {
		t.Fatal("ZIP64 or expansion limits drifted")
	}
	if _, ok := addImportExpandedBytesV1(ImportStagingMaximumExpandedBytesV1, 1); ok {
		t.Fatal("aggregate expanded-byte overflow was accepted")
	}
	if total, ok := addImportExpandedBytesV1(ImportStagingMaximumExpandedBytesV1-1, 1); !ok ||
		total != ImportStagingMaximumExpandedBytesV1 {
		t.Fatal("exact aggregate expanded-byte budget was rejected")
	}
}

func TestImportCSVResourceAndShapeLimitsFailClosed(t *testing.T) {
	canonical := domainevidence.FundsTransactionCSVColumnsV1()
	columns := make([]string, ImportStagingMaximumColumnsV1+1)
	row := make([]string, len(columns))
	for index := range columns {
		columns[index] = fmt.Sprintf("column_%d", index)
	}
	for name, body := range map[string][]byte{
		"column count": csvRowsForTestV1(t, columns, [][]string{row}),
		"cell bytes":   canonicalImportCSVForTestV1(t, 1, strings.Repeat("x", int(ImportStagingMaximumCellBytesV1)+1)),
		"header bytes": csvRowsForTestV1(t,
			[]string{strings.Repeat("a", 4096), strings.Repeat("b", 4096), strings.Repeat("c", 4096), strings.Repeat("d", 4096), strings.Repeat("e", 4096)},
			[][]string{{"1", "2", "3", "4", "5"}},
		),
		"width drift": csvRowsForTestV1(t,
			[]string{canonical[0].Header, canonical[4].Header, canonical[5].Header, canonical[7].Header, canonical[14].Header},
			[][]string{{"card", "2026-08-22 01:02:03", "1.00", "进"}},
		),
		"row parse failure": []byte("one,two\r\n\"unterminated"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseImportSourceV1(context.Background(), "csv", body); err == nil {
				t.Fatal("over-limit or malformed CSV was accepted")
			}
		})
	}

	minimalHeader := []string{
		canonical[0].Header, canonical[4].Header, canonical[5].Header,
		canonical[7].Header, canonical[14].Header,
	}
	rows := make([][]string, ImportStagingMaximumRowsV1+1)
	for index := range rows {
		rows[index] = []string{"card", "2026-08-22 01:02:03", "1.00", "进", "CNY"}
	}
	overflowRows := csvRowsForTestV1(t, minimalHeader, rows)
	if _, err := parseImportSourceV1(context.Background(), "csv", overflowRows); err == nil {
		t.Fatal("aggregate row limit overflow was accepted")
	}
	clear(overflowRows)
}

func TestImportEncodingAndFiftyThousandRowBudget(t *testing.T) {
	utf8BOM := append([]byte{0xef, 0xbb, 0xbf}, canonicalImportCSVForTestV1(t, 1, "BOM")...)
	parsed, err := parseImportSourceV1(context.Background(), "csv", utf8BOM)
	if err != nil || parsed.rowCount != 1 {
		t.Fatalf("UTF-8 BOM policy failed: rows=%d err=%v", parsed.rowCount, err)
	}
	clearParsedImportV1(&parsed)
	gbSource := canonicalImportCSVForTestV1(t, 1, "中文值")
	gb, _, err := transform.Bytes(simplifiedchinese.GB18030.NewEncoder(), gbSource)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = parseImportSourceV1(context.Background(), "csv", gb)
	clear(gb)
	if err != nil || parsed.rowCount != 1 {
		t.Fatalf("GB18030 policy failed: rows=%d err=%v", parsed.rowCount, err)
	}
	clearParsedImportV1(&parsed)
	if _, err := decodeImportTextV1([]byte{0xff, 0xff, 0xff}); err == nil {
		t.Fatal("invalid encoding was accepted")
	}
	if _, err := decodeImportTextV1([]byte{0xc2, 0xa9}); err == nil {
		t.Fatal("UTF-8/GB18030 ambiguous bytes were accepted without a BOM")
	}
	if decoded, err := decodeImportTextV1([]byte{0xef, 0xbb, 0xbf, 0xc2, 0xa9}); err != nil || string(decoded) != "©" {
		t.Fatalf("explicit UTF-8 BOM did not disambiguate input: decoded=%q err=%v", string(decoded), err)
	}
	fiftyThousand := canonicalImportCSVForTestV1(t, 50_000, "BUDGET")
	parsed, err = parseImportSourceV1(context.Background(), "csv", fiftyThousand)
	clear(fiftyThousand)
	if err != nil || parsed.rowCount != 50_000 {
		t.Fatalf("50k-row supported budget failed: rows=%d err=%v", parsed.rowCount, err)
	}
	clearParsedImportV1(&parsed)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := parseImportSourceV1(canceled, "csv", canonicalImportCSVForTestV1(t, 1, "CANCEL")); err == nil {
		t.Fatal("canceled parse continued")
	}
}

func canonicalImportCSVForTestV1(t *testing.T, rowCount int, canary string) []byte {
	t.Helper()
	columns := domainevidence.FundsTransactionCSVColumnsV1()
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	writer.UseCRLF = true
	header := make([]string, len(columns))
	for index := range columns {
		header[index] = columns[index].Header
	}
	if err := writer.Write(header); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < rowCount; index++ {
		row := make([]string, len(columns))
		row[0] = canary
		row[4] = "2026-08-22 01:02:03"
		row[5] = "123.45"
		row[6] = "456.78"
		row[7] = "进"
		row[14] = "CNY"
		if err := writer.Write(row); err != nil {
			t.Fatal(err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func csvRowsForTestV1(t *testing.T, header []string, rows [][]string) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	writer.UseCRLF = true
	if err := writer.Write(header); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			t.Fatal(err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func zipImportForTestV1(t *testing.T, members map[string][]byte) []byte {
	t.Helper()
	keys := make([]string, 0, len(members))
	for name := range members {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, name := range keys {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		setZIPUnixModeForTestV1(header, zipUnixRegularModeV1|0o600)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(members[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func zipEntryForTestV1(t *testing.T, name string, body []byte, mode uint32) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	header := &zip.FileHeader{Name: name, Method: zip.Store}
	if mode != 0 {
		setZIPUnixModeForTestV1(header, mode)
	} else {
		if strings.HasSuffix(name, "/") {
			setZIPUnixModeForTestV1(header, zipUnixDirectoryModeV1|0o700)
		} else {
			setZIPUnixModeForTestV1(header, zipUnixRegularModeV1|0o600)
		}
	}
	entry, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write(body)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func zipWithExtraForTestV1(t *testing.T, extra []byte) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	header := &zip.FileHeader{Name: "one.csv", Method: zip.Store, Extra: append([]byte(nil), extra...)}
	setZIPUnixModeForTestV1(header, zipUnixRegularModeV1|0o600)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write(canonicalImportCSVForTestV1(t, 1, "A"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

const (
	zipUnixNamedPipeModeV1 = uint32(0o010000)
	zipUnixDirectoryModeV1 = uint32(0o040000)
	zipUnixRegularModeV1   = uint32(0o100000)
	zipUnixSymlinkModeV1   = uint32(0o120000)
)

func setZIPUnixModeForTestV1(header *zip.FileHeader, mode uint32) {
	// ZIP stores the originating system in the high byte of CreatorVersion and
	// the stable Unix mode bits in the high word of ExternalAttrs. Constructing
	// those archive fields directly keeps this app-layer fixture OS-independent.
	header.CreatorVersion = header.CreatorVersion&0x00ff | 3<<8
	header.ExternalAttrs = mode << 16
	if mode&0o170000 == zipUnixDirectoryModeV1 {
		header.ExternalAttrs |= 0x10
	}
}

func zipWithCentralCommentForTestV1(t *testing.T, comment string) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	header := &zip.FileHeader{Name: "one.csv", Comment: comment, Method: zip.Store}
	header.SetMode(0o600)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write(canonicalImportCSVForTestV1(t, 1, "COMMENT_MAGIC"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func zipNonUTF8NameForTestV1(t *testing.T, name string) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	header := &zip.FileHeader{Name: name, Method: zip.Store, NonUTF8: true}
	header.SetMode(0o600)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write(canonicalImportCSVForTestV1(t, 1, "NAME_ENCODING"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func zipDuplicateMemberForTestV1(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, body := range [][]byte{
		canonicalImportCSVForTestV1(t, 1, "A"),
		canonicalImportCSVForTestV1(t, 1, "B"),
	} {
		header := &zip.FileHeader{Name: "duplicate.csv", Method: zip.Store}
		header.SetMode(0o600)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func mutateZIPFlagsV1(t *testing.T, body []byte, flags uint16) []byte {
	t.Helper()
	mutated := append([]byte(nil), body...)
	local := bytes.Index(mutated, []byte("PK\x03\x04"))
	central := bytes.Index(mutated, []byte("PK\x01\x02"))
	if local < 0 || central < 0 {
		t.Fatal("ZIP fixture headers unavailable")
	}
	binary.LittleEndian.PutUint16(mutated[local+6:local+8], flags)
	binary.LittleEndian.PutUint16(mutated[central+8:central+10], flags)
	return mutated
}

func mutateZIPLocalFlagsV1(t *testing.T, body []byte, flags uint16) []byte {
	t.Helper()
	mutated := append([]byte(nil), body...)
	local := bytes.Index(mutated, []byte("PK\x03\x04"))
	if local < 0 {
		t.Fatal("ZIP local header unavailable")
	}
	binary.LittleEndian.PutUint16(mutated[local+6:local+8], flags)
	return mutated
}

func mutateZIPMethodV1(t *testing.T, body []byte, method uint16) []byte {
	t.Helper()
	mutated := append([]byte(nil), body...)
	local := bytes.Index(mutated, []byte("PK\x03\x04"))
	central := bytes.Index(mutated, []byte("PK\x01\x02"))
	if local < 0 || central < 0 {
		t.Fatal("ZIP fixture headers unavailable")
	}
	binary.LittleEndian.PutUint16(mutated[local+8:local+10], method)
	binary.LittleEndian.PutUint16(mutated[central+10:central+12], method)
	return mutated
}

func mutateZIPDeclaredSizeV1(t *testing.T, body []byte) []byte {
	t.Helper()
	mutated := append([]byte(nil), body...)
	central := bytes.Index(mutated, []byte("PK\x01\x02"))
	if central < 0 {
		t.Fatal("ZIP central header unavailable")
	}
	declared := binary.LittleEndian.Uint32(mutated[central+24 : central+28])
	binary.LittleEndian.PutUint32(mutated[central+24:central+28], declared+1)
	return mutated
}

func mutateZIPUncompressedSizeV1(t *testing.T, body []byte, size uint32) []byte {
	t.Helper()
	mutated := append([]byte(nil), body...)
	central := bytes.Index(mutated, []byte("PK\x01\x02"))
	if central < 0 {
		t.Fatal("ZIP central header unavailable")
	}
	binary.LittleEndian.PutUint32(mutated[central+24:central+28], size)
	return mutated
}

func mutateZIP64LocatorV1(t *testing.T, body []byte) []byte {
	t.Helper()
	eocd := bytes.LastIndex(body, []byte("PK\x05\x06"))
	if eocd < 0 {
		t.Fatal("ZIP end record unavailable")
	}
	zip64End := make([]byte, 56)
	copy(zip64End, []byte("PK\x06\x06"))
	binary.LittleEndian.PutUint64(zip64End[4:12], 44)
	locator := make([]byte, 20)
	copy(locator, []byte("PK\x06\x07"))
	binary.LittleEndian.PutUint64(locator[8:16], uint64(eocd))
	binary.LittleEndian.PutUint32(locator[16:20], 1)
	mutated := make([]byte, 0, len(body)+len(zip64End)+len(locator))
	mutated = append(mutated, body[:eocd]...)
	mutated = append(mutated, zip64End...)
	mutated = append(mutated, locator...)
	mutated = append(mutated, body[eocd:]...)
	return mutated
}

func mutateZIPEOCDSentinelV1(t *testing.T, body []byte) []byte {
	t.Helper()
	mutated := append([]byte(nil), body...)
	eocd := findZIPEndOfCentralDirectoryV1(mutated)
	if eocd < 0 {
		t.Fatal("ZIP end record unavailable")
	}
	binary.LittleEndian.PutUint16(mutated[eocd+10:eocd+12], 0xffff)
	return mutated
}

func mutateZIPLocalSizeSentinelV1(t *testing.T, body []byte) []byte {
	t.Helper()
	mutated := append([]byte(nil), body...)
	local := bytes.Index(mutated, []byte("PK\x03\x04"))
	if local < 0 {
		t.Fatal("ZIP local header unavailable")
	}
	binary.LittleEndian.PutUint32(mutated[local+18:local+22], 0xffffffff)
	return mutated
}

func mutateZIPEOCDDiskV1(t *testing.T, body []byte, disk uint16) []byte {
	t.Helper()
	mutated := append([]byte(nil), body...)
	eocd := findZIPEndOfCentralDirectoryV1(mutated)
	if eocd < 0 {
		t.Fatal("ZIP end record unavailable")
	}
	binary.LittleEndian.PutUint16(mutated[eocd+4:eocd+6], disk)
	return mutated
}

func mutateZIPEOCDCentralSizeV1(t *testing.T, body []byte, size uint32) []byte {
	t.Helper()
	mutated := append([]byte(nil), body...)
	eocd := findZIPEndOfCentralDirectoryV1(mutated)
	if eocd < 0 {
		t.Fatal("ZIP end record unavailable")
	}
	binary.LittleEndian.PutUint32(mutated[eocd+12:eocd+16], size)
	return mutated
}

func corruptZIPStoredBodyV1(t *testing.T) []byte {
	t.Helper()
	body := canonicalImportCSVForTestV1(t, 1, "CRC_CANARY")
	zipBody := zipEntryForTestV1(t, "one.csv", body, 0)
	offset := bytes.Index(zipBody, []byte("CRC_CANARY"))
	if offset < 0 {
		t.Fatal("stored ZIP fixture payload not found")
	}
	zipBody[offset] ^= 0xff
	return zipBody
}

func importObservationForTestV1(t *testing.T, workspace, caseID string) domainsecurity.CaseBindingObservationV1 {
	t.Helper()
	observation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: workspace, State: domainsecurity.CaseBindingStateValid, CaseID: caseID,
		BindingSHA256:   domainsecurity.SHA256Hex([]byte("binding-" + caseID)),
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("context-" + caseID)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func importIdentityForTestV1(t *testing.T, seed string) *importIdentityStubV1 {
	t.Helper()
	principal, err := domainidentity.NewPrincipalV1(
		domainsecurity.SHA256Hex([]byte("installation-"+seed)), "tenant", "user",
	)
	if err != nil {
		t.Fatal(err)
	}
	return &importIdentityStubV1{principal: principal, valid: true}
}
