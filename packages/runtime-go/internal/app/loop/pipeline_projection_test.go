package loop

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestProviderPipelineStageRejectsUnknownFieldsAndTrace(t *testing.T) {
	malicious := map[string]any{
		"attempt": float64(1), "rawResponse": "SOL_PRIVATE_TRACE_7C", "account": "6222021234567890",
	}
	if _, _, err := projectPipelineTelemetry("pre_send", malicious, nil, true); !errors.Is(err, errInvalidPipelineTelemetry) {
		t.Fatalf("unknown provider telemetry was accepted: %v", err)
	}
	if _, _, err := projectPipelineTelemetry("pre_send", map[string]any{"attempt": float64(1)}, map[string]any{
		"private_branch": "private branch alpha was considered before answering",
	}, true); !errors.Is(err, errInvalidPipelineTelemetry) {
		t.Fatalf("unknown provider trace was accepted: %v", err)
	}
}

func TestProviderPreSendTelemetryDoesNotClaimTransportAlreadySent(t *testing.T) {
	details, trace, err := projectPipelineTelemetry("pre_send", map[string]any{
		"attempt": float64(1), "physicalAttempt": float64(1),
		"provider_request_pre_send_at": float64(10),
	}, map[string]any{"provider_request_pre_send_at": float64(10)}, true)
	if err != nil || details["provider_request_pre_send_at"] != float64(10) ||
		trace["provider_request_pre_send_at"] != float64(10) {
		t.Fatalf("pre-send telemetry projection mismatch: details=%#v trace=%#v err=%v", details, trace, err)
	}
	if _, _, err := projectPipelineTelemetry("pre_send", map[string]any{
		"attempt": float64(1), "provider_request_sent_at": float64(10),
	}, nil, false); !errors.Is(err, errInvalidPipelineTelemetry) {
		t.Fatalf("pre-send telemetry claimed transport was already sent: %v", err)
	}
}

func TestProviderPipelineStageNormalizesFinishReasonAndKeepsCounters(t *testing.T) {
	details, trace, err := projectPipelineTelemetry("response_received", map[string]any{
		"stopReason": "SOL_PRIVATE_TRACE_7C", "toolCallCount": float64(2), "streamCompleted": true,
		"durationMs": float64(100), "firstTokenLatencyMs": float64(20),
	}, map[string]any{"provider_request_sent_at": float64(10)}, true)
	if err != nil {
		t.Fatal(err)
	}
	if details["stopReason"] != "unknown" || details["toolCallCount"] != float64(2) || trace["provider_request_sent_at"] != float64(10) {
		t.Fatalf("typed pipeline projection mismatch: details=%#v trace=%#v", details, trace)
	}
	if serialized := fmt.Sprint(details); strings.Contains(serialized, "SOL_PRIVATE_TRACE_7C") {
		t.Fatalf("provider finish reason leaked through projection: %s", serialized)
	}
}

func TestProviderPipelineStageRejectsUnknownStage(t *testing.T) {
	if _, _, err := projectPipelineTelemetry("provider_private_trace", nil, nil, false); !errors.Is(err, errInvalidPipelineTelemetry) {
		t.Fatalf("unknown pipeline stage was accepted: %v", err)
	}
}
