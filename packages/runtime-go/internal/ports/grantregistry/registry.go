package grantregistry

// Reader is the durable authority boundary used to rebuild the append-only
// execution-grant registry projection from host-owned turn items. A cached or
// provider-supplied grant object never implements registry membership.
type Reader interface {
	GetThread(string) (map[string]any, error)
}
