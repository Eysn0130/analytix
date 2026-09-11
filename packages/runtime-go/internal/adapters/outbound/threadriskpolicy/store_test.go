package threadriskpolicy

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"path/filepath"
	"testing"
	"time"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestStoreEmptyCanonicalRoundTripAndRestart(t *testing.T) {
	root := filepath.Join(t.TempDir(), "thread-risk-policy")
	store, err := newTestThreadRiskPolicyStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	if has, err := store.HasRecords(context.Background()); err != nil || has {
		t.Fatalf("empty policy store inventory mismatch: has=%t err=%v", has, err)
	}
	if policies, err := store.List(context.Background()); err != nil || len(policies) != 0 {
		t.Fatalf("empty policy list mismatch: policies=%#v err=%v", policies, err)
	}
	policy := signedStoreTestPolicy(t, 41, domainsecurity.ThreadRiskPolicyInputV1{
		ThreadID: "thread-store", WorkspaceRealPath: "/workspace/store", RiskClass: domainsecurity.RiskClassGeneral,
		Origin: domainsecurity.RiskPolicyOriginGeneralWorkspace, SignalsDigest: domainsecurity.SHA256Hex([]byte("general")),
		IssuedAt: time.Date(2026, 7, 12, 8, 0, 0, 0, time.UTC),
	})
	if err := store.PutIfAbsent(context.Background(), policy); err != nil {
		t.Fatal(err)
	}
	if err := store.PutIfAbsent(context.Background(), policy); err != nil {
		t.Fatalf("idempotent policy write failed: %v", err)
	}
	if has, err := store.HasRecords(context.Background()); err != nil || !has {
		t.Fatalf("persisted policy was absent from strict inventory: has=%t err=%v", has, err)
	}
	resolved, err := store.Resolve(context.Background(), policy.PolicyDigest)
	if err != nil || resolved != policy {
		t.Fatalf("policy resolve mismatch: resolved=%#v err=%v", resolved, err)
	}
	policies, err := store.List(context.Background())
	if err != nil || len(policies) != 1 || policies[0] != policy {
		t.Fatalf("policy list mismatch: policies=%#v err=%v", policies, err)
	}
	reopened, err := newTestThreadRiskPolicyStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := reopened.Resolve(context.Background(), policy.PolicyDigest)
	if err != nil || restarted != policy {
		t.Fatalf("restarted policy resolve mismatch: policy=%#v err=%v", restarted, err)
	}
}

func TestStoreRejectsContentAddressConflictAndNonCanonicalBytes(t *testing.T) {
	t.Run("conflict", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "thread-risk-policy")
		store, err := newTestThreadRiskPolicyStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := newTestThreadRiskPolicyRawCAS(t, root)
		if err != nil {
			t.Fatal(err)
		}
		policy := signedStoreTestPolicy(t, 42, domainsecurity.ThreadRiskPolicyInputV1{
			ThreadID: "thread-conflict", WorkspaceRealPath: "/workspace/conflict", RiskClass: domainsecurity.RiskClassCase,
			Origin: domainsecurity.RiskPolicyOriginDesktopCaseEntry, SignalsDigest: domainsecurity.SHA256Hex([]byte("case")),
			IssuedAt: time.Date(2026, 7, 12, 8, 1, 0, 0, time.UTC),
		})
		if err := raw.PutIfAbsent(context.Background(), policy.PolicyDigest, []byte(`{"forged":true}`)); err != nil {
			t.Fatal(err)
		}
		if err := store.PutIfAbsent(context.Background(), policy); err == nil {
			t.Fatal("policy store accepted conflicting bytes at the same content address")
		}
		if _, err := store.Resolve(context.Background(), policy.PolicyDigest); err == nil {
			t.Fatal("policy store resolved forged CAS bytes")
		}
	})

	t.Run("non-canonical", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "thread-risk-policy")
		store, err := newTestThreadRiskPolicyStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := newTestThreadRiskPolicyRawCAS(t, root)
		if err != nil {
			t.Fatal(err)
		}
		policy := signedStoreTestPolicy(t, 43, domainsecurity.ThreadRiskPolicyInputV1{
			ThreadID: "thread-bytes", WorkspaceRealPath: "/workspace/bytes", RiskClass: domainsecurity.RiskClassGeneral,
			Origin: domainsecurity.RiskPolicyOriginGeneralWorkspace, SignalsDigest: domainsecurity.SHA256Hex([]byte("general")),
			IssuedAt: time.Date(2026, 7, 12, 8, 2, 0, 0, time.UTC),
		})
		body, err := domainsecurity.ThreadRiskPolicyV1Bytes(policy)
		if err != nil {
			t.Fatal(err)
		}
		body = append(body, '\n')
		if err := raw.PutIfAbsent(context.Background(), policy.PolicyDigest, body); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Resolve(context.Background(), policy.PolicyDigest); err == nil {
			t.Fatal("policy store accepted non-canonical serialized bytes")
		}
	})
}

func TestStoreHonorsCancellation(t *testing.T) {
	store, err := newTestThreadRiskPolicyStore(t, filepath.Join(t.TempDir(), "thread-risk-policy"))
	if err != nil {
		t.Fatal(err)
	}
	policy := signedStoreTestPolicy(t, 44, domainsecurity.ThreadRiskPolicyInputV1{
		ThreadID: "thread-cancel", WorkspaceRealPath: "/workspace/cancel", RiskClass: domainsecurity.RiskClassGeneral,
		Origin: domainsecurity.RiskPolicyOriginGeneralWorkspace, SignalsDigest: domainsecurity.SHA256Hex([]byte("general")),
		IssuedAt: time.Date(2026, 7, 12, 8, 3, 0, 0, time.UTC),
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.PutIfAbsent(ctx, policy); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled write returned %v", err)
	}
	if _, err := store.List(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled list returned %v", err)
	}
	if has, err := store.HasRecords(context.Background()); err != nil || has {
		t.Fatalf("canceled write changed inventory: has=%t err=%v", has, err)
	}
}

func signedStoreTestPolicy(t *testing.T, seed byte, input domainsecurity.ThreadRiskPolicyInputV1) domainsecurity.ThreadRiskPolicyV1 {
	t.Helper()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	input.AuthorityKeyID = domainsecurity.SHA256Hex(publicKey)
	input.AuthorityPublicKey = publicKey
	policy, err := domainsecurity.NewThreadRiskPolicyV1(input, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func newTestThreadRiskPolicyStore(t *testing.T, root string) (*Store, error) {
	t.Helper()
	mutation, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	return NewStore(root, mutation)
}

func newTestThreadRiskPolicyRawCAS(t *testing.T, root string) (*finalauthorityadapter.SecurePrivateCAS, error) {
	t.Helper()
	mutation, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	return finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(root, maxThreadRiskPolicyBytes, mutation)
}
