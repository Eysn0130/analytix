package authorityadvance

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var (
	monotonicAdvanceIntentSignatureDomainV1 = []byte("analytix.monotonic-advance-intent/signature/v1\x00")
	monotonicAdvanceIntentDigestDomainV1    = []byte("analytix.monotonic-advance-intent/digest/v1\x00")
)

// MonotonicAdvanceIntentV1 is the durable write-ahead boundary for one exact
// witness CAS. Persisting it authorizes only byte-identical replay of its
// MutationID and AdvanceRequest; it is not evidence that the advance committed.
type MonotonicAdvanceIntentV1 struct {
	SchemaVersion      int                                          `json:"schemaVersion"`
	Purpose            string                                       `json:"purpose"`
	InstallationID     string                                       `json:"installationId"`
	EnrollmentID       string                                       `json:"enrollmentId"`
	Namespace          string                                       `json:"namespace"`
	Root               AdvanceRootV1                                `json:"root"`
	MutationID         string                                       `json:"mutationId"`
	RequestDigest      string                                       `json:"requestDigest"`
	PreviousCheckpoint domainsecurity.MonotonicHeadCheckpointV1     `json:"previousCheckpoint"`
	AdvanceRequest     domainsecurity.MonotonicHeadAdvanceRequestV1 `json:"advanceRequest"`
	Transition         AdvanceTransitionBindingV1                   `json:"transition"`
	AuthorityAlgorithm string                                       `json:"authorityAlgorithm"`
	AuthorityKeyID     string                                       `json:"authorityKeyId"`
	AuthorityPublicKey string                                       `json:"authorityPublicKey"`
	AuthoritySignature string                                       `json:"authoritySignature"`
	RecordDigest       string                                       `json:"recordDigest"`
}

type MonotonicAdvanceIntentInputV1 struct {
	Root               AdvanceRootV1
	PreviousCheckpoint domainsecurity.MonotonicHeadCheckpointV1
	AdvanceRequest     domainsecurity.MonotonicHeadAdvanceRequestV1
	Transition         AdvanceTransitionBindingV1
	AuthorityKeyID     string
	AuthorityPublicKey []byte
}

type MonotonicAdvanceSignFuncV1 func([]byte) ([]byte, error)

func NewMonotonicAdvanceIntentV1(
	input MonotonicAdvanceIntentInputV1,
	sign MonotonicAdvanceSignFuncV1,
) (MonotonicAdvanceIntentV1, error) {
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	intent := MonotonicAdvanceIntentV1{
		SchemaVersion:      MonotonicAdvanceIntentSchemaVersionV1,
		Purpose:            MonotonicAdvanceIntentPurposeV1,
		InstallationID:     input.AdvanceRequest.InstallationID,
		EnrollmentID:       input.AdvanceRequest.EnrollmentID,
		Namespace:          input.AdvanceRequest.Namespace,
		Root:               input.Root,
		MutationID:         input.AdvanceRequest.MutationID,
		RequestDigest:      input.AdvanceRequest.RequestDigest,
		PreviousCheckpoint: input.PreviousCheckpoint,
		AdvanceRequest:     input.AdvanceRequest,
		Transition:         input.Transition,
		AuthorityAlgorithm: MonotonicAdvanceAlgorithmV1,
		AuthorityKeyID:     strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	if sign == nil || len(publicKey) != ed25519.PublicKeySize ||
		intent.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) {
		return MonotonicAdvanceIntentV1{}, errors.New("monotonic advance intent signing authority is invalid")
	}
	if err := validateMonotonicAdvanceIntentPayloadV1(intent); err != nil {
		return MonotonicAdvanceIntentV1{}, err
	}
	signature, err := sign(MonotonicAdvanceIntentSigningBytesV1(intent))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return MonotonicAdvanceIntentV1{}, errors.New("monotonic advance intent signing failed")
	}
	intent.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	intent.RecordDigest = monotonicAdvanceIntentDigestV1(intent)
	if err := ValidateMonotonicAdvanceIntentV1(intent); err != nil {
		return MonotonicAdvanceIntentV1{}, err
	}
	return intent, nil
}

func ValidateMonotonicAdvanceIntentV1(intent MonotonicAdvanceIntentV1) error {
	if err := validateMonotonicAdvanceIntentPayloadV1(intent); err != nil {
		return err
	}
	encoded, err := json.Marshal(intent)
	if err != nil || len(encoded) > maxMonotonicAdvanceIntentBytesV1 {
		return errors.New("monotonic advance intent exceeds its canonical bound")
	}
	if !canonicalSHA256V1(intent.RecordDigest) || intent.RecordDigest != monotonicAdvanceIntentDigestV1(intent) {
		return errors.New("monotonic advance intent digest is invalid")
	}
	publicKey, publicErr := decodeCanonicalPublicKeyV1(intent.AuthorityPublicKey)
	signature, signatureErr := decodeCanonicalSignatureV1(intent.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || intent.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), MonotonicAdvanceIntentSigningBytesV1(intent), signature) {
		return errors.New("monotonic advance intent signature is invalid")
	}
	return nil
}

func ValidateMonotonicAdvanceIntentForPreviousCheckpointV1(
	intent MonotonicAdvanceIntentV1,
	checkpoint domainsecurity.MonotonicHeadCheckpointV1,
) error {
	if err := ValidateMonotonicAdvanceIntentV1(intent); err != nil {
		return err
	}
	if err := domainsecurity.ValidateMonotonicHeadCheckpointV1(checkpoint); err != nil {
		return err
	}
	if intent.PreviousCheckpoint != checkpoint {
		return errors.New("monotonic advance intent previous checkpoint is not exact")
	}
	return nil
}

// ValidateMonotonicAdvanceIntentExactReplayV1 prohibits mutation-ID reuse with
// changed request bytes. A self-valid request with the same semantic fields but
// another signature is still not an exact replay.
func ValidateMonotonicAdvanceIntentExactReplayV1(
	intent MonotonicAdvanceIntentV1,
	replayRequest domainsecurity.MonotonicHeadAdvanceRequestV1,
) error {
	if err := ValidateMonotonicAdvanceIntentV1(intent); err != nil {
		return err
	}
	if err := domainsecurity.ValidateMonotonicHeadAdvanceRequestV1(replayRequest); err != nil {
		return err
	}
	if replayRequest.MutationID != intent.MutationID {
		return errors.New("monotonic advance replay mutation ID does not match intent")
	}
	committedBytes, _ := domainsecurity.MonotonicHeadAdvanceRequestV1Bytes(intent.AdvanceRequest)
	replayBytes, _ := domainsecurity.MonotonicHeadAdvanceRequestV1Bytes(replayRequest)
	if !bytes.Equal(committedBytes, replayBytes) {
		return errors.New("monotonic advance replay request is not exact")
	}
	return nil
}

func ParseMonotonicAdvanceIntentV1(body []byte) (MonotonicAdvanceIntentV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxMonotonicAdvanceIntentBytesV1, MaxDepth: 16,
		MaxTokens: 500_000, MaxStringBytes: 64 << 10,
	}); err != nil {
		return MonotonicAdvanceIntentV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var intent MonotonicAdvanceIntentV1
	if err := decoder.Decode(&intent); err != nil {
		return MonotonicAdvanceIntentV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return MonotonicAdvanceIntentV1{}, errors.New("monotonic advance intent contains trailing JSON")
	}
	canonical, err := json.Marshal(intent)
	if err != nil || !bytes.Equal(body, canonical) {
		return MonotonicAdvanceIntentV1{}, errors.New("monotonic advance intent is not canonically encoded")
	}
	return intent, ValidateMonotonicAdvanceIntentV1(intent)
}

func MonotonicAdvanceIntentV1Bytes(intent MonotonicAdvanceIntentV1) ([]byte, error) {
	if err := ValidateMonotonicAdvanceIntentV1(intent); err != nil {
		return nil, err
	}
	return json.Marshal(intent)
}

func MonotonicAdvanceIntentSigningBytesV1(intent MonotonicAdvanceIntentV1) []byte {
	intent.AuthoritySignature = ""
	intent.RecordDigest = ""
	body, _ := json.Marshal(intent)
	digest := sha256.Sum256(body)
	out := append([]byte(nil), monotonicAdvanceIntentSignatureDomainV1...)
	return append(out, digest[:]...)
}

func validateMonotonicAdvanceIntentPayloadV1(intent MonotonicAdvanceIntentV1) error {
	if intent.SchemaVersion != MonotonicAdvanceIntentSchemaVersionV1 || intent.Purpose != MonotonicAdvanceIntentPurposeV1 ||
		!canonicalSHA256V1(intent.InstallationID) || !canonicalSHA256V1(intent.EnrollmentID) ||
		!canonicalSHA256V1(intent.MutationID) || !canonicalSHA256V1(intent.RequestDigest) ||
		intent.AuthorityAlgorithm != MonotonicAdvanceAlgorithmV1 || !canonicalSHA256V1(intent.AuthorityKeyID) {
		return errors.New("monotonic advance intent is incomplete")
	}
	namespace, err := NamespaceForAdvanceRootV1(intent.Root)
	if err != nil || namespace != intent.Namespace {
		return errors.New("monotonic advance intent root namespace mismatch")
	}
	if domainsecurity.ValidateMonotonicHeadCheckpointV1(intent.PreviousCheckpoint) != nil ||
		domainsecurity.ValidateMonotonicHeadAdvanceRequestV1(intent.AdvanceRequest) != nil ||
		ValidateAdvanceTransitionBindingV1(intent.Root, intent.Transition) != nil {
		return errors.New("monotonic advance intent embedded authority is invalid")
	}
	request := intent.AdvanceRequest
	previous := intent.PreviousCheckpoint
	if intent.InstallationID != request.InstallationID || intent.InstallationID != previous.InstallationID ||
		intent.EnrollmentID != request.EnrollmentID || intent.EnrollmentID != previous.EnrollmentID ||
		intent.Namespace != request.Namespace || intent.Namespace != previous.Namespace ||
		intent.MutationID != request.MutationID || intent.RequestDigest != request.RequestDigest ||
		request.ExpectedGeneration != previous.Generation || request.ExpectedCheckpointDigest != previous.CheckpointDigest ||
		request.ExpectedStateDigest != previous.CurrentStateDigest || request.ExpectedFenceNonce != previous.FenceNonce ||
		request.NextGeneration != previous.Generation+1 || intent.Transition.PreviousGeneration != previous.Generation ||
		intent.Transition.NextGeneration != request.NextGeneration ||
		intent.Transition.PreviousStateDigest != previous.CurrentStateDigest ||
		intent.Transition.NextStateDigest != request.NextStateDigest ||
		intent.AuthorityKeyID != request.AuthorityKeyID || intent.AuthorityPublicKey != request.AuthorityPublicKey {
		return errors.New("monotonic advance intent checkpoint, request, or transition binding mismatch")
	}
	switch intent.Root {
	case AdvanceRootThreadRisk:
		previousIndex := intent.Transition.ThreadRisk.PreviousIndex
		nextIndex := intent.Transition.ThreadRisk.NextIndex
		if domainsecurity.ValidateThreadRiskAuthorityIndexCheckpointV1(previousIndex, previous) != nil ||
			nextIndex.InstallationID != request.InstallationID || nextIndex.EnrollmentID != request.EnrollmentID ||
			nextIndex.Namespace != request.Namespace || nextIndex.Generation != request.NextGeneration ||
			nextIndex.IndexDigest != request.NextStateDigest || nextIndex.MutationID != request.MutationID ||
			nextIndex.AuthorityKeyID != request.AuthorityKeyID || nextIndex.AuthorityPublicKey != request.AuthorityPublicKey {
			return errors.New("thread risk intent is not bound to exact indexes")
		}
	case AdvanceRootDatasetSnapshot, AdvanceRootEvidenceRegistry, AdvanceRootPublication:
		previousBundle := intent.Transition.EvidenceBundle.PreviousBundle
		nextBundle := intent.Transition.EvidenceBundle.NextBundle
		if domainevidence.ValidateEvidenceAuthorityBundleCheckpointV1(previousBundle, previous) != nil ||
			nextBundle.InstallationID != request.InstallationID || nextBundle.EnrollmentID != request.EnrollmentID ||
			nextBundle.Namespace != request.Namespace || nextBundle.Generation != request.NextGeneration ||
			nextBundle.RecordDigest != request.NextStateDigest || nextBundle.MutationID != request.MutationID ||
			nextBundle.AuthorityKeyID != request.AuthorityKeyID || nextBundle.AuthorityPublicKey != request.AuthorityPublicKey {
			return errors.New("evidence intent is not bound to exact bundles")
		}
	default:
		return errors.New("monotonic advance intent root is unknown")
	}
	return nil
}

func monotonicAdvanceIntentDigestV1(intent MonotonicAdvanceIntentV1) string {
	intent.RecordDigest = ""
	body, _ := json.Marshal(intent)
	payload := append([]byte(nil), monotonicAdvanceIntentDigestDomainV1...)
	return domainsecurity.SHA256Hex(append(payload, body...))
}

func canonicalSHA256V1(value string) bool {
	return value == strings.ToLower(value) && value == strings.TrimSpace(value) && domainsecurity.IsSHA256Hex(value)
}

func decodeCanonicalPublicKeyV1(encoded string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(decoded) != ed25519.PublicKeySize || base64.RawURLEncoding.EncodeToString(decoded) != encoded {
		return nil, errors.New("monotonic advance public key is invalid")
	}
	return decoded, nil
}

func decodeCanonicalSignatureV1(encoded string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(decoded) != ed25519.SignatureSize || base64.RawURLEncoding.EncodeToString(decoded) != encoded {
		return nil, errors.New("monotonic advance signature is invalid")
	}
	return decoded, nil
}
