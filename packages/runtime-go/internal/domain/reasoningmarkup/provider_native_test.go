package reasoningmarkup

import (
	"errors"
	"strings"
	"testing"
)

func TestProviderNativeReasoningIsRejectedForEveryChunkPartition(t *testing.T) {
	t.Parallel()
	private := []string{
		`{"reasoning":"PRIVATE"}`,
		`{"thinking":"PRIVATE"}`,
		`{"type":"thinking","thinking":"PRIVATE"}`,
		`{"kind":"thinking_delta","text":"PRIVATE"}`,
		`{"type":"response.reasoning_summary_text.delta","delta":"PRIVATE"}`,
		`{"type":"redacted_thinking","data":"PRIVATE"}`,
		`{"thinkingSignature":"PRIVATE"}`,
		`{"thought":"PRIVATE"}`,
		`{"thoughtSignature":"PRIVATE"}`,
		`{"channel":"analysis","content":"PRIVATE"}`,
		`{"type":"assistantReasoning","text":"PRIVATE"}`,
		`{"type":"thinking.delta","text":"PRIVATE"}`,
		`{"type":"response.reasoning.delta","delta":"PRIVATE"}`,
		`{"payload":"{\"reasoning\":\"PRIVATE\"}"}`,
		`{"payload":"\"{\\\"type\\\":\\\"thinking\\\",\\\"thinking\\\":\\\"PRIVATE\\\"}\""}`,
		`{"reason\u0069ng":"PRIVATE"}`,
		`reasoning: "PRIVATE"`,
		`type: thinking`,
	}
	for _, input := range private {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			assertProviderNativePrivateForChunks(t, []string{input})
			for split := 0; split <= len(input); split++ {
				assertProviderNativePrivateForChunks(t, []string{input[:split], input[split:]})
			}
			chunks := make([]string, 0, len(input))
			for index := range len(input) {
				chunks = append(chunks, input[index:index+1])
			}
			assertProviderNativePrivateForChunks(t, chunks)
		})
	}
}

func TestProviderNativeInspectionPreservesOrdinaryPublicText(t *testing.T) {
	t.Parallel()
	public := []string{
		"The model explains its reasoning and thinking in public terms.",
		"I was thinking: verify the source first.",
		"reasoning_content is a field name discussed in documentation.",
		`{"message":"I was thinking: verify the source first."}`,
		`{"usage":{"reasoningTokens":12},"reasoningEffort":"high"}`,
		"<thinker>safe</thinker>",
	}
	for _, input := range public {
		outcome, err := Filter(input)
		if err != nil || outcome.PublicText != input {
			t.Fatalf("Filter(%q) = (%#v, %v), want byte-exact public text", input, outcome, err)
		}
	}
}

func TestProviderNativeAmbiguousOrUninspectableJSONFailsClosed(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "duplicate public metadata key",
			input: `{"reasoningTokens":1,"reasoningTokens":"PRIVATE"}`,
		},
		{
			name:  "candidate byte limit",
			input: `{"message":"` + strings.Repeat("a", maxProviderJSONCandidateBytes) + `"}`,
		},
		{
			name:  "depth limit",
			input: strings.Repeat("[", maxProviderJSONDepth+2) + `"public"` + strings.Repeat("]", maxProviderJSONDepth+2),
		},
		{
			name:  "candidate count limit",
			input: strings.Repeat(`{"message":"public"}`, maxProviderJSONCandidates+1),
		},
		{
			name:  "aggregate byte limit",
			input: strings.Repeat(`{"message":"`+strings.Repeat("a", 900_000)+`"}`, 10),
		},
		{
			name:  "token limit",
			input: `[` + strings.Repeat(`0,`, maxProviderJSONTokens) + `0]`,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			outcome, err := Filter(test.input)
			if err == nil || outcome != (Outcome{}) {
				t.Fatalf("Filter() = (%#v, %v), want zero fail-closed outcome", outcome, err)
			}
		})
	}
}

func TestProviderNativeMalformedPrivateFieldAssignmentFailsClosed(t *testing.T) {
	t.Parallel()
	for _, input := range []string{
		`{"reasoning":"PRIVATE",}`,
		`{"reason\u0069ng":"PRIVATE",}`,
		`{"type":"think\u0069ng",}`,
		`prefix { "type": "thinking", } suffix`,
		"prefix\n  'thinking' = 'PRIVATE'",
	} {
		if err := ValidateProviderNativeTextV1(input); !errors.Is(err, ErrPrivateContent) {
			t.Fatalf("ValidateProviderNativeTextV1(%q) error = %v, want ErrPrivateContent", input, err)
		}
	}
}

func assertProviderNativePrivateForChunks(t *testing.T, chunks []string) {
	t.Helper()
	decoder := NewDecoder()
	for _, chunk := range chunks {
		if err := decoder.Push(chunk); err != nil {
			t.Fatalf("Push(%q) error = %v", chunk, err)
		}
	}
	outcome, err := decoder.Finish()
	if !errors.Is(err, ErrPrivateContent) || outcome != (Outcome{}) {
		t.Fatalf("Finish(%q) = (%#v, %v), want zero outcome and ErrPrivateContent", strings.Join(chunks, "|"), outcome, err)
	}
}
