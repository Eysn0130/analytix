package backendgeneration

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	AllocationSchemaVersionV1  = 1
	AllocationPurposeV1        = "analytix.runtime-backend-generation-allocation/v1"
	MaxAllocationRecordBytesV1 = 1024
	MaxSafeGenerationV1        = uint64(1<<53 - 1)
	allocationNonceBytesV1     = 32
)

var allocationRecordFieldsV1 = [...]string{
	"schemaVersion", "purpose", "generation", "previousRecordDigest", "allocationNonce",
}

type AllocationRecordV1 struct {
	SchemaVersion        int    `json:"schemaVersion"`
	Purpose              string `json:"purpose"`
	Generation           uint64 `json:"generation"`
	PreviousRecordDigest string `json:"previousRecordDigest"`
	AllocationNonce      string `json:"allocationNonce"`
}

type AllocationMaterialV1 struct {
	Digest string
	Body   []byte
}

type AllocationHeadV1 struct {
	Generation uint64
	Digest     string
}

func NewAllocationRecordV1(
	generation uint64,
	previousRecordDigest string,
	allocationNonce string,
) (AllocationRecordV1, error) {
	record := AllocationRecordV1{
		SchemaVersion:        AllocationSchemaVersionV1,
		Purpose:              AllocationPurposeV1,
		Generation:           generation,
		PreviousRecordDigest: previousRecordDigest,
		AllocationNonce:      allocationNonce,
	}
	if err := ValidateAllocationRecordV1(record); err != nil {
		return AllocationRecordV1{}, err
	}
	return record, nil
}

func ValidateAllocationRecordV1(record AllocationRecordV1) error {
	if record.SchemaVersion != AllocationSchemaVersionV1 || record.Purpose != AllocationPurposeV1 ||
		record.Generation == 0 || record.Generation > MaxSafeGenerationV1 || !validAllocationNonceV1(record.AllocationNonce) {
		return errors.New("backend generation allocation record is invalid")
	}
	if record.Generation == 1 {
		if record.PreviousRecordDigest != "" {
			return errors.New("backend generation allocation genesis predecessor is invalid")
		}
	} else if !isSHA256Hex(record.PreviousRecordDigest) {
		return errors.New("backend generation allocation predecessor is invalid")
	}
	return nil
}

func AllocationRecordBytesV1(record AllocationRecordV1) ([]byte, error) {
	if err := ValidateAllocationRecordV1(record); err != nil {
		return nil, err
	}
	body, err := json.Marshal(record)
	if err != nil || len(body) == 0 || len(body) > MaxAllocationRecordBytesV1 {
		return nil, errors.New("backend generation allocation record encoding failed")
	}
	return body, nil
}

func ParseAllocationRecordV1(body []byte) (AllocationRecordV1, error) {
	object, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: MaxAllocationRecordBytesV1, MaxDepth: 2,
		MaxTokens: 16, MaxStringBytes: 256, MaxNumberBytes: 16, MaxAbsExponent: 0,
	})
	if err != nil || len(object) != len(allocationRecordFieldsV1) {
		return AllocationRecordV1{}, errors.New("backend generation allocation record is invalid")
	}
	for _, field := range allocationRecordFieldsV1 {
		if _, ok := object[field]; !ok {
			return AllocationRecordV1{}, errors.New("backend generation allocation record is invalid")
		}
	}
	var record AllocationRecordV1
	if err := json.Unmarshal(body, &record); err != nil || ValidateAllocationRecordV1(record) != nil {
		return AllocationRecordV1{}, errors.New("backend generation allocation record is invalid")
	}
	canonical, err := AllocationRecordBytesV1(record)
	if err != nil || !bytes.Equal(canonical, body) {
		return AllocationRecordV1{}, errors.New("backend generation allocation record is not canonical")
	}
	return record, nil
}

func AllocationRecordDigestV1(record AllocationRecordV1) (string, error) {
	body, err := AllocationRecordBytesV1(record)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func ValidateAllocationChainV1(materials []AllocationMaterialV1) (AllocationHeadV1, error) {
	entries := make([]struct {
		record AllocationRecordV1
		digest string
	}, 0, len(materials))
	seenDigests := make(map[string]struct{}, len(materials))
	for _, material := range materials {
		record, err := ParseAllocationRecordV1(material.Body)
		if err != nil {
			return AllocationHeadV1{}, errors.New("backend generation allocation chain is corrupt")
		}
		digest, err := AllocationRecordDigestV1(record)
		if err != nil || digest != material.Digest {
			return AllocationHeadV1{}, errors.New("backend generation allocation chain is corrupt")
		}
		if _, duplicate := seenDigests[digest]; duplicate {
			return AllocationHeadV1{}, errors.New("backend generation allocation chain is corrupt")
		}
		seenDigests[digest] = struct{}{}
		entries = append(entries, struct {
			record AllocationRecordV1
			digest string
		}{record: record, digest: digest})
	}
	if len(entries) == 0 {
		return AllocationHeadV1{}, nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].record.Generation < entries[j].record.Generation })
	previousDigest := ""
	for index, entry := range entries {
		if entry.record.Generation != uint64(index+1) || entry.record.PreviousRecordDigest != previousDigest {
			return AllocationHeadV1{}, errors.New("backend generation allocation chain is corrupt")
		}
		previousDigest = entry.digest
	}
	last := entries[len(entries)-1]
	return AllocationHeadV1{Generation: last.record.Generation, Digest: last.digest}, nil
}

func validAllocationNonceV1(value string) bool {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || len(decoded) != allocationNonceBytesV1 ||
		base64.RawURLEncoding.EncodeToString(decoded) != value {
		clearBytes(decoded)
		return false
	}
	clearBytes(decoded)
	return true
}

func isSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

func clearBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
