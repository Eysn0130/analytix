//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestRuntimeOptionalPIIRecordAuthenticationFailurePreservesOrdinaryWork(t *testing.T) {
	for _, fault := range []string{"signature", "business digest", "installation", "claim ledger"} {
		t.Run(fault, func(t *testing.T) {
			_, enrolled := runtimeWitnessedRegistryConfigV2(t)
			runRuntimePlanThenProtectedOrdinaryRestartV1(t, func(t *testing.T, config Config) {
				keyPath := filepath.Join(config.DataDir, "private", "authority", "final-answer-ed25519-v1.json")
				if fault == "installation" {
					keyPath = filepath.Join(t.TempDir(), "other-installation.json")
				}
				authority, err := finalauthority.OpenOrCreateFileAuthority(keyPath, fault != "installation")
				if err != nil {
					t.Fatal(err)
				}
				grant := runtimeOptionalPIIGrantFixture(t, authority)
				if fault == "signature" {
					signature, err := base64.RawURLEncoding.DecodeString(grant.AuthoritySignature)
					if err != nil {
						t.Fatal(err)
					}
					signature[0] ^= 1
					grant.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
				} else if fault == "business digest" {
					grant.RecordDigest = domainsecurity.SHA256Hex([]byte("wrong business digest"))
				}
				body, err := json.Marshal(grant)
				if err != nil {
					t.Fatal(err)
				}
				access, err := privatecastest.NewAccessAuthority(filepath.Join(config.DataDir, "private"))
				if err != nil {
					t.Fatal(err)
				}
				owner := filepath.Join(config.DataDir, "private", "pii-authorization")
				cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(owner, "grants"), 2<<20, access)
				if err != nil {
					t.Fatal(err)
				}
				writeErr := cas.PutIfAbsent(context.Background(), grant.RecordDigest, body)
				if err := errors.Join(writeErr, cas.Close()); err != nil {
					t.Fatal(err)
				}
				before := startupWholeTreeDigest(t, owner)
				t.Cleanup(func() {
					if startupWholeTreeDigest(t, owner) != before {
						t.Error("untrusted optional grant changed during ordinary work or restart")
					}
				})
			}, func(config *Config) {
				config.DataDir = enrolled.DataDir
				config.AuthorityAnchorV1 = enrolled.AuthorityAnchorV1
				config.AuthorityManifestRoot = enrolled.AuthorityManifestRoot
				config.AuthorityCredentialProfileRoot = enrolled.AuthorityCredentialProfileRoot
				config.AuthorityCredentialBundleRoot = enrolled.AuthorityCredentialBundleRoot
			})
		})
	}
}

func runtimeOptionalPIIGrantFixture(t *testing.T, authority *finalauthority.FileAuthority) domainpii.PIIProjectionGrantV1 {
	t.Helper()
	grant, _ := runtimeOptionalPIIGrantLedgerFixture(t, authority)
	return grant
}

func runtimeOptionalPIIGrantLedgerFixture(t *testing.T, authority *finalauthority.FileAuthority) (domainpii.PIIProjectionGrantV1, domainpublication.ClaimLedgerV1) {
	t.Helper()
	now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-optional-pii", TurnID: "turn-optional-pii", WorkspaceRealPath: "/synthetic/optional-pii",
		CaseID: "case-optional-pii", CaseBindingHash: domainsecurity.SHA256Hex([]byte("optional-pii-binding")), ContextEpoch: 2, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	evidenceID := "evr_" + domainsecurity.SHA256Hex([]byte("optional-pii-evidence"))
	const account = "R129-SYNTHETIC-ACCOUNT"
	payload := domainevidence.NormalizedClaimPayload{SubjectID: "entity-optional-pii", AccountID: account}
	proposal := domainevidence.ClaimProposal{
		SchemaVersion: domainevidence.ClaimProposalVersion, ProposalID: "proposal-optional-pii", ClaimType: domainevidence.ClaimAccount,
		NormalizedPayload: payload, EvidenceIDs: []string{evidenceID}, CounterEvidenceIDs: []string{},
	}
	claim, err := domainevidence.NewClaimRecord(domainevidence.ClaimRecordInput{
		ClaimID: "claim-optional-pii", Proposal: proposal, SupportState: domainevidence.ClaimVerified,
		EvidenceIDs: []string{evidenceID}, CounterEvidenceIDs: []string{},
		SupportedScope: &domainevidence.EvidenceQueryRange{
			EntityIDs: []string{"entity-optional-pii"}, AccountIDs: []string{account}, Directions: []string{}, SourceIDs: []string{"synthetic-flow"},
			FiltersHash: domainsecurity.SHA256Hex([]byte("optional-pii-scope")),
		},
		AllowedWording: []string{"exact account is verified"}, ProhibitedUpgrades: []string{"do not infer ownership"},
		VerifierReceiptID:  domainevidence.VerifierReceiptDigest("claim-optional-pii", domainevidence.ClaimAccount, payload, []string{evidenceID}, []string{}, domainevidence.ClaimVerified),
		VerificationReason: "synthetic exact account match", VerifiedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := domainpublication.NewClaimLedgerV1(domainpublication.ClaimLedgerInputV1{
		Context: securityContext, EvidenceReceiptIDs: []string{evidenceID}, Claims: []domainevidence.ClaimRecord{claim}, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := domainpii.NewPIIProjectionGrantV1(domainpii.GrantInputV1{
		SecurityContext: securityContext, RequesterUserID: securityContext.UserID, DisclosurePurpose: domainpii.DisclosurePurposeCaseReportV1,
		ClaimLedgerDigest: ledger.LedgerDigest,
		FieldBindings: []domainpii.FieldBindingV1{{
			PIIClass: domainpii.PIIClassFinancialAccountV1, ClaimID: claim.ClaimID, ClaimRecordDigest: claim.RecordDigest,
			ClaimType: domainevidence.ClaimAccount, FieldName: "accountId", ValueSHA256: domainsecurity.SHA256Hex([]byte(account)),
			EvidenceReceiptIDs: []string{evidenceID},
		}},
		ProjectionRulesetHash: domainsecurity.SHA256Hex([]byte("optional-pii-rules")), ProjectedContentSHA256: domainsecurity.SHA256Hex([]byte("optional-pii-report")),
		PreservedControlledFieldCount: 1, TargetIdentityDigest: domainsecurity.SHA256Hex([]byte("optional-pii-target")),
		AllowedAccessActions: []string{domainpii.ControlledArtifactAccessActionDisplayV1}, AccessPolicyDigest: domainsecurity.SHA256Hex([]byte("optional-pii-access")),
		RetentionPolicyDigest: domainsecurity.SHA256Hex([]byte("optional-pii-retention")), RetentionUntil: now.Add(24 * time.Hour),
		ApprovalID: "appr_optional_pii_12345678", ApprovalRecordDigest: domainsecurity.SHA256Hex([]byte("optional-pii-approval")),
		IssuedAt: now.Add(time.Minute), ExpiresAt: now.Add(10 * time.Minute), AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(body []byte) ([]byte, error) { return authority.Sign(context.Background(), body) })
	if err != nil {
		t.Fatal(err)
	}
	return grant, ledger
}

func TestRuntimeOptionalDomainFaultDoesNotMaskInstallationKeyFault(t *testing.T) {
	for _, fault := range []string{"corrupt key", "replaced key"} {
		t.Run(fault, func(t *testing.T) {
			fixture, config := runtimeWitnessedRegistryConfigV2(t)
			access, err := privatecastest.NewAccessAuthority(filepath.Join(config.DataDir, "private"))
			if err != nil {
				t.Fatal(err)
			}
			cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(config.DataDir, "private", "pii-authorization", "grants"), 2<<20, access)
			if err != nil {
				t.Fatal(err)
			}
			writeErr := cas.PutIfAbsent(context.Background(), domainsecurity.SHA256Hex([]byte("invalid optional grant")), []byte(`{}`))
			if err := errors.Join(writeErr, cas.Close()); err != nil {
				t.Fatal(err)
			}
			keyBody := []byte(`{}`)
			if fault == "replaced key" {
				path := filepath.Join(t.TempDir(), "foreign-key.json")
				if _, err := finalauthority.OpenOrCreateFileAuthority(path, false); err != nil {
					t.Fatal(err)
				}
				keyBody, err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(fixture.AuthorityPath, keyBody, 0o600); err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeDigest(t, config.DataDir, config.ProductionDurableRoot)
			handler, err := NewRuntimeServerHandlerE(config)
			if err == nil {
				shutdownOwnedRuntimeHandler(t, handler)
				t.Fatal("optional domain failure masked shared installation key fault")
			}
			if startupWholeTreeDigest(t, config.DataDir, config.ProductionDurableRoot) != before {
				t.Fatal("shared installation key rejection changed persistence")
			}
		})
	}
}

func TestRuntimeOptionalWitnessOutagePreservesOrdinaryWork(t *testing.T) {
	fixture, enrolled := runtimeWitnessedRegistryConfigV2(t)
	if !fixture.SetWitnessAvailable(domainenrollment.SharedEvidenceNamespaceV1, false) {
		t.Fatal("synthetic witness outage setup failed")
	}
	before := fixture.TotalAttempts()
	runRuntimePlanThenProtectedOrdinaryRestartV1(t, nil, func(config *Config) {
		config.DataDir = enrolled.DataDir
		config.AuthorityAnchorV1 = enrolled.AuthorityAnchorV1
		config.AuthorityManifestRoot = enrolled.AuthorityManifestRoot
		config.AuthorityCredentialProfileRoot = enrolled.AuthorityCredentialProfileRoot
		config.AuthorityCredentialBundleRoot = enrolled.AuthorityCredentialBundleRoot
	})
	if fixture.TotalAttempts() != before {
		t.Fatal("ordinary lifecycle or unavailable source request reached witness authority")
	}
}

func TestUnavailablePublicationWithoutEnrollmentDoesNotMaskForeignRecords(t *testing.T) {
	for _, name := range []string{"sibling owner", "same owner", "unverifiable replaced key"} {
		t.Run(name, func(t *testing.T) {
			sameOwner := name != "sibling owner"
			config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: t.TempDir(), DurableTempDir: t.TempDir()}
			initial, err := NewRuntimeServerHandlerE(config)
			if err != nil {
				t.Fatal(err)
			}
			shutdownOwnedRuntimeHandler(t, initial)
			foreignPath := filepath.Join(t.TempDir(), "foreign.json")
			foreign, err := finalauthority.OpenOrCreateFileAuthority(foreignPath, false)
			if err != nil {
				t.Fatal(err)
			}
			grant := runtimeOptionalPIIGrantFixture(t, foreign)
			body, err := json.Marshal(grant)
			if err != nil {
				t.Fatal(err)
			}
			access, err := privatecastest.NewAccessAuthority(filepath.Join(config.DataDir, "private"))
			if err != nil {
				t.Fatal(err)
			}
			put := func(root, address string, maxBytes int, body []byte) {
				t.Helper()
				cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(root, maxBytes, access)
				if err != nil {
					t.Fatal(err)
				}
				if err := errors.Join(cas.PutIfAbsent(context.Background(), address, body), cas.Close()); err != nil {
					t.Fatal(err)
				}
			}
			grantRoot := filepath.Join(config.DataDir, "private", "pii-authorization", "grants")
			if name != "unverifiable replaced key" {
				put(grantRoot, grant.RecordDigest, 2<<20, body)
			} else {
				keyBody, err := os.ReadFile(foreignPath)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(config.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), keyBody, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			badRoot, maxBytes := filepath.Join(config.DataDir, "private", "report-publication", "attempts"), 512<<10
			if sameOwner {
				badRoot, maxBytes = grantRoot, 2<<20
			}
			put(badRoot, strings.Repeat("0", 63)+"1", maxBytes, []byte(`{}`))
			before := startupWholeTreeDigest(t, config.DataDir, config.DurableTempDir)
			handler, err := NewRuntimeServerHandlerE(config)
			if err == nil {
				shutdownOwnedRuntimeHandler(t, handler)
				t.Fatal("unavailable publication masked foreign installation record without independent enrollment")
			}
			if !errors.Is(err, errRuntimeOptionalDomainInstallationRequired) {
				t.Fatalf("unanchored mixed identity did not reach global boundary: %v", err)
			}
			if startupWholeTreeDigest(t, config.DataDir, config.DurableTempDir) != before {
				t.Fatal("unanchored identity rejection changed persistence")
			}
		})
	}
}

func TestRuntimeHealthyPIIGrantLedgerPreservesOrdinaryWork(t *testing.T) {
	_, enrolled := runtimeWitnessedRegistryConfigV2(t)
	runRuntimePlanThenProtectedOrdinaryRestartV1(t, func(t *testing.T, config Config) {
		initial, err := NewRuntimeServerHandlerE(config)
		if err != nil {
			t.Fatal(err)
		}
		shutdownOwnedRuntimeHandler(t, initial)
		authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(config.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
		if err != nil {
			t.Fatal(err)
		}
		grant, ledger := runtimeOptionalPIIGrantLedgerFixture(t, authority)
		access, err := privatecastest.NewAccessAuthority(filepath.Join(config.DataDir, "private"))
		if err != nil {
			t.Fatal(err)
		}
		grantBody, err := domainpii.PIIProjectionGrantV1Bytes(grant)
		if err != nil {
			t.Fatal(err)
		}
		ledgerBody, err := json.Marshal(ledger)
		if err != nil {
			t.Fatal(err)
		}
		var roots []string
		for _, record := range []struct {
			owner, leaf, digest string
			maxBytes            int
			body                []byte
		}{
			{"pii-authorization", "grants", grant.RecordDigest, 2 << 20, grantBody},
			{"report-publication", "claim-ledgers", ledger.LedgerDigest, 8 << 20, ledgerBody},
		} {
			root := filepath.Join(config.DataDir, "private", record.owner, record.leaf)
			cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(root, record.maxBytes, access)
			if err != nil {
				t.Fatal(err)
			}
			writeErr := cas.PutIfAbsent(context.Background(), record.digest, record.body)
			if err := errors.Join(writeErr, cas.Close()); err != nil {
				t.Fatal(err)
			}
			roots = append(roots, root)
		}
		capabilities, err := prepareRuntimeOptionalDomainCapabilities(context.Background(), config.DataDir, access, nil)
		if err != nil {
			t.Fatal(err)
		}
		for name, capability := range capabilities {
			if runtimePublicationDomainOwner(name) && !capability.available {
				t.Fatalf("healthy publication owner was incorrectly made unavailable: %s", name)
			}
		}
		if !capabilities["pii-authorization"].hasRecords || !capabilities["report-publication"].hasRecords {
			t.Fatal("healthy grant or ledger inventory was discarded")
		}
		before := startupWholeTreeDigest(t, roots...)
		t.Cleanup(func() {
			if startupWholeTreeDigest(t, roots...) != before {
				t.Error("healthy historical grant or ledger changed during ordinary work or restart")
			}
		})
	}, func(config *Config) {
		config.DataDir = enrolled.DataDir
		config.AuthorityAnchorV1 = enrolled.AuthorityAnchorV1
		config.AuthorityManifestRoot = enrolled.AuthorityManifestRoot
		config.AuthorityCredentialProfileRoot = enrolled.AuthorityCredentialProfileRoot
		config.AuthorityCredentialBundleRoot = enrolled.AuthorityCredentialBundleRoot
	})
}
