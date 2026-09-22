package steering

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const steeringAdmissionSignatureDomainV1 = "analytix/steering-admission-signature/v1\x00"
const steeringPromotionSignatureDomainV1 = "analytix/steering-promotion-signature/v1\x00"

type AuthorityMaterialV1 struct {
	KeyID        string
	PublicKey    []byte
	Signature    []byte
	SigningBytes []byte
}

func PendingEntrySigningBytesV1(raw map[string]any, contextDigest string) ([]byte, error) {
	content, err := parseSteeringAuthorityContentV1(raw, contextDigest)
	if err != nil {
		return nil, err
	}
	canonical := entryContentMapV1(content)
	canonical["projectionVersion"] = ProjectionVersionV1
	canonical["contentDigest"] = contentDigestV1(content)
	canonical["contextDigest"] = contextDigest
	canonical["status"] = "pending"
	payload, err := json.Marshal(canonical)
	if err != nil {
		return nil, errors.New("steering admission authority payload is invalid")
	}
	return append([]byte(steeringAdmissionSignatureDomainV1), payload...), nil
}

func SealPendingEntryAuthorityV1(raw map[string]any, contextDigest, keyID string, publicKey, signature []byte) (map[string]any, error) {
	if ValidatePendingEntryForContextV1(raw, contextDigest) != nil || !validAuthorityMaterialV1(keyID, publicKey, signature) {
		return nil, errors.New("steering admission authority cannot be sealed")
	}
	signingBytes, err := PendingEntrySigningBytesV1(raw, contextDigest)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(publicKey), signingBytes, signature) {
		return nil, errors.New("steering admission authority signature is invalid")
	}
	out := cloneAuthorityMapV1(raw)
	out["authorityKeyId"] = keyID
	out["authorityPublicKey"] = base64.RawURLEncoding.EncodeToString(publicKey)
	out["authoritySignature"] = base64.RawURLEncoding.EncodeToString(signature)
	if ValidatePendingEntryForContextV1(out, contextDigest) != nil {
		return nil, errors.New("sealed steering admission authority is invalid")
	}
	return out, nil
}

func PromotedEntrySigningBytesV1(raw map[string]any, contextDigest string) ([]byte, error) {
	canonical, err := promotedEntryCanonicalBaseV1(raw, contextDigest)
	if err != nil {
		return nil, err
	}
	if _, err := AdmissionAuthorityMaterialV1(raw, contextDigest); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return nil, errors.New("steering promotion authority payload is invalid")
	}
	return append([]byte(steeringPromotionSignatureDomainV1), payload...), nil
}

func SealPromotedEntryAuthorityV1(raw map[string]any, contextDigest, keyID string, publicKey, signature []byte) (map[string]any, error) {
	if !validAuthorityMaterialV1(keyID, publicKey, signature) {
		return nil, errors.New("steering promotion authority cannot be sealed")
	}
	signingBytes, err := PromotedEntrySigningBytesV1(raw, contextDigest)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(publicKey), signingBytes, signature) {
		return nil, errors.New("steering promotion authority signature is invalid")
	}
	out := cloneAuthorityMapV1(raw)
	out["promotionAuthorityKeyId"] = keyID
	out["promotionAuthorityPublicKey"] = base64.RawURLEncoding.EncodeToString(publicKey)
	out["promotionAuthoritySignature"] = base64.RawURLEncoding.EncodeToString(signature)
	if ValidatePromotedEntryForContextV1(out, contextDigest) != nil {
		return nil, errors.New("sealed steering promotion authority is invalid")
	}
	return out, nil
}

func AdmissionAuthorityMaterialV1(raw map[string]any, contextDigest string) (AuthorityMaterialV1, error) {
	switch stringField(raw, "status") {
	case "pending":
		if ValidatePendingEntryForContextV1(raw, contextDigest) != nil {
			return AuthorityMaterialV1{}, errors.New("pending steering admission authority is invalid")
		}
	case "promoted":
		if _, err := promotedEntryCanonicalBaseV1(raw, contextDigest); err != nil {
			return AuthorityMaterialV1{}, errors.New("promoted steering admission authority is invalid")
		}
	default:
		return AuthorityMaterialV1{}, errors.New("steering admission authority status is invalid")
	}
	material, err := admissionAuthorityMaterialV1(raw, contextDigest)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(material.PublicKey), material.SigningBytes, material.Signature) {
		return AuthorityMaterialV1{}, errors.New("steering admission authority signature is invalid")
	}
	return material, nil
}

func EntryAuthorityMaterialV1(raw map[string]any, contextDigest string) (AuthorityMaterialV1, error) {
	status := stringField(raw, "status")
	switch status {
	case "pending":
		return AdmissionAuthorityMaterialV1(raw, contextDigest)
	case "promoted":
		if ValidatePromotedEntryForContextV1(raw, contextDigest) != nil {
			return AuthorityMaterialV1{}, errors.New("promoted steering authority is invalid")
		}
		canonical, err := promotedEntryCanonicalBaseV1(raw, contextDigest)
		if err != nil {
			return AuthorityMaterialV1{}, err
		}
		return promotionAuthorityMaterialV1(raw, canonical)
	default:
		return AuthorityMaterialV1{}, errors.New("steering authority status is invalid")
	}

}

func admissionAuthorityMaterialV1(raw map[string]any, contextDigest string) (AuthorityMaterialV1, error) {
	keyID := stringField(raw, "authorityKeyId")
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(stringField(raw, "authorityPublicKey"))
	signature, signatureErr := base64.RawURLEncoding.DecodeString(stringField(raw, "authoritySignature"))
	if publicErr != nil || signatureErr != nil || !validAuthorityMaterialV1(keyID, publicKey, signature) {
		return AuthorityMaterialV1{}, errors.New("steering authority material is invalid")
	}
	signingBytes, err := PendingEntrySigningBytesV1(raw, contextDigest)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(publicKey), signingBytes, signature) {
		return AuthorityMaterialV1{}, errors.New("steering authority signature is invalid")
	}
	return AuthorityMaterialV1{KeyID: keyID, PublicKey: publicKey, Signature: signature, SigningBytes: signingBytes}, nil
}

func promotionAuthorityMaterialV1(raw, canonical map[string]any) (AuthorityMaterialV1, error) {
	keyID := stringField(raw, "promotionAuthorityKeyId")
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(stringField(raw, "promotionAuthorityPublicKey"))
	signature, signatureErr := base64.RawURLEncoding.DecodeString(stringField(raw, "promotionAuthoritySignature"))
	if publicErr != nil || signatureErr != nil || !validAuthorityMaterialV1(keyID, publicKey, signature) {
		return AuthorityMaterialV1{}, errors.New("steering promotion authority material is invalid")
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return AuthorityMaterialV1{}, errors.New("steering promotion authority payload is invalid")
	}
	signingBytes := append([]byte(steeringPromotionSignatureDomainV1), payload...)
	return AuthorityMaterialV1{KeyID: keyID, PublicKey: publicKey, Signature: signature, SigningBytes: signingBytes}, nil
}

func parseSteeringAuthorityContentV1(raw map[string]any, contextDigest string) (entryContentV1, error) {
	if !domainsecurity.IsSHA256Hex(contextDigest) {
		return entryContentV1{}, errors.New("steering authority context is invalid")
	}
	status := stringField(raw, "status")
	allowed := pendingFieldsV1
	if status == "promoted" {
		allowed = promotedFieldsV1
	}
	content, err := parseEntryContentV1(raw, allowed)
	if err != nil || stringField(raw, "contextDigest") != contextDigest || stringField(raw, "contentDigest") != contentDigestV1(content) {
		return entryContentV1{}, errors.New("steering authority content is invalid")
	}
	return content, nil
}

func appendAuthorityCanonicalV1(raw, canonical map[string]any) bool {
	keyID := stringField(raw, "authorityKeyId")
	encodedPublicKey := stringField(raw, "authorityPublicKey")
	encodedSignature := stringField(raw, "authoritySignature")
	if keyID == "" && encodedPublicKey == "" && encodedSignature == "" {
		return true
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(encodedPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(encodedSignature)
	if publicErr != nil || signatureErr != nil || !validAuthorityMaterialV1(keyID, publicKey, signature) {
		return false
	}
	canonical["authorityKeyId"] = keyID
	canonical["authorityPublicKey"] = encodedPublicKey
	canonical["authoritySignature"] = encodedSignature
	return true
}

func appendPromotionAuthorityCanonicalV1(raw, canonical map[string]any) bool {
	keyID := stringField(raw, "promotionAuthorityKeyId")
	encodedPublicKey := stringField(raw, "promotionAuthorityPublicKey")
	encodedSignature := stringField(raw, "promotionAuthoritySignature")
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(encodedPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(encodedSignature)
	if publicErr != nil || signatureErr != nil || !validAuthorityMaterialV1(keyID, publicKey, signature) {
		return false
	}
	canonical["promotionAuthorityKeyId"] = keyID
	canonical["promotionAuthorityPublicKey"] = encodedPublicKey
	canonical["promotionAuthoritySignature"] = encodedSignature
	return true
}

func validAuthorityMaterialV1(keyID string, publicKey, signature []byte) bool {
	return len(publicKey) == ed25519.PublicKeySize && len(signature) == ed25519.SignatureSize &&
		keyID == domainsecurity.SHA256Hex(publicKey)
}

func cloneAuthorityMapV1(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
