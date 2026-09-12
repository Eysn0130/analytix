package mediaexecution

import (
	"context"
	"errors"
	"io"
	"time"
)

var ErrInvalidRequest = errors.New("media transport request is invalid")

// Factory creates one transport for an execution and its optional image
// download. The application selects the committed proxy and bounded timeout.
type Factory func(proxy string, timeout time.Duration) (Transport, error)

// Transport returns after response headers so the application can revalidate
// authority before reading its bounded body. It must not follow redirects or
// retain Credential after Send. The caller owns the response body and Close.
type Transport interface {
	Send(context.Context, Request) (*Response, error)
	Close()
}

type Request struct {
	Method       string
	URL          string
	Body         io.Reader
	ContentType  string
	Credential   []byte
	Credentialed bool
}

type Response struct {
	StatusCode  int
	ContentType string
	Body        io.ReadCloser
}
