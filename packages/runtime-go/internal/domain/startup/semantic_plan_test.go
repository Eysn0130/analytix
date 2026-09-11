package startup

import (
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestSemanticStartupPlanBindsExactOrderedTransitions(t *testing.T) {
	fileBefore := SemanticEntryStateV1{Type: ManagedEntryTypeFile, Mode: uint32(0o100600), Size: 3, SHA256: domainsecurity.SHA256Hex([]byte("old"))}
	fileAfter := SemanticEntryStateV1{Type: ManagedEntryTypeFile, Mode: uint32(0o100600), Size: 3, SHA256: domainsecurity.SHA256Hex([]byte("new"))}
	directory := SemanticEntryStateV1{Type: ManagedEntryTypeDirectory, Mode: uint32(0o40700)}
	absent := SemanticEntryStateV1{Type: ManagedEntryTypeAbsent}
	plan, err := NewSemanticStartupPlanV1(
		domainsecurity.SHA256Hex([]byte("baseline")),
		domainsecurity.SHA256Hex([]byte("configuration")),
		domainsecurity.SHA256Hex([]byte("final")),
		[]SemanticStartupOperationV1{
			{Kind: SemanticOperationRemoveDirectory, Path: "durable/runtime-go/threads", Before: directory, After: absent},
			{Kind: SemanticOperationInstallFile, Path: "durable/threads/thread-a/thread.json", Before: fileBefore, After: fileAfter},
			{Kind: SemanticOperationCreateDirectory, Path: "data/private", Before: absent, After: directory},
		},
	)
	if err != nil || ValidateSemanticStartupPlanV1(plan) != nil || len(plan.Operations) != 3 {
		t.Fatalf("semantic plan was not sealed: plan=%#v err=%v", plan, err)
	}
	if plan.Operations[0].Kind != SemanticOperationCreateDirectory ||
		plan.Operations[1].Kind != SemanticOperationInstallFile ||
		plan.Operations[2].Kind != SemanticOperationRemoveDirectory {
		t.Fatalf("semantic operations were not ordered by safe apply phase: %#v", plan.Operations)
	}

	tampered := plan
	tampered.Operations = append([]SemanticStartupOperationV1(nil), plan.Operations...)
	tampered.Operations[1].After.SHA256 = domainsecurity.SHA256Hex([]byte("other"))
	if ValidateSemanticStartupPlanV1(tampered) == nil {
		t.Fatal("tampered semantic transition retained plan authority")
	}
	tampered = plan
	tampered.ConfigurationDigest = domainsecurity.SHA256Hex([]byte("other-config"))
	if ValidateSemanticStartupPlanV1(tampered) == nil {
		t.Fatal("tampered configuration binding retained plan authority")
	}
}

func TestSemanticStartupPlanRejectsUnsafeAndAmbiguousOperations(t *testing.T) {
	absent := SemanticEntryStateV1{Type: ManagedEntryTypeAbsent}
	directory := SemanticEntryStateV1{Type: ManagedEntryTypeDirectory, Mode: uint32(0o40700)}
	for _, operation := range []SemanticStartupOperationV1{
		{Kind: SemanticOperationCreateDirectory, Path: "data/../outside", Before: absent, After: directory},
		{Kind: SemanticOperationCreateDirectory, Path: "/absolute", Before: absent, After: directory},
		{Kind: SemanticOperationInstallFile, Path: "data/private/file.json", Before: directory, After: SemanticEntryStateV1{Type: ManagedEntryTypeFile, Mode: uint32(0o100600), Size: 1, SHA256: domainsecurity.SHA256Hex([]byte("x"))}},
	} {
		if _, err := NewSemanticStartupPlanV1(
			domainsecurity.SHA256Hex([]byte("baseline")), domainsecurity.SHA256Hex([]byte("config")),
			domainsecurity.SHA256Hex([]byte("final")), []SemanticStartupOperationV1{operation},
		); err == nil {
			t.Fatalf("unsafe semantic operation was accepted: %#v", operation)
		}
	}
}
