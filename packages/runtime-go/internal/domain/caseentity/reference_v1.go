package caseentity

import (
	"errors"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	domainprivacyprojection "analytix.local/runtime-go/internal/domain/privacyprojection"
)

const ReferencePrefixV1 = "cer1_"

const (
	referenceDigestLengthV1   = 64
	referenceLengthV1         = len(ReferencePrefixV1) + referenceDigestLengthV1
	referenceCandidateFloorV1 = referenceDigestLengthV1
)

// ReferenceV1 is a provider-safe, case-scoped internal entity reference. It is
// an analysis identity, not user-visible product language or evidence version
// authority.
type ReferenceV1 string

// NewReferenceV1FromKeyedDigest encodes an installation-keyed 256-bit digest
// without decimal characters. The a-p alphabet prevents account-number
// projectors from mistaking an internal reference for a complete account.
func NewReferenceV1FromKeyedDigest(keyedDigest string) (ReferenceV1, error) {
	if len(keyedDigest) != referenceDigestLengthV1 {
		return "", errors.New("case entity keyed digest is invalid")
	}
	encoded := make([]byte, referenceLengthV1)
	copy(encoded, ReferencePrefixV1)
	for index := range keyedDigest {
		switch current := keyedDigest[index]; {
		case current >= '0' && current <= '9':
			encoded[len(ReferencePrefixV1)+index] = 'a' + current - '0'
		case current >= 'a' && current <= 'f':
			encoded[len(ReferencePrefixV1)+index] = 'k' + current - 'a'
		default:
			return "", errors.New("case entity keyed digest is invalid")
		}
	}
	reference := ReferenceV1(encoded)
	if err := ValidateReferenceV1(string(reference)); err != nil {
		return "", err
	}
	return reference, nil
}

type referenceCandidateSpanV1 struct {
	startByte int
	endByte   int
}

type referenceNormalizedSourceRuneV1 struct {
	value     rune
	startByte int
	endByte   int
}

// ContainsReferenceCandidateV1 detects complete references and raw tokens
// normalized for Unicode compatibility, case, and visual separators without
// treating ordinary code identifiers such as cer1_parser as case identity.
func ContainsReferenceCandidateV1(value string) bool {
	return len(referenceCandidateSpansV1(value)) != 0
}

// MaskReferenceCandidatesV1 uses the same scanner as
// ContainsReferenceCandidateV1 and replaces every exact source span it found.
// Detection and withholding therefore cannot disagree about inserted visual
// separators, full-width ASCII, or letter case.
func MaskReferenceCandidatesV1(value string) (string, bool) {
	candidateSpans := referenceCandidateSpansV1(value)
	if len(candidateSpans) == 0 {
		return value, false
	}
	spans := referenceCandidateMaskSpansV1(value, candidateSpans)
	var projected strings.Builder
	projected.Grow(len(value))
	cursor := 0
	for _, span := range spans {
		projected.WriteString(value[cursor:span.startByte])
		projected.WriteString(domainprivacyprojection.Placeholder(domainprivacyprojection.KindAccount))
		cursor = span.endByte
	}
	projected.WriteString(value[cursor:])
	masked := projected.String()
	if ContainsReferenceCandidateV1(masked) {
		return domainprivacyprojection.Placeholder(domainprivacyprojection.KindAccount), true
	}
	return masked, true
}

func referenceCandidateMaskSpansV1(
	value string,
	candidateSpans []referenceCandidateSpanV1,
) []referenceCandidateSpanV1 {
	spans := make([]referenceCandidateSpanV1, 0, len(candidateSpans)*2)
	spans = append(spans, candidateSpans...)
	normalized := referenceNormalizedSourceRunesV1(value)
	for startIndex, current := range normalized {
		if current.value == 'c' {
			if prefixEnd, matched := referenceCandidatePrefixEndV1(normalized, startIndex); matched {
				spans = append(spans, referenceCandidateSpanV1{
					startByte: current.startByte,
					endByte:   normalized[prefixEnd-1].endByte,
				})
			}
		}
	}
	sort.Slice(spans, func(left, right int) bool {
		if spans[left].startByte != spans[right].startByte {
			return spans[left].startByte < spans[right].startByte
		}
		return spans[left].endByte < spans[right].endByte
	})
	merged := spans[:0]
	for _, span := range spans {
		if len(merged) != 0 && span.startByte <= merged[len(merged)-1].endByte {
			merged[len(merged)-1].endByte = max(merged[len(merged)-1].endByte, span.endByte)
			continue
		}
		merged = append(merged, span)
	}
	return merged
}

func referenceCandidateSpansV1(value string) []referenceCandidateSpanV1 {
	if value == "" || !utf8.ValidString(value) {
		return nil
	}
	return referenceCandidateSpansInViewV1(referenceNormalizedSourceRunesV1(value))
}

func referenceCandidateSpansInViewV1(
	normalized []referenceNormalizedSourceRuneV1,
) []referenceCandidateSpanV1 {
	spans := make([]referenceCandidateSpanV1, 0, 2)
	for startIndex, current := range normalized {
		if current.value != 'c' {
			continue
		}
		suffixStart, prefixMatched := referenceCandidatePrefixEndV1(normalized, startIndex)
		if !prefixMatched {
			continue
		}
		suffixCount := 0
		lastSuffixEnd := normalized[suffixStart-1].endByte
	scanSuffix:
		for offset := suffixStart; offset < len(normalized); offset++ {
			currentSuffix := normalized[offset]
			switch {
			case currentSuffix.value >= 'a' && currentSuffix.value <= 'p':
				suffixCount++
				lastSuffixEnd = currentSuffix.endByte
			case referenceCandidateSeparatorV1(currentSuffix.value):
			default:
				break scanSuffix
			}
		}
		if suffixCount < referenceCandidateFloorV1 {
			continue
		}
		span := referenceCandidateSpanV1{
			startByte: current.startByte,
			endByte:   lastSuffixEnd,
		}
		if len(spans) != 0 && span.startByte <= spans[len(spans)-1].endByte {
			spans[len(spans)-1].endByte = max(spans[len(spans)-1].endByte, span.endByte)
		} else {
			spans = append(spans, span)
		}
		// Continue from the next normalized rune, rather than the first span's end,
		// so a separator-obfuscated suffix cannot consume the beginning of a
		// second candidate and hide it from the masker's exact-span union.
	}
	return spans
}

func referenceCandidatePrefixEndV1(
	value []referenceNormalizedSourceRuneV1,
	startIndex int,
) (int, bool) {
	offset := startIndex
	for expectedIndex, expected := range []rune{'c', 'e', 'r', '1', '_'} {
		if expectedIndex != 0 {
			for offset < len(value) {
				current := value[offset].value
				if expected == '_' && current == '_' {
					break
				}
				if !referenceCandidateSeparatorV1(current) {
					break
				}
				offset++
			}
		}
		if offset >= len(value) {
			return 0, false
		}
		if value[offset].value != expected {
			return 0, false
		}
		offset++
	}
	return offset, true
}

func referenceNormalizedSourceRunesV1(value string) []referenceNormalizedSourceRuneV1 {
	var iterator norm.Iter
	iterator.InitString(norm.NFKC, value)
	result := make([]referenceNormalizedSourceRuneV1, 0, utf8.RuneCountInString(value))
	segmentStart := 0
	for !iterator.Done() {
		segment := iterator.Next()
		segmentEnd := iterator.Pos()
		for len(segment) > 0 {
			current, width := utf8.DecodeRune(segment)
			result = append(result, referenceNormalizedSourceRuneV1{
				value:     unicode.ToLower(current),
				startByte: segmentStart,
				endByte:   segmentEnd,
			})
			segment = segment[width:]
		}
		segmentStart = segmentEnd
	}
	return result
}

func referenceCandidateSeparatorV1(value rune) bool {
	return unicode.IsSpace(value) || unicode.IsPunct(value) || unicode.IsSymbol(value) ||
		unicode.IsMark(value) || unicode.IsControl(value) || unicode.In(value, unicode.Cf)
}

func ValidateReferenceV1(value string) error {
	if len(value) != referenceLengthV1 || value[:len(ReferencePrefixV1)] != ReferencePrefixV1 {
		return errors.New("case entity reference is invalid")
	}
	for index := len(ReferencePrefixV1); index < len(value); index++ {
		if value[index] < 'a' || value[index] > 'p' {
			return errors.New("case entity reference is invalid")
		}
	}
	return nil
}

// ContainsReferenceV1 reports whether public-facing text contains a complete
// provider-safe case entity reference. References are intentionally useful to
// the model and host, but they are not user-facing product language and must
// not survive an ordinary-result or final renderer boundary.
func ContainsReferenceV1(value string) bool {
	for searchAt := 0; searchAt < len(value); {
		offset := strings.Index(value[searchAt:], ReferencePrefixV1)
		if offset < 0 {
			return false
		}
		start := searchAt + offset
		end := start + referenceLengthV1
		if end <= len(value) && ValidateReferenceV1(value[start:end]) == nil {
			return true
		}
		searchAt = start + len(ReferencePrefixV1)
	}
	return false
}
