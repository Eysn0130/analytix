//go:build !analytix_dev_credentials || (!darwin && !linux && !windows)

package runtimeapp

import "os"

const developmentProviderAuthorityEnabled = false

func developmentProviderDirectorySecure(string, os.FileInfo) bool { return false }
