//go:build !darwin

package nativecomponentrunner

import (
	"context"
	"io"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
)

func newRunner(Config) (*Runner, error) {
	return nil, ErrUnavailable
}

func (runner *Runner) Execute(context.Context, domainnative.Request) (domainnative.Result, error) {
	return domainnative.Result{}, ErrUnavailable
}

func (runner *Runner) AnalyzeAccountFlows(
	context.Context,
	domainnative.Request,
	domainfundsquerysource.DescriptorV1,
	fundsquerysourceport.ExactReadLease,
) (domainnative.AnalyzeAccountFlowsResultV1, error) {
	return domainnative.AnalyzeAccountFlowsResultV1{}, ErrUnavailable
}

func (runner *Runner) DirectSourcePreview(
	context.Context,
	domainnative.DirectSourcePreviewArgumentsV1,
	domainfundsquerysource.DescriptorV1,
	fundsquerysourceport.ExactReadLease,
) (domainnative.DirectSourcePreviewResultV1, error) {
	return domainnative.DirectSourcePreviewResultV1{}, ErrUnavailable
}

func (runner *Runner) DeterministicCleaning(
	context.Context,
	domainnative.DeterministicCleaningArgumentsV1,
	domainfundsquerysource.DescriptorV1,
	fundsquerysourceport.ExactReadLease,
) (domainnative.DeterministicCleaningResultV1, []byte, error) {
	return domainnative.DeterministicCleaningResultV1{}, nil, ErrUnavailable
}

func (runner *Runner) ResolveAccountIngress(
	context.Context,
	domainsecurity.TurnSecurityContext,
	domainnative.ResolveAccountIngressArgumentsV1,
	domainfundsquerysource.DescriptorV1,
	fundsquerysourceport.ExactReadLease,
) (domainnative.ResolveAccountIngressResultV1, error) {
	return domainnative.ResolveAccountIngressResultV1{}, ErrUnavailable
}

func (runner *Runner) TransactionSourceRowPage(
	context.Context,
	domainnative.TransactionSourceRowPageArgumentsV1,
	domainfundsquerysource.ImmutableSnapshotObjectV1,
	fundsquerysourceport.ExactReadLease,
) (domainnative.TransactionSourceRowPageV1, error) {
	return domainnative.TransactionSourceRowPageV1{}, ErrUnavailable
}

func (runner *Runner) BuildFundsCanonicalCSVSnapshot(
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
	return domainnative.FundsCanonicalCSVSnapshotBuildResultV1{},
		domainfundsquerysource.ImmutableSnapshotObjectV1{}, "", ErrUnavailable
}
