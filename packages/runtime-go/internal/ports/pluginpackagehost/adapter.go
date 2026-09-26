// Package pluginpackagehost defines the host-owned static editor adapter port.
package pluginpackagehost

import (
	"context"
	"encoding/json"

	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainskill "analytix.local/runtime-go/internal/domain/skill"
)

// Binding is derived from current trusted materialization and activation, never
// from caller-supplied roots, installed flags, scripts or principal projections.
type Binding struct {
	PackageID                string
	PackageVersion           string
	GenerationID             string
	ActivationRevision       uint64
	SourceRegistrationSHA256 string
}

type Readiness struct {
	Available  bool
	Operations []string
}

type Call struct {
	Binding        Binding
	Principal      domainidentity.PrincipalV1
	ContributionID string
	Operation      string
	Input          json.RawMessage
	// Admit revalidates the captured Host authority after adapter readiness and
	// lock acquisition, immediately before dispatch to the operation owner. It
	// is synchronous and invocation-local; never retain it or re-enter the Host.
	// A later revocation does not roll back an already admitted effect.
	Admit func() error
}

type Result struct{ Output json.RawMessage }

// Adapter implementations are injected by trusted Runtime composition. Readiness
// is read-only and reports actual adapter availability with a finite operation
// allowlist. Invoke must decode an operation-specific strict input schema and
// perform only that operation; it must not interpret shell/UNO/remote commands.
// Output belongs to the protected-local public contract, never private roots,
// keys, source registration JSON or internal errors. Principal remains Core-only.
// An adapter must not re-enter the Host: invocation holds its serialization lock.
type Adapter interface {
	Readiness(context.Context, Binding) (Readiness, error)
	Invoke(context.Context, Call) (Result, error)
}

// SkillReader reads only the fixed, admitted contribution from the verified
// installed generation. It must not resolve caller-supplied filesystem roots.
type SkillReader interface {
	ReadSkill(context.Context, Binding) (domainskill.PackageSnapshot, error)
}

// CapturedScopeLifecycle only revokes in-memory capture leases. An empty binding
// revokes all; otherwise retain only scopes bound to the currently verified Host
// generation/activation. This cannot read/write files, grant new authority or
// re-enter the serialized Host. Implementations are optional trusted adapters.
type CapturedScopeLifecycle interface {
	SynchronizeCapturedScopes(Binding)
}
