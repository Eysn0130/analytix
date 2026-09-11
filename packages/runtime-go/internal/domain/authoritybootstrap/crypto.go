package authoritybootstrap

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var (
	bindingSignatureDomainV1         = []byte("analytix.authority-bootstrap-binding/signature/v1\x00")
	bindingDigestDomainV1            = []byte("analytix.authority-bootstrap-binding/digest/v1\x00")
	observationSignatureDomainV1     = []byte("analytix.authority-bootstrap-observation/signature/v1\x00")
	observationDigestDomainV1        = []byte("analytix.authority-bootstrap-observation/digest/v1\x00")
	prepareReceiptSignatureDomainV1  = []byte("analytix.authority-bootstrap-prepare-receipt/signature/v1\x00")
	prepareReceiptDigestDomainV1     = []byte("analytix.authority-bootstrap-prepare-receipt/digest/v1\x00")
	commitReceiptSignatureDomainV1   = []byte("analytix.authority-bootstrap-commit-receipt/signature/v1\x00")
	commitReceiptDigestDomainV1      = []byte("analytix.authority-bootstrap-commit-receipt/digest/v1\x00")
	projectionSlotDigestDomainV1     = []byte("analytix.authority-bootstrap-projection-slot/v1\x00")
	bootstrapEnrollmentDomainV1      = []byte("analytix.authority-bootstrap-enrollment/v1\x00")
	bootstrapStateDigestDomainV1     = []byte("analytix.authority-bootstrap-state/v1\x00")
	bootstrapInitialFenceDomainV1    = []byte("analytix.authority-bootstrap-initial-fence/v1\x00")
	manifestEnrollmentDigestDomainV1 = []byte("analytix.authority-bootstrap-manifest-enrollment/v1\x00")
)

func BootstrapBindingSigningBytesV1(binding BootstrapBindingV1) []byte {
	binding.InstallationAuthoritySignature = ""
	binding.BindingDigest = ""
	body, _ := json.Marshal(binding)
	return signingPayloadV1(bindingSignatureDomainV1, body)
}

func BootstrapObservationSigningBytesV1(observation BootstrapObservationV1) []byte {
	observation.WitnessSignature = ""
	observation.ObservationDigest = ""
	body, _ := json.Marshal(observation)
	return signingPayloadV1(observationSignatureDomainV1, body)
}

func BootstrapPrepareReceiptSigningBytesV1(receipt BootstrapPrepareReceiptV1) []byte {
	receipt.WitnessSignature = ""
	receipt.ReceiptDigest = ""
	body, _ := json.Marshal(receipt)
	return signingPayloadV1(prepareReceiptSignatureDomainV1, body)
}

func BootstrapCommitReceiptSigningBytesV1(receipt BootstrapCommitReceiptV1) []byte {
	receipt.WitnessSignature = ""
	receipt.ReceiptDigest = ""
	body, _ := json.Marshal(receipt)
	return signingPayloadV1(commitReceiptSignatureDomainV1, body)
}

func bindingDigestV1(binding BootstrapBindingV1) string {
	binding.BindingDigest = ""
	body, _ := json.Marshal(binding)
	return digestPayloadV1(bindingDigestDomainV1, body)
}

func observationDigestV1(observation BootstrapObservationV1) string {
	observation.ObservationDigest = ""
	body, _ := json.Marshal(observation)
	return digestPayloadV1(observationDigestDomainV1, body)
}

func prepareReceiptDigestV1(receipt BootstrapPrepareReceiptV1) string {
	receipt.ReceiptDigest = ""
	body, _ := json.Marshal(receipt)
	return digestPayloadV1(prepareReceiptDigestDomainV1, body)
}

func commitReceiptDigestV1(receipt BootstrapCommitReceiptV1) string {
	receipt.ReceiptDigest = ""
	body, _ := json.Marshal(receipt)
	return digestPayloadV1(commitReceiptDigestDomainV1, body)
}

func projectionSlotIDV1(installationID, namespace string) string {
	body, _ := json.Marshal(struct {
		InstallationID string `json:"installationId"`
		Namespace      string `json:"namespace"`
	}{InstallationID: installationID, Namespace: namespace})
	return digestPayloadV1(projectionSlotDigestDomainV1, body)
}

func bootstrapEnrollmentIDV1(installationID, namespace, projectionSlotID string) string {
	body, _ := json.Marshal(struct {
		InstallationID   string `json:"installationId"`
		Namespace        string `json:"namespace"`
		ProjectionSlotID string `json:"projectionSlotId"`
	}{InstallationID: installationID, Namespace: namespace, ProjectionSlotID: projectionSlotID})
	return digestPayloadV1(bootstrapEnrollmentDomainV1, body)
}

func manifestEnrollmentDigestV1(enrollment domainenrollment.WitnessEnrollmentV1) string {
	body, _ := json.Marshal(enrollment)
	return digestPayloadV1(manifestEnrollmentDigestDomainV1, body)
}

func bootstrapStateDigestV1(
	bindingDigest, bootstrapEnrollmentID, projectionSlotID, floorProjectionDigest string,
	phase BootstrapPhaseV1,
	prepareMutationID, prepareReceiptDigest, commitMutationID string,
) string {
	body, _ := json.Marshal(struct {
		SchemaVersion         int              `json:"schemaVersion"`
		Purpose               string           `json:"purpose"`
		BindingDigest         string           `json:"bindingDigest"`
		BootstrapEnrollmentID string           `json:"bootstrapEnrollmentId"`
		ProjectionSlotID      string           `json:"projectionSlotId"`
		FloorProjectionDigest string           `json:"floorProjectionDigest"`
		Phase                 BootstrapPhaseV1 `json:"phase"`
		PrepareMutationID     string           `json:"prepareMutationId"`
		PrepareReceiptDigest  string           `json:"prepareReceiptDigest"`
		CommitMutationID      string           `json:"commitMutationId"`
	}{
		SchemaVersion: SchemaVersionV1, Purpose: "analytix.authority-bootstrap-state/v1",
		BindingDigest: bindingDigest, BootstrapEnrollmentID: bootstrapEnrollmentID,
		ProjectionSlotID: projectionSlotID, FloorProjectionDigest: floorProjectionDigest, Phase: phase,
		PrepareMutationID: prepareMutationID, PrepareReceiptDigest: prepareReceiptDigest, CommitMutationID: commitMutationID,
	})
	return digestPayloadV1(bootstrapStateDigestDomainV1, body)
}

func unpreparedStateDigestV1(binding BootstrapBindingV1) string {
	// Genesis is stable for the derived projection slot and deliberately does
	// not bind a candidate floor. The exact prepare CAS is the one-shot point
	// that selects a binding/floor; alternate binding attempts therefore race
	// on one identical enrolled head instead of minting forked generation zero.
	return bootstrapStateDigestV1(
		"", binding.BootstrapEnrollmentID, binding.ProjectionSlotID, "",
		PhaseUnprepared, "", "", "",
	)
}

func bootstrapInitialFenceNonceV1(binding BootstrapBindingV1) string {
	body, _ := json.Marshal(struct {
		BootstrapEnrollmentID string `json:"bootstrapEnrollmentId"`
		ProjectionSlotID      string `json:"projectionSlotId"`
	}{
		BootstrapEnrollmentID: binding.BootstrapEnrollmentID,
		ProjectionSlotID:      binding.ProjectionSlotID,
	})
	return digestPayloadV1(bootstrapInitialFenceDomainV1, body)
}

func PreparedStateDigestV1(anchored AnchoredBootstrapBindingV1, mutationID string) (string, error) {
	binding, err := anchored.BindingV1()
	if err != nil || !canonicalDigest(mutationID) {
		return "", errors.Join(ErrInvalidContract, err)
	}
	return bootstrapStateDigestV1(
		binding.BindingDigest, binding.BootstrapEnrollmentID, binding.ProjectionSlotID, binding.FloorProjectionDigest,
		PhasePrepared, mutationID, "", "",
	), nil
}

func CommittedStateDigestV1(anchored AnchoredBootstrapBindingV1, prepare BootstrapPrepareReceiptV1, mutationID string) (string, error) {
	binding, err := anchored.BindingV1()
	if err != nil || ValidateBootstrapPrepareReceiptForBindingV1(prepare, anchored) != nil || !canonicalDigest(mutationID) ||
		mutationID == prepare.AdvanceRequest.MutationID {
		return "", errors.Join(ErrInvalidContract, err)
	}
	return bootstrapStateDigestV1(
		binding.BindingDigest, binding.BootstrapEnrollmentID, binding.ProjectionSlotID, binding.FloorProjectionDigest,
		PhaseCommitted, prepare.AdvanceRequest.MutationID, prepare.ReceiptDigest, mutationID,
	), nil
}

func checkpointCanonicalDigestV1(checkpoint domainsecurity.MonotonicHeadCheckpointV1) (string, error) {
	body, err := domainsecurity.MonotonicHeadCheckpointV1Bytes(checkpoint)
	if err != nil {
		return "", err
	}
	return domainsecurity.SHA256Hex(body), nil
}

func equalCheckpointV1(left, right domainsecurity.MonotonicHeadCheckpointV1) bool {
	leftBody, leftErr := domainsecurity.MonotonicHeadCheckpointV1Bytes(left)
	rightBody, rightErr := domainsecurity.MonotonicHeadCheckpointV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func equalObserveRequestV1(left, right domainsecurity.MonotonicHeadObserveRequestV1) bool {
	leftBody, leftErr := domainsecurity.MonotonicHeadObserveRequestV1Bytes(left)
	rightBody, rightErr := domainsecurity.MonotonicHeadObserveRequestV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func equalMonotonicObservationV1(left, right domainsecurity.MonotonicHeadObservationV1) bool {
	leftBody, leftErr := domainsecurity.MonotonicHeadObservationV1Bytes(left)
	rightBody, rightErr := domainsecurity.MonotonicHeadObservationV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func signingPayloadV1(domain, body []byte) []byte {
	sum := sha256.Sum256(body)
	payload := append([]byte(nil), domain...)
	return append(payload, sum[:]...)
}

func digestPayloadV1(domain, body []byte) string {
	payload := append([]byte(nil), domain...)
	return domainsecurity.SHA256Hex(append(payload, body...))
}

func verifySignedV1(algorithm, keyID, encodedPublicKey, encodedSignature string, message []byte) error {
	if algorithm != AlgorithmV1 || len(encodedPublicKey) != base64.RawURLEncoding.EncodedLen(ed25519.PublicKeySize) ||
		len(encodedSignature) != base64.RawURLEncoding.EncodedLen(ed25519.SignatureSize) {
		return ErrInvalidContract
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(encodedPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(encodedSignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != encodedPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != encodedSignature ||
		keyID != domainsecurity.SHA256Hex(publicKey) || !ed25519.Verify(ed25519.PublicKey(publicKey), message, signature) {
		return ErrInvalidContract
	}
	return nil
}

func decodeCanonicalPublicKeyV1(encoded, expectedKeyID string) ([]byte, error) {
	if len(encoded) != base64.RawURLEncoding.EncodedLen(ed25519.PublicKeySize) {
		return nil, ErrInvalidContract
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(publicKey) != ed25519.PublicKeySize || base64.RawURLEncoding.EncodeToString(publicKey) != encoded ||
		expectedKeyID != domainsecurity.SHA256Hex(publicKey) {
		return nil, ErrInvalidContract
	}
	return publicKey, nil
}

func validNamespaceV1(namespace string) bool {
	return namespace == domainsecurity.ThreadRiskAuthorityNamespaceV1 || namespace == domainsecurity.EvidenceRegistryAuthorityNamespaceV1
}

func canonicalDigest(value string) bool {
	return value == strings.TrimSpace(value) && domainsecurity.IsSHA256Hex(value)
}
