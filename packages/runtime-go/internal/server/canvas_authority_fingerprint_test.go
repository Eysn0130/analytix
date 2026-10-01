package server

import (
	"context"
	"strings"
	"testing"

	"analytix.local/runtime-go/internal/adapters/outbound/filestore"
	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	riskapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	security "analytix.local/runtime-go/internal/domain/security"
)

// These test fingerprint invalidation only. Current witness proof validation
// still runs in the production projector before this pure helper is used.
func TestCanvasFingerprintBindsStableAuthorityNotPerReadWitnessProof(t *testing.T) {
	p, thread, scope := newSelectionProjectorFixture(t)
	risk, err := riskapp.NewGeneralOnlyAuthority(filestore.CaseBindingReader{})
	if err != nil {
		t.Fatal(err)
	}
	p.handler.turnSecurity.RiskAuthority = risk
	current, err := p.selectionSecurityContext(context.Background(), thread, scope)
	if err != nil {
		t.Fatal(err)
	}
	current.RiskAuthorityBinding = security.RiskAuthorityBindingV1{SchemaVersion: 1, Purpose: "test-shape", State: security.RiskAuthorityBindingStateWitnessed, IndexDigest: strings.Repeat("a", 64), CheckpointDigest: strings.Repeat("b", 64), ObservationDigest: strings.Repeat("c", 64), Generation: 3}
	first, err := selectionAuthorityFingerprint(scope, current)
	if err != nil {
		t.Fatal(err)
	}
	next := current
	next.RiskAuthorityBinding.ObservationDigest = strings.Repeat("d", 64)
	next.TurnID = "new-turn"
	next.IssuedAt = "new timestamp"
	next.ContextDigest = "new digest"
	same, err := selectionAuthorityFingerprint(scope, next)
	if err != nil || same != first {
		t.Fatal("fresh proof/turn invalidates stable scope", same, err)
	}
	changes := map[string]func(*editingapp.ScopeAuthority, *security.TurnSecurityContext){
		"thread":           func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) { s.ThreadID += "new" },
		"workspace":        func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) { s.Workspace += "new" },
		"object":           func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) { s.ObjectID += "new" },
		"path":             func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) { s.Path += "new" },
		"identity-version": func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) { s.Principal.Version++ },
		"installation": func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) {
			s.Principal.InstallationID += "new"
		},
		"principal-digest": func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) {
			s.Principal.PrincipalDigest += "new"
		},
		"tenant":   func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) { s.Principal.TenantID += "new" },
		"user":     func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) { s.Principal.UserID += "new" },
		"case":     func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) { c.CaseID += "new" },
		"binding":  func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) { c.CaseBindingHash += "new" },
		"dataset":  func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) { c.DatasetSnapshotID += "new" },
		"manifest": func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) { c.SourceManifestHash += "new" },
		"epoch":    func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) { c.ContextEpoch++ },
		"publication": func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) {
			c.PublicationPolicy.PolicyDigest += "new"
		},
		"risk-index": func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) {
			c.RiskAuthorityBinding.IndexDigest += "new"
		},
		"checkpoint": func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) {
			c.RiskAuthorityBinding.CheckpointDigest += "new"
		},
		"generation": func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) {
			c.RiskAuthorityBinding.Generation++
		},
		"risk-state": func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) {
			c.RiskAuthorityBinding.State += "new"
		},
		"risk-policy": func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) {
			c.RiskAuthorityBinding.ThreadPolicyDigest += "new"
		},
		"general-policy": func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) {
			c.RiskAuthorityBinding.GeneralPolicyDigest += "new"
		},
		"host-policy": func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) {
			c.RiskAuthorityBinding.HostPolicyDigest += "new"
		},
		"risk-binding-observation": func(s *editingapp.ScopeAuthority, c *security.TurnSecurityContext) {
			c.RiskAuthorityBinding.BindingObservationDigest += "new"
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			s, c := scope, current
			change(&s, &c)
			value, e := selectionAuthorityFingerprint(s, c)
			if e != nil || value == first {
				t.Fatal("changed stable security tuple accepted", e)
			}
		})
	}
}
