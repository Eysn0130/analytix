package security

import (
	"strings"
	"unicode"

	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
)

var caseFactAssertionPhrases = []string{
	"实际控制", "实际控制人", "控制关系", "关联关系", "人员关系",
	"亲属", "配偶", "夫妻", "父子", "父女", "母子", "母女", "兄弟", "姐妹",
	"银行账号", "银行卡号", "账户余额", "交易金额", "转账金额", "收款金额", "付款金额",
	"投标报价", "报价金额", "串通投标", "围标", "行贿", "利益输送", "具备立案条件",
	"mac地址", "设备标识", "设备指纹",
}

var monetaryCaseCues = []string{
	"金额", "余额", "转账", "收款", "付款", "支付", "收入", "支出", "流水", "报价", "价款", "涉案",
}

var unboundCaseAssertionPhrases = []string{
	"实际控制", "控制关系", "关联关系", "人员关系", "亲属", "配偶", "夫妻",
	"是父子", "为父子", "系父子",
	"是父女", "为父女", "系父女",
	"是母子", "为母子", "系母子",
	"是母女", "为母女", "系母女",
	"是兄弟", "为兄弟", "系兄弟",
	"是姐妹", "为姐妹", "系姐妹",
	"串通投标", "围标",
	"行贿", "利益输送", "具备立案条件", "当前案件", "案件账户", "案件账号", "涉案",
}

var unboundConcreteCaseAssertionPhrases = []string{
	"实际控制", "是父子", "为父子", "系父子", "是父女", "为父女", "系父女",
	"是母子", "为母子", "系母子", "是母女", "为母女", "系母女",
	"是兄弟", "为兄弟", "系兄弟", "是姐妹", "为姐妹", "系姐妹",
	"串通投标", "行贿", "利益输送", "具备立案条件",
}

var unboundMonetaryActionCues = []string{"支付", "转账", "收款", "付款", "汇入", "汇出"}

// ContainsProtectedCaseData detects structured values whose publication must
// never depend on a model or a lexical case-intent guess. It is deliberately a
// data-shape guard: case identity and evidence authority still come only from
// the host security context.
func ContainsProtectedCaseData(text string) bool {
	return domainprivacy.ContainsRestrictedPII(text)
}

// ContainsProtectedCaseFactCandidate is the conservative draft guard for an
// already case-sensitive turn. It does not prove a case fact or grant evidence
// authority. Ordinary turns use ContainsUnboundCaseRiskV1 so a field label or
// amount in ordinary work does not create a case binding.
func ContainsProtectedCaseFactCandidate(text string) bool {
	if domainprivacy.ContainsRestrictedPII(text) {
		return true
	}
	normalized := normalizeCaseFactText(text)
	if containsAnyCaseFactPhrase(normalized, caseFactAssertionPhrases) {
		return true
	}
	if !containsDecimalDigit(normalized) {
		return false
	}
	if containsCurrencyAmount(normalized) || strings.Contains(normalized, "人民币") ||
		strings.Contains(normalized, "美元") || strings.Contains(normalized, "欧元") || strings.Contains(normalized, "英镑") {
		return true
	}
	return containsAnyCaseFactPhrase(normalized, monetaryCaseCues)
}

// ContainsUnboundCaseRiskV1 is the lexical signal for an ordinary turn or
// result without case authority. A field label or amount alone is not evidence
// that the work belongs to a case. Structured restricted values and explicit
// case assertions still raise risk; the Host binding remains authoritative.
func ContainsUnboundCaseRiskV1(text string) bool {
	if ContainsProtectedCaseData(text) {
		return true
	}
	if containsAnyCaseFactPhrase(normalizeCaseFactText(text), unboundCaseAssertionPhrases) {
		return true
	}
	return containsUnboundMonetaryFactV1(text)
}

// ContainsUnboundCaseFactAssertionV1 keeps an explicit case assertion from
// being treated as software work merely because the same clause mentions code.
// Bare case vocabulary remains a weaker admission signal.
func ContainsUnboundCaseFactAssertionV1(text string) bool {
	if containsAnyCaseFactPhrase(normalizeCaseFactText(text), unboundConcreteCaseAssertionPhrases) {
		return true
	}
	return containsUnboundMonetaryFactV1(text)
}

func containsUnboundMonetaryFactV1(text string) bool {
	for _, rawClause := range strings.FieldsFunc(text, func(character rune) bool {
		return strings.ContainsRune("。！？!?；;\n", character)
	}) {
		clause := normalizeCaseFactText(rawClause)
		if index := strings.Index(clause, "取得"); index >= 0 && containsCurrencyAmount(clause[index+len("取得"):]) {
			return true
		}
		for _, cue := range unboundMonetaryActionCues {
			if actionHasAdjacentAmount(clause, cue) {
				return true
			}
		}
	}
	return false
}

func actionHasAdjacentAmount(clause, cue string) bool {
	for offset := 0; offset < len(clause); {
		index := strings.Index(clause[offset:], cue)
		if index < 0 {
			return false
		}
		offset += index + len(cue)
		tail := strings.TrimLeftFunc(clause[offset:], unicode.IsSpace)
		tail = strings.TrimPrefix(tail, "了")
		tail = strings.TrimLeftFunc(tail, unicode.IsSpace)
		currencyPrefix := false
		for _, prefix := range []string{"人民币", "￥", "¥", "$", "€", "£"} {
			if strings.HasPrefix(tail, prefix) {
				tail = strings.TrimPrefix(tail, prefix)
				currencyPrefix = true
				break
			}
		}
		tail = strings.TrimLeftFunc(tail, unicode.IsSpace)
		if len(tail) > 0 && tail[0] >= '0' && tail[0] <= '9' &&
			(currencyPrefix || amountHasCurrencyUnit(tail)) {
			return true
		}
		// A bounded payee may follow the action directly or after 给/向. The
		// number still needs a currency marker or unit, so "支付2次测试" remains
		// an ordinary action count.
		payeeAndAmount := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(tail, "给"), "向"))
		for byteIndex, character := range payeeAndAmount {
			if character < '0' || character > '9' {
				continue
			}
			if byteIndex == 0 || len([]rune(payeeAndAmount[:byteIndex])) > 16 {
				break
			}
			beforeAmount := strings.TrimSpace(payeeAndAmount[:byteIndex])
			if strings.HasSuffix(beforeAmount, "￥") || strings.HasSuffix(beforeAmount, "¥") ||
				strings.HasSuffix(beforeAmount, "$") || strings.HasSuffix(beforeAmount, "€") ||
				strings.HasSuffix(beforeAmount, "£") || amountHasCurrencyUnit(payeeAndAmount[byteIndex:]) {
				return true
			}
			break
		}
	}
	return false
}

func amountHasCurrencyUnit(text string) bool {
	if len(text) == 0 || text[0] < '0' || text[0] > '9' {
		return false
	}
	afterAmount := strings.TrimSpace(strings.TrimLeft(text, "0123456789,."))
	for _, unit := range []string{"元", "人民币", "美元", "欧元", "英镑"} {
		if strings.HasPrefix(afterAmount, unit) {
			return true
		}
	}
	return false
}

func containsCurrencyAmount(text string) bool {
	runes := []rune(text)
	for index, character := range runes {
		if !strings.ContainsRune("$¥￥€£", character) {
			continue
		}
		digits := 0
		hasAmountSeparator := false
		for cursor := index + 1; cursor < len(runes); cursor++ {
			next := runes[cursor]
			if unicode.IsSpace(next) && digits == 0 {
				continue
			}
			if next >= '0' && next <= '9' {
				digits++
				continue
			}
			if next == ',' || next == '.' {
				hasAmountSeparator = true
				continue
			}
			break
		}
		if digits >= 2 || (digits > 0 && hasAmountSeparator) || (character != '$' && digits > 0) {
			return true
		}
	}
	return false
}

func normalizeCaseFactText(text string) string {
	var builder strings.Builder
	builder.Grow(len(text))
	for _, character := range strings.ToLower(text) {
		switch {
		case character >= '０' && character <= '９':
			builder.WriteRune('0' + character - '０')
		case character == '\u200b' || character == '\u200c' || character == '\u200d' ||
			character == '\u2060' || character == '\ufeff':
			continue
		case character == '，':
			builder.WriteRune(',')
		case character == '。':
			builder.WriteRune('.')
		case character == '：':
			builder.WriteRune(':')
		case character == '－' || character == '—' || character == '–':
			builder.WriteRune('-')
		default:
			builder.WriteRune(character)
		}
	}
	return builder.String()
}

func containsAnyCaseFactPhrase(text string, phrases []string) bool {
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

func containsDecimalDigit(text string) bool {
	for _, character := range text {
		if character >= '0' && character <= '9' {
			return true
		}
	}
	return false
}
