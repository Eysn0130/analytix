//go:build darwin && !analytix_prod

package nativecomponentrunner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
)

func TestRunnerResolveAccountIngressUsesExactOneShotFD3SnapshotAndSafeResult(t *testing.T) {
	snapshotBody := []byte("exact callback-scoped immutable DuckDB account-ingress fixture")
	runner, registry, opener, stagingRoot := newAccountFlowDarwinRunnerTestFixture(
		t,
		snapshotBody,
		"valid",
		false,
	)
	health := darwinRunnerRequest(t, "case-account-ingress", 11)
	descriptor := darwinRunnerAccountIngressDescriptorV1(t, health, snapshotBody)
	first, err := domainnative.NewResolveAccountIngressCandidateInputV1(3, "6222021234567890123")
	if err != nil {
		t.Fatal(err)
	}
	second, err := domainnative.NewResolveAccountIngressCandidateInputV1(8, "6217009876543210987")
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := domainnative.NewResolveAccountIngressArgumentsV1(
		domainnative.ResolveAccountIngressArgumentsInputV1{
			CaseID:                               health.Context.CaseID,
			DatasetSnapshotID:                    health.Context.DatasetSnapshotID,
			ContextEpoch:                         health.Context.ContextEpoch,
			ContextDigest:                        health.Context.ContextDigest,
			CaseBindingHash:                      health.Context.CaseBindingHash,
			ExpectedProducerContentID:            descriptor.FundsProducerContentID,
			ExpectedProducerManifestSHA256:       descriptor.FundsProducerContentManifestSHA256,
			ExpectedDuckDBContentSnapshotDigest:  descriptor.DuckDBContentSnapshotDigest,
			ExpectedDuckDBSnapshotManifestSHA256: descriptor.DuckDBSnapshotManifestSHA256,
			ExpectedMaterializationIdentity:      descriptor.MaterializationIdentity,
			Candidates:                           []domainnative.ResolveAccountIngressCandidateInputV1{first, second},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	source := &exactBytesReadLease{body: append([]byte(nil), snapshotBody...)}
	result, err := runner.ResolveAccountIngress(
		context.Background(),
		health.Context,
		arguments,
		descriptor,
		source,
	)
	if err != nil {
		t.Fatalf("resolve account ingress: %v", err)
	}
	if len(result.Resolutions) != 2 || result.Resolutions[0].Ordinal != 3 ||
		result.Resolutions[0].Disposition != domainnative.AccountIngressResolutionDispositionResolvedV1 ||
		result.Resolutions[1].Ordinal != 8 ||
		result.Resolutions[1].Disposition != domainnative.AccountIngressResolutionDispositionNotFoundV1 ||
		source.Calls() != 1 || runner.session != nil || runner.sessionBinding != "" {
		t.Fatalf("unexpected one-shot result=%#v copies=%d session=%T binding=%q", result, source.Calls(), runner.session, runner.sessionBinding)
	}
	if registry.Acquisitions() != 1 || !reflect.DeepEqual(
		opener.Events(),
		[]string{"open:flow:1", "roundtrip:flow:1", "close:flow:1"},
	) {
		t.Fatalf("one-shot lifecycle acquisitions=%d events=%#v", registry.Acquisitions(), opener.Events())
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"6222021234567890123", "6217009876543210987"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("safe native result reflected complete candidate %q", private)
		}
	}
	assertRunnerStageEmpty(t, stagingRoot)
}

func TestRunnerResolveAccountIngressDeadlineCancelsBlockedExactCopyAndSettlesStage(t *testing.T) {
	snapshotBody := []byte("exact immutable DuckDB blocked account-ingress fixture")
	runner, registry, opener, stagingRoot := newAccountFlowDarwinRunnerTestFixture(
		t,
		snapshotBody,
		"valid",
		false,
	)
	health := darwinRunnerRequest(t, "case-account-ingress-deadline", 13)
	descriptor := darwinRunnerAccountIngressDescriptorV1(t, health, snapshotBody)
	candidate, err := domainnative.NewResolveAccountIngressCandidateInputV1(1, "6222021234567890123")
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := domainnative.NewResolveAccountIngressArgumentsV1(
		domainnative.ResolveAccountIngressArgumentsInputV1{
			CaseID:                               health.Context.CaseID,
			DatasetSnapshotID:                    health.Context.DatasetSnapshotID,
			ContextEpoch:                         health.Context.ContextEpoch,
			ContextDigest:                        health.Context.ContextDigest,
			CaseBindingHash:                      health.Context.CaseBindingHash,
			ExpectedProducerContentID:            descriptor.FundsProducerContentID,
			ExpectedProducerManifestSHA256:       descriptor.FundsProducerContentManifestSHA256,
			ExpectedDuckDBContentSnapshotDigest:  descriptor.DuckDBContentSnapshotDigest,
			ExpectedDuckDBSnapshotManifestSHA256: descriptor.DuckDBSnapshotManifestSHA256,
			ExpectedMaterializationIdentity:      descriptor.MaterializationIdentity,
			Candidates:                           []domainnative.ResolveAccountIngressCandidateInputV1{candidate},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	source := &deadlineBlockingExactReadLeaseV1{entered: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	result, err := runner.ResolveAccountIngress(
		ctx,
		health.Context,
		arguments,
		descriptor,
		source,
	)
	if !errors.Is(err, context.DeadlineExceeded) ||
		!reflect.DeepEqual(result, domainnative.ResolveAccountIngressResultV1{}) {
		t.Fatalf("blocked exact copy result=%#v err=%v", result, err)
	}
	if source.Calls() != 1 || registry.Acquisitions() != 0 || len(opener.Events()) != 0 {
		t.Fatalf(
			"deadline crossed process admission: copies=%d acquisitions=%d events=%#v",
			source.Calls(),
			registry.Acquisitions(),
			opener.Events(),
		)
	}
	assertRunnerStageEmpty(t, stagingRoot)
}

func TestRunnerResolveAccountIngressRejectsProfileDriftBeforeSourceCopy(t *testing.T) {
	snapshotBody := []byte("exact immutable DuckDB account-ingress profile fixture")
	runner, registry, opener, stagingRoot := newAccountFlowDarwinRunnerTestFixture(
		t,
		snapshotBody,
		"valid",
		false,
	)
	health := darwinRunnerRequest(t, "case-account-ingress-profile", 12)
	descriptor := darwinRunnerAccountIngressDescriptorV1(t, health, snapshotBody)
	candidate, err := domainnative.NewResolveAccountIngressCandidateInputV1(1, "6222021234567890123")
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := domainnative.NewResolveAccountIngressArgumentsV1(
		domainnative.ResolveAccountIngressArgumentsInputV1{
			CaseID:                               health.Context.CaseID,
			DatasetSnapshotID:                    health.Context.DatasetSnapshotID,
			ContextEpoch:                         health.Context.ContextEpoch,
			ContextDigest:                        health.Context.ContextDigest,
			CaseBindingHash:                      health.Context.CaseBindingHash,
			ExpectedProducerContentID:            descriptor.FundsProducerContentID,
			ExpectedProducerManifestSHA256:       descriptor.FundsProducerContentManifestSHA256,
			ExpectedDuckDBContentSnapshotDigest:  descriptor.DuckDBContentSnapshotDigest,
			ExpectedDuckDBSnapshotManifestSHA256: descriptor.DuckDBSnapshotManifestSHA256,
			ExpectedMaterializationIdentity:      descriptor.MaterializationIdentity,
			Candidates:                           []domainnative.ResolveAccountIngressCandidateInputV1{candidate},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	descriptor = darwinRunnerAccountIngressDescriptorWithProfileV1(
		t,
		health,
		snapshotBody,
		domainfundsquerysource.FixedAccountFlowQueryProfileDigestV1(),
	)
	source := &exactBytesReadLease{body: snapshotBody}
	result, err := runner.ResolveAccountIngress(
		context.Background(), health.Context, arguments, descriptor, source,
	)
	if err == nil || !reflect.DeepEqual(result, domainnative.ResolveAccountIngressResultV1{}) ||
		source.Calls() != 0 || registry.Acquisitions() != 0 || len(opener.Events()) != 0 {
		t.Fatalf("account-flow-only profile reached ingress source: result=%#v err=%v copies=%d acquisitions=%d events=%#v", result, err, source.Calls(), registry.Acquisitions(), opener.Events())
	}
	assertRunnerStageEmpty(t, stagingRoot)
}

type runnerAccountIngressCandidateV1 struct {
	Ordinal             uint32 `json:"ordinal"`
	NormalizedCandidate string `json:"normalizedCandidate"`
}

type deadlineBlockingExactReadLeaseV1 struct {
	mu      sync.Mutex
	entered chan struct{}
	calls   int
}

func (lease *deadlineBlockingExactReadLeaseV1) CopyExactTo(
	ctx context.Context,
	destination *os.File,
) error {
	lease.mu.Lock()
	lease.calls++
	calls := lease.calls
	lease.mu.Unlock()
	if calls != 1 || ctx == nil || destination == nil {
		return errors.New("invalid blocked exact-source copy")
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		return errors.New("exact-source copy did not receive the run deadline")
	}
	if _, err := destination.WriteAt([]byte("private-prefix"), 0); err != nil {
		return err
	}
	close(lease.entered)
	<-ctx.Done()
	return ctx.Err()
}

func (lease *deadlineBlockingExactReadLeaseV1) Calls() int {
	lease.mu.Lock()
	defer lease.mu.Unlock()
	return lease.calls
}

type runnerAccountIngressArgumentsV1 struct {
	CaseID                         string                            `json:"caseId"`
	DatasetSnapshotID              string                            `json:"datasetSnapshotId"`
	ContextEpoch                   uint64                            `json:"contextEpoch"`
	ContextDigest                  string                            `json:"contextDigest"`
	CaseBindingHash                string                            `json:"caseBindingHash"`
	ExpectedProducerContentID      string                            `json:"expectedProducerContentId"`
	ExpectedProducerManifestSHA256 string                            `json:"expectedProducerManifestSha256"`
	Candidates                     []runnerAccountIngressCandidateV1 `json:"candidates"`
}

func runnerAccountIngressSuccessDataV1(arguments runnerAccountIngressArgumentsV1) ([]byte, error) {
	resolutions := make([]domainnative.AccountIngressResolutionV1, len(arguments.Candidates))
	for index, candidate := range arguments.Candidates {
		resolutions[index] = domainnative.AccountIngressResolutionV1{
			Ordinal:     candidate.Ordinal,
			Disposition: domainnative.AccountIngressResolutionDispositionNotFoundV1,
		}
	}
	if len(resolutions) > 0 {
		resolutions[0] = domainnative.AccountIngressResolutionV1{
			Ordinal:         resolutions[0].Ordinal,
			Disposition:     domainnative.AccountIngressResolutionDispositionResolvedV1,
			EntityType:      "bank_account_number",
			BankInstitution: "中国银行",
			AccountType:     "储蓄账户",
		}
	}
	return json.Marshal(domainnative.ResolveAccountIngressResultV1{
		SchemaVersion: 1,
		Contract:      domainnative.AccountIngressResolutionResultContractV1,
		Resolutions:   resolutions,
		Provenance: domainnative.AccountIngressResolutionProvenanceV1{
			DatasetSnapshotID:              arguments.DatasetSnapshotID,
			ContextEpoch:                   arguments.ContextEpoch,
			ContextDigest:                  arguments.ContextDigest,
			CaseBindingHash:                arguments.CaseBindingHash,
			ExpectedProducerContentID:      arguments.ExpectedProducerContentID,
			ExpectedProducerManifestSHA256: arguments.ExpectedProducerManifestSHA256,
			DuckDBContentSnapshotDigest:    strings.Repeat("b", 64),
			DuckDBSnapshotManifestSHA256:   strings.Repeat("c", 64),
			MaterializationIdentity:        "txn_daily_snapshot:v12:" + strings.Repeat("a", 64),
			SourceSignature:                strings.Repeat("a", 64),
			ResultSignature:                strings.Repeat("9", 64),
			ProducerContentID:              arguments.ExpectedProducerContentID,
			ProducerManifestSHA256:         arguments.ExpectedProducerManifestSHA256,
			QueryContract:                  domainnative.AccountIngressResolutionQueryContractV1,
			QuerySQLHash:                   domainnative.AccountIngressResolutionQuerySQLHashV1,
		},
	})
}

func darwinRunnerAccountIngressDescriptorV1(
	t *testing.T,
	request domainnative.Request,
	snapshotBody []byte,
) domainfundsquerysource.DescriptorV1 {
	t.Helper()
	return darwinRunnerAccountIngressDescriptorWithProfileV1(
		t,
		request,
		snapshotBody,
		domainfundsquerysource.FixedFundsQueryProfileDigestV1(),
	)
}

func darwinRunnerAccountIngressDescriptorWithProfileV1(
	t *testing.T,
	request domainnative.Request,
	snapshotBody []byte,
	queryProfileDigest string,
) domainfundsquerysource.DescriptorV1 {
	t.Helper()
	base := darwinRunnerAccountFlowDescriptor(t, request, snapshotBody)
	descriptor, err := domainfundsquerysource.NewDescriptorV1(
		domainfundsquerysource.DescriptorInputV1{
			SnapshotRecordDigest:                   base.SnapshotRecordDigest,
			DatasetSnapshotID:                      base.DatasetSnapshotID,
			SourceManifestHash:                     base.SourceManifestHash,
			CaseID:                                 base.CaseID,
			CaseBindingHash:                        base.CaseBindingHash,
			DatasetBindingDigest:                   base.DatasetBindingDigest,
			BindingObservationDigest:               request.Context.PublicationPolicy.BindingObservationDigest,
			FundsProducerContentID:                 base.FundsProducerContentID,
			FundsProducerContentManifestSHA256:     base.FundsProducerContentManifestSHA256,
			FundsProducerContentManifestByteLength: base.FundsProducerContentManifestByteLength,
			DuckDBSHA256:                           base.DuckDBSHA256,
			DuckDBByteLength:                       base.DuckDBByteLength,
			DuckDBContentSnapshotDigest:            base.DuckDBContentSnapshotDigest,
			DuckDBSnapshotManifestSHA256:           base.DuckDBSnapshotManifestSHA256,
			MaterializationIdentity:                base.MaterializationIdentity,
			SchemaDigest:                           base.SchemaDigest,
			DatasetUTCOffsetMinutes:                base.DatasetUTCOffsetMinutes,
			ExpectedCurrency:                       base.ExpectedCurrency,
			MinorUnitScale:                         base.MinorUnitScale,
			QueryProfileDigest:                     queryProfileDigest,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return descriptor
}
