package client

import (
	"crypto/tls"
	"net/http/httptrace"
	"sync"
	"time"
)

// These are monotonic offsets from transport invocation, not independently
// additive phase durations. Hooks can overlap (for example parallel dials) or
// run after Do returns. Keep only the first observed milestone, never addresses,
// certificates, headers, connection objects or provider content.
type providerTransportTrace struct {
	mu      sync.Mutex
	started time.Time
	values  map[string]any
}

func newProviderTransportTrace(started time.Time) *providerTransportTrace {
	return &providerTransportTrace{started: started, values: make(map[string]any, 9)}
}

func (t *providerTransportTrace) milestone(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.values[key]; !exists {
		t.values[key] = float64(time.Since(t.started)) / float64(time.Millisecond)
	}
}

func (t *providerTransportTrace) hooks() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		DNSDone: func(info httptrace.DNSDoneInfo) {
			if info.Err == nil {
				t.milestone("provider_dns_done_ms")
			}
		},
		ConnectDone: func(_, _ string, err error) {
			if err == nil {
				t.milestone("provider_connect_done_ms")
			}
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			if err == nil {
				t.milestone("provider_tls_done_ms")
			}
		},
		GotConn: func(info httptrace.GotConnInfo) {
			t.mu.Lock()
			defer t.mu.Unlock()
			if _, exists := t.values["provider_connection_acquired_ms"]; !exists {
				t.values["provider_connection_acquired_ms"] = float64(time.Since(t.started)) / float64(time.Millisecond)
				t.values["provider_connection_reused"] = info.Reused
				t.values["provider_connection_was_idle"] = info.WasIdle
				t.values["provider_connection_idle_ms"] = float64(info.IdleTime) / float64(time.Millisecond)
			}
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				t.milestone("provider_request_written_ms")
			}
		},
		GotFirstResponseByte: func() { t.milestone("provider_first_response_byte_ms") },
	}
}

func (t *providerTransportTrace) snapshotInto(details map[string]any) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for key, value := range t.values {
		details[key] = value
	}
}
