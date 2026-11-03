package fetch

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
)

// RefusedError is what an AllowURL function returns to refuse an address: its reason is safe to show and survives the
// HTTP client's own error wrapping.
type RefusedError struct{ Reason string }

func (e *RefusedError) Error() string { return e.Reason }

// StreamOptions are the choices of one streaming download.
type StreamOptions struct {
	Policy LocalPolicy
	// Header is added to the request (a Range header to resume, for example).
	Header http.Header
	// AllowURL, when set, is asked about the first address and about every redirect target; an error refuses it. This is
	// how a feature limits a download to the hosts it expects.
	AllowURL func(*url.URL) error
}

// Stream is an open response whose body is read by the caller. Unlike Get it has no limit on the whole request (a
// download of gigabytes takes minutes) and never buffers the body: the caller reads it, applies its own size limit and
// stall detection, and must call Close.
type Stream struct {
	Status        int
	ContentLength int64 // -1 when unknown
	Header        http.Header
	Body          io.ReadCloser
	// Local reports whether the server was reached on a local address.
	Local  bool
	closer func()
	g      *guard
}

// Close releases the connection.
func (s *Stream) Close() {
	if s.Body != nil {
		s.Body.Close()
	}
	if s.closer != nil {
		s.closer()
	}
}

// Stream opens rawURL under the same address guard as Get. A response with any HTTP status is returned (the caller decides
// what 200, 206 or 416 mean); only a request that could not be made at all is an error, and its message never contains
// what the server sent.
func (f *Fetcher) Stream(ctx context.Context, rawURL string, opts StreamOptions) (*Stream, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" {
		return nil, errors.New("the link is not a valid http(s) address")
	}
	if u.User != nil {
		return nil, errors.New("the link must not contain credentials")
	}
	if opts.AllowURL != nil {
		if err := opts.AllowURL(u); err != nil {
			return nil, err
		}
	}
	g := f.newGuard(opts.Policy, u.Scheme)
	client := f.newClient(g, 0, opts.AllowURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.New("the link is not a valid http(s) address")
	}
	req.Header.Set("User-Agent", "cinqo-download")
	for name, values := range opts.Header {
		for _, v := range values {
			req.Header.Add(name, v)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		client.CloseIdleConnections()
		return nil, describe(err)
	}
	g.mu.Lock()
	local := g.local
	g.mu.Unlock()
	return &Stream{
		Status:        resp.StatusCode,
		ContentLength: resp.ContentLength,
		Header:        resp.Header,
		Body:          resp.Body,
		Local:         local,
		closer:        client.CloseIdleConnections,
		g:             g,
	}, nil
}
