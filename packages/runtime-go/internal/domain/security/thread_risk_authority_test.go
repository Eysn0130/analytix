package security

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

type threadRiskAuthorityTestKeys struct {
	privateKey     ed25519.PrivateKey
	publicKey      ed25519.PublicKey
	keyID          string
	installationID string
	enrollmentID   string
}

func newThreadRiskAuthorityTestKeys(t *testing.T) threadRiskAuthorityTestKeys {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return threadRiskAuthorityTestKeys{
		privateKey: privateKey, publicKey: publicKey, keyID: SHA256Hex(publicKey),
		installationID: SHA256Hex([]byte("installation-a")), enrollmentID: SHA256Hex([]byte("enrollment-a")),
	}
}

func (keys threadRiskAuthorityTestKeys) sign(message []byte) ([]byte, error) {
	return ed25519.Sign(keys.privateKey, message), nil
}

func testThreadRiskAuthorityEntry(threadID, workspace, riskClass, current, previous string) ThreadRiskAuthorityEntryV1 {
	return ThreadRiskAuthorityEntryV1{
		ThreadID: threadID, WorkspaceRealPath: workspace, RiskClass: riskClass,
		CurrentPolicyDigest: current, PreviousPolicyDigest: previous,
	}
}

func newTestThreadRiskAuthorityIndex(t *testing.T, keys threadRiskAuthorityTestKeys, generation uint64, previous, mutation string, entries []ThreadRiskAuthorityEntryV1) ThreadRiskAuthorityIndexV1 {
	t.Helper()
	index, err := NewThreadRiskAuthorityIndexV1(ThreadRiskAuthorityIndexInputV1{
		InstallationID: keys.installationID, EnrollmentID: keys.enrollmentID,
		Namespace: ThreadRiskAuthorityNamespaceV1, Generation: generation, Entries: entries,
		PreviousIndexDigest: previous, MutationID: mutation,
		AuthorityKeyID: keys.keyID, AuthorityPublicKey: keys.publicKey,
	}, keys.sign)
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func resignTestThreadRiskAuthorityIndex(t *testing.T, keys threadRiskAuthorityTestKeys, index ThreadRiskAuthorityIndexV1) ThreadRiskAuthorityIndexV1 {
	t.Helper()
	index.StateDigest = threadRiskAuthorityStateDigestV1(index.Entries)
	index.AuthoritySignature = ""
	index.IndexDigest = ""
	index.AuthoritySignature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(keys.privateKey, ThreadRiskAuthorityIndexSigningBytesV1(index)))
	index.IndexDigest = threadRiskAuthorityIndexDigestV1(index)
	return index
}

func TestThreadRiskAuthorityIndexV1CanonicalRoundTripAndInstallationAnchor(t *testing.T) {
	keys := newThreadRiskAuthorityTestKeys(t)
	entry := testThreadRiskAuthorityEntry("thread-a", "/workspace/a", RiskClassGeneral, SHA256Hex([]byte("policy-a")), "")
	index := newTestThreadRiskAuthorityIndex(t, keys, 1, "", SHA256Hex([]byte("mutation-1")), []ThreadRiskAuthorityEntryV1{entry})
	body, err := ThreadRiskAuthorityIndexV1Bytes(index)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseThreadRiskAuthorityIndexV1(body)
	if err != nil || parsed.IndexDigest != index.IndexDigest {
		t.Fatalf("canonical round trip failed: parsed=%+v err=%v", parsed, err)
	}
	if err := ValidateThreadRiskAuthorityIndexForInstallationV1(index, keys.installationID, keys.keyID, keys.publicKey); err != nil {
		t.Fatal(err)
	}
	attacker := newThreadRiskAuthorityTestKeys(t)
	attackerIndex := newTestThreadRiskAuthorityIndex(t, attacker, 1, "", SHA256Hex([]byte("attacker-mutation")), []ThreadRiskAuthorityEntryV1{entry})
	if ValidateThreadRiskAuthorityIndexV1(attackerIndex) != nil {
		t.Fatal("self-signed attacker index should be authentic under its own key")
	}
	if ValidateThreadRiskAuthorityIndexForInstallationV1(attackerIndex, keys.installationID, keys.keyID, keys.publicKey) == nil {
		t.Fatal("self-signed attacker index was accepted as installation authority")
	}
	if _, err := NewThreadRiskAuthorityIndexV1(ThreadRiskAuthorityIndexInputV1{
		InstallationID: keys.installationID, EnrollmentID: keys.enrollmentID,
		Namespace: EvidenceRegistryAuthorityNamespaceV1, Generation: 1, Entries: []ThreadRiskAuthorityEntryV1{},
		MutationID: SHA256Hex([]byte("wrong-namespace-mutation")), AuthorityKeyID: keys.keyID, AuthorityPublicKey: keys.publicKey,
	}, keys.sign); err == nil {
		t.Fatal("thread risk index accepted the generic evidence-registry namespace")
	}

	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	object["unknown"] = true
	unknown, _ := json.Marshal(object)
	if _, err := ParseThreadRiskAuthorityIndexV1(unknown); err == nil {
		t.Fatal("unknown index property was accepted")
	}
	duplicate := bytes.Replace(body, []byte(`"generation":1`), []byte(`"generation":1,"generation":2`), 1)
	if _, err := ParseThreadRiskAuthorityIndexV1(duplicate); err == nil {
		t.Fatal("duplicate index property was accepted")
	}
	wrongCase := bytes.Replace(body, []byte(`"schemaVersion"`), []byte(`"SchemaVersion"`), 1)
	if _, err := ParseThreadRiskAuthorityIndexV1(wrongCase); err == nil {
		t.Fatal("non-canonical index property casing was accepted")
	}
	if _, err := ParseThreadRiskAuthorityIndexV1(append(body, []byte(` {}`)...)); err == nil {
		t.Fatal("trailing index JSON was accepted")
	}
}

func TestThreadRiskAuthorityEntryMustMatchExactSignedPolicy(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := NewThreadRiskPolicyV1(ThreadRiskPolicyInputV1{
		ThreadID: "thread-policy", WorkspaceRealPath: "/workspace/policy", RiskClass: RiskClassCase,
		Origin: RiskPolicyOriginValidCaseBinding, SignalsDigest: SHA256Hex([]byte("signals")),
		IssuedAt: time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC), AuthorityKeyID: SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	entry, err := ThreadRiskAuthorityEntryFromPolicyV1(policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateThreadRiskAuthorityEntryForPolicyV1(entry, policy); err != nil {
		t.Fatal(err)
	}
	entry.WorkspaceRealPath = "/workspace/other"
	if ValidateThreadRiskAuthorityEntryForPolicyV1(entry, policy) == nil {
		t.Fatal("entry with mismatched policy semantics was accepted")
	}
}

func TestThreadRiskAuthorityIndexV1RequiresUniqueSortedEntries(t *testing.T) {
	keys := newThreadRiskAuthorityTestKeys(t)
	first := testThreadRiskAuthorityEntry("thread-a", "/workspace/a", RiskClassGeneral, SHA256Hex([]byte("policy-a")), "")
	second := testThreadRiskAuthorityEntry("thread-b", "/workspace/b", RiskClassCase, SHA256Hex([]byte("policy-b")), "")
	valid := newTestThreadRiskAuthorityIndex(t, keys, 1, "", SHA256Hex([]byte("mutation-1")), []ThreadRiskAuthorityEntryV1{first, second})

	unsorted := valid
	unsorted.Entries = []ThreadRiskAuthorityEntryV1{second, first}
	unsorted = resignTestThreadRiskAuthorityIndex(t, keys, unsorted)
	if ValidateThreadRiskAuthorityIndexV1(unsorted) == nil {
		t.Fatal("authentically signed unsorted index was accepted")
	}
	duplicateThread := valid
	duplicateThread.Entries = []ThreadRiskAuthorityEntryV1{first, first}
	duplicateThread = resignTestThreadRiskAuthorityIndex(t, keys, duplicateThread)
	if ValidateThreadRiskAuthorityIndexV1(duplicateThread) == nil {
		t.Fatal("authentically signed duplicate thread was accepted")
	}
	duplicatePolicy := valid
	duplicatePolicy.Entries[1].CurrentPolicyDigest = duplicatePolicy.Entries[0].CurrentPolicyDigest
	duplicatePolicy = resignTestThreadRiskAuthorityIndex(t, keys, duplicatePolicy)
	if ValidateThreadRiskAuthorityIndexV1(duplicatePolicy) == nil {
		t.Fatal("authentically signed duplicate policy head was accepted")
	}
	nullEntries := valid
	nullEntries.Entries = nil
	nullEntries = resignTestThreadRiskAuthorityIndex(t, keys, nullEntries)
	if ValidateThreadRiskAuthorityIndexV1(nullEntries) == nil {
		t.Fatal("authentically signed null entries inventory was accepted")
	}
}

func TestThreadRiskAuthorityIndexTransitionIsExactlyOneMonotonicThreadStep(t *testing.T) {
	keys := newThreadRiskAuthorityTestKeys(t)
	oldPolicy := SHA256Hex([]byte("policy-general"))
	newPolicy := SHA256Hex([]byte("policy-case"))
	oldEntry := testThreadRiskAuthorityEntry("thread-a", "/workspace/a", RiskClassGeneral, oldPolicy, "")
	previous := newTestThreadRiskAuthorityIndex(t, keys, 1, "", SHA256Hex([]byte("mutation-1")), []ThreadRiskAuthorityEntryV1{oldEntry})
	newEntry := testThreadRiskAuthorityEntry("thread-a", "/workspace/a", RiskClassCase, newPolicy, oldPolicy)
	next := newTestThreadRiskAuthorityIndex(t, keys, 2, previous.IndexDigest, SHA256Hex([]byte("mutation-2")), []ThreadRiskAuthorityEntryV1{newEntry})
	if err := ValidateThreadRiskAuthorityIndexTransitionV1(previous, next); err != nil {
		t.Fatal(err)
	}

	tests := map[string]func(ThreadRiskAuthorityIndexV1) ThreadRiskAuthorityIndexV1{
		"generation skip": func(value ThreadRiskAuthorityIndexV1) ThreadRiskAuthorityIndexV1 {
			value.Generation = 3
			return value
		},
		"wrong old head": func(value ThreadRiskAuthorityIndexV1) ThreadRiskAuthorityIndexV1 {
			value.PreviousIndexDigest = SHA256Hex([]byte("other-index"))
			return value
		},
		"policy chain skip": func(value ThreadRiskAuthorityIndexV1) ThreadRiskAuthorityIndexV1 {
			value.Entries[0].PreviousPolicyDigest = SHA256Hex([]byte("other-policy"))
			return value
		},
		"workspace swap": func(value ThreadRiskAuthorityIndexV1) ThreadRiskAuthorityIndexV1 {
			value.Entries[0].WorkspaceRealPath = "/workspace/attacker"
			return value
		},
		"enrollment swap": func(value ThreadRiskAuthorityIndexV1) ThreadRiskAuthorityIndexV1 {
			value.EnrollmentID = SHA256Hex([]byte("other-enrollment"))
			return value
		},
		"mutation reuse": func(value ThreadRiskAuthorityIndexV1) ThreadRiskAuthorityIndexV1 {
			value.MutationID = previous.MutationID
			return value
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			forged := resignTestThreadRiskAuthorityIndex(t, keys, mutate(next))
			if ValidateThreadRiskAuthorityIndexTransitionV1(previous, forged) == nil {
				t.Fatal("malicious transition was accepted")
			}
		})
	}

	deleted := newTestThreadRiskAuthorityIndex(t, keys, 2, previous.IndexDigest, SHA256Hex([]byte("mutation-delete")), nil)
	if ValidateThreadRiskAuthorityIndexTransitionV1(previous, deleted) == nil {
		t.Fatal("entry deletion was accepted")
	}

	caseEntry := testThreadRiskAuthorityEntry("thread-a", "/workspace/a", RiskClassCase, SHA256Hex([]byte("case-1")), "")
	casePrevious := newTestThreadRiskAuthorityIndex(t, keys, 1, "", SHA256Hex([]byte("case-mutation-1")), []ThreadRiskAuthorityEntryV1{caseEntry})
	downgradeEntry := testThreadRiskAuthorityEntry("thread-a", "/workspace/a", RiskClassGeneral, SHA256Hex([]byte("case-2")), caseEntry.CurrentPolicyDigest)
	downgrade := newTestThreadRiskAuthorityIndex(t, keys, 2, casePrevious.IndexDigest, SHA256Hex([]byte("case-mutation-2")), []ThreadRiskAuthorityEntryV1{downgradeEntry})
	if ValidateThreadRiskAuthorityIndexTransitionV1(casePrevious, downgrade) == nil {
		t.Fatal("case-to-general downgrade was accepted")
	}

	generalMoved := testThreadRiskAuthorityEntry("thread-a", "/workspace/general-successor", RiskClassGeneral, SHA256Hex([]byte("general-moved")), oldPolicy)
	generalSuccessor := newTestThreadRiskAuthorityIndex(t, keys, 2, previous.IndexDigest, SHA256Hex([]byte("general-move-mutation")), []ThreadRiskAuthorityEntryV1{generalMoved})
	if err := ValidateThreadRiskAuthorityIndexTransitionV1(previous, generalSuccessor); err != nil {
		t.Fatalf("signed general-to-general workspace successor was rejected: %v", err)
	}
}

func TestThreadRiskAuthorityIndexRejectsBatchMutation(t *testing.T) {
	keys := newThreadRiskAuthorityTestKeys(t)
	oldA := testThreadRiskAuthorityEntry("thread-a", "/workspace/a", RiskClassGeneral, SHA256Hex([]byte("a-1")), "")
	oldB := testThreadRiskAuthorityEntry("thread-b", "/workspace/b", RiskClassGeneral, SHA256Hex([]byte("b-1")), "")
	previous := newTestThreadRiskAuthorityIndex(t, keys, 1, "", SHA256Hex([]byte("mutation-1")), []ThreadRiskAuthorityEntryV1{oldA, oldB})
	newA := testThreadRiskAuthorityEntry("thread-a", "/workspace/a", RiskClassCase, SHA256Hex([]byte("a-2")), oldA.CurrentPolicyDigest)
	newB := testThreadRiskAuthorityEntry("thread-b", "/workspace/b", RiskClassCase, SHA256Hex([]byte("b-2")), oldB.CurrentPolicyDigest)
	next := newTestThreadRiskAuthorityIndex(t, keys, 2, previous.IndexDigest, SHA256Hex([]byte("mutation-2")), []ThreadRiskAuthorityEntryV1{newA, newB})
	if ValidateThreadRiskAuthorityIndexTransitionV1(previous, next) == nil {
		t.Fatal("two-thread batch mutation was accepted")
	}

	added := testThreadRiskAuthorityEntry("thread-c", "/workspace/c", RiskClassGeneral, SHA256Hex([]byte("c-1")), "")
	oneAdd := newTestThreadRiskAuthorityIndex(t, keys, 2, previous.IndexDigest, SHA256Hex([]byte("mutation-add")), []ThreadRiskAuthorityEntryV1{oldA, oldB, added})
	if err := ValidateThreadRiskAuthorityIndexTransitionV1(previous, oneAdd); err != nil {
		t.Fatalf("single new thread step rejected: %v", err)
	}
}
