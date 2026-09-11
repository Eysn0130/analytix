//go:build !analytix_prod

package livelocal

import "testing"

func TestRunDurableReplayContractUsesOrdinarySafeFixtureIdentity(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunDurableReplayContract(map[string]any{
		"deepseekCacheHitTokens":  int64(21),
		"deepseekCacheMissTokens": int64(3),
		"deepseekCacheHitRate":    0.875,
	}, store)
	if err != nil {
		t.Fatal(err)
	}
	if result["durableReplayFromEventSink"] != true || result["usageCacheAccountingPersisted"] != true {
		t.Fatalf("durable replay result = %#v", result)
	}
}
