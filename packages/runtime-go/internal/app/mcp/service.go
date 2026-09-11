package mcp

import "analytix.local/runtime-go/internal/ports"

type Service struct {
	manager ports.MCPManager
}

func NewService(manager ports.MCPManager) *Service {
	return &Service{manager: manager}
}
