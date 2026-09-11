package nativecomponenthost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	nativecomponentregistry "analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	fundsquerysourcefixture "analytix.local/runtime-go/internal/testsupport/fundsquerysourcefixture"
)

func TestPackagedRuntimeRootUsesOnlyRuntimeServerLayout(t *testing.T) {
	resources := filepath.Join(string(filepath.Separator), "Applications", "Analytix.app", "Contents", "Resources")
	root, err := PackagedRuntimeRoot(filepath.Join(resources, "runtime-go", "bin", "runtime-server"))
	if err != nil || root != filepath.Join(resources, "runtime") {
		t.Fatalf("runtime root = %q err=%v", root, err)
	}
	for _, path := range []string{
		filepath.Join(resources, "runtime-server"),
		filepath.Join(resources, "runtime-go", "runtime-server"),
		filepath.Join(resources, "runtime-go", "bin", "renamed-runtime"),
		"runtime-go/bin/runtime-server",
	} {
		if _, err := PackagedRuntimeRoot(path); !errors.Is(err, ErrTrust) {
			t.Fatalf("untrusted executable layout %q survived: %v", path, err)
		}
	}
}

func TestEmbeddedTrustMustBeEntirelyAbsentOrComplete(t *testing.T) {
	if available, err := embeddedTrustState(nativecomponentregistry.Trust{}); available || err != nil {
		t.Fatalf("blank development trust = available %t err=%v", available, err)
	}
	partial := nativecomponentregistry.Trust{ReceiptSHA256: "not-a-digest"}
	if available, err := embeddedTrustState(partial); available || !errors.Is(err, ErrTrust) {
		t.Fatalf("partial trust = available %t err=%v", available, err)
	}
}

func TestOwnerCloseIsOrderedAndRetryable(t *testing.T) {
	order := []string{}
	runner := &fakeRunnerCloser{name: "runner", order: &order, failCount: 1}
	registry := &fakeCloser{name: "registry", order: &order}
	roots := &fakeCloser{name: "roots", order: &order}
	owner, err := newOwner(runner, registry, roots)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(); !errors.Is(err, ErrLifecycle) {
		t.Fatalf("first close error = %v", err)
	}
	if !reflect.DeepEqual(order, []string{"runner"}) {
		t.Fatalf("unsafe close order after failure = %#v", order)
	}
	if _, err := owner.Execute(context.Background(), domainnative.Request{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("stopping owner accepted execution: %v", err)
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("retry close: %v", err)
	}
	if !reflect.DeepEqual(order, []string{"runner", "runner", "registry", "roots"}) {
		t.Fatalf("close order = %#v", order)
	}
	if err := owner.Close(); err != nil || len(order) != 4 {
		t.Fatalf("completed close was not idempotent: err=%v order=%#v", err, order)
	}
}

func TestOwnerCloseWaitsForAdmittedExecutionAndExecutionCannotPublishAfterStop(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	runner := &blockingRunnerCloser{started: started, release: release}
	owner, err := newOwner(runner, &fakeCloser{}, &fakeCloser{})
	if err != nil {
		t.Fatal(err)
	}

	executeDone := make(chan error, 1)
	go func() {
		_, executeErr := owner.Execute(context.Background(), domainnative.Request{})
		executeDone <- executeErr
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("execution did not enter runner")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- owner.Close() }()
	deadline := time.Now().Add(time.Second)
	for !owner.stopping.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !owner.stopping.Load() {
		t.Fatal("close did not stop admission")
	}
	select {
	case err := <-closeDone:
		t.Fatalf("close passed admitted execution: %v", err)
	default:
	}
	close(release)
	select {
	case err := <-executeDone:
		if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("execution published after stop: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("execution did not finish")
	}
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("close did not finish")
	}
	if runner.closeBeforeExecuteReturned {
		t.Fatal("runner closed before execution returned")
	}
}

func TestOwnerCloseCancelsEveryActiveExecutionBeforeClosingDependencies(t *testing.T) {
	order := []string{}
	runner := &cancelAwareRunnerCloser{
		entered: make(chan string, 2),
		order:   &order,
	}
	owner, err := newOwner(
		runner,
		&fakeCloser{name: "registry", order: &order},
		&fakeCloser{name: "roots", order: &order},
	)
	if err != nil {
		t.Fatal(err)
	}
	executeParent, cancelExecuteParent := context.WithCancel(context.Background())
	defer cancelExecuteParent()
	analyzeParent, cancelAnalyzeParent := context.WithCancel(context.Background())
	defer cancelAnalyzeParent()

	executeDone := make(chan error, 1)
	go func() {
		_, executeErr := owner.Execute(executeParent, domainnative.Request{})
		executeDone <- executeErr
	}()
	analyzeDone := make(chan error, 1)
	go func() {
		_, analyzeErr := owner.AnalyzeAccountFlows(
			analyzeParent,
			domainnative.Request{},
			domainfundsquerysource.DescriptorV1{},
			nil,
		)
		analyzeDone <- analyzeErr
	}()

	entered := map[string]bool{}
	for len(entered) < 2 {
		select {
		case operation := <-runner.entered:
			entered[operation] = true
		case <-time.After(time.Second):
			t.Fatalf("active executions did not enter runner: %#v", entered)
		}
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	for operation, done := range map[string]<-chan error{
		"execute": executeDone,
		"analyze": analyzeDone,
	} {
		select {
		case callErr := <-done:
			if !errors.Is(callErr, ErrUnavailable) {
				t.Fatalf("%s published after stop: %v", operation, callErr)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s did not return after owner cancellation", operation)
		}
	}
	if executeParent.Err() != nil || analyzeParent.Err() != nil {
		t.Fatalf("owner canceled caller contexts: execute=%v analyze=%v", executeParent.Err(), analyzeParent.Err())
	}
	if !reflect.DeepEqual(order, []string{"runner", "registry", "roots"}) {
		t.Fatalf("close order = %#v", order)
	}
}

func TestOwnerCloseCancelsAccountIngressExactCopyAndDrainsBeforeClosingDependencies(t *testing.T) {
	order := []string{}
	runner := &accountIngressCopyRunnerCloserV1{
		destination: fundsquerysourcefixture.MustNewUnlinkedDestinationV1(t),
		order:       &order,
	}
	owner, err := newOwner(
		runner,
		&fakeCloser{name: "registry", order: &order},
		&fakeCloser{name: "roots", order: &order},
	)
	if err != nil {
		t.Fatal(err)
	}
	source := &ownerBlockingExactReadLeaseV1{entered: make(chan struct{})}
	callerContext := context.Background()
	resolveDone := make(chan error, 1)
	go func() {
		_, resolveErr := owner.ResolveAccountIngress(
			callerContext,
			domainsecurity.TurnSecurityContext{},
			domainnative.ResolveAccountIngressArgumentsV1{},
			domainfundsquerysource.DescriptorV1{},
			source,
		)
		resolveDone <- resolveErr
	}()
	select {
	case <-source.entered:
	case <-time.After(time.Second):
		t.Fatal("account-ingress exact copy did not start")
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	select {
	case resolveErr := <-resolveDone:
		if !errors.Is(resolveErr, ErrUnavailable) {
			t.Fatalf("account-ingress result published after stop: %v", resolveErr)
		}
	case <-time.After(time.Second):
		t.Fatal("account-ingress exact copy did not drain after owner cancellation")
	}
	if !reflect.DeepEqual(order, []string{"runner", "registry", "roots"}) {
		t.Fatalf("close order = %#v", order)
	}
	if runner.closeBeforeCopyReturned {
		t.Fatal("runner closed before the cancelled exact copy returned")
	}
}

func TestOwnerCloseTimeoutPreservesDependenciesAndCanRetryAfterDrain(t *testing.T) {
	started := make(chan struct{})
	cancelObserved := make(chan struct{})
	release := make(chan struct{})
	order := []string{}
	runner := &blockingRunnerCloser{
		started:        started,
		cancelObserved: cancelObserved,
		release:        release,
		name:           "runner",
		order:          &order,
	}
	owner, err := newOwner(
		runner,
		&fakeCloser{name: "registry", order: &order},
		&fakeCloser{name: "roots", order: &order},
	)
	if err != nil {
		t.Fatal(err)
	}
	owner.drainDeadline = 20 * time.Millisecond

	executeDone := make(chan error, 1)
	go func() {
		_, executeErr := owner.Execute(context.Background(), domainnative.Request{})
		executeDone <- executeErr
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("execution did not enter runner")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- owner.Close() }()
	select {
	case <-cancelObserved:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("close did not cancel active execution")
	}
	select {
	case closeErr := <-closeDone:
		if !errors.Is(closeErr, ErrLifecycle) || !errors.Is(closeErr, context.DeadlineExceeded) {
			close(release)
			t.Fatalf("close timeout error = %v", closeErr)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("close wait was not bounded")
	}
	if len(order) != 0 || runner.closeCalls != 0 {
		close(release)
		t.Fatalf("dependencies closed before active execution drained: order=%#v runner_calls=%d", order, runner.closeCalls)
	}
	select {
	case callErr := <-executeDone:
		close(release)
		t.Fatalf("active execution returned before release: %v", callErr)
	default:
	}
	if _, executeErr := owner.Execute(context.Background(), domainnative.Request{}); !errors.Is(executeErr, ErrUnavailable) {
		close(release)
		t.Fatalf("stopping owner admitted Execute: %v", executeErr)
	}
	if _, analyzeErr := owner.AnalyzeAccountFlows(
		context.Background(),
		domainnative.Request{},
		domainfundsquerysource.DescriptorV1{},
		nil,
	); !errors.Is(analyzeErr, ErrUnavailable) {
		close(release)
		t.Fatalf("stopping owner admitted AnalyzeAccountFlows: %v", analyzeErr)
	}

	close(release)
	select {
	case executeErr := <-executeDone:
		if !errors.Is(executeErr, ErrUnavailable) {
			t.Fatalf("execution published after stop: %v", executeErr)
		}
	case <-time.After(time.Second):
		t.Fatal("execution did not return after release")
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("retry close: %v", err)
	}
	if !reflect.DeepEqual(order, []string{"runner", "registry", "roots"}) || runner.closeCalls != 1 {
		t.Fatalf("retry close order=%#v runner_calls=%d", order, runner.closeCalls)
	}
}

func TestOwnerWithoutTypedRunnerRejectsOnlyPrivateFundsOperations(t *testing.T) {
	runner := &healthOnlyRunnerCloser{}
	owner, err := newOwner(runner, &fakeCloser{}, &fakeCloser{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := owner.Execute(context.Background(), domainnative.Request{}); err != nil {
		t.Fatalf("health execute was disabled: %v", err)
	}
	result, err := owner.AnalyzeAccountFlows(
		context.Background(),
		domainnative.Request{},
		domainfundsquerysource.DescriptorV1{},
		nil,
	)
	if !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(result, domainnative.AnalyzeAccountFlowsResultV1{}) {
		t.Fatalf("unsupported typed flow result=%#v err=%v", result, err)
	}
	resolved, err := owner.ResolveAccountIngress(
		context.Background(),
		domainsecurity.TurnSecurityContext{},
		domainnative.ResolveAccountIngressArgumentsV1{},
		domainfundsquerysource.DescriptorV1{},
		nil,
	)
	if !errors.Is(err, ErrUnavailable) ||
		!reflect.DeepEqual(resolved, domainnative.ResolveAccountIngressResultV1{}) {
		t.Fatalf("unsupported typed account ingress result=%#v err=%v", resolved, err)
	}
}

type fakeRunnerCloser struct {
	name      string
	order     *[]string
	failCount int
}

type healthOnlyRunnerCloser struct{}

type cancelAwareRunnerCloser struct {
	entered chan string
	order   *[]string
}

type accountIngressCopyRunnerCloserV1 struct {
	destination             *os.File
	order                   *[]string
	mu                      sync.Mutex
	copyReturned            bool
	closeBeforeCopyReturned bool
}

type ownerBlockingExactReadLeaseV1 struct {
	entered chan struct{}
}

func (*healthOnlyRunnerCloser) Execute(context.Context, domainnative.Request) (domainnative.Result, error) {
	return domainnative.Result{}, nil
}

func (*healthOnlyRunnerCloser) Close() error { return nil }

func (*accountIngressCopyRunnerCloserV1) Execute(
	context.Context,
	domainnative.Request,
) (domainnative.Result, error) {
	return domainnative.Result{}, nil
}

func (runner *accountIngressCopyRunnerCloserV1) ResolveAccountIngress(
	ctx context.Context,
	_ domainsecurity.TurnSecurityContext,
	_ domainnative.ResolveAccountIngressArgumentsV1,
	_ domainfundsquerysource.DescriptorV1,
	source fundsquerysourceport.ExactReadLease,
) (domainnative.ResolveAccountIngressResultV1, error) {
	err := source.CopyExactTo(ctx, runner.destination)
	runner.mu.Lock()
	runner.copyReturned = true
	runner.mu.Unlock()
	return domainnative.ResolveAccountIngressResultV1{}, err
}

func (runner *accountIngressCopyRunnerCloserV1) Close() error {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if !runner.copyReturned {
		runner.closeBeforeCopyReturned = true
	}
	*runner.order = append(*runner.order, "runner")
	return nil
}

func (lease *ownerBlockingExactReadLeaseV1) CopyExactTo(
	ctx context.Context,
	destination *os.File,
) error {
	if lease == nil || ctx == nil || destination == nil {
		return fundsquerysourceport.ErrUnavailable
	}
	close(lease.entered)
	<-ctx.Done()
	return ctx.Err()
}

func (runner *cancelAwareRunnerCloser) Execute(ctx context.Context, _ domainnative.Request) (domainnative.Result, error) {
	runner.entered <- "execute"
	<-ctx.Done()
	return domainnative.Result{}, ctx.Err()
}

func (runner *cancelAwareRunnerCloser) AnalyzeAccountFlows(
	ctx context.Context,
	_ domainnative.Request,
	_ domainfundsquerysource.DescriptorV1,
	_ fundsquerysourceport.ExactReadLease,
) (domainnative.AnalyzeAccountFlowsResultV1, error) {
	runner.entered <- "analyze"
	<-ctx.Done()
	return domainnative.AnalyzeAccountFlowsResultV1{}, ctx.Err()
}

func (runner *cancelAwareRunnerCloser) Close() error {
	*runner.order = append(*runner.order, "runner")
	return nil
}

func (*fakeRunnerCloser) Execute(context.Context, domainnative.Request) (domainnative.Result, error) {
	return domainnative.Result{}, nil
}

func (*fakeRunnerCloser) AnalyzeAccountFlows(
	context.Context,
	domainnative.Request,
	domainfundsquerysource.DescriptorV1,
	fundsquerysourceport.ExactReadLease,
) (domainnative.AnalyzeAccountFlowsResultV1, error) {
	return domainnative.AnalyzeAccountFlowsResultV1{}, nil
}

func (closer *fakeRunnerCloser) Close() error {
	*closer.order = append(*closer.order, closer.name)
	if closer.failCount > 0 {
		closer.failCount--
		return errors.New("injected close failure")
	}
	return nil
}

type fakeCloser struct {
	name  string
	order *[]string
}

func (closer *fakeCloser) Close() error {
	if closer.order != nil {
		*closer.order = append(*closer.order, closer.name)
	}
	return nil
}

type blockingRunnerCloser struct {
	started                    chan struct{}
	cancelObserved             chan struct{}
	release                    chan struct{}
	name                       string
	order                      *[]string
	mu                         sync.Mutex
	closeCalls                 int
	executeReturned            bool
	closeBeforeExecuteReturned bool
}

func (runner *blockingRunnerCloser) Execute(ctx context.Context, _ domainnative.Request) (domainnative.Result, error) {
	close(runner.started)
	if runner.cancelObserved != nil {
		<-ctx.Done()
		close(runner.cancelObserved)
	}
	<-runner.release
	runner.mu.Lock()
	runner.executeReturned = true
	runner.mu.Unlock()
	return domainnative.Result{}, nil
}

func (runner *blockingRunnerCloser) AnalyzeAccountFlows(
	ctx context.Context,
	_ domainnative.Request,
	_ domainfundsquerysource.DescriptorV1,
	_ fundsquerysourceport.ExactReadLease,
) (domainnative.AnalyzeAccountFlowsResultV1, error) {
	close(runner.started)
	if runner.cancelObserved != nil {
		<-ctx.Done()
		close(runner.cancelObserved)
	}
	<-runner.release
	runner.mu.Lock()
	runner.executeReturned = true
	runner.mu.Unlock()
	return domainnative.AnalyzeAccountFlowsResultV1{}, nil
}

func (runner *blockingRunnerCloser) Close() error {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.closeCalls++
	if runner.order != nil {
		name := runner.name
		if name == "" {
			name = "runner"
		}
		*runner.order = append(*runner.order, name)
	}
	if !runner.executeReturned {
		runner.closeBeforeExecuteReturned = true
	}
	return nil
}
