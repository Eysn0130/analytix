package toolcatalog

import (
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func toolCatalogWitnessedBoundaryContext(t *testing.T, suffix string) domainsecurity.TurnSecurityContext {
	t.Helper()
	issuedAt := time.Date(2026, 7, 27, 13, 0, 0, 0, time.UTC)
	boundary, err := securitycontexttest.BoundaryOnlyContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-tool-boundary-" + suffix, TurnID: "turn-tool-boundary-" + suffix,
		WorkspaceRealPath: "/workspace/tool-boundary-" + suffix, ContextEpoch: 1, IssuedAt: issuedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding(
		boundary.ThreadID,
		boundary.WorkspaceRealPath,
		domainsecurity.RiskClassCase,
		boundary.PublicationPolicy.ThreadRiskPolicyDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: boundary.ThreadID, TurnID: boundary.TurnID, WorkspaceRealPath: boundary.WorkspaceRealPath,
		TenantID: boundary.TenantID, UserID: boundary.UserID, CaseID: boundary.CaseID,
		CaseBindingHash: boundary.CaseBindingHash, DatasetSnapshotID: boundary.DatasetSnapshotID,
		SourceManifestHash: boundary.SourceManifestHash, ContextEpoch: boundary.ContextEpoch, IssuedAt: issuedAt,
		PublicationPolicy: boundary.PublicationPolicy, RiskAuthorityBinding: binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}
