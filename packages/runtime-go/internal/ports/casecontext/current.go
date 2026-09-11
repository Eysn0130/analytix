package casecontext

import domainsecurity "analytix.local/runtime-go/internal/domain/security"

// CurrentBindingReader resolves the workspace through the host filesystem and
// reads its current Analytix case binding. A missing or malformed binding is an
// error; authority-known case reads must never fall back to an unbound value.
type CurrentBindingReader interface {
	ReadCurrentBinding(string) (domainsecurity.CaseBinding, error)
}

// Observer classifies the current workspace marker without collapsing a
// missing, malformed, unreadable, or unstable binding into an unbound case.
type Observer interface {
	Observe(string) (domainsecurity.CaseBindingObservationV1, error)
}

// CurrentSnapshotValidator is the extension point for a side-effect-free,
// host-authoritative dataset freshness check. The filesystem binding/epoch
// validator does not claim source snapshot freshness when this port is absent.
type CurrentSnapshotValidator interface {
	ValidateCurrentSnapshot(domainsecurity.TurnSecurityContext) error
}
