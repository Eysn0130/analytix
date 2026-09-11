//go:build darwin && !analytix_prod

package nativecomponentrunner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	nativecomponentregistry "analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry"
	processauthority "analytix.local/runtime-go/internal/adapters/outbound/processauthority"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	"golang.org/x/sys/unix"
)

type privateCSVBuildDiagnosticErrorV1 struct{}

func (privateCSVBuildDiagnosticErrorV1) Error() string {
	panic("private error must never be formatted")
}

func TestRunnerFundsCanonicalCSVDiagnosticPrivacy(t *testing.T) {
	for _, test := range []struct {
		err   error
		class string
	}{
		{ErrTermination, "TERMINATION"}, {context.Canceled, "CANCELLED"}, {context.DeadlineExceeded, "DEADLINE"},
		{ErrProtocol, "PROTOCOL"}, {ErrRegistry, "REGISTRY"}, {ErrRequestInvalid, "REQUEST_INVALID"},
		{fundsquerysourceport.ErrNotFound, "SOURCE_NOT_FOUND"}, {fundsquerysourceport.ErrMismatch, "SOURCE_MISMATCH"},
		{fundsquerysourceport.ErrCorrupt, "SOURCE_CORRUPT"}, {ErrUnavailable, "UNAVAILABLE"},
		{privateCSVBuildDiagnosticErrorV1{}, "UNKNOWN"},
	} {
		var output bytes.Buffer
		original := errors.Join(test.err, privateCSVBuildDiagnosticErrorV1{})
		writeFundsCanonicalCSVBuildFailureV1(&output, "SESSION_OPEN", original)
		if output.String() != "[analytix] event=ANALYTIX_FUNDS_CSV_NATIVE_FAILURE_V1 layer=RUNNER stage=SESSION_OPEN class="+test.class+"\n" || !errors.Is(original, test.err) {
			t.Fatal("runner diagnostic leaked or changed a sentinel class")
		}
	}
	var output bytes.Buffer
	writeFundsCanonicalCSVBuildFailureV1(&output, "private-stage-canary", &os.PathError{Path: "/private/canary", Err: privateCSVBuildDiagnosticErrorV1{}})
	if output.String() != "[analytix] event=ANALYTIX_FUNDS_CSV_NATIVE_FAILURE_V1 layer=RUNNER stage=UNKNOWN class=UNKNOWN\n" {
		t.Fatal("unknown runner stage or private path escaped")
	}
	output.Reset()
	writeFundsCanonicalCSVBuildFailureV1(&output, "INSTALL", nil)
	if output.Len() != 0 {
		t.Fatal("runner success emitted a failure")
	}
}

// Existing fake sessions read/write only isolated files; no process or
// platform signing command is invoked by this fixture.
func TestRunnerFundsCanonicalCSVDiagnosticOutcome(t *testing.T) {
	for _, test := range []struct {
		mode, stage, class string
		want               error
		poisoned           bool
	}{
		{mode: "success"},
		{mode: "wrong-diagnostics", stage: "RESULT_PARSE", class: "PROTOCOL", want: ErrProtocol},
		{mode: "wal-drift", stage: "CLEANUP", class: "TERMINATION", want: ErrTermination, poisoned: true},
	} {
		t.Run(test.mode, func(t *testing.T) {
			sourceBody := bytes.Repeat([]byte("private-diagnostic-source-canary"), 128)
			arguments, responseData := fundsCanonicalCSVRunnerContractV1(t, sourceBody)
			outputBody := bytes.Repeat([]byte("private-diagnostic-output-canary"), 256)
			runner, _, opener, _ := newFundsCanonicalCSVRunnerFixtureV1(t, sourceBody, responseData, outputBody, test.mode)
			installer := &fundsCanonicalCSVRecordingInstallerV1{opener: opener, expectedBody: outputBody}
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal("diagnostic capture unavailable")
			}
			previous := os.Stderr
			os.Stderr = writer
			defer func() { os.Stderr = previous; _ = reader.Close(); _ = writer.Close() }()
			_, _, _, got := runner.BuildFundsCanonicalCSVSnapshot(context.Background(), arguments, bytes.NewReader(sourceBody), installer)
			os.Stderr = previous
			_ = writer.Close()
			output, readErr := io.ReadAll(reader)
			if readErr != nil || !errors.Is(got, test.want) || runner.poisoned.Load() != test.poisoned {
				t.Fatal("diagnostic changed build/cleanup return or poison state")
			}
			expected := ""
			if test.want != nil {
				expected = "[analytix] event=ANALYTIX_FUNDS_CSV_NATIVE_FAILURE_V1 layer=RUNNER stage=" + test.stage + " class=" + test.class + "\n"
			}
			if string(output) != expected {
				t.Fatal("failure stage lost, cleanup misattributed or success not silent")
			}
		})
	}
}

func TestRunnerFundsCanonicalCSVBuildInstallsOnlyValidatedClosedOutput(t *testing.T) {
	sourceBody := bytes.Repeat([]byte("canonical-private-source\n"), 256)
	arguments, responseData := fundsCanonicalCSVRunnerContractV1(t, sourceBody)
	outputBody := bytes.Repeat([]byte("sealed-duckdb-output"), 512)
	runner, registry, opener, stagingRoot := newFundsCanonicalCSVRunnerFixtureV1(
		t,
		sourceBody,
		responseData,
		outputBody,
		"success",
	)
	installer := &fundsCanonicalCSVRecordingInstallerV1{opener: opener, expectedBody: outputBody}
	result, object, disposition, err := runner.BuildFundsCanonicalCSVSnapshot(
		context.Background(),
		arguments,
		bytes.NewReader(sourceBody),
		installer,
	)
	if err != nil || disposition != fundsquerysourceport.ImmutableSnapshotInstallCreatedV1 ||
		domainfundsquerysource.ValidateImmutableSnapshotObjectV1(object) != nil {
		t.Fatalf("canonical build result=%s object=%#v disposition=%q err=%v", result.String(), object, disposition, err)
	}
	expectedDigest := sha256.Sum256(outputBody)
	if object.CaseID != arguments.CaseIDV1() || object.DuckDBSHA256 != hex.EncodeToString(expectedDigest[:]) ||
		object.DuckDBByteLength != uint64(len(outputBody)) || installer.Calls() != 1 ||
		!installer.ObservedClosedChild() || registry.Acquisitions() != 1 || runner.poisoned.Load() {
		t.Fatalf(
			"canonical install binding drifted: object=%#v installs=%d closed=%t acquisitions=%d poisoned=%t",
			object,
			installer.Calls(),
			installer.ObservedClosedChild(),
			registry.Acquisitions(),
			runner.poisoned.Load(),
		)
	}
	assertRunnerStageEmpty(t, stagingRoot)
}

func TestRunnerFundsCanonicalCSVRejectsInheritedGroupBeforeNativeExecution(t *testing.T) {
	sourceBody := []byte("account,direction,amount\nfixture-account,in,1.00\n")
	arguments, responseData := fundsCanonicalCSVRunnerContractV1(t, sourceBody)
	outputBody := bytes.Repeat([]byte("fixture-output"), 512)
	runner, registry, opener, stagingRoot := newFundsCanonicalCSVRunnerFixtureV1(t, sourceBody, responseData, outputBody, "success")
	opens := 0
	runner.sessionOpener = func(ctx context.Context, request sessionOpenRequest) (processauthority.Session, error) {
		opens++
		return opener.Open(ctx, request)
	}
	installer := &fundsCanonicalCSVRecordingInstallerV1{opener: opener, expectedBody: outputBody}
	fd := int(runner.stagingAuthority.Fd())
	before, err := validatePrivateSnapshotDirectory(fd)
	if err != nil {
		t.Fatal("valid staging fixture rejected")
	}
	privateSnapshotSetNonEffectiveGroup(t, fd)
	result, object, disposition, err := runner.BuildFundsCanonicalCSVSnapshot(context.Background(), arguments, bytes.NewReader(sourceBody), installer)
	if !errors.Is(err, ErrRegistry) || !fundsCanonicalCSVResultIsZeroV1(result) ||
		domainfundsquerysource.ValidateImmutableSnapshotObjectV1(object) == nil || disposition != "" ||
		registry.Acquisitions() != 0 || opens != 0 || installer.Calls() != 0 || runner.poisoned.Load() {
		t.Fatal("wrong staging group crossed materialization or changed failure semantics")
	}
	assertRunnerStageEmpty(t, stagingRoot)
	if unix.Fchown(fd, -1, os.Getegid()) != nil || validatePrivateSnapshotDirectoryExact(fd, before) != nil {
		t.Fatal("restore exact staging authority")
	}
	_, object, disposition, err = runner.BuildFundsCanonicalCSVSnapshot(context.Background(), arguments, bytes.NewReader(sourceBody), installer)
	if err != nil || domainfundsquerysource.ValidateImmutableSnapshotObjectV1(object) != nil ||
		disposition != fundsquerysourceport.ImmutableSnapshotInstallCreatedV1 || opens != 1 ||
		installer.Calls() != 1 || !installer.ObservedClosedChild() {
		t.Fatal("valid group failed to cross materialization and complete the fake session")
	}
	assertRunnerStageEmpty(t, stagingRoot)
}

func TestRunnerFundsCanonicalCSVRejectsSourceHashMismatchBeforeNativeExecution(t *testing.T) {
	sourceBody := bytes.Repeat([]byte("exact-source"), 512)
	arguments, responseData := fundsCanonicalCSVRunnerContractV1(t, sourceBody)
	runner, registry, _, stagingRoot := newFundsCanonicalCSVRunnerFixtureV1(
		t,
		sourceBody,
		responseData,
		[]byte("unused-output"),
		"success",
	)
	mutated := append([]byte(nil), sourceBody...)
	mutated[len(mutated)-1] ^= 0xff
	installer := &fundsCanonicalCSVRecordingInstallerV1{}
	result, object, disposition, err := runner.BuildFundsCanonicalCSVSnapshot(
		context.Background(), arguments, bytes.NewReader(mutated), installer,
	)
	if !errors.Is(err, ErrRegistry) || !fundsCanonicalCSVResultIsZeroV1(result) ||
		domainfundsquerysource.ValidateImmutableSnapshotObjectV1(object) == nil || disposition != "" ||
		installer.Calls() != 0 || registry.Acquisitions() != 0 || runner.poisoned.Load() {
		t.Fatalf(
			"source mismatch crossed native/install boundary: result=%s object=%#v disposition=%q err=%v installs=%d acquisitions=%d poisoned=%t",
			result.String(), object, disposition, err, installer.Calls(), registry.Acquisitions(), runner.poisoned.Load(),
		)
	}
	assertRunnerStageEmpty(t, stagingRoot)
}

func TestRunnerFundsCanonicalCSVInheritedOutputNeverWritesReplacementPath(t *testing.T) {
	for _, mode := range []string{"path-replacement", "wal-drift"} {
		t.Run(mode, func(t *testing.T) {
			sourceBody := bytes.Repeat([]byte("drift-source"), 512)
			arguments, responseData := fundsCanonicalCSVRunnerContractV1(t, sourceBody)
			outputBody := bytes.Repeat([]byte("drift-output"), 512)
			runner, _, opener, _ := newFundsCanonicalCSVRunnerFixtureV1(
				t, sourceBody, responseData, outputBody, mode,
			)
			installer := &fundsCanonicalCSVRecordingInstallerV1{}
			result, object, disposition, err := runner.BuildFundsCanonicalCSVSnapshot(
				context.Background(), arguments, bytes.NewReader(sourceBody), installer,
			)
			if !errors.Is(err, ErrTermination) || !fundsCanonicalCSVResultIsZeroV1(result) ||
				domainfundsquerysource.ValidateImmutableSnapshotObjectV1(object) == nil || disposition != "" ||
				installer.Calls() != 0 || !runner.poisoned.Load() {
				t.Fatalf(
					"%s survived: result=%s object=%#v disposition=%q err=%v installs=%d poisoned=%t",
					mode, result.String(), object, disposition, err, installer.Calls(), runner.poisoned.Load(),
				)
			}
			if mode == "path-replacement" {
				body, readErr := os.ReadFile(opener.OutputPath())
				if readErr != nil || string(body) != "attacker-controlled-replacement" ||
					bytes.Contains(body, outputBody) {
					t.Fatalf("replacement path received protected output: body=%q err=%v", body, readErr)
				}
			}
			// Production preserves an unrecognized path. This isolated fixture
			// removes only the exact attacker path created by the fake.
			if path := opener.OutputPath(); path != "" {
				_ = os.Remove(path + ".wal")
				_ = os.Remove(path)
				_ = os.Remove(filepath.Dir(path))
			}
		})
	}
}

func TestRunnerFundsCanonicalCSVRejectsOutputModeAndResponseDiagnosticDrift(t *testing.T) {
	for _, scenario := range []struct {
		mode string
		want error
	}{
		{mode: "mode-drift", want: ErrRegistry},
		{mode: "wrong-diagnostics", want: ErrProtocol},
	} {
		t.Run(scenario.mode, func(t *testing.T) {
			sourceBody := bytes.Repeat([]byte("closed-contract-source"), 384)
			arguments, responseData := fundsCanonicalCSVRunnerContractV1(t, sourceBody)
			outputBody := bytes.Repeat([]byte("closed-contract-output"), 384)
			runner, _, _, stagingRoot := newFundsCanonicalCSVRunnerFixtureV1(
				t, sourceBody, responseData, outputBody, scenario.mode,
			)
			installer := &fundsCanonicalCSVRecordingInstallerV1{}
			result, object, disposition, err := runner.BuildFundsCanonicalCSVSnapshot(
				context.Background(), arguments, bytes.NewReader(sourceBody), installer,
			)
			if !errors.Is(err, scenario.want) || !fundsCanonicalCSVResultIsZeroV1(result) ||
				domainfundsquerysource.ValidateImmutableSnapshotObjectV1(object) == nil ||
				disposition != "" || installer.Calls() != 0 || runner.poisoned.Load() {
				t.Fatalf(
					"%s survived: result=%s object=%#v disposition=%q err=%v installs=%d poisoned=%t",
					scenario.mode, result.String(), object, disposition, err,
					installer.Calls(), runner.poisoned.Load(),
				)
			}
			assertRunnerStageEmpty(t, stagingRoot)
		})
	}
}

func TestRunnerFundsCanonicalCSVCancellationAndUnconfirmedTermination(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		mode       string
		cancel     bool
		want       error
		poisoned   bool
		outputBody []byte
	}{
		{name: "canceled", mode: "block", cancel: true, want: context.Canceled},
		{name: "termination", mode: "close-failure", want: ErrTermination, poisoned: true, outputBody: []byte("closed-uncertain-output")},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			sourceBody := bytes.Repeat([]byte("lifecycle-source"), 512)
			arguments, responseData := fundsCanonicalCSVRunnerContractV1(t, sourceBody)
			runner, _, opener, stagingRoot := newFundsCanonicalCSVRunnerFixtureV1(
				t, sourceBody, responseData, scenario.outputBody, scenario.mode,
			)
			installer := &fundsCanonicalCSVRecordingInstallerV1{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			type outcomeV1 struct {
				result      domainnative.FundsCanonicalCSVSnapshotBuildResultV1
				object      domainfundsquerysource.ImmutableSnapshotObjectV1
				disposition fundsquerysourceport.ImmutableSnapshotInstallDispositionV1
				err         error
			}
			done := make(chan outcomeV1, 1)
			go func() {
				result, object, disposition, err := runner.BuildFundsCanonicalCSVSnapshot(
					ctx, arguments, bytes.NewReader(sourceBody), installer,
				)
				done <- outcomeV1{result: result, object: object, disposition: disposition, err: err}
			}()
			if scenario.cancel {
				select {
				case <-opener.RoundTripEntered():
					cancel()
				case <-time.After(5 * time.Second):
					cancel()
					t.Fatal("canonical builder did not enter round trip")
				}
			}
			outcome := <-done
			if !errors.Is(outcome.err, scenario.want) || !fundsCanonicalCSVResultIsZeroV1(outcome.result) ||
				domainfundsquerysource.ValidateImmutableSnapshotObjectV1(outcome.object) == nil ||
				outcome.disposition != "" || installer.Calls() != 0 || runner.poisoned.Load() != scenario.poisoned {
				t.Fatalf(
					"lifecycle result=%s object=%#v disposition=%q err=%v installs=%d poisoned=%t",
					outcome.result.String(), outcome.object, outcome.disposition, outcome.err,
					installer.Calls(), runner.poisoned.Load(),
				)
			}
			assertRunnerStageEmpty(t, stagingRoot)
		})
	}
}

type fundsCanonicalCSVSessionOpenerV1 struct {
	mu            sync.Mutex
	sourceBody    []byte
	responseData  []byte
	outputBody    []byte
	mode          string
	lastSession   *fundsCanonicalCSVSessionV1
	outputPath    string
	roundTripGate chan struct{}
}

type fundsCanonicalCSVSessionV1 struct {
	pid     int
	nonce   string
	opener  *fundsCanonicalCSVSessionOpenerV1
	output  *os.File
	closed  bool
	entered sync.Once
}

type fundsCanonicalCSVRecordingInstallerV1 struct {
	mu           sync.Mutex
	opener       *fundsCanonicalCSVSessionOpenerV1
	expectedBody []byte
	calls        int
	closedChild  bool
}

func (opener *fundsCanonicalCSVSessionOpenerV1) Open(
	_ context.Context,
	config sessionOpenRequest,
) (processauthority.Session, error) {
	if config.Executable == nil || config.ReadOnlyInput == nil || config.ReadOnlyInput.File == nil ||
		config.ReadWriteOutput == nil || config.ReadWriteOutput.File == nil {
		return nil, processauthority.ErrRequestInvalid
	}
	if err := config.Executable.Close(); err != nil {
		_ = config.ReadOnlyInput.File.Close()
		_ = config.ReadWriteOutput.File.Close()
		return nil, err
	}
	input := config.ReadOnlyInput
	if input.ExpectedSize != int64(len(opener.sourceBody)) {
		_ = input.File.Close()
		_ = config.ReadWriteOutput.File.Close()
		return nil, processauthority.ErrExecutableIdentity
	}
	expectedDigest := sha256.Sum256(opener.sourceBody)
	if input.ExpectedSHA256 != hex.EncodeToString(expectedDigest[:]) {
		_ = input.File.Close()
		_ = config.ReadWriteOutput.File.Close()
		return nil, processauthority.ErrExecutableIdentity
	}
	var stat unix.Stat_t
	inputFD := int(input.File.Fd())
	flags, flagsErr := unix.FcntlInt(input.File.Fd(), unix.F_GETFL, 0)
	statErr := unix.Fstat(inputFD, &stat)
	body, readErr := io.ReadAll(input.File)
	closeErr := input.File.Close()
	if flagsErr != nil || statErr != nil || flags&unix.O_ACCMODE != unix.O_RDONLY ||
		stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0o7777 != 0o400 || stat.Nlink != 0 ||
		stat.Uid != uint32(os.Geteuid()) || stat.Size != int64(len(opener.sourceBody)) ||
		readErr != nil || closeErr != nil ||
		!bytes.Equal(body, opener.sourceBody) {
		_ = config.ReadWriteOutput.File.Close()
		return nil, processauthority.ErrExecutableIdentity
	}
	output := config.ReadWriteOutput.File
	outputFD := int(output.Fd())
	var outputStat unix.Stat_t
	outputFlags, outputFlagsErr := unix.FcntlInt(output.Fd(), unix.F_GETFL, 0)
	outputDescriptorFlags, outputDescriptorErr := unix.FcntlInt(output.Fd(), unix.F_GETFD, 0)
	if outputFlagsErr != nil || outputDescriptorErr != nil || unix.Fstat(outputFD, &outputStat) != nil ||
		outputFlags&unix.O_ACCMODE != unix.O_RDWR || outputDescriptorFlags&unix.FD_CLOEXEC == 0 ||
		outputStat.Mode&unix.S_IFMT != unix.S_IFREG || outputStat.Mode&0o7777 != 0o600 ||
		outputStat.Nlink != 0 || outputStat.Uid != uint32(os.Geteuid()) || outputStat.Size != 0 {
		_ = output.Close()
		return nil, processauthority.ErrExecutableIdentity
	}
	formerPath, pathErr := darwinFileDescriptorPath(outputFD)
	if pathErr != nil || filepath.Base(formerPath) != fundsCanonicalCSVOutputBasenameV1 {
		_ = output.Close()
		return nil, processauthority.ErrExecutableIdentity
	}
	nonce := ""
	for _, entry := range config.Environment {
		if strings.HasPrefix(entry, "ANALYTIX_NATIVE_LAUNCH_NONCE=") {
			nonce = strings.TrimPrefix(entry, "ANALYTIX_NATIVE_LAUNCH_NONCE=")
		}
	}
	if !domainsecurity.IsSHA256Hex(nonce) {
		_ = output.Close()
		return nil, processauthority.ErrProtocol
	}
	session := &fundsCanonicalCSVSessionV1{
		pid: 9101, nonce: nonce, opener: opener, output: output,
	}
	opener.mu.Lock()
	opener.lastSession = session
	opener.outputPath = formerPath
	opener.mu.Unlock()
	return session, nil
}

func (session *fundsCanonicalCSVSessionV1) PID() int { return session.pid }

func (session *fundsCanonicalCSVSessionV1) AuthenticateReadiness(
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

func (session *fundsCanonicalCSVSessionV1) RoundTrip(
	ctx context.Context,
	request []byte,
	limit int,
) ([]byte, error) {
	if session.closed || limit != fundsCanonicalCSVResponseFrameLimit {
		return nil, processauthority.ErrProtocol
	}
	var frame struct {
		RequestID string          `json:"request_id"`
		Command   string          `json:"command"`
		CaseID    string          `json:"case_id"`
		Payload   json.RawMessage `json:"payload"`
	}
	decoder := json.NewDecoder(bytes.NewReader(request))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&frame) != nil || !domainsecurity.IsSHA256Hex(frame.RequestID) ||
		frame.Command != domainnative.OperationFundsBuildCanonicalCSVSnapshotV1 ||
		frame.CaseID == "" || len(frame.Payload) == 0 || session.output == nil {
		return nil, processauthority.ErrProtocol
	}
	if session.opener.mode == "block" {
		session.entered.Do(func() { close(session.opener.roundTripGate) })
		<-ctx.Done()
		return nil, processauthority.ErrCanceled
	}
	if err := writeFundsCanonicalCSVFakeOutputV1(
		session.output,
		session.opener.OutputPath(),
		session.opener.outputBody,
		session.opener.mode,
	); err != nil {
		return nil, processauthority.ErrProtocol
	}
	dbBound := "true"
	if session.opener.mode == "wrong-diagnostics" {
		dbBound = "false"
	}
	return []byte(fmt.Sprintf(
		`{"request_id":"%s","ok":true,"data":%s,"diagnostics":{"engine":"analytix-data-engine","command":"%s","case_bound":true,"db_bound":%s,"pid":%d,"queue_wait_ms":0,"run_ms":1,"owner_epoch":""}}`,
		frame.RequestID,
		session.opener.responseData,
		domainnative.OperationFundsBuildCanonicalCSVSnapshotV1,
		dbBound,
		session.pid,
	)), nil
}

func (session *fundsCanonicalCSVSessionV1) Close() error {
	closeErr := error(nil)
	if session.output != nil {
		closeErr = session.output.Close()
		session.output = nil
	}
	if session.opener.mode == "close-failure" {
		return processauthority.ErrTermination
	}
	session.closed = true
	return closeErr
}

func (opener *fundsCanonicalCSVSessionOpenerV1) RoundTripEntered() <-chan struct{} {
	return opener.roundTripGate
}

func (opener *fundsCanonicalCSVSessionOpenerV1) OutputPath() string {
	opener.mu.Lock()
	defer opener.mu.Unlock()
	return opener.outputPath
}

func writeFundsCanonicalCSVFakeOutputV1(file *os.File, formerPath string, body []byte, mode string) error {
	if file == nil || formerPath == "" {
		return processauthority.ErrExecutableIdentity
	}
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if len(body) > 0 {
		if _, err := file.Write(body); err != nil {
			return err
		}
	}
	if err := file.Sync(); err != nil {
		return err
	}
	switch mode {
	case "path-replacement":
		return os.WriteFile(formerPath, []byte("attacker-controlled-replacement"), 0o600)
	case "wal-drift":
		return os.WriteFile(formerPath+".wal", []byte("unexpected-wal"), 0o600)
	case "mode-drift":
		return file.Chmod(0o640)
	default:
		return nil
	}
}

func (installer *fundsCanonicalCSVRecordingInstallerV1) InstallExact(
	ctx context.Context,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
	file *os.File,
) (fundsquerysourceport.ImmutableSnapshotInstallDispositionV1, error) {
	installer.mu.Lock()
	defer installer.mu.Unlock()
	installer.calls++
	if ctx == nil || file == nil || domainfundsquerysource.ValidateImmutableSnapshotObjectV1(object) != nil {
		return "", fundsquerysourceport.ErrMismatch
	}
	if installer.opener != nil {
		installer.opener.mu.Lock()
		installer.closedChild = installer.opener.lastSession != nil && installer.opener.lastSession.closed
		installer.opener.mu.Unlock()
	}
	var stat unix.Stat_t
	flags, flagsErr := unix.FcntlInt(file.Fd(), unix.F_GETFL, 0)
	body, readErr := io.ReadAll(io.NewSectionReader(file, 0, int64(object.DuckDBByteLength)+1))
	if flagsErr != nil || unix.Fstat(int(file.Fd()), &stat) != nil ||
		flags&unix.O_ACCMODE != unix.O_RDONLY || stat.Mode&unix.S_IFMT != unix.S_IFREG ||
		stat.Mode&0o7777 != 0o400 || stat.Nlink != 0 || stat.Size != int64(object.DuckDBByteLength) ||
		readErr != nil || !bytes.Equal(body, installer.expectedBody) {
		return "", fundsquerysourceport.ErrMismatch
	}
	digest := sha256.Sum256(body)
	if hex.EncodeToString(digest[:]) != object.DuckDBSHA256 || !installer.closedChild {
		return "", fundsquerysourceport.ErrMismatch
	}
	return fundsquerysourceport.ImmutableSnapshotInstallCreatedV1, nil
}

func (installer *fundsCanonicalCSVRecordingInstallerV1) Calls() int {
	installer.mu.Lock()
	defer installer.mu.Unlock()
	return installer.calls
}

func (installer *fundsCanonicalCSVRecordingInstallerV1) ObservedClosedChild() bool {
	installer.mu.Lock()
	defer installer.mu.Unlock()
	return installer.closedChild
}

func newFundsCanonicalCSVRunnerFixtureV1(
	t *testing.T,
	sourceBody []byte,
	responseData []byte,
	outputBody []byte,
	mode string,
) (*Runner, *fakeExecutionRegistry, *fundsCanonicalCSVSessionOpenerV1, string) {
	t.Helper()
	payload := []byte("funds-canonical-csv-native-runner-fixture")
	executable := filepath.Join(t.TempDir(), "analytix-data-engine")
	if err := os.WriteFile(executable, payload, 0o500); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	digestText := hex.EncodeToString(digest[:])
	registryDigest := domainsecurity.SHA256Hex([]byte("funds-canonical-csv-runner-registry"))
	registry := &fakeExecutionRegistry{
		digest:     registryDigest,
		executable: executable,
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
	opener := &fundsCanonicalCSVSessionOpenerV1{
		sourceBody: append([]byte(nil), sourceBody...), responseData: append([]byte(nil), responseData...),
		outputBody: append([]byte(nil), outputBody...), mode: mode, roundTripGate: make(chan struct{}),
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

func fundsCanonicalCSVRunnerContractV1(
	t *testing.T,
	sourceBody []byte,
) (domainnative.FundsCanonicalCSVSnapshotBuildArgumentsV1, []byte) {
	t.Helper()
	const caseID = "case-canonical-csv-runner"
	const privateImportFileID = "0123456789abcdef0123"
	sourceDigest := sha256.Sum256(sourceBody)
	sourceSHA256 := hex.EncodeToString(sourceDigest[:])
	rawManifestSHA256 := strings.Repeat("a", 64)
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
			Binding: binding, PrivateImportFileID: privateImportFileID, SourceRevision: 7,
			RawArtifactManifestSHA256: rawManifestSHA256, SourceArtifactSHA256: sourceSHA256,
			SourceArtifactByteLength: uint64(len(sourceBody)), SourceRowCount: 2,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	producer, err := domainsecurity.NewFundsProducerContentManifestV1(
		domainsecurity.FundsProducerContentManifestInputV1{
			CaseID: caseID, SourceRevision: 7, RawManifestSHA256: rawManifestSHA256,
			NormalizedContentSHA256: strings.Repeat("b", 64), DetailContentSHA256: strings.Repeat("c", 64),
			AggregateContentSHA256: strings.Repeat("d", 64), KeywordContentSHA256: strings.Repeat("e", 64),
			AccountContentSHA256: strings.Repeat("f", 64), NormalizedRowCount: 2, AcceptedRowCount: 2,
			DetailRowCount: 2, AggregateRowCount: 1, KeywordRowCount: 1, AccountRowCount: 1,
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
		CleanedStatus: "done", FileID: privateImportFileID, RowsImportedNorm: 2,
		SHA256: sourceSHA256, Status: "已完成",
	}}
	rawSourceBody, err := json.Marshal(rawSources)
	if err != nil {
		t.Fatal(err)
	}
	materialization := domainsecurity.FundsMaterializationResultV1{
		CaseID: caseID, RowCount: producer.AggregateRowCount,
		AggregateName:                        domainsecurity.FundsMaterializationIdentityPrefixV1 + strings.Repeat("0", 64),
		AggregateVersion:                     domainsecurity.FundsMaterializationAggregateVersionV1,
		MaterializationIdentity:              domainsecurity.FundsMaterializationIdentityPrefixV1 + strings.Repeat("0", 64),
		MaterializationIdentitySchemaVersion: domainsecurity.FundsMaterializationIdentitySchemaVersionV1,
		ProducerContentID:                    domainsecurity.DeriveFundsProducerContentIDV1(producer),
		ProducerContentManifestBase64:        base64.StdEncoding.EncodeToString(producerBody),
		ProducerContentManifestSHA256:        domainsecurity.SHA256Hex(producerBody),
		ProducerContentManifestByteLength:    uint64(len(producerBody)),
		RawArtifactManifestSHA256:            rawManifestSHA256,
		RawSourceManifestBase64:              base64.StdEncoding.EncodeToString(rawSourceBody),
		RawSourceManifestSHA256:              domainsecurity.SHA256Hex(rawSourceBody),
		RawSourceManifestByteLength:          uint64(len(rawSourceBody)),
		DuckDBContentSnapshotDigest:          strings.Repeat("1", 64),
		DuckDBSnapshotManifestSHA256:         strings.Repeat("2", 64),
		SchemaDigest:                         domainsecurity.FixedFundsAnalyticalSchemaDigestV1(),
	}
	materializationBody, err := json.Marshal(materialization)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if json.Unmarshal(materializationBody, &result) != nil {
		t.Fatal("materialization fixture is not JSON")
	}
	for key, value := range map[string]any{
		"schemaVersion":            1,
		"operation":                domainnative.OperationFundsBuildCanonicalCSVSnapshotV1,
		"profile":                  domainnative.FundsCanonicalDirectCSVProfileV1,
		"privateImportFileId":      privateImportFileID,
		"sourceArtifactSha256":     sourceSHA256,
		"sourceArtifactByteLength": uint64(len(sourceBody)),
		"sourceRowCount":           uint64(2),
		"sourceMaxTxnTs":           "2026-01-02 03:04:05",
		"sourceMaxId":              uint64(2),
		"rowHashContract":          domainnative.FundsCanonicalCSVRowHashContractV1,
	} {
		result[key] = value
	}
	responseData, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return arguments, responseData
}

func fundsCanonicalCSVResultIsZeroV1(
	result domainnative.FundsCanonicalCSVSnapshotBuildResultV1,
) bool {
	sha256, byteLength, rowCount, maxTxnTS, maxID := result.SourceStatsV1()
	return sha256 == "" && byteLength == 0 && rowCount == 0 && maxTxnTS == "" && maxID == 0
}
