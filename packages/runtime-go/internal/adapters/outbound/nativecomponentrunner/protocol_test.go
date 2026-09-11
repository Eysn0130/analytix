package nativecomponentrunner

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
)

const protocolTestNonce = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const protocolTestRequestID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func protocolTestReadinessFrame() []byte {
	return []byte(`{"kind":"analytix_native_ready","schema_version":3,"component_id":"data-engine","launch_nonce":"` + protocolTestNonce + `","protocol_version":"analytix-native-v1","process_id":4242}`)
}

func protocolTestPingResponseFrame() []byte {
	return []byte(`{"request_id":"` + protocolTestRequestID + `","ok":true,"data":{"pong":true,"pid":4242},"diagnostics":{"engine":"analytix-data-engine","command":"ping","case_bound":false,"db_bound":false,"pid":4242,"queue_wait_ms":0,"run_ms":0,"owner_epoch":""}}`)
}

func protocolTestReadinessFrameFor(componentID string) []byte {
	return []byte(`{"kind":"analytix_native_ready","schema_version":3,"component_id":"` + componentID + `","launch_nonce":"` + protocolTestNonce + `","protocol_version":"analytix-native-v1","process_id":4242}`)
}

func protocolTestPingResponseFrameFor(engine string) []byte {
	return []byte(`{"request_id":"` + protocolTestRequestID + `","ok":true,"data":{"pong":true,"pid":4242},"diagnostics":{"engine":"` + engine + `","command":"ping","case_bound":false,"db_bound":false,"pid":4242,"queue_wait_ms":0,"run_ms":0,"owner_epoch":""}}`)
}

func decodeProtocolTestReadiness(frame []byte) error {
	var target readinessEnvelope
	return decodeStrictFrame(frame, readinessFrameLimit, readinessJSONShape, &target)
}

func decodeProtocolTestResponse(frame []byte) error {
	var target pingResponse
	return decodeStrictFrame(frame, responseFrameLimit, pingResponseJSONShape, &target)
}

func TestNativeProtocolRequestEncoderIsTypedDeterministicAndBounded(t *testing.T) {
	requestID := strings.Repeat("a", 64)
	first, err := encodePingRequestFrame(requestID)
	if err != nil {
		t.Fatalf("encode ping: %v", err)
	}
	second, err := encodePingRequestFrame(requestID)
	if err != nil || !bytes.Equal(second, first) {
		t.Fatalf("request encoding is not deterministic: first=%q second=%q err=%v", first, second, err)
	}
	want := `{"request_id":"` + requestID + `","command":"ping"}` + "\n"
	if string(first) != want || len(first) > requestFrameLimit {
		t.Fatalf("request frame = %q, want %q", first, want)
	}
	for _, invalid := range []string{"", strings.Repeat("A", 64), strings.Repeat("a", 65), strings.Repeat("x", requestFrameLimit)} {
		if frame, encodeErr := encodePingRequestFrame(invalid); frame != nil || !errors.Is(encodeErr, ErrProtocol) {
			t.Fatalf("invalid request id survived: len=%d frame=%q err=%v", len(invalid), frame, encodeErr)
		}
	}
}

func TestDeterministicCleaningProtocolReservesTheExactOuterEnvelopeTokens(t *testing.T) {
	dataScalars := domainnative.DeterministicCleaningMaximumResultTokensV1 - 2
	var data strings.Builder
	data.Grow(dataScalars * 2)
	data.WriteByte('[')
	for index := 0; index < dataScalars; index++ {
		if index > 0 {
			data.WriteByte(',')
		}
		data.WriteByte('0')
	}
	data.WriteByte(']')
	frame := []byte(fmt.Sprintf(
		`{"request_id":"%s","ok":true,"data":%s,"diagnostics":{"engine":"analytix-data-engine","command":"funds.deterministic_cleaning_v1","case_bound":true,"db_bound":true,"pid":4242,"queue_wait_ms":0,"run_ms":0,"owner_epoch":""}}`,
		protocolTestRequestID,
		data.String(),
	))
	var response accountFlowResponse
	if err := decodeStrictFrameWithLimits(
		frame,
		deterministicCleaningResponseFrameLimit,
		deterministicCleaningResponseMaximumTokens,
		accountFlowResponseJSONShape,
		&response,
	); err != nil {
		t.Fatalf("cleaning result at the inner token boundary exceeded its fixed outer envelope: %v", err)
	}
	if len(response.Data) == 0 {
		t.Fatal("cleaning boundary frame lost its nested data")
	}
	if err := decodeStrictFrameWithLimits(
		frame,
		deterministicCleaningResponseFrameLimit,
		deterministicCleaningResponseMaximumTokens-1,
		accountFlowResponseJSONShape,
		&accountFlowResponse{},
	); err == nil {
		t.Fatal("cleaning outer frame above its exact token budget was accepted")
	}
}

func TestNativeProbeProtocolBindsEveryComponentToItsExactIdentity(t *testing.T) {
	tests := []struct {
		componentID string
		engine      string
	}{
		{componentID: domainnative.ComponentImportAccelerator, engine: "analytix-import-accelerator"},
		{componentID: domainnative.ComponentCleaningOps, engine: "analytix-cleaning-ops"},
		{componentID: domainnative.ComponentAnalysisCompute, engine: "analytix-analysis-compute"},
		{componentID: domainnative.ComponentDataEngine, engine: "analytix-data-engine"},
	}
	for _, test := range tests {
		t.Run(test.componentID, func(t *testing.T) {
			if !ValidateProbeReadiness(protocolTestReadinessFrameFor(test.componentID), test.componentID, protocolTestNonce, 4242) {
				t.Fatal("exact readiness identity was rejected")
			}
			if !ValidateProbePingResponse(protocolTestPingResponseFrameFor(test.engine), test.componentID, protocolTestRequestID, 4242) {
				t.Fatal("exact ping identity was rejected")
			}
			for _, other := range tests {
				if other.componentID == test.componentID {
					continue
				}
				if ValidateProbeReadiness(protocolTestReadinessFrameFor(other.componentID), test.componentID, protocolTestNonce, 4242) {
					t.Fatalf("readiness for %q was accepted as %q", other.componentID, test.componentID)
				}
				if ValidateProbePingResponse(protocolTestPingResponseFrameFor(other.engine), test.componentID, protocolTestRequestID, 4242) {
					t.Fatalf("engine %q was accepted as %q", other.engine, test.engine)
				}
			}
		})
	}
}

func TestNativeProbeProtocolRejectsUnknownOrMismatchedAuthority(t *testing.T) {
	validReadiness := protocolTestReadinessFrameFor(domainnative.ComponentDataEngine)
	validResponse := protocolTestPingResponseFrameFor("analytix-data-engine")
	for _, invalidComponentID := range []string{"", "unknown", "data_engine", "DATA-ENGINE"} {
		if ValidateProbeReadiness(validReadiness, invalidComponentID, protocolTestNonce, 4242) {
			t.Fatalf("unknown component readiness survived: %q", invalidComponentID)
		}
		if ValidateProbePingResponse(validResponse, invalidComponentID, protocolTestRequestID, 4242) {
			t.Fatalf("unknown component response survived: %q", invalidComponentID)
		}
	}
	if ValidateProbeReadiness(validReadiness, domainnative.ComponentDataEngine, strings.Repeat("c", 64), 4242) ||
		ValidateProbeReadiness(validReadiness, domainnative.ComponentDataEngine, protocolTestNonce, 4243) {
		t.Fatal("readiness survived a nonce or pid mismatch")
	}
	if ValidateProbePingResponse(validResponse, domainnative.ComponentDataEngine, strings.Repeat("c", 64), 4242) ||
		ValidateProbePingResponse(validResponse, domainnative.ComponentDataEngine, protocolTestRequestID, 4243) {
		t.Fatal("ping response survived a request id or pid mismatch")
	}
}

func TestEncodeDataEngineHealthRequestV1RejectsBeforeAllocation(t *testing.T) {
	for _, invalid := range []string{strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64)} {
		frame, err := encodePingRequestFrame(invalid)
		if frame != nil || !errors.Is(err, ErrProtocol) {
			t.Fatalf("invalid id len=%d allocated frame=%q err=%v", len(invalid), frame, err)
		}
	}
}

func TestDecodeStrictFrameRejectsCaseAliasesAtEveryProtocolObjectLevel(t *testing.T) {
	readiness := string(protocolTestReadinessFrame())
	response := string(protocolTestPingResponseFrame())
	fixtures := []struct {
		name   string
		frame  string
		decode func([]byte) error
	}{
		{name: "readiness root", frame: strings.Replace(readiness, `"kind":`, `"KIND":`, 1), decode: decodeProtocolTestReadiness},
		{name: "readiness root exact plus alias", frame: strings.Replace(readiness, `"kind":`, `"KIND":"analytix_native_ready","kind":`, 1), decode: decodeProtocolTestReadiness},
		{name: "readiness component", frame: strings.Replace(readiness, `"component_id":`, `"COMPONENT_ID":`, 1), decode: decodeProtocolTestReadiness},
		{name: "response root", frame: strings.Replace(response, `"request_id":`, `"REQUEST_ID":`, 1), decode: decodeProtocolTestResponse},
		{name: "response data", frame: strings.Replace(response, `"pong":`, `"PONG":`, 1), decode: decodeProtocolTestResponse},
		{name: "response diagnostics", frame: strings.Replace(response, `"owner_epoch":`, `"OWNER_EPOCH":`, 1), decode: decodeProtocolTestResponse},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			if err := fixture.decode([]byte(fixture.frame)); !errors.Is(err, ErrProtocol) {
				t.Fatalf("case alias survived: err=%v frame=%s", err, fixture.frame)
			}
		})
	}
}

func TestDecodeStrictFrameRequiresEveryExactProtocolField(t *testing.T) {
	readiness := string(protocolTestReadinessFrame())
	response := string(protocolTestPingResponseFrame())
	fixtures := []struct {
		name   string
		frame  string
		decode func([]byte) error
	}{
		{name: "missing readiness field", frame: strings.Replace(readiness, `,"protocol_version":"analytix-native-v1"`, "", 1), decode: decodeProtocolTestReadiness},
		{name: "extra readiness field", frame: strings.TrimSuffix(readiness, "}") + `,"extra":true}`, decode: decodeProtocolTestReadiness},
		{name: "target self-reported containment", frame: strings.TrimSuffix(readiness, "}") + `,"process_containment":{"mechanism":"darwin_rlimit_nproc","soft_limit":0,"hard_limit":0,"fork_probe":{"operation":"fork","outcome":"denied","errno":"EAGAIN"}}}`, decode: decodeProtocolTestReadiness},
		{name: "missing response field", frame: strings.Replace(response, `"engine":"analytix-data-engine",`, "", 1), decode: decodeProtocolTestResponse},
		{name: "extra nested response field", frame: strings.Replace(response, `"pong":true`, `"pong":true,"extra":true`, 1), decode: decodeProtocolTestResponse},
		{name: "array response data", frame: strings.Replace(response, `"data":{"pong":true,"pid":4242}`, `"data":[]`, 1), decode: decodeProtocolTestResponse},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			if err := fixture.decode([]byte(fixture.frame)); !errors.Is(err, ErrProtocol) {
				t.Fatalf("invalid shape survived: err=%v frame=%s", err, fixture.frame)
			}
		})
	}
}

func TestReadinessV3RejectsTargetSelfReportedContainmentAuthority(t *testing.T) {
	frame := strings.TrimSuffix(string(protocolTestReadinessFrame()), "}") +
		`,"process_containment":{"mechanism":"darwin_rlimit_nproc","soft_limit":0,"hard_limit":0,"fork_probe":{"operation":"fork","outcome":"denied","errno":"EAGAIN"}}}`
	if err := decodeProtocolTestReadiness([]byte(frame)); !errors.Is(err, ErrProtocol) {
		t.Fatalf("target containment claim survived exact readiness schema: err=%v frame=%s", err, frame)
	}
}

func TestDecodeStrictFrameRejectsBOMAndUnsignedNegativeZero(t *testing.T) {
	readiness := string(protocolTestReadinessFrame())
	response := string(protocolTestPingResponseFrame())
	fixtures := []struct {
		name   string
		frame  []byte
		decode func([]byte) error
	}{
		{name: "BOM", frame: append([]byte{0xef, 0xbb, 0xbf}, protocolTestReadinessFrame()...), decode: decodeProtocolTestReadiness},
		{name: "invalid UTF-8", frame: append([]byte(`{"kind":"`), 0xff), decode: decodeProtocolTestReadiness},
		{name: "negative zero queue wait", frame: []byte(strings.Replace(response, `"queue_wait_ms":0`, `"queue_wait_ms":-0`, 1)), decode: decodeProtocolTestResponse},
		{name: "negative zero run", frame: []byte(strings.Replace(response, `"run_ms":0`, `"run_ms":-0`, 1)), decode: decodeProtocolTestResponse},
		{name: "fractional schema", frame: []byte(strings.Replace(readiness, `"schema_version":3`, `"schema_version":3.0`, 1)), decode: decodeProtocolTestReadiness},
		{name: "exponent uint", frame: []byte(strings.Replace(response, `"run_ms":0`, `"run_ms":0e0`, 1)), decode: decodeProtocolTestResponse},
		{name: "escaped duplicate", frame: []byte(strings.Replace(readiness, `"kind":"analytix_native_ready"`, `"kind":"analytix_native_ready","k\u0069nd":"analytix_native_ready"`, 1)), decode: decodeProtocolTestReadiness},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			if err := fixture.decode(fixture.frame); !errors.Is(err, ErrProtocol) {
				t.Fatalf("ambiguous protocol frame survived: err=%v frame=%q", err, fixture.frame)
			}
		})
	}
}

func TestDecodeStrictFrameHonorsTransportFrameBoundary(t *testing.T) {
	tests := []struct {
		name   string
		frame  []byte
		limit  int
		decode func([]byte) error
	}{
		{name: "readiness", frame: protocolTestReadinessFrame(), limit: readinessFrameLimit, decode: decodeProtocolTestReadiness},
		{name: "response", frame: protocolTestPingResponseFrame(), limit: responseFrameLimit, decode: decodeProtocolTestResponse},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			atLimit := append(append([]byte(nil), test.frame...), bytes.Repeat([]byte(" "), test.limit-1-len(test.frame))...)
			if len(atLimit) != test.limit-1 {
				t.Fatalf("fixture len=%d, want %d", len(atLimit), test.limit-1)
			}
			if err := test.decode(atLimit); err != nil {
				t.Fatalf("maximum transport payload was rejected: %v", err)
			}
			overLimit := append(append([]byte(nil), atLimit...), ' ')
			if err := test.decode(overLimit); !errors.Is(err, ErrProtocol) {
				t.Fatalf("payload without LF allowance survived: len=%d err=%v", len(overLimit), err)
			}
		})
	}
}
