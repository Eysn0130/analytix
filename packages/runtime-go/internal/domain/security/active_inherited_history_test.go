package security

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func activeInheritedHistoryFixtureV1() ActiveInheritedHistoryBindingV1 {
	turns := []ActiveInheritedTurnV1{
		{TurnID: "turn-1", ContentSHA256: SHA256Hex([]byte("exact first turn"))},
		{TurnID: "turn-2", ContentSHA256: SHA256Hex([]byte("exact second turn"))},
	}
	return ActiveInheritedHistoryBindingV1{
		SchemaVersion: 1, Purpose: ActiveInheritedHistoryPurposeV1,
		SourceThreadID: "source-thread", SourcePrimarySHA256: SHA256Hex([]byte("immutable primary")),
		SourceAuthorityRecordDigest: SHA256Hex([]byte("committed source authority")),
		TargetThreadID:              "target-thread", Derivation: "fork", CutoffTurnID: "turn-2",
		SourceTurnCount: 3, TargetCreatedAt: "2026-09-09T01:02:03.456Z", TargetRelation: "fork",
		Turns: turns, InventoryDigest: ActiveInheritedHistoryInventoryDigestV1(turns),
	}
}

func activeInheritedHistorySigningKeyV1() (ed25519.PublicKey, ed25519.PrivateKey) {
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{41}, ed25519.SeedSize))
	return private.Public().(ed25519.PublicKey), private
}

func TestActiveInheritedHistorySignedLineageRoundTripV1(t *testing.T) {
	public, private := activeInheritedHistorySigningKeyV1()
	binding := activeInheritedHistoryFixtureV1()
	calls := 0
	record, err := NewCaseThreadLineageAuthorityRecordWithInheritedHistoryV1(
		binding.TargetThreadID, binding.SourceThreadID, binding.SourceAuthorityRecordDigest, binding.Derivation,
		binding, SHA256Hex(public), public, func(message []byte) ([]byte, error) {
			calls++
			return ed25519.Sign(private, message), nil
		},
	)
	if err != nil || calls != 1 {
		t.Fatalf("signed lineage failed: calls=%d err=%v", calls, err)
	}
	body, err := CaseThreadAuthorityRecordBytes(record)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCaseThreadAuthorityRecord(body)
	if err != nil || !reflect.DeepEqual(parsed, record) || CaseThreadAuthorityCanAuthorizeExecution(parsed) {
		t.Fatalf("inert lineage round trip changed authority: err=%v", err)
	}
	binding.Turns[0].ContentSHA256 = SHA256Hex([]byte("caller mutation"))
	if ValidateCaseThreadAuthorityRecord(record) != nil {
		t.Fatal("constructor retained caller-owned inventory")
	}
	cloned := CloneActiveInheritedHistoryBindingV1(record.ActiveInheritedHistory)
	cloned.Turns[0].TurnID = "another-turn"
	if reflect.DeepEqual(cloned, record.ActiveInheritedHistory) || ValidateCaseThreadAuthorityRecord(record) != nil {
		t.Fatal("clone retained signed inventory aliases")
	}
}

func TestActiveInheritedHistoryLegacyNilBytesAndSignatureV1(t *testing.T) {
	public, private := activeInheritedHistorySigningKeyV1()
	parentDigest := SHA256Hex([]byte("legacy parent"))
	keyID := SHA256Hex(public)
	publicText := base64.RawURLEncoding.EncodeToString(public)
	// This is the exact pre-extension lineage wire order, including the empty
	// signature/digest fields used by the established signing calculation.
	legacyBody := func(signature, digest string) []byte {
		return []byte(fmt.Sprintf(`{"schemaVersion":1,"authorityPurpose":"analytix.case-thread-authority/v1","authorityAlgorithm":"Ed25519","authorityKeyId":%q,"authorityPublicKey":%q,"threadId":"target-thread","parentThreadId":"source-thread","parentRecordDigest":%q,"derivation":"fork","authoritySignature":%q,"recordDigest":%q}`, keyID, publicText, parentDigest, signature, digest))
	}
	unsignedDigest := sha256.Sum256(legacyBody("", ""))
	expectedSigning := append([]byte("analytix.case-thread-authority/v1\x00"), unsignedDigest[:]...)
	expectedSignature := base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, expectedSigning))
	expectedDigest := SHA256Hex(legacyBody(expectedSignature, ""))
	calls := 0
	record, err := NewCaseThreadLineageAuthorityRecord("target-thread", "source-thread", parentDigest, "fork", keyID, public, func(message []byte) ([]byte, error) {
		calls++
		if !bytes.Equal(message, expectedSigning) {
			t.Fatal("legacy signing bytes changed")
		}
		return ed25519.Sign(private, message), nil
	})
	if err != nil || calls != 1 || record.ActiveInheritedHistory != nil || record.AuthoritySignature != expectedSignature || record.RecordDigest != expectedDigest {
		t.Fatalf("legacy lineage identity changed: calls=%d err=%v", calls, err)
	}
	body, err := CaseThreadAuthorityRecordBytes(record)
	if err != nil || !bytes.Equal(body, legacyBody(expectedSignature, expectedDigest)) {
		t.Fatalf("legacy lineage wire bytes changed: err=%v", err)
	}
	if _, err := ParseCaseThreadAuthorityRecord(body); err != nil {
		t.Fatal(err)
	}
}

func TestActiveInheritedHistoryRejectsInvalidBindingsV1(t *testing.T) {
	tests := map[string]func(*ActiveInheritedHistoryBindingV1){
		"version":      func(b *ActiveInheritedHistoryBindingV1) { b.SchemaVersion = 2 },
		"purpose":      func(b *ActiveInheritedHistoryBindingV1) { b.Purpose += " " },
		"source alias": func(b *ActiveInheritedHistoryBindingV1) { b.SourceThreadID += " " },
		"target alias": func(b *ActiveInheritedHistoryBindingV1) { b.TargetThreadID = "../target" },
		"same thread":  func(b *ActiveInheritedHistoryBindingV1) { b.TargetThreadID = b.SourceThreadID },
		"source digest": func(b *ActiveInheritedHistoryBindingV1) {
			b.SourcePrimarySHA256 = strings.ToUpper(b.SourcePrimarySHA256)
		},
		"authority digest": func(b *ActiveInheritedHistoryBindingV1) { b.SourceAuthorityRecordDigest = "" },
		"derivation":       func(b *ActiveInheritedHistoryBindingV1) { b.Derivation = "fork " },
		"relation":         func(b *ActiveInheritedHistoryBindingV1) { b.TargetRelation = "primary" },
		"cutoff":           func(b *ActiveInheritedHistoryBindingV1) { b.CutoffTurnID = "turn-1" },
		"count too small":  func(b *ActiveInheritedHistoryBindingV1) { b.SourceTurnCount = 1 },
		"negative count":   func(b *ActiveInheritedHistoryBindingV1) { b.SourceTurnCount = -1 },
		"resume prefix":    func(b *ActiveInheritedHistoryBindingV1) { b.Derivation, b.TargetRelation = "resume", "primary" },
		"time offset":      func(b *ActiveInheritedHistoryBindingV1) { b.TargetCreatedAt = "2026-09-09T01:02:03.456+00:00" },
		"time alias":       func(b *ActiveInheritedHistoryBindingV1) { b.TargetCreatedAt = "2026-09-09T01:02:03.4560Z" },
		"time zero":        func(b *ActiveInheritedHistoryBindingV1) { b.TargetCreatedAt = "0001-01-01T00:00:00Z" },
		"nil turns":        func(b *ActiveInheritedHistoryBindingV1) { b.Turns = nil },
		"duplicate turns": func(b *ActiveInheritedHistoryBindingV1) {
			b.Turns[0] = b.Turns[1]
			b.InventoryDigest = ActiveInheritedHistoryInventoryDigestV1(b.Turns)
		},
		"turn alias": func(b *ActiveInheritedHistoryBindingV1) {
			b.Turns[0].TurnID += " "
			b.InventoryDigest = ActiveInheritedHistoryInventoryDigestV1(b.Turns)
		},
		"content digest": func(b *ActiveInheritedHistoryBindingV1) {
			b.Turns[0].ContentSHA256 = "bad"
			b.InventoryDigest = ActiveInheritedHistoryInventoryDigestV1(b.Turns)
		},
		"reordered inventory":      func(b *ActiveInheritedHistoryBindingV1) { b.Turns[0], b.Turns[1] = b.Turns[1], b.Turns[0] },
		"missing inventory digest": func(b *ActiveInheritedHistoryBindingV1) { b.InventoryDigest = "" },
		"empty cutoff mismatch": func(b *ActiveInheritedHistoryBindingV1) {
			b.SourceTurnCount, b.Turns = 0, []ActiveInheritedTurnV1{}
			b.InventoryDigest = ActiveInheritedHistoryInventoryDigestV1(b.Turns)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			binding := activeInheritedHistoryFixtureV1()
			mutate(&binding)
			if ValidateActiveInheritedHistoryBindingV1(binding) == nil {
				t.Fatal("invalid inherited history binding was accepted")
			}
		})
	}
	for _, derivation := range []string{"fork", "resume"} {
		binding := activeInheritedHistoryFixtureV1()
		binding.Derivation = derivation
		binding.SourceTurnCount = len(binding.Turns)
		if derivation == "resume" {
			binding.TargetRelation = "primary"
		} else {
			binding.TargetRelation = "side"
		}
		if err := ValidateActiveInheritedHistoryBindingV1(binding); err != nil {
			t.Fatal(err)
		}
		binding.Turns, binding.SourceTurnCount, binding.CutoffTurnID = []ActiveInheritedTurnV1{}, 0, ""
		binding.InventoryDigest = ActiveInheritedHistoryInventoryDigestV1(binding.Turns)
		if err := ValidateActiveInheritedHistoryBindingV1(binding); err != nil || CloneActiveInheritedHistoryBindingV1(&binding).Turns == nil {
			t.Fatalf("valid empty source lost its explicit inventory: %v", err)
		}
	}
	if CloneActiveInheritedHistoryBindingV1(nil) != nil || ActiveInheritedHistoryInventoryDigestV1(nil) != "" {
		t.Fatal("absent binding or inventory acquired authority")
	}
}

func TestActiveInheritedHistoryRejectsCrossBindingAndTamperingV1(t *testing.T) {
	public, private := activeInheritedHistorySigningKeyV1()
	sign := func(message []byte) ([]byte, error) { return ed25519.Sign(private, message), nil }
	binding := activeInheritedHistoryFixtureV1()
	record, err := NewCaseThreadLineageAuthorityRecordWithInheritedHistoryV1(binding.TargetThreadID, binding.SourceThreadID, binding.SourceAuthorityRecordDigest, binding.Derivation, binding, SHA256Hex(public), public, sign)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*CaseThreadAuthorityRecord){
		"source":           func(r *CaseThreadAuthorityRecord) { r.ParentThreadID = "another-source" },
		"target":           func(r *CaseThreadAuthorityRecord) { r.ThreadID = "another-target" },
		"parent authority": func(r *CaseThreadAuthorityRecord) { r.ParentRecordDigest = SHA256Hex([]byte("another parent")) },
		"relation":         func(r *CaseThreadAuthorityRecord) { r.Derivation = "resume" },
		"content": func(r *CaseThreadAuthorityRecord) {
			r.ActiveInheritedHistory.Turns[0].ContentSHA256 = SHA256Hex([]byte("substituted content"))
			r.ActiveInheritedHistory.InventoryDigest = ActiveInheritedHistoryInventoryDigestV1(r.ActiveInheritedHistory.Turns)
		},
		"source state": func(r *CaseThreadAuthorityRecord) {
			r.ActiveInheritedHistory.SourcePrimarySHA256 = SHA256Hex([]byte("substituted primary"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			tampered := record
			tampered.ActiveInheritedHistory = CloneActiveInheritedHistoryBindingV1(record.ActiveInheritedHistory)
			mutate(&tampered)
			if ValidateCaseThreadAuthorityRecord(tampered) == nil {
				t.Fatal("signature did not bind inherited history")
			}
		})
	}
	calls := 0
	_, err = NewCaseThreadLineageAuthorityRecordWithInheritedHistoryV1(binding.TargetThreadID, binding.SourceThreadID, SHA256Hex([]byte("wrong parent")), binding.Derivation, binding, SHA256Hex(public), public, func(message []byte) ([]byte, error) {
		calls++
		return sign(message)
	})
	if err == nil || calls != 0 {
		t.Fatal("cross-parent binding reached signing")
	}
	securityContext := mustCaseTurnSecurityContextV2(t, "context-thread", "context-turn", "/cases/a", "case-a", 3, time.Unix(3, 0))
	contextRecord, err := NewCaseThreadAuthorityRecord(securityContext, SHA256Hex(public), public, sign)
	if err != nil {
		t.Fatal(err)
	}
	contextRecord.ActiveInheritedHistory = &binding
	if _, err := signCaseThreadAuthorityRecord(contextRecord, sign); err == nil {
		t.Fatal("context record acquired inherited history authority")
	}
	body, err := CaseThreadAuthorityRecordBytes(record)
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string][]byte{
		"outer duplicate": append(append([]byte(nil), body[:len(body)-1]...), []byte(`,"schemaVersion":1}`)...),
		"inner duplicate": bytes.Replace(body, []byte(`"sourceThreadId":"source-thread"`), []byte(`"sourceThreadId":"source-thread","sourceThreadId":"source-thread"`), 1),
		"inner unknown":   bytes.Replace(body, []byte(`"purpose":"analytix.active-inherited-history/v1"`), []byte(`"purpose":"analytix.active-inherited-history/v1","unexpected":true`), 1),
	} {
		t.Run(name, func(t *testing.T) {
			if !json.Valid(raw) {
				t.Fatal("fixture is not valid JSON")
			}
			if _, err := ParseCaseThreadAuthorityRecord(raw); err == nil {
				t.Fatal("ambiguous or unknown binding field was accepted")
			}
		})
	}
}
