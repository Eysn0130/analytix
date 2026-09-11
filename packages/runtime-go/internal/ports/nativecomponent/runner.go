package nativecomponent

import (
	"context"
	"errors"
	"io"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
)

var (
	ErrUnavailable            = errors.New("native_component_runner_unavailable")
	ErrRequestInvalid         = errors.New("native_component_runner_request_invalid")
	ErrRegistryInvalid        = errors.New("native_component_runner_registry_invalid")
	ErrProtocolInvalid        = errors.New("native_component_runner_protocol_invalid")
	ErrTerminationUnconfirmed = errors.New("native_component_runner_termination_failed")
)

// Runner owns the already-pinned native registry and process authority. It
// receives no caller-selected executable path, hash, working directory,
// environment, or staging root.
type Runner interface {
	Execute(context.Context, domainnative.Request) (domainnative.Result, error)
}

// AccountFlowRunner owns the separate host-private account-flow data path.
// The fixed request has already been admitted by the app service; descriptor
// metadata and the callback-scoped exact source lease remain explicit so the
// adapter never receives a database path, arbitrary SQL, or reusable handle.
type AccountFlowRunner interface {
	AnalyzeAccountFlows(
		context.Context,
		domainnative.Request,
		domainfundsquerysource.DescriptorV1,
		fundsquerysourceport.ExactReadLease,
	) (domainnative.AnalyzeAccountFlowsResultV1, error)
}

// AccountIngressRunner owns the fixed host-private account-resolution path.
// Current TurnSecurityContextV2 and the callback-scoped exact source lease are
// explicit; the adapter receives no caller path, SQL, reusable database handle,
// provider tool call, or complete-identifier output surface.
type AccountIngressRunner interface {
	ResolveAccountIngress(
		context.Context,
		domainsecurity.TurnSecurityContext,
		domainnative.ResolveAccountIngressArgumentsV1,
		domainfundsquerysource.DescriptorV1,
		fundsquerysourceport.ExactReadLease,
	) (domainnative.ResolveAccountIngressResultV1, error)
}

// DirectSourcePreviewRunner owns the fixed host-private local display path.
// It receives only an allowlisted typed projection and one callback-scoped
// exact source lease; no thread, turn, provider, MCP, path, SQL, evidence, or
// publication value crosses this seam.
type DirectSourcePreviewRunner interface {
	DirectSourcePreview(
		context.Context,
		domainnative.DirectSourcePreviewArgumentsV1,
		domainfundsquerysource.DescriptorV1,
		fundsquerysourceport.ExactReadLease,
	) (domainnative.DirectSourcePreviewResultV1, error)
}

// DeterministicCleaningRunner owns the fixed host-private transformation of
// one current immutable FPC1 DSV2 snapshot. The source crosses only the FD3
// exact lease and the canonical CSV result only the already-unlinked FD5
// output. No path, SQL, model rule, or reusable handle is caller supplied.
type DeterministicCleaningRunner interface {
	DeterministicCleaning(
		context.Context,
		domainnative.DeterministicCleaningArgumentsV1,
		domainfundsquerysource.DescriptorV1,
		fundsquerysourceport.ExactReadLease,
	) (domainnative.DeterministicCleaningResultV1, []byte, error)
}

// TransactionSourceRowPageRunner owns the staging-only fixed source-row
// projection. The request is assembled before DSV2 publication from
// authoritative host bindings and an inert installed snapshot object. The
// exact source is provided as a callback-scoped FD3 lease; neither a path nor
// SQL is caller-selectable.
type TransactionSourceRowPageRunner interface {
	TransactionSourceRowPage(
		context.Context,
		domainnative.TransactionSourceRowPageArgumentsV1,
		domainfundsquerysource.ImmutableSnapshotObjectV1,
		fundsquerysourceport.ExactReadLease,
	) (domainnative.TransactionSourceRowPageV1, error)
}

// FundsCanonicalCSVSnapshotBuilder owns the staging-only native conversion of
// one exact canonical CSV source into an inert immutable DuckDB object. The
// source exists only for the synchronous call; the installer is the existing
// no-replace immutable-object boundary. No returned value grants DSV2, query,
// evidence, publication, or PII-display authority.
type FundsCanonicalCSVSnapshotBuilder interface {
	BuildFundsCanonicalCSVSnapshot(
		context.Context,
		domainnative.FundsCanonicalCSVSnapshotBuildArgumentsV1,
		io.Reader,
		fundsquerysourceport.ImmutableSnapshotInstaller,
	) (
		domainnative.FundsCanonicalCSVSnapshotBuildResultV1,
		domainfundsquerysource.ImmutableSnapshotObjectV1,
		fundsquerysourceport.ImmutableSnapshotInstallDispositionV1,
		error,
	)
}

// Owner binds execution and lifecycle to the same concrete authority. A
// caller cannot accidentally execute through one native owner and close a
// different one during shutdown.
type Owner interface {
	Runner
	Close() error
}
