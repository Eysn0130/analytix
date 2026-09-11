package monotonicheadprojection

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityprojectionport "analytix.local/runtime-go/internal/ports/authorityprojection"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
)

const (
	checkpointFloorFileName = "witness-checkpoint-floor-v1.json"
	maxCheckpointFloorBytes = 64 << 10
)

type Config struct {
	Root              string
	InstallationID    string
	EnrollmentID      string
	Namespace         string
	WitnessKeyID      string
	WitnessPublicKey  []byte
	InitialCheckpoint domainsecurity.MonotonicHeadCheckpointV1
}

type Projection struct {
	store             authorityprojectionport.Store
	installationID    string
	enrollmentID      string
	namespace         string
	witnessKeyID      string
	witnessPublicKey  []byte
	initialCheckpoint domainsecurity.MonotonicHeadCheckpointV1
}

var _ monotonicheadport.CheckpointFloor = (*Projection)(nil)

func New(config Config) (*Projection, error) {
	witnessKey := append([]byte(nil), config.WitnessPublicKey...)
	if config.Root == "" || config.Root != strings.TrimSpace(config.Root) ||
		!domainsecurity.IsSHA256Hex(config.InstallationID) || !domainsecurity.IsSHA256Hex(config.EnrollmentID) ||
		strings.TrimSpace(config.Namespace) == "" || !domainsecurity.IsSHA256Hex(config.WitnessKeyID) ||
		len(witnessKey) != ed25519.PublicKeySize || config.WitnessKeyID != domainsecurity.SHA256Hex(witnessKey) ||
		config.InitialCheckpoint.Generation != 0 || validateCheckpoint(config.InitialCheckpoint, config.InstallationID,
		config.EnrollmentID, config.Namespace, config.WitnessKeyID, witnessKey) != nil {
		return nil, errors.New("monotonic checkpoint floor enrollment is invalid")
	}
	store, err := finalauthorityadapter.OpenSecureMutableProjection(config.Root, checkpointFloorFileName, maxCheckpointFloorBytes)
	if err != nil {
		return nil, errors.Join(monotonicheadport.ErrCheckpointFloorUnavailable, err)
	}
	return &Projection{
		store: store, installationID: config.InstallationID, enrollmentID: config.EnrollmentID,
		namespace: config.Namespace, witnessKeyID: config.WitnessKeyID, witnessPublicKey: witnessKey,
		initialCheckpoint: config.InitialCheckpoint,
	}, nil
}

func (projection *Projection) ProjectWitnessSelected(ctx context.Context, selected domainsecurity.MonotonicHeadCheckpointV1) error {
	if projection == nil || projection.store == nil || ctx == nil {
		return monotonicheadport.ErrCheckpointFloorUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := projection.validate(selected); err != nil {
		return errors.Join(monotonicheadport.ErrCheckpointFloorConflict, err)
	}
	selectedBody, err := domainsecurity.MonotonicHeadCheckpointV1Bytes(selected)
	if err != nil {
		return errors.Join(monotonicheadport.ErrCheckpointFloorConflict, err)
	}
	selectedDigest := domainsecurity.SHA256Hex(selectedBody)
	observed, observeErr := projection.store.Observe(ctx)
	if observeErr != nil {
		if !errors.Is(observeErr, authorityprojectionport.ErrResidue) {
			return errors.Join(monotonicheadport.ErrCheckpointFloorUnavailable, observeErr)
		}
		return projection.reconcileResidue(ctx, observed, selected, selectedBody, selectedDigest, observeErr)
	}
	if !observed.Present {
		if observed.Digest != "" || observed.Body != nil || !equalCheckpoint(selected, projection.initialCheckpoint) {
			return monotonicheadport.ErrCheckpointFloorBootstrap
		}
	} else {
		previous, parseErr := projection.parse(observed)
		if parseErr != nil {
			return errors.Join(monotonicheadport.ErrCheckpointFloorConflict, parseErr)
		}
		if bytes.Equal(observed.Body, selectedBody) && observed.Digest == selectedDigest {
			return nil
		}
		if domainsecurity.ValidateMonotonicHeadCheckpointDirectSuccessorV1(previous, selected) != nil {
			return monotonicheadport.ErrCheckpointFloorConflict
		}
	}
	result, replaceErr := projection.store.ReplaceExact(ctx, observed.Digest, selectedBody)
	return projection.finishMutation(ctx, result, replaceErr, selectedBody, selectedDigest)
}

func (projection *Projection) reconcileResidue(
	ctx context.Context,
	observed authorityprojectionport.Observation,
	selected domainsecurity.MonotonicHeadCheckpointV1,
	selectedBody []byte,
	selectedDigest string,
	priorErr error,
) error {
	if !observed.Present {
		if observed.Digest != "" || observed.Body != nil || !equalCheckpoint(selected, projection.initialCheckpoint) {
			return errors.Join(monotonicheadport.ErrCheckpointFloorBootstrap, priorErr)
		}
	} else {
		previous, err := projection.parse(observed)
		if err != nil || (!equalCheckpoint(previous, selected) &&
			domainsecurity.ValidateMonotonicHeadCheckpointDirectSuccessorV1(previous, selected) != nil) {
			return errors.Join(monotonicheadport.ErrCheckpointFloorConflict, priorErr, err)
		}
	}
	result, err := projection.store.ReconcileExact(ctx, observed.Digest, selectedDigest)
	if result.State != authorityprojectionport.Committed || err != nil {
		if errors.Is(err, authorityprojectionport.ErrCompareFailed) {
			exact, verifyErr := projection.exactSelected(ctx, selectedBody, selectedDigest)
			if verifyErr != nil {
				return errors.Join(monotonicheadport.ErrCheckpointFloorIndeterminate, priorErr, err, verifyErr)
			}
			if exact {
				return nil
			}
		}
		return errors.Join(monotonicheadport.ErrCheckpointFloorIndeterminate, priorErr, err)
	}
	return projection.verify(ctx, selectedBody, selectedDigest)
}

func (projection *Projection) finishMutation(
	ctx context.Context,
	result authorityprojectionport.ReplaceResult,
	mutationErr error,
	selectedBody []byte,
	selectedDigest string,
) error {
	switch result.State {
	case authorityprojectionport.Committed:
		if mutationErr != nil {
			return errors.Join(monotonicheadport.ErrCheckpointFloorIndeterminate, mutationErr)
		}
		if verifyErr := projection.verify(ctx, selectedBody, selectedDigest); verifyErr != nil {
			return errors.Join(monotonicheadport.ErrCheckpointFloorIndeterminate, mutationErr, verifyErr)
		}
		return nil
	case authorityprojectionport.Indeterminate:
		return errors.Join(monotonicheadport.ErrCheckpointFloorIndeterminate, mutationErr)
	case authorityprojectionport.NotCommitted:
		if errors.Is(mutationErr, authorityprojectionport.ErrCompareFailed) {
			exact, verifyErr := projection.exactSelected(ctx, selectedBody, selectedDigest)
			if verifyErr != nil {
				return errors.Join(monotonicheadport.ErrCheckpointFloorIndeterminate, mutationErr, verifyErr)
			}
			if exact {
				return nil
			}
			return errors.Join(monotonicheadport.ErrCheckpointFloorConflict, mutationErr)
		}
		return errors.Join(monotonicheadport.ErrCheckpointFloorIndeterminate, mutationErr)
	default:
		return monotonicheadport.ErrCheckpointFloorIndeterminate
	}
}

func (projection *Projection) verify(ctx context.Context, body []byte, digest string) error {
	exact, err := projection.exactSelected(ctx, body, digest)
	if err != nil || !exact {
		return errors.Join(monotonicheadport.ErrCheckpointFloorIndeterminate, err)
	}
	return nil
}

func (projection *Projection) exactSelected(ctx context.Context, body []byte, digest string) (bool, error) {
	observed, err := projection.store.Observe(ctx)
	if err != nil {
		return false, err
	}
	if !observed.Present || observed.Digest != digest || !bytes.Equal(observed.Body, body) {
		return false, nil
	}
	_, err = projection.parse(observed)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (projection *Projection) parse(observed authorityprojectionport.Observation) (domainsecurity.MonotonicHeadCheckpointV1, error) {
	if !observed.Present || !domainsecurity.IsSHA256Hex(observed.Digest) || len(observed.Body) == 0 ||
		len(observed.Body) > maxCheckpointFloorBytes || domainsecurity.SHA256Hex(observed.Body) != observed.Digest {
		return domainsecurity.MonotonicHeadCheckpointV1{}, errors.New("monotonic checkpoint floor bytes are invalid")
	}
	checkpoint, err := domainsecurity.ParseMonotonicHeadCheckpointV1(observed.Body)
	if err != nil || projection.validate(checkpoint) != nil {
		return domainsecurity.MonotonicHeadCheckpointV1{}, errors.New("monotonic checkpoint floor record is invalid")
	}
	canonical, err := domainsecurity.MonotonicHeadCheckpointV1Bytes(checkpoint)
	if err != nil || !bytes.Equal(canonical, observed.Body) {
		return domainsecurity.MonotonicHeadCheckpointV1{}, errors.New("monotonic checkpoint floor record is not canonical")
	}
	return checkpoint, nil
}

func (projection *Projection) validate(checkpoint domainsecurity.MonotonicHeadCheckpointV1) error {
	return validateCheckpoint(checkpoint, projection.installationID, projection.enrollmentID,
		projection.namespace, projection.witnessKeyID, projection.witnessPublicKey)
}

func validateCheckpoint(checkpoint domainsecurity.MonotonicHeadCheckpointV1, installationID, enrollmentID, namespace, witnessKeyID string, witnessKey []byte) error {
	if checkpoint.Namespace != namespace {
		return errors.New("monotonic checkpoint floor namespace mismatch")
	}
	return domainsecurity.ValidateMonotonicHeadCheckpointForWitnessV1(
		checkpoint, installationID, enrollmentID, witnessKeyID, witnessKey,
	)
}

func equalCheckpoint(left, right domainsecurity.MonotonicHeadCheckpointV1) bool {
	leftBody, leftErr := domainsecurity.MonotonicHeadCheckpointV1Bytes(left)
	rightBody, rightErr := domainsecurity.MonotonicHeadCheckpointV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}
