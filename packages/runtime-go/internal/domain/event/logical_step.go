package event

import (
	"regexp"
	"strings"
	"time"

	"analytix.local/runtime-go/internal/contracts"
)

// LogicalStepMetricsVersionV1 marks executions whose event log can prove
// logical closes. A signed provider attempt is billing evidence, not a close.
const LogicalStepMetricsVersionV1 = 1

// ProjectLogicalStepDetailsV1 is the single closed numeric/correlation grammar
// used by the producer, public event projection and full-log usage fold.
func ProjectLogicalStepDetailsV1(stage string, details map[string]any) (map[string]any, bool) {
	version, valid := logicalMetricNumberV1(details["schemaVersion"])
	id, idOK := details["logicalStepId"].(string)
	if !valid || version != LogicalStepMetricsVersionV1 || !idOK || id != strings.TrimSpace(id) || !contracts.IsCanonicalOpaqueUUIDV4(id) {
		return nil, false
	}
	out := map[string]any{"schemaVersion": float64(version), "logicalStepId": id}
	if stage == "logical_step_started" || stage == "logical_step_assembled" {
		return out, len(details) == 2
	}
	if stage != "logical_step_closed" {
		return nil, false
	}
	assembled, valid := details["assistantMessageAssembled"].(bool)
	if !valid {
		return nil, false
	}
	out["assistantMessageAssembled"] = assembled
	for key, value := range details {
		switch key {
		case "schemaVersion", "logicalStepId", "assistantMessageAssembled":
		case "llmMs", "ttftMs", "decodeMs", "decodeTokens":
			number, ok := logicalMetricNumberV1(value)
			if !assembled || !ok {
				return nil, false
			}
			out[key] = float64(number)
		default:
			return nil, false
		}
	}
	llm, hasLLM := out["llmMs"].(float64)
	ttft, hasTTFT := out["ttftMs"].(float64)
	decode, hasDecode := out["decodeMs"].(float64)
	_, hasOutput := out["decodeTokens"].(float64)
	if hasTTFT && (!hasLLM || ttft > llm) || hasDecode != hasOutput || hasDecode && (!hasTTFT || decode != llm-ttft) {
		return nil, false
	}
	return out, true
}

func logicalMetricNumberV1(value any) (uint64, bool) {
	number, ok := contracts.NumericSeq(value)
	return uint64(number), ok && number >= 0 && uint64(number) <= 9007199254740991
}

// ProjectClosedLogicalStepEventDetailsV1 recognizes only the closed root event
// envelope. Before persistence the host may still need to assign seq/timestamp;
// supplied values must already satisfy the durable grammar. This is structural
// classification, not proof of publication authority.
func ProjectClosedLogicalStepEventDetailsV1(event map[string]any) (map[string]any, bool) {
	if !validLogicalStepEnvelopeV1(event) {
		return nil, false
	}
	details, ok := event["details"].(map[string]any)
	if !ok {
		return nil, false
	}
	return ProjectLogicalStepDetailsV1(event["stage"].(string), details)
}

// The public ID and timestamp grammar matches the durable assembled boundary.
var (
	logicalStepPublicIDV1        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,255}$`)
	logicalStepPublicTimestampV1 = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]+)?(?:Z|[+-](?:[01][0-9]|2[0-3]):[0-5][0-9])$`)
)

func validLogicalStepEnvelopeV1(event map[string]any) bool {
	if event["kind"] != "pipeline_stage" {
		return false
	}
	stage, stageOK := event["stage"].(string)
	label := ""
	switch stage {
	case "logical_step_started":
		label = "Logical step started"
	case "logical_step_assembled":
		label = "Logical step assembled"
	case "logical_step_closed":
		label = "Logical step closed"
	default:
		return false
	}
	threadID, threadOK := event["threadId"].(string)
	turnID, turnOK := event["turnId"].(string)
	if !stageOK || event["label"] != label ||
		!threadOK || !logicalStepPublicIDV1.MatchString(threadID) ||
		!turnOK || !logicalStepPublicIDV1.MatchString(turnID) {
		return false
	}
	for key := range event {
		switch key {
		case "kind", "threadId", "turnId", "stage", "label", "details", "seq", "timestamp":
		default:
			return false
		}
	}
	if value, present := event["seq"]; present {
		seq, ok := contracts.NumericSeq(value)
		if !ok || seq < 1 || uint64(seq) > 9007199254740991 {
			return false
		}
	}
	if value, present := event["timestamp"]; present {
		timestamp, ok := value.(string)
		if !ok || len(timestamp) > 64 || !logicalStepPublicTimestampV1.MatchString(timestamp) {
			return false
		}
		at, err := time.Parse(time.RFC3339Nano, timestamp)
		if err != nil || at.IsZero() {
			return false
		}
	}
	return true
}
