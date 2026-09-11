//go:build darwin || linux

package runtimeapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	datasetsnapshotstore "analytix.local/runtime-go/internal/adapters/outbound/datasetsnapshot"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	fundscsvsourceadapter "analytix.local/runtime-go/internal/adapters/outbound/fundscsvsource"
	datasetsnapshotapp "analytix.local/runtime-go/internal/app/datasetsnapshot"
	fundscsvadmissionapp "analytix.local/runtime-go/internal/app/fundscsvadmission"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	fundsquerysourcefixture "analytix.local/runtime-go/internal/testsupport/fundsquerysourcefixture"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

const (
	runtimeFundsCSVConfirmStagePhaseV1        = "stage_and_frozen_source_revalidation"
	runtimeFundsCSVConfirmPrincipalPhaseV1    = "principal_and_case_binding_recheck"
	runtimeFundsCSVConfirmEvidencePhaseV1     = "evidence_initialize_and_initial_head"
	runtimeFundsCSVConfirmCurrentPhaseV1      = "initial_current_snapshot_resolution"
	runtimeFundsCSVConfirmNativePhaseV1       = "composer_native_source_row"
	runtimeFundsCSVConfirmMaterialPhaseV1     = "production_material_writer"
	runtimeFundsCSVConfirmAdmitPhaseV1        = "sealed_dsv2_admit_cas_current_witness"
	runtimeFundsCSVConfirmPostBindingPhaseV1  = "post_admit_case_binding_recheck"
	runtimeFundsCSVConfirmResultPhaseV1       = "confirm_result_binding"
	runtimeFundsCSVConfirmSourceMaxTxnTSV1    = "2026-08-28 12:00:00"
	runtimeFundsCSVConfirmPrivateDuckDBBodyV1 = "AB-R2 deterministic private DuckDB fixture"
)

type runtimeFundsCSVConfirmCountersV1 struct {
	builds           int
	installs         int
	installedReads   int
	rowPages         int
	exactLeaseCopies int
}

type runtimeFundsCSVConfirmNativeOwnerV1 struct {
	tempRoot        string
	counters        *runtimeFundsCSVConfirmCountersV1
	materialization domainsecurity.FundsMaterializationResultV1
}

type runtimeFundsCSVConfirmImmutableSourceV1 struct {
	counters *runtimeFundsCSVConfirmCountersV1
	object   domainfundsquerysource.ImmutableSnapshotObjectV1
	body     []byte
}

type runtimeFundsCSVConfirmMaterialsV1 struct {
	stores *datasetsnapshotstore.StoresV2
	total  int
	kinds  map[domainevidence.FundsCanonicalCSVAdmissionMaterialKindV1]int
}

type runtimeFundsCSVConfirmSnapshotsV1 struct {
	delegate         *datasetsnapshotapp.SealedServiceV2
	resolveAttempts  int
	resolveSuccesses int
	admitAttempts    int
	admitSuccesses   int
	admitAfterCalls  int
	admitted         datasetsnapshotport.ResolvedSnapshotV2
}

func TestFundsCSVConfirmImportV1UsesProductionEvidenceDatasetSnapshotComposition(t *testing.T) {
	ctx := context.Background()
	fixture, config := runtimeWitnessedRegistryConfigV2(t)
	privateRoot := filepath.Join(fixture.DataDir, "private")
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(runtimeFundsCSVConfirmEvidencePhaseV1)
	}
	stores, err := datasetsnapshotstore.OpenStoresV2(
		filepath.Join(privateRoot, "dataset-snapshot-authority"), access,
	)
	if err != nil {
		t.Fatal(runtimeFundsCSVConfirmEvidencePhaseV1)
	}
	evidenceStores, err := openRuntimeSharedEvidenceStoresV2(fixture.DataDir, access)
	if err != nil {
		t.Fatal(runtimeFundsCSVConfirmEvidencePhaseV1)
	}
	if hasEvidence, checkErr := evidenceStores.hasRecords(ctx); checkErr != nil || hasEvidence {
		t.Fatal(runtimeFundsCSVConfirmEvidencePhaseV1)
	}
	if hasSnapshots, checkErr := stores.HasRecords(ctx); checkErr != nil || hasSnapshots || fixture.TotalAttempts() != 0 {
		t.Fatal(runtimeFundsCSVConfirmEvidencePhaseV1)
	}
	observer := filestore.CaseBindingReader{}
	composition, configured, err := newRuntimeSharedEvidenceDatasetSnapshotV2(
		ctx, config, fixture.Authority, stores, evidenceStores, access, observer, nil,
	)
	if err != nil || !configured || composition.evidence == nil || composition.snapshot == nil ||
		composition.registry != nil || fixture.TotalAttempts() != 0 {
		t.Fatal(runtimeFundsCSVConfirmEvidencePhaseV1)
	}
	identity, err := newRuntimeHostIdentityAuthority(fixture.Authority)
	if err != nil {
		t.Fatal(runtimeFundsCSVConfirmPrincipalPhaseV1)
	}

	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(runtimeFundsCSVConfirmStagePhaseV1)
	}
	runtimeSharedEvidenceWriteCaseBindingV2(t, workspace)
	source := []byte("交易卡号,交易账号,账户开户名称,开户人证件号码,交易时间,交易金额,交易余额,收付标志,交易对手账卡号,现金标志,对手户名,对手身份证号,对手开户银行,摘要说明,交易币种,交易网点名称,交易网点代码,交易发生地,交易是否成功,传票号,终端号,IP地址,MAC地址,对手交易余额,交易流水号,日志号,凭证种类,凭证号,交易柜员号,商户名称,商户号,备注,交易类型,查询反馈结果原因\r\n" +
		"6222021234567890001,1000001,合成账户甲,SYNTHID0001,2026-08-28 12:00:00,12.5,1000,进,CP001,否,合成对手甲,SYNTHCPID001,合成银行,合成交易,CNY,合成网点,001,合成地,是,V001,T001,127.0.0.1,000000000001,88.5,TXN001,LOG001,SYNTH,VID001,TEL001,合成商户,M001,AB_R2_PRIVATE_SOURCE_SENTINEL_9f07f00d,synthetic,synthetic\r\n")
	sourcePath := filepath.Join(workspace, "synthetic.csv")
	if err := os.WriteFile(sourcePath, source, 0o600); err != nil {
		t.Fatal(runtimeFundsCSVConfirmStagePhaseV1)
	}
	defer clear(source)

	counters := &runtimeFundsCSVConfirmCountersV1{}
	immutableSource := &runtimeFundsCSVConfirmImmutableSourceV1{counters: counters}
	nativeOwner := &runtimeFundsCSVConfirmNativeOwnerV1{tempRoot: t.TempDir(), counters: counters}
	materials := &runtimeFundsCSVConfirmMaterialsV1{
		stores: stores,
		kinds:  make(map[domainevidence.FundsCanonicalCSVAdmissionMaterialKindV1]int),
	}
	snapshots := &runtimeFundsCSVConfirmSnapshotsV1{delegate: composition.snapshot}
	service, err := fundscsvadmissionapp.NewServiceV1(fundscsvadmissionapp.ConfigV1{
		Observer: observer, Identity: identity, Evidence: composition.evidence,
		Snapshots: snapshots, Materials: materials, Native: nativeOwner, Source: immutableSource,
		ReadImportSource: fundscsvsourceadapter.ReadImportExactV1,
		Now: func() time.Time {
			return time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
		},
		Random: bytes.NewReader(bytes.Repeat([]byte{0x5a}, 1024)),
	})
	if err != nil {
		t.Fatal(runtimeFundsCSVConfirmPrincipalPhaseV1)
	}

	staged, err := service.StageMainSelectedImportV1(ctx, fundscsvadmissionapp.StageInputV1{
		WorkspaceRoot: workspace,
		SourcePath:    sourcePath,
	})
	if err != nil || staged.Status != "ready" || staged.TotalRowCount != 1 || len(staged.Items) != 1 ||
		staged.Items[0].Selector == "" || staged.Items[0].RowCount != 1 || fixture.TotalAttempts() != 0 {
		t.Fatal(runtimeFundsCSVConfirmStagePhaseV1)
	}

	confirmed, confirmErr := service.ConfirmImportV1(ctx, staged.Items[0].Selector)
	if confirmErr != nil {
		t.Fatal(runtimeFundsCSVConfirmFailurePhaseV1(ctx, evidenceStores, counters, materials, snapshots))
	}
	if confirmed.SourceArtifactSHA256 != domainsecurity.SHA256Hex(source) ||
		confirmed.SourceArtifactByteLength != uint64(len(source)) || confirmed.SourceRowCount != 1 {
		t.Fatal(runtimeFundsCSVConfirmResultPhaseV1)
	}
	if counters.builds != 1 || counters.installs != 1 || counters.installedReads != 1 ||
		counters.rowPages != 1 || counters.exactLeaseCopies != 1 {
		t.Fatal(runtimeFundsCSVConfirmNativePhaseV1)
	}
	if !runtimeFundsCSVConfirmExactMaterialGraphV1(materials) {
		t.Fatal(runtimeFundsCSVConfirmMaterialPhaseV1)
	}
	if snapshots.admitAttempts != 1 || snapshots.admitSuccesses != 1 || snapshots.admitAfterCalls != 0 ||
		snapshots.resolveAttempts != 1 || snapshots.resolveSuccesses != 0 ||
		datasetsnapshotport.ValidateResolvedSnapshotV2(snapshots.admitted) != nil {
		t.Fatal(runtimeFundsCSVConfirmAdmitPhaseV1)
	}
	finalObservation, err := observer.Observe(workspace)
	if err != nil {
		t.Fatal(runtimeFundsCSVConfirmPostBindingPhaseV1)
	}
	resolved, err := snapshots.ResolveWitnessedV2(ctx, datasetsnapshotport.ResolveInputV2{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation:               finalObservation,
		ExpectedDatasetSnapshotID: snapshots.admitted.Record.DatasetSnapshotID,
	})
	if err != nil || datasetsnapshotport.ValidateResolvedSnapshotV2(resolved) != nil ||
		resolved.Record.RecordDigest != snapshots.admitted.Record.RecordDigest ||
		resolved.Record.DatasetSnapshotID != snapshots.admitted.Record.DatasetSnapshotID ||
		resolved.Manifest.ManifestDigest != snapshots.admitted.Manifest.ManifestDigest ||
		resolved.FundsProducerContent != snapshots.admitted.FundsProducerContent ||
		snapshots.resolveAttempts != 2 || snapshots.resolveSuccesses != 1 || fixture.TotalAttempts() == 0 {
		t.Fatal(runtimeFundsCSVConfirmAdmitPhaseV1)
	}
	if current, err := observer.Observe(workspace); err != nil || current != finalObservation {
		t.Fatal(runtimeFundsCSVConfirmPostBindingPhaseV1)
	}
	if hasEvidence, err := evidenceStores.hasRecords(ctx); err != nil || !hasEvidence {
		t.Fatal(runtimeFundsCSVConfirmEvidencePhaseV1)
	}
	if hasSnapshots, err := stores.HasRecords(ctx); err != nil || !hasSnapshots {
		t.Fatal(runtimeFundsCSVConfirmAdmitPhaseV1)
	}
}

func (owner *runtimeFundsCSVConfirmNativeOwnerV1) BuildFundsCanonicalCSVSnapshot(
	ctx context.Context,
	arguments domainnative.FundsCanonicalCSVSnapshotBuildArgumentsV1,
	source io.Reader,
	installer fundsquerysourceport.ImmutableSnapshotInstaller,
) (
	domainnative.FundsCanonicalCSVSnapshotBuildResultV1,
	domainfundsquerysource.ImmutableSnapshotObjectV1,
	fundsquerysourceport.ImmutableSnapshotInstallDispositionV1,
	error,
) {
	if owner == nil || owner.counters == nil || ctx == nil || ctx.Err() != nil || source == nil || installer == nil {
		return domainnative.FundsCanonicalCSVSnapshotBuildResultV1{}, domainfundsquerysource.ImmutableSnapshotObjectV1{}, "", errors.New(runtimeFundsCSVConfirmNativePhaseV1)
	}
	owner.counters.builds++
	sourceBody, err := io.ReadAll(io.LimitReader(source, int64(domainnative.FundsCanonicalCSVMaximumSourceBytesV1)+1))
	if err != nil {
		return domainnative.FundsCanonicalCSVSnapshotBuildResultV1{}, domainfundsquerysource.ImmutableSnapshotObjectV1{}, "", errors.New(runtimeFundsCSVConfirmNativePhaseV1)
	}
	defer clear(sourceBody)
	sourceSHA256, sourceByteLength := arguments.SourceIdentityV1()
	if sourceSHA256 == "" || sourceSHA256 != domainsecurity.SHA256Hex(sourceBody) || sourceByteLength != uint64(len(sourceBody)) {
		return domainnative.FundsCanonicalCSVSnapshotBuildResultV1{}, domainfundsquerysource.ImmutableSnapshotObjectV1{}, "", errors.New(runtimeFundsCSVConfirmNativePhaseV1)
	}
	payload, err := runtimeFundsCSVConfirmDecodeBuildPayloadV1(arguments)
	if err != nil || payload.CaseID != arguments.CaseIDV1() {
		return domainnative.FundsCanonicalCSVSnapshotBuildResultV1{}, domainfundsquerysource.ImmutableSnapshotObjectV1{}, "", errors.New(runtimeFundsCSVConfirmNativePhaseV1)
	}
	duckDBBody := []byte(runtimeFundsCSVConfirmPrivateDuckDBBodyV1)
	defer clear(duckDBBody)
	object, err := domainfundsquerysource.NewImmutableSnapshotObjectV1(
		payload.CaseID, domainsecurity.SHA256Hex(duckDBBody), uint64(len(duckDBBody)),
	)
	if err != nil {
		return domainnative.FundsCanonicalCSVSnapshotBuildResultV1{}, domainfundsquerysource.ImmutableSnapshotObjectV1{}, "", errors.New(runtimeFundsCSVConfirmNativePhaseV1)
	}
	file, err := fundsquerysourcefixture.NewUnlinkedDestinationV1(owner.tempRoot)
	if err != nil {
		return domainnative.FundsCanonicalCSVSnapshotBuildResultV1{}, domainfundsquerysource.ImmutableSnapshotObjectV1{}, "", errors.New(runtimeFundsCSVConfirmNativePhaseV1)
	}
	defer file.Close()
	if _, err := file.WriteAt(duckDBBody, 0); err != nil || file.Sync() != nil || file.Chmod(0o400) != nil {
		return domainnative.FundsCanonicalCSVSnapshotBuildResultV1{}, domainfundsquerysource.ImmutableSnapshotObjectV1{}, "", errors.New(runtimeFundsCSVConfirmNativePhaseV1)
	}
	disposition, err := installer.InstallExact(ctx, object, file)
	if err != nil || disposition != fundsquerysourceport.ImmutableSnapshotInstallCreatedV1 {
		return domainnative.FundsCanonicalCSVSnapshotBuildResultV1{}, domainfundsquerysource.ImmutableSnapshotObjectV1{}, "", errors.New(runtimeFundsCSVConfirmNativePhaseV1)
	}
	materialization, result, err := runtimeFundsCSVConfirmBuildResultV1(
		arguments, payload, sourceSHA256, sourceByteLength, object,
	)
	if err != nil {
		return domainnative.FundsCanonicalCSVSnapshotBuildResultV1{}, domainfundsquerysource.ImmutableSnapshotObjectV1{}, "", errors.New(runtimeFundsCSVConfirmNativePhaseV1)
	}
	owner.materialization = materialization
	return result, object, disposition, nil
}

func (owner *runtimeFundsCSVConfirmNativeOwnerV1) TransactionSourceRowPage(
	ctx context.Context,
	arguments domainnative.TransactionSourceRowPageArgumentsV1,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
	lease fundsquerysourceport.ExactReadLease,
) (domainnative.TransactionSourceRowPageV1, error) {
	if owner == nil || owner.counters == nil || ctx == nil || ctx.Err() != nil || lease == nil ||
		domainnative.ValidateTransactionSourceRowPageArgumentsAuthorityV1(arguments, object) != nil {
		return domainnative.TransactionSourceRowPageV1{}, errors.New(runtimeFundsCSVConfirmNativePhaseV1)
	}
	owner.counters.rowPages++
	destination, err := fundsquerysourcefixture.NewUnlinkedDestinationV1(owner.tempRoot)
	if err != nil {
		return domainnative.TransactionSourceRowPageV1{}, errors.New(runtimeFundsCSVConfirmNativePhaseV1)
	}
	defer destination.Close()
	if err := lease.CopyExactTo(ctx, destination); err != nil {
		return domainnative.TransactionSourceRowPageV1{}, errors.New(runtimeFundsCSVConfirmNativePhaseV1)
	}
	installed, err := fundsquerysourcefixture.ReadExactDestinationV1(destination)
	if err != nil || uint64(len(installed)) != object.DuckDBByteLength || domainsecurity.SHA256Hex(installed) != object.DuckDBSHA256 {
		clear(installed)
		return domainnative.TransactionSourceRowPageV1{}, errors.New(runtimeFundsCSVConfirmNativePhaseV1)
	}
	clear(installed)
	page, err := runtimeFundsCSVConfirmSourceRowPageV1(arguments, owner.materialization)
	if err != nil {
		return domainnative.TransactionSourceRowPageV1{}, errors.New(runtimeFundsCSVConfirmNativePhaseV1)
	}
	return page, nil
}

func (source *runtimeFundsCSVConfirmImmutableSourceV1) InstallExact(
	ctx context.Context,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
	file *os.File,
) (fundsquerysourceport.ImmutableSnapshotInstallDispositionV1, error) {
	if source == nil || source.counters == nil || ctx == nil || ctx.Err() != nil || file == nil ||
		domainfundsquerysource.ValidateImmutableSnapshotObjectV1(object) != nil {
		return "", errors.New(runtimeFundsCSVConfirmNativePhaseV1)
	}
	source.counters.installs++
	body := make([]byte, object.DuckDBByteLength)
	if _, err := file.ReadAt(body, 0); err != nil || domainsecurity.SHA256Hex(body) != object.DuckDBSHA256 {
		clear(body)
		return "", errors.New(runtimeFundsCSVConfirmNativePhaseV1)
	}
	clear(source.body)
	source.object = object
	source.body = body
	return fundsquerysourceport.ImmutableSnapshotInstallCreatedV1, nil
}

func (source *runtimeFundsCSVConfirmImmutableSourceV1) WithInstalledExact(
	ctx context.Context,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
	use func(context.Context, fundsquerysourceport.ExactReadLease) error,
) error {
	if source == nil || source.counters == nil || ctx == nil || ctx.Err() != nil || use == nil ||
		object != source.object || uint64(len(source.body)) != object.DuckDBByteLength ||
		domainsecurity.SHA256Hex(source.body) != object.DuckDBSHA256 {
		return errors.New(runtimeFundsCSVConfirmNativePhaseV1)
	}
	source.counters.installedReads++
	body := append([]byte(nil), source.body...)
	defer clear(body)
	lease := fundsquerysourcefixture.NewExactReadLeaseV1WithCounter(body, &source.counters.exactLeaseCopies)
	defer lease.Close()
	return use(ctx, lease)
}

func (writer *runtimeFundsCSVConfirmMaterialsV1) PutFundsCanonicalCSVAdmissionMaterialV1(
	ctx context.Context,
	material domainevidence.FundsCanonicalCSVAdmissionMaterialV1,
) error {
	if writer == nil || writer.stores == nil || writer.kinds == nil {
		return errors.New(runtimeFundsCSVConfirmMaterialPhaseV1)
	}
	var kind domainevidence.FundsCanonicalCSVAdmissionMaterialKindV1
	if err := material.UseExactV1(func(
		observed domainevidence.FundsCanonicalCSVAdmissionMaterialKindV1,
		_ string,
		_ []byte,
	) error {
		kind = observed
		return nil
	}); err != nil {
		return errors.New(runtimeFundsCSVConfirmMaterialPhaseV1)
	}
	if err := writer.stores.PutFundsCanonicalCSVAdmissionMaterialV1(ctx, material); err != nil {
		return errors.New(runtimeFundsCSVConfirmMaterialPhaseV1)
	}
	writer.total++
	writer.kinds[kind]++
	return nil
}

func (snapshots *runtimeFundsCSVConfirmSnapshotsV1) AdmitExactV2(
	ctx context.Context,
	input datasetsnapshotapp.AdmitInputV2,
) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	snapshots.admitAttempts++
	resolved, err := snapshots.delegate.AdmitExactV2(ctx, input)
	if err == nil {
		snapshots.admitSuccesses++
		snapshots.admitted = resolved
	}
	return resolved, err
}

func (snapshots *runtimeFundsCSVConfirmSnapshotsV1) AdmitAfterExactV2(
	ctx context.Context,
	input datasetsnapshotapp.AdmitAfterInputV2,
) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	snapshots.admitAfterCalls++
	return snapshots.delegate.AdmitAfterExactV2(ctx, input)
}

func (snapshots *runtimeFundsCSVConfirmSnapshotsV1) ResolveWitnessedV2(
	ctx context.Context,
	input datasetsnapshotport.ResolveInputV2,
) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	snapshots.resolveAttempts++
	resolved, err := snapshots.delegate.ResolveWitnessedV2(ctx, input)
	if err == nil {
		snapshots.resolveSuccesses++
	}
	return resolved, err
}

type runtimeFundsCSVConfirmBuildPayloadV1 struct {
	CaseID                    string `json:"caseId"`
	Profile                   string `json:"profile"`
	PrivateImportFileID       string `json:"privateImportFileId"`
	SourceRevision            uint64 `json:"sourceRevision"`
	RawArtifactManifestSHA256 string `json:"rawArtifactManifestSha256"`
}

func runtimeFundsCSVConfirmDecodeBuildPayloadV1(
	arguments domainnative.FundsCanonicalCSVSnapshotBuildArgumentsV1,
) (runtimeFundsCSVConfirmBuildPayloadV1, error) {
	frame, err := domainnative.EncodeFundsBuildCanonicalCSVSnapshotNativeRequestFrameV1(strings.Repeat("a", 64), arguments)
	if err != nil {
		return runtimeFundsCSVConfirmBuildPayloadV1{}, err
	}
	defer clear(frame)
	var request struct {
		RequestID string                               `json:"request_id"`
		Command   string                               `json:"command"`
		CaseID    string                               `json:"case_id"`
		Payload   runtimeFundsCSVConfirmBuildPayloadV1 `json:"payload"`
	}
	decoder := json.NewDecoder(bytes.NewReader(frame))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || request.RequestID != strings.Repeat("a", 64) ||
		request.Command != domainnative.OperationFundsBuildCanonicalCSVSnapshotV1 || request.CaseID != request.Payload.CaseID ||
		request.Payload.Profile != domainnative.FundsCanonicalDirectCSVProfileV1 {
		return runtimeFundsCSVConfirmBuildPayloadV1{}, errors.New(runtimeFundsCSVConfirmNativePhaseV1)
	}
	return request.Payload, nil
}

func runtimeFundsCSVConfirmBuildResultV1(
	arguments domainnative.FundsCanonicalCSVSnapshotBuildArgumentsV1,
	payload runtimeFundsCSVConfirmBuildPayloadV1,
	sourceSHA256 string,
	sourceByteLength uint64,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
) (
	domainsecurity.FundsMaterializationResultV1,
	domainnative.FundsCanonicalCSVSnapshotBuildResultV1,
	error,
) {
	producer, err := domainsecurity.NewFundsProducerContentManifestV1(domainsecurity.FundsProducerContentManifestInputV1{
		CaseID: payload.CaseID, SourceRevision: payload.SourceRevision,
		RawManifestSHA256:       payload.RawArtifactManifestSHA256,
		NormalizedContentSHA256: runtimeFundsCSVConfirmDigestV1("normalized"),
		DetailContentSHA256:     runtimeFundsCSVConfirmDigestV1("detail"),
		AggregateContentSHA256:  runtimeFundsCSVConfirmDigestV1("aggregate"),
		KeywordContentSHA256:    runtimeFundsCSVConfirmDigestV1("keyword"),
		AccountContentSHA256:    runtimeFundsCSVConfirmDigestV1("account"),
		NormalizedRowCount:      1, AcceptedRowCount: 1, DetailRowCount: 1,
		AggregateRowCount: 1, KeywordRowCount: 1, AccountRowCount: 1,
	})
	if err != nil {
		return domainsecurity.FundsMaterializationResultV1{}, domainnative.FundsCanonicalCSVSnapshotBuildResultV1{}, err
	}
	producerBody, err := domainsecurity.FundsProducerContentManifestV1Bytes(producer)
	if err != nil {
		return domainsecurity.FundsMaterializationResultV1{}, domainnative.FundsCanonicalCSVSnapshotBuildResultV1{}, err
	}
	rawSources := domainsecurity.FundsRawSourceManifestV1{{
		CleanedStatus: "done", FileID: payload.PrivateImportFileID, RowsImportedNorm: 1,
		SHA256: sourceSHA256, Status: "已完成",
	}}
	rawSourceBody, err := json.Marshal(rawSources)
	if err != nil {
		return domainsecurity.FundsMaterializationResultV1{}, domainnative.FundsCanonicalCSVSnapshotBuildResultV1{}, err
	}
	materialization := domainsecurity.FundsMaterializationResultV1{
		CaseID: payload.CaseID, RowCount: 1,
		AggregateName:                        domainsecurity.FundsMaterializationIdentityPrefixV1 + runtimeFundsCSVConfirmDigestV1("materialization"),
		AggregateVersion:                     domainsecurity.FundsMaterializationAggregateVersionV1,
		MaterializationIdentity:              domainsecurity.FundsMaterializationIdentityPrefixV1 + runtimeFundsCSVConfirmDigestV1("materialization"),
		MaterializationIdentitySchemaVersion: domainsecurity.FundsMaterializationIdentitySchemaVersionV1,
		ProducerContentID:                    domainsecurity.DeriveFundsProducerContentIDV1(producer),
		ProducerContentManifestBase64:        base64.StdEncoding.EncodeToString(producerBody),
		ProducerContentManifestSHA256:        domainsecurity.SHA256Hex(producerBody),
		ProducerContentManifestByteLength:    uint64(len(producerBody)),
		RawArtifactManifestSHA256:            payload.RawArtifactManifestSHA256,
		RawSourceManifestBase64:              base64.StdEncoding.EncodeToString(rawSourceBody),
		RawSourceManifestSHA256:              domainsecurity.SHA256Hex(rawSourceBody),
		RawSourceManifestByteLength:          uint64(len(rawSourceBody)),
		DuckDBContentSnapshotDigest:          object.DuckDBSHA256,
		DuckDBSnapshotManifestSHA256:         runtimeFundsCSVConfirmDigestV1("duckdb-manifest"),
		SchemaDigest:                         domainsecurity.FixedFundsAnalyticalSchemaDigestV1(),
	}
	materializationBody, err := json.Marshal(materialization)
	if err != nil {
		return domainsecurity.FundsMaterializationResultV1{}, domainnative.FundsCanonicalCSVSnapshotBuildResultV1{}, err
	}
	materialization, err = domainsecurity.ParseFundsMaterializationResultV1(materializationBody)
	if err != nil {
		return domainsecurity.FundsMaterializationResultV1{}, domainnative.FundsCanonicalCSVSnapshotBuildResultV1{}, err
	}
	wire := struct {
		SchemaVersion                uint8                                       `json:"schemaVersion"`
		Operation                    string                                      `json:"operation"`
		Profile                      string                                      `json:"profile"`
		PrivateImportFileID          string                                      `json:"privateImportFileId"`
		SourceArtifactSHA256         string                                      `json:"sourceArtifactSha256"`
		SourceArtifactByteLength     uint64                                      `json:"sourceArtifactByteLength"`
		SourceRowCount               uint64                                      `json:"sourceRowCount"`
		SourceMaxTxnTS               string                                      `json:"sourceMaxTxnTs"`
		SourceMaxID                  uint64                                      `json:"sourceMaxId"`
		RowHashContract              string                                      `json:"rowHashContract"`
		FundsMaterializationResultV1 domainsecurity.FundsMaterializationResultV1 `json:"fundsMaterializationResultV1"`
	}{
		SchemaVersion: 1, Operation: domainnative.OperationFundsBuildCanonicalCSVSnapshotV1,
		Profile: domainnative.FundsCanonicalDirectCSVProfileV1, PrivateImportFileID: payload.PrivateImportFileID,
		SourceArtifactSHA256: sourceSHA256, SourceArtifactByteLength: sourceByteLength, SourceRowCount: 1,
		SourceMaxTxnTS: runtimeFundsCSVConfirmSourceMaxTxnTSV1, SourceMaxID: 1,
		RowHashContract:              domainnative.FundsCanonicalCSVRowHashContractV1,
		FundsMaterializationResultV1: materialization,
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return domainsecurity.FundsMaterializationResultV1{}, domainnative.FundsCanonicalCSVSnapshotBuildResultV1{}, err
	}
	result, err := domainnative.ParseFundsCanonicalCSVSnapshotBuildResultV1(body, arguments)
	return materialization, result, err
}

type runtimeFundsCSVConfirmScalarWireV1 struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type runtimeFundsCSVConfirmFieldWireV1 struct {
	Name   string                             `json:"name"`
	Scalar runtimeFundsCSVConfirmScalarWireV1 `json:"scalar"`
}

type runtimeFundsCSVConfirmSnapshotWireV1 struct {
	SourceRevision       uint64 `json:"sourceRevision"`
	SourceRowCount       uint64 `json:"sourceRowCount"`
	SourceMaxTxnTS       string `json:"sourceMaxTxnTs"`
	SourceMaxID          uint64 `json:"sourceMaxId"`
	AcceptedRowCount     uint64 `json:"acceptedRowCount"`
	RejectedRowCount     uint64 `json:"rejectedRowCount"`
	DuplicateRowCount    uint64 `json:"duplicateRowCount"`
	InventoryDigest      string `json:"inventoryDigest"`
	SourceSnapshotDigest string `json:"sourceSnapshotDigest"`
}

type runtimeFundsCSVConfirmInventoryWireV1 struct {
	SourceFileID         string `json:"sourceFileId"`
	SourceFileIDDigest   string `json:"sourceFileIdDigest"`
	PrivateImportFileID  string `json:"privateImportFileId"`
	SourceArtifactSHA256 string `json:"sourceArtifactSha256"`
	FileType             string `json:"fileType"`
	RowCount             uint64 `json:"rowCount"`
	AcceptedRowCount     uint64 `json:"acceptedRowCount"`
	RejectedRowCount     uint64 `json:"rejectedRowCount"`
}

type runtimeFundsCSVConfirmRowWireV1 struct {
	SourceFileID         string                              `json:"sourceFileId"`
	SourceFileIDDigest   string                              `json:"sourceFileIdDigest"`
	SourceRowNumber      uint64                              `json:"sourceRowNumber"`
	SourceArtifactSHA256 string                              `json:"sourceArtifactSha256"`
	CanonicalRowSHA256   string                              `json:"canonicalRowSha256"`
	CanonicalTypedRow    []runtimeFundsCSVConfirmFieldWireV1 `json:"canonicalTypedRow"`
	Disposition          string                              `json:"disposition"`
}

type runtimeFundsCSVConfirmPageWireV1 struct {
	SchemaVersion                  uint8                                      `json:"schemaVersion"`
	Operation                      string                                     `json:"operation"`
	CaseID                         string                                     `json:"caseId"`
	BindingKeyDigest               string                                     `json:"bindingKeyDigest"`
	ParsedGenerationIdentitySHA256 string                                     `json:"parsedGenerationIdentitySha256"`
	Relation                       string                                     `json:"relation"`
	MaterializationIdentity        string                                     `json:"materializationIdentity"`
	ProducerContentContract        string                                     `json:"producerContentContract"`
	ProducerContentID              string                                     `json:"producerContentId"`
	ProducerContentManifestSHA256  string                                     `json:"producerContentManifestSha256"`
	RawArtifactManifestSHA256      string                                     `json:"rawArtifactManifestSha256"`
	DuckDBContentSnapshotDigest    string                                     `json:"duckdbContentSnapshotDigest"`
	DuckDBSnapshotManifestSHA256   string                                     `json:"duckdbSnapshotManifestSha256"`
	AnalyticalSchemaDigest         string                                     `json:"analyticalSchemaDigest"`
	ParserID                       string                                     `json:"parserId"`
	ParserVersion                  string                                     `json:"parserVersion"`
	LocatorOrdering                string                                     `json:"locatorOrdering"`
	SourceProofStatus              string                                     `json:"sourceProofStatus"`
	RequiredHostRawReplayProfile   string                                     `json:"requiredHostRawReplayProfile"`
	HostRawReplayRequired          bool                                       `json:"hostRawReplayRequired"`
	SourceSnapshot                 runtimeFundsCSVConfirmSnapshotWireV1       `json:"sourceSnapshot"`
	Inventory                      []runtimeFundsCSVConfirmInventoryWireV1    `json:"inventory"`
	Rows                           []runtimeFundsCSVConfirmRowWireV1          `json:"rows"`
	NextCursor                     *domainnative.TransactionSourceRowCursorV1 `json:"nextCursor"`
	Complete                       bool                                       `json:"complete"`
	PageDigest                     string                                     `json:"pageDigest"`
}

func runtimeFundsCSVConfirmSourceRowPageV1(
	arguments domainnative.TransactionSourceRowPageArgumentsV1,
	materialization domainsecurity.FundsMaterializationResultV1,
) (domainnative.TransactionSourceRowPageV1, error) {
	frame, err := domainnative.EncodeTransactionSourceRowPageNativeRequestFrameV1(strings.Repeat("b", 64), arguments)
	if err != nil {
		return domainnative.TransactionSourceRowPageV1{}, err
	}
	defer clear(frame)
	var request struct {
		RequestID string `json:"request_id"`
		Command   string `json:"command"`
		CaseID    string `json:"case_id"`
		Payload   struct {
			CaseID                         string                                     `json:"caseId"`
			BindingKeyDigest               string                                     `json:"bindingKeyDigest"`
			ParsedGenerationIdentitySHA256 string                                     `json:"parsedGenerationIdentitySha256"`
			Relation                       string                                     `json:"relation"`
			MaxRows                        uint16                                     `json:"maxRows"`
			Cursor                         *domainnative.TransactionSourceRowCursorV1 `json:"cursor"`
		} `json:"payload"`
	}
	decoder := json.NewDecoder(bytes.NewReader(frame))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || request.Command != domainnative.OperationFundsTransactionSourceRowPageV1 ||
		request.CaseID != request.Payload.CaseID || request.Payload.Cursor != nil || request.Payload.MaxRows == 0 ||
		len(materialization.RawSourceManifest) != 1 {
		return domainnative.TransactionSourceRowPageV1{}, errors.New(runtimeFundsCSVConfirmNativePhaseV1)
	}
	source := materialization.RawSourceManifest[0]
	sourceFileID, err := domainevidence.DeriveFundsCanonicalCSVSourceFileIDV1(request.CaseID, source.FileID)
	if err != nil {
		return domainnative.TransactionSourceRowPageV1{}, err
	}
	sourceFileIDDigest := runtimeFundsCSVConfirmDomainDigestBytesV1(
		[]byte("analytix.source-row-source-file-id/digest/v1\x00"), []byte(sourceFileID),
	)
	fields := runtimeFundsCSVConfirmTypedFieldsV1()
	rowDigest, err := runtimeFundsCSVConfirmDomainDigestJSONV1(
		[]byte("analytix.parsed-canonical-row/digest/v1\x00"), fields,
	)
	if err != nil {
		return domainnative.TransactionSourceRowPageV1{}, err
	}
	inventory := []runtimeFundsCSVConfirmInventoryWireV1{{
		SourceFileID: sourceFileID, SourceFileIDDigest: sourceFileIDDigest,
		PrivateImportFileID: source.FileID, SourceArtifactSHA256: source.SHA256,
		FileType: "CSV", RowCount: 1, AcceptedRowCount: 1,
	}}
	inventoryDigest, err := runtimeFundsCSVConfirmDomainDigestJSONV1(
		[]byte("analytix.funds-source-inventory/v1\x00"), inventory,
	)
	if err != nil {
		return domainnative.TransactionSourceRowPageV1{}, err
	}
	snapshot := runtimeFundsCSVConfirmSnapshotWireV1{
		SourceRevision: materialization.ProducerContentManifest.SourceRevision,
		SourceRowCount: 1, SourceMaxTxnTS: runtimeFundsCSVConfirmSourceMaxTxnTSV1, SourceMaxID: 1,
		AcceptedRowCount: 1, InventoryDigest: inventoryDigest,
	}
	snapshot.SourceSnapshotDigest, err = runtimeFundsCSVConfirmDomainDigestJSONV1(
		[]byte("analytix.funds-source-snapshot/v1\x00"), snapshot,
	)
	if err != nil {
		return domainnative.TransactionSourceRowPageV1{}, err
	}
	page := runtimeFundsCSVConfirmPageWireV1{
		SchemaVersion: 1, Operation: domainnative.OperationFundsTransactionSourceRowPageV1,
		CaseID: request.CaseID, BindingKeyDigest: request.Payload.BindingKeyDigest,
		ParsedGenerationIdentitySHA256: request.Payload.ParsedGenerationIdentitySHA256,
		Relation:                       request.Payload.Relation,
		MaterializationIdentity:        materialization.MaterializationIdentity,
		ProducerContentContract:        materialization.ProducerContentManifest.Contract,
		ProducerContentID:              materialization.ProducerContentID,
		ProducerContentManifestSHA256:  materialization.ProducerContentManifestSHA256,
		RawArtifactManifestSHA256:      materialization.RawArtifactManifestSHA256,
		DuckDBContentSnapshotDigest:    materialization.DuckDBContentSnapshotDigest,
		DuckDBSnapshotManifestSHA256:   materialization.DuckDBSnapshotManifestSHA256,
		AnalyticalSchemaDigest:         materialization.SchemaDigest,
		ParserID:                       domainnative.TransactionSourceRowParserIDV1,
		ParserVersion:                  domainnative.TransactionSourceRowParserVersionV1,
		LocatorOrdering:                domainnative.TransactionSourceRowLocatorOrderingV1,
		SourceProofStatus:              domainnative.TransactionSourceRowProofStatusV1,
		RequiredHostRawReplayProfile:   domainnative.TransactionSourceRowHostReplayProfileV1,
		HostRawReplayRequired:          true, SourceSnapshot: snapshot, Inventory: inventory,
		Rows: []runtimeFundsCSVConfirmRowWireV1{{
			SourceFileID: sourceFileID, SourceFileIDDigest: sourceFileIDDigest, SourceRowNumber: 1,
			SourceArtifactSHA256: source.SHA256, CanonicalRowSHA256: rowDigest,
			CanonicalTypedRow: fields, Disposition: "accepted",
		}},
		NextCursor: nil, Complete: true, PageDigest: strings.Repeat("0", 64),
	}
	page.PageDigest, err = runtimeFundsCSVConfirmPageDigestV1(page)
	if err != nil {
		return domainnative.TransactionSourceRowPageV1{}, err
	}
	body, err := json.Marshal(page)
	if err != nil {
		return domainnative.TransactionSourceRowPageV1{}, err
	}
	return domainnative.ParseTransactionSourceRowPageResultV1(body, arguments)
}

func runtimeFundsCSVConfirmTypedFieldsV1() []runtimeFundsCSVConfirmFieldWireV1 {
	kinds := map[string]string{
		"norm.clean_acct_no": "text", "norm.clean_amount": "decimal", "norm.clean_balance": "decimal",
		"norm.clean_card_no": "text", "norm.clean_dc_flag": "text", "norm.clean_failed": "integer",
		"norm.clean_invalid": "integer", "norm.clean_reversal": "integer", "norm.txn_ts": "text",
		"raw.account_open_name": "text", "raw.acct_no": "text", "raw.amount": "text", "raw.balance": "text",
		"raw.branch_code": "text", "raw.branch_name": "text", "raw.card_no": "text", "raw.cash_flag": "text",
		"raw.counterparty_acct": "text", "raw.counterparty_balance": "text", "raw.counterparty_bank": "text",
		"raw.counterparty_id_no": "text", "raw.counterparty_name": "text", "raw.currency": "text",
		"raw.dc_flag": "text", "raw.ip_addr": "text", "raw.is_success": "text", "raw.location": "text",
		"raw.log_id": "text", "raw.mac_addr": "text", "raw.merchant_name": "text", "raw.merchant_no": "text",
		"raw.opener_id_no": "text", "raw.query_feedback_reason": "text", "raw.remark": "text", "raw.summary": "text",
		"raw.teller_no": "text", "raw.terminal_no": "text", "raw.txn_id": "text", "raw.txn_time": "text",
		"raw.txn_type": "text", "raw.voucher_id": "text", "raw.voucher_no": "text", "raw.voucher_type": "text",
	}
	names := make([]string, 0, len(kinds))
	for name := range kinds {
		names = append(names, name)
	}
	sort.Strings(names)
	fields := make([]runtimeFundsCSVConfirmFieldWireV1, 0, len(names))
	for _, name := range names {
		scalar := runtimeFundsCSVConfirmScalarWireV1{Kind: "null"}
		if kinds[name] == "integer" {
			scalar = runtimeFundsCSVConfirmScalarWireV1{Kind: "integer", Value: "0"}
		}
		fields = append(fields, runtimeFundsCSVConfirmFieldWireV1{Name: name, Scalar: scalar})
	}
	return fields
}

func runtimeFundsCSVConfirmPageDigestV1(page runtimeFundsCSVConfirmPageWireV1) (string, error) {
	body, err := json.Marshal(page)
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	delete(value, "pageDigest")
	return runtimeFundsCSVConfirmDomainDigestJSONV1(
		[]byte("analytix.funds-transaction-source-page/v1\x00"), value,
	)
}

func runtimeFundsCSVConfirmDomainDigestJSONV1(domain []byte, value any) (string, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return runtimeFundsCSVConfirmDomainDigestBytesV1(domain, body), nil
}

func runtimeFundsCSVConfirmDomainDigestBytesV1(domain, body []byte) string {
	hash := sha256.New()
	_, _ = hash.Write(domain)
	_, _ = hash.Write(body)
	return hex.EncodeToString(hash.Sum(nil))
}

func runtimeFundsCSVConfirmDigestV1(label string) string {
	return domainsecurity.SHA256Hex([]byte("analytix.ab-r2.confirm-integration/v1\x00" + label))
}

func runtimeFundsCSVConfirmExactMaterialGraphV1(materials *runtimeFundsCSVConfirmMaterialsV1) bool {
	if materials == nil || materials.total != 21 || len(materials.kinds) != 21 {
		return false
	}
	want := []domainevidence.FundsCanonicalCSVAdmissionMaterialKindV1{
		domainevidence.FundsCanonicalCSVMaterialSnapshotManifestV1,
		domainevidence.FundsCanonicalCSVMaterialProducerContentV1,
		domainevidence.FundsCanonicalCSVMaterialRawIntentV1,
		domainevidence.FundsCanonicalCSVMaterialRawManifestV1,
		domainevidence.FundsCanonicalCSVMaterialRawManifestPageV1,
		domainevidence.FundsCanonicalCSVMaterialRawEntryV1,
		domainevidence.FundsCanonicalCSVMaterialRawSourceLocatorV1,
		domainevidence.FundsCanonicalCSVMaterialRawContentRootV1,
		domainevidence.FundsCanonicalCSVMaterialRawContentIndexPageV1,
		domainevidence.FundsCanonicalCSVMaterialRawContentChunkV1,
		domainevidence.FundsCanonicalCSVMaterialParsedConfigurationV1,
		domainevidence.FundsCanonicalCSVMaterialParsedIdentityV1,
		domainevidence.FundsCanonicalCSVMaterialParsedReceiptV1,
		domainevidence.FundsCanonicalCSVMaterialClassificationLedgerV1,
		domainevidence.FundsCanonicalCSVMaterialParsedIndexPageV1,
		domainevidence.FundsCanonicalCSVMaterialParsedPageV1,
		domainevidence.FundsCanonicalCSVMaterialSourceRowRootV1,
		domainevidence.FundsCanonicalCSVMaterialSourceRowIndexPageV1,
		domainevidence.FundsCanonicalCSVMaterialSourceRowPageV1,
		domainevidence.FundsCanonicalCSVMaterialSourceRowRecordV1,
		domainevidence.FundsCanonicalCSVMaterialSourceRowLineageV1,
	}
	for _, kind := range want {
		if materials.kinds[kind] != 1 {
			return false
		}
	}
	return true
}

func runtimeFundsCSVConfirmFailurePhaseV1(
	ctx context.Context,
	evidenceStores runtimeSharedEvidenceStoresV2,
	counters *runtimeFundsCSVConfirmCountersV1,
	materials *runtimeFundsCSVConfirmMaterialsV1,
	snapshots *runtimeFundsCSVConfirmSnapshotsV1,
) string {
	hasEvidence, _ := evidenceStores.hasRecords(ctx)
	if !hasEvidence && snapshots.resolveAttempts == 0 && counters.builds == 0 {
		return runtimeFundsCSVConfirmPrincipalPhaseV1
	}
	if !hasEvidence {
		return runtimeFundsCSVConfirmEvidencePhaseV1
	}
	if snapshots.resolveAttempts == 0 && counters.builds == 0 {
		return runtimeFundsCSVConfirmCurrentPhaseV1
	}
	if counters.builds != 1 || counters.installs != 1 || counters.installedReads != 1 ||
		counters.rowPages != 1 || counters.exactLeaseCopies != 1 {
		return runtimeFundsCSVConfirmNativePhaseV1
	}
	if !runtimeFundsCSVConfirmExactMaterialGraphV1(materials) {
		return runtimeFundsCSVConfirmMaterialPhaseV1
	}
	if snapshots.admitAttempts != 1 || snapshots.admitSuccesses != 1 {
		return runtimeFundsCSVConfirmAdmitPhaseV1
	}
	return runtimeFundsCSVConfirmPostBindingPhaseV1
}

var _ fundscsvadmissionapp.NativeOwnerV1 = (*runtimeFundsCSVConfirmNativeOwnerV1)(nil)
var _ fundscsvadmissionapp.ImmutableSourceV1 = (*runtimeFundsCSVConfirmImmutableSourceV1)(nil)
var _ fundscsvadmissionapp.MaterialWriterV1 = (*runtimeFundsCSVConfirmMaterialsV1)(nil)
var _ fundscsvadmissionapp.SnapshotAuthorityV2 = (*runtimeFundsCSVConfirmSnapshotsV1)(nil)
