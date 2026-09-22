package pluginpackagehost

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	officeapp "analytix.local/runtime-go/internal/app/officeediting"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
	materializationport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
)

// This observes a real file read through the production Office/object owners,
// not merely whether the Host called an adapter. The signed activation changes
// after the Host check but before the adapter admits its operation.
func TestOfficeEffectAdmissionRechecksHostActivation(t *testing.T) {
	for _, boundary := range []string{"current", "before-admission", "after-effect", "before-admission-disable-enable"} {
		t.Run(boundary, func(t *testing.T) {
			f := fixture(t)
			f.enable(t)
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "synthetic.txt")
			if err := os.WriteFile(path, []byte("SYNTH_PRIVATE_EFFECT_VALUE"), 0600); err != nil {
				t.Fatal(err)
			}
			revoke := func() {
				states := []domainplugin.DesiredStateV1{domainplugin.DesiredDisabledV1}
				if strings.HasSuffix(boundary, "disable-enable") {
					states = append(states, domainplugin.DesiredEnabledV1)
				}
				for _, state := range states {
					_, err := f.state.SetDesiredState(ctx, materializationport.SetDesiredStateRequestV1{GenerationID: f.state.current.Receipt.GenerationID, ExpectedRevision: f.state.activation.Revision, DesiredState: state}, f.authority, f.now.Add(time.Second))
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			files := &admissionFiles{path: path}
			if boundary == "after-effect" {
				files.afterRead = revoke
			}
			checks := 0
			adapter := officeapp.New("docx", editingapp.New(f.identity, files), func(context.Context) bool {
				checks++
				if checks == 2 && strings.HasPrefix(boundary, "before-admission") {
					revoke()
				}
				return true
			})
			registration := f.registration
			registration.Adapter = adapter
			host, err := New(f.identity, f.authority, []Registration{registration}, func() time.Time { return f.now })
			if err != nil {
				t.Fatal(err)
			}
			request := f.invokeRequest()
			request.Operation = "open-object"
			request.Input = json.RawMessage(`{"object":{"workspace":"/synthetic","path":"synthetic.txt"}}`)
			result, err := host.Invoke(ctx, request)
			expectedReads := 1
			if strings.HasPrefix(boundary, "before-admission") {
				expectedReads = 0
			}
			if files.reads != expectedReads {
				t.Fatalf("actual file reads=%d, want=%d", files.reads, expectedReads)
			}
			if boundary == "current" {
				if err != nil || !strings.Contains(string(result.Output), "SYNTH_PRIVATE_EFFECT_VALUE") {
					t.Fatal("current operation failed", err)
				}
			} else if err == nil || len(result.Output) != 0 {
				t.Fatal("revoked operation published an output")
			}
			if strings.HasSuffix(boundary, "disable-enable") {
				if _, err := host.Invoke(ctx, request); err == nil || files.reads != 0 {
					t.Fatal("old revision revived")
				}
				request.ExpectedRevision = f.state.activation.Revision
				if _, err := host.Invoke(ctx, request); err != nil || files.reads != 1 {
					t.Fatal("new revision did not recover", err)
				}
			}
		})
	}
}

type admissionFiles struct {
	path      string
	reads     int
	afterRead func()
}

func (f *admissionFiles) Read(_ context.Context, workspace, path string) (fileport.Document, error) {
	f.reads++
	body, err := os.ReadFile(f.path)
	if f.afterRead != nil {
		f.afterRead()
	}
	return fileport.Document{Workspace: workspace, IdentityPath: path, Path: path, Content: string(body), Revision: strings.Repeat("a", 64)}, err
}
func (*admissionFiles) Commit(context.Context, fileport.CommitInput) (fileport.Receipt, error) {
	panic("unexpected write")
}
func (*admissionFiles) Status(context.Context, string, string, string, string) (fileport.Receipt, error) {
	panic("unexpected status")
}
