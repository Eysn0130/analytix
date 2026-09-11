//go:build !darwin && analytix_native_build_probe && !analytix_prod

package main

func openNativeBuildHostV1(any) (any, error) { return nil, nil }
