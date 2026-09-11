//go:build darwin

package nativecomponentrunner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"reflect"
	"sync/atomic"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	"golang.org/x/sys/unix"
)

const (
	fundsCanonicalCSVOperationDirectoryPrefixV1 = ".funds-canonical-csv-"
	// The macOS DuckDB path normalizer preserves the former vnode basename
	// while opening /dev/fd/5. Create the exclusive inode under that exact
	// basename, then unlink it before any source row can reach the child.
	fundsCanonicalCSVOutputBasenameV1     = "5"
	fundsCanonicalCSVHashBufferBytesV1    = 256 * 1024
	fundsCanonicalCSVMaximumOutputBytesV1 = int64(domainnative.TransactionSourceRowMaximumSnapshotBytesV1)
)

type fundsCanonicalCSVExactReaderLeaseV1 struct {
	source         io.Reader
	expectedSHA256 string
	expectedSize   int64
	used           atomic.Bool
}

type fundsCanonicalCSVContextReaderV1 struct {
	ctx    context.Context
	source io.Reader
}

type fundsCanonicalCSVOutputV1 struct {
	rootPath              string
	rootAuthority         *os.File
	rootIdentity          privateSnapshotDirectoryIdentity
	rootRetiredLinks      uint64
	operation             *os.File
	operationIdentity     privateSnapshotDirectoryIdentity
	operationRetiredLinks uint64
	operationName         string
	writeFile             *os.File
	readFile              *os.File
	initialIdentity       privateSnapshotIdentity
	outputLinked          bool
	operationLinked       bool
}

type sealedFundsCanonicalCSVOutputV1 struct {
	file     *os.File
	identity privateSnapshotIdentity
	object   domainfundsquerysource.ImmutableSnapshotObjectV1
}

func (runner *Runner) BuildFundsCanonicalCSVSnapshot(
	ctx context.Context,
	arguments domainnative.FundsCanonicalCSVSnapshotBuildArgumentsV1,
	source io.Reader,
	installer fundsquerysourceport.ImmutableSnapshotInstaller,
) (
	result domainnative.FundsCanonicalCSVSnapshotBuildResultV1,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
	disposition fundsquerysourceport.ImmutableSnapshotInstallDispositionV1,
	resultErr error,
) {
	stage := "ADMISSION"
	// Registered before cleanup so a cleanup failure is reported as the final
	// result, without changing the return value or the cleanup order.
	defer func() { writeFundsCanonicalCSVBuildFailureV1(os.Stderr, stage, resultErr) }()
	zeroResult := domainnative.FundsCanonicalCSVSnapshotBuildResultV1{}
	zeroObject := domainfundsquerysource.ImmutableSnapshotObjectV1{}
	caseID := arguments.CaseIDV1()
	sourceSHA256, sourceByteLength := arguments.SourceIdentityV1()
	if runner == nil || ctx == nil || ctx.Err() != nil || runner.poisoned.Load() ||
		caseID == "" || sourceByteLength == 0 ||
		sourceByteLength > domainnative.FundsCanonicalCSVMaximumSourceBytesV1 ||
		readerIsNilV1(source) || interfaceIsNilV1(installer) {
		return zeroResult, zeroObject, "", ErrRequestInvalid
	}
	if sourceByteLength > uint64(^uint64(0)>>1) {
		return zeroResult, zeroObject, "", ErrRequestInvalid
	}
	executionContext, cancel := context.WithTimeout(ctx, domainnative.FundsCanonicalCSVMaximumDurationV1)
	defer cancel()
	stage = "ACQUIRE"
	if err := runner.acquire(executionContext); err != nil {
		return zeroResult, zeroObject, "", err
	}
	defer runner.release()
	stage = "ADMISSION"
	if runner.closed || runner.poisoned.Load() || runner.registry.Digest() != runner.registryDigest ||
		arguments.CaseIDV1() != caseID {
		return zeroResult, zeroObject, "", ErrUnavailable
	}
	stage = "PREVIOUS_SESSION"
	if err := runner.closeSessionLocked(); err != nil {
		runner.poisoned.Store(true)
		return zeroResult, zeroObject, "", ErrTermination
	}

	var inputSnapshot *privateAccountFlowSnapshot
	var output *fundsCanonicalCSVOutputV1
	var sealed *sealedFundsCanonicalCSVOutputV1
	defer func() {
		cleanupErr := error(nil)
		if output != nil {
			cleanupErr = errors.Join(cleanupErr, output.cleanup())
		}
		if inputSnapshot != nil {
			cleanupErr = errors.Join(cleanupErr, inputSnapshot.settle())
		}
		if sealed != nil {
			cleanupErr = errors.Join(cleanupErr, sealed.close())
		}
		if cleanupErr != nil {
			stage = "CLEANUP"
			runner.poisoned.Store(true)
			result = zeroResult
			object = zeroObject
			disposition = ""
			resultErr = ErrTermination
			return
		}
		if errors.Is(resultErr, ErrTermination) {
			runner.poisoned.Store(true)
		}
	}()

	stage = "SOURCE_MATERIALIZATION"
	sourceObject, err := domainfundsquerysource.NewImmutableSnapshotObjectV1(
		caseID,
		sourceSHA256,
		sourceByteLength,
	)
	if err != nil {
		return zeroResult, zeroObject, "", ErrRequestInvalid
	}
	readerLease := &fundsCanonicalCSVExactReaderLeaseV1{
		source: source, expectedSHA256: sourceSHA256, expectedSize: int64(sourceByteLength),
	}
	inputSnapshot, err = materializePrivateImmutableSnapshot(
		executionContext,
		runner.stagingAuthority,
		sourceObject,
		readerLease,
	)
	if err != nil {
		return zeroResult, zeroObject, "", err
	}
	stage = "INPUT_DUPLICATE"
	privateInput, err := inputSnapshot.duplicateForProcess()
	if err != nil {
		return zeroResult, zeroObject, "", err
	}
	input := &sessionReadOnlyInput{
		File: privateInput.file, ExpectedSHA256: privateInput.sha256, ExpectedSize: privateInput.size,
	}

	stage = "OUTPUT_PREPARE"
	requestID, err := randomProtocolID()
	if err != nil {
		if input.File.Close() != nil {
			input.File = nil
			return zeroResult, zeroObject, "", ErrTermination
		}
		input.File = nil
		return zeroResult, zeroObject, "", ErrUnavailable
	}
	output, err = prepareFundsCanonicalCSVOutputV1(
		runner.stagingRoot,
		runner.stagingAuthority,
		requestID,
	)
	if err != nil {
		if input.File.Close() != nil {
			input.File = nil
			return zeroResult, zeroObject, "", ErrTermination
		}
		input.File = nil
		return zeroResult, zeroObject, "", err
	}
	stage = "REQUEST_FRAME"
	requestFrame, err := domainnative.EncodeFundsBuildCanonicalCSVSnapshotNativeRequestFrameV1(
		requestID,
		arguments,
	)
	if err != nil {
		if input.File.Close() != nil {
			input.File = nil
			return zeroResult, zeroObject, "", ErrTermination
		}
		input.File = nil
		return zeroResult, zeroObject, "", ErrProtocol
	}
	stage = "OUTPUT_HANDOFF"
	if err := output.validateBeforeChild(); err != nil {
		if input.File.Close() != nil {
			input.File = nil
			return zeroResult, zeroObject, "", ErrTermination
		}
		input.File = nil
		return zeroResult, zeroObject, "", err
	}
	privateOutput, err := output.duplicateForProcess()
	if err != nil {
		if input.File.Close() != nil {
			input.File = nil
			return zeroResult, zeroObject, "", ErrTermination
		}
		input.File = nil
		return zeroResult, zeroObject, "", err
	}
	stage = "SESSION_OPEN"
	session, err := runner.openCanonicalCSVSessionLocked(
		executionContext,
		input,
		&sessionReadWriteOutput{File: privateOutput},
	)
	if err != nil {
		return zeroResult, zeroObject, "", err
	}
	processID := session.PID()
	stage = "ROUND_TRIP"
	responseFrame, roundTripErr := session.RoundTrip(
		executionContext,
		requestFrame,
		fundsCanonicalCSVResponseFrameLimit,
	)
	closeErr := session.Close()
	if closeErr != nil {
		stage = "SESSION_CLOSE"
		return zeroResult, zeroObject, "", ErrTermination
	}
	if roundTripErr != nil {
		return zeroResult, zeroObject, "", mapProcessAuthorityError(roundTripErr)
	}
	stage = "POST_SESSION"
	if runner.poisoned.Load() || runner.registry.Digest() != runner.registryDigest {
		return zeroResult, zeroObject, "", ErrRegistry
	}
	if executionContext.Err() != nil {
		return zeroResult, zeroObject, "", contextError(executionContext)
	}
	stage = "RESULT_PARSE"
	result, err = parseFundsCanonicalCSVSnapshotBuildResponse(
		responseFrame,
		requestID,
		processID,
		arguments,
	)
	if err != nil {
		return zeroResult, zeroObject, "", err
	}
	stage = "OUTPUT_SEAL"
	sealed, err = output.seal(executionContext, caseID)
	if err != nil {
		return zeroResult, zeroObject, "", err
	}
	object = sealed.object
	stage = "SOURCE_SETTLE"
	if err := inputSnapshot.settle(); err != nil {
		return zeroResult, zeroObject, "", err
	}
	inputSnapshot = nil
	if executionContext.Err() != nil {
		return zeroResult, zeroObject, "", contextError(executionContext)
	}
	if runner.registry.Digest() != runner.registryDigest {
		return zeroResult, zeroObject, "", ErrRegistry
	}
	stage = "INSTALL"
	disposition, err = installer.InstallExact(executionContext, object, sealed.file)
	if err != nil {
		return zeroResult, zeroObject, "", mapImmutableSnapshotInstallErrorV1(executionContext, err)
	}
	if disposition != fundsquerysourceport.ImmutableSnapshotInstallCreatedV1 &&
		disposition != fundsquerysourceport.ImmutableSnapshotInstallExistingEqualV1 {
		return zeroResult, zeroObject, "", ErrProtocol
	}
	stage = "INSTALLED_VALIDATION"
	if err := sealed.validate(executionContext); err != nil {
		return zeroResult, zeroObject, "", err
	}
	stage = "CLEANUP"
	if err := sealed.close(); err != nil {
		return zeroResult, zeroObject, "", err
	}
	sealed = nil
	output = nil
	return result, object, disposition, nil
}

func writeFundsCanonicalCSVBuildFailureV1(output io.Writer, stage string, err error) {
	if err == nil {
		return
	}
	switch stage {
	case "ADMISSION", "ACQUIRE", "PREVIOUS_SESSION", "SOURCE_MATERIALIZATION", "INPUT_DUPLICATE",
		"OUTPUT_PREPARE", "REQUEST_FRAME", "OUTPUT_HANDOFF", "SESSION_OPEN", "ROUND_TRIP", "SESSION_CLOSE",
		"POST_SESSION", "RESULT_PARSE", "OUTPUT_SEAL", "SOURCE_SETTLE", "INSTALL", "INSTALLED_VALIDATION", "CLEANUP":
	default:
		stage = "UNKNOWN"
	}
	class := "UNKNOWN"
	switch {
	case errors.Is(err, ErrTermination):
		class = "TERMINATION"
	case errors.Is(err, context.Canceled):
		class = "CANCELLED"
	case errors.Is(err, context.DeadlineExceeded):
		class = "DEADLINE"
	case errors.Is(err, ErrProtocol):
		class = "PROTOCOL"
	case errors.Is(err, ErrRegistry):
		class = "REGISTRY"
	case errors.Is(err, ErrRequestInvalid):
		class = "REQUEST_INVALID"
	case errors.Is(err, fundsquerysourceport.ErrNotFound):
		class = "SOURCE_NOT_FOUND"
	case errors.Is(err, fundsquerysourceport.ErrMismatch):
		class = "SOURCE_MISMATCH"
	case errors.Is(err, fundsquerysourceport.ErrCorrupt):
		class = "SOURCE_CORRUPT"
	case errors.Is(err, ErrUnavailable), errors.Is(err, fundsquerysourceport.ErrUnavailable):
		class = "UNAVAILABLE"
	}
	_, _ = io.WriteString(output, "[analytix] event=ANALYTIX_FUNDS_CSV_NATIVE_FAILURE_V1 layer=RUNNER stage="+stage+" class="+class+"\n")
}

func (lease *fundsCanonicalCSVExactReaderLeaseV1) CopyExactTo(
	ctx context.Context,
	destination *os.File,
) error {
	if lease == nil || ctx == nil || destination == nil || readerIsNilV1(lease.source) ||
		lease.expectedSize <= 0 || lease.expectedSize > int64(domainnative.FundsCanonicalCSVMaximumSourceBytesV1) ||
		lease.used.Swap(true) {
		return fundsquerysourceport.ErrMismatch
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	hasher := sha256.New()
	buffer := make([]byte, 64*1024)
	defer clear(buffer)
	bounded := io.LimitReader(
		fundsCanonicalCSVContextReaderV1{ctx: ctx, source: lease.source},
		lease.expectedSize+1,
	)
	written, err := io.CopyBuffer(io.MultiWriter(destination, hasher), bounded, buffer)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fundsquerysourceport.ErrMismatch
	}
	if written != lease.expectedSize || hex.EncodeToString(hasher.Sum(nil)) != lease.expectedSHA256 {
		return fundsquerysourceport.ErrMismatch
	}
	return nil
}

func (reader fundsCanonicalCSVContextReaderV1) Read(target []byte) (int, error) {
	if reader.ctx == nil || reader.source == nil {
		return 0, fundsquerysourceport.ErrMismatch
	}
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	read, err := reader.source.Read(target)
	if contextErr := reader.ctx.Err(); contextErr != nil {
		return read, contextErr
	}
	return read, err
}

func prepareFundsCanonicalCSVOutputV1(
	rootPath string,
	rootAuthority *os.File,
	requestID string,
) (_ *fundsCanonicalCSVOutputV1, resultErr error) {
	if rootAuthority == nil || !validRunnerDirectory(rootPath) || !domainsecurity.IsSHA256Hex(requestID) {
		return nil, ErrRequestInvalid
	}
	rootFD := int(rootAuthority.Fd())
	rootBefore, err := validatePrivateSnapshotDirectory(rootFD)
	if err != nil || validateFundsCanonicalCSVRootPathV1(rootPath, rootBefore) != nil {
		return nil, ErrRegistry
	}
	operationName := fundsCanonicalCSVOperationDirectoryPrefixV1 + requestID
	if err := unix.Mkdirat(rootFD, operationName, 0o700); err != nil {
		return nil, ErrUnavailable
	}
	output := &fundsCanonicalCSVOutputV1{
		rootPath: rootPath, rootAuthority: rootAuthority, rootRetiredLinks: rootBefore.links,
		operationName: operationName, operationLinked: true,
	}
	defer func() {
		if resultErr != nil && output != nil && output.cleanup() != nil {
			resultErr = ErrTermination
		}
	}()
	rootActive, err := validatePrivateSnapshotDirectory(rootFD)
	if err != nil || !validPrivateSnapshotDirectoryCreationTransition(rootBefore, rootActive) {
		return nil, ErrRegistry
	}
	output.rootIdentity = rootActive
	if unix.Fsync(rootFD) != nil || validateFundsCanonicalCSVRootPathV1(rootPath, rootActive) != nil {
		return nil, ErrTermination
	}
	operationFD, err := unix.Openat(
		rootFD,
		operationName,
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return nil, ErrUnavailable
	}
	output.operation = os.NewFile(uintptr(operationFD), "funds-canonical-csv-operation")
	if output.operation == nil {
		_ = unix.Close(operationFD)
		return nil, ErrTermination
	}
	operationBefore, err := validatePrivateSnapshotDirectory(operationFD)
	if err != nil || operationBefore.filesystem != rootActive.filesystem ||
		operationBefore.extendedSecurity != rootActive.extendedSecurity ||
		operationBefore.uid != rootActive.uid || operationBefore.gid != rootActive.gid ||
		validatePrivateSnapshotDirectoryBinding(rootFD, operationName, operationBefore) != nil {
		return nil, ErrRegistry
	}
	output.operationIdentity = operationBefore
	output.operationRetiredLinks = operationBefore.links
	writeFD, err := unix.Openat(
		operationFD,
		fundsCanonicalCSVOutputBasenameV1,
		unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0o600,
	)
	if err != nil {
		return nil, ErrUnavailable
	}
	output.outputLinked = true
	output.writeFile = os.NewFile(uintptr(writeFD), "funds-canonical-csv-output")
	if output.writeFile == nil {
		_ = unix.Close(writeFD)
		return nil, ErrTermination
	}
	operationActive, err := validatePrivateSnapshotDirectory(operationFD)
	if err != nil || !validPrivateSnapshotDirectoryCreationTransition(operationBefore, operationActive) {
		return nil, ErrRegistry
	}
	output.operationIdentity = operationActive
	if unix.Fchmod(writeFD, 0o600) != nil {
		return nil, ErrUnavailable
	}
	identity, err := validatePrivateSnapshotFile(
		writeFD,
		0,
		1,
		0o600,
		unix.O_RDWR,
		operationActive.filesystem,
		operationActive.extendedSecurity,
	)
	if err != nil || validatePrivateSnapshotFileBinding(
		operationFD,
		fundsCanonicalCSVOutputBasenameV1,
		identity,
		1,
		0o600,
	) != nil || ensureNoFundsCanonicalCSVWALV1(operationFD) != nil ||
		validatePrivateSnapshotDirectoryExact(rootFD, rootActive) != nil ||
		validatePrivateSnapshotDirectoryExact(operationFD, operationActive) != nil {
		return nil, ErrRegistry
	}
	output.initialIdentity = identity
	readFD, err := unix.Openat(
		operationFD,
		fundsCanonicalCSVOutputBasenameV1,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return nil, ErrUnavailable
	}
	output.readFile = os.NewFile(uintptr(readFD), "funds-canonical-csv-output-read-only")
	if output.readFile == nil {
		_ = unix.Close(readFD)
		return nil, ErrTermination
	}
	readIdentity, err := validatePrivateSnapshotFile(
		readFD,
		0,
		1,
		0o600,
		unix.O_RDONLY,
		operationActive.filesystem,
		operationActive.extendedSecurity,
	)
	if err != nil || readIdentity != identity {
		return nil, ErrRegistry
	}
	// No sensitive DuckDB byte may ever have a linked pathname. Both host
	// handles are pinned before the still-empty inode is durably unlinked.
	if err := unix.Unlinkat(operationFD, fundsCanonicalCSVOutputBasenameV1, 0); err != nil {
		return nil, ErrTermination
	}
	output.outputLinked = false
	restoredOperation, err := validatePrivateSnapshotDirectory(operationFD)
	if err != nil || !validPrivateSnapshotDirectoryRemovalTransition(
		operationActive,
		restoredOperation,
		output.operationRetiredLinks,
	) {
		return nil, ErrRegistry
	}
	output.operationIdentity = restoredOperation
	if unix.Fsync(operationFD) != nil ||
		validatePrivateSnapshotDirectoryExact(operationFD, restoredOperation) != nil ||
		validatePrivateSnapshotDirectoryExact(rootFD, rootActive) != nil {
		return nil, ErrTermination
	}
	if empty, emptyErr := privateSnapshotDirectoryEmpty(operationFD); emptyErr != nil || !empty {
		return nil, ErrRegistry
	}
	writeIdentity, writeErr := validatePrivateSnapshotFile(
		writeFD,
		0,
		0,
		0o600,
		unix.O_RDWR,
		restoredOperation.filesystem,
		restoredOperation.extendedSecurity,
	)
	readIdentity, readErr := validatePrivateSnapshotFile(
		readFD,
		0,
		0,
		0o600,
		unix.O_RDONLY,
		restoredOperation.filesystem,
		restoredOperation.extendedSecurity,
	)
	if writeErr != nil || readErr != nil || writeIdentity != identity || readIdentity != identity {
		return nil, ErrRegistry
	}
	return output, nil
}

func (output *fundsCanonicalCSVOutputV1) validateBeforeChild() error {
	if output == nil || output.rootAuthority == nil || output.operation == nil ||
		output.writeFile == nil || output.readFile == nil || output.outputLinked || !output.operationLinked {
		return ErrRegistry
	}
	rootFD := int(output.rootAuthority.Fd())
	operationFD := int(output.operation.Fd())
	writeFD := int(output.writeFile.Fd())
	readFD := int(output.readFile.Fd())
	if validatePrivateSnapshotDirectoryExact(rootFD, output.rootIdentity) != nil ||
		validateFundsCanonicalCSVRootPathV1(output.rootPath, output.rootIdentity) != nil ||
		validatePrivateSnapshotDirectoryExact(operationFD, output.operationIdentity) != nil ||
		validatePrivateSnapshotDirectoryBinding(
			rootFD, output.operationName, output.operationIdentity,
		) != nil || !privateSnapshotFileMatchesExact(
		writeFD, output.initialIdentity, 0, 0o600, unix.O_RDWR,
	) || !privateSnapshotFileMatchesExact(
		readFD, output.initialIdentity, 0, 0o600, unix.O_RDONLY,
	) || ensureNoFundsCanonicalCSVWALV1(operationFD) != nil {
		return ErrRegistry
	}
	if empty, err := privateSnapshotDirectoryEmpty(operationFD); err != nil || !empty {
		return ErrRegistry
	}
	return nil
}

func (output *fundsCanonicalCSVOutputV1) duplicateForProcess() (*os.File, error) {
	if err := output.validateBeforeChild(); err != nil {
		return nil, err
	}
	raw, err := output.writeFile.SyscallConn()
	if err != nil {
		return nil, ErrRegistry
	}
	duplicateFD := -1
	var duplicateErr error
	controlErr := raw.Control(func(fd uintptr) {
		duplicateFD, duplicateErr = unix.FcntlInt(fd, unix.F_DUPFD_CLOEXEC, privateSnapshotDuplicateMinimumFD)
	})
	if controlErr != nil || duplicateErr != nil || duplicateFD < 0 {
		if duplicateFD >= 0 {
			_ = unix.Close(duplicateFD)
		}
		return nil, ErrRegistry
	}
	duplicate := os.NewFile(uintptr(duplicateFD), "funds-canonical-csv-output-process")
	if duplicate == nil {
		_ = unix.Close(duplicateFD)
		return nil, ErrTermination
	}
	if !privateSnapshotFileMatchesExact(
		duplicateFD, output.initialIdentity, 0, 0o600, unix.O_RDWR,
	) || output.validateBeforeChild() != nil {
		if duplicate.Close() != nil {
			return nil, ErrTermination
		}
		return nil, ErrRegistry
	}
	return duplicate, nil
}

func (output *fundsCanonicalCSVOutputV1) seal(
	ctx context.Context,
	caseID string,
) (*sealedFundsCanonicalCSVOutputV1, error) {
	if ctx == nil || caseID == "" || output == nil || output.writeFile == nil ||
		output.readFile == nil || output.operation == nil || output.rootAuthority == nil ||
		output.outputLinked || !output.operationLinked {
		return nil, ErrRegistry
	}
	if err := ctx.Err(); err != nil {
		return nil, contextError(ctx)
	}
	rootFD := int(output.rootAuthority.Fd())
	operationFD := int(output.operation.Fd())
	writeFD := int(output.writeFile.Fd())
	readFD := int(output.readFile.Fd())
	if validatePrivateSnapshotDirectoryExact(rootFD, output.rootIdentity) != nil ||
		validateFundsCanonicalCSVRootPathV1(output.rootPath, output.rootIdentity) != nil ||
		validatePrivateSnapshotDirectoryExact(operationFD, output.operationIdentity) != nil ||
		validatePrivateSnapshotDirectoryBinding(
			rootFD, output.operationName, output.operationIdentity,
		) != nil || ensureNoFundsCanonicalCSVWALV1(operationFD) != nil {
		return nil, ErrRegistry
	}
	if empty, err := privateSnapshotDirectoryEmpty(operationFD); err != nil || !empty {
		return nil, ErrRegistry
	}
	var stat unix.Stat_t
	if unix.Fstat(writeFD, &stat) != nil || stat.Size <= 0 || stat.Size > fundsCanonicalCSVMaximumOutputBytesV1 {
		return nil, ErrRegistry
	}
	identity, err := validatePrivateSnapshotFile(
		writeFD,
		stat.Size,
		0,
		0o600,
		unix.O_RDWR,
		output.initialIdentity.filesystem,
		output.initialIdentity.extendedSecurity,
	)
	if err != nil || !samePrivateSnapshotObject(identity, output.initialIdentity) {
		return nil, ErrRegistry
	}
	if err := output.writeFile.Sync(); err != nil {
		return nil, ErrUnavailable
	}
	readIdentity, err := validatePrivateSnapshotFile(
		readFD,
		identity.size,
		0,
		0o600,
		unix.O_RDONLY,
		identity.filesystem,
		identity.extendedSecurity,
	)
	if err != nil || readIdentity != identity {
		return nil, ErrRegistry
	}
	digest, err := hashFundsCanonicalCSVOutputV1(ctx, readFD, identity, 0, 0o600)
	if err != nil {
		return nil, err
	}
	if ensureNoFundsCanonicalCSVWALV1(operationFD) != nil ||
		validatePrivateSnapshotDirectoryExact(rootFD, output.rootIdentity) != nil ||
		validatePrivateSnapshotDirectoryExact(operationFD, output.operationIdentity) != nil {
		return nil, ErrRegistry
	}
	if empty, err := privateSnapshotDirectoryEmpty(operationFD); err != nil || !empty {
		return nil, ErrRegistry
	}
	if unix.Fchmod(writeFD, 0o400) != nil || output.writeFile.Sync() != nil {
		return nil, ErrUnavailable
	}
	sealedIdentity, err := validatePrivateSnapshotFile(
		writeFD,
		identity.size,
		0,
		0o400,
		unix.O_RDWR,
		identity.filesystem,
		identity.extendedSecurity,
	)
	if err != nil || sealedIdentity != identity {
		return nil, ErrRegistry
	}
	readIdentity, err = validatePrivateSnapshotFile(
		readFD,
		identity.size,
		0,
		0o400,
		unix.O_RDONLY,
		identity.filesystem,
		identity.extendedSecurity,
	)
	if err != nil || readIdentity != identity {
		return nil, ErrRegistry
	}
	if err := output.writeFile.Close(); err != nil {
		output.writeFile = nil
		return nil, ErrTermination
	}
	output.writeFile = nil
	if unix.Fsync(operationFD) != nil || ensureNoFundsCanonicalCSVWALV1(operationFD) != nil {
		return nil, ErrTermination
	}
	readIdentity, err = validatePrivateSnapshotFile(
		readFD,
		identity.size,
		0,
		0o400,
		unix.O_RDONLY,
		identity.filesystem,
		identity.extendedSecurity,
	)
	if err != nil || readIdentity != identity {
		return nil, ErrRegistry
	}
	if empty, err := privateSnapshotDirectoryEmpty(operationFD); err != nil || !empty {
		return nil, ErrRegistry
	}
	confirmedDigest, err := hashFundsCanonicalCSVOutputV1(ctx, readFD, identity, 0, 0o400)
	if err != nil || confirmedDigest != digest {
		return nil, ErrRegistry
	}
	if err := output.retireOperationDirectory(); err != nil {
		return nil, err
	}
	object, err := domainfundsquerysource.NewImmutableSnapshotObjectV1(
		caseID,
		digest,
		uint64(identity.size),
	)
	if err != nil {
		return nil, ErrProtocol
	}
	sealed := &sealedFundsCanonicalCSVOutputV1{
		file: output.readFile, identity: identity, object: object,
	}
	output.readFile = nil
	return sealed, nil
}

func (output *fundsCanonicalCSVOutputV1) retireOperationDirectory() error {
	if output == nil || output.rootAuthority == nil || output.operation == nil ||
		output.operationName == "" || !output.operationLinked || output.outputLinked {
		return ErrTermination
	}
	rootFD := int(output.rootAuthority.Fd())
	operation := output.operation
	operationFD := int(operation.Fd())
	if validatePrivateSnapshotDirectoryExact(rootFD, output.rootIdentity) != nil ||
		validateFundsCanonicalCSVRootPathV1(output.rootPath, output.rootIdentity) != nil ||
		validatePrivateSnapshotDirectoryExact(operationFD, output.operationIdentity) != nil ||
		validatePrivateSnapshotDirectoryBinding(
			rootFD, output.operationName, output.operationIdentity,
		) != nil {
		return ErrTermination
	}
	empty, err := privateSnapshotDirectoryEmpty(operationFD)
	if err != nil || !empty || unix.Fsync(operationFD) != nil {
		return ErrTermination
	}
	if err := unix.Unlinkat(rootFD, output.operationName, unix.AT_REMOVEDIR); err != nil {
		return ErrTermination
	}
	output.operationLinked = false
	unlinked, err := validatePrivateSnapshotUnlinkedDirectory(operationFD)
	if err != nil || unlinked != output.operationIdentity || unix.Fsync(rootFD) != nil {
		return ErrTermination
	}
	currentRoot, err := validatePrivateSnapshotDirectory(rootFD)
	if err != nil || !validPrivateSnapshotDirectoryRemovalTransition(
		output.rootIdentity,
		currentRoot,
		output.rootRetiredLinks,
	) || validateFundsCanonicalCSVRootPathV1(output.rootPath, currentRoot) != nil {
		return ErrTermination
	}
	if err := operation.Close(); err != nil {
		output.operation = nil
		return ErrTermination
	}
	output.operation = nil
	output.operationName = ""
	output.rootAuthority = nil
	return nil
}

func (output *fundsCanonicalCSVOutputV1) cleanup() error {
	if output == nil {
		return nil
	}
	cleanupErr := error(nil)
	if output.outputLinked {
		if err := output.unlinkExactOutputForCleanup(); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	if output.writeFile != nil {
		cleanupErr = errors.Join(cleanupErr, output.writeFile.Close())
		output.writeFile = nil
	}
	if output.readFile != nil {
		cleanupErr = errors.Join(cleanupErr, output.readFile.Close())
		output.readFile = nil
	}
	if output.operationLinked {
		cleanupErr = errors.Join(cleanupErr, output.retireOperationDirectory())
	}
	if cleanupErr != nil {
		return ErrTermination
	}
	return nil
}

func (output *fundsCanonicalCSVOutputV1) unlinkExactOutputForCleanup() error {
	if output == nil || output.operation == nil || output.rootAuthority == nil ||
		!output.outputLinked || !output.operationLinked {
		return ErrTermination
	}
	authority := output.writeFile
	if authority == nil {
		authority = output.readFile
	}
	if authority == nil {
		return ErrTermination
	}
	operationFD := int(output.operation.Fd())
	var held unix.Stat_t
	var linked unix.Stat_t
	if unix.Fstat(int(authority.Fd()), &held) != nil ||
		unix.Fstatat(
			operationFD,
			fundsCanonicalCSVOutputBasenameV1,
			&linked,
			unix.AT_SYMLINK_NOFOLLOW,
		) != nil || held.Mode&unix.S_IFMT != unix.S_IFREG || linked.Mode&unix.S_IFMT != unix.S_IFREG ||
		held.Dev != linked.Dev || held.Ino != linked.Ino || linked.Nlink != 1 ||
		linked.Uid != uint32(os.Geteuid()) || linked.Gid != uint32(os.Getegid()) || linked.Flags != 0 ||
		uint64(linked.Dev) != output.initialIdentity.device || linked.Ino != output.initialIdentity.inode {
		return ErrTermination
	}
	before := output.operationIdentity
	if err := unix.Unlinkat(operationFD, fundsCanonicalCSVOutputBasenameV1, 0); err != nil {
		return ErrTermination
	}
	output.outputLinked = false
	after, err := validatePrivateSnapshotDirectory(operationFD)
	if err != nil || !validPrivateSnapshotDirectoryRemovalTransition(
		before,
		after,
		output.operationRetiredLinks,
	) || unix.Fsync(operationFD) != nil {
		return ErrTermination
	}
	output.operationIdentity = after
	return nil
}

func (sealed *sealedFundsCanonicalCSVOutputV1) validate(ctx context.Context) error {
	if sealed == nil || sealed.file == nil || ctx == nil ||
		domainfundsquerysource.ValidateImmutableSnapshotObjectV1(sealed.object) != nil {
		return ErrRegistry
	}
	identity, err := validatePrivateSnapshotFile(
		int(sealed.file.Fd()),
		sealed.identity.size,
		0,
		0o400,
		unix.O_RDONLY,
		sealed.identity.filesystem,
		sealed.identity.extendedSecurity,
	)
	if err != nil || identity != sealed.identity {
		return ErrRegistry
	}
	digest, err := hashFundsCanonicalCSVOutputV1(
		ctx,
		int(sealed.file.Fd()),
		sealed.identity,
		0,
		0o400,
	)
	if err != nil || digest != sealed.object.DuckDBSHA256 {
		return ErrRegistry
	}
	return nil
}

func (sealed *sealedFundsCanonicalCSVOutputV1) close() error {
	if sealed == nil || sealed.file == nil {
		return nil
	}
	err := sealed.file.Close()
	sealed.file = nil
	if err != nil {
		return ErrTermination
	}
	return nil
}

func hashFundsCanonicalCSVOutputV1(
	ctx context.Context,
	fd int,
	identity privateSnapshotIdentity,
	expectedLinks uint16,
	expectedMode uint16,
) (string, error) {
	if ctx == nil || fd < 0 || identity.size <= 0 || identity.size > fundsCanonicalCSVMaximumOutputBytesV1 {
		return "", ErrRegistry
	}
	hasher := sha256.New()
	buffer := make([]byte, fundsCanonicalCSVHashBufferBytesV1)
	defer clear(buffer)
	var offset int64
	for offset < identity.size {
		if err := ctx.Err(); err != nil {
			return "", contextError(ctx)
		}
		if !privateSnapshotFileMatchesExact(fd, identity, expectedLinks, expectedMode, unix.O_RDONLY) {
			return "", ErrRegistry
		}
		chunk := buffer
		if remaining := identity.size - offset; remaining < int64(len(chunk)) {
			chunk = chunk[:int(remaining)]
		}
		read, readErr := unix.Pread(fd, chunk, offset)
		if read > 0 {
			_, _ = hasher.Write(chunk[:read])
			offset += int64(read)
		}
		if readErr != nil && !errors.Is(readErr, unix.EINTR) {
			return "", ErrRegistry
		}
		if read == 0 && readErr == nil {
			return "", ErrRegistry
		}
	}
	extra := []byte{0}
	read, readErr := unix.Pread(fd, extra, identity.size)
	if read != 0 || readErr != nil && !errors.Is(readErr, io.EOF) ||
		!privateSnapshotFileMatchesExact(fd, identity, expectedLinks, expectedMode, unix.O_RDONLY) {
		return "", ErrRegistry
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func validateFundsCanonicalCSVRootPathV1(
	path string,
	expected privateSnapshotDirectoryIdentity,
) error {
	if !validRunnerDirectory(path) {
		return ErrRegistry
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return ErrRegistry
	}
	identity, identityErr := validatePrivateSnapshotDirectory(fd)
	closeErr := unix.Close(fd)
	if identityErr != nil || closeErr != nil || identity != expected {
		return ErrRegistry
	}
	return nil
}

func ensureNoFundsCanonicalCSVWALV1(operationFD int) error {
	if operationFD < 0 {
		return ErrRegistry
	}
	var stat unix.Stat_t
	err := unix.Fstatat(
		operationFD,
		fundsCanonicalCSVOutputBasenameV1+".wal",
		&stat,
		unix.AT_SYMLINK_NOFOLLOW,
	)
	if !errors.Is(err, unix.ENOENT) {
		return ErrRegistry
	}
	return nil
}

func readerIsNilV1(reader io.Reader) bool {
	return interfaceIsNilV1(reader)
}

func interfaceIsNilV1(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func mapImmutableSnapshotInstallErrorV1(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return contextError(ctx)
	}
	switch {
	case errors.Is(err, fundsquerysourceport.ErrMismatch), errors.Is(err, fundsquerysourceport.ErrCorrupt):
		return ErrRegistry
	case errors.Is(err, fundsquerysourceport.ErrUnavailable):
		return ErrUnavailable
	default:
		return ErrUnavailable
	}
}
