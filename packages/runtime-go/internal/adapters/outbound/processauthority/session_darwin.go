//go:build darwin

package processauthority

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type darwinSessionState uint8

const (
	darwinSessionAwaitingReadiness darwinSessionState = iota + 1
	darwinSessionReady
	darwinSessionClosed
)

const (
	darwinReadOnlyInputTargetFD   = 3
	darwinReadWriteOutputTargetFD = 5
	maxDarwinReadOnlyInputBytes   = int64(1 << 40)
)

type darwinSession struct {
	gate             chan struct{}
	state            darwinSessionState
	direct           bool
	pid              int
	identity         darwinProcessIdentity
	process          *os.Process
	stdin            *os.File
	stdout           *os.File
	stdoutReader     *bufio.Reader
	stderr           *os.File
	stderrReader     *bufio.Reader
	ownerLiveness    *os.File
	targetStaged     *pinnedDarwinObject
	targetPath       string
	targetHash       [sha256.Size]byte
	targetCDHashes   [][cdHashBytes]byte
	workingDirectory *pinnedDarwinObject
	workingAuthority *os.File
	stagingRoot      string
	stagingAuthority *os.File
	bootstrapStage   *os.File
	cleanupTarget    func() error
	cleanupBootstrap func() error
	tree             *darwinProcessTree
	transitions      *darwinProcessTransitionMonitor
	cleanupOnce      sync.Once
	cleanupErr       error
	channelProofErr  error
	forcedClose      atomic.Bool
	gateDeadline     time.Duration
	terminationErr   error
}

type darwinTargetArm func(context.Context, darwinProcessIdentity) error

func openSessionWithTargetArm(
	ctx context.Context,
	config SessionConfig,
	arm darwinTargetArm,
) (sessionResult Session, returnErr error) {
	if config.Executable == nil {
		if config.ReadOnlyInput != nil && config.ReadOnlyInput.File != nil {
			_ = config.ReadOnlyInput.File.Close()
		}
		if config.ReadWriteOutput != nil && config.ReadWriteOutput.File != nil {
			_ = config.ReadWriteOutput.File.Close()
		}
		return nil, ErrRequestInvalid
	}
	executable := config.Executable
	defer executable.Close()
	ownedReadOnlyInput, err := takeDarwinReadOnlyInput(config.ReadOnlyInput)
	if err != nil {
		if config.ReadWriteOutput != nil && config.ReadWriteOutput.File != nil {
			_ = config.ReadWriteOutput.File.Close()
		}
		return nil, err
	}
	config.ReadOnlyInput = ownedReadOnlyInput
	if ownedReadOnlyInput != nil {
		defer ownedReadOnlyInput.File.Close()
	}
	ownedReadWriteOutput, err := takeDarwinReadWriteOutput(config.ReadWriteOutput)
	if err != nil {
		return nil, err
	}
	config.ReadWriteOutput = ownedReadWriteOutput
	if ownedReadWriteOutput != nil {
		defer ownedReadWriteOutput.File.Close()
	}
	if !currentBootstrapMainAllowed() && !(arm != nil && currentBuildProbeMainAllowed()) {
		return nil, ErrUnavailable
	}
	if darwinAuthorityPoisoned.Load() {
		return nil, ErrUnavailable
	}
	validated, executableHash, err := validateDarwinSessionConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	if arm != nil && (validated.ReadOnlyInput != nil || validated.ReadWriteOutput != nil) {
		return nil, ErrRequestInvalid
	}
	workingDirectory, err := openPinnedDarwinObject(validated.WorkingDirectory, true)
	if err != nil {
		return nil, ErrWorkingDirectory
	}
	defer workingDirectory.file.Close()
	if workingDirectory.stat.Mode&unix.S_IFMT != unix.S_IFDIR || workingDirectory.stat.Mode&0o022 != 0 ||
		!darwinDirectoryAuthorityMatches(workingDirectory, validated.WorkingDirectoryAuthority) {
		return nil, ErrWorkingDirectory
	}
	stageExecutable := stageDarwinExecutableWithAuthority
	if arm != nil {
		stageExecutable = stageDarwinBuildProbeExecutableWithAuthority
	}
	targetPath, targetFile, cleanupTarget, err := stageExecutable(
		ctx, validated.StagingRoot, validated.StagingRootAuthority,
		executable, validated.ExpectedExecutableSize, executableHash,
	)
	if err != nil {
		return nil, ErrExecutableIdentity
	}
	targetCDHashes, err := openedFileCodeDirectoryHashes(targetFile)
	if err != nil {
		_ = targetFile.Close()
		_ = cleanupTarget()
		return nil, ErrExecutableIdentity
	}
	var targetStat unix.Stat_t
	if unix.Fstat(int(targetFile.Fd()), &targetStat) != nil {
		_ = targetFile.Close()
		_ = cleanupTarget()
		return nil, ErrExecutableIdentity
	}
	targetObject := &pinnedDarwinObject{file: targetFile, stat: targetStat}
	bootstrapSource, bootstrapHash, err := openCurrentDarwinBootstrap(ctx)
	if err != nil {
		_ = targetFile.Close()
		_ = cleanupTarget()
		return nil, err
	}
	defer bootstrapSource.file.Close()
	bootstrapPath, bootstrapFile, cleanupBootstrap, err := stageExecutable(
		ctx, validated.StagingRoot, validated.StagingRootAuthority,
		bootstrapSource.file, bootstrapSource.stat.Size, bootstrapHash,
	)
	if err != nil {
		_ = targetFile.Close()
		_ = cleanupTarget()
		return nil, ErrExecutableIdentity
	}
	bootstrapCDHashes, err := openedFileCodeDirectoryHashes(bootstrapFile)
	if err != nil {
		_ = bootstrapFile.Close()
		_ = cleanupBootstrap()
		_ = targetFile.Close()
		_ = cleanupTarget()
		return nil, ErrExecutableIdentity
	}
	var bootstrapStat unix.Stat_t
	if unix.Fstat(int(bootstrapFile.Fd()), &bootstrapStat) != nil {
		_ = bootstrapFile.Close()
		_ = cleanupBootstrap()
		_ = targetFile.Close()
		_ = cleanupTarget()
		return nil, ErrExecutableIdentity
	}
	bootstrapObject := &pinnedDarwinObject{file: bootstrapFile, stat: bootstrapStat}
	cleanupOnFailure := true
	defer func() {
		if !cleanupOnFailure {
			return
		}
		bootstrapCloseErr := bootstrapFile.Close()
		bootstrapCleanupErr := cleanupBootstrap()
		targetCloseErr := targetFile.Close()
		targetCleanupErr := cleanupTarget()
		if bootstrapCloseErr != nil || bootstrapCleanupErr != nil || targetCloseErr != nil || targetCleanupErr != nil {
			darwinAuthorityPoisoned.Store(true)
			returnErr = ErrTermination
		}
	}()
	stdinReader, stdinWriter, stdoutReader, stdoutWriter, stderrReader, stderrWriter, err := darwinProcessPipes()
	if err != nil {
		return nil, ErrUnavailable
	}
	var ownerLivenessReader *os.File
	var ownerLivenessWriter *os.File
	if arm == nil {
		ownerLivenessReader, ownerLivenessWriter, err = openDarwinOwnerLivenessPipe()
		if err != nil {
			_ = stdinReader.Close()
			_ = stdinWriter.Close()
			_ = stdoutReader.Close()
			_ = stdoutWriter.Close()
			_ = stderrReader.Close()
			_ = stderrWriter.Close()
			return nil, err
		}
	}
	closePipes := func() {
		_ = stdinReader.Close()
		_ = stdinWriter.Close()
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		_ = stderrReader.Close()
		_ = stderrWriter.Close()
		if ownerLivenessReader != nil {
			_ = ownerLivenessReader.Close()
		}
		if ownerLivenessWriter != nil {
			_ = ownerLivenessWriter.Close()
		}
	}
	spawned := false
	var process *os.Process
	var pid int
	defer func() {
		if cleanupOnFailure {
			closePipes()
			if spawned && !killAndWaitDarwinProcess(process, pid) {
				darwinAuthorityPoisoned.Store(true)
				returnErr = ErrTermination
			}
		}
	}()
	if err := darwinContextFailure(ctx); err != nil {
		return nil, err
	}
	if !darwinDirectoryPathMatchesAuthority(validated.StagingRoot, validated.StagingRootAuthority) ||
		!pinnedDarwinObjectUnchanged(workingDirectory) ||
		!darwinDirectoryAuthorityMatches(workingDirectory, validated.WorkingDirectoryAuthority) {
		return nil, ErrWorkingDirectory
	}
	if validated.ReadOnlyInput != nil {
		if err := validateDarwinReadOnlyInput(ctx, *validated.ReadOnlyInput); err != nil {
			return nil, err
		}
	}
	if validated.ReadWriteOutput != nil {
		if err := validateDarwinReadWriteOutput(*validated.ReadWriteOutput); err != nil {
			return nil, err
		}
	}
	bootstrapNonce, err := randomDarwinBootstrapNonce()
	if err != nil {
		return nil, ErrUnavailable
	}
	bootstrapArguments := []string{bootstrapPath, "-test.run=^$", "--", darwinBootstrapMarker}
	bootstrapEnvironment := []string{
		darwinBootstrapEnvironment + "=1",
		darwinBootstrapNonceEnv + "=" + bootstrapNonce,
		"LANG=C", "LC_ALL=C", "TZ=UTC",
	}
	spawn := spawnSuspended
	if arm != nil {
		bootstrapEnvironment = append(bootstrapEnvironment, darwinBuildProbeBootstrapEnvironment+"=1")
		spawn = spawnSuspendedInInheritedProcessGroup
	}
	if arm != nil {
		pid, err = spawn(
			bootstrapPath,
			bootstrapArguments,
			bootstrapEnvironment,
			int(workingDirectory.file.Fd()),
			int(stdinReader.Fd()),
			int(stdoutWriter.Fd()),
			int(stderrWriter.Fd()),
		)
	} else {
		extra := []darwinSpawnFile{{
			source: int(ownerLivenessReader.Fd()),
			target: darwinOwnerLivenessTargetFD,
		}}
		if validated.ReadOnlyInput != nil {
			extra = append(extra, darwinSpawnFile{
				source: int(validated.ReadOnlyInput.File.Fd()),
				target: darwinReadOnlyInputTargetFD,
			})
		}
		if validated.ReadWriteOutput != nil {
			extra = append(extra, darwinSpawnFile{
				source: int(validated.ReadWriteOutput.File.Fd()),
				target: darwinReadWriteOutputTargetFD,
			})
		}
		pid, err = spawnSuspendedWithExtraFiles(
			bootstrapPath,
			bootstrapArguments,
			bootstrapEnvironment,
			int(workingDirectory.file.Fd()),
			int(stdinReader.Fd()),
			int(stdoutWriter.Fd()),
			int(stderrWriter.Fd()),
			extra,
		)
	}
	if err != nil {
		return nil, ErrUnavailable
	}
	spawned = true
	if ownerLivenessReader != nil {
		if err := ownerLivenessReader.Close(); err != nil {
			return nil, ErrTermination
		}
		ownerLivenessReader = nil
	}
	process, err = os.FindProcess(pid)
	if err != nil {
		return nil, ErrTermination
	}
	if err := darwinContextFailure(ctx); err != nil {
		return nil, err
	}
	loadedCDHash, err := loadedProcessCDHash(pid)
	if err != nil || !codeDirectoryHashMatches(loadedCDHash, bootstrapCDHashes) ||
		!pinnedDarwinObjectUnchanged(bootstrapObject) ||
		!loadedDarwinExecutableMatchesOpenedFile(ctx, pid, bootstrapObject, bootstrapHash) ||
		!pinnedDarwinObjectUnchanged(workingDirectory) ||
		!darwinDirectoryAuthorityMatches(workingDirectory, validated.WorkingDirectoryAuthority) ||
		!darwinDirectoryPathMatchesAuthority(validated.StagingRoot, validated.StagingRootAuthority) {
		return nil, ErrExecutableIdentity
	}
	if err := darwinContextFailure(ctx); err != nil {
		return nil, err
	}
	if darwinAuthorityPoisoned.Load() {
		return nil, ErrUnavailable
	}
	rootState, err := readDarwinProcessState(ctx, pid, true)
	wantInitialPGID := pid
	if arm != nil {
		wantInitialPGID = syscall.Getpgrp()
	}
	if err != nil || rootState.Identity.PID != pid || rootState.Identity.PPID != os.Getpid() ||
		rootState.Identity.PGID != wantInitialPGID || rootState.Status != darwinProcStatusStopped {
		return nil, ErrExecutableIdentity
	}
	if arm != nil {
		if err := arm(ctx, rootState.Identity); err != nil {
			return nil, err
		}
	}
	if err := syscall.Kill(pid, syscall.SIGCONT); err != nil {
		return nil, ErrUnavailable
	}
	_ = stdinReader.Close()
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	stdoutBuffered := bufio.NewReaderSize(stdoutReader, 64*1024)
	stderrBuffered := bufio.NewReaderSize(stderrReader, 64*1024)
	proof, err := readDarwinBootstrapProof(ctx, stdoutReader, stdoutBuffered, 4096)
	wantProof := darwinBootstrapProof(bootstrapNonce, pid)
	stderrPending := darwinReaderHasPending(stderrReader, stderrBuffered)
	if err != nil || !bytes.Equal(proof, wantProof) || stderrPending {
		return nil, fmt.Errorf("%w: bootstrap proof", ErrProtocol)
	}
	if arm != nil {
		rootState, err = readDarwinProcessState(ctx, pid, true)
		if err != nil || rootState.Identity.PID != pid || rootState.Identity.PPID != os.Getpid() ||
			rootState.Identity.PGID != pid || rootState.Status == darwinProcStatusZombie {
			return nil, ErrExecutableIdentity
		}
	}
	processTree := newDarwinProcessTree(rootState.Identity)
	if processTree == nil {
		return nil, ErrExecutableIdentity
	}
	if err := processTree.Freeze(ctx); err != nil || processTree.RequireNoDescendants() != nil ||
		!darwinFrozenImageValid(ctx, processTree, bootstrapObject, bootstrapHash, bootstrapCDHashes) ||
		darwinReaderHasPending(stdoutReader, stdoutBuffered) || darwinReaderHasPending(stderrReader, stderrBuffered) {
		return nil, fmt.Errorf("%w: bootstrap frozen image", ErrProtocol)
	}
	command, err := encodeDarwinBootstrapCommand(darwinBootstrapCommand{
		target: targetPath, arguments: validated.Arguments, environment: validated.Environment,
	})
	if err != nil {
		return nil, ErrRequestInvalid
	}
	if err := processTree.ResumeRoot(ctx); err != nil {
		return nil, fmt.Errorf("%w: bootstrap resume", ErrProtocol)
	}
	if err := writeDarwinSessionBytes(ctx, stdinWriter, command); err != nil {
		return nil, err
	}
	if err := awaitDarwinTargetHandoff(
		ctx, processTree, bootstrapObject, targetObject, executableHash, targetCDHashes,
	); err != nil {
		return nil, err
	}
	if darwinReaderHasPending(stderrReader, stderrBuffered) {
		return nil, fmt.Errorf("%w: target stderr", ErrProtocol)
	}
	transitions, err := openDarwinProcessTransitionMonitor(pid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cleanupOnFailure {
			_ = transitions.Close()
		}
	}()
	if err := processTree.ResumeRoot(ctx); err != nil {
		return nil, fmt.Errorf("%w: target resume", ErrProtocol)
	}
	session := &darwinSession{
		gate: make(chan struct{}, 1), state: darwinSessionAwaitingReadiness,
		pid: pid, process: process, stdin: stdinWriter, stdout: stdoutReader,
		stdoutReader: stdoutBuffered, stderr: stderrReader, stderrReader: stderrBuffered,
		targetStaged: targetObject, targetHash: executableHash, targetCDHashes: targetCDHashes,
		ownerLiveness:  ownerLivenessWriter,
		bootstrapStage: bootstrapFile, cleanupTarget: cleanupTarget,
		cleanupBootstrap: cleanupBootstrap, tree: processTree,
		transitions:  transitions,
		gateDeadline: darwinSessionCleanupDeadline,
	}
	ownerLivenessWriter = nil
	session.gate <- struct{}{}
	cleanupOnFailure = false
	return session, nil
}

// takeDarwinReadOnlyInput consumes the caller handle and pins an
// authority-owned duplicate while os.File still serializes Close against the
// raw descriptor callback. Later validation and spawn never consult the
// caller's descriptor number, so close-and-reuse cannot substitute FD 3.
func takeDarwinReadOnlyInput(input *ReadOnlyInput) (*ReadOnlyInput, error) {
	if input == nil {
		return nil, nil
	}
	if input.File == nil {
		return nil, ErrRequestInvalid
	}
	original := input.File
	raw, err := original.SyscallConn()
	if err != nil {
		_ = original.Close()
		return nil, ErrExecutableIdentity
	}
	duplicateFD := -1
	var duplicateErr error
	controlErr := raw.Control(func(fd uintptr) {
		duplicateFD, duplicateErr = unix.FcntlInt(fd, unix.F_DUPFD_CLOEXEC, 8)
	})
	closeErr := original.Close()
	if controlErr != nil || duplicateErr != nil || closeErr != nil || duplicateFD < 0 {
		if duplicateFD >= 0 {
			_ = unix.Close(duplicateFD)
		}
		return nil, ErrExecutableIdentity
	}
	owned := os.NewFile(uintptr(duplicateFD), "process-authority-read-only-input")
	if owned == nil {
		_ = unix.Close(duplicateFD)
		return nil, ErrTermination
	}
	return &ReadOnlyInput{
		File:           owned,
		ExpectedSHA256: input.ExpectedSHA256,
		ExpectedSize:   input.ExpectedSize,
	}, nil
}

// takeDarwinReadWriteOutput consumes the caller handle and pins the exact
// unlinked output inode behind an authority-owned duplicate. Descriptor-number
// reuse by the caller therefore cannot substitute the child FD 5 capability.
func takeDarwinReadWriteOutput(output *ReadWriteOutput) (*ReadWriteOutput, error) {
	if output == nil {
		return nil, nil
	}
	if output.File == nil {
		return nil, ErrRequestInvalid
	}
	original := output.File
	raw, err := original.SyscallConn()
	if err != nil {
		_ = original.Close()
		return nil, ErrExecutableIdentity
	}
	duplicateFD := -1
	var duplicateErr error
	controlErr := raw.Control(func(fd uintptr) {
		duplicateFD, duplicateErr = unix.FcntlInt(fd, unix.F_DUPFD_CLOEXEC, 8)
	})
	closeErr := original.Close()
	if controlErr != nil || duplicateErr != nil || closeErr != nil || duplicateFD < 0 {
		if duplicateFD >= 0 {
			_ = unix.Close(duplicateFD)
		}
		return nil, ErrExecutableIdentity
	}
	owned := os.NewFile(uintptr(duplicateFD), "process-authority-read-write-output")
	if owned == nil {
		_ = unix.Close(duplicateFD)
		return nil, ErrTermination
	}
	return &ReadWriteOutput{File: owned}, nil
}

func openCurrentDarwinBootstrap(ctx context.Context) (*pinnedDarwinObject, [sha256.Size]byte, error) {
	var zero [sha256.Size]byte
	path, err := loadedDarwinProcessPath(os.Getpid())
	if err != nil {
		return nil, zero, ErrExecutableIdentity
	}
	opened, err := openPinnedDarwinObject(path, false)
	if err != nil {
		return nil, zero, ErrExecutableIdentity
	}
	if opened.stat.Mode&unix.S_IFMT != unix.S_IFREG || opened.stat.Nlink != 1 ||
		opened.stat.Size <= 0 || opened.stat.Size > maxExecutableBytes ||
		(opened.stat.Uid != 0 && opened.stat.Uid != uint32(os.Geteuid())) || opened.stat.Mode&0o022 != 0 {
		_ = opened.file.Close()
		return nil, zero, ErrExecutableIdentity
	}
	digest, err := sha256OpenedFile(ctx, opened.file)
	if err != nil || !pinnedDarwinObjectUnchanged(opened) ||
		!loadedDarwinExecutableMatchesOpenedFile(ctx, os.Getpid(), opened, digest) {
		_ = opened.file.Close()
		return nil, zero, ErrExecutableIdentity
	}
	openedCDHashes, err := openedFileCodeDirectoryHashes(opened.file)
	loadedCDHash, loadedErr := loadedProcessCDHash(os.Getpid())
	if err != nil || loadedErr != nil || !codeDirectoryHashMatches(loadedCDHash, openedCDHashes) {
		_ = opened.file.Close()
		return nil, zero, ErrExecutableIdentity
	}
	return opened, digest, nil
}

func randomDarwinBootstrapNonce() (string, error) {
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(nonce[:]), nil
}

func darwinBootstrapProof(nonce string, pid int) []byte {
	return []byte(
		`{"kind":"analytix_process_authority_bootstrap","schema_version":1,"nonce":"` + nonce +
			`","process_id":` + strconv.Itoa(pid) +
			`,"nproc_soft":0,"nproc_hard":0,"fork_probe_errno":"EAGAIN"}`,
	)
}

func readDarwinBootstrapProof(ctx context.Context, file *os.File, reader *bufio.Reader, limit int) ([]byte, error) {
	if ctx == nil || file == nil || reader == nil || limit <= 0 {
		return nil, ErrRequestInvalid
	}
	stop, err := applyDarwinFileDeadline(ctx, file, true)
	if err != nil {
		return nil, err
	}
	defer stop()
	frame, readErr := reader.ReadSlice('\n')
	if readErr != nil || len(frame) <= 1 || len(frame) > limit ||
		bytes.IndexByte(frame, '\r') >= 0 || bytes.IndexByte(frame, 0) >= 0 {
		if readErr != nil {
			return nil, classifyDarwinSessionIOError(ctx, readErr)
		}
		return nil, ErrProtocol
	}
	return append([]byte(nil), frame[:len(frame)-1]...), nil
}

func darwinReaderHasPending(file *os.File, reader *bufio.Reader) bool {
	if file == nil || reader == nil || reader.Buffered() != 0 {
		return true
	}
	if err := file.SetReadDeadline(time.Now().Add(time.Millisecond)); err != nil {
		return true
	}
	_, err := reader.Peek(1)
	_ = file.SetReadDeadline(time.Time{})
	return err == nil || (!errors.Is(err, os.ErrDeadlineExceeded) && !errors.Is(err, syscall.EAGAIN))
}

func writeDarwinSessionBytes(ctx context.Context, file *os.File, payload []byte) error {
	if len(payload) == 0 {
		return ErrRequestInvalid
	}
	stop, err := applyDarwinFileDeadline(ctx, file, false)
	if err != nil {
		return err
	}
	defer stop()
	for len(payload) > 0 {
		if err := darwinContextFailure(ctx); err != nil {
			return err
		}
		written, writeErr := file.Write(payload)
		if writeErr != nil || written <= 0 {
			return classifyDarwinSessionIOError(ctx, writeErr)
		}
		payload = payload[written:]
	}
	return nil
}

func awaitDarwinTargetHandoff(
	ctx context.Context,
	tree *darwinProcessTree,
	bootstrap *pinnedDarwinObject,
	target *pinnedDarwinObject,
	targetHash [sha256.Size]byte,
	targetCDHashes [][cdHashBytes]byte,
) error {
	if tree == nil || bootstrap == nil || bootstrap.file == nil || target == nil || target.file == nil {
		return ErrRequestInvalid
	}
	for {
		if err := darwinContextFailure(ctx); err != nil {
			return err
		}
		if exited, err := pollDarwinProcessExit(tree.root.PID); err != nil || exited {
			return ErrAbnormalExit
		}
		loadedPath, err := loadedDarwinProcessPath(tree.root.PID)
		if err != nil {
			return ErrExecutableIdentity
		}
		if darwinPathMatchesPinnedObject(loadedPath, target) {
			if err := tree.Freeze(ctx); err != nil || tree.RequireNoDescendants() != nil ||
				!darwinFrozenImageValid(ctx, tree, target, targetHash, targetCDHashes) {
				return ErrExecutableIdentity
			}
			return nil
		}
		if !darwinPathMatchesPinnedObject(loadedPath, bootstrap) {
			return ErrExecutableIdentity
		}
		time.Sleep(time.Millisecond)
	}
}

func darwinFrozenImageValid(
	ctx context.Context,
	tree *darwinProcessTree,
	image *pinnedDarwinObject,
	expectedHash [sha256.Size]byte,
	openedCDHashes [][cdHashBytes]byte,
) bool {
	if tree == nil || image == nil || image.file == nil || !tree.frozen || tree.RequireNoDescendants() != nil ||
		darwinContextFailure(ctx) != nil || !pinnedDarwinObjectUnchanged(image) {
		return false
	}
	state, err := readDarwinProcessState(ctx, tree.root.PID, true)
	if err != nil || !state.sameIdentity(tree.root) || state.Status != darwinProcStatusStopped {
		return false
	}
	loadedCDHash, err := loadedProcessCDHash(tree.root.PID)
	return err == nil && codeDirectoryHashMatches(loadedCDHash, openedCDHashes) &&
		loadedDarwinExecutableMatchesOpenedFile(ctx, tree.root.PID, image, expectedHash)
}

func validateDarwinSessionConfig(ctx context.Context, config SessionConfig) (SessionConfig, [sha256.Size]byte, error) {
	var expected [sha256.Size]byte
	if config.ReadOnlyInput != nil {
		input := *config.ReadOnlyInput
		config.ReadOnlyInput = &input
	}
	if config.ReadWriteOutput != nil {
		output := *config.ReadWriteOutput
		config.ReadWriteOutput = &output
	}
	deadline, hasDeadline := time.Time{}, false
	if ctx != nil {
		deadline, hasDeadline = ctx.Deadline()
	}
	if ctx == nil || darwinContextFailure(ctx) != nil || !hasDeadline || !deadline.After(time.Now()) || config.Executable == nil ||
		(config.ReadWriteOutput != nil && config.ReadOnlyInput == nil) ||
		config.WorkingDirectoryAuthority == nil || config.StagingRootAuthority == nil ||
		config.ExpectedExecutableSize <= 0 || config.ExpectedExecutableSize > maxExecutableBytes ||
		!filepath.IsAbs(config.WorkingDirectory) || !filepath.IsAbs(config.StagingRoot) ||
		len(config.Arguments) > MaxArguments-1 ||
		len(config.Environment) == 0 || len(config.Environment) > MaxEnvironment ||
		len(config.ExpectedExecutableSHA256) != sha256.Size*2 ||
		config.ExpectedExecutableSHA256 != strings.ToLower(config.ExpectedExecutableSHA256) {
		return SessionConfig{}, expected, ErrRequestInvalid
	}
	decoded, err := hex.DecodeString(config.ExpectedExecutableSHA256)
	if err != nil {
		return SessionConfig{}, expected, ErrRequestInvalid
	}
	copy(expected[:], decoded)
	argumentBytes := 0
	for _, value := range config.Arguments {
		if value == "" || strings.ContainsRune(value, 0) {
			return SessionConfig{}, expected, ErrRequestInvalid
		}
		argumentBytes += len(value) + 1
	}
	if argumentBytes > MaxArgumentBytes {
		return SessionConfig{}, expected, ErrRequestInvalid
	}
	environmentBytes := 0
	seen := make(map[string]struct{}, len(config.Environment))
	for _, value := range config.Environment {
		separator := strings.IndexByte(value, '=')
		if separator <= 0 || strings.ContainsRune(value, 0) {
			return SessionConfig{}, expected, ErrRequestInvalid
		}
		name := value[:separator]
		if name != strings.ToUpper(name) {
			return SessionConfig{}, expected, ErrRequestInvalid
		}
		if _, ok := allowedDarwinEnvironment[name]; !ok {
			return SessionConfig{}, expected, ErrRequestInvalid
		}
		if _, duplicate := seen[name]; duplicate {
			return SessionConfig{}, expected, ErrRequestInvalid
		}
		seen[name] = struct{}{}
		environmentBytes += len(value) + 1
	}
	if environmentBytes > MaxEnvironmentByte {
		return SessionConfig{}, expected, ErrRequestInvalid
	}
	var before unix.Stat_t
	if unix.Fstat(int(config.Executable.Fd()), &before) != nil || before.Mode&unix.S_IFMT != unix.S_IFREG ||
		before.Nlink != 1 || before.Size != config.ExpectedExecutableSize || before.Mode&0o022 != 0 {
		return SessionConfig{}, expected, ErrExecutableIdentity
	}
	currentHash, err := sha256OpenedFile(ctx, config.Executable)
	var after unix.Stat_t
	if err != nil || currentHash != expected || unix.Fstat(int(config.Executable.Fd()), &after) != nil ||
		componentDarwinFileIdentity(before) != componentDarwinFileIdentity(after) {
		return SessionConfig{}, expected, ErrExecutableIdentity
	}
	if config.ReadOnlyInput != nil {
		if err := validateDarwinReadOnlyInput(ctx, *config.ReadOnlyInput); err != nil {
			return SessionConfig{}, expected, err
		}
	}
	if config.ReadWriteOutput != nil {
		if err := validateDarwinReadWriteOutput(*config.ReadWriteOutput); err != nil {
			return SessionConfig{}, expected, err
		}
	}
	config.Arguments = append([]string(nil), config.Arguments...)
	config.Environment = append([]string(nil), config.Environment...)
	config.WorkingDirectory = filepath.Clean(config.WorkingDirectory)
	config.StagingRoot = filepath.Clean(config.StagingRoot)
	return config, expected, nil
}

func validateDarwinReadOnlyInput(ctx context.Context, input ReadOnlyInput) error {
	if ctx == nil || input.File == nil || input.ExpectedSize <= 0 ||
		input.ExpectedSize > maxDarwinReadOnlyInputBytes ||
		len(input.ExpectedSHA256) != sha256.Size*2 ||
		input.ExpectedSHA256 != strings.ToLower(input.ExpectedSHA256) {
		return ErrRequestInvalid
	}
	expected, err := hex.DecodeString(input.ExpectedSHA256)
	if err != nil || len(expected) != sha256.Size {
		return ErrRequestInvalid
	}
	fd := int(input.File.Fd())
	var before unix.Stat_t
	statusFlags, flagsErr := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	descriptorFlags, descriptorErr := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	if unix.Fstat(fd, &before) != nil || flagsErr != nil || descriptorErr != nil ||
		before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 0 ||
		before.Uid != uint32(os.Geteuid()) || before.Size != input.ExpectedSize ||
		before.Mode&0o777 != 0o400 || statusFlags&unix.O_ACCMODE != unix.O_RDONLY ||
		descriptorFlags&unix.FD_CLOEXEC == 0 {
		return ErrExecutableIdentity
	}
	if _, err := input.File.Seek(0, io.SeekStart); err != nil {
		return ErrExecutableIdentity
	}
	hasher := sha256.New()
	written, hashErr := io.Copy(
		hasher,
		io.LimitReader(&darwinContextReader{ctx: ctx, reader: input.File}, input.ExpectedSize+1),
	)
	if _, err := input.File.Seek(0, io.SeekStart); err != nil {
		return ErrExecutableIdentity
	}
	var after unix.Stat_t
	if hashErr != nil || written != input.ExpectedSize ||
		!bytes.Equal(hasher.Sum(nil), expected) || unix.Fstat(fd, &after) != nil ||
		componentDarwinFileIdentity(before) != componentDarwinFileIdentity(after) {
		return ErrExecutableIdentity
	}
	return nil
}

func validateDarwinReadWriteOutput(output ReadWriteOutput) error {
	if output.File == nil {
		return ErrRequestInvalid
	}
	fd := int(output.File.Fd())
	var before unix.Stat_t
	statusFlags, flagsErr := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	descriptorFlags, descriptorErr := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	if unix.Fstat(fd, &before) != nil || flagsErr != nil || descriptorErr != nil ||
		before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 0 ||
		before.Uid != uint32(os.Geteuid()) || before.Gid != uint32(os.Getegid()) ||
		before.Size != 0 || before.Mode&0o7777 != 0o600 || before.Flags != 0 ||
		statusFlags&unix.O_ACCMODE != unix.O_RDWR || descriptorFlags&unix.FD_CLOEXEC == 0 {
		return ErrExecutableIdentity
	}
	if offset, err := output.File.Seek(0, io.SeekStart); err != nil || offset != 0 {
		return ErrExecutableIdentity
	}
	var after unix.Stat_t
	if unix.Fstat(fd, &after) != nil || componentDarwinFileIdentity(before) != componentDarwinFileIdentity(after) {
		return ErrExecutableIdentity
	}
	return nil
}

func (session *darwinSession) PID() int {
	if session == nil {
		return 0
	}
	return session.pid
}

func (session *darwinSession) AuthenticateReadiness(ctx context.Context, limit int, validator ReadinessValidator) error {
	if session == nil || limit <= 0 || limit > MaxSessionFrameBytes || validator == nil {
		return ErrRequestInvalid
	}
	if err := session.acquire(ctx); err != nil {
		return err
	}
	defer session.release()
	if session.state != darwinSessionAwaitingReadiness || session.unhealthyLocked() || darwinContextFailure(ctx) != nil {
		return session.terminateWithCauseLocked(ErrProtocol)
	}
	if session.direct {
		return session.authenticateDirectReadinessLocked(ctx, limit, validator)
	}
	frame, err := session.readFrameLocked(ctx, limit)
	freezeErr := error(nil)
	if err == nil {
		freezeErr = session.tree.Freeze(ctx)
	}
	pendingStdout := freezeErr == nil && session.pendingStdoutLocked()
	valid := err == nil && freezeErr == nil && session.tree.RequireNoDescendants() == nil &&
		session.healthyFrozenLocked(ctx) && darwinContextFailure(ctx) == nil &&
		!pendingStdout && callReadinessValidator(validator, frame, session.pid)
	if !valid || !session.healthyFrozenLocked(ctx) || darwinContextFailure(ctx) != nil || pendingStdout {
		if err == nil {
			err = classifyDarwinSessionState(ctx)
		}
		return session.terminateWithCauseLocked(err)
	}
	if err := session.tree.ResumeRoot(ctx); err != nil || session.unhealthyLocked() || darwinContextFailure(ctx) != nil {
		return session.terminateWithCauseLocked(ErrProtocol)
	}
	session.state = darwinSessionReady
	return nil
}

func (session *darwinSession) RoundTrip(ctx context.Context, request []byte, limit int) ([]byte, error) {
	if session == nil || len(request) == 0 || len(request) > MaxSessionFrameBytes {
		return nil, ErrRequestInvalid
	}
	requestFrame := append([]byte(nil), request...)
	if requestFrame[len(requestFrame)-1] != '\n' || bytes.IndexByte(requestFrame[:len(requestFrame)-1], '\n') >= 0 ||
		bytes.IndexByte(requestFrame, '\r') >= 0 || bytes.IndexByte(requestFrame, 0) >= 0 ||
		limit <= 0 || limit > MaxSessionFrameBytes {
		return nil, ErrRequestInvalid
	}
	if err := session.acquire(ctx); err != nil {
		return nil, err
	}
	defer session.release()
	if session.state != darwinSessionReady || session.unhealthyLocked() || darwinContextFailure(ctx) != nil ||
		session.pendingStdoutLocked() {
		return nil, session.terminateWithCauseLocked(ErrProtocol)
	}
	if session.direct {
		return session.directRoundTripLocked(ctx, requestFrame, limit)
	}
	if err := session.writeFrameLocked(ctx, requestFrame); err != nil {
		return nil, session.terminateWithCauseLocked(err)
	}
	if session.unhealthyLocked() || darwinContextFailure(ctx) != nil {
		return nil, session.terminateWithCauseLocked(classifyDarwinSessionState(ctx))
	}
	frame, err := session.readFrameLocked(ctx, limit)
	freezeErr := error(nil)
	if err == nil {
		freezeErr = session.tree.Freeze(ctx)
	}
	pendingStdout := freezeErr == nil && session.pendingStdoutLocked()
	if err != nil || freezeErr != nil || session.tree.RequireNoDescendants() != nil ||
		!session.healthyFrozenLocked(ctx) || darwinContextFailure(ctx) != nil || pendingStdout {
		if err == nil {
			err = classifyDarwinSessionState(ctx)
		}
		return nil, session.terminateWithCauseLocked(err)
	}
	if err := session.tree.ResumeRoot(ctx); err != nil || session.unhealthyLocked() || darwinContextFailure(ctx) != nil {
		return nil, session.terminateWithCauseLocked(ErrProtocol)
	}
	return frame, nil
}

func (session *darwinSession) Close() error {
	if session == nil {
		return nil
	}
	gateDeadline := session.gateDeadline
	if gateDeadline <= 0 || gateDeadline > darwinSessionCleanupDeadline {
		gateDeadline = darwinSessionCleanupDeadline
	}
	timer := time.NewTimer(gateDeadline)
	defer timer.Stop()
	select {
	case <-session.gate:
		defer session.release()
		return session.terminateLocked()
	case <-timer.C:
		// A callback or blocked I/O owns the gate beyond the bounded close
		// window. Do not depend on that untrusted work returning: poison the
		// authority and use a fresh immutable tree view to kill/reap the retained
		// direct child and clean staging exactly once.
		session.forcedClose.Store(true)
		darwinAuthorityPoisoned.Store(true)
		if session.direct {
			_ = session.cleanup(nil)
		} else {
			forcedTree := newDarwinProcessTree(session.tree.root)
			_ = session.cleanup(forcedTree)
		}
		return ErrTermination
	}
}

func (session *darwinSession) acquire(ctx context.Context) error {
	if ctx == nil {
		return ErrRequestInvalid
	}
	select {
	case <-session.gate:
		if err := darwinContextFailure(ctx); err != nil || darwinAuthorityPoisoned.Load() {
			session.release()
			if err == nil {
				return ErrUnavailable
			}
			return err
		}
		return nil
	case <-ctx.Done():
		return darwinContextFailure(ctx)
	}
}

func (session *darwinSession) release() {
	select {
	case session.gate <- struct{}{}:
	default:
		panic("process authority session gate released twice")
	}
}

func (session *darwinSession) unhealthyLocked() bool {
	if session == nil || session.state == darwinSessionClosed || darwinAuthorityPoisoned.Load() ||
		session.transitions == nil || !session.transitions.Unchanged() || session.pendingStderrLocked() {
		return true
	}
	exited, err := pollDarwinProcessExit(session.pid)
	return err != nil || exited
}

func (session *darwinSession) authenticateDirectReadinessLocked(
	ctx context.Context,
	limit int,
	validator ReadinessValidator,
) error {
	if !session.healthyDirectLocked(ctx) {
		return session.terminateWithCauseLocked(ErrProtocol)
	}
	frame, err := session.readFrameLocked(ctx, limit)
	pendingStdout := err == nil && session.pendingStdoutLocked()
	valid := err == nil && !pendingStdout && session.healthyDirectLocked(ctx) &&
		darwinContextFailure(ctx) == nil && callReadinessValidator(validator, frame, session.pid)
	if !valid || !session.healthyDirectLocked(ctx) || darwinContextFailure(ctx) != nil || pendingStdout {
		if err == nil {
			err = classifyDarwinSessionState(ctx)
		}
		return session.terminateWithCauseLocked(err)
	}
	session.state = darwinSessionReady
	return nil
}

func (session *darwinSession) directRoundTripLocked(
	ctx context.Context,
	request []byte,
	limit int,
) ([]byte, error) {
	if !session.healthyDirectLocked(ctx) {
		return nil, session.terminateWithCauseLocked(ErrProtocol)
	}
	if err := session.writeFrameLocked(ctx, request); err != nil {
		return nil, session.terminateWithCauseLocked(err)
	}
	if !session.healthyDirectLocked(ctx) || darwinContextFailure(ctx) != nil {
		return nil, session.terminateWithCauseLocked(classifyDarwinSessionState(ctx))
	}
	frame, err := session.readFrameLocked(ctx, limit)
	pendingStdout := err == nil && session.pendingStdoutLocked()
	if err != nil || pendingStdout || !session.healthyDirectLocked(ctx) || darwinContextFailure(ctx) != nil {
		if err == nil {
			err = classifyDarwinSessionState(ctx)
		}
		return nil, session.terminateWithCauseLocked(err)
	}
	return frame, nil
}

func (session *darwinSession) healthyDirectLocked(ctx context.Context) bool {
	if session == nil || !session.direct || session.pid <= 0 || session.identity.PID != session.pid ||
		session.identity.PPID != os.Getpid() || session.identity.PGID != session.pid ||
		session.targetStaged == nil || session.targetStaged.file == nil || session.targetPath == "" ||
		session.workingDirectory == nil || session.workingDirectory.file == nil || session.workingAuthority == nil ||
		session.stagingRoot == "" || session.stagingAuthority == nil || darwinAuthorityPoisoned.Load() ||
		darwinContextFailure(ctx) != nil || session.transitions == nil || !session.transitions.Unchanged() ||
		session.pendingStderrLocked() || !pinnedDarwinObjectUnchanged(session.targetStaged) ||
		!directSessionWorkingDirectoryAuthorityValid(session.workingDirectory, session.workingAuthority) ||
		!darwinDirectoryPathMatchesAuthority(session.stagingRoot, session.stagingAuthority) ||
		!darwinPathMatchesPinnedObject(session.targetPath, session.targetStaged) {
		return false
	}
	state, err := readDarwinProcessState(ctx, session.pid, true)
	if err != nil || !state.sameIdentity(session.identity) || state.Identity.PPID != os.Getpid() ||
		state.Identity.PGID != session.pid || state.Status == darwinProcStatusStopped ||
		state.Status == darwinProcStatusZombie {
		return false
	}
	children, err := listDarwinDirectChildren(ctx, session.identity)
	if err != nil || len(children) != 0 {
		return false
	}
	loadedCDHash, err := loadedProcessCDHash(session.pid)
	if err != nil || !codeDirectoryHashMatches(loadedCDHash, session.targetCDHashes) ||
		!loadedDarwinExecutableMatchesOpenedFile(ctx, session.pid, session.targetStaged, session.targetHash) {
		return false
	}
	exited, err := pollDarwinProcessExit(session.pid)
	return err == nil && !exited && session.transitions.Unchanged()
}

func directSessionWorkingDirectoryAuthorityValid(opened *pinnedDarwinObject, authority *os.File) bool {
	if opened == nil || opened.file == nil || authority == nil {
		return false
	}
	var current unix.Stat_t
	if unix.Fstat(int(opened.file.Fd()), &current) != nil || current.Mode&unix.S_IFMT != unix.S_IFDIR ||
		current.Dev != opened.stat.Dev || current.Ino != opened.stat.Ino || current.Uid != opened.stat.Uid ||
		current.Mode != opened.stat.Mode || current.Uid != uint32(os.Geteuid()) || current.Mode&0o077 != 0 {
		return false
	}
	return darwinDirectoryAuthorityMatches(opened, authority) &&
		darwinDirectoryPathMatchesAuthority(opened.file.Name(), authority)
}

func (session *darwinSession) healthyFrozenLocked(ctx context.Context) bool {
	if session == nil || session.tree == nil || !session.tree.frozen || darwinAuthorityPoisoned.Load() ||
		darwinContextFailure(ctx) != nil || session.transitions == nil || !session.transitions.Unchanged() ||
		session.pendingStderrLocked() {
		return false
	}
	return darwinFrozenImageValid(
		ctx, session.tree, session.targetStaged, session.targetHash, session.targetCDHashes,
	)
}

func (session *darwinSession) pendingStdoutLocked() bool {
	return darwinReaderHasPending(session.stdout, session.stdoutReader)
}

func (session *darwinSession) pendingStderrLocked() bool {
	if session == nil {
		return true
	}
	return darwinReaderHasPending(session.stderr, session.stderrReader)
}

func (session *darwinSession) readFrameLocked(ctx context.Context, limit int) ([]byte, error) {
	stop, err := applyDarwinFileDeadline(ctx, session.stdout, true)
	if err != nil {
		return nil, err
	}
	defer stop()
	frame := make([]byte, 0, min(limit, 64*1024))
	for {
		if err := darwinContextFailure(ctx); err != nil || darwinAuthorityPoisoned.Load() {
			if err != nil {
				return nil, err
			}
			return nil, ErrUnavailable
		}
		fragment, readErr := session.stdoutReader.ReadSlice('\n')
		if len(fragment) > limit-len(frame) {
			return nil, ErrOutputLimit
		}
		frame = append(frame, fragment...)
		if readErr == nil {
			return append([]byte(nil), frame[:len(frame)-1]...), nil
		}
		if errors.Is(readErr, bufio.ErrBufferFull) {
			continue
		}
		if len(frame) != 0 && errors.Is(readErr, io.EOF) {
			return nil, ErrProtocol
		}
		return nil, classifyDarwinSessionIOError(ctx, readErr)
	}
}

func (session *darwinSession) writeFrameLocked(ctx context.Context, request []byte) error {
	stop, err := applyDarwinFileDeadline(ctx, session.stdin, false)
	if err != nil {
		return err
	}
	defer stop()
	remaining := request
	for len(remaining) > 0 {
		if err := darwinContextFailure(ctx); err != nil {
			return err
		}
		if darwinAuthorityPoisoned.Load() {
			return ErrUnavailable
		}
		written, writeErr := session.stdin.Write(remaining)
		if written > 0 {
			remaining = remaining[written:]
		}
		if writeErr != nil {
			return classifyDarwinSessionIOError(ctx, writeErr)
		}
		if written == 0 {
			return ErrProtocol
		}
	}
	return nil
}

func applyDarwinFileDeadline(ctx context.Context, file *os.File, read bool) (func(), error) {
	if ctx == nil || file == nil {
		return nil, ErrRequestInvalid
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, ErrRequestInvalid
	}
	if !deadline.After(time.Now()) {
		if contextErr := darwinContextFailure(ctx); contextErr != nil {
			return nil, contextErr
		}
		return nil, ErrTimeout
	}
	setDeadline := file.SetWriteDeadline
	if read {
		setDeadline = file.SetReadDeadline
	}
	if err := setDeadline(deadline); err != nil {
		return nil, ErrUnavailable
	}
	if contextErr := darwinContextFailure(ctx); contextErr != nil {
		_ = setDeadline(time.Time{})
		return nil, contextErr
	}
	if darwinAuthorityPoisoned.Load() {
		_ = setDeadline(time.Time{})
		return nil, ErrUnavailable
	}
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = setDeadline(time.Now())
		close(done)
	})
	return func() {
		if stop() {
			close(done)
		}
		<-done
		_ = setDeadline(time.Time{})
	}, nil
}

func classifyDarwinSessionIOError(ctx context.Context, err error) error {
	if contextErr := darwinContextFailure(ctx); contextErr != nil {
		return contextErr
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return ErrTimeout
	}
	if errors.Is(err, io.EOF) {
		return ErrAbnormalExit
	}
	return ErrUnavailable
}

func (session *darwinSession) terminateLocked() error {
	if session.state == darwinSessionClosed {
		return session.terminationErr
	}
	session.state = darwinSessionClosed
	if session.cleanup(session.tree) != nil || session.forcedClose.Load() {
		darwinAuthorityPoisoned.Store(true)
		session.terminationErr = ErrTermination
	}
	return session.terminationErr
}

func (session *darwinSession) cleanup(tree *darwinProcessTree) error {
	cleanupErr, _ := session.cleanupWithChannelProof(tree, false)
	return cleanupErr
}

func (session *darwinSession) cleanupWithChannelProof(tree *darwinProcessTree, requireEmptyEOF bool) (error, error) {
	if session == nil {
		return ErrTermination, ErrProtocol
	}
	session.cleanupOnce.Do(func() {
		cleanupContext, cancelCleanup := context.WithTimeout(context.Background(), darwinSessionCleanupDeadline)
		defer cancelCleanup()
		processErr := errDarwinProcessTree
		if session.direct {
			processErr = terminateDirectDarwinProcess(cleanupContext, session.process, session.pid, session.identity)
		} else if tree != nil {
			processErr = tree.Terminate(cleanupContext, session.process)
		}
		if !session.direct && processErr != nil && killAndWaitDarwinProcess(session.process, session.pid) {
			// The exact direct child is reaped, but containment proof failed.
			processErr = errDarwinProcessTree
		}
		transitionCloseErr := error(nil)
		if session.transitions != nil {
			transitionCloseErr = session.transitions.Close()
		}
		stdinErr := session.stdin.Close()
		ownerLivenessErr := error(nil)
		if session.ownerLiveness != nil {
			ownerLivenessErr = session.ownerLiveness.Close()
		}
		stdoutProofErr := error(nil)
		stderrProofErr := error(nil)
		if requireEmptyEOF {
			stdoutProofErr = darwinSessionChannelAtEmptyEOF(cleanupContext, session.stdout, session.stdoutReader)
			stderrProofErr = darwinSessionChannelAtEmptyEOF(cleanupContext, session.stderr, session.stderrReader)
			if stdoutProofErr != nil || stderrProofErr != nil {
				session.channelProofErr = ErrProtocol
			}
		}
		stdoutErr := session.stdout.Close()
		stderrErr := session.stderr.Close()
		targetCloseErr := error(nil)
		if session.targetStaged != nil && session.targetStaged.file != nil {
			targetCloseErr = session.targetStaged.file.Close()
		}
		bootstrapCloseErr := error(nil)
		if session.bootstrapStage != nil {
			bootstrapCloseErr = session.bootstrapStage.Close()
		}
		workingCloseErr := error(nil)
		if session.workingDirectory != nil && session.workingDirectory.file != nil {
			workingCloseErr = session.workingDirectory.file.Close()
		}
		targetCleanupErr := error(nil)
		if session.cleanupTarget != nil {
			targetCleanupErr = session.cleanupTarget()
		}
		bootstrapCleanupErr := error(nil)
		if session.cleanupBootstrap != nil {
			bootstrapCleanupErr = session.cleanupBootstrap()
		}
		workingAuthorityCloseErr := error(nil)
		if session.workingAuthority != nil {
			workingAuthorityCloseErr = session.workingAuthority.Close()
		}
		stagingAuthorityCloseErr := error(nil)
		if session.stagingAuthority != nil {
			stagingAuthorityCloseErr = session.stagingAuthority.Close()
		}
		if processErr != nil || transitionCloseErr != nil || stdinErr != nil || ownerLivenessErr != nil ||
			stdoutErr != nil || stderrErr != nil ||
			targetCloseErr != nil || bootstrapCloseErr != nil || workingCloseErr != nil ||
			targetCleanupErr != nil || bootstrapCleanupErr != nil ||
			workingAuthorityCloseErr != nil || stagingAuthorityCloseErr != nil {
			session.cleanupErr = ErrTermination
		}
	})
	return session.cleanupErr, session.channelProofErr
}

func darwinSessionChannelAtEmptyEOF(ctx context.Context, file *os.File, reader *bufio.Reader) error {
	if ctx == nil || file == nil || reader == nil {
		return ErrProtocol
	}
	stop, err := applyDarwinFileDeadline(ctx, file, true)
	if err != nil {
		return err
	}
	defer stop()
	_, err = reader.ReadByte()
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return classifyDarwinSessionIOError(ctx, err)
	}
	return ErrProtocol
}

func (session *darwinSession) terminateWithCauseLocked(cause error) error {
	if terminateErr := session.terminateLocked(); terminateErr != nil {
		return terminateErr
	}
	return cause
}

func callReadinessValidator(validator ReadinessValidator, frame []byte, pid int) (valid bool) {
	defer func() {
		if recover() != nil {
			valid = false
		}
	}()
	return validator(append([]byte(nil), frame...), pid)
}

func classifyDarwinSessionState(ctx context.Context) error {
	if err := darwinContextFailure(ctx); err != nil {
		return err
	}
	if darwinAuthorityPoisoned.Load() {
		return ErrUnavailable
	}
	return ErrProtocol
}
