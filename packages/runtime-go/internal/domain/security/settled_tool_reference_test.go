package security

import "testing"

func TestValidateSettledToolReferencesRejectsForgedOrDuplicateAuthority(t *testing.T) {
	valid := SettledToolReference{GrantID: SHA256Hex([]byte("grant")), ResultItemID: "item_result_turn_call"}
	if err := ValidateSettledToolReferences([]SettledToolReference{valid}); err != nil {
		t.Fatalf("valid settled reference rejected: %v", err)
	}
	for name, references := range map[string][]SettledToolReference{
		"forged grant":     {{GrantID: "grant", ResultItemID: valid.ResultItemID}},
		"missing result":   {{GrantID: valid.GrantID}},
		"duplicate grant":  {valid, {GrantID: valid.GrantID, ResultItemID: "item_other"}},
		"duplicate result": {valid, {GrantID: SHA256Hex([]byte("other")), ResultItemID: valid.ResultItemID}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateSettledToolReferences(references); err == nil {
				t.Fatal("invalid settled tool references were accepted")
			}
		})
	}
}
