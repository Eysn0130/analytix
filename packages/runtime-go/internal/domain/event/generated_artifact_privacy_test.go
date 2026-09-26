package event

import (
	"errors"
	"reflect"
	"testing"

	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func exactGeneratedArtifactItemV1(t *testing.T) map[string]any {
	t.Helper()
	item := exactPlanToolResultItemV1(t, planDigestWithDecimalRunV1)
	item["toolName"] = "generate_office_document"
	projection := domaintoolresult.PublicToolResultProjectionV1{
		SchemaVersion:  domaintoolresult.PublicProjectionSchemaVersion,
		ProjectionKind: domaintoolresult.ProjectionArtifactStatus,
		Disclosure:     domaintoolresult.MetadataOnlyDisclosure,
		MessageKey:     "artifact_created", Status: "completed", Code: "artifact_created",
		PrivatePayloadWithheld: true,
		Artifact: &domaintoolresult.ArtifactStatusV1{
			ArtifactID: planDigestWithDecimalRunV1, Kind: "docx", ContentHash: planDigestWithDecimalRunV1,
			ByteSize: 1200, SavedAt: "2026-09-12T12:35:15.123456789Z",
		},
	}
	item["output"] = domaintoolresult.PublicToolResultProjectionRecordV1(projection)
	return item
}

func TestGeneratedArtifactClosedMetadataSurvivesPrivacyProjection(t *testing.T) {
	item := exactGeneratedArtifactItemV1(t)
	artifact := item["output"].(map[string]any)["artifact"].(map[string]any)
	for _, field := range []string{"artifactId", "contentHash", "savedAt"} {
		value := artifact[field].(string)
		if domainprivacy.ProjectText(value).Text == value {
			t.Fatalf("%s fixture does not exercise ordinary PII classification", field)
		}
	}
	for name, value := range map[string]any{
		"item":    item,
		"event":   map[string]any{"kind": "tool_call_finished", "item": item},
		"history": []any{map[string]any{"items": []any{item}}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidatePublicRecord(value); err != nil {
				t.Fatalf("closed artifact metadata rejected: %v", err)
			}
			projected, changed := ProjectPublicValuePreservingClosedPlanDigestV1(value)
			if changed || !reflect.DeepEqual(projected, value) {
				t.Fatal("closed artifact metadata changed during projection")
			}
			if err := ValidatePublicRecord(projected); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGeneratedArtifactMetadataDoesNotExemptLookalikesOrDisplayPII(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"wrong tool":      func(item map[string]any) { item["toolName"] = "read_file" },
		"wrong kind":      func(item map[string]any) { item["kind"] = "tool_progress" },
		"wrong tool kind": func(item map[string]any) { item["toolKind"] = "tool_call" },
		"missing call":    func(item map[string]any) { delete(item, "callId") },
		"wrong item id":   func(item map[string]any) { item["id"] = "item_result_wrong" },
		"missing thread":  func(item map[string]any) { delete(item, "threadId") },
		"wrong lifecycle": func(item map[string]any) { item["isError"] = true },
		"unknown root":    func(item map[string]any) { item["extra"] = true },
		"invalid timestamp": func(item map[string]any) {
			item["output"].(map[string]any)["artifact"].(map[string]any)["savedAt"] = "13812345678"
		},
		"invalid digest": func(item map[string]any) {
			item["output"].(map[string]any)["artifact"].(map[string]any)["contentHash"] = "13812345678"
		},
		"unknown artifact field": func(item map[string]any) {
			item["output"].(map[string]any)["artifact"].(map[string]any)["path"] = "13812345678"
		},
		"display PII": func(item map[string]any) { item["text"] = "13812345678" },
	} {
		t.Run(name, func(t *testing.T) {
			item := exactGeneratedArtifactItemV1(t)
			mutate(item)
			if err := ValidatePublicRecord(item); !errors.Is(err, ErrPrivacyProjection) {
				t.Fatalf("noncanonical artifact acquired privacy exemption: %v", err)
			}
			if _, changed := ProjectPublicValuePreservingClosedPlanDigestV1(item); !changed {
				t.Fatal("noncanonical artifact bypassed ordinary projection")
			}
		})
	}
	bare := map[string]any{"output": exactGeneratedArtifactItemV1(t)["output"]}
	if err := ValidatePublicRecord(bare); !errors.Is(err, ErrPrivacyProjection) {
		t.Fatal("bare artifact lookalike acquired metadata exemption")
	}
}
