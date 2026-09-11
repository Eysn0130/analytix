package nativecomponenthost

import (
	"context"
	"errors"
	"testing"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
)

func TestOwnerTransactionSourceRowPageIsOptionalAndLifecycleOwned(t *testing.T) {
	withoutPage, err := newOwner(&healthOnlyRunnerCloser{}, &fakeCloser{}, &fakeCloser{})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := withoutPage.TransactionSourceRowPage(
		context.Background(),
		domainnative.TransactionSourceRowPageArgumentsV1{},
		domainfundsquerysource.ImmutableSnapshotObjectV1{},
		nil,
	); !errors.Is(err, ErrUnavailable) || result.RowCountV1() != 0 {
		t.Fatalf("health-only owner exposed source-row capability: result=%s err=%v", result.String(), err)
	}
	if err := withoutPage.Close(); err != nil {
		t.Fatal(err)
	}

	runner := &transactionSourceRowOwnerRunnerV1{}
	owner, err := newOwner(runner, &fakeCloser{}, &fakeCloser{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := owner.Close(); err != nil {
			t.Error(err)
		}
	}()
	arguments := domainnative.TransactionSourceRowPageArgumentsV1{}
	installedSnapshot := domainfundsquerysource.ImmutableSnapshotObjectV1{}
	result, err := owner.TransactionSourceRowPage(
		context.Background(), arguments, installedSnapshot, nil,
	)
	if err != nil || result.RowCountV1() != 0 || runner.calls != 1 {
		t.Fatalf("owner source-row delegation calls=%d result=%s err=%v", runner.calls, result.String(), err)
	}
}

type transactionSourceRowOwnerRunnerV1 struct {
	calls int
}

func (*transactionSourceRowOwnerRunnerV1) Execute(
	context.Context,
	domainnative.Request,
) (domainnative.Result, error) {
	return domainnative.Result{}, nil
}

func (runner *transactionSourceRowOwnerRunnerV1) TransactionSourceRowPage(
	context.Context,
	domainnative.TransactionSourceRowPageArgumentsV1,
	domainfundsquerysource.ImmutableSnapshotObjectV1,
	fundsquerysourceport.ExactReadLease,
) (domainnative.TransactionSourceRowPageV1, error) {
	runner.calls++
	return domainnative.TransactionSourceRowPageV1{}, nil
}

func (*transactionSourceRowOwnerRunnerV1) Close() error { return nil }
