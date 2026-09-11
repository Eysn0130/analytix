package runtimeapp

import (
	"errors"
	"reflect"
	"sync"
	"testing"
)

func TestControlledPIIResourceOwnerClosesReverseOnceAndJoinsErrors(t *testing.T) {
	wantErr := errors.New("report close failed")
	var mu sync.Mutex
	var order []string
	newCloser := func(name string, closeErr error) *recordingControlledPIICloserV1 {
		return &recordingControlledPIICloserV1{close: func() error {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, name)
			return closeErr
		}}
	}
	owner := &controlledPIIResourceOwnerV1{}
	for _, resource := range []controlledPIIResourceCloserV1{
		newCloser("grant", nil), newCloser("report", wantErr), newCloser("access", nil),
	} {
		if err := owner.Add(resource); err != nil {
			t.Fatal(err)
		}
	}

	start := make(chan struct{})
	results := make(chan error, 16)
	var wait sync.WaitGroup
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			results <- owner.Close()
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	for err := range results {
		if !errors.Is(err, wantErr) {
			t.Fatalf("close did not preserve the joined error: %v", err)
		}
	}
	if !reflect.DeepEqual(order, []string{"access", "report", "grant"}) {
		t.Fatalf("resources closed in order %v", order)
	}
	if err := owner.Add(newCloser("late", nil)); !errors.Is(err, errControlledPIIResourceOwnerClosedV1) {
		t.Fatalf("closed owner accepted a resource: %v", err)
	}
}

type recordingControlledPIICloserV1 struct {
	close func() error
}

func (closer *recordingControlledPIICloserV1) Close() error {
	return closer.close()
}
