package steering

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const steeringAdmissionSignatureDomainV1 = "analytix/steering-admission-signature/v1\x00"
const steeringCancellationSignatureDomainV1 = "analytix/steering-user-input-discard-signature/v1\x00"

// This reason cannot be confused with a whole-turn terminal cancellation.
const UserInputDiscardReasonV1 = "user_input_discarded"

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
	case "cancelled":
		canonical, err := cancelledEntryCanonicalBaseV1(raw, contextDigest)
		if err != nil {
			return AuthorityMaterialV1{}, err
		}
		raw = cloneAuthorityMapV1(canonical)
		delete(raw, "cancelledAt")
		delete(raw, "cancelReason")
		raw["status"] = "pending"
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
	case "cancelled":
		if ValidateCancelledUserInputEntryForContextV1(raw, contextDigest) != nil {
			return AuthorityMaterialV1{}, errors.New("cancelled steering authority is invalid")
		}
		canonical, err := cancelledEntryCanonicalBaseV1(raw, contextDigest)
		if err != nil {
			return AuthorityMaterialV1{}, err
		}
		return cancellationAuthorityMaterialV1(raw, canonical)
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

// HasUserInputDiscardAuthorityV1 prevents incomplete discard proof from being
// interpreted as the older whole-turn cancellation record.
func HasUserInputDiscardAuthorityV1(raw map[string]any) bool {
	if stringField(raw, "cancelReason") == UserInputDiscardReasonV1 {
		return true
	}
	for _, key := range []string{"cancellationAuthorityKeyId", "cancellationAuthorityPublicKey", "cancellationAuthoritySignature"} {
		if _, present := raw[key]; present {
			return true
		}
	}
	return false
}

func CancelledUserInputEntrySigningBytesV1(raw map[string]any, contextDigest string) ([]byte, error) {
	canonical, err := cancelledEntryCanonicalBaseV1(raw, contextDigest)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return nil, errors.New("steering cancellation authority payload is invalid")
	}
	return append([]byte(steeringCancellationSignatureDomainV1), payload...), nil
}

func SealCancelledUserInputEntryAuthorityV1(raw map[string]any, contextDigest, keyID string, publicKey, signature []byte) (map[string]any, error) {
	if !validAuthorityMaterialV1(keyID, publicKey, signature) {
		return nil, errors.New("steering cancellation authority cannot be sealed")
	}
	signingBytes, err := CancelledUserInputEntrySigningBytesV1(raw, contextDigest)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(publicKey), signingBytes, signature) {
		return nil, errors.New("steering cancellation signature is invalid")
	}
	out := cloneAuthorityMapV1(raw)
	out["cancellationAuthorityKeyId"] = keyID
	out["cancellationAuthorityPublicKey"] = base64.RawURLEncoding.EncodeToString(publicKey)
	out["cancellationAuthoritySignature"] = base64.RawURLEncoding.EncodeToString(signature)
	if ValidateCancelledUserInputEntryForContextV1(out, contextDigest) != nil {
		return nil, errors.New("sealed steering cancellation authority is invalid")
	}
	return out, nil
}

func ValidateCancelledUserInputEntryForContextV1(raw map[string]any, contextDigest string) error {
	canonical, err := cancelledEntryCanonicalBaseV1(raw, contextDigest)
	if err != nil {
		return err
	}
	material, err := cancellationAuthorityMaterialV1(raw, canonical)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(material.PublicKey), material.SigningBytes, material.Signature) {
		return errors.New("steering cancellation authority signature is invalid")
	}
	for _, key := range []string{"cancellationAuthorityKeyId", "cancellationAuthorityPublicKey", "cancellationAuthoritySignature"} {
		canonical[key] = raw[key]
	}
	if !sameCanonicalEntryMapV1(raw, canonical) {
		return errors.New("cancelled steering projection is not canonical")
	}
	return nil
}

func cancelledEntryCanonicalBaseV1(raw map[string]any, contextDigest string) (map[string]any, error) {
	cancelledAt := stringField(raw, "cancelledAt")
	if stringField(raw, "status") != "cancelled" || stringField(raw, "cancelReason") != UserInputDiscardReasonV1 || !canonicalRFC3339NanoUTC(cancelledAt) {
		return nil, errors.New("steering cancellation metadata is invalid")
	}
	pending := cloneAuthorityMapV1(raw)
	for _, key := range []string{"cancelledAt", "cancelReason", "cancellationAuthorityKeyId", "cancellationAuthorityPublicKey", "cancellationAuthoritySignature"} {
		delete(pending, key)
	}
	pending["status"] = "pending"
	if ValidatePendingEntryForContextV1(pending, contextDigest) != nil || stringField(pending, "userInputId") == "" || stringField(pending, "userInputCaptureReceiptId") == "" {
		return nil, errors.New("steering cancellation admission is invalid")
	}
	if _, err := AdmissionAuthorityMaterialV1(pending, contextDigest); err != nil {
		return nil, err
	}
	admitted, _ := time.Parse(time.RFC3339Nano, stringField(pending, "admittedAt"))
	cancelled, _ := time.Parse(time.RFC3339Nano, cancelledAt)
	if cancelled.Before(admitted) {
		return nil, errors.New("steering cancellation precedes admission")
	}
	canonical := cloneAuthorityMapV1(pending)
	// JSON recovery uses float64; canonical expectations retain the typed
	// projection integer, as admission and promotion already do.
	canonical["projectionVersion"] = ProjectionVersionV1
	canonical["status"] = "cancelled"
	canonical["cancelledAt"] = cancelledAt
	canonical["cancelReason"] = UserInputDiscardReasonV1
	return canonical, nil
}

func cancellationAuthorityMaterialV1(raw, canonical map[string]any) (AuthorityMaterialV1, error) {
	keyID := stringField(raw, "cancellationAuthorityKeyId")
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(stringField(raw, "cancellationAuthorityPublicKey"))
	signature, signatureErr := base64.RawURLEncoding.DecodeString(stringField(raw, "cancellationAuthoritySignature"))
	if publicErr != nil || signatureErr != nil || !validAuthorityMaterialV1(keyID, publicKey, signature) {
		return AuthorityMaterialV1{}, errors.New("steering cancellation authority material is invalid")
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return AuthorityMaterialV1{}, errors.New("steering cancellation authority payload is invalid")
	}
	return AuthorityMaterialV1{KeyID: keyID, PublicKey: publicKey, Signature: signature, SigningBytes: append([]byte(steeringCancellationSignatureDomainV1), payload...)}, nil
}
