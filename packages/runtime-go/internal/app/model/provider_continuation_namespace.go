package model

import (
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func ProviderNamespace(usageSource, childRunID string, providerCallSequence uint64, manifest *domainjob.DelegatedToolManifestV1) domaincontinuation.ProviderContinuationNamespaceV1 {
	return domaincontinuation.NewProviderContinuationNamespaceV1(usageSource, childRunID, providerCallSequence, manifest)
}

func ProviderNamespaceRuntimeValues(namespace domaincontinuation.ProviderContinuationNamespaceV1) (string, string, uint64, *domainjob.DelegatedToolManifestV1) {
	namespace = domaincontinuation.CloneProviderContinuationNamespaceV1(namespace)
	return string(namespace.UsageSource), namespace.ChildRunID, namespace.ProviderCallSequence, namespace.DelegatedToolManifest
}

func PendingProviderRuntimeValues(pending PendingToolCall) (string, string, uint64, *domainjob.DelegatedToolManifestV1, domaincontinuation.TerminalRecoveryKindV1) {
	usageSource, childRunID, sequence, manifest := ProviderNamespaceRuntimeValues(pending.ProviderNamespace)
	return usageSource, childRunID, sequence, manifest, pending.TerminalRecoveryKind
}
