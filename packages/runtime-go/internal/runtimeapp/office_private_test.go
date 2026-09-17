package runtimeapp

import (
	"analytix.local/runtime-go/internal/app/officeediting"
	materializationport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
	"context"
	"testing"
)

type officeResolverFixture struct{ calls int }

func (r *officeResolverFixture) ResolveActive(context.Context) (materializationport.ResultV1, error) {
	r.calls++
	return materializationport.ResultV1{}, nil
}
func TestQualifiedOfficeResolverChecksBeforeAndAfter(t *testing.T) {
	for _, badAt := range []int{0, 1, 2} {
		inner := &officeResolverFixture{}
		checks := 0
		r := qualifiedOfficeResolver{inner: inner, current: func(context.Context) bool { checks++; return checks != badAt }}
		_, err := r.ResolveActive(context.Background())
		if (err != nil) != (badAt != 0) {
			t.Fatalf("badAt %d: unexpected result", badAt)
		}
		if badAt == 1 && inner.calls != 0 {
			t.Fatal("stale package reached materialization")
		}
		if badAt != 1 && checks != 2 {
			t.Fatal("missing post-materialization package check")
		}
	}
}

type officeAdapterFixture struct{ calls int }

func (a *officeAdapterFixture) Readiness(context.Context, adapterport.Binding) (adapterport.Readiness, error) {
	a.calls++
	return adapterport.Readiness{Available: true}, nil
}
func (a *officeAdapterFixture) Invoke(context.Context, adapterport.Call) (adapterport.Result, error) {
	a.calls++
	return adapterport.Result{Output: []byte(`{"ok":true}`)}, nil
}
func TestQualifiedOfficeAdapterDriftMakesResultUnconfirmed(t *testing.T) {
	for _, invoke := range []bool{false, true} {
		for _, badAt := range []int{0, 1, 2} {
			inner := &officeAdapterFixture{}
			checks := 0
			a := qualifiedOfficeAdapter{inner: inner, current: func(context.Context) bool { checks++; return checks != badAt }}
			var err error
			if invoke {
				var result adapterport.Result
				result, err = a.Invoke(context.Background(), adapterport.Call{})
				if err != nil && len(result.Output) != 0 {
					t.Fatal("stale result leaked")
				}
			} else {
				_, err = a.Readiness(context.Background(), adapterport.Binding{})
			}
			if (err != nil) != (badAt != 0) {
				t.Fatal("incorrect qualification result")
			}
			if badAt == 1 && inner.calls != 0 {
				t.Fatal("stale qualification reached adapter")
			}
			if badAt != 1 && checks != 2 {
				t.Fatal("missing post call check")
			}
		}
	}
}

func TestQualifiedOfficeMapPreservesConcreteServerBinding(t *testing.T) {
	current := func(context.Context) bool { return true }
	office := officeediting.New("docx", nil, current)
	source := map[string]adapterport.Adapter{"analytix-documents": office}
	hosted := qualifyOfficeAdapters(source, current)
	concrete, ok := source["analytix-documents"].(*officeediting.Adapter)
	if !ok || concrete != office {
		t.Fatal("server selection binding lost concrete adapter")
	}
	wrapped, ok := hosted["analytix-documents"].(qualifiedOfficeAdapter)
	if !ok || wrapped.inner != concrete {
		t.Fatal("host and server do not share bound adapter")
	}
}
