//go:build !race

package runtimeapp

func runRuntimeAppRaceTestShards() (int, bool) {
	return 0, false
}
