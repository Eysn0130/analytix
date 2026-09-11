//go:build darwin && analytix_native_build_probe && !analytix_prod

package processauthority

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	buildProbeControlTargetFD = 4
	buildProbeEventTargetFD   = 5
	buildProbeRequestTargetFD = 6
	buildProbeFrameLimit      = 8 * 1024
	buildProbeArmAck          = byte(0xa1)
	buildProbeFinishAck       = byte(0xf1)
	buildProbeReadinessLimit  = 4 * 1024
	buildProbeResponseLimit   = 64 * 1024
	buildProbeTempParent      = "/private/tmp"
)

type buildProbeArmEnvelope struct {
	Kind          string `json:"kind"`
	SchemaVersion int    `json:"schema_version"`
	PID           int    `json:"pid"`
	PPID          int    `json:"ppid"`
	PGID          int    `json:"pgid"`
	UID           uint32 `json:"uid"`
	StartSec      uint64 `json:"start_sec"`
	StartUSec     uint64 `json:"start_usec"`
}

type buildProbeControlEvent struct {
	value byte
	err   error
}

type buildProbePrivateDirectories struct {
	root            string
	work            string
	stage           string
	rootName        string
	parentPath      string
	parentOwner     uint32
	parentMode      uint16
	provenance      []byte
	parentAuthority *os.File
	rootAuthority   *os.File
	workAuthority   *os.File
	stageAuthority  *os.File
	closeOnce       sync.Once
	closeErr        error
}

func openBuildProbeTarget() (*os.File, error) {
	var stat unix.Stat_t
	if unix.Fstat(3, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 ||
		stat.Size <= 0 || stat.Size > buildProbeMaxExecutableBytes || stat.Mode&0o022 != 0 ||
		(stat.Uid != 0 && stat.Uid != uint32(os.Geteuid())) {
		return nil, ErrExecutableIdentity
	}
	duplicated, err := unix.FcntlInt(uintptr(3), unix.F_DUPFD_CLOEXEC, 7)
	if err != nil || duplicated < 7 {
		if err == nil && duplicated >= 0 {
			_ = unix.Close(duplicated)
		}
		return nil, ErrExecutableIdentity
	}
	target := os.NewFile(uintptr(duplicated), "analytix-native-build-probe-target")
	if target == nil {
		_ = unix.Close(duplicated)
		return nil, ErrExecutableIdentity
	}
	return target, nil
}

func openBuildProbeRequestInput(fd int, name string) (*os.File, error) {
	if fd < 0 || name == "" {
		return nil, ErrBuildProbeInvalid
	}
	duplicated, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 7)
	if err != nil || duplicated < 7 {
		if err == nil && duplicated >= 0 {
			_ = unix.Close(duplicated)
		}
		return nil, ErrBuildProbeInvalid
	}
	if err := unix.SetNonblock(duplicated, true); err != nil {
		_ = unix.Close(duplicated)
		return nil, ErrBuildProbeInvalid
	}
	file := os.NewFile(uintptr(duplicated), name)
	if file == nil {
		_ = unix.Close(duplicated)
		return nil, ErrBuildProbeInvalid
	}
	return file, nil
}

func probePinnedBuildArtifact(
	ctx context.Context,
	target *os.File,
	request BuildProbeRequestV1,
	protocol BuildProbeProtocol,
) (BuildProbeReceipt, error) {
	if ctx == nil || target == nil || ctx.Err() != nil || !protocol.valid() ||
		validateBuildProbeRequest(request) != nil || !currentBuildProbeMainAllowed() {
		return BuildProbeReceipt{}, ErrBuildProbeInvalid
	}
	duplicated, err := unix.FcntlInt(target.Fd(), unix.F_DUPFD_CLOEXEC, 7)
	if err != nil || duplicated < 7 {
		if err == nil && duplicated >= 0 {
			_ = unix.Close(duplicated)
		}
		return BuildProbeReceipt{}, ErrExecutableIdentity
	}
	pinned := os.NewFile(uintptr(duplicated), "analytix-native-build-probe-api-target")
	if pinned == nil {
		_ = unix.Close(duplicated)
		return BuildProbeReceipt{}, ErrExecutableIdentity
	}
	defer pinned.Close()
	if err := validateBuildProbeTargetDescriptor(ctx, pinned, request); err != nil {
		return BuildProbeReceipt{}, err
	}
	return runBuildProbeCoordinator(ctx, request, pinned, protocol)
}

func validateBuildProbeTargetDescriptor(ctx context.Context, target *os.File, request BuildProbeRequestV1) error {
	if ctx == nil || target == nil || ctx.Err() != nil || validateBuildProbeRequest(request) != nil {
		return ErrBuildProbeInvalid
	}
	var before unix.Stat_t
	if unix.Fstat(int(target.Fd()), &before) != nil || before.Mode&unix.S_IFMT != unix.S_IFREG ||
		before.Nlink != 1 || before.Size != request.ExpectedExecutableSize || before.Size <= 0 ||
		before.Size > buildProbeMaxExecutableBytes || before.Mode&0o022 != 0 ||
		(before.Uid != 0 && before.Uid != uint32(os.Geteuid())) {
		return ErrExecutableIdentity
	}
	digest, err := sha256OpenedFile(ctx, target)
	var after unix.Stat_t
	if err != nil || hex.EncodeToString(digest[:]) != request.ExpectedExecutableSHA256 ||
		unix.Fstat(int(target.Fd()), &after) != nil ||
		componentDarwinFileIdentity(before) != componentDarwinFileIdentity(after) {
		return ErrExecutableIdentity
	}
	return nil
}

func currentBuildProbeAuthorityIdentity(ctx context.Context) (BuildProbeAuthorityIdentityV1, error) {
	if ctx == nil || ctx.Err() != nil {
		return BuildProbeAuthorityIdentityV1{}, ErrBuildProbeInvalid
	}
	authority, digest, err := openCurrentDarwinBootstrap(ctx)
	if err != nil {
		return BuildProbeAuthorityIdentityV1{}, err
	}
	identity := BuildProbeAuthorityIdentityV1{
		SHA256: hex.EncodeToString(digest[:]), Size: authority.stat.Size, HostTarget: buildProbeHostTarget(),
	}
	if closeErr := authority.file.Close(); closeErr != nil {
		return BuildProbeAuthorityIdentityV1{}, ErrExecutableIdentity
	}
	if !validBuildProbeAuthorityIdentity(identity) {
		return BuildProbeAuthorityIdentityV1{}, ErrExecutableIdentity
	}
	return identity, nil
}

func runBuildProbeCoordinator(
	ctx context.Context,
	request buildProbeRequest,
	target *os.File,
	protocol BuildProbeProtocol,
) (receipt BuildProbeReceipt, returnErr error) {
	if ctx == nil || target == nil || !protocol.valid() || validateBuildProbeRequest(request) != nil ||
		!currentBuildProbeMainAllowed() {
		return BuildProbeReceipt{}, ErrBuildProbeInvalid
	}
	authoritySource, authorityHash, err := openCurrentDarwinBootstrap(ctx)
	if err != nil {
		return BuildProbeReceipt{}, err
	}
	defer authoritySource.file.Close()
	authoritySHA256 := hex.EncodeToString(authorityHash[:])
	directories, err := openBuildProbePrivateDirectories()
	if err != nil {
		return BuildProbeReceipt{}, err
	}
	defer func() {
		if closeErr := directories.close(); closeErr != nil && returnErr == nil {
			receipt = BuildProbeReceipt{}
			returnErr = ErrTermination
		}
	}()

	guardianPath, guardianFile, cleanupGuardianStage, err := stageDarwinBuildProbeExecutableWithAuthority(
		ctx,
		directories.stage,
		directories.stageAuthority,
		authoritySource.file,
		authoritySource.stat.Size,
		authorityHash,
	)
	if err != nil {
		return BuildProbeReceipt{}, ErrExecutableIdentity
	}
	defer guardianFile.Close()
	defer func() {
		if cleanupErr := cleanupGuardianStage(); cleanupErr != nil && returnErr == nil {
			receipt = BuildProbeReceipt{}
			returnErr = ErrTermination
		}
	}()
	guardianCDHashes, err := openedFileCodeDirectoryHashes(guardianFile)
	if err != nil {
		return BuildProbeReceipt{}, ErrExecutableIdentity
	}
	var guardianStat unix.Stat_t
	if unix.Fstat(int(guardianFile.Fd()), &guardianStat) != nil {
		return BuildProbeReceipt{}, ErrExecutableIdentity
	}
	guardianObject := &pinnedDarwinObject{file: guardianFile, stat: guardianStat}

	controlReader, controlWriter, err := os.Pipe()
	if err != nil {
		return BuildProbeReceipt{}, ErrUnavailable
	}
	eventReader, eventWriter, err := os.Pipe()
	if err != nil {
		_ = controlReader.Close()
		_ = controlWriter.Close()
		return BuildProbeReceipt{}, ErrUnavailable
	}
	requestReader, requestWriter, err := os.Pipe()
	if err != nil {
		_ = controlReader.Close()
		_ = controlWriter.Close()
		_ = eventReader.Close()
		_ = eventWriter.Close()
		return BuildProbeReceipt{}, ErrUnavailable
	}
	defer controlReader.Close()
	defer controlWriter.Close()
	defer eventReader.Close()
	defer eventWriter.Close()
	defer requestReader.Close()
	defer requestWriter.Close()
	nullFile, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return BuildProbeReceipt{}, ErrUnavailable
	}
	defer nullFile.Close()

	pid, err := spawnSuspendedWithExtraFiles(
		guardianPath,
		[]string{guardianPath, buildProbeGuardianArg},
		[]string{"LANG=C", "LC_ALL=C", "TZ=UTC"},
		int(directories.workAuthority.Fd()),
		int(nullFile.Fd()),
		int(nullFile.Fd()),
		int(nullFile.Fd()),
		[]darwinSpawnFile{
			{source: int(target.Fd()), target: 3},
			{source: int(controlReader.Fd()), target: buildProbeControlTargetFD},
			{source: int(eventWriter.Fd()), target: buildProbeEventTargetFD},
			{source: int(requestReader.Fd()), target: buildProbeRequestTargetFD},
		},
	)
	if err != nil {
		return BuildProbeReceipt{}, ErrUnavailable
	}
	guardian, err := os.FindProcess(pid)
	if err != nil {
		_ = killAndWaitDarwinProcess(nil, pid)
		return BuildProbeReceipt{}, ErrTermination
	}
	guardianReaped := false
	var targetIdentity *darwinUnreusableIdentity
	cleanGuardian := func() error {
		cleanupContext, cancelCleanup := context.WithTimeout(context.Background(), buildProbeCleanupLimit)
		defer cancelCleanup()
		if targetIdentity != nil {
			killDarwinUnreusableIdentity(cleanupContext, *targetIdentity)
		}
		killDarwinProcessGroup(pid)
		if !guardianReaped {
			guardianReaped = killAndWaitDarwinProcess(guardian, pid)
		}
		if targetIdentity != nil && waitDarwinUnreusableIdentityESRCH(cleanupContext, *targetIdentity) != nil {
			return ErrTermination
		}
		if !guardianReaped || waitDarwinProcessGroupESRCH(cleanupContext, pid) != nil {
			return ErrTermination
		}
		return nil
	}
	completed := false
	defer func() {
		if completed {
			return
		}
		if cleanupErr := cleanGuardian(); cleanupErr != nil {
			receipt = BuildProbeReceipt{}
			returnErr = cleanupErr
		}
	}()

	loadedCDHash, err := loadedProcessCDHash(pid)
	state, stateErr := readDarwinProcessState(ctx, pid, true)
	if err != nil || stateErr != nil || !codeDirectoryHashMatches(loadedCDHash, guardianCDHashes) ||
		!loadedDarwinExecutableMatchesOpenedFile(ctx, pid, guardianObject, authorityHash) ||
		state.Identity.PID != pid || state.Identity.PPID != os.Getpid() || state.Identity.PGID != pid ||
		state.Status != darwinProcStatusStopped || !darwinDirectoryPathMatchesAuthority(directories.work, directories.workAuthority) {
		return BuildProbeReceipt{}, ErrExecutableIdentity
	}
	if err := syscall.Kill(pid, syscall.SIGCONT); err != nil {
		return BuildProbeReceipt{}, ErrUnavailable
	}
	_ = controlReader.Close()
	_ = eventWriter.Close()
	_ = requestReader.Close()

	requestBody, err := json.Marshal(request)
	if err != nil || len(requestBody)+1 > BuildProbeRequestLimit {
		return BuildProbeReceipt{}, ErrBuildProbeInvalid
	}
	requestBody = append(requestBody, '\n')
	if err := writeBuildProbeAll(requestWriter, requestBody); err != nil || requestWriter.Close() != nil {
		return BuildProbeReceipt{}, ErrUnavailable
	}

	events := bufio.NewReaderSize(eventReader, buildProbeFrameLimit)
	armFrame, err := readBuildProbeFrame(ctx, eventReader, events, buildProbeFrameLimit)
	if err != nil {
		return BuildProbeReceipt{}, err
	}
	arm, err := decodeBuildProbeArm(armFrame)
	if err != nil || arm.PPID != pid || arm.PGID != pid || arm.UID != uint32(os.Geteuid()) {
		return BuildProbeReceipt{}, ErrBuildProbeInvalid
	}
	armedState, err := readDarwinProcessState(ctx, arm.PID, true)
	if err != nil || armedState.Status != darwinProcStatusStopped || armedState.Identity.PID != arm.PID ||
		armedState.Identity.PPID != arm.PPID || armedState.Identity.PGID != arm.PGID || armedState.Identity.UID != arm.UID ||
		armedState.Identity.StartSec != arm.StartSec || armedState.Identity.StartUSec != arm.StartUSec {
		return BuildProbeReceipt{}, ErrExecutableIdentity
	}
	stableTarget := darwinUnreusableIdentityFrom(armedState.Identity)
	targetIdentity = &stableTarget
	if _, err := controlWriter.Write([]byte{buildProbeArmAck}); err != nil {
		return BuildProbeReceipt{}, ErrUnavailable
	}

	receiptFrame, err := readBuildProbeFrame(ctx, eventReader, events, buildProbeFrameLimit)
	if err != nil {
		return BuildProbeReceipt{}, err
	}
	receipt, err = decodeBuildProbeReceipt(receiptFrame)
	if err != nil || !validBuildProbeReceipt(receipt, request) ||
		receipt.AuthoritySHA256 != authoritySHA256 {
		return BuildProbeReceipt{}, ErrBuildProbeInvalid
	}
	if _, err := controlWriter.Write([]byte{buildProbeFinishAck}); err != nil || controlWriter.Close() != nil {
		return BuildProbeReceipt{}, ErrUnavailable
	}
	if err := waitDarwinProcessExitUntil(pid, time.Now().Add(buildProbeCleanupLimit)); err != nil {
		return BuildProbeReceipt{}, ErrTermination
	}
	// Keep the waitable guardian PID reserved until any pre-arm process-group
	// member is killed, then reap the exact direct child.
	killDarwinProcessGroup(pid)
	guardianState, waitErr := guardian.Wait()
	guardianReaped = true
	if waitErr != nil || guardianState == nil || !guardianState.Success() {
		return BuildProbeReceipt{}, ErrTermination
	}
	cleanupContext, cancelCleanup := context.WithTimeout(context.Background(), buildProbeCleanupLimit)
	defer cancelCleanup()
	if waitDarwinUnreusableIdentityESRCH(cleanupContext, stableTarget) != nil {
		return BuildProbeReceipt{}, ErrTermination
	}
	if waitDarwinProcessGroupESRCH(cleanupContext, pid) != nil {
		return BuildProbeReceipt{}, ErrTermination
	}
	completed = true
	return receipt, nil
}

func runBuildProbeGuardianMain(protocol BuildProbeProtocol) int {
	if !protocol.valid() || !currentBuildProbeMainAllowed() || len(os.Args) != 2 || os.Args[1] != buildProbeGuardianArg {
		return 1
	}
	target := os.NewFile(3, "analytix-native-build-probe-target")
	control := os.NewFile(buildProbeControlTargetFD, "analytix-native-build-probe-control")
	events := os.NewFile(buildProbeEventTargetFD, "analytix-native-build-probe-events")
	requestFile, requestFileErr := openBuildProbeRequestInput(
		buildProbeRequestTargetFD, "analytix-native-build-probe-request",
	)
	if target == nil || control == nil || events == nil || requestFileErr != nil || requestFile == nil {
		return 1
	}
	defer target.Close()
	defer control.Close()
	defer events.Close()
	defer requestFile.Close()
	ctx, cancel := context.WithTimeout(context.Background(), buildProbeDeadline)
	defer cancel()
	request, err := readBuildProbeRequest(ctx, requestFile)
	if err != nil {
		return 1
	}
	armControlEvents := make(chan buildProbeControlEvent, 1)
	finishControlEvents := make(chan buildProbeControlEvent, 1)
	controlDone := make(chan struct{})
	go func() {
		defer close(controlDone)
		payload := []byte{0}
		_, readErr := io.ReadFull(control, payload)
		armEvent := buildProbeControlEvent{value: payload[0], err: readErr}
		armControlEvents <- armEvent
		if readErr != nil {
			finishControlEvents <- armEvent
			return
		}
		payload[0] = 0
		_, readErr = io.ReadFull(control, payload)
		finishControlEvents <- buildProbeControlEvent{value: payload[0], err: readErr}
	}()
	arm := func(armContext context.Context, identity darwinProcessIdentity) error {
		frame, marshalErr := json.Marshal(buildProbeArmEnvelope{
			Kind: "analytix_native_build_probe_arm", SchemaVersion: 1,
			PID: identity.PID, PPID: identity.PPID, PGID: identity.PGID, UID: identity.UID,
			StartSec: identity.StartSec, StartUSec: identity.StartUSec,
		})
		if marshalErr != nil || len(frame)+1 > buildProbeFrameLimit ||
			writeBuildProbeAll(events, append(frame, '\n')) != nil {
			return ErrBuildProbeInvalid
		}
		select {
		case controlEvent := <-armControlEvents:
			if controlEvent.err != nil || controlEvent.value != buildProbeArmAck {
				return ErrTermination
			}
			return nil
		case <-armContext.Done():
			return darwinContextFailure(armContext)
		}
	}

	probeDone := make(chan struct{})
	var receipt BuildProbeReceipt
	var probeErr error
	go func() {
		defer close(probeDone)
		receipt, probeErr = runBuildProbeWorker(ctx, request, target, protocol, arm)
	}()
	var finishEvent buildProbeControlEvent
	select {
	case <-probeDone:
	case finishEvent = <-finishControlEvents:
		cancel()
		<-probeDone
		if finishEvent.err != nil || finishEvent.value != buildProbeFinishAck {
			return 1
		}
		// A finish acknowledgement before a receipt is never valid.
		return 1
	}
	if probeErr != nil || ctx.Err() != nil || !validBuildProbeReceipt(receipt, request) {
		return 1
	}
	receiptBody, err := json.Marshal(receipt)
	if err != nil || len(receiptBody)+1 > buildProbeFrameLimit ||
		writeBuildProbeAll(events, append(receiptBody, '\n')) != nil {
		return 1
	}
	select {
	case finishEvent = <-finishControlEvents:
		if finishEvent.err != nil || finishEvent.value != buildProbeFinishAck {
			return 1
		}
	case <-ctx.Done():
		return 1
	}
	_ = control.Close()
	select {
	case <-controlDone:
	case <-time.After(buildProbeCleanupLimit):
		return 1
	}
	return 0
}

func runBuildProbeWorker(
	ctx context.Context,
	request buildProbeRequest,
	target *os.File,
	protocol BuildProbeProtocol,
	arm darwinTargetArm,
) (receipt BuildProbeReceipt, returnErr error) {
	if ctx == nil || target == nil || !protocol.valid() || arm == nil || validateBuildProbeRequest(request) != nil {
		return BuildProbeReceipt{}, ErrBuildProbeInvalid
	}
	policy := buildProbePolicies[BuildProbeComponent(request.ComponentID)]
	authority, authorityHash, err := openCurrentDarwinBootstrap(ctx)
	if err != nil {
		return BuildProbeReceipt{}, err
	}
	defer authority.file.Close()
	directories, err := openBuildProbePrivateDirectories()
	if err != nil {
		return BuildProbeReceipt{}, err
	}
	defer func() {
		if closeErr := directories.close(); closeErr != nil && returnErr == nil {
			receipt = BuildProbeReceipt{}
			returnErr = ErrTermination
		}
	}()
	launchNonce, err := randomBuildProbeID()
	if err != nil {
		return BuildProbeReceipt{}, ErrUnavailable
	}
	environment := append([]string(nil), policy.environment...)
	environment = append(environment,
		"ANALYTIX_NATIVE_LAUNCH_NONCE="+launchNonce,
		"ANALYTIX_NATIVE_PROTOCOL_VERSION=analytix-native-v1",
	)
	session, err := openSessionWithTargetArm(ctx, SessionConfig{
		Executable: target, ExpectedExecutableSHA256: request.ExpectedExecutableSHA256,
		ExpectedExecutableSize: request.ExpectedExecutableSize,
		Arguments:              append([]string(nil), policy.arguments...),
		WorkingDirectory:       directories.work, WorkingDirectoryAuthority: directories.workAuthority,
		StagingRoot: directories.stage, StagingRootAuthority: directories.stageAuthority,
		Environment: environment,
	}, arm)
	if err != nil {
		return BuildProbeReceipt{}, err
	}
	closed := false
	defer func() {
		if closed {
			return
		}
		if closeErr := session.Close(); closeErr != nil {
			receipt = BuildProbeReceipt{}
			returnErr = ErrTermination
		}
	}()
	pid := session.PID()
	if err := session.AuthenticateReadiness(ctx, buildProbeReadinessLimit, func(frame []byte, processID int) bool {
		return processID == pid && protocol.ValidateReady(frame, request.ComponentID, launchNonce, processID)
	}); err != nil {
		return BuildProbeReceipt{}, err
	}
	requestID, err := randomBuildProbeID()
	if err != nil {
		return BuildProbeReceipt{}, ErrUnavailable
	}
	requestFrame, err := protocol.EncodePing(requestID)
	if err != nil || len(requestFrame) == 0 || len(requestFrame) > buildProbeReadinessLimit {
		return BuildProbeReceipt{}, ErrProtocol
	}
	darwinAuthoritySession, ok := session.(*darwinSession)
	if !ok {
		return BuildProbeReceipt{}, ErrUnavailable
	}
	err = darwinAuthoritySession.buildProbeTerminalRoundTrip(
		ctx,
		requestFrame,
		buildProbeResponseLimit,
		directories,
		func(response []byte, processID int) bool {
			return processID == pid && protocol.ValidateResponse(response, request.ComponentID, requestID, processID)
		},
	)
	if err != nil {
		return BuildProbeReceipt{}, err
	}
	closed = true
	if !directories.valid() {
		return BuildProbeReceipt{}, ErrWorkingDirectory
	}
	if !buildProbeDirectoryEmpty(directories.workAuthority) || !buildProbeDirectoryEmpty(directories.stageAuthority) {
		return BuildProbeReceipt{}, ErrTermination
	}
	return BuildProbeReceipt{
		Kind: "analytix_native_build_probe_receipt", SchemaVersion: 1, Status: "passed",
		ComponentID: request.ComponentID, RequestNonce: request.RequestNonce,
		ExecutableSHA256: request.ExpectedExecutableSHA256, ExecutableSize: request.ExpectedExecutableSize,
		ManifestSHA256: request.ManifestSHA256, PolicySHA256: policy.policySHA256,
		AuthoritySHA256: hex.EncodeToString(authorityHash[:]), HostPlatform: runtime.GOOS,
		HostArch: buildProbeHostArch(), LoadedImageBound: true, WorkingDirectoryBound: true,
		GuardianAuthenticated: true, ProcessTreeEmpty: true,
	}, nil
}

type buildProbeTerminalValidator func([]byte, int) bool

// buildProbeTerminalRoundTrip has deliberately different semantics from the
// reusable Session.RoundTrip contract. Once the response frame is read, the
// target is frozen and is never resumed: validation, cwd/image/tree checks,
// kill/reap, and empty stdout/stderr EOF proof all happen before success.
func (session *darwinSession) buildProbeTerminalRoundTrip(
	ctx context.Context,
	request []byte,
	limit int,
	directories *buildProbePrivateDirectories,
	validator buildProbeTerminalValidator,
) error {
	if session == nil || ctx == nil || len(request) == 0 || len(request) > MaxSessionFrameBytes ||
		request[len(request)-1] != '\n' || bytes.IndexByte(request[:len(request)-1], '\n') >= 0 ||
		bytes.IndexByte(request, '\r') >= 0 || bytes.IndexByte(request, 0) >= 0 ||
		limit <= 0 || limit > MaxSessionFrameBytes || directories == nil || validator == nil {
		return ErrRequestInvalid
	}
	requestFrame := append([]byte(nil), request...)
	if err := session.acquire(ctx); err != nil {
		return err
	}
	defer session.release()
	if session.state != darwinSessionReady || session.unhealthyLocked() || darwinContextFailure(ctx) != nil ||
		session.pendingStdoutLocked() || !directories.valid() {
		return session.terminateWithCauseLocked(ErrProtocol)
	}
	if err := session.writeFrameLocked(ctx, requestFrame); err != nil {
		return session.terminateWithCauseLocked(err)
	}
	frame, frameErr := session.readFrameLocked(ctx, limit)
	freezeErr := error(nil)
	if frameErr == nil {
		freezeErr = session.tree.Freeze(ctx)
	}
	valid := frameErr == nil && freezeErr == nil && session.tree.RequireNoDescendants() == nil &&
		session.healthyFrozenLocked(ctx) && darwinContextFailure(ctx) == nil &&
		!session.pendingStdoutLocked() && directories.valid() &&
		darwinBuildProbeFrozenCWDMatches(ctx, session.pid, directories.work, directories.workAuthority)
	if valid {
		valid = callBuildProbeTerminalValidator(validator, frame, session.pid)
	}
	// The callback receives a copy and is not authoritative. Re-check every
	// frozen host fact after it returns before terminal cleanup.
	valid = valid && session.tree.RequireNoDescendants() == nil && session.healthyFrozenLocked(ctx) &&
		darwinContextFailure(ctx) == nil && !session.pendingStdoutLocked() && directories.valid() &&
		darwinBuildProbeFrozenCWDMatches(ctx, session.pid, directories.work, directories.workAuthority)
	session.state = darwinSessionClosed
	cleanupErr, channelProofErr := session.cleanupWithChannelProof(session.tree, true)
	if cleanupErr != nil || session.forcedClose.Load() {
		darwinAuthorityPoisoned.Store(true)
		session.terminationErr = ErrTermination
		return ErrTermination
	}
	if channelProofErr != nil || !valid {
		if frameErr != nil {
			return frameErr
		}
		return ErrProtocol
	}
	return nil
}

func callBuildProbeTerminalValidator(validator buildProbeTerminalValidator, frame []byte, pid int) (valid bool) {
	defer func() {
		if recover() != nil {
			valid = false
		}
	}()
	return validator(append([]byte(nil), frame...), pid)
}

func buildProbeDirectoryEmpty(directory *os.File) bool {
	if directory == nil {
		return false
	}
	duplicate, err := unix.FcntlInt(directory.Fd(), unix.F_DUPFD_CLOEXEC, 7)
	if err != nil || duplicate < 7 {
		if err == nil && duplicate >= 0 {
			_ = unix.Close(duplicate)
		}
		return false
	}
	reader := os.NewFile(uintptr(duplicate), "analytix-build-probe-directory-read")
	if reader == nil {
		_ = unix.Close(duplicate)
		return false
	}
	defer reader.Close()
	_, err = reader.Readdirnames(1)
	return errors.Is(err, io.EOF)
}

func openBuildProbePrivateDirectories() (*buildProbePrivateDirectories, error) {
	// TMPDIR is caller-controlled and therefore cannot select an authority
	// parent. /private/tmp is opened without following a final symlink and must
	// remain the root-owned, sticky, metadata-free Darwin system temporary root.
	return openBuildProbePrivateDirectoriesAt(buildProbeTempParent, 0, 0o1777)
}

func openBuildProbePrivateDirectoriesAt(parentPath string, parentOwner uint32, parentMode uint16) (*buildProbePrivateDirectories, error) {
	parentPath = filepath.Clean(parentPath)
	parentObject, err := openPinnedDarwinObject(parentPath, true)
	if err != nil || !darwinBuildProbeObjectSafe(parentObjectFile(parentObject), unix.S_IFDIR, parentMode, parentOwner) {
		if parentObject != nil && parentObject.file != nil {
			_ = parentObject.file.Close()
		}
		return nil, ErrWorkingDirectory
	}
	rootID, err := randomBuildProbeID()
	if err != nil {
		_ = parentObject.file.Close()
		return nil, ErrUnavailable
	}
	rootName := "analytix-native-build-probe-" + rootID
	parentFD := int(parentObject.file.Fd())
	if err := unix.Mkdirat(parentFD, rootName, 0o700); err != nil {
		_ = parentObject.file.Close()
		return nil, ErrWorkingDirectory
	}
	rootFD, err := unix.Openat(parentFD, rootName, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		_ = unix.Unlinkat(parentFD, rootName, unix.AT_REMOVEDIR)
		_ = parentObject.file.Close()
		return nil, ErrWorkingDirectory
	}
	rootAuthority := os.NewFile(uintptr(rootFD), filepath.Join(parentPath, rootName))
	provenance, provenanceOK := darwinBuildProbeProvenance(rootFD)
	if rootAuthority == nil || !provenanceOK ||
		!darwinBuildProbeCreatedObjectSafe(rootAuthority, unix.S_IFDIR, 0o700, uint32(os.Geteuid()), provenance) {
		if rootAuthority != nil {
			_ = rootAuthority.Close()
		} else {
			_ = unix.Close(rootFD)
		}
		_ = unix.Unlinkat(parentFD, rootName, unix.AT_REMOVEDIR)
		_ = parentObject.file.Close()
		return nil, ErrWorkingDirectory
	}
	workAuthority, err := createBuildProbePrivateDirectory(rootFD, "work", provenance)
	if err != nil {
		_ = rootAuthority.Close()
		_ = unix.Unlinkat(parentFD, rootName, unix.AT_REMOVEDIR)
		_ = parentObject.file.Close()
		return nil, ErrWorkingDirectory
	}
	stageAuthority, err := createBuildProbePrivateDirectory(rootFD, "stage", provenance)
	if err != nil {
		_ = workAuthority.Close()
		_ = unix.Unlinkat(rootFD, "work", unix.AT_REMOVEDIR)
		_ = rootAuthority.Close()
		_ = unix.Unlinkat(parentFD, rootName, unix.AT_REMOVEDIR)
		_ = parentObject.file.Close()
		return nil, ErrWorkingDirectory
	}
	root := filepath.Join(parentPath, rootName)
	directories := &buildProbePrivateDirectories{
		root: root, work: filepath.Join(root, "work"), stage: filepath.Join(root, "stage"),
		rootName: rootName, parentPath: parentPath, parentOwner: parentOwner, parentMode: parentMode,
		provenance:      append([]byte(nil), provenance...),
		parentAuthority: parentObject.file, rootAuthority: rootAuthority,
		workAuthority: workAuthority, stageAuthority: stageAuthority,
	}
	if !directories.valid() {
		_ = directories.close()
		return nil, ErrWorkingDirectory
	}
	return directories, nil
}

func parentObjectFile(object *pinnedDarwinObject) *os.File {
	if object == nil {
		return nil
	}
	return object.file

}

func createBuildProbePrivateDirectory(parentFD int, name string, provenance []byte) (*os.File, error) {
	if parentFD < 0 || (name != "work" && name != "stage") || unix.Mkdirat(parentFD, name, 0o700) != nil {
		return nil, ErrWorkingDirectory
	}
	fd, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		_ = unix.Unlinkat(parentFD, name, unix.AT_REMOVEDIR)
		return nil, ErrWorkingDirectory
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil ||
		!darwinBuildProbeCreatedObjectSafe(file, unix.S_IFDIR, 0o700, uint32(os.Geteuid()), provenance) {
		if file != nil {
			_ = file.Close()
		} else {
			_ = unix.Close(fd)
		}
		_ = unix.Unlinkat(parentFD, name, unix.AT_REMOVEDIR)
		return nil, ErrWorkingDirectory
	}
	return file, nil

}

func (directories *buildProbePrivateDirectories) valid() bool {
	if directories == nil || directories.parentAuthority == nil || directories.rootAuthority == nil ||
		directories.workAuthority == nil || directories.stageAuthority == nil {
		return false
	}
	return darwinBuildProbeObjectSafe(directories.parentAuthority, unix.S_IFDIR, directories.parentMode, directories.parentOwner) &&
		buildProbeParentPathMatchesAuthority(
			directories.parentPath, directories.parentAuthority, directories.parentMode, directories.parentOwner,
		) &&
		darwinBuildProbeCreatedObjectSafe(directories.rootAuthority, unix.S_IFDIR, 0o700, uint32(os.Geteuid()), directories.provenance) &&
		darwinBuildProbeCreatedObjectSafe(directories.workAuthority, unix.S_IFDIR, 0o700, uint32(os.Geteuid()), directories.provenance) &&
		darwinBuildProbeCreatedObjectSafe(directories.stageAuthority, unix.S_IFDIR, 0o700, uint32(os.Geteuid()), directories.provenance) &&
		darwinDirectoryPathMatchesAuthority(directories.root, directories.rootAuthority) &&
		darwinDirectoryPathMatchesAuthority(directories.work, directories.workAuthority) &&
		darwinDirectoryPathMatchesAuthority(directories.stage, directories.stageAuthority)
}

func buildProbeParentPathMatchesAuthority(path string, authority *os.File, mode uint16, owner uint32) bool {
	opened, err := openPinnedDarwinObject(path, true)
	if err != nil {
		return false
	}
	defer opened.file.Close()
	if !darwinBuildProbeObjectSafe(opened.file, unix.S_IFDIR, mode, owner) ||
		!darwinBuildProbeObjectSafe(authority, unix.S_IFDIR, mode, owner) {
		return false
	}
	var current unix.Stat_t
	return unix.Fstat(int(authority.Fd()), &current) == nil && current.Dev == opened.stat.Dev &&
		current.Ino == opened.stat.Ino && current.Mode == opened.stat.Mode && current.Uid == opened.stat.Uid
}

func (directories *buildProbePrivateDirectories) close() error {
	if directories == nil {
		return nil
	}
	directories.closeOnce.Do(func() {
		stageRemoveErr := unix.Unlinkat(int(directories.rootAuthority.Fd()), "stage", unix.AT_REMOVEDIR)
		workRemoveErr := unix.Unlinkat(int(directories.rootAuthority.Fd()), "work", unix.AT_REMOVEDIR)
		stageCloseErr := directories.stageAuthority.Close()
		workCloseErr := directories.workAuthority.Close()
		rootCloseErr := directories.rootAuthority.Close()
		rootRemoveErr := unix.Unlinkat(int(directories.parentAuthority.Fd()), directories.rootName, unix.AT_REMOVEDIR)
		parentCloseErr := directories.parentAuthority.Close()
		directories.closeErr = errors.Join(
			stageRemoveErr, workRemoveErr, stageCloseErr, workCloseErr, rootCloseErr, rootRemoveErr, parentCloseErr,
		)
	})
	return directories.closeErr
}

func decodeBuildProbeArm(raw []byte) (buildProbeArmEnvelope, error) {
	var arm buildProbeArmEnvelope
	if len(raw) == 0 || len(raw) >= buildProbeFrameLimit {
		return arm, ErrBuildProbeInvalid
	}
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: buildProbeFrameLimit - 1, MaxDepth: 2, MaxTokens: 32,
		MaxStringBytes: 128, MaxNumberBytes: 32, MaxAbsExponent: 1,
	}); err != nil {
		return arm, ErrBuildProbeInvalid
	}
	var shape map[string]json.RawMessage
	if json.Unmarshal(raw, &shape) != nil || len(shape) != 8 {
		return arm, ErrBuildProbeInvalid
	}
	for _, key := range []string{"kind", "schema_version", "pid", "ppid", "pgid", "uid", "start_sec", "start_usec"} {
		if _, ok := shape[key]; !ok {
			return arm, ErrBuildProbeInvalid
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&arm) != nil || arm.Kind != "analytix_native_build_probe_arm" || arm.SchemaVersion != 1 ||
		arm.PID <= 0 || arm.PPID <= 0 || arm.PGID <= 0 || arm.UID != uint32(os.Geteuid()) ||
		arm.StartSec == 0 || arm.StartUSec >= 1_000_000 {
		return buildProbeArmEnvelope{}, ErrBuildProbeInvalid
	}
	return arm, nil
}

func readBuildProbeFrame(ctx context.Context, file *os.File, reader *bufio.Reader, limit int) ([]byte, error) {
	if ctx == nil || file == nil || reader == nil || limit <= 1 || limit > buildProbeFrameLimit {
		return nil, ErrBuildProbeInvalid
	}
	stop, err := applyDarwinFileDeadline(ctx, file, true)
	if err != nil {
		return nil, err
	}
	defer stop()
	frame, readErr := reader.ReadSlice('\n')
	if readErr != nil || len(frame) <= 1 || len(frame) > limit || bytes.IndexByte(frame, '\r') >= 0 || bytes.IndexByte(frame, 0) >= 0 {
		if readErr != nil {
			return nil, classifyDarwinSessionIOError(ctx, readErr)
		}
		return nil, ErrBuildProbeInvalid
	}
	return append([]byte(nil), frame[:len(frame)-1]...), nil
}

func randomBuildProbeID() (string, error) {
	var value [sha256.Size]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

type darwinUnreusableIdentity struct {
	PID       int
	UID       uint32
	StartSec  uint64
	StartUSec uint64
}

func darwinUnreusableIdentityFrom(identity darwinProcessIdentity) darwinUnreusableIdentity {
	return darwinUnreusableIdentity{
		PID: identity.PID, UID: identity.UID, StartSec: identity.StartSec, StartUSec: identity.StartUSec,
	}
}

func (identity darwinUnreusableIdentity) matches(state darwinProcessState) bool {
	return identity.PID > 0 && state.Identity.PID == identity.PID && state.Identity.UID == identity.UID &&
		state.Identity.StartSec == identity.StartSec && state.Identity.StartUSec == identity.StartUSec
}

func killDarwinUnreusableIdentity(ctx context.Context, identity darwinUnreusableIdentity) {
	state, err := readDarwinProcessState(ctx, identity.PID, true)
	if err != nil || !identity.matches(state) {
		return
	}
	if state.Identity.PGID > 0 {
		_ = syscall.Kill(-state.Identity.PGID, syscall.SIGKILL)
	}
	_ = syscall.Kill(identity.PID, syscall.SIGKILL)
}

func waitDarwinUnreusableIdentityESRCH(ctx context.Context, identity darwinUnreusableIdentity) error {
	for {
		if err := darwinContextFailure(ctx); err != nil {
			return err
		}
		state, err := readDarwinProcessState(ctx, identity.PID, true)
		if errors.Is(err, syscall.ESRCH) || errors.Is(err, unix.ESRCH) {
			return nil
		}
		if err != nil || !identity.matches(state) {
			return ErrTermination
		}
		time.Sleep(time.Millisecond)
	}
}
