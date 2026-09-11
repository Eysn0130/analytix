package pendingwork

import (
	"bytes"
	"crypto/ed25519"
	"fmt"
	"reflect"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func legacyChildProducerReceiptFixture(t *testing.T) (ReceiptInputV1, ed25519.PrivateKey) {
	t.Helper()
	input, _ := receiptFixture(t, KindSideEffectIntent)
	input.GrantMembers = input.GrantMembers[:1]
	var seed [ed25519.SeedSize]byte
	copy(seed[:], "synthetic-r129-legacy-receipt")
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	input.AuthorityPublicKey = privateKey.Public().(ed25519.PublicKey)
	input.AuthorityKeyID = domainsecurity.SHA256Hex(input.AuthorityPublicKey)
	return input, privateKey
}

// Fixed from the pre-extension producer in R129 step57. Optional-field changes
// must not alter historical canonical, signing, receipt, or signature bytes.
func TestLegacyChildProducerReceiptFixedBytes(t *testing.T) {
	input, privateKey := legacyChildProducerReceiptFixture(t)
	receipt := mustReceipt(t, input, privateKey)
	body, err := PendingWorkReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePendingWorkReceiptV1(body)
	if err != nil || parsed.ReceiptID != receipt.ReceiptID {
		t.Fatal("legacy receipt did not retain its exact signed identity")
	}
	if domainsecurity.SHA256Hex(body) != "821a80c98207c779be2a6225e6d0ffca5b1897d4e86731bc44bfc34c77bd0a64" ||
		domainsecurity.SHA256Hex(PendingWorkReceiptV1SigningBytes(receipt)) != "5fb29ddf749fea4db42e37da3d505bfdbc023a18a370f75bf0d39a77560ec99f" ||
		receipt.ReceiptID != "81a83c4740bca81932fdc5e24667af260f1046ad194a3472439221bb5b202165" ||
		receipt.AuthoritySignature != "1QqdTu4Y-DebRVsSjjDprmbbt76GCTqf1HYJ3B686bOEz7qXcLJLIYPBoOVMn3fOkxWrOhIao3JXIOfqJ2CTDQ" {
		t.Fatal("optional child producer extension changed fixed legacy authority bytes")
	}
	if strings.Contains(string(body), "childProducer") {
		t.Fatal("absent child allocation was backfilled into a legacy receipt")
	}
}

func childProducerFixture() *ChildProducerV1 {
	return &ChildProducerV1{
		ParentBindingDigest: domainsecurity.SHA256Hex([]byte("synthetic parent security binding")),
		Children: []ChildProducerTargetV1{
			{Ordinal: 1, JobID: "job-11", ChildThreadID: "thr_durable_21", ChildTurnID: "turn_31"},
			{Ordinal: 2, JobID: "job-12", ChildThreadID: "thr_durable_fork_22", ChildTurnID: "turn_32"},
		},
	}
}

func TestChildProducerReceiptSignsCompleteImmutableAllocation(t *testing.T) {
	input, key := legacyChildProducerReceiptFixture(t)
	input.ChildProducer = childProducerFixture()
	receipt := mustReceipt(t, input, key)
	want := CloneChildProducerV1(receipt.ChildProducer)
	input.ChildProducer.Children[0].JobID = "job-99"
	input.ChildProducer.ParentBindingDigest = domainsecurity.SHA256Hex([]byte("changed parent"))
	if !reflect.DeepEqual(receipt.ChildProducer, want) || ValidatePendingWorkReceiptV1(receipt) != nil {
		t.Fatal("caller-owned allocation mutated the signed receipt")
	}
	body, err := PendingWorkReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	read, err := ParsePendingWorkReceiptV1(body)
	if err != nil || !reflect.DeepEqual(read, receipt) {
		t.Fatal("complete signed allocation changed across exact serialization/readback")
	}
	for name, mutate := range map[string]func(*ChildProducerV1){
		"parent":         func(p *ChildProducerV1) { p.ParentBindingDigest = domainsecurity.SHA256Hex([]byte("other parent")) },
		"job":            func(p *ChildProducerV1) { p.Children[0].JobID = "job-99" },
		"thread":         func(p *ChildProducerV1) { p.Children[0].ChildThreadID = "thr_durable_99" },
		"turn":           func(p *ChildProducerV1) { p.Children[0].ChildTurnID = "turn_99" },
		"ordinal":        func(p *ChildProducerV1) { p.Children[0].Ordinal = 2 },
		"order":          func(p *ChildProducerV1) { p.Children[0], p.Children[1] = p.Children[1], p.Children[0] },
		"omitted member": func(p *ChildProducerV1) { p.Children = p.Children[:1] },
	} {
		t.Run(name, func(t *testing.T) {
			changed := receipt
			changed.ChildProducer = CloneChildProducerV1(receipt.ChildProducer)
			mutate(changed.ChildProducer)
			if ValidatePendingWorkReceiptV1(changed) == nil {
				t.Fatal("changed child association retained its signed receipt authority")
			}
		})
	}
}

func TestChildProducerAllocationDoesNotRemintSemanticWorkID(t *testing.T) {
	input, key := legacyChildProducerReceiptFixture(t)
	legacy := mustReceipt(t, input, key)
	input.ChildProducer = childProducerFixture()
	first := mustReceipt(t, input, key)
	input.ChildProducer.Children[0], input.ChildProducer.Children[1] = input.ChildProducer.Children[1], input.ChildProducer.Children[0]
	input.ChildProducer.Children[0].Ordinal, input.ChildProducer.Children[1].Ordinal = 1, 2
	second := mustReceipt(t, input, key)
	input.ChildProducer.ParentBindingDigest = domainsecurity.SHA256Hex([]byte("other exact parent binding"))
	third := mustReceipt(t, input, key)
	for _, receipt := range []PendingWorkReceiptV1{first, second, third} {
		if receipt.WorkID != legacy.WorkID || receipt.PayloadHash != legacy.PayloadHash || receipt.ReceiptID == legacy.ReceiptID {
			t.Fatal("child allocation changed semantic work identity or escaped receipt identity")
		}
	}
	if first.ReceiptID == second.ReceiptID || second.ReceiptID == third.ReceiptID {
		t.Fatal("child allocation order or parent binding is absent from receipt identity")
	}
}

func TestChildProducerMalformedAllocationRejectsBeforeSigner(t *testing.T) {
	for name, mutate := range map[string]func(*ChildProducerV1){
		"empty":          func(p *ChildProducerV1) { p.Children = nil },
		"duplicate job":  func(p *ChildProducerV1) { p.Children[1].JobID = p.Children[0].JobID },
		"duplicate turn": func(p *ChildProducerV1) { p.Children[1].ChildTurnID = p.Children[0].ChildTurnID },
		"ordinal gap":    func(p *ChildProducerV1) { p.Children[1].Ordinal = 3 },
		"alias":          func(p *ChildProducerV1) { p.Children[0].JobID = "job-01" },
		"overflow":       func(p *ChildProducerV1) { p.Children[0].ChildTurnID = "turn_9223372036854775808" },
		"path alias":     func(p *ChildProducerV1) { p.Children[0].ChildThreadID = "../thr_durable_1" },
		"oversized": func(p *ChildProducerV1) {
			p.Children = make([]ChildProducerTargetV1, 5000)
			for i := range p.Children {
				p.Children[i] = ChildProducerTargetV1{uint32(i + 1), fmt.Sprintf("job-%d", i+1), "thr_durable_21", fmt.Sprintf("turn_%d", i+1)}
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			input, key := legacyChildProducerReceiptFixture(t)
			input.ChildProducer = childProducerFixture()
			mutate(input.ChildProducer)
			signs := 0
			_, err := NewPendingWorkReceiptV1(input, func(b []byte) ([]byte, error) { signs++; return ed25519.Sign(key, b), nil })
			if err == nil || signs != 0 {
				t.Fatalf("invalid child allocation crossed signer: err=%v signs=%d", err, signs)
			}
		})
	}
}

func TestChildProducerDowngradeDecoderFailsBeforeRecordUse(t *testing.T) {
	input, key := legacyChildProducerReceiptFixture(t)
	legacy := mustReceipt(t, input, key)
	legacyBytes, _ := PendingWorkReceiptV1Bytes(legacy)
	var oldRecord legacyPendingWorkReceiptV1
	if err := decodeStrict(legacyBytes, &oldRecord); err != nil {
		t.Fatal(err)
	}
	input.ChildProducer = childProducerFixture()
	current := mustReceipt(t, input, key)
	currentBytes, _ := PendingWorkReceiptV1Bytes(current)
	before := append([]byte(nil), currentBytes...)
	if err := decodeStrict(currentBytes, &oldRecord); err == nil || !strings.Contains(err.Error(), `unknown field "childProducer"`) {
		t.Fatalf("legacy DisallowUnknownFields rule did not reject the private increment: %v", err)
	}
	if !bytes.Equal(before, currentBytes) {
		t.Fatal("downgrade refusal changed the candidate input bytes")
	}
	if _, err := ParsePendingWorkReceiptV1(currentBytes); err != nil {
		t.Fatalf("new reader rejected the exact signed allocation: %v", err)
	}
}

// Frozen pre-extension private receipt schema for the downgrade decoder test.
type legacyPendingWorkReceiptV1 struct {
	SchemaVersion         int              `json:"schemaVersion"`
	Purpose               string           `json:"purpose"`
	WorkID                string           `json:"workId"`
	ReceiptID             string           `json:"receiptId"`
	Kind                  string           `json:"kind"`
	Context               ContextBindingV1 `json:"context"`
	GrantRegistrySequence uint64           `json:"grantRegistrySequence"`
	GrantRegistryDigest   string           `json:"grantRegistryDigest"`
	GrantMembers          []GrantMemberV1  `json:"grantMembers"`
	PayloadHash           string           `json:"payloadHash"`
	RouteHash             string           `json:"routeHash"`
	IssuedAt              string           `json:"issuedAt"`
	ExpiresAt             string           `json:"expiresAt"`
	AuthorityAlgorithm    string           `json:"authorityAlgorithm"`
	AuthorityKeyID        string           `json:"authorityKeyId"`
	AuthorityPublicKey    string           `json:"authorityPublicKey"`
	AuthoritySignature    string           `json:"authoritySignature"`
}
