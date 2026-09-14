package documentgeneration

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestGenerationRequestBindsExplicitContentWithoutPathsInCodec(t *testing.T) {
	r, err := ParseRequest(map[string]any{"path": "报告.docx", "kind": "docx", "markdown": "# 报告\n\n|项|值|\n|-|-|\n|合计|42|", "title": "报告"})
	if err != nil || r.CodecInput().SchemaVersion != 1 || r.CodecInput().Markdown != r.Markdown {
		t.Fatal("valid request rejected")
	}
	for _, change := range []map[string]any{
		{"title": nil}, {"images": nil}, {"title": strings.Repeat("😀", 129)},
		{"images": []any{map[string]any{"id": "image-1", "type": "png", "dataBase64": strings.Repeat("YQ==", (1<<18)+1)}}},
		{"kind": "pptx"}, {"path": "report.xlsx"}, {"markdown": " "}, {"markdown": strings.Repeat("a", (1<<20)+1)},
		{"script": "process.exit()"}, {"images": []any{map[string]any{"id": "../private", "type": "png", "dataBase64": "YWJj"}}},
		{"images": []any{map[string]any{"id": "image-1", "type": "png", "dataBase64": "YWJj\n"}}},
		{"images": []any{map[string]any{"id": "image-1", "type": "svg", "dataBase64": base64.StdEncoding.EncodeToString([]byte("svg"))}}},
	} {
		args := map[string]any{"path": "report.docx", "kind": "docx", "markdown": "Synthetic"}
		for key, value := range change {
			args[key] = value
		}
		if _, err := ParseRequest(args); err == nil {
			t.Fatal("unsafe request accepted")
		}
	}
}

func TestGenerationRequestAcceptsContentWithinExecutionGrantBudget(t *testing.T) {
	_, err := ParseRequest(map[string]any{"path": "report.docx", "kind": "docx", "markdown": strings.Repeat("a", 600<<10), "images": []any{map[string]any{"id": "image-1", "type": "png", "dataBase64": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("b", 525<<10)))}}})
	if err != nil {
		t.Fatal("valid content within canonical authority budget was rejected")
	}
}
