package server

import (
	"testing"

	appturnterminal "analytix.local/runtime-go/internal/app/turnterminal"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	caseterminaltest "analytix.local/runtime-go/internal/testsupport/caseturnterminal"
)

func newServerTestTurnTerminalCoordinator(t *testing.T, authority finalauthorityport.Authority, privateFinals finalauthorityport.PrivateFinalStore) *appturnterminal.Coordinator {
	t.Helper()
	coordinator, err := caseterminaltest.NewInMemoryCoordinatorV1(authority, privateFinals)
	if err != nil {
		t.Fatal(err)
	}
	return coordinator
}
