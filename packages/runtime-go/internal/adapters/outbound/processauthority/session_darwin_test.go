//go:build darwin && !analytix_prod

package processauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

func TestSessionCarriesOneExactUnlinkedReadOnlyInputAcrossBootstrapExec(t *testing.T) {
	resetDarwinSessionAuthority(t)
	input, digest, size := newDarwinUnlinkedReadOnlyInput(t, []byte("exact-private-snapshot"))
	executablePath := canonicalDarwinTestExecutable(t)
	executable, err := os.Open(executablePath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := executable.Stat()
	if err != nil {
		t.Fatal(err)
	}
	stagingRoot := canonicalDarwinTestPath(t, t.TempDir())
	workingDirectory := canonicalDarwinTestPath(t, t.TempDir())
	workingAuthority := openDarwinTestDirectoryAuthority(t, workingDirectory)
	stagingAuthority := openDarwinTestDirectoryAuthority(t, stagingRoot)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := OpenSession(ctx, SessionConfig{
		Executable: executable, ExpectedExecutableSHA256: darwinTestFileSHA256(t, executablePath),
		ExpectedExecutableSize: info.Size(),
		ReadOnlyInput:          &ReadOnlyInput{File: input, ExpectedSHA256: digest, ExpectedSize: size},
		Arguments:              []string{"-test.run=TestProcessAuthorityHelper", "--", "session-readonly-fd"},
		WorkingDirectory:       workingDirectory, WorkingDirectoryAuthority: workingAuthority,
		StagingRoot: stagingRoot, StagingRootAuthority: stagingAuthority,
		Environment: []string{
			"ANALYTIX_NATIVE_LAUNCH_NONCE=session-test-nonce",
			"ANALYTIX_NATIVE_PROTOCOL_VERSION=1", "LANG=C", "LC_ALL=C", "TZ=UTC",
		},
	})
	if err != nil {
		t.Fatalf("open inherited-input session: %v", err)
	}
	if _, err := input.Stat(); err == nil {
		t.Fatal("OpenSession did not consume the inherited input descriptor")
	}
	authenticateDarwinTestSession(t, session)
	requestContext, cancelRequest := context.WithTimeout(context.Background(), 5*time.Second)
	response, err := session.RoundTrip(requestContext, []byte("{\"id\":\"fd-proof\"}\n"), 4096)
	cancelRequest()
	if err != nil || !strings.Contains(string(response), `"id":"fd-proof"`) {
		t.Fatalf("inherited input target did not remain usable: response=%s err=%v", response, err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close inherited-input session: %v", err)
	}
	assertDarwinStageEmpty(t, stagingRoot)
}

func TestSessionCarriesExactUnlinkedReadWriteOutputAcrossBootstrapExec(t *testing.T) {
	resetDarwinSessionAuthority(t)
	input, digest, size := newDarwinUnlinkedReadOnlyInput(t, []byte("exact-private-snapshot"))
	output, outputReader := newDarwinUnlinkedReadWriteOutput(t)
	defer outputReader.Close()
	executablePath := canonicalDarwinTestExecutable(t)
	executable, err := os.Open(executablePath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := executable.Stat()
	if err != nil {
		t.Fatal(err)
	}
	stagingRoot := canonicalDarwinTestPath(t, t.TempDir())
	workingDirectory := canonicalDarwinTestPath(t, t.TempDir())
	workingAuthority := openDarwinTestDirectoryAuthority(t, workingDirectory)
	stagingAuthority := openDarwinTestDirectoryAuthority(t, stagingRoot)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := OpenSession(ctx, SessionConfig{
		Executable: executable, ExpectedExecutableSHA256: darwinTestFileSHA256(t, executablePath),
		ExpectedExecutableSize: info.Size(),
		ReadOnlyInput:          &ReadOnlyInput{File: input, ExpectedSHA256: digest, ExpectedSize: size},
		ReadWriteOutput:        &ReadWriteOutput{File: output},
		Arguments:              []string{"-test.run=TestProcessAuthorityHelper", "--", "session-readwrite-fd"},
		WorkingDirectory:       workingDirectory, WorkingDirectoryAuthority: workingAuthority,
		StagingRoot: stagingRoot, StagingRootAuthority: stagingAuthority,
		Environment: []string{
			"ANALYTIX_NATIVE_LAUNCH_NONCE=session-test-nonce",
			"ANALYTIX_NATIVE_PROTOCOL_VERSION=1", "LANG=C", "LC_ALL=C", "TZ=UTC",
		},
	})
	if err != nil {
		t.Fatalf("open inherited-output session: %v", err)
	}
	if _, err := output.Stat(); err == nil {
		t.Fatal("OpenSession did not consume the inherited output descriptor")
	}
	authenticateDarwinTestSession(t, session)
	response, err := session.RoundTrip(ctx, []byte("{\"id\":\"fd-proof\"}\n"), 4096)
	if err != nil || !strings.Contains(string(response), `"id":"fd-proof"`) {
		t.Fatalf("inherited output target did not remain usable: response=%s err=%v", response, err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close inherited-output session: %v", err)
	}
	body, err := io.ReadAll(io.NewSectionReader(outputReader, 0, 128))
	if err != nil || string(body) != "exact-private-output" {
		t.Fatalf("exact output inode was not written: body=%q err=%v", body, err)
	}
	assertDarwinStageEmpty(t, stagingRoot)
}

func TestDarwinReadOnlyInputPinsAuthorityOwnedDescriptorBeforeCallerReuse(t *testing.T) {
	body := []byte("authority-owned-private-snapshot")
	input, digest, size := newDarwinUnlinkedReadOnlyInput(t, body)
	callerFD := int(input.Fd())
	owned, err := takeDarwinReadOnlyInput(&ReadOnlyInput{
		File: input, ExpectedSHA256: digest, ExpectedSize: size,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owned.File.Close() })
	if _, err := input.Stat(); err == nil {
		t.Fatal("authority ownership did not consume caller descriptor")
	}
	if int(owned.File.Fd()) == callerFD {
		t.Fatal("authority-owned input reused the caller descriptor number")
	}

	replacementPath := filepath.Join(t.TempDir(), "replacement-private-snapshot")
	if err := os.WriteFile(replacementPath, []byte("different-private-snapshot"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(replacementPath, 0o400); err != nil {
		t.Fatal(err)
	}
	replacement, err := os.Open(replacementPath)
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
	reusedFD, err := unix.FcntlInt(replacement.Fd(), unix.F_DUPFD_CLOEXEC, callerFD)
	if err != nil {
		t.Fatal(err)
	}
	reused := os.NewFile(uintptr(reusedFD), "caller-reused-private-snapshot")
	if reused == nil {
		_ = unix.Close(reusedFD)
		t.Fatal("create reused caller descriptor")
	}
	defer reused.Close()
	if reusedFD != callerFD {
		t.Fatalf("caller descriptor was not deterministically reused: got=%d want=%d", reusedFD, callerFD)
	}

	if err := validateDarwinReadOnlyInput(context.Background(), *owned); err != nil {
		t.Fatalf("caller FD reuse changed authority-owned input: %v", err)
	}
	content, err := io.ReadAll(owned.File)
	if err != nil || !bytes.Equal(content, body) {
		t.Fatalf("authority-owned content changed: body=%q err=%v", content, err)
	}
}

func TestSessionOwnsOneFixedOwnerLivenessWriterUntilClose(t *testing.T) {
	resetDarwinSessionAuthority(t)
	opened, stagingRoot := openDarwinTestSession(t, "session-owner-liveness-fd")
	session, ok := opened.(*darwinSession)
	if !ok || session.ownerLiveness == nil ||
		!validDarwinOwnerLivenessEndpoint(session.ownerLiveness, unix.O_WRONLY) {
		t.Fatalf("owner liveness session = %T writer=%v", opened, session.ownerLiveness)
	}
	writer := session.ownerLiveness
	root := session.tree.root
	authenticateDarwinTestSession(t, opened)
	if err := opened.Close(); err != nil {
		t.Fatalf("close owner-liveness session: %v", err)
	}
	if _, err := writer.Stat(); err == nil {
		t.Fatal("session Close retained the owner-liveness writer")
	}
	proofContext, cancelProof := context.WithTimeout(context.Background(), time.Second)
	defer cancelProof()
	if err := waitDarwinIdentityESRCHWith(proofContext, root, realDarwinProcInfoCaller); err != nil {
		t.Fatalf("owner-liveness Close retained root %#v: %v", root, err)
	}
	assertDarwinStageEmpty(t, stagingRoot)
}

func TestSessionAuthenticatesRealDataEngineOwnerLiveness(t *testing.T) {
	executablePath := os.Getenv("ANALYTIX_TEST_DATA_ENGINE_BINARY")
	if executablePath == "" {
		t.Skip("set ANALYTIX_TEST_DATA_ENGINE_BINARY to a freshly built real data-engine binary")
	}
	resetDarwinSessionAuthority(t)
	executablePath = canonicalDarwinTestPath(t, executablePath)
	executable, err := os.Open(executablePath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := executable.Stat()
	if err != nil {
		_ = executable.Close()
		t.Fatal(err)
	}
	stagingRoot := canonicalDarwinTestPath(t, t.TempDir())
	workingDirectory := canonicalDarwinTestPath(t, t.TempDir())
	workingAuthority := openDarwinTestDirectoryAuthority(t, workingDirectory)
	stagingAuthority := openDarwinTestDirectoryAuthority(t, stagingRoot)
	nonce := strings.Repeat("a", 64)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	opened, err := OpenSession(ctx, SessionConfig{
		Executable: executable, ExpectedExecutableSHA256: darwinTestFileSHA256(t, executablePath),
		ExpectedExecutableSize: info.Size(),
		WorkingDirectory:       workingDirectory, WorkingDirectoryAuthority: workingAuthority,
		StagingRoot: stagingRoot, StagingRootAuthority: stagingAuthority,
		Environment: []string{
			"ANALYTIX_NATIVE_LAUNCH_NONCE=" + nonce,
			"ANALYTIX_NATIVE_PROTOCOL_VERSION=analytix-native-v1",
			"LANG=C", "LC_ALL=C", "TZ=UTC",
		},
	})
	if err != nil {
		t.Fatalf("open real data-engine session: %v", err)
	}
	defer func() { _ = opened.Close() }()
	session := opened.(*darwinSession)
	writer := session.ownerLiveness
	if writer == nil || !validDarwinOwnerLivenessEndpoint(writer, unix.O_WRONLY) {
		t.Fatal("real data-engine session lost its sole owner-liveness writer")
	}
	pID := opened.PID()
	if err := opened.AuthenticateReadiness(ctx, 4096, func(frame []byte, processID int) bool {
		var ready map[string]any
		return processID == pID && json.Unmarshal(frame, &ready) == nil && len(ready) == 6 &&
			ready["kind"] == "analytix_native_ready" && ready["component_id"] == "data-engine" &&
			ready["launch_nonce"] == nonce && ready["protocol_version"] == "analytix-native-v1" &&
			ready["schema_version"] == float64(3) && ready["process_id"] == float64(pID)
	}); err != nil {
		_ = opened.Close()
		t.Fatalf("authenticate real data-engine readiness: %v", err)
	}
	response, err := opened.RoundTrip(ctx, []byte("{\"request_id\":\"real-ping\",\"command\":\"ping\"}\n"), 4096)
	if err != nil || !strings.Contains(string(response), `"request_id":"real-ping"`) ||
		!strings.Contains(string(response), `"ok":true`) {
		_ = opened.Close()
		t.Fatalf("real data-engine ping: response=%s err=%v", response, err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("close real data-engine session: %v", err)
	}
	if _, err := writer.Stat(); err == nil {
		t.Fatal("real data-engine Close retained the owner-liveness writer")
	}
	assertDarwinStageEmpty(t, stagingRoot)
}

func newDarwinUnlinkedReadOnlyInput(t *testing.T, body []byte) (*os.File, string, int64) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private-snapshot")
	writer, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if written, err := writer.Write(body); err != nil || written != len(body) {
		_ = writer.Close()
		t.Fatalf("write private input: written=%d err=%v", written, err)
	}
	if err := writer.Sync(); err != nil {
		_ = writer.Close()
		t.Fatal(err)
	}
	if err := writer.Chmod(0o400); err != nil {
		_ = writer.Close()
		t.Fatal(err)
	}
	readerFD, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		_ = writer.Close()
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		_ = unix.Close(readerFD)
		_ = writer.Close()
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		_ = unix.Close(readerFD)
		t.Fatal(err)
	}
	reader := os.NewFile(uintptr(readerFD), "private-snapshot")
	if reader == nil {
		_ = unix.Close(readerFD)
		t.Fatal("create private input reader")
	}
	sum := sha256.Sum256(body)
	return reader, hex.EncodeToString(sum[:]), int64(len(body))
}

func newDarwinUnlinkedReadWriteOutput(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "5")
	writer, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	readerFD, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		_ = writer.Close()
		t.Fatal(err)
	}
	reader := os.NewFile(uintptr(readerFD), "private-output-reader")
	if reader == nil {
		_ = unix.Close(readerFD)
		_ = writer.Close()
		t.Fatal("create private output reader")
	}
	if err := os.Remove(path); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		t.Fatal(err)
	}
	return writer, reader
}

func TestDarwinSessionOwnsNoUnjoinedWorkerGoroutines(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "session_darwin.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(parsed, func(node ast.Node) bool {
		if _, ok := node.(*ast.GoStmt); ok {
			t.Fatal("long-lived process session must not own a goroutine that can outlive cancellation or Close")
		}
		return true
	})
}

func TestSessionRequiresReadinessBeforeInputAndReusesOneProcess(t *testing.T) {
	resetDarwinSessionAuthority(t)
	session, stagingRoot := openDarwinTestSession(t, "session")
	pid := session.PID()
	if pid <= 0 {
		t.Fatalf("session pid = %d", pid)
	}

	readinessContext, cancelReadiness := context.WithTimeout(context.Background(), 5*time.Second)
	var readiness []byte
	err := session.AuthenticateReadiness(readinessContext, 4096, func(frame []byte, processID int) bool {
		readiness = append([]byte(nil), frame...)
		return processID == pid
	})
	cancelReadiness()
	if err != nil {
		t.Fatalf("read readiness: %v", err)
	}
	wantReadiness := `{"protocolVersion":"1","launchNonce":"session-test-nonce","processId":` + decimalPID(pid) + `,"ready":true}`
	if string(readiness) != wantReadiness {
		t.Fatalf("readiness = %s, want %s", readiness, wantReadiness)
	}

	for sequence, requestID := range []string{"request-one", "request-two"} {
		requestContext, cancelRequest := context.WithTimeout(context.Background(), 5*time.Second)
		response, roundTripErr := session.RoundTrip(
			requestContext,
			[]byte(`{"id":"`+requestID+`"}`+"\n"),
			4096,
		)
		cancelRequest()
		if roundTripErr != nil {
			t.Fatalf("round trip %d: %v", sequence+1, roundTripErr)
		}
		want := `{"id":"` + requestID + `","processId":` + decimalPID(pid) + `,"sequence":` + decimalPID(sequence+1) + `}`
		if string(response) != want {
			t.Fatalf("response %d = %s, want %s", sequence+1, response, want)
		}
		if session.PID() != pid {
			t.Fatalf("session pid changed from %d to %d", pid, session.PID())
		}
	}

	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}
	assertDarwinStageEmpty(t, stagingRoot)
}

func TestSessionBindsLoadedTargetAndCleansBootstrapAndTargetStages(t *testing.T) {
	resetDarwinSessionAuthority(t)
	opened, stagingRoot := openDarwinTestSession(t, "session")
	session, ok := opened.(*darwinSession)
	if !ok {
		t.Fatalf("session type = %T", opened)
	}
	entries, err := os.ReadDir(stagingRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name() == entries[1].Name() {
		t.Fatalf("private launch stages = %#v, want distinct bootstrap and target", entries)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	loadedCDHash, err := loadedProcessCDHash(session.pid)
	if err != nil || !codeDirectoryHashMatches(loadedCDHash, session.targetCDHashes) ||
		!loadedDarwinExecutableMatchesOpenedFile(ctx, session.pid, session.targetStaged, session.targetHash) {
		t.Fatalf("loaded target was not bound to retained target stage: cdhash=%x err=%v", loadedCDHash, err)
	}
	if err := session.AuthenticateReadiness(ctx, 4096, func([]byte, int) bool { return true }); err != nil {
		t.Fatalf("authenticate exact target: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close exact target: %v", err)
	}
	assertDarwinStageEmpty(t, stagingRoot)
}

func TestSessionRejectsExtraReadinessFrameAndCleansUp(t *testing.T) {
	resetDarwinSessionAuthority(t)
	session, stagingRoot := openDarwinTestSession(t, "session-extra-readiness")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := session.AuthenticateReadiness(ctx, 4096, func([]byte, int) bool { return true }); !errors.Is(err, ErrProtocol) {
		t.Fatalf("extra readiness frame survived: err=%v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close rejected session: %v", err)
	}
	assertDarwinStageEmpty(t, stagingRoot)
}

func TestSessionHostBootstrapMakesTargetSelfReportedContainmentIrrelevant(t *testing.T) {
	resetDarwinSessionAuthority(t)
	opened, stagingRoot, workingDirectory := openDarwinTestSessionWithPaths(t, "session-setsid-descendant")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := opened.AuthenticateReadiness(ctx, 4096, func(frame []byte, _ int) bool {
		return string(frame) == `{"target_claims_containment":true}`
	}); err != nil {
		t.Fatalf("authenticate target after host bootstrap: %v", err)
	}
	assertDarwinTargetContainmentProof(t, filepath.Join(workingDirectory, "containment-proof.txt"))
	if err := opened.Close(); err != nil {
		t.Fatalf("close contained target session: %v", err)
	}
	assertDarwinStageEmpty(t, stagingRoot)
}

func TestSessionKernelRejectsForkSetsidAndDoubleForkDuringRoundTrip(t *testing.T) {
	resetDarwinSessionAuthority(t)
	opened, stagingRoot, workingDirectory := openDarwinTestSessionWithPaths(t, "session-spawn-setsid-on-request")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := opened.AuthenticateReadiness(ctx, 4096, func([]byte, int) bool { return true }); err != nil {
		t.Fatalf("authenticate child-free readiness: %v", err)
	}
	response, err := opened.RoundTrip(ctx, []byte("{\"id\":\"spawn\"}\n"), 4096)
	if err != nil || !strings.Contains(string(response), `"id":"spawn"`) {
		t.Fatalf("contained target response failed: response=%s err=%v", response, err)
	}
	assertDarwinTargetContainmentProof(t, filepath.Join(workingDirectory, "containment-proof.txt"))
	if err := opened.Close(); err != nil {
		t.Fatalf("close contained response session: %v", err)
	}
	assertDarwinStageEmpty(t, stagingRoot)
}

func TestSessionRejectsTransientExecSwapBackBeforeAcceptingResponse(t *testing.T) {
	resetDarwinSessionAuthority(t)
	session, stagingRoot := openDarwinTestSession(t, "session-exec-swap-back")
	authenticateDarwinTestSession(t, session)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	response, err := session.RoundTrip(ctx, []byte("{\"id\":\"swap-back\"}\n"), 4096)
	cancel()
	if response != nil || !errors.Is(err, ErrProtocol) {
		t.Fatalf("transient exec-away/exec-back response survived: response=%s err=%v", response, err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close rejected swap-back session: %v", err)
	}
	assertDarwinStageEmpty(t, stagingRoot)
}

func assertDarwinTargetContainmentProof(t *testing.T, path string) {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read target containment proof: %v", err)
	}
	want := "soft=0 hard=0 fork=EAGAIN setsid=EPERM setsid_spawn=EAGAIN double_fork=EAGAIN raise=denied"
	if strings.TrimSpace(string(payload)) != want {
		t.Fatalf("target containment proof = %q, want %q", payload, want)
	}
}

func TestSessionPartialEOFBeforeContainmentProofPoisonsAuthority(t *testing.T) {
	resetDarwinSessionAuthority(t)
	session, stagingRoot := openDarwinTestSession(t, "session-partial-readiness")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := session.AuthenticateReadiness(ctx, 4096, func([]byte, int) bool { return true }); !errors.Is(err, ErrTermination) {
		t.Fatalf("partial readiness survived: err=%v", err)
	}
	if err := session.Close(); !errors.Is(err, ErrTermination) {
		t.Fatalf("partial session did not retain termination failure: %v", err)
	}
	if !darwinAuthorityPoisoned.Load() {
		t.Fatal("unproved pre-readiness exit did not poison process authority")
	}
	assertDarwinStageEmpty(t, stagingRoot)
}

func TestSessionReadinessCancellationCannotAuthenticate(t *testing.T) {
	resetDarwinSessionAuthority(t)
	session, stagingRoot := openDarwinTestSession(t, "session")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := session.AuthenticateReadiness(ctx, 4096, func([]byte, int) bool {
		cancel()
		return true
	})
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("canceled readiness authenticated: err=%v", err)
	}
	requestContext, cancelRequest := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelRequest()
	if response, requestErr := session.RoundTrip(requestContext, []byte("{}\n"), 4096); !errors.Is(requestErr, ErrProtocol) || response != nil {
		t.Fatalf("canceled readiness left a usable session: response=%s err=%v", response, requestErr)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close canceled readiness session: %v", err)
	}
	assertDarwinStageEmpty(t, stagingRoot)
}

func TestSessionAuthorityPoisonDuringReadinessCannotAuthenticate(t *testing.T) {
	resetDarwinSessionAuthority(t)
	session, stagingRoot := openDarwinTestSession(t, "session")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := session.AuthenticateReadiness(ctx, 4096, func([]byte, int) bool {
		darwinAuthorityPoisoned.Store(true)
		return true
	})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("poisoned readiness authenticated: err=%v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close poisoned readiness session: %v", err)
	}
	assertDarwinStageEmpty(t, stagingRoot)
}

func TestSessionRoundTripDeadlineTerminatesProcess(t *testing.T) {
	resetDarwinSessionAuthority(t)
	session, stagingRoot := openDarwinTestSession(t, "session-hang")
	readinessContext, cancelReadiness := context.WithTimeout(context.Background(), 5*time.Second)
	if err := session.AuthenticateReadiness(readinessContext, 4096, func([]byte, int) bool { return true }); err != nil {
		cancelReadiness()
		t.Fatalf("read readiness: %v", err)
	}
	cancelReadiness()

	requestContext, cancelRequest := context.WithTimeout(context.Background(), 50*time.Millisecond)
	_, err := session.RoundTrip(requestContext, []byte("{}\n"), 4096)
	cancelRequest()
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("round trip error = %v, want %v", err, ErrTimeout)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close timed out session: %v", err)
	}
	assertDarwinStageEmpty(t, stagingRoot)
}

func TestSessionCloseKillsReapsAndCleansWhileReadinessValidatorBlocks(t *testing.T) {
	resetDarwinSessionAuthority(t)
	opened, stagingRoot := openDarwinTestSession(t, "session")
	session, ok := opened.(*darwinSession)
	if !ok {
		t.Fatalf("session type = %T", opened)
	}
	session.gateDeadline = 25 * time.Millisecond
	root := session.tree.root
	validatorEntered := make(chan struct{})
	validatorRelease := make(chan struct{})
	authenticated := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		authenticated <- session.AuthenticateReadiness(ctx, 4096, func([]byte, int) bool {
			close(validatorEntered)
			<-validatorRelease
			return true
		})
	}()
	select {
	case <-validatorEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("readiness validator was not entered")
	}

	if err := session.Close(); !errors.Is(err, ErrTermination) {
		t.Fatalf("blocked close error = %v, want %v", err, ErrTermination)
	}
	proofContext, cancelProof := context.WithTimeout(context.Background(), time.Second)
	defer cancelProof()
	if err := waitDarwinIdentityESRCHWith(proofContext, root, realDarwinProcInfoCaller); err != nil {
		t.Fatalf("blocked close retained root %#v: %v", root, err)
	}
	assertDarwinStageEmpty(t, stagingRoot)
	close(validatorRelease)
	select {
	case err := <-authenticated:
		if !errors.Is(err, ErrTermination) {
			t.Fatalf("released validator error = %v, want %v", err, ErrTermination)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("released validator did not return")
	}
}

func TestSessionTerminationFailureStillImmediatelyKillsAndReapsRoot(t *testing.T) {
	resetDarwinSessionAuthority(t)
	opened, stagingRoot := openDarwinTestSession(t, "session")
	session, ok := opened.(*darwinSession)
	if !ok {
		t.Fatalf("session type = %T", opened)
	}
	root := session.tree.root
	session.tree.caller = func(int, int, int, uint64, unsafe.Pointer, int) (int, syscall.Errno) {
		return -1, syscall.EIO
	}
	started := time.Now()
	if err := session.Close(); !errors.Is(err, ErrTermination) {
		t.Fatalf("unavailable proc_info close error = %v, want %v", err, ErrTermination)
	}
	if elapsed := time.Since(started); elapsed >= darwinSessionCleanupDeadline {
		t.Fatalf("termination waited before killing root: %s", elapsed)
	}
	proofContext, cancelProof := context.WithTimeout(context.Background(), time.Second)
	defer cancelProof()
	if err := waitDarwinIdentityESRCHWith(proofContext, root, realDarwinProcInfoCaller); err != nil {
		t.Fatalf("failed containment proof retained root %#v: %v", root, err)
	}
	assertDarwinStageEmpty(t, stagingRoot)
}

func TestSessionCannotWriteBeforeAuthenticatedReadiness(t *testing.T) {
	resetDarwinSessionAuthority(t)
	session, stagingRoot := openDarwinTestSession(t, "session")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if response, err := session.RoundTrip(ctx, []byte("{}\n"), 4096); !errors.Is(err, ErrProtocol) || response != nil {
		t.Fatalf("pre-readiness request survived: response=%s err=%v", response, err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close rejected session: %v", err)
	}
	assertDarwinStageEmpty(t, stagingRoot)
}

func TestSessionRejectsMultipleRequestFramesWithoutWriting(t *testing.T) {
	resetDarwinSessionAuthority(t)
	session, stagingRoot := openDarwinTestSession(t, "session")
	authenticateDarwinTestSession(t, session)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if response, err := session.RoundTrip(ctx, []byte("{}\n{}\n"), 4096); !errors.Is(err, ErrRequestInvalid) || response != nil {
		cancel()
		t.Fatalf("multiple frames survived: response=%s err=%v", response, err)
	}
	cancel()
	requestContext, cancelRequest := context.WithTimeout(context.Background(), 5*time.Second)
	response, err := session.RoundTrip(requestContext, []byte("{\"id\":\"single\"}\n"), 4096)
	cancelRequest()
	if err != nil || string(response) != `{"id":"single","processId":`+decimalPID(session.PID())+`,"sequence":1}` {
		t.Fatalf("invalid frames reached helper before valid request: response=%s err=%v", response, err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}
	assertDarwinStageEmpty(t, stagingRoot)
}

func TestSessionCanceledRequestDoesNotReachHelper(t *testing.T) {
	resetDarwinSessionAuthority(t)
	session, stagingRoot := openDarwinTestSession(t, "session")
	authenticateDarwinTestSession(t, session)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if response, err := session.RoundTrip(canceled, []byte("{\"id\":\"canceled\"}\n"), 4096); !errors.Is(err, ErrCanceled) || response != nil {
		t.Fatalf("canceled frame survived: response=%s err=%v", response, err)
	}
	ctx, cancelValid := context.WithTimeout(context.Background(), 5*time.Second)
	response, err := session.RoundTrip(ctx, []byte("{\"id\":\"valid\"}\n"), 4096)
	cancelValid()
	if err != nil || string(response) != `{"id":"valid","processId":`+decimalPID(session.PID())+`,"sequence":1}` {
		t.Fatalf("canceled frame reached helper: response=%s err=%v", response, err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}
	assertDarwinStageEmpty(t, stagingRoot)
}

func TestSessionClosePreservesPriorTerminationFailure(t *testing.T) {
	session := &darwinSession{gate: make(chan struct{}, 1), state: darwinSessionClosed, terminationErr: ErrTermination}
	session.gate <- struct{}{}
	if err := session.Close(); !errors.Is(err, ErrTermination) {
		t.Fatalf("close error = %v, want persistent %v", err, ErrTermination)
	}
}

func TestOpenSessionRejectsContextWithoutDeadlineAndConsumesFile(t *testing.T) {
	resetDarwinSessionAuthority(t)
	executablePath := canonicalDarwinTestExecutable(t)
	executable, err := os.Open(executablePath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := executable.Stat()
	if err != nil {
		t.Fatal(err)
	}
	_, err = OpenSession(context.Background(), SessionConfig{
		Executable: executable, ExpectedExecutableSHA256: darwinTestFileSHA256(t, executablePath),
		ExpectedExecutableSize: info.Size(), WorkingDirectory: canonicalDarwinTestPath(t, t.TempDir()),
		StagingRoot: canonicalDarwinTestPath(t, t.TempDir()), Environment: []string{"LANG=C"},
	})
	if !errors.Is(err, ErrRequestInvalid) {
		t.Fatalf("open session error = %v, want %v", err, ErrRequestInvalid)
	}
	if _, statErr := executable.Stat(); statErr == nil {
		t.Fatal("OpenSession did not consume and close the executable file")
	}
}

func TestOpenSessionRejectsDirectoryPathReplacementAgainstRetainedAuthority(t *testing.T) {
	for _, fixture := range []struct {
		name    string
		target  string
		wantErr error
	}{
		{name: "working directory", target: "working", wantErr: ErrWorkingDirectory},
		{name: "staging root", target: "staging", wantErr: ErrExecutableIdentity},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			resetDarwinSessionAuthority(t)
			executablePath := canonicalDarwinTestExecutable(t)
			executable, err := os.Open(executablePath)
			if err != nil {
				t.Fatal(err)
			}
			info, err := executable.Stat()
			if err != nil {
				_ = executable.Close()
				t.Fatal(err)
			}
			workingDirectory := canonicalDarwinTestPath(t, t.TempDir())
			stagingRoot := canonicalDarwinTestPath(t, t.TempDir())
			workingAuthority := openDarwinTestDirectoryAuthority(t, workingDirectory)
			stagingAuthority := openDarwinTestDirectoryAuthority(t, stagingRoot)
			replacedPath := workingDirectory
			if fixture.target == "staging" {
				replacedPath = stagingRoot
			}
			retainedPath := replacedPath + "-retained"
			if err := os.Rename(replacedPath, retainedPath); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(replacedPath, 0o700); err != nil {
				t.Fatal(err)
			}
			defer func() {
				_ = os.RemoveAll(replacedPath)
				_ = os.Rename(retainedPath, replacedPath)
			}()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			session, err := OpenSession(ctx, SessionConfig{
				Executable: executable, ExpectedExecutableSHA256: darwinTestFileSHA256(t, executablePath),
				ExpectedExecutableSize: info.Size(), Arguments: []string{"-test.run=TestProcessAuthorityHelper", "--", "session"},
				WorkingDirectory: workingDirectory, WorkingDirectoryAuthority: workingAuthority,
				StagingRoot: stagingRoot, StagingRootAuthority: stagingAuthority,
				Environment: []string{"ANALYTIX_NATIVE_LAUNCH_NONCE=session-test-nonce", "LANG=C"},
			})
			if session != nil || !errors.Is(err, fixture.wantErr) {
				t.Fatalf("replaced %s survived: session=%#v err=%v", fixture.target, session, err)
			}
		})
	}
}

func openDarwinTestSession(t *testing.T, mode string) (Session, string) {
	t.Helper()
	session, stagingRoot, _ := openDarwinTestSessionWithPaths(t, mode)
	return session, stagingRoot
}

func openDarwinTestSessionWithPaths(t *testing.T, mode string) (Session, string, string) {
	t.Helper()
	executablePath := canonicalDarwinTestExecutable(t)
	executable, err := os.Open(executablePath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := executable.Stat()
	if err != nil {
		_ = executable.Close()
		t.Fatal(err)
	}
	stagingRoot := canonicalDarwinTestPath(t, t.TempDir())
	workingDirectory := canonicalDarwinTestPath(t, t.TempDir())
	workingAuthority := openDarwinTestDirectoryAuthority(t, workingDirectory)
	stagingAuthority := openDarwinTestDirectoryAuthority(t, stagingRoot)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := OpenSession(ctx, SessionConfig{
		Executable: executable, ExpectedExecutableSHA256: darwinTestFileSHA256(t, executablePath),
		ExpectedExecutableSize: info.Size(),
		Arguments:              []string{"-test.run=TestProcessAuthorityHelper", "--", mode},
		WorkingDirectory:       workingDirectory, WorkingDirectoryAuthority: workingAuthority,
		StagingRoot: stagingRoot, StagingRootAuthority: stagingAuthority,
		Environment: []string{
			"ANALYTIX_NATIVE_LAUNCH_NONCE=session-test-nonce",
			"ANALYTIX_NATIVE_PROTOCOL_VERSION=1", "LANG=C", "LC_ALL=C", "TZ=UTC",
		},
	})
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, stagingRoot, workingDirectory
}

func openDarwinTestDirectoryAuthority(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func authenticateDarwinTestSession(t *testing.T, session Session) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := session.AuthenticateReadiness(ctx, 4096, func([]byte, int) bool { return true }); err != nil {
		t.Fatalf("authenticate readiness: %v", err)
	}
}

func resetDarwinSessionAuthority(t *testing.T) {
	t.Helper()
	darwinAuthorityPoisoned.Store(false)
	t.Cleanup(func() { darwinAuthorityPoisoned.Store(false) })
}

func assertDarwinStageEmpty(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("staging root retained launch material: %#v", entries)
	}
}

func decimalPID(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	buffer := [32]byte{}
	index := len(buffer)
	for value > 0 {
		index--
		buffer[index] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		index--
		buffer[index] = '-'
	}
	return string(buffer[index:])
}
