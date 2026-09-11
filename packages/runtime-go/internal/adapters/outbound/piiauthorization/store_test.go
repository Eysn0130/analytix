package piiauthorization

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestStorePersistsCanonicalPIIGrantInProtectedCAS(t *testing.T) {
	root := t.TempDir()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(filepath.Join(root, "pii-grants"), access)
	if err != nil {
		t.Fatal(err)
	}
	grant := storeTestGrant(t)
	for range 2 {
		if err := store.PutGrantIfAbsent(context.Background(), grant); err != nil {
			t.Fatal(err)
		}
	}
	stored, err := store.ResolveGrant(context.Background(), grant.RecordDigest)
	if err != nil || stored.RecordDigest != grant.RecordDigest || stored.GrantID != grant.GrantID {
		t.Fatalf("protected PII grant readback failed: stored=%#v err=%v", stored, err)
	}
	body, _ := domainpii.PIIProjectionGrantV1Bytes(stored)
	if bytes.Contains(body, []byte("6222020202020202020")) {
		t.Fatalf("protected PII grant contains a raw account: %s", body)
	}
	if _, err := store.ResolveGrant(context.Background(), domainsecurity.SHA256Hex([]byte("missing"))); !errors.Is(err, piiauthorizationport.ErrNotFound) {
		t.Fatalf("missing audit digest did not fail closed: %v", err)
	}
	visited := 0
	if err := store.VisitGrants(context.Background(), func(candidate domainpii.PIIProjectionGrantV1) error {
		visited++
		if candidate.RecordDigest != grant.RecordDigest {
			t.Fatalf("inventory returned another grant: %#v", candidate)
		}
		return nil
	}); err != nil || visited != 1 {
		t.Fatalf("protected PII grant inventory failed: visited=%d err=%v", visited, err)
	}
}

func TestPIIAuthorizationStoreCloseIsIdempotent(t *testing.T) {
	root := t.TempDir()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(filepath.Join(root, "pii-grants"), access)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errorsOut := make(chan error, 16)
	var wait sync.WaitGroup
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			errorsOut <- store.Close()
		}()
	}
	close(start)
	wait.Wait()
	close(errorsOut)
	for err := range errorsOut {
		if err != nil {
			t.Fatalf("PII authorization store close was not concurrent and idempotent: %v", err)
		}
	}
	if _, err := store.HasRecords(context.Background()); err == nil {
		t.Fatal("closed PII authorization store retained live private-CAS authority")
	}
}

func TestPIIAuthorizationPreparedRecoveryValidatesCanonicalGrantInventory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pii-authorization")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(filepath.Join(root, "grants"), access)
	if err != nil {
		t.Fatal(err)
	}
	grant := storeTestGrant(t)
	if err := store.PutGrantIfAbsent(context.Background(), grant); err != nil {
		t.Fatal(err)
	}
	if has, err := store.HasRecords(context.Background()); err != nil || !has {
		t.Fatalf("PII authorization inventory was not detected: has=%v err=%v", has, err)
	}
	prepared, err := PrepareRecoveryV1(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Revalidate(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPIIAuthorizationPreparedRecoveryRejectsContentAddressGarbage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pii-authorization")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	cas, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(root, "grants"), maxPIIProjectionGrantBytesV1, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := cas.PutIfAbsent(context.Background(), domainsecurity.SHA256Hex([]byte("garbage-grant")), []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareRecoveryV1(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(context.Background()); err == nil {
		t.Fatal("PII authorization recovery accepted non-contract bytes")
	}
}

func storeTestGrant(t *testing.T) domainpii.PIIProjectionGrantV1 {
	t.Helper()
	now := time.Date(2026, 7, 16, 11, 0, 0, 0, time.UTC)
	securityContext, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-pii-store", TurnID: "turn-pii-store", WorkspaceRealPath: "/workspace/pii-store",
		CaseID: "case-pii-store", CaseBindingHash: domainsecurity.SHA256Hex([]byte("pii-store-binding")), ContextEpoch: 2, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x45}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	grant, err := domainpii.NewPIIProjectionGrantV1(domainpii.GrantInputV1{
		SecurityContext: securityContext, RequesterUserID: securityContext.UserID, DisclosurePurpose: domainpii.DisclosurePurposeCaseReportV1,
		ClaimLedgerDigest: domainsecurity.SHA256Hex([]byte("pii-store-ledger")),
		FieldBindings: []domainpii.FieldBindingV1{{
			PIIClass: domainpii.PIIClassFinancialAccountV1, ClaimID: "claim-account", ClaimRecordDigest: domainsecurity.SHA256Hex([]byte("pii-store-claim")),
			ClaimType: domainevidence.ClaimAccount, FieldName: "accountId", ValueSHA256: domainsecurity.SHA256Hex([]byte("6222020202020202020")),
			EvidenceReceiptIDs: []string{"evr_" + domainsecurity.SHA256Hex([]byte("pii-store-evidence"))},
		}},
		ProjectionRulesetHash:  domainsecurity.SHA256Hex([]byte("pii-store-rules")),
		ProjectedContentSHA256: domainsecurity.SHA256Hex([]byte("pii-store-report")), PreservedControlledFieldCount: 1,
		TargetIdentityDigest:  domainsecurity.SHA256Hex([]byte("pii-store-target")),
		AllowedAccessActions:  []string{domainpii.ControlledArtifactAccessActionDisplayV1, domainpii.ControlledArtifactAccessActionExportV1},
		AccessPolicyDigest:    domainsecurity.SHA256Hex([]byte("pii-store-access-policy")),
		RetentionPolicyDigest: domainsecurity.SHA256Hex([]byte("pii-store-retention-policy")),
		RetentionUntil:        now.Add(24 * time.Hour),
		ApprovalID:            "appr_pii_store_12345678", ApprovalRecordDigest: domainsecurity.SHA256Hex([]byte("pii-store-approval")),
		IssuedAt: now.Add(time.Minute), ExpiresAt: now.Add(10 * time.Minute), AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return grant
}
