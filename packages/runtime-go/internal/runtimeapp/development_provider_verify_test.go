//go:build analytix_dev_credentials && (darwin || linux)

package runtimeapp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
	provider "analytix.local/runtime-go/internal/provider"
)

type developmentVerificationClient func(context.Context, provider.Request) (provider.Result, error)

func (f developmentVerificationClient) Stream(ctx context.Context, r provider.Request) (provider.Result, error) {
	return f(ctx, r)
}

func TestDevelopmentVerificationBootstrapReuseFailureReplacementAndNoLeak(t *testing.T) {
	parent, _ := filepath.EvalSymlinks(t.TempDir())
	_ = os.Chmod(parent, 0700)
	root := filepath.Join(parent, "provider-credentials")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	current := "synthetic-development-first"
	calls := 0
	client := developmentVerificationClient(func(ctx context.Context, r provider.Request) (provider.Result, error) {
		calls++
		if r.APIKey != current || r.Model != "deepseek-flash" || r.BaseURL != "https://api.deepseek.com" || len(r.Tools) != 0 || r.MaxOutputTokens != 32 {
			t.Fatal("verification escaped bounded committed Provider")
		}
		if err := r.PrivateProviderCurrentnessBeforeSend(0); err != nil {
			return provider.Result{}, err
		}
		return provider.Result{StreamCompleted: true, Chunks: []provider.Chunk{{Kind: provider.ChunkText, Text: "OK"}}, Usage: provider.Usage{PromptTokens: 10, CompletionTokens: 1}}, nil
	})
	run := func(input *strings.Reader, wantPass bool, c developmentVerificationClient) {
		t.Helper()
		var out bytes.Buffer
		var err error
		if input == nil {
			err = verifyDevelopmentProvider(ctx, root, nil, &out, c)
		} else {
			err = verifyDevelopmentProvider(ctx, root, input, &out, c)
		}
		if (err == nil) != wantPass {
			t.Fatalf("verification success=%v expected=%v", err == nil, wantPass)
		}
		if strings.Contains(out.String(), current) || strings.Contains(out.String(), "Authorization") {
			t.Fatal("credential leaked")
		}
	}
	run(strings.NewReader(current), true, client)
	run(nil, true, client) // independently opens and closes normal authority each call
	failed := developmentVerificationClient(func(context.Context, provider.Request) (provider.Result, error) {
		return provider.Result{}, errors.New("unsafe-response-" + current)
	})
	run(nil, false, failed)
	run(nil, true, client)
	a, err := openDevelopmentProviderAuthority(ctx, Config{DevelopmentProviderAuthorityDir: root}, false)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := a.Manager().Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	current = "synthetic-development-replacement"
	mutation, _ := secretstoreport.SetCredential([]byte(current))
	_, err = a.Manager().ReplaceCredential(ctx, providerregistryapp.CredentialReplaceCommand{ProviderID: "deepseek", Expected: expectedProviderRegistryStateV1(snapshot, "deepseek"), CredentialPurpose: "provider-api-key", Credential: mutation})
	if err != nil {
		t.Fatal(err)
	}
	a.Close()
	run(nil, true, client)
	if calls != 4 {
		t.Fatalf("unexpected calls %d", calls)
	}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if info.Mode().Perm() != 0700 {
				t.Fatal("non-private directory")
			}
			return nil
		}
		if info.Mode().Perm() != 0600 {
			t.Fatal("non-private file")
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(body, []byte(current)) || bytes.Contains(body, []byte("synthetic-development-first")) {
			t.Fatal("plaintext credential persisted")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
