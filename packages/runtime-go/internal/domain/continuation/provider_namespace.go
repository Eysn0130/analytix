package continuation

import (
	"errors"
	"strings"

	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainjob "analytix.local/runtime-go/internal/domain/job"
)

const ProviderContinuationNamespaceVersionV1 = 1

type TerminalRecoveryKindV1 string

const (
	TerminalRecoveryNoneV1      TerminalRecoveryKindV1 = ""
	TerminalRecoveryAppliedV1   TerminalRecoveryKindV1 = "recovery"
	TerminalRecoveryStepLimitV1 TerminalRecoveryKindV1 = "step_limit"
)

// ProviderContinuationNamespaceV1 freezes the provider/cache and delegated
// tool namespace on a private approval or user-input receipt. Outer provider
// retry and vision-lane counters deliberately restart inside the next logical
// call; only the completed logical sequence crosses the pause boundary.
type ProviderContinuationNamespaceV1 struct {
	Version               int                                        `json:"version"`
	UsageSource           domaincachetelemetry.ProviderUsageSourceV1 `json:"usageSource"`
	ChildRunID            string                                     `json:"childRunId"`
	ProviderCallSequence  uint64                                     `json:"providerCallSequence"`
	DelegatedToolManifest *domainjob.DelegatedToolManifestV1         `json:"delegatedToolManifest"`
}

func ValidateTerminalRecoveryKindV1(kind TerminalRecoveryKindV1) error {
	switch kind {
	case TerminalRecoveryNoneV1, TerminalRecoveryAppliedV1, TerminalRecoveryStepLimitV1:
		return nil
	default:
		return errors.New("continuation terminal recovery kind is invalid")
	}
}

func NewProviderContinuationNamespaceV1(usageSource, childRunID string, providerCallSequence uint64, manifest *domainjob.DelegatedToolManifestV1) ProviderContinuationNamespaceV1 {
	return ProviderContinuationNamespaceV1{
		Version:     ProviderContinuationNamespaceVersionV1,
		UsageSource: domaincachetelemetry.NormalizeProviderUsageSourceV1(usageSource),
		ChildRunID:  strings.TrimSpace(childRunID), ProviderCallSequence: providerCallSequence,
		DelegatedToolManifest: domainjob.CloneDelegatedToolManifestV1(manifest),
	}
}

func ValidateProviderContinuationNamespaceV1(namespace ProviderContinuationNamespaceV1, subagentDepth int, toolScope []string) error {
	if namespace.Version != ProviderContinuationNamespaceVersionV1 || namespace.ProviderCallSequence == 0 ||
		namespace.ChildRunID != strings.TrimSpace(namespace.ChildRunID) {
		return errors.New("provider continuation namespace identity is invalid")
	}
	isSubagent := namespace.UsageSource == domaincachetelemetry.ProviderUsageSourceSubagent
	if !isSubagent && namespace.UsageSource != domaincachetelemetry.ProviderUsageSourceTurn {
		return errors.New("provider continuation usage source is invalid")
	}
	if isSubagent != (subagentDepth > 0) || isSubagent != (namespace.ChildRunID != "") || isSubagent != (namespace.DelegatedToolManifest != nil) {
		return errors.New("provider continuation child namespace is inconsistent")
	}
	if !isSubagent {
		return nil
	}
	if !validText(namespace.ChildRunID, true) || len(namespace.ChildRunID) > maxContinuationTextBytes ||
		domainjob.ValidateDelegatedToolManifestV1(namespace.DelegatedToolManifest, toolScope, namespace.DelegatedToolManifest.ToolSchemaHash) != nil {
		return errors.New("provider continuation delegated namespace is invalid")
	}
	return nil
}

func CloneProviderContinuationNamespaceV1(namespace ProviderContinuationNamespaceV1) ProviderContinuationNamespaceV1 {
	namespace.DelegatedToolManifest = domainjob.CloneDelegatedToolManifestV1(namespace.DelegatedToolManifest)
	return namespace
}
