package security

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	MonotonicHeadMutationResolveRequestPurposeV1 = "analytix.monotonic-head-mutation-resolve-request/v1"
	MonotonicHeadMutationResolutionPurposeV1     = "analytix.monotonic-head-mutation-resolution/v1"

	MonotonicHeadMutationResolutionCommittedV1 = "committed"
	MonotonicHeadMutationResolutionAbsentV1    = "absent"
)

var (
	monotonicHeadMutationResolveRequestSignatureDomainV1 = []byte("analytix.monotonic-head-mutation-resolve-request/v1\x00")
	monotonicHeadMutationResolutionSignatureDomainV1     = []byte("analytix.monotonic-head-mutation-resolution/v1\x00")
)

// MonotonicHeadMutationResolveRequestV1 asks an enrolled witness for the
// exact durable outcome of one previously signed CAS request. The fresh
// challenge prevents an old authentic response from being treated as a
// current recovery observation.
type MonotonicHeadMutationResolveRequestV1 struct {
	SchemaVersion      int                           `json:"schemaVersion"`
	Purpose            string                        `json:"purpose"`
	ChallengeNonce     string                        `json:"challengeNonce"`
	AdvanceRequest     MonotonicHeadAdvanceRequestV1 `json:"advanceRequest"`
	AuthorityAlgorithm string                        `json:"authorityAlgorithm"`
	AuthorityKeyID     string                        `json:"authorityKeyId"`
	AuthorityPublicKey string                        `json:"authorityPublicKey"`
	AuthoritySignature string                        `json:"authoritySignature"`
	RequestDigest      string                        `json:"requestDigest"`
}

type MonotonicHeadCommittedMutationV1 struct {
	AdvanceRequest MonotonicHeadAdvanceRequestV1 `json:"advanceRequest"`
	AdvanceReceipt MonotonicHeadAdvanceReceiptV1 `json:"advanceReceipt"`
}

// MonotonicHeadMutationResolutionV1 is a strict discriminated union. An
// absent response is signed evidence only that this exact lookup found no
// retained mutation; it is never authority to delete the local intent, retry
// with a new mutation ID, create a superseded settlement, or publish.
type MonotonicHeadMutationResolutionV1 struct {
	SchemaVersion        int                               `json:"schemaVersion"`
	Purpose              string                            `json:"purpose"`
	ResolveRequestDigest string                            `json:"resolveRequestDigest"`
	ChallengeNonce       string                            `json:"challengeNonce"`
	Status               string                            `json:"status"`
	CurrentCheckpoint    MonotonicHeadCheckpointV1         `json:"currentCheckpoint"`
	Committed            *MonotonicHeadCommittedMutationV1 `json:"committed,omitempty"`
	WitnessAlgorithm     string                            `json:"witnessAlgorithm"`
	WitnessKeyID         string                            `json:"witnessKeyId"`
	WitnessPublicKey     string                            `json:"witnessPublicKey"`
	WitnessSignature     string                            `json:"witnessSignature"`
	ResolutionDigest     string                            `json:"resolutionDigest"`
}

func NewMonotonicHeadMutationResolveRequestV1(
	advanceRequest MonotonicHeadAdvanceRequestV1,
	challengeNonce string,
	sign MonotonicHeadSignFunc,
) (MonotonicHeadMutationResolveRequestV1, error) {
	request := MonotonicHeadMutationResolveRequestV1{
		SchemaVersion:      MonotonicHeadCheckpointSchemaVersion,
		Purpose:            MonotonicHeadMutationResolveRequestPurposeV1,
		ChallengeNonce:     strings.TrimSpace(challengeNonce),
		AdvanceRequest:     advanceRequest,
		AuthorityAlgorithm: advanceRequest.AuthorityAlgorithm,
		AuthorityKeyID:     advanceRequest.AuthorityKeyID,
		AuthorityPublicKey: advanceRequest.AuthorityPublicKey,
	}
	if sign == nil || validateMonotonicHeadMutationResolveRequestUnsignedV1(request) != nil {
		return MonotonicHeadMutationResolveRequestV1{}, errors.New("monotonic head mutation resolve request input is invalid")
	}
	signature, err := sign(MonotonicHeadMutationResolveRequestSigningBytesV1(request))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return MonotonicHeadMutationResolveRequestV1{}, errors.New("monotonic head mutation resolve request signing failed")
	}
	request.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	request.RequestDigest = monotonicHeadMutationResolveRequestDigestV1(request)
	if err := ValidateMonotonicHeadMutationResolveRequestV1(request); err != nil {
		return MonotonicHeadMutationResolveRequestV1{}, err
	}
	return request, nil
}

func ValidateMonotonicHeadMutationResolveRequestV1(request MonotonicHeadMutationResolveRequestV1) error {
	if err := validateMonotonicHeadMutationResolveRequestUnsignedV1(request); err != nil {
		return err
	}
	if !isCanonicalSHA256Hex(request.RequestDigest) || request.RequestDigest != monotonicHeadMutationResolveRequestDigestV1(request) {
		return errors.New("monotonic head mutation resolve request digest is invalid")
	}
	if err := verifyMonotonicHeadSignature(
		request.AuthorityAlgorithm,
		request.AuthorityKeyID,
		request.AuthorityPublicKey,
		request.AuthoritySignature,
		MonotonicHeadMutationResolveRequestSigningBytesV1(request),
	); err != nil {
		return errors.New("monotonic head mutation resolve request signature is invalid")
	}
	return nil
}

func ValidateMonotonicHeadMutationResolveRequestForInstallationV1(
	request MonotonicHeadMutationResolveRequestV1,
	installationID, authorityKeyID string,
	authorityPublicKey []byte,
) error {
	if err := ValidateMonotonicHeadMutationResolveRequestV1(request); err != nil {
		return err
	}
	authorityPublicKey = append([]byte(nil), authorityPublicKey...)
	if len(authorityPublicKey) != ed25519.PublicKeySize || strings.TrimSpace(authorityKeyID) != SHA256Hex(authorityPublicKey) ||
		request.AdvanceRequest.InstallationID != strings.TrimSpace(installationID) ||
		request.AuthorityKeyID != strings.TrimSpace(authorityKeyID) ||
		request.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(authorityPublicKey) ||
		ValidateMonotonicHeadAdvanceRequestForInstallationV1(
			request.AdvanceRequest,
			installationID,
			authorityKeyID,
			authorityPublicKey,
		) != nil {
		return errors.New("monotonic head mutation resolve request installation authority mismatch")
	}
	return nil
}

func NewMonotonicHeadMutationResolutionV1(
	request MonotonicHeadMutationResolveRequestV1,
	currentCheckpoint MonotonicHeadCheckpointV1,
	committed *MonotonicHeadCommittedMutationV1,
	sign MonotonicHeadSignFunc,
) (MonotonicHeadMutationResolutionV1, error) {
	status := MonotonicHeadMutationResolutionAbsentV1
	if committed != nil {
		status = MonotonicHeadMutationResolutionCommittedV1
	}
	resolution := MonotonicHeadMutationResolutionV1{
		SchemaVersion:        MonotonicHeadCheckpointSchemaVersion,
		Purpose:              MonotonicHeadMutationResolutionPurposeV1,
		ResolveRequestDigest: request.RequestDigest,
		ChallengeNonce:       request.ChallengeNonce,
		Status:               status,
		CurrentCheckpoint:    currentCheckpoint,
		Committed:            committed,
		WitnessAlgorithm:     currentCheckpoint.WitnessAlgorithm,
		WitnessKeyID:         currentCheckpoint.WitnessKeyID,
		WitnessPublicKey:     currentCheckpoint.WitnessPublicKey,
	}
	if sign == nil || ValidateMonotonicHeadMutationResolveRequestV1(request) != nil ||
		validateMonotonicHeadMutationResolutionUnsignedV1(resolution) != nil {
		return MonotonicHeadMutationResolutionV1{}, errors.New("monotonic head mutation resolution input is invalid")
	}
	signature, err := sign(MonotonicHeadMutationResolutionSigningBytesV1(resolution))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return MonotonicHeadMutationResolutionV1{}, errors.New("monotonic head mutation resolution signing failed")
	}
	resolution.WitnessSignature = base64.RawURLEncoding.EncodeToString(signature)
	resolution.ResolutionDigest = monotonicHeadMutationResolutionDigestV1(resolution)
	if err := ValidateMonotonicHeadMutationResolutionV1(resolution); err != nil {
		return MonotonicHeadMutationResolutionV1{}, err
	}
	return resolution, nil
}

func ValidateMonotonicHeadMutationResolutionV1(resolution MonotonicHeadMutationResolutionV1) error {
	if err := validateMonotonicHeadMutationResolutionUnsignedV1(resolution); err != nil {
		return err
	}
	if !isCanonicalSHA256Hex(resolution.ResolutionDigest) || resolution.ResolutionDigest != monotonicHeadMutationResolutionDigestV1(resolution) {
		return errors.New("monotonic head mutation resolution digest is invalid")
	}
	if err := verifyMonotonicHeadSignature(
		resolution.WitnessAlgorithm,
		resolution.WitnessKeyID,
		resolution.WitnessPublicKey,
		resolution.WitnessSignature,
		MonotonicHeadMutationResolutionSigningBytesV1(resolution),
	); err != nil {
		return errors.New("monotonic head mutation resolution signature is invalid")
	}
	return nil
}

func ValidateMonotonicHeadMutationResolutionForRequestV1(
	resolution MonotonicHeadMutationResolutionV1,
	request MonotonicHeadMutationResolveRequestV1,
	installationID, authorityKeyID string,
	authorityPublicKey []byte,
	enrollmentID, witnessKeyID string,
	witnessPublicKey []byte,
) error {
	if err := ValidateMonotonicHeadMutationResolutionV1(resolution); err != nil {
		return err
	}
	if err := ValidateMonotonicHeadMutationResolveRequestForInstallationV1(
		request,
		installationID,
		authorityKeyID,
		authorityPublicKey,
	); err != nil {
		return err
	}
	if resolution.ResolveRequestDigest != request.RequestDigest || resolution.ChallengeNonce != request.ChallengeNonce ||
		resolution.CurrentCheckpoint.Namespace != request.AdvanceRequest.Namespace ||
		resolution.CurrentCheckpoint.Generation < request.AdvanceRequest.ExpectedGeneration {
		return errors.New("monotonic head mutation resolution request binding mismatch")
	}
	if err := ValidateMonotonicHeadCheckpointForWitnessV1(
		resolution.CurrentCheckpoint,
		installationID,
		enrollmentID,
		witnessKeyID,
		witnessPublicKey,
	); err != nil {
		return err
	}
	if resolution.CurrentCheckpoint.Generation == request.AdvanceRequest.ExpectedGeneration &&
		(resolution.CurrentCheckpoint.CheckpointDigest != request.AdvanceRequest.ExpectedCheckpointDigest ||
			resolution.CurrentCheckpoint.CurrentStateDigest != request.AdvanceRequest.ExpectedStateDigest ||
			resolution.CurrentCheckpoint.FenceNonce != request.AdvanceRequest.ExpectedFenceNonce) {
		return errors.New("monotonic head mutation resolution current checkpoint conflicts with request anchor")
	}
	if resolution.Status != MonotonicHeadMutationResolutionCommittedV1 {
		return nil
	}
	committed := resolution.Committed
	if committed == nil || committed.AdvanceRequest != request.AdvanceRequest ||
		committed.AdvanceReceipt.RequestDigest != request.AdvanceRequest.RequestDigest ||
		committed.AdvanceReceipt.MutationID != request.AdvanceRequest.MutationID ||
		ValidateMonotonicHeadCheckpointForWitnessV1(
			committed.AdvanceReceipt.Checkpoint,
			installationID,
			enrollmentID,
			witnessKeyID,
			witnessPublicKey,
		) != nil || resolution.CurrentCheckpoint.Generation < committed.AdvanceReceipt.Checkpoint.Generation {
		return errors.New("monotonic head committed mutation resolution is not exact")
	}
	if resolution.CurrentCheckpoint.Generation == committed.AdvanceReceipt.Checkpoint.Generation &&
		resolution.CurrentCheckpoint != committed.AdvanceReceipt.Checkpoint {
		return errors.New("monotonic head mutation resolution equivocates at committed generation")
	}
	return nil
}

func ParseMonotonicHeadMutationResolveRequestV1(body []byte) (MonotonicHeadMutationResolveRequestV1, error) {
	var request MonotonicHeadMutationResolveRequestV1
	if err := decodeCanonicalMonotonicHeadRecoveryV1(body, &request); err != nil {
		return MonotonicHeadMutationResolveRequestV1{}, err
	}
	return request, ValidateMonotonicHeadMutationResolveRequestV1(request)
}

func ParseMonotonicHeadMutationResolutionV1(body []byte) (MonotonicHeadMutationResolutionV1, error) {
	var resolution MonotonicHeadMutationResolutionV1
	if err := decodeCanonicalMonotonicHeadRecoveryV1(body, &resolution); err != nil {
		return MonotonicHeadMutationResolutionV1{}, err
	}
	return resolution, ValidateMonotonicHeadMutationResolutionV1(resolution)
}

func MonotonicHeadMutationResolveRequestV1Bytes(request MonotonicHeadMutationResolveRequestV1) ([]byte, error) {
	if err := ValidateMonotonicHeadMutationResolveRequestV1(request); err != nil {
		return nil, err
	}
	return json.Marshal(request)
}

func MonotonicHeadMutationResolutionV1Bytes(resolution MonotonicHeadMutationResolutionV1) ([]byte, error) {
	if err := ValidateMonotonicHeadMutationResolutionV1(resolution); err != nil {
		return nil, err
	}
	return json.Marshal(resolution)
}

func MonotonicHeadMutationResolveRequestSigningBytesV1(request MonotonicHeadMutationResolveRequestV1) []byte {
	request.AuthoritySignature = ""
	request.RequestDigest = ""
	body, _ := json.Marshal(request)
	return monotonicHeadSigningPayload(monotonicHeadMutationResolveRequestSignatureDomainV1, body)
}

func MonotonicHeadMutationResolutionSigningBytesV1(resolution MonotonicHeadMutationResolutionV1) []byte {
	resolution.WitnessSignature = ""
	resolution.ResolutionDigest = ""
	body, _ := json.Marshal(resolution)
	return monotonicHeadSigningPayload(monotonicHeadMutationResolutionSignatureDomainV1, body)
}

func validateMonotonicHeadMutationResolveRequestUnsignedV1(request MonotonicHeadMutationResolveRequestV1) error {
	if request.SchemaVersion != MonotonicHeadCheckpointSchemaVersion || request.Purpose != MonotonicHeadMutationResolveRequestPurposeV1 ||
		!isCanonicalSHA256Hex(request.ChallengeNonce) || ValidateMonotonicHeadAdvanceRequestV1(request.AdvanceRequest) != nil ||
		request.AuthorityAlgorithm != MonotonicHeadAlgorithm || request.AuthorityAlgorithm != request.AdvanceRequest.AuthorityAlgorithm ||
		request.AuthorityKeyID != request.AdvanceRequest.AuthorityKeyID || request.AuthorityPublicKey != request.AdvanceRequest.AuthorityPublicKey {
		return errors.New("monotonic head mutation resolve request is invalid")
	}
	return nil
}

func validateMonotonicHeadMutationResolutionUnsignedV1(resolution MonotonicHeadMutationResolutionV1) error {
	if resolution.SchemaVersion != MonotonicHeadCheckpointSchemaVersion || resolution.Purpose != MonotonicHeadMutationResolutionPurposeV1 ||
		!isCanonicalSHA256Hex(resolution.ResolveRequestDigest) || !isCanonicalSHA256Hex(resolution.ChallengeNonce) ||
		ValidateMonotonicHeadCheckpointV1(resolution.CurrentCheckpoint) != nil || resolution.WitnessAlgorithm != MonotonicHeadAlgorithm ||
		resolution.WitnessKeyID != resolution.CurrentCheckpoint.WitnessKeyID || resolution.WitnessPublicKey != resolution.CurrentCheckpoint.WitnessPublicKey {
		return errors.New("monotonic head mutation resolution is invalid")
	}
	switch resolution.Status {
	case MonotonicHeadMutationResolutionCommittedV1:
		if resolution.Committed == nil || ValidateMonotonicHeadAdvanceRequestV1(resolution.Committed.AdvanceRequest) != nil ||
			ValidateMonotonicHeadAdvanceReceiptV1(resolution.Committed.AdvanceReceipt) != nil ||
			resolution.Committed.AdvanceReceipt.RequestDigest != resolution.Committed.AdvanceRequest.RequestDigest ||
			resolution.Committed.AdvanceReceipt.MutationID != resolution.Committed.AdvanceRequest.MutationID {
			return errors.New("monotonic head committed mutation resolution is invalid")
		}
	case MonotonicHeadMutationResolutionAbsentV1:
		if resolution.Committed != nil {
			return errors.New("monotonic head absent mutation resolution carries a committed mutation")
		}
	default:
		return errors.New("monotonic head mutation resolution status is unknown")
	}
	return nil
}

func decodeCanonicalMonotonicHeadRecoveryV1(body []byte, target any) error {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 256 << 10, MaxDepth: 24, MaxTokens: 20_000, MaxStringBytes: 64 * 1024,
	}); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("monotonic head recovery contract contains trailing JSON")
	}
	canonical, err := json.Marshal(target)
	if err != nil || !bytes.Equal(body, canonical) {
		return errors.New("monotonic head recovery contract is not canonically encoded")
	}
	return nil
}

func monotonicHeadMutationResolveRequestDigestV1(request MonotonicHeadMutationResolveRequestV1) string {
	request.RequestDigest = ""
	body, _ := json.Marshal(request)
	return SHA256Hex(body)
}

func monotonicHeadMutationResolutionDigestV1(resolution MonotonicHeadMutationResolutionV1) string {
	resolution.ResolutionDigest = ""
	body, _ := json.Marshal(resolution)
	return SHA256Hex(body)
}
