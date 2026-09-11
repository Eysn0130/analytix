package plugincapability

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	domainpluginpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
)

func TestFundsSourceReadLifecycleExactGrantAndFreshRecovery(t *testing.T) {
	binding := fundsSourceReadBindingFixtureV1(t, []domainpluginpackage.CapabilityRequestV1{
		exactFundsSourceReadRequestFixtureV1(),
	})
	lifecycle, setup, err := NewFundsSourceReadLifecycleV1(binding)
	if err != nil || setup.State != FundsSourceReadStateSetupV1 ||
		setup.Decision != FundsSourceReadDecisionPendingV1 {
		t.Fatalf("exact binding did not start in setup without a grant: event=%#v err=%v", setup, err)
	}
	if _, ok := lifecycle.CurrentGrant(); ok {
		t.Fatal("setup minted a source-read grant")
	}
	authority1 := fundsSourceReadAuthorityFixtureV1(7)
	grant1, granted1, err := lifecycle.Activate(authority1)
	if err != nil || granted1.State != FundsSourceReadStateReadyV1 ||
		granted1.Health != FundsSourceReadHealthHealthyV1 ||
		granted1.Decision != FundsSourceReadDecisionGrantedV1 ||
		!lifecycle.Authorizes(grant1, FundsSourceReadOperationV1, authority1) {
		t.Fatalf("exact current Host authority did not mint the typed grant: event=%#v err=%v", granted1, err)
	}
	if lifecycle.Authorizes(grant1, "analyze_account_flows", authority1) ||
		lifecycle.Authorizes(grant1, "funds.case.read", authority1) {
		t.Fatal("source-read grant widened to an unrelated capability or operation")
	}
	if _, err := json.Marshal(grant1); err == nil {
		t.Fatal("source-read grant was serializable")
	}
	stableBindingDigest := grant1.BindingDigest()
	oldGeneration := grant1.Generation()
	if _, err := lifecycle.Revoke(FundsSourceReadStateStoppedV1, FundsSourceReadReasonDisconnectedV1); err != nil {
		t.Fatal(err)
	}
	if lifecycle.Authorizes(grant1, FundsSourceReadOperationV1, authority1) {
		t.Fatal("stopped lifecycle retained its prior grant")
	}
	if _, err := lifecycle.BeginSetup(); err != nil {
		t.Fatal(err)
	}
	authority2 := fundsSourceReadAuthorityFixtureV1(8)
	grant2, recovered, err := lifecycle.Activate(authority2)
	if err != nil || !lifecycle.Authorizes(grant2, FundsSourceReadOperationV1, authority2) {
		t.Fatalf("fresh revalidation did not recover source-read authority: event=%#v err=%v", recovered, err)
	}
	if grant2.BindingDigest() != stableBindingDigest || grant2.Generation() <= oldGeneration ||
		grant2.GrantDigest() == grant1.GrantDigest() ||
		lifecycle.Authorizes(grant1, FundsSourceReadOperationV1, authority2) {
		t.Fatalf("recovery reused stale authority or changed the stable binding: old=%#v new=%#v", grant1, grant2)
	}

	rebuilt := fundsSourceReadBindingFixtureV1(t, []domainpluginpackage.CapabilityRequestV1{
		exactFundsSourceReadRequestFixtureV1(),
	})
	if rebuilt.BindingDigest() != stableBindingDigest || rebuilt.AdmissionDigest() != binding.AdmissionDigest() {
		t.Fatal("identical admitted identity/declaration/spec did not rebuild deterministic digests")
	}
}

func TestFundsSourceReadMissingOrNarrowedRequestCannotMintGrant(t *testing.T) {
	for name, requests := range map[string][]domainpluginpackage.CapabilityRequestV1{
		"missing": {{
			ID: "funds.case.read", ProtocolVersion: 1,
			ScopeConstraints: []string{FundsSourceReadScopeCaseBoundV1, FundsSourceReadScopeSourceVerifyV1},
		}},
		"narrowed scope": {{
			ID: FundsSourceReadCapabilityIDV1, ProtocolVersion: 1,
			ScopeConstraints: []string{FundsSourceReadScopeCaseBoundV1},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			binding := fundsSourceReadBindingFixtureV1(t, requests)
			if binding.ExactRequest() {
				t.Fatal("non-exact request became eligible")
			}
			lifecycle, event, err := NewFundsSourceReadLifecycleV1(binding)
			if err != nil || event.State != FundsSourceReadStateDisabledV1 ||
				event.Decision != FundsSourceReadDecisionDeniedV1 {
				t.Fatalf("non-exact request did not fail closed: event=%#v err=%v", event, err)
			}
			if _, _, err := lifecycle.Activate(fundsSourceReadAuthorityFixtureV1(1)); !errors.Is(err, ErrFundsSourceReadLifecycleInvalidV1) {
				t.Fatalf("non-exact request minted authority: %v", err)
			}
		})
	}
}

func TestFundsSourceReadWrongProtocolAndPackageLifecycleAreDeniedByStaticAdmission(t *testing.T) {
	for name, mutate := range map[string]func(*domainpluginpackage.DeclarationV1){
		"wrong protocol": func(declaration *domainpluginpackage.DeclarationV1) {
			declaration.RequestedCapabilities[0].ProtocolVersion = 2
		},
		"package lifecycle": func(declaration *domainpluginpackage.DeclarationV1) {
			declaration.Lifecycle.EntryPolicy = "package-authored-ready"
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := fundsSourceReadAdmissionInputFixtureV1(t, []domainpluginpackage.CapabilityRequestV1{
				exactFundsSourceReadRequestFixtureV1(),
			}, mutate)
			if _, err := BindFundsSourceReadV1(input, strings.Repeat("d", 64)); !errors.Is(err, ErrFundsSourceReadBindingInvalidV1) {
				t.Fatalf("invalid package-authored authority reached a binding: %v", err)
			}
		})
	}
	if lifecycle, _, err := NewFundsSourceReadLifecycleV1(FundsSourceReadBindingV1{}); lifecycle != nil ||
		!errors.Is(err, ErrFundsSourceReadBindingInvalidV1) {
		t.Fatalf("zero or caller-fabricated binding reached lifecycle authority: lifecycle=%#v err=%v", lifecycle, err)
	}
}

func TestFundsSourceReadDecisionEventIsTypedAndValueFree(t *testing.T) {
	binding := fundsSourceReadBindingFixtureV1(t, []domainpluginpackage.CapabilityRequestV1{
		exactFundsSourceReadRequestFixtureV1(),
	})
	lifecycle, _, err := NewFundsSourceReadLifecycleV1(binding)
	if err != nil {
		t.Fatal(err)
	}
	_, event, err := lifecycle.Activate(fundsSourceReadAuthorityFixtureV1(3))
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"path", "credential", "password", "token", "error", "result", "expires", "receipt",
		"/users/", "case-id", "account", "card",
	} {
		if strings.Contains(strings.ToLower(string(body)), forbidden) {
			t.Fatalf("decision event exposed forbidden value-bearing field %q: %s", forbidden, body)
		}
	}
	if event.Schema != FundsSourceReadDecisionSchemaV1 || event.PackageID != FundsSourceReadPackageIDV1 ||
		event.CapabilityID != FundsSourceReadCapabilityIDV1 || event.Operation != FundsSourceReadOperationV1 ||
		len(event.ScopeConstraints) != 2 || event.BindingDigest == "" || event.GrantDigest == "" {
		t.Fatalf("decision event omitted its typed value-free contract: %#v", event)
	}
}

func fundsSourceReadBindingFixtureV1(
	t *testing.T,
	requests []domainpluginpackage.CapabilityRequestV1,
) FundsSourceReadBindingV1 {
	t.Helper()
	binding, err := BindFundsSourceReadV1(
		fundsSourceReadAdmissionInputFixtureV1(t, requests, nil),
		strings.Repeat("d", 64),
	)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func fundsSourceReadAdmissionInputFixtureV1(
	t *testing.T,
	requests []domainpluginpackage.CapabilityRequestV1,
	mutate func(*domainpluginpackage.DeclarationV1),
) domainpluginpackage.StaticAdmissionInputV1 {
	t.Helper()
	declaration := domainpluginpackage.DeclarationV1{
		SchemaVersion: 1, PackageID: FundsSourceReadPackageIDV1, PackageVersion: "0.16.16",
		Contributions: domainpluginpackage.ContributionsV1{
			Skills:     []domainpluginpackage.PathContributionV1{},
			MCPServers: []domainpluginpackage.MCPServerContributionV1{{ID: "analytix_funds", Entrypoint: "mcp/server.mjs"}},
			Hooks:      []domainpluginpackage.PathContributionV1{}, Assets: []domainpluginpackage.PathContributionV1{},
			PublicUI: []domainpluginpackage.PathContributionV1{},
		},
		RequestedCapabilities: requests,
		Lifecycle:             domainpluginpackage.LifecycleV1{ProtocolVersion: 1, EntryPolicy: domainpluginpackage.StaticFirstPartyEntryPolicyV1},
	}
	if mutate != nil {
		mutate(&declaration)
	}
	canonical, err := domainpluginpackage.CanonicalDeclarationV1Bytes(declaration)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(canonical)
	digestHex := hex.EncodeToString(digest[:])
	return domainpluginpackage.StaticAdmissionInputV1{
		CanonicalDeclaration: canonical, DeclarationRawSHA256: digestHex,
		DeclarationCanonicalSHA256: digestHex,
		Evidence: domainpluginpackage.StaticAdmissionEvidenceV1{
			ArtifactIntegrityVerified: true,
			PackageAuthoritySHA256:    strings.Repeat("a", 64),
			ProvenanceAuthorityDigest: strings.Repeat("b", 64),
			ProvenanceClassification:  "controlled_release_clean_candidate_non_publishable",
			ProvenanceDispositionKind: "controlled_release_receipt",
			PlatformAnchor:            "macos_developer_id_resource_seal",
			SigningAlgorithm:          domainpluginpackage.StaticAdmissionSigningAlgorithmV1,
		},
	}
}

func exactFundsSourceReadRequestFixtureV1() domainpluginpackage.CapabilityRequestV1 {
	return domainpluginpackage.CapabilityRequestV1{
		ID: FundsSourceReadCapabilityIDV1, ProtocolVersion: FundsSourceReadProtocolVersionV1,
		ScopeConstraints: []string{FundsSourceReadScopeCaseBoundV1, FundsSourceReadScopeSourceVerifyV1},
	}
}

func fundsSourceReadAuthorityFixtureV1(epoch uint64) FundsSourceReadHostAuthorityV1 {
	return FundsSourceReadHostAuthorityV1{
		SpecFingerprint:      strings.Repeat("d", 64),
		ServerIdentityDigest: strings.Repeat("e", 64),
		CatalogDigest:        strings.Repeat("f", 64),
		ConnectionEpoch:      epoch,
	}
}
