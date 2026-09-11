package filetools

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

type DeleteSymbolPreview struct {
	After            string
	Match            DeleteSymbolMatch
	FirstDeletedLine int
	DeletedLines     int
}

type DeleteSymbolMatch struct {
	Name   string
	Kind   string
	Parent string
	Line   int

	start    token.Pos
	docStart token.Pos
	end      token.Pos
	siblings []string
}

func PreviewDeleteSymbol(path string, content string, args map[string]any) (DeleteSymbolPreview, error) {
	name := strings.TrimSpace(firstNonEmptyAnyString(args["name"], args["symbol"], args["symbol_name"], args["symbolName"]))
	if name == "" {
		return DeleteSymbolPreview{}, ToolError{Code: "validation_error", Message: "name is required"}
	}
	kind := strings.TrimSpace(stringField(args, "kind"))
	parent := strings.TrimSpace(stringField(args, "parent"))
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, []byte(content), parser.ParseComments)
	if err != nil {
		return DeleteSymbolPreview{}, ToolError{Code: "parse_failed", Message: err.Error()}
	}
	match, err := findGoSymbol(fset, file, path, name, kind, parent)
	if err != nil {
		return DeleteSymbolPreview{}, err
	}
	after, firstDeletedLine, deletedLines := deleteSymbolLines(content, fset, match)
	return DeleteSymbolPreview{
		After:            after,
		Match:            match,
		FirstDeletedLine: firstDeletedLine,
		DeletedLines:     deletedLines,
	}, nil
}

func findGoSymbol(fset *token.FileSet, file *ast.File, path string, name string, kind string, parent string) (DeleteSymbolMatch, error) {
	matches := collectGoSymbols(fset, file)
	byName := make([]DeleteSymbolMatch, 0, 1)
	for _, match := range matches {
		if match.Name == name {
			byName = append(byName, match)
		}
	}
	if len(byName) == 0 {
		return DeleteSymbolMatch{}, ToolError{Code: "symbol_not_found", Message: fmt.Sprintf("symbol %q not found in %s", name, path)}
	}
	filtered := byName
	if kind != "" {
		byKind := make([]DeleteSymbolMatch, 0, len(filtered))
		for _, match := range filtered {
			if match.Kind == kind {
				byKind = append(byKind, match)
			}
		}
		if len(byKind) == 0 {
			return DeleteSymbolMatch{}, ToolError{Code: "symbol_not_found", Message: fmt.Sprintf("symbol %q with kind %q not found", name, kind)}
		}
		filtered = byKind
	}
	if parent != "" {
		byParent := make([]DeleteSymbolMatch, 0, len(filtered))
		for _, match := range filtered {
			if match.Parent == parent {
				byParent = append(byParent, match)
			}
		}
		if len(byParent) == 0 {
			return DeleteSymbolMatch{}, ToolError{Code: "symbol_not_found", Message: fmt.Sprintf("symbol %q (kind=%q parent=%q) not found", name, kind, parent)}
		}
		filtered = byParent
	}
	if len(filtered) > 1 {
		var builder strings.Builder
		fmt.Fprintf(&builder, "Multiple matches for %q; disambiguate with kind/parent:\n", name)
		for _, match := range filtered {
			fmt.Fprintf(&builder, "  line %d: %s %s", match.Line, match.Kind, match.Name)
			if match.Parent != "" {
				fmt.Fprintf(&builder, " (on %s)", match.Parent)
			}
			builder.WriteString("\n")
		}
		return DeleteSymbolMatch{}, ToolError{Code: "ambiguous_symbol", Message: strings.TrimSpace(builder.String())}
	}
	if len(filtered[0].siblings) > 1 {
		return DeleteSymbolMatch{}, ToolError{
			Code:    "multi_name_spec",
			Message: fmt.Sprintf("%s %q is declared in a multi-name %s spec with %s; delete_symbol refuses to remove it because that would also delete sibling symbols", filtered[0].Kind, name, filtered[0].Kind, strings.Join(filtered[0].siblings, ", ")),
		}
	}
	return filtered[0], nil
}

func collectGoSymbols(fset *token.FileSet, file *ast.File) []DeleteSymbolMatch {
	matches := []DeleteSymbolMatch{}
	for _, decl := range file.Decls {
		switch typed := decl.(type) {
		case *ast.FuncDecl:
			match := DeleteSymbolMatch{
				Name:  typed.Name.Name,
				Kind:  "func",
				start: typed.Pos(),
				end:   typed.End(),
				Line:  fset.Position(typed.Pos()).Line,
			}
			if typed.Doc != nil {
				match.docStart = typed.Doc.Pos()
			}
			if typed.Recv != nil && len(typed.Recv.List) > 0 {
				match.Kind = "method"
				match.Parent = receiverTypeName(typed.Recv.List[0].Type)
			}
			matches = append(matches, match)
		case *ast.GenDecl:
			for _, spec := range typed.Specs {
				switch specTyped := spec.(type) {
				case *ast.TypeSpec:
					match := DeleteSymbolMatch{
						Name:  specTyped.Name.Name,
						Kind:  "type",
						start: specTyped.Pos(),
						end:   specTyped.End(),
						Line:  fset.Position(specTyped.Pos()).Line,
					}
					if _, ok := specTyped.Type.(*ast.InterfaceType); ok {
						match.Kind = "interface"
					}
					if doc := goSpecDoc(typed, specTyped.Doc); doc != nil {
						match.docStart = doc.Pos()
					}
					matches = append(matches, match)
				case *ast.ValueSpec:
					kind := "var"
					if typed.Tok == token.CONST {
						kind = "const"
					}
					siblings := make([]string, 0, len(specTyped.Names))
					for _, ident := range specTyped.Names {
						siblings = append(siblings, ident.Name)
					}
					var docStart token.Pos
					if doc := goSpecDoc(typed, specTyped.Doc); doc != nil {
						docStart = doc.Pos()
					}
					for _, ident := range specTyped.Names {
						matches = append(matches, DeleteSymbolMatch{
							Name:     ident.Name,
							Kind:     kind,
							start:    ident.Pos(),
							docStart: docStart,
							end:      specTyped.End(),
							Line:     fset.Position(ident.Pos()).Line,
							siblings: append([]string(nil), siblings...),
						})
					}
				}
			}
		}
	}
	return matches
}

func receiverTypeName(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.StarExpr:
		if ident, ok := typed.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

func goSpecDoc(gen *ast.GenDecl, own *ast.CommentGroup) *ast.CommentGroup {
	if own != nil {
		return own
	}
	if gen.Lparen == token.NoPos {
		return gen.Doc
	}
	return nil
}

func deleteSymbolLines(content string, fset *token.FileSet, match DeleteSymbolMatch) (string, int, int) {
	start := match.start
	if match.docStart.IsValid() {
		start = match.docStart
	}
	startOffset := fset.Position(start).Offset
	endOffset := fset.Position(match.end).Offset
	lineStart := startOffset
	for lineStart > 0 && content[lineStart-1] != '\n' {
		lineStart--
	}
	lineEnd := endOffset
	for lineEnd < len(content) && content[lineEnd] != '\n' {
		lineEnd++
	}
	if lineEnd < len(content) {
		lineEnd++
	}
	deleted := content[lineStart:lineEnd]
	deletedLines := strings.Count(deleted, "\n")
	if deleted != "" && !strings.HasSuffix(deleted, "\n") {
		deletedLines++
	}
	if deletedLines == 0 && deleted != "" {
		deletedLines = 1
	}
	firstDeletedLine := 1 + strings.Count(content[:lineStart], "\n")
	return content[:lineStart] + content[lineEnd:], firstDeletedLine, deletedLines
}

func stringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}
