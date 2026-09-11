package loop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainreasoningmarkup "analytix.local/runtime-go/internal/domain/reasoningmarkup"
)

type fakeProviderRetryError struct {
	retryable bool
	delay     time.Duration
}

type stagedProviderDiagnosticError struct {
	cause error
}

func (err stagedProviderDiagnosticError) Error() string {
	if err.cause == nil {
		return "staged provider failure"
	}
	return err.cause.Error()
}

func (err stagedProviderDiagnosticError) Unwrap() error { return err.cause }

func (stagedProviderDiagnosticError) Diagnostics() map[string]any {
	return map[string]any{
		"family":        "openai-compatible",
		"status":        float64(503),
		"kind":          "server",
		"retryable":     true,
		"failureStage":  "transport_after_observed_send",
		"dispatchState": "sent",
		"attempt":       float64(2),
		"requestUrl":    "https://provider.invalid/private-account-6222020000000000000",
		"message":       "PRIVATE_PROVIDER_RESPONSE_BODY",
		"reasoning":     "PRIVATE_REASONING_SENTINEL",
	}
}

type fakeProviderRetryForbiddenError struct{}

func (fakeProviderRetryForbiddenError) Error() string { return "provider returned 503" }
func (fakeProviderRetryForbiddenError) ProviderRetryForbidden() bool {
	return true
}
func (fakeProviderRetryForbiddenError) Retryable() bool { return true }

func (err fakeProviderRetryError) Error() string {
	return "fake provider retry error"
}

func (err fakeProviderRetryError) Retryable() bool {
	return err.retryable
}

func (err fakeProviderRetryError) RetryAfterDelay() time.Duration {
	return err.delay
}

func (err fakeProviderRetryError) Diagnostics() map[string]any {
	return map[string]any{"retryable": err.retryable}
}

func TestStepLimitExceededErrorCarriesTerminalFailureDetails(t *testing.T) {
	err := StepLimitExceededError(7)
	typed, ok := err.(TurnFailureError)
	if !ok {
		t.Fatalf("expected TurnFailureError, got %T", err)
	}
	if typed.Code != "turn_step_limit_exceeded" || typed.Severity != "error" || typed.Details["maxModelSteps"] != float64(7) {
		t.Fatalf("unexpected step-limit error: %#v", typed)
	}
	if !strings.Contains(StepLimitFinalAnswerPrompt(7), "Do not call more tools") {
		t.Fatalf("step-limit final prompt must forbid additional tools")
	}
}

func TestInterruptedStreamRecoveryClassification(t *testing.T) {
	if !InterruptedStreamCanRecover(io.ErrUnexpectedEOF) {
		t.Fatal("unexpected EOF should be recoverable")
	}
	if !InterruptedStreamCanRecover(context.DeadlineExceeded) {
		t.Fatal("deadline exceeded should be recoverable")
	}
	if !InterruptedStreamCanRecover(errors.New("stream stalled before done")) {
		t.Fatal("stream stalled text should be recoverable")
	}
	if !InterruptedStreamCanRecover(domainfailure.NewError(domainfailure.CodeProviderStreamInterrupted, nil)) {
		t.Fatal("closed provider stream interruption should remain recoverable")
	}
	if InterruptedStreamCanRecover(context.Canceled) {
		t.Fatal("explicit cancellation should not be recovered")
	}
	if InterruptedStreamCanRecover(errors.New("invalid API key")) {
		t.Fatal("auth/config errors should not use interrupted-stream recovery")
	}
	if InterruptedStreamCanRecover(domainreasoningmarkup.NewProtocolError(io.ErrUnexpectedEOF)) {
		t.Fatal("reasoning-markup protocol blockers must override transport recovery causes")
	}
}

func TestCancelOrDeadlineNeverEntersStreamRecovery(t *testing.T) {
	for _, test := range []struct {
		name   string
		parent context.Context
		err    error
	}{
		{name: "cancel", parent: cancelledRecoveryContext(), err: io.ErrUnexpectedEOF},
		{name: "deadline", parent: expiredRecoveryContext(), err: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			if InterruptedStreamCanRecoverWithContext(test.parent, test.err) {
				t.Fatal("host terminal context was upgraded to stream recovery")
			}
		})
	}
	if !InterruptedStreamCanRecoverWithContext(context.Background(), context.DeadlineExceeded) {
		t.Fatal("provider-local deadline on a live parent should remain classifiable as a transport interruption")
	}
}

func cancelledRecoveryContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func expiredRecoveryContext() context.Context {
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(1, 0))
	defer cancel()
	return ctx
}

func TestInterruptedStreamRecoveryPromptCoversPartialToolAndText(t *testing.T) {
	if !strings.Contains(InterruptedStreamRecoveryPrompt(false, true), "fresh complete tool call") {
		t.Fatal("partial tool prompt should force a fresh complete call")
	}
	if prompt := InterruptedStreamRecoveryPrompt(true, false); !strings.Contains(prompt, "discarded") || !strings.Contains(prompt, "never shown") || !strings.Contains(prompt, "complete, self-contained replacement") {
		t.Fatalf("partial text prompt must replace the private draft: %q", prompt)
	}
	if !strings.Contains(InterruptedStreamRecoveryPrompt(false, false), "complete, self-contained replacement") {
		t.Fatal("empty interrupted stream prompt should require a complete replacement")
	}
}

func TestProviderRetryPolicyUsesProviderInterfaceAndFallbackText(t *testing.T) {
	err := fakeProviderRetryError{retryable: true, delay: 250 * time.Millisecond}
	if !ProviderErrorLooksRetryable(err) {
		t.Fatal("provider retry interface should decide retryability")
	}
	if delay := ProviderRetryDelay(err); delay != 250*time.Millisecond {
		t.Fatalf("provider retry-after delay mismatch: %v", delay)
	}
	if diagnostics := ProviderErrorDiagnostic(err); diagnostics["retryable"] != true {
		t.Fatalf("provider diagnostics mismatch: %#v", diagnostics)
	}
	if !ProviderErrorLooksRetryable(errors.New("provider returned 503")) {
		t.Fatal("text fallback should treat 503 as retryable")
	}
	if ProviderErrorLooksRetryable(errors.New("invalid api key")) {
		t.Fatal("auth/config text should not be retryable")
	}
	if ProviderErrorLooksRetryable(fmt.Errorf("wrapped: %w", fakeProviderRetryForbiddenError{})) {
		t.Fatal("durable authority failures must override retryable interfaces and fallback text")
	}
}

func TestSanitizeProviderRetryMessageUsesFixedHostText(t *testing.T) {
	long := "provider failed sk-liveSECRET123456789 bearer abc.def authorization: secret-token " + strings.Repeat("x", 260)
	got := SanitizeProviderRetryMessage(errors.New(long))
	if strings.Contains(got, "sk-liveSECRET") || strings.Contains(strings.ToLower(got), "bearer abc") || strings.Contains(got, "secret-token") {
		t.Fatalf("message leaked secret: %s", got)
	}
	if got != "The turn failed before a verified response was available." {
		t.Fatalf("provider text should be replaced by a fixed host message: %q", got)
	}
}

type maliciousProviderDiagnosticError struct{}

func (maliciousProviderDiagnosticError) Error() string {
	return "SOL_PRIVATE_TRACE_7C 6222021234567890"
}
func (maliciousProviderDiagnosticError) Diagnostics() map[string]any {
	return map[string]any{
		"providerId": "SOL_PRIVATE_TRACE_7C", "family": "thinking_family", "status": float64(401), "kind": "auth",
		"message": "private branch alpha was considered before answering", "requestUrl": "https://example.invalid/6222021234567890",
		"reasoningPayload": map[string]any{"private": "SOL_PRIVATE_TRACE_7C"},
	}
}

func TestProviderFailureProjectionDropsArbitraryIdentifiersMessagesAndURLs(t *testing.T) {
	err := maliciousProviderDiagnosticError{}
	diagnostic := ProviderErrorDiagnostic(err)
	serialized := fmt.Sprint(diagnostic)
	for _, forbidden := range []string{"SOL_PRIVATE_TRACE_7C", "thinking_family", "private branch", "6222021234567890", "requestUrl", "message", "reasoningPayload"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("provider diagnostic leaked %q: %#v", forbidden, diagnostic)
		}
	}
	public := PublicFailureForError(err)
	if public.Code() != "provider_authentication_failed" || public.Message() != "Provider authentication failed. Check the configured credential." {
		t.Fatalf("malicious provider error was not classified by host: code=%q message=%q", public.Code(), public.Message())
	}
}

func TestProviderFailureDiagnosticProjectsClosedStageDispatchAndAttempt(t *testing.T) {
	err := stagedProviderDiagnosticError{cause: fmt.Errorf("provider failed: %w", errors.New("PRIVATE_PROVIDER_RESPONSE_BODY"))}
	diagnostic := ProviderErrorDiagnostic(err)
	if diagnostic["failureStage"] != "transport_after_observed_send" ||
		diagnostic["dispatchState"] != "sent" || diagnostic["attempt"] != float64(2) ||
		diagnostic["status"] != float64(503) || diagnostic["kind"] != "server" || diagnostic["retryable"] != true {
		t.Fatalf("closed provider failure diagnostic mismatch: %#v", diagnostic)
	}
	serialized := fmt.Sprint(diagnostic)
	for _, forbidden := range []string{
		"requestUrl", "message", "reasoning", "PRIVATE_PROVIDER_RESPONSE_BODY", "PRIVATE_REASONING_SENTINEL",
		"provider.invalid", "6222020000000000000",
	} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("provider failure diagnostic leaked %q: %#v", forbidden, diagnostic)
		}
	}
	if public := PublicFailureForError(err); public.Code() != domainfailure.CodeProviderUnavailable {
		t.Fatalf("provider failure public code changed while adding stage context: %q", public.Code())
	}
}

func TestProviderFailureDiagnosticUnknownStageFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name          string
		failureStage  string
		dispatchState string
	}{
		{name: "unknown", failureStage: "guessed-stage", dispatchState: "maybe-sent"},
		{name: "non-canonical whitespace", failureStage: " transport_after_observed_send", dispatchState: " sent"},
	} {
		t.Run(test.name, func(t *testing.T) {
			diagnostic := projectProviderDiagnostic(map[string]any{
				"kind": "network", "failureStage": test.failureStage, "dispatchState": test.dispatchState, "attempt": float64(3),
			})
			if diagnostic["kind"] != "network" || diagnostic["attempt"] != float64(3) {
				t.Fatalf("safe provider fields were lost: %#v", diagnostic)
			}
			if _, exists := diagnostic["failureStage"]; exists {
				t.Fatalf("unknown provider failure stage crossed projection: %#v", diagnostic)
			}
			if _, exists := diagnostic["dispatchState"]; exists {
				t.Fatalf("unknown provider dispatch state crossed projection: %#v", diagnostic)
			}
		})
	}
}

func TestPublicFailureForErrorPreservesClosedDomainFailureCode(t *testing.T) {
	public := PublicFailureForError(domainfailure.NewError(domainfailure.CodeProviderEmptyFinal, map[string]any{
		"message": "SOL_PRIVATE_TRACE_7C 6222021234567890",
	}))
	if public.Code() != domainfailure.CodeProviderEmptyFinal ||
		public.Message() != "The provider returned no final response after the bounded recovery attempt." || public.Details() != nil {
		t.Fatalf("closed domain failure was not preserved: code=%q message=%q details=%#v", public.Code(), public.Message(), public.Details())
	}
}
