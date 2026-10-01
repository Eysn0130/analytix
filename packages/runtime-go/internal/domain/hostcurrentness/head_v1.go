package hostcurrentness

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	HeadSchemaVersionV1 = 1
	HeadPurposeV1       = "analytix.host-local-evidence-head/v1"
	HeadModeV1          = "host_local"
	HeadAlgorithmV1     = "Ed25519"
	maxHeadBytesV1      = 64 << 10
)

var headSignatureDomainV1 = []byte("analytix.host-local-evidence-head/v1\x00")

// HeadV1 selects the existing DSV2, Evidence Registry, and publication roots
// for a host-local profile. Generation zero is also its durable mode commitment.
// Its signature and local CAS do not prove freshness after whole-profile rollback.
type HeadV1 struct {
	SchemaVersion               int    `json:"schemaVersion"`
	Purpose                     string `json:"purpose"`
	Mode                        string `json:"mode"`
	InstallationID              string `json:"installationId"`
	RootBindingDigest           string `json:"rootBindingDigest"`
	Generation                  uint64 `json:"generation"`
	PreviousHeadDigest          string `json:"previousHeadDigest"`
	MutationID                  string `json:"mutationId"`
	DatasetSnapshotIndexDigest  string `json:"datasetSnapshotIndexDigest"`
	DatasetSnapshotCount        uint64 `json:"datasetSnapshotCount"`
	EvidenceRegistryIndexDigest string `json:"evidenceRegistryIndexDigest"`
	EvidenceRegistryCount       uint64 `json:"evidenceRegistryCount"`
	PublicationIndexDigest      string `json:"publicationIndexDigest"`
	PublicationCount            uint64 `json:"publicationCount"`
	AuthorityAlgorithm          string `json:"authorityAlgorithm"`
	AuthorityKeyID              string `json:"authorityKeyId"`
	AuthorityPublicKey          string `json:"authorityPublicKey"`
	AuthoritySignature          string `json:"authoritySignature"`
	RecordDigest                string `json:"recordDigest"`
}

type HeadInputV1 struct {
	InstallationID              string
	RootBindingDigest           string
	Generation                  uint64
	PreviousHeadDigest          string
	MutationID                  string
	DatasetSnapshotIndexDigest  string
	DatasetSnapshotCount        uint64
	EvidenceRegistryIndexDigest string
	EvidenceRegistryCount       uint64
	PublicationIndexDigest      string
	PublicationCount            uint64
	AuthorityKeyID              string
	AuthorityPublicKey          []byte
}

func NewHeadV1(input HeadInputV1, sign func([]byte) ([]byte, error)) (HeadV1, error) {
	key := append([]byte(nil), input.AuthorityPublicKey...)
	head := HeadV1{
		SchemaVersion: HeadSchemaVersionV1, Purpose: HeadPurposeV1, Mode: HeadModeV1,
		InstallationID: input.InstallationID, RootBindingDigest: input.RootBindingDigest,
		Generation: input.Generation, PreviousHeadDigest: input.PreviousHeadDigest,
		MutationID: input.MutationID, DatasetSnapshotIndexDigest: input.DatasetSnapshotIndexDigest,
		DatasetSnapshotCount:        input.DatasetSnapshotCount,
		EvidenceRegistryIndexDigest: input.EvidenceRegistryIndexDigest,
		EvidenceRegistryCount:       input.EvidenceRegistryCount,
		PublicationIndexDigest:      input.PublicationIndexDigest,
		PublicationCount:            input.PublicationCount,
		AuthorityAlgorithm:          HeadAlgorithmV1, AuthorityKeyID: input.AuthorityKeyID,
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(key),
	}
	if sign == nil || validateUnsignedHeadV1(head) != nil {
		return HeadV1{}, errors.New("host-local head input is invalid")
	}
	signature, err := sign(HeadSigningBytesV1(head))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return HeadV1{}, errors.New("host-local head signing failed")
	}
	head.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	head.RecordDigest = headDigestV1(head)
	if err := ValidateHeadV1(head); err != nil {
		return HeadV1{}, err
	}
	return head, nil
}

func ValidateHeadV1(head HeadV1) error {
	if err := validateUnsignedHeadV1(head); err != nil {
		return err
	}
	key, keyErr := base64.RawURLEncoding.DecodeString(head.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(head.AuthoritySignature)
	if keyErr != nil || signatureErr != nil || len(key) != ed25519.PublicKeySize ||
		len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(key) != head.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != head.AuthoritySignature ||
		!ed25519.Verify(key, HeadSigningBytesV1(head), signature) ||
		head.RecordDigest != headDigestV1(head) {
		return errors.New("host-local head signature or digest is invalid")
	}
	return nil
}

func ValidateHeadForInstallationV1(head HeadV1, installationID, rootBindingDigest, authorityKeyID string, authorityPublicKey []byte) error {
	if ValidateHeadV1(head) != nil || head.InstallationID != installationID ||
		head.RootBindingDigest != rootBindingDigest || head.AuthorityKeyID != authorityKeyID ||
		head.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(authorityPublicKey) ||
		authorityKeyID != domainsecurity.SHA256Hex(authorityPublicKey) {
		return errors.New("host-local head installation binding is invalid")
	}
	return nil
}

func ValidateHeadTransitionV1(previous, next HeadV1) error {
	if ValidateHeadV1(previous) != nil || ValidateHeadV1(next) != nil ||
		previous.Generation == ^uint64(0) || next.Generation != previous.Generation+1 ||
		next.PreviousHeadDigest != previous.RecordDigest || next.MutationID == previous.MutationID ||
		next.InstallationID != previous.InstallationID || next.RootBindingDigest != previous.RootBindingDigest ||
		next.AuthorityKeyID != previous.AuthorityKeyID || next.AuthorityPublicKey != previous.AuthorityPublicKey {
		return errors.New("host-local head predecessor is invalid")
	}
	changes := 0
	for _, pair := range [][3]uint64{
		{previous.DatasetSnapshotCount, next.DatasetSnapshotCount, boolToUint64(previous.DatasetSnapshotIndexDigest != next.DatasetSnapshotIndexDigest)},
		{previous.EvidenceRegistryCount, next.EvidenceRegistryCount, boolToUint64(previous.EvidenceRegistryIndexDigest != next.EvidenceRegistryIndexDigest)},
		{previous.PublicationCount, next.PublicationCount, boolToUint64(previous.PublicationIndexDigest != next.PublicationIndexDigest)},
	} {
		if pair[0] == pair[1] && pair[2] == 0 {
			continue
		}
		if pair[0] == ^uint64(0) || pair[1] != pair[0]+1 || pair[2] != 1 {
			return errors.New("host-local head child transition is invalid")
		}
		changes++
	}
	if changes != 1 {
		return errors.New("host-local head must advance exactly one child")
	}
	return nil
}

func HeadSigningBytesV1(head HeadV1) []byte {
	head.AuthoritySignature, head.RecordDigest = "", ""
	body, _ := json.Marshal(head)
	digest := sha256.Sum256(body)
	return append(append([]byte(nil), headSignatureDomainV1...), digest[:]...)
}

func HeadBytesV1(head HeadV1) ([]byte, error) {
	if err := ValidateHeadV1(head); err != nil {
		return nil, err
	}
	return json.Marshal(head)
}

func ParseHeadV1(body []byte) (HeadV1, error) {
	var head HeadV1
	object, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxHeadBytesV1, MaxDepth: 4, MaxTokens: 128, MaxStringBytes: 4 << 10,
	})
	if err != nil || len(object) != 19 {
		return HeadV1{}, errors.New("host-local head fields are invalid")
	}
	for _, name := range [...]string{
		"schemaVersion", "purpose", "mode", "installationId", "rootBindingDigest", "generation",
		"previousHeadDigest", "mutationId", "datasetSnapshotIndexDigest", "datasetSnapshotCount",
		"evidenceRegistryIndexDigest", "evidenceRegistryCount", "publicationIndexDigest", "publicationCount",
		"authorityAlgorithm", "authorityKeyId", "authorityPublicKey", "authoritySignature", "recordDigest",
	} {
		if _, ok := object[name]; !ok {
			return HeadV1{}, errors.New("host-local head field is missing")
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&head); err != nil {
		return HeadV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return HeadV1{}, errors.New("host-local head has trailing JSON")
	}
	canonical, _ := json.Marshal(head)
	if !bytes.Equal(canonical, body) {
		return HeadV1{}, errors.New("host-local head is not canonical")
	}
	return head, ValidateHeadV1(head)
}

func validateUnsignedHeadV1(head HeadV1) error {
	key, err := base64.RawURLEncoding.DecodeString(head.AuthorityPublicKey)
	if head.SchemaVersion != HeadSchemaVersionV1 || head.Purpose != HeadPurposeV1 || head.Mode != HeadModeV1 ||
		!domainsecurity.IsSHA256Hex(head.InstallationID) || !domainsecurity.IsSHA256Hex(head.RootBindingDigest) ||
		!domainsecurity.IsSHA256Hex(head.MutationID) ||
		!domainsecurity.IsSHA256Hex(head.DatasetSnapshotIndexDigest) ||
		!domainsecurity.IsSHA256Hex(head.EvidenceRegistryIndexDigest) ||
		!domainsecurity.IsSHA256Hex(head.PublicationIndexDigest) ||
		head.AuthorityAlgorithm != HeadAlgorithmV1 || err != nil || len(key) != ed25519.PublicKeySize ||
		base64.RawURLEncoding.EncodeToString(key) != head.AuthorityPublicKey ||
		head.AuthorityKeyID != domainsecurity.SHA256Hex(key) ||
		(head.Generation == 0 && (head.PreviousHeadDigest != "" || head.DatasetSnapshotCount != 0 ||
			head.EvidenceRegistryCount != 0 || head.PublicationCount != 0)) ||
		(head.Generation > 0 && !domainsecurity.IsSHA256Hex(head.PreviousHeadDigest)) {
		return errors.New("host-local head structure is invalid")
	}
	return nil
}

func headDigestV1(head HeadV1) string {
	head.RecordDigest = ""
	body, _ := json.Marshal(head)
	return domainsecurity.SHA256Hex(body)
}

func boolToUint64(value bool) uint64 {
	if value {
		return 1
	}
	return 0
}
