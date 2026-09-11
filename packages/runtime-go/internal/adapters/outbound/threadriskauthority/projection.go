package threadriskauthority

import (
	"bytes"
	"context"
	"errors"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityprojectionport "analytix.local/runtime-go/internal/ports/authorityprojection"
	storeport "analytix.local/runtime-go/internal/ports/threadriskauthority"
)

const witnessSelectedProjectionName = "witness-selected-index.json"

var _ storeport.Projection = (*Projection)(nil)

var (
	ErrProjectionConflict      = errors.New("thread risk authority projection conflicts with witnessed state")
	ErrProjectionIndeterminate = errors.New("thread risk authority projection is indeterminate")
)

// Projection is a disposable local view of an index that the app has already
// selected with a fresh witness observation. It has no read/current method.
// The exact target body supplied here is the only value authorized for
// residue reconciliation.
type Projection struct {
	store authorityprojectionport.Store
}

func NewProjection(root string) (*Projection, error) {
	if root == "" || root != strings.TrimSpace(root) {
		return nil, errors.New("thread risk authority projection root is invalid")
	}
	store, err := finalauthorityadapter.OpenSecureMutableProjection(
		root,
		witnessSelectedProjectionName,
		maxThreadRiskAuthorityIndexBytes,
	)
	if err != nil {
		return nil, err
	}
	return &Projection{store: store}, nil
}

func (projection *Projection) ValidateWitnessEmpty(ctx context.Context) error {
	if projection == nil || projection.store == nil {
		return errors.New("thread risk authority projection is unavailable")
	}
	observed, err := projection.store.Observe(ctx)
	if err == nil {
		if observed.Present || observed.Digest != "" || observed.Body != nil {
			return ErrProjectionConflict
		}
		return nil
	}
	if !errors.Is(err, authorityprojectionport.ErrResidue) {
		return err
	}
	var residue *authorityprojectionport.ResidueError
	if !errors.As(err, &residue) || residue.Target.Present || residue.Target.Digest != "" || residue.Target.Body != nil {
		return errors.Join(ErrProjectionConflict, err)
	}
	result, reconcileErr := projection.store.ReconcileExact(ctx, "", "")
	if result.State != authorityprojectionport.Committed || reconcileErr != nil {
		return errors.Join(ErrProjectionIndeterminate, err, reconcileErr)
	}
	verified, verifyErr := projection.store.Observe(ctx)
	if verifyErr != nil || verified.Present || verified.Digest != "" || verified.Body != nil {
		return errors.Join(ErrProjectionIndeterminate, verifyErr)
	}
	return nil
}

func (projection *Projection) ProjectWitnessSelected(ctx context.Context, index domainsecurity.ThreadRiskAuthorityIndexV1) error {
	if projection == nil || projection.store == nil {
		return errors.New("thread risk authority projection is unavailable")
	}
	nextBody, err := domainsecurity.ThreadRiskAuthorityIndexV1Bytes(index)
	if err != nil || len(nextBody) == 0 || len(nextBody) > maxThreadRiskAuthorityIndexBytes {
		return errors.New("thread risk authority witness-selected index is invalid")
	}
	nextDigest := domainsecurity.SHA256Hex(nextBody)

	observed, observeErr := projection.store.Observe(ctx)
	if observeErr != nil {
		if !errors.Is(observeErr, authorityprojectionport.ErrResidue) {
			return observeErr
		}
		return projection.reconcileSelected(ctx, observed, nextDigest, nextBody, observeErr)
	}
	if observed.Present {
		if observed.Digest == nextDigest && bytes.Equal(observed.Body, nextBody) {
			return nil
		}
		prior, parseErr := parseProjectionIndex(observed)
		if parseErr != nil || !projectionCanAdvance(prior, index) {
			return errors.Join(ErrProjectionConflict, parseErr)
		}
	} else if observed.Digest != "" || observed.Body != nil {
		return ErrProjectionConflict
	}

	result, replaceErr := projection.store.ReplaceExact(ctx, observed.Digest, nextBody)
	switch result.State {
	case authorityprojectionport.Committed:
		if verifyErr := projection.verifySelected(ctx, nextDigest, nextBody); verifyErr != nil {
			return errors.Join(ErrProjectionIndeterminate, replaceErr, verifyErr)
		}
		if replaceErr != nil {
			return errors.Join(ErrProjectionIndeterminate, replaceErr)
		}
		return nil
	case authorityprojectionport.Indeterminate:
		return projection.reconcileFreshSelected(ctx, nextDigest, nextBody, replaceErr)
	case authorityprojectionport.NotCommitted:
		if errors.Is(replaceErr, authorityprojectionport.ErrResidue) {
			return projection.reconcileFreshSelected(ctx, nextDigest, nextBody, replaceErr)
		}
		if errors.Is(replaceErr, authorityprojectionport.ErrCompareFailed) {
			if verifyErr := projection.verifySelected(ctx, nextDigest, nextBody); verifyErr == nil {
				return nil
			}
		}
		return errors.Join(ErrProjectionConflict, replaceErr)
	default:
		return errors.Join(ErrProjectionIndeterminate, replaceErr)
	}
}

func (projection *Projection) reconcileFreshSelected(ctx context.Context, selectedDigest string, selectedBody []byte, priorErr error) error {
	observed, err := projection.store.Observe(ctx)
	if err == nil {
		if observed.Present && observed.Digest == selectedDigest && bytes.Equal(observed.Body, selectedBody) {
			return nil
		}
		return errors.Join(ErrProjectionConflict, priorErr)
	}
	if !errors.Is(err, authorityprojectionport.ErrResidue) {
		return errors.Join(ErrProjectionIndeterminate, priorErr, err)
	}
	return projection.reconcileSelected(ctx, observed, selectedDigest, selectedBody, errors.Join(priorErr, err))
}

func (projection *Projection) reconcileSelected(ctx context.Context, observed authorityprojectionport.Observation, selectedDigest string, selectedBody []byte, priorErr error) error {
	result, err := projection.store.ReconcileExact(ctx, observed.Digest, selectedDigest)
	if result.State != authorityprojectionport.Committed {
		return errors.Join(ErrProjectionIndeterminate, priorErr, err)
	}
	if verifyErr := projection.verifySelected(ctx, selectedDigest, selectedBody); verifyErr != nil {
		return errors.Join(ErrProjectionIndeterminate, priorErr, err, verifyErr)
	}
	if err != nil {
		return errors.Join(ErrProjectionIndeterminate, priorErr, err)
	}
	return nil
}

func (projection *Projection) verifySelected(ctx context.Context, selectedDigest string, selectedBody []byte) error {
	observed, err := projection.store.Observe(ctx)
	if err != nil {
		return err
	}
	if !observed.Present || observed.Digest != selectedDigest || !bytes.Equal(observed.Body, selectedBody) {
		return ErrProjectionIndeterminate
	}
	stored, err := parseProjectionIndex(observed)
	if err != nil {
		return err
	}
	canonical, err := domainsecurity.ThreadRiskAuthorityIndexV1Bytes(stored)
	if err != nil || !bytes.Equal(canonical, selectedBody) {
		return ErrProjectionIndeterminate
	}
	return nil
}

func parseProjectionIndex(observed authorityprojectionport.Observation) (domainsecurity.ThreadRiskAuthorityIndexV1, error) {
	if !observed.Present || observed.Digest == "" || !domainsecurity.IsSHA256Hex(observed.Digest) ||
		len(observed.Body) == 0 || len(observed.Body) > maxThreadRiskAuthorityIndexBytes ||
		domainsecurity.SHA256Hex(observed.Body) != observed.Digest {
		return domainsecurity.ThreadRiskAuthorityIndexV1{}, ErrProjectionConflict
	}
	index, err := domainsecurity.ParseThreadRiskAuthorityIndexV1(observed.Body)
	if err != nil {
		return domainsecurity.ThreadRiskAuthorityIndexV1{}, err
	}
	canonical, err := domainsecurity.ThreadRiskAuthorityIndexV1Bytes(index)
	if err != nil || !bytes.Equal(canonical, observed.Body) {
		return domainsecurity.ThreadRiskAuthorityIndexV1{}, ErrProjectionConflict
	}
	return index, nil
}

func projectionCanAdvance(previous, next domainsecurity.ThreadRiskAuthorityIndexV1) bool {
	return domainsecurity.ValidateThreadRiskAuthorityIndexTransitionV1(previous, next) == nil
}
