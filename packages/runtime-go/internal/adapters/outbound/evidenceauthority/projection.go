package evidenceauthority

import (
	"bytes"
	"context"
	"errors"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityprojectionport "analytix.local/runtime-go/internal/ports/authorityprojection"
	storeport "analytix.local/runtime-go/internal/ports/evidenceauthority"
)

const evidenceWitnessProjectionName = "witness-selected-bundle.json"

var _ storeport.Projection = (*Projection)(nil)

var (
	ErrProjectionConflict      = errors.New("evidence authority projection conflicts with witnessed state")
	ErrProjectionIndeterminate = errors.New("evidence authority projection is indeterminate")
)

// Projection is a disposable local view of a bundle already selected by the
// application with a fresh witness challenge. It cannot read or select an
// authority head. A higher witnessed generation may legitimately skip local
// generations written by another process; a rollback or same-generation fork
// is rejected.
type Projection struct {
	store   authorityprojectionport.Store
	bundles storeport.BundleStore
}

func NewProjection(root string, bundleStores ...storeport.BundleStore) (*Projection, error) {
	if root == "" || root != strings.TrimSpace(root) || len(bundleStores) > 1 {
		return nil, errors.New("evidence authority projection root is invalid")
	}
	var bundles storeport.BundleStore
	if len(bundleStores) == 1 {
		bundles = bundleStores[0]
		if bundles == nil {
			return nil, errors.New("evidence authority projection bundle store is invalid")
		}
	}
	store, err := finalauthorityadapter.OpenSecureMutableProjection(
		root,
		evidenceWitnessProjectionName,
		maxEvidenceAuthorityBundleBytes,
	)
	if err != nil {
		return nil, err
	}
	return &Projection{store: store, bundles: bundles}, nil
}

func (projection *Projection) ValidateWitnessEmpty(ctx context.Context) error {
	if projection == nil || projection.store == nil {
		return errors.New("evidence authority projection is unavailable")
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

func (projection *Projection) ProjectWitnessSelected(ctx context.Context, bundle domainevidence.EvidenceAuthorityBundleV1) error {
	if projection == nil || projection.store == nil {
		return errors.New("evidence authority projection is unavailable")
	}
	nextBody, err := domainevidence.EvidenceAuthorityBundleV1Bytes(bundle)
	if err != nil || len(nextBody) == 0 || len(nextBody) > maxEvidenceAuthorityBundleBytes {
		return errors.New("evidence authority witness-selected bundle is invalid")
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
		prior, parseErr := parseProjectionBundle(observed)
		if parseErr != nil || !projection.canAdvance(ctx, prior, bundle) {
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
	stored, err := parseProjectionBundle(observed)
	if err != nil {
		return err
	}
	canonical, err := domainevidence.EvidenceAuthorityBundleV1Bytes(stored)
	if err != nil || !bytes.Equal(canonical, selectedBody) {
		return ErrProjectionIndeterminate
	}
	return nil
}

func parseProjectionBundle(observed authorityprojectionport.Observation) (domainevidence.EvidenceAuthorityBundleV1, error) {
	if !observed.Present || observed.Digest == "" || !domainsecurity.IsSHA256Hex(observed.Digest) ||
		len(observed.Body) == 0 || len(observed.Body) > maxEvidenceAuthorityBundleBytes ||
		domainsecurity.SHA256Hex(observed.Body) != observed.Digest {
		return domainevidence.EvidenceAuthorityBundleV1{}, ErrProjectionConflict
	}
	bundle, err := domainevidence.ParseEvidenceAuthorityBundleV1(observed.Body)
	if err != nil {
		return domainevidence.EvidenceAuthorityBundleV1{}, ErrProjectionConflict
	}
	canonical, err := domainevidence.EvidenceAuthorityBundleV1Bytes(bundle)
	if err != nil || !bytes.Equal(canonical, observed.Body) {
		return domainevidence.EvidenceAuthorityBundleV1{}, ErrProjectionConflict
	}
	return bundle, nil
}

func (projection *Projection) canAdvance(ctx context.Context, previous, next domainevidence.EvidenceAuthorityBundleV1) bool {
	if domainevidence.ValidateEvidenceAuthorityBundleV1(previous) != nil || domainevidence.ValidateEvidenceAuthorityBundleV1(next) != nil ||
		next.Generation <= previous.Generation || next.InstallationID != previous.InstallationID ||
		next.EnrollmentID != previous.EnrollmentID || next.Namespace != previous.Namespace ||
		next.AuthorityAlgorithm != previous.AuthorityAlgorithm || next.AuthorityKeyID != previous.AuthorityKeyID ||
		next.AuthorityPublicKey != previous.AuthorityPublicKey {
		return false
	}
	if next.Generation == previous.Generation+1 {
		return domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(previous, next) == nil
	}
	if projection == nil || projection.bundles == nil {
		return false
	}
	cursor := next
	for depth := 0; cursor.Generation > previous.Generation; depth++ {
		if depth >= maxProjectionBundleAncestryDepth || cursor.PreviousBundleDigest == "" {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		default:
		}
		ancestor, err := projection.bundles.Resolve(ctx, cursor.PreviousBundleDigest)
		if err != nil || ancestor.RecordDigest != cursor.PreviousBundleDigest ||
			domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(ancestor, cursor) != nil {
			return false
		}
		cursor = ancestor
	}
	return equalBundle(cursor, previous)
}

const maxProjectionBundleAncestryDepth = 100_000
