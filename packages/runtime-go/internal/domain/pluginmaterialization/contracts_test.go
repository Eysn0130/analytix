package pluginmaterialization

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const fixturePluginVersionV1 = "0.16.16"

func TestBundledPluginMaterializationContractsCanonicalSignedLifecycle(t *testing.T) {
	now := time.Date(2026, 7, 23, 10, 11, 12, 123, time.UTC)
	intent := testIntent(t, now)
	intentBody, err := IntentV1Bytes(intent)
	if err != nil {
		t.Fatal(err)
	}
	parsedIntent, err := ParseIntentV1(intentBody)
	if err != nil || parsedIntent != intent {
		t.Fatalf("intent canonical round trip failed: err=%v parsed=%#v", err, parsedIntent)
	}
	seed := sha256.Sum256([]byte("plugin-materialization-authority"))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	keyID := sha256Hex(publicKey)
	receipt, err := NewReceiptV1(intent, strings.Repeat("b", 64), "plugins/cache/analytix-hub/analytix-fund-analysis/0.16.16", now, keyID, publicKey, func(body []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, body), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.FactToolsEnabled {
		t.Fatal("materialization receipt enabled fact tools")
	}
	if err := ValidateTrustedReceiptV1(receipt, keyID, publicKey); err != nil {
		t.Fatalf("trusted receipt rejected: %v", err)
	}
	receiptBody, err := ReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if parsed, err := ParseReceiptV1(receiptBody); err != nil || parsed != receipt {
		t.Fatalf("receipt canonical round trip failed: err=%v parsed=%#v", err, parsed)
	}
	index, err := NewIndexV1(receipt, now)
	if err != nil {
		t.Fatal(err)
	}
	if index.DiscoverableGenerationCount != 1 || index.FactToolsEnabled {
		t.Fatalf("index widened discovery or fact authority: %#v", index)
	}
	indexBody, _ := IndexV1Bytes(index)
	if parsed, err := ParseIndexV1(indexBody); err != nil || parsed != index {
		t.Fatalf("index canonical round trip failed: err=%v parsed=%#v", err, parsed)
	}
	for sequence, phase := range []JournalPhaseV1{
		JournalPreparedV1, JournalStagedV1, JournalAuthorizedV1, JournalPriorQuarantinedV1,
		JournalGenerationCommittedV1, JournalIndexCommittedV1, JournalCompletedV1,
	} {
		record, err := NewJournalV1(intent, receipt.GenerationID, "plugins/cache/analytix-hub/analytix-fund-analysis/.staging-"+intent.IntentID, receipt.ActiveRelativePath, uint64(sequence+1), phase, now)
		if err != nil {
			t.Fatalf("journal phase %s: %v", phase, err)
		}
		body, _ := JournalV1Bytes(record)
		if parsed, err := ParseJournalV1(body); err != nil || parsed != record {
			t.Fatalf("journal phase %s canonical round trip failed: err=%v", phase, err)
		}
	}
}

func TestBundledPluginMaterializationReceiptRejectsSelfMintedJSONAuthority(t *testing.T) {
	now := time.Date(2026, 7, 23, 10, 11, 12, 0, time.UTC)
	intent := testIntent(t, now)
	trustedSeed := sha256.Sum256([]byte("trusted-installation-authority"))
	trustedPrivate := ed25519.NewKeyFromSeed(trustedSeed[:])
	trustedPublic := trustedPrivate.Public().(ed25519.PublicKey)
	attackerSeed := sha256.Sum256([]byte("self-authored-json-marker"))
	attackerPrivate := ed25519.NewKeyFromSeed(attackerSeed[:])
	attackerPublic := attackerPrivate.Public().(ed25519.PublicKey)
	forged, err := NewReceiptV1(intent, strings.Repeat("c", 64), "plugins/cache/analytix-hub/analytix-fund-analysis/0.16.16", now, sha256Hex(attackerPublic), attackerPublic, func(body []byte) ([]byte, error) {
		return ed25519.Sign(attackerPrivate, body), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateReceiptV1(forged); err != nil {
		t.Fatalf("self-consistent audit value should parse before trust anchoring: %v", err)
	}
	if err := ValidateTrustedReceiptV1(forged, sha256Hex(trustedPublic), trustedPublic); err == nil {
		t.Fatal("ordinary JSON plus an attacker key minted installation authority")
	}
	forged.FactToolsEnabled = true
	if err := ValidateReceiptV1(forged); err == nil {
		t.Fatal("receipt enabled fact tools")
	}
}

func TestBundledPluginMaterializationContractsRejectUnknownDuplicateAndNonCanonicalJSON(t *testing.T) {
	intent := testIntent(t, time.Date(2026, 7, 23, 10, 11, 12, 0, time.UTC))
	body, _ := IntentV1Bytes(intent)
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	object["unknown"] = true
	unknown, _ := json.Marshal(object)
	if _, err := ParseIntentV1(unknown); err == nil {
		t.Fatal("intent accepted an unknown field")
	}
	duplicate := strings.Replace(string(body), `"purpose":`, `"purpose":"duplicate","purpose":`, 1)
	if _, err := ParseIntentV1([]byte(duplicate)); err == nil {
		t.Fatal("intent accepted a duplicate field")
	}
	if _, err := ParseIntentV1(append([]byte(" "), body...)); err == nil {
		t.Fatal("intent accepted non-canonical leading whitespace")
	}
}

func TestBundledPluginMaterializationTargetContract(t *testing.T) {
	for _, target := range []TargetV1{{"darwin", "arm64"}, {"darwin", "amd64"}, {"windows", "amd64"}, {"windows", "arm64"}, {"linux", "amd64"}, {"linux", "arm64"}} {
		if err := ValidateTargetV1(target); err != nil {
			t.Fatalf("supported target %#v rejected: %v", target, err)
		}
	}
	for _, target := range []TargetV1{{"win32", "x64"}, {"darwin", "x64"}, {"windows", "386"}, {"", "arm64"}} {
		if err := ValidateTargetV1(target); err == nil {
			t.Fatalf("non-canonical target %#v accepted", target)
		}
	}
}

func TestBundledFundsReadyV1BindsSignedReceiptAndIndexWithoutFactAuthority(t *testing.T) {
	now := time.Date(2026, 7, 23, 18, 0, 0, 0, time.UTC)
	intent := testIntent(t, now)
	seed := sha256.Sum256([]byte("ready-installation-authority"))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	receipt, err := NewReceiptV1(
		intent, strings.Repeat("b", 64),
		"plugins/cache/analytix-hub/analytix-fund-analysis/0.16.16",
		now, sha256Hex(publicKey), publicKey,
		func(body []byte) ([]byte, error) { return ed25519.Sign(privateKey, body), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	index, err := NewIndexV1(receipt, now)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := NewReadyV1(ReadyInputV1{
		InvocationID: strings.Repeat("4", 64),
		PackageAuthority: PackageAuthorityBindingV1{
			FileSHA256: intent.PackageAuthoritySHA256, AuthorityDigest: strings.Repeat("5", 64),
			Classification:  "controlled_release_clean_candidate_non_publishable",
			DispositionKind: "controlled_release_receipt", PlatformAnchor: "macos_developer_id_resource_seal",
		},
		RuntimeIdentity: RuntimeIdentityV1{
			PayloadSHA256: strings.Repeat("6", 64), PayloadBytes: 4096, Format: "mach-o", Arch: "arm64",
		},
		ActivePluginRoot: "/tmp/analytix/plugins/cache/analytix-hub/analytix-fund-analysis/0.16.16",
		Receipt:          receipt, Index: index, CompletedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ready.Publishable || ready.FactToolsEnabled || ready.Receipt.FactToolsEnabled || ready.Index.FactToolsEnabled {
		t.Fatal("ready result widened publication or fact authority")
	}
	body, err := ReadyV1Bytes(ready)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseReadyV1(body)
	if err != nil || parsed.ConfigurationBindingDigest != ready.ConfigurationBindingDigest {
		t.Fatalf("ready result did not round trip: err=%v parsed=%#v", err, parsed)
	}

	ready.Receipt.SourceTreeSHA256 = strings.Repeat("7", 64)
	ready.ConfigurationBindingDigest = deriveReadyBindingDigestV1(ready)
	if ValidateReadyV1(ready) == nil {
		t.Fatal("ready result accepted a receipt/index mismatch")
	}
}

func testIntent(t *testing.T, now time.Time) IntentV1 {
	t.Helper()
	intent, err := NewIntentV1(IntentInputV1{
		PackageAuthoritySHA256: strings.Repeat("a", 64), Target: TargetV1{Platform: "darwin", Arch: "arm64"},
		PluginName: PluginNameV1, PluginVersion: fixturePluginVersionV1,
		SourceRoot:       "/Applications/Analytix.app/Contents/Resources/plugins/analytix-fund-analysis",
		SourceTreeSHA256: strings.Repeat("1", 64), SourceTreeFileCount: 221,
		ManifestSHA256: strings.Repeat("2", 64), EntrypointSHA256: strings.Repeat("3", 64), RequestedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return intent
}
