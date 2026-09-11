package usage

import (
	"encoding/json"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainterminaltelemetry "analytix.local/runtime-go/internal/domain/terminaltelemetry"
)

const PrefixBaselineSchemaV1 = domainterminaltelemetry.PrefixBaselineSchemaV1

type PrefixBaseline struct {
	SchemaVersion           string
	ContinuityDigest        string
	ProviderNamespaceDigest string
	Shape                   domainmodel.PrefixShape
}

type continuityScopeV1 struct {
	SchemaVersion      string `json:"schemaVersion"`
	TenantID           string `json:"tenantId"`
	UserID             string `json:"userId"`
	ThreadID           string `json:"threadId"`
	WorkspaceRealPath  string `json:"workspaceRealPath"`
	CaseID             string `json:"caseId"`
	CaseBindingHash    string `json:"caseBindingHash"`
	DatasetSnapshotID  string `json:"datasetSnapshotId"`
	SourceManifestHash string `json:"sourceManifestHash"`
	ContextEpoch       uint64 `json:"contextEpoch"`
}

type providerNamespaceV1 struct {
	SchemaVersion  string `json:"schemaVersion"`
	Provider       string `json:"provider"`
	ProviderID     string `json:"providerId"`
	EndpointFormat string `json:"endpointFormat"`
	Model          string `json:"model"`
	Route          string `json:"route"`
}

func NewPrefixBaseline(context domainsecurity.TurnSecurityContext, shape domainmodel.PrefixShape) (PrefixBaseline, bool) {
	if domainsecurity.ValidateTurnSecurityContextForExecution(context) != nil || strings.TrimSpace(shape.PrefixHash) == "" {
		return PrefixBaseline{}, false
	}
	continuityBody, err := json.Marshal(continuityScopeV1{
		SchemaVersion: PrefixBaselineSchemaV1, TenantID: context.TenantID, UserID: context.UserID,
		ThreadID: context.ThreadID, WorkspaceRealPath: context.WorkspaceRealPath, CaseID: context.CaseID,
		CaseBindingHash: context.CaseBindingHash, DatasetSnapshotID: context.DatasetSnapshotID,
		SourceManifestHash: context.SourceManifestHash, ContextEpoch: context.ContextEpoch,
	})
	if err != nil {
		return PrefixBaseline{}, false
	}
	namespaceBody, err := json.Marshal(providerNamespaceV1{
		SchemaVersion: PrefixBaselineSchemaV1, Provider: strings.TrimSpace(shape.Provider),
		ProviderID: strings.TrimSpace(shape.ProviderID), EndpointFormat: strings.TrimSpace(shape.EndpointFormat),
		Model: strings.TrimSpace(shape.Model), Route: strings.TrimSpace(shape.Route),
	})
	if err != nil {
		return PrefixBaseline{}, false
	}
	return PrefixBaseline{
		SchemaVersion: PrefixBaselineSchemaV1, ContinuityDigest: domainmodel.BytesHash(continuityBody),
		ProviderNamespaceDigest: domainmodel.BytesHash(namespaceBody), Shape: shape,
	}, true
}

func ComparablePrefix(previous PrefixBaseline, current PrefixBaseline) domainmodel.PrefixShape {
	if previous.SchemaVersion != PrefixBaselineSchemaV1 || current.SchemaVersion != PrefixBaselineSchemaV1 ||
		previous.ContinuityDigest == "" || previous.ContinuityDigest != current.ContinuityDigest ||
		previous.ProviderNamespaceDigest == "" || previous.ProviderNamespaceDigest != current.ProviderNamespaceDigest {
		return domainmodel.PrefixShape{}
	}
	return previous.Shape
}

func PrefixBaselineDiagnostics(baseline PrefixBaseline) map[string]any {
	if baseline.SchemaVersion != PrefixBaselineSchemaV1 || baseline.ContinuityDigest == "" || baseline.ProviderNamespaceDigest == "" {
		return nil
	}
	return map[string]any{
		"cacheBaselineSchema":          PrefixBaselineSchemaV1,
		"cacheContinuityDigest":        baseline.ContinuityDigest,
		"cacheProviderNamespaceDigest": baseline.ProviderNamespaceDigest,
	}
}
