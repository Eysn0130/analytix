//go:build !darwin

package nativecomponenthost

func OpenProduction(string) (*Owner, error) {
	return nil, ErrUnavailable
}

func LocalBuildPackageActive() bool { return false }
