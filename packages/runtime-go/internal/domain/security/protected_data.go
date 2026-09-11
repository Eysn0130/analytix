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

// ContainsProtectedCaseData detects structured values whose publication must
// never depend on a model or a lexical case-intent guess. It is deliberately a
// data-shape guard: case identity and evidence authority still come only from
// the host security context.
func ContainsProtectedCaseData(text string) bool {
	return domainprivacy.ContainsRestrictedPII(text)
}

// ContainsProtectedCaseFactCandidate is a fail-closed admission and public
// draft containment guard. It does not prove that text is a case fact and it
// never grants evidence authority. It only identifies concrete fact-shaped
// material that must use the case publication lane instead of arbitrary
// provider prose.
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
