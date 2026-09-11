package nativecomponent

import (
	"testing"
	"time"
)

func TestNativeComponentToolNameAndResultAreClosedToRegisteredPolicy(t *testing.T) {
	policy, ok := ParseToolName("native__data_engine__health")
	if !ok || policy.ComponentID != ComponentDataEngine || policy.Operation != "health" || policy.MaxDuration != 10*time.Second {
		t.Fatalf("registered native policy was not recovered: %#v", policy)
	}
	if _, ok := ParseToolName("native__data_engine__duckdb_query"); ok {
		t.Fatal("unavailable native operation was parsed as executable")
	}
	valid := Result{
		SchemaVersion:  ResultSchemaVersion,
		ComponentID:    policy.ComponentID,
		Operation:      policy.Operation,
		Status:         "ready",
		RegistryDigest: "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899",
	}
	if err := ValidateResult(valid, policy); err != nil {
		t.Fatalf("valid native result rejected: %v", err)
	}
	invalid := valid
	invalid.RegistryDigest = "A" + invalid.RegistryDigest[1:]
	if err := ValidateResult(invalid, policy); err == nil {
		t.Fatal("non-canonical native registry digest was accepted")
	}
	flowPolicy, ok := Policy(ComponentDataEngine, OperationFundsAnalyzeAccountFlows)
	if !ok {
		t.Fatal("fixed flow policy missing")
	}
	invalid = valid
	invalid.Operation = OperationFundsAnalyzeAccountFlows
	if err := ValidateResult(invalid, flowPolicy); err == nil {
		t.Fatal("health result impersonated the typed account-flow result")
	}
}
