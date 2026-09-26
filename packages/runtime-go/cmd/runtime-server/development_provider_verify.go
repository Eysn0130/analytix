package main

import (
	"analytix.local/runtime-go/internal/runtimeapp"
	"context"
	"errors"
	"flag"
	"io"
)

func runDevelopmentProviderVerify(args []string, input io.Reader, output io.Writer) error {
	if len(args) == 0 || args[0] != "verify" {
		return errors.New("expected provider verify")
	}
	flags := flag.NewFlagSet("provider verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("development-authority-dir", "", "existing protected development authority")
	bootstrap := flags.Bool("bootstrap-stdin", false, "one-time credential input from stdin")
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 || *root == "" {
		return errors.New("invalid provider verification arguments")
	}
	if !*bootstrap {
		input = nil
	}
	return runtimeapp.VerifyDevelopmentProvider(context.Background(), *root, input, output)
}
