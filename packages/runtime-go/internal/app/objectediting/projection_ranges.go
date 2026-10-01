package objectediting

import (
	"sort"
	"strings"
	"unicode/utf8"

	ordinary "analytix.local/runtime-go/internal/domain/ordinaryprojection"
	privacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	secret "analytix.local/runtime-go/internal/domain/secretprojection"
)

// ProtectedSelectionRanges maps current ordinary-output protection to original
// UTF-8 bytes. It provides no principal, thread, object or purpose authority.
// Unlocatable protection and unsafe residual literals protect the whole selection;
// malformed or oversized inputs fail without returning any source text.
func ProtectedSelectionRanges(text string) ([]ProtectedRange, error) {
	if len(text) > MaxSelectionBytes || !utf8.ValidString(text) {
		return nil, ErrProjection
	}
	ranges := []ProtectedRange{}
	if text == "" || strings.TrimSpace(text) == "" {
		return ranges, nil
	}
	whole := func() ([]ProtectedRange, error) { return []ProtectedRange{{0, len(text)}}, nil }
	// Search original bytes, never reverse-map a projected placeholder or normalize
	// a value. Repeated original values must not survive in another literal.
	protectedValues := make(map[string]struct{})
	addLiteral := func(value string) bool {
		if _, found := protectedValues[value]; found {
			return true
		}
		if value == "" || !utf8.ValidString(value) {
			return false
		}
		found := false
		for from := 0; from < len(text); {
			relative := strings.Index(text[from:], value)
			if relative < 0 {
				break
			}
			start := from + relative
			end := start + len(value)
			if !selectionByteBoundary(text, start) || !selectionByteBoundary(text, end) {
				return false
			}
			ranges = append(ranges, ProtectedRange{start, end})
			ranges = mergeSelectionRanges(ranges)
			if len(ranges) > MaxProtectedSpans {
				return false
			}
			found = true
			from = start + 1 // Include overlapping occurrences of the same literal.
		}
		if found {
			protectedValues[value] = struct{}{}
		}
		return found
	}
	for _, finding := range privacy.ProjectText(text).Findings {
		if finding.StartByte < 0 || finding.EndByte <= finding.StartByte || finding.EndByte > len(text) ||
			!selectionByteBoundary(text, finding.StartByte) || !selectionByteBoundary(text, finding.EndByte) {
			return whole()
		}
		if !addLiteral(text[finding.StartByte:finding.EndByte]) {
			return whole()
		}
	}
	credentials := secret.ExtractExplicitSecretsV1(text)
	if secret.ContainsCredentialV1(text) && len(credentials) == 0 {
		return whole()
	}
	for _, value := range credentials {
		// URL-decoded credentials may have no exact original-byte spelling. They
		// cannot be assigned speculative offsets even if another value was located.
		if !addLiteral(value) {
			return whole()
		}
	}
	var literals strings.Builder
	cursor := 0
	for _, span := range ranges {
		literal := text[cursor:span.StartByte]
		if !selectionLiteralFixedPoint(literal) {
			return whole()
		}
		literals.WriteString(literal)
		cursor = span.EndByte
	}
	tail := text[cursor:]
	if !selectionLiteralFixedPoint(tail) {
		return whole()
	}
	literals.WriteString(tail)
	remaining := literals.String()
	// Each projector can expose new boundaries, including across separate literal
	// parts. Validate the composed fixed point as well as each individual part.
	if !selectionLiteralFixedPoint(remaining) {
		return whole()
	}
	for _, span := range ranges {
		if strings.Contains(remaining, text[span.StartByte:span.EndByte]) {
			return whole()
		}
	}
	for value := range protectedValues {
		if strings.Contains(remaining, value) {
			return whole()
		}
	}
	return ranges, nil
}

func selectionByteBoundary(text string, offset int) bool {
	return offset >= 0 && offset <= len(text) && (offset == len(text) || utf8.RuneStart(text[offset]))
}

func selectionLiteralFixedPoint(text string) bool {
	// The existing ordinary projector canonicalizes whitespace-only strings to
	// empty; retaining such separators leaks no protected data.
	return strings.TrimSpace(text) == "" || ordinary.ProjectTextV1(text) == text
}

func mergeSelectionRanges(ranges []ProtectedRange) []ProtectedRange {
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].StartByte == ranges[j].StartByte {
			return ranges[i].EndByte < ranges[j].EndByte
		}
		return ranges[i].StartByte < ranges[j].StartByte
	})
	merged := ranges[:0]
	for _, span := range ranges {
		if len(merged) == 0 || span.StartByte > merged[len(merged)-1].EndByte {
			merged = append(merged, span)
		} else if span.EndByte > merged[len(merged)-1].EndByte {
			merged[len(merged)-1].EndByte = span.EndByte
		}
	}
	return merged
}
