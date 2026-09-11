package pendingworkstore_test

import (
	"context"
	"crypto/ed25519"
	"path/filepath"
	"testing"
	"time"

	pendingworkstore "analytix.local/runtime-go/internal/adapters/outbound/pendingworkstore"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	pendingworkstoreport "analytix.local/runtime-go/internal/ports/pendingworkstore"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

var _ pendingworkstoreport.Store = (*pendingworkstore.Store)(nil)

func TestPendingWorkAuthorityLifecycleSurvivesProcessRestart(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private", "pending-work")
	mutation, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := pendingworkstore.NewStore(root, mutation)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_720_000_000, 0).UTC()
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-integration", TurnID: "turn-integration", WorkspaceRealPath: "/cases/integration",
		CaseID: "case-integration", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-integration")),
		DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-integration"), SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-integration")),
		ContextEpoch: 9, IssuedAt: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
		Kind: domainpendingwork.KindProviderContinuation, SecurityContext: securityContext,
		GrantRegistrySequence: 3, GrantRegistryDigest: domainsecurity.SHA256Hex([]byte("registry-integration")),
		GrantMembers: []domainpendingwork.GrantMemberV1{{
			Ordinal: 1, GrantID: domainsecurity.SHA256Hex([]byte("grant-integration")), RegistrySequence: 3,
			RegistryEntryDigest: domainsecurity.SHA256Hex([]byte("entry-integration")), ResultItemID: "item-result-integration",
			ResultItemDigest: domainsecurity.SHA256Hex([]byte("exact durable result projection")),
		}},
		PayloadHash: domainsecurity.SHA256Hex([]byte("sanitized-provider-request")),
		RouteHash:   domainsecurity.SHA256Hex([]byte("provider-route")), IssuedAt: now, ExpiresAt: now.Add(10 * time.Minute),
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutReceiptIfAbsent(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	restarted, err := pendingworkstore.NewStore(root, mutation)
	if err != nil {
		t.Fatal(err)
	}
	openReceipt, err := restarted.ReadReceipt(context.Background(), receipt.WorkID)
	if err != nil || openReceipt.ReceiptID != receipt.ReceiptID {
		t.Fatalf("restart lost pending work receipt: receipt=%#v err=%v", openReceipt, err)
	}
	disposition, err := domainpendingwork.NewPendingWorkDispositionV1(
		openReceipt, domainpendingwork.StatusRestartInvalid, "restart_provider_continuation_closed", now.Add(time.Minute),
		domainsecurity.SHA256Hex(publicKey), publicKey,
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.PutDispositionIfAbsent(context.Background(), disposition); err != nil {
		t.Fatal(err)
	}
	closedAfterRestart, err := pendingworkstore.NewStore(root, mutation)
	if err != nil {
		t.Fatal(err)
	}
	closed, err := closedAfterRestart.ReadDisposition(context.Background(), receipt.WorkID)
	if err != nil || closed.Status != domainpendingwork.StatusRestartInvalid || closed.ReceiptID != receipt.ReceiptID {
		t.Fatalf("restart lost exact pending work disposition: disposition=%#v err=%v", closed, err)
	}
}
