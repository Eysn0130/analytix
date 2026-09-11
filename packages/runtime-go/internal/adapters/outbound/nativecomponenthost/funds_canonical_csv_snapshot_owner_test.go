package nativecomponenthost

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
)

func TestOwnerFundsCanonicalCSVBuilderIsOptionalAndLifecycleOwned(t *testing.T) {
	withoutBuilder, err := newOwner(&healthOnlyRunnerCloser{}, &fakeCloser{}, &fakeCloser{})
	if err != nil {
		t.Fatal(err)
	}
	result, object, disposition, err := withoutBuilder.BuildFundsCanonicalCSVSnapshot(
		context.Background(),
		domainnative.FundsCanonicalCSVSnapshotBuildArgumentsV1{},
		bytes.NewReader([]byte("source")),
		&ownerFundsCanonicalCSVInstallerV1{},
	)
	if !errors.Is(err, ErrUnavailable) || !ownerFundsCanonicalCSVResultIsZeroV1(result) ||
		object != (domainfundsquerysource.ImmutableSnapshotObjectV1{}) || disposition != "" {
		t.Fatalf("health-only owner exposed builder: result=%s object=%#v disposition=%q err=%v", result.String(), object, disposition, err)
	}
	if err := withoutBuilder.Close(); err != nil {
		t.Fatal(err)
	}

	runner := &ownerFundsCanonicalCSVRunnerV1{}
	owner, err := newOwner(runner, &fakeCloser{}, &fakeCloser{})
	if err != nil {
		t.Fatal(err)
	}
	result, object, disposition, err = owner.BuildFundsCanonicalCSVSnapshot(
		context.Background(),
		domainnative.FundsCanonicalCSVSnapshotBuildArgumentsV1{},
		bytes.NewReader([]byte("source")),
		&ownerFundsCanonicalCSVInstallerV1{},
	)
	if err != nil || runner.calls != 1 || disposition != fundsquerysourceport.ImmutableSnapshotInstallCreatedV1 ||
		!ownerFundsCanonicalCSVResultIsZeroV1(result) || object != (domainfundsquerysource.ImmutableSnapshotObjectV1{}) {
		t.Fatalf("owner builder delegation calls=%d result=%s object=%#v disposition=%q err=%v", runner.calls, result.String(), object, disposition, err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOwnerCloseCancelsFundsCanonicalCSVBuildAndDrainsBeforeDependencies(t *testing.T) {
	order := []string{}
	runner := &ownerBlockingFundsCanonicalCSVRunnerV1{
		entered: make(chan struct{}),
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
	done := make(chan error, 1)
	go func() {
		_, _, _, buildErr := owner.BuildFundsCanonicalCSVSnapshot(
			context.Background(),
			domainnative.FundsCanonicalCSVSnapshotBuildArgumentsV1{},
			bytes.NewReader([]byte("source")),
			&ownerFundsCanonicalCSVInstallerV1{},
		)
		done <- buildErr
	}()
	select {
	case <-runner.entered:
	case <-time.After(time.Second):
		t.Fatal("builder did not enter owner lifecycle")
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	select {
	case buildErr := <-done:
		if !errors.Is(buildErr, ErrUnavailable) {
			t.Fatalf("builder published after owner stop: %v", buildErr)
		}
	case <-time.After(time.Second):
		t.Fatal("builder did not drain after owner cancellation")
	}
	if runner.CloseBeforeBuildReturned() {
		t.Fatal("runner closed before canceled builder returned")
	}
	wantOrder := []string{"runner", "registry", "roots"}
	if len(order) != len(wantOrder) {
		t.Fatalf("close order=%#v", order)
	}
	for index := range wantOrder {
		if order[index] != wantOrder[index] {
			t.Fatalf("close order=%#v", order)
		}
	}
}

type ownerFundsCanonicalCSVRunnerV1 struct {
	calls int
}

func (*ownerFundsCanonicalCSVRunnerV1) Execute(
	context.Context,
	domainnative.Request,
) (domainnative.Result, error) {
	return domainnative.Result{}, nil
}

func (runner *ownerFundsCanonicalCSVRunnerV1) BuildFundsCanonicalCSVSnapshot(
	context.Context,
	domainnative.FundsCanonicalCSVSnapshotBuildArgumentsV1,
	io.Reader,
	fundsquerysourceport.ImmutableSnapshotInstaller,
) (
	domainnative.FundsCanonicalCSVSnapshotBuildResultV1,
	domainfundsquerysource.ImmutableSnapshotObjectV1,
	fundsquerysourceport.ImmutableSnapshotInstallDispositionV1,
	error,
) {
	runner.calls++
	return domainnative.FundsCanonicalCSVSnapshotBuildResultV1{},
		domainfundsquerysource.ImmutableSnapshotObjectV1{},
		fundsquerysourceport.ImmutableSnapshotInstallCreatedV1,
		nil
}

func (*ownerFundsCanonicalCSVRunnerV1) Close() error { return nil }

type ownerBlockingFundsCanonicalCSVRunnerV1 struct {
	entered                  chan struct{}
	order                    *[]string
	mu                       sync.Mutex
	returned                 bool
	closeBeforeBuildReturned bool
}

func (*ownerBlockingFundsCanonicalCSVRunnerV1) Execute(
	context.Context,
	domainnative.Request,
) (domainnative.Result, error) {
	return domainnative.Result{}, nil
}

func (runner *ownerBlockingFundsCanonicalCSVRunnerV1) BuildFundsCanonicalCSVSnapshot(
	ctx context.Context,
	_ domainnative.FundsCanonicalCSVSnapshotBuildArgumentsV1,
	_ io.Reader,
	_ fundsquerysourceport.ImmutableSnapshotInstaller,
) (
	domainnative.FundsCanonicalCSVSnapshotBuildResultV1,
	domainfundsquerysource.ImmutableSnapshotObjectV1,
	fundsquerysourceport.ImmutableSnapshotInstallDispositionV1,
	error,
) {
	close(runner.entered)
	<-ctx.Done()
	runner.mu.Lock()
	runner.returned = true
	runner.mu.Unlock()
	return domainnative.FundsCanonicalCSVSnapshotBuildResultV1{},
		domainfundsquerysource.ImmutableSnapshotObjectV1{}, "", ctx.Err()
}

func (runner *ownerBlockingFundsCanonicalCSVRunnerV1) Close() error {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if !runner.returned {
		runner.closeBeforeBuildReturned = true
	}
	*runner.order = append(*runner.order, "runner")
	return nil
}

func (runner *ownerBlockingFundsCanonicalCSVRunnerV1) CloseBeforeBuildReturned() bool {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.closeBeforeBuildReturned
}

type ownerFundsCanonicalCSVInstallerV1 struct{}

func (*ownerFundsCanonicalCSVInstallerV1) InstallExact(
	context.Context,
	domainfundsquerysource.ImmutableSnapshotObjectV1,
	*os.File,
) (fundsquerysourceport.ImmutableSnapshotInstallDispositionV1, error) {
	return fundsquerysourceport.ImmutableSnapshotInstallCreatedV1, nil
}

func ownerFundsCanonicalCSVResultIsZeroV1(
	result domainnative.FundsCanonicalCSVSnapshotBuildResultV1,
) bool {
	sha256, byteLength, rowCount, maxTxnTS, maxID := result.SourceStatsV1()
	return sha256 == "" && byteLength == 0 && rowCount == 0 && maxTxnTS == "" && maxID == 0
}
