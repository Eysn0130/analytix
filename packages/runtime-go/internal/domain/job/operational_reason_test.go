package job

import "testing"

func TestNormalizeOperationalReasonV1IsClosed(t *testing.T) {
	for _, reason := range []string{"append_parent_turn_failed", "parent_tool_identity_invalid", "parent_tool_result_invalid"} {
		if got := NormalizeOperationalReasonV1(" " + reason + " "); got != reason {
			t.Fatalf("known operational reason %q = %q", reason, got)
		}
	}
	for _, input := range []string{
		"PRIVATE database error account=6222020202020202020",
		`{"reasoning_content":"PRIVATE_REASONING"}`,
		"parent authority missing",
	} {
		if got := NormalizeOperationalReasonV1(input); got != OperationalReasonWithheld {
			t.Fatalf("untrusted operational reason %q = %q", input, got)
		}
	}
	if got := NormalizeOperationalReasonV1("  "); got != "" {
		t.Fatalf("empty operational reason = %q", got)
	}
}
