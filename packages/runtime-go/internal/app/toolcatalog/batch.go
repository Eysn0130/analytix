package toolcatalog

import (
	"context"
	"sync"
)

type BatchResult[T any] struct {
	Call    T
	Output  any
	IsError bool
}

func ExecuteBatch[T any](
	ctx context.Context,
	calls []T,
	execute func(context.Context, T) (any, bool),
	cancelled func(T, error) (any, bool),
) []BatchResult[T] {
	if len(calls) == 0 {
		return nil
	}
	results := make([]BatchResult[T], len(calls))
	if cancelErr := ctx.Err(); cancelErr != nil {
		for index, call := range calls {
			output, isError := cancelled(call, cancelErr)
			results[index] = BatchResult[T]{Call: call, Output: output, IsError: isError}
		}
		return results
	}
	if len(calls) == 1 {
		output, isError := execute(ctx, calls[0])
		results[0] = BatchResult[T]{Call: calls[0], Output: output, IsError: isError}
		return results
	}
	var wg sync.WaitGroup
	for index, call := range calls {
		index := index
		call := call
		wg.Add(1)
		go func() {
			defer wg.Done()
			if cancelErr := ctx.Err(); cancelErr != nil {
				output, isError := cancelled(call, cancelErr)
				results[index] = BatchResult[T]{Call: call, Output: output, IsError: isError}
				return
			}
			output, isError := execute(ctx, call)
			results[index] = BatchResult[T]{Call: call, Output: output, IsError: isError}
		}()
	}
	wg.Wait()
	return results
}
