package loop

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestRuntimeRunnerRejectsEscapedPrivateReferencesBeforeOrdinaryToolEffects(t *testing.T) {
	for name, reference := range map[string]string{
		"case entity": `\u0063\u0065\u0072\u0031\u005f` + strings.Repeat(`\u0061`, 64),
		"source row":  `\u0073\u0072\u006f\u0077\u0031\u005f` + strings.Repeat(`\u0061`, 64),
	} {
		t.Run(name, func(t *testing.T) {
			workspace := t.TempDir()
			securityContext, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{
				ThreadID: "thread-runtime-private-ref-" + name, TurnID: "turn-runtime-private-ref-" + name,
				WorkspaceRealPath: workspace, ContextEpoch: 1, IssuedAt: time.Now().UTC(),
			})
			if err != nil {
				t.Fatal(err)
			}
			provider := &providerStreamStub{responses: []providerStreamResponse{
				runtimeRunnerTerminalToolResponse("ordinary-private-ref", "write_file", `{"content":"`+reference+`"}`),
			}}
			events := &runtimeLoopEventRecorderStub{}
			driver := &toolStepDriverStub{}
			deps := runtimeRunnerTestDependencies(provider, events)
			deps.ToolDriver = driver
			deps.ResolveToolCatalog = func(bool, RuntimeProviderStep) RuntimeRunnerToolCatalog {
				return RuntimeRunnerToolCatalog{
					PromptRoute: "tool_agent",
					Schemas: []domainmodel.ToolSchema{{
						Name:       "write_file",
						Parameters: []byte(`{"type":"object","properties":{"content":{"type":"string"}},"required":["content"],"additionalProperties":false}`),
					}},
				}
			}
			result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
				ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
				Prompt: "write ordinary output", ProviderID: "provider", Model: "model",
				ProviderConfig: domainmodel.TurnConfig{
					ProviderID: "provider", EndpointFormat: "chat_completions", BaseURL: "https://provider.invalid", Model: "model",
				},
				ApprovalPolicy: "auto", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
				SecurityContext: securityContext, OrdinaryResultInputIsolated: true,
				Messages: []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "write ordinary output"}},
			}, deps)
			var failure TurnFailureError
			if !errors.As(err, &failure) || failure.Code != "tool_private_arguments" || len(provider.requests) != 1 {
				t.Fatalf("runtime did not reject the escaped private reference: result=%#v calls=%d err=%v", result, len(provider.requests), err)
			}
			if len(driver.ready) != 0 || len(driver.grants) != 0 || len(driver.executed) != 0 ||
				len(driver.batches) != 0 || len(driver.batchAttempts) != 0 || len(driver.pendings) != 0 {
				t.Fatalf("runtime rejection produced an ordinary tool effect: %#v", driver)
			}
			serializedEvents, marshalErr := json.Marshal(events.events)
			if marshalErr != nil || strings.Contains(string(serializedEvents), reference) || strings.Contains(string(serializedEvents), "srow1_") ||
				strings.Contains(string(serializedEvents), "cer1_") || strings.Contains(string(serializedEvents), "arguments") {
				t.Fatalf("SSE/durable runtime events retained private arguments: %s err=%v", serializedEvents, marshalErr)
			}
		})
	}
}
