//go:build darwin && !analytix_prod

package nativecomponentrunner

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	processauthority "analytix.local/runtime-go/internal/adapters/outbound/processauthority"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestRunnerTransactionSourceRowRejectsUnboundRequestBeforeExactCopy(t *testing.T) {
	snapshotBody := []byte("exact immutable source-row rejection fixture")
	runner, registry, opener, stagingRoot := newAccountFlowDarwinRunnerTestFixture(
		t,
		snapshotBody,
		"valid",
		false,
	)
	source := &exactBytesReadLease{body: snapshotBody}
	result, err := runner.TransactionSourceRowPage(
		context.Background(),
		domainnative.TransactionSourceRowPageArgumentsV1{},
		domainfundsquerysource.ImmutableSnapshotObjectV1{},
		source,
	)
	if !errors.Is(err, ErrRequestInvalid) || result.RowCountV1() != 0 ||
		source.Calls() != 0 || registry.Acquisitions() != 0 || len(opener.Events()) != 0 {
		t.Fatalf(
			"unbound source-row request crossed admission: result=%s err=%v copies=%d acquisitions=%d events=%#v",
			result.String(), err, source.Calls(), registry.Acquisitions(), opener.Events(),
		)
	}
	assertRunnerStageEmpty(t, stagingRoot)
}

func TestRunnerTransactionSourceRowOwnsDeadlineExactFD3AndOneShotClose(t *testing.T) {
	snapshotBody := []byte("exact immutable source-row deadline fixture")
	runner, registry, opener, stagingRoot := newAccountFlowDarwinRunnerTestFixture(
		t,
		snapshotBody,
		"valid",
		false,
	)
	arguments, installedSnapshot := transactionSourceRowRunnerArgumentsV1(
		t,
		snapshotBody,
		uint64(len(snapshotBody)),
	)
	source := &transactionSourceRowDeadlineLeaseV1{
		delegate: exactBytesReadLease{body: append([]byte(nil), snapshotBody...)},
	}
	started := time.Now()
	result, err := runner.TransactionSourceRowPage(
		context.Background(),
		arguments,
		installedSnapshot,
		source,
	)
	deadline, observed := source.Deadline()
	if !errors.Is(err, ErrProtocol) || result.RowCountV1() != 0 || !observed ||
		deadline.Before(started.Add(transactionSourceRowMinimumObservedDeadlineV1)) ||
		deadline.After(started.Add(domainnative.TransactionSourceRowMaximumDurationV1+time.Second)) {
		t.Fatalf("runner-owned deadline/result invalid: result=%s err=%v observed=%t deadline=%v", result.String(), err, observed, deadline)
	}
	if source.Calls() != 1 || registry.Acquisitions() != 1 ||
		strings.Join(opener.Events(), ",") != "open:flow:1,roundtrip:flow:1,close:flow:1" ||
		runner.session != nil || runner.sessionBinding != "" {
		t.Fatalf(
			"one-shot FD3 lifecycle invalid: copies=%d acquisitions=%d events=%#v session=%T binding=%q",
			source.Calls(), registry.Acquisitions(), opener.Events(), runner.session, runner.sessionBinding,
		)
	}
	assertRunnerStageEmpty(t, stagingRoot)
}

func TestRunnerTransactionSourceRowRejectsSnapshotOverFixedByteCapBeforeCopy(t *testing.T) {
	snapshotBody := []byte("oversized source-row admission fixture")
	runner, registry, opener, stagingRoot := newAccountFlowDarwinRunnerTestFixture(
		t,
		snapshotBody,
		"valid",
		false,
	)
	arguments, installedSnapshot := transactionSourceRowRunnerArgumentsV1(
		t,
		snapshotBody,
		uint64(len(snapshotBody)),
	)
	oversizedSnapshot, err := domainfundsquerysource.NewImmutableSnapshotObjectV1(
		installedSnapshot.CaseID,
		installedSnapshot.DuckDBSHA256,
		domainnative.TransactionSourceRowMaximumSnapshotBytesV1+1,
	)
	if err != nil {
		t.Fatal(err)
	}
	source := &exactBytesReadLease{body: snapshotBody}
	result, err := runner.TransactionSourceRowPage(
		context.Background(), arguments, oversizedSnapshot, source,
	)
	if !errors.Is(err, ErrRequestInvalid) || result.RowCountV1() != 0 ||
		source.Calls() != 0 || registry.Acquisitions() != 0 || len(opener.Events()) != 0 {
		t.Fatalf(
			"oversized source crossed fixed admission: result=%s err=%v copies=%d acquisitions=%d events=%#v",
			result.String(), err, source.Calls(), registry.Acquisitions(), opener.Events(),
		)
	}
	assertRunnerStageEmpty(t, stagingRoot)
}

func TestRunnerTransactionSourceRowCancellationClosesOneShotChild(t *testing.T) {
	snapshotBody := []byte("source-row cancellation fixture")
	runner, registry, opener, stagingRoot := newAccountFlowDarwinRunnerTestFixture(
		t,
		snapshotBody,
		"valid",
		false,
	)
	baseOpener := runner.sessionOpener
	entered := make(chan struct{})
	var blocked *transactionSourceRowBlockingSessionV1
	runner.sessionOpener = func(ctx context.Context, request sessionOpenRequest) (runnerSession, error) {
		session, err := baseOpener(ctx, request)
		if err != nil {
			return nil, err
		}
		blocked = &transactionSourceRowBlockingSessionV1{runnerSession: session, entered: entered}
		return blocked, nil
	}
	arguments, installedSnapshot := transactionSourceRowRunnerArgumentsV1(
		t,
		snapshotBody,
		uint64(len(snapshotBody)),
	)
	source := &exactBytesReadLease{body: snapshotBody}
	ctx, cancel := context.WithCancel(context.Background())
	type outcomeV1 struct {
		result domainnative.TransactionSourceRowPageV1
		err    error
	}
	outcome := make(chan outcomeV1, 1)
	go func() {
		result, err := runner.TransactionSourceRowPage(ctx, arguments, installedSnapshot, source)
		outcome <- outcomeV1{result: result, err: err}
	}()
	select {
	case <-entered:
		cancel()
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("source-row child did not enter the cancellable round trip")
	}
	completed := <-outcome
	if !errors.Is(completed.err, context.Canceled) || completed.result.RowCountV1() != 0 ||
		blocked == nil || !blocked.RoundTripStarted() || source.Calls() != 1 ||
		registry.Acquisitions() != 1 || strings.Join(opener.Events(), ",") != "open:flow:1,close:flow:1" {
		t.Fatalf(
			"cancellation did not close child: result=%s err=%v blocked=%#v copies=%d acquisitions=%d events=%#v",
			completed.result.String(), completed.err, blocked, source.Calls(), registry.Acquisitions(), opener.Events(),
		)
	}
	assertRunnerStageEmpty(t, stagingRoot)
}

const transactionSourceRowMinimumObservedDeadlineV1 = 20 * time.Second

type transactionSourceRowDeadlineLeaseV1 struct {
	mu       sync.Mutex
	delegate exactBytesReadLease
	deadline time.Time
	observed bool
}

type transactionSourceRowBlockingSessionV1 struct {
	runnerSession
	mu      sync.Mutex
	started bool
	entered chan struct{}
}

func (session *transactionSourceRowBlockingSessionV1) RoundTrip(
	ctx context.Context,
	request []byte,
	limit int,
) ([]byte, error) {
	session.mu.Lock()
	session.started = true
	session.mu.Unlock()
	if ctx == nil || len(request) == 0 || limit != transactionSourceRowResponseFrameLimit {
		return nil, processauthority.ErrProtocol
	}
	close(session.entered)
	<-ctx.Done()
	return nil, processauthority.ErrCanceled
}

func (session *transactionSourceRowBlockingSessionV1) RoundTripStarted() bool {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.started
}

func (lease *transactionSourceRowDeadlineLeaseV1) CopyExactTo(
	ctx context.Context,
	destination *os.File,
) error {
	deadline, observed := ctx.Deadline()
	lease.mu.Lock()
	lease.deadline = deadline
	lease.observed = observed
	lease.mu.Unlock()
	if !observed {
		return errors.New("source-row exact copy received no runner deadline")
	}
	return lease.delegate.CopyExactTo(ctx, destination)
}

func (lease *transactionSourceRowDeadlineLeaseV1) Deadline() (time.Time, bool) {
	lease.mu.Lock()
	defer lease.mu.Unlock()
	return lease.deadline, lease.observed
}

func (lease *transactionSourceRowDeadlineLeaseV1) Calls() int {
	return lease.delegate.Calls()
}

func transactionSourceRowRunnerArgumentsV1(
	t *testing.T,
	snapshotBody []byte,
	duckDBByteLength uint64,
) (domainnative.TransactionSourceRowPageArgumentsV1, domainfundsquerysource.ImmutableSnapshotObjectV1) {
	t.Helper()
	const caseID = "case-source-row-runner"
	binding, err := domainsecurity.NewDatasetSnapshotBindingKeyV1(domainsecurity.DatasetSnapshotBindingKeyInputV1{
		TenantID:                 domainsecurity.LocalTenantID,
		UserID:                   domainsecurity.LocalUserID,
		WorkspaceRealPath:        "/cases/case-source-row-runner",
		CaseID:                   caseID,
		CaseBindingHash:          strings.Repeat("5", 64),
		BindingObservationDigest: strings.Repeat("6", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	rawArtifactManifestSHA256 := strings.Repeat("a", 64)
	producer, err := domainsecurity.NewFundsProducerContentManifestV1(domainsecurity.FundsProducerContentManifestInputV1{
		CaseID: caseID, SourceRevision: 7, RawManifestSHA256: rawArtifactManifestSHA256,
		NormalizedContentSHA256: strings.Repeat("b", 64), DetailContentSHA256: strings.Repeat("c", 64),
		AggregateContentSHA256: strings.Repeat("d", 64), KeywordContentSHA256: strings.Repeat("e", 64),
		AccountContentSHA256: strings.Repeat("f", 64), NormalizedRowCount: 2, AcceptedRowCount: 2,
		DetailRowCount: 2, AggregateRowCount: 1, KeywordRowCount: 1, AccountRowCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	producerBody, err := domainsecurity.FundsProducerContentManifestV1Bytes(producer)
	if err != nil {
		t.Fatal(err)
	}
	rawSources := domainsecurity.FundsRawSourceManifestV1{{
		CleanedStatus: "done", FileID: "import-private-a", RowsImportedNorm: 2,
		SHA256: strings.Repeat("7", 64), Status: "已完成",
	}}
	rawSourceBody, err := json.Marshal(rawSources)
	if err != nil {
		t.Fatal(err)
	}
	materializationBody, err := json.Marshal(domainsecurity.FundsMaterializationResultV1{
		CaseID: caseID, RowCount: producer.AggregateRowCount,
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
	})
	if err != nil {
		t.Fatal(err)
	}
	materialization, err := domainsecurity.ParseFundsMaterializationResultV1(materializationBody)
	if err != nil {
		t.Fatal(err)
	}
	duckDBHash := sha256.Sum256(snapshotBody)
	duckDBSHA256 := hex.EncodeToString(duckDBHash[:])
	installedSnapshot, err := domainfundsquerysource.NewImmutableSnapshotObjectV1(
		caseID,
		duckDBSHA256,
		duckDBByteLength,
	)
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := domainnative.NewTransactionSourceRowPageArgumentsV1(domainnative.TransactionSourceRowPageArgumentsInputV1{
		Binding: binding, ParsedGenerationIdentitySHA256: strings.Repeat("9", 64),
		Materialization: materialization, InstalledSnapshot: installedSnapshot, MaxRows: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return arguments, installedSnapshot
}
