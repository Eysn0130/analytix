//go:build darwin && !analytix_prod

package nativecomponentrunner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	nativecomponentregistry "analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	"golang.org/x/sys/unix"
)

func TestRunnerFundsCanonicalCSVRealDataEngineFD3FD5InstallerSeam(t *testing.T) {
	binaryPath := strings.TrimSpace(os.Getenv("ANALYTIX_TEST_REAL_DATA_ENGINE"))
	if binaryPath == "" {
		t.Skip("real canonical CSV seam requires an explicit isolated data-engine binary")
	}
	if !filepath.IsAbs(binaryPath) || !strings.HasPrefix(filepath.Clean(binaryPath), "/Volumes/AnalytixCache/") {
		t.Fatalf("real data-engine escaped the trusted cache volume: %q", binaryPath)
	}
	canonicalBinary, err := filepath.EvalSymlinks(binaryPath)
	if err != nil || canonicalBinary != filepath.Clean(binaryPath) {
		t.Fatalf("real data-engine path is not canonical: path=%q err=%v", binaryPath, err)
	}
	binaryBody, err := os.ReadFile(canonicalBinary)
	if err != nil || len(binaryBody) == 0 {
		t.Fatalf("read real data-engine: %v", err)
	}
	binaryDigest := sha256.Sum256(binaryBody)
	binaryDigestText := hex.EncodeToString(binaryDigest[:])
	registryDigest := domainsecurity.SHA256Hex([]byte("real-canonical-csv-data-engine-seam-registry"))
	registry := &fakeExecutionRegistry{
		digest:     registryDigest,
		executable: canonicalBinary,
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

	sourceBody := realCanonicalCSVSourceV1(t)
	sourceDigest := sha256.Sum256(sourceBody)
	sourceSHA256 := hex.EncodeToString(sourceDigest[:])
	const caseID = "case-real-canonical-seam"
	binding, err := domainsecurity.NewDatasetSnapshotBindingKeyV1(
		domainsecurity.DatasetSnapshotBindingKeyInputV1{
			TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
			WorkspaceRealPath: "/cases/" + caseID, CaseID: caseID,
			CaseBindingHash: strings.Repeat("5", 64), BindingObservationDigest: strings.Repeat("6", 64),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := domainnative.NewFundsCanonicalCSVSnapshotBuildArgumentsV1(
		domainnative.FundsCanonicalCSVSnapshotBuildInputV1{
			Binding: binding, PrivateImportFileID: "0123456789abcdef0123", SourceRevision: 7,
			RawArtifactManifestSHA256: strings.Repeat("a", 64), SourceArtifactSHA256: sourceSHA256,
			SourceArtifactByteLength: uint64(len(sourceBody)), SourceRowCount: 1,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	installer := &realCanonicalCSVInstallerV1{}
	result, object, disposition, err := runner.BuildFundsCanonicalCSVSnapshot(
		context.Background(), arguments, bytes.NewReader(sourceBody), installer,
	)
	if err != nil {
		t.Fatalf("real canonical CSV runner seam: %v", err)
	}
	sha256Text, byteLength, rowCount, maxTxnTS, maxID := result.SourceStatsV1()
	materialization, materializationErr := result.MaterializationV1()
	if materializationErr != nil || sha256Text != sourceSHA256 || byteLength != uint64(len(sourceBody)) ||
		rowCount != 1 || maxTxnTS != "2026-01-02 03:04:05" || maxID != 1 ||
		disposition != fundsquerysourceport.ImmutableSnapshotInstallCreatedV1 || installer.calls != 1 ||
		domainfundsquerysource.ValidateImmutableSnapshotObjectV1(object) != nil ||
		object.CaseID != caseID || object.DuckDBSHA256 != installer.sha256 ||
		object.DuckDBByteLength != installer.byteLength ||
		materialization.RawArtifactManifestSHA256 != strings.Repeat("a", 64) ||
		registry.Acquisitions() != 1 || runner.session != nil || runner.sessionBinding != "" {
		t.Fatalf(
			"real canonical seam binding drifted: result=%s object=%#v disposition=%q installs=%d materialization_err=%v acquisitions=%d session=%T",
			result.String(), object, disposition, installer.calls, materializationErr,
			registry.Acquisitions(), runner.session,
		)
	}
	// The descriptor needs only the current witnessed security context. Do not
	// manufacture an unrelated account-flow subject/Grant for this local-display
	// and cleaning native seam.
	request := darwinRunnerRequest(t, caseID, 7)
	descriptor := darwinRunnerAccountFlowDescriptorWithProducerAndProfile(
		t,
		request,
		installer.body,
		materialization.ProducerContentID,
		materialization.ProducerContentManifestSHA256,
		domainfundsquerysource.FixedFundsLocalDisplayQueryProfileDigestV1(),
	)
	previewArguments, err := domainnative.NewDirectSourcePreviewArgumentsV1(
		descriptor,
		[]domainnative.DirectSourcePreviewFieldV1{
			domainnative.DirectSourcePreviewFieldAccountV1,
			domainnative.DirectSourcePreviewFieldAmountTextV1,
			domainnative.DirectSourcePreviewFieldAccountNameV1,
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
		descriptor,
		&exactBytesReadLease{body: installer.body},
	)
	if err != nil || len(preview.Rows) != 1 || len(preview.Rows[0].Cells) != 3 ||
		registry.Acquisitions() != 2 || runner.session != nil || runner.sessionBinding != "" {
		t.Fatalf("real canonical direct preview did not preserve the one-shot typed seam")
	}
	wantExact := []string{"6222021234567890123", "12.5", "完整敏感姓名"}
	for index, cell := range preview.Rows[0].Cells {
		matched := false
		if useErr := cell.UseExactV1(func(value string) error {
			matched = value == wantExact[index]
			return nil
		}); useErr != nil || !matched {
			t.Fatalf("real canonical direct preview field %d was not source-exact", index)
		}
	}
	cleaningArguments, err := domainnative.NewDeterministicCleaningArgumentsV1(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	cleaning, cleanedCSV, err := runner.DeterministicCleaning(
		context.Background(),
		cleaningArguments,
		descriptor,
		&exactBytesReadLease{body: installer.body},
	)
	if err != nil || cleaning.RowCount != 1 || cleaning.ChangedRowCount != 1 ||
		cleaning.UnchangedRowCount != 0 || len(cleaning.ChangedRows) != 1 ||
		cleaning.OutputArtifactSHA256 != domainsecurity.SHA256Hex(cleanedCSV) ||
		cleaning.OutputArtifactByteLength != uint64(len(cleanedCSV)) {
		t.Fatalf("real deterministic cleaning seam failed: result=%#v bytes=%d err=%v", cleaning, len(cleanedCSV), err)
	}
	defer clear(cleanedCSV)
	if cleaning.ChangedRows[0].Status != "changed" || len(cleaning.ChangedRows[0].Cells) != 1 {
		t.Fatalf("real cleaning did not return one allowlisted changed cell: %#v", cleaning)
	}
	var changedField domainnative.DirectSourcePreviewFieldV1
	var beforeExact, afterExact string
	if err := cleaning.ChangedRows[0].Cells[0].UseExactV1(func(
		field domainnative.DirectSourcePreviewFieldV1,
		beforeValue, afterValue, _, _ string,
	) error {
		changedField, beforeExact, afterExact = field, beforeValue, afterValue
		return nil
	}); err != nil || changedField != domainnative.DirectSourcePreviewFieldCounterpartyAccountV1 ||
		beforeExact != "CP-001_ 23" || afterExact != "CP00123" {
		t.Fatalf("real cleaning exact diff drifted: field=%q before=%q after=%q err=%v", changedField, beforeExact, afterExact, err)
	}
	if !bytes.Contains(cleanedCSV, []byte("CP00123")) {
		t.Fatal("real cleaning output did not contain the canonical counterparty account")
	}
	if !bytes.Contains(cleanedCSV, []byte(",12.5,100.01,")) {
		t.Fatal("real cleaning output changed an already-canonical exact decimal")
	}

	cleanedDigest := domainsecurity.SHA256Hex(cleanedCSV)
	cleanedArguments, err := domainnative.NewFundsCanonicalCSVSnapshotBuildArgumentsV1(
		domainnative.FundsCanonicalCSVSnapshotBuildInputV1{
			Binding: binding, PrivateImportFileID: "abcdef0123456789abcd", SourceRevision: 8,
			RawArtifactManifestSHA256: cleaning.ResultDigest, SourceArtifactSHA256: cleanedDigest,
			SourceArtifactByteLength: uint64(len(cleanedCSV)), SourceRowCount: cleaning.RowCount,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	cleanedInstaller := &realCanonicalCSVInstallerV1{}
	cleanedBuild, cleanedObject, cleanedDisposition, err := runner.BuildFundsCanonicalCSVSnapshot(
		context.Background(), cleanedArguments, bytes.NewReader(cleanedCSV), cleanedInstaller,
	)
	cleanedMaterialization, cleanedMaterializationErr := cleanedBuild.MaterializationV1()
	if err != nil || cleanedMaterializationErr != nil ||
		cleanedDisposition != fundsquerysourceport.ImmutableSnapshotInstallCreatedV1 ||
		domainfundsquerysource.ValidateImmutableSnapshotObjectV1(cleanedObject) != nil ||
		cleanedObject.DuckDBSHA256 != cleanedInstaller.sha256 || cleanedInstaller.calls != 1 ||
		cleanedMaterialization.ProducerContentID == materialization.ProducerContentID ||
		registry.Acquisitions() != 4 || runner.session != nil || runner.sessionBinding != "" {
		t.Fatalf(
			"real cleaned output did not return through the immutable builder: result=%s object=%#v disposition=%q materialization_err=%v acquisitions=%d err=%v",
			cleanedBuild.String(), cleanedObject, cleanedDisposition, cleanedMaterializationErr,
			registry.Acquisitions(), err,
		)
	}
	replayRequest := darwinRunnerRequest(t, caseID, 8)
	replayDescriptor := darwinRunnerAccountFlowDescriptorWithProducerAndProfile(
		t,
		replayRequest,
		cleanedInstaller.body,
		cleanedMaterialization.ProducerContentID,
		cleanedMaterialization.ProducerContentManifestSHA256,
		domainfundsquerysource.FixedFundsLocalDisplayQueryProfileDigestV1(),
	)
	replayArguments, err := domainnative.NewDeterministicCleaningArgumentsV1(replayDescriptor)
	if err != nil {
		t.Fatal(err)
	}
	replayed, replayedCSV, err := runner.DeterministicCleaning(
		context.Background(), replayArguments, replayDescriptor,
		&exactBytesReadLease{body: cleanedInstaller.body},
	)
	if err != nil || replayed.ChangedRowCount != 0 || replayed.UnchangedRowCount != 1 ||
		len(replayed.ChangedRows) != 0 || !bytes.Equal(replayedCSV, cleanedCSV) ||
		registry.Acquisitions() != 5 || runner.session != nil || runner.sessionBinding != "" {
		clear(replayedCSV)
		t.Fatalf("real canonical cleaning replay drifted: result=%#v bytes=%d acquisitions=%d err=%v", replayed, len(replayedCSV), registry.Acquisitions(), err)
	}
	clear(replayedCSV)
	assertRunnerStageEmpty(t, stagingRoot)
	if entries, readErr := os.ReadDir(workingDirectory); readErr != nil || len(entries) != 0 {
		t.Fatalf("real canonical seam retained working artifacts: entries=%d err=%v", len(entries), readErr)
	}
}

type realCanonicalCSVInstallerV1 struct {
	calls      int
	sha256     string
	byteLength uint64
	body       []byte
}

func (installer *realCanonicalCSVInstallerV1) InstallExact(
	ctx context.Context,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
	file *os.File,
) (fundsquerysourceport.ImmutableSnapshotInstallDispositionV1, error) {
	installer.calls++
	if ctx == nil || ctx.Err() != nil || file == nil ||
		domainfundsquerysource.ValidateImmutableSnapshotObjectV1(object) != nil {
		return "", fundsquerysourceport.ErrMismatch
	}
	var stat unix.Stat_t
	flags, flagsErr := unix.FcntlInt(file.Fd(), unix.F_GETFL, 0)
	if flagsErr != nil || unix.Fstat(int(file.Fd()), &stat) != nil ||
		flags&unix.O_ACCMODE != unix.O_RDONLY || stat.Mode&unix.S_IFMT != unix.S_IFREG ||
		stat.Mode&0o7777 != 0o400 || stat.Nlink != 0 || stat.Uid != uint32(os.Geteuid()) ||
		stat.Size <= 0 || uint64(stat.Size) != object.DuckDBByteLength {
		return "", fundsquerysourceport.ErrMismatch
	}
	body, err := io.ReadAll(io.NewSectionReader(file, 0, stat.Size+1))
	if err != nil || int64(len(body)) != stat.Size {
		return "", fundsquerysourceport.ErrMismatch
	}
	digest := sha256.Sum256(body)
	installer.sha256 = hex.EncodeToString(digest[:])
	installer.byteLength = uint64(len(body))
	installer.body = append([]byte(nil), body...)
	if installer.sha256 != object.DuckDBSHA256 {
		return "", fundsquerysourceport.ErrMismatch
	}
	return fundsquerysourceport.ImmutableSnapshotInstallCreatedV1, nil
}

func realCanonicalCSVSourceV1(t *testing.T) []byte {
	t.Helper()
	headers := []string{
		"交易卡号", "交易账号", "账户开户名称", "开户人证件号码", "交易时间", "交易金额",
		"交易余额", "收付标志", "交易对手账卡号", "现金标志", "对手户名", "对手身份证号",
		"对手开户银行", "摘要说明", "交易币种", "交易网点名称", "交易网点代码", "交易发生地",
		"交易是否成功", "传票号", "终端号", "IP地址", "MAC地址", "对手交易余额",
		"交易流水号", "日志号", "凭证种类", "凭证号", "交易柜员号", "商户名称", "商户号",
		"备注", "交易类型", "查询反馈结果原因",
	}
	row := make([]string, len(headers))
	row[1] = "6222021234567890123"
	row[2] = "完整敏感姓名"
	row[4] = "2026-01-02 03:04:05"
	row[5] = "12.5"
	row[6] = "100.01"
	row[7] = "进"
	row[8] = "CP-001_ 23"
	row[14] = "CNY"
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	writer.UseCRLF = true
	if writer.Write(headers) != nil || writer.Write(row) != nil {
		t.Fatal("write canonical CSV fixture")
	}
	writer.Flush()
	if writer.Error() != nil {
		t.Fatal("flush canonical CSV fixture")
	}
	return output.Bytes()
}
