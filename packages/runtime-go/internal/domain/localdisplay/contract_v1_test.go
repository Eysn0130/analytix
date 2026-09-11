package localdisplay

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestCanonicalContractV1IsClosedFourVariantFamily(t *testing.T) {
	contract := CanonicalContractV1()
	if !reflect.DeepEqual(contract.Kinds, []string{
		KindImportMappingPreviewV1,
		KindCleaningDiffPreviewV1,
		KindDirectSourcePreviewV1,
		KindAcceptedSlotDisplayV1,
	}) {
		t.Fatalf("unexpected typed-local kind family: %#v", contract.Kinds)
	}
	if !reflect.DeepEqual(CleaningDiffPreviewFieldsV1(), DirectSourcePreviewFieldsV1()) {
		t.Fatal("cleaning and direct transaction fields drifted")
	}
	var decoded canonicalContractV1
	if err := json.Unmarshal([]byte(CanonicalContractJSONV1), &decoded); err != nil {
		t.Fatalf("canonical contract JSON is invalid: %v", err)
	}
	if !reflect.DeepEqual(decoded, contract) {
		t.Fatalf("canonical JSON and Go contract drifted:\njson=%#v\ngo=%#v", decoded, contract)
	}
}

func TestExactValueV1NeverSerializesOrFormats(t *testing.T) {
	value, err := NewExactValueV1("SOURCE_EXACT_CANARY")
	if err != nil {
		t.Fatalf("new exact value: %v", err)
	}
	if got := value.String(); got != "[typed-local exact value]" {
		t.Fatalf("private value formatted exact bytes: %q", got)
	}
	if _, err := json.Marshal(value); err == nil {
		t.Fatal("private value serialized outside its typed projection callback")
	}
	if rendered := fmt.Sprintf("%#v", value); rendered != "[typed-local exact value]" {
		t.Fatalf("private value Go formatting exposed its shape: %q", rendered)
	}
	if err := json.Unmarshal([]byte(`{}`), &value); err == nil {
		t.Fatal("private value deserialized outside its authority reader")
	}
	var projected string
	if err := value.UseExactV1(func(exact string) error {
		projected = exact
		return nil
	}); err != nil || projected != "SOURCE_EXACT_CANARY" {
		t.Fatalf("private projection failed: projected=%q err=%v", projected, err)
	}
	if _, err := NewExactValueV1("💠" + string(make([]byte, MaximumCellBytesV1))); err == nil {
		t.Fatal("oversized exact UTF-8 value was accepted")
	}
}
