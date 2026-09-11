package pluginmaterialization

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

var receiptSignatureDomainV1 = []byte("analytix.bundled-plugin-materialization-receipt/signature/v1\x00")

type ReceiptV1 struct {
	SchemaVersion          int      `json:"schemaVersion"`
	Purpose                string   `json:"purpose"`
	ReceiptID              string   `json:"receiptId"`
	IntentID               string   `json:"intentId"`
	PackageAuthoritySHA256 string   `json:"packageAuthoritySha256"`
	Target                 TargetV1 `json:"target"`
	PluginName             string   `json:"pluginName"`
	PluginVersion          string   `json:"pluginVersion"`
	GenerationID           string   `json:"generationId"`
	ActiveRelativePath     string   `json:"activeRelativePath"`
	SourceTreeSHA256       string   `json:"sourceTreeSha256"`
	SourceTreeFileCount    uint64   `json:"sourceTreeFileCount"`
	ManifestSHA256         string   `json:"manifestSha256"`
	EntrypointSHA256       string   `json:"entrypointSha256"`
	FactToolsEnabled       bool     `json:"factToolsEnabled"`
	IssuedAt               string   `json:"issuedAt"`
	AuthorityAlgorithm     string   `json:"authorityAlgorithm"`
	AuthorityKeyID         string   `json:"authorityKeyId"`
	AuthorityPublicKey     string   `json:"authorityPublicKey"`
	AuthoritySignature     string   `json:"authoritySignature"`
}

type SignFuncV1 func([]byte) ([]byte, error)

func NewReceiptV1(intent IntentV1, generationID, activeRelativePath string, issuedAt time.Time, keyID string, publicKey []byte, sign SignFuncV1) (ReceiptV1, error) {
	publicKey = append([]byte(nil), publicKey...)
	receipt := ReceiptV1{
		SchemaVersion: SchemaVersionV1, Purpose: ReceiptPurposeV1, IntentID: intent.IntentID,
		PackageAuthoritySHA256: intent.PackageAuthoritySHA256, Target: intent.Target,
		PluginName: intent.PluginName, PluginVersion: intent.PluginVersion,
		GenerationID: generationID, ActiveRelativePath: activeRelativePath,
		SourceTreeSHA256: intent.SourceTreeSHA256, SourceTreeFileCount: intent.SourceTreeFileCount,
		ManifestSHA256: intent.ManifestSHA256, EntrypointSHA256: intent.EntrypointSHA256,
		FactToolsEnabled: false, IssuedAt: issuedAt.UTC().Format(time.RFC3339Nano),
		AuthorityAlgorithm: AuthorityAlgorithmV1, AuthorityKeyID: keyID,
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	receipt.ReceiptID = deriveReceiptID(receipt)
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || keyID != sha256Hex(publicKey) || validateReceiptPayload(receipt) != nil {
		return ReceiptV1{}, errors.New("bundled plugin materialization receipt signing authority is invalid")
	}
	signature, err := sign(ReceiptSigningBytesV1(receipt))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ReceiptV1{}, errors.New("bundled plugin materialization receipt signing failed")
	}
	receipt.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	if err := ValidateReceiptV1(receipt); err != nil {
		return ReceiptV1{}, err
	}
	return receipt, nil
}

func ValidateReceiptV1(receipt ReceiptV1) error {
	if err := validateReceiptPayload(receipt); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(receipt.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(receipt.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		receipt.AuthorityKeyID != sha256Hex(publicKey) || !ed25519.Verify(ed25519.PublicKey(publicKey), ReceiptSigningBytesV1(receipt), signature) {
		return errors.New("bundled plugin materialization receipt signature is invalid")
	}
	return nil
}

func ValidateTrustedReceiptV1(receipt ReceiptV1, keyID string, publicKey []byte) error {
	if err := ValidateReceiptV1(receipt); err != nil {
		return err
	}
	encoded := base64.RawURLEncoding.EncodeToString(publicKey)
	if keyID == "" || keyID != sha256Hex(publicKey) || receipt.AuthorityKeyID != keyID || receipt.AuthorityPublicKey != encoded {
		return errors.New("bundled plugin materialization receipt is not signed by the installation authority")
	}
	return nil
}

func ValidateReceiptForIntentV1(receipt ReceiptV1, intent IntentV1) error {
	if ValidateReceiptV1(receipt) != nil || ValidateIntentV1(intent) != nil ||
		receipt.IntentID != intent.IntentID || receipt.PackageAuthoritySHA256 != intent.PackageAuthoritySHA256 ||
		receipt.Target != intent.Target || receipt.PluginName != intent.PluginName || receipt.PluginVersion != intent.PluginVersion ||
		receipt.SourceTreeSHA256 != intent.SourceTreeSHA256 || receipt.SourceTreeFileCount != intent.SourceTreeFileCount ||
		receipt.ManifestSHA256 != intent.ManifestSHA256 || receipt.EntrypointSHA256 != intent.EntrypointSHA256 {
		return errors.New("bundled plugin materialization receipt does not bind the intent")
	}
	return nil
}

func ReceiptSigningBytesV1(receipt ReceiptV1) []byte {
	receipt.AuthoritySignature = ""
	body, _ := json.Marshal(receipt)
	digest := sha256.Sum256(body)
	return append(append([]byte(nil), receiptSignatureDomainV1...), digest[:]...)
}

func ReceiptV1Bytes(receipt ReceiptV1) ([]byte, error) {
	return canonicalBytes(receipt, func() error { return ValidateReceiptV1(receipt) })
}

func ParseReceiptV1(body []byte) (ReceiptV1, error) {
	var receipt ReceiptV1
	err := strictParse(body, &receipt, func() error { return ValidateReceiptV1(receipt) })
	return receipt, err
}

func ReceiptSHA256V1(receipt ReceiptV1) string {
	body, err := ReceiptV1Bytes(receipt)
	if err != nil {
		return ""
	}
	return sha256Hex(body)
}

func validateReceiptPayload(receipt ReceiptV1) error {
	if receipt.SchemaVersion != SchemaVersionV1 || receipt.Purpose != ReceiptPurposeV1 ||
		!canonicalDigest(receipt.ReceiptID) || receipt.ReceiptID != deriveReceiptID(receipt) ||
		!canonicalDigest(receipt.IntentID) || !canonicalDigest(receipt.PackageAuthoritySHA256) ||
		ValidateTargetV1(receipt.Target) != nil || !validPluginIdentityV1(receipt.PluginName, receipt.PluginVersion) ||
		!canonicalDigest(receipt.GenerationID) || !canonicalRelativePath(receipt.ActiveRelativePath) ||
		!canonicalDigest(receipt.SourceTreeSHA256) || receipt.SourceTreeFileCount == 0 || receipt.SourceTreeFileCount > MaxSourceTreeFilesV1 ||
		!canonicalDigest(receipt.ManifestSHA256) || !canonicalDigest(receipt.EntrypointSHA256) || receipt.FactToolsEnabled ||
		!canonicalTime(receipt.IssuedAt) || receipt.AuthorityAlgorithm != AuthorityAlgorithmV1 || !canonicalDigest(receipt.AuthorityKeyID) {
		return errors.New("bundled plugin materialization receipt is invalid")
	}
	return nil
}

func deriveReceiptID(receipt ReceiptV1) string {
	receipt.ReceiptID = ""
	receipt.AuthoritySignature = ""
	return digestWithDomain("analytix.bundled-plugin-materialization-receipt/id/v1", receipt)
}

func sha256Hex(body []byte) string {
	digest := sha256.Sum256(body)
	return fmtHex(digest[:])
}

func fmtHex(body []byte) string {
	const alphabet = "0123456789abcdef"
	out := make([]byte, len(body)*2)
	for index, value := range body {
		out[index*2] = alphabet[value>>4]
		out[index*2+1] = alphabet[value&15]
	}
	return string(out)
}
