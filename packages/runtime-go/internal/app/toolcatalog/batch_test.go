package toolcatalog

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestExecuteBatchUsesCancelledOutputWhenContextAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results := ExecuteBatch(ctx, []string{"a", "b"}, func(context.Context, string) (any, bool) {
		t.Fatal("execute should not run for already-cancelled batch")
		return nil, false
	}, func(call string, cause error) (any, bool) {
		if !errors.Is(cause, context.Canceled) {
			t.Fatalf("cancel cause mismatch: %v", cause)
		}
		return "cancelled:" + call, true
	})
	if got := outputs(results); !reflect.DeepEqual(got, []any{"cancelled:a", "cancelled:b"}) {
		t.Fatalf("cancelled outputs mismatch: %#v", got)
	}
}

func TestExecuteBatchKeepsResultOrderWhileRunningInParallel(t *testing.T) {
	results := ExecuteBatch(context.Background(), []int{1, 2, 3}, func(_ context.Context, call int) (any, bool) {
		if call == 1 {
			time.Sleep(20 * time.Millisecond)
		}
		return call * 10, false
	}, func(call int, _ error) (any, bool) {
		return call, true
	})
	if got := outputs(results); !reflect.DeepEqual(got, []any{10, 20, 30}) {
		t.Fatalf("result order mismatch: %#v", got)
	}
	for _, result := range results {
		if result.IsError {
			t.Fatalf("unexpected batch error: %#v", results)
		}
	}
}

func outputs[T any](results []BatchResult[T]) []any {
	out := make([]any, 0, len(results))
	for _, result := range results {
		out = append(out, result.Output)
	}
	return out
}
