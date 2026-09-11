package steering

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestPromoteTurnEntryPrefixV1PromotesExactTwoTwoOneGroups(t *testing.T) {
	publicKey, privateKey, verify, promote := steeringPrefixAuthorityForTest(t)
	effects := []struct {
		effect       domainsecurity.LogicalEffect
		ordinaryWork bool
	}{
		{domainsecurity.LogicalEffectOrdinary, true},
		{domainsecurity.LogicalEffectOrdinary, true},
		{domainsecurity.LogicalEffectCaseData, false},
		{domainsecurity.LogicalEffectCaseData, true},
		{domainsecurity.LogicalEffectFundsData, false},
	}
	steering := make([]any, 0, len(effects))
	for index, effect := range effects {
		steering = append(steering, steeringPrefixPendingForTest(
			t, publicKey, privateKey, index+1, effect.effect, effect.ordinaryWork,
		))
	}
	turn := map[string]any{"steering": steering, "items": []any{}}
	groupEffects := []domainsecurity.LogicalEffect{
		domainsecurity.LogicalEffectOrdinary,
		domainsecurity.LogicalEffectCaseData,
		domainsecurity.LogicalEffectFundsData,
	}

	for groupIndex, groupSize := range []int{2, 2, 1} {
		pending, err := PendingEntriesFromTurnV1("thread-prefix", "turn-prefix", turn, testContextDigest, verify)
		if err != nil {
			t.Fatal(err)
		}
		expected := make([]PendingEntryExpectationV1, 0, groupSize)
		for _, entry := range pending[:groupSize] {
			candidate, expectationErr := NewPendingEntryExpectationV1(entry)
			if expectationErr != nil {
				t.Fatal(expectationErr)
			}
			expected = append(expected, candidate)
		}
		updated, entries, items, err := PromoteTurnEntryPrefixV1(
			"thread-prefix", "turn-prefix", turn, expected, testContextDigest,
			fmt.Sprintf("2026-07-18T01:02:%02dZ", 10+groupIndex), verify, promote,
		)
		if err != nil || len(entries) != groupSize || len(items) != groupSize {
			t.Fatalf("group %d promotion failed: entries=%d items=%d err=%v", groupIndex, len(entries), len(items), err)
		}
		for _, entry := range entries {
			binding, present, bindingErr := LogicalEffectBindingFromEntryV1(entry)
			if bindingErr != nil || !present || binding.LogicalEffect != groupEffects[groupIndex] {
				t.Fatalf("group %d crossed a logical-effect boundary: binding=%#v present=%t err=%v", groupIndex, binding, present, bindingErr)
			}
		}
		turn = updated
		remaining, err := PendingEntriesFromTurnV1("thread-prefix", "turn-prefix", turn, testContextDigest, verify)
		if err != nil {
			t.Fatal(err)
		}
		wantRemaining := len(effects)
		for _, consumed := range []int{2, 2, 1}[:groupIndex+1] {
			wantRemaining -= consumed
		}
		if len(remaining) != wantRemaining {
			t.Fatalf("group %d consumed the pending tail: got %d remaining want %d", groupIndex, len(remaining), wantRemaining)
		}
	}
	if len(steeringListV1(turn["items"])) != 5 {
		t.Fatalf("promoted item count = %d, want 5", len(steeringListV1(turn["items"])))
	}
}

func TestPromoteTurnEntryPrefixV1RejectsStaleReorderedAndSkippedCAS(t *testing.T) {
	publicKey, privateKey, verify, promote := steeringPrefixAuthorityForTest(t)
	entries := []map[string]any{
		steeringPrefixPendingForTest(t, publicKey, privateKey, 1, domainsecurity.LogicalEffectOrdinary, true),
		steeringPrefixPendingForTest(t, publicKey, privateKey, 2, domainsecurity.LogicalEffectCaseData, false),
		steeringPrefixPendingForTest(t, publicKey, privateKey, 3, domainsecurity.LogicalEffectFundsData, false),
	}
	turn := map[string]any{"steering": []any{entries[0], entries[1], entries[2]}, "items": []any{}}
	expectations := make([]PendingEntryExpectationV1, 0, len(entries))
	for _, entry := range entries {
		expected, err := NewPendingEntryExpectationV1(entry)
		if err != nil {
			t.Fatal(err)
		}
		expectations = append(expectations, expected)
	}
	invalid := map[string][]PendingEntryExpectationV1{
		"empty while pending": nil,
		"skipped head":        {expectations[1]},
		"reordered":           {expectations[1], expectations[0]},
		"stale digest": {{
			ID: expectations[0].ID, ContentDigest: domainsecurity.SHA256Hex([]byte("stale")),
		}},
		"duplicate": {expectations[0], expectations[0]},
		"past tail": append(append([]PendingEntryExpectationV1(nil), expectations...), PendingEntryExpectationV1{
			ID: "item_steer_missing", ContentDigest: domainsecurity.SHA256Hex([]byte("missing")),
		}),
		"malformed": {{ID: expectations[0].ID, ContentDigest: "not-a-digest"}},
	}
	for name, expected := range invalid {
		t.Run(name, func(t *testing.T) {
			updated, promotedEntries, items, err := PromoteTurnEntryPrefixV1(
				"thread-prefix", "turn-prefix", turn, expected, testContextDigest,
				"2026-07-18T01:02:10Z", verify, promote,
			)
			if !errors.Is(err, ErrPendingPrefixMismatch) || updated != nil || len(promotedEntries) != 0 || len(items) != 0 {
				t.Fatalf("invalid CAS input mutated promotion: updated=%#v entries=%#v items=%#v err=%v", updated, promotedEntries, items, err)
			}
			for _, entry := range steeringListV1(turn["steering"]) {
				if stringField(entry.(map[string]any), "status") != "pending" {
					t.Fatal("failed CAS mutated the input turn")
				}
			}
		})
	}
}

func TestPromoteTurnEntryPrefixV1ValidatesPendingTailBeforeCAS(t *testing.T) {
	publicKey, privateKey, verify, promote := steeringPrefixAuthorityForTest(t)
	first := steeringPrefixPendingForTest(t, publicKey, privateKey, 1, domainsecurity.LogicalEffectOrdinary, true)
	tail := steeringPrefixPendingForTest(t, publicKey, privateKey, 2, domainsecurity.LogicalEffectFundsData, false)
	expected, err := NewPendingEntryExpectationV1(first)
	if err != nil {
		t.Fatal(err)
	}
	tamperedTail := cloneEntryMap(tail)
	tamperedTail["text"] = "tampered tail"
	promoteCalls := 0
	updated, entries, items, err := PromoteTurnEntryPrefixV1(
		"thread-prefix", "turn-prefix",
		map[string]any{"steering": []any{first, tamperedTail}, "items": []any{}},
		[]PendingEntryExpectationV1{expected}, testContextDigest, "2026-07-18T01:02:10Z", verify,
		func(entry map[string]any, contextDigest string) (map[string]any, error) {
			promoteCalls++
			return promote(entry, contextDigest)
		},
	)
	if !errors.Is(err, ErrProjectionInvalid) || updated != nil || len(entries) != 0 || len(items) != 0 || promoteCalls != 0 {
		t.Fatalf("tampered pending tail was not rejected before promotion: updated=%#v entries=%#v items=%#v calls=%d err=%v", updated, entries, items, promoteCalls, err)
	}
}

func steeringPrefixAuthorityForTest(
	t *testing.T,
) (ed25519.PublicKey, ed25519.PrivateKey, AuthorityVerifierV1, AuthorityPromoterV1) {
	t.Helper()
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
			entry, contextDigest, sha256HexForTest(publicKey), publicKey,
			ed25519.Sign(privateKey, signingBytes),
		)
	}
	return publicKey, privateKey, verify, promote
}

func steeringPrefixPendingForTest(
	t *testing.T,
	publicKey ed25519.PublicKey,
	privateKey ed25519.PrivateKey,
	index int,
	effect domainsecurity.LogicalEffect,
	ordinaryWork bool,
) map[string]any {
	t.Helper()
	clientID := fmt.Sprintf("client-%d", index)
	pending, err := BindPendingEntryV1(map[string]any{
		"id": EntryIDV1("turn-prefix", clientID), "clientUserMessageId": clientID,
		"text": fmt.Sprintf("steering %d", index), "admittedAt": fmt.Sprintf("2026-07-18T01:02:%02dZ", index),
		"delivery": "steer", "logicalEffect": string(effect), "ordinaryWork": ordinaryWork,
	}, testContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	return sealPendingEntryForTest(t, pending, publicKey, privateKey)
}
