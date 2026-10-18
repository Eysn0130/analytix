package mcp

import (
	"context"
	"errors"
	"sync"

	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
)

var errMCPEffectAdmission = errors.New("MCP uncontained effect admission is unavailable")

type mcpEffectAdmission struct {
	mu      sync.Mutex
	begin   func(context.Context) error
	started bool
}

// BindEffectAdmission is a one-time production composition step before Connect.
// The callback shares the runtime editing Registry and must not reenter manager.
func (m *ProductionManager) BindEffectAdmission(begin func(context.Context) error) error {
	if m == nil || begin == nil {
		return errMCPEffectAdmission
	}
	m.sourceExecutionMu.Lock()
	defer m.sourceExecutionMu.Unlock()
	m.effectAdmission.mu.Lock()
	defer m.effectAdmission.mu.Unlock()
	if m.effectAdmission.begin != nil || m.effectAdmission.started {
		return errMCPEffectAdmission
	}
	m.effectAdmission.begin = begin
	return nil
}

func (m *ProductionManager) effectAdmissionCallback() (func(context.Context) error, error) {
	m.effectAdmission.mu.Lock()
	defer m.effectAdmission.mu.Unlock()
	m.effectAdmission.started = true
	if m.effectAdmission.begin == nil {
		return nil, errMCPEffectAdmission
	}
	return m.effectAdmission.begin, nil
}

// The caller holds the source lease/current binding across classification and
// effect. Only concrete private clients can avoid opaque admission. Remote
// names, readonly hints, loopback addresses and config policy are not authority.
func (m *ProductionManager) admitMCPEffect(ctx context.Context, client mcpTransportClient, rawName string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	begin, err := m.effectAdmissionCallback()
	if err != nil {
		return err
	}
	switch private := client.(type) {
	case *hostScheduleClientV1:
		if private.owner == m && (rawName == "" || rawName == hostScheduleMCPListToolNameV1 || rawName == hostScheduleMCPLegacyListToolNameV1) {
			return nil
		}
	}
	if begin(ctx) != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errMCPEffectAdmission
	}
	return nil
}

func (m *ProductionManager) newAdmittedMCPTransport(ctx context.Context, spec ServerSpec, proxy string, protected []string) (mcpTransportClient, error) {
	if err := m.admitMCPEffect(ctx, nil, ""); err != nil {
		return nil, err
	}
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return newMCPTransportClient(spec, proxy, protected)
}

func (m *ProductionManager) admittedToolCall(ctx context.Context, client mcpTransportClient, name string, arguments map[string]any, hostContext *domainmcp.HostContextEnvelope) (any, error) {
	if err := m.admitMCPEffect(ctx, client, name); err != nil {
		return nil, err
	}
	return callMCPTransportTool(ctx, client, name, arguments, hostContext)
}
