package loop

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func TestRecoverableProtectedLaneFailureCodeV1UsesClosedTypedClassification(t *testing.T) {
	const staleCode = "execution_grant_connection_epoch_mismatch"
	for name, err := range map[string]error{
		"typed validation": executiongrantapp.ValidationError{Code: staleCode},
		"wrapped typed validation": fmt.Errorf("provider attempt failed: %w",
			executiongrantapp.ValidationError{Code: staleCode}),
		"typed pointer": &executiongrantapp.ValidationError{Code: staleCode},
		"structured turn failure": TurnFailureError{
			Code: "execution_grant_rejected", Details: map[string]any{"code": staleCode},
		},
		"same-code joined causes": errors.Join(
			executiongrantapp.ValidationError{Code: staleCode},
			TurnFailureError{Code: "execution_grant_rejected", Details: map[string]any{"code": staleCode}},
		),
	} {
		t.Run(name, func(t *testing.T) {
			code, ok := RecoverableProtectedLaneFailureCodeV1(err)
			if !ok || code != staleCode {
				t.Fatalf("closed typed failure was not recognized exactly: code=%q ok=%t err=%v", code, ok, err)
			}
		})
	}

	for name, err := range map[string]error{
		"free text":               errors.New("tool_source_unavailable"),
		"workspace mismatch":      TurnFailureError{Code: "turn_security_workspace_mismatch"},
		"case mismatch":           TurnFailureError{Code: "turn_security_case_binding_mismatch"},
		"risk mismatch":           TurnFailureError{Code: "turn_security_risk_policy_mismatch"},
		"generic context invalid": TurnFailureError{Code: "execution_grant_context_invalid"},
		"identity invalid":        executiongrantapp.ValidationError{Code: "execution_grant_server_identity_invalid"},
		"mixed unsafe join": errors.Join(
			executiongrantapp.ValidationError{Code: staleCode},
			errors.New("signer integrity failure"),
		),
		"ambiguous recoverable join": errors.Join(
			executiongrantapp.ValidationError{Code: staleCode},
			executiongrantapp.ValidationError{Code: "execution_grant_source_unavailable"},
		),
	} {
		t.Run(name, func(t *testing.T) {
			if code, ok := RecoverableProtectedLaneFailureCodeV1(err); ok {
				t.Fatalf("unsafe or ambiguous failure was classified as recoverable: code=%q err=%v", code, err)
			}
		})
	}
}

func TestRecoverableProtectedLaneToolMessageCodeV1RequiresStrictPublicProjection(t *testing.T) {
	projection := domaintoolresult.PublicToolResultProjectionV1{
		SchemaVersion:  domaintoolresult.PublicProjectionSchemaVersion,
		ProjectionKind: domaintoolresult.ProjectionHostStatus,
		Disclosure:     domaintoolresult.MetadataOnlyDisclosure, MessageKey: "tool_blocked",
		Status: "blocked", Code: "tool_source_unavailable", PrivatePayloadWithheld: true,
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	valid := domainmodel.Message{Role: "tool", ToolCallID: "call-source", Content: string(encoded)}
	if code, ok := RecoverableProtectedLaneToolMessageCodeV1(valid); !ok || code != "tool_source_unavailable" {
		t.Fatalf("strict source-unavailable projection was rejected: code=%q ok=%t", code, ok)
	}

	completed := projection
	completed.Status = "completed"
	completedEncoded, _ := json.Marshal(completed)
	for name, message := range map[string]domainmodel.Message{
		"free text":        {Role: "tool", ToolCallID: "call-source", Content: "tool_source_unavailable"},
		"partial json":     {Role: "tool", ToolCallID: "call-source", Content: `{"code":"tool_source_unavailable"}`},
		"unknown field":    {Role: "tool", ToolCallID: "call-source", Content: string(encoded[:len(encoded)-1]) + `,"raw":"private"}`},
		"completed status": {Role: "tool", ToolCallID: "call-source", Content: string(completedEncoded)},
		"assistant role":   {Role: "assistant", ToolCallID: "call-source", Content: string(encoded)},
	} {
		t.Run(name, func(t *testing.T) {
			if code, ok := RecoverableProtectedLaneToolMessageCodeV1(message); ok {
				t.Fatalf("non-canonical message was classified as recoverable: code=%q message=%#v", code, message)
			}
		})
	}
}
