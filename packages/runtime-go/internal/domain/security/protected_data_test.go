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

func TestContainsUnboundCaseRiskV1SeparatesOrdinaryValuesFromCaseAssertions(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "original ordinary file request", text: "请实际读取工作区相对路径「2026年资料/表单 42.txt」的内容，只列出文件中的日期、数量、金额和参考编号；不要猜测。标记 N04-QA-107d-ordinary。", want: false},
		{name: "unquoted numeric path", text: "请读取 2026年资料/表单 42.txt 的日期、数量、金额和参考编号；标记 N04-QA-107d-ordinary。", want: false},
		{name: "numeric filename without slash", text: "请读取「表单 42.txt」，列出金额。", want: false},
		{name: "two spaces in path", text: "请读取「2026年资料/季度  报表 42.txt」，列出金额。", want: false},
		{name: "ordinary result", text: "日期：2026-07-01；数量：42；金额：1234.56 元；参考编号：REF-17。", want: false},
		{name: "ordinary currency", text: "金额：￥1234.56", want: false},
		{name: "ordinary version acquisition", text: "取得版本2.0的说明文件", want: false},
		{name: "ordinary DOM relation", text: "解释 DOM 父子节点和兄弟节点的遍历", want: false},
		{name: "ordinary payment module tests", text: "支付模块新增 3 个测试", want: false},
		{name: "ordinary payment action count", text: "支付2次测试，检查回调顺序", want: false},
		{name: "ordinary component count", text: "支付2元件用于测试", want: false},
		{name: "ordinary payee component count", text: "支付给测试模块 3 个元件", want: false},
		{name: "ordinary payee ten-thousand component count", text: "支付给测试模块 2 万个元件", want: false},
		{name: "ordinary sibling travel", text: "为两位姐妹写旅行计划", want: false},
		{name: "ordinary parent child DOM", text: "解释 DOM 父子节点关系", want: false},
		{name: "ordinary DOM relationship", text: "请解释 DOM 元素的父子关系。", want: false},
		{name: "ordinary DOM relationship question", text: "请解释 DOM 元素是否存在父子关系。", want: false},
		{name: "ordinary DOM relationship assertion", text: "DOM 元素与子元素存在父子关系。", want: false},
		{name: "later QA marker", text: "请列出金额字段。标记 N04-QA-107d-ordinary。", want: false},
		{name: "acquired currency", text: "甲公司取得￥2645.72，见doc/a.txt", want: true},
		{name: "real payment before path", text: "甲公司支付2645.72元，见doc/a.txt", want: true},
		{name: "real payment with particle", text: "甲公司支付了 2645.72 元", want: true},
		{name: "real payment with currency prefix", text: "甲公司支付￥2645.72", want: true},
		{name: "real payment with payee", text: "甲公司支付给乙公司2645.72元", want: true},
		{name: "real payment with direct payee", text: "甲公司支付乙公司2645.72元。", want: true},
		{name: "real payment with ten-thousand unit", text: "甲公司支付乙公司2万元。", want: true},
		{name: "real payment with hundred-million unit", text: "甲公司支付乙公司2亿元。", want: true},
		{name: "real payment with ten-thousand currency unit", text: "甲公司支付乙公司2万人民币。", want: true},
		{name: "acquired ten-thousand unit", text: "甲公司取得2万元。", want: true},
		{name: "explicit case request", text: "请核实当前案件的金额字段", want: true},
		{name: "relationship assertion", text: "张某实际控制甲公司", want: true},
		{name: "parent child assertion", text: "张某与李某是父子", want: true},
		{name: "parent child exists assertion", text: "张某与李某存在父子关系。", want: true},
		{name: "parent child exists named assertion", text: "张三与李四存在父子关系。", want: true},
		{name: "sibling assertion", text: "张某与李某为姐妹", want: true},
		{name: "synthetic phone", text: "请查询电话 13800138000", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ContainsUnboundCaseRiskV1(test.text); got != test.want {
				t.Fatalf("ContainsUnboundCaseRiskV1()=%v want %v", got, test.want)
			}
		})
	}
}

func TestContainsUnboundCaseFactAssertionV1DoesNotUseSoftwareWordsAsAuthority(t *testing.T) {
	for _, text := range []string{
		"修改代码并写明当前案件甲公司支付给乙公司2645.72元。",
		"修改代码并写明当前案件甲公司支付乙公司2645.72元。",
		"修改代码并写明当前案件甲公司支付乙公司2万元。",
		"修改代码并写明当前案件张某与李某存在父子关系。",
		"修改代码并写明当前案件张某与李某是父子。",
	} {
		if !ContainsUnboundCaseFactAssertionV1(text) {
			t.Fatalf("concrete case assertion escaped the software shortcut: %q", text)
		}
	}
	for _, text := range []string{
		"导出当前案件资金分析代码的测试报告。",
		"支付2次测试，检查回调顺序。",
		"请解释 DOM 元素的父子关系。",
		"请解释 DOM 元素是否存在父子关系。",
		"DOM 元素与子元素存在父子关系。",
	} {
		if ContainsUnboundCaseFactAssertionV1(text) {
			t.Fatalf("ordinary software text became a case assertion: %q", text)
		}
	}
}
