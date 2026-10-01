package toolcatalog

import (
	"encoding/json"
	"strings"
	"testing"

	generationapp "analytix.local/runtime-go/internal/app/documentgeneration"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func TestGeneratedArtifactPublicProjectionRequiresTypedHostReceipt(t *testing.T) {
	receipt := generationapp.CreatedReceipt{ArtifactID: strings.Repeat("a", 64), Kind: "docx", ContentHash: strings.Repeat("b", 64), ByteSize: 1200, SavedAt: "2026-09-15T01:00:00Z"}
	projection := BuildPublicToolResultProjectionV1("generate_office_document", receipt, false)
	if projection.ProjectionKind != domaintoolresult.ProjectionArtifactStatus || projection.Artifact == nil || domaintoolresult.ValidatePublicToolResultProjectionV1(projection) != nil {
		t.Fatal("valid host artifact receipt was not projected")
	}
	body, _ := json.Marshal(projection)
	if strings.Contains(string(body), "path") || strings.Contains(string(body), "markdown") || projection.FactAnswerAllowed || projection.EvidenceAuthority {
		t.Fatal("artifact projection crossed the metadata-only boundary")
	}
	for _, test := range []struct {
		name   string
		output any
		failed bool
	}{
		{"failed", receipt, true},
		{"forged map", map[string]any{"artifactId": receipt.ArtifactID, "kind": "docx", "contentHash": receipt.ContentHash, "byteSize": 1200, "savedAt": receipt.SavedAt}, false},
		{"invalid metadata", generationapp.CreatedReceipt{ArtifactID: "private-path"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := BuildPublicToolResultProjectionV1("generate_office_document", test.output, test.failed)
			if got.ProjectionKind == domaintoolresult.ProjectionArtifactStatus || got.Artifact != nil {
				t.Fatal("untrusted artifact authority was published")
			}
		})
	}
	if got := BuildPublicToolResultProjectionV1("mcp__remote__tool", receipt, false); got.Artifact != nil {
		t.Fatal("non-generation tool acquired artifact authority")
	}
}

func TestDocumentGenerationCatalogRequiresCapabilityAndRetainsMutationPolicy(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, child := range []bool{false, true} {
			tools := MaterializeToolSchemas(MaterializeInput{DocumentGeneration: enabled, Subagent: child, ToolScope: []string{"generate_office_document"}, PromptRoute: "coding"})
			found := false
			for _, tool := range tools {
				if tool.Name == "generate_office_document" {
					found = true
				}
			}
			if found != (enabled && !child) {
				t.Fatalf("generation capability mismatch enabled=%v child=%v", enabled, child)
			}
		}
	}
	if !IsMutatingTool("generate_office_document") || !IsFileMutationTool("generate_office_document") || ToolKind("generate_office_document") != "file_change" {
		t.Fatal("generation lost file mutation policy")
	}
}
