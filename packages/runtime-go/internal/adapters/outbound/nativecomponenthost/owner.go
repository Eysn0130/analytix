package nativecomponenthost

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	nativecomponentregistry "analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	nativecomponentport "analytix.local/runtime-go/internal/ports/nativecomponent"
)

var (
	ErrUnavailable = errors.New("native_component_host_unavailable")
	ErrTrust       = errors.New("native_component_host_trust_invalid")
	ErrLifecycle   = errors.New("native_component_host_lifecycle_failed")
)

var _ nativecomponentport.Owner = (*Owner)(nil)
var _ nativecomponentport.FundsCanonicalCSVSnapshotBuilder = (*Owner)(nil)
var _ nativecomponentport.DeterministicCleaningRunner = (*Owner)(nil)

const ownerActiveDrainDeadline = 15 * time.Second

type closeRunner interface {
	nativecomponentport.Runner
	Close() error
}

type closer interface {
	Close() error
}

type currentExecutionLeaseRegistry interface {
	AcquireExecutionLease(context.Context, string) (*nativecomponentregistry.ExecutionLease, error)
}

type activeExecution struct {
	cancel context.CancelFunc
}

// Owner is the sole runtime lifecycle owner for the pinned native registry,
// long-lived runner, and private process directories. Once Close starts it
// permanently stops admission, even when a cleanup step must be retried.
type Owner struct {
	stateMu        sync.Mutex
	closeMu        sync.Mutex
	stopping       atomic.Bool
	active         map[*activeExecution]struct{}
	activeDrained  chan struct{}
	drainDeadline  time.Duration
	runner         closeRunner
	registry       closer
	roots          closer
	runnerClosed   bool
	registryClosed bool
	rootsClosed    bool
	closed         bool
}

func newOwner(runner closeRunner, registry, roots closer) (*Owner, error) {
	if runner == nil || registry == nil || roots == nil {
		return nil, ErrUnavailable
	}
	drained := make(chan struct{})
	close(drained)
	return &Owner{
		active:        make(map[*activeExecution]struct{}),
		activeDrained: drained,
		drainDeadline: ownerActiveDrainDeadline,
		runner:        runner,
		registry:      registry,
		roots:         roots,
	}, nil
}

// ValidateCurrentDataEngine revalidates the exact pinned data-engine component
// without transferring its descriptor to a runner or starting native work.
func (owner *Owner) ValidateCurrentDataEngine(ctx context.Context) error {
	if owner == nil || ctx == nil || ctx.Err() != nil {
		return ErrUnavailable
	}
	executionContext, execution, err := owner.beginExecution(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer func() {
		if execution != nil {
			owner.finishExecution(execution)
		}
	}()
	registry, ok := owner.registry.(currentExecutionLeaseRegistry)
	if !ok || dependencyIsNil(registry) {
		return ErrUnavailable
	}
	lease, err := registry.AcquireExecutionLease(executionContext, domainnative.ComponentDataEngine)
	if err != nil {
		if errors.Is(err, nativecomponentregistry.ErrComponentInvalid) {
			return ErrTrust
		}
		return ErrUnavailable
	}
	if lease == nil || lease.Close() != nil {
		return ErrUnavailable
	}
	stopped := owner.finishExecution(execution)
	execution = nil
	if stopped {
		return ErrUnavailable
	}
	return nil
}

func (owner *Owner) Execute(ctx context.Context, request domainnative.Request) (domainnative.Result, error) {
	if owner == nil {
		return domainnative.Result{}, errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	executionContext, execution, err := owner.beginExecution(ctx)
	if err != nil {
		return domainnative.Result{}, err
	}
	defer func() {
		if execution != nil {
			owner.finishExecution(execution)
		}
	}()
	result, err := owner.runner.Execute(executionContext, request)
	stopped := owner.finishExecution(execution)
	execution = nil
	if stopped {
		return domainnative.Result{}, errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	return result, err
}

func (owner *Owner) AnalyzeAccountFlows(
	ctx context.Context,
	request domainnative.Request,
	descriptor domainfundsquerysource.DescriptorV1,
	source fundsquerysourceport.ExactReadLease,
) (domainnative.AnalyzeAccountFlowsResultV1, error) {
	zero := domainnative.AnalyzeAccountFlowsResultV1{}
	if owner == nil {
		return zero, errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	flowRunner, ok := owner.runner.(nativecomponentport.AccountFlowRunner)
	if !ok || dependencyIsNil(flowRunner) {
		return zero, errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	executionContext, execution, err := owner.beginExecution(ctx)
	if err != nil {
		return zero, err
	}
	defer func() {
		if execution != nil {
			owner.finishExecution(execution)
		}
	}()
	result, err := flowRunner.AnalyzeAccountFlows(executionContext, request, descriptor, source)
	stopped := owner.finishExecution(execution)
	execution = nil
	if stopped {
		return zero, errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	return result, err
}

func (owner *Owner) ResolveAccountIngress(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	arguments domainnative.ResolveAccountIngressArgumentsV1,
	descriptor domainfundsquerysource.DescriptorV1,
	source fundsquerysourceport.ExactReadLease,
) (domainnative.ResolveAccountIngressResultV1, error) {
	zero := domainnative.ResolveAccountIngressResultV1{}
	if owner == nil {
		return zero, errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	resolver, ok := owner.runner.(nativecomponentport.AccountIngressRunner)
	if !ok || dependencyIsNil(resolver) {
		return zero, errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	executionContext, execution, err := owner.beginExecution(ctx)
	if err != nil {
		return zero, err
	}
	defer func() {
		if execution != nil {
			owner.finishExecution(execution)
		}
	}()
	result, err := resolver.ResolveAccountIngress(
		executionContext,
		securityContext,
		arguments,
		descriptor,
		source,
	)
	stopped := owner.finishExecution(execution)
	execution = nil
	if stopped {
		return zero, errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	return result, err
}

func (owner *Owner) DirectSourcePreview(
	ctx context.Context,
	arguments domainnative.DirectSourcePreviewArgumentsV1,
	descriptor domainfundsquerysource.DescriptorV1,
	source fundsquerysourceport.ExactReadLease,
) (domainnative.DirectSourcePreviewResultV1, error) {
	zero := domainnative.DirectSourcePreviewResultV1{}
	if owner == nil {
		return zero, errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	previewer, ok := owner.runner.(nativecomponentport.DirectSourcePreviewRunner)
	if !ok || dependencyIsNil(previewer) {
		return zero, errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	executionContext, execution, err := owner.beginExecution(ctx)
	if err != nil {
		return zero, err
	}
	defer func() {
		if execution != nil {
			owner.finishExecution(execution)
		}
	}()
	result, err := previewer.DirectSourcePreview(
		executionContext, arguments, descriptor, source,
	)
	stopped := owner.finishExecution(execution)
	execution = nil
	if stopped {
		return zero, errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	return result, err
}

func (owner *Owner) DeterministicCleaning(
	ctx context.Context,
	arguments domainnative.DeterministicCleaningArgumentsV1,
	descriptor domainfundsquerysource.DescriptorV1,
	source fundsquerysourceport.ExactReadLease,
) (domainnative.DeterministicCleaningResultV1, []byte, error) {
	zero := domainnative.DeterministicCleaningResultV1{}
	if owner == nil {
		return zero, nil, errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	cleaner, ok := owner.runner.(nativecomponentport.DeterministicCleaningRunner)
	if !ok || dependencyIsNil(cleaner) {
		return zero, nil, errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	executionContext, execution, err := owner.beginExecution(ctx)
	if err != nil {
		return zero, nil, err
	}
	defer func() {
		if execution != nil {
			owner.finishExecution(execution)
		}
	}()
	result, body, err := cleaner.DeterministicCleaning(
		executionContext, arguments, descriptor, source,
	)
	stopped := owner.finishExecution(execution)
	execution = nil
	if stopped {
		clear(body)
		return zero, nil, errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	return result, body, err
}

func (owner *Owner) TransactionSourceRowPage(
	ctx context.Context,
	arguments domainnative.TransactionSourceRowPageArgumentsV1,
	installedSnapshot domainfundsquerysource.ImmutableSnapshotObjectV1,
	source fundsquerysourceport.ExactReadLease,
) (domainnative.TransactionSourceRowPageV1, error) {
	zero := domainnative.TransactionSourceRowPageV1{}
	if owner == nil {
		return zero, errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	pageRunner, ok := owner.runner.(nativecomponentport.TransactionSourceRowPageRunner)
	if !ok || dependencyIsNil(pageRunner) {
		return zero, errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	executionContext, execution, err := owner.beginExecution(ctx)
	if err != nil {
		return zero, err
	}
	defer func() {
		if execution != nil {
			owner.finishExecution(execution)
		}
	}()
	result, err := pageRunner.TransactionSourceRowPage(
		executionContext,
		arguments,
		installedSnapshot,
		source,
	)
	stopped := owner.finishExecution(execution)
	execution = nil
	if stopped {
		return zero, errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	return result, err
}

func (owner *Owner) BuildFundsCanonicalCSVSnapshot(
	ctx context.Context,
	arguments domainnative.FundsCanonicalCSVSnapshotBuildArgumentsV1,
	source io.Reader,
	installer fundsquerysourceport.ImmutableSnapshotInstaller,
) (
	domainnative.FundsCanonicalCSVSnapshotBuildResultV1,
	domainfundsquerysource.ImmutableSnapshotObjectV1,
	fundsquerysourceport.ImmutableSnapshotInstallDispositionV1,
	error,
) {
	zeroResult := domainnative.FundsCanonicalCSVSnapshotBuildResultV1{}
	zeroObject := domainfundsquerysource.ImmutableSnapshotObjectV1{}
	if owner == nil {
		return zeroResult, zeroObject, "", errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	builder, ok := owner.runner.(nativecomponentport.FundsCanonicalCSVSnapshotBuilder)
	if !ok || dependencyIsNil(builder) {
		return zeroResult, zeroObject, "", errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	executionContext, execution, err := owner.beginExecution(ctx)
	if err != nil {
		return zeroResult, zeroObject, "", err
	}
	defer func() {
		if execution != nil {
			owner.finishExecution(execution)
		}
	}()
	result, object, disposition, err := builder.BuildFundsCanonicalCSVSnapshot(
		executionContext,
		arguments,
		source,
		installer,
	)
	stopped := owner.finishExecution(execution)
	execution = nil
	if stopped {
		return zeroResult, zeroObject, "", errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	return result, object, disposition, err
}

func dependencyIsNil(value any) bool {
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

func (owner *Owner) beginExecution(ctx context.Context) (context.Context, *activeExecution, error) {
	if ctx == nil {
		return nil, nil, nativecomponentport.ErrRequestInvalid
	}
	owner.stateMu.Lock()
	defer owner.stateMu.Unlock()
	if owner.stopping.Load() || owner.runner == nil {
		return nil, nil, errors.Join(ErrUnavailable, nativecomponentport.ErrUnavailable)
	}
	executionContext, cancel := context.WithCancel(ctx)
	if len(owner.active) == 0 {
		owner.activeDrained = make(chan struct{})
	}
	execution := &activeExecution{cancel: cancel}
	owner.active[execution] = struct{}{}
	return executionContext, execution, nil
}

func (owner *Owner) finishExecution(execution *activeExecution) bool {
	if owner == nil || execution == nil {
		return true
	}
	execution.cancel()
	owner.stateMu.Lock()
	defer owner.stateMu.Unlock()
	stopped := owner.stopping.Load()
	if _, ok := owner.active[execution]; !ok {
		return stopped
	}
	delete(owner.active, execution)
	if len(owner.active) == 0 {
		close(owner.activeDrained)
	}
	return stopped
}

func (owner *Owner) stopAndCancelActive() <-chan struct{} {
	owner.stateMu.Lock()
	owner.stopping.Store(true)
	if owner.activeDrained == nil {
		owner.activeDrained = make(chan struct{})
		if len(owner.active) == 0 {
			close(owner.activeDrained)
		}
	}
	drained := owner.activeDrained
	cancels := make([]context.CancelFunc, 0, len(owner.active))
	for execution := range owner.active {
		cancels = append(cancels, execution.cancel)
	}
	owner.stateMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	return drained
}

func (owner *Owner) waitForActiveDrain(drained <-chan struct{}) error {
	deadline := owner.drainDeadline
	if deadline <= 0 {
		deadline = ownerActiveDrainDeadline
	}
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	select {
	case <-drained:
		return nil
	case <-timer.C:
		return errors.Join(ErrLifecycle, context.DeadlineExceeded)
	}
}

func (owner *Owner) Close() error {
	if owner == nil {
		return nil
	}
	owner.closeMu.Lock()
	defer owner.closeMu.Unlock()
	if err := owner.waitForActiveDrain(owner.stopAndCancelActive()); err != nil {
		return err
	}
	if owner.closed {
		return nil
	}
	if !owner.runnerClosed {
		if err := owner.runner.Close(); err != nil {
			return errors.Join(ErrLifecycle, err)
		}
		owner.runnerClosed = true
	}
	if !owner.registryClosed {
		if err := owner.registry.Close(); err != nil {
			return errors.Join(ErrLifecycle, err)
		}
		owner.registryClosed = true
	}
	if !owner.rootsClosed {
		if err := owner.roots.Close(); err != nil {
			return errors.Join(ErrLifecycle, err)
		}
		owner.rootsClosed = true
	}
	owner.closed = true
	return nil
}

func PackagedRuntimeRoot(executable string) (string, error) {
	executable = filepath.Clean(strings.TrimSpace(executable))
	if !filepath.IsAbs(executable) || executable == string(filepath.Separator) || strings.ContainsRune(executable, 0) {
		return "", ErrTrust
	}
	leaf := filepath.Base(executable)
	if leaf != "runtime-server" && leaf != "runtime-server.exe" {
		return "", ErrTrust
	}
	binDirectory := filepath.Dir(executable)
	runtimeGoDirectory := filepath.Dir(binDirectory)
	resourcesDirectory := filepath.Dir(runtimeGoDirectory)
	if filepath.Base(binDirectory) != "bin" || filepath.Base(runtimeGoDirectory) != "runtime-go" ||
		resourcesDirectory == runtimeGoDirectory || resourcesDirectory == string(filepath.Separator) {
		return "", ErrTrust
	}
	runtimeRoot := filepath.Join(resourcesDirectory, "runtime")
	if !filepath.IsAbs(runtimeRoot) || runtimeRoot == string(filepath.Separator) {
		return "", ErrTrust
	}
	return runtimeRoot, nil
}

func embeddedTrustState(trust nativecomponentregistry.Trust) (bool, error) {
	values := []string{
		trust.ReceiptSHA256,
		trust.ManifestSHA256,
		trust.TargetKey,
		trust.SigningPolicySHA256,
		trust.SigningMode,
		trust.AppleTeamIdentifier,
	}
	nonEmpty := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			nonEmpty++
		}
	}
	if nonEmpty == 0 {
		return false, nil
	}
	if !domainsecurity.IsSHA256Hex(strings.TrimSpace(trust.ReceiptSHA256)) ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(trust.ManifestSHA256)) ||
		strings.TrimSpace(trust.TargetKey) == "" ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(trust.SigningPolicySHA256)) {
		return false, ErrTrust
	}
	if trust.SigningMode == "developer-id" && strings.TrimSpace(trust.AppleTeamIdentifier) == "" {
		return false, ErrTrust
	}
	if trust.SigningMode == "ad-hoc" && strings.TrimSpace(trust.AppleTeamIdentifier) != "" {
		return false, ErrTrust
	}
	return true, nil
}
