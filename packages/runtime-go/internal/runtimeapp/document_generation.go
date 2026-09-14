package runtimeapp

import (
	"os"
	"path/filepath"

	"analytix.local/runtime-go/internal/adapters/outbound/documentcodec"
	"analytix.local/runtime-go/internal/adapters/outbound/officecodec"
	codecport "analytix.local/runtime-go/internal/ports/documentgeneration"
)

func newDocumentCodec(config Config) codecport.Codec {
	return officecodec.New(newDocumentNodeCodec(config))
}

func newDocumentNodeCodec(config Config) codecport.Codec {
	if !filepath.IsAbs(config.DocumentCodecExecutable) || !filepath.IsAbs(config.DocumentCodecEntry) || filepath.Base(config.DocumentCodecEntry) != "office-generation-codec-entry.js" {
		return nil
	}
	entry, err := os.Stat(config.DocumentCodecEntry)
	if err != nil || !entry.Mode().IsRegular() || entry.Size() == 0 || entry.Size() > 16<<20 {
		return nil
	}
	executable, err := os.Stat(config.DocumentCodecExecutable)
	if err != nil || !executable.Mode().IsRegular() {
		return nil
	}
	return documentcodec.New(config.DocumentCodecExecutable, config.DocumentCodecEntry)
}
