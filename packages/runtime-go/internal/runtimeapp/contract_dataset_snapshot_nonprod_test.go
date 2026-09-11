//go:build !analytix_prod

package runtimeapp

import (
	"context"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
)

func TestContractDatasetSnapshotAuthorityEnabledOnlyForExplicitTempMode(t *testing.T) {
	installation := newContractRiskTestInstallationAuthority()
	for _, test := range []struct {
		name    string
		config  Config
		enabled bool
	}{
		{name: "missing durable temp root", config: Config{}, enabled: false},
		{name: "production root wins", config: Config{DurableTempDir: t.TempDir(), ProductionDurableRoot: t.TempDir()}, enabled: false},
		{name: "candidate root wins", config: Config{DurableTempDir: t.TempDir(), CandidateDurableRoot: t.TempDir()}, enabled: false},
		{name: "explicit contract temp root", config: Config{DurableTempDir: t.TempDir()}, enabled: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			authority, enabled, err := newContractDatasetSnapshotAuthority(test.config, installation)
			if err != nil || enabled != test.enabled || (enabled && authority == nil) || (!enabled && authority != nil) {
				t.Fatalf("contract dataset authority mismatch: authority=%T enabled=%t err=%v", authority, enabled, err)
			}
		})
	}
}

func TestContractDatasetSnapshotAuthorityBindsExactObservationAndPrincipal(t *testing.T) {
	installation := newContractRiskTestInstallationAuthority()
	authority, enabled, err := newContractDatasetSnapshotAuthority(Config{DurableTempDir: t.TempDir()}, installation)
	if err != nil || !enabled || authority == nil {
		t.Fatalf("create contract dataset authority: enabled=%t err=%v", enabled, err)
	}
	observation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: "/workspace/case-a", State: domainsecurity.CaseBindingStateValid, CaseID: "case-a",
		BindingSHA256: domainsecurity.SHA256Hex([]byte("binding-file")), CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")),
	})
	if err != nil {
		t.Fatal(err)
	}
	input := datasetsnapshotport.ResolveInput{TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, Observation: observation}
	first, err := authority.ResolveWitnessed(context.Background(), input)
	if err != nil || domainsecurity.ValidateDatasetSnapshotAuthorityRecordForBindingV1(first, observation) != nil {
		t.Fatalf("resolve witnessed contract snapshot: record=%#v err=%v", first, err)
	}
	second, err := authority.ResolveWitnessed(context.Background(), input)
	if err != nil || first != second {
		t.Fatalf("contract snapshot selection must be deterministic: first=%#v second=%#v err=%v", first, second, err)
	}
	input.Observation.CaseID = "case-b"
	if _, err := authority.ResolveWitnessed(context.Background(), input); err == nil {
		t.Fatal("tampered case observation was accepted")
	}
}
