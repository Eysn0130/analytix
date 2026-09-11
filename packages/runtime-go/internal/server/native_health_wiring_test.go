package server

import (
	"context"
	"errors"
	"testing"
	"time"

	nativecomponentapp "analytix.local/runtime-go/internal/app/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestRuntimeNativeAuthorityNilNeverSkipsExecutableCaseTurn(t *testing.T) {
	securityContext := runtimeNativeHealthCaseContext(t)
	handler := &runtimeServerHandler{}
	err := handler.nativeAuthority.PrepareCaseTurn(context.Background(), securityContext)
	if !errors.Is(err, nativecomponentapp.ErrRuntimeAuthorityUnavailable) {
		t.Fatalf("nil native authority error = %v", err)
	}
}

func TestRuntimeNativeAuthorityIsNotRequiredForBoundaryOnlyTurn(t *testing.T) {
	securityContext, err := securitytest.BoundaryOnlyContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-native-boundary", TurnID: "turn-native-boundary",
		WorkspaceRealPath: "/cases/native-boundary", ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := &runtimeServerHandler{}
	if err := handler.nativeAuthority.PrepareCaseTurn(context.Background(), securityContext); err != nil {
		t.Fatalf("boundary-only native preparation: %v", err)
	}
}

func TestRuntimeNativeAuthorityRejectsMalformedContextBeforeProvider(t *testing.T) {
	handler := &runtimeServerHandler{}
	if err := handler.nativeAuthority.PrepareCaseTurn(
		context.Background(),
		domainsecurity.TurnSecurityContext{},
	); !errors.Is(
		err,
		nativecomponentapp.ErrRuntimeAuthorityUnavailable,
	) {
		t.Fatalf("malformed context error = %v", err)
	}
}

func runtimeNativeHealthCaseContext(t *testing.T) domainsecurity.TurnSecurityContext {
	t.Helper()
	now := time.Now().UTC().Add(-time.Second)
	securityContext, err := securitytest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-native-health", TurnID: "turn-native-health", WorkspaceRealPath: "/cases/native-health",
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, CaseID: "case-native-health",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("native-health-binding")),
		DatasetSnapshotID:  securitytest.DatasetSnapshotID("native-health-snapshot"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("native-health-source")), ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}
