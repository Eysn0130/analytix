//go:build darwin

package nativecomponentrunner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"time"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	"golang.org/x/sys/unix"
)

func (runner *Runner) DeterministicCleaning(
	ctx context.Context,
	arguments domainnative.DeterministicCleaningArgumentsV1,
	descriptor domainfundsquerysource.DescriptorV1,
	source fundsquerysourceport.ExactReadLease,
) (
	result domainnative.DeterministicCleaningResultV1,
	body []byte,
	resultErr error,
) {
	zero := domainnative.DeterministicCleaningResultV1{}
	if runner == nil || ctx == nil || ctx.Err() != nil || runner.poisoned.Load() ||
		domainnative.ValidateDeterministicCleaningArgumentsAuthorityV1(arguments, descriptor) != nil ||
		exactReadLeaseIsNil(source) {
		return zero, nil, ErrRequestInvalid
	}
	executionContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := runner.acquire(executionContext); err != nil {
		return zero, nil, err
	}
	defer runner.release()
	if runner.closed || runner.poisoned.Load() || runner.registry.Digest() != runner.registryDigest {
		return zero, nil, ErrUnavailable
	}
	if err := runner.closeSessionLocked(); err != nil {
		runner.poisoned.Store(true)
		return zero, nil, ErrTermination
	}

	var inputSnapshot *privateAccountFlowSnapshot
	var output *fundsCanonicalCSVOutputV1
	defer func() {
		cleanupErr := error(nil)
		if output != nil {
			cleanupErr = errors.Join(cleanupErr, output.cleanup())
		}
		if inputSnapshot != nil {
			cleanupErr = errors.Join(cleanupErr, inputSnapshot.settle())
		}
		if cleanupErr != nil {
			runner.poisoned.Store(true)
			clear(body)
			result = zero
			body = nil
			resultErr = ErrTermination
			return
		}
		if resultErr != nil {
			clear(body)
			body = nil
		}
		if errors.Is(resultErr, ErrTermination) {
			runner.poisoned.Store(true)
		}
	}()

	var err error
	inputSnapshot, err = materializePrivateAccountFlowSnapshot(
		executionContext, runner.stagingAuthority, descriptor, source,
	)
	if err != nil {
		return zero, nil, err
	}
	privateInput, err := inputSnapshot.duplicateForProcess()
	if err != nil {
		return zero, nil, err
	}
	input := &sessionReadOnlyInput{
		File: privateInput.file, ExpectedSHA256: privateInput.sha256, ExpectedSize: privateInput.size,
	}
	requestID, err := randomProtocolID()
	if err != nil {
		_ = input.File.Close()
		input.File = nil
		return zero, nil, ErrUnavailable
	}
	output, err = prepareFundsCanonicalCSVOutputV1(
		runner.stagingRoot, runner.stagingAuthority, requestID,
	)
	if err != nil {
		_ = input.File.Close()
		input.File = nil
		return zero, nil, err
	}
	requestFrame, err := domainnative.EncodeDeterministicCleaningNativeRequestFrameV1(
		requestID, arguments,
	)
	if err != nil {
		_ = input.File.Close()
		input.File = nil
		return zero, nil, ErrProtocol
	}
	privateOutput, err := output.duplicateForProcess()
	if err != nil {
		_ = input.File.Close()
		input.File = nil
		return zero, nil, err
	}
	session, err := runner.openCanonicalCSVSessionLocked(
		executionContext, input, &sessionReadWriteOutput{File: privateOutput},
	)
	if err != nil {
		return zero, nil, err
	}
	processID := session.PID()
	responseFrame, roundTripErr := session.RoundTrip(
		executionContext, requestFrame, deterministicCleaningResponseFrameLimit,
	)
	closeErr := session.Close()
	if closeErr != nil {
		return zero, nil, ErrTermination
	}
	if roundTripErr != nil {
		return zero, nil, mapProcessAuthorityError(roundTripErr)
	}
	if runner.poisoned.Load() || runner.registry.Digest() != runner.registryDigest {
		return zero, nil, ErrRegistry
	}
	if executionContext.Err() != nil {
		return zero, nil, contextError(executionContext)
	}
	result, err = parseDeterministicCleaningResponse(
		responseFrame, requestID, processID, arguments,
	)
	if err != nil {
		return zero, nil, err
	}
	body, err = output.readExactCleaningCSV(
		executionContext, result.OutputArtifactSHA256, result.OutputArtifactByteLength,
	)
	if err != nil {
		return zero, nil, err
	}
	// Input and output operation directories share the same staging root.
	// Retire the newer output directory first so the root identity returns to
	// the exact state captured by the older input lease before it is settled.
	if err := output.cleanup(); err != nil {
		return zero, nil, err
	}
	output = nil
	if err := inputSnapshot.settle(); err != nil {
		return zero, nil, err
	}
	inputSnapshot = nil
	if executionContext.Err() != nil || runner.registry.Digest() != runner.registryDigest {
		return zero, nil, ErrRegistry
	}
	return result, body, nil
}

func (output *fundsCanonicalCSVOutputV1) readExactCleaningCSV(
	ctx context.Context,
	expectedSHA256 string,
	expectedByteLength uint64,
) ([]byte, error) {
	if ctx == nil || output == nil || output.rootAuthority == nil || output.operation == nil ||
		output.writeFile == nil || output.readFile == nil || output.outputLinked || !output.operationLinked ||
		expectedByteLength == 0 || expectedByteLength > domainnative.DeterministicCleaningMaximumSourceBytesV1 {
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
		validatePrivateSnapshotDirectoryBinding(rootFD, output.operationName, output.operationIdentity) != nil ||
		ensureNoFundsCanonicalCSVWALV1(operationFD) != nil {
		return nil, ErrRegistry
	}
	var stat unix.Stat_t
	if unix.Fstat(writeFD, &stat) != nil || stat.Size <= 0 || uint64(stat.Size) != expectedByteLength {
		return nil, ErrRegistry
	}
	identity, err := validatePrivateSnapshotFile(
		writeFD, stat.Size, 0, 0o600, unix.O_RDWR,
		output.initialIdentity.filesystem, output.initialIdentity.extendedSecurity,
	)
	if err != nil || !samePrivateSnapshotObject(identity, output.initialIdentity) || output.writeFile.Sync() != nil {
		return nil, ErrRegistry
	}
	readIdentity, err := validatePrivateSnapshotFile(
		readFD, stat.Size, 0, 0o600, unix.O_RDONLY,
		identity.filesystem, identity.extendedSecurity,
	)
	offset, seekErr := unix.Seek(readFD, 0, io.SeekStart)
	if err != nil || readIdentity != identity || seekErr != nil || offset != 0 {
		return nil, ErrRegistry
	}
	body, err := io.ReadAll(io.LimitReader(output.readFile, int64(expectedByteLength)+1))
	if err != nil || uint64(len(body)) != expectedByteLength {
		clear(body)
		return nil, ErrProtocol
	}
	digest := sha256.Sum256(body)
	if hex.EncodeToString(digest[:]) != expectedSHA256 {
		clear(body)
		return nil, ErrProtocol
	}
	if validatePrivateSnapshotDirectoryExact(rootFD, output.rootIdentity) != nil ||
		validatePrivateSnapshotDirectoryExact(operationFD, output.operationIdentity) != nil ||
		ensureNoFundsCanonicalCSVWALV1(operationFD) != nil {
		clear(body)
		return nil, ErrRegistry
	}
	return body, nil
}
