//go:build darwin

package nativecomponenthost

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	datasetsnapshotstore "analytix.local/runtime-go/internal/adapters/outbound/datasetsnapshot"
	fundsquerysource "analytix.local/runtime-go/internal/adapters/outbound/fundsquerysource"
	nativecomponentregistry "analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry"
	datasetsnapshotapp "analytix.local/runtime-go/internal/app/datasetsnapshot"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	"analytix.local/runtime-go/internal/testsupport/toolidentity"
	"golang.org/x/sys/unix"
)

func TestLocalNonpublishablePackagedOwnerBuildsCanonicalCSVUnderProductionSession(t *testing.T) {
	executable := strings.TrimSpace(os.Getenv("ANALYTIX_TEST_LOCAL_NONPUBLISHABLE_RUNTIME_SERVER"))
	if executable == "" {
		t.Skip("local non-publishable packaged owner seam requires an explicit isolated runtime-server")
	}
	if !filepath.IsAbs(executable) || !strings.HasPrefix(filepath.Clean(executable), "/Volumes/AnalytixCache/") {
		t.Fatal("packaged runtime-server escaped the trusted cache volume")
	}
	executable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal("resolve packaged runtime-server")
	}
	dataDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal("resolve isolated production profile")
	}
	if err := os.Chmod(dataDir, 0o700); err != nil {
		t.Fatal("make isolated production profile private")
	}
	immutableSource, err := fundsquerysource.NewHostExactSource(dataDir)
	if err != nil {
		t.Fatal("open production immutable Funds source")
	}
	privateCASRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal("resolve isolated material store root")
	}
	if err := os.Chmod(privateCASRoot, 0o700); err != nil {
		t.Fatal("make isolated material store root private")
	}
	privateCASAccess, err := privatecastest.NewAccessAuthority(privateCASRoot)
	if err != nil {
		t.Fatal("open isolated private-CAS test authority")
	}
	stores, err := datasetsnapshotstore.OpenStoresV2(
		filepath.Join(privateCASRoot, "dataset-snapshot-authority"),
		privateCASAccess,
	)
	if err != nil {
		t.Fatal("open production dataset snapshot material stores")
	}
	owner, err := openDarwinHostCandidateForExecutable(dataDir, executable)
	if err != nil {
		t.Fatal("open local non-publishable packaged owner")
	}
	defer func() {
		if closeErr := owner.Close(); closeErr != nil {
			t.Error("close local non-publishable packaged owner")
		}
	}()

	const caseID = "case-ab-r2-local-nonpublishable"
	now := time.Now().UTC().Add(-time.Second)
	observation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: "/synthetic/" + caseID,
		State:             domainsecurity.CaseBindingStateValid,
		CaseID:            caseID,
		BindingSHA256:     strings.Repeat("4", 64),
		CaseBindingHash:   strings.Repeat("5", 64),
	})
	if err != nil {
		t.Fatal("construct production case-binding observation")
	}
	securityContext, err := securitytest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-" + caseID, TurnID: "turn-" + caseID,
		WorkspaceRealPath: observation.WorkspaceRealPath, TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: observation.CaseID, CaseBindingHash: observation.CaseBindingHash,
		DatasetSnapshotID:  securitytest.DatasetSnapshotID("snapshot:" + caseID),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest:" + caseID)), ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	policy, ok := domainnative.Policy(domainnative.ComponentDataEngine, "health")
	if !ok {
		t.Fatal("missing native health policy")
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: domainnative.NativeProvider, ServerIdentity: domainnative.NativeServerIdentity,
		ToolName:   domainnative.ToolName(policy.ComponentID, policy.Operation),
		ToolCallID: toolidentity.MustHostToolCallIDV1("ab-r2-local-package-health"),
		ArgsHash:   domainsecurity.CanonicalJSONHash(json.RawMessage(`{}`)), SchemaHash: policy.SchemaHash,
		ScopeHash: domainnative.ScopeHash(securityContext, policy.ComponentID, policy.Operation), ReadOnly: true,
		ApprovalState: "not_required", IssuedAt: now, ExpiresAt: now.Add(30 * time.Second),
	})
	healthRequest := domainnative.Request{
		ComponentID: domainnative.ComponentDataEngine, Operation: "health", Context: securityContext,
		Grant: grant, Deadline: time.Now().UTC().Add(policy.MaxDuration),
	}
	healthContext, healthCancel := context.WithDeadline(context.Background(), healthRequest.Deadline)
	health, err := owner.Execute(healthContext, healthRequest)
	healthCancel()
	if err != nil || health.Status != "ready" {
		t.Fatal("execute packaged production-session health")
	}

	binding, err := domainsecurity.NewDatasetSnapshotBindingKeyV1(
		domainsecurity.DatasetSnapshotBindingKeyInputV1{
			TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
			WorkspaceRealPath: observation.WorkspaceRealPath, CaseID: observation.CaseID,
			CaseBindingHash: observation.CaseBindingHash, BindingObservationDigest: observation.ObservationDigest,
		},
	)
	if err != nil || domainsecurity.ValidateDatasetSnapshotBindingKeyForObservationV1(
		binding, domainsecurity.LocalTenantID, domainsecurity.LocalUserID, observation,
	) != nil {
		t.Fatal("construct production dataset binding")
	}
	source := []byte("交易卡号,交易账号,账户开户名称,开户人证件号码,交易时间,交易金额,交易余额,收付标志,交易对手账卡号,现金标志,对手户名,对手身份证号,对手开户银行,摘要说明,交易币种,交易网点名称,交易网点代码,交易发生地,交易是否成功,传票号,终端号,IP地址,MAC地址,对手交易余额,交易流水号,日志号,凭证种类,凭证号,交易柜员号,商户名称,商户号,备注,交易类型,查询反馈结果原因\r\n" +
		"6222021234567890001,1000001,合成账户甲,SYNTHID0001,2026-08-27 10:00:00,12.5,1000,进,CP001,否,合成对手甲,SYNTHCPID001,合成银行,合成交易,CNY,合成网点,001,合成地,是,V001,T001,127.0.0.1,000000000001,88.5,TXN001,LOG001,SYNTH,VID001,TEL001,合成商户,M001,AB_R2_PRIVATE_SOURCE_SENTINEL_9f07f00d,synthetic,synthetic\r\n")
	sourceArtifactSHA256 := domainsecurity.SHA256Hex(source)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	composerEntered := true
	nativeCallbackEntered := 0
	nativeCallbackCompleted := 0
	materialWriterAccepted := 0
	materialWriterFailed := false
	materialCounts := make(map[domainevidence.FundsCanonicalCSVAdmissionMaterialKindV1]int)
	summary, err := domainevidence.ComposeFundsCanonicalCSVAdmissionV1(
		ctx,
		domainevidence.FundsCanonicalCSVAdmissionInputV1{
			Binding: binding, AcquiredAt: now,
			AcquisitionActorDigest:   strings.Repeat("8", 64),
			IntentNonceDigest:        strings.Repeat("9", 64),
			SourceArtifactSHA256:     sourceArtifactSHA256,
			SourceArtifactByteLength: uint64(len(source)),
			SourceRowCount:           1,
		},
		bytes.NewReader(source),
		func(
			buildCtx context.Context,
			build domainevidence.FundsCanonicalCSVNativeBuildContextV1,
		) (domainevidence.FundsCanonicalCSVNativeBuildResultV1, error) {
			nativeCallbackEntered++
			arguments, buildErr := domainnative.NewFundsCanonicalCSVSnapshotBuildArgumentsV1(
				domainnative.FundsCanonicalCSVSnapshotBuildInputV1{
					Binding: build.Binding, PrivateImportFileID: build.PrivateImportFileID, SourceRevision: 1,
					RawArtifactManifestSHA256: build.RawArtifactManifestSHA256,
					SourceArtifactSHA256:      build.SourceArtifactSHA256,
					SourceArtifactByteLength:  build.SourceArtifactByteLength,
					SourceRowCount:            build.SourceRowCount,
				},
			)
			if buildErr != nil {
				return domainevidence.FundsCanonicalCSVNativeBuildResultV1{}, errors.New("production native build arguments are invalid")
			}
			result, object, disposition, buildErr := owner.BuildFundsCanonicalCSVSnapshot(
				buildCtx, arguments, bytes.NewReader(source), immutableSource,
			)
			if buildErr != nil {
				return domainevidence.FundsCanonicalCSVNativeBuildResultV1{}, errors.New("production canonical CSV build failed")
			}
			sha256Text, byteLength, rowCount, _, _ := result.SourceStatsV1()
			if sha256Text != sourceArtifactSHA256 || byteLength != uint64(len(source)) || rowCount != 1 ||
				object.CaseID != caseID || disposition != fundsquerysourceport.ImmutableSnapshotInstallCreatedV1 {
				return domainevidence.FundsCanonicalCSVNativeBuildResultV1{}, errors.New("production canonical CSV binding changed")
			}
			materialization, materializationErr := result.MaterializationV1()
			if materializationErr != nil {
				return domainevidence.FundsCanonicalCSVNativeBuildResultV1{}, errors.New("production materialization is invalid")
			}
			pageArguments, pageArgumentsErr := domainnative.NewTransactionSourceRowPageArgumentsV1(
				domainnative.TransactionSourceRowPageArgumentsInputV1{
					Binding:                        build.Binding,
					ParsedGenerationIdentitySHA256: build.ParsedGenerationIdentitySHA256,
					Materialization:                materialization,
					InstalledSnapshot:              object,
					MaxRows:                        domainnative.TransactionSourceRowMaximumRowsV1,
					Cursor:                         nil,
				},
			)
			if pageArgumentsErr != nil {
				return domainevidence.FundsCanonicalCSVNativeBuildResultV1{}, errors.New("production source-row arguments are invalid")
			}
			var page domainnative.TransactionSourceRowPageV1
			readErr := immutableSource.WithInstalledExact(buildCtx, object, func(
				leaseCtx context.Context,
				lease fundsquerysourceport.ExactReadLease,
			) error {
				var ownerReadErr error
				page, ownerReadErr = owner.TransactionSourceRowPage(leaseCtx, pageArguments, object, lease)
				return ownerReadErr
			})
			if readErr != nil {
				return domainevidence.FundsCanonicalCSVNativeBuildResultV1{}, errors.New("production installed source-row read failed")
			}
			var nativeResult domainevidence.FundsCanonicalCSVNativeBuildResultV1
			consumeErr := page.UseFundsCanonicalCSVAdmissionRowsV1(func(
				rows []domainevidence.FundsCanonicalCSVHostRowV1,
				next *domainnative.TransactionSourceRowCursorV1,
				complete bool,
			) error {
				defer func() {
					for index := range rows {
						rows[index] = domainevidence.FundsCanonicalCSVHostRowV1{}
					}
					clear(rows)
				}()
				if len(rows) != 1 || next != nil || !complete || page.RowCountV1() != 1 || !page.CompleteV1() {
					return errors.New("production source-row page contract changed")
				}
				var resultErr error
				nativeResult, resultErr = domainevidence.NewFundsCanonicalCSVNativeBuildResultV1(
					materialization, object.DuckDBSHA256, object.DuckDBByteLength, rows,
				)
				return resultErr
			})
			page = domainnative.TransactionSourceRowPageV1{}
			if consumeErr != nil {
				return domainevidence.FundsCanonicalCSVNativeBuildResultV1{}, errors.New("production source-row page consumption failed")
			}
			nativeCallbackCompleted++
			return nativeResult, nil
		},
		func(material domainevidence.FundsCanonicalCSVAdmissionMaterialV1) error {
			var kind domainevidence.FundsCanonicalCSVAdmissionMaterialKindV1
			if materialErr := material.UseExactV1(func(
				observedKind domainevidence.FundsCanonicalCSVAdmissionMaterialKindV1,
				_ string,
				_ []byte,
			) error {
				kind = observedKind
				return nil
			}); materialErr != nil {
				materialWriterFailed = true
				return materialErr
			}
			if materialErr := stores.PutFundsCanonicalCSVAdmissionMaterialV1(ctx, material); materialErr != nil {
				materialWriterFailed = true
				return materialErr
			}
			materialCounts[kind]++
			materialWriterAccepted++
			return nil
		},
	)
	clear(source)
	if err != nil {
		switch {
		case materialWriterFailed:
			t.Fatal("production admission material writer failed closed")
		case nativeCallbackEntered == 0:
			t.Fatal("production admission composer failed before native callback")
		case nativeCallbackCompleted == 0:
			t.Fatal("production admission native callback failed closed")
		default:
			t.Fatal("production admission composer failed after native callback")
		}
	}
	expectedMaterials := map[domainevidence.FundsCanonicalCSVAdmissionMaterialKindV1]int{
		domainevidence.FundsCanonicalCSVMaterialSnapshotManifestV1:     1,
		domainevidence.FundsCanonicalCSVMaterialProducerContentV1:      1,
		domainevidence.FundsCanonicalCSVMaterialRawIntentV1:            1,
		domainevidence.FundsCanonicalCSVMaterialRawManifestV1:          1,
		domainevidence.FundsCanonicalCSVMaterialRawManifestPageV1:      1,
		domainevidence.FundsCanonicalCSVMaterialRawEntryV1:             1,
		domainevidence.FundsCanonicalCSVMaterialRawSourceLocatorV1:     1,
		domainevidence.FundsCanonicalCSVMaterialRawContentRootV1:       1,
		domainevidence.FundsCanonicalCSVMaterialRawContentIndexPageV1:  1,
		domainevidence.FundsCanonicalCSVMaterialRawContentChunkV1:      1,
		domainevidence.FundsCanonicalCSVMaterialParsedConfigurationV1:  1,
		domainevidence.FundsCanonicalCSVMaterialParsedIdentityV1:       1,
		domainevidence.FundsCanonicalCSVMaterialParsedReceiptV1:        1,
		domainevidence.FundsCanonicalCSVMaterialClassificationLedgerV1: 1,
		domainevidence.FundsCanonicalCSVMaterialParsedIndexPageV1:      1,
		domainevidence.FundsCanonicalCSVMaterialParsedPageV1:           1,
		domainevidence.FundsCanonicalCSVMaterialSourceRowRootV1:        1,
		domainevidence.FundsCanonicalCSVMaterialSourceRowIndexPageV1:   1,
		domainevidence.FundsCanonicalCSVMaterialSourceRowPageV1:        1,
		domainevidence.FundsCanonicalCSVMaterialSourceRowRecordV1:      1,
		domainevidence.FundsCanonicalCSVMaterialSourceRowLineageV1:     1,
	}
	if !composerEntered || nativeCallbackEntered != 1 || nativeCallbackCompleted != 1 || materialWriterFailed ||
		materialWriterAccepted != len(expectedMaterials) || len(materialCounts) != len(expectedMaterials) {
		t.Fatal("production admission composer phase observations changed")
	}
	for kind, count := range expectedMaterials {
		if materialCounts[kind] != count {
			t.Fatal("production admission material vocabulary changed")
		}
	}
	if !domainsecurity.IsSHA256Hex(summary.ManifestSHA256) || summary.ManifestByteLength == 0 ||
		!domainsecurity.IsSHA256Hex(summary.ProducerSHA256) || summary.ProducerByteLength == 0 ||
		summary.SourceArtifactSHA256 != sourceArtifactSHA256 || summary.SourceRowCount != 1 {
		t.Fatal("production admission manifest or producer summary reference is invalid")
	}

	phase := &abR2DSV2PhaseTrackerV1{phase: abR2DSV2PhaseFreshHeadBeforeMaterialV1}
	authority := newABR2DSV2AuthorityV1(phase)
	coordinator, err := newABR2DSV2CoordinatorV1(authority, phase)
	if err != nil {
		t.Fatal("dsv2 fresh_head_before_material failed closed")
	}
	service, err := datasetsnapshotapp.NewSealedV2(datasetsnapshotapp.SealedConfigV2{
		InstallationID: coordinator.installationID,
		EnrollmentID:   coordinator.enrollmentID,
		Authority:      authority,
		Coordinator:    coordinator,
		LegacyRecords: &abR2DSV2RecordStoreV1{
			delegate: stores.LegacyRecords,
			phase:    phase,
		},
		Bundles: &abR2DSV2BundleStoreV1{
			delegate: stores.AuthorityBundles,
			phase:    phase,
		},
		Indexes: &abR2DSV2IndexStoreV1{
			delegate: stores.Indexes,
			phase:    phase,
		},
		Materials: &abR2DSV2MaterialReaderV1{
			delegate: stores.Materials,
			phase:    phase,
		},
		Random: bytes.NewReader(bytes.Repeat([]byte{0x5a}, 32)),
	})
	if err != nil {
		t.Fatal("dsv2 fresh_head_before_material failed closed")
	}
	resolved, err := service.AdmitExactV2(ctx, datasetsnapshotapp.AdmitInputV2{
		TenantID:    domainsecurity.LocalTenantID,
		UserID:      domainsecurity.LocalUserID,
		Observation: observation,
		ManifestReference: datasetsnapshotport.ExactMaterialReferenceV2{
			Address: summary.ManifestSHA256, SHA256: summary.ManifestSHA256, ByteLength: summary.ManifestByteLength,
		},
		FundsProducerReference: datasetsnapshotport.ExactMaterialReferenceV2{
			Address: summary.ProducerSHA256, SHA256: summary.ProducerSHA256, ByteLength: summary.ProducerByteLength,
		},
		AcceptedAt: now,
	})
	if err != nil {
		t.Fatal(phase.failureLabelV1())
	}
	selected, err := service.ResolveWitnessedV2(ctx, datasetsnapshotport.ResolveInputV2{
		TenantID:                  domainsecurity.LocalTenantID,
		UserID:                    domainsecurity.LocalUserID,
		Observation:               observation,
		ExpectedDatasetSnapshotID: resolved.Record.DatasetSnapshotID,
	})
	if err != nil {
		t.Fatal("dsv2 post_cas_current_resolve failed closed")
	}
	currentBundle := coordinator.currentBundleV1()
	if !domainsecurity.IsDatasetSnapshotIDV2Syntax(resolved.Record.DatasetSnapshotID) ||
		resolved.Record.DatasetSnapshotID != domainsecurity.DeriveDatasetSnapshotIDV2(resolved.Manifest) ||
		domainsecurity.ValidateDatasetSnapshotAuthorityRecordForFundsProducerContentV2(
			resolved.Record, resolved.Manifest, resolved.FundsProducerContent,
		) != nil ||
		selected.Record != resolved.Record || selected.Manifest != resolved.Manifest ||
		selected.FundsProducerContent != resolved.FundsProducerContent ||
		phase.freshHeadObservations == 0 || phase.materialReads == 0 || phase.sealedRecordSigns < 2 ||
		phase.bundlePuts != 1 || phase.bundleResolves == 0 ||
		phase.indexPuts != 1 || phase.indexResolves == 0 ||
		phase.witnessCASCalls != 1 || phase.postCASReads == 0 ||
		coordinator.successfulAdvancesV1() != 1 ||
		currentBundle.Generation != 2 || currentBundle.DatasetSnapshotCount != 1 ||
		currentBundle.DatasetSnapshotIndexDigest == domainsecurity.DatasetSnapshotIndexGenesisDigestV1() {
		t.Fatal("dsv2 post_cas_current_resolve failed closed")
	}

	guardedRunner := &abR2CurrentnessNoExecutionRunnerV1{delegate: owner.runner}
	owner.runner = guardedRunner
	if err := owner.ValidateCurrentDataEngine(context.Background()); err != nil {
		t.Fatal("current packaged data-engine did not validate")
	}
	runtimeRoot, err := PackagedRuntimeRoot(executable)
	if err != nil {
		t.Fatal("resolve packaged runtime root")
	}
	componentPath := filepath.Join(runtimeRoot, "analytix-data-engine")
	componentInfo, err := os.Stat(componentPath)
	if err != nil || componentInfo.Size() <= 0 {
		t.Fatal("stat packaged data-engine")
	}
	componentBody, err := os.ReadFile(componentPath)
	if err != nil || len(componentBody) == 0 {
		t.Fatal("read packaged data-engine for exact restoration")
	}
	restored := false
	t.Cleanup(func() {
		if !restored {
			_ = abR2RestorePackagedComponentV1(componentPath, componentBody, componentInfo.Mode().Perm())
		}
		clear(componentBody)
	})
	componentFile, err := os.OpenFile(componentPath, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal("open packaged data-engine damage stimulus")
	}
	damagedByte := []byte{componentBody[0] ^ 0xff}
	_, writeErr := componentFile.WriteAt(damagedByte, 0)
	clear(damagedByte)
	syncErr := componentFile.Sync()
	closeErr := componentFile.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		t.Fatal("apply packaged data-engine damage stimulus")
	}
	if err := abR2RestorePackagedComponentV1(componentPath, componentBody, componentInfo.Mode().Perm()); err != nil {
		t.Fatal("restore packaged data-engine bytes")
	}
	restored = true
	restoredBody, err := os.ReadFile(componentPath)
	if err != nil || !bytes.Equal(restoredBody, componentBody) {
		clear(restoredBody)
		t.Fatal("packaged data-engine restoration changed bytes")
	}
	clear(restoredBody)
	if err := owner.ValidateCurrentDataEngine(context.Background()); !errors.Is(err, ErrTrust) {
		t.Fatal("stale opened data-engine remained current")
	}
	if guardedRunner.executeCalls != 0 {
		t.Fatal("data-engine currentness validation executed a native component")
	}
}

type abR2CurrentnessNoExecutionRunnerV1 struct {
	delegate     closeRunner
	executeCalls int
}

func (runner *abR2CurrentnessNoExecutionRunnerV1) Execute(
	ctx context.Context,
	request domainnative.Request,
) (domainnative.Result, error) {
	runner.executeCalls++
	return runner.delegate.Execute(ctx, request)
}

func (runner *abR2CurrentnessNoExecutionRunnerV1) Close() error {
	return runner.delegate.Close()
}

func abR2RestorePackagedComponentV1(path string, body []byte, mode os.FileMode) (returnErr error) {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".ab-r2-component-restore-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		if temporary != nil {
			if closeErr := temporary.Close(); returnErr == nil && closeErr != nil {
				returnErr = closeErr
			}
		}
		if temporaryPath != "" {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	temporary = nil
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	temporaryPath = ""
	return nil
}

type abR2DSV2PhaseV1 string

const (
	abR2DSV2PhaseFreshHeadBeforeMaterialV1  abR2DSV2PhaseV1 = "fresh_head_before_material"
	abR2DSV2PhaseExactMaterialGraphVerifyV1 abR2DSV2PhaseV1 = "exact_material_graph_verify"
	abR2DSV2PhaseSealedRecordIssueV1        abR2DSV2PhaseV1 = "sealed_record_issue"
	abR2DSV2PhaseBundlePersistReadbackV1    abR2DSV2PhaseV1 = "bundle_persist_readback"
	abR2DSV2PhaseIndexPersistReadbackV1     abR2DSV2PhaseV1 = "index_persist_readback"
	abR2DSV2PhaseWitnessCASAdvanceV1        abR2DSV2PhaseV1 = "witness_cas_advance"
	abR2DSV2PhasePostCASCurrentResolveV1    abR2DSV2PhaseV1 = "post_cas_current_resolve"
)

type abR2DSV2PhaseTrackerV1 struct {
	phase                 abR2DSV2PhaseV1
	freshHeadObservations int
	materialReads         int
	sealedRecordSigns     int
	bundlePuts            int
	bundleResolves        int
	indexPuts             int
	indexResolves         int
	witnessCASCalls       int
	postCASReads          int
}

func (phase *abR2DSV2PhaseTrackerV1) failureLabelV1() string {
	switch phase.phase {
	case abR2DSV2PhaseFreshHeadBeforeMaterialV1:
		return "dsv2 fresh_head_before_material failed closed"
	case abR2DSV2PhaseExactMaterialGraphVerifyV1:
		return "dsv2 exact_material_graph_verify failed closed"
	case abR2DSV2PhaseSealedRecordIssueV1:
		return "dsv2 sealed_record_issue failed closed"
	case abR2DSV2PhaseBundlePersistReadbackV1:
		return "dsv2 bundle_persist_readback failed closed"
	case abR2DSV2PhaseIndexPersistReadbackV1:
		return "dsv2 index_persist_readback failed closed"
	case abR2DSV2PhaseWitnessCASAdvanceV1:
		return "dsv2 witness_cas_advance failed closed"
	case abR2DSV2PhasePostCASCurrentResolveV1:
		return "dsv2 post_cas_current_resolve failed closed"
	default:
		return "dsv2 fresh_head_before_material failed closed"
	}
}

type abR2DSV2AuthorityV1 struct {
	private ed25519.PrivateKey
	public  ed25519.PublicKey
	phase   *abR2DSV2PhaseTrackerV1
}

func newABR2DSV2AuthorityV1(phase *abR2DSV2PhaseTrackerV1) *abR2DSV2AuthorityV1 {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x32}, ed25519.SeedSize))
	return &abR2DSV2AuthorityV1{
		private: privateKey,
		public:  privateKey.Public().(ed25519.PublicKey),
		phase:   phase,
	}
}

func (authority *abR2DSV2AuthorityV1) KeyID() string {
	return domainsecurity.SHA256Hex(authority.public)
}

func (authority *abR2DSV2AuthorityV1) PublicKey() []byte {
	return append([]byte(nil), authority.public...)
}

func (authority *abR2DSV2AuthorityV1) Sign(ctx context.Context, body []byte) ([]byte, error) {
	if authority == nil || ctx == nil || authority.phase == nil {
		return nil, errors.New("dsv2 test signing authority is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if authority.phase.materialReads > 0 && authority.phase.bundlePuts == 0 {
		authority.phase.phase = abR2DSV2PhaseSealedRecordIssueV1
		authority.phase.sealedRecordSigns++
	}
	return ed25519.Sign(authority.private, body), nil
}

func (authority *abR2DSV2AuthorityV1) VerifyTrusted(
	ctx context.Context,
	keyID string,
	publicKey []byte,
	body []byte,
	signature []byte,
) error {
	if authority == nil || ctx == nil {
		return errors.New("dsv2 test verification authority is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if keyID != authority.KeyID() || !bytes.Equal(publicKey, authority.public) ||
		!ed25519.Verify(authority.public, body, signature) {
		return errors.New("dsv2 test signature is untrusted")
	}
	return nil
}

type abR2DSV2CoordinatorV1 struct {
	mu                 sync.Mutex
	authority          *abR2DSV2AuthorityV1
	phase              *abR2DSV2PhaseTrackerV1
	witnessPrivate     ed25519.PrivateKey
	witnessPublic      ed25519.PublicKey
	installationID     string
	enrollmentID       string
	bundle             domainevidence.EvidenceAuthorityBundleV1
	checkpoint         domainsecurity.MonotonicHeadCheckpointV1
	observeCount       uint64
	successfulAdvances int
}

func newABR2DSV2CoordinatorV1(
	authority *abR2DSV2AuthorityV1,
	phase *abR2DSV2PhaseTrackerV1,
) (*abR2DSV2CoordinatorV1, error) {
	if authority == nil || phase == nil {
		return nil, errors.New("dsv2 test coordinator is unavailable")
	}
	witnessPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x72}, ed25519.SeedSize))
	coordinator := &abR2DSV2CoordinatorV1{
		authority:      authority,
		phase:          phase,
		witnessPrivate: witnessPrivate,
		witnessPublic:  witnessPrivate.Public().(ed25519.PublicKey),
		installationID: domainsecurity.SHA256Hex([]byte("ab-r2-dsv2-installation")),
		enrollmentID:   domainsecurity.SHA256Hex([]byte("ab-r2-dsv2-enrollment")),
	}
	bundle, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID:              coordinator.installationID,
		EnrollmentID:                coordinator.enrollmentID,
		Generation:                  1,
		MutationID:                  domainsecurity.SHA256Hex([]byte("ab-r2-dsv2-genesis-bundle")),
		DatasetSnapshotIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
		EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
		AuthorityKeyID:              authority.KeyID(),
		AuthorityPublicKey:          authority.PublicKey(),
	}, func(message []byte) ([]byte, error) {
		return authority.Sign(context.Background(), message)
	})
	if err != nil {
		return nil, err
	}
	checkpoint, err := coordinator.checkpointForV1(bundle, domainsecurity.MonotonicHeadCheckpointV1{})
	if err != nil {
		return nil, err
	}
	coordinator.bundle = bundle
	coordinator.checkpoint = checkpoint
	return coordinator, nil
}

func (coordinator *abR2DSV2CoordinatorV1) ObserveFresh(
	ctx context.Context,
) (evidenceauthorityport.FreshHead, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	coordinator.phase.freshHeadObservations++
	if coordinator.phase.witnessCASCalls > 0 {
		coordinator.phase.phase = abR2DSV2PhasePostCASCurrentResolveV1
	} else if coordinator.phase.materialReads == 0 {
		coordinator.phase.phase = abR2DSV2PhaseFreshHeadBeforeMaterialV1
	}
	return coordinator.observeLockedV1()
}

func (coordinator *abR2DSV2CoordinatorV1) AdvanceDatasetSnapshot(
	ctx context.Context,
	input evidenceauthorityport.DatasetAdvanceInput,
) (evidenceauthorityport.FreshHead, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.phase.phase = abR2DSV2PhaseWitnessCASAdvanceV1
	coordinator.phase.witnessCASCalls++
	if err := ctx.Err(); err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	if input.ExpectedBundleDigest != coordinator.bundle.RecordDigest {
		return evidenceauthorityport.FreshHead{}, monotonicheadport.ErrCASConflict
	}
	if !domainsecurity.IsSHA256Hex(input.NextIndexDigest) {
		return evidenceauthorityport.FreshHead{}, errors.New("dsv2 test next index is invalid")
	}
	previous := coordinator.bundle
	next, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID:              previous.InstallationID,
		EnrollmentID:                previous.EnrollmentID,
		Generation:                  previous.Generation + 1,
		PreviousBundleDigest:        previous.RecordDigest,
		MutationID:                  domainsecurity.SHA256Hex([]byte("ab-r2-dsv2-bundle-generation-2")),
		DatasetSnapshotIndexDigest:  input.NextIndexDigest,
		DatasetSnapshotCount:        previous.DatasetSnapshotCount + 1,
		EvidenceRegistryIndexDigest: previous.EvidenceRegistryIndexDigest,
		EvidenceRegistryCount:       previous.EvidenceRegistryCount,
		PublicationIndexDigest:      previous.PublicationIndexDigest,
		PublicationCount:            previous.PublicationCount,
		AuthorityKeyID:              coordinator.authority.KeyID(),
		AuthorityPublicKey:          coordinator.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) {
		return coordinator.authority.Sign(ctx, message)
	})
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	checkpoint, err := coordinator.checkpointForV1(next, coordinator.checkpoint)
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	coordinator.bundle = next
	coordinator.checkpoint = checkpoint
	coordinator.successfulAdvances++
	return coordinator.observeLockedV1()
}

func (coordinator *abR2DSV2CoordinatorV1) observeLockedV1() (evidenceauthorityport.FreshHead, error) {
	coordinator.observeCount++
	request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID:     coordinator.installationID,
		EnrollmentID:       coordinator.enrollmentID,
		Namespace:          domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1,
		ChallengeNonce:     domainsecurity.SHA256Hex([]byte(fmt.Sprintf("ab-r2-dsv2-challenge:%d", coordinator.observeCount))),
		AuthorityKeyID:     coordinator.authority.KeyID(),
		AuthorityPublicKey: coordinator.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) {
		return coordinator.authority.Sign(context.Background(), message)
	})
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(
		request,
		coordinator.checkpoint,
		func(message []byte) ([]byte, error) {
			return ed25519.Sign(coordinator.witnessPrivate, message), nil
		},
	)
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	return evidenceauthorityport.FreshHead{
		HasBundle:   true,
		Bundle:      coordinator.bundle,
		Request:     request,
		Observation: observation,
	}, nil
}

func (coordinator *abR2DSV2CoordinatorV1) checkpointForV1(
	bundle domainevidence.EvidenceAuthorityBundleV1,
	previous domainsecurity.MonotonicHeadCheckpointV1,
) (domainsecurity.MonotonicHeadCheckpointV1, error) {
	previousState := domainsecurity.SHA256Hex([]byte("ab-r2-dsv2-enrolled-state"))
	previousCheckpoint := domainsecurity.SHA256Hex([]byte("ab-r2-dsv2-enrolled-checkpoint"))
	if previous.Generation != 0 {
		previousState = previous.CurrentStateDigest
		previousCheckpoint = previous.CheckpointDigest
	}
	return domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID:           coordinator.installationID,
		EnrollmentID:             coordinator.enrollmentID,
		Namespace:                domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1,
		Generation:               bundle.Generation,
		CurrentStateDigest:       bundle.RecordDigest,
		PreviousStateDigest:      previousState,
		PreviousCheckpointDigest: previousCheckpoint,
		FenceNonce:               domainsecurity.SHA256Hex([]byte(fmt.Sprintf("ab-r2-dsv2-fence:%d", bundle.Generation))),
		MutationID:               bundle.MutationID,
		WitnessKeyID:             domainsecurity.SHA256Hex(coordinator.witnessPublic),
		WitnessPublicKey:         coordinator.witnessPublic,
	}, func(message []byte) ([]byte, error) {
		return ed25519.Sign(coordinator.witnessPrivate, message), nil
	})
}

func (coordinator *abR2DSV2CoordinatorV1) currentBundleV1() domainevidence.EvidenceAuthorityBundleV1 {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	return coordinator.bundle
}

func (coordinator *abR2DSV2CoordinatorV1) successfulAdvancesV1() int {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	return coordinator.successfulAdvances
}

type abR2DSV2MaterialReaderV1 struct {
	delegate datasetsnapshotport.AdmissionMaterialReaderV2
	phase    *abR2DSV2PhaseTrackerV1
}

func (reader *abR2DSV2MaterialReaderV1) ResolveExact(
	ctx context.Context,
	kind datasetsnapshotport.MaterialKindV2,
	reference datasetsnapshotport.ExactMaterialReferenceV2,
) ([]byte, error) {
	reader.phase.materialReads++
	if reader.phase.witnessCASCalls > 0 {
		reader.phase.phase = abR2DSV2PhasePostCASCurrentResolveV1
		reader.phase.postCASReads++
	} else {
		reader.phase.phase = abR2DSV2PhaseExactMaterialGraphVerifyV1
	}
	return reader.delegate.ResolveExact(ctx, kind, reference)
}

type abR2DSV2BundleStoreV1 struct {
	delegate datasetsnapshotport.AuthorityBundleStoreV2
	phase    *abR2DSV2PhaseTrackerV1
}

func (store *abR2DSV2BundleStoreV1) PutIfAbsent(
	ctx context.Context,
	bundle datasetsnapshotport.AuthorityBundleV2,
) error {
	store.phase.phase = abR2DSV2PhaseBundlePersistReadbackV1
	store.phase.bundlePuts++
	return store.delegate.PutIfAbsent(ctx, bundle)
}

func (store *abR2DSV2BundleStoreV1) Resolve(
	ctx context.Context,
	recordDigest string,
) (datasetsnapshotport.AuthorityBundleV2, error) {
	store.phase.bundleResolves++
	if store.phase.witnessCASCalls > 0 {
		store.phase.phase = abR2DSV2PhasePostCASCurrentResolveV1
		store.phase.postCASReads++
	} else {
		store.phase.phase = abR2DSV2PhaseBundlePersistReadbackV1
	}
	return store.delegate.Resolve(ctx, recordDigest)
}

type abR2DSV2IndexStoreV1 struct {
	delegate datasetsnapshotport.IndexStore
	phase    *abR2DSV2PhaseTrackerV1
}

func (store *abR2DSV2IndexStoreV1) PutIfAbsent(
	ctx context.Context,
	index domainsecurity.DatasetSnapshotIndexV1,
) error {
	store.phase.phase = abR2DSV2PhaseIndexPersistReadbackV1
	store.phase.indexPuts++
	return store.delegate.PutIfAbsent(ctx, index)
}

func (store *abR2DSV2IndexStoreV1) Resolve(
	ctx context.Context,
	indexDigest string,
) (domainsecurity.DatasetSnapshotIndexV1, error) {
	store.phase.indexResolves++
	if store.phase.witnessCASCalls > 0 {
		store.phase.phase = abR2DSV2PhasePostCASCurrentResolveV1
		store.phase.postCASReads++
	} else {
		store.phase.phase = abR2DSV2PhaseIndexPersistReadbackV1
	}
	return store.delegate.Resolve(ctx, indexDigest)
}

type abR2DSV2RecordStoreV1 struct {
	delegate datasetsnapshotport.RecordStore
	phase    *abR2DSV2PhaseTrackerV1
}

func (store *abR2DSV2RecordStoreV1) PutIfAbsent(
	ctx context.Context,
	record domainsecurity.DatasetSnapshotAuthorityRecordV1,
) error {
	return store.delegate.PutIfAbsent(ctx, record)
}

func (store *abR2DSV2RecordStoreV1) Resolve(
	ctx context.Context,
	recordDigest string,
) (domainsecurity.DatasetSnapshotAuthorityRecordV1, error) {
	if store.phase.witnessCASCalls > 0 {
		store.phase.phase = abR2DSV2PhasePostCASCurrentResolveV1
		store.phase.postCASReads++
	}
	return store.delegate.Resolve(ctx, recordDigest)
}

func privateRuntimeInheritedGroupFixture(t *testing.T) (string, unix.Stat_t) {
	t.Helper()
	profile, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil || os.Chmod(profile, 0o700) != nil {
		t.Fatal("prepare private group fixture")
	}
	parent, err := os.Open(profile)
	if err != nil {
		t.Fatal("open private group fixture")
	}
	defer parent.Close()
	groups, err := os.Getgroups()
	if err != nil {
		t.Fatal("read available fixture groups")
	}
	changed := false
	for _, group := range groups {
		if group != os.Getegid() && unix.Fchown(int(parent.Fd()), -1, group) == nil {
			changed = true
			break
		}
	}
	if !changed {
		t.Skip("inherited-group counterexample unavailable: no usable non-effective group")
	}
	var before unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &before) != nil || before.Uid != uint32(os.Geteuid()) ||
		before.Gid == uint32(os.Getegid()) || before.Mode&0o7777 != 0o700 {
		t.Fatal("group fixture did not establish the counterexample")
	}
	return profile, before
}

func TestPrivateRuntimeRootsNormalizeInheritedGroup(t *testing.T) {
	profile, before := privateRuntimeInheritedGroupFixture(t)
	roots, err := createPrivateRoots(profile)
	if err != nil {
		t.Fatal("create private roots from owned non-effective-group parent")
	}
	t.Cleanup(func() {
		if roots.Close() != nil {
			t.Error("close private group-normalized roots")
		}
		var after unix.Stat_t
		if unix.Lstat(profile, &after) != nil || before.Dev != after.Dev || before.Ino != after.Ino ||
			before.Uid != after.Uid || before.Gid != after.Gid || before.Mode != after.Mode ||
			before.Nlink != after.Nlink || before.Flags != after.Flags {
			t.Error("existing profile identity or security metadata changed")
		}
		if entries, err := os.ReadDir(profile); err != nil || len(entries) != 0 {
			t.Error("private roots survived exact cleanup")
		}
	})
	fds := []int{roots.runRoot}
	for _, name := range privateRuntimeChildren {
		fds = append(fds, int(roots.authorityFile(name).Fd()))
	}
	for _, fd := range fds {
		var info unix.Stat_t
		if unix.Fstat(fd, &info) != nil || info.Mode&unix.S_IFMT != unix.S_IFDIR ||
			info.Uid != uint32(os.Geteuid()) || info.Gid != uint32(os.Getegid()) ||
			info.Mode&0o7777 != 0o700 {
			t.Error("new runtime directory did not satisfy effective-group precondition")
		}
	}
}

func TestPrivateRuntimeRootsGroupNormalizationFailure(t *testing.T) {
	for _, failure := range []string{"denied", "unchanged", "mode-drift"} {
		t.Run(failure, func(t *testing.T) {
			profile, before := privateRuntimeInheritedGroupFixture(t)
			calledFD := -1
			roots, err := createPrivateRootsWithFchown(profile, func(fd, owner, group int) error {
				calledFD = fd
				if owner != -1 || group != os.Getegid() {
					t.Fatal("normalization attempted an unexpected ownership change")
				}
				switch failure {
				case "denied":
					return unix.EPERM
				case "mode-drift":
					if unix.Fchown(fd, owner, group) != nil || unix.Fchmod(fd, 0o600) != nil {
						t.Fatal("prepare normalization mode drift")
					}
				}
				return nil
			})
			want := ErrTrust
			if failure == "denied" {
				want = unix.EPERM
			}
			if roots != nil || !errors.Is(err, want) || calledFD < 0 {
				t.Fatal("failed normalization exposed private roots or lost its cause")
			}
			if _, err := unix.FcntlInt(uintptr(calledFD), unix.F_GETFD, 0); !errors.Is(err, unix.EBADF) {
				t.Fatal("failed normalization retained its directory descriptor")
			}
			var after unix.Stat_t
			if unix.Lstat(profile, &after) != nil || before.Dev != after.Dev || before.Ino != after.Ino ||
				before.Uid != after.Uid || before.Gid != after.Gid || before.Mode != after.Mode ||
				before.Nlink != after.Nlink || before.Flags != after.Flags {
				t.Fatal("failed normalization modified the existing profile")
			}
			if entries, err := os.ReadDir(profile); err != nil || len(entries) != 0 {
				t.Fatal("failed normalization left a private root")
			}
		})
	}
}

func TestNewPrivateRuntimeDirectoryGroupNormalizationPreservesIdentity(t *testing.T) {
	profile, _ := privateRuntimeInheritedGroupFixture(t)
	parent, err := os.Open(profile)
	if err != nil {
		t.Fatal("open normalization fixture parent")
	}
	defer parent.Close()
	const name = "new-directory"
	if unix.Mkdirat(int(parent.Fd()), name, 0o700) != nil {
		t.Fatal("create normalization fixture child")
	}
	defer unix.Unlinkat(int(parent.Fd()), name, unix.AT_REMOVEDIR)
	fd, err := openPrivateChildDirectory(int(parent.Fd()), name)
	if err != nil {
		t.Fatal("open normalization fixture child")
	}
	defer unix.Close(fd)
	const attribute = "com.analytix.group-normalization-fixture"
	value := []byte("test-only-preserved-value")
	if unix.Fsetxattr(fd, attribute, value, 0) != nil || unix.Fchflags(fd, unix.UF_HIDDEN) != nil {
		t.Fatal("prepare unrelated security metadata")
	}
	var before unix.Stat_t
	if unix.Fstat(fd, &before) != nil || before.Gid == uint32(os.Getegid()) {
		t.Fatal("new child did not inherit the non-effective group")
	}
	if normalizeNewPrivateDirectoryGroup(int(parent.Fd()), name, fd, unix.Fchown) != nil {
		t.Fatal("normalize new directory group")
	}
	var after unix.Stat_t
	if unix.Fstat(fd, &after) != nil || before.Dev != after.Dev || before.Ino != after.Ino ||
		before.Uid != after.Uid || before.Mode != after.Mode || before.Nlink != after.Nlink ||
		before.Flags != after.Flags || after.Gid != uint32(os.Getegid()) {
		t.Fatal("normalization changed more than the new directory group")
	}
	got := make([]byte, len(value))
	if n, err := unix.Fgetxattr(fd, attribute, got); err != nil || n != len(value) || !bytes.Equal(got, value) {
		t.Fatal("normalization modified unrelated extended metadata")
	}
	if normalizeNewPrivateDirectoryGroup(int(parent.Fd()), name, fd, func(int, int, int) error {
		t.Error("already-correct group invoked ownership mutation")
		return unix.EPERM
	}) != nil {
		t.Fatal("already-correct group rejected")
	}
	if err := normalizeNewPrivateDirectoryGroup(int(parent.Fd()), "unbound", fd, unix.Fchown); !errors.Is(err, ErrTrust) {
		t.Fatal("unbound new directory accepted for normalization")
	}
}

func TestNewPrivateRuntimeDirectoryGroupFailureCleansExactChild(t *testing.T) {
	profile, _ := privateRuntimeInheritedGroupFixture(t)
	parent, err := os.Open(profile)
	if err != nil {
		t.Fatal("open failed-child fixture parent")
	}
	defer parent.Close()
	calledFD := -1
	child, err := createPrivateChildDirectory(int(parent.Fd()), "failed-child", func(fd, owner, group int) error {
		calledFD = fd
		return unix.EPERM
	})
	if child != nil || !errors.Is(err, unix.EPERM) || calledFD < 0 {
		t.Fatal("failed child normalization exposed authority or lost its cause")
	}
	if _, err := unix.FcntlInt(uintptr(calledFD), unix.F_GETFD, 0); !errors.Is(err, unix.EBADF) {
		t.Fatal("failed child normalization retained a descriptor")
	}
	if entries, err := os.ReadDir(profile); err != nil || len(entries) != 0 {
		t.Fatal("failed child normalization left its directory")
	}
}

func TestPrivateRuntimeRootsAreOwnedAndRemoved(t *testing.T) {
	profileRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(profileRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	roots, err := createPrivateRoots(profileRoot)
	if err != nil {
		t.Fatalf("create private roots: %v", err)
	}
	runPath := filepath.Join(profileRoot, roots.runName)
	runInfo, err := os.Lstat(runPath)
	if err != nil || !runInfo.IsDir() || runInfo.Mode().Perm() != 0o700 ||
		filepath.Dir(runPath) != profileRoot || !strings.HasPrefix(filepath.Base(runPath), privateRuntimePrefix) {
		t.Fatalf("process-local runtime root escaped exact profile root: path=%q info=%v err=%v", runPath, runInfo, err)
	}
	if _, err := os.Lstat(filepath.Join(profileRoot, "private")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("process-local runtime roots created a persistent private substrate: %v", err)
	}
	for _, child := range privateRuntimeChildren {
		path := roots.childPath(child)
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Fatalf("private child %q info=%v err=%v", path, info, statErr)
		}
	}
	if err := roots.Close(); err != nil {
		t.Fatalf("close private roots: %v", err)
	}
	if _, err := os.Lstat(runPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("private runtime root survived close: %v", err)
	}
	if err := roots.Close(); err != nil {
		t.Fatalf("repeat private root close: %v", err)
	}
}

func TestPrivateRuntimeRootsRejectUnsafeProfileModeBeforeCreation(t *testing.T) {
	profileRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(profileRoot, 0o770); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir(profileRoot)
	if err != nil {
		t.Fatal(err)
	}
	if roots, err := createPrivateRoots(profileRoot); err == nil || roots != nil {
		t.Fatalf("unsafe profile mode survived: roots=%#v err=%v", roots, err)
	}
	after, err := os.ReadDir(profileRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("rejected unsafe profile changed entries: before=%d after=%d", len(before), len(after))
	}
}

func TestPrivateRuntimeRootsRejectWrongOwnerBeforeCreation(t *testing.T) {
	const rootOwnedDirectory = "/private"
	var state unix.Stat_t
	if err := unix.Stat(rootOwnedDirectory, &state); err != nil {
		t.Fatal(err)
	}
	if state.Uid == uint32(os.Geteuid()) || state.Mode&0o022 != 0 {
		t.Skip("host does not expose the expected root-owned non-writable Darwin directory")
	}
	fd, _, err := openPrivateDirectoryPath(rootOwnedDirectory)
	if fd >= 0 {
		_ = unix.Close(fd)
	}
	if err == nil {
		t.Fatal("wrong-owner profile root survived")
	}
}

func TestOpenProductionWithoutEmbeddedTrustHasNoFilesystemEffects(t *testing.T) {
	available, trustErr := embeddedTrustState(nativecomponentregistry.EmbeddedTrust())
	if available || trustErr != nil {
		t.Fatalf("ordinary test binary unexpectedly contains release trust: available=%t err=%v", available, trustErr)
	}
	dataDir := filepath.Join(t.TempDir(), "must-not-be-created")
	owner, err := OpenProduction(dataDir)
	if owner != nil || err != ErrUnavailable {
		t.Fatalf("untrusted production native host = %#v err=%v, want exact unavailable", owner, err)
	}
	if _, statErr := os.Lstat(dataDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("trust rejection touched data directory: %v", statErr)
	}
}

func TestPackagedDarwinRuntimeRootRequiresAppResourcesLayout(t *testing.T) {
	trusted := filepath.Join(string(filepath.Separator), "Applications", "Analytix.app", "Contents", "Resources", "runtime-go", "bin", "runtime-server")
	root, err := packagedDarwinRuntimeRoot(trusted)
	if err != nil || root != filepath.Join(string(filepath.Separator), "Applications", "Analytix.app", "Contents", "Resources", "runtime") {
		t.Fatalf("trusted package root = %q err=%v", root, err)
	}
	for _, path := range []string{
		filepath.Join(string(filepath.Separator), "tmp", "runtime-go", "bin", "runtime-server"),
		filepath.Join(string(filepath.Separator), "Applications", "Analytix", "Contents", "Resources", "runtime-go", "bin", "runtime-server"),
		filepath.Join(string(filepath.Separator), "Applications", "Analytix.app", "resources", "runtime-go", "bin", "runtime-server"),
	} {
		if _, err := packagedDarwinRuntimeRoot(path); !errors.Is(err, ErrTrust) {
			t.Fatalf("untrusted Darwin package layout %q survived: %v", path, err)
		}
	}
}

func TestPrivateRuntimeRootsRejectSymlinkDataDirectoryBeforeCreation(t *testing.T) {
	target, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "data-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	if roots, err := createPrivateRoots(link); err == nil || roots != nil {
		t.Fatalf("symlink data directory survived: roots=%#v err=%v", roots, err)
	}
	after, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("rejected symlink changed target entries: before=%d after=%d", len(before), len(after))
	}
}

func TestPrivateRuntimeRootsRejectRunRootReplacementWithoutDeletingReplacement(t *testing.T) {
	dataDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	roots, err := createPrivateRoots(dataDir)
	if err != nil {
		t.Fatalf("create private roots: %v", err)
	}
	runPath := filepath.Join(dataDir, roots.runName)
	movedPath := runPath + "-retained"
	if err := os.Rename(runPath, movedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(runPath, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(runPath, "replacement-marker")
	if err := os.WriteFile(marker, []byte("must-survive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := roots.Close(); !errors.Is(err, ErrLifecycle) {
		t.Fatalf("replacement root close error = %v, want %v", err, ErrLifecycle)
	}
	if payload, err := os.ReadFile(marker); err != nil || string(payload) != "must-survive" {
		t.Fatalf("replacement root was mutated: payload=%q err=%v", payload, err)
	}
	for _, child := range privateRuntimeChildren {
		if _, err := os.Stat(filepath.Join(movedPath, child)); err != nil {
			t.Fatalf("retained root child %q was mutated before identity validation: %v", child, err)
		}
	}
	if err := os.RemoveAll(runPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(movedPath, runPath); err != nil {
		t.Fatal(err)
	}
	if err := roots.Close(); err != nil {
		t.Fatalf("close restored roots: %v", err)
	}
}
