package continuation

import (
	"testing"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestProviderContinuationNamespaceRequiresExactParentChildRelationship(t *testing.T) {
	root := NewProviderContinuationNamespaceV1("turn", "", 3, nil)
	if err := ValidateProviderContinuationNamespaceV1(root, 0, []string{"read"}); err != nil {
		t.Fatalf("valid root namespace rejected: %v", err)
	}
	for name, namespace := range map[string]ProviderContinuationNamespaceV1{
		"root_with_child":  NewProviderContinuationNamespaceV1("turn", "child-a", 3, nil),
		"child_without_id": NewProviderContinuationNamespaceV1("subagent", "", 3, nil),
		"zero_sequence":    NewProviderContinuationNamespaceV1("turn", "", 0, nil),
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateProviderContinuationNamespaceV1(namespace, 0, []string{"read"}); err == nil {
				t.Fatal("inconsistent provider continuation namespace was accepted")
			}
		})
	}

	schemaHash := domainsecurity.SHA256Hex([]byte("schema"))
	manifest, err := domainjob.NewDelegatedToolManifestV1([]string{"read"}, schemaHash, domainsecurity.SHA256Hex([]byte("mcp")))
	if err != nil {
		t.Fatal(err)
	}
	child := NewProviderContinuationNamespaceV1("subagent", "child-a", 4, manifest)
	if err := ValidateProviderContinuationNamespaceV1(child, 1, []string{"read"}); err != nil {
		t.Fatalf("valid child namespace rejected: %v", err)
	}
	tampered := CloneProviderContinuationNamespaceV1(child)
	tampered.DelegatedToolManifest.ScopeHash = domainsecurity.SHA256Hex([]byte("tampered"))
	if err := ValidateProviderContinuationNamespaceV1(tampered, 1, []string{"read"}); err == nil {
		t.Fatal("tampered delegated namespace was accepted")
	}
}

func TestTerminalRecoveryKindV1IsClosed(t *testing.T) {
	for _, kind := range []TerminalRecoveryKindV1{TerminalRecoveryNoneV1, TerminalRecoveryAppliedV1, TerminalRecoveryStepLimitV1} {
		if err := ValidateTerminalRecoveryKindV1(kind); err != nil {
			t.Fatalf("valid recovery kind %q rejected: %v", kind, err)
		}
	}
	if err := ValidateTerminalRecoveryKindV1("success"); err == nil {
		t.Fatal("unknown recovery kind was accepted")
	}
}
