package event

import "testing"

func TestFilterPublicTextUsesCanonicalTransactionalMarkupSemantics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{
			name:  "attributed split semantics",
			input: `public<ThInK class="x>y">PRIVATE_REASONING_SENTINEL</tHiNk >tail`,
			want:  "publictail",
			ok:    true,
		},
		{
			name:  "orphan close discards candidate",
			input: "PRIVATE_REASONING_SENTINEL</think>public",
			want:  "public",
			ok:    true,
		},
		{
			name:  "plain bytes are not trimmed",
			input: "  public\n",
			want:  "  public\n",
			ok:    true,
		},
		{
			name:  "unclosed block",
			input: "public<think>PRIVATE_REASONING_SENTINEL",
			ok:    false,
		},
		{
			name:  "partial closing marker",
			input: "public</thi",
			ok:    false,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := FilterPublicText(test.input)
			if (err == nil) != test.ok || got != test.want {
				t.Fatalf("FilterPublicText(%q) = (%q, %v), want (%q, success=%v)", test.input, got, err, test.want, test.ok)
			}
		})
	}
}

func TestFilterPublicTextDoesNotTreatLongerTagNameAsReasoning(t *testing.T) {
	t.Parallel()
	input := "<thinker>safe</thinker>"
	got, err := FilterPublicText(input)
	if err != nil || got != input {
		t.Fatalf("FilterPublicText(%q) = (%q, %v), want byte-exact public text", input, got, err)
	}
}
