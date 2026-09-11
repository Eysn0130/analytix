package finalauthority

import (
	"context"
	"errors"
	"strings"

	authorityprojectionport "analytix.local/runtime-go/internal/ports/authorityprojection"
)

// SecureMutableProjectionCommitState describes the externally observable
// result of a mutable projection CAS. Callers must inspect the state even when
// ReplaceExact also returns an error: an error after the atomic replacement
// must never be mistaken for a failed write.
type SecureMutableProjectionCommitState = authorityprojectionport.CommitState

const (
	SecureMutableProjectionNotCommitted  = authorityprojectionport.NotCommitted
	SecureMutableProjectionCommitted     = authorityprojectionport.Committed
	SecureMutableProjectionIndeterminate = authorityprojectionport.Indeterminate
)

var (
	ErrSecureMutableProjectionCompareFailed       = authorityprojectionport.ErrCompareFailed
	ErrSecureMutableProjectionCommitIndeterminate = authorityprojectionport.ErrCommitIndeterminate
	ErrSecureMutableProjectionResidue             = authorityprojectionport.ErrResidue
	ErrSecureMutableProjectionUnsupported         = authorityprojectionport.ErrUnsupported
)

// SecureMutableProjectionObservation is an exact, bounded observation of the
// single projection file. An absent projection has Present=false, an empty
// Digest, and a nil Body. Projection bodies are deliberately required to be
// non-empty, so absence cannot be confused with valid content.
type SecureMutableProjectionObservation = authorityprojectionport.Observation

// SecureMutableProjectionResidue is an exact observation of a prepared or
// exchanged-out file left by an interrupted replacement. The primitive never
// chooses between Target and Residues: only an external monotonic witness can
// make that authority decision.
type SecureMutableProjectionResidue = authorityprojectionport.Residue

// SecureMutableProjectionResidueError quarantines a locally ambiguous
// projection while exposing enough exact state for an external witness to
// reconcile it. Observe returns Target together with this error; ReplaceExact
// returns not_committed and this error without changing any file.
type SecureMutableProjectionResidueError = authorityprojectionport.ResidueError

// SecureMutableProjectionReplaceResult reports whether the selected bytes
// became the named projection. Digest is SHA-256(exactProjectionBytes) for
// committed and indeterminate results, is empty for not_committed, and is also
// empty when ReconcileExact commits an externally authorized absent state.
type SecureMutableProjectionReplaceResult = authorityprojectionport.ReplaceResult

// SecureMutableProjection owns one fixed, host-private, relative file name.
// Semantic signing and monotonicity remain caller responsibilities; this type
// supplies exact byte observation, cross-process compare-and-swap, and durable
// atomic replacement.
type SecureMutableProjection struct {
	root     privateRootAuthority
	name     string
	maxBytes int
	gate     chan struct{}
	faults   *secureMutableProjectionFaults
}

var _ authorityprojectionport.Store = (*SecureMutableProjection)(nil)

// secureMutableProjectionFaults exists only so package tests can exercise
// commit cut points deterministically. Production constructors leave it nil.
type secureMutableProjectionFaults struct {
	BeforeExpectedRecheck   func()
	BeforeAtomicReplace     func()
	AfterAtomicReplace      func() error
	AfterDirectorySync      func() error
	BeforeReadback          func()
	BeforeCleanup           func() error
	BeforeReconcileExchange func()
	BeforeReconcileCleanup  func()
}

func OpenSecureMutableProjection(root, name string, maxBytes int) (*SecureMutableProjection, error) {
	root = strings.TrimSpace(root)
	if root == "" || name != strings.TrimSpace(name) || maxBytes <= 0 || maxBytes > maxPrivateAcceptedFinalBytes {
		return nil, errors.New("secure mutable projection configuration is invalid")
	}
	authority, err := openSecureMutableProjection(root, name, maxBytes)
	if err != nil {
		return nil, err
	}
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	return &SecureMutableProjection{root: authority, name: name, maxBytes: maxBytes, gate: gate}, nil
}

func (store *SecureMutableProjection) Observe(ctx context.Context) (SecureMutableProjectionObservation, error) {
	if store == nil {
		return SecureMutableProjectionObservation{}, errors.New("secure mutable projection is unavailable")
	}
	if err := store.acquire(ctx); err != nil {
		return SecureMutableProjectionObservation{}, err
	}
	defer store.release()
	return observeSecureMutableProjection(ctx, store.root, store.name, store.maxBytes)
}

// ReplaceExact atomically replaces the projection only when expectedDigest
// matches the current exact bytes. An empty expectedDigest means the projection
// must be absent. nextBytes must be non-empty and bounded.
func (store *SecureMutableProjection) ReplaceExact(ctx context.Context, expectedDigest string, nextBytes []byte) (SecureMutableProjectionReplaceResult, error) {
	if store == nil || len(nextBytes) == 0 || len(nextBytes) > store.maxBytes {
		return SecureMutableProjectionReplaceResult{State: SecureMutableProjectionNotCommitted}, errors.New("secure mutable projection replacement input is invalid")
	}
	if expectedDigest != strings.TrimSpace(expectedDigest) || expectedDigest != "" && !validPrivateDigest(expectedDigest) {
		return SecureMutableProjectionReplaceResult{State: SecureMutableProjectionNotCommitted}, errors.New("secure mutable projection expected digest is invalid")
	}
	if err := store.acquire(ctx); err != nil {
		return SecureMutableProjectionReplaceResult{State: SecureMutableProjectionNotCommitted}, err
	}
	defer store.release()
	// Copy before crossing the filesystem boundary so a caller cannot mutate
	// the staged bytes concurrently with this operation.
	next := append([]byte(nil), nextBytes...)
	return replaceSecureMutableProjection(ctx, store.root, store.name, store.maxBytes, expectedDigest, next, store.faults)
}

// ReconcileExact resolves interrupted replacement residue only when the
// current target still matches expectedTargetDigest and according to an exact
// projection-bytes digest selected by an external witness verifier. Both
// digests refer to canonical projection bytes, never the witness's semantic
// state/index/head digest. An empty expectedTargetDigest means the observed
// target was absent; an empty verifiedProjectionDigest means the external
// verifier selected absence.
func (store *SecureMutableProjection) ReconcileExact(ctx context.Context, expectedTargetDigest, verifiedProjectionDigest string) (SecureMutableProjectionReplaceResult, error) {
	if store == nil {
		return SecureMutableProjectionReplaceResult{State: SecureMutableProjectionNotCommitted}, errors.New("secure mutable projection is unavailable")
	}
	if expectedTargetDigest != strings.TrimSpace(expectedTargetDigest) || expectedTargetDigest != "" && !validPrivateDigest(expectedTargetDigest) ||
		verifiedProjectionDigest != strings.TrimSpace(verifiedProjectionDigest) || verifiedProjectionDigest != "" && !validPrivateDigest(verifiedProjectionDigest) {
		return SecureMutableProjectionReplaceResult{State: SecureMutableProjectionNotCommitted}, errors.New("secure mutable projection verified bytes digest is invalid")
	}
	if err := store.acquire(ctx); err != nil {
		return SecureMutableProjectionReplaceResult{State: SecureMutableProjectionNotCommitted}, err
	}
	defer store.release()
	return reconcileSecureMutableProjection(ctx, store.root, store.name, store.maxBytes, expectedTargetDigest, verifiedProjectionDigest, store.faults)
}

func (store *SecureMutableProjection) acquire(ctx context.Context) error {
	if ctx == nil {
		<-store.gate
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-store.gate:
		return nil
	}
}

func (store *SecureMutableProjection) release() {
	store.gate <- struct{}{}
}
