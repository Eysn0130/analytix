package steering

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const testContextDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestBindPendingEntryV1ClosesAndBindsAdmission(t *testing.T) {
	raw := map[string]any{
		"id": "steer_1", "clientUserMessageId": "steer_1", "text": "continue",
		"admittedAt": "2026-07-18T01:02:03Z", "delivery": "steer",
		"jobId": "job_1", "childRunId": "job_1", "steerMessageId": "steer_1",
		"parentThreadId": "thread_parent", "childThreadId": "thread_child",
		"sourceTurnId": "turn_parent", "sourceToolCallId": "call_1",
		"jobProjectionVersion": 1, "jobContentDigest": strings.Repeat("b", 64),
		"jobContextDigest": testContextDigest, "jobAuthorityDigest": testContextDigest,
		"jobQueueAuthorityDigest": strings.Repeat("c", 64),
	}
	bound, err := BindPendingEntryV1(raw, testContextDigest)
	if err != nil {
		t.Fatalf("bind pending steering: %v", err)
	}
	if bound["projectionVersion"] != ProjectionVersionV1 || bound["status"] != "pending" ||
		bound["contextDigest"] != testContextDigest {
		t.Fatalf("pending steering authority mismatch: %#v", bound)
	}
	if len(bound["contentDigest"].(string)) != 64 || ValidatePendingEntryForContextV1(bound, testContextDigest) != nil {
		t.Fatalf("bound steering should validate: %#v", bound)
	}

	replay, err := BindPendingEntryV1(raw, testContextDigest)
	if err != nil || !SamePendingEntryV1(bound, replay) {
		t.Fatalf("exact replay should match: replay=%#v err=%v", replay, err)
	}
	replay["text"] = "different authorized payload"
	if SamePendingEntryV1(bound, replay) {
		t.Fatal("same id with different content must not replay")
	}
}

func TestBindPendingEntryV1RejectsUnknownAndPartialProvenance(t *testing.T) {
	base := map[string]any{
		"id": "steer_1", "clientUserMessageId": "steer_1", "text": "continue",
		"admittedAt": "2026-07-18T01:02:03Z", "delivery": "steer",
	}
	for name, mutate := range map[string]func(map[string]any){
		"unknown field":             func(value map[string]any) { value["attachmentIds"] = []any{"att_1"} },
		"partial task job lineage":  func(value map[string]any) { value["childRunId"] = "job_1" },
		"unbound source provenance": func(value map[string]any) { value["sourceTurnId"] = "turn_parent" },
		"effect without ordinary work": func(value map[string]any) {
			value["logicalEffect"] = string(domainsecurity.LogicalEffectOrdinary)
		},
		"ordinary work without effect": func(value map[string]any) { value["ordinaryWork"] = true },
		"unknown logical effect": func(value map[string]any) {
			value["logicalEffect"] = "case"
			value["ordinaryWork"] = false
		},
		"wrong ordinary work type": func(value map[string]any) {
			value["logicalEffect"] = string(domainsecurity.LogicalEffectCaseData)
			value["ordinaryWork"] = "false"
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := cloneEntryMap(base)
			mutate(value)
			if bound, err := BindPendingEntryV1(value, testContextDigest); err == nil {
				t.Fatalf("unsafe admission should fail closed: %#v", bound)
			}
		})
	}
}

func TestSteeringLogicalEffectBindingIsAdmissionAndPromotionSigned(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := BindPendingEntryV1(map[string]any{
		"id": EntryIDV1("turn-effect", "client-effect"), "clientUserMessageId": "client-effect",
		"text": "continue the mixed investigation", "admittedAt": "2026-07-18T01:02:03Z", "delivery": "steer",
		"logicalEffect": string(domainsecurity.LogicalEffectFundsData), "ordinaryWork": true,
	}, testContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	binding, present, err := LogicalEffectBindingFromEntryV1(pending)
	if err != nil || !present || binding.LogicalEffect != domainsecurity.LogicalEffectFundsData || !binding.OrdinaryWork {
		t.Fatalf("logical effect binding was not preserved: binding=%#v present=%t err=%v", binding, present, err)
	}
	pending = sealPendingEntryForTest(t, pending, publicKey, privateKey)

	for name, mutate := range map[string]func(map[string]any){
		"effect":        func(value map[string]any) { value["logicalEffect"] = string(domainsecurity.LogicalEffectCaseData) },
		"ordinary work": func(value map[string]any) { value["ordinaryWork"] = false },
	} {
		t.Run("admission "+name, func(t *testing.T) {
			tampered := cloneEntryMap(pending)
			mutate(tampered)
			content, parseErr := parseEntryContentV1(tampered, pendingFieldsV1)
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			tampered["contentDigest"] = contentDigestV1(content)
			if _, materialErr := EntryAuthorityMaterialV1(tampered, testContextDigest); materialErr == nil {
				t.Fatal("self-consistent logical effect rewrite forged admission authority")
			}
		})
	}

	promoted := cloneEntryMap(pending)
	promoted["status"] = "promoted"
	promoted["promotedAt"] = "2026-07-18T01:02:04Z"
	promoted["promotedItemId"] = promoted["id"]
	promotionBytes, err := PromotedEntrySigningBytesV1(promoted, testContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	promoted, err = SealPromotedEntryAuthorityV1(
		promoted, testContextDigest, sha256HexForTest(publicKey), publicKey,
		ed25519.Sign(privateKey, promotionBytes),
	)
	if err != nil {
		t.Fatal(err)
	}
	tamperedPromotion := cloneEntryMap(promoted)
	tamperedPromotion["ordinaryWork"] = false
	content, err := parseEntryContentV1(tamperedPromotion, promotedFieldsV1)
	if err != nil {
		t.Fatal(err)
	}
	tamperedPromotion["contentDigest"] = contentDigestV1(content)
	newAdmissionBytes, err := PendingEntrySigningBytesV1(tamperedPromotion, testContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	tamperedPromotion["authoritySignature"] = base64.RawURLEncoding.EncodeToString(
		ed25519.Sign(privateKey, newAdmissionBytes),
	)
	if err := ValidatePromotedEntryForContextV1(tamperedPromotion, testContextDigest); err == nil {
		t.Fatal("new admission signature reused an old promotion signature for changed logical effect metadata")
	}
}

func TestLegacySteeringSignatureBytesRemainCompatibleWithoutEffectBinding(t *testing.T) {
	pending, err := BindPendingEntryV1(map[string]any{
		"id": EntryIDV1("turn-legacy", "client-legacy"), "clientUserMessageId": "client-legacy",
		"text": "legacy guidance", "admittedAt": "2026-07-18T01:02:03Z", "delivery": "steer",
	}, testContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	if binding, present, parseErr := LogicalEffectBindingFromEntryV1(pending); parseErr != nil || present || binding != (EntryLogicalEffectBinding{}) {
		t.Fatalf("legacy entry did not remain explicitly unbound: binding=%#v present=%t err=%v", binding, present, parseErr)
	}
	const legacyContentDigest = "300a008f6bb0668016b95d924a52aaf15cde840986c62b26e970ba06c49c8830"
	const legacySignature = "juIoCXs_gwqqMiU-5iHUaniOZVXBycKktxt5MgZv8qCpmqrKl5jbSEIGT81f_EXVXJNahcUZwsCEvzPeY8_mDw"
	if pending["contentDigest"] != legacyContentDigest {
		t.Fatalf("legacy content bytes changed: got %v want %s", pending["contentDigest"], legacyContentDigest)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x42}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	signingBytes, err := PendingEntrySigningBytesV1(pending, testContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(privateKey, signingBytes)
	if encoded := base64.RawURLEncoding.EncodeToString(signature); encoded != legacySignature {
		t.Fatalf("legacy signing bytes changed: signature=%s", encoded)
	}
	sealed, err := SealPendingEntryAuthorityV1(
		pending, testContextDigest, sha256HexForTest(publicKey), publicKey, signature,
	)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(sealed)
	if err != nil {
		t.Fatal(err)
	}
	restored := map[string]any{}
	if err := json.Unmarshal(body, &restored); err != nil {
		t.Fatal(err)
	}
	if _, err := EntryAuthorityMaterialV1(restored, testContextDigest); err != nil {
		t.Fatalf("legacy signed entry failed recovery verification: %v", err)
	}
}

func TestValidatePendingEntryForContextV1RejectsTampering(t *testing.T) {
	bound, err := BindPendingEntryV1(map[string]any{
		"id": "steer_1", "clientUserMessageId": "steer_1", "text": "continue",
		"admittedAt": "2026-07-18T01:02:03Z", "delivery": "steer",
	}, testContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"fractional version":   func(value map[string]any) { value["projectionVersion"] = 1.5 },
		"wrong version type":   func(value map[string]any) { value["projectionVersion"] = "1" },
		"wrong context":        func(value map[string]any) { value["contextDigest"] = strings.Repeat("b", 64) },
		"wrong content digest": func(value map[string]any) { value["contentDigest"] = strings.Repeat("b", 64) },
		"changed text":         func(value map[string]any) { value["text"] = "fabricated account 6222021234567890123" },
		"changed status":       func(value map[string]any) { value["status"] = "promoted" },
		"unknown field":        func(value map[string]any) { value["reasoning"] = "private chain of thought" },
	} {
		t.Run(name, func(t *testing.T) {
			value := cloneEntryMap(bound)
			mutate(value)
			if err := ValidatePendingEntryForContextV1(value, testContextDigest); err == nil {
				t.Fatalf("tampered pending steering should fail closed: %#v", value)
			}
		})
	}
}

func TestSteeringAuthorityRejectsSelfConsistentDiskRewrite(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := BindPendingEntryV1(map[string]any{
		"id": EntryIDV1("turn_1", "client_1"), "clientUserMessageId": "client_1", "text": "authorized guidance",
		"admittedAt": "2026-07-18T01:02:03Z", "delivery": "steer",
	}, testContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	signingBytes, err := PendingEntrySigningBytesV1(pending, testContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := SealPendingEntryAuthorityV1(
		pending, testContextDigest, sha256HexForTest(publicKey), publicKey, ed25519.Sign(privateKey, signingBytes),
	)
	if err != nil {
		t.Fatal(err)
	}
	material, err := EntryAuthorityMaterialV1(sealed, testContextDigest)
	if err != nil || !ed25519.Verify(publicKey, material.SigningBytes, material.Signature) {
		t.Fatalf("sealed steering authority did not verify: material=%#v err=%v", material, err)
	}

	tampered := cloneEntryMap(sealed)
	tampered["text"] = "fabricated account 6222021234567890123"
	content, err := parseEntryContentV1(tampered, pendingFieldsV1)
	if err != nil {
		t.Fatal(err)
	}
	tampered["contentDigest"] = contentDigestV1(content)
	if _, err := EntryAuthorityMaterialV1(tampered, testContextDigest); err == nil {
		t.Fatal("self-consistent content hash rewrite must not forge host authority")
	}
}

func TestPendingAdmissionSignatureCannotForgePromotion(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	entryID := EntryIDV1("turn_1", "client_1")
	pending, err := BindPendingEntryV1(map[string]any{
		"id": entryID, "clientUserMessageId": "client_1", "text": "authorized guidance",
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
	forged := cloneEntryMap(pending)
	forged["status"] = "promoted"
	forged["promotedAt"] = "2026-07-18T01:02:04Z"
	forged["promotedItemId"] = entryID
	forged["promotionAuthorityKeyId"] = forged["authorityKeyId"]
	forged["promotionAuthorityPublicKey"] = forged["authorityPublicKey"]
	forged["promotionAuthoritySignature"] = forged["authoritySignature"]
	if _, err := EntryAuthorityMaterialV1(forged, testContextDigest); err == nil {
		t.Fatal("pending admission signature forged a host promotion")
	}

	promoted := cloneEntryMap(pending)
	promoted["status"] = "promoted"
	promoted["promotedAt"] = "2026-07-18T01:02:04Z"
	promoted["promotedItemId"] = entryID
	promotionBytes, err := PromotedEntrySigningBytesV1(promoted, testContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	promoted, err = SealPromotedEntryAuthorityV1(
		promoted, testContextDigest, sha256HexForTest(publicKey), publicKey, ed25519.Sign(privateKey, promotionBytes),
	)
	if err != nil {
		t.Fatal(err)
	}
	material, err := EntryAuthorityMaterialV1(promoted, testContextDigest)
	if err != nil || !ed25519.Verify(publicKey, material.SigningBytes, material.Signature) {
		t.Fatalf("host promotion signature did not verify: material=%#v err=%v", material, err)
	}
	tampered := cloneEntryMap(promoted)
	tampered["promotedAt"] = "2026-07-18T01:02:05Z"
	if _, err := EntryAuthorityMaterialV1(tampered, testContextDigest); err == nil {
		t.Fatal("promotion metadata changed without a new host signature")
	}
}

func sha256HexForTest(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func cloneEntryMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func sealPendingEntryForTest(
	t *testing.T,
	pending map[string]any,
	publicKey ed25519.PublicKey,
	privateKey ed25519.PrivateKey,
) map[string]any {
	t.Helper()
	signingBytes, err := PendingEntrySigningBytesV1(pending, testContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := SealPendingEntryAuthorityV1(
		pending, testContextDigest, sha256HexForTest(publicKey), publicKey,
		ed25519.Sign(privateKey, signingBytes),
	)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}
