package steering

import (
	"bytes"
	"crypto/ed25519"
	"testing"
)

func TestPromotionCommitV1BindsExactSignedEntryAndItem(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	verify := func(entry map[string]any, contextDigest string) error {
		material, materialErr := EntryAuthorityMaterialV1(entry, contextDigest)
		if materialErr != nil || !bytes.Equal(material.PublicKey, publicKey) ||
			!ed25519.Verify(publicKey, material.SigningBytes, material.Signature) {
			return ErrProjectionInvalid
		}
		return nil
	}
	promote := func(entry map[string]any, contextDigest string) (map[string]any, error) {
		signingBytes, signingErr := PromotedEntrySigningBytesV1(entry, contextDigest)
		if signingErr != nil {
			return nil, signingErr
		}
		return SealPromotedEntryAuthorityV1(
			entry, contextDigest, sha256HexForTest(publicKey), publicKey, ed25519.Sign(privateKey, signingBytes),
		)
	}
	entryID := EntryIDV1("turn-1", "client-1")
	pending, err := BindPendingEntryV1(map[string]any{
		"id": entryID, "clientUserMessageId": "client-1", "text": "authorized guidance",
		"admittedAt": "2026-07-18T01:02:03Z", "delivery": "steer",
	}, testContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	admissionBytes, err := PendingEntrySigningBytesV1(pending, testContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	pending, err = SealPendingEntryAuthorityV1(
		pending, testContextDigest, sha256HexForTest(publicKey), publicKey, ed25519.Sign(privateKey, admissionBytes),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, entries, items, err := PromoteTurnEntriesV1(
		"thread-1", "turn-1", map[string]any{"steering": []any{pending}, "items": []any{}},
		testContextDigest, "2026-07-18T01:02:04Z", verify, promote,
	)
	if err != nil || len(entries) != 1 || len(items) != 1 {
		t.Fatalf("promote steering: entries=%#v items=%#v err=%v", entries, items, err)
	}
	commit, err := NewPromotionCommitV1("thread-1", "turn-1", testContextDigest, entries[0], items[0], verify)
	if err != nil || commit.CommitID == "" || commit.EntryID != entryID || commit.Origin != "ordinary" {
		t.Fatalf("build promotion commit: commit=%#v err=%v", commit, err)
	}
	replayed, err := NewPromotionCommitV1("thread-1", "turn-1", testContextDigest, entries[0], items[0], verify)
	if err != nil || replayed.CommitID != commit.CommitID {
		t.Fatalf("exact promotion replay changed commit id: replay=%#v err=%v", replayed, err)
	}

	for name, mutate := range map[string]func(map[string]any){
		"unknown field":        func(item map[string]any) { item["reasoning"] = "must never persist" },
		"changed created at":   func(item map[string]any) { item["createdAt"] = "2026-07-18T01:02:02Z" },
		"changed finished at":  func(item map[string]any) { item["finishedAt"] = "2026-07-18T01:02:05Z" },
		"changed display text": func(item map[string]any) { item["displayText"] = "different" },
	} {
		t.Run(name, func(t *testing.T) {
			item := cloneSteeringMapV1(items[0])
			mutate(item)
			if ValidatePromotedItemForContextV1(entries[0], item, "thread-1", "turn-1", testContextDigest) == nil {
				t.Fatal("tampered promoted item passed exact validation")
			}
			if _, err := NewPromotionCommitV1("thread-1", "turn-1", testContextDigest, entries[0], item, verify); err == nil {
				t.Fatal("tampered promoted item minted a commit")
			}
		})
	}
}
