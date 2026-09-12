//go:build !analytix_prod

package runtimego

import "testing"

func TestG5SpeechSettingsGuardRejectsFacadeAndSourceDrift(t *testing.T) {
	for _, mutation := range []string{"facade", "source", "reader", "evidence"} {
		t.Run(mutation, func(t *testing.T) {
			input := loadG5FullLoopContract(t).ControlExecutableCases
			matrix := &input.DesktopSovereignty.RendererSettingsReadFacadeMatrix
			switch mutation {
			case "facade":
				matrix.Facade = "window.analytix.settings.getSettings"
			case "source":
				matrix.SpeechToTextSourceUsesSettingsClient = false
			case "reader":
				matrix.SettingsReaders = []string{"useSpeechToTextSettings"}
			case "evidence":
				input.DesktopSovereignty.EvidenceIDs = nil
			}
			if BuildG5ControlExecutableOutput(input).DesktopSovereignty.RendererSettingsReadFacadeCoversSpeechToText {
				t.Fatal("speech settings guard accepted invalid facade/source evidence")
			}
		})
	}
}
