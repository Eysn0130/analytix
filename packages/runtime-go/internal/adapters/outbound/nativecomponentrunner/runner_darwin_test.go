//go:build darwin && !analytix_prod

package nativecomponentrunner

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	nativecomponentregistry "analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry"
	processauthority "analytix.local/runtime-go/internal/adapters/outbound/processauthority"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	"analytix.local/runtime-go/internal/testsupport/toolidentity"
	"golang.org/x/sys/unix"
)

func TestRunnerHealthUsesExactRegistryLeaseAndReusesProcessForBinding(t *testing.T) {
	runner, registry, stagingRoot := newDarwinRunnerTestFixture(t, "valid")
	request := darwinRunnerRequest(t, "case-one", 1)
	for index := 0; index < 2; index++ {
		ctx, cancel := context.WithDeadline(context.Background(), request.Deadline)
		result, err := runner.Execute(ctx, request)
		cancel()
		if err != nil {
			t.Fatalf("execute %d: %v", index+1, err)
		}
		if result.SchemaVersion != domainnative.ResultSchemaVersion || result.ComponentID != domainnative.ComponentDataEngine ||
			result.Operation != "health" || result.Status != "ready" || result.RegistryDigest != registry.digest {
			t.Fatalf("result %d = %#v", index+1, result)
		}
	}
	if registry.Acquisitions() != 1 {
		t.Fatalf("execution lease acquisitions = %d, want 1 long-lived process", registry.Acquisitions())
	}
	if err := runner.Close(); err != nil {
		t.Fatalf("close runner: %v", err)
	}
	assertRunnerStageEmpty(t, stagingRoot)
}

func TestRunnerRestartsSessionWhenCaseEpochBindingChanges(t *testing.T) {
	runner, registry, opener := newDeterministicDarwinRunnerTestFixture(t, -1)
	for _, testCase := range []struct {
		caseID string
		epoch  uint64
	}{
		{caseID: "case-one", epoch: 1},
		{caseID: "case-two", epoch: 2},
	} {
		request := darwinRunnerRequest(t, testCase.caseID, testCase.epoch)
		ctx, cancel := context.WithDeadline(context.Background(), request.Deadline)
		_, err := runner.Execute(ctx, request)
		cancel()
		if err != nil {
			t.Fatalf("execute %s: %v", request.Context.CaseID, err)
		}
	}
	if registry.Acquisitions() != 2 {
		t.Fatalf("execution lease acquisitions = %d, want one per case/epoch binding", registry.Acquisitions())
	}
	wantBeforeClose := []string{"open:1", "roundtrip:1", "close:1", "open:2", "roundtrip:2"}
	if !reflect.DeepEqual(opener.Events(), wantBeforeClose) {
		t.Fatalf("binding restart order = %#v, want %#v", opener.Events(), wantBeforeClose)
	}
	if err := runner.Close(); err != nil {
		t.Fatalf("close runner: %v", err)
	}
	wantAfterClose := append(wantBeforeClose, "close:2")
	if !reflect.DeepEqual(opener.Events(), wantAfterClose) {
		t.Fatalf("runner close order = %#v, want %#v", opener.Events(), wantAfterClose)
	}
}

func TestRunnerBindingChangeRejectsUnconfirmedPriorSessionTermination(t *testing.T) {
	runner, registry, opener := newDeterministicDarwinRunnerTestFixture(t, 1)
	first := darwinRunnerRequest(t, "case-one", 1)
	firstContext, firstCancel := context.WithDeadline(context.Background(), first.Deadline)
	if result, err := runner.Execute(firstContext, first); err != nil || result.Status != "ready" {
		firstCancel()
		t.Fatalf("first execute result=%#v err=%v", result, err)
	}
	firstCancel()
	second := darwinRunnerRequest(t, "case-two", 2)
	secondContext, secondCancel := context.WithDeadline(context.Background(), second.Deadline)
	result, err := runner.Execute(secondContext, second)
	secondCancel()
	if !errors.Is(err, ErrTermination) || result != (domainnative.Result{}) || !runner.poisoned.Load() {
		t.Fatalf("unconfirmed close survived result=%#v err=%v poisoned=%v", result, err, runner.poisoned.Load())
	}
	if registry.Acquisitions() != 1 || !reflect.DeepEqual(opener.Events(), []string{"open:1", "roundtrip:1", "close:1"}) {
		t.Fatalf("new binding acquired before prior close was confirmed: acquisitions=%d events=%#v", registry.Acquisitions(), opener.Events())
	}
}

func TestRunnerRejectsUnboundReadinessAndResponseFrames(t *testing.T) {
	for _, fixture := range []struct {
		name string
		mode string
	}{
		{name: "wrong readiness nonce", mode: "wrong-readiness-nonce"},
		{name: "wrong readiness pid", mode: "wrong-readiness-pid"},
		{name: "target self-reported containment", mode: "target-self-reported-containment"},
		{name: "unknown readiness field", mode: "unknown-readiness-field"},
		{name: "delayed extra readiness", mode: "delayed-extra-readiness"},
		{name: "closed stderr", mode: "closed-stderr"},
		{name: "wrong response id", mode: "wrong-response-id"},
		{name: "wrong response pid", mode: "wrong-response-pid"},
		{name: "unknown response field", mode: "unknown-response-field"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			runner, registry, stagingRoot := newDarwinRunnerTestFixture(t, fixture.mode)
			request := darwinRunnerRequest(t, "case-invalid-protocol", 1)
			ctx, cancel := context.WithDeadline(context.Background(), request.Deadline)
			result, err := runner.Execute(ctx, request)
			cancel()
			if !errors.Is(err, ErrProtocol) || result != (domainnative.Result{}) || registry.Acquisitions() != 1 {
				t.Fatalf("invalid protocol survived: result=%#v err=%v acquisitions=%d", result, err, registry.Acquisitions())
			}
			if err := runner.Close(); err != nil {
				t.Fatalf("close runner: %v", err)
			}
			assertRunnerStageEmpty(t, stagingRoot)
		})
	}
}

func TestValidReadinessV3AcceptsOnlyTargetIdentity(t *testing.T) {
	nonce := strings.Repeat("a", 64)
	pid := 4242
	valid := fmt.Sprintf(
		`{"kind":"analytix_native_ready","schema_version":3,"component_id":"data-engine","launch_nonce":"%s","protocol_version":"analytix-native-v1","process_id":%d}`,
		nonce,
		pid,
	)
	if !validReadiness([]byte(valid), nonce, pid) {
		t.Fatal("valid identity-only readiness v3 was rejected")
	}
	for _, fixture := range []struct {
		name  string
		frame string
	}{
		{name: "schema v2", frame: strings.Replace(valid, `"schema_version":3`, `"schema_version":2`, 1)},
		{name: "missing protocol", frame: strings.Replace(valid, `,"protocol_version":"analytix-native-v1"`, "", 1)},
		{name: "target containment claim", frame: strings.TrimSuffix(valid, "}") + `,"process_containment":{"mechanism":"darwin_rlimit_nproc","soft_limit":0,"hard_limit":0,"fork_probe":{"operation":"fork","outcome":"denied","errno":"EAGAIN"}}}`},
		{name: "unknown target claim", frame: strings.TrimSuffix(valid, "}") + `,"unknown":true}`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if validReadiness([]byte(fixture.frame), nonce, pid) {
				t.Fatalf("invalid readiness survived: %s", fixture.frame)
			}
		})
	}
}

func TestValidPingResponseRejectsRunMillisecondsAboveProtocolBound(t *testing.T) {
	requestID := strings.Repeat("b", 64)
	pid := 4242
	valid := fmt.Sprintf(
		`{"request_id":"%s","ok":true,"data":{"pong":true,"pid":%d},"diagnostics":{"engine":"analytix-data-engine","command":"ping","case_bound":false,"db_bound":false,"pid":%d,"queue_wait_ms":0,"run_ms":10000,"owner_epoch":""}}`,
		requestID,
		pid,
		pid,
	)
	if !validPingResponse([]byte(valid), requestID, pid) {
		t.Fatal("maximum bounded run_ms was rejected")
	}
	tooLarge := strings.Replace(valid, `"run_ms":10000`, `"run_ms":10001`, 1)
	if validPingResponse([]byte(tooLarge), requestID, pid) {
		t.Fatal("run_ms above the cross-language protocol bound survived")
	}
}

func TestRunnerDiscardsValidHelperResponseWhenRegistryDigestChanges(t *testing.T) {
	runner, registry, stagingRoot := newDarwinRunnerTestFixture(t, "valid")
	registry.changeAfterDigestRead = 3
	request := darwinRunnerRequest(t, "case-registry-change", 1)
	ctx, cancel := context.WithDeadline(context.Background(), request.Deadline)
	result, err := runner.Execute(ctx, request)
	cancel()
	if !errors.Is(err, ErrRegistry) || result != (domainnative.Result{}) {
		t.Fatalf("changed registry survived: result=%#v err=%v", result, err)
	}
	if err := runner.Close(); err != nil {
		t.Fatalf("close runner: %v", err)
	}
	assertRunnerStageEmpty(t, stagingRoot)
}

func TestRunnerRejectsInvalidGrantBeforeAcquiringExecutable(t *testing.T) {
	runner, registry, stagingRoot := newDarwinRunnerTestFixture(t, "valid")
	request := darwinRunnerRequest(t, "case-invalid-grant", 1)
	request.Grant.ToolName = "native__data_engine__arbitrary_exec"
	ctx, cancel := context.WithDeadline(context.Background(), request.Deadline)
	result, err := runner.Execute(ctx, request)
	cancel()
	if !errors.Is(err, ErrRequestInvalid) || result != (domainnative.Result{}) || registry.Acquisitions() != 0 {
		t.Fatalf("invalid grant reached executable: result=%#v err=%v acquisitions=%d", result, err, registry.Acquisitions())
	}
	if err := runner.Close(); err != nil {
		t.Fatalf("close runner: %v", err)
	}
	assertRunnerStageEmpty(t, stagingRoot)
}

func TestRunnerAccountFlowClosesCachedHealthAndUsesExactOneShotSnapshot(t *testing.T) {
	snapshotBody := []byte("exact callback-scoped immutable DuckDB snapshot fixture")
	runner, registry, opener, stagingRoot := newAccountFlowDarwinRunnerTestFixture(t, snapshotBody, "valid", false)
	request := darwinRunnerAccountFlowRequest(t, "case-flow", 3)
	descriptor := darwinRunnerAccountFlowDescriptor(t, request, snapshotBody)
	health := darwinRunnerRequest(t, "case-flow", 3)
	if result, err := runner.Execute(context.Background(), health); err != nil || result.Status != "ready" {
		t.Fatalf("prime cached health session: result=%#v err=%v", result, err)
	}

	for index := 0; index < 2; index++ {
		source := &exactBytesReadLease{body: append([]byte(nil), snapshotBody...)}
		result, err := runner.AnalyzeAccountFlows(context.Background(), request, descriptor, source)
		if err != nil {
			t.Fatalf("account flow %d: %v", index+1, err)
		}
		if result.SubjectRef != "cer1_"+strings.Repeat("a", 64) || result.TransactionCount != 0 ||
			result.InflowMinor != "0" || result.OutflowMinor != "0" || result.NetMinor != "0" ||
			result.Coverage.State != domainnative.AccountFlowCoverageObservedNoHitPendingBindingV1 {
			t.Fatalf("account flow %d result = %#v", index+1, result)
		}
		if source.Calls() != 1 || runner.session != nil || runner.sessionBinding != "" {
			t.Fatalf("flow %d reused private state: copies=%d session=%T binding=%q", index+1, source.Calls(), runner.session, runner.sessionBinding)
		}
		assertRunnerStageEmpty(t, stagingRoot)
	}
	if registry.Acquisitions() != 3 {
		t.Fatalf("execution acquisitions = %d, want health plus two one-shot flows", registry.Acquisitions())
	}
	wantEvents := []string{
		"open:health:1", "roundtrip:health:1", "close:health:1",
		"open:flow:2", "roundtrip:flow:2", "close:flow:2",
		"open:flow:3", "roundtrip:flow:3", "close:flow:3",
	}
	if !reflect.DeepEqual(opener.Events(), wantEvents) {
		t.Fatalf("one-shot flow lifecycle = %#v, want %#v", opener.Events(), wantEvents)
	}
	if err := runner.Close(); err != nil {
		t.Fatalf("close runner: %v", err)
	}
	if !reflect.DeepEqual(opener.Events(), wantEvents) {
		t.Fatalf("runner retained a flow session after success: %#v", opener.Events())
	}
}

func TestRunnerAccountFlowRejectsSnapshotHashMismatchBeforeExecutableAcquisition(t *testing.T) {
	snapshotBody := []byte("exact callback-scoped immutable DuckDB snapshot fixture")
	runner, registry, opener, stagingRoot := newAccountFlowDarwinRunnerTestFixture(t, snapshotBody, "valid", false)
	request := darwinRunnerAccountFlowRequest(t, "case-flow-mismatch", 4)
	descriptor := darwinRunnerAccountFlowDescriptor(t, request, snapshotBody)
	tampered := append([]byte(nil), snapshotBody...)
	tampered[len(tampered)-1] ^= 1
	source := &exactBytesReadLease{body: tampered}
	result, err := runner.AnalyzeAccountFlows(context.Background(), request, descriptor, source)
	if !errors.Is(err, ErrRegistry) || !reflect.DeepEqual(result, domainnative.AnalyzeAccountFlowsResultV1{}) ||
		registry.Acquisitions() != 0 || source.Calls() != 1 || len(opener.Events()) != 0 {
		t.Fatalf("snapshot mismatch survived: result=%#v err=%v acquisitions=%d copies=%d events=%#v", result, err, registry.Acquisitions(), source.Calls(), opener.Events())
	}
	assertRunnerStageEmpty(t, stagingRoot)
}

func TestPrivateAccountFlowSnapshotRejectsUnsupportedProductSizeBeforeSourceRead(t *testing.T) {
	snapshotBody := []byte("unsupported product-size snapshot must never be read")
	request := darwinRunnerAccountFlowRequest(t, "case-flow-product-size", 8)
	baseline := darwinRunnerAccountFlowDescriptor(t, request, snapshotBody)
	oversized, err := domainfundsquerysource.NewDescriptorV1(domainfundsquerysource.DescriptorInputV1{
		SnapshotRecordDigest:                   baseline.SnapshotRecordDigest,
		DatasetSnapshotID:                      baseline.DatasetSnapshotID,
		SourceManifestHash:                     baseline.SourceManifestHash,
		CaseID:                                 baseline.CaseID,
		CaseBindingHash:                        baseline.CaseBindingHash,
		DatasetBindingDigest:                   baseline.DatasetBindingDigest,
		BindingObservationDigest:               baseline.BindingObservationDigest,
		FundsProducerContentID:                 baseline.FundsProducerContentID,
		FundsProducerContentManifestSHA256:     baseline.FundsProducerContentManifestSHA256,
		FundsProducerContentManifestByteLength: baseline.FundsProducerContentManifestByteLength,
		DuckDBSHA256:                           baseline.DuckDBSHA256,
		DuckDBByteLength:                       uint64(privateAccountFlowSnapshotMaximumBytes + 1),
		DuckDBContentSnapshotDigest:            baseline.DuckDBContentSnapshotDigest,
		DuckDBSnapshotManifestSHA256:           baseline.DuckDBSnapshotManifestSHA256,
		MaterializationIdentity:                baseline.MaterializationIdentity,
		DatasetUTCOffsetMinutes:                baseline.DatasetUTCOffsetMinutes,
		ExpectedCurrency:                       baseline.ExpectedCurrency,
		MinorUnitScale:                         baseline.MinorUnitScale,
		SchemaDigest:                           baseline.SchemaDigest,
		QueryProfileDigest:                     baseline.QueryProfileDigest,
	})
	if err != nil {
		t.Fatalf("build domain-valid product-oversized descriptor: %v", err)
	}
	stagingRoot := canonicalRunnerTestPath(t, t.TempDir())
	stagingAuthority, err := os.Open(stagingRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stagingAuthority.Close() })
	source := &exactBytesReadLease{body: snapshotBody}
	snapshot, err := materializePrivateAccountFlowSnapshot(
		context.Background(), stagingAuthority, oversized, source,
	)
	if snapshot != nil || !errors.Is(err, ErrRequestInvalid) || source.Calls() != 0 {
		t.Fatalf("unsupported snapshot reached copy: snapshot=%T err=%v copies=%d", snapshot, err, source.Calls())
	}
	assertRunnerStageEmpty(t, stagingRoot)
}

func TestPrivateAccountFlowSnapshotDiskAdmissionRoundsUpAndPreservesReserve(t *testing.T) {
	const (
		blockSize    = uint64(4096)
		snapshotSize = uint64(4097)
	)
	required := snapshotSize + privateAccountFlowSnapshotFreeSpaceReserveBytes
	requiredBlocks := required / blockSize
	if required%blockSize != 0 {
		requiredBlocks++
	}
	exact := unix.Statfs_t{Bsize: uint32(blockSize), Bavail: requiredBlocks}
	if !privateAccountFlowSnapshotHasAvailableBytes(exact, required) {
		t.Fatal("exact disk-space admission was rejected")
	}
	oneBlockShort := exact
	oneBlockShort.Bavail--
	if privateAccountFlowSnapshotHasAvailableBytes(oneBlockShort, required) {
		t.Fatal("disk-space admission ignored the reserved free-space floor")
	}
	if privateAccountFlowSnapshotHasAvailableBytes(unix.Statfs_t{}, required) ||
		privateAccountFlowSnapshotHasAvailableBytes(exact, 0) {
		t.Fatal("invalid filesystem capacity was admitted")
	}
}

func TestPrivateAccountFlowSnapshotCrashLeavesNoLinkedDuckDBBytes(t *testing.T) {
	const (
		helperEnvironment = "ANALYTIX_TEST_PRIVATE_SNAPSHOT_CRASH_ROOT"
		helperExitCode    = 93
	)
	if stagingRoot := os.Getenv(helperEnvironment); stagingRoot != "" {
		stagingAuthority, err := os.Open(stagingRoot)
		if err != nil {
			t.Fatal(err)
		}
		defer stagingAuthority.Close()
		snapshotBody := []byte("DuckDB bytes written immediately before simulated process loss")
		request := darwinRunnerAccountFlowRequest(t, "case-flow-crash", 9)
		descriptor := darwinRunnerAccountFlowDescriptor(t, request, snapshotBody)
		_, err = materializePrivateAccountFlowSnapshot(
			context.Background(),
			stagingAuthority,
			descriptor,
			&exitAfterPrivateSnapshotWriteLease{body: snapshotBody, exitCode: helperExitCode},
		)
		t.Fatalf("crash helper returned instead of exiting: %v", err)
	}

	stagingRoot := canonicalRunnerTestPath(t, t.TempDir())
	command := exec.Command(os.Args[0], "-test.run=^TestPrivateAccountFlowSnapshotCrashLeavesNoLinkedDuckDBBytes$")
	command.Env = append(os.Environ(), helperEnvironment+"="+stagingRoot)
	output, err := command.CombinedOutput()
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) || exitError.ExitCode() != helperExitCode {
		t.Fatalf("crash helper exit=%v output=%q", err, output)
	}
	entries, err := os.ReadDir(stagingRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > 1 {
		t.Fatalf("crash retained unexpected staging objects: %#v", entries)
	}
	if len(entries) == 0 {
		return
	}
	entry := entries[0]
	if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ".account-flow-") {
		t.Fatalf("crash retained a linked snapshot object: name=%q type=%v", entry.Name(), entry.Type())
	}
	operationEntries, err := os.ReadDir(filepath.Join(stagingRoot, entry.Name()))
	if err != nil {
		t.Fatal(err)
	}
	if len(operationEntries) != 0 {
		t.Fatalf("crash retained linked DuckDB bytes: %#v", operationEntries)
	}
}

func TestRunnerAccountFlowRejectsProtocolOnlyAfterConfirmedOneShotClose(t *testing.T) {
	for _, mode := range []string{"wrong-diagnostics", "unknown-outer-field", "invalid-typed-data"} {
		t.Run(mode, func(t *testing.T) {
			snapshotBody := []byte("exact callback-scoped immutable DuckDB snapshot fixture")
			runner, registry, opener, stagingRoot := newAccountFlowDarwinRunnerTestFixture(t, snapshotBody, mode, false)
			request := darwinRunnerAccountFlowRequest(t, "case-flow-protocol-"+mode, 5)
			descriptor := darwinRunnerAccountFlowDescriptor(t, request, snapshotBody)
			result, err := runner.AnalyzeAccountFlows(
				context.Background(), request, descriptor, &exactBytesReadLease{body: snapshotBody},
			)
			if !errors.Is(err, ErrProtocol) || !reflect.DeepEqual(result, domainnative.AnalyzeAccountFlowsResultV1{}) ||
				registry.Acquisitions() != 1 || runner.poisoned.Load() || runner.session != nil {
				t.Fatalf("invalid response survived: result=%#v err=%v acquisitions=%d poisoned=%v session=%T", result, err, registry.Acquisitions(), runner.poisoned.Load(), runner.session)
			}
			want := []string{"open:flow:1", "roundtrip:flow:1", "close:flow:1"}
			if !reflect.DeepEqual(opener.Events(), want) {
				t.Fatalf("protocol rejection lifecycle = %#v, want %#v", opener.Events(), want)
			}
			assertRunnerStageEmpty(t, stagingRoot)
		})
	}
}

func TestRunnerAccountFlowPoisonsOnUnconfirmedOneShotTermination(t *testing.T) {
	snapshotBody := []byte("exact callback-scoped immutable DuckDB snapshot fixture")
	runner, registry, opener, stagingRoot := newAccountFlowDarwinRunnerTestFixture(t, snapshotBody, "valid", true)
	request := darwinRunnerAccountFlowRequest(t, "case-flow-close", 6)
	descriptor := darwinRunnerAccountFlowDescriptor(t, request, snapshotBody)
	result, err := runner.AnalyzeAccountFlows(
		context.Background(), request, descriptor, &exactBytesReadLease{body: snapshotBody},
	)
	if !errors.Is(err, ErrTermination) || !reflect.DeepEqual(result, domainnative.AnalyzeAccountFlowsResultV1{}) ||
		registry.Acquisitions() != 1 || !runner.poisoned.Load() || runner.session != nil {
		t.Fatalf("unconfirmed flow close survived: result=%#v err=%v acquisitions=%d poisoned=%v session=%T", result, err, registry.Acquisitions(), runner.poisoned.Load(), runner.session)
	}
	want := []string{"open:flow:1", "roundtrip:flow:1", "close:flow:1"}
	if !reflect.DeepEqual(opener.Events(), want) {
		t.Fatalf("unconfirmed close lifecycle = %#v, want %#v", opener.Events(), want)
	}
	assertRunnerStageEmpty(t, stagingRoot)
}

func TestRunnerAccountFlowRealDataEngineSeam(t *testing.T) {
	binaryPath := strings.TrimSpace(os.Getenv("ANALYTIX_TEST_REAL_DATA_ENGINE"))
	snapshotPath := strings.TrimSpace(os.Getenv("ANALYTIX_TEST_REAL_DUCKDB_SNAPSHOT"))
	producerContentID := strings.TrimSpace(os.Getenv("ANALYTIX_TEST_REAL_FUNDS_PRODUCER_CONTENT_ID"))
	producerManifestSHA256 := strings.TrimSpace(os.Getenv("ANALYTIX_TEST_REAL_FUNDS_PRODUCER_MANIFEST_SHA256"))
	if binaryPath == "" || snapshotPath == "" || producerContentID == "" || producerManifestSHA256 == "" {
		t.Skip("real data-engine seam requires an explicit isolated binary, DuckDB snapshot, and producer binding")
	}
	for _, path := range []string{binaryPath, snapshotPath} {
		if !filepath.IsAbs(path) || !strings.HasPrefix(filepath.Clean(path), "/Volumes/AnalytixCache/") {
			t.Fatalf("real seam input escaped the trusted cache volume: %q", path)
		}
	}
	canonicalBinary, err := filepath.EvalSymlinks(binaryPath)
	if err != nil || canonicalBinary != filepath.Clean(binaryPath) {
		t.Fatalf("real data-engine path is not canonical: path=%q err=%v", binaryPath, err)
	}
	binaryBody, err := os.ReadFile(canonicalBinary)
	if err != nil || len(binaryBody) == 0 {
		t.Fatalf("read real data-engine: %v", err)
	}
	snapshotBody, err := os.ReadFile(snapshotPath)
	if err != nil || len(snapshotBody) == 0 {
		t.Fatalf("read isolated DuckDB snapshot: %v", err)
	}
	binaryDigest := sha256.Sum256(binaryBody)
	binaryDigestText := hex.EncodeToString(binaryDigest[:])
	registryDigest := domainsecurity.SHA256Hex([]byte("real-data-engine-runner-seam-registry"))
	registry := &fakeExecutionRegistry{
		digest: registryDigest, executable: canonicalBinary,
		identity: nativecomponentregistry.ExecutionIdentity{
			ComponentID: domainnative.ComponentDataEngine, BinaryName: "analytix-data-engine",
			CurrentSHA256: binaryDigestText, CurrentSize: int64(len(binaryBody)),
			PayloadSHA256: binaryDigestText, PayloadSize: int64(len(binaryBody)),
			Format: "mach-o", Arch: runnerTestArch(t, canonicalBinary), RegistryDigest: registryDigest,
		},
	}
	workingDirectory := canonicalRunnerTestPath(t, t.TempDir())
	stagingRoot := canonicalRunnerTestPath(t, t.TempDir())
	for _, path := range []string{workingDirectory, stagingRoot} {
		if !strings.HasPrefix(path, "/Volumes/AnalytixCache/") {
			t.Fatalf("real seam private directory escaped the trusted cache volume: %q", path)
		}
	}
	workingAuthority, err := os.Open(workingDirectory)
	if err != nil {
		t.Fatal(err)
	}
	stagingAuthority, err := os.Open(stagingRoot)
	if err != nil {
		_ = workingAuthority.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = workingAuthority.Close()
		_ = stagingAuthority.Close()
	})
	runner := &Runner{
		gate: make(chan struct{}, 1), registry: registry, registryDigest: registryDigest,
		workingDirectory: workingDirectory, workingAuthority: workingAuthority,
		stagingRoot: stagingRoot, stagingAuthority: stagingAuthority,
		sessionOpener: openProcessAuthoritySession,
	}
	runner.gate <- struct{}{}
	t.Cleanup(func() { _ = runner.Close() })
	request := darwinRunnerAccountFlowRequestWithProducer(
		t, "case-real-seam", 7, producerContentID, producerManifestSHA256,
	)
	descriptor := darwinRunnerAccountFlowDescriptorWithProducer(
		t, request, snapshotBody, producerContentID, producerManifestSHA256,
	)
	result, err := runner.AnalyzeAccountFlows(
		context.Background(), request, descriptor, &exactBytesReadLease{body: snapshotBody},
	)
	if err != nil {
		t.Fatalf("real data-engine account-flow seam: %v", err)
	}
	if result.InflowMinor != "1250" || result.OutflowMinor != "325" || result.NetMinor != "925" ||
		result.TransactionCount != 2 || result.EvidenceRowCountV1() != 2 || !result.AggregateComplete ||
		!result.EvidenceRowsComplete || result.Provenance.ProducerContentID != producerContentID ||
		result.Provenance.ProducerManifestSHA256 != producerManifestSHA256 ||
		!domainsecurity.IsSHA256Hex(result.Provenance.DuckDBContentSnapshotDigest) {
		t.Fatalf(
			"real data-engine result mismatch: inflow=%q outflow=%q net=%q transactions=%d evidence=%d aggregate_complete=%v evidence_complete=%v producer_match=%v manifest_match=%v snapshot_digest_valid=%v",
			result.InflowMinor,
			result.OutflowMinor,
			result.NetMinor,
			result.TransactionCount,
			result.EvidenceRowCountV1(),
			result.AggregateComplete,
			result.EvidenceRowsComplete,
			result.Provenance.ProducerContentID == producerContentID,
			result.Provenance.ProducerManifestSHA256 == producerManifestSHA256,
			domainsecurity.IsSHA256Hex(result.Provenance.DuckDBContentSnapshotDigest),
		)
	}
	previewDescriptor := darwinRunnerAccountFlowDescriptorWithProducerAndProfile(
		t,
		request,
		snapshotBody,
		producerContentID,
		producerManifestSHA256,
		domainfundsquerysource.FixedFundsLocalDisplayQueryProfileDigestV1(),
	)
	previewArguments, err := domainnative.NewDirectSourcePreviewArgumentsV1(
		previewDescriptor,
		[]domainnative.DirectSourcePreviewFieldV1{
			domainnative.DirectSourcePreviewFieldTransactionTimeV1,
			domainnative.DirectSourcePreviewFieldAccountV1,
			domainnative.DirectSourcePreviewFieldAmountTextV1,
			domainnative.DirectSourcePreviewFieldCounterpartyNameV1,
		},
		0,
		25,
	)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := runner.DirectSourcePreview(
		context.Background(),
		previewArguments,
		previewDescriptor,
		&exactBytesReadLease{body: snapshotBody},
	)
	if err != nil {
		t.Fatalf("real data-engine direct-preview seam: %v", err)
	}
	if len(preview.Rows) != 2 || preview.RowOffset != 0 || preview.RowLimit != 25 ||
		preview.HasMore || preview.DatasetSnapshotID != request.Context.DatasetSnapshotID ||
		preview.Currentness != domainnative.DirectSourcePreviewCurrentnessRequiredV1 {
		t.Fatalf("real data-engine direct-preview shape mismatch: %s", preview)
	}
	var exactAmount string
	if err := preview.Rows[0].Cells[2].UseExactV1(func(value string) error {
		exactAmount = value
		return nil
	}); err != nil || exactAmount == "" {
		t.Fatalf("real direct-preview amount is unavailable: value=%q err=%v", exactAmount, err)
	}
	afterBody, err := os.ReadFile(snapshotPath)
	if err != nil || !bytes.Equal(afterBody, snapshotBody) {
		t.Fatalf("source DuckDB changed during read-only runner seam: %v", err)
	}
	if registry.Acquisitions() != 2 || runner.session != nil || runner.sessionBinding != "" {
		t.Fatalf("real flow retained reusable process state: acquisitions=%d session=%T binding=%q", registry.Acquisitions(), runner.session, runner.sessionBinding)
	}
	assertRunnerStageEmpty(t, stagingRoot)
}

type exactBytesReadLease struct {
	mu    sync.Mutex
	body  []byte
	err   error
	calls int
}

type exitAfterPrivateSnapshotWriteLease struct {
	body     []byte
	exitCode int
}

func (lease *exitAfterPrivateSnapshotWriteLease) CopyExactTo(
	ctx context.Context,
	destination *os.File,
) error {
	if lease == nil || ctx == nil || ctx.Err() != nil || destination == nil || len(lease.body) == 0 || lease.exitCode <= 0 {
		os.Exit(92)
	}
	written, err := destination.WriteAt(lease.body, 0)
	if err != nil || written != len(lease.body) {
		os.Exit(92)
	}
	os.Exit(lease.exitCode)
	return nil
}

func (lease *exactBytesReadLease) CopyExactTo(ctx context.Context, destination *os.File) error {
	lease.mu.Lock()
	defer lease.mu.Unlock()
	lease.calls++
	if lease.calls != 1 || ctx == nil || destination == nil {
		return errors.New("exact source lease was reused")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if lease.err != nil {
		return lease.err
	}
	written, err := destination.WriteAt(lease.body, 0)
	if err != nil {
		return err
	}
	if written != len(lease.body) {
		return io.ErrShortWrite
	}
	return nil
}

func (lease *exactBytesReadLease) Calls() int {
	lease.mu.Lock()
	defer lease.mu.Unlock()
	return lease.calls
}

type accountFlowSessionOpener struct {
	mu             sync.Mutex
	events         []string
	opened         int
	snapshotBody   []byte
	snapshotDigest string
	responseMode   string
	failFlowClose  bool
}

func (opener *accountFlowSessionOpener) Open(_ context.Context, config sessionOpenRequest) (processauthority.Session, error) {
	if config.Executable == nil {
		return nil, processauthority.ErrRequestInvalid
	}
	if err := config.Executable.Close(); err != nil {
		return nil, err
	}
	kind := "health"
	if config.ReadOnlyInput != nil {
		kind = "flow"
		if config.ReadOnlyInput.File == nil || config.ReadOnlyInput.ExpectedSize != int64(len(opener.snapshotBody)) ||
			config.ReadOnlyInput.ExpectedSHA256 != opener.snapshotDigest {
			return nil, processauthority.ErrExecutableIdentity
		}
		fd := int(config.ReadOnlyInput.File.Fd())
		var stat unix.Stat_t
		statusFlags, statusErr := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
		descriptorFlags, descriptorErr := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		if unix.Fstat(fd, &stat) != nil || statusErr != nil || descriptorErr != nil ||
			stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0o7777 != 0o400 || stat.Nlink != 0 ||
			stat.Uid != uint32(os.Geteuid()) || stat.Size != int64(len(opener.snapshotBody)) ||
			statusFlags&unix.O_ACCMODE != unix.O_RDONLY || descriptorFlags&unix.FD_CLOEXEC == 0 {
			_ = config.ReadOnlyInput.File.Close()
			return nil, processauthority.ErrExecutableIdentity
		}
		formerPath, pathErr := darwinFileDescriptorPath(fd)
		if pathErr != nil || filepath.Base(formerPath) != privateAccountFlowSnapshotBasename {
			_ = config.ReadOnlyInput.File.Close()
			return nil, processauthority.ErrExecutableIdentity
		}
		body, err := io.ReadAll(io.LimitReader(config.ReadOnlyInput.File, int64(len(opener.snapshotBody))+1))
		closeErr := config.ReadOnlyInput.File.Close()
		if err != nil || closeErr != nil || !bytes.Equal(body, opener.snapshotBody) {
			return nil, processauthority.ErrExecutableIdentity
		}
	}
	nonce := ""
	for _, entry := range config.Environment {
		if strings.HasPrefix(entry, "ANALYTIX_NATIVE_LAUNCH_NONCE=") {
			nonce = strings.TrimPrefix(entry, "ANALYTIX_NATIVE_LAUNCH_NONCE=")
			break
		}
	}
	if !domainsecurity.IsSHA256Hex(nonce) {
		return nil, processauthority.ErrProtocol
	}
	opener.mu.Lock()
	defer opener.mu.Unlock()
	opener.opened++
	index := opener.opened
	opener.events = append(opener.events, fmt.Sprintf("open:%s:%d", kind, index))
	return &accountFlowRunnerSession{
		pid: 8000 + index, index: index, kind: kind, nonce: nonce, owner: opener,
		failClose: kind == "flow" && opener.failFlowClose,
	}, nil
}

func darwinFileDescriptorPath(fd int) (string, error) {
	var path [unix.PathMax]byte
	_, _, errno := unix.Syscall(
		unix.SYS_FCNTL,
		uintptr(fd),
		uintptr(unix.F_GETPATH),
		uintptr(unsafe.Pointer(&path[0])),
	)
	if errno != 0 {
		return "", errno
	}
	end := bytes.IndexByte(path[:], 0)
	if end <= 0 {
		return "", processauthority.ErrExecutableIdentity
	}
	return string(path[:end]), nil
}

func (opener *accountFlowSessionOpener) Events() []string {
	opener.mu.Lock()
	defer opener.mu.Unlock()
	return append([]string(nil), opener.events...)
}

func (opener *accountFlowSessionOpener) record(event string) {
	opener.mu.Lock()
	defer opener.mu.Unlock()
	opener.events = append(opener.events, event)
}

type accountFlowRunnerSession struct {
	pid       int
	index     int
	kind      string
	nonce     string
	owner     *accountFlowSessionOpener
	failClose bool
	closed    bool
}

func (session *accountFlowRunnerSession) PID() int { return session.pid }

func (session *accountFlowRunnerSession) AuthenticateReadiness(
	_ context.Context,
	_ int,
	validate processauthority.ReadinessValidator,
) error {
	frame := []byte(fmt.Sprintf(
		`{"kind":"analytix_native_ready","schema_version":3,"component_id":"data-engine","launch_nonce":"%s","protocol_version":"analytix-native-v1","process_id":%d}`,
		session.nonce,
		session.pid,
	))
	if session.closed || validate == nil || !validate(frame, session.pid) {
		return processauthority.ErrProtocol
	}
	return nil
}

func (session *accountFlowRunnerSession) RoundTrip(_ context.Context, request []byte, limit int) ([]byte, error) {
	if session.closed {
		return nil, processauthority.ErrAbnormalExit
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(request, &object) != nil {
		return nil, processauthority.ErrProtocol
	}
	var envelope struct {
		RequestID string          `json:"request_id"`
		Command   string          `json:"command"`
		CaseID    string          `json:"case_id"`
		Payload   json.RawMessage `json:"payload"`
	}
	if json.Unmarshal(request, &envelope) != nil || !domainsecurity.IsSHA256Hex(envelope.RequestID) {
		return nil, processauthority.ErrProtocol
	}
	session.owner.record(fmt.Sprintf("roundtrip:%s:%d", session.kind, session.index))
	if session.kind == "health" {
		if len(object) != 2 || envelope.Command != "ping" || limit != responseFrameLimit {
			return nil, processauthority.ErrProtocol
		}
		return []byte(fmt.Sprintf(
			`{"request_id":"%s","ok":true,"data":{"pong":true,"pid":%d},"diagnostics":{"engine":"analytix-data-engine","command":"ping","case_bound":false,"db_bound":false,"pid":%d,"queue_wait_ms":0,"run_ms":0,"owner_epoch":""}}`,
			envelope.RequestID, session.pid, session.pid,
		)), nil
	}
	if len(object) != 4 || envelope.CaseID == "" {
		return nil, processauthority.ErrProtocol
	}
	var data []byte
	var err error
	switch envelope.Command {
	case domainnative.OperationFundsAnalyzeAccountFlows:
		var arguments runnerAccountFlowArguments
		if json.Unmarshal(envelope.Payload, &arguments) != nil ||
			envelope.CaseID != arguments.CaseID || limit != accountFlowResponseFrameLimit {
			return nil, processauthority.ErrProtocol
		}
		data, err = runnerAccountFlowSuccessData(arguments)
	case domainnative.OperationFundsResolveAccountIngress:
		var arguments runnerAccountIngressArgumentsV1
		if json.Unmarshal(envelope.Payload, &arguments) != nil ||
			envelope.CaseID != arguments.CaseID || limit != accountIngressResponseFrameLimit {
			return nil, processauthority.ErrProtocol
		}
		data, err = runnerAccountIngressSuccessDataV1(arguments)
	default:
		return nil, processauthority.ErrProtocol
	}
	if err != nil {
		return nil, processauthority.ErrProtocol
	}
	if session.owner.responseMode == "invalid-typed-data" &&
		envelope.Command == domainnative.OperationFundsAnalyzeAccountFlows {
		var invalid map[string]any
		if json.Unmarshal(data, &invalid) != nil {
			return nil, processauthority.ErrProtocol
		}
		invalid["queryHash"] = strings.Repeat("f", 64)
		data, err = json.Marshal(invalid)
		if err != nil {
			return nil, processauthority.ErrProtocol
		}
	}
	caseBound := "true"
	if session.owner.responseMode == "wrong-diagnostics" {
		caseBound = "false"
	}
	extra := ""
	if session.owner.responseMode == "unknown-outer-field" {
		extra = `,"unknown":true`
	}
	return []byte(fmt.Sprintf(
		`{"request_id":"%s","ok":true,"data":%s,"diagnostics":{"engine":"analytix-data-engine","command":"%s","case_bound":%s,"db_bound":true,"pid":%d,"queue_wait_ms":0,"run_ms":0,"owner_epoch":""}%s}`,
		envelope.RequestID, data, envelope.Command, caseBound, session.pid, extra,
	)), nil
}

func (session *accountFlowRunnerSession) Close() error {
	session.owner.record(fmt.Sprintf("close:%s:%d", session.kind, session.index))
	if session.failClose {
		return processauthority.ErrTermination
	}
	session.closed = true
	return nil
}

type runnerAccountFlowArguments struct {
	CaseID                         string `json:"caseId"`
	DatasetSnapshotID              string `json:"datasetSnapshotId"`
	ContextEpoch                   uint64 `json:"contextEpoch"`
	ContextDigest                  string `json:"contextDigest"`
	CaseBindingHash                string `json:"caseBindingHash"`
	ExpectedProducerContentID      string `json:"expectedProducerContentId"`
	ExpectedProducerManifestSHA256 string `json:"expectedProducerManifestSha256"`
	SubjectRef                     string `json:"subjectRef"`
	ResolvedAccountKey             string `json:"resolvedAccountKey"`
	SubjectResolutionDigest        string `json:"subjectResolutionDigest"`
	StartInclusive                 string `json:"startInclusive"`
	EndInclusive                   string `json:"endInclusive"`
	EvidenceRowLimit               uint32 `json:"evidenceRowLimit"`
	DatasetUTCOffsetMinutes        int16  `json:"datasetUtcOffsetMinutes"`
	ExpectedCurrency               string `json:"expectedCurrency"`
	MinorUnitScale                 uint8  `json:"minorUnitScale"`
	ScanCap                        uint32 `json:"scanCap"`
}

type runnerAccountFlowQueryScope struct {
	CaseBindingHash                string `json:"caseBindingHash"`
	ContextDigest                  string `json:"contextDigest"`
	ContextEpoch                   uint64 `json:"contextEpoch"`
	Contract                       string `json:"contract"`
	DatasetSnapshotID              string `json:"datasetSnapshotId"`
	DatasetUTCOffsetMinutes        int16  `json:"datasetUtcOffsetMinutes"`
	EndInclusive                   string `json:"endInclusive"`
	EvidenceRowLimit               uint32 `json:"evidenceRowLimit"`
	ExpectedCurrency               string `json:"expectedCurrency"`
	ExpectedProducerContentID      string `json:"expectedProducerContentId"`
	ExpectedProducerManifestSHA256 string `json:"expectedProducerManifestSha256"`
	MinorUnitScale                 uint8  `json:"minorUnitScale"`
	QuerySQLHash                   string `json:"querySqlHash"`
	ScanCap                        uint32 `json:"scanCap"`
	StartInclusive                 string `json:"startInclusive"`
	SubjectRef                     string `json:"subjectRef"`
	SubjectResolutionDigest        string `json:"subjectResolutionDigest"`
}

type runnerAccountFlowCoverage struct {
	State                  string   `json:"state"`
	Gaps                   []string `json:"gaps"`
	NormalizedSnapshotRows uint64   `json:"normalizedSnapshotRows"`
	AcceptedSnapshotRows   uint64   `json:"acceptedSnapshotRows"`
	RejectedSnapshotRows   uint64   `json:"rejectedSnapshotRows"`
	DuplicateSnapshotRows  uint64   `json:"duplicateSnapshotRows"`
	UntimedSubjectRows     uint64   `json:"untimedSubjectRows"`
	ObservedMatchingRows   uint64   `json:"observedMatchingRows"`
}

type runnerAccountFlowProvenance struct {
	DatasetSnapshotID              string `json:"datasetSnapshotId"`
	ContextEpoch                   uint64 `json:"contextEpoch"`
	ContextDigest                  string `json:"contextDigest"`
	CaseBindingHash                string `json:"caseBindingHash"`
	ExpectedProducerContentID      string `json:"expectedProducerContentId"`
	ExpectedProducerManifestSHA256 string `json:"expectedProducerManifestSha256"`
	SubjectResolutionDigest        string `json:"subjectResolutionDigest"`
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

type runnerAccountFlowResultWire struct {
	SubjectRef              string                      `json:"subjectRef"`
	StartInclusive          string                      `json:"startInclusive"`
	EndInclusive            string                      `json:"endInclusive"`
	Timezone                string                      `json:"timezone"`
	Currency                string                      `json:"currency"`
	MinorUnitScale          uint8                       `json:"minorUnitScale"`
	InflowMinor             string                      `json:"inflowMinor"`
	OutflowMinor            string                      `json:"outflowMinor"`
	NetMinor                string                      `json:"netMinor"`
	TransactionCount        uint64                      `json:"transactionCount"`
	AggregateComplete       bool                        `json:"aggregateComplete"`
	EvidenceRowsComplete    bool                        `json:"evidenceRowsComplete"`
	Coverage                runnerAccountFlowCoverage   `json:"coverage"`
	Provenance              runnerAccountFlowProvenance `json:"provenance"`
	Currentness             string                      `json:"currentness"`
	SemanticProjectionState string                      `json:"semanticProjectionState"`
	EvidenceRows            []json.RawMessage           `json:"evidenceRows"`
	QueryHash               string                      `json:"queryHash"`
	ResultHash              string                      `json:"resultHash,omitempty"`
}

func runnerAccountFlowSuccessData(arguments runnerAccountFlowArguments) ([]byte, error) {
	const querySQLHash = "d06b98ac697a24f6cca52d33ead6839ad9d2eed6ce9147f967d1a0a4e79a5b51"
	const duckDBContentSnapshotDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	queryBody, err := json.Marshal(runnerAccountFlowQueryScope{
		CaseBindingHash: arguments.CaseBindingHash, ContextDigest: arguments.ContextDigest,
		ContextEpoch: arguments.ContextEpoch, Contract: domainnative.AccountFlowQueryContractV1,
		DatasetSnapshotID: arguments.DatasetSnapshotID, DatasetUTCOffsetMinutes: arguments.DatasetUTCOffsetMinutes,
		EndInclusive: arguments.EndInclusive, EvidenceRowLimit: arguments.EvidenceRowLimit,
		ExpectedCurrency: arguments.ExpectedCurrency, ExpectedProducerContentID: arguments.ExpectedProducerContentID,
		ExpectedProducerManifestSHA256: arguments.ExpectedProducerManifestSHA256,
		MinorUnitScale:                 arguments.MinorUnitScale, QuerySQLHash: querySQLHash, ScanCap: arguments.ScanCap,
		StartInclusive: arguments.StartInclusive, SubjectRef: arguments.SubjectRef,
		SubjectResolutionDigest: arguments.SubjectResolutionDigest,
	})
	if err != nil {
		return nil, err
	}
	sourceSignature := strings.Repeat("a", 64)
	wire := runnerAccountFlowResultWire{
		SubjectRef: arguments.SubjectRef, StartInclusive: arguments.StartInclusive, EndInclusive: arguments.EndInclusive,
		Timezone: runnerAccountFlowTimezone(arguments.DatasetUTCOffsetMinutes), Currency: arguments.ExpectedCurrency,
		MinorUnitScale: arguments.MinorUnitScale, InflowMinor: "0", OutflowMinor: "0", NetMinor: "0",
		TransactionCount: 0, AggregateComplete: true, EvidenceRowsComplete: true,
		Coverage: runnerAccountFlowCoverage{
			State: domainnative.AccountFlowCoverageObservedNoHitPendingBindingV1, Gaps: []string{},
		},
		Provenance: runnerAccountFlowProvenance{
			DatasetSnapshotID: arguments.DatasetSnapshotID, ContextEpoch: arguments.ContextEpoch,
			ContextDigest: arguments.ContextDigest, CaseBindingHash: arguments.CaseBindingHash,
			ExpectedProducerContentID:      arguments.ExpectedProducerContentID,
			ExpectedProducerManifestSHA256: arguments.ExpectedProducerManifestSHA256,
			SubjectResolutionDigest:        arguments.SubjectResolutionDigest,
			DuckDBContentSnapshotDigest:    duckDBContentSnapshotDigest, DuckDBSnapshotManifestSHA256: strings.Repeat("c", 64),
			MaterializationIdentity: "txn_daily_snapshot:v12:" + sourceSignature,
			SourceSignature:         sourceSignature, ResultSignature: strings.Repeat("9", 64),
			ProducerContentID:      arguments.ExpectedProducerContentID,
			ProducerManifestSHA256: arguments.ExpectedProducerManifestSHA256,
			QueryContract:          domainnative.AccountFlowQueryContractV1, QuerySQLHash: querySQLHash,
		},
		Currentness:             domainnative.AccountFlowCurrentnessHostRevalidationRequiredV1,
		SemanticProjectionState: domainnative.AccountFlowSemanticHostResolutionRequiredV1,
		EvidenceRows:            []json.RawMessage{},
		QueryHash:               runnerAccountFlowFramedHash("analytix.account-flow-query-hash/v1", []byte(duckDBContentSnapshotDigest), queryBody),
	}
	hashBody, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	wire.ResultHash = runnerAccountFlowFramedHash("analytix.account-flow-result-hash/v1", hashBody)
	return json.Marshal(wire)
}

func runnerAccountFlowFramedHash(domain string, values ...[]byte) string {
	hasher := sha256.New()
	for _, value := range append([][]byte{[]byte(domain)}, values...) {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(value)))
		_, _ = hasher.Write(size[:])
		_, _ = hasher.Write(value)
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func runnerAccountFlowTimezone(offset int16) string {
	if offset == 0 {
		return "Z"
	}
	absolute := int(offset)
	sign := "+"
	if absolute < 0 {
		sign = "-"
		absolute = -absolute
	}
	return fmt.Sprintf("%s%02d:%02d", sign, absolute/60, absolute%60)
}

type fakeExecutionRegistry struct {
	mu                    sync.Mutex
	digest                string
	executable            string
	identity              nativecomponentregistry.ExecutionIdentity
	acquisitions          int
	digestReads           int
	changeAfterDigestRead int
}

func (registry *fakeExecutionRegistry) Digest() string {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.digestReads++
	if registry.changeAfterDigestRead > 0 && registry.digestReads > registry.changeAfterDigestRead {
		return domainsecurity.SHA256Hex([]byte("changed-native-registry"))
	}
	return registry.digest
}

func (registry *fakeExecutionRegistry) Acquire(ctx context.Context, componentID string) (executionLease, error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if ctx == nil || ctx.Err() != nil || componentID != domainnative.ComponentDataEngine {
		return nil, ErrRegistry
	}
	file, err := os.Open(registry.executable)
	if err != nil {
		return nil, err
	}
	registry.acquisitions++
	return &fakeExecutionLease{file: file, identity: registry.identity}, nil
}

func (registry *fakeExecutionRegistry) Acquisitions() int {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	return registry.acquisitions
}

type fakeExecutionLease struct {
	file     *os.File
	identity nativecomponentregistry.ExecutionIdentity
}

type deterministicSessionOpener struct {
	mu             sync.Mutex
	events         []string
	opened         int
	failCloseIndex int
}

func (opener *deterministicSessionOpener) Open(_ context.Context, config sessionOpenRequest) (processauthority.Session, error) {
	if config.Executable == nil {
		return nil, processauthority.ErrRequestInvalid
	}
	if err := config.Executable.Close(); err != nil {
		return nil, err
	}
	nonce := ""
	for _, entry := range config.Environment {
		if strings.HasPrefix(entry, "ANALYTIX_NATIVE_LAUNCH_NONCE=") {
			nonce = strings.TrimPrefix(entry, "ANALYTIX_NATIVE_LAUNCH_NONCE=")
			break
		}
	}
	opener.mu.Lock()
	defer opener.mu.Unlock()
	opener.opened++
	index := opener.opened
	opener.events = append(opener.events, fmt.Sprintf("open:%d", index))
	return &deterministicRunnerSession{
		pid: 7000 + index, index: index, nonce: nonce, owner: opener,
		closeErr: index == opener.failCloseIndex,
	}, nil
}

func (opener *deterministicSessionOpener) Events() []string {
	opener.mu.Lock()
	defer opener.mu.Unlock()
	return append([]string(nil), opener.events...)
}

func (opener *deterministicSessionOpener) record(event string) {
	opener.mu.Lock()
	defer opener.mu.Unlock()
	opener.events = append(opener.events, event)
}

type deterministicRunnerSession struct {
	pid      int
	index    int
	nonce    string
	owner    *deterministicSessionOpener
	closeErr bool
	closed   bool
}

func (session *deterministicRunnerSession) PID() int { return session.pid }

func (session *deterministicRunnerSession) AuthenticateReadiness(_ context.Context, _ int, validate processauthority.ReadinessValidator) error {
	frame := []byte(fmt.Sprintf(
		`{"kind":"analytix_native_ready","schema_version":3,"component_id":"data-engine","launch_nonce":"%s","protocol_version":"analytix-native-v1","process_id":%d}`,
		session.nonce, session.pid,
	))
	if session.closed || validate == nil || !validate(frame, session.pid) {
		return processauthority.ErrProtocol
	}
	return nil
}

func (session *deterministicRunnerSession) RoundTrip(_ context.Context, request []byte, _ int) ([]byte, error) {
	if session.closed {
		return nil, processauthority.ErrAbnormalExit
	}
	var decoded struct {
		RequestID string `json:"request_id"`
		Command   string `json:"command"`
	}
	if json.Unmarshal(request, &decoded) != nil || decoded.Command != "ping" || !domainsecurity.IsSHA256Hex(decoded.RequestID) {
		return nil, processauthority.ErrProtocol
	}
	session.owner.record(fmt.Sprintf("roundtrip:%d", session.index))
	return []byte(fmt.Sprintf(
		`{"request_id":"%s","ok":true,"data":{"pong":true,"pid":%d},"diagnostics":{"engine":"analytix-data-engine","command":"ping","case_bound":false,"db_bound":false,"pid":%d,"queue_wait_ms":0,"run_ms":0,"owner_epoch":""}}`,
		decoded.RequestID, session.pid, session.pid,
	)), nil
}

func (session *deterministicRunnerSession) Close() error {
	session.owner.record(fmt.Sprintf("close:%d", session.index))
	if session.closeErr {
		return processauthority.ErrTermination
	}
	session.closed = true
	return nil
}

func newDeterministicDarwinRunnerTestFixture(t *testing.T, failCloseIndex int) (*Runner, *fakeExecutionRegistry, *deterministicSessionOpener) {
	t.Helper()
	payload := []byte("deterministic-native-runner-fixture")
	executable := filepath.Join(t.TempDir(), "analytix-data-engine")
	if err := os.WriteFile(executable, payload, 0o500); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	digestText := hex.EncodeToString(digest[:])
	registryDigest := domainsecurity.SHA256Hex([]byte("deterministic-native-runner-registry"))
	registry := &fakeExecutionRegistry{
		digest: registryDigest, executable: executable,
		identity: nativecomponentregistry.ExecutionIdentity{
			ComponentID: domainnative.ComponentDataEngine, BinaryName: "analytix-data-engine",
			CurrentSHA256: digestText, CurrentSize: int64(len(payload)),
			PayloadSHA256: digestText, PayloadSize: int64(len(payload)),
			Format: "mach-o", Arch: "arm64", RegistryDigest: registryDigest,
		},
	}
	opener := &deterministicSessionOpener{failCloseIndex: failCloseIndex}
	runner := &Runner{
		gate: make(chan struct{}, 1), registry: registry, registryDigest: registryDigest,
		workingDirectory: t.TempDir(), stagingRoot: t.TempDir(), sessionOpener: opener.Open,
	}
	runner.gate <- struct{}{}
	t.Cleanup(func() { _ = runner.Close() })
	return runner, registry, opener
}

func newAccountFlowDarwinRunnerTestFixture(
	t *testing.T,
	snapshotBody []byte,
	responseMode string,
	failFlowClose bool,
) (*Runner, *fakeExecutionRegistry, *accountFlowSessionOpener, string) {
	t.Helper()
	payload := []byte("deterministic-native-account-flow-runner-fixture")
	executable := filepath.Join(t.TempDir(), "analytix-data-engine")
	if err := os.WriteFile(executable, payload, 0o500); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	digestText := hex.EncodeToString(digest[:])
	registryDigest := domainsecurity.SHA256Hex([]byte("deterministic-native-account-flow-runner-registry"))
	registry := &fakeExecutionRegistry{
		digest: registryDigest, executable: executable,
		identity: nativecomponentregistry.ExecutionIdentity{
			ComponentID: domainnative.ComponentDataEngine, BinaryName: "analytix-data-engine",
			CurrentSHA256: digestText, CurrentSize: int64(len(payload)),
			PayloadSHA256: digestText, PayloadSize: int64(len(payload)),
			Format: "mach-o", Arch: "arm64", RegistryDigest: registryDigest,
		},
	}
	workingDirectory := canonicalRunnerTestPath(t, t.TempDir())
	stagingRoot := canonicalRunnerTestPath(t, t.TempDir())
	workingAuthority, err := os.Open(workingDirectory)
	if err != nil {
		t.Fatal(err)
	}
	stagingAuthority, err := os.Open(stagingRoot)
	if err != nil {
		_ = workingAuthority.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = workingAuthority.Close()
		_ = stagingAuthority.Close()
	})
	snapshotHash := sha256.Sum256(snapshotBody)
	opener := &accountFlowSessionOpener{
		snapshotBody: append([]byte(nil), snapshotBody...), snapshotDigest: hex.EncodeToString(snapshotHash[:]),
		responseMode: responseMode, failFlowClose: failFlowClose,
	}
	runner := &Runner{
		gate: make(chan struct{}, 1), registry: registry, registryDigest: registryDigest,
		workingDirectory: workingDirectory, workingAuthority: workingAuthority,
		stagingRoot: stagingRoot, stagingAuthority: stagingAuthority, sessionOpener: opener.Open,
	}
	runner.gate <- struct{}{}
	t.Cleanup(func() { _ = runner.Close() })
	return runner, registry, opener, stagingRoot
}

func (lease *fakeExecutionLease) TakeExecutionFile() (*os.File, nativecomponentregistry.ExecutionIdentity, error) {
	if lease == nil || lease.file == nil {
		return nil, nativecomponentregistry.ExecutionIdentity{}, ErrRegistry
	}
	file := lease.file
	identity := lease.identity
	lease.file = nil
	lease.identity = nativecomponentregistry.ExecutionIdentity{}
	return file, identity, nil
}

func (lease *fakeExecutionLease) Close() error {
	if lease == nil || lease.file == nil {
		return nil
	}
	err := lease.file.Close()
	lease.file = nil
	return err
}

func newDarwinRunnerTestFixture(t *testing.T, mode string) (*Runner, *fakeExecutionRegistry, string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable = canonicalRunnerTestPath(t, executable)
	payload, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	digestText := hex.EncodeToString(digest[:])
	registryDigest := domainsecurity.SHA256Hex([]byte("native-runner-registry"))
	registry := &fakeExecutionRegistry{
		digest: registryDigest, executable: executable,
		identity: nativecomponentregistry.ExecutionIdentity{
			ComponentID: domainnative.ComponentDataEngine, BinaryName: "analytix-data-engine",
			CurrentSHA256: digestText, CurrentSize: int64(len(payload)),
			PayloadSHA256: digestText, PayloadSize: int64(len(payload)),
			Format: "mach-o", Arch: runnerTestArch(t, executable), RegistryDigest: registryDigest,
		},
	}
	stagingRoot := canonicalRunnerTestPath(t, t.TempDir())
	workingDirectory := canonicalRunnerTestPath(t, t.TempDir())
	workingAuthority, err := os.Open(workingDirectory)
	if err != nil {
		t.Fatal(err)
	}
	stagingAuthority, err := os.Open(stagingRoot)
	if err != nil {
		_ = workingAuthority.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = workingAuthority.Close()
		_ = stagingAuthority.Close()
	})
	runner := &Runner{
		gate: make(chan struct{}, 1), registry: registry, registryDigest: registryDigest,
		workingDirectory: workingDirectory, workingAuthority: workingAuthority,
		stagingRoot: stagingRoot, stagingAuthority: stagingAuthority,
		arguments:     []string{"-test.run=TestNativeComponentRunnerHelper", "--", mode},
		sessionOpener: openProcessAuthoritySession,
	}
	runner.gate <- struct{}{}
	t.Cleanup(func() { _ = runner.Close() })
	return runner, registry, stagingRoot
}

func darwinRunnerRequest(t *testing.T, caseID string, epoch uint64) domainnative.Request {
	t.Helper()
	now := time.Now().UTC().Add(-time.Second)
	securityContext, err := securitytest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-" + caseID, TurnID: "turn-" + caseID,
		WorkspaceRealPath: "/cases/" + caseID, TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: caseID, CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding:" + caseID)),
		DatasetSnapshotID:  securitytest.DatasetSnapshotID("snapshot:" + caseID),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest:" + caseID)), ContextEpoch: epoch, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := json.RawMessage(`{}`)
	policy, ok := domainnative.Policy(domainnative.ComponentDataEngine, "health")
	if !ok {
		t.Fatal("missing native health policy")
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: domainnative.NativeProvider, ServerIdentity: domainnative.NativeServerIdentity,
		ToolName: domainnative.ToolName(policy.ComponentID, policy.Operation), ToolCallID: toolidentity.MustHostToolCallIDV1("native-runner:" + caseID),
		ArgsHash: domainsecurity.CanonicalJSONHash(payload), SchemaHash: policy.SchemaHash,
		ScopeHash: domainnative.ScopeHash(securityContext, policy.ComponentID, policy.Operation), ReadOnly: true,
		ApprovalState: "not_required", IssuedAt: now, ExpiresAt: now.Add(30 * time.Second),
	})
	return domainnative.Request{
		ComponentID: domainnative.ComponentDataEngine, Operation: "health", Context: securityContext,
		Grant: grant, Deadline: time.Now().UTC().Add(policy.MaxDuration),
	}
}

func darwinRunnerAccountFlowRequest(t *testing.T, caseID string, epoch uint64) domainnative.Request {
	return darwinRunnerAccountFlowRequestWithProducer(
		t,
		caseID,
		epoch,
		domainsecurity.FundsProducerContentIDPrefixV1+strings.Repeat("4", 64),
		strings.Repeat("5", 64),
	)
}

func darwinRunnerAccountFlowRequestWithProducer(
	t *testing.T,
	caseID string,
	epoch uint64,
	producerContentID string,
	producerManifestSHA256 string,
) domainnative.Request {
	t.Helper()
	request := darwinRunnerRequest(t, caseID, epoch)
	policy, ok := domainnative.Policy(domainnative.ComponentDataEngine, domainnative.OperationFundsAnalyzeAccountFlows)
	if !ok {
		t.Fatal("missing fixed account-flow policy")
	}
	arguments, err := domainnative.NewAnalyzeAccountFlowsArgumentsV1(domainnative.NewAnalyzeAccountFlowsArgumentsInputV1(domainnative.AnalyzeAccountFlowsArgumentsInputV1{
		CaseID: request.Context.CaseID, DatasetSnapshotID: request.Context.DatasetSnapshotID,
		ContextEpoch: request.Context.ContextEpoch, ContextDigest: request.Context.ContextDigest,
		CaseBindingHash:                      request.Context.CaseBindingHash,
		ExpectedProducerContentID:            producerContentID,
		ExpectedProducerManifestSHA256:       producerManifestSHA256,
		ExpectedDuckDBContentSnapshotDigest:  strings.Repeat("b", 64),
		ExpectedDuckDBSnapshotManifestSHA256: strings.Repeat("c", 64),
		ExpectedMaterializationIdentity:      "txn_daily_snapshot:v12:" + strings.Repeat("a", 64),
		SubjectAlias:                         "acct:1",
		SubjectRef:                           "cer1_" + strings.Repeat("a", 64),
		SubjectResolutionDigest:              strings.Repeat("6", 64),
		StartInclusive:                       "2026-01-01T00:00:00Z", EndInclusive: "2026-01-01T00:02:00Z",
		EvidenceRowLimit: 10, DatasetUTCOffsetMinutes: 0, ExpectedCurrency: "CNY",
		MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1, ScanCap: 100,
	}, "host-private-account-key"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Second)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: request.Context, Provider: domainnative.NativeProvider, ServerIdentity: domainnative.NativeServerIdentity,
		ToolName:   domainnative.ToolName(policy.ComponentID, policy.Operation),
		ToolCallID: toolidentity.MustHostToolCallIDV1("native-runner-account-flow:" + caseID),
		ArgsHash:   domainnative.AnalyzeAccountFlowsArgumentsHashV1(arguments), SchemaHash: policy.SchemaHash,
		ScopeHash: domainnative.ScopeHash(request.Context, policy.ComponentID, policy.Operation), ReadOnly: true,
		ApprovalState: "not_required", IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	return domainnative.Request{
		ComponentID: domainnative.ComponentDataEngine, Operation: domainnative.OperationFundsAnalyzeAccountFlows,
		AccountFlowArguments: &arguments, Context: request.Context, Grant: grant,
		Deadline: time.Now().UTC().Add(policy.MaxDuration),
	}
}

func darwinRunnerAccountFlowDescriptor(
	t *testing.T,
	request domainnative.Request,
	snapshotBody []byte,
) domainfundsquerysource.DescriptorV1 {
	return darwinRunnerAccountFlowDescriptorWithProducer(
		t,
		request,
		snapshotBody,
		domainsecurity.FundsProducerContentIDPrefixV1+strings.Repeat("4", 64),
		strings.Repeat("5", 64),
	)
}

func darwinRunnerAccountFlowDescriptorWithProducer(
	t *testing.T,
	request domainnative.Request,
	snapshotBody []byte,
	producerContentID string,
	producerManifestSHA256 string,
) domainfundsquerysource.DescriptorV1 {
	return darwinRunnerAccountFlowDescriptorWithProducerAndProfile(
		t,
		request,
		snapshotBody,
		producerContentID,
		producerManifestSHA256,
		domainfundsquerysource.FixedFundsQueryProfileDigestV1(),
	)
}

func darwinRunnerAccountFlowDescriptorWithProducerAndProfile(
	t *testing.T,
	request domainnative.Request,
	snapshotBody []byte,
	producerContentID string,
	producerManifestSHA256 string,
	queryProfileDigest string,
) domainfundsquerysource.DescriptorV1 {
	t.Helper()
	digest := sha256.Sum256(snapshotBody)
	duckDBSHA256 := hex.EncodeToString(digest[:])
	descriptor, err := domainfundsquerysource.NewDescriptorV1(domainfundsquerysource.DescriptorInputV1{
		SnapshotRecordDigest: domainsecurity.SHA256Hex([]byte("snapshot-record:" + request.Context.DatasetSnapshotID)),
		DatasetSnapshotID:    request.Context.DatasetSnapshotID, SourceManifestHash: request.Context.SourceManifestHash,
		CaseID: request.Context.CaseID, CaseBindingHash: request.Context.CaseBindingHash,
		DatasetBindingDigest:                   domainsecurity.SHA256Hex([]byte("dataset-binding:" + request.Context.ContextDigest)),
		BindingObservationDigest:               domainsecurity.SHA256Hex([]byte("binding-observation:" + request.Context.ContextDigest)),
		FundsProducerContentID:                 producerContentID,
		FundsProducerContentManifestSHA256:     producerManifestSHA256,
		FundsProducerContentManifestByteLength: 1,
		DuckDBSHA256:                           duckDBSHA256, DuckDBByteLength: uint64(len(snapshotBody)),
		DuckDBContentSnapshotDigest:  strings.Repeat("b", 64),
		DuckDBSnapshotManifestSHA256: strings.Repeat("c", 64),
		MaterializationIdentity:      "txn_daily_snapshot:v12:" + strings.Repeat("a", 64),
		DatasetUTCOffsetMinutes:      0, ExpectedCurrency: "CNY",
		MinorUnitScale:     domainnative.AccountFlowMinorUnitScaleV1,
		SchemaDigest:       domainfundsquerysource.FixedFundsAnalyticalSchemaDigestV1(),
		QueryProfileDigest: queryProfileDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	return descriptor
}

func TestNativeComponentRunnerHelper(t *testing.T) {
	mode := ""
	for index, value := range os.Args {
		if value == "--" && index+1 < len(os.Args) {
			mode = os.Args[index+1]
			break
		}
	}
	if mode == "" {
		return
	}
	nonce := os.Getenv("ANALYTIX_NATIVE_LAUNCH_NONCE")
	pid := os.Getpid()
	readinessNonce := nonce
	readinessPID := pid
	if mode == "wrong-readiness-nonce" {
		readinessNonce = strings.Repeat("0", 64)
	}
	if mode == "wrong-readiness-pid" {
		readinessPID++
	}
	if mode == "unknown-readiness-field" {
		fmt.Printf(`{"kind":"analytix_native_ready","schema_version":3,"component_id":"data-engine","launch_nonce":"%s","protocol_version":"analytix-native-v1","process_id":%d,"unknown":true}`+"\n", readinessNonce, readinessPID)
	} else if mode == "target-self-reported-containment" {
		fmt.Printf(`{"kind":"analytix_native_ready","schema_version":3,"component_id":"data-engine","launch_nonce":"%s","protocol_version":"analytix-native-v1","process_id":%d,"process_containment":{"mechanism":"darwin_rlimit_nproc","soft_limit":0,"hard_limit":0,"fork_probe":{"operation":"fork","outcome":"denied","errno":"EAGAIN"}}}`+"\n", readinessNonce, readinessPID)
	} else {
		fmt.Printf(`{"kind":"analytix_native_ready","schema_version":3,"component_id":"data-engine","launch_nonce":"%s","protocol_version":"analytix-native-v1","process_id":%d}`+"\n", readinessNonce, readinessPID)
	}
	if mode == "delayed-extra-readiness" {
		time.Sleep(50 * time.Millisecond)
		fmt.Println(`{"kind":"delayed-unbound-frame"}`)
	}
	if mode == "closed-stderr" {
		_ = os.Stderr.Close()
		time.Sleep(50 * time.Millisecond)
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			os.Exit(0)
		}
		var request struct {
			RequestID string `json:"request_id"`
			Command   string `json:"command"`
		}
		if json.Unmarshal(line, &request) != nil || request.Command != "ping" {
			os.Exit(31)
		}
		responseID := request.RequestID
		responsePID := pid
		if mode == "wrong-response-id" {
			responseID = strings.Repeat("f", 64)
		}
		if mode == "wrong-response-pid" {
			responsePID++
		}
		unknown := ""
		if mode == "unknown-response-field" {
			unknown = `,"unknown":true`
		}
		fmt.Printf(`{"request_id":"%s","ok":true,"data":{"pong":true,"pid":%d},"diagnostics":{"engine":"analytix-data-engine","command":"ping","case_bound":false,"db_bound":false,"pid":%d,"queue_wait_ms":0,"run_ms":0,"owner_epoch":""}%s}`+"\n", responseID, responsePID, responsePID, unknown)
	}
}

func canonicalRunnerTestPath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(canonical, 0o700); err != nil {
		t.Fatal(err)
	}
	return canonical
}

func runnerTestArch(t *testing.T, path string) string {
	t.Helper()
	output, err := os.ReadFile(path)
	if err != nil || len(output) < 8 {
		t.Fatal("read Mach-O header")
	}
	// Go test binaries on the current host are thin little-endian Mach-O.
	switch hex.EncodeToString(output[4:8]) {
	case "07000001":
		return "x64"
	case "0c000001":
		return "arm64"
	default:
		t.Fatalf("unsupported test Mach-O CPU header %x", output[4:8])
		return ""
	}
}

func assertRunnerStageEmpty(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("staging root retained launch material: %#v", entries)
	}
}
