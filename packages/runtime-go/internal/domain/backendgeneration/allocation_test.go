package backendgeneration

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestAllocationRecordV1IsCanonicalAndRejectsAmbiguousJSON(t *testing.T) {
	nonce := base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	record, err := NewAllocationRecordV1(1, "", nonce)
	if err != nil {
		t.Fatal(err)
	}
	body, err := AllocationRecordBytesV1(record)
	if err != nil {
		t.Fatal(err)
	}
	expected := `{"schemaVersion":1,"purpose":"analytix.runtime-backend-generation-allocation/v1","generation":1,"previousRecordDigest":"","allocationNonce":"MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY"}`
	if string(body) != expected {
		t.Fatalf("allocation record is not canonical: %s", body)
	}
	parsed, err := ParseAllocationRecordV1(body)
	if err != nil || parsed != record {
		t.Fatalf("canonical allocation record did not round trip: %#v err=%v", parsed, err)
	}
	for _, invalid := range []string{
		strings.Replace(expected, `"generation":1`, `"generation":1.0`, 1),
		strings.Replace(expected, `"generation":1`, `"generation":1e0`, 1),
		strings.Replace(expected, `"generation":1`, `"generation":1,"generation":2`, 1),
		strings.TrimSuffix(expected, "}") + `,"extra":true}`,
		" " + expected,
	} {
		if _, err := ParseAllocationRecordV1([]byte(invalid)); err == nil {
			t.Fatalf("ambiguous allocation JSON was accepted: %s", invalid)
		}
	}
}

func TestAllocationRecordV1RequiresExactPredecessorAndSafeGeneration(t *testing.T) {
	nonce := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	for _, candidate := range []AllocationRecordV1{
		{SchemaVersion: 1, Purpose: AllocationPurposeV1, Generation: 0, AllocationNonce: nonce},
		{SchemaVersion: 1, Purpose: AllocationPurposeV1, Generation: MaxSafeGenerationV1 + 1, AllocationNonce: nonce},
		{SchemaVersion: 1, Purpose: AllocationPurposeV1, Generation: 1, PreviousRecordDigest: strings.Repeat("a", 64), AllocationNonce: nonce},
		{SchemaVersion: 1, Purpose: AllocationPurposeV1, Generation: 2, PreviousRecordDigest: "", AllocationNonce: nonce},
		{SchemaVersion: 1, Purpose: AllocationPurposeV1, Generation: 2, PreviousRecordDigest: strings.Repeat("A", 64), AllocationNonce: nonce},
	} {
		if err := ValidateAllocationRecordV1(candidate); err == nil {
			t.Fatalf("invalid allocation record was accepted: %#v", candidate)
		}
	}
}
