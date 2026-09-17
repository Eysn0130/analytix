package privacyprojection

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestNumericMeasureProjectionDoesNotDependOnMapRepresentation(t *testing.T) {
	for _, amount := range []string{"9007199254740993.01", "-9007199254740993.01", "6222020000000000000", "0.00"} {
		t.Run(amount, func(t *testing.T) {
			stringMap := map[string]string{"amount": amount, "balanceMinor": amount, "金额": amount, "direction": "out"}
			anyMap := map[string]any{}
			for key, value := range stringMap {
				anyMap[key] = value
			}
			for name, input := range map[string]any{
				"string-map": stringMap, "any-map": anyMap,
				"string-map-list": []map[string]string{stringMap}, "any-map-list": []map[string]any{anyMap},
			} {
				t.Run(name, func(t *testing.T) {
					public := map[string]any{"output": input}
					if err := ValidatePublicValue(public); err != nil {
						t.Fatal("valid typed measure depended on Go map representation")
					}
					for mode, project := range map[string]func(any) (any, bool){
						"public": ProjectPublicValue, "untrusted": ProjectUntrustedValue,
					} {
						got, changed := project(public)
						if changed || !reflect.DeepEqual(got, public) {
							t.Fatalf("%s projection changed exact measures: %#v", mode, got)
						}
					}
				})
			}
		})
	}
}

func TestStringMapMeasureExceptionDoesNotExemptPIIOrMutateSource(t *testing.T) {
	input := map[string]string{
		"amount": "9007199254740993.01", "amountMinor": "900719925474099301",
		"account": "6222020000000000000", "phone": "13800138000",
		"note": "电话 13800138000", "opaque": "6222020000000000000",
		"balance": "电话 13800138000",
	}
	original := map[string]string{}
	for key, value := range input {
		original[key] = value
	}
	projected, changed := ProjectUntrustedValue(input)
	got := projected.(map[string]string)
	if !changed || got["amount"] != input["amount"] || got["amountMinor"] != input["amountMinor"] {
		t.Fatal("projection lost exact decimal measures")
	}
	if got["account"] != "[ACCOUNT]" || got["phone"] != "[PHONE]" || got["opaque"] != "[ACCOUNT]" ||
		strings.Contains(got["note"], input["phone"]) || strings.Contains(got["balance"], input["phone"]) {
		t.Fatal("numeric exception expanded into PII or nonnumeric content")
	}
	if !reflect.DeepEqual(input, original) {
		t.Fatal("projection modified the private source map")
	}
	again, changedAgain := ProjectUntrustedValue(projected)
	if changedAgain || !reflect.DeepEqual(again, projected) || ValidatePublicValue(map[string]any{"output": projected}) != nil {
		t.Fatal("projection is not a validated fixed point")
	}
}

func TestUntrustedQuestionShapeCannotGrantHostCorrelationException(t *testing.T) {
	const raw = "6222020000000000000"
	for _, kind := range []string{"user_input", "user_input_requested"} {
		for _, mode := range []string{"untrusted-root", "public-content"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				record := map[string]any{
					"kind": kind, "inputId": "账号 " + raw,
					"questions": []any{map[string]any{"id": "账号 " + raw + "_1", "question": "Continue?"}},
				}
				var input any = record
				project := ProjectUntrustedValue
				if mode == "public-content" {
					input = map[string]any{"output": record}
					project = ProjectPublicValue
					if ValidatePublicValue(input) == nil {
						t.Fatal("untrusted content gained host authority during validation")
					}
				}
				before, _ := json.Marshal(input)
				got, changed := project(input)
				encoded, err := json.Marshal(got)
				if err != nil || !changed || strings.Contains(string(encoded), raw) {
					t.Fatal("untrusted host-shaped question retained PII after projection")
				}
				after, _ := json.Marshal(input)
				if string(before) != string(after) {
					t.Fatal("untrusted projection mutated its source")
				}
				again, changedAgain := project(got)
				if changedAgain || !reflect.DeepEqual(got, again) || ValidatePublicValue(got) != nil {
					t.Fatal("untrusted projection requires another pass to close PII")
				}
			})
		}
	}
}
