package toolsideeffect

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	apploop "analytix.local/runtime-go/internal/app/loop"
	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestPrepareNativeSelectionProposeAcceptsRealRequest(t *testing.T) {
	// This task-owned synthetic request reconstructs the native ready event's
	// exact canonical argument hash. Keep its scope, operation and typed payload;
	// proposalId is generated only after the Office owner accepts a proposal.
	arguments := json.RawMessage(`{"scopeId":"abdccbadecddaaebaebadfecefeeeedfbecabfdeadebdfad","operationId":"desktop178_b7e49c1032ad865f","parts":[{"kind":"literal","text":"DSH182 ORIGINAL sentence for native selection and replacement.\nDesktop178 reviewed local edit."}]}`)
	const argsHash = "b3a2334d51e263f9647b4164debe3dbf780674b30ed5c99818a4a6687da9831b"
	const callID = "call_host_a8d741e345efac874f10ad40c020d3cfe159d2d05a5c03b7ab37c449d5cdde31"
	if actual := domainsecurity.CanonicalJSONHash(arguments); actual != argsHash {
		t.Fatalf("native request identity changed: %s", actual)
	}
	pending := appmodel.PendingToolCall{
		ThreadID: "thr_durable_1", TurnID: "turn_2",
		Call: domainmodel.ToolCall{ID: callID, Name: "native_selection_propose", Arguments: arguments},
		ExecutionGrant: domainsecurity.ExecutionGrant{
			ToolName: "native_selection_propose", ToolCallID: callID, ArgsHash: argsHash,
			ReadOnly:      toolcatalogapp.HostAuthorizesReadOnly("native_selection_propose", false, false),
			ApprovalState: "not_required",
		},
	}
	schemas := toolcatalogapp.BuiltinToolSchemas(toolcatalogapp.BuiltinToolSchemaInput{NativeSelections: true})
	if _, blocked := toolcatalogapp.ValidateToolCallArguments(pending.Call, schemas); blocked {
		t.Fatal("the actual native request must pass its registered schema")
	}
	if !apploop.RequiresSideEffectIntent(pending, nil) {
		t.Fatal("a scoped proposal must retain the side-effect intent boundary")
	}
	prepared, err := Prepare(context.Background(), pending, time.Unix(0, 0).UTC(), Dependencies{})
	if err != nil || prepared.Rejection != nil || prepared.SemanticIdentity.ToolName != pending.Call.Name || prepared.SemanticIdentity.ArgsHash == "" || prepared.Bind == nil {
		t.Fatalf("registered scoped proposal was not prepared: %#v err=%v", prepared, err)
	}
	if domainsecurity.CanonicalJSONHash(pending.Call.Arguments) != argsHash || pending.ExecutionGrant.ArgsHash != argsHash {
		t.Fatal("preparation changed the original grant argument identity")
	}
	t.Logf("registered native proposal prepared without changing scope authority: argsHash=%s semanticHash=%s", argsHash, prepared.SemanticIdentity.ArgsHash)
}

func TestPrepareNativeSelectionProposeRejectsInjectedScopeAuthority(t *testing.T) {
	for field, value := range map[string]any{
		"sessionId": strings.Repeat("a", 48), "threadId": "another_thread",
		"selectionToken": strings.Repeat("a", 48), "baseRevision": strings.Repeat("a", 64),
		"changeSequence": 0, "proposalId": strings.Repeat("a", 48),
		"hostBinding": map[string]any{"generationId": strings.Repeat("a", 64), "activationRevision": 1},
	} {
		t.Run(field, func(t *testing.T) {
			arguments := map[string]any{
				"scopeId": "abdccbadecddaaebaebadfecefeeeedfbecabfdeadebdfad", "operationId": "desktop178_b7e49c1032ad865f",
				"parts": []any{map[string]any{"kind": "literal", "text": "Scoped synthetic edit."}},
				field:   value,
			}
			body, err := json.Marshal(arguments)
			if err != nil {
				t.Fatal(err)
			}
			pending := appmodel.PendingToolCall{
				Call:           domainmodel.ToolCall{Name: "native_selection_propose", Arguments: body},
				ExecutionGrant: domainsecurity.ExecutionGrant{ToolName: "native_selection_propose", ArgsHash: domainsecurity.CanonicalJSONHash(body)},
			}
			prepared, err := Prepare(context.Background(), pending, time.Unix(0, 0).UTC(), Dependencies{})
			var validation executiongrantapp.ValidationError
			if !errors.As(err, &validation) || validation.Code != "side_effect_identity_invalid" {
				t.Fatalf("model-supplied authority was accepted: %T %v", err, err)
			}
			if prepared.Rejection != nil || prepared.SemanticIdentity.ArgsHash != "" || prepared.Bind != nil {
				t.Fatal("invalid request published an identity, binding or executable override")
			}
		})
	}
}
