//go:build !analytix_prod

package runtimego

import (
	"encoding/json"
	"sort"

	contracts "analytix.local/runtime-go/internal/contracts"
)

// Compatibility shim: shared contract helpers live in internal/contracts. Root
// wrappers keep legacy package-local tests from reaching into internal paths.
type LiveLocalSidecarBoundary = contracts.LiveLocalSidecarBoundary
type ProductBoundary = contracts.ProductBoundary

func shortHexHash(data []byte) string {
	return contracts.ShortHexHash(data)
}

func LiveProductionCandidateProductBoundary() LiveLocalSidecarBoundary {
	return contracts.LiveProductionCandidateProductBoundary()
}

func RuntimeServerContractProductBoundary() LiveLocalSidecarBoundary {
	return contracts.RuntimeServerContractProductBoundary()
}

func threadSummary(thread map[string]any) map[string]any {
	return contracts.ThreadSummary(thread)
}

func responseID(body json.RawMessage) string {
	return contracts.ResponseID(body)
}

func stringField(record map[string]any, key string) string {
	return contracts.StringField(record, key)
}

func cloneMap(value map[string]any) map[string]any {
	return contracts.CloneMap(value)
}

func cloneValue(value any) any {
	return contracts.CloneValue(value)
}

func numericSeq(value any) (int, bool) {
	return contracts.NumericSeq(value)
}

func containsString(values []string, needle string) bool {
	return contracts.ContainsString(values, needle)
}

func safeDurableID(value string) string {
	return contracts.SafeRecordID(value)
}

func countThreadItems(thread map[string]any) int {
	return contracts.CountThreadItems(thread)
}

func eventKinds(events []map[string]any) []string {
	return contracts.EventKinds(events)
}

func allPersistBeforePublish(orders [][]string) bool {
	return contracts.AllPersistBeforePublish(orders)
}

func sameStringSet(actual []string, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	left := append([]string(nil), actual...)
	right := append([]string(nil), expected...)
	sort.Strings(left)
	sort.Strings(right)
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
