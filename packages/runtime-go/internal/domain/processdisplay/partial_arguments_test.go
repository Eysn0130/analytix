package processdisplay

import (
	"strings"
	"testing"
)

func TestPartialArgumentsStreamingSummaryAndAuthoritativeSettlementV1(t *testing.T) {
	p := NewPartialArgumentsV1("write")
	if !p.AppendV1(`{"path":"draft.txt","content":"A\uD8`) {
		t.Fatal("named prefix was rejected")
	}
	first := p.ProjectV1()
	if first.Path != "draft.txt" || first.InputUtf16CodeUnits == nil || *first.InputUtf16CodeUnits != 1 {
		t.Fatalf("first decoded summary = %#v", first)
	}
	if !p.AppendV1(`3D\uDE00中"}`) {
		t.Fatal("split unicode escape was rejected")
	}
	complete := p.ProjectV1()
	if complete.InputUtf16CodeUnits == nil || *complete.InputUtf16CodeUnits != 4 {
		t.Fatalf("decoded UTF-16 count = %#v", complete.InputUtf16CodeUnits)
	}
	final := `{"path":"final.txt","content":"done😀"}`
	if !p.SettleV1(final) || p.RawBytesV1() != len(final) || p.AppendV1(` ignored`) {
		t.Fatal("authoritative final did not replace/seal the source")
	}
	settled := p.ProjectV1()
	if settled.Path != "final.txt" || settled.InputUtf16CodeUnits == nil || *settled.InputUtf16CodeUnits != 6 {
		t.Fatalf("settled source = %#v", settled)
	}
	for _, tc := range []struct {
		tool, raw string
		want      int64
	}{
		{"edit", `{"path":"a","old_string":"😀","new_string":"x\n`, 4},
		{"edit_file", `{"path":"a","oldText":"A","old_string":"ignored","new_text":"中`, 2},
		{"multi_edit", `{"path":"a","edits":[{"old_string":"A","new_string":"😀"},{"old_string":"B","new_string":"中`, 5},
	} {
		reader := NewPartialArgumentsV1(tc.tool)
		reader.AppendV1(tc.raw)
		value := reader.ProjectV1()
		if value.InputUtf16CodeUnits == nil || *value.InputUtf16CodeUnits != tc.want {
			t.Fatalf("%s partial count = %#v, want %d", tc.tool, value.InputUtf16CodeUnits, tc.want)
		}
	}
	other := NewPartialArgumentsV1("mcp__example__write")
	other.AppendV1(`{"content":"actual text","description":"Working customer Alice `)
	if value := other.ProjectV1(); value.InputUtf16CodeUnits != nil || value.Preview != "Working customer Alice " {
		t.Fatalf("generic tool acquired mutation size or lost authorized local text: %#v", value)
	}
}

func TestPartialArgumentsProjectionAndQuotaStayBoundedV1(t *testing.T) {
	secret := "test-partial-known-credential-93851"
	p := NewPartialArgumentsV1("write")
	p.AppendV1(`{"path":"/local/` + secret + `/report.txt","content":"api_key=` + secret + `"}`)
	value := p.ProjectV1(secret)
	if value.Classification != "credential_redacted" || strings.Contains(value.Path, secret) || !strings.Contains(value.Path, "report.txt") {
		t.Fatalf("credential-safe path = %#v", value)
	}
	for _, raw := range []string{
		`{"path":"a","content":"bad\q`,
		`{"path":"first","path":"second","content":"x"}`,
		`{"path":"bad\uD800","content":"x"} trailing`,
		`["not a tool argument object"]`,
	} {
		reader := NewPartialArgumentsV1("write")
		reader.AppendV1(raw)
		got := reader.ProjectV1()
		if got.Classification != "withheld" || got.ToolName != "" || got.Path != "" || got.Preview != "" || got.InputUtf16CodeUnits != nil {
			t.Fatalf("invalid input retained display fields: %#v", got)
		}
	}
	open := NewPartialArgumentsV1("bash")
	open.AppendV1(`{"description":"Working customer Alice ` + secret[:12])
	if got := open.ProjectV1(secret); strings.Contains(got.Preview, secret[:12]) || got.Preview != "Working customer Alice " {
		t.Fatalf("open credential suffix = %#v", got)
	}
	overflow := NewPartialArgumentsV1("write")
	if overflow.AppendV1(strings.Repeat("x", RawCaptureByteLimitV1+1)) || overflow.RawBytesV1() != 0 || overflow.ProjectV1().ToolName != "" {
		t.Fatal("overflow was retained or displayed")
	}
}
