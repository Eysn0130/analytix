package nativecomponent

import (
	"errors"
	"strings"
)

const ResultSchemaVersion = 1

var ErrResultInvalid = errors.New("native_component_result_invalid")

// Result is the only app-visible projection from the currently available
// native health operation. Raw helper stdout is adapter-private and can never
// cross the admission boundary.
type Result struct {
	SchemaVersion  int    `json:"schemaVersion"`
	ComponentID    string `json:"componentId"`
	Operation      string `json:"operation"`
	Status         string `json:"status"`
	RegistryDigest string `json:"registryDigest"`
}

func ValidateResult(result Result, policy OperationPolicy) error {
	if policy.ComponentID != ComponentDataEngine || policy.Operation != "health" ||
		result.SchemaVersion != ResultSchemaVersion || result.ComponentID != policy.ComponentID ||
		result.Operation != policy.Operation || result.Status != "ready" ||
		result.RegistryDigest != strings.ToLower(result.RegistryDigest) ||
		len(result.RegistryDigest) != 64 {
		return ErrResultInvalid
	}
	for _, value := range result.RegistryDigest {
		if (value < '0' || value > '9') && (value < 'a' || value > 'f') {
			return ErrResultInvalid
		}
	}
	return nil
}
