package filestore

import (
	"context"
	"strings"
	"time"

	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	generationapp "analytix.local/runtime-go/internal/app/documentgeneration"
	codecport "analytix.local/runtime-go/internal/ports/documentgeneration"
)

// ExecuteGenerateDocumentTool uses the ordinary Core mutation and checkpoint
// authority. The codec receives only the typed content, never the target path.
// Generation creates a new file; replacing an existing file is a separate action.
func ExecuteGenerateDocumentTool(input MutationToolInput, codec codecport.Codec) (any, bool) {
	if output, blocked := sandboxBlockedMutationOutput(input); blocked {
		return output, true
	}
	request, err := generationapp.ParseRequest(input.Args)
	if err != nil {
		return map[string]any{"code": "validation_error", "error": "Invalid document generation request."}, true
	}
	if codec == nil || input.Checkpoint.BeginOperation == nil || input.Checkpoint.SettleOperation == nil || input.ToolName != "generate_office_document" {
		return map[string]any{"code": "document_generation_unavailable", "error": "Document generation is unavailable."}, true
	}
	release, err := input.acquireMutation()
	if err != nil {
		return mutationCoordinatorUnavailableOutput(), true
	}
	defer release()
	if output, blocked := mandatoryProtectedRequestedMutationOutput(input, request.Path); blocked {
		return output, true
	}
	path, ok := ResolveWritePathForSandbox(input.Workspace, request.Path, input.SandboxMode, input.AllowWriteRoots)
	if !ok {
		return map[string]any{"code": "workspace_escape", "error": "Document path is outside the authorized workspace."}, true
	}
	if output, blocked := mandatoryProtectedMutationOutput(input, path); blocked {
		return output, true
	}
	// At most one byte is read if a file races into this absent-only target.
	// Never load an existing binary merely to reject an overwrite.
	before, err := inspectAtomicTextTargetWithPolicy(path, true, 1, atomicTextReadPolicy{RequireSingleLink: true})
	if err != nil || before.Exists {
		return map[string]any{"code": "generation_target_unavailable", "error": "Choose a new document filename."}, true
	}
	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}
	content, err := codec.Encode(ctx, request.CodecInput())
	if err != nil || len(content) == 0 || len(content) > codecport.MaxDocumentBytes || InspectOfficePackage(content, request.Kind) != nil {
		return map[string]any{"code": "document_generation_failed", "error": "Document generation did not produce a valid file."}, true
	}
	if ctx.Err() != nil {
		return map[string]any{"code": "document_generation_cancelled", "error": "Document generation was cancelled before saving."}, true
	}
	if input.ValidateCreationIdentity != nil && input.ValidateCreationIdentity() != nil {
		return map[string]any{"code": "document_generation_unavailable", "error": "Document identity changed before saving."}, true
	}
	digest := checkpointapp.HashBytes(content)
	draft, err := input.beginOperation([]MutationOperationPath{{ResolvedPath: path, ArgumentKey: "path", RequestedPath: request.Path, Role: "target", ExpectedAfterExisted: true, ExpectedAfterHash: digest}})
	if err != nil || !draft.Enabled {
		return checkpointUnavailableOutput(), true
	}
	err = atomicReplaceText(atomicTextReplaceRequest{Path: path, Content: content, MaxBytes: codecport.MaxDocumentBytes,
		ReadPolicy: atomicTextReadPolicy{RequireSingleLink: true}, ExpectedExists: false, CreateParents: true, DefaultMode: 0o600})
	if settleErr := input.settleOperation(draft, err == nil); settleErr != nil {
		return checkpointUnavailableOutput(), true
	}
	if err != nil {
		return MutationOperationErrorOutput(err), true
	}
	return generationapp.CreatedReceipt{ArtifactID: strings.TrimSpace(draft.AuthorityIntent.OperationGroupID), Kind: request.Kind, ContentHash: digest, ByteSize: int64(len(content)), SavedAt: time.Now().UTC().Format(time.RFC3339Nano)}, false
}
