//go:build !analytix_dev_credentials || (!darwin && !linux)

package runtimeapp

import "os"

const developmentProviderAuthorityEnabled = false

func developmentProviderDirectoryOwned(os.FileInfo) bool { return false }
