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
	monotonicAdvanceIntentSignatureDomainV2 = []byte("analytix.monotonic-advance-intent/signature/v2\x00")
	monotonicAdvanceIntentDigestDomainV2    = []byte("analytix.monotonic-advance-intent/digest/v2\x00")
)

type MonotonicAdvanceIntentV2 struct {
	SchemaVersion      int                                          `json:"schemaVersion"`
	Purpose            string                                       `json:"purpose"`
	InstallationID     string                                       `json:"installationId"`
	EnrollmentID       string                                       `json:"enrollmentId"`
	Namespace          string                                       `json:"namespace"`
	Root               AdvanceRootV2                                `json:"root"`
	MutationID         string                                       `json:"mutationId"`
	RequestDigest      string                                       `json:"requestDigest"`
	PreviousCheckpoint domainsecurity.MonotonicHeadCheckpointV1     `json:"previousCheckpoint"`
	AdvanceRequest     domainsecurity.MonotonicHeadAdvanceRequestV1 `json:"advanceRequest"`
	Transition         AdvanceTransitionBindingV2                   `json:"transition"`
	AuthorityAlgorithm string                                       `json:"authorityAlgorithm"`
	AuthorityKeyID     string                                       `json:"authorityKeyId"`
	AuthorityPublicKey string                                       `json:"authorityPublicKey"`
	AuthoritySignature string                                       `json:"authoritySignature"`
	RecordDigest       string                                       `json:"recordDigest"`
}

type MonotonicAdvanceIntentInputV2 struct {
	Root               AdvanceRootV2
	PreviousCheckpoint domainsecurity.MonotonicHeadCheckpointV1
	AdvanceRequest     domainsecurity.MonotonicHeadAdvanceRequestV1
	Transition         AdvanceTransitionBindingV2
	AuthorityKeyID     string
	AuthorityPublicKey []byte
}

type MonotonicAdvanceSignFuncV2 func([]byte) ([]byte, error)

func NewMonotonicAdvanceIntentV2(
	input MonotonicAdvanceIntentInputV2,
	sign MonotonicAdvanceSignFuncV2,
) (MonotonicAdvanceIntentV2, error) {
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	intent := MonotonicAdvanceIntentV2{
		SchemaVersion:      MonotonicAdvanceIntentSchemaVersionV2,
		Purpose:            MonotonicAdvanceIntentPurposeV2,
		InstallationID:     input.AdvanceRequest.InstallationID,
		EnrollmentID:       input.AdvanceRequest.EnrollmentID,
		Namespace:          input.AdvanceRequest.Namespace,
		Root:               input.Root,
		MutationID:         input.AdvanceRequest.MutationID,
		RequestDigest:      input.AdvanceRequest.RequestDigest,
		PreviousCheckpoint: input.PreviousCheckpoint,
		AdvanceRequest:     input.AdvanceRequest,
		Transition:         input.Transition,
		AuthorityAlgorithm: MonotonicAdvanceAlgorithmV2,
		AuthorityKeyID:     strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	if sign == nil || len(publicKey) != ed25519.PublicKeySize ||
		intent.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) {
		return MonotonicAdvanceIntentV2{}, errors.New("monotonic advance V2 intent signing authority is invalid")
	}
	if err := validateMonotonicAdvanceIntentPayloadV2(intent); err != nil {
		return MonotonicAdvanceIntentV2{}, err
	}
	signature, err := sign(MonotonicAdvanceIntentSigningBytesV2(intent))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return MonotonicAdvanceIntentV2{}, errors.New("monotonic advance V2 intent signing failed")
	}
	intent.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	intent.RecordDigest = monotonicAdvanceIntentDigestV2(intent)
	if err := ValidateMonotonicAdvanceIntentV2(intent); err != nil {
		return MonotonicAdvanceIntentV2{}, err
	}
	return intent, nil
}

func ValidateMonotonicAdvanceIntentV2(intent MonotonicAdvanceIntentV2) error {
	if err := validateMonotonicAdvanceIntentPayloadV2(intent); err != nil {
		return err
	}
	encoded, err := json.Marshal(intent)
	if err != nil || len(encoded) > MaxMonotonicAdvanceJournalRecordBytesV2 {
		return errors.New("monotonic advance V2 intent exceeds its canonical bound")
	}
	if !canonicalSHA256V1(intent.RecordDigest) || intent.RecordDigest != monotonicAdvanceIntentDigestV2(intent) {
		return errors.New("monotonic advance V2 intent digest is invalid")
	}
	publicKey, publicErr := decodeCanonicalPublicKeyV1(intent.AuthorityPublicKey)
	signature, signatureErr := decodeCanonicalSignatureV1(intent.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || intent.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), MonotonicAdvanceIntentSigningBytesV2(intent), signature) {
		return errors.New("monotonic advance V2 intent signature is invalid")
	}
	return nil
}

func ValidateMonotonicAdvanceIntentForPreviousCheckpointV2(
	intent MonotonicAdvanceIntentV2,
	checkpoint domainsecurity.MonotonicHeadCheckpointV1,
) error {
	if err := ValidateMonotonicAdvanceIntentV2(intent); err != nil {
		return err
	}
	if domainsecurity.ValidateMonotonicHeadCheckpointV1(checkpoint) != nil || intent.PreviousCheckpoint != checkpoint {
		return errors.New("monotonic advance V2 intent previous checkpoint is not exact")
	}
	return nil
}

func ValidateMonotonicAdvanceIntentExactReplayV2(
	intent MonotonicAdvanceIntentV2,
	replay domainsecurity.MonotonicHeadAdvanceRequestV1,
) error {
	if err := ValidateMonotonicAdvanceIntentV2(intent); err != nil {
		return err
	}
	if err := domainsecurity.ValidateMonotonicHeadAdvanceRequestV1(replay); err != nil {
		return err
	}
	committedBytes, _ := domainsecurity.MonotonicHeadAdvanceRequestV1Bytes(intent.AdvanceRequest)
	replayBytes, _ := domainsecurity.MonotonicHeadAdvanceRequestV1Bytes(replay)
	if replay.MutationID != intent.MutationID || !bytes.Equal(committedBytes, replayBytes) {
		return errors.New("monotonic advance V2 replay request is not exact")
	}
	return nil
}

func ParseMonotonicAdvanceIntentV2(body []byte) (MonotonicAdvanceIntentV2, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: MaxMonotonicAdvanceJournalRecordBytesV2, MaxDepth: 16,
		MaxTokens: 500_000, MaxStringBytes: 64 << 10,
	}); err != nil {
		return MonotonicAdvanceIntentV2{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var intent MonotonicAdvanceIntentV2
	if err := decoder.Decode(&intent); err != nil {
		return MonotonicAdvanceIntentV2{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return MonotonicAdvanceIntentV2{}, errors.New("monotonic advance V2 intent contains trailing JSON")
	}
	canonical, err := json.Marshal(intent)
	if err != nil || !bytes.Equal(body, canonical) {
		return MonotonicAdvanceIntentV2{}, errors.New("monotonic advance V2 intent is not canonically encoded")
	}
	return intent, ValidateMonotonicAdvanceIntentV2(intent)
}

func MonotonicAdvanceIntentV2Bytes(intent MonotonicAdvanceIntentV2) ([]byte, error) {
	if err := ValidateMonotonicAdvanceIntentV2(intent); err != nil {
		return nil, err
	}
	return json.Marshal(intent)
}

func MonotonicAdvanceIntentSigningBytesV2(intent MonotonicAdvanceIntentV2) []byte {
	intent.AuthoritySignature = ""
	intent.RecordDigest = ""
	body, _ := json.Marshal(intent)
	digest := sha256.Sum256(body)
	out := append([]byte(nil), monotonicAdvanceIntentSignatureDomainV2...)
	return append(out, digest[:]...)
}

func validateMonotonicAdvanceIntentPayloadV2(intent MonotonicAdvanceIntentV2) error {
	if intent.SchemaVersion != MonotonicAdvanceIntentSchemaVersionV2 || intent.Purpose != MonotonicAdvanceIntentPurposeV2 ||
		!canonicalSHA256V1(intent.InstallationID) || !canonicalSHA256V1(intent.EnrollmentID) ||
		!canonicalSHA256V1(intent.MutationID) || !canonicalSHA256V1(intent.RequestDigest) ||
		intent.AuthorityAlgorithm != MonotonicAdvanceAlgorithmV2 || !canonicalSHA256V1(intent.AuthorityKeyID) {
		return errors.New("monotonic advance V2 intent is incomplete")
	}
	namespace, err := NamespaceForAdvanceRootV2(intent.Root)
	if err != nil || namespace != intent.Namespace {
		return errors.New("monotonic advance V2 intent root namespace mismatch")
	}
	if domainsecurity.ValidateMonotonicHeadCheckpointV1(intent.PreviousCheckpoint) != nil ||
		domainsecurity.ValidateMonotonicHeadAdvanceRequestV1(intent.AdvanceRequest) != nil ||
		ValidateAdvanceTransitionBindingV2(intent.Root, intent.Transition) != nil {
		return errors.New("monotonic advance V2 intent embedded authority is invalid")
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
		return errors.New("monotonic advance V2 intent checkpoint, request, or transition binding mismatch")
	}
	switch intent.Root {
	case AdvanceRootThreadRiskV2:
		if intent.Transition.ThreadRiskGenesis != nil {
			genesis := intent.Transition.ThreadRiskGenesis
			first := genesis.FirstIndex
			if genesis.EnrollmentCheckpoint != previous || first.IndexDigest != request.NextStateDigest ||
				first.MutationID != request.MutationID || first.AuthorityKeyID != request.AuthorityKeyID ||
				first.AuthorityPublicKey != request.AuthorityPublicKey {
				return errors.New("thread risk genesis V2 intent is not exact")
			}
			return nil
		}
		previousIndex := intent.Transition.ThreadRisk.PreviousIndex
		nextIndex := intent.Transition.ThreadRisk.NextIndex
		if domainsecurity.ValidateThreadRiskAuthorityIndexCheckpointV1(previousIndex, previous) != nil ||
			nextIndex.InstallationID != request.InstallationID || nextIndex.EnrollmentID != request.EnrollmentID ||
			nextIndex.Namespace != request.Namespace || nextIndex.Generation != request.NextGeneration ||
			nextIndex.IndexDigest != request.NextStateDigest || nextIndex.MutationID != request.MutationID ||
			nextIndex.AuthorityKeyID != request.AuthorityKeyID || nextIndex.AuthorityPublicKey != request.AuthorityPublicKey {
			return errors.New("thread risk successor V2 intent is not exact")
		}
	case AdvanceRootEvidenceGenesisV2:
		genesis := intent.Transition.EvidenceGenesis
		first := genesis.FirstBundle
		if genesis.EnrollmentCheckpoint != previous || first.RecordDigest != request.NextStateDigest ||
			first.MutationID != request.MutationID || first.AuthorityKeyID != request.AuthorityKeyID ||
			first.AuthorityPublicKey != request.AuthorityPublicKey {
			return errors.New("evidence authority genesis V2 intent is not exact")
		}
	case AdvanceRootDatasetSnapshotV2, AdvanceRootEvidenceRegistryV2, AdvanceRootPublicationV2:
		previousBundle := intent.Transition.EvidenceBundle.PreviousBundle
		nextBundle := intent.Transition.EvidenceBundle.NextBundle
		if domainevidence.ValidateEvidenceAuthorityBundleCheckpointV1(previousBundle, previous) != nil ||
			nextBundle.InstallationID != request.InstallationID || nextBundle.EnrollmentID != request.EnrollmentID ||
			nextBundle.Namespace != request.Namespace || nextBundle.Generation != request.NextGeneration ||
			nextBundle.RecordDigest != request.NextStateDigest || nextBundle.MutationID != request.MutationID ||
			nextBundle.AuthorityKeyID != request.AuthorityKeyID || nextBundle.AuthorityPublicKey != request.AuthorityPublicKey {
			return errors.New("evidence successor V2 intent is not exact")
		}
	default:
		return errors.New("monotonic advance V2 intent root is unknown")
	}
	return nil
}

func monotonicAdvanceIntentDigestV2(intent MonotonicAdvanceIntentV2) string {
	intent.RecordDigest = ""
	body, _ := json.Marshal(intent)
	payload := append([]byte(nil), monotonicAdvanceIntentDigestDomainV2...)
	return domainsecurity.SHA256Hex(append(payload, body...))
}
