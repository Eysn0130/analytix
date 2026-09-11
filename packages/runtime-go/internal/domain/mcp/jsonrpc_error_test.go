package mcp

import (
	"strings"
	"testing"
)

func TestJSONRPCErrorClassifiesStandardAndServerErrorsWithoutLeakingText(t *testing.T) {
	cases := map[int]string{
		-32700: "parse_error",
		-32600: "invalid_request",
		-32601: "method_not_found",
		-32602: "invalid_params",
		-32603: "internal_error",
		-32042: "server_error",
		41000:  "application_error",
	}
	for code, want := range cases {
		err := &JSONRPCError{Code: code, Message: "account 6217000012345678901", Data: []byte(`{"secret":"pii"}`)}
		if got := err.Class(); got != want {
			t.Fatalf("code %d class=%q want=%q", code, got, want)
		}
		if text := err.Error(); text == "" || containsSensitiveJSONRPCText(text) {
			t.Fatalf("Error leaked untrusted message/data: %q", text)
		}
	}
}

func TestJSONRPCDiagnosticRecordIsBoundedAndSelfConsistent(t *testing.T) {
	err := &JSONRPCError{Code: -32602, Message: "account 6217000012345678901", Data: []byte(`{"secret":"pii"}`)}
	record := JSONRPCDiagnosticRecord(err)
	bounded, ok := BoundedJSONRPCDiagnostic(record)
	if !ok || bounded["class"] != "invalid_params" || bounded["code"] != -32602 || bounded["dataPresent"] != true {
		t.Fatalf("bounded diagnostic mismatch: %#v", bounded)
	}
	for _, mutation := range []func(map[string]any){
		func(value map[string]any) { value["message"] = err.Message },
		func(value map[string]any) { value["class"] = "internal_error" },
		func(value map[string]any) { value["dataSHA256"] = "short" },
	} {
		copyRecord := map[string]any{}
		for key, value := range record {
			copyRecord[key] = value
		}
		mutation(copyRecord)
		if _, ok := BoundedJSONRPCDiagnostic(copyRecord); ok {
			t.Fatalf("mutated diagnostic was accepted: %#v", copyRecord)
		}
	}
}

func containsSensitiveJSONRPCText(value string) bool {
	return strings.Contains(value, "6217000012345678901") || strings.Contains(value, "secret") || strings.Contains(value, "pii")
}
