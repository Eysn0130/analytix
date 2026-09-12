package providerregistry

import "testing"

func TestAccountObservationBindingRequiresExactReadOnlyMethod(t *testing.T) {
	for _, method := range []string{"GET", "POST", "HEAD", "get", " GET", "GET ", ""} {
		t.Run(method, func(t *testing.T) {
			binding := AccountObservationBinding{
				SchemaVersion: 1,
				Method:        method,
				Endpoint:      "https://provider.invalid/quota",
				Projection:    "normalized-quota-v1",
			}
			err := binding.Validate()
			if method == "GET" {
				if err != nil {
					t.Fatalf("read-only observation rejected: %v", err)
				}
			} else if err != ErrInvalidRegistry {
				t.Fatalf("invalid observation method accepted: %v", err)
			}
		})
	}
}
