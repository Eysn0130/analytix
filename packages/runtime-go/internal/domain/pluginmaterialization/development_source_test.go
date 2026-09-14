package pluginmaterialization

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDevelopmentSourceOriginIsSignedAndCannotBecomeFormalV1(t *testing.T) {
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	input := IntentInputV1{Origin: DevelopmentSourceOriginV1, SourceRegistrationSHA256: strings.Repeat("a", 64), Target: TargetV1{Platform: "darwin", Arch: "arm64"}, PluginName: "analytix-documents", PluginVersion: "1.0.0", SourceRoot: "/source/documents", SourceTreeSHA256: strings.Repeat("b", 64), SourceTreeFileCount: 4, ManifestSHA256: strings.Repeat("c", 64), RequestedAt: now}
	intent, err := NewIntentV1(input)
	if err != nil {
		t.Fatal(err)
	}
	seed := sha256.Sum256([]byte("development-source-test"))
	private := ed25519.NewKeyFromSeed(seed[:])
	public := private.Public().(ed25519.PublicKey)
	receipt, err := NewReceiptV1(intent, strings.Repeat("d", 64), "plugins/cache/analytix-hub/analytix-documents/1.0.0", now, sha256Hex(public), public, func(b []byte) ([]byte, error) { return ed25519.Sign(private, b), nil })
	if err != nil {
		t.Fatal(err)
	}
	body, _ := ReceiptV1Bytes(receipt)
	if bytes.Contains(body, []byte(`"packageAuthoritySha256"`)) || !bytes.Contains(body, []byte(`"origin":"development-source"`)) {
		t.Fatal("source receipt masquerades as packaged authority")
	}
	parsed, err := ParseReceiptV1(body)
	if err != nil || parsed != receipt {
		t.Fatal("source receipt round trip failed", err)
	}
	index, err := NewIndexV1(receipt, now)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := NewJournalV1(intent, receipt.GenerationID, "plugins/.staging-test", receipt.ActiveRelativePath, 1, JournalPreparedV1, now)
	if err != nil {
		t.Fatal(err)
	}
	if index.SourceRegistrationSHA256 != intent.SourceRegistrationSHA256 || journal.SourceRegistrationSHA256 != intent.SourceRegistrationSHA256 {
		t.Fatal("origin lost in persisted projections")
	}
	changed := receipt
	changed.SourceRegistrationSHA256 = strings.Repeat("e", 64)
	changed.ReceiptID = deriveReceiptID(changed)
	if ValidateReceiptV1(changed) == nil {
		t.Fatal("changing source binding preserved signature")
	}
	changed = receipt
	changed.Origin = ""
	changed.PackageAuthoritySHA256 = changed.SourceRegistrationSHA256
	changed.SourceRegistrationSHA256 = ""
	changed.EntrypointSHA256 = strings.Repeat("e", 64)
	changed.ReceiptID = deriveReceiptID(changed)
	if ValidateReceiptV1(changed) == nil {
		t.Fatal("source receipt relabeled formal retained signature")
	}
	for _, mutate := range []func(*IntentInputV1){func(i *IntentInputV1) { i.PackageAuthoritySHA256 = strings.Repeat("e", 64) }, func(i *IntentInputV1) { i.Origin = "" }, func(i *IntentInputV1) { i.PluginName = "analytix-fund-analysis" }, func(i *IntentInputV1) { i.EntrypointSHA256 = strings.Repeat("e", 64) }} {
		bad := input
		mutate(&bad)
		if _, err := NewIntentV1(bad); err == nil {
			t.Fatal("origin ambiguity accepted")
		}
	}
	activation, err := NewActivationV1(receipt, 1, DesiredDisabledV1, now, sha256Hex(public), public, func(b []byte) ([]byte, error) { return ed25519.Sign(private, b), nil })
	if err != nil {
		t.Fatal(err)
	}
	if ValidateTrustedActivationForReceiptV1(activation, receipt, sha256Hex(public), public) != nil {
		t.Fatal("activation rejected source receipt")
	}
}

// Frozen pre-origin layouts keep field order and signature input independently
// of the extended structs. New fields must never alter legacy V1 bytes.
func TestDevelopmentSourceExtensionPreservesLegacyCanonicalBytesV1(t *testing.T) {
	now := time.Date(2026, 7, 23, 10, 11, 12, 123, time.UTC)
	intent := testIntent(t, now)
	seed := sha256.Sum256([]byte("plugin-materialization-authority"))
	private := ed25519.NewKeyFromSeed(seed[:])
	public := private.Public().(ed25519.PublicKey)
	receipt, err := NewReceiptV1(intent, strings.Repeat("b", 64), "plugins/cache/analytix-hub/analytix-fund-analysis/0.16.16", now, sha256Hex(public), public, func(b []byte) ([]byte, error) { return ed25519.Sign(private, b), nil })
	if err != nil {
		t.Fatal(err)
	}
	index, err := NewIndexV1(receipt, now)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := NewJournalV1(intent, receipt.GenerationID, "plugins/.staging-legacy", receipt.ActiveRelativePath, 1, JournalPreparedV1, now)
	if err != nil {
		t.Fatal(err)
	}
	pairs := []struct {
		current any
		legacy  any
	}{{intent, &legacyIntentV1{}}, {receipt, &legacyReceiptV1{}}, {index, &legacyIndexV1{}}, {journal, &legacyJournalV1{}}}
	for _, pair := range pairs {
		body, _ := json.Marshal(pair.current)
		if err := json.Unmarshal(body, pair.legacy); err != nil {
			t.Fatal(err)
		}
		old, _ := json.Marshal(pair.legacy)
		if !bytes.Equal(body, old) {
			t.Fatalf("legacy bytes changed for %T", pair.current)
		}
	}
	old := legacyReceiptV1{}
	body, _ := json.Marshal(receipt)
	_ = json.Unmarshal(body, &old)
	old.AuthoritySignature = ""
	oldBody, _ := json.Marshal(old)
	digest := sha256.Sum256(oldBody)
	expected := append([]byte("analytix.bundled-plugin-materialization-receipt/signature/v1\x00"), digest[:]...)
	if !bytes.Equal(ReceiptSigningBytesV1(receipt), expected) {
		t.Fatal("legacy receipt signature input changed")
	}
}

type legacyIntentV1 struct {
	SchemaVersion          int      `json:"schemaVersion"`
	Purpose                string   `json:"purpose"`
	IntentID               string   `json:"intentId"`
	PackageAuthoritySHA256 string   `json:"packageAuthoritySha256"`
	Target                 TargetV1 `json:"target"`
	PluginName             string   `json:"pluginName"`
	PluginVersion          string   `json:"pluginVersion"`
	SourceRoot             string   `json:"sourceRoot"`
	SourceTreeSHA256       string   `json:"sourceTreeSha256"`
	SourceTreeFileCount    uint64   `json:"sourceTreeFileCount"`
	ManifestSHA256         string   `json:"manifestSha256"`
	EntrypointSHA256       string   `json:"entrypointSha256"`
	RequestedAt            string   `json:"requestedAt"`
}

type legacyReceiptV1 struct {
	SchemaVersion          int      `json:"schemaVersion"`
	Purpose                string   `json:"purpose"`
	ReceiptID              string   `json:"receiptId"`
	IntentID               string   `json:"intentId"`
	PackageAuthoritySHA256 string   `json:"packageAuthoritySha256"`
	Target                 TargetV1 `json:"target"`
	PluginName             string   `json:"pluginName"`
	PluginVersion          string   `json:"pluginVersion"`
	GenerationID           string   `json:"generationId"`
	ActiveRelativePath     string   `json:"activeRelativePath"`
	SourceTreeSHA256       string   `json:"sourceTreeSha256"`
	SourceTreeFileCount    uint64   `json:"sourceTreeFileCount"`
	ManifestSHA256         string   `json:"manifestSha256"`
	EntrypointSHA256       string   `json:"entrypointSha256"`
	FactToolsEnabled       bool     `json:"factToolsEnabled"`
	IssuedAt               string   `json:"issuedAt"`
	AuthorityAlgorithm     string   `json:"authorityAlgorithm"`
	AuthorityKeyID         string   `json:"authorityKeyId"`
	AuthorityPublicKey     string   `json:"authorityPublicKey"`
	AuthoritySignature     string   `json:"authoritySignature"`
}

type legacyIndexV1 struct {
	SchemaVersion               int    `json:"schemaVersion"`
	Purpose                     string `json:"purpose"`
	IndexDigest                 string `json:"indexDigest"`
	PluginName                  string `json:"pluginName"`
	PluginVersion               string `json:"pluginVersion"`
	GenerationID                string `json:"generationId"`
	ActiveRelativePath          string `json:"activeRelativePath"`
	ReceiptID                   string `json:"receiptId"`
	ReceiptSHA256               string `json:"receiptSha256"`
	IntentID                    string `json:"intentId"`
	SourceTreeSHA256            string `json:"sourceTreeSha256"`
	SourceTreeFileCount         uint64 `json:"sourceTreeFileCount"`
	DiscoverableGenerationCount int    `json:"discoverableGenerationCount"`
	FactToolsEnabled            bool   `json:"factToolsEnabled"`
	CommittedAt                 string `json:"committedAt"`
}

type legacyJournalV1 struct {
	SchemaVersion       int            `json:"schemaVersion"`
	Purpose             string         `json:"purpose"`
	RecordDigest        string         `json:"recordDigest"`
	TransactionID       string         `json:"transactionId"`
	Sequence            uint64         `json:"sequence"`
	Phase               JournalPhaseV1 `json:"phase"`
	IntentID            string         `json:"intentId"`
	GenerationID        string         `json:"generationId"`
	StagingRelativePath string         `json:"stagingRelativePath"`
	ActiveRelativePath  string         `json:"activeRelativePath"`
	RecordedAt          string         `json:"recordedAt"`
}
