package runtimeapp

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
)

func TestRuntimeOwnedResourceClosesAfterInnerDrainAndRetriesSafely(t *testing.T) {
	blocker := errors.New("drain blocked")
	order := []string{}
	inner := &runtimeOwnedResourceLifecycleStubV1{
		results: []error{blocker, nil}, order: &order,
	}
	resource := &runtimeOwnedResourceCloserStubV1{order: &order}
	bound, err := bindRuntimeOwnedResourceV1(inner, resource)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := bound.(interface{ Shutdown(context.Context) error })
	if err := lifecycle.Shutdown(context.Background()); !errors.Is(err, blocker) {
		t.Fatalf("first shutdown error = %v", err)
	}
	if resource.calls != 0 {
		t.Fatal("resource closed before the runtime drained")
	}
	if err := lifecycle.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if inner.calls != 2 || resource.calls != 1 ||
		!reflect.DeepEqual(order, []string{"inner", "inner", "resource"}) {
		t.Fatalf("shutdown calls inner=%d resource=%d order=%v", inner.calls, resource.calls, order)
	}
}

type runtimeOwnedResourceLifecycleStubV1 struct {
	results []error
	order   *[]string
	calls   int
}

func (*runtimeOwnedResourceLifecycleStubV1) ServeHTTP(http.ResponseWriter, *http.Request) {}

func (stub *runtimeOwnedResourceLifecycleStubV1) Shutdown(context.Context) error {
	stub.calls++
	*stub.order = append(*stub.order, "inner")
	if len(stub.results) == 0 {
		return nil
	}
	result := stub.results[0]
	stub.results = stub.results[1:]
	return result
}

type runtimeOwnedResourceCloserStubV1 struct {
	order *[]string
	calls int
}

func (stub *runtimeOwnedResourceCloserStubV1) Close() error {
	stub.calls++
	*stub.order = append(*stub.order, "resource")
	return nil
}
