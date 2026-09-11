package server

import "net/http"

func NewRuntimeServerHandler(config RuntimeServerConfig) http.Handler {
	return newCompatibilityRuntimeServerHandler(config)
}
