//go:build !analytix_prod

package runtimeapp

import "strings"

func runtimeStartupPersistenceMode(config Config) string {
	if strings.TrimSpace(config.ProductionDurableRoot) != "" {
		return "production"
	}
	if strings.TrimSpace(config.CandidateDurableRoot) != "" {
		return "candidate"
	}
	return "temp"
}
