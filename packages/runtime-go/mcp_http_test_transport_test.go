package runtimego

import runtimemcp "analytix.local/runtime-go/internal/mcp"

func init() {
	if err := runtimemcp.UseLoopbackHTTPTransportForTests(); err != nil {
		panic(err)
	}
}
