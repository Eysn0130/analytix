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

func TestProviderConnectionMilestonesRemainClosedAndNumeric(t *testing.T) {
	details := map[string]any{
		"provider_connection_reused": true, "provider_connection_was_idle": true,
		"provider_connection_acquired_ms": float64(1), "provider_connection_idle_ms": float64(20),
		"provider_dns_done_ms": float64(0.1), "provider_connect_done_ms": float64(0.2),
		"provider_tls_done_ms": float64(0.4), "provider_request_written_ms": float64(2),
		"provider_first_response_byte_ms": float64(3),
	}
	if _, _, err := projectPipelineTelemetry("post_send", details, nil, false); err != nil {
		t.Fatal(err)
	}
	details["connection"] = "synthetic-private-address"
	if _, _, err := projectPipelineTelemetry("post_send", details, nil, false); err == nil {
		t.Fatal("connection objects or addresses must not become telemetry")
	}
}

func TestRetryLifecycleProjectionIsClosedAcrossPhases(t *testing.T) {
	valid := func(phase string) map[string]any {
		m := map[string]any{"schemaVersion": float64(1), "source": "transport_reconnect", "phase": phase, "logicalSequence": float64(3), "outerAttempt": float64(2), "physicalAttempt": float64(4)}
		if phase == "scheduled" {
			m["delayMs"] = float64(0)
			m["deadlineUnixMs"] = float64(1700000000000)
		}
		if phase == "cancelled" {
			m["cancelReason"] = "attempt_rejected"
		}
		return m
	}
	for _, phase := range []string{"scheduled", "started", "cancelled"} {
		if _, err := projectProviderRetryV1(valid(phase)); err != nil {
			t.Fatalf("valid phase=%s err=%v", phase, err)
		}
	}
	for _, name := range []string{"private_key", "version", "source", "phase", "missing_sequence", "fractional_sequence", "missing_physical", "outer_physical", "missing_delay", "missing_deadline", "scheduled_reason", "started_delay", "cancelled_reason", "unknown_reason"} {
		t.Run(name, func(t *testing.T) {
			m := valid("scheduled")
			switch name {
			case "private_key":
				m["privateBody"] = "PRIVATE_RESPONSE"
			case "version":
				m["schemaVersion"] = float64(2)
			case "source":
				m["source"] = "private"
			case "phase":
				m["phase"] = "elapsed"
			case "missing_sequence":
				delete(m, "logicalSequence")
			case "fractional_sequence":
				m["logicalSequence"] = 1.5
			case "missing_physical":
				delete(m, "physicalAttempt")
			case "outer_physical":
				m = valid("started")
				m["source"] = "provider_retry"
				m["logicalSequence"] = float64(7)
				m["physicalAttempt"] = float64(3)
			case "missing_delay":
				delete(m, "delayMs")
			case "missing_deadline":
				delete(m, "deadlineUnixMs")
			case "scheduled_reason":
				m["cancelReason"] = "context_cancelled"
			case "started_delay":
				m["phase"] = "started"
			case "cancelled_reason":
				m = valid("cancelled")
				delete(m, "cancelReason")
			case "unknown_reason":
				m = valid("cancelled")
				m["cancelReason"] = "PRIVATE_RAW_REASON"
			}
			if _, err := projectProviderRetryV1(m); !errors.Is(err, errInvalidPipelineTelemetry) {
				t.Fatal("invalid lifecycle accepted")
			}
		})
	}
}

func TestResponseObservationLogicalSequenceRequiresPositiveInteger(t *testing.T) {
	for _, value := range []any{float64(1), 2, int64(3)} {
		if _, _, err := projectPipelineTelemetry("response_received", map[string]any{"logicalSequence": value, "stopReason": "length", "streamCompleted": true}, nil, false); err != nil {
			t.Fatalf("completed cap sequence=%v: %v", value, err)
		}
		if _, _, err := projectPipelineTelemetry("response_received", map[string]any{"logicalSequence": value, "stopReason": "length", "streamCompleted": false}, nil, false); err != nil {
			t.Fatalf("valid sequence=%v: %v", value, err)
		}
	}
	for _, value := range []any{float64(0), -1, 1.5, "1", true} {
		if _, _, err := projectPipelineTelemetry("response_received", map[string]any{"logicalSequence": value, "stopReason": "length", "streamCompleted": false}, nil, false); !errors.Is(err, errInvalidPipelineTelemetry) {
			t.Fatalf("invalid sequence=%v was accepted", value)
		}
	}
}

func TestResponseObservationSequenceDoesNotExpandOrdinaryResponseContract(t *testing.T) {
	for _, details := range []map[string]any{
		{"logicalSequence": float64(1), "stopReason": "stop", "streamCompleted": false},
		{"logicalSequence": float64(1), "streamCompleted": false},
		{"logicalSequence": float64(1), "stopReason": "length"},
		{"logicalSequence": float64(1), "stopReason": "length", "streamCompleted": false, "toolCallCount": 1.5},
	} {
		if _, _, err := projectPipelineTelemetry("response_received", details, nil, false); !errors.Is(err, errInvalidPipelineTelemetry) {
			t.Fatal("non-cap response invented observation sequence")
		}
	}
	if _, _, err := projectPipelineTelemetry("response_received", map[string]any{"stopReason": "stop", "streamCompleted": true, "toolCallCount": float64(0)}, nil, false); err != nil {
		t.Fatal("ordinary response regressed")
	}
}

func TestLogicalStepPipelineWireIsClosedV1(t *testing.T) {
	id := "f13e1680-e522-4d40-bc18-40dde3bb66ea"
	for _, stage := range []string{"logical_step_started", "logical_step_assembled", "logical_step_closed"} {
		details := map[string]any{"schemaVersion": float64(1), "logicalStepId": id}
		if stage == "logical_step_closed" {
			details["assistantMessageAssembled"] = true
			details["llmMs"] = float64(4)
			details["ttftMs"] = float64(1)
			details["decodeMs"] = float64(3)
			details["decodeTokens"] = float64(0)
		}
		if _, _, err := projectPipelineTelemetry(stage, details, nil, true); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"extra", "version", "identity", "trace", "fraction", "pair", "order", "unassembled"} {
			t.Run(stage+"/"+name, func(t *testing.T) {
				bad := map[string]any{}
				for k, v := range details {
					bad[k] = v
				}
				var trace map[string]any
				switch name {
				case "extra":
					bad["StepBindingHMAC"] = "private"
				case "version":
					bad["schemaVersion"] = float64(2)
				case "identity":
					bad["logicalStepId"] = "PRIVATE_ID"
				case "trace":
					trace = map[string]any{"provider_request_sent_at": float64(1)}
				case "fraction":
					bad["llmMs"] = float64(0.5)
				case "pair":
					delete(bad, "decodeTokens")
					if stage != "logical_step_closed" {
						bad["decodeMs"] = float64(1)
					}
				case "order":
					bad["ttftMs"] = float64(5)
				case "unassembled":
					bad["assistantMessageAssembled"] = false
					if stage == "logical_step_closed" {
						bad["llmMs"] = float64(0)
					}
				}
				if _, _, err := projectPipelineTelemetry(stage, bad, trace, true); err == nil {
					t.Fatalf("invalid wire accepted: %s %#v", name, bad)
				}
			})
		}
	}
	for _, value := range []any{nil, float64(0), float64(2), "1"} {
		if _, _, err := projectPipelineTelemetry("post_start", map[string]any{"logicalStepMetricsVersion": value}, nil, false); err == nil {
			t.Fatalf("invalid marker: %#v", value)
		}
	}
}
