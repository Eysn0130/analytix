package authoritybootstrap

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	maxContractBytes       = 256 << 10
	maxContractDepth       = 16
	maxContractTokens      = 4096
	maxContractStringBytes = 16 << 10
)

var bindingRequiredFieldsV1 = [...]string{
	"schemaVersion", "purpose", "installationId", "currentManifestDigest", "manifestEnrollmentDigest", "namespace", "enrollmentId",
	"bootstrapEnrollmentId", "witnessKeyId", "witnessPublicKey", "enrollmentInitialCheckpoint",
	"floorObserveRequest", "floorObservation", "floorCheckpoint", "floorProjectionDigest", "projectionSlotId",
	"bootstrapInitialObserveRequest", "bootstrapInitialObservation", "bootstrapInitialCheckpoint",
	"installationAuthorityAlgorithm", "installationAuthorityKeyId",
	"installationAuthorityPublicKey", "installationAuthoritySignature", "bindingDigest",
}

var observationRequiredFieldsV1 = [...]string{
	"schemaVersion", "purpose", "bindingDigest", "phase", "prepareMutationId", "prepareReceiptDigest",
	"commitReceiptDigest", "observeRequest", "monotonicObservation", "witnessAlgorithm", "witnessKeyId",
	"witnessPublicKey", "witnessSignature", "observationDigest",
}

var prepareReceiptRequiredFieldsV1 = [...]string{
	"schemaVersion", "purpose", "bindingDigest", "expectedPhase", "nextPhase", "expectedObservationDigest",
	"advanceRequest", "advanceReceipt", "witnessAlgorithm", "witnessKeyId", "witnessPublicKey",
	"witnessSignature", "receiptDigest",
}

var commitReceiptRequiredFieldsV1 = [...]string{
	"schemaVersion", "purpose", "bindingDigest", "prepareReceiptDigest", "expectedPhase", "nextPhase",
	"expectedObservationDigest", "advanceRequest", "advanceReceipt", "witnessAlgorithm", "witnessKeyId",
	"witnessPublicKey", "witnessSignature", "receiptDigest",
}

func ParseBootstrapBindingV1(body []byte) (BootstrapBindingV1, error) {
	return parseCanonicalContractV1(body, bindingRequiredFieldsV1[:], "authority bootstrap binding", ValidateBootstrapBindingV1)
}

func BootstrapBindingV1Bytes(binding BootstrapBindingV1) ([]byte, error) {
	if err := ValidateBootstrapBindingV1(binding); err != nil {
		return nil, err
	}
	return json.Marshal(binding)
}

func ParseBootstrapObservationV1(body []byte) (BootstrapObservationV1, error) {
	return parseCanonicalContractV1(body, observationRequiredFieldsV1[:], "authority bootstrap observation", ValidateBootstrapObservationV1)
}

func BootstrapObservationV1Bytes(observation BootstrapObservationV1) ([]byte, error) {
	if err := ValidateBootstrapObservationV1(observation); err != nil {
		return nil, err
	}
	return json.Marshal(observation)
}

func ParseBootstrapPrepareReceiptV1(body []byte) (BootstrapPrepareReceiptV1, error) {
	return parseCanonicalContractV1(body, prepareReceiptRequiredFieldsV1[:], "authority bootstrap prepare receipt", ValidateBootstrapPrepareReceiptV1)
}

func BootstrapPrepareReceiptV1Bytes(receipt BootstrapPrepareReceiptV1) ([]byte, error) {
	if err := ValidateBootstrapPrepareReceiptV1(receipt); err != nil {
		return nil, err
	}
	return json.Marshal(receipt)
}

func ParseBootstrapCommitReceiptV1(body []byte) (BootstrapCommitReceiptV1, error) {
	return parseCanonicalContractV1(body, commitReceiptRequiredFieldsV1[:], "authority bootstrap commit receipt", ValidateBootstrapCommitReceiptV1)
}

func BootstrapCommitReceiptV1Bytes(receipt BootstrapCommitReceiptV1) ([]byte, error) {
	if err := ValidateBootstrapCommitReceiptV1(receipt); err != nil {
		return nil, err
	}
	return json.Marshal(receipt)
}

func parseCanonicalContractV1[T any](body []byte, required []string, name string, validate func(T) error) (T, error) {
	var zero T
	object, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxContractBytes, MaxDepth: maxContractDepth,
		MaxTokens: maxContractTokens, MaxStringBytes: maxContractStringBytes,
	})
	if err != nil {
		return zero, errors.Join(ErrInvalidContract, err)
	}
	if err := requireExactContractFieldsV1(object, required, name); err != nil {
		return zero, errors.Join(ErrInvalidContract, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var contract T
	if err := decoder.Decode(&contract); err != nil {
		return zero, errors.Join(ErrInvalidContract, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return zero, errors.Join(ErrInvalidContract, errors.New(name+" contains trailing JSON"))
	}
	canonical, err := json.Marshal(contract)
	if err != nil || !bytes.Equal(body, canonical) {
		return zero, errors.Join(ErrInvalidContract, errors.New(name+" is not canonically encoded"), err)
	}
	if err := validate(contract); err != nil {
		return zero, err
	}
	return contract, nil
}

func requireExactContractFieldsV1(object map[string]json.RawMessage, required []string, name string) error {
	allowed := make(map[string]struct{}, len(required))
	for _, field := range required {
		allowed[field] = struct{}{}
		raw, exists := object[field]
		if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return errors.New(name + " required field is missing or null")
		}
	}
	for field := range object {
		if _, exists := allowed[field]; !exists {
			return errors.New(name + " contains an unknown field")
		}
	}
	return nil
}
