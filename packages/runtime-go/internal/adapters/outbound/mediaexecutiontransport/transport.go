package mediaexecutiontransport

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"analytix.local/runtime-go/internal/netclient"
	mediaport "analytix.local/runtime-go/internal/ports/mediaexecution"
)

type transport struct{ client *http.Client }

var _ mediaport.Transport = (*transport)(nil)

// New fixes proxy policy, timeout and redirect refusal for the complete media
// execution. A downloaded provider image therefore uses the same transport.
func New(proxy string, timeout time.Duration) (mediaport.Transport, error) {
	spec := netclient.ProxySpec{Mode: netclient.ModeOff}
	if raw := strings.TrimSpace(proxy); raw != "" {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawFragment != "" {
			return nil, errors.New("media transport proxy is invalid")
		}
		spec = netclient.ProxySpec{Mode: netclient.ModeCustom, URL: raw}
	}
	client, err := netclient.NewHTTPClient(spec, netclient.TransportOptions{
		DialTimeout: 30 * time.Second, KeepAlive: 30 * time.Second,
		TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 120 * time.Second,
		ClientTimeout: timeout,
	})
	if err != nil {
		return nil, err
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &transport{client: client}, nil
}

func (transport *transport) Send(ctx context.Context, input mediaport.Request) (*mediaport.Response, error) {
	if transport == nil || transport.client == nil || ctx == nil {
		return nil, mediaport.ErrInvalidRequest
	}
	request, err := http.NewRequestWithContext(ctx, input.Method, input.URL, input.Body)
	if err != nil {
		return nil, mediaport.ErrInvalidRequest
	}
	if input.ContentType != "" {
		request.Header.Set("Content-Type", input.ContentType)
	}
	if input.Credentialed {
		request.Header.Set("Authorization", "Bearer "+string(input.Credential))
	}
	defer request.Header.Del("Authorization")
	response, err := transport.client.Do(request)
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, err
	}
	return &mediaport.Response{StatusCode: response.StatusCode, ContentType: response.Header.Get("Content-Type"), Body: response.Body}, nil
}

func (transport *transport) Close() {
	if transport != nil && transport.client != nil {
		transport.client.CloseIdleConnections()
	}
}
