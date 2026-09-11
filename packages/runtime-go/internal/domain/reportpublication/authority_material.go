package reportpublication

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
)

func PublicationAttemptAuthorityMaterialV1(attempt PublicationAttemptV1) (string, []byte, []byte, error) {
	if err := ValidatePublicationAttemptV1(attempt); err != nil {
		return "", nil, nil, err
	}
	return decodePublicationAuthorityMaterialV1(attempt.AuthorityKeyID, attempt.AuthorityPublicKey, attempt.AuthoritySignature)
}

func PublicationReceiptAuthorityMaterialV1(receipt PublicationReceiptV1) (string, []byte, []byte, error) {
	if err := ValidatePublicationReceiptV1(receipt); err != nil {
		return "", nil, nil, err
	}
	return decodePublicationAuthorityMaterialV1(receipt.AuthorityKeyID, receipt.AuthorityPublicKey, receipt.AuthoritySignature)
}

func PublicationIndexAuthorityMaterialV1(index PublicationIndexV1) (string, []byte, []byte, error) {
	if err := ValidatePublicationIndexV1(index); err != nil {
		return "", nil, nil, err
	}
	return decodePublicationAuthorityMaterialV1(index.AuthorityKeyID, index.AuthorityPublicKey, index.AuthoritySignature)
}

func PublicationCommitReceiptAuthorityMaterialV1(receipt PublicationCommitReceiptV1) (string, []byte, []byte, error) {
	if err := ValidatePublicationCommitReceiptV1(receipt); err != nil {
		return "", nil, nil, err
	}
	return decodePublicationAuthorityMaterialV1(receipt.AuthorityKeyID, receipt.AuthorityPublicKey, receipt.AuthoritySignature)
}

func PublicationCommitSelectionAuthorityMaterialV1(selection PublicationCommitSelectionV1) (string, []byte, []byte, error) {
	if err := ValidatePublicationCommitSelectionV1(selection); err != nil {
		return "", nil, nil, err
	}
	return decodePublicationAuthorityMaterialV1(selection.AuthorityKeyID, selection.AuthorityPublicKey, selection.AuthoritySignature)
}

func ReportDeliveryDecisionAuthorityMaterialV1(decision ReportDeliveryDecisionV1) (string, []byte, []byte, error) {
	if err := ValidateReportDeliveryDecisionV1(decision); err != nil {
		return "", nil, nil, err
	}
	return decodePublicationAuthorityMaterialV1(decision.AuthorityKeyID, decision.AuthorityPublicKey, decision.AuthoritySignature)
}

func decodePublicationAuthorityMaterialV1(keyID, encodedPublicKey, encodedSignature string) (string, []byte, []byte, error) {
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(encodedPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(encodedSignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != encodedPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != encodedSignature {
		return "", nil, nil, errors.New("publication authority material is invalid")
	}
	return keyID, append([]byte(nil), publicKey...), append([]byte(nil), signature...), nil
}
