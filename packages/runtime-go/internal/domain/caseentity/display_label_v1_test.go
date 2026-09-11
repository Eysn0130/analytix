package caseentity

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
)

func TestDisplayLabelV1RendersCanonicalNaturalNonCollidingLabels(t *testing.T) {
	first, err := NewDisplayLabelV1(DisplayLabelInputV1{
		EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		StableOrdinal: 7,
		SafeSuffix:    "1234",
		Institution:   "  中国  银行 ",
		AccountType:   "储蓄-账户",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "第7个银行账户（中国 银行；储蓄-账户；尾号1234）"
	if first.Text != want || first.Institution != "中国 银行" {
		t.Fatalf("canonical display label mismatch: %#v", first)
	}
	if rendered, err := RenderDisplayLabelV1(first); err != nil || rendered != want {
		t.Fatalf("validated display rendering failed: text=%q err=%v", rendered, err)
	}

	second, err := NewDisplayLabelV1(DisplayLabelInputV1{
		EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		StableOrdinal: 8,
		SafeSuffix:    "1234",
		Institution:   "中国 银行",
		AccountType:   "储蓄-账户",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Text == second.Text {
		t.Fatalf("two stable ordinals with the same suffix collided: %q", first.Text)
	}
	if !strings.Contains(first.Text, "第7个") || !strings.Contains(second.Text, "第8个") {
		t.Fatalf("stable ordinal is not visibly collision-resistant: first=%q second=%q", first.Text, second.Text)
	}
}

func TestDisplayLabelV1SupportsClosedAccountAndCardVocabulary(t *testing.T) {
	account, err := NewDisplayLabelV1(DisplayLabelInputV1{
		EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		StableOrdinal: 1,
	})
	if err != nil || account.Text != "第1个银行账户" {
		t.Fatalf("account display label = %#v err=%v", account, err)
	}
	card, err := NewDisplayLabelV1(DisplayLabelInputV1{
		EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1,
		StableOrdinal: 2,
		Institution:   "Bank A",
		AccountType:   "借记卡",
	})
	if err != nil || card.Text != "第2个银行卡（Bank A；借记卡）" {
		t.Fatalf("card display label = %#v err=%v", card, err)
	}

	for _, label := range []DisplayLabelV1{account, card} {
		if ValidateDisplayLabelV1(label) != nil || ContainsReferenceV1(label.Text) ||
			strings.Contains(label.Text, ReferencePrefixV1) {
			t.Fatalf("valid label retained an internal reference or failed validation: %#v", label)
		}
	}
}

func TestDisplayLabelV1RejectsPIIAndNonCanonicalInputs(t *testing.T) {
	reference := mustPublicValueReferenceV1(t, "d")
	fullAccount := "6222021234567890123"
	tests := map[string]DisplayLabelInputV1{
		"unknown entity": {
			EntityType: "person", StableOrdinal: 1,
		},
		"zero ordinal": {
			EntityType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		},
		"short suffix": {
			EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			StableOrdinal: 1, SafeSuffix: "123",
		},
		"unicode suffix": {
			EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			StableOrdinal: 1, SafeSuffix: "１２３４",
		},
		"complete identifier as suffix": {
			EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			StableOrdinal: 1, SafeSuffix: fullAccount,
		},
		"complete identifier as institution": {
			EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			StableOrdinal: 1, Institution: "银行" + fullAccount,
		},
		"split identifier as account type": {
			EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			StableOrdinal: 1, AccountType: "6222 A 0212 B 3456",
		},
		"internal reference": {
			EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			StableOrdinal: 1, Institution: reference,
		},
		"internal prefix": {
			EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			StableOrdinal: 1, Institution: ReferencePrefixV1 + "documentation",
		},
		"control character": {
			EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			StableOrdinal: 1, Institution: "中国\n银行",
		},
		"render delimiter": {
			EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			StableOrdinal: 1, Institution: "银行；尾号0000",
		},
		"oversized institution": {
			EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			StableOrdinal: 1, Institution: strings.Repeat("银", maxDisplayLabelInstitutionBytes),
		},
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			label, err := NewDisplayLabelV1(input)
			if !errors.Is(err, ErrDisplayLabelInvalidV1) || label != (DisplayLabelV1{}) {
				t.Fatalf("invalid display input was accepted: label=%#v err=%v", label, err)
			}
			if err != nil && (strings.Contains(err.Error(), fullAccount) || strings.Contains(err.Error(), reference)) {
				t.Fatalf("display validation reflected private input: %v", err)
			}
		})
	}
}

func TestDisplayLabelV1DoesNotImposeACaseEntityCountLimit(t *testing.T) {
	label, err := NewDisplayLabelV1(DisplayLabelInputV1{
		EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		StableOrdinal: ^uint32(0),
	})
	if err != nil || label.Text != "第4294967295个银行账户" {
		t.Fatalf("large case-private ordinal was rejected: label=%#v err=%v", label, err)
	}
}

func TestDisplayLabelV1ValidationClosesCanonicalRendering(t *testing.T) {
	label, err := NewDisplayLabelV1(DisplayLabelInputV1{
		EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1,
		StableOrdinal: 3,
		SafeSuffix:    "5678",
		Institution:   "Cafe\u0301 Bank",
		AccountType:   "信用卡",
	})
	if err != nil {
		t.Fatal(err)
	}
	if label.Institution != "Café Bank" {
		t.Fatalf("semantic field was not NFC-canonicalized: %q", label.Institution)
	}

	mutations := map[string]func(DisplayLabelV1) DisplayLabelV1{
		"forged text": func(value DisplayLabelV1) DisplayLabelV1 {
			value.Text += "（已授权）"
			return value
		},
		"non-canonical semantic field": func(value DisplayLabelV1) DisplayLabelV1 {
			value.Institution = "Café  Bank"
			return value
		},
		"changed suffix without rendering": func(value DisplayLabelV1) DisplayLabelV1 {
			value.SafeSuffix = "0000"
			return value
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			mutated := mutate(label)
			if err := ValidateDisplayLabelV1(mutated); !errors.Is(err, ErrDisplayLabelInvalidV1) {
				t.Fatalf("mutated value validated: %#v err=%v", mutated, err)
			}
			if rendered, err := RenderDisplayLabelV1(mutated); !errors.Is(err, ErrDisplayLabelInvalidV1) || rendered != "" {
				t.Fatalf("mutated value rendered: text=%q err=%v", rendered, err)
			}
		})
	}

	body, err := json.Marshal(label)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{ReferencePrefixV1, "6222021234567890123"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("display value exposed forbidden content %q: %s", forbidden, body)
		}
	}
}
