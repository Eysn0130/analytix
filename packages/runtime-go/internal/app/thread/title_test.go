package thread

import "testing"

func TestTitleFromTurnPrefersDisplayTextAndNormalizesWhitespace(t *testing.T) {
	title := TitleFromTurn(map[string]any{
		"prompt": "fallback prompt",
		"items": []any{
			map[string]any{"kind": "assistant_text", "text": "skip"},
			map[string]any{"kind": "user_message", "displayText": "  hello\n  world  ", "text": "raw"},
		},
	})
	if title != "hello world" {
		t.Fatalf("title mismatch: %q", title)
	}
}

func TestThreadTitleFromTextTruncatesByRune(t *testing.T) {
	title := ThreadTitleFromText("abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrst")
	if len([]rune(title)) != 64 || title[61:] != "..." {
		t.Fatalf("truncated title mismatch: %q", title)
	}
}

func TestThreadTitleFromTextProjectsRestrictedPII(t *testing.T) {
	if title := ThreadTitleFromText("核对卡号 6222020000000000000"); title != "核对卡号 [ACCOUNT]" {
		t.Fatalf("projected title = %q", title)
	}
}

func TestShouldAutoTitleThread(t *testing.T) {
	if !ShouldAutoTitleThread(map[string]any{"title": "New thread"}) {
		t.Fatal("New thread should be auto titled")
	}
	if !ShouldAutoTitleThread(map[string]any{"title": "Custom", "autoTitle": true}) {
		t.Fatal("autoTitle flag should force title update")
	}
	if ShouldAutoTitleThread(map[string]any{"title": "Custom"}) {
		t.Fatal("custom title should not auto title")
	}
}
