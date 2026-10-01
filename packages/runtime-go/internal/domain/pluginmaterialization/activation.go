package pluginmaterialization

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

const ActivationPurposeV1 = "analytix.bundled-plugin-activation/v1"

type DesiredStateV1 string

const (
	DesiredEnabledV1  DesiredStateV1 = "enabled"
	DesiredDisabledV1 DesiredStateV1 = "disabled"
)

// ActivationV1 records an installation-local desired state for one exact
// materialized generation. It is neither a runtime grant nor a release receipt.
// A new generation has no inherited activation and must be explicitly enabled.
type ActivationV1 struct {
	SchemaVersion      int            `json:"schemaVersion"`
	Purpose            string         `json:"purpose"`
	ActivationID       string         `json:"activationId"`
	PluginName         string         `json:"pluginName"`
	PluginVersion      string         `json:"pluginVersion"`
	GenerationID       string         `json:"generationId"`
	ReceiptID          string         `json:"receiptId"`
	ReceiptSHA256      string         `json:"receiptSha256"`
	Revision           uint64         `json:"revision"`
	DesiredState       DesiredStateV1 `json:"desiredState"`
	RecordedAt         string         `json:"recordedAt"`
	AuthorityAlgorithm string         `json:"authorityAlgorithm"`
	AuthorityKeyID     string         `json:"authorityKeyId"`
	AuthorityPublicKey string         `json:"authorityPublicKey"`
	AuthoritySignature string         `json:"authoritySignature"`
}

func ValidDesiredStateV1(state DesiredStateV1) bool {
	return state == DesiredEnabledV1 || state == DesiredDisabledV1
}

func NewActivationV1(receipt ReceiptV1, revision uint64, desired DesiredStateV1, recordedAt time.Time, keyID string, publicKey []byte, sign SignFuncV1) (ActivationV1, error) {
	if ValidateTrustedReceiptV1(receipt, keyID, publicKey) != nil || sign == nil || recordedAt.IsZero() {
		return ActivationV1{}, errors.New("plugin activation installation authority is invalid")
	}
	activation := ActivationV1{
		SchemaVersion: SchemaVersionV1, Purpose: ActivationPurposeV1,
		PluginName: receipt.PluginName, PluginVersion: receipt.PluginVersion,
		GenerationID: receipt.GenerationID, ReceiptID: receipt.ReceiptID,
		ReceiptSHA256: ReceiptSHA256V1(receipt), Revision: revision, DesiredState: desired,
		RecordedAt:         recordedAt.UTC().Format(time.RFC3339Nano),
		AuthorityAlgorithm: AuthorityAlgorithmV1, AuthorityKeyID: keyID,
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	activation.ActivationID = deriveActivationIDV1(activation)
	if validateActivationPayloadV1(activation) != nil {
		return ActivationV1{}, errors.New("plugin activation payload is invalid")
	}
	signature, err := sign(ActivationSigningBytesV1(activation))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ActivationV1{}, errors.New("plugin activation signing failed")
	}
	activation.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	if err := ValidateTrustedActivationForReceiptV1(activation, receipt, keyID, publicKey); err != nil {
		return ActivationV1{}, err
	}
	return activation, nil
}

func validateActivationPayloadV1(activation ActivationV1) error {
	if activation.SchemaVersion != SchemaVersionV1 || activation.Purpose != ActivationPurposeV1 ||
		!canonicalDigest(activation.ActivationID) || activation.ActivationID != deriveActivationIDV1(activation) ||
		!validPluginIdentityV1(activation.PluginName, activation.PluginVersion) ||
		!canonicalDigest(activation.GenerationID) || !canonicalDigest(activation.ReceiptID) ||
		!canonicalDigest(activation.ReceiptSHA256) || activation.Revision == 0 ||
		!ValidDesiredStateV1(activation.DesiredState) || !canonicalTime(activation.RecordedAt) ||
		activation.AuthorityAlgorithm != AuthorityAlgorithmV1 || !canonicalDigest(activation.AuthorityKeyID) {
		return errors.New("plugin activation payload is invalid")
	}
	return nil
}

func ValidateActivationV1(activation ActivationV1) error {
	if err := validateActivationPayloadV1(activation); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(activation.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(activation.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize ||
		len(signature) != ed25519.SignatureSize || activation.AuthorityKeyID != sha256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), ActivationSigningBytesV1(activation), signature) {
		return errors.New("plugin activation signature is invalid")
	}
	return nil
}

func ValidateTrustedActivationForReceiptV1(activation ActivationV1, receipt ReceiptV1, keyID string, publicKey []byte) error {
	if ValidateActivationV1(activation) != nil || ValidateTrustedReceiptV1(receipt, keyID, publicKey) != nil ||
		activation.AuthorityKeyID != keyID || activation.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(publicKey) ||
		activation.PluginName != receipt.PluginName || activation.PluginVersion != receipt.PluginVersion ||
		activation.GenerationID != receipt.GenerationID || activation.ReceiptID != receipt.ReceiptID ||
		activation.ReceiptSHA256 != ReceiptSHA256V1(receipt) {
		return errors.New("plugin activation does not bind the current installation receipt")
	}
	return nil
}

func ActivationSigningBytesV1(activation ActivationV1) []byte {
	activation.AuthoritySignature = ""
	body, _ := json.Marshal(activation)
	digest := sha256.Sum256(body)
	return append([]byte("analytix.bundled-plugin-activation/signature/v1\x00"), digest[:]...)
}

func deriveActivationIDV1(activation ActivationV1) string {
	activation.ActivationID = ""
	activation.AuthoritySignature = ""
	return digestWithDomain("analytix.bundled-plugin-activation/id/v1", activation)
}

func ActivationV1Bytes(activation ActivationV1) ([]byte, error) {
	return canonicalBytes(activation, func() error { return ValidateActivationV1(activation) })
}

func ParseActivationV1(body []byte) (ActivationV1, error) {
	var activation ActivationV1
	err := strictParse(body, &activation, func() error { return ValidateActivationV1(activation) })
	return activation, err
}
