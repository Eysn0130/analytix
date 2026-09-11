package thread

import "testing"

func TestReviewTitleAndPrompt(t *testing.T) {
	cases := []struct {
		name   string
		target map[string]any
		title  string
		prompt string
	}{
		{
			name:   "base branch",
			target: map[string]any{"kind": "baseBranch", "branch": "main"},
			title:  "Review against main",
			prompt: "Review changes against base branch main.",
		},
		{
			name:   "commit",
			target: map[string]any{"kind": "commit", "sha": "abc123"},
			title:  "Review commit abc123",
			prompt: "Review commit abc123.",
		},
		{
			name:   "custom",
			target: map[string]any{"kind": "custom", "instructions": "Check only API changes."},
			title:  "Review custom target",
			prompt: "Check only API changes.",
		},
		{
			name:   "default",
			target: map[string]any{},
			title:  "Review uncommitted changes",
			prompt: "Review uncommitted changes.",
		},
	}
	for _, tc := range cases {
		if title := ReviewTitle(tc.target); title != tc.title {
			t.Fatalf("%s title = %q, want %q", tc.name, title, tc.title)
		}
		if prompt := ReviewPrompt(tc.target); prompt != tc.prompt {
			t.Fatalf("%s prompt = %q, want %q", tc.name, prompt, tc.prompt)
		}
	}
}
