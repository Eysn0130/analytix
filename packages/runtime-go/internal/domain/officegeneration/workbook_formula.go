package officegeneration

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

const workbookFormulaMaxWork = 1000000
const workbookFormulaMaxDepth = 64

var workbookFormulaBounds = errors.New("workbook formula exceeds supported bounded formula scope")
var workbookFormulaNumber = regexp.MustCompile(`^(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?`)

type workbookFormulaToken struct{ kind, text string }
type workbookFormulaReference struct {
	sheet string
	area  workbookRectangle
}
type workbookFormulaParser struct {
	tokens          []workbookFormulaToken
	position, depth int
	sheet           string
	names           map[string]bool
	references      []workbookFormulaReference
}

func workbookValidateFormulas(w Workbook, names map[string]bool) error {
	type formulaCell struct{ sheet, expression string }
	var formulas []formulaCell
	indices := map[string]map[int]int{}
	for _, sheet := range w.Sheets {
		name := workbookSheetKey(sheet.Name)
		indices[name] = map[int]int{}
		for _, cell := range sheet.Cells {
			if cell.Type != "formula" {
				continue
			}
			c, r, _, _ := workbookAddress(cell.Address)
			indices[name][r*WorkbookMaxColumns+c] = len(formulas)
			formulas = append(formulas, formulaCell{name, cell.Formula})
		}
	}
	dependencies := make([][]int, len(formulas))
	work := 0
	for index, cell := range formulas {
		tokens, err := workbookFormulaTokens(cell.expression)
		if err != nil {
			return err
		}
		parser := workbookFormulaParser{tokens: tokens, sheet: cell.sheet, names: names}
		if err := parser.expression(0); err != nil {
			return err
		}
		if parser.current().kind != "end" {
			return workbookInvalid
		}
		seen := map[int]bool{}
		for _, reference := range parser.references {
			work += reference.area.size()
			if work > workbookFormulaMaxWork {
				return workbookFormulaBounds
			}
			for row := reference.area.r1; row <= reference.area.r2; row++ {
				for col := reference.area.c1; col <= reference.area.c2; col++ {
					if target, ok := indices[reference.sheet][row*WorkbookMaxColumns+col]; ok && !seen[target] {
						seen[target] = true
						dependencies[index] = append(dependencies[index], target)
					}
				}
			}
		}
	}
	state, heights := make([]int, len(formulas)), make([]int, len(formulas))
	var visit func(int, int) (int, error)
	visit = func(index, depth int) (int, error) {
		if depth > workbookFormulaMaxDepth {
			return 0, workbookFormulaBounds
		}
		if state[index] == 1 {
			return 0, errors.New("workbook formula contains a circular reference")
		}
		if state[index] == 2 {
			return heights[index], nil
		}
		state[index] = 1
		height := 1
		for _, target := range dependencies[index] {
			h, err := visit(target, depth+1)
			if err != nil {
				return 0, err
			}
			if h+1 > height {
				height = h + 1
			}
		}
		if height > workbookFormulaMaxDepth {
			return 0, workbookFormulaBounds
		}
		state[index], heights[index] = 2, height
		return height, nil
	}
	for index := range formulas {
		if _, err := visit(index, 1); err != nil {
			return err
		}
	}
	return nil
}

func workbookFormulaTokens(expression string) ([]workbookFormulaToken, error) {
	expression = strings.TrimSpace(expression)
	expression = strings.TrimPrefix(expression, "=")
	var tokens []workbookFormulaToken
	for len(expression) > 0 {
		c := expression[0]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			expression = expression[1:]
			continue
		}
		if c == '"' || c == '\'' {
			quote := c
			var text strings.Builder
			position, closed := 1, false
			for position < len(expression) {
				if expression[position] == quote {
					if position+1 < len(expression) && expression[position+1] == quote {
						text.WriteByte(quote)
						position += 2
						continue
					}
					position++
					closed = true
					break
				}
				text.WriteByte(expression[position])
				position++
			}
			if !closed {
				return nil, workbookInvalid
			}
			kind := "string"
			if quote == '\'' {
				kind = "sheet"
			}
			tokens = append(tokens, workbookFormulaToken{kind, text.String()})
			expression = expression[position:]
			continue
		}
		if (c >= '0' && c <= '9') || c == '.' {
			text := workbookFormulaNumber.FindString(expression)
			if text == "" {
				return nil, workbookInvalid
			}
			value, err := strconv.ParseFloat(text, 64)
			if err != nil || !workbookFinite(value) {
				return nil, workbookInvalid
			}
			tokens = append(tokens, workbookFormulaToken{"number", text})
			expression = expression[len(text):]
			continue
		}
		if workbookFormulaWord(c) {
			position := 1
			for position < len(expression) && workbookFormulaWord(expression[position]) {
				position++
			}
			tokens = append(tokens, workbookFormulaToken{"word", expression[:position]})
			expression = expression[position:]
			continue
		}
		if strings.ContainsRune("+-*/^%=<>()!,:", rune(c)) {
			text := expression[:1]
			if len(expression) > 1 && ((c == '<' && (expression[1] == '>' || expression[1] == '=')) || (c == '>' && expression[1] == '=')) {
				text = expression[:2]
			}
			tokens = append(tokens, workbookFormulaToken{text, text})
			expression = expression[len(text):]
			continue
		}
		return nil, workbookInvalid
	}
	return append(tokens, workbookFormulaToken{"end", ""}), nil
}
func workbookFormulaWord(c byte) bool {
	return c >= 128 || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '$'
}
func (p *workbookFormulaParser) current() workbookFormulaToken { return p.tokens[p.position] }
func (p *workbookFormulaParser) take(kind string) bool {
	if p.current().kind == kind {
		p.position++
		return true
	}
	return false
}
func workbookFormulaPrecedence(kind string) int {
	switch kind {
	case "=", "<>", "<", ">", "<=", ">=":
		return 1
	case "+", "-":
		return 2
	case "*", "/":
		return 3
	case "^":
		return 4
	}
	return -1
}
func (p *workbookFormulaParser) expression(minimum int) error {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > workbookFormulaMaxDepth {
		return workbookFormulaBounds
	}
	if err := p.atom(); err != nil {
		return err
	}
	for {
		if p.take("%") {
			continue
		}
		precedence := workbookFormulaPrecedence(p.current().kind)
		if precedence < minimum {
			return nil
		}
		p.position++
		if err := p.expression(precedence + 1); err != nil {
			return err
		}
	}
}
func (p *workbookFormulaParser) atom() error {
	if p.take("+") || p.take("-") {
		return p.expression(5)
	}
	if p.take("(") {
		if err := p.expression(0); err != nil {
			return err
		}
		if !p.take(")") {
			return workbookInvalid
		}
		return nil
	}
	token := p.current()
	if token.kind == "number" || token.kind == "string" {
		p.position++
		return nil
	}
	if token.kind != "word" && token.kind != "sheet" {
		return workbookInvalid
	}
	p.position++
	if token.kind == "word" && p.take("(") {
		return p.function(strings.ToUpper(token.text))
	}
	sheet, address := p.sheet, token.text
	if p.take("!") {
		sheet = workbookSheetKey(token.text)
		if !p.names[sheet] || p.current().kind != "word" {
			return workbookInvalid
		}
		address = p.current().text
		p.position++
	} else if token.kind == "sheet" {
		return workbookInvalid
	} else if strings.EqualFold(token.text, "TRUE") || strings.EqualFold(token.text, "FALSE") {
		return nil
	}
	if p.take(":") {
		if p.current().kind != "word" {
			return workbookInvalid
		}
		address += ":" + p.current().text
		p.position++
	}
	area, err := workbookRange(address)
	if err != nil {
		return err
	}
	p.references = append(p.references, workbookFormulaReference{sheet, area})
	return nil
}
func (p *workbookFormulaParser) function(name string) error {
	minimum, maximum := 1, 255
	switch name {
	case "SUM", "AVERAGE", "MIN", "MAX", "COUNT", "COUNTA":
	case "COUNTIF", "ROUND":
		minimum, maximum = 2, 2
	case "SUMIF", "IF":
		minimum, maximum = 2, 3
	case "ABS":
		minimum, maximum = 1, 1
	default:
		return errors.New("workbook formula function is not supported")
	}
	count := 0
	var sumRanges []workbookRectangle
	if p.take(")") {
		return workbookInvalid
	}
	for {
		start := p.position
		if err := p.expression(0); err != nil {
			return err
		}
		count++
		if name == "SUMIF" && (count == 1 || count == 3) {
			reference, err := p.staticRange(p.tokens[start:p.position])
			if err != nil {
				return err
			}
			sumRanges = append(sumRanges, reference.area)
		}
		if count > maximum {
			return workbookInvalid
		}
		if p.take(")") {
			break
		}
		if !p.take(",") {
			return workbookInvalid
		}
	}
	if len(sumRanges) == 2 && (sumRanges[0].r2-sumRanges[0].r1 != sumRanges[1].r2-sumRanges[1].r1 || sumRanges[0].c2-sumRanges[0].c1 != sumRanges[1].c2-sumRanges[1].c1) {
		return workbookInvalid
	}
	if count < minimum {
		return workbookInvalid
	}
	return nil
}

// SUMIF must not implicitly enlarge sum_range beyond the references whose
// bounds and dependencies Core validated. Only explicit, equally shaped ranges
// are admitted; a native reader must use the same cells as the calculator.
func (p *workbookFormulaParser) staticRange(tokens []workbookFormulaToken) (workbookFormulaReference, error) {
	for _, token := range tokens {
		if token.kind != "word" && token.kind != "sheet" && token.kind != "!" && token.kind != ":" {
			return workbookFormulaReference{}, workbookInvalid
		}
	}
	copyTokens := append(append([]workbookFormulaToken{}, tokens...), workbookFormulaToken{"end", ""})
	reader := workbookFormulaParser{tokens: copyTokens, sheet: p.sheet, names: p.names}
	if reader.atom() != nil || reader.current().kind != "end" || len(reader.references) != 1 {
		return workbookFormulaReference{}, workbookInvalid
	}
	return reader.references[0], nil
}
