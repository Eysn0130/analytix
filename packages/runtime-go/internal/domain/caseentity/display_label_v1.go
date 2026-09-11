package caseentity

import (
	"errors"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	"golang.org/x/text/unicode/norm"
)

const (
	maxDisplayLabelInstitutionBytes  = 128
	maxDisplayLabelAccountTypeBytes  = 64
	maxDisplayLabelSemanticDigitsV1  = 4
	maxDisplayLabelRenderedTextBytes = 384
)

var ErrDisplayLabelInvalidV1 = errors.New("case entity display label is invalid")

// DisplayLabelV1 is a canonical, user-facing identity label. StableOrdinal is
// assigned from case-private state and is always rendered, so two entities do
// not collapse merely because their safe suffix or semantic attributes match.
// ReferenceV1 and source-exact identifiers are intentionally absent.
type DisplayLabelV1 struct {
	EntityType    string `json:"entityType"`
	StableOrdinal uint32 `json:"stableOrdinal"`
	SafeSuffix    string `json:"safeSuffix,omitempty"`
	Institution   string `json:"institution,omitempty"`
	AccountType   string `json:"accountType,omitempty"`
	Text          string `json:"text"`
}

// DisplayLabelInputV1 contains only the closed inputs needed to derive a
// natural ordinary label. In particular it accepts neither an internal
// reference nor a source-exact account/card value.
type DisplayLabelInputV1 struct {
	EntityType    string
	StableOrdinal uint32
	SafeSuffix    string
	Institution   string
	AccountType   string
}

func NewDisplayLabelV1(input DisplayLabelInputV1) (DisplayLabelV1, error) {
	institution, err := canonicalDisplayLabelSemanticV1(input.Institution, maxDisplayLabelInstitutionBytes)
	if err != nil {
		return DisplayLabelV1{}, ErrDisplayLabelInvalidV1
	}
	accountType, err := canonicalDisplayLabelSemanticV1(input.AccountType, maxDisplayLabelAccountTypeBytes)
	if err != nil {
		return DisplayLabelV1{}, ErrDisplayLabelInvalidV1
	}
	label := DisplayLabelV1{
		EntityType:    input.EntityType,
		StableOrdinal: input.StableOrdinal,
		SafeSuffix:    input.SafeSuffix,
		Institution:   institution,
		AccountType:   accountType,
	}
	label.Text, err = renderDisplayLabelV1(label)
	if err != nil || ValidateDisplayLabelV1(label) != nil {
		return DisplayLabelV1{}, ErrDisplayLabelInvalidV1
	}
	return label, nil
}

func ValidateDisplayLabelV1(label DisplayLabelV1) error {
	if !IsFinancialEntityTypeV1(label.EntityType) ||
		label.StableOrdinal == 0 ||
		!validDisplayLabelSuffixV1(label.SafeSuffix) {
		return ErrDisplayLabelInvalidV1
	}
	institution, err := canonicalDisplayLabelSemanticV1(label.Institution, maxDisplayLabelInstitutionBytes)
	if err != nil || institution != label.Institution {
		return ErrDisplayLabelInvalidV1
	}
	accountType, err := canonicalDisplayLabelSemanticV1(label.AccountType, maxDisplayLabelAccountTypeBytes)
	if err != nil || accountType != label.AccountType {
		return ErrDisplayLabelInvalidV1
	}
	expected, err := renderDisplayLabelV1(label)
	if err != nil || label.Text != expected || label.Text == "" ||
		len(label.Text) > maxDisplayLabelRenderedTextBytes ||
		!utf8.ValidString(label.Text) || ContainsReferenceV1(label.Text) ||
		strings.Contains(label.Text, ReferencePrefixV1) {
		return ErrDisplayLabelInvalidV1
	}
	return nil
}

// RenderDisplayLabelV1 returns only the validated canonical user-facing text.
// A manually assembled or mutated value cannot render through this boundary.
func RenderDisplayLabelV1(label DisplayLabelV1) (string, error) {
	if ValidateDisplayLabelV1(label) != nil {
		return "", ErrDisplayLabelInvalidV1
	}
	return label.Text, nil
}

func renderDisplayLabelV1(label DisplayLabelV1) (string, error) {
	entityName := ""
	switch label.EntityType {
	case domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1:
		entityName = "银行账户"
	case domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1:
		entityName = "银行卡"
	default:
		return "", ErrDisplayLabelInvalidV1
	}
	if label.StableOrdinal == 0 ||
		!validDisplayLabelSuffixV1(label.SafeSuffix) {
		return "", ErrDisplayLabelInvalidV1
	}

	var builder strings.Builder
	builder.Grow(64 + len(label.Institution) + len(label.AccountType))
	builder.WriteString("第")
	builder.WriteString(strconv.FormatUint(uint64(label.StableOrdinal), 10))
	builder.WriteString("个")
	builder.WriteString(entityName)

	parts := make([]string, 0, 3)
	if label.Institution != "" {
		parts = append(parts, label.Institution)
	}
	if label.AccountType != "" {
		parts = append(parts, label.AccountType)
	}
	if label.SafeSuffix != "" {
		parts = append(parts, "尾号"+label.SafeSuffix)
	}
	if len(parts) > 0 {
		builder.WriteString("（")
		builder.WriteString(strings.Join(parts, "；"))
		builder.WriteString("）")
	}
	return builder.String(), nil
}

func validDisplayLabelSuffixV1(value string) bool {
	if value == "" {
		return true
	}
	if len(value) != 4 {
		return false
	}
	for index := range value {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

func canonicalDisplayLabelSemanticV1(value string, maxBytes int) (string, error) {
	if value == "" {
		return "", nil
	}
	if maxBytes <= 0 || len(value) > maxBytes || !utf8.ValidString(value) ||
		ContainsReferenceV1(value) || strings.Contains(value, ReferencePrefixV1) {
		return "", ErrDisplayLabelInvalidV1
	}

	var builder strings.Builder
	builder.Grow(len(value))
	previousSpace := true
	for _, current := range value {
		if unicode.IsControl(current) || unicode.In(current, unicode.Cf) {
			return "", ErrDisplayLabelInvalidV1
		}
		if unicode.IsSpace(current) {
			if !previousSpace {
				builder.WriteByte(' ')
				previousSpace = true
			}
			continue
		}
		builder.WriteRune(current)
		previousSpace = false
	}
	canonical := strings.TrimSpace(norm.NFC.String(builder.String()))
	if canonical == "" || len(canonical) > maxBytes || !safeDisplayLabelSemanticV1(canonical) {
		return "", ErrDisplayLabelInvalidV1
	}
	return canonical, nil
}

func safeDisplayLabelSemanticV1(value string) bool {
	digitCount := 0
	for _, current := range value {
		switch {
		case unicode.IsLetter(current), unicode.IsMark(current):
		case current >= '0' && current <= '9':
			digitCount++
			if digitCount > maxDisplayLabelSemanticDigitsV1 {
				return false
			}
		case current == ' ', current == '-', current == '·', current == '&',
			current == '/', current == '.', current == '\'':
		default:
			return false
		}
	}
	return true
}
