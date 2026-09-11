package filetools

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
)

type CodeIndexRequest struct {
	Action string
	Path   string
	Query  string
	Kind   string
	Limit  int
}

type CodeIndexSymbol struct {
	Name      string
	Kind      string
	File      string
	Path      string
	Line      int
	Parent    string
	Signature string
}

type CodeIndexToolOutputInput struct {
	Request          CodeIndexRequest
	Path             string
	RelativePath     string
	Symbols          []CodeIndexSymbol
	SkippedProtected []string
	SkippedDirs      []string
	Truncated        bool
}

func ParseCodeIndexRequest(args map[string]any, defaultLimit int, maxLimit int) (CodeIndexRequest, error) {
	request := CodeIndexRequest{
		Action: strings.ToLower(strings.TrimSpace(firstNonEmptyAnyString(args["action"]))),
		Path:   strings.TrimSpace(firstNonEmptyAnyString(args["path"], ".")),
		Query:  strings.TrimSpace(firstNonEmptyAnyString(args["query"])),
		Kind:   strings.ToLower(strings.TrimSpace(firstNonEmptyAnyString(args["kind"]))),
		Limit:  PositiveLimit(args["limit"], defaultLimit, maxLimit),
	}
	if request.Action != "outline" && request.Action != "search" {
		return request, ToolError{Code: "validation_error", Message: "action must be outline or search"}
	}
	if request.Path == "" {
		request.Path = "."
	}
	if request.Action == "search" && request.Query == "" {
		return request, ToolError{Code: "query_required", Message: "query is required for action=search"}
	}
	return request, nil
}

func CodeIndexHasFilter(request CodeIndexRequest) bool {
	return strings.TrimSpace(request.Kind) != "" || strings.TrimSpace(request.Query) != ""
}

func FilterCodeIndexSymbols(symbols []CodeIndexSymbol, request CodeIndexRequest) []CodeIndexSymbol {
	query := strings.ToLower(strings.TrimSpace(request.Query))
	kind := strings.ToLower(strings.TrimSpace(request.Kind))
	out := make([]CodeIndexSymbol, 0, len(symbols))
	for _, symbol := range symbols {
		if kind != "" && strings.ToLower(symbol.Kind) != kind {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(symbol.Name + " " + symbol.Parent + " " + symbol.Signature)
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		out = append(out, symbol)
	}
	return out
}

func BuildCodeIndexToolOutput(input CodeIndexToolOutputInput) map[string]any {
	request := input.Request
	symbols := FilterCodeIndexSymbols(input.Symbols, request)
	truncated := input.Truncated
	if len(symbols) > request.Limit {
		symbols = symbols[:request.Limit]
		truncated = true
	}
	rows := make([]map[string]any, 0, len(symbols))
	for _, symbol := range symbols {
		rows = append(rows, map[string]any{
			"name":      symbol.Name,
			"kind":      symbol.Kind,
			"file":      symbol.File,
			"path":      symbol.Path,
			"line":      symbol.Line,
			"parent":    symbol.Parent,
			"signature": symbol.Signature,
		})
	}
	return map[string]any{
		"action":               request.Action,
		"path":                 input.Path,
		"relative_path":        input.RelativePath,
		"query":                request.Query,
		"kind":                 request.Kind,
		"limit":                request.Limit,
		"symbols":              rows,
		"symbol_count":         len(rows),
		"text":                 FormatCodeIndexSymbols(symbols, truncated),
		"skipped_protected":    append([]string(nil), input.SkippedProtected...),
		"skipped_directories":  append([]string(nil), input.SkippedDirs...),
		"truncated":            truncated,
		"result_limit_reached": len(symbols) >= request.Limit,
	}
}

func FormatCodeIndexSymbols(symbols []CodeIndexSymbol, truncated bool) string {
	if len(symbols) == 0 {
		return "(no symbols)"
	}
	var builder strings.Builder
	for _, symbol := range symbols {
		name := symbol.Name
		if symbol.Parent != "" {
			name = symbol.Parent + "." + name
		}
		if symbol.Signature != "" {
			fmt.Fprintf(&builder, "%s:%d: %s %s - %s\n", symbol.File, symbol.Line, symbol.Kind, name, symbol.Signature)
		} else {
			fmt.Fprintf(&builder, "%s:%d: %s %s\n", symbol.File, symbol.Line, symbol.Kind, name)
		}
	}
	if truncated {
		builder.WriteString("... (truncated; narrow path/query/kind or raise limit)\n")
	}
	return strings.TrimRight(builder.String(), "\n")
}

func SupportedCodeIndexFileExt(ext string) bool {
	switch strings.ToLower(strings.TrimSpace(ext)) {
	case ".go", ".js", ".jsx", ".ts", ".tsx", ".py", ".java", ".kt", ".kts", ".cs", ".rs", ".c", ".cc", ".cpp", ".h", ".hpp":
		return true
	default:
		return false
	}
}

func SkipCodeIndexDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "__pycache__", ".idea", ".vscode", ".next", "dist", "build", "target", "coverage":
		return true
	default:
		return false
	}
}

func ParseCodeIndexGo(file string, path string, data []byte) ([]CodeIndexSymbol, error) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, data, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	out := []CodeIndexSymbol{}
	add := func(name string, kind string, pos token.Pos, parent string, signature string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		out = append(out, CodeIndexSymbol{
			Name:      name,
			Kind:      kind,
			File:      file,
			Path:      path,
			Line:      fset.Position(pos).Line,
			Parent:    parent,
			Signature: signature,
		})
	}
	for _, decl := range parsed.Decls {
		switch typed := decl.(type) {
		case *ast.FuncDecl:
			parent := ""
			kind := "func"
			if typed.Recv != nil && len(typed.Recv.List) > 0 {
				parent = codeIndexExprName(typed.Recv.List[0].Type)
				kind = "method"
			}
			add(typed.Name.Name, kind, typed.Name.Pos(), parent, codeIndexGoSignature(typed, parent))
		case *ast.GenDecl:
			for _, spec := range typed.Specs {
				switch specTyped := spec.(type) {
				case *ast.TypeSpec:
					kind := "type"
					switch specTyped.Type.(type) {
					case *ast.StructType:
						kind = "struct"
					case *ast.InterfaceType:
						kind = "interface"
					}
					add(specTyped.Name.Name, kind, specTyped.Name.Pos(), "", kind+" "+specTyped.Name.Name)
				case *ast.ValueSpec:
					kind := strings.ToLower(typed.Tok.String())
					for _, name := range specTyped.Names {
						add(name.Name, kind, name.Pos(), "", kind+" "+name.Name)
					}
				}
			}
		}
	}
	return out, nil
}

func ParseCodeIndexText(ext string, file string, path string, content string) []CodeIndexSymbol {
	out := []CodeIndexSymbol{}
	for index, line := range SplitTextLines(content, false) {
		for _, matcher := range codeIndexMatchers(strings.ToLower(strings.TrimSpace(ext))) {
			symbol, ok := matcher.match(line)
			if !ok {
				continue
			}
			symbol.File = file
			symbol.Path = path
			symbol.Line = index + 1
			out = append(out, symbol)
			break
		}
	}
	return out
}

func codeIndexExprName(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.StarExpr:
		return codeIndexExprName(typed.X)
	case *ast.SelectorExpr:
		prefix := codeIndexExprName(typed.X)
		if prefix == "" {
			return typed.Sel.Name
		}
		return prefix + "." + typed.Sel.Name
	default:
		return ""
	}
}

func codeIndexGoSignature(fn *ast.FuncDecl, parent string) string {
	if parent == "" {
		return "func " + fn.Name.Name
	}
	return "func (" + parent + ") " + fn.Name.Name
}

type codeIndexMatcher struct {
	kind      string
	re        *regexp.Regexp
	nameGroup int
	kindGroup int
}

func (matcher codeIndexMatcher) match(line string) (CodeIndexSymbol, bool) {
	match := matcher.re.FindStringSubmatch(line)
	if match == nil || matcher.nameGroup >= len(match) {
		return CodeIndexSymbol{}, false
	}
	name := strings.TrimSpace(match[matcher.nameGroup])
	if name == "" {
		return CodeIndexSymbol{}, false
	}
	kind := matcher.kind
	if matcher.kindGroup > 0 && matcher.kindGroup < len(match) && strings.TrimSpace(match[matcher.kindGroup]) != "" {
		kind = normalizeCodeIndexKind(match[matcher.kindGroup])
	}
	return CodeIndexSymbol{Name: name, Kind: kind, Signature: strings.TrimSpace(line)}, true
}

func normalizeCodeIndexKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "function", "fn":
		return "func"
	default:
		return strings.ToLower(strings.TrimSpace(kind))
	}
}

var (
	codeIndexPyClass     = regexp.MustCompile(`^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	codeIndexPyFunc      = regexp.MustCompile(`^\s*(?:async\s+)?def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	codeIndexJSClass     = regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)\b`)
	codeIndexJSFunc      = regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`)
	codeIndexJSInterface = regexp.MustCompile(`^\s*(?:export\s+)?interface\s+([A-Za-z_$][A-Za-z0-9_$]*)\b`)
	codeIndexJSType      = regexp.MustCompile(`^\s*(?:export\s+)?type\s+([A-Za-z_$][A-Za-z0-9_$]*)\b`)
	codeIndexJSEnum      = regexp.MustCompile(`^\s*(?:export\s+)?enum\s+([A-Za-z_$][A-Za-z0-9_$]*)\b`)
	codeIndexJSArrow     = regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:async\s*)?(?:\([^)]*\)|[A-Za-z_$][A-Za-z0-9_$]*)\s*=>`)
	codeIndexJavaType    = regexp.MustCompile(`^\s*(?:public|protected|private|abstract|final|static|sealed|data|\s)*\s*(class|interface|enum|record|object)\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	codeIndexJavaMethod  = regexp.MustCompile(`^\s*(?:public|protected|private|static|final|abstract|synchronized|native|\s)+[\w<>\[\], ?]+\s+([A-Za-z_][A-Za-z0-9_]*)\s*\([^;]*\)\s*(?:\{|$)`)
	codeIndexRustItem    = regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?(fn|struct|enum|trait)\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	codeIndexCFunc       = regexp.MustCompile(`^\s*(?:[\w:*&<>\[\],]+\s+)+([A-Za-z_][A-Za-z0-9_]*)\s*\([^;]*\)\s*(?:\{|$)`)
)

func codeIndexMatchers(ext string) []codeIndexMatcher {
	switch ext {
	case ".py":
		return []codeIndexMatcher{{kind: "class", re: codeIndexPyClass, nameGroup: 1}, {kind: "func", re: codeIndexPyFunc, nameGroup: 1}}
	case ".js", ".jsx", ".ts", ".tsx":
		return []codeIndexMatcher{
			{kind: "class", re: codeIndexJSClass, nameGroup: 1},
			{kind: "func", re: codeIndexJSFunc, nameGroup: 1},
			{kind: "interface", re: codeIndexJSInterface, nameGroup: 1},
			{kind: "type", re: codeIndexJSType, nameGroup: 1},
			{kind: "enum", re: codeIndexJSEnum, nameGroup: 1},
			{kind: "func", re: codeIndexJSArrow, nameGroup: 1},
		}
	case ".java", ".kt", ".kts", ".cs":
		return []codeIndexMatcher{{re: codeIndexJavaType, nameGroup: 2, kindGroup: 1}, {kind: "method", re: codeIndexJavaMethod, nameGroup: 1}}
	case ".rs":
		return []codeIndexMatcher{{re: codeIndexRustItem, nameGroup: 2, kindGroup: 1}}
	case ".c", ".cc", ".cpp", ".h", ".hpp":
		return []codeIndexMatcher{{kind: "func", re: codeIndexCFunc, nameGroup: 1}}
	default:
		return nil
	}
}
