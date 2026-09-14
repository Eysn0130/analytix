// Package pluginpackagehost defines the host-owned static editor adapter port.
package pluginpackagehost

import (
	"context"
	"encoding/json"

	domainidentity "analytix.local/runtime-go/internal/domain/identity"
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
