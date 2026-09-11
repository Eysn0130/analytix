package event

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	AcceptedFinalDeliverySealSchemaVersion = "accepted-final-delivery-seal.v1"
	AcceptedFinalDeliverySealPurpose       = "analytix.accepted-final-delivery-seal/v1"
	AcceptedFinalDeliverySealAlgorithm     = "Ed25519"
)

var acceptedFinalDeliverySealIDDomain = []byte("analytix/accepted-final-delivery-seal-id/v1\x00")
var acceptedFinalDeliverySealSignatureDomain = []byte("analytix/accepted-final-delivery-seal-signature/v1\x00")

// AcceptedFinalDeliverySealV1 is a transport attestation issued only after an
// accepted-final event manifest has durable sequence numbers and exact
// readback. It is a child of the existing committed disposition authority;
// it does not make, upgrade, or replace a Final Evidence Gate decision.
type AcceptedFinalDeliverySealV1 struct {
	SchemaVersion                  string `json:"schemaVersion"`
	Purpose                        string `json:"purpose"`
	SealID                         string `json:"sealId"`
	ThreadID                       string `json:"threadId"`
	TurnID                         string `json:"turnId"`
	PublicationCommitID            string `json:"publicationCommitId"`
	AcceptedFinalDispositionDigest string `json:"acceptedFinalDispositionDigest"`
	TerminalDispositionID          string `json:"terminalDispositionId"`
	EventManifestDigest            string `json:"eventManifestDigest"`
	SequencedEventsDigest          string `json:"sequencedEventsDigest"`
	BatchID                        string `json:"batchId"`
	FirstSeq                       int    `json:"firstSeq"`
	LastSeq                        int    `json:"lastSeq"`
	Timestamp                      string `json:"timestamp"`
	AuthorityAlgorithm             string `json:"authorityAlgorithm"`
	AuthorityKeyID                 string `json:"authorityKeyId"`
	AuthorityPublicKey             string `json:"authorityPublicKey"`
	AuthoritySignature             string `json:"authoritySignature"`
}

type AcceptedFinalDeliverySealInputV1 struct {
	ThreadID                       string
	TurnID                         string
	PublicationCommitID            string
	AcceptedFinalDispositionDigest string
	TerminalDispositionID          string
	EventManifestDigest            string
	SequencedEventsDigest          string
	BatchID                        string
	FirstSeq                       int
	LastSeq                        int
	Timestamp                      string
	AuthorityKeyID                 string
	AuthorityPublicKey             []byte
}

type AcceptedFinalDeliverySignFuncV1 func([]byte) ([]byte, error)

func NewAcceptedFinalDeliverySealV1(
	input AcceptedFinalDeliverySealInputV1,
	sign AcceptedFinalDeliverySignFuncV1,
) (AcceptedFinalDeliverySealV1, error) {
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	seal := AcceptedFinalDeliverySealV1{
		SchemaVersion: AcceptedFinalDeliverySealSchemaVersion, Purpose: AcceptedFinalDeliverySealPurpose,
		ThreadID: strings.TrimSpace(input.ThreadID), TurnID: strings.TrimSpace(input.TurnID),
		PublicationCommitID:            strings.TrimSpace(input.PublicationCommitID),
		AcceptedFinalDispositionDigest: strings.TrimSpace(input.AcceptedFinalDispositionDigest),
		TerminalDispositionID:          strings.TrimSpace(input.TerminalDispositionID),
		EventManifestDigest:            strings.TrimSpace(input.EventManifestDigest),
		SequencedEventsDigest:          strings.TrimSpace(input.SequencedEventsDigest), BatchID: strings.TrimSpace(input.BatchID),
		FirstSeq: input.FirstSeq, LastSeq: input.LastSeq, Timestamp: strings.TrimSpace(input.Timestamp),
		AuthorityAlgorithm: AcceptedFinalDeliverySealAlgorithm, AuthorityKeyID: strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	if sign == nil || len(publicKey) != ed25519.PublicKeySize ||
		seal.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) {
		return AcceptedFinalDeliverySealV1{}, errors.New("accepted final delivery seal authority is invalid")
	}
	seal.SealID = acceptedFinalDeliverySealIDV1(seal)
	signature, err := sign(AcceptedFinalDeliverySealSigningBytesV1(seal))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return AcceptedFinalDeliverySealV1{}, errors.New("accepted final delivery seal signing failed")
	}
	seal.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	if err := ValidateAcceptedFinalDeliverySealV1(seal); err != nil {
		return AcceptedFinalDeliverySealV1{}, err
	}
	return seal, nil
}

func ValidateAcceptedFinalDeliverySealV1(seal AcceptedFinalDeliverySealV1) error {
	if seal.SchemaVersion != AcceptedFinalDeliverySealSchemaVersion || seal.Purpose != AcceptedFinalDeliverySealPurpose ||
		!domainsecurity.IsSHA256Hex(seal.SealID) || strings.TrimSpace(seal.ThreadID) == "" || strings.TrimSpace(seal.TurnID) == "" ||
		!domainsecurity.IsSHA256Hex(seal.PublicationCommitID) || !domainsecurity.IsSHA256Hex(seal.AcceptedFinalDispositionDigest) ||
		!domainsecurity.IsSHA256Hex(seal.TerminalDispositionID) || !domainsecurity.IsSHA256Hex(seal.EventManifestDigest) ||
		!domainsecurity.IsSHA256Hex(seal.SequencedEventsDigest) || !domainsecurity.IsSHA256Hex(seal.BatchID) ||
		seal.FirstSeq <= 0 || seal.LastSeq < seal.FirstSeq || seal.AuthorityAlgorithm != AcceptedFinalDeliverySealAlgorithm ||
		!domainsecurity.IsSHA256Hex(seal.AuthorityKeyID) {
		return errors.New("accepted final delivery seal is incomplete")
	}
	if _, err := time.Parse(time.RFC3339Nano, seal.Timestamp); err != nil {
		return errors.New("accepted final delivery seal timestamp is invalid")
	}
	publicKey, publicErr := base64.RawURLEncoding.Strict().DecodeString(seal.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.Strict().DecodeString(seal.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != seal.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != seal.AuthoritySignature ||
		domainsecurity.SHA256Hex(publicKey) != seal.AuthorityKeyID || seal.SealID != acceptedFinalDeliverySealIDV1(seal) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), AcceptedFinalDeliverySealSigningBytesV1(seal), signature) {
		return errors.New("accepted final delivery seal integrity is invalid")
	}
	return nil
}

func AcceptedFinalDeliverySealSigningBytesV1(seal AcceptedFinalDeliverySealV1) []byte {
	seal.AuthoritySignature = ""
	body, _ := json.Marshal(seal)
	digest := sha256.Sum256(body)
	out := make([]byte, 0, len(acceptedFinalDeliverySealSignatureDomain)+len(digest))
	out = append(out, acceptedFinalDeliverySealSignatureDomain...)
	out = append(out, digest[:]...)
	return out
}

func AcceptedFinalDeliverySealV1Map(seal AcceptedFinalDeliverySealV1) map[string]any {
	if ValidateAcceptedFinalDeliverySealV1(seal) != nil {
		return nil
	}
	body, _ := json.Marshal(seal)
	var out map[string]any
	if json.Unmarshal(body, &out) != nil {
		return nil
	}
	return out
}

func ParseAcceptedFinalDeliverySealV1(value any) (AcceptedFinalDeliverySealV1, error) {
	body, err := json.Marshal(value)
	if err != nil || len(body) == 0 {
		return AcceptedFinalDeliverySealV1{}, errors.New("accepted final delivery seal is unavailable")
	}
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 8 << 10, MaxDepth: 2, MaxTokens: 64, MaxStringBytes: 4 << 10,
	}); err != nil {
		return AcceptedFinalDeliverySealV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var seal AcceptedFinalDeliverySealV1
	if err := decoder.Decode(&seal); err != nil {
		return AcceptedFinalDeliverySealV1{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return AcceptedFinalDeliverySealV1{}, errors.New("accepted final delivery seal contains trailing JSON")
	}
	if err := ValidateAcceptedFinalDeliverySealV1(seal); err != nil {
		return AcceptedFinalDeliverySealV1{}, err
	}
	return seal, nil
}

func acceptedFinalDeliverySealIDV1(seal AcceptedFinalDeliverySealV1) string {
	seal.SealID = ""
	seal.AuthoritySignature = ""
	body, _ := json.Marshal(seal)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), acceptedFinalDeliverySealIDDomain...), body...))
}
