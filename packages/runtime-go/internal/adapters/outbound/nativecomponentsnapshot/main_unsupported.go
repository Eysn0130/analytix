//go:build !darwin && analytix_native_build_probe && !analytix_prod

package nativecomponentsnapshot

const (
	ArgumentV1        = "--analytix-native-source-snapshot-v1"
	ArgumentDiscardV1 = "--analytix-native-source-snapshot-discard-v1"
)

// Main is deliberately unavailable off Darwin until an equivalent
// descriptor, ACL, and publication authority is implemented and tested.
func Main() int { return 125 }

func MainDiscard() int { return 125 }
