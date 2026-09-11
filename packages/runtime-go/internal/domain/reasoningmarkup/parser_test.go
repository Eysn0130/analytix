package reasoningmarkup

import (
	"errors"
	"strings"
	"testing"
)

func TestFilterPreservesPublicBytesAndRemovesReasoningMarkup(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		input         string
		want          string
		wantReasoning bool
		wantRecovery  RecoveryKind
	}{
		{
			name:  "plain text byte exact",
			input: "  alpha\nbeta  ",
			want:  "  alpha\nbeta  ",
		},
		{
			name:          "case attributes quoted delimiter and closing whitespace",
			input:         "before<ThInK class=\"x>y\" data='z'>PRIVATE</tHiNk >after",
			want:          "beforeafter",
			wantReasoning: true,
		},
		{
			name:          "closing attributes",
			input:         "before<think>PRIVATE</think ignored='>' >after",
			want:          "beforeafter",
			wantReasoning: true,
		},
		{
			name:          "nested multiple and self closing",
			input:         "A<think>x<think>y</think>z</think>B<think/>C",
			want:          "ABC",
			wantReasoning: true,
		},
		{
			name:          "provider analysis and reasoning tags",
			input:         "A<analysis>PRIVATE</analysis>B<reasoning>SECRET</reasoning>C",
			want:          "ABC",
			wantReasoning: true,
		},
		{
			name:          "orphan close discards uncommitted candidate",
			input:         "PRIVATE</think>public",
			want:          "public",
			wantReasoning: true,
			wantRecovery:  RecoveryOrphanClose,
		},
		{
			name:          "orphan close cannot revoke text committed before a real block",
			input:         "keep<think>secret</think>PRIVATE</think>tail",
			want:          "keeptail",
			wantReasoning: true,
			wantRecovery:  RecoveryOrphanClose,
		},
		{
			name:  "non marker tag name",
			input: "<thinker>safe</thinker>",
			want:  "<thinker>safe</thinker>",
		},
		{
			name:  "escaped marker discussion",
			input: "&lt;think&gt;safe&lt;/think&gt;",
			want:  "&lt;think&gt;safe&lt;/think&gt;",
		},
		{
			name:  "ordinary trailing less than",
			input: "one <",
			want:  "one <",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertAllPartitions(t, test.input, test.want, test.wantReasoning, test.wantRecovery)
		})
	}
}

func TestFilterFailsClosedForIncompleteReasoningMarkup(t *testing.T) {
	t.Parallel()
	for _, input := range []string{
		"<think>PRIVATE",
		"public</thi",
		"<thi",
		"<think class=\"unterminated>PRIVATE",
		"<think class='unterminated>PRIVATE</think>",
		"<analysis>PRIVATE",
		"<reasoning>PRIVATE</analysis>",
	} {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			outcome, err := Filter(input)
			if !errors.Is(err, ErrIncomplete) {
				t.Fatalf("Filter() error = %v, want ErrIncomplete", err)
			}
			if outcome != (Outcome{}) {
				t.Fatalf("Filter() outcome = %#v, want zero outcome", outcome)
			}
		})
	}
}

func TestFilterEnforcesHostLimitsWithoutReturningPublicText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{name: "total bytes", input: strings.Repeat("a", MaxInputBytes+1)},
		{name: "tag bytes", input: "<think " + strings.Repeat("a", MaxTagBytes) + ">PRIVATE</think>"},
		{name: "nesting depth", input: strings.Repeat("<think>", MaxDepth+1) + strings.Repeat("</think>", MaxDepth+1)},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			outcome, err := Filter(test.input)
			if !errors.Is(err, ErrLimit) {
				t.Fatalf("Filter() error = %v, want ErrLimit", err)
			}
			if outcome != (Outcome{}) {
				t.Fatalf("Filter() outcome = %#v, want zero outcome", outcome)
			}
		})
	}
}

func TestDecoderIsTransactionalAcrossOrphanCloseChunks(t *testing.T) {
	t.Parallel()
	decoder := NewDecoder()
	if err := decoder.Push("PRIVATE"); err != nil {
		t.Fatalf("Push(private) error = %v", err)
	}
	if err := decoder.Push("</think>public"); err != nil {
		t.Fatalf("Push(close) error = %v", err)
	}
	outcome, err := decoder.Finish()
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if outcome.PublicText != "public" || outcome.Recovery != RecoveryOrphanClose {
		t.Fatalf("Finish() = %#v, want public orphan-close recovery", outcome)
	}
}

func TestDecoderPoisonAndLifecycleAreFailClosed(t *testing.T) {
	t.Parallel()
	decoder := NewDecoder()
	if err := decoder.Push(strings.Repeat("x", MaxInputBytes+1)); !errors.Is(err, ErrLimit) {
		t.Fatalf("oversized Push() error = %v, want ErrLimit", err)
	}
	if err := decoder.Push("public"); !errors.Is(err, ErrLimit) {
		t.Fatalf("Push() after poison error = %v, want ErrLimit", err)
	}
	if outcome, err := decoder.Finish(); !errors.Is(err, ErrLimit) || outcome != (Outcome{}) {
		t.Fatalf("Finish() after poison = (%#v, %v), want zero and ErrLimit", outcome, err)
	}
	if err := decoder.Push("public"); !errors.Is(err, ErrLifecycle) {
		t.Fatalf("Push() after Finish error = %v, want ErrLifecycle", err)
	}
	if outcome, err := decoder.Finish(); !errors.Is(err, ErrLifecycle) || outcome != (Outcome{}) {
		t.Fatalf("second Finish() = (%#v, %v), want zero and ErrLifecycle", outcome, err)
	}
	var nilDecoder *Decoder
	if err := nilDecoder.Push("public"); !errors.Is(err, ErrLifecycle) {
		t.Fatalf("nil Push() error = %v, want ErrLifecycle", err)
	}
	if outcome, err := nilDecoder.Finish(); !errors.Is(err, ErrLifecycle) || outcome != (Outcome{}) {
		t.Fatalf("nil Finish() = (%#v, %v), want zero and ErrLifecycle", outcome, err)
	}
}

func TestFilterAllowsExactlyMaximumNestingDepth(t *testing.T) {
	t.Parallel()
	input := "before" + strings.Repeat("<think>", MaxDepth) + "PRIVATE" + strings.Repeat("</think>", MaxDepth) + "after"
	outcome, err := Filter(input)
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}
	if outcome.PublicText != "beforeafter" {
		t.Fatalf("Filter() public text = %q, want %q", outcome.PublicText, "beforeafter")
	}
}

func assertAllPartitions(t *testing.T, input string, want string, wantReasoning bool, wantRecovery RecoveryKind) {
	t.Helper()
	assertDecodedChunks(t, []string{input}, want, wantReasoning, wantRecovery)
	for split := 0; split <= len(input); split++ {
		assertDecodedChunks(t, []string{input[:split], input[split:]}, want, wantReasoning, wantRecovery)
	}
	chunks := make([]string, 0, len(input))
	for index := range len(input) {
		chunks = append(chunks, input[index:index+1])
	}
	assertDecodedChunks(t, chunks, want, wantReasoning, wantRecovery)
}

func assertDecodedChunks(t *testing.T, chunks []string, want string, wantReasoning bool, wantRecovery RecoveryKind) {
	t.Helper()
	decoder := NewDecoder()
	for _, chunk := range chunks {
		if err := decoder.Push(chunk); err != nil {
			t.Fatalf("Push(%q) error = %v", chunk, err)
		}
	}
	outcome, err := decoder.Finish()
	if err != nil {
		t.Fatalf("Finish(%q) error = %v", strings.Join(chunks, "|"), err)
	}
	if outcome.PublicText != want || outcome.HadReasoning != wantReasoning || outcome.Recovery != wantRecovery {
		t.Fatalf("Finish(%q) = %#v, want text %q reasoning=%v recovery=%v", strings.Join(chunks, "|"), outcome, want, wantReasoning, wantRecovery)
	}
}
