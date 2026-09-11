//go:build !analytix_prod

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	runtimego "analytix.local/runtime-go"
	evidence "analytix.local/runtime-go/internal/upstreamaudit"
)

func main() {
	readiness := runtimego.RuntimeReadinessStatusFromEnv(envMap(os.Environ()))
	matrix := evidence.BuildReasonixSuperiorityMatrix(readiness)
	data, err := json.MarshalIndent(matrix, "", "  ")
	if err != nil {
		log.Fatalf("marshal Reasonix superiority matrix: %v", err)
	}
	fmt.Printf("%s\n", data)
}

func envMap(values []string) map[string]string {
	out := map[string]string{}
	for _, value := range values {
		key, item, ok := strings.Cut(value, "=")
		if ok {
			out[key] = item
		}
	}
	return out
}
