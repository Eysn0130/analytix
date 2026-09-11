package filetools

import "testing"

func TestGlobMatchesDoubleStarAndBasename(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"src/**/*.go", "src/runtime/server.go", true},
		{"*.md", "docs/readme.md", true},
		{"*.md", "docs/readme.txt", false},
		{"src/*.go", "src/internal/server.go", false},
	}
	for _, tc := range cases {
		if got := GlobMatches(tc.pattern, tc.path); got != tc.want {
			t.Fatalf("GlobMatches(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

func TestNormalizeGlobRejectsEscapesAndAbsolutes(t *testing.T) {
	if got, ok := NormalizeRequiredGlob(`./src\**\*.go`); !ok || got != "src/**/*.go" {
		t.Fatalf("normalized glob = %q, %v", got, ok)
	}
	for _, pattern := range []string{"", "/tmp/*.go", "../*.go"} {
		if got, ok := NormalizeRequiredGlob(pattern); ok {
			t.Fatalf("NormalizeRequiredGlob(%q) = %q, true", pattern, got)
		}
	}
	if got, ok := NormalizeRequiredGlob("src/../*.go"); !ok || got != "*.go" {
		t.Fatalf("NormalizeRequiredGlob cleans in-tree parent segments = %q, %v", got, ok)
	}
	if got, ok := NormalizeOptionalGlob(""); !ok || got != "" {
		t.Fatalf("NormalizeOptionalGlob empty = %q, %v", got, ok)
	}
}

func TestNewLineMatcherLiteralAndRegex(t *testing.T) {
	literal, err := NewLineMatcher("hello", true, true)
	if err != nil {
		t.Fatalf("literal matcher: %v", err)
	}
	if ok, col := literal("Say Hello"); !ok || col != 5 {
		t.Fatalf("literal match = %v, %d", ok, col)
	}
	regex, err := NewLineMatcher(`h.llo`, true, false)
	if err != nil {
		t.Fatalf("regex matcher: %v", err)
	}
	if ok, col := regex("Say Hello"); !ok || col != 5 {
		t.Fatalf("regex match = %v, %d", ok, col)
	}
}

func TestSearchSkipPolicies(t *testing.T) {
	if !GlobSkipDirName("node_modules") {
		t.Fatalf("GlobSkipDirName(node_modules) = false")
	}
	if GlobSkipDirName("src") {
		t.Fatalf("GlobSkipDirName(src) = true")
	}
	if !GrepSkipDirName(".next") {
		t.Fatalf("GrepSkipDirName(.next) = false")
	}
	if GrepSkipDirName("src") {
		t.Fatalf("GrepSkipDirName(src) = true")
	}
	if !HiddenSearchEntry(".cache") {
		t.Fatalf("HiddenSearchEntry(.cache) = false")
	}
	if HiddenSearchEntry("src") {
		t.Fatalf("HiddenSearchEntry(src) = true")
	}
}

func TestSearchPathMatchesOutputUsesStableWireKeys(t *testing.T) {
	out := SearchPathMatchesOutput([]SearchPathMatch{{Path: "/work/a.go", RelativePath: "a.go"}})
	if len(out) != 1 || out[0]["path"] != "/work/a.go" || out[0]["relative_path"] != "a.go" {
		t.Fatalf("search output mismatch: %#v", out)
	}
}

func TestBuildListAndSearchToolOutputs(t *testing.T) {
	list := BuildListDirToolOutput(ListDirToolOutputInput{
		Path:         "/work",
		RelativePath: ".",
		Entries:      []ListDirEntry{{Name: "src", DisplayName: "src/", Kind: "directory"}},
		Names:        []string{"src/"},
		Truncated:    true,
		Limit:        1,
	})
	if list["entry_limit_reached"] != float64(1) || list["truncated"] != true {
		t.Fatalf("list output mismatch: %#v", list)
	}
	find := BuildFindToolOutput(FindToolOutputInput{
		Path:             "/work",
		RelativePath:     ".",
		Pattern:          "main",
		Matches:          []SearchPathMatch{{Path: "/work/main.go", RelativePath: "main.go"}},
		SkippedProtected: []string{".analytix"},
		Truncated:        false,
		Limit:            200,
	})
	if find["pattern"] != "main" || find["result_limit_reached"] != float64(200) {
		t.Fatalf("find output mismatch: %#v", find)
	}
	glob := BuildGlobToolOutput(GlobToolOutputInput{
		Path:              "/work",
		RelativePath:      ".",
		Pattern:           "**/*.go",
		NormalizedPattern: "**/*.go",
		Matches:           []SearchPathMatch{{Path: "/work/main.go", RelativePath: "main.go"}},
		SkippedDirs:       []string{"node_modules"},
		Limit:             1000,
	})
	if glob["normalized_pattern"] != "**/*.go" || glob["result_limit_reached"] != float64(1000) {
		t.Fatalf("glob output mismatch: %#v", glob)
	}
}

func TestBuildGrepToolOutputIncludesContextWhenRequested(t *testing.T) {
	output := BuildGrepToolOutput(GrepToolOutputInput{
		Path:         "/work",
		RelativePath: ".",
		Pattern:      "needle",
		ContextLines: 1,
		Matches: []GrepMatch{{
			Path:          "/work/a.txt",
			RelativePath:  "a.txt",
			Line:          2,
			Column:        3,
			Text:          "needle",
			ContextBefore: []string{"before"},
			ContextAfter:  []string{"after"},
		}},
		Limit: 100,
	})
	matches, ok := output["matches"].([]map[string]any)
	if !ok || len(matches) != 1 || matches[0]["context_before"] == nil || matches[0]["context_after"] == nil {
		t.Fatalf("grep output mismatch: %#v", output)
	}
}
