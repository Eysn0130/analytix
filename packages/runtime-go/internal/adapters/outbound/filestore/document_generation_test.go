package filestore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	generationapp "analytix.local/runtime-go/internal/app/documentgeneration"
	codecport "analytix.local/runtime-go/internal/ports/documentgeneration"
)

type generationCodecFixture func(context.Context, codecport.Input) ([]byte, error)

func (f generationCodecFixture) Encode(ctx context.Context, input codecport.Input) ([]byte, error) {
	return f(ctx, input)
}

func TestGeneratedDocumentCreatesOnlyAfterCheckpointAndConfirmsBytes(t *testing.T) {
	for _, scenario := range []string{"success", "exists", "race", "invalid-package", "settlement-failure", "disabled-draft"} {
		t.Run(scenario, func(t *testing.T) {
			workspace := t.TempDir()
			target := filepath.Join(workspace, "report.docx")
			content := officeTestZIP(t, officeTestParts("docx"))
			encoded, began, settled := 0, 0, 0
			codec := generationCodecFixture(func(_ context.Context, input codecport.Input) ([]byte, error) {
				encoded++
				if input.Markdown != "Synthetic" {
					t.Fatal("codec content mismatch")
				}
				if scenario == "invalid-package" {
					return []byte("invalid"), nil
				}
				return content, nil
			})
			if scenario == "exists" {
				if err := os.WriteFile(target, []byte("existing"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			input := MutationToolInput{Context: context.Background(), Workspace: workspace, ToolName: "generate_office_document", Args: map[string]any{"path": "report.docx", "kind": "docx", "markdown": "Synthetic"}, Checkpoint: MutationCheckpointHooks{
				BeginOperation: func(paths []MutationOperationPath) (MutationOperationDraft, error) {
					began++
					if len(paths) != 1 || paths[0].ExpectedAfterHash != checkpointapp.HashBytes(content) {
						t.Fatal("checkpoint did not bind binary bytes")
					}
					if scenario == "race" {
						if err := os.WriteFile(target, []byte("competitor"), 0o600); err != nil {
							t.Fatal(err)
						}
					}
					return MutationOperationDraft{Enabled: scenario != "disabled-draft"}, nil
				},
				SettleOperation: func(_ MutationOperationDraft, succeeded bool) (string, error) {
					settled++
					if scenario == "settlement-failure" {
						return "", errors.New("synthetic receipt failure")
					}
					if scenario == "race" {
						if succeeded {
							t.Fatal("overwrite race succeeded")
						}
						return "quarantined", nil
					}
					actual, err := os.ReadFile(target)
					if err != nil || !bytes.Equal(actual, content) || !succeeded {
						t.Fatal("saved bytes not confirmed")
					}
					return "completed", nil
				},
			}}
			output, isError := ExecuteGenerateDocumentTool(input, codec)
			if (scenario == "success") == isError {
				t.Fatalf("unexpected generation result for %s", scenario)
			}
			if scenario == "success" {
				if output.(generationapp.CreatedReceipt).ContentHash != checkpointapp.HashBytes(content) || began != 1 || settled != 1 {
					t.Fatal("missing exact receipt")
				}
			}
			if scenario == "exists" && encoded != 0 {
				t.Fatal("existing path invoked codec")
			}
			if scenario == "invalid-package" && began != 0 {
				t.Fatal("invalid file began mutation")
			}
			if scenario == "race" {
				data, _ := os.ReadFile(target)
				if string(data) != "competitor" {
					t.Fatal("competitor overwritten")
				}
			}
			if scenario == "disabled-draft" || scenario == "invalid-package" {
				if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("failed preparation wrote file")
				}
			}
		})
	}
}
