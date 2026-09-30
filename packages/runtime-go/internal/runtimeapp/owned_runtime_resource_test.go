package runtimeapp

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sync"
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

func TestRuntimeOwnedResourcesRejectPartialBindingWithoutTakingOwnership(t *testing.T) {
	if bound, err := bindRuntimeOwnedResourcesV1(nil); bound != nil || err == nil {
		t.Fatalf("empty resource list accepted missing handler: handler=%v err=%v", bound, err)
	}
	for _, test := range []struct {
		name    string
		handler http.Handler
	}{
		{name: "missing handler"},
		{name: "missing middle resource", handler: http.NotFoundHandler()},
	} {
		t.Run(test.name, func(t *testing.T) {
			order := []string{}
			first := &runtimeOwnedResourceCloserStubV1{order: &order, name: "first"}
			last := &runtimeOwnedResourceCloserStubV1{order: &order, name: "last"}
			bound, err := bindRuntimeOwnedResourcesV1(test.handler, first, nil, last)
			if err == nil || bound != nil || first.calls != 0 || last.calls != 0 {
				t.Fatalf("partial bind took ownership: handler=%v err=%v calls=%d/%d", bound, err, first.calls, last.calls)
			}
		})
	}
}

func TestRuntimeOwnedResourcesCloseInBindingOrderAndRetryOnlyUnfinishedOwners(t *testing.T) {
	drainErr := errors.New("drain blocked")
	closeErr := errors.New("middle close blocked")
	order := []string{}
	inner := &runtimeOwnedResourceLifecycleStubV1{results: []error{drainErr, nil}, order: &order}
	first := &runtimeOwnedResourceCloserStubV1{order: &order, name: "first"}
	middle := &runtimeOwnedResourceCloserStubV1{order: &order, name: "middle", results: []error{closeErr, nil}}
	last := &runtimeOwnedResourceCloserStubV1{order: &order, name: "last"}
	bound, err := bindRuntimeOwnedResourcesV1(inner, first, middle, last)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := bound.(interface{ Shutdown(context.Context) error })
	if err := lifecycle.Shutdown(context.Background()); !errors.Is(err, drainErr) || len(order) != 1 {
		t.Fatalf("drain failure closed resources: err=%v order=%v", err, order)
	}
	if err := lifecycle.Shutdown(context.Background()); !errors.Is(err, closeErr) ||
		!reflect.DeepEqual(order, []string{"inner", "inner", "first", "middle"}) {
		t.Fatalf("close failure lost owner/order: err=%v order=%v", err, order)
	}
	if err := lifecycle.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"inner", "inner", "first", "middle", "middle", "last"}) {
		t.Fatalf("retry repeated completed owners or skipped pending owners: %v", order)
	}
}

func TestRuntimeOwnedResourcesSerializeConcurrentShutdown(t *testing.T) {
	order := []string{}
	inner := &runtimeOwnedResourceLifecycleStubV1{order: &order}
	resource := &runtimeOwnedResourceCloserStubV1{order: &order}
	bound, err := bindRuntimeOwnedResourcesV1(inner, resource)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := bound.(interface{ Shutdown(context.Context) error })
	var callers sync.WaitGroup
	for range 8 {
		callers.Add(1)
		go func() {
			defer callers.Done()
			if err := lifecycle.Shutdown(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	callers.Wait()
	if inner.calls != 1 || resource.calls != 1 {
		t.Fatalf("concurrent shutdown repeated owners: inner=%d resource=%d", inner.calls, resource.calls)
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
	order   *[]string
	calls   int
	name    string
	results []error
}

func (stub *runtimeOwnedResourceCloserStubV1) Close() error {
	stub.calls++
	name := stub.name
	if name == "" {
		name = "resource"
	}
	*stub.order = append(*stub.order, name)
	if len(stub.results) > 0 {
		result := stub.results[0]
		stub.results = stub.results[1:]
		return result
	}
	return nil
}
