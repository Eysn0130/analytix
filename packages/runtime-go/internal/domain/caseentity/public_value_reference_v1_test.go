package caseentity

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
)

type publicReferenceStructV1 struct {
	Value   any    `json:"value"`
	Ignored string `json:"-"`
	hidden  string
}

type recursivePublicReferenceStructV1 struct {
	Next *recursivePublicReferenceStructV1 `json:"next"`
}

type opaquePublicReferenceJSONV1 struct{}

func (opaquePublicReferenceJSONV1) MarshalJSON() ([]byte, error) {
	return []byte(`"opaque"`), nil
}

type opaquePublicReferenceKeyV1 string

func (opaquePublicReferenceKeyV1) MarshalText() ([]byte, error) {
	return []byte("opaque"), nil
}

func TestValidatePublicValueWithoutReferenceV1FindsCanonicalReferenceRecursively(t *testing.T) {
	reference := mustPublicValueReferenceV1(t, "a")
	value := map[string]any{
		"safe": []any{"ordinary", json.Number("42"), 3.5},
		"nested": &publicReferenceStructV1{
			Value: map[string]string{"answer": "prefix " + reference + " suffix"},
		},
	}
	if err := ValidatePublicValueWithoutReferenceV1(value); !errors.Is(err, ErrPublicValueInternalReferenceV1) {
		t.Fatalf("nested canonical reference was not rejected: %v", err)
	}
	if !ContainsReferenceInPublicValueV1(value) {
		t.Fatal("conservative helper did not reject a nested canonical reference")
	}

	if err := ValidatePublicValueWithoutReferenceV1(map[string]any{reference: "safe"}); !errors.Is(err, ErrPublicValueInternalReferenceV1) {
		t.Fatalf("canonical reference in a public JSON key was not rejected: %v", err)
	}
	if err := ValidatePublicValueWithoutReferenceV1(ReferenceV1(reference)); !errors.Is(err, ErrPublicValueInternalReferenceV1) {
		t.Fatalf("typed canonical reference was not rejected: %v", err)
	}
}

func TestValidatePublicValueWithoutReferenceV1UsesStrictCanonicalSyntax(t *testing.T) {
	ordinary := []string{
		"ordinary discussion of the cer1_ prefix",
		"cer1_" + strings.Repeat("a", 63),
		"cer1_" + strings.Repeat("a", 63) + "q",
		"cer1_" + strings.Repeat("A", 64),
		"cer2_" + strings.Repeat("a", 64),
		"cer1_not-an-internal-reference",
	}
	for _, value := range ordinary {
		if err := ValidatePublicValueWithoutReferenceV1(map[string]any{"text": value}); err != nil {
			t.Fatalf("ordinary non-canonical text was misdetected: value=%q err=%v", value, err)
		}
	}

	reference := mustPublicValueReferenceV1(t, "b")
	if err := ValidatePublicValueWithoutReferenceV1("before:" + reference + ":after"); !errors.Is(err, ErrPublicValueInternalReferenceV1) {
		t.Fatalf("complete reference embedded in prose was not detected: %v", err)
	}
}

func TestValidatePublicValueWithoutReferenceV1CoversStructPointerInterfaceAndRawJSON(t *testing.T) {
	reference := mustPublicValueReferenceV1(t, "c")
	safe := publicReferenceStructV1{
		Value:   any(map[string]any{"text": "safe"}),
		Ignored: reference,
		hidden:  reference,
	}
	if err := ValidatePublicValueWithoutReferenceV1(&safe); err != nil {
		t.Fatalf("non-public struct fields affected inspection: %v", err)
	}

	safe.Value = any(&publicReferenceStructV1{Value: reference})
	if err := ValidatePublicValueWithoutReferenceV1(&safe); !errors.Is(err, ErrPublicValueInternalReferenceV1) {
		t.Fatalf("reference behind struct/interface/pointer layers was not rejected: %v", err)
	}

	raw := json.RawMessage(`{"nested":["` + reference + `"]}`)
	if err := ValidatePublicValueWithoutReferenceV1(raw); !errors.Is(err, ErrPublicValueInternalReferenceV1) {
		t.Fatalf("reference in strict raw JSON was not rejected: %v", err)
	}
	escaped := json.RawMessage(`"cer1_` + strings.Repeat(`\u0064`, 64) + `"`)
	if err := ValidatePublicValueWithoutReferenceV1(escaped); !errors.Is(err, ErrPublicValueInternalReferenceV1) {
		t.Fatalf("escaped canonical reference in raw JSON was not rejected: %v", err)
	}
}

func TestValidatePublicValueWithoutReferenceV1RejectsSourceRowReferencesRawAndEscaped(t *testing.T) {
	reference := privateSourceRowReferencePrefixV1 + strings.Repeat("a", 64)
	for name, value := range map[string]any{
		"raw text":           "evidence " + reference,
		"raw JSON":           json.RawMessage(`{"evidenceRef":"` + reference + `"}`),
		"escaped JSON":       json.RawMessage(`{"evidenceRef":"srow1_` + strings.Repeat(`\u0061`, 64) + `"}`),
		"escaped text":       "evidence srow1_" + strings.Repeat(`\u0061`, 64),
		"fully escaped text": strings.Repeat(`\u0073\u0072\u006f\u0077\u0031\u005f`, 1) + strings.Repeat(`\u0061`, 64),
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidatePublicValueWithoutReferenceV1(value); !errors.Is(err, ErrPublicValueInternalReferenceV1) {
				t.Fatalf("source-row reference was not rejected: value=%#v err=%v", value, err)
			}
		})
	}
	for _, value := range []string{
		"ordinary discussion of the srow1_ prefix",
		"srow1_" + strings.Repeat("a", 63),
		"srow1_" + strings.Repeat("A", 64),
	} {
		if ContainsInternalReferenceV1(value) {
			t.Fatalf("non-canonical source-row reference was rejected: %q", value)
		}
	}
}

func TestValidatePublicValueWithoutReferenceV1FailsClosedOnCyclesAndLimits(t *testing.T) {
	mapCycle := map[string]any{}
	mapCycle["self"] = mapCycle

	pointerCycle := &recursivePublicReferenceStructV1{}
	pointerCycle.Next = pointerCycle

	sliceCycle := make([]any, 1)
	sliceCycle[0] = sliceCycle

	for name, value := range map[string]any{
		"map cycle":        mapCycle,
		"pointer cycle":    pointerCycle,
		"slice cycle":      sliceCycle,
		"opaque marshaler": opaquePublicReferenceJSONV1{},
		"opaque map key":   map[opaquePublicReferenceKeyV1]string{"key": "safe"},
		"unsupported func": func() {},
		"invalid number":   json.Number("01"),
		"non-finite float": math.Inf(1),
		"binary slice":     []byte("safe"),
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidatePublicValueWithoutReferenceV1(value); !errors.Is(err, ErrPublicValueReferenceInspectionV1) {
				t.Fatalf("unsafe value did not fail closed: %v", err)
			}
			if !ContainsReferenceInPublicValueV1(value) {
				t.Fatal("conservative helper accepted an incompletely inspectable value")
			}
		})
	}

	var deep any = "safe"
	for index := 0; index < maxPublicValueReferenceDepthV1+2; index++ {
		deep = []any{deep}
	}
	if err := ValidatePublicValueWithoutReferenceV1(deep); !errors.Is(err, ErrPublicValueReferenceInspectionV1) {
		t.Fatalf("depth limit did not fail closed: %v", err)
	}

	wide := make([]string, maxPublicValueReferenceNodesV1)
	if err := ValidatePublicValueWithoutReferenceV1(wide); !errors.Is(err, ErrPublicValueReferenceInspectionV1) {
		t.Fatalf("node limit did not fail closed: %v", err)
	}

	invalidRaw := json.RawMessage(`{"a":1,"a":2}`)
	if err := ValidatePublicValueWithoutReferenceV1(invalidRaw); !errors.Is(err, ErrPublicValueReferenceInspectionV1) {
		t.Fatalf("ambiguous raw JSON did not fail closed: %v", err)
	}
}

func TestValidatePublicValueWithoutReferenceV1AllowsClosedJSONLikeValues(t *testing.T) {
	value := map[string]any{
		"nil":    nil,
		"bool":   true,
		"int":    int64(42),
		"float":  1.25,
		"number": json.Number("123.45"),
		"array":  [2]string{"ordinary", "cer1_ is documentation text"},
		"raw":    json.RawMessage(`{"status":"safe","items":[1,2,3]}`),
	}
	if err := ValidatePublicValueWithoutReferenceV1(value); err != nil {
		t.Fatalf("closed JSON-like value was rejected: %v", err)
	}
	if ContainsReferenceInPublicValueV1(value) {
		t.Fatal("conservative helper rejected a safe closed JSON-like value")
	}
}

func mustPublicValueReferenceV1(t *testing.T, digestCharacter string) string {
	t.Helper()
	reference, err := NewReferenceV1FromKeyedDigest(strings.Repeat(digestCharacter, 64))
	if err != nil {
		t.Fatal(err)
	}
	return string(reference)
}
