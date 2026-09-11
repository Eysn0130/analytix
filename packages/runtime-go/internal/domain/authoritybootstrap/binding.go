package authoritybootstrap

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"

	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// NewBootstrapBindingV1 binds authenticated observation pairs to an exact
// opaque current-manifest anchor. It deliberately has no witness signer and
// cannot mint a generation-zero witness checkpoint. A production caller must
// obtain both pairs from live witness operations; this pure constructor does
// not infer wall-clock freshness from their signatures.
func NewBootstrapBindingV1(
	input BootstrapBindingInputV1,
	anchoredManifest domainenrollment.AnchoredManifestV2,
	installationSign InstallationSignFuncV1,
) (BootstrapBindingV1, error) {
	installationPublicKey := append([]byte(nil), input.InstallationAuthorityPublicKey...)
	witnessPublicKey := append([]byte(nil), input.WitnessPublicKey...)
	manifestProjection, manifestErr := domainenrollment.ProjectAnchoredManifestForNamespaceV2(anchoredManifest, input.Namespace)
	if installationSign == nil || len(installationPublicKey) != ed25519.PublicKeySize ||
		len(witnessPublicKey) != ed25519.PublicKeySize ||
		input.InstallationAuthorityKeyID != domainsecurity.SHA256Hex(installationPublicKey) ||
		input.WitnessKeyID != domainsecurity.SHA256Hex(witnessPublicKey) ||
		input.InstallationAuthorityKeyID == input.WitnessKeyID ||
		domainsecurity.ValidateMonotonicHeadCheckpointForWitnessV1(
			input.EnrollmentInitialCheckpoint, input.InstallationID, input.EnrollmentID, input.WitnessKeyID, witnessPublicKey,
		) != nil || input.EnrollmentInitialCheckpoint.Generation != 0 || input.EnrollmentInitialCheckpoint.Namespace != input.Namespace ||
		validateManifestProjectionV2(input, manifestProjection, manifestErr) != nil {
		return BootstrapBindingV1{}, ErrInvalidContract
	}
	floorCheckpoint := input.FloorObservation.Checkpoint
	floorProjectionDigest, err := checkpointCanonicalDigestV1(floorCheckpoint)
	if err != nil {
		return BootstrapBindingV1{}, errors.Join(ErrInvalidContract, err)
	}
	projectionSlotID := projectionSlotIDV1(input.InstallationID, input.Namespace)
	bootstrapEnrollmentID := bootstrapEnrollmentIDV1(input.InstallationID, input.Namespace, projectionSlotID)
	if input.EnrollmentID == bootstrapEnrollmentID {
		return BootstrapBindingV1{}, ErrInvalidContract
	}
	binding := BootstrapBindingV1{
		SchemaVersion:                  SchemaVersionV1,
		Purpose:                        BindingPurposeV1,
		InstallationID:                 input.InstallationID,
		CurrentManifestDigest:          manifestProjection.ManifestDigest,
		ManifestEnrollmentDigest:       manifestEnrollmentDigestV1(manifestProjection.Enrollment),
		Namespace:                      input.Namespace,
		EnrollmentID:                   input.EnrollmentID,
		BootstrapEnrollmentID:          bootstrapEnrollmentID,
		WitnessKeyID:                   input.WitnessKeyID,
		WitnessPublicKey:               base64.RawURLEncoding.EncodeToString(witnessPublicKey),
		EnrollmentInitialCheckpoint:    input.EnrollmentInitialCheckpoint,
		FloorObserveRequest:            input.FloorObserveRequest,
		FloorObservation:               input.FloorObservation,
		FloorCheckpoint:                floorCheckpoint,
		FloorProjectionDigest:          floorProjectionDigest,
		ProjectionSlotID:               projectionSlotID,
		BootstrapInitialObserveRequest: input.BootstrapInitialObserveRequest,
		BootstrapInitialObservation:    input.BootstrapInitialObservation,
		BootstrapInitialCheckpoint:     input.BootstrapInitialObservation.Checkpoint,
		InstallationAuthorityAlgorithm: AlgorithmV1,
		InstallationAuthorityKeyID:     input.InstallationAuthorityKeyID,
		InstallationAuthorityPublicKey: base64.RawURLEncoding.EncodeToString(installationPublicKey),
	}
	if err := validateBindingUnsignedV1(binding); err != nil {
		return BootstrapBindingV1{}, err
	}
	signature, err := installationSign(BootstrapBindingSigningBytesV1(binding))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return BootstrapBindingV1{}, errors.Join(ErrInvalidContract, errors.New("authority bootstrap binding signing failed"))
	}
	binding.InstallationAuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	binding.BindingDigest = bindingDigestV1(binding)
	if err := ValidateBootstrapBindingV1(binding); err != nil {
		return BootstrapBindingV1{}, err
	}
	return binding, nil
}

// ValidateBootstrapBindingV1 verifies self-contained authenticity only. It
// does not trust the embedded installation key or manifest selection.
func ValidateBootstrapBindingV1(binding BootstrapBindingV1) error {
	if err := validateBindingUnsignedV1(binding); err != nil {
		return err
	}
	if err := verifySignedV1(
		binding.InstallationAuthorityAlgorithm,
		binding.InstallationAuthorityKeyID,
		binding.InstallationAuthorityPublicKey,
		binding.InstallationAuthoritySignature,
		BootstrapBindingSigningBytesV1(binding),
	); err != nil || !canonicalDigest(binding.BindingDigest) || binding.BindingDigest != bindingDigestV1(binding) {
		return errors.Join(ErrInvalidContract, errors.New("authority bootstrap binding signature or digest is invalid"), err)
	}
	return nil
}

// ValidateBootstrapBindingExactV1 is an equality validator only. Raw expected
// values are not a trust capability and this function never returns an
// AnchoredBootstrapBindingV1.
func ValidateBootstrapBindingExactV1(
	binding BootstrapBindingV1,
	expected BootstrapBindingInputV1,
	expectedManifestDigest string,
	expectedManifestEnrollmentDigest string,
) error {
	if err := ValidateBootstrapBindingV1(binding); err != nil {
		return err
	}
	installationPublicKey := append([]byte(nil), expected.InstallationAuthorityPublicKey...)
	witnessPublicKey := append([]byte(nil), expected.WitnessPublicKey...)
	expectedFloorDigest, floorErr := checkpointCanonicalDigestV1(expected.FloorObservation.Checkpoint)
	if floorErr != nil || !canonicalDigest(expectedManifestDigest) || !canonicalDigest(expectedManifestEnrollmentDigest) ||
		!canonicalDigest(expected.InstallationID) ||
		!validNamespaceV1(expected.Namespace) ||
		!canonicalDigest(expected.EnrollmentID) || !canonicalDigest(expected.WitnessKeyID) ||
		!canonicalDigest(expected.InstallationAuthorityKeyID) ||
		len(installationPublicKey) != ed25519.PublicKeySize || len(witnessPublicKey) != ed25519.PublicKeySize ||
		expected.InstallationAuthorityKeyID == expected.WitnessKeyID ||
		expected.InstallationAuthorityKeyID != domainsecurity.SHA256Hex(installationPublicKey) ||
		expected.WitnessKeyID != domainsecurity.SHA256Hex(witnessPublicKey) ||
		domainsecurity.ValidateMonotonicHeadCheckpointForWitnessV1(
			expected.EnrollmentInitialCheckpoint, expected.InstallationID, expected.EnrollmentID, expected.WitnessKeyID, witnessPublicKey,
		) != nil || expected.EnrollmentInitialCheckpoint.Generation != 0 || expected.EnrollmentInitialCheckpoint.Namespace != expected.Namespace ||
		domainsecurity.ValidateMonotonicHeadObservationForRequestV1(
			expected.FloorObservation,
			expected.FloorObserveRequest,
			expected.InstallationID,
			expected.InstallationAuthorityKeyID,
			installationPublicKey,
			expected.EnrollmentID,
			expected.WitnessKeyID,
			witnessPublicKey,
		) != nil ||
		binding.InstallationID != expected.InstallationID || binding.CurrentManifestDigest != expectedManifestDigest ||
		binding.ManifestEnrollmentDigest != expectedManifestEnrollmentDigest ||
		binding.Namespace != expected.Namespace ||
		binding.EnrollmentID != expected.EnrollmentID ||
		binding.WitnessKeyID != expected.WitnessKeyID ||
		binding.WitnessPublicKey != base64.RawURLEncoding.EncodeToString(witnessPublicKey) ||
		!equalCheckpointV1(binding.EnrollmentInitialCheckpoint, expected.EnrollmentInitialCheckpoint) ||
		!equalObserveRequestV1(binding.FloorObserveRequest, expected.FloorObserveRequest) ||
		!equalMonotonicObservationV1(binding.FloorObservation, expected.FloorObservation) ||
		!equalCheckpointV1(binding.FloorCheckpoint, expected.FloorObservation.Checkpoint) ||
		binding.FloorProjectionDigest != expectedFloorDigest ||
		!equalObserveRequestV1(binding.BootstrapInitialObserveRequest, expected.BootstrapInitialObserveRequest) ||
		!equalMonotonicObservationV1(binding.BootstrapInitialObservation, expected.BootstrapInitialObservation) ||
		!equalCheckpointV1(binding.BootstrapInitialCheckpoint, expected.BootstrapInitialObservation.Checkpoint) ||
		binding.InstallationAuthorityKeyID != expected.InstallationAuthorityKeyID ||
		binding.InstallationAuthorityPublicKey != base64.RawURLEncoding.EncodeToString(installationPublicKey) {
		return ErrAnchorMismatch
	}
	return nil
}

// AnchorBootstrapBindingV1 is the only constructor for the opaque bootstrap
// anchor. It projects manifest identity internally from AnchoredManifestV2;
// raw expected values alone can never mint the capability.
func AnchorBootstrapBindingV1(
	binding BootstrapBindingV1,
	anchoredManifest domainenrollment.AnchoredManifestV2,
	expected BootstrapBindingInputV1,
) (AnchoredBootstrapBindingV1, error) {
	manifestProjection, manifestErr := domainenrollment.ProjectAnchoredManifestForNamespaceV2(anchoredManifest, binding.Namespace)
	if manifestErr != nil {
		return AnchoredBootstrapBindingV1{}, errors.Join(ErrAnchorMismatch, manifestErr)
	}
	if err := ValidateBootstrapBindingExactV1(
		binding,
		expected,
		manifestProjection.ManifestDigest,
		manifestEnrollmentDigestV1(manifestProjection.Enrollment),
	); err != nil {
		return AnchoredBootstrapBindingV1{}, err
	}
	if validateManifestProjectionV2(expected, manifestProjection, nil) != nil ||
		binding.CurrentManifestDigest != manifestProjection.ManifestDigest ||
		binding.ManifestEnrollmentDigest != manifestEnrollmentDigestV1(manifestProjection.Enrollment) ||
		binding.InstallationID != manifestProjection.InstallationID ||
		binding.InstallationAuthorityAlgorithm != manifestProjection.InstallationAuthorityAlgorithm ||
		binding.InstallationAuthorityKeyID != manifestProjection.InstallationAuthorityKeyID ||
		binding.InstallationAuthorityPublicKey != base64.RawURLEncoding.EncodeToString(
			manifestProjection.InstallationAuthorityPublicKey,
		) {
		return AnchoredBootstrapBindingV1{}, ErrAnchorMismatch
	}
	return AnchoredBootstrapBindingV1{binding: binding}, nil
}

func (anchored AnchoredBootstrapBindingV1) BindingV1() (BootstrapBindingV1, error) {
	if ValidateBootstrapBindingV1(anchored.binding) != nil || anchored.binding.BindingDigest == "" {
		return BootstrapBindingV1{}, ErrAnchorMismatch
	}
	return anchored.binding, nil
}

func validateBindingUnsignedV1(binding BootstrapBindingV1) error {
	witnessPublicKey, witnessErr := decodeCanonicalPublicKeyV1(binding.WitnessPublicKey, binding.WitnessKeyID)
	installationPublicKey, installationErr := decodeCanonicalPublicKeyV1(
		binding.InstallationAuthorityPublicKey, binding.InstallationAuthorityKeyID,
	)
	floorDigest, floorErr := checkpointCanonicalDigestV1(binding.FloorCheckpoint)
	if witnessErr != nil || installationErr != nil || floorErr != nil || binding.SchemaVersion != SchemaVersionV1 || binding.Purpose != BindingPurposeV1 ||
		!canonicalDigest(binding.InstallationID) || !canonicalDigest(binding.CurrentManifestDigest) ||
		!canonicalDigest(binding.ManifestEnrollmentDigest) ||
		!validNamespaceV1(binding.Namespace) ||
		!canonicalDigest(binding.EnrollmentID) || !canonicalDigest(binding.BootstrapEnrollmentID) ||
		!canonicalDigest(binding.WitnessKeyID) || binding.ProjectionSlotID != projectionSlotIDV1(binding.InstallationID, binding.Namespace) ||
		binding.BootstrapEnrollmentID != bootstrapEnrollmentIDV1(binding.InstallationID, binding.Namespace, binding.ProjectionSlotID) ||
		binding.EnrollmentID == binding.BootstrapEnrollmentID ||
		binding.FloorProjectionDigest != floorDigest || binding.InstallationAuthorityAlgorithm != AlgorithmV1 ||
		!canonicalDigest(binding.InstallationAuthorityKeyID) || binding.InstallationAuthorityKeyID == binding.WitnessKeyID ||
		domainsecurity.ValidateMonotonicHeadCheckpointForWitnessV1(
			binding.EnrollmentInitialCheckpoint, binding.InstallationID, binding.EnrollmentID, binding.WitnessKeyID, witnessPublicKey,
		) != nil || binding.EnrollmentInitialCheckpoint.Generation != 0 || binding.EnrollmentInitialCheckpoint.Namespace != binding.Namespace ||
		domainsecurity.ValidateMonotonicHeadObservationForRequestV1(
			binding.FloorObservation,
			binding.FloorObserveRequest,
			binding.InstallationID,
			binding.InstallationAuthorityKeyID,
			installationPublicKey,
			binding.EnrollmentID,
			binding.WitnessKeyID,
			witnessPublicKey,
		) != nil || !equalCheckpointV1(binding.FloorCheckpoint, binding.FloorObservation.Checkpoint) ||
		(binding.FloorCheckpoint.Generation == 0 &&
			!equalCheckpointV1(binding.FloorCheckpoint, binding.EnrollmentInitialCheckpoint)) ||
		domainsecurity.ValidateMonotonicHeadObservationForRequestV1(
			binding.BootstrapInitialObservation,
			binding.BootstrapInitialObserveRequest,
			binding.InstallationID,
			binding.InstallationAuthorityKeyID,
			installationPublicKey,
			binding.BootstrapEnrollmentID,
			binding.WitnessKeyID,
			witnessPublicKey,
		) != nil || !equalCheckpointV1(binding.BootstrapInitialCheckpoint, binding.BootstrapInitialObservation.Checkpoint) ||
		binding.BootstrapInitialCheckpoint.Generation != 0 ||
		binding.BootstrapInitialCheckpoint.Namespace != binding.Namespace ||
		binding.BootstrapInitialCheckpoint.CurrentStateDigest != unpreparedStateDigestV1(binding) ||
		binding.BootstrapInitialCheckpoint.FenceNonce != bootstrapInitialFenceNonceV1(binding) {
		return ErrInvalidContract
	}
	return nil
}

func validateManifestProjectionV2(
	input BootstrapBindingInputV1,
	projection domainenrollment.ManifestAnchorProjectionV2,
	projectionErr error,
) error {
	enrollment := projection.Enrollment
	if projectionErr != nil || !canonicalDigest(projection.ManifestDigest) || projection.InstallationID != input.InstallationID ||
		projection.InstallationAuthorityAlgorithm != AlgorithmV1 ||
		projection.InstallationAuthorityKeyID != input.InstallationAuthorityKeyID ||
		!bytes.Equal(projection.InstallationAuthorityPublicKey, input.InstallationAuthorityPublicKey) ||
		enrollment.Namespace != input.Namespace || enrollment.EnrollmentID != input.EnrollmentID ||
		enrollment.WitnessAlgorithm != AlgorithmV1 || enrollment.WitnessKeyID != input.WitnessKeyID ||
		enrollment.WitnessPublicKey != base64.RawURLEncoding.EncodeToString(input.WitnessPublicKey) ||
		!equalCheckpointV1(enrollment.InitialCheckpoint, input.EnrollmentInitialCheckpoint) {
		return ErrAnchorMismatch
	}
	return nil
}
