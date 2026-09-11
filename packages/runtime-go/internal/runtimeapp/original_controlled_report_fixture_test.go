//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	piistore "analytix.local/runtime-go/internal/adapters/outbound/piiauthorization"
	piiapp "analytix.local/runtime-go/internal/app/piiauthorization"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

// These are synthetic private source bytes. Every downstream evidence,
// artifact, grant and V2 access digest is derived from the actual records.
// Historical fixture signatures are not a live approval or release proof.
func runtimeControlledEvidenceDraftForTestV1(t *testing.T) func(*domainevidence.EvidenceReceiptInput) ([]byte, []byte) {
	t.Helper()
	return func(draft *domainevidence.EvidenceReceiptInput) ([]byte, []byte) {
		row := map[string]any{"sourceRecordId": "row-controlled", "entityId": "entity-controlled", "accountId": "0012-3456789012345678"}
		rowBody, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(map[string]any{"rows": []any{row}})
		if err != nil {
			t.Fatal(err)
		}
		binding, err := domainevidence.NewSourceFieldBindingV2(domainevidence.SourceFieldBindingInputV2{
			FactID: "fact-account", ClaimType: domainevidence.ClaimAccount,
			CanonicalEntityID: "entity-controlled", CanonicalAccountID: "00123456789012345678",
			SourceRecordID: "row-controlled", RawArtifactSHA256: domainsecurity.SHA256Hex(raw), SourceRecordSHA256: domainsecurity.SHA256Hex(rowBody),
			SourceRecordPath: "/rows/0", SourceRecordIDPath: "/rows/0/sourceRecordId", SourceEntityIDPath: "/rows/0/entityId", SourceFieldPath: "/rows/0/accountId",
			SourceScalarKind: domainevidence.SourceFieldBindingScalarTextV2, SourceExactValue: row["accountId"].(string),
		})
		if err != nil {
			t.Fatal(err)
		}
		bindings, err := domainevidence.CanonicalSourceFieldBindingsV2([]domainevidence.SourceFieldBindingV2{binding})
		if err != nil {
			t.Fatal(err)
		}
		digest, err := domainevidence.SourceFieldBindingSetDigestV2(bindings)
		if err != nil {
			t.Fatal(err)
		}
		material := domainevidence.CanonicalEvidenceMaterial{
			SchemaVersion: domainevidence.CanonicalEvidenceVersionV2, Purpose: domainevidence.CanonicalEvidencePurposeV2,
			Facts:               []domainevidence.CanonicalEvidenceFact{{FactID: "fact-account", ClaimType: domainevidence.ClaimAccount, NormalizedPayload: domainevidence.NormalizedClaimPayload{SubjectID: "entity-controlled", AccountID: "00123456789012345678"}}},
			SourceFieldBindings: bindings, SourceFieldBindingSetDigest: digest,
		}
		if err := domainevidence.ValidateSourceFieldBindingsAgainstRawResultV2(raw, domainsecurity.SHA256Hex(raw), material); err != nil {
			t.Fatal(err)
		}
		canonical, err := json.Marshal(material)
		if err != nil {
			t.Fatal(err)
		}
		draft.SourceType, draft.Granularity, draft.Currency = "transactions", "transaction", "CNY"
		draft.QueryRange.EntityIDs = []string{"entity-controlled"}
		draft.QueryRange.AccountIDs = []string{"00123456789012345678"}
		draft.QueryRange.Directions = []string{"out"}
		draft.QueryRange.StartAt, draft.QueryRange.EndAt = "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"
		draft.QueryRange.SourceIDs = []string{"row-controlled"}
		draft.SourceRecordIDs = []string{"row-controlled"}
		draft.PIIClassification = domainevidence.PIIControlled
		return raw, canonical
	}
}

func runtimeControlledReportArtifactForTestV1(t *testing.T, core *runtimeChildIdentityStartupV1, authority authorityport.Authority, securityContext domainsecurity.TurnSecurityContext, ledger domainpublication.ClaimLedgerV1, material domainevidence.CanonicalEvidenceMaterial, target string, now time.Time) ([]byte, domainpublication.PIIProjectionV1, *domainpii.PIIProjectionGrantV1) {
	t.Helper()
	ctx := context.Background()
	if len(ledger.Claims) != 1 || len(material.SourceFieldBindings) != 1 {
		t.Fatal("controlled fixture needs one exact source field")
	}
	claim, source := ledger.Claims[0], material.SourceFieldBindings[0]
	rules := domainsecurity.SHA256Hex([]byte("terminal report rules"))
	protected, err := domainpii.NewControlledPIIArtifactV1(domainpii.ControlledPIIArtifactInputV1{
		SecurityContext: securityContext, ClaimLedgerDigest: ledger.LedgerDigest, ProjectionRulesetHash: rules, TargetIdentityDigest: target, RenderedAt: now.Add(4 * time.Minute),
		Fields: []domainpii.ControlledPIIFieldV1{{PIIClass: domainpii.PIIClassFinancialAccountV1, ClaimID: claim.ClaimID, ClaimRecordDigest: claim.RecordDigest, ClaimType: claim.ClaimType,
			FieldName: "accountId", ExactValue: source.SourceExactValue, ValueSHA256: source.SourceExactValueSHA256, EvidenceReceiptIDs: claim.EvidenceIDs}},
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := domainpii.ControlledPIIArtifactV1Bytes(protected)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := domainpii.ControlledPIIArtifactMetadataFromBytesV1(artifact)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := domainpii.NewPIIProjectionGrantV1(domainpii.GrantInputV1{
		SecurityContext: securityContext, RequesterUserID: securityContext.UserID, DisclosurePurpose: domainpii.DisclosurePurposeCaseReportV1,
		ClaimLedgerDigest: ledger.LedgerDigest, FieldBindings: metadata.FieldBindings, ProjectionRulesetHash: rules,
		ProjectedContentSHA256: metadata.SHA256, PreservedControlledFieldCount: metadata.PreservedControlledFieldCount, TargetIdentityDigest: target,
		AllowedAccessActions: []string{domainpii.ControlledArtifactAccessActionDisplayV1},
		AccessPolicyDigest:   domainsecurity.SHA256Hex([]byte("synthetic controlled policy")), RetentionPolicyDigest: domainsecurity.SHA256Hex([]byte("synthetic retention policy")), RetentionUntil: now.Add(24 * time.Hour),
		ApprovalID: "appr_synthetic_history_1234", ApprovalRecordDigest: domainsecurity.SHA256Hex([]byte("synthetic historical approval")),
		IssuedAt: now.Add(4 * time.Minute), ExpiresAt: now.Add(15 * time.Minute), AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) })
	if err != nil {
		t.Fatal(err)
	}
	if err := piiapp.ValidateStoredGrantClaimLedgerV1(grant, ledger); err != nil {
		t.Fatal(err)
	}
	store, err := piistore.NewStore(filepath.Join(core.roots.DataDir, "private", "pii-authorization", "grants"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(store.PutGrantIfAbsent(ctx, grant), store.Close()); err != nil {
		t.Fatal(err)
	}
	projection, err := domainpublication.NewPIIProjectionV1(domainpublication.PIIProjectionInputV1{
		ProjectionClass: domainpublication.PIIProjectionControlledFull, RulesetHash: rules, ProjectedContentSHA256: metadata.SHA256,
		RestrictedFieldCount: metadata.PreservedControlledFieldCount, PreservedControlledFieldCount: metadata.PreservedControlledFieldCount, AuthorizationAuditDigest: grant.RecordDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	return artifact, projection, &grant
}

func runtimeStoreControlledAccessForTestV2(t *testing.T, core *runtimeChildIdentityStartupV1, authority authorityport.Authority, securityContext domainsecurity.TurnSecurityContext, grant domainpii.PIIProjectionGrantV1, ledger domainpublication.ClaimLedgerV1, receipt domainpublication.PublicationReceiptV1, projection domainpublication.PIIProjectionV1, artifact []byte, commit domainpublication.PublicationCommitReceiptV1, outcome domainpublication.ReportDeliveryOutcomeV1, now time.Time, open bool, slotToken string) {
	t.Helper()
	ctx := context.Background()
	metadata, err := domainpii.ControlledPIIArtifactMetadataFromBytesV1(artifact)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := domainpii.ControlledAccessHandleDigestV2("synthetic-history-handle")
	if err != nil {
		t.Fatal(err)
	}
	slot, err := domainpii.ControlledAccessUseSlotDigestV2(slotToken)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := domainpii.ControlledAccessRendererPrincipalDigestV2("synthetic-history-principal")
	if err != nil {
		t.Fatal(err)
	}
	sign := func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) }
	access, err := domainpii.NewControlledArtifactAccessReceiptV2(domainpii.ControlledArtifactAccessReceiptInputV2{
		SecurityContext: securityContext, AccessAction: domainpii.ControlledArtifactAccessActionDisplayV1,
		ControlledHandleDigest: handle, UseSlotDigest: slot, RendererPrincipalDigest: principal, RendererGeneration: 1, BackendGeneration: 1,
		AccessPolicyDigest: grant.AccessPolicyDigest, RetentionPolicyDigest: grant.RetentionPolicyDigest,
		DeliveryID: domainpublication.ReportDeliveryOutcomeID(outcome), DeliveryOutcomeRecordDigest: domainpublication.ReportDeliveryOutcomeRecordDigest(outcome),
		PublicationCommitDigest: commit.RecordDigest, PublicationReceiptDigest: receipt.RecordDigest, PIIProjectionDigest: projection.ProjectionDigest,
		PIIAuthorizationDigest: grant.RecordDigest, ClaimLedgerDigest: ledger.LedgerDigest, TargetIdentityDigest: receipt.TargetIdentityDigest,
		ReleaseTargetIdentityDigest: domainsecurity.SHA256Hex([]byte("synthetic display target")),
		ArtifactSHA256:              metadata.SHA256, ArtifactByteLength: metadata.ByteLength, MediaType: metadata.MediaType,
		RequestedAt: now.Add(9 * time.Minute), AuthorizedUntil: now.Add(10 * time.Minute), AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	if err := domainpii.ValidateControlledArtifactAccessReceiptForGrantV2(access, grant); err != nil {
		t.Fatal(err)
	}
	// Closed cancellation is historical data, not an assertion of a host release.
	disposition, err := domainpii.NewControlledArtifactAccessDispositionV2(access, domainpii.ControlledArtifactAccessDispositionCancelledV1, domainpii.ControlledArtifactAccessReasonAccessCancelledV1, 0, now.Add(9*time.Minute+time.Second), authority.KeyID(), authority.PublicKey(), sign)
	if err != nil {
		t.Fatal(err)
	}
	store, err := piistore.NewAccessStoreV2(filepath.Join(core.roots.DataDir, "private", "controlled-artifact-access-v2"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReserveAccessReceiptV2(ctx, access); err != nil {
		t.Fatal(err)
	}
	if open {
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := errors.Join(store.PutAccessDispositionIfAbsentV2(ctx, disposition), store.Close()); err != nil {
		t.Fatal(err)
	}
}
