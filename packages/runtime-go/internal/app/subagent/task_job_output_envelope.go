package subagent

import (
	"errors"
	"math"
	"strings"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

const (
	taskJobOutputSchemaVersionV1       = 1
	taskJobOutputWithheldV1            = "withheld"
	taskJobOutputSecurityBoundReasonV1 = "security_bound_child_output"
)

// TaskJobOutputWithheldResponseV1 is a closed metadata-only response. The
// fixed booleans are intentional defense in depth for legacy consumers, while
// availability is the authoritative discriminator for new consumers.
func TaskJobOutputWithheldResponseV1(record domainjob.Record) map[string]any {
	status := normalizedChildStatus(record.Status)
	return map[string]any{
		"schemaVersion":     taskJobOutputSchemaVersionV1,
		"availability":      taskJobOutputWithheldV1,
		"jobId":             publicTaskJobReferenceIDV1(record.ID),
		"status":            status,
		"reasonCode":        taskJobOutputSecurityBoundReasonV1,
		"outputWithheld":    true,
		"outputTrustStatus": untrustedChildOutputStatus,
		"factAnswerAllowed": false,
		"evidenceAuthority": false,
		"canReadOutput":     false,
		"canContinueParent": false,
	}
}

// ValidateTaskJobOutputResponseV1 rejects mixed variants and every unknown
// field before a task-job response reaches HTTP, provider, or desktop IPC.
func ValidateTaskJobOutputResponseV1(value map[string]any) error {
	if len(value) == 0 || !numericLiteralV1(value["schemaVersion"], taskJobOutputSchemaVersionV1) ||
		strings.TrimSpace(firstNonEmptyAnyString(value["jobId"])) == "" ||
		domainjob.PublicStatusV1(firstNonEmptyAnyString(value["status"])) != strings.TrimSpace(firstNonEmptyAnyString(value["status"])) {
		return errors.New("task-job output response identity is invalid")
	}
	availability := strings.TrimSpace(firstNonEmptyAnyString(value["availability"]))
	switch availability {
	case taskJobOutputWithheldV1:
		if !exactTaskJobOutputKeysV1(value, []string{
			"schemaVersion", "availability", "jobId", "status", "reasonCode", "outputWithheld", "outputTrustStatus",
			"factAnswerAllowed", "evidenceAuthority", "canReadOutput", "canContinueParent",
		}) || strings.TrimSpace(firstNonEmptyAnyString(value["reasonCode"])) != taskJobOutputSecurityBoundReasonV1 ||
			strings.TrimSpace(firstNonEmptyAnyString(value["outputTrustStatus"])) != untrustedChildOutputStatus ||
			value["outputWithheld"] != true || value["factAnswerAllowed"] != false || value["evidenceAuthority"] != false ||
			value["canReadOutput"] != false || value["canContinueParent"] != false {
			return errors.New("task-job withheld output response shape is invalid")
		}
		return nil
	default:
		return errors.New("task-job output response availability is invalid")
	}
}

func exactTaskJobOutputKeysV1(value map[string]any, expected []string) bool {
	if len(value) != len(expected) {
		return false
	}
	for _, key := range expected {
		if _, ok := value[key]; !ok {
			return false
		}
	}
	return true
}

func numericLiteralV1(value any, expected int) bool {
	number, ok := numericValueV1(value)
	return ok && number == float64(expected)
}

func numericValueV1(value any) (float64, bool) {
	var number float64
	switch typed := value.(type) {
	case int:
		number = float64(typed)
	case int32:
		number = float64(typed)
	case int64:
		number = float64(typed)
	case uint:
		number = float64(typed)
	case uint32:
		number = float64(typed)
	case uint64:
		number = float64(typed)
	case float32:
		number = float64(typed)
	case float64:
		number = typed
	default:
		return 0, false
	}
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, false
	}
	return number, true
}
