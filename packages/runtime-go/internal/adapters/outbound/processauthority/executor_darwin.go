//go:build darwin

package processauthority

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const (
	maxExecutableBytes          = int64(2 * 1024 * 1024 * 1024)
	darwinSystemToolOutputLimit = 1024 * 1024
)

var allowedDarwinEnvironment = map[string]struct{}{
	"ANALYTIX_NATIVE_LAUNCH_NONCE":     {},
	"ANALYTIX_NATIVE_PROTOCOL_VERSION": {},
	"LANG":                             {},
	"LC_ALL":                           {},
	"TZ":                               {},
}

var darwinAuthorityPoisoned atomic.Bool

type pinnedDarwinObject struct {
	file *os.File
	stat unix.Stat_t
}

type boundedReadResult struct {
	payload []byte
	limit   bool
	err     error
}

type darwinOutputBudget struct {
	remaining atomic.Int64
}

type darwinPreparedSystemTool struct {
	executable                *pinnedDarwinObject
	executableHash            [sha256.Size]byte
	arguments                 []string
	environment               []string
	workingDirectory          *pinnedDarwinObject
	workingDirectoryAuthority *os.File
	launchOpenedPath          string
}

func executeSystemTool(ctx context.Context, request SystemToolRequest) (result Result, runErr error) {
	if ctx == nil {
		return Result{}, ErrRequestInvalid
	}
	if darwinAuthorityPoisoned.Load() {
		return Result{}, ErrUnavailable
	}
	defer func() {
		poisonDarwinAuthorityOnTermination(runErr)
	}()
	validated, err := validateDarwinSystemToolRequest(request)
	if err != nil {
		return Result{}, err
	}
	deadline, cancel := context.WithTimeout(ctx, validated.Timeout)
	defer cancel()
	if err := darwinContextFailure(deadline); err != nil {
		return Result{}, err
	}
	executablePath, ok := trustedDarwinSystemToolPath(validated.Tool)
	if !ok {
		return Result{}, ErrRequestInvalid
	}
	executable, err := openPinnedDarwinObject(executablePath, false)
	if err != nil {
		return Result{}, ErrExecutableIdentity
	}
	defer executable.file.Close()
	if !trustedDarwinSystemExecutable(executable) {
		return Result{}, ErrExecutableIdentity
	}
	executableHash, err := sha256OpenedFile(deadline, executable.file)
	if contextErr := darwinContextFailure(deadline); contextErr != nil {
		return Result{}, contextErr
	}
	if err != nil || !pinnedDarwinObjectUnchanged(executable) {
		return Result{}, ErrExecutableIdentity
	}
	workingDirectory, err := openPinnedDarwinObject(validated.WorkingDirectory, true)
	if err != nil {
		return Result{}, ErrWorkingDirectory
	}
	defer workingDirectory.file.Close()
	if workingDirectory.stat.Mode&unix.S_IFMT != unix.S_IFDIR || workingDirectory.stat.Mode&0o022 != 0 ||
		!darwinDirectoryAuthorityMatches(workingDirectory, validated.WorkingDirectoryAuthority) {
		return Result{}, ErrWorkingDirectory
	}
	return executePreparedDarwinSystemTool(deadline, darwinPreparedSystemTool{
		executable: executable, executableHash: executableHash,
		arguments:        validated.Arguments,
		environment:      []string{"LANG=C", "LC_ALL=C", "TZ=UTC"},
		workingDirectory: workingDirectory, workingDirectoryAuthority: validated.WorkingDirectoryAuthority,
		launchOpenedPath: executablePath,
	})
}

func executePreparedDarwinSystemTool(
	ctx context.Context,
	prepared darwinPreparedSystemTool,
) (result Result, runErr error) {
	if prepared.executable == nil || prepared.executable.file == nil ||
		prepared.workingDirectory == nil || prepared.workingDirectory.file == nil ||
		prepared.workingDirectoryAuthority == nil || prepared.launchOpenedPath == "" {
		return Result{}, ErrRequestInvalid
	}
	if contextErr := darwinContextFailure(ctx); contextErr != nil {
		return Result{}, contextErr
	}
	stagedPath := prepared.launchOpenedPath
	stagedFile := prepared.executable.file
	openedCDHashes, err := openedFileCodeDirectoryHashes(stagedFile)
	if err != nil {
		return Result{}, ErrExecutableIdentity
	}
	var stagedStat unix.Stat_t
	if unix.Fstat(int(stagedFile.Fd()), &stagedStat) != nil {
		return Result{}, ErrExecutableIdentity
	}
	stagedObject := &pinnedDarwinObject{file: stagedFile, stat: stagedStat}

	stdinReader, stdinWriter, stdoutReader, stdoutWriter, stderrReader, stderrWriter, err := darwinProcessPipes()
	if err != nil {
		return Result{}, ErrUnavailable
	}
	defer stdinReader.Close()
	defer stdinWriter.Close()
	defer stdoutReader.Close()
	defer stdoutWriter.Close()
	defer stderrReader.Close()
	defer stderrWriter.Close()

	if contextErr := darwinContextFailure(ctx); contextErr != nil {
		return Result{}, contextErr
	}
	if !preparedDarwinExecutionIdentityValid(prepared) {
		return Result{}, ErrExecutableIdentity
	}
	spawnArguments := append([]string{stagedPath}, prepared.arguments...)
	pid, err := spawnSuspended(
		stagedPath,
		spawnArguments,
		prepared.environment,
		int(prepared.workingDirectory.file.Fd()),
		int(stdinReader.Fd()),
		int(stdoutWriter.Fd()),
		int(stderrWriter.Fd()),
	)
	if err != nil {
		return Result{}, ErrUnavailable
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		killAndWaitDarwinProcess(nil, pid)
		return Result{}, ErrTermination
	}
	resumed := false
	defer func() {
		finalizeSuspendedDarwinTermination(resumed, process, pid, &result, &runErr)
	}()
	loadedCDHash, err := loadedProcessCDHash(pid)
	if err != nil || !codeDirectoryHashMatches(loadedCDHash, openedCDHashes) ||
		!preparedDarwinExecutionIdentityValid(prepared) ||
		!loadedDarwinExecutableMatchesOpenedFile(ctx, pid, stagedObject, prepared.executableHash) {
		return Result{}, ErrExecutableIdentity
	}
	if contextErr := darwinContextFailure(ctx); contextErr != nil {
		return Result{}, contextErr
	}
	if err := syscall.Kill(pid, syscall.SIGCONT); err != nil {
		return Result{}, ErrUnavailable
	}
	resumed = true
	_ = stdinReader.Close()
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()

	return superviseDarwinSystemTool(
		ctx,
		process,
		pid,
		stdinWriter,
		stdoutReader,
		stderrReader,
	)
}

func preparedDarwinExecutionIdentityValid(prepared darwinPreparedSystemTool) bool {
	if !pinnedDarwinObjectUnchanged(prepared.executable) ||
		!pinnedDarwinObjectUnchanged(prepared.workingDirectory) ||
		!darwinDirectoryAuthorityMatches(prepared.workingDirectory, prepared.workingDirectoryAuthority) {
		return false
	}
	return darwinPathMatchesPinnedObject(prepared.launchOpenedPath, prepared.executable)
}

func darwinPathMatchesPinnedObject(path string, expected *pinnedDarwinObject) bool {
	if expected == nil || expected.file == nil {
		return false
	}
	opened, err := openPinnedDarwinObject(path, false)
	if err != nil {
		return false
	}
	defer opened.file.Close()
	return opened.stat.Dev == expected.stat.Dev && opened.stat.Ino == expected.stat.Ino &&
		opened.stat.Size == expected.stat.Size && opened.stat.Mode == expected.stat.Mode &&
		opened.stat.Uid == expected.stat.Uid && opened.stat.Gid == expected.stat.Gid &&
		opened.stat.Mtim == expected.stat.Mtim && opened.stat.Ctim == expected.stat.Ctim &&
		pinnedDarwinObjectUnchanged(opened) && pinnedDarwinObjectUnchanged(expected)
}

func trustedDarwinSystemToolPath(tool SystemTool) (string, bool) {
	switch tool {
	case SystemToolDarwinCodeSign:
		return "/usr/bin/codesign", true
	default:
		return "", false
	}
}

func trustedDarwinSystemExecutable(executable *pinnedDarwinObject) bool {
	if executable == nil || executable.file == nil ||
		executable.stat.Mode&unix.S_IFMT != unix.S_IFREG || executable.stat.Nlink != 1 ||
		executable.stat.Size <= 0 || executable.stat.Size > maxExecutableBytes ||
		executable.stat.Uid != 0 || executable.stat.Mode&0o022 != 0 {
		return false
	}
	var filesystem unix.Statfs_t
	return unix.Fstatfs(int(executable.file.Fd()), &filesystem) == nil &&
		filesystem.Flags&unix.MNT_RDONLY != 0 && pinnedDarwinObjectUnchanged(executable)
}

func poisonDarwinAuthorityOnTermination(runErr error) {
	if errors.Is(runErr, ErrTermination) {
		darwinAuthorityPoisoned.Store(true)
	}
}

func finalizeSuspendedDarwinTermination(resumed bool, process *os.Process, pid int, result *Result, runErr *error) {
	if resumed || killAndWaitDarwinProcess(process, pid) {
		return
	}
	if result != nil {
		*result = Result{}
	}
	if runErr != nil {
		*runErr = ErrTermination
	}
}

func superviseDarwinSystemTool(
	ctx context.Context,
	process *os.Process,
	pid int,
	stdinWriter *os.File,
	stdoutReader *os.File,
	stderrReader *os.File,
) (Result, error) {
	defer stdinWriter.Close()
	defer stdoutReader.Close()
	defer stderrReader.Close()

	budget := &darwinOutputBudget{}
	budget.remaining.Store(darwinSystemToolOutputLimit)
	stdoutDone := make(chan boundedReadResult, 1)
	stderrDone := make(chan boundedReadResult, 1)
	inputDone := make(chan error, 1)
	exitDone := make(chan error, 1)
	go func() { stdoutDone <- readBoundedDarwin(stdoutReader, budget, true) }()
	go func() { stderrDone <- readBoundedDarwin(stderrReader, budget, true) }()
	go func() { inputDone <- writeDarwinInput(ctx, stdinWriter, nil) }()
	go func() { exitDone <- observeDarwinProcessExit(pid) }()

	var stdoutResult boundedReadResult
	var stderrResult boundedReadResult
	stdoutReceived := false
	stderrReceived := false
	inputReceived := false
	waitReceived := false
	var processState *os.ProcessState
	var terminalErr error
	contextDone := ctx.Done()
	var terminationTimer *time.Timer
	var terminationDeadline <-chan time.Time
	startTermination := func(err error) {
		if terminalErr != nil {
			return
		}
		terminalErr = err
		if !waitReceived {
			killDarwinProcessGroup(pid)
		}
		_ = stdinWriter.Close()
		_ = stdoutReader.Close()
		_ = stderrReader.Close()
		terminationTimer = time.NewTimer(5 * time.Second)
		terminationDeadline = terminationTimer.C
	}
	defer func() {
		if terminationTimer != nil {
			terminationTimer.Stop()
		}
	}()

	for !stdoutReceived || !stderrReceived || !inputReceived || !waitReceived {
		select {
		case stdoutResult = <-stdoutDone:
			stdoutReceived = true
			if stdoutResult.limit {
				startTermination(ErrOutputLimit)
			} else if stdoutResult.err != nil {
				startTermination(ErrUnavailable)
			}
		case stderrResult = <-stderrDone:
			stderrReceived = true
			if stderrResult.limit {
				startTermination(ErrOutputLimit)
			} else if stderrResult.err != nil {
				startTermination(ErrUnavailable)
			}
		case inputErr := <-inputDone:
			inputReceived = true
			if inputErr != nil {
				if contextErr := darwinContextFailure(ctx); contextErr != nil {
					startTermination(contextErr)
				} else {
					startTermination(ErrUnavailable)
				}
			}
		case observationErr := <-exitDone:
			if observationErr != nil {
				startTermination(ErrTermination)
				waitReceived = killAndWaitDarwinProcess(process, pid)
				if !waitReceived {
					return Result{}, ErrTermination
				}
				continue
			}
			// waitid left the leader waitable, so pid/pgid cannot be reused
			// until descendants are killed and Process.Wait reaps the leader.
			killDarwinProcessGroup(pid)
			state, waitErr := process.Wait()
			waitReceived = true
			processState = state
			if waitErr != nil || state == nil {
				startTermination(ErrTermination)
			}
		case <-contextDone:
			contextDone = nil
			startTermination(darwinContextFailure(ctx))
		case <-terminationDeadline:
			return Result{}, ErrTermination
		}
	}
	if terminalErr != nil {
		return Result{}, terminalErr
	}
	if processState == nil {
		return Result{}, ErrTermination
	}
	result := Result{ExitCode: processState.ExitCode(), TerminationStatus: TerminationExited}
	if waitStatus, ok := processState.Sys().(syscall.WaitStatus); ok && waitStatus.Signaled() {
		result.TerminationStatus = TerminationSignaled
		result.Signal = int(waitStatus.Signal())
		return result, ErrAbnormalExit
	}
	result.Stdout = stdoutResult.payload
	result.Stderr = stderrResult.payload
	return result, nil
}

func validateDarwinSystemToolRequest(request SystemToolRequest) (SystemToolRequest, error) {
	if _, ok := trustedDarwinSystemToolPath(request.Tool); !ok ||
		len(request.Arguments) == 0 || len(request.Arguments) > MaxArguments-1 ||
		request.WorkingDirectoryAuthority == nil ||
		request.Timeout <= 0 || request.Timeout > 24*time.Hour ||
		!filepath.IsAbs(request.WorkingDirectory) {
		return SystemToolRequest{}, ErrRequestInvalid
	}
	argumentBytes := 0
	for _, value := range request.Arguments {
		if value == "" || strings.ContainsAny(value, "\x00\r\n") {
			return SystemToolRequest{}, ErrRequestInvalid
		}
		argumentBytes += len(value) + 1
	}
	if argumentBytes > MaxArgumentBytes || !validDarwinCodeSignArguments(request.Arguments) {
		return SystemToolRequest{}, ErrRequestInvalid
	}
	request.Arguments = append([]string(nil), request.Arguments...)
	request.WorkingDirectory = filepath.Clean(request.WorkingDirectory)
	if request.WorkingDirectory == string(filepath.Separator) {
		return SystemToolRequest{}, ErrRequestInvalid
	}
	return request, nil
}

func validDarwinCodeSignArguments(arguments []string) bool {
	if len(arguments) == 4 && arguments[0] == "--verify" && arguments[1] == "--strict" &&
		arguments[2] == "--verbose=2" {
		return validDarwinPIDTarget(arguments[3], false)
	}
	if len(arguments) == 5 && arguments[0] == "--verify" && arguments[1] == "--strict" &&
		arguments[2] == "--verbose=2" && validDarwinDeveloperIDRequirement(arguments[3]) {
		return validDarwinPIDTarget(arguments[4], false)
	}
	if len(arguments) == 3 && arguments[0] == "--display" && arguments[1] == "--verbose=4" {
		return validDarwinPIDTarget(arguments[2], true)
	}
	if len(arguments) == 4 && arguments[0] == "--display" && arguments[1] == "--entitlements" &&
		arguments[2] == ":-" {
		return validDarwinPIDTarget(arguments[3], true)
	}
	return false
}

func validDarwinDeveloperIDRequirement(requirement string) bool {
	const prefix = `-R=anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and certificate leaf[subject.OU] = "`
	const suffix = `"`
	if !strings.HasPrefix(requirement, prefix) || !strings.HasSuffix(requirement, suffix) {
		return false
	}
	teamIdentifier := strings.TrimSuffix(strings.TrimPrefix(requirement, prefix), suffix)
	if len(teamIdentifier) != 10 {
		return false
	}
	for _, value := range teamIdentifier {
		if (value < 'A' || value > 'Z') && (value < '0' || value > '9') {
			return false
		}
	}
	return true
}

func validDarwinPIDTarget(target string, dynamic bool) bool {
	if dynamic {
		if !strings.HasPrefix(target, "+") {
			return false
		}
		target = strings.TrimPrefix(target, "+")
	} else if strings.HasPrefix(target, "+") {
		return false
	}
	pid, err := strconv.Atoi(target)
	return err == nil && pid > 0 && strconv.Itoa(pid) == target
}

func openPinnedDarwinObject(path string, directory bool) (*pinnedDarwinObject, error) {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) || clean == string(filepath.Separator) || strings.ContainsRune(clean, 0) {
		return nil, ErrRequestInvalid
	}
	components := strings.Split(strings.TrimPrefix(clean, string(filepath.Separator)), string(filepath.Separator))
	current, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	for index, component := range components {
		if component == "" || component == "." || component == ".." {
			_ = unix.Close(current)
			return nil, ErrRequestInvalid
		}
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
		if index < len(components)-1 || directory {
			flags |= unix.O_DIRECTORY
		}
		next, openErr := unix.Openat(current, component, flags, 0)
		_ = unix.Close(current)
		if openErr != nil {
			return nil, openErr
		}
		current = next
	}
	var stat unix.Stat_t
	if err := unix.Fstat(current, &stat); err != nil {
		_ = unix.Close(current)
		return nil, err
	}
	return &pinnedDarwinObject{file: os.NewFile(uintptr(current), clean), stat: stat}, nil
}

func pinnedDarwinObjectUnchanged(object *pinnedDarwinObject) bool {
	if object == nil || object.file == nil {
		return false
	}
	var current unix.Stat_t
	return unix.Fstat(int(object.file.Fd()), &current) == nil &&
		current.Dev == object.stat.Dev && current.Ino == object.stat.Ino && current.Size == object.stat.Size &&
		current.Mode == object.stat.Mode && current.Mtim == object.stat.Mtim && current.Ctim == object.stat.Ctim
}

func sha256OpenedFile(ctx context.Context, file *os.File) ([sha256.Size]byte, error) {
	var result [sha256.Size]byte
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return result, err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, io.LimitReader(&darwinContextReader{ctx: ctx, reader: file}, maxExecutableBytes+1)); err != nil {
		return result, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return result, err
	}
	copy(result[:], hash.Sum(nil))
	return result, nil
}

func stageDarwinExecutableWithAuthority(
	ctx context.Context,
	root string,
	authority *os.File,
	source *os.File,
	size int64,
	expectedHash [sha256.Size]byte,
) (string, *os.File, func() error, error) {
	return stageDarwinExecutableWithAuthorityPolicy(ctx, root, authority, source, size, expectedHash, false)
}

func stageDarwinBuildProbeExecutableWithAuthority(
	ctx context.Context,
	root string,
	authority *os.File,
	source *os.File,
	size int64,
	expectedHash [sha256.Size]byte,
) (string, *os.File, func() error, error) {
	return stageDarwinExecutableWithAuthorityPolicy(ctx, root, authority, source, size, expectedHash, true)
}

func stageDarwinExecutableWithAuthorityPolicy(
	ctx context.Context,
	root string,
	authority *os.File,
	source *os.File,
	size int64,
	expectedHash [sha256.Size]byte,
	strictBuildProbe bool,
) (string, *os.File, func() error, error) {
	rootObject, err := openPinnedDarwinObject(root, true)
	if err != nil {
		return "", nil, func() error { return nil }, err
	}
	var buildProbeProvenance []byte
	if strictBuildProbe {
		var provenanceOK bool
		buildProbeProvenance, provenanceOK = darwinBuildProbeProvenance(int(rootObject.file.Fd()))
		if !provenanceOK {
			_ = rootObject.file.Close()
			return "", nil, func() error { return nil }, ErrExecutableIdentity
		}
	}
	if rootObject.stat.Uid != uint32(os.Geteuid()) || rootObject.stat.Mode&unix.S_IFMT != unix.S_IFDIR ||
		rootObject.stat.Mode&0o077 != 0 || (authority != nil && !darwinDirectoryAuthorityMatches(rootObject, authority)) ||
		(strictBuildProbe && !darwinBuildProbeCreatedObjectSafe(
			rootObject.file, unix.S_IFDIR, 0o700, uint32(os.Geteuid()), buildProbeProvenance,
		)) {
		_ = rootObject.file.Close()
		return "", nil, func() error { return nil }, ErrExecutableIdentity
	}
	stageName, err := randomDarwinStageName()
	if err != nil {
		_ = rootObject.file.Close()
		return "", nil, func() error { return nil }, ErrUnavailable
	}
	directoryName := ".launch-" + stageName
	if err := unix.Mkdirat(int(rootObject.file.Fd()), directoryName, 0o700); err != nil {
		_ = rootObject.file.Close()
		return "", nil, func() error { return nil }, err
	}
	directoryFD, err := unix.Openat(int(rootObject.file.Fd()), directoryName, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		_ = unix.Unlinkat(int(rootObject.file.Fd()), directoryName, unix.AT_REMOVEDIR)
		_ = rootObject.file.Close()
		return "", nil, func() error { return nil }, err
	}
	if strictBuildProbe && !darwinBuildProbeCreatedFDObjectSafe(
		directoryFD, unix.S_IFDIR, 0o700, uint32(os.Geteuid()), buildProbeProvenance,
	) {
		_ = unix.Close(directoryFD)
		_ = unix.Unlinkat(int(rootObject.file.Fd()), directoryName, unix.AT_REMOVEDIR)
		_ = rootObject.file.Close()
		return "", nil, func() error { return nil }, ErrExecutableIdentity
	}
	stagePath := filepath.Join(root, directoryName)
	payloadPath := filepath.Join(stagePath, "payload")
	cleaned := false
	cleanup := func() error {
		if cleaned {
			return nil
		}
		cleaned = true
		unlinkErr := unix.Unlinkat(directoryFD, "payload", 0)
		syncErr := unix.Fsync(directoryFD)
		closeErr := unix.Close(directoryFD)
		removeErr := unix.Unlinkat(int(rootObject.file.Fd()), directoryName, unix.AT_REMOVEDIR)
		rootSyncErr := unix.Fsync(int(rootObject.file.Fd()))
		rootCloseErr := rootObject.file.Close()
		return errors.Join(ignoreDarwinNotExist(unlinkErr), syncErr, closeErr, ignoreDarwinNotExist(removeErr), rootSyncErr, rootCloseErr)
	}
	// Incomplete bytes are never executable. The file begins as exact 0600 and
	// transitions to exact 0500 only after size/hash validation and durability.
	destinationFD, err := unix.Openat(directoryFD, "payload", unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		_ = cleanup()
		return "", nil, func() error { return nil }, err
	}
	destination := os.NewFile(uintptr(destinationFD), payloadPath)
	if strictBuildProbe && !darwinBuildProbeCreatedObjectSafe(
		destination, unix.S_IFREG, 0o600, uint32(os.Geteuid()), buildProbeProvenance,
	) {
		_ = destination.Close()
		_ = cleanup()
		return "", nil, func() error { return nil }, ErrExecutableIdentity
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		_ = destination.Close()
		_ = cleanup()
		return "", nil, func() error { return nil }, err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(
		io.MultiWriter(destination, hash),
		io.LimitReader(&darwinContextReader{ctx: ctx, reader: source}, size+1),
	)
	if copyErr != nil || written != size || !equalDarwinDigest(hash.Sum(nil), expectedHash[:]) || destination.Sync() != nil ||
		unix.Fchmod(destinationFD, 0o500) != nil || destination.Sync() != nil ||
		(strictBuildProbe && !darwinBuildProbeCreatedObjectSafe(
			destination, unix.S_IFREG, 0o500, uint32(os.Geteuid()), buildProbeProvenance,
		)) ||
		unix.Fsync(directoryFD) != nil {
		_ = destination.Close()
		_ = cleanup()
		return "", nil, func() error { return nil }, ErrExecutableIdentity
	}
	if err := destination.Close(); err != nil {
		_ = cleanup()
		return "", nil, func() error { return nil }, err
	}
	readOnlyFD, err := unix.Openat(directoryFD, "payload", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		_ = cleanup()
		return "", nil, func() error { return nil }, err
	}
	readOnly := os.NewFile(uintptr(readOnlyFD), payloadPath)
	var stagedStat unix.Stat_t
	if unix.Fstat(readOnlyFD, &stagedStat) != nil || stagedStat.Mode&unix.S_IFMT != unix.S_IFREG ||
		stagedStat.Nlink != 1 || stagedStat.Size != size || stagedStat.Mode&0o7777 != 0o500 ||
		(strictBuildProbe && (!darwinBuildProbeCreatedFDObjectSafe(
			readOnlyFD, unix.S_IFREG, 0o500, uint32(os.Geteuid()), buildProbeProvenance,
		) || !darwinBuildProbeCreatedFDObjectSafe(
			directoryFD, unix.S_IFDIR, 0o700, uint32(os.Geteuid()), buildProbeProvenance,
		) || !darwinBuildProbeCreatedObjectSafe(
			rootObject.file, unix.S_IFDIR, 0o700, uint32(os.Geteuid()), buildProbeProvenance,
		))) {
		_ = readOnly.Close()
		_ = cleanup()
		return "", nil, func() error { return nil }, ErrExecutableIdentity
	}
	return payloadPath, readOnly, cleanup, nil
}

func randomDarwinStageName() (string, error) {
	var payload [16]byte
	if _, err := rand.Read(payload[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(payload[:]), nil
}

func equalDarwinDigest(left []byte, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	difference := byte(0)
	for index := range left {
		difference |= left[index] ^ right[index]
	}
	return difference == 0
}

func ignoreDarwinNotExist(err error) error {
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	return err
}

func darwinProcessPipes() (*os.File, *os.File, *os.File, *os.File, *os.File, *os.File, error) {
	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		stdinReader.Close()
		stdinWriter.Close()
		return nil, nil, nil, nil, nil, nil, err
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		stdinReader.Close()
		stdinWriter.Close()
		stdoutReader.Close()
		stdoutWriter.Close()
		return nil, nil, nil, nil, nil, nil, err
	}
	return stdinReader, stdinWriter, stdoutReader, stdoutWriter, stderrReader, stderrWriter, nil
}

type darwinContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *darwinContextReader) Read(payload []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(payload)
}

func darwinContextFailure(ctx context.Context) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return ErrCanceled
	}
	return ErrTimeout
}

func (budget *darwinOutputBudget) take(count int) bool {
	if budget == nil || count < 0 {
		return false
	}
	for {
		remaining := budget.remaining.Load()
		if int64(count) > remaining {
			return false
		}
		if budget.remaining.CompareAndSwap(remaining, remaining-int64(count)) {
			return true
		}
	}
}

func readBoundedDarwin(reader io.Reader, budget *darwinOutputBudget, capture bool) boundedReadResult {
	buffer := make([]byte, 32*1024)
	var payload []byte
	for {
		read, err := reader.Read(buffer)
		if read > 0 {
			if !budget.take(read) {
				return boundedReadResult{limit: true}
			}
			if capture {
				payload = append(payload, buffer[:read]...)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return boundedReadResult{payload: payload}
			}
			return boundedReadResult{err: err}
		}
		if read == 0 {
			return boundedReadResult{err: io.ErrNoProgress}
		}
	}
}

func writeDarwinInput(ctx context.Context, writer *os.File, payload []byte) error {
	remaining := payload
	for len(remaining) > 0 {
		if err := ctx.Err(); err != nil {
			return errors.Join(err, writer.Close())
		}
		written, err := writer.Write(remaining)
		if written > 0 {
			remaining = remaining[written:]
		}
		if err != nil {
			return errors.Join(err, writer.Close())
		}
		if written == 0 {
			return errors.Join(io.ErrNoProgress, writer.Close())
		}
	}
	return writer.Close()
}

func killDarwinProcessGroup(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

func killAndWaitDarwinProcess(process *os.Process, pid int) bool {
	killDarwinProcessGroup(pid)
	if process == nil {
		return false
	}
	if waitDarwinProcessExitUntil(pid, time.Now().Add(5*time.Second)) != nil {
		return false
	}
	// Reserve the leader pid until every member of its original process group
	// has received the final kill, then reap synchronously with no worker left.
	killDarwinProcessGroup(pid)
	state, waitErr := process.Wait()
	return waitErr == nil && state != nil
}
