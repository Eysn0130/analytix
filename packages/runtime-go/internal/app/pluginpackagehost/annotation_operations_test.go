package pluginpackagehost

import (
	"context"
	"fmt"
	"testing"

	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

func TestAnnotationHostOperationLimit(t *testing.T) {
	f := fixture(t)
	for _, size := range []int{18, 19} {
		f.adapter.operations = nil
		for i := 0; i < size; i++ {
			f.adapter.operations = append(f.adapter.operations, fmt.Sprintf("operation-%d", i))
		}
		f.adapter.ready = true
		_, err := readiness(context.Background(), f.registration, adapterport.Binding{})
		if (err == nil) != (size == 18) {
			t.Fatalf("%d operations: %v", size, err)
		}
	}
}
