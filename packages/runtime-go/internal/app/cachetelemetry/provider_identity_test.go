package cachetelemetry

import (
	"context"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestStableProviderUserIDSurvivesServiceRestartAndSeparatesPrivacyDomains(t *testing.T) {
	fixture := newDurableServiceFixtureV1(t)
	service, _ := NewDurableService(fixture.authority, fixture.store)
	reopened, _ := NewDurableService(fixture.authority, fixture.store)
	scope := func(turn, workspace, caseID string) domainsecurity.TurnSecurityContext {
		value, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
			ThreadID: "thread-identity", TurnID: turn, WorkspaceRealPath: workspace,
			CaseID: caseID, CaseBindingHash: domainsecurity.SHA256Hex([]byte(caseID)),
			DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot"), SourceManifestHash: domainsecurity.SHA256Hex([]byte("sources")), ContextEpoch: 3,
			IssuedAt: time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	a, err := service.DeriveStableProviderUserIDV1(context.Background(), scope("turn-1", "/workspace-a", "case-a"), "provider-a")
	if err != nil || len(a) != 68 {
		t.Fatal("derive identity", err)
	}
	b, err := reopened.DeriveStableProviderUserIDV1(context.Background(), scope("turn-2", "/workspace-a", "case-a"), "provider-a")
	if err != nil || a != b {
		t.Fatal("identity changed with turn or service restart")
	}
	for _, tc := range []struct{ workspace, caseID, provider string }{{"/workspace-b", "case-a", "provider-a"}, {"/workspace-a", "case-b", "provider-a"}, {"/workspace-a", "case-a", "provider-b"}} {
		other, err := reopened.DeriveStableProviderUserIDV1(context.Background(), scope("turn-3", tc.workspace, tc.caseID), tc.provider)
		if err != nil || other == a {
			t.Fatal("privacy domains collided")
		}
	}
	if strings.Contains(a, "workspace") || strings.Contains(a, "case-a") || strings.Contains(a, "provider") {
		t.Fatal("identity exposed scope")
	}
	if _, err := service.DeriveStableProviderUserIDV1(context.Background(), domainsecurity.TurnSecurityContext{}, "provider-a"); err == nil {
		t.Fatal("invalid scope accepted")
	}
}
