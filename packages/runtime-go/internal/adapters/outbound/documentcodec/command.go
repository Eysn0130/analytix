// Package documentcodec runs a fixed, bundled format encoder using bounded
// stdin/stdout. No workspace path or inherited credential environment is sent.
package documentcodec

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"path/filepath"
	"time"

	codecport "analytix.local/runtime-go/internal/ports/documentgeneration"
)

type Command struct {
	executable string
	entry      string
}

// New accepts trusted startup composition only, never model/tool parameters.
func New(executable, entry string) *Command {
	if !filepath.IsAbs(executable) || !filepath.IsAbs(entry) {
		return nil
	}
	return &Command{executable: executable, entry: entry}
}

type boundedOutput struct {
	bytes.Buffer
	limit int
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, codecport.ErrCodecUnavailable
	}
	return b.Buffer.Write(p)
}

func (c *Command) Encode(ctx context.Context, input codecport.Input) ([]byte, error) {
	if c == nil || ctx == nil || ctx.Err() != nil {
		return nil, codecport.ErrCodecUnavailable
	}
	body, err := json.Marshal(input)
	if err != nil || len(body) > 36<<20 {
		return nil, codecport.ErrCodecUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, c.executable, c.entry)
	command.Env = []string{"ELECTRON_RUN_AS_NODE=1", "LANG=en_US.UTF-8"}
	command.Dir = filepath.Dir(c.entry)
	command.Stdin = bytes.NewReader(body)
	output := &boundedOutput{limit: codecport.MaxDocumentBytes}
	command.Stdout, command.Stderr = output, io.Discard
	command.WaitDelay = time.Second
	if command.Run() != nil || output.Len() == 0 {
		return nil, codecport.ErrCodecUnavailable
	}
	return output.Bytes(), nil
}

func (c *Command) Supports(kind string) bool { return c != nil && (kind == "docx" || kind == "pptx") }
