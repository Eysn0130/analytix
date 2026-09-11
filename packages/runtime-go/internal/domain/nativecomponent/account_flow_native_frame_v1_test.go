package nativecomponent

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAnalyzeAccountFlowsNativeFrameHasOneFixedPathFreeContract(t *testing.T) {
	arguments := nativeComponentTestAccountFlowArguments(
		t,
		nativeComponentTestContext(t, "case-native-frame", time.Date(2026, 7, 27, 1, 0, 0, 0, time.UTC)),
	)
	requestID := strings.Repeat("9", 64)
	frame, err := EncodeAnalyzeAccountFlowsNativeRequestFrameV1(requestID, arguments)
	if err != nil {
		t.Fatalf("encode private native frame: %v", err)
	}
	if len(frame) == 0 || frame[len(frame)-1] != '\n' || len(frame) > AccountFlowMaximumNativeRequestFrameBytesV1 {
		t.Fatalf("native frame boundary is invalid: bytes=%d", len(frame))
	}
	decoder := json.NewDecoder(bytes.NewReader(frame[:len(frame)-1]))
	decoder.DisallowUnknownFields()
	var wire analyzeAccountFlowsNativeRequestFrameWireV1
	if err := decoder.Decode(&wire); err != nil {
		t.Fatalf("decode private native frame: %v", err)
	}
	if wire.RequestID != requestID || wire.Command != OperationFundsAnalyzeAccountFlows || wire.CaseID == "" {
		t.Fatalf("native frame identity mismatch: %#v", wire)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(frame[:len(frame)-1], &root); err != nil || len(root) != 4 {
		t.Fatalf("native frame root shape mismatch: keys=%v err=%v", root, err)
	}
	for _, forbidden := range []string{"db_path", "database_path", "sql", "query"} {
		if _, found := root[forbidden]; found {
			t.Fatalf("native frame exposed forbidden root field %q", forbidden)
		}
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(wire.Payload, &payload); err != nil || len(payload) != len(analyzeAccountFlowsRequiredFieldsV1) {
		t.Fatalf("native payload shape mismatch: keys=%v err=%v", payload, err)
	}
	for _, forbidden := range []string{
		"db_path",
		"databasePath",
		"sql",
		"query",
		"expectedDuckdbContentSnapshotDigest",
		"expectedDuckdbSnapshotManifestSha256",
		"expectedMaterializationIdentity",
	} {
		if _, found := payload[forbidden]; found {
			t.Fatalf("native payload exposed forbidden field %q", forbidden)
		}
	}
}

func TestAnalyzeAccountFlowsNativeFrameRejectsInvalidIdentityAndArguments(t *testing.T) {
	arguments := nativeComponentTestAccountFlowArguments(
		t,
		nativeComponentTestContext(t, "case-native-frame", time.Date(2026, 7, 27, 1, 0, 0, 0, time.UTC)),
	)
	if frame, err := EncodeAnalyzeAccountFlowsNativeRequestFrameV1("not-a-digest", arguments); err == nil || frame != nil {
		t.Fatalf("invalid request id survived: frame=%q err=%v", frame, err)
	}
	if frame, err := EncodeAnalyzeAccountFlowsNativeRequestFrameV1(strings.Repeat("8", 64), AnalyzeAccountFlowsArgumentsV1{}); err == nil || frame != nil {
		t.Fatalf("invalid arguments survived: frame=%q err=%v", frame, err)
	}
}
