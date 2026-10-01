package objectediting

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	ordinary "analytix.local/runtime-go/internal/domain/ordinaryprojection"
	privacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	secret "analytix.local/runtime-go/internal/domain/secretprojection"
)

func TestProtectedSelectionRangesExactOriginalBytes(t *testing.T) {
	cases := []struct {
		name, text string
		protected  []string
	}{
		{"empty", "", nil},
		{"ordinary", "普通说明🙂\n金额 42 元", nil},
		{"whitespace", " \t\n\u3000", nil},
		{"labeled_name", "说明🙂\n姓名：张三\n完成", []string{"张三"}},
		{"leading_zero_account", "账号：00001234567890123456\n完成", []string{"00001234567890123456"}},
		{"email", "联系 synthetic@example.test 后继续", []string{"synthetic@example.test"}},
		{"explicit_secret", "说明\npassword='local-fixture-secret'\n完成", []string{"local-fixture-secret"}},
		{"unicode_boundaries", "e\u0301🙂姓名：𠮷野家\n完成", []string{"𠮷野家"}},
		{"repeated_name", "姓名：张三\n备注张三与张三", []string{"张三", "张三", "张三"}},
		{"repeated_email", "a@example.test 与 a@example.test", []string{"a@example.test", "a@example.test"}},
		{"overlapping_secrets", "password=abab; ababab", []string{"abab", "ababab"}},
		{"privacy_secret_overlap", "password=synthetic@example.test; done", []string{"synthetic@example.test"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ProtectedSelectionRanges(c.text)
			if err != nil {
				t.Fatal("unexpected range projection error")
			}
			if len(got) != len(c.protected) {
				t.Fatalf("range count: got %d want %d", len(got), len(c.protected))
			}
			cursor := 0
			for i, span := range got {
				if span.StartByte < cursor || span.EndByte <= span.StartByte || span.EndByte > len(c.text) || !utf8.ValidString(c.text[:span.StartByte]) || !utf8.ValidString(c.text[:span.EndByte]) {
					t.Fatal("invalid original byte boundary")
				}
				if c.text[span.StartByte:span.EndByte] != c.protected[i] {
					t.Fatalf("original span mismatch at index %d", i)
				}
				cursor = span.EndByte
			}
			assertSelectionLiteralsSafe(t, c.text, got)
		})
	}
}

func TestProtectedSelectionRangesConservativeFallback(t *testing.T) {
	cases := []struct{ name, text string }{
		{"short_unlocatable_credential", "keep password=abc end"},
		{"decoded_url_credential", "https://example.test/?token=a%2Fb%2Fc"},
		{"private_key_without_literal_extractor", "before\n-----BEGIN PRIVATE KEY-----\nSYNTHETIC\n-----END PRIVATE KEY-----\nafter"},
		{"unmapped_private_source", "说明 /cases/synthetic/note.txt"},
		{"privacy_fixed_point_cascade", "账户 6222020000000000000xxxxxxxxxx 12345678"},
		{"credential_fixed_point_cascade", "账户 6222020000000000000sk-abcdefghijk"},
		{"cross_literal_email", "姓名：张三\nuser张三@example.test"},
		{"merged_range_name_rejoin", "password='姓名：张三 '; 张ABCD三; password=ABCD"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ProtectedSelectionRanges(c.text)
			if err != nil || !reflect.DeepEqual(got, []ProtectedRange{{0, len(c.text)}}) {
				t.Fatalf("expected whole-selection protection, ranges=%v error=%t", got, err != nil)
			}
		})
	}
	// Confirm that these cases exercise a missing extraction and a newly exposed
	// literal boundary, rather than succeeding solely due to an unrelated detector.
	if !secret.ContainsCredentialV1("password=abc") || len(secret.ExtractExplicitSecretsV1("password=abc")) != 0 {
		t.Fatal("short-credential fixture no longer exercises fallback")
	}
	cross := "姓名：张三\nuser张三@example.test"
	// This name is initially covered by a larger credential span. Removing a
	// different credential reassembles the same name, which unlabeled PII alone
	// would not detect. The mapper must retain the pre-merge sensitive values.
	rejoined := "password=' '; 张三; password="
	if ordinary.ProjectTextV1(rejoined) != rejoined {
		t.Fatal("merged-range fixture no longer exercises raw value reassembly")
	}
	findings := privacy.ProjectText(cross).Findings
	if len(findings) != 1 || privacy.ContainsRestrictedPII("user") || privacy.ContainsRestrictedPII("@example.test") || !privacy.ContainsRestrictedPII("user@example.test") {
		t.Fatal("cross-literal fixture no longer exercises concatenation")
	}
}

func TestProtectedSelectionRangesBounds(t *testing.T) {
	for _, text := range []string{string([]byte{0xff}), "汉" + string([]byte{0x80}), strings.Repeat("a", MaxSelectionBytes+1)} {
		got, err := ProtectedSelectionRanges(text)
		if !errors.Is(err, ErrProjection) || got != nil {
			t.Fatal("invalid input was not rejected")
		}
	}
	for _, text := range []string{strings.Repeat("a", MaxSelectionBytes), strings.Repeat("汉", MaxSelectionBytes/3)} {
		got, err := ProtectedSelectionRanges(text)
		if err != nil || len(got) != 0 {
			t.Fatal("valid bounded ordinary input was not retained")
		}
	}
	for _, count := range []int{MaxProtectedSpans, MaxProtectedSpans + 1} {
		text := strings.Repeat("a@example.test\n", count)
		got, err := ProtectedSelectionRanges(text)
		if err != nil {
			t.Fatal("unexpected span capacity error")
		}
		if count == MaxProtectedSpans && len(got) != count {
			t.Fatal("exact span capacity not retained")
		}
		if count > MaxProtectedSpans && !reflect.DeepEqual(got, []ProtectedRange{{0, len(text)}}) {
			t.Fatal("excess spans were not protected as a whole")
		}
	}
}

func assertSelectionLiteralsSafe(t *testing.T, text string, ranges []ProtectedRange) {
	t.Helper()
	cursor := 0
	var joined strings.Builder
	check := func(literal string) {
		if strings.TrimSpace(literal) != "" && ordinary.ProjectTextV1(literal) != literal {
			t.Fatal("retained literal is not an ordinary fixed point")
		}
	}
	for _, span := range ranges {
		literal := text[cursor:span.StartByte]
		check(literal)
		joined.WriteString(literal)
		cursor = span.EndByte
	}
	check(text[cursor:])
	joined.WriteString(text[cursor:])
	check(joined.String())
	for _, span := range ranges {
		if strings.Contains(joined.String(), text[span.StartByte:span.EndByte]) {
			t.Fatal("protected original bytes survive in literals")
		}
	}
}
