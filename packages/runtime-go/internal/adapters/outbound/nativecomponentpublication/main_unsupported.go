//go:build !darwin && analytix_native_build_probe && !analytix_prod

package nativecomponentpublication

const PublisherArgumentV1 = "--analytix-native-generation-publisher-v1"

func Main() int { return 125 }
