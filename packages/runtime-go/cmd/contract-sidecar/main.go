//go:build !analytix_prod

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	agent "analytix.local/runtime-go/internal/agent"
	livelocal "analytix.local/runtime-go/internal/conformance/livelocal"
	mcpdomain "analytix.local/runtime-go/internal/mcp"
	provider "analytix.local/runtime-go/internal/provider"
)

type g1ContractFixture struct {
	RuntimeToken string `json:"runtimeToken"`
	StartedAt    string `json:"startedAt"`
}

type g2ContractFixture struct {
	Routes []livelocal.G2RouteReplayCase `json:"routes"`
}

func main() {
	addr := flag.String("addr", "127.0.0.1:0", "listen address")
	fixturesDir := flag.String("fixtures-dir", "../runtime/src/conformance/fixtures", "TypeScript contract fixture directory")
	durableTempDir := flag.String("durable-temp-dir", "", "explicit temp dir for durable conformance store")
	runtimeToken := flag.String("runtime-token", "", "optional runtime bearer token override for internal conformance gates")
	flag.Parse()

	g1, err := loadJSON[g1ContractFixture](filepath.Join(*fixturesDir, "go-g1-shadow-contract.json"))
	if err != nil {
		log.Fatalf("load G1 fixture: %v", err)
	}
	g2, err := loadJSON[g2ContractFixture](filepath.Join(*fixturesDir, "go-g2-route-replay-contract.json"))
	if err != nil {
		log.Fatalf("load G2 fixture: %v", err)
	}
	g3, err := loadJSON[provider.G3ProviderConformanceContract](filepath.Join(*fixturesDir, "go-g3-provider-streaming-usage-cache-contract.json"))
	if err != nil {
		log.Fatalf("load G3 fixture: %v", err)
	}
	g4, err := loadJSON[mcpdomain.G4ToolsConformanceContract](filepath.Join(*fixturesDir, "go-g4-tools-approval-user-input-mcp-contract.json"))
	if err != nil {
		log.Fatalf("load G4 fixture: %v", err)
	}
	approvalUserInput, err := loadJSON[agent.ApprovalUserInputRouteContract](filepath.Join(*fixturesDir, "approval-user-input-route-contract.json"))
	if err != nil {
		log.Fatalf("load approval/user-input fixture: %v", err)
	}
	mcp, err := loadJSON[mcpdomain.MCPToolLifecycleContract](filepath.Join(*fixturesDir, "mcp-tool-lifecycle-contract.json"))
	if err != nil {
		log.Fatalf("load MCP lifecycle fixture: %v", err)
	}
	loop, err := loadJSON[agent.GoMinimalAgentLoopContract](filepath.Join(*fixturesDir, "go-minimal-agent-loop-contract.json"))
	if err != nil {
		log.Fatalf("load minimal loop fixture: %v", err)
	}
	kernel, err := loadJSON[livelocal.GoKernelLiveScaffoldContract](filepath.Join(*fixturesDir, "go-kernel-live-scaffold-contract.json"))
	if err != nil {
		log.Fatalf("load kernel scaffold fixture: %v", err)
	}
	if *runtimeToken != "" {
		g1.RuntimeToken = *runtimeToken
	}
	handler := livelocal.NewLiveLocalSidecarHarness(livelocal.LiveLocalSidecarConfig{
		RuntimeToken:              g1.RuntimeToken,
		StartedAt:                 g1.StartedAt,
		Routes:                    g2.Routes,
		ProviderContract:          g3,
		G4Contract:                g4,
		ApprovalUserInputContract: approvalUserInput,
		MCPToolLifecycleContract:  mcp,
		DurableTempDir:            *durableTempDir,
		LoopContract:              loop,
		KernelContract:            kernel,
	})
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: handler}
	ready := map[string]any{
		"url":            "http://" + listener.Addr().String(),
		"runtimeToken":   g1.RuntimeToken,
		"durableTempDir": *durableTempDir,
	}
	readyJSON, _ := json.Marshal(ready)
	fmt.Printf("ANALYTIX_SIDECAR_READY %s\n", readyJSON)

	errCh := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case <-stop:
	case err := <-errCh:
		if err != nil {
			log.Fatalf("serve: %v", err)
		}
		return
	}
	_ = server.Shutdown(context.Background())
}

func loadJSON[T any](path string) (T, error) {
	var out T
	data, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}
