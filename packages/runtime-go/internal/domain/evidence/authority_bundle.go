package evidence

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	EvidenceAuthorityBundleSchemaVersion = 1
	EvidenceAuthorityBundlePurpose       = "analytix.evidence-authority-bundle/v1"
	EvidenceAuthorityBundleAlgorithm     = "Ed25519"

	// EvidenceAuthorityBundleWitnessNamespaceV1 is deliberately the one
	// evidence-authority namespace enrolled by the generic monotonic witness.
	// Dataset snapshot selection, evidence registry membership, and final
	// publication therefore cannot race through independent local "current"
	// files or overwrite one another at the witness.
	EvidenceAuthorityBundleWitnessNamespaceV1 = domainsecurity.EvidenceRegistryAuthorityNamespaceV1
)

var evidenceAuthorityBundleSignatureDomain = []byte("analytix.evidence-authority-bundle/v1\x00")

// EvidenceAuthorityBundleV1 is the installation-signed state committed to the
// evidence monotonic witness. Its signature proves authenticity only;
// freshness requires an enrolled, freshly observed monotonic checkpoint whose
// CurrentStateDigest is this bundle's RecordDigest.
type EvidenceAuthorityBundleV1 struct {
	SchemaVersion               int    `json:"schemaVersion"`
	Purpose                     string `json:"purpose"`
	InstallationID              string `json:"installationId"`
	EnrollmentID                string `json:"enrollmentId"`
	Namespace                   string `json:"namespace"`
	Generation                  uint64 `json:"generation"`
	PreviousBundleDigest        string `json:"previousBundleDigest"`
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

type EvidenceAuthorityBundleInputV1 struct {
	InstallationID              string
	EnrollmentID                string
	Generation                  uint64
	PreviousBundleDigest        string
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

type EvidenceAuthorityBundleSignFunc func([]byte) ([]byte, error)

func NewEvidenceAuthorityBundleV1(input EvidenceAuthorityBundleInputV1, sign EvidenceAuthorityBundleSignFunc) (EvidenceAuthorityBundleV1, error) {
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	bundle := EvidenceAuthorityBundleV1{
		SchemaVersion:               EvidenceAuthorityBundleSchemaVersion,
		Purpose:                     EvidenceAuthorityBundlePurpose,
		InstallationID:              strings.TrimSpace(input.InstallationID),
		EnrollmentID:                strings.TrimSpace(input.EnrollmentID),
		Namespace:                   EvidenceAuthorityBundleWitnessNamespaceV1,
		Generation:                  input.Generation,
		PreviousBundleDigest:        strings.TrimSpace(input.PreviousBundleDigest),
		MutationID:                  strings.TrimSpace(input.MutationID),
		DatasetSnapshotIndexDigest:  strings.TrimSpace(input.DatasetSnapshotIndexDigest),
		DatasetSnapshotCount:        input.DatasetSnapshotCount,
		EvidenceRegistryIndexDigest: strings.TrimSpace(input.EvidenceRegistryIndexDigest),
		EvidenceRegistryCount:       input.EvidenceRegistryCount,
		PublicationIndexDigest:      strings.TrimSpace(input.PublicationIndexDigest),
		PublicationCount:            input.PublicationCount,
		AuthorityAlgorithm:          EvidenceAuthorityBundleAlgorithm,
		AuthorityKeyID:              strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey:          base64.RawURLEncoding.EncodeToString(publicKey),
	}
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || bundle.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) {
		return EvidenceAuthorityBundleV1{}, errors.New("evidence authority bundle signing authority is invalid")
	}
	if err := validateEvidenceAuthorityBundleUnsignedV1(bundle); err != nil {
		return EvidenceAuthorityBundleV1{}, err
	}
	signature, err := sign(EvidenceAuthorityBundleSigningBytesV1(bundle))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return EvidenceAuthorityBundleV1{}, errors.New("evidence authority bundle signing failed")
	}
	bundle.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	bundle.RecordDigest = evidenceAuthorityBundleDigestV1(bundle)
	if err := ValidateEvidenceAuthorityBundleV1(bundle); err != nil {
		return EvidenceAuthorityBundleV1{}, err
	}
	return bundle, nil
}

func ValidateEvidenceAuthorityBundleV1(bundle EvidenceAuthorityBundleV1) error {
	if err := validateEvidenceAuthorityBundleUnsignedV1(bundle); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(bundle.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(bundle.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != bundle.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != bundle.AuthoritySignature ||
		bundle.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), EvidenceAuthorityBundleSigningBytesV1(bundle), signature) {
		return errors.New("evidence authority bundle signature is invalid")
	}
	if !isCanonicalEvidenceAuthoritySHA256(bundle.RecordDigest) || bundle.RecordDigest != evidenceAuthorityBundleDigestV1(bundle) {
		return errors.New("evidence authority bundle digest is invalid")
	}
	return nil
}

// ValidateEvidenceAuthorityBundleForInstallationV1 anchors a self-authentic
// bundle to the expected installation, enrollment, and installation authority.
// A caller must not infer those anchors from the bundle itself.
func ValidateEvidenceAuthorityBundleForInstallationV1(
	bundle EvidenceAuthorityBundleV1,
	installationID, enrollmentID, authorityKeyID string,
	authorityPublicKey []byte,
) error {
	if err := ValidateEvidenceAuthorityBundleV1(bundle); err != nil {
		return err
	}
	authorityPublicKey = append([]byte(nil), authorityPublicKey...)
	if bundle.InstallationID != strings.TrimSpace(installationID) || bundle.EnrollmentID != strings.TrimSpace(enrollmentID) ||
		len(authorityPublicKey) != ed25519.PublicKeySize || strings.TrimSpace(authorityKeyID) != domainsecurity.SHA256Hex(authorityPublicKey) ||
		bundle.AuthorityKeyID != strings.TrimSpace(authorityKeyID) ||
		bundle.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(authorityPublicKey) {
		return errors.New("evidence authority bundle installation anchor mismatch")
	}
	return nil
}

// ValidateEvidenceAuthorityBundleTransitionV1 admits exactly one child-index
// append. The changed child's digest and count must move together and the
// count must advance by exactly one; the other two child pairs remain exact.
func ValidateEvidenceAuthorityBundleTransitionV1(previous, next EvidenceAuthorityBundleV1) error {
	if ValidateEvidenceAuthorityBundleV1(previous) != nil || ValidateEvidenceAuthorityBundleV1(next) != nil {
		return errors.New("evidence authority bundle transition authenticity is invalid")
	}
	if previous.Generation == ^uint64(0) || next.Generation != previous.Generation+1 ||
		next.PreviousBundleDigest != previous.RecordDigest || next.InstallationID != previous.InstallationID ||
		next.EnrollmentID != previous.EnrollmentID || next.Namespace != previous.Namespace ||
		next.AuthorityAlgorithm != previous.AuthorityAlgorithm || next.AuthorityKeyID != previous.AuthorityKeyID ||
		next.AuthorityPublicKey != previous.AuthorityPublicKey || next.MutationID == previous.MutationID {
		return errors.New("evidence authority bundle lineage is invalid")
	}
	type childTransition struct {
		previousDigest string
		previousCount  uint64
		nextDigest     string
		nextCount      uint64
	}
	changed := 0
	pairs := [...]childTransition{
		{previous.DatasetSnapshotIndexDigest, previous.DatasetSnapshotCount, next.DatasetSnapshotIndexDigest, next.DatasetSnapshotCount},
		{previous.EvidenceRegistryIndexDigest, previous.EvidenceRegistryCount, next.EvidenceRegistryIndexDigest, next.EvidenceRegistryCount},
		{previous.PublicationIndexDigest, previous.PublicationCount, next.PublicationIndexDigest, next.PublicationCount},
	}
	for _, pair := range pairs {
		digestChanged := pair.previousDigest != pair.nextDigest
		countChanged := pair.previousCount != pair.nextCount
		if digestChanged != countChanged {
			return errors.New("evidence authority bundle child digest and count are mismatched")
		}
		if !digestChanged {
			continue
		}
		if pair.previousCount == ^uint64(0) || pair.nextCount != pair.previousCount+1 {
			return errors.New("evidence authority bundle child count does not advance one step")
		}
		changed++
	}
	if changed != 1 {
		return errors.New("evidence authority bundle must advance exactly one child index")
	}
	return nil
}

func ParseEvidenceAuthorityBundleV1(body []byte) (EvidenceAuthorityBundleV1, error) {
	var bundle EvidenceAuthorityBundleV1
	if err := decodeStrictEvidenceAuthorityBundleV1(body, &bundle); err != nil {
		return EvidenceAuthorityBundleV1{}, err
	}
	canonical, err := json.Marshal(bundle)
	if err != nil || !bytes.Equal(body, canonical) {
		return EvidenceAuthorityBundleV1{}, errors.New("evidence authority bundle is not canonically encoded")
	}
	return bundle, ValidateEvidenceAuthorityBundleV1(bundle)
}

func EvidenceAuthorityBundleV1Bytes(bundle EvidenceAuthorityBundleV1) ([]byte, error) {
	if err := ValidateEvidenceAuthorityBundleV1(bundle); err != nil {
		return nil, err
	}
	return json.Marshal(bundle)
}

func EvidenceAuthorityBundleSigningBytesV1(bundle EvidenceAuthorityBundleV1) []byte {
	bundle.AuthoritySignature = ""
	bundle.RecordDigest = ""
	body, _ := json.Marshal(bundle)
	digest := sha256.Sum256(body)
	out := append([]byte(nil), evidenceAuthorityBundleSignatureDomain...)
	return append(out, digest[:]...)
}

// ValidateEvidenceAuthorityBundleCheckpointV1 binds a bundle to the exact
// witnessed head. A signed checkpoint alone is authentic but may be stale;
// callers still need a fresh observe challenge at the app boundary.
func ValidateEvidenceAuthorityBundleCheckpointV1(bundle EvidenceAuthorityBundleV1, checkpoint domainsecurity.MonotonicHeadCheckpointV1) error {
	if ValidateEvidenceAuthorityBundleV1(bundle) != nil || domainsecurity.ValidateMonotonicHeadCheckpointV1(checkpoint) != nil ||
		bundle.InstallationID != checkpoint.InstallationID || bundle.EnrollmentID != checkpoint.EnrollmentID ||
		bundle.Namespace != checkpoint.Namespace || bundle.Generation != checkpoint.Generation ||
		bundle.RecordDigest != checkpoint.CurrentStateDigest || bundle.MutationID != checkpoint.MutationID {
		return errors.New("evidence authority bundle does not match monotonic checkpoint")
	}
	return nil
}

func ValidateEvidenceAuthorityBundleWitnessAdvanceV1(
	previousBundle, nextBundle EvidenceAuthorityBundleV1,
	previousCheckpoint domainsecurity.MonotonicHeadCheckpointV1,
	request domainsecurity.MonotonicHeadAdvanceRequestV1,
	receipt domainsecurity.MonotonicHeadAdvanceReceiptV1,
) error {
	if err := ValidateEvidenceAuthorityBundleTransitionV1(previousBundle, nextBundle); err != nil {
		return err
	}
	if err := ValidateEvidenceAuthorityBundleCheckpointV1(previousBundle, previousCheckpoint); err != nil {
		return err
	}
	if err := domainsecurity.ValidateMonotonicHeadAdvanceV1(previousCheckpoint, request, receipt); err != nil {
		return err
	}
	if nextBundle.MutationID != request.MutationID || nextBundle.RecordDigest != request.NextStateDigest ||
		nextBundle.AuthorityKeyID != request.AuthorityKeyID || nextBundle.AuthorityPublicKey != request.AuthorityPublicKey {
		return errors.New("evidence authority bundle is not bound to witness request")
	}
	return ValidateEvidenceAuthorityBundleCheckpointV1(nextBundle, receipt.Checkpoint)
}

// ValidateEvidenceAuthorityBundleFirstWitnessAdvanceV1 binds the generation-
// one bundle to an enrolled generation-zero witness checkpoint. Later changes
// must use ValidateEvidenceAuthorityBundleWitnessAdvanceV1.
func ValidateEvidenceAuthorityBundleFirstWitnessAdvanceV1(
	firstBundle EvidenceAuthorityBundleV1,
	enrollmentCheckpoint domainsecurity.MonotonicHeadCheckpointV1,
	request domainsecurity.MonotonicHeadAdvanceRequestV1,
	receipt domainsecurity.MonotonicHeadAdvanceReceiptV1,
) error {
	if ValidateEvidenceAuthorityBundleV1(firstBundle) != nil || firstBundle.Generation != 1 ||
		firstBundle.PreviousBundleDigest != "" || enrollmentCheckpoint.Generation != 0 {
		return errors.New("evidence authority initial bundle is invalid")
	}
	if err := domainsecurity.ValidateMonotonicHeadAdvanceV1(enrollmentCheckpoint, request, receipt); err != nil {
		return err
	}
	if firstBundle.InstallationID != enrollmentCheckpoint.InstallationID || firstBundle.EnrollmentID != enrollmentCheckpoint.EnrollmentID ||
		firstBundle.Namespace != enrollmentCheckpoint.Namespace || firstBundle.MutationID != request.MutationID ||
		firstBundle.RecordDigest != request.NextStateDigest || firstBundle.AuthorityKeyID != request.AuthorityKeyID ||
		firstBundle.AuthorityPublicKey != request.AuthorityPublicKey {
		return errors.New("evidence authority initial bundle is not bound to witness request")
	}
	return ValidateEvidenceAuthorityBundleCheckpointV1(firstBundle, receipt.Checkpoint)
}

// ValidateEvidenceAuthorityBundleIdempotentReplayV1 requires both the generic
// witness request/receipt and the installation bundle to replay byte-for-byte.
func ValidateEvidenceAuthorityBundleIdempotentReplayV1(
	committedBundle EvidenceAuthorityBundleV1,
	committedRequest domainsecurity.MonotonicHeadAdvanceRequestV1,
	committedReceipt domainsecurity.MonotonicHeadAdvanceReceiptV1,
	replayBundle EvidenceAuthorityBundleV1,
	replayRequest domainsecurity.MonotonicHeadAdvanceRequestV1,
	replayReceipt domainsecurity.MonotonicHeadAdvanceReceiptV1,
) error {
	if ValidateEvidenceAuthorityBundleV1(committedBundle) != nil || ValidateEvidenceAuthorityBundleV1(replayBundle) != nil {
		return errors.New("evidence authority replay bundle is invalid")
	}
	if err := domainsecurity.ValidateMonotonicHeadIdempotentReplayV1(committedRequest, committedReceipt, replayRequest, replayReceipt); err != nil {
		return err
	}
	if err := ValidateEvidenceAuthorityBundleCheckpointV1(committedBundle, committedReceipt.Checkpoint); err != nil {
		return err
	}
	if err := ValidateEvidenceAuthorityBundleCheckpointV1(replayBundle, replayReceipt.Checkpoint); err != nil {
		return err
	}
	committedBytes, _ := EvidenceAuthorityBundleV1Bytes(committedBundle)
	replayBytes, _ := EvidenceAuthorityBundleV1Bytes(replayBundle)
	if !bytes.Equal(committedBytes, replayBytes) {
		return errors.New("evidence authority idempotent replay bundle is not exact")
	}
	return nil
}

func validateEvidenceAuthorityBundleUnsignedV1(bundle EvidenceAuthorityBundleV1) error {
	if bundle.SchemaVersion != EvidenceAuthorityBundleSchemaVersion || bundle.Purpose != EvidenceAuthorityBundlePurpose ||
		!isCanonicalEvidenceAuthoritySHA256(bundle.InstallationID) || !isCanonicalEvidenceAuthoritySHA256(bundle.EnrollmentID) ||
		bundle.Namespace != EvidenceAuthorityBundleWitnessNamespaceV1 || bundle.Generation == 0 ||
		!isCanonicalEvidenceAuthoritySHA256(bundle.MutationID) || !isCanonicalEvidenceAuthoritySHA256(bundle.DatasetSnapshotIndexDigest) ||
		!isCanonicalEvidenceAuthoritySHA256(bundle.EvidenceRegistryIndexDigest) || !isCanonicalEvidenceAuthoritySHA256(bundle.PublicationIndexDigest) ||
		bundle.AuthorityAlgorithm != EvidenceAuthorityBundleAlgorithm || !isCanonicalEvidenceAuthoritySHA256(bundle.AuthorityKeyID) ||
		(bundle.Generation == 1 && bundle.PreviousBundleDigest != "") ||
		(bundle.Generation > 1 && !isCanonicalEvidenceAuthoritySHA256(bundle.PreviousBundleDigest)) {
		return errors.New("evidence authority bundle is incomplete")
	}
	return nil
}

func isCanonicalEvidenceAuthoritySHA256(value string) bool {
	return value == strings.TrimSpace(value) && domainsecurity.IsSHA256Hex(value)
}

func evidenceAuthorityBundleDigestV1(bundle EvidenceAuthorityBundleV1) string {
	bundle.RecordDigest = ""
	body, _ := json.Marshal(bundle)
	return domainsecurity.SHA256Hex(body)
}

func decodeStrictEvidenceAuthorityBundleV1(body []byte, target *EvidenceAuthorityBundleV1) error {
	if target == nil {
		return errors.New("evidence authority bundle target is invalid")
	}
	object, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 64 << 10, MaxDepth: 4, MaxTokens: 128, MaxStringBytes: 4 << 10,
	})
	if err != nil {
		return err
	}
	expected := [...]string{
		"schemaVersion", "purpose", "installationId", "enrollmentId", "namespace", "generation",
		"previousBundleDigest", "mutationId", "datasetSnapshotIndexDigest", "datasetSnapshotCount",
		"evidenceRegistryIndexDigest", "evidenceRegistryCount", "publicationIndexDigest", "publicationCount",
		"authorityAlgorithm", "authorityKeyId", "authorityPublicKey", "authoritySignature", "recordDigest",
	}
	if len(object) != len(expected) {
		return errors.New("evidence authority bundle object fields are incomplete")
	}
	for _, name := range expected {
		if _, exists := object[name]; !exists {
			return errors.New("evidence authority bundle object field name is not canonical")
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("evidence authority bundle contains trailing JSON")
	}
	return nil
}
