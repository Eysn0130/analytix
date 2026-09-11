package controlledaccount

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	maxControlledAccountContractBytesV1 = 256 * 1024
	maxControlledAccountStringBytesV1   = 4 * 1024
	maxCanonicalFinancialAccountBytesV2 = 256
)

// CanonicalFinancialAccountTextV2 preserves the existing bank-account/card
// normalization contract: exact source text may contain only ASCII digits
// separated by single spaces or hyphens, and canonical output contains the
// digits unchanged. It deliberately rejects Unicode digit lookalikes and
// lossy numeric spellings.
func CanonicalFinancialAccountTextV2(value string) (string, error) {
	if value == "" || len(value) > maxCanonicalFinancialAccountBytesV2 ||
		!utf8.ValidString(value) || value != strings.TrimSpace(value) {
		return "", errors.New("financial account source value is invalid")
	}

	var canonical strings.Builder
	canonical.Grow(len(value))
	previousSeparator := false
	digitCount := 0
	for index, character := range value {
		switch {
		case character >= '0' && character <= '9':
			canonical.WriteRune(character)
			digitCount++
			previousSeparator = false
		case character == ' ' || character == '-':
			if index == 0 || previousSeparator || digitCount == 0 {
				return "", errors.New("financial account source value is invalid")
			}
			previousSeparator = true
		default:
			return "", errors.New("financial account source value is invalid")
		}
	}
	if previousSeparator || digitCount == 0 {
		return "", errors.New("financial account source value is invalid")
	}
	return canonical.String(), nil
}

// ContainsCompleteFinancialAccountCandidateV2 reports only the complete
// account/card spelling already accepted by CanonicalFinancialAccountTextV2.
// It is a narrow owner-boundary guard, not a general PII classifier.
func ContainsCompleteFinancialAccountCandidateV2(value string) bool {
	for start := 0; start < len(value); {
		if value[start] < '0' || value[start] > '9' {
			start++
			continue
		}
		end := start
		lastDigitEnd := start
		previousSeparator := false
		for end < len(value) {
			current := value[end]
			if current >= '0' && current <= '9' {
				end++
				lastDigitEnd = end
				previousSeparator = false
				continue
			}
			if (current == ' ' || current == '-') && !previousSeparator {
				end++
				previousSeparator = true
				continue
			}
			break
		}
		candidate := value[start:lastDigitEnd]
		canonical, err := CanonicalFinancialAccountTextV2(candidate)
		if err == nil && len(canonical) >= 8 && len(canonical) <= 32 {
			return true
		}
		if end <= start {
			start++
		} else {
			start = end
		}
	}
	return false
}

func parseCanonicalControlledAccountContractV1(raw []byte, target any) error {
	if target == nil {
		return errors.New("controlled account contract target is nil")
	}
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject:  true,
		MaxBytes:       maxControlledAccountContractBytesV1,
		MaxDepth:       16,
		MaxTokens:      8_192,
		MaxStringBytes: maxControlledAccountStringBytesV1,
	}); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("controlled account contract contains trailing JSON")
	}
	canonical, err := json.Marshal(target)
	if err != nil || !bytes.Equal(raw, canonical) {
		return errors.New("controlled account contract is not canonically encoded")
	}
	return nil
}

func controlledAccountContractBytesV1(value any) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil || len(body) > maxControlledAccountContractBytesV1 {
		return nil, errors.New("controlled account contract exceeds its canonical byte limit")
	}
	return body, nil
}

func canonicalControlledAccountDigestV1(value string) bool {
	return value == strings.TrimSpace(value) && domainsecurity.IsSHA256Hex(value)
}

func canonicalControlledAccountOpaqueTextV1(value string, maxBytes int) bool {
	if maxBytes <= 0 {
		maxBytes = maxControlledAccountStringBytesV1
	}
	if value == "" || len(value) > maxBytes || !utf8.ValidString(value) ||
		value != strings.TrimSpace(value) {
		return false
	}
	for _, current := range value {
		if unicode.IsControl(current) || unicode.In(current, unicode.Cf) {
			return false
		}
	}
	return true
}

// looksLikeCompleteFinancialAccountV1 is intentionally conservative. These
// contracts never need a complete financial identifier, so an eight-or-more
// digit run (including common visual separators), or a mostly numeric
// alphanumeric account-shaped token, is rejected rather than retained.
func looksLikeCompleteFinancialAccountV1(value string) bool {
	if value == "" || !utf8.ValidString(value) {
		return false
	}
	runDigits := 0
	for _, current := range value {
		switch {
		case unicode.IsDigit(current):
			runDigits++
			if runDigits >= 8 {
				return true
			}
		case isFinancialAccountSeparatorV1(current):
			// A separator does not break a candidate account number.
		default:
			runDigits = 0
		}
	}

	compactRunes := make([]rune, 0, len(value))
	digitCount := 0
	for _, current := range value {
		if isFinancialAccountSeparatorV1(current) {
			continue
		}
		if !unicode.IsLetter(current) && !unicode.IsDigit(current) {
			return false
		}
		if unicode.IsDigit(current) {
			digitCount++
		}
		compactRunes = append(compactRunes, current)
	}
	return len(compactRunes) >= 8 && len(compactRunes) <= 34 && digitCount >= 8
}

func isFinancialAccountSeparatorV1(value rune) bool {
	// Treat every Unicode separator, punctuation mark, symbol, and formatting
	// character as a visual separator. This is defense in depth only: exact
	// account values are excluded by authority boundaries, not by this
	// heuristic. Keeping the class broad prevents common bypasses such as an
	// en dash, full-width punctuation, a colon, or an invisible joiner.
	return unicode.IsSpace(value) || unicode.IsPunct(value) ||
		unicode.IsSymbol(value) || unicode.In(value, unicode.Cf)
}

func canonicalPrefixedSHA256V1(value, prefix string) bool {
	return strings.HasPrefix(value, prefix) &&
		canonicalControlledAccountDigestV1(strings.TrimPrefix(value, prefix))
}
