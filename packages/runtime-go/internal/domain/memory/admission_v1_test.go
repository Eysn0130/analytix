package memory

import (
	"errors"
	"strings"
	"testing"
)

func TestManualMemoryContentRejectsCaseAuthorityAndRawChannels(t *testing.T) {
	for _, content := range []string{
		"authority cer1_" + strings.Repeat("a", 64),
		"continue with acct:1",
		"source /private/case.duckdb",
		`{"structuredContent":{"subjectRef":"private"}}`,
	} {
		if err := ValidateManualContentV1(content); err == nil || strings.Contains(err.Error(), content) {
			t.Fatalf("case memory content was accepted or echoed: %q err=%v", content, err)
		}
	}
	for _, content := range []string{
		"Use pnpm for frontend projects",
		"Prefer concise explanations and deterministic tests",
		"Manual general text is not classified by a guessed name regex",
	} {
		if err := ValidateManualContentV1(content); err != nil {
			t.Fatalf("general manual memory was rejected: %q err=%v", content, err)
		}
	}
}

func TestValidateManualContentV1RejectsSupportedCompleteAccountAndCardShapes(t *testing.T) {
	for _, content := range []string{
		"account 6222021234567890123",
		"card 6222-0212 3456-7890",
	} {
		if err := ValidateManualContentV1(content); !errors.Is(err, ErrContentNotAdmissibleV1) {
			t.Fatalf("supported account/card shape was admitted: content=%q err=%v", content, err)
		}
	}
}
