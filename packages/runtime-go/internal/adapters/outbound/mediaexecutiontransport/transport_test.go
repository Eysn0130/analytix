package mediaexecutiontransport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	mediaport "analytix.local/runtime-go/internal/ports/mediaexecution"
)

func TestTransportFixesProxyTimeoutAndRedirectPolicy(t *testing.T) {
	for _, proxy := range []string{"", "http://proxy.example:8080"} {
		t.Run(proxy, func(t *testing.T) {
			created, err := New(proxy, 42*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer created.Close()
			client := created.(*transport).client
			network := client.Transport.(*http.Transport)
			if client.Timeout != 42*time.Second || network.TLSHandshakeTimeout != 15*time.Second || network.ResponseHeaderTimeout != 120*time.Second {
				t.Fatal("transport timeout policy changed")
			}
			if !errors.Is(client.CheckRedirect(nil, nil), http.ErrUseLastResponse) {
				t.Fatal("redirect must be refused")
			}
			if proxy == "" {
				if network.Proxy != nil {
					t.Fatal("unconfigured proxy must ignore environment")
				}
			} else {
				request, _ := http.NewRequest(http.MethodGet, "https://provider.example", nil)
				resolved, err := network.Proxy(request)
				if err != nil || resolved == nil || resolved.String() != proxy {
					t.Fatal("committed proxy was not retained")
				}
			}
		})
	}
	for _, proxy := range []string{"http://user:password@proxy.example", "http://proxy.example?query", "http://proxy.example?", "http://proxy.example#fragment"} {
		if _, err := New(proxy, time.Second); err == nil {
			t.Fatal("unsafe proxy accepted")
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

type observedBody struct {
	read   bool
	closed bool
}

func (body *observedBody) Read([]byte) (int, error) { body.read = true; return 0, io.EOF }
func (body *observedBody) Close() error             { body.closed = true; return nil }

func TestSendPreservesStreamingAndClearsCredentialOnEveryReturn(t *testing.T) {
	for _, credentialed := range []bool{false, true} {
		for _, fails := range []bool{false, true} {
			body := &observedBody{}
			var captured *http.Request
			ctx, cancel := context.WithCancel(context.Background())
			client := &transport{client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				captured = request
				wantAuthorization := ""
				if credentialed {
					wantAuthorization = "Bearer synthetic-test-key"
				}
				if request.Header.Get("Authorization") != wantAuthorization || request.Header.Get("Content-Type") != "application/json" || request.Context() != ctx {
					t.Fatal("request authority, content type or context changed")
				}
				if fails {
					return nil, errors.New("synthetic transport failure")
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/png"}}, Body: body}, nil
			})}}
			response, err := client.Send(ctx, mediaport.Request{Method: "POST", URL: "https://provider.example", Body: strings.NewReader("{}"), ContentType: "application/json", Credential: []byte("synthetic-test-key"), Credentialed: credentialed})
			cancel()
			if (err != nil) != fails || captured == nil || captured.Header.Get("Authorization") != "" {
				t.Fatal("send result or credential cleanup changed")
			}
			if !fails {
				if response.StatusCode != http.StatusOK || response.ContentType != "image/png" || response.Body != body || body.read || body.closed {
					t.Fatal("application must own bounded reading and closing after header revalidation")
				}
				_ = response.Body.Close()
			}
		}
	}
}
