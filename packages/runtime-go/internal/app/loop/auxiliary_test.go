package loop

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type auxiliaryProviderFunc func(context.Context, domainmodel.Request) (domainmodel.Result, error)

func (f auxiliaryProviderFunc) Stream(ctx context.Context, r domainmodel.Request) (domainmodel.Result, error) {
	return f(ctx, r)
}
func (auxiliaryProviderFunc) RequiresDurablePipelineStagesV1() {}

type auxiliaryCloser struct {
	calls  int
	err    error
	reason domaincache.ProviderTurnTerminalReasonV1
}

func (c *auxiliaryCloser) CloseTurn(ctx context.Context, _ domainsecurity.TurnSecurityContext, reason domaincache.ProviderTurnTerminalReasonV1, _ time.Time) (domaincache.ProviderTurnClosureV1, error) {
	c.calls++
	c.reason = reason
	if ctx.Err() != nil {
		return domaincache.ProviderTurnClosureV1{}, ctx.Err()
	}
	return domaincache.ProviderTurnClosureV1{}, c.err
}

func TestAuxiliaryProviderRequiresDispatchSettlementAndClosure(t *testing.T) {
	for _, fault := range []string{"none", "pre", "post", "settlement", "closure", "cancel", "tool", "large", "late"} {
		t.Run(fault, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			frozen := newLoopGeneralContextV2(t, "thread", "aux_test", t.TempDir())
			closer := &auxiliaryCloser{}
			failure := errors.New("synthetic failure")
			if fault == "closure" {
				closer.err = failure
			}
			sent, settled, released := 0, 0, 0
			provider := auxiliaryProviderFunc(func(ctx context.Context, r domainmodel.Request) (domainmodel.Result, error) {
				if len(r.Tools) != 0 || r.MaxOutputTokens != InlineCompletionMaxOutputTokens || r.PrivateProviderTelemetry == nil || r.PrivateProviderTelemetry.ChildRunID != "" || !r.PrivateProviderTelemetry.OrdinaryEffect {
					t.Fatal("auxiliary provider authority widened")
				}
				if err := r.OnPipelineStage(domainmodel.PipelineStage{Stage: "pre_send"}); err != nil {
					return domainmodel.Result{}, err
				}
				sent++
				if err := r.OnPipelineStage(domainmodel.PipelineStage{Stage: "post_send"}); err != nil {
					return domainmodel.Result{}, err
				}
				if fault == "cancel" {
					cancel()
				}
				chunk := domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: "allowed continuation"}
				if fault == "tool" {
					chunk = domainmodel.Chunk{Kind: domainmodel.ChunkToolCall, ToolCall: domainmodel.ToolCall{ID: "tool-1", Name: "read", Arguments: []byte(`{}`)}}
				}
				if fault == "large" {
					chunk.Text = strings.Repeat("x", InlineCompletionMaxOutputBytes+1)
				}
				if err := r.OnChunk(chunk); err != nil {
					return domainmodel.Result{}, err
				}
				return domainmodel.Result{Chunks: []domainmodel.Chunk{chunk}, StreamCompleted: true}, nil
			})
			text, err := RunAuxiliaryProvider(ctx, AuxiliaryProviderInput{Request: domainmodel.Request{Messages: []domainmodel.Message{{Role: "user", Content: "continue"}}}, SecurityContext: frozen}, AuxiliaryProviderDependencies{
				Provider: provider, Closer: closer,
				Stage: func(stage domainmodel.PipelineStage) error {
					if stage.Stage == fault+"_send" {
						return failure
					}
					return nil
				},
				Acquire: func(ctx context.Context, _ int) (context.Context, func(), error) {
					return ctx, func() { released++ }, nil
				},
				Prepare: func(_ context.Context, _ int, r domainmodel.Request) (domainmodel.Request, ProviderAttemptSettlement, error) {
					return r, func(context.Context, string, string) error {
						settled++
						if fault == "settlement" {
							return failure
						}
						return nil
					}, nil
				},
				ValidateCurrent: func(context.Context) error {
					if fault == "late" {
						return failure
					}
					return nil
				},
			})
			if fault == "none" {
				if err != nil || text != "allowed continuation" {
					t.Fatalf("ordinary continuation failed: %v", err)
				}
			} else if err == nil || text != "" {
				t.Fatalf("fault %s returned text", fault)
			}
			if closer.calls != 1 || settled != 1 || released != 1 {
				t.Fatalf("closure=%d settlement=%d release=%d", closer.calls, settled, released)
			}
			if fault == "pre" && sent != 0 {
				t.Fatal("pre-stage failure dispatched")
			}
			if fault == "cancel" && closer.reason != domaincache.ProviderTurnTerminalCancelV1 {
				t.Fatal("canceled attempt lacked closure")
			}
		})
	}
}

func TestInlineCompletionRequestDocumentUnionAndBudgets(t *testing.T) {
	valid := InlineCompletionRequest{ThreadID: "thread", RequestID: "00000000-0000-4000-8000-000000000001", Document: InlineCompletionDocument{Path: "plan.md"}, Prompt: "continue", Model: "model"}
	if ValidateInlineCompletionRequest(valid) != nil {
		t.Fatal("valid path request rejected")
	}
	for _, mutate := range []func(*InlineCompletionRequest){
		func(r *InlineCompletionRequest) { r.Model = "" }, func(r *InlineCompletionRequest) { r.Prompt = strings.Repeat("x", InlineCompletionMaxPromptBytes+1) },
		func(r *InlineCompletionRequest) { r.Document.SessionID = strings.Repeat("a", 48) }, func(r *InlineCompletionRequest) { r.Document.Path = "" },
		func(r *InlineCompletionRequest) { r.Document.Path = "file\x00.md" }, func(r *InlineCompletionRequest) { r.RequestID = "not-a-uuid" },
	} {
		invalid := valid
		mutate(&invalid)
		if !errors.Is(ValidateInlineCompletionRequest(invalid), ErrInlineCompletionInvalid) {
			t.Fatal("invalid envelope admitted")
		}
	}
}
