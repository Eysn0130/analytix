package filetools

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"
)

const (
	ReadModeWindow   = "line_window"
	ReadModeNumbered = "numbered"
	DefaultReadLimit = 2000

	TextEncodingUTF8         = "utf8"
	TextEncodingUTF8BOM      = "utf8-bom"
	TextEncodingUTF16LE      = "utf16le"
	TextEncodingUTF16BE      = "utf16be"
	TextEncodingUTF16LENoBOM = "utf16le-nobom"
	TextEncodingUTF16BENoBOM = "utf16be-nobom"
)

type ReadTextView struct {
	Content     string
	RawContent  string
	StartLine   int
	EndLine     int
	TotalLines  int
	Truncated   bool
	TruncatedBy string
}

type ReadTextToolOutputInput struct {
	Path         string
	RelativePath string
	Encoding     string
	View         ReadTextView
}

func BuildReadTextToolOutput(input ReadTextToolOutputInput) map[string]any {
	output := map[string]any{
		"path":          input.Path,
		"relative_path": input.RelativePath,
		"content":       input.View.Content,
		"encoding":      input.Encoding,
		"start_line":    float64(input.View.StartLine),
		"end_line":      float64(input.View.EndLine),
		"total_lines":   float64(input.View.TotalLines),
		"truncated":     input.View.Truncated,
	}
	if input.View.TruncatedBy != "" {
		output["truncation_by"] = input.View.TruncatedBy
	} else {
		output["truncation_by"] = nil
	}
	return output
}

func ReadText(content string, offset int, limit int, mode string, hasOffset bool) ReadTextView {
	switch mode {
	case ReadModeNumbered:
		return NumberedReadTextView(content, offset, limit)
	default:
		if !hasOffset {
			offset = 1
		}
		return WindowReadTextView(content, offset, limit)
	}
}

func WindowReadTextView(content string, offset int, limit int) ReadTextView {
	lines := SplitTextLines(content, true)
	totalLines := len(lines)
	if totalLines == 0 {
		return ReadTextView{Content: "(empty file)", RawContent: "", TotalLines: 0}
	}
	startLine := offset
	if startLine < 1 {
		startLine = 1
	}
	if limit <= 0 {
		limit = DefaultReadLimit
	}
	startIndex := startLine - 1
	if startIndex >= totalLines {
		message := fmt.Sprintf("(offset %d is past EOF -- file has %d lines)", startLine, totalLines)
		return ReadTextView{Content: message, RawContent: "", StartLine: startLine, EndLine: startLine, TotalLines: totalLines}
	}
	endIndex := startIndex + limit
	if endIndex > totalLines {
		endIndex = totalLines
	}
	selected := strings.Join(lines[startIndex:endIndex], "\n")
	endLine := startLine + len(lines[startIndex:endIndex]) - 1
	hasMore := endIndex < totalLines
	display := selected
	if hasMore {
		display = fmt.Sprintf("%s\n\n[%d more lines in file. Use offset=%d to continue.]", selected, totalLines-endIndex, endLine+1)
	}
	truncatedBy := ""
	if hasMore {
		truncatedBy = "lines"
	}
	return ReadTextView{
		Content:     display,
		RawContent:  selected,
		StartLine:   startLine,
		EndLine:     endLine,
		TotalLines:  totalLines,
		Truncated:   hasMore,
		TruncatedBy: truncatedBy,
	}
}

func NumberedReadTextView(content string, offset int, limit int) ReadTextView {
	lines := SplitTextLines(content, false)
	totalLines := len(lines)
	if totalLines == 0 {
		return ReadTextView{Content: "(empty file)", RawContent: "", TotalLines: 0}
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = DefaultReadLimit
	}
	if offset >= totalLines {
		message := fmt.Sprintf("(offset %d is past EOF -- file has %d lines)", offset, totalLines)
		return ReadTextView{Content: message, RawContent: "", StartLine: offset + 1, EndLine: offset + 1, TotalLines: totalLines}
	}
	endIndex := offset + limit
	if endIndex > totalLines {
		endIndex = totalLines
	}
	selectedLines := lines[offset:endIndex]
	maxShown := offset + len(selectedLines)
	width := len(fmt.Sprint(maxShown))
	var builder strings.Builder
	for index, line := range selectedLines {
		fmt.Fprintf(&builder, "%*d→%s\n", width, offset+index+1, line)
	}
	hasMore := endIndex < totalLines
	if hasMore {
		fmt.Fprintf(&builder, "\n[more lines below; pass offset=%d to continue]\n", endIndex)
	}
	truncatedBy := ""
	if hasMore {
		truncatedBy = "lines"
	}
	return ReadTextView{
		Content:     builder.String(),
		RawContent:  strings.Join(selectedLines, "\n"),
		StartLine:   offset + 1,
		EndLine:     offset + len(selectedLines),
		TotalLines:  totalLines,
		Truncated:   hasMore,
		TruncatedBy: truncatedBy,
	}
}

func SplitTextLines(content string, keepFinalEmptyLine bool) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	if normalized == "" {
		return []string{}
	}
	lines := strings.Split(normalized, "\n")
	if !keepFinalEmptyLine && len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func DecodeTextBytes(data []byte) (string, string, bool) {
	switch {
	case bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}):
		return string(data[3:]), TextEncodingUTF8BOM, true
	case bytes.HasPrefix(data, []byte{0xff, 0xfe}):
		return decodeUTF16Bytes(data[2:], binary.LittleEndian), TextEncodingUTF16LE, true
	case bytes.HasPrefix(data, []byte{0xfe, 0xff}):
		return decodeUTF16Bytes(data[2:], binary.BigEndian), TextEncodingUTF16BE, true
	}
	if encoding, ok := DetectUTF16NoBOM(data); ok {
		switch encoding {
		case TextEncodingUTF16LENoBOM:
			return decodeUTF16Bytes(data, binary.LittleEndian), encoding, true
		case TextEncodingUTF16BENoBOM:
			return decodeUTF16Bytes(data, binary.BigEndian), encoding, true
		}
	}
	if BytesContainNUL(data) {
		return "", "", false
	}
	return string(data), TextEncodingUTF8, true
}

func EncodeTextBytes(content string, encoding string) []byte {
	switch encoding {
	case TextEncodingUTF8BOM:
		return append([]byte{0xef, 0xbb, 0xbf}, []byte(content)...)
	case TextEncodingUTF16LE:
		return append([]byte{0xff, 0xfe}, encodeUTF16Bytes(content, binary.LittleEndian)...)
	case TextEncodingUTF16BE:
		return append([]byte{0xfe, 0xff}, encodeUTF16Bytes(content, binary.BigEndian)...)
	case TextEncodingUTF16LENoBOM:
		return encodeUTF16Bytes(content, binary.LittleEndian)
	case TextEncodingUTF16BENoBOM:
		return encodeUTF16Bytes(content, binary.BigEndian)
	default:
		return []byte(content)
	}
}

func LooksUTF16Text(data []byte) bool {
	if bytes.HasPrefix(data, []byte{0xff, 0xfe}) || bytes.HasPrefix(data, []byte{0xfe, 0xff}) {
		return true
	}
	_, ok := DetectUTF16NoBOM(data)
	return ok
}

func DetectUTF16NoBOM(data []byte) (string, bool) {
	n := len(data)
	if n < 16 {
		return "", false
	}
	n &^= 1
	var evenNUL, oddNUL int
	for index := 0; index < n; index++ {
		if data[index] != 0 {
			continue
		}
		if index%2 == 0 {
			evenNUL++
		} else {
			oddNUL++
		}
	}
	half := n / 2
	switch {
	case oddNUL*10 >= half*3 && evenNUL*20 <= half:
		return TextEncodingUTF16LENoBOM, true
	case evenNUL*10 >= half*3 && oddNUL*20 <= half:
		return TextEncodingUTF16BENoBOM, true
	default:
		return "", false
	}
}

func BytesContainNUL(data []byte) bool {
	for _, item := range data {
		if item == 0 {
			return true
		}
	}
	return false
}

func decodeUTF16Bytes(data []byte, order binary.ByteOrder) string {
	if len(data)%2 == 1 {
		data = data[:len(data)-1]
	}
	units := make([]uint16, 0, len(data)/2)
	for index := 0; index+1 < len(data); index += 2 {
		units = append(units, order.Uint16(data[index:index+2]))
	}
	return string(utf16.Decode(units))
}

func encodeUTF16Bytes(content string, order binary.ByteOrder) []byte {
	units := utf16.Encode([]rune(content))
	out := make([]byte, len(units)*2)
	for index, unit := range units {
		order.PutUint16(out[index*2:index*2+2], unit)
	}
	return out
}
