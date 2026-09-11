//go:build darwin

package nativecomponentrunner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	nativecomponentregistry "analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry"
	processauthority "analytix.local/runtime-go/internal/adapters/outbound/processauthority"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
)

type registryAuthority struct {
	registry *nativecomponentregistry.Registry
}

func (authority registryAuthority) Digest() string {
	return authority.registry.Digest()
}

func (authority registryAuthority) Acquire(ctx context.Context, componentID string) (executionLease, error) {
	return authority.registry.AcquireExecutionLease(ctx, componentID)
}

func newRunner(config Config) (*Runner, error) {
	if config.Registry == nil || config.WorkingDirectoryAuthority == nil || config.StagingRootAuthority == nil ||
		!validRunnerDirectory(config.WorkingDirectory) || !validRunnerDirectory(config.StagingRoot) {
		return nil, ErrRegistry
	}
	registry := registryAuthority{registry: config.Registry}
	digest := registry.Digest()
	if !domainsecurity.IsSHA256Hex(digest) {
		return nil, ErrRegistry
	}
	runner := &Runner{
		gate: make(chan struct{}, 1), registry: registry, registryDigest: digest,
		workingDirectory: filepath.Clean(config.WorkingDirectory), workingAuthority: config.WorkingDirectoryAuthority,
		stagingRoot: filepath.Clean(config.StagingRoot), stagingAuthority: config.StagingRootAuthority,
		sessionOpener: openProcessAuthoritySession,
	}
	runner.gate <- struct{}{}
	return runner, nil
}

func (runner *Runner) Execute(ctx context.Context, request domainnative.Request) (domainnative.Result, error) {
	if runner == nil || ctx == nil || ctx.Err() != nil || runner.poisoned.Load() {
		return domainnative.Result{}, ErrUnavailable
	}
	now := time.Now().UTC()
	policy, ok := domainnative.Policy(request.ComponentID, request.Operation)
	if !ok || domainnative.ValidateRequest(request, now) != nil ||
		request.ComponentID != domainnative.ComponentDataEngine || request.Operation != "health" {
		return domainnative.Result{}, ErrRequestInvalid
	}
	executionContext, cancel := context.WithDeadline(ctx, request.Deadline)
	defer cancel()
	if err := runner.acquire(executionContext); err != nil {
		return domainnative.Result{}, err
	}
	defer runner.release()
	if runner.closed || runner.poisoned.Load() || runner.registry.Digest() != runner.registryDigest {
		return domainnative.Result{}, ErrUnavailable
	}
	if domainnative.ValidateRequest(request, time.Now().UTC()) != nil {
		return domainnative.Result{}, ErrRequestInvalid
	}
	if runner.session != nil && runner.sessionBinding != request.Context.ContextDigest {
		if err := runner.closeSessionLocked(); err != nil {
			runner.poisoned.Store(true)
			return domainnative.Result{}, ErrTermination
		}
	}
	if runner.session == nil {
		if err := runner.openSessionLocked(executionContext, request.Context.ContextDigest); err != nil {
			return domainnative.Result{}, err
		}
	}

	requestID, err := randomProtocolID()
	if err != nil {
		return domainnative.Result{}, runner.failSessionLocked(ErrUnavailable)
	}
	requestFrame, err := encodePingRequestFrame(requestID)
	if err != nil {
		return domainnative.Result{}, runner.failSessionLocked(ErrProtocol)
	}
	responseFrame, err := runner.session.RoundTrip(executionContext, requestFrame, responseFrameLimit)
	if err != nil {
		return domainnative.Result{}, runner.failSessionLocked(mapProcessAuthorityError(err))
	}
	if !validPingResponse(responseFrame, requestID, runner.session.PID()) {
		return domainnative.Result{}, runner.failSessionLocked(ErrProtocol)
	}
	if executionContext.Err() != nil || runner.poisoned.Load() || runner.registry.Digest() != runner.registryDigest ||
		runner.sessionBinding != request.Context.ContextDigest {
		return domainnative.Result{}, runner.failSessionLocked(ErrRegistry)
	}
	result := domainnative.Result{
		SchemaVersion: domainnative.ResultSchemaVersion, ComponentID: policy.ComponentID,
		Operation: policy.Operation, Status: "ready", RegistryDigest: runner.registryDigest,
	}
	if domainnative.ValidateResult(result, policy) != nil {
		return domainnative.Result{}, runner.failSessionLocked(ErrProtocol)
	}
	return result, nil
}

func (runner *Runner) AnalyzeAccountFlows(
	ctx context.Context,
	request domainnative.Request,
	descriptor domainfundsquerysource.DescriptorV1,
	source fundsquerysourceport.ExactReadLease,
) (domainnative.AnalyzeAccountFlowsResultV1, error) {
	zero := domainnative.AnalyzeAccountFlowsResultV1{}
	if runner == nil || ctx == nil || ctx.Err() != nil || runner.poisoned.Load() {
		return zero, ErrUnavailable
	}
	now := time.Now().UTC()
	policy, ok := domainnative.Policy(request.ComponentID, request.Operation)
	if !ok || policy.ComponentID != domainnative.ComponentDataEngine ||
		policy.Operation != domainnative.OperationFundsAnalyzeAccountFlows ||
		request.AccountFlowArguments == nil || domainnative.ValidateRequest(request, now) != nil ||
		domainfundsquerysource.ValidateDescriptorV1(descriptor) != nil ||
		!accountFlowDescriptorMatchesRequest(descriptor, request) || exactReadLeaseIsNil(source) {
		return zero, ErrRequestInvalid
	}
	executionContext, cancel := context.WithDeadline(ctx, request.Deadline)
	defer cancel()
	if err := runner.acquire(executionContext); err != nil {
		return zero, err
	}
	defer runner.release()
	if runner.closed || runner.poisoned.Load() || runner.registry.Digest() != runner.registryDigest {
		return zero, ErrUnavailable
	}
	if domainnative.ValidateRequest(request, time.Now().UTC()) != nil ||
		domainfundsquerysource.ValidateDescriptorV1(descriptor) != nil ||
		!accountFlowDescriptorMatchesRequest(descriptor, request) {
		return zero, ErrRequestInvalid
	}
	// A case-bound flow is one-shot. A previously cached health process must be
	// confirmed gone before any private snapshot byte is copied or inherited.
	if err := runner.closeSessionLocked(); err != nil {
		runner.poisoned.Store(true)
		return zero, ErrTermination
	}
	requestID, err := randomProtocolID()
	if err != nil {
		return zero, ErrUnavailable
	}
	requestFrame, err := domainnative.EncodeAnalyzeAccountFlowsNativeRequestFrameV1(
		requestID,
		*request.AccountFlowArguments,
	)
	if err != nil {
		return zero, ErrProtocol
	}
	snapshot, err := materializePrivateAccountFlowSnapshot(executionContext, runner.stagingAuthority, descriptor, source)
	if err != nil {
		if errors.Is(err, ErrTermination) {
			runner.poisoned.Store(true)
		}
		return zero, err
	}
	privateInput, err := snapshot.duplicateForProcess()
	if err != nil {
		settleErr := snapshot.settle()
		if errors.Is(err, ErrTermination) || errors.Is(settleErr, ErrTermination) {
			runner.poisoned.Store(true)
			return zero, ErrTermination
		}
		if settleErr != nil {
			return zero, settleErr
		}
		return zero, err
	}
	input := &sessionReadOnlyInput{
		File: privateInput.file, ExpectedSHA256: privateInput.sha256, ExpectedSize: privateInput.size,
	}
	session, err := runner.openAccountFlowSessionLocked(executionContext, input)
	if err != nil {
		settleErr := snapshot.settle()
		if errors.Is(err, ErrTermination) || errors.Is(settleErr, ErrTermination) {
			runner.poisoned.Store(true)
			return zero, ErrTermination
		}
		if settleErr != nil {
			return zero, settleErr
		}
		return zero, err
	}
	processID := session.PID()
	responseFrame, roundTripErr := session.RoundTrip(
		executionContext,
		requestFrame,
		accountFlowResponseFrameLimit,
	)
	closeErr := session.Close()
	settleErr := snapshot.settle()
	if closeErr != nil || errors.Is(settleErr, ErrTermination) {
		runner.poisoned.Store(true)
		return zero, ErrTermination
	}
	if settleErr != nil {
		return zero, settleErr
	}
	if roundTripErr != nil {
		return zero, mapProcessAuthorityError(roundTripErr)
	}
	if runner.poisoned.Load() || runner.registry.Digest() != runner.registryDigest {
		return zero, ErrRegistry
	}
	if executionContext.Err() != nil {
		return zero, contextError(executionContext)
	}
	result, err := parseAnalyzeAccountFlowsResponse(
		responseFrame,
		requestID,
		processID,
		*request.AccountFlowArguments,
	)
	if err != nil {
		return zero, err
	}
	return result, nil
}

func (runner *Runner) ResolveAccountIngress(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	arguments domainnative.ResolveAccountIngressArgumentsV1,
	descriptor domainfundsquerysource.DescriptorV1,
	source fundsquerysourceport.ExactReadLease,
) (domainnative.ResolveAccountIngressResultV1, error) {
	zero := domainnative.ResolveAccountIngressResultV1{}
	if runner == nil || ctx == nil || ctx.Err() != nil || runner.poisoned.Load() {
		return zero, ErrUnavailable
	}
	if domainnative.ValidateResolveAccountIngressArgumentsAuthorityV1(
		arguments,
		securityContext,
		descriptor,
	) != nil || exactReadLeaseIsNil(source) {
		return zero, ErrRequestInvalid
	}
	if err := runner.acquire(ctx); err != nil {
		return zero, err
	}
	defer runner.release()
	if runner.closed || runner.poisoned.Load() || runner.registry.Digest() != runner.registryDigest {
		return zero, ErrUnavailable
	}
	if domainnative.ValidateResolveAccountIngressArgumentsAuthorityV1(
		arguments,
		securityContext,
		descriptor,
	) != nil {
		return zero, ErrRequestInvalid
	}
	// Every case-bound exact read is one-shot. A cached health process must be
	// confirmed gone before private snapshot bytes are copied or inherited.
	if err := runner.closeSessionLocked(); err != nil {
		runner.poisoned.Store(true)
		return zero, ErrTermination
	}
	requestID, err := randomProtocolID()
	if err != nil {
		return zero, ErrUnavailable
	}
	requestFrame, err := domainnative.EncodeResolveAccountIngressNativeRequestFrameV1(
		requestID,
		arguments,
	)
	if err != nil {
		return zero, ErrProtocol
	}
	snapshot, err := materializePrivateAccountFlowSnapshot(ctx, runner.stagingAuthority, descriptor, source)
	if err != nil {
		if errors.Is(err, ErrTermination) {
			runner.poisoned.Store(true)
		}
		return zero, err
	}
	privateInput, err := snapshot.duplicateForProcess()
	if err != nil {
		settleErr := snapshot.settle()
		if errors.Is(err, ErrTermination) || errors.Is(settleErr, ErrTermination) {
			runner.poisoned.Store(true)
			return zero, ErrTermination
		}
		if settleErr != nil {
			return zero, settleErr
		}
		return zero, err
	}
	input := &sessionReadOnlyInput{
		File: privateInput.file, ExpectedSHA256: privateInput.sha256, ExpectedSize: privateInput.size,
	}
	session, err := runner.openAccountFlowSessionLocked(ctx, input)
	if err != nil {
		settleErr := snapshot.settle()
		if errors.Is(err, ErrTermination) || errors.Is(settleErr, ErrTermination) {
			runner.poisoned.Store(true)
			return zero, ErrTermination
		}
		if settleErr != nil {
			return zero, settleErr
		}
		return zero, err
	}
	processID := session.PID()
	responseFrame, roundTripErr := session.RoundTrip(
		ctx,
		requestFrame,
		accountIngressResponseFrameLimit,
	)
	closeErr := session.Close()
	settleErr := snapshot.settle()
	if closeErr != nil || errors.Is(settleErr, ErrTermination) {
		runner.poisoned.Store(true)
		return zero, ErrTermination
	}
	if settleErr != nil {
		return zero, settleErr
	}
	if roundTripErr != nil {
		return zero, mapProcessAuthorityError(roundTripErr)
	}
	if runner.poisoned.Load() || runner.registry.Digest() != runner.registryDigest {
		return zero, ErrRegistry
	}
	if ctx.Err() != nil {
		return zero, contextError(ctx)
	}
	result, err := parseResolveAccountIngressResponse(
		responseFrame,
		requestID,
		processID,
		arguments,
	)
	if err != nil {
		return zero, err
	}
	return result, nil
}

func (runner *Runner) DirectSourcePreview(
	ctx context.Context,
	arguments domainnative.DirectSourcePreviewArgumentsV1,
	descriptor domainfundsquerysource.DescriptorV1,
	source fundsquerysourceport.ExactReadLease,
) (domainnative.DirectSourcePreviewResultV1, error) {
	zero := domainnative.DirectSourcePreviewResultV1{}
	if runner == nil || ctx == nil || ctx.Err() != nil || runner.poisoned.Load() ||
		domainnative.ValidateDirectSourcePreviewArgumentsAuthorityV1(arguments, descriptor) != nil ||
		exactReadLeaseIsNil(source) {
		return zero, ErrRequestInvalid
	}
	executionContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := runner.acquire(executionContext); err != nil {
		return zero, err
	}
	defer runner.release()
	if runner.closed || runner.poisoned.Load() || runner.registry.Digest() != runner.registryDigest {
		return zero, ErrUnavailable
	}
	if domainnative.ValidateDirectSourcePreviewArgumentsAuthorityV1(arguments, descriptor) != nil {
		return zero, ErrRequestInvalid
	}
	if err := runner.closeSessionLocked(); err != nil {
		runner.poisoned.Store(true)
		return zero, ErrTermination
	}
	requestID, err := randomProtocolID()
	if err != nil {
		return zero, ErrUnavailable
	}
	requestFrame, err := domainnative.EncodeDirectSourcePreviewNativeRequestFrameV1(requestID, arguments)
	if err != nil {
		return zero, ErrProtocol
	}
	snapshot, err := materializePrivateAccountFlowSnapshot(
		executionContext, runner.stagingAuthority, descriptor, source,
	)
	if err != nil {
		if errors.Is(err, ErrTermination) {
			runner.poisoned.Store(true)
		}
		return zero, err
	}
	privateInput, err := snapshot.duplicateForProcess()
	if err != nil {
		settleErr := snapshot.settle()
		if errors.Is(err, ErrTermination) || errors.Is(settleErr, ErrTermination) {
			runner.poisoned.Store(true)
			return zero, ErrTermination
		}
		if settleErr != nil {
			return zero, settleErr
		}
		return zero, err
	}
	input := &sessionReadOnlyInput{
		File: privateInput.file, ExpectedSHA256: privateInput.sha256, ExpectedSize: privateInput.size,
	}
	session, err := runner.openAccountFlowSessionLocked(executionContext, input)
	if err != nil {
		settleErr := snapshot.settle()
		if errors.Is(err, ErrTermination) || errors.Is(settleErr, ErrTermination) {
			runner.poisoned.Store(true)
			return zero, ErrTermination
		}
		if settleErr != nil {
			return zero, settleErr
		}
		return zero, err
	}
	processID := session.PID()
	responseFrame, roundTripErr := session.RoundTrip(
		executionContext,
		requestFrame,
		directSourcePreviewResponseFrameLimit,
	)
	closeErr := session.Close()
	settleErr := snapshot.settle()
	if closeErr != nil || errors.Is(settleErr, ErrTermination) {
		runner.poisoned.Store(true)
		return zero, ErrTermination
	}
	if settleErr != nil {
		return zero, settleErr
	}
	if roundTripErr != nil {
		return zero, mapProcessAuthorityError(roundTripErr)
	}
	if runner.poisoned.Load() || runner.registry.Digest() != runner.registryDigest {
		return zero, ErrRegistry
	}
	if executionContext.Err() != nil {
		return zero, contextError(executionContext)
	}
	result, err := parseDirectSourcePreviewResponse(
		responseFrame, requestID, processID, arguments,
	)
	if err != nil {
		return zero, err
	}
	return result, nil
}

func (runner *Runner) TransactionSourceRowPage(
	ctx context.Context,
	arguments domainnative.TransactionSourceRowPageArgumentsV1,
	installedSnapshot domainfundsquerysource.ImmutableSnapshotObjectV1,
	source fundsquerysourceport.ExactReadLease,
) (domainnative.TransactionSourceRowPageV1, error) {
	zero := domainnative.TransactionSourceRowPageV1{}
	if runner == nil || ctx == nil || ctx.Err() != nil || runner.poisoned.Load() {
		return zero, ErrUnavailable
	}
	if domainnative.ValidateTransactionSourceRowPageArgumentsAuthorityV1(arguments, installedSnapshot) != nil ||
		installedSnapshot.DuckDBByteLength > domainnative.TransactionSourceRowMaximumSnapshotBytesV1 ||
		exactReadLeaseIsNil(source) {
		return zero, ErrRequestInvalid
	}
	executionContext, cancel := context.WithTimeout(ctx, domainnative.TransactionSourceRowMaximumDurationV1)
	defer cancel()
	if err := runner.acquire(executionContext); err != nil {
		return zero, err
	}
	defer runner.release()
	if runner.closed || runner.poisoned.Load() || runner.registry.Digest() != runner.registryDigest {
		return zero, ErrUnavailable
	}
	if domainnative.ValidateTransactionSourceRowPageArgumentsAuthorityV1(arguments, installedSnapshot) != nil ||
		installedSnapshot.DuckDBByteLength > domainnative.TransactionSourceRowMaximumSnapshotBytesV1 {
		return zero, ErrRequestInvalid
	}
	// Staging source rows are a one-shot private snapshot effect. Confirm any
	// cached health process is gone before copying or inheriting exact bytes.
	if err := runner.closeSessionLocked(); err != nil {
		runner.poisoned.Store(true)
		return zero, ErrTermination
	}
	requestID, err := randomProtocolID()
	if err != nil {
		return zero, ErrUnavailable
	}
	requestFrame, err := domainnative.EncodeTransactionSourceRowPageNativeRequestFrameV1(requestID, arguments)
	if err != nil {
		return zero, ErrProtocol
	}
	snapshot, err := materializePrivateImmutableSnapshot(
		executionContext,
		runner.stagingAuthority,
		installedSnapshot,
		source,
	)
	if err != nil {
		if errors.Is(err, ErrTermination) {
			runner.poisoned.Store(true)
		}
		return zero, err
	}
	privateInput, err := snapshot.duplicateForProcess()
	if err != nil {
		settleErr := snapshot.settle()
		if errors.Is(err, ErrTermination) || errors.Is(settleErr, ErrTermination) {
			runner.poisoned.Store(true)
			return zero, ErrTermination
		}
		if settleErr != nil {
			return zero, settleErr
		}
		return zero, err
	}
	input := &sessionReadOnlyInput{
		File: privateInput.file, ExpectedSHA256: privateInput.sha256, ExpectedSize: privateInput.size,
	}
	session, err := runner.openAccountFlowSessionLocked(executionContext, input)
	if err != nil {
		settleErr := snapshot.settle()
		if errors.Is(err, ErrTermination) || errors.Is(settleErr, ErrTermination) {
			runner.poisoned.Store(true)
			return zero, ErrTermination
		}
		if settleErr != nil {
			return zero, settleErr
		}
		return zero, err
	}
	processID := session.PID()
	responseFrame, roundTripErr := session.RoundTrip(executionContext, requestFrame, transactionSourceRowResponseFrameLimit)
	closeErr := session.Close()
	settleErr := snapshot.settle()
	if closeErr != nil || errors.Is(settleErr, ErrTermination) {
		runner.poisoned.Store(true)
		return zero, ErrTermination
	}
	if settleErr != nil {
		return zero, settleErr
	}
	if roundTripErr != nil {
		return zero, mapProcessAuthorityError(roundTripErr)
	}
	if runner.poisoned.Load() || runner.registry.Digest() != runner.registryDigest {
		return zero, ErrRegistry
	}
	if executionContext.Err() != nil {
		return zero, contextError(executionContext)
	}
	result, err := parseTransactionSourceRowPageResponse(responseFrame, requestID, processID, arguments)
	if err != nil {
		return zero, err
	}
	return result, nil
}

func (runner *Runner) openSessionLocked(ctx context.Context, binding string) error {
	if runner.session != nil || runner.sessionOpener == nil || !domainsecurity.IsSHA256Hex(binding) || runner.registry.Digest() != runner.registryDigest {
		return ErrRegistry
	}
	lease, err := runner.registry.Acquire(ctx, domainnative.ComponentDataEngine)
	if err != nil || lease == nil {
		return ErrRegistry
	}
	defer lease.Close()
	if ctx.Err() != nil || runner.poisoned.Load() || runner.registry.Digest() != runner.registryDigest {
		return ErrUnavailable
	}
	executable, identity, err := lease.TakeExecutionFile()
	if err != nil || executable == nil || !validExecutionIdentity(identity, runner.registryDigest) {
		if executable != nil {
			_ = executable.Close()
		}
		return ErrRegistry
	}
	nonce, err := randomProtocolID()
	if err != nil {
		_ = executable.Close()
		return ErrUnavailable
	}
	session, err := runner.sessionOpener(ctx, sessionOpenRequest{
		Executable: executable, ExpectedExecutableSHA256: identity.CurrentSHA256,
		ExpectedExecutableSize: identity.CurrentSize, Arguments: append([]string(nil), runner.arguments...),
		WorkingDirectory: runner.workingDirectory, WorkingDirectoryAuthority: runner.workingAuthority,
		StagingRoot: runner.stagingRoot, StagingRootAuthority: runner.stagingAuthority,
		Environment: []string{
			"ANALYTIX_NATIVE_LAUNCH_NONCE=" + nonce,
			"ANALYTIX_NATIVE_PROTOCOL_VERSION=" + nativeProtocolVersion,
			"LANG=C", "LC_ALL=C", "TZ=UTC",
		},
	})
	if err != nil {
		return mapProcessAuthorityError(err)
	}
	err = session.AuthenticateReadiness(ctx, readinessFrameLimit, func(frame []byte, processID int) bool {
		return validReadiness(frame, nonce, processID)
	})
	registryChanged := runner.registry.Digest() != runner.registryDigest
	if err != nil || registryChanged {
		closeErr := session.Close()
		if closeErr != nil {
			runner.session = session
			runner.sessionBinding = binding
			runner.poisoned.Store(true)
			return ErrTermination
		}
		if err != nil {
			return mapProcessAuthorityError(err)
		}
		return ErrRegistry
	}
	runner.session = session
	runner.sessionBinding = binding
	return nil
}

func (runner *Runner) openAccountFlowSessionLocked(
	ctx context.Context,
	input *sessionReadOnlyInput,
) (_ runnerSession, resultErr error) {
	return runner.openDataEnginePrivateSessionLocked(ctx, input, nil)
}

func (runner *Runner) openCanonicalCSVSessionLocked(
	ctx context.Context,
	input *sessionReadOnlyInput,
	output *sessionReadWriteOutput,
) (_ runnerSession, resultErr error) {
	if output == nil || output.File == nil {
		if input != nil && input.File != nil {
			_ = input.File.Close()
		}
		return nil, ErrRegistry
	}
	return runner.openDataEnginePrivateSessionLocked(ctx, input, output)
}

func (runner *Runner) openDataEnginePrivateSessionLocked(
	ctx context.Context,
	input *sessionReadOnlyInput,
	output *sessionReadWriteOutput,
) (_ runnerSession, resultErr error) {
	inputHandedOff := false
	outputHandedOff := false
	defer func() {
		if !inputHandedOff && input != nil && input.File != nil {
			if err := input.File.Close(); err != nil {
				resultErr = ErrTermination
			}
		}
		if !outputHandedOff && output != nil && output.File != nil {
			if err := output.File.Close(); err != nil {
				resultErr = ErrTermination
			}
		}
	}()
	if runner.session != nil || runner.sessionOpener == nil || input == nil || input.File == nil ||
		runner.registry.Digest() != runner.registryDigest {
		return nil, ErrRegistry
	}
	lease, err := runner.registry.Acquire(ctx, domainnative.ComponentDataEngine)
	if err != nil || lease == nil {
		return nil, ErrRegistry
	}
	defer lease.Close()
	if ctx.Err() != nil || runner.poisoned.Load() || runner.registry.Digest() != runner.registryDigest {
		return nil, ErrUnavailable
	}
	executable, identity, err := lease.TakeExecutionFile()
	if err != nil || executable == nil || !validExecutionIdentity(identity, runner.registryDigest) {
		if executable != nil {
			_ = executable.Close()
		}
		return nil, ErrRegistry
	}
	nonce, err := randomProtocolID()
	if err != nil {
		_ = executable.Close()
		return nil, ErrUnavailable
	}
	session, err := runner.sessionOpener(ctx, sessionOpenRequest{
		Executable: executable, ExpectedExecutableSHA256: identity.CurrentSHA256,
		ExpectedExecutableSize: identity.CurrentSize, ReadOnlyInput: input, ReadWriteOutput: output,
		Arguments:        append([]string(nil), runner.arguments...),
		WorkingDirectory: runner.workingDirectory, WorkingDirectoryAuthority: runner.workingAuthority,
		StagingRoot: runner.stagingRoot, StagingRootAuthority: runner.stagingAuthority,
		Environment: []string{
			"ANALYTIX_NATIVE_LAUNCH_NONCE=" + nonce,
			"ANALYTIX_NATIVE_PROTOCOL_VERSION=" + nativeProtocolVersion,
			"LANG=C", "LC_ALL=C", "TZ=UTC",
		},
	})
	inputHandedOff = true
	outputHandedOff = true
	if err != nil {
		return nil, mapProcessAuthorityError(err)
	}
	if session == nil {
		runner.poisoned.Store(true)
		return nil, ErrTermination
	}
	err = session.AuthenticateReadiness(ctx, readinessFrameLimit, func(frame []byte, processID int) bool {
		return validReadiness(frame, nonce, processID)
	})
	registryChanged := runner.registry.Digest() != runner.registryDigest
	if err != nil || registryChanged {
		if closeErr := session.Close(); closeErr != nil {
			runner.poisoned.Store(true)
			return nil, ErrTermination
		}
		if err != nil {
			return nil, mapProcessAuthorityError(err)
		}
		return nil, ErrRegistry
	}
	return session, nil
}

func openProcessAuthoritySession(ctx context.Context, request sessionOpenRequest) (runnerSession, error) {
	var readOnlyInput *processauthority.ReadOnlyInput
	if request.ReadOnlyInput != nil {
		readOnlyInput = &processauthority.ReadOnlyInput{
			File: request.ReadOnlyInput.File, ExpectedSHA256: request.ReadOnlyInput.ExpectedSHA256,
			ExpectedSize: request.ReadOnlyInput.ExpectedSize,
		}
	}
	var readWriteOutput *processauthority.ReadWriteOutput
	if request.ReadWriteOutput != nil {
		readWriteOutput = &processauthority.ReadWriteOutput{File: request.ReadWriteOutput.File}
	}
	return processauthority.OpenSession(ctx, processauthority.SessionConfig{
		Executable: request.Executable, ExpectedExecutableSHA256: request.ExpectedExecutableSHA256,
		ExpectedExecutableSize: request.ExpectedExecutableSize, ReadOnlyInput: readOnlyInput,
		ReadWriteOutput:  readWriteOutput,
		Arguments:        append([]string(nil), request.Arguments...),
		WorkingDirectory: request.WorkingDirectory, WorkingDirectoryAuthority: request.WorkingDirectoryAuthority,
		StagingRoot: request.StagingRoot, StagingRootAuthority: request.StagingRootAuthority,
		Environment: append([]string(nil), request.Environment...),
	})
}

func accountFlowDescriptorMatchesRequest(
	descriptor domainfundsquerysource.DescriptorV1,
	request domainnative.Request,
) bool {
	return descriptor.CaseID == request.Context.CaseID &&
		descriptor.CaseBindingHash == request.Context.CaseBindingHash &&
		descriptor.DatasetSnapshotID == request.Context.DatasetSnapshotID &&
		descriptor.SourceManifestHash == request.Context.SourceManifestHash
}

func exactReadLeaseIsNil(source fundsquerysourceport.ExactReadLease) bool {
	if source == nil {
		return true
	}
	value := reflect.ValueOf(source)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func validRunnerDirectory(path string) bool {
	clean := filepath.Clean(path)
	return filepath.IsAbs(clean) && clean != string(filepath.Separator) && !strings.ContainsRune(clean, 0)
}

func validExecutionIdentity(identity nativecomponentregistry.ExecutionIdentity, registryDigest string) bool {
	return identity.ComponentID == domainnative.ComponentDataEngine && identity.BinaryName == "analytix-data-engine" &&
		domainsecurity.IsSHA256Hex(identity.CurrentSHA256) && identity.CurrentSize > 0 &&
		domainsecurity.IsSHA256Hex(identity.PayloadSHA256) && identity.PayloadSize > 0 &&
		identity.PayloadSize <= identity.CurrentSize && identity.Format == "mach-o" &&
		(identity.Arch == "arm64" || identity.Arch == "x64") && identity.RegistryDigest == registryDigest
}

func validReadiness(frame []byte, nonce string, pid int) bool {
	return ValidateProbeReadiness(frame, domainnative.ComponentDataEngine, nonce, pid)
}

func validPingResponse(frame []byte, requestID string, pid int) bool {
	return ValidateProbePingResponse(frame, domainnative.ComponentDataEngine, requestID, pid)
}

func randomProtocolID() (string, error) {
	payload := make([]byte, 32)
	if _, err := rand.Read(payload); err != nil {
		return "", err
	}
	return hex.EncodeToString(payload), nil
}

func mapProcessAuthorityError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, processauthority.ErrProtocol), errors.Is(err, processauthority.ErrOutputLimit),
		errors.Is(err, processauthority.ErrAbnormalExit):
		return ErrProtocol
	case errors.Is(err, processauthority.ErrExecutableIdentity):
		return ErrRegistry
	case errors.Is(err, processauthority.ErrTermination):
		return ErrTermination
	case errors.Is(err, processauthority.ErrCanceled):
		return context.Canceled
	case errors.Is(err, processauthority.ErrTimeout):
		return context.DeadlineExceeded
	default:
		return ErrUnavailable
	}
}
