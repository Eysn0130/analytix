package authorityprojection

import (
	"context"
	"errors"
	"fmt"
)

// CommitState is the externally observable result of one exact projection
// CAS. Callers must inspect State even when the operation also returns an
// error, because post-commit failures cannot be treated as non-commits.
type CommitState string

const (
	NotCommitted  CommitState = "not_committed"
	Committed     CommitState = "committed"
	Indeterminate CommitState = "indeterminate"
)

var (
	ErrCompareFailed       = errors.New("secure mutable projection compare failed")
	ErrCommitIndeterminate = errors.New("secure mutable projection commit is indeterminate")
	ErrResidue             = errors.New("secure mutable projection has indeterminate residue")
	ErrUnsupported         = errors.New("secure mutable projection is unsupported on this platform")
)

type Observation struct {
	Present bool
	Digest  string
	Body    []byte
}

type Residue struct {
	Name   string
	Digest string
	Body   []byte
}

// ResidueError exposes every exact local candidate without choosing a head.
// Only an independently verified authority may select an exact byte digest
// for reconciliation.
type ResidueError struct {
	Target   Observation
	Residues []Residue
}

func (err *ResidueError) Error() string {
	if err == nil {
		return ErrResidue.Error()
	}
	return fmt.Sprintf("%s: %d residue file(s)", ErrResidue, len(err.Residues))
}

func (err *ResidueError) Unwrap() error { return ErrResidue }

type ReplaceResult struct {
	State  CommitState
	Digest string
}

// Store is the app-facing mutable projection boundary. expectedTargetDigest
// in ReconcileExact is the exact target observed by the upper layer before it
// selected a residue. witnessSelectedDigest is the exact canonical projection
// bytes SHA selected after validating semantic witness authority. Reconcile
// must compare expectedTargetDigest while holding the same filesystem lock as
// the mutation so a stale observation cannot replace a newer target.
type Store interface {
	Observe(context.Context) (Observation, error)
	ReplaceExact(context.Context, string, []byte) (ReplaceResult, error)
	ReconcileExact(context.Context, string, string) (ReplaceResult, error)
}
