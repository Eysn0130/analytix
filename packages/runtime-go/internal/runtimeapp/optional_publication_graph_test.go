//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestRuntimeOptionalAccessReferenceFailurePreservesOrdinaryWork(t *testing.T) {
	for _, owner := range []string{"controlled-artifact-access", "controlled-artifact-access-v2"} {
		t.Run(owner, func(t *testing.T) {
			for _, fault := range []string{"missing grant", "missing publication"} {
				t.Run(fault, func(t *testing.T) {
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
						now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
						securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
							ThreadID: "thread-optional-pii", TurnID: "turn-optional-pii", WorkspaceRealPath: "/synthetic/optional-pii",
							CaseID: "case-optional-pii", CaseBindingHash: domainsecurity.SHA256Hex([]byte("optional-pii-binding")), ContextEpoch: 2, IssuedAt: now,
						})
						if err != nil {
							t.Fatal(err)
						}
						hash := func(label string) string { return domainsecurity.SHA256Hex([]byte("R129-synthetic-" + label)) }
						input := domainpii.ControlledArtifactAccessReceiptInputV1{
							SecurityContext: securityContext, AccessAction: domainpii.ControlledArtifactAccessActionDisplayV1,
							ControlledHandleDigest: hash("handle"), UseSlotDigest: hash("slot"), RendererPrincipalDigest: hash("renderer"),
							RendererGeneration: 1, BackendGeneration: 1, AccessPolicyDigest: grant.AccessPolicyDigest, RetentionPolicyDigest: grant.RetentionPolicyDigest,
							PublicationCommitDigest: hash("commit"), PublicationReceiptDigest: hash("receipt"), PIIProjectionDigest: hash("projection"),
							PIIAuthorizationDigest: grant.RecordDigest, ClaimLedgerDigest: grant.ClaimLedgerDigest, TargetIdentityDigest: grant.TargetIdentityDigest,
							ArtifactSHA256: grant.ProjectedContentSHA256, ArtifactByteLength: 128, MediaType: domainpii.ControlledPIIArtifactMediaTypeV1,
							RequestedAt: now.Add(2 * time.Minute), AuthorizedUntil: now.Add(5 * time.Minute), AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
						}
						sign := func(body []byte) ([]byte, error) { return authority.Sign(context.Background(), body) }
						var address string
						var body []byte
						if owner == "controlled-artifact-access" {
							receipt, err := domainpii.NewControlledArtifactAccessReceiptV1(input, sign)
							if err != nil || domainpii.ValidateControlledArtifactAccessReceiptForGrantV1(receipt, grant) != nil {
								t.Fatal("invalid synthetic access/grant fixture")
							}
							address = receipt.AccessID
							body, err = domainpii.ControlledArtifactAccessReceiptV1Bytes(receipt)
							if err != nil {
								t.Fatal(err)
							}
						} else {
							receipt, err := domainpii.NewControlledArtifactAccessReceiptV2(domainpii.ControlledArtifactAccessReceiptInputV2{
								SecurityContext: input.SecurityContext, AccessAction: input.AccessAction, ControlledHandleDigest: input.ControlledHandleDigest,
								UseSlotDigest: input.UseSlotDigest, RendererPrincipalDigest: input.RendererPrincipalDigest, RendererGeneration: input.RendererGeneration, BackendGeneration: input.BackendGeneration,
								AccessPolicyDigest: input.AccessPolicyDigest, RetentionPolicyDigest: input.RetentionPolicyDigest,
								DeliveryID: hash("delivery"), DeliveryOutcomeRecordDigest: hash("outcome"), ReleaseTargetIdentityDigest: hash("release-target"),
								PublicationCommitDigest: input.PublicationCommitDigest, PublicationReceiptDigest: input.PublicationReceiptDigest,
								PIIProjectionDigest: input.PIIProjectionDigest, PIIAuthorizationDigest: input.PIIAuthorizationDigest, ClaimLedgerDigest: input.ClaimLedgerDigest,
								TargetIdentityDigest: input.TargetIdentityDigest, ArtifactSHA256: input.ArtifactSHA256, ArtifactByteLength: input.ArtifactByteLength, MediaType: input.MediaType,
								RequestedAt: input.RequestedAt, AuthorizedUntil: input.AuthorizedUntil, AuthorityKeyID: input.AuthorityKeyID, AuthorityPublicKey: input.AuthorityPublicKey,
							}, sign)
							if err != nil || domainpii.ValidateControlledArtifactAccessReceiptForGrantV2(receipt, grant) != nil {
								t.Fatal("invalid synthetic access V2/grant fixture")
							}
							address = receipt.AccessID
							body, err = domainpii.ControlledArtifactAccessReceiptV2Bytes(receipt)
							if err != nil {
								t.Fatal(err)
							}
						}
						access, err := privatecastest.NewAccessAuthority(filepath.Join(config.DataDir, "private"))
						if err != nil {
							t.Fatal(err)
						}
						if fault == "missing publication" {
							grantBody, err := domainpii.PIIProjectionGrantV1Bytes(grant)
							if err != nil {
								t.Fatal(err)
							}
							ledgerBody, err := json.Marshal(ledger)
							if err != nil {
								t.Fatal(err)
							}
							for _, record := range []struct {
								root, digest string
								maxBytes     int
								body         []byte
							}{
								{"pii-authorization/grants", grant.RecordDigest, 2 << 20, grantBody},
								{"report-publication/claim-ledgers", ledger.LedgerDigest, 8 << 20, ledgerBody},
							} {
								cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(config.DataDir, "private", record.root), record.maxBytes, access)
								if err != nil {
									t.Fatal(err)
								}
								writeErr := cas.PutIfAbsent(context.Background(), record.digest, record.body)
								if err := errors.Join(writeErr, cas.Close()); err != nil {
									t.Fatal(err)
								}
							}
						}
						cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(config.DataDir, "private", owner, "access-receipts"), 256<<10, access)
						if err != nil {
							t.Fatal(err)
						}
						writeErr := cas.PutIfAbsent(context.Background(), address, body)
						if err := errors.Join(writeErr, cas.Close()); err != nil {
							t.Fatal(err)
						}
						var roots []string
						for _, name := range []string{"pii-authorization", "report-publication", "controlled-artifact-access", "controlled-artifact-access-v2"} {
							roots = append(roots, filepath.Join(config.DataDir, "private", name))
						}
						before := startupWholeTreeDigest(t, roots...)
						t.Cleanup(func() {
							if startupWholeTreeDigest(t, roots...) != before {
								t.Error("unavailable publication dependency closure changed during ordinary work or restart")
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
		})
	}
}
