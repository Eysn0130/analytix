//go:build !darwin && analytix_native_build_probe && !analytix_prod

package nativebuildcli

const ArgumentV1 = "--analytix-native-build-coordinator-v1"

func Main(any) int { return 125 }
