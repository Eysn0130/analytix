package security

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

const (
	MonotonicHeadCheckpointSchemaVersion = 1
	MonotonicHeadCheckpointPurpose       = "analytix.monotonic-head-checkpoint/v1"
	MonotonicHeadObserveRequestPurpose   = "analytix.monotonic-head-observe-request/v1"
	MonotonicHeadObservationPurpose      = "analytix.monotonic-head-observation/v1"
	MonotonicHeadAdvanceRequestPurpose   = "analytix.monotonic-head-advance-request/v1"
	MonotonicHeadAdvanceReceiptPurpose   = "analytix.monotonic-head-advance-receipt/v1"
	MonotonicHeadAlgorithm               = "Ed25519"
	EvidenceRegistryAuthorityNamespaceV1 = "analytix.evidence-registry-authority/v1"
)

var (
	monotonicHeadCheckpointSignatureDomain     = []byte("analytix.monotonic-head-checkpoint/v1\x00")
	monotonicHeadObserveRequestSignatureDomain = []byte("analytix.monotonic-head-observe-request/v1\x00")
	monotonicHeadObservationSignatureDomain    = []byte("analytix.monotonic-head-observation/v1\x00")
	monotonicHeadRequestSignatureDomain        = []byte("analytix.monotonic-head-advance-request/v1\x00")
	monotonicHeadReceiptSignatureDomain        = []byte("analytix.monotonic-head-advance-receipt/v1\x00")
)

// MonotonicHeadCheckpointV1 is an enrolled witness's signed view of one
// namespace head. A valid signature proves that this witness issued the
// checkpoint; it does not, in isolation, prove that the checkpoint is still
// the witness's latest value. Latest-head authority requires a fresh,
// installation-signed observe request and the enrolled witness's exact signed
// observation before validating the CAS transition below.
type MonotonicHeadCheckpointV1 struct {
	SchemaVersion            int    `json:"schemaVersion"`
	Purpose                  string `json:"purpose"`
	InstallationID           string `json:"installationId"`
	EnrollmentID             string `json:"enrollmentId"`
	Namespace                string `json:"namespace"`
	Generation               uint64 `json:"generation"`
	CurrentStateDigest       string `json:"currentStateDigest"`
	PreviousStateDigest      string `json:"previousStateDigest"`
	PreviousCheckpointDigest string `json:"previousCheckpointDigest"`
	FenceNonce               string `json:"fenceNonce"`
	MutationID               string `json:"mutationId"`
	WitnessAlgorithm         string `json:"witnessAlgorithm"`
	WitnessKeyID             string `json:"witnessKeyId"`
	WitnessPublicKey         string `json:"witnessPublicKey"`
	WitnessSignature         string `json:"witnessSignature"`
	CheckpointDigest         string `json:"checkpointDigest"`
}

type MonotonicHeadCheckpointInputV1 struct {
	InstallationID           string
	EnrollmentID             string
	Namespace                string
	Generation               uint64
	CurrentStateDigest       string
	PreviousStateDigest      string
	PreviousCheckpointDigest string
	FenceNonce               string
	MutationID               string
	WitnessKeyID             string
	WitnessPublicKey         []byte
}

// MonotonicHeadObserveRequestV1 authenticates a fresh caller challenge to the
// installation and enrolled namespace before a witness reveals or signs its
// current head.
type MonotonicHeadObserveRequestV1 struct {
	SchemaVersion      int    `json:"schemaVersion"`
	Purpose            string `json:"purpose"`
	InstallationID     string `json:"installationId"`
	EnrollmentID       string `json:"enrollmentId"`
	Namespace          string `json:"namespace"`
	ChallengeNonce     string `json:"challengeNonce"`
	AuthorityAlgorithm string `json:"authorityAlgorithm"`
	AuthorityKeyID     string `json:"authorityKeyId"`
	AuthorityPublicKey string `json:"authorityPublicKey"`
	AuthoritySignature string `json:"authoritySignature"`
	RequestDigest      string `json:"requestDigest"`
}

type MonotonicHeadObserveRequestInputV1 struct {
	InstallationID     string
	EnrollmentID       string
	Namespace          string
	ChallengeNonce     string
	AuthorityKeyID     string
	AuthorityPublicKey []byte
}

type MonotonicHeadAdvanceRequestV1 struct {
	SchemaVersion            int    `json:"schemaVersion"`
	Purpose                  string `json:"purpose"`
	InstallationID           string `json:"installationId"`
	EnrollmentID             string `json:"enrollmentId"`
	Namespace                string `json:"namespace"`
	ExpectedGeneration       uint64 `json:"expectedGeneration"`
	ExpectedCheckpointDigest string `json:"expectedCheckpointDigest"`
	ExpectedStateDigest      string `json:"expectedStateDigest"`
	NextGeneration           uint64 `json:"nextGeneration"`
	NextStateDigest          string `json:"nextStateDigest"`
	ExpectedFenceNonce       string `json:"expectedFenceNonce"`
	MutationID               string `json:"mutationId"`
	AuthorityAlgorithm       string `json:"authorityAlgorithm"`
	AuthorityKeyID           string `json:"authorityKeyId"`
	AuthorityPublicKey       string `json:"authorityPublicKey"`
	AuthoritySignature       string `json:"authoritySignature"`
	RequestDigest            string `json:"requestDigest"`
}

type MonotonicHeadAdvanceRequestInputV1 struct {
	InstallationID           string
	EnrollmentID             string
	Namespace                string
	ExpectedGeneration       uint64
	ExpectedCheckpointDigest string
	ExpectedStateDigest      string
	NextGeneration           uint64
	NextStateDigest          string
	ExpectedFenceNonce       string
	MutationID               string
	AuthorityKeyID           string
	AuthorityPublicKey       []byte
}

// MonotonicHeadAdvanceReceiptV1 binds the exact request digest to the exact
// witness checkpoint. An idempotent replay must return byte-identical
// canonical receipt bytes, not merely another valid signature over the same
// state.
type MonotonicHeadAdvanceReceiptV1 struct {
	SchemaVersion    int                       `json:"schemaVersion"`
	Purpose          string                    `json:"purpose"`
	RequestDigest    string                    `json:"requestDigest"`
	MutationID       string                    `json:"mutationId"`
	Checkpoint       MonotonicHeadCheckpointV1 `json:"checkpoint"`
	WitnessAlgorithm string                    `json:"witnessAlgorithm"`
	WitnessKeyID     string                    `json:"witnessKeyId"`
	WitnessPublicKey string                    `json:"witnessPublicKey"`
	WitnessSignature string                    `json:"witnessSignature"`
	ReceiptDigest    string                    `json:"receiptDigest"`
}

// MonotonicHeadObservationV1 proves that the enrolled witness answered a
// caller-selected fresh challenge with the exact stable checkpoint. Observe
// does not mutate checkpoint generation, digest, or fence nonce. Callers must
// compare ChallengeNonce to their unpredictable request; an old signed
// observation is otherwise authentic but replayable.
type MonotonicHeadObservationV1 struct {
	SchemaVersion     int                       `json:"schemaVersion"`
	Purpose           string                    `json:"purpose"`
	RequestDigest     string                    `json:"requestDigest"`
	ChallengeNonce    string                    `json:"challengeNonce"`
	Checkpoint        MonotonicHeadCheckpointV1 `json:"checkpoint"`
	WitnessAlgorithm  string                    `json:"witnessAlgorithm"`
	WitnessKeyID      string                    `json:"witnessKeyId"`
	WitnessPublicKey  string                    `json:"witnessPublicKey"`
	WitnessSignature  string                    `json:"witnessSignature"`
	ObservationDigest string                    `json:"observationDigest"`
}

type MonotonicHeadSignFunc func([]byte) ([]byte, error)

func NewMonotonicHeadCheckpointV1(input MonotonicHeadCheckpointInputV1, sign MonotonicHeadSignFunc) (MonotonicHeadCheckpointV1, error) {
	publicKey := append([]byte(nil), input.WitnessPublicKey...)
	checkpoint := MonotonicHeadCheckpointV1{
		SchemaVersion:            MonotonicHeadCheckpointSchemaVersion,
		Purpose:                  MonotonicHeadCheckpointPurpose,
		InstallationID:           strings.TrimSpace(input.InstallationID),
		EnrollmentID:             strings.TrimSpace(input.EnrollmentID),
		Namespace:                strings.TrimSpace(input.Namespace),
		Generation:               input.Generation,
		CurrentStateDigest:       strings.TrimSpace(input.CurrentStateDigest),
		PreviousStateDigest:      strings.TrimSpace(input.PreviousStateDigest),
		PreviousCheckpointDigest: strings.TrimSpace(input.PreviousCheckpointDigest),
		FenceNonce:               strings.TrimSpace(input.FenceNonce),
		MutationID:               strings.TrimSpace(input.MutationID),
		WitnessAlgorithm:         MonotonicHeadAlgorithm,
		WitnessKeyID:             strings.TrimSpace(input.WitnessKeyID),
		WitnessPublicKey:         base64.RawURLEncoding.EncodeToString(publicKey),
	}
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || checkpoint.WitnessKeyID != SHA256Hex(publicKey) {
		return MonotonicHeadCheckpointV1{}, errors.New("monotonic head witness authority is invalid")
	}
	if err := validateMonotonicHeadCheckpointUnsignedV1(checkpoint); err != nil {
		return MonotonicHeadCheckpointV1{}, err
	}
	signature, err := sign(MonotonicHeadCheckpointSigningBytesV1(checkpoint))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return MonotonicHeadCheckpointV1{}, errors.New("monotonic head checkpoint signing failed")
	}
	checkpoint.WitnessSignature = base64.RawURLEncoding.EncodeToString(signature)
	checkpoint.CheckpointDigest = monotonicHeadCheckpointDigestV1(checkpoint)
	if err := ValidateMonotonicHeadCheckpointV1(checkpoint); err != nil {
		return MonotonicHeadCheckpointV1{}, err
	}
	return checkpoint, nil
}

func ValidateMonotonicHeadCheckpointV1(checkpoint MonotonicHeadCheckpointV1) error {
	if err := validateMonotonicHeadCheckpointUnsignedV1(checkpoint); err != nil {
		return err
	}
	if !isCanonicalSHA256Hex(checkpoint.CheckpointDigest) || checkpoint.CheckpointDigest != monotonicHeadCheckpointDigestV1(checkpoint) {
		return errors.New("monotonic head checkpoint digest is invalid")
	}
	if err := verifyMonotonicHeadSignature(
		checkpoint.WitnessAlgorithm, checkpoint.WitnessKeyID, checkpoint.WitnessPublicKey,
		checkpoint.WitnessSignature, MonotonicHeadCheckpointSigningBytesV1(checkpoint),
	); err != nil {
		return errors.New("monotonic head checkpoint signature is invalid")
	}
	return nil
}

func ValidateMonotonicHeadCheckpointForWitnessV1(checkpoint MonotonicHeadCheckpointV1, installationID, enrollmentID, witnessKeyID string, witnessPublicKey []byte) error {
	if err := ValidateMonotonicHeadCheckpointV1(checkpoint); err != nil {
		return err
	}
	witnessPublicKey = append([]byte(nil), witnessPublicKey...)
	if checkpoint.InstallationID != strings.TrimSpace(installationID) || checkpoint.EnrollmentID != strings.TrimSpace(enrollmentID) ||
		len(witnessPublicKey) != ed25519.PublicKeySize || strings.TrimSpace(witnessKeyID) != SHA256Hex(witnessPublicKey) ||
		checkpoint.WitnessKeyID != strings.TrimSpace(witnessKeyID) ||
		checkpoint.WitnessPublicKey != base64.RawURLEncoding.EncodeToString(witnessPublicKey) {
		return errors.New("monotonic head witness enrollment mismatch")
	}
	return nil
}

func NewMonotonicHeadObserveRequestV1(input MonotonicHeadObserveRequestInputV1, sign MonotonicHeadSignFunc) (MonotonicHeadObserveRequestV1, error) {
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	request := MonotonicHeadObserveRequestV1{
		SchemaVersion:      MonotonicHeadCheckpointSchemaVersion,
		Purpose:            MonotonicHeadObserveRequestPurpose,
		InstallationID:     strings.TrimSpace(input.InstallationID),
		EnrollmentID:       strings.TrimSpace(input.EnrollmentID),
		Namespace:          strings.TrimSpace(input.Namespace),
		ChallengeNonce:     strings.TrimSpace(input.ChallengeNonce),
		AuthorityAlgorithm: MonotonicHeadAlgorithm,
		AuthorityKeyID:     strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || request.AuthorityKeyID != SHA256Hex(publicKey) ||
		validateMonotonicHeadObserveRequestUnsignedV1(request) != nil {
		return MonotonicHeadObserveRequestV1{}, errors.New("monotonic head observe request input is invalid")
	}
	signature, err := sign(MonotonicHeadObserveRequestSigningBytesV1(request))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return MonotonicHeadObserveRequestV1{}, errors.New("monotonic head observe request signing failed")
	}
	request.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	request.RequestDigest = monotonicHeadObserveRequestDigestV1(request)
	if err := ValidateMonotonicHeadObserveRequestV1(request); err != nil {
		return MonotonicHeadObserveRequestV1{}, err
	}
	return request, nil
}

func ValidateMonotonicHeadObserveRequestV1(request MonotonicHeadObserveRequestV1) error {
	if err := validateMonotonicHeadObserveRequestUnsignedV1(request); err != nil {
		return err
	}
	if !isCanonicalSHA256Hex(request.RequestDigest) || request.RequestDigest != monotonicHeadObserveRequestDigestV1(request) {
		return errors.New("monotonic head observe request digest is invalid")
	}
	if err := verifyMonotonicHeadSignature(
		request.AuthorityAlgorithm, request.AuthorityKeyID, request.AuthorityPublicKey,
		request.AuthoritySignature, MonotonicHeadObserveRequestSigningBytesV1(request),
	); err != nil {
		return errors.New("monotonic head observe request signature is invalid")
	}
	return nil
}

func ValidateMonotonicHeadObserveRequestForInstallationV1(request MonotonicHeadObserveRequestV1, installationID, authorityKeyID string, authorityPublicKey []byte) error {
	if err := ValidateMonotonicHeadObserveRequestV1(request); err != nil {
		return err
	}
	authorityPublicKey = append([]byte(nil), authorityPublicKey...)
	if request.InstallationID != strings.TrimSpace(installationID) || len(authorityPublicKey) != ed25519.PublicKeySize ||
		strings.TrimSpace(authorityKeyID) != SHA256Hex(authorityPublicKey) || request.AuthorityKeyID != strings.TrimSpace(authorityKeyID) ||
		request.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(authorityPublicKey) {
		return errors.New("monotonic head observe request installation authority mismatch")
	}
	return nil
}

func NewMonotonicHeadObservationV1(request MonotonicHeadObserveRequestV1, checkpoint MonotonicHeadCheckpointV1, sign MonotonicHeadSignFunc) (MonotonicHeadObservationV1, error) {
	if ValidateMonotonicHeadObserveRequestV1(request) != nil || ValidateMonotonicHeadCheckpointV1(checkpoint) != nil || sign == nil ||
		request.InstallationID != checkpoint.InstallationID || request.EnrollmentID != checkpoint.EnrollmentID ||
		request.Namespace != checkpoint.Namespace {
		return MonotonicHeadObservationV1{}, errors.New("monotonic head observation input is invalid")
	}
	observation := MonotonicHeadObservationV1{
		SchemaVersion:    MonotonicHeadCheckpointSchemaVersion,
		Purpose:          MonotonicHeadObservationPurpose,
		RequestDigest:    request.RequestDigest,
		ChallengeNonce:   request.ChallengeNonce,
		Checkpoint:       checkpoint,
		WitnessAlgorithm: MonotonicHeadAlgorithm,
		WitnessKeyID:     checkpoint.WitnessKeyID,
		WitnessPublicKey: checkpoint.WitnessPublicKey,
	}
	signature, err := sign(MonotonicHeadObservationSigningBytesV1(observation))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return MonotonicHeadObservationV1{}, errors.New("monotonic head observation signing failed")
	}
	observation.WitnessSignature = base64.RawURLEncoding.EncodeToString(signature)
	observation.ObservationDigest = monotonicHeadObservationDigestV1(observation)
	if err := ValidateMonotonicHeadObservationV1(observation); err != nil {
		return MonotonicHeadObservationV1{}, err
	}
	return observation, nil
}

func ValidateMonotonicHeadObservationV1(observation MonotonicHeadObservationV1) error {
	if observation.SchemaVersion != MonotonicHeadCheckpointSchemaVersion || observation.Purpose != MonotonicHeadObservationPurpose ||
		!isCanonicalSHA256Hex(observation.RequestDigest) || !isCanonicalSHA256Hex(observation.ChallengeNonce) ||
		ValidateMonotonicHeadCheckpointV1(observation.Checkpoint) != nil ||
		observation.WitnessAlgorithm != MonotonicHeadAlgorithm || observation.WitnessKeyID != observation.Checkpoint.WitnessKeyID ||
		observation.WitnessPublicKey != observation.Checkpoint.WitnessPublicKey ||
		!isCanonicalSHA256Hex(observation.ObservationDigest) || observation.ObservationDigest != monotonicHeadObservationDigestV1(observation) {
		return errors.New("monotonic head observation integrity is invalid")
	}
	if err := verifyMonotonicHeadSignature(
		observation.WitnessAlgorithm, observation.WitnessKeyID, observation.WitnessPublicKey,
		observation.WitnessSignature, MonotonicHeadObservationSigningBytesV1(observation),
	); err != nil {
		return errors.New("monotonic head observation signature is invalid")
	}
	return nil
}

func ValidateMonotonicHeadObservationForRequestV1(
	observation MonotonicHeadObservationV1,
	request MonotonicHeadObserveRequestV1,
	installationID, authorityKeyID string,
	authorityPublicKey []byte,
	enrollmentID, witnessKeyID string,
	witnessPublicKey []byte,
) error {
	if err := ValidateMonotonicHeadObservationV1(observation); err != nil {
		return err
	}
	if err := ValidateMonotonicHeadObserveRequestForInstallationV1(request, installationID, authorityKeyID, authorityPublicKey); err != nil {
		return err
	}
	if observation.RequestDigest != request.RequestDigest || observation.ChallengeNonce != request.ChallengeNonce ||
		observation.Checkpoint.InstallationID != request.InstallationID || observation.Checkpoint.EnrollmentID != request.EnrollmentID ||
		observation.Checkpoint.Namespace != request.Namespace {
		return errors.New("monotonic head observation request binding mismatch")
	}
	return ValidateMonotonicHeadCheckpointForWitnessV1(
		observation.Checkpoint, installationID, enrollmentID, witnessKeyID, witnessPublicKey,
	)
}

func NewMonotonicHeadAdvanceRequestV1(input MonotonicHeadAdvanceRequestInputV1, sign MonotonicHeadSignFunc) (MonotonicHeadAdvanceRequestV1, error) {
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	request := MonotonicHeadAdvanceRequestV1{
		SchemaVersion:            MonotonicHeadCheckpointSchemaVersion,
		Purpose:                  MonotonicHeadAdvanceRequestPurpose,
		InstallationID:           strings.TrimSpace(input.InstallationID),
		EnrollmentID:             strings.TrimSpace(input.EnrollmentID),
		Namespace:                strings.TrimSpace(input.Namespace),
		ExpectedGeneration:       input.ExpectedGeneration,
		ExpectedCheckpointDigest: strings.TrimSpace(input.ExpectedCheckpointDigest),
		ExpectedStateDigest:      strings.TrimSpace(input.ExpectedStateDigest),
		NextGeneration:           input.NextGeneration,
		NextStateDigest:          strings.TrimSpace(input.NextStateDigest),
		ExpectedFenceNonce:       strings.TrimSpace(input.ExpectedFenceNonce),
		MutationID:               strings.TrimSpace(input.MutationID),
		AuthorityAlgorithm:       MonotonicHeadAlgorithm,
		AuthorityKeyID:           strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey:       base64.RawURLEncoding.EncodeToString(publicKey),
	}
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || request.AuthorityKeyID != SHA256Hex(publicKey) {
		return MonotonicHeadAdvanceRequestV1{}, errors.New("monotonic head request authority is invalid")
	}
	if err := validateMonotonicHeadAdvanceRequestUnsignedV1(request); err != nil {
		return MonotonicHeadAdvanceRequestV1{}, err
	}
	signature, err := sign(MonotonicHeadAdvanceRequestSigningBytesV1(request))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return MonotonicHeadAdvanceRequestV1{}, errors.New("monotonic head request signing failed")
	}
	request.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	request.RequestDigest = monotonicHeadAdvanceRequestDigestV1(request)
	if err := ValidateMonotonicHeadAdvanceRequestV1(request); err != nil {
		return MonotonicHeadAdvanceRequestV1{}, err
	}
	return request, nil
}

func ValidateMonotonicHeadAdvanceRequestV1(request MonotonicHeadAdvanceRequestV1) error {
	if err := validateMonotonicHeadAdvanceRequestUnsignedV1(request); err != nil {
		return err
	}
	if !isCanonicalSHA256Hex(request.RequestDigest) || request.RequestDigest != monotonicHeadAdvanceRequestDigestV1(request) {
		return errors.New("monotonic head request digest is invalid")
	}
	if err := verifyMonotonicHeadSignature(
		request.AuthorityAlgorithm, request.AuthorityKeyID, request.AuthorityPublicKey,
		request.AuthoritySignature, MonotonicHeadAdvanceRequestSigningBytesV1(request),
	); err != nil {
		return errors.New("monotonic head request signature is invalid")
	}
	return nil
}

func ValidateMonotonicHeadAdvanceRequestForInstallationV1(request MonotonicHeadAdvanceRequestV1, installationID, authorityKeyID string, authorityPublicKey []byte) error {
	if err := ValidateMonotonicHeadAdvanceRequestV1(request); err != nil {
		return err
	}
	authorityPublicKey = append([]byte(nil), authorityPublicKey...)
	if request.InstallationID != strings.TrimSpace(installationID) || len(authorityPublicKey) != ed25519.PublicKeySize ||
		strings.TrimSpace(authorityKeyID) != SHA256Hex(authorityPublicKey) || request.AuthorityKeyID != strings.TrimSpace(authorityKeyID) ||
		request.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(authorityPublicKey) {
		return errors.New("monotonic head request installation authority mismatch")
	}
	return nil
}

func NewMonotonicHeadAdvanceReceiptV1(request MonotonicHeadAdvanceRequestV1, checkpoint MonotonicHeadCheckpointV1, sign MonotonicHeadSignFunc) (MonotonicHeadAdvanceReceiptV1, error) {
	if ValidateMonotonicHeadAdvanceRequestV1(request) != nil || ValidateMonotonicHeadCheckpointV1(checkpoint) != nil ||
		checkpoint.InstallationID != request.InstallationID || checkpoint.EnrollmentID != request.EnrollmentID ||
		checkpoint.Namespace != request.Namespace || checkpoint.Generation != request.NextGeneration ||
		checkpoint.CurrentStateDigest != request.NextStateDigest || checkpoint.PreviousStateDigest != request.ExpectedStateDigest ||
		checkpoint.MutationID != request.MutationID {
		return MonotonicHeadAdvanceReceiptV1{}, errors.New("monotonic head receipt input is invalid")
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(checkpoint.WitnessPublicKey)
	if err != nil || sign == nil {
		return MonotonicHeadAdvanceReceiptV1{}, errors.New("monotonic head receipt witness is invalid")
	}
	receipt := MonotonicHeadAdvanceReceiptV1{
		SchemaVersion:    MonotonicHeadCheckpointSchemaVersion,
		Purpose:          MonotonicHeadAdvanceReceiptPurpose,
		RequestDigest:    request.RequestDigest,
		MutationID:       request.MutationID,
		Checkpoint:       checkpoint,
		WitnessAlgorithm: MonotonicHeadAlgorithm,
		WitnessKeyID:     checkpoint.WitnessKeyID,
		WitnessPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	signature, signErr := sign(MonotonicHeadAdvanceReceiptSigningBytesV1(receipt))
	if signErr != nil || len(signature) != ed25519.SignatureSize {
		return MonotonicHeadAdvanceReceiptV1{}, errors.New("monotonic head receipt signing failed")
	}
	receipt.WitnessSignature = base64.RawURLEncoding.EncodeToString(signature)
	receipt.ReceiptDigest = monotonicHeadAdvanceReceiptDigestV1(receipt)
	if err := ValidateMonotonicHeadAdvanceReceiptV1(receipt); err != nil {
		return MonotonicHeadAdvanceReceiptV1{}, err
	}
	return receipt, nil
}

func ValidateMonotonicHeadAdvanceReceiptV1(receipt MonotonicHeadAdvanceReceiptV1) error {
	if receipt.SchemaVersion != MonotonicHeadCheckpointSchemaVersion || receipt.Purpose != MonotonicHeadAdvanceReceiptPurpose ||
		!isCanonicalSHA256Hex(receipt.RequestDigest) || !isCanonicalSHA256Hex(receipt.MutationID) ||
		receipt.MutationID != receipt.Checkpoint.MutationID || ValidateMonotonicHeadCheckpointV1(receipt.Checkpoint) != nil ||
		receipt.WitnessAlgorithm != MonotonicHeadAlgorithm || receipt.WitnessKeyID != receipt.Checkpoint.WitnessKeyID ||
		receipt.WitnessPublicKey != receipt.Checkpoint.WitnessPublicKey ||
		!isCanonicalSHA256Hex(receipt.ReceiptDigest) || receipt.ReceiptDigest != monotonicHeadAdvanceReceiptDigestV1(receipt) {
		return errors.New("monotonic head receipt integrity is invalid")
	}
	if err := verifyMonotonicHeadSignature(
		receipt.WitnessAlgorithm, receipt.WitnessKeyID, receipt.WitnessPublicKey,
		receipt.WitnessSignature, MonotonicHeadAdvanceReceiptSigningBytesV1(receipt),
	); err != nil {
		return errors.New("monotonic head receipt signature is invalid")
	}
	return nil
}

// ValidateMonotonicHeadAdvanceV1 validates exact compare-and-swap semantics:
// the request compares generation, checkpoint digest, state digest, and fence
// nonce against previous; the returned checkpoint advances exactly one
// generation and extends both the checkpoint and state chains.
func ValidateMonotonicHeadAdvanceV1(previous MonotonicHeadCheckpointV1, request MonotonicHeadAdvanceRequestV1, receipt MonotonicHeadAdvanceReceiptV1) error {
	if ValidateMonotonicHeadCheckpointV1(previous) != nil || ValidateMonotonicHeadAdvanceRequestV1(request) != nil ||
		ValidateMonotonicHeadAdvanceReceiptV1(receipt) != nil {
		return errors.New("monotonic head advance authenticity is invalid")
	}
	if request.InstallationID != previous.InstallationID || request.EnrollmentID != previous.EnrollmentID ||
		request.Namespace != previous.Namespace || request.ExpectedGeneration != previous.Generation ||
		request.ExpectedCheckpointDigest != previous.CheckpointDigest || request.ExpectedStateDigest != previous.CurrentStateDigest ||
		request.ExpectedFenceNonce != previous.FenceNonce || request.NextGeneration != previous.Generation+1 ||
		request.MutationID == previous.MutationID ||
		receipt.RequestDigest != request.RequestDigest || receipt.MutationID != request.MutationID {
		return errors.New("monotonic head compare-and-swap precondition failed")
	}
	next := receipt.Checkpoint
	if previous.Generation == ^uint64(0) || next.InstallationID != previous.InstallationID || next.EnrollmentID != previous.EnrollmentID ||
		next.Namespace != previous.Namespace || next.Generation != request.NextGeneration ||
		next.PreviousCheckpointDigest != previous.CheckpointDigest || next.PreviousStateDigest != previous.CurrentStateDigest ||
		next.CurrentStateDigest != request.NextStateDigest || next.MutationID != request.MutationID ||
		next.FenceNonce == previous.FenceNonce || next.WitnessKeyID != previous.WitnessKeyID ||
		next.WitnessPublicKey != previous.WitnessPublicKey || receipt.WitnessKeyID != previous.WitnessKeyID ||
		receipt.WitnessPublicKey != previous.WitnessPublicKey {
		return errors.New("monotonic head checkpoint does not extend previous head")
	}
	return nil
}

func ValidateMonotonicHeadAdvanceForAuthoritiesV1(
	previous MonotonicHeadCheckpointV1,
	request MonotonicHeadAdvanceRequestV1,
	receipt MonotonicHeadAdvanceReceiptV1,
	installationID, authorityKeyID string,
	authorityPublicKey []byte,
	enrollmentID, witnessKeyID string,
	witnessPublicKey []byte,
) error {
	if err := ValidateMonotonicHeadAdvanceV1(previous, request, receipt); err != nil {
		return err
	}
	if err := ValidateMonotonicHeadAdvanceRequestForInstallationV1(request, installationID, authorityKeyID, authorityPublicKey); err != nil {
		return err
	}
	if err := ValidateMonotonicHeadCheckpointForWitnessV1(previous, installationID, enrollmentID, witnessKeyID, witnessPublicKey); err != nil {
		return err
	}
	if err := ValidateMonotonicHeadCheckpointForWitnessV1(receipt.Checkpoint, installationID, enrollmentID, witnessKeyID, witnessPublicKey); err != nil {
		return err
	}
	return nil
}

// ValidateMonotonicHeadIdempotentReplayV1 enforces exact idempotency. Reusing
// a mutation ID with different request bytes is a conflict; replaying the same
// request is valid only when the witness returns the exact canonical receipt.
func ValidateMonotonicHeadIdempotentReplayV1(
	committedRequest MonotonicHeadAdvanceRequestV1,
	committedReceipt MonotonicHeadAdvanceReceiptV1,
	replayRequest MonotonicHeadAdvanceRequestV1,
	replayReceipt MonotonicHeadAdvanceReceiptV1,
) error {
	if ValidateMonotonicHeadAdvanceRequestV1(committedRequest) != nil || ValidateMonotonicHeadAdvanceReceiptV1(committedReceipt) != nil ||
		ValidateMonotonicHeadAdvanceRequestV1(replayRequest) != nil || ValidateMonotonicHeadAdvanceReceiptV1(replayReceipt) != nil {
		return errors.New("monotonic head replay contract is invalid")
	}
	if committedReceipt.RequestDigest != committedRequest.RequestDigest || committedReceipt.MutationID != committedRequest.MutationID ||
		replayReceipt.RequestDigest != replayRequest.RequestDigest || replayReceipt.MutationID != replayRequest.MutationID {
		return errors.New("monotonic head replay receipt is not bound to request")
	}
	if replayRequest.MutationID != committedRequest.MutationID {
		return errors.New("monotonic head replay mutation ID does not match")
	}
	committedRequestBytes, _ := MonotonicHeadAdvanceRequestV1Bytes(committedRequest)
	replayRequestBytes, _ := MonotonicHeadAdvanceRequestV1Bytes(replayRequest)
	if !bytes.Equal(committedRequestBytes, replayRequestBytes) {
		return errors.New("monotonic head mutation ID was reused for a different request")
	}
	committedReceiptBytes, _ := MonotonicHeadAdvanceReceiptV1Bytes(committedReceipt)
	replayReceiptBytes, _ := MonotonicHeadAdvanceReceiptV1Bytes(replayReceipt)
	if !bytes.Equal(committedReceiptBytes, replayReceiptBytes) {
		return errors.New("monotonic head idempotent replay receipt is not exact")
	}
	return nil
}

func ValidateMonotonicHeadCheckpointDirectSuccessorV1(previous, next MonotonicHeadCheckpointV1) error {
	if ValidateMonotonicHeadCheckpointV1(previous) != nil || ValidateMonotonicHeadCheckpointV1(next) != nil ||
		previous.InstallationID != next.InstallationID || previous.EnrollmentID != next.EnrollmentID ||
		previous.Namespace != next.Namespace || previous.WitnessAlgorithm != next.WitnessAlgorithm ||
		previous.WitnessKeyID != next.WitnessKeyID || previous.WitnessPublicKey != next.WitnessPublicKey ||
		previous.Generation == ^uint64(0) || next.Generation != previous.Generation+1 ||
		next.PreviousCheckpointDigest != previous.CheckpointDigest ||
		next.PreviousStateDigest != previous.CurrentStateDigest || next.MutationID == "" ||
		next.MutationID == previous.MutationID ||
		next.FenceNonce == previous.FenceNonce {
		return errors.New("monotonic head checkpoint is not the exact direct successor")
	}
	return nil
}

func ValidateThreadRiskAuthorityIndexCheckpointV1(index ThreadRiskAuthorityIndexV1, checkpoint MonotonicHeadCheckpointV1) error {
	if ValidateThreadRiskAuthorityIndexV1(index) != nil || ValidateMonotonicHeadCheckpointV1(checkpoint) != nil ||
		index.InstallationID != checkpoint.InstallationID || index.EnrollmentID != checkpoint.EnrollmentID ||
		index.Namespace != checkpoint.Namespace || index.Generation != checkpoint.Generation ||
		index.IndexDigest != checkpoint.CurrentStateDigest || index.MutationID != checkpoint.MutationID {
		return errors.New("thread risk authority index does not match monotonic checkpoint")
	}
	return nil
}

func ValidateThreadRiskAuthorityWitnessAdvanceV1(
	previousIndex, nextIndex ThreadRiskAuthorityIndexV1,
	previousCheckpoint MonotonicHeadCheckpointV1,
	request MonotonicHeadAdvanceRequestV1,
	receipt MonotonicHeadAdvanceReceiptV1,
) error {
	if err := ValidateThreadRiskAuthorityIndexTransitionV1(previousIndex, nextIndex); err != nil {
		return err
	}
	if err := ValidateThreadRiskAuthorityIndexCheckpointV1(previousIndex, previousCheckpoint); err != nil {
		return err
	}
	if err := ValidateMonotonicHeadAdvanceV1(previousCheckpoint, request, receipt); err != nil {
		return err
	}
	if nextIndex.MutationID != request.MutationID || nextIndex.IndexDigest != request.NextStateDigest ||
		nextIndex.AuthorityKeyID != request.AuthorityKeyID || nextIndex.AuthorityPublicKey != request.AuthorityPublicKey {
		return errors.New("thread risk authority index is not bound to witness request")
	}
	if err := ValidateThreadRiskAuthorityIndexCheckpointV1(nextIndex, receipt.Checkpoint); err != nil {
		return err
	}
	return nil
}

// ValidateThreadRiskAuthorityFirstWitnessAdvanceV1 binds a generation-one
// inventory to an enrolled generation-zero witness checkpoint. The initial
// inventory may contain a migrated set of entries, but every later index
// transition is restricted to one thread step by
// ValidateThreadRiskAuthorityWitnessAdvanceV1.
func ValidateThreadRiskAuthorityFirstWitnessAdvanceV1(
	firstIndex ThreadRiskAuthorityIndexV1,
	enrollmentCheckpoint MonotonicHeadCheckpointV1,
	request MonotonicHeadAdvanceRequestV1,
	receipt MonotonicHeadAdvanceReceiptV1,
) error {
	if ValidateThreadRiskAuthorityIndexV1(firstIndex) != nil || firstIndex.Generation != 1 || firstIndex.PreviousIndexDigest != "" ||
		enrollmentCheckpoint.Generation != 0 {
		return errors.New("thread risk authority initial index is invalid")
	}
	if err := ValidateMonotonicHeadAdvanceV1(enrollmentCheckpoint, request, receipt); err != nil {
		return err
	}
	if firstIndex.InstallationID != enrollmentCheckpoint.InstallationID || firstIndex.EnrollmentID != enrollmentCheckpoint.EnrollmentID ||
		firstIndex.Namespace != enrollmentCheckpoint.Namespace || firstIndex.MutationID != request.MutationID ||
		firstIndex.IndexDigest != request.NextStateDigest || firstIndex.AuthorityKeyID != request.AuthorityKeyID ||
		firstIndex.AuthorityPublicKey != request.AuthorityPublicKey {
		return errors.New("thread risk authority initial index is not bound to witness request")
	}
	return ValidateThreadRiskAuthorityIndexCheckpointV1(firstIndex, receipt.Checkpoint)
}

func ParseMonotonicHeadCheckpointV1(body []byte) (MonotonicHeadCheckpointV1, error) {
	var checkpoint MonotonicHeadCheckpointV1
	if err := decodeStrictThreadRiskAuthorityContract(body, &checkpoint); err != nil {
		return MonotonicHeadCheckpointV1{}, err
	}
	return checkpoint, ValidateMonotonicHeadCheckpointV1(checkpoint)
}

func ParseMonotonicHeadObserveRequestV1(body []byte) (MonotonicHeadObserveRequestV1, error) {
	var request MonotonicHeadObserveRequestV1
	if err := decodeStrictThreadRiskAuthorityContract(body, &request); err != nil {
		return MonotonicHeadObserveRequestV1{}, err
	}
	return request, ValidateMonotonicHeadObserveRequestV1(request)
}

func ParseMonotonicHeadObservationV1(body []byte) (MonotonicHeadObservationV1, error) {
	var observation MonotonicHeadObservationV1
	if err := decodeStrictThreadRiskAuthorityContract(body, &observation); err != nil {
		return MonotonicHeadObservationV1{}, err
	}
	return observation, ValidateMonotonicHeadObservationV1(observation)
}

func ParseMonotonicHeadAdvanceRequestV1(body []byte) (MonotonicHeadAdvanceRequestV1, error) {
	var request MonotonicHeadAdvanceRequestV1
	if err := decodeStrictThreadRiskAuthorityContract(body, &request); err != nil {
		return MonotonicHeadAdvanceRequestV1{}, err
	}
	return request, ValidateMonotonicHeadAdvanceRequestV1(request)
}

func ParseMonotonicHeadAdvanceReceiptV1(body []byte) (MonotonicHeadAdvanceReceiptV1, error) {
	var receipt MonotonicHeadAdvanceReceiptV1
	if err := decodeStrictThreadRiskAuthorityContract(body, &receipt); err != nil {
		return MonotonicHeadAdvanceReceiptV1{}, err
	}
	return receipt, ValidateMonotonicHeadAdvanceReceiptV1(receipt)
}

func MonotonicHeadCheckpointV1Bytes(checkpoint MonotonicHeadCheckpointV1) ([]byte, error) {
	if err := ValidateMonotonicHeadCheckpointV1(checkpoint); err != nil {
		return nil, err
	}
	return json.Marshal(checkpoint)
}

func MonotonicHeadObserveRequestV1Bytes(request MonotonicHeadObserveRequestV1) ([]byte, error) {
	if err := ValidateMonotonicHeadObserveRequestV1(request); err != nil {
		return nil, err
	}
	return json.Marshal(request)
}

func MonotonicHeadObservationV1Bytes(observation MonotonicHeadObservationV1) ([]byte, error) {
	if err := ValidateMonotonicHeadObservationV1(observation); err != nil {
		return nil, err
	}
	return json.Marshal(observation)
}

func MonotonicHeadAdvanceRequestV1Bytes(request MonotonicHeadAdvanceRequestV1) ([]byte, error) {
	if err := ValidateMonotonicHeadAdvanceRequestV1(request); err != nil {
		return nil, err
	}
	return json.Marshal(request)
}

func MonotonicHeadAdvanceReceiptV1Bytes(receipt MonotonicHeadAdvanceReceiptV1) ([]byte, error) {
	if err := ValidateMonotonicHeadAdvanceReceiptV1(receipt); err != nil {
		return nil, err
	}
	return json.Marshal(receipt)
}

func MonotonicHeadCheckpointSigningBytesV1(checkpoint MonotonicHeadCheckpointV1) []byte {
	checkpoint.WitnessSignature = ""
	checkpoint.CheckpointDigest = ""
	body, _ := json.Marshal(checkpoint)
	return monotonicHeadSigningPayload(monotonicHeadCheckpointSignatureDomain, body)
}

func MonotonicHeadObserveRequestSigningBytesV1(request MonotonicHeadObserveRequestV1) []byte {
	request.AuthoritySignature = ""
	request.RequestDigest = ""
	body, _ := json.Marshal(request)
	return monotonicHeadSigningPayload(monotonicHeadObserveRequestSignatureDomain, body)
}

func MonotonicHeadObservationSigningBytesV1(observation MonotonicHeadObservationV1) []byte {
	observation.WitnessSignature = ""
	observation.ObservationDigest = ""
	body, _ := json.Marshal(observation)
	return monotonicHeadSigningPayload(monotonicHeadObservationSignatureDomain, body)
}

func MonotonicHeadAdvanceRequestSigningBytesV1(request MonotonicHeadAdvanceRequestV1) []byte {
	request.AuthoritySignature = ""
	request.RequestDigest = ""
	body, _ := json.Marshal(request)
	return monotonicHeadSigningPayload(monotonicHeadRequestSignatureDomain, body)
}

func MonotonicHeadAdvanceReceiptSigningBytesV1(receipt MonotonicHeadAdvanceReceiptV1) []byte {
	receipt.WitnessSignature = ""
	receipt.ReceiptDigest = ""
	body, _ := json.Marshal(receipt)
	return monotonicHeadSigningPayload(monotonicHeadReceiptSignatureDomain, body)
}

func validateMonotonicHeadCheckpointUnsignedV1(checkpoint MonotonicHeadCheckpointV1) error {
	if checkpoint.SchemaVersion != MonotonicHeadCheckpointSchemaVersion || checkpoint.Purpose != MonotonicHeadCheckpointPurpose ||
		!isCanonicalSHA256Hex(checkpoint.InstallationID) || !isCanonicalSHA256Hex(checkpoint.EnrollmentID) ||
		!validMonotonicHeadNamespace(checkpoint.Namespace) || !isCanonicalSHA256Hex(checkpoint.CurrentStateDigest) ||
		!isCanonicalSHA256Hex(checkpoint.FenceNonce) || checkpoint.WitnessAlgorithm != MonotonicHeadAlgorithm ||
		!isCanonicalSHA256Hex(checkpoint.WitnessKeyID) {
		return errors.New("monotonic head checkpoint is incomplete")
	}
	if checkpoint.Generation == 0 {
		if checkpoint.PreviousStateDigest != "" || checkpoint.PreviousCheckpointDigest != "" || checkpoint.MutationID != "" {
			return errors.New("monotonic head enrollment checkpoint is invalid")
		}
	} else if !isCanonicalSHA256Hex(checkpoint.PreviousStateDigest) || !isCanonicalSHA256Hex(checkpoint.PreviousCheckpointDigest) ||
		!isCanonicalSHA256Hex(checkpoint.MutationID) || checkpoint.PreviousStateDigest == checkpoint.CurrentStateDigest {
		return errors.New("monotonic head checkpoint lineage is invalid")
	}
	return nil
}

func validateMonotonicHeadObserveRequestUnsignedV1(request MonotonicHeadObserveRequestV1) error {
	if request.SchemaVersion != MonotonicHeadCheckpointSchemaVersion || request.Purpose != MonotonicHeadObserveRequestPurpose ||
		!isCanonicalSHA256Hex(request.InstallationID) || !isCanonicalSHA256Hex(request.EnrollmentID) ||
		!validMonotonicHeadNamespace(request.Namespace) || !isCanonicalSHA256Hex(request.ChallengeNonce) ||
		request.AuthorityAlgorithm != MonotonicHeadAlgorithm || !isCanonicalSHA256Hex(request.AuthorityKeyID) {
		return errors.New("monotonic head observe request is invalid")
	}
	return nil
}

func validateMonotonicHeadAdvanceRequestUnsignedV1(request MonotonicHeadAdvanceRequestV1) error {
	if request.SchemaVersion != MonotonicHeadCheckpointSchemaVersion || request.Purpose != MonotonicHeadAdvanceRequestPurpose ||
		!isCanonicalSHA256Hex(request.InstallationID) || !isCanonicalSHA256Hex(request.EnrollmentID) ||
		!validMonotonicHeadNamespace(request.Namespace) || !isCanonicalSHA256Hex(request.ExpectedCheckpointDigest) ||
		!isCanonicalSHA256Hex(request.ExpectedStateDigest) || !isCanonicalSHA256Hex(request.NextStateDigest) ||
		request.ExpectedStateDigest == request.NextStateDigest || !isCanonicalSHA256Hex(request.ExpectedFenceNonce) ||
		!isCanonicalSHA256Hex(request.MutationID) || request.ExpectedGeneration == ^uint64(0) ||
		request.NextGeneration != request.ExpectedGeneration+1 || request.AuthorityAlgorithm != MonotonicHeadAlgorithm ||
		!isCanonicalSHA256Hex(request.AuthorityKeyID) {
		return errors.New("monotonic head advance request is invalid")
	}
	return nil
}

func validMonotonicHeadNamespace(namespace string) bool {
	return namespace == ThreadRiskAuthorityNamespaceV1 || namespace == EvidenceRegistryAuthorityNamespaceV1
}

func verifyMonotonicHeadSignature(algorithm, keyID, encodedPublicKey, encodedSignature string, message []byte) error {
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(encodedPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(encodedSignature)
	if algorithm != MonotonicHeadAlgorithm || publicErr != nil || signatureErr != nil ||
		len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != encodedPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != encodedSignature || keyID != SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), message, signature) {
		return errors.New("signature verification failed")
	}
	return nil
}

func monotonicHeadCheckpointDigestV1(checkpoint MonotonicHeadCheckpointV1) string {
	checkpoint.CheckpointDigest = ""
	body, _ := json.Marshal(checkpoint)
	return SHA256Hex(body)
}

func monotonicHeadObserveRequestDigestV1(request MonotonicHeadObserveRequestV1) string {
	request.RequestDigest = ""
	body, _ := json.Marshal(request)
	return SHA256Hex(body)
}

func monotonicHeadObservationDigestV1(observation MonotonicHeadObservationV1) string {
	observation.ObservationDigest = ""
	body, _ := json.Marshal(observation)
	return SHA256Hex(body)
}

func monotonicHeadAdvanceRequestDigestV1(request MonotonicHeadAdvanceRequestV1) string {
	request.RequestDigest = ""
	body, _ := json.Marshal(request)
	return SHA256Hex(body)
}

func monotonicHeadAdvanceReceiptDigestV1(receipt MonotonicHeadAdvanceReceiptV1) string {
	receipt.ReceiptDigest = ""
	body, _ := json.Marshal(receipt)
	return SHA256Hex(body)
}

func monotonicHeadSigningPayload(domain, body []byte) []byte {
	digest := sha256.Sum256(body)
	out := append([]byte(nil), domain...)
	return append(out, digest[:]...)
}
