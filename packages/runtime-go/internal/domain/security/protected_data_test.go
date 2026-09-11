package security

import "testing"

func TestContainsProtectedCaseDataUsesStructuredValuesNotCaseKeywords(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "bank card", text: "卡号 6222021234567890123 余额多少？", want: true},
		{name: "spaced card", text: "6222 0212 3456 7890", want: true},
		{name: "hyphenated account", text: "6222-0212-3456-7890-123", want: true},
		{name: "mac", text: "device aa:bb:cc:dd:ee:ff", want: true},
		{name: "ipv4", text: "source 10.24.1.8", want: true},
		{name: "phone", text: "call 13800138000", want: true},
		{name: "dates", text: "compare 2026-07-14 and 2026-07-15", want: false},
		{name: "version", text: "release 1.22.4", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ContainsProtectedCaseData(test.text); got != test.want {
				t.Fatalf("ContainsProtectedCaseData()=%v want %v", got, test.want)
			}
		})
	}
}

func TestContainsProtectedCaseFactCandidateCoversConcreteFactShapes(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "currency amount", text: "合成样例事件甲于2026-07-01取得￥2,645,472.00，请确认。", want: true},
		{name: "zero width full width amount", text: "余额为￥２，６４５，４７２。００", want: true},
		{name: "control relation", text: "张某实际控制甲公司，王某是其配偶。", want: true},
		{name: "bid quote", text: "甲公司的投标报价高于乙公司。", want: true},
		{name: "legal characterization", text: "现有材料具备立案条件。", want: true},
		{name: "amount cue", text: "本次交易金额是2645472.00。", want: true},
		{name: "payment cue", text: "甲公司支付2645472元。", want: true},
		{name: "generic evidence guidance", text: "请说明核验证据需要哪些步骤。", want: false},
		{name: "ordinary code", text: "run go test ./... and fix the failing package", want: false},
		{name: "localhost code", text: "curl http://127.0.0.1:3000/health", want: false},
		{name: "json schema draft", text: `{"$schema":"https://json-schema.org/draft/2020-12/schema"}`, want: false},
		{name: "replacement capture", text: "replace the match with $1", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ContainsProtectedCaseFactCandidate(test.text); got != test.want {
				t.Fatalf("ContainsProtectedCaseFactCandidate()=%v want %v", got, test.want)
			}
		})
	}
}
