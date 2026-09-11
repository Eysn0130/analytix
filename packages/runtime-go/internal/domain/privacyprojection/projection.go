package privacyprojection

import (
	"errors"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const Version = "analytix.ordinary-privacy-projection/v1"

const maxProjectionPasses = 8

type Kind string

const (
	KindAccount Kind = "account"
	KindID      Kind = "identity_number"
	KindPhone   Kind = "phone_number"
	KindEmail   Kind = "email"
	KindMAC     Kind = "mac_address"
	KindIP      Kind = "ip_address"
	KindDevice  Kind = "device_identifier"
	KindAddress Kind = "address"
	KindPerson  Kind = "person"
	KindNumber  Kind = "restricted_number"
)

var ErrRestrictedPII = errors.New("ordinary projection contains restricted PII")

type Finding struct {
	Kind      Kind
	StartByte int
	EndByte   int
}

type Projection struct {
	Text     string
	Findings []Finding
}

type runeUnit struct {
	r     rune
	start int
	end   int
}

type span struct {
	kind  Kind
	start int
	end   int
}

type labelSpec struct {
	label string
	kind  Kind
}

var labeledPII = []labelSpec{
	{label: "对手银行卡号", kind: KindAccount},
	{label: "对手账户", kind: KindAccount},
	{label: "对手账号", kind: KindAccount},
	{label: "银行卡号", kind: KindAccount},
	{label: "身份证号", kind: KindID},
	{label: "设备标识", kind: KindDevice},
	{label: "设备指纹", kind: KindDevice},
	{label: "开户地址", kind: KindAddress},
	{label: "电子邮箱", kind: KindEmail},
	{label: "持卡人", kind: KindPerson},
	{label: "开户人", kind: KindPerson},
	{label: "手机号", kind: KindPhone},
	{label: "证件号", kind: KindID},
	{label: "MAC地址", kind: KindMAC},
	{label: "IP地址", kind: KindIP},
	{label: "account number", kind: KindAccount},
	{label: "card number", kind: KindAccount},
	{label: "identity number", kind: KindID},
	{label: "device identifier", kind: KindDevice},
	{label: "email address", kind: KindEmail},
	{label: "phone number", kind: KindPhone},
	{label: "账号", kind: KindAccount},
	{label: "帐号", kind: KindAccount},
	{label: "账户", kind: KindAccount},
	{label: "卡号", kind: KindAccount},
	{label: "身份证", kind: KindID},
	{label: "证件", kind: KindID},
	{label: "电话", kind: KindPhone},
	{label: "手机", kind: KindPhone},
	{label: "邮箱", kind: KindEmail},
	{label: "住址", kind: KindAddress},
	{label: "地址", kind: KindAddress},
	{label: "姓名", kind: KindPerson},
	{label: "户名", kind: KindPerson},
	{label: "account", kind: KindAccount},
	{label: "card", kind: KindAccount},
	{label: "phone", kind: KindPhone},
	{label: "email", kind: KindEmail},
	{label: "address", kind: KindAddress},
	{label: "imei", kind: KindDevice},
}

func ProjectText(text string) Projection {
	first := projectTextOnce(text)
	if len(first.Findings) == 0 {
		return first
	}
	current := first.Text
	for pass := 1; pass < maxProjectionPasses; pass++ {
		next := projectTextOnce(current)
		if len(next.Findings) == 0 {
			return Projection{Text: current, Findings: first.Findings}
		}
		if next.Text == current {
			break
		}
		current = next.Text
	}
	// Projection must be a fixed point before ordinary output can cross a
	// process, persistence, provider, or UI boundary. A newly exposed cascade
	// that does not converge within the deterministic pass budget is withheld
	// in full; returning a partially projected string would be fail-open.
	return Projection{
		Text:     Placeholder(KindNumber),
		Findings: []Finding{{Kind: KindNumber, StartByte: 0, EndByte: len(text)}},
	}
}

func projectTextOnce(text string) Projection {
	if text == "" {
		return Projection{Text: text, Findings: []Finding{}}
	}
	if !utf8.ValidString(text) {
		return Projection{Text: "[NUMBER]", Findings: []Finding{{Kind: KindNumber, StartByte: 0, EndByte: len(text)}}}
	}
	units := textRuneUnits(text)
	spans := make([]span, 0, 8)
	spans = append(spans, findLabeledPII(text, units)...)
	spans = append(spans, findEmails(units)...)
	spans = append(spans, findMACs(units)...)
	spans = append(spans, findIPs(text, units)...)
	spans = append(spans, findUUIDDevices(text, units)...)
	spans = append(spans, findDigitIdentifiers(text, units)...)
	spans = mergeSpans(spans)
	if len(spans) == 0 {
		return Projection{Text: text, Findings: []Finding{}}
	}
	var out strings.Builder
	out.Grow(len(text))
	findings := make([]Finding, 0, len(spans))
	cursor := 0
	for _, item := range spans {
		if item.start < cursor || item.start < 0 || item.end > len(text) || item.start >= item.end {
			continue
		}
		out.WriteString(text[cursor:item.start])
		out.WriteString(Placeholder(item.kind))
		findings = append(findings, Finding{Kind: item.kind, StartByte: item.start, EndByte: item.end})
		cursor = item.end
	}
	out.WriteString(text[cursor:])
	return Projection{Text: out.String(), Findings: findings}
}

func ContainsRestrictedPII(text string) bool {
	return len(ProjectText(text).Findings) > 0
}

func ValidateOrdinaryText(text string) error {
	if ContainsRestrictedPII(text) {
		return ErrRestrictedPII
	}
	return nil
}

func Placeholder(kind Kind) string {
	switch kind {
	case KindAccount:
		return "[ACCOUNT]"
	case KindID:
		return "[ID_NO]"
	case KindPhone:
		return "[PHONE]"
	case KindEmail:
		return "[EMAIL]"
	case KindMAC:
		return "[MAC]"
	case KindIP:
		return "[IP]"
	case KindDevice:
		return "[DEVICE]"
	case KindAddress:
		return "[ADDRESS]"
	case KindPerson:
		return "[PERSON]"
	default:
		return "[NUMBER]"
	}
}

func textRuneUnits(text string) []runeUnit {
	units := make([]runeUnit, 0, utf8.RuneCountInString(text))
	for offset, character := range text {
		size := utf8.RuneLen(character)
		if size < 1 {
			size = 1
		}
		units = append(units, runeUnit{r: character, start: offset, end: offset + size})
	}
	return units
}

func findLabeledPII(text string, units []runeUnit) []span {
	result := []span{}
	for _, spec := range labeledPII {
		needle := []rune(spec.label)
		searchAt := 0
		for searchAt < len(units) {
			if !asciiFoldedRunesAt(units, searchAt, needle) {
				searchAt++
				continue
			}
			labelEnd := searchAt + len(needle)
			searchAt = labelEnd
			cursor := labelEnd
			for cursor < len(units) && isInlineSpaceOrFormat(units[cursor].r) {
				cursor++
			}
			if cursor >= len(units) || (units[cursor].r != ':' && units[cursor].r != '\uff1a' && units[cursor].r != '=') {
				continue
			}
			cursor++
			for cursor < len(units) && isInlineSpaceOrFormat(units[cursor].r) {
				cursor++
			}
			if cursor >= len(units) {
				continue
			}
			valueStart := units[cursor].start
			end := cursor
			for end < len(units) && !labeledValueDelimiterAt(text, units, end, spec.kind) {
				end++
			}
			valueEnd := len(text)
			if end < len(units) {
				valueEnd = units[end].start
			}
			for valueEnd > valueStart {
				character, size := utf8.DecodeLastRuneInString(text[valueStart:valueEnd])
				if size <= 0 || !isInlineSpaceOrFormat(character) {
					break
				}
				valueEnd -= size
			}
			value := strings.TrimSpace(text[valueStart:valueEnd])
			if value == "" || isSafePlaceholder(value) {
				continue
			}
			result = append(result, span{kind: spec.kind, start: valueStart, end: valueEnd})
		}
	}
	return result
}

func asciiFoldedRunesAt(units []runeUnit, start int, needle []rune) bool {
	if len(needle) == 0 || start < 0 || start+len(needle) > len(units) {
		return false
	}
	for index, expected := range needle {
		if asciiFoldRune(units[start+index].r) != asciiFoldRune(expected) {
			return false
		}
	}
	return true
}

func asciiFoldRune(character rune) rune {
	if character >= 'A' && character <= 'Z' {
		return character + ('a' - 'A')
	}
	return character
}

func findEmails(units []runeUnit) []span {
	result := []span{}
	for index, unit := range units {
		if unit.r != '@' {
			continue
		}
		left := index - 1
		for left >= 0 && isEmailLocalRune(units[left].r) {
			left--
		}
		right := index + 1
		for right < len(units) && isEmailDomainRune(units[right].r) {
			right++
		}
		left++
		if left >= index || right <= index+1 || units[index+1].r == '.' || units[right-1].r == '.' {
			continue
		}
		hasDot := false
		for cursor := index + 1; cursor < right; cursor++ {
			if units[cursor].r == '.' {
				hasDot = true
				break
			}
		}
		if hasDot {
			result = append(result, span{kind: KindEmail, start: units[left].start, end: units[right-1].end})
		}
	}
	return result
}

func findMACs(units []runeUnit) []span {
	result := []span{}
	for start := 0; start+16 < len(units); start++ {
		separator := units[start+2].r
		if separator != ':' && separator != '-' {
			continue
		}
		matched := true
		for offset := 0; offset < 17; offset++ {
			character := units[start+offset].r
			if offset%3 == 2 {
				matched = matched && character == separator
			} else {
				matched = matched && isASCIIHex(character)
			}
		}
		if !matched || (start > 0 && isASCIIHex(units[start-1].r)) || (start+17 < len(units) && isASCIIHex(units[start+17].r)) {
			continue
		}
		result = append(result, span{kind: KindMAC, start: units[start].start, end: units[start+16].end})
		start += 16
	}
	return result
}

func findIPs(text string, units []runeUnit) []span {
	result := []span{}
	for start := 0; start < len(units); {
		if !isIPTokenRune(units[start].r) {
			start++
			continue
		}
		end := start + 1
		for end < len(units) && isIPTokenRune(units[end].r) {
			end++
		}
		first, last := start, end
		for first < last && (units[first].r == ':' || units[first].r == '.') {
			first++
		}
		for last > first && (units[last-1].r == ':' || units[last-1].r == '.') {
			last--
		}
		if first < last {
			candidate := text[units[first].start:units[last-1].end]
			parsed := net.ParseIP(candidate)
			if strings.ContainsAny(candidate, ".:") && parsed != nil && !parsed.IsLoopback() && !parsed.IsUnspecified() {
				result = append(result, span{kind: KindIP, start: units[first].start, end: units[last-1].end})
			}
		}
		start = end
	}
	return result
}

func findUUIDDevices(text string, units []runeUnit) []span {
	result := []span{}
	for start := 0; start+35 < len(units); start++ {
		matched := true
		for offset := 0; offset < 36; offset++ {
			character := units[start+offset].r
			switch offset {
			case 8, 13, 18, 23:
				matched = matched && character == '-'
			default:
				matched = matched && isASCIIHex(character)
			}
		}
		if !matched || !nearDeviceCue(text, units, start) {
			continue
		}
		result = append(result, span{kind: KindDevice, start: units[start].start, end: units[start+35].end})
		start += 35
	}
	return result
}

func findDigitIdentifiers(text string, units []runeUnit) []span {
	result := []span{}
	for start := 0; start < len(units); {
		if _, ok := decimalDigit(units[start].r); !ok {
			start++
			continue
		}
		if start > 0 {
			if _, previousDigit := decimalDigit(units[start-1].r); previousDigit {
				start++
				continue
			}
		}
		digits := make([]byte, 0, 32)
		groups := []int{}
		groupSize := 0
		hadSeparator := false
		end := start
		trailingX := false
		for end < len(units) {
			if digit, ok := decimalDigit(units[end].r); ok {
				digits = append(digits, digit)
				groupSize++
				end++
				continue
			}
			if (units[end].r == 'x' || units[end].r == 'X' || units[end].r == '\uff38' || units[end].r == '\uff58') && len(digits) == 17 {
				trailingX = true
				end++
				break
			}
			if isIdentifierSeparator(units[end].r) && len(digits) > 0 {
				if groupSize > 0 {
					groups = append(groups, groupSize)
					groupSize = 0
				}
				hadSeparator = true
				end++
				continue
			}
			if unicode.In(units[end].r, unicode.Cf) && len(digits) > 0 {
				end++
				continue
			}
			break
		}
		if groupSize > 0 {
			groups = append(groups, groupSize)
		}
		trimmedEnd := end
		for trimmedEnd > start && (isIdentifierSeparator(units[trimmedEnd-1].r) || unicode.In(units[trimmedEnd-1].r, unicode.Cf)) {
			trimmedEnd--
		}
		if trimmedEnd <= start || len(digits) == 0 {
			start++
			continue
		}
		kind, matched := classifyDigitIdentifier(text, units, start, digits, groups, hadSeparator, trailingX)
		if matched {
			result = append(result, span{kind: kind, start: units[start].start, end: units[trimmedEnd-1].end})
		}
		if end <= start {
			start++
		} else {
			start = end
		}
	}
	return result
}

func classifyDigitIdentifier(text string, units []runeUnit, start int, digits []byte, groups []int, hadSeparator bool, trailingX bool) (Kind, bool) {
	value := string(digits)
	if trailingX {
		value += "X"
	}
	if looksLikeChineseIdentity(value) {
		return KindID, true
	}
	if len(digits) == 11 && digits[0] == '1' && digits[1] >= '3' && digits[1] <= '9' {
		return KindPhone, true
	}
	if nearDeviceCue(text, units, start) && (len(digits) == 14 || len(digits) == 15 || len(digits) == 16) {
		return KindDevice, true
	}
	context := precedingContext(text, units, start, 32)
	accountCueIndex := lastCueIndex(context, []string{"账号", "帐号", "账户", "卡号", "银行卡", "account", "acct", "card"})
	measureCueIndex := lastCueIndex(context, []string{
		"金额", "余额", "合计", "总计", "数量", "笔数", "计数", "时间戳", "时间", "时点", "日期", "amount", "balance", "total", "count", "timestamp", "time", "date",
	})
	accountCue := accountCueIndex >= 0
	accountCueDominates := accountCue && accountCueIndex > measureCueIndex
	if len(digits) < 12 {
		if accountCueDominates && len(digits) >= 8 {
			return KindAccount, true
		}
		return "", false
	}
	if len(digits) > 32 {
		return KindNumber, true
	}
	if measureCueIndex >= 0 && !accountCueDominates {
		return "", false
	}
	if !accountCue && !hadSeparator && (looksLikeCalendarTimestamp(digits) || looksLikeEpochMillis(digits)) {
		return "", false
	}
	if hadSeparator && !wellGroupedIdentifier(groups) && !accountCue {
		return "", false
	}
	return KindAccount, true
}

func looksLikeChineseIdentity(value string) bool {
	if len(value) == 15 {
		if !asciiDigits(value) || value[:6] == "000000" {
			return false
		}
		_, err := time.Parse("060102", value[6:12])
		return err == nil
	}
	if len(value) != 18 || !asciiDigits(value[:17]) || value[:6] == "000000" {
		return false
	}
	if _, err := time.Parse("20060102", value[6:14]); err != nil {
		return false
	}
	weights := [...]int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
	checks := "10X98765432"
	sum := 0
	for index := 0; index < 17; index++ {
		sum += int(value[index]-'0') * weights[index]
	}
	return byte(unicode.ToUpper(rune(value[17]))) == checks[sum%11]
}

func looksLikeCalendarTimestamp(digits []byte) bool {
	if len(digits) != 12 && len(digits) != 14 {
		return false
	}
	layout := "200601021504"
	if len(digits) == 14 {
		layout = "20060102150405"
	}
	_, err := time.Parse(layout, string(digits))
	return err == nil
}

func looksLikeEpochMillis(digits []byte) bool {
	if len(digits) != 13 {
		return false
	}
	value, err := strconv.ParseInt(string(digits), 10, 64)
	return err == nil && value >= 946684800000 && value <= 4102444800000
}

func wellGroupedIdentifier(groups []int) bool {
	if len(groups) <= 1 {
		return true
	}
	for _, size := range groups {
		if size < 3 || size > 6 {
			return false
		}
	}
	return true
}

func nearDeviceCue(text string, units []runeUnit, start int) bool {
	context := precedingContext(text, units, start, 32)
	for _, cue := range []string{"设备", "终端", "imei", "device", "fingerprint", "mac"} {
		if strings.Contains(context, cue) {
			return true
		}
	}
	return false
}

func lastCueIndex(context string, cues []string) int {
	latest := -1
	for _, cue := range cues {
		searchAt := 0
		for searchAt < len(context) {
			relative := strings.Index(context[searchAt:], cue)
			if relative < 0 {
				break
			}
			index := searchAt + relative
			end := index + len(cue)
			if !asciiCue(cue) || (index == 0 || !asciiWordByte(context[index-1])) && (end == len(context) || !asciiWordByte(context[end])) {
				if index > latest {
					latest = index
				}
			}
			searchAt = end
		}
	}
	return latest
}

func asciiCue(value string) bool {
	if value == "" {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] < 'a' || value[index] > 'z' {
			return false
		}
	}
	return true
}

func asciiWordByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '_'
}

func precedingContext(text string, units []runeUnit, start, limit int) string {
	if start <= 0 || len(units) == 0 {
		return ""
	}
	left := start - limit
	if left < 0 {
		left = 0
	}
	return normalizeCueContext(text[units[left].start:units[start].start])
}

func normalizeCueContext(value string) string {
	var normalized strings.Builder
	normalized.Grow(len(value))
	for _, character := range value {
		if unicode.In(character, unicode.Cf) {
			continue
		}
		switch {
		case character >= '\uff01' && character <= '\uff5e':
			character -= '\uff01' - '!'
		case character == '\u3000':
			character = ' '
		}
		normalized.WriteRune(unicode.ToLower(character))
	}
	return normalized.String()
}

func mergeSpans(values []span) []span {
	filtered := make([]span, 0, len(values))
	for _, value := range values {
		if value.start >= 0 && value.end > value.start {
			filtered = append(filtered, value)
		}
	}
	sort.SliceStable(filtered, func(left, right int) bool {
		if filtered[left].start != filtered[right].start {
			return filtered[left].start < filtered[right].start
		}
		if filtered[left].end != filtered[right].end {
			return filtered[left].end > filtered[right].end
		}
		return kindPriority(filtered[left].kind) > kindPriority(filtered[right].kind)
	})
	merged := make([]span, 0, len(filtered))
	for _, value := range filtered {
		if len(merged) == 0 || value.start >= merged[len(merged)-1].end {
			merged = append(merged, value)
			continue
		}
		last := &merged[len(merged)-1]
		if value.end > last.end {
			last.end = value.end
		}
		if kindPriority(value.kind) > kindPriority(last.kind) {
			last.kind = value.kind
		}
	}
	return merged
}

func kindPriority(kind Kind) int {
	switch kind {
	case KindAccount, KindID, KindDevice:
		return 5
	case KindEmail, KindPhone, KindMAC, KindIP:
		return 4
	case KindAddress, KindPerson:
		return 3
	default:
		return 1
	}
}

func unitIndexAtOrAfter(units []runeUnit, byteOffset int) int {
	return sort.Search(len(units), func(index int) bool { return units[index].start >= byteOffset })
}

func labeledValueDelimiter(character rune, kind Kind) bool {
	if character == '\r' || character == '\n' || character == '|' || character == ',' || character == '\uff0c' ||
		character == ';' || character == '\uff1b' || character == '\u3002' {
		return true
	}
	if kind != KindAddress && unicode.IsSpace(character) {
		return true
	}
	return false
}

var structuredOrdinaryFieldLabels = []string{
	"格式化金额", "时间戳", "记录数", "交易方向", "对手方向", "金额", "余额", "合计", "总计", "数量", "笔数", "计数", "日期", "时间", "方向", "币种", "时区",
	"formatted amount", "timestamp", "record count", "transaction direction", "amount", "balance", "total", "count", "date", "time", "direction", "currency", "timezone",
}

func labeledValueDelimiterAt(text string, units []runeUnit, index int, kind Kind) bool {
	if index < 0 || index >= len(units) {
		return true
	}
	if labeledValueDelimiter(units[index].r, kind) {
		return true
	}
	if units[index].r != ':' && units[index].r != '\uff1a' {
		return false
	}
	cursor := index + 1
	for cursor < len(units) && isInlineSpaceOrFormat(units[cursor].r) {
		cursor++
	}
	if cursor >= len(units) {
		return false
	}
	for _, spec := range labeledPII {
		if structuredFieldLabelAt(text, units, cursor, spec.label) {
			return true
		}
	}
	for _, label := range structuredOrdinaryFieldLabels {
		if structuredFieldLabelAt(text, units, cursor, label) {
			return true
		}
	}
	return false
}

func structuredFieldLabelAt(text string, units []runeUnit, cursor int, label string) bool {
	if cursor < 0 || cursor >= len(units) || label == "" {
		return false
	}
	start := units[cursor].start
	if start < 0 || start >= len(text) {
		return false
	}
	labelRunes := []rune(label)
	if !asciiFoldedRunesAt(units, cursor, labelRunes) {
		return false
	}
	next := cursor + len(labelRunes)
	if next >= len(units) {
		return false
	}
	character := units[next].r
	return character == ':' || character == '\uff1a' || character == '=' || isInlineSpaceOrFormat(character)
}

func isSafePlaceholder(value string) bool {
	switch strings.TrimSpace(value) {
	case "[ACCOUNT]", "[ID_NO]", "[PHONE]", "[EMAIL]", "[MAC]", "[IP]", "[DEVICE]", "[ADDRESS]", "[PERSON]", "[NUMBER]":
		return true
	default:
		return false
	}
}

func isInlineSpaceOrFormat(character rune) bool {
	return character != '\r' && character != '\n' && (unicode.IsSpace(character) || unicode.In(character, unicode.Cf))
}

func isIdentifierSeparator(character rune) bool {
	if character != '\r' && character != '\n' && unicode.IsSpace(character) {
		return true
	}
	switch character {
	case '-', '/', '_', '.', '*', '\u00b7', '\u2022', '\u2010', '\u2011', '\u2012', '\u2013', '\u2014', '\u2015', '\u2212', '\ufe58', '\ufe63', '\uff0d':
		return true
	default:
		return false
	}
}

func decimalDigit(character rune) (byte, bool) {
	switch {
	case character >= '0' && character <= '9':
		return byte(character), true
	case character >= '\uff10' && character <= '\uff19':
		return byte('0' + character - '\uff10'), true
	case unicode.IsDigit(character):
		return '0', true
	default:
		return 0, false
	}
}

func asciiDigits(value string) bool {
	if value == "" {
		return false
	}
	for index := range value {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

func isASCIIHex(character rune) bool {
	return (character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F')
}

func isIPTokenRune(character rune) bool {
	return isASCIIHex(character) || character == '.' || character == ':'
}

func isEmailLocalRune(character rune) bool {
	return (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
		(character >= '0' && character <= '9') || strings.ContainsRune("._%+-", character)
}

func isEmailDomainRune(character rune) bool {
	return (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
		(character >= '0' && character <= '9') || character == '.' || character == '-'
}
