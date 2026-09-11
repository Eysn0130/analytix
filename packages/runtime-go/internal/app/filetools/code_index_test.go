package filetools

import (
	"strings"
	"testing"
)

func TestParseCodeIndexGoFindsFunctionsTypesAndMethods(t *testing.T) {
	source := []byte(`package demo

type Client struct{}
const DefaultName = "demo"

func NewClient() *Client { return &Client{} }
func (c *Client) Run() {}
`)
	symbols, err := ParseCodeIndexGo("demo/client.go", "/tmp/demo/client.go", source)
	if err != nil {
		t.Fatalf("parse go index: %v", err)
	}
	got := map[string]CodeIndexSymbol{}
	for _, symbol := range symbols {
		got[symbol.Kind+":"+symbol.Name] = symbol
	}
	if got["struct:Client"].Signature != "struct Client" {
		t.Fatalf("missing struct symbol: %#v", got)
	}
	if got["const:DefaultName"].Line != 4 {
		t.Fatalf("missing const line: %#v", got)
	}
	if got["func:NewClient"].Signature != "func NewClient" {
		t.Fatalf("missing func symbol: %#v", got)
	}
	method := got["method:Run"]
	if method.Parent != "Client" || method.Signature != "func (Client) Run" {
		t.Fatalf("unexpected method symbol: %#v", method)
	}
}

func TestParseCodeIndexTextFiltersAndFormats(t *testing.T) {
	symbols := ParseCodeIndexText(".ts", "src/demo.ts", "/tmp/src/demo.ts", `
export interface Contract {}
export type Result = string
export const runTask = async () => {}
`)
	request := CodeIndexRequest{Action: "search", Query: "run", Kind: "func", Limit: 10}
	filtered := FilterCodeIndexSymbols(symbols, request)
	if len(filtered) != 1 || filtered[0].Name != "runTask" {
		t.Fatalf("unexpected filtered symbols: %#v", filtered)
	}
	formatted := FormatCodeIndexSymbols(filtered, true)
	if !strings.Contains(formatted, "src/demo.ts:4: func runTask") || !strings.Contains(formatted, "truncated") {
		t.Fatalf("unexpected formatted symbols: %q", formatted)
	}
}

func TestParseCodeIndexRequestValidatesSearchQuery(t *testing.T) {
	_, err := ParseCodeIndexRequest(map[string]any{"action": "search"}, 100, 200)
	if err == nil {
		t.Fatal("expected query_required error")
	}
	typed, ok := err.(ToolError)
	if !ok || typed.Code != "query_required" {
		t.Fatalf("unexpected error: %T %v", err, err)
	}
}

func TestBuildCodeIndexToolOutputFiltersAndTruncates(t *testing.T) {
	output := BuildCodeIndexToolOutput(CodeIndexToolOutputInput{
		Request:      CodeIndexRequest{Action: "search", Query: "run", Kind: "func", Limit: 1},
		Path:         "/work/src",
		RelativePath: "src",
		Symbols: []CodeIndexSymbol{
			{Name: "Run", Kind: "func", File: "src/a.go", Path: "/work/src/a.go", Line: 10, Signature: "func Run()"},
			{Name: "Runner", Kind: "func", File: "src/b.go", Path: "/work/src/b.go", Line: 20, Signature: "func Runner()"},
			{Name: "Other", Kind: "type", File: "src/c.go", Path: "/work/src/c.go", Line: 30, Signature: "type Other struct{}"},
		},
		SkippedProtected: []string{".analytix"},
		SkippedDirs:      []string{"node_modules"},
	})
	if output["symbol_count"] != 1 || output["truncated"] != true || output["result_limit_reached"] != true {
		t.Fatalf("code index output count/truncation mismatch: %#v", output)
	}
	rows, ok := output["symbols"].([]map[string]any)
	if !ok || len(rows) != 1 || rows[0]["name"] != "Run" {
		t.Fatalf("code index rows mismatch: %#v", output["symbols"])
	}
	if output["text"] == "(no symbols)" {
		t.Fatalf("code index text should describe matched symbols: %#v", output)
	}
}

func TestToolLimitsClampPositiveAndNonNegativeValues(t *testing.T) {
	if got := PositiveLimit(float64(500), 100, 200); got != 200 {
		t.Fatalf("expected positive limit to clamp to max, got %d", got)
	}
	if got := PositiveLimit(float64(-1), 100, 200); got != 100 {
		t.Fatalf("expected positive limit fallback, got %d", got)
	}
	if got := BoundedNonNegativeInteger(float64(-1), 2, 20); got != 2 {
		t.Fatalf("expected non-negative integer fallback, got %d", got)
	}
	if got := BoundedNonNegativeInteger(float64(99), 2, 20); got != 20 {
		t.Fatalf("expected non-negative integer clamp, got %d", got)
	}
}
