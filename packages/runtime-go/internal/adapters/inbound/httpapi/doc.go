// Package httpapi is the inbound HTTP adapter boundary for runtime-go.
// It owns route matching, auth/dispatch shell behavior, and HTTP/SSE response
// encoding while internal/server still supplies concrete route implementations
// during migration.
package httpapi
