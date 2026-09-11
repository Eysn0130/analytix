package sideeffectidentity

import "testing"

func TestIdentityV1ValidationAndToolAliases(t *testing.T) {
	valid := IdentityV1{
		SchemaVersion: SchemaVersionV1,
		ToolName:      "write_file",
		ArgsHash:      "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
	if err := ValidateV1(valid); err != nil {
		t.Fatal(err)
	}
	for alias, canonical := range map[string]string{
		"write": "write_file", "edit": "edit_file", "multi_edit": "edit_file", "delegate_task": "task", "todo_patch": "todo_ops",
	} {
		if got := CanonicalToolNameV1(alias); got != canonical {
			t.Fatalf("CanonicalToolNameV1(%q) = %q, want %q", alias, got, canonical)
		}
	}
	invalid := valid
	invalid.ArgsHash = "not-a-hash"
	if err := ValidateV1(invalid); err == nil {
		t.Fatal("invalid semantic identity hash was accepted")
	}
}
