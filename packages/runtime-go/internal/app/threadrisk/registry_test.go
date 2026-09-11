package threadrisk

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestRegistryEmptyResolveRaiseIdempotencyAndNoDowngrade(t *testing.T) {
	authority := newThreadRiskTestAuthority(51)
	store := newMemoryThreadRiskStore()
	registry, err := NewRegistry(context.Background(), authority, store)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := registry.Current("thread-a"); found {
		t.Fatal("empty registry reported a current policy")
	}
	at := time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC)
	generalInput := ResolveOrRaiseInput{
		ThreadID: "thread-a", WorkspaceRealPath: "/workspace/a", RequestedRisk: domainsecurity.RiskClassGeneral,
		Origin: domainsecurity.RiskPolicyOriginGeneralWorkspace, SignalsDigest: domainsecurity.SHA256Hex([]byte("general")), IssuedAt: at,
	}
	general, err := registry.ResolveOrRaise(context.Background(), generalInput)
	if err != nil || general.RiskClass != domainsecurity.RiskClassGeneral || general.PredecessorPolicyDigest != "" {
		t.Fatalf("initial general policy failed: policy=%#v err=%v", general, err)
	}
	idempotentInput := generalInput
	idempotentInput.SignalsDigest = domainsecurity.SHA256Hex([]byte("idempotent-retry"))
	idempotentInput.IssuedAt = at.Add(time.Hour)
	idempotent, err := registry.ResolveOrRaise(context.Background(), idempotentInput)
	if err != nil || idempotent != general {
		t.Fatalf("same-level resolution was not idempotent: policy=%#v err=%v", idempotent, err)
	}
	caseInput := ResolveOrRaiseInput{
		ThreadID: general.ThreadID, WorkspaceRealPath: general.WorkspaceRealPath, RequestedRisk: domainsecurity.RiskClassCase,
		Origin: domainsecurity.RiskPolicyOriginBindingMarkerPresent, SignalsDigest: domainsecurity.SHA256Hex([]byte("case-marker")),
		IssuedAt: at.Add(time.Minute),
	}
	casePolicy, err := registry.ResolveOrRaise(context.Background(), caseInput)
	if err != nil || casePolicy.RiskClass != domainsecurity.RiskClassCase || casePolicy.PredecessorPolicyDigest != general.PolicyDigest {
		t.Fatalf("general-to-case raise failed: policy=%#v err=%v", casePolicy, err)
	}
	downgrade, err := registry.ResolveOrRaise(context.Background(), generalInput)
	if err != nil || downgrade != casePolicy {
		t.Fatalf("case policy downgraded or changed: policy=%#v err=%v", downgrade, err)
	}
	if records, err := store.List(context.Background()); err != nil || len(records) != 2 {
		t.Fatalf("monotonic chain inventory mismatch: records=%#v err=%v", records, err)
	}
	changedWorkspace := caseInput
	changedWorkspace.WorkspaceRealPath = "/workspace/other"
	if _, err := registry.ResolveOrRaise(context.Background(), changedWorkspace); err == nil {
		t.Fatal("case workspace changed without an explicit signed rebind authority")
	}
}

func TestRegistrySignsGeneralWorkspaceSuccessorWithoutRiskDowngrade(t *testing.T) {
	authority := newThreadRiskTestAuthority(58)
	store := newMemoryThreadRiskStore()
	registry, err := NewRegistry(context.Background(), authority, store)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 7, 12, 9, 30, 0, 0, time.UTC)
	first, err := registry.ResolveOrRaise(context.Background(), ResolveOrRaiseInput{
		ThreadID: "thread-workspace", WorkspaceRealPath: "/workspace/first", RequestedRisk: domainsecurity.RiskClassGeneral,
		Origin: domainsecurity.RiskPolicyOriginGeneralWorkspace, SignalsDigest: domainsecurity.SHA256Hex([]byte("first")), IssuedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	next, err := registry.ResolveOrRaise(context.Background(), ResolveOrRaiseInput{
		ThreadID: first.ThreadID, WorkspaceRealPath: "/workspace/next", RequestedRisk: domainsecurity.RiskClassGeneral,
		Origin: domainsecurity.RiskPolicyOriginGeneralWorkspace, SignalsDigest: domainsecurity.SHA256Hex([]byte("workspace-rebind")), IssuedAt: at.Add(time.Second),
	})
	if err != nil || next.WorkspaceRealPath != "/workspace/next" || next.RiskClass != domainsecurity.RiskClassGeneral ||
		next.PredecessorPolicyDigest != first.PolicyDigest {
		t.Fatalf("signed general workspace successor failed: next=%#v err=%v", next, err)
	}
	if records, err := store.List(context.Background()); err != nil || len(records) != 2 {
		t.Fatalf("general workspace successor inventory mismatch: records=%#v err=%v", records, err)
	}
}

func TestRegistryResolveAndValidateContextRequireExactSignedPolicyMembership(t *testing.T) {
	authority := newThreadRiskTestAuthority(58)
	registry, err := NewRegistry(context.Background(), authority, newMemoryThreadRiskStore())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 7, 12, 9, 30, 0, 0, time.UTC)
	policy, err := registry.ResolveOrRaise(context.Background(), ResolveOrRaiseInput{
		ThreadID: "thread-context", WorkspaceRealPath: "/workspace/context", RequestedRisk: domainsecurity.RiskClassGeneral,
		Origin: domainsecurity.RiskPolicyOriginGeneralWorkspace, SignalsDigest: domainsecurity.SHA256Hex([]byte("general")), IssuedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved, found := registry.Resolve(policy.PolicyDigest)
	if !found || resolved != policy {
		t.Fatalf("exact verified policy did not resolve: policy=%#v found=%t", resolved, found)
	}
	if _, found := registry.Resolve(domainsecurity.SHA256Hex([]byte("unknown"))); found {
		t.Fatal("unknown non-empty policy digest resolved as authority")
	}
	publication := mustThreadRiskTestPublication(t, domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: policy.PolicyDigest, RiskClass: domainsecurity.RiskClassGeneral,
		Disposition: domainsecurity.PublicationDispositionGeneralOutput, CaseBindingState: domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("not-applicable")), BlockerCode: domainsecurity.PublicationBlockerNone,
	})
	valid := mustThreadRiskTestContext(t, policy.ThreadID, policy.WorkspaceRealPath, publication, at.Add(time.Second))
	if err := registry.ValidateContext(valid); err != nil {
		t.Fatalf("exact V2 context failed signed policy validation: %v", err)
	}
	legacy := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: policy.ThreadID, TurnID: "turn-legacy", WorkspaceRealPath: policy.WorkspaceRealPath, IssuedAt: at,
	})
	if err := registry.ValidateContext(legacy); err == nil {
		t.Fatal("legacy V1 context passed thread risk authority validation")
	}
	wrongThread := mustThreadRiskTestContext(t, "thread-other", policy.WorkspaceRealPath, publication, at.Add(time.Second))
	if err := registry.ValidateContext(wrongThread); err == nil {
		t.Fatal("policy from another thread validated a V2 context")
	}
	wrongWorkspace := mustThreadRiskTestContext(t, policy.ThreadID, "/workspace/other", publication, at.Add(time.Second))
	if err := registry.ValidateContext(wrongWorkspace); err == nil {
		t.Fatal("policy from another workspace validated a V2 context")
	}
	unknownPublication := mustThreadRiskTestPublication(t, domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: domainsecurity.SHA256Hex([]byte("unknown-policy")), RiskClass: domainsecurity.RiskClassGeneral,
		Disposition: domainsecurity.PublicationDispositionGeneralOutput, CaseBindingState: domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("not-applicable")), BlockerCode: domainsecurity.PublicationBlockerNone,
	})
	unknownContext := mustThreadRiskTestContext(t, policy.ThreadID, policy.WorkspaceRealPath, unknownPublication, at.Add(time.Second))
	if err := registry.ValidateContext(unknownContext); err == nil {
		t.Fatal("unknown policy digest validated a V2 context")
	}
	casePublication := mustThreadRiskTestPublication(t, domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: policy.PolicyDigest, RiskClass: domainsecurity.RiskClassCase,
		Disposition: domainsecurity.PublicationDispositionCaseEvidenceGate, CaseBindingState: domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("valid-binding")), BlockerCode: domainsecurity.PublicationBlockerNone,
	})
	caseBinding, err := securitycontexttest.WitnessedRiskBinding(
		policy.ThreadID, policy.WorkspaceRealPath, domainsecurity.RiskClassCase, casePublication.ThreadRiskPolicyDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	caseContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: policy.ThreadID, TurnID: "turn-risk-mismatch", WorkspaceRealPath: policy.WorkspaceRealPath,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, CaseID: "case-context",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-context"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: at.Add(time.Second),
		PublicationPolicy: casePublication, RiskAuthorityBinding: caseBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.ValidateContext(caseContext); err == nil {
		t.Fatal("publication risk class mismatched its signed thread policy")
	}
}

func TestRegistryStartupRejectsUntrustedForkedAndDetachedInventory(t *testing.T) {
	at := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	trusted := newThreadRiskTestAuthority(52)
	root := signThreadRiskTestPolicy(t, trusted, domainsecurity.ThreadRiskPolicyInputV1{
		ThreadID: "thread-inventory", WorkspaceRealPath: "/workspace/inventory", RiskClass: domainsecurity.RiskClassGeneral,
		Origin: domainsecurity.RiskPolicyOriginGeneralWorkspace, SignalsDigest: domainsecurity.SHA256Hex([]byte("root")), IssuedAt: at,
	})

	t.Run("installation-key-mismatch", func(t *testing.T) {
		store := memoryThreadRiskStoreWith(root)
		if _, err := NewRegistry(context.Background(), newThreadRiskTestAuthority(53), store); err == nil {
			t.Fatal("inventory signed by another installation key was accepted")
		}
	})

	t.Run("self-signed-forgery", func(t *testing.T) {
		attacker := newThreadRiskTestAuthority(54)
		forgedChild := signThreadRiskTestPolicy(t, attacker, domainsecurity.ThreadRiskPolicyInputV1{
			ThreadID: root.ThreadID, WorkspaceRealPath: root.WorkspaceRealPath, RiskClass: domainsecurity.RiskClassCase,
			Origin: domainsecurity.RiskPolicyOriginLexicalGuard, SignalsDigest: domainsecurity.SHA256Hex([]byte("forged")),
			PredecessorPolicyDigest: root.PolicyDigest, IssuedAt: at.Add(time.Second),
		})
		store := memoryThreadRiskStoreWith(root, forgedChild)
		if _, err := NewRegistry(context.Background(), trusted, store); err == nil {
			t.Fatal("internally valid self-signed forged policy was trusted")
		}
	})

	t.Run("fork", func(t *testing.T) {
		left := signThreadRiskTestPolicy(t, trusted, domainsecurity.ThreadRiskPolicyInputV1{
			ThreadID: root.ThreadID, WorkspaceRealPath: root.WorkspaceRealPath, RiskClass: domainsecurity.RiskClassCase,
			Origin: domainsecurity.RiskPolicyOriginBindingMarkerPresent, SignalsDigest: domainsecurity.SHA256Hex([]byte("left")),
			PredecessorPolicyDigest: root.PolicyDigest, IssuedAt: at.Add(time.Second),
		})
		right := signThreadRiskTestPolicy(t, trusted, domainsecurity.ThreadRiskPolicyInputV1{
			ThreadID: root.ThreadID, WorkspaceRealPath: root.WorkspaceRealPath, RiskClass: domainsecurity.RiskClassCase,
			Origin: domainsecurity.RiskPolicyOriginDesktopCaseEntry, SignalsDigest: domainsecurity.SHA256Hex([]byte("right")),
			PredecessorPolicyDigest: root.PolicyDigest, IssuedAt: at.Add(2 * time.Second),
		})
		if _, err := NewRegistry(context.Background(), trusted, memoryThreadRiskStoreWith(root, left, right)); err == nil {
			t.Fatal("forked thread risk policy inventory was accepted")
		}
	})

	t.Run("detached", func(t *testing.T) {
		detached := signThreadRiskTestPolicy(t, trusted, domainsecurity.ThreadRiskPolicyInputV1{
			ThreadID: "thread-detached", WorkspaceRealPath: "/workspace/detached", RiskClass: domainsecurity.RiskClassCase,
			Origin: domainsecurity.RiskPolicyOriginBindingMarkerPresent, SignalsDigest: domainsecurity.SHA256Hex([]byte("detached")),
			PredecessorPolicyDigest: domainsecurity.SHA256Hex([]byte("missing")), IssuedAt: at,
		})
		if _, err := NewRegistry(context.Background(), trusted, memoryThreadRiskStoreWith(detached)); err == nil {
			t.Fatal("detached thread risk policy inventory was accepted")
		}
	})
}

func TestRegistryConcurrentRaiseCreatesOneChild(t *testing.T) {
	authority := newThreadRiskTestAuthority(55)
	store := newMemoryThreadRiskStore()
	registry, err := NewRegistry(context.Background(), authority, store)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 7, 12, 11, 0, 0, 0, time.UTC)
	_, err = registry.ResolveOrRaise(context.Background(), ResolveOrRaiseInput{
		ThreadID: "thread-concurrent", WorkspaceRealPath: "/workspace/concurrent", RequestedRisk: domainsecurity.RiskClassGeneral,
		Origin: domainsecurity.RiskPolicyOriginGeneralWorkspace, SignalsDigest: domainsecurity.SHA256Hex([]byte("general")), IssuedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	input := ResolveOrRaiseInput{
		ThreadID: "thread-concurrent", WorkspaceRealPath: "/workspace/concurrent", RequestedRisk: domainsecurity.RiskClassCase,
		Origin: domainsecurity.RiskPolicyOriginBindingMarkerPresent, SignalsDigest: domainsecurity.SHA256Hex([]byte("case")),
		IssuedAt: at.Add(time.Second),
	}
	const workers = 32
	var wait sync.WaitGroup
	results := make(chan domainsecurity.ThreadRiskPolicyV1, workers)
	errorsSeen := make(chan error, workers)
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			policy, resolveErr := registry.ResolveOrRaise(context.Background(), input)
			results <- policy
			errorsSeen <- resolveErr
		}()
	}
	wait.Wait()
	close(results)
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatalf("concurrent raise failed: %v", err)
		}
	}
	var expected string
	for policy := range results {
		if policy.RiskClass != domainsecurity.RiskClassCase {
			t.Fatalf("concurrent raise returned non-case policy: %#v", policy)
		}
		if expected == "" {
			expected = policy.PolicyDigest
		} else if policy.PolicyDigest != expected {
			t.Fatalf("concurrent raise produced multiple heads: got %s want %s", policy.PolicyDigest, expected)
		}
	}
	if records, err := store.List(context.Background()); err != nil || len(records) != 2 {
		t.Fatalf("concurrent raise persisted multiple children: records=%#v err=%v", records, err)
	}
}

func TestRegistryRestartRecoveryFromVerifiedStore(t *testing.T) {
	store := newMemoryThreadRiskStore()
	authority := newThreadRiskTestAuthority(56)
	registry, err := NewRegistry(context.Background(), authority, store)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	_, err = registry.ResolveOrRaise(context.Background(), ResolveOrRaiseInput{
		ThreadID: "thread-restart", WorkspaceRealPath: "/workspace/restart", RequestedRisk: domainsecurity.RiskClassGeneral,
		Origin: domainsecurity.RiskPolicyOriginGeneralWorkspace, SignalsDigest: domainsecurity.SHA256Hex([]byte("general")), IssuedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	casePolicy, err := registry.ResolveOrRaise(context.Background(), ResolveOrRaiseInput{
		ThreadID: "thread-restart", WorkspaceRealPath: "/workspace/restart", RequestedRisk: domainsecurity.RiskClassCase,
		Origin: domainsecurity.RiskPolicyOriginValidCaseBinding, SignalsDigest: domainsecurity.SHA256Hex([]byte("binding")),
		IssuedAt: at.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewRegistry(context.Background(), authority, store)
	if err != nil {
		t.Fatal(err)
	}
	current, found := restarted.Current("thread-restart")
	if !found || current != casePolicy {
		t.Fatalf("restart did not recover unique current policy: current=%#v found=%t", current, found)
	}
}

func TestRegistryWriteConflictReadbackFailureAndCancellationFailClosed(t *testing.T) {
	authority := newThreadRiskTestAuthority(57)
	input := ResolveOrRaiseInput{
		ThreadID: "thread-failure", WorkspaceRealPath: "/workspace/failure", RequestedRisk: domainsecurity.RiskClassGeneral,
		Origin: domainsecurity.RiskPolicyOriginGeneralWorkspace, SignalsDigest: domainsecurity.SHA256Hex([]byte("general")),
		IssuedAt: time.Date(2026, 7, 12, 13, 0, 0, 0, time.UTC),
	}

	t.Run("write-conflict", func(t *testing.T) {
		store := newMemoryThreadRiskStore()
		store.putErr = errors.New("write conflict")
		registry, err := NewRegistry(context.Background(), authority, store)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := registry.ResolveOrRaise(context.Background(), input); err == nil {
			t.Fatal("write conflict was ignored")
		}
		if _, found := registry.Current(input.ThreadID); found {
			t.Fatal("failed write updated in-memory current policy")
		}
	})

	t.Run("readback", func(t *testing.T) {
		store := newMemoryThreadRiskStore()
		store.resolveMismatch = true
		registry, err := NewRegistry(context.Background(), authority, store)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := registry.ResolveOrRaise(context.Background(), input); err == nil {
			t.Fatal("mismatched persistence readback was accepted")
		}
		if _, found := registry.Current(input.ThreadID); found {
			t.Fatal("failed readback updated in-memory current policy")
		}
	})

	t.Run("canceled", func(t *testing.T) {
		store := newMemoryThreadRiskStore()
		startupCtx, startupCancel := context.WithCancel(context.Background())
		startupCancel()
		if _, err := NewRegistry(startupCtx, authority, store); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled registry startup returned %v", err)
		}
		registry, err := NewRegistry(context.Background(), authority, store)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := registry.ResolveOrRaise(ctx, input); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled resolution returned %v", err)
		}
		if records, err := store.List(context.Background()); err != nil || len(records) != 0 {
			t.Fatalf("canceled resolution persisted policy: records=%#v err=%v", records, err)
		}
	})
}

type threadRiskTestAuthority struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	keyID      string
}

func newThreadRiskTestAuthority(seed byte) *threadRiskTestAuthority {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	return &threadRiskTestAuthority{privateKey: privateKey, publicKey: publicKey, keyID: domainsecurity.SHA256Hex(publicKey)}
}

func (authority *threadRiskTestAuthority) KeyID() string { return authority.keyID }

func (authority *threadRiskTestAuthority) PublicKey() []byte {
	return append([]byte(nil), authority.publicKey...)
}

func (authority *threadRiskTestAuthority) Sign(ctx context.Context, message []byte) ([]byte, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	return ed25519.Sign(authority.privateKey, append([]byte(nil), message...)), nil
}

func (authority *threadRiskTestAuthority) VerifyTrusted(ctx context.Context, keyID string, publicKey, message, signature []byte) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if keyID != authority.keyID || !bytes.Equal(publicKey, authority.publicKey) ||
		!ed25519.Verify(authority.publicKey, message, signature) {
		return errors.New("untrusted signature")
	}
	return nil
}

func signThreadRiskTestPolicy(t *testing.T, authority *threadRiskTestAuthority, input domainsecurity.ThreadRiskPolicyInputV1) domainsecurity.ThreadRiskPolicyV1 {
	t.Helper()
	input.AuthorityKeyID = authority.KeyID()
	input.AuthorityPublicKey = authority.PublicKey()
	policy, err := domainsecurity.NewThreadRiskPolicyV1(input, func(message []byte) ([]byte, error) {
		return authority.Sign(context.Background(), message)
	})
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func mustThreadRiskTestPublication(t *testing.T, input domainsecurity.TurnPublicationPolicyInputV1) domainsecurity.TurnPublicationPolicyV1 {
	t.Helper()
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(input)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func mustThreadRiskTestContext(t *testing.T, threadID, workspace string, publication domainsecurity.TurnPublicationPolicyV1, issuedAt time.Time) domainsecurity.TurnSecurityContext {
	t.Helper()
	turnSuffix := domainsecurity.SHA256Hex([]byte(threadID + "\x00" + workspace))[:12]
	binding, err := securitycontexttest.WitnessedRiskBinding(threadID, workspace, publication.RiskClass, publication.ThreadRiskPolicyDigest)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn-" + turnSuffix, WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: domainsecurity.UnboundCaseID, CaseBindingHash: domainsecurity.UnboundCaseBindingHash(workspace),
		DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID, SourceManifestHash: domainsecurity.EmptySourceManifestHash,
		ContextEpoch: 1, IssuedAt: issuedAt, PublicationPolicy: publication, RiskAuthorityBinding: binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

type memoryThreadRiskStore struct {
	mu              sync.Mutex
	records         map[string]domainsecurity.ThreadRiskPolicyV1
	putErr          error
	resolveMismatch bool
}

func newMemoryThreadRiskStore() *memoryThreadRiskStore {
	return &memoryThreadRiskStore{records: map[string]domainsecurity.ThreadRiskPolicyV1{}}
}

func memoryThreadRiskStoreWith(policies ...domainsecurity.ThreadRiskPolicyV1) *memoryThreadRiskStore {
	store := newMemoryThreadRiskStore()
	for _, policy := range policies {
		store.records[policy.PolicyDigest] = policy
	}
	return store
}

func (store *memoryThreadRiskStore) PutIfAbsent(ctx context.Context, policy domainsecurity.ThreadRiskPolicyV1) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.putErr != nil {
		return store.putErr
	}
	if current, found := store.records[policy.PolicyDigest]; found {
		if current == policy {
			return nil
		}
		return errors.New("memory policy write conflict")
	}
	store.records[policy.PolicyDigest] = policy
	return nil
}

func (store *memoryThreadRiskStore) Resolve(ctx context.Context, digest string) (domainsecurity.ThreadRiskPolicyV1, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return domainsecurity.ThreadRiskPolicyV1{}, err
		}
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	policy, found := store.records[digest]
	if !found {
		return domainsecurity.ThreadRiskPolicyV1{}, errors.New("memory policy not found")
	}
	if store.resolveMismatch {
		policy.SignalsDigest = domainsecurity.SHA256Hex([]byte("mismatch"))
	}
	return policy, nil
}

func (store *memoryThreadRiskStore) List(ctx context.Context) ([]domainsecurity.ThreadRiskPolicyV1, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	policies := make([]domainsecurity.ThreadRiskPolicyV1, 0, len(store.records))
	for _, policy := range store.records {
		policies = append(policies, policy)
	}
	sort.Slice(policies, func(left, right int) bool { return policies[left].PolicyDigest < policies[right].PolicyDigest })
	return policies, nil
}

func (store *memoryThreadRiskStore) HasRecords(ctx context.Context) (bool, error) {
	policies, err := store.List(ctx)
	return len(policies) > 0, err
}
