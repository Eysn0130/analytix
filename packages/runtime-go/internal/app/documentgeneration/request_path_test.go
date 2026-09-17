package documentgeneration

import "testing"

// The application validates format suffixes without interpreting host filesystem
// paths. Whether a syntactically valid target is authorized is the file port's job.
func TestDocumentGenerationLexicalExtension(t *testing.T) {
	for _, name := range []string{"report.docx", "reports.v2/调查报告.DOCX", "/synthetic/report.docx"} {
		t.Run(name, func(t *testing.T) {
			request, err := ParseRequest(map[string]any{"kind": "docx", "path": name, "markdown": "# 标题\n\n内容"})
			if err != nil || request.Path != name {
				t.Fatalf("lexical suffix rejected or rewrote target: %q, %v", request.Path, err)
			}
		})
	}
	for _, name := range []string{"report.xlsx", "report.docx.tmp", "report.docx/child", "reports.docx/report", "report.docx/"} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseRequest(map[string]any{"kind": "docx", "path": name, "markdown": "content"}); err == nil {
				t.Fatal("accepted a target whose final component has the wrong format")
			}
		})
	}
}
