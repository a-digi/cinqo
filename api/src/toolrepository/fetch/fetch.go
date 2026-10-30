// Package fetch is the guarded HTTP client for tool repositories: it downloads
// a repository file (and, in a later step, packages) from an address an
// administrator typed or a repository listed — see
// plan/ai/tools/repository/tool-repositories.md, "Security considerations".
//
// Local addresses (localhost, a private network) are allowed. The limits that
// always hold are enforced where the connection is made, on the resolved IP, so
// DNS rebinding cannot swap an address after a check:
//   - link-local (including the cloud metadata address 169.254.169.254),
//     unspecified and multicast addresses are never reached;
//   - plain http is only used for local addresses;
//   - an address on a public network never leads to a local one, whether by a
//     redirect or a later download;
//   - at most 5 redirects, a timeout, a size cap, and no proxy.
package fetch

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	DefaultTimeout = 20 * time.Second
	MaxRedirects   = 5
)

// LocalPolicy says whether a request may reach local addresses.
type LocalPolicy int

const (
	// LocalByFirstHop lets the first connection decide: a repository on a local
	// address may use local addresses, one on a public address may not.
	LocalByFirstHop LocalPolicy = iota
	// LocalAllowed is for a download that belongs to a local repository.
	LocalAllowed
	// LocalForbidden is for a download that belongs to a public repository.
	LocalForbidden
)

type addrClass int

const (
	classPublic addrClass = iota
	classLocal
	classBlocked
)

// Result is a successful download.
type Result struct {
	Body []byte
	// Local reports whether the server was reached on a local address, so a
	// download that belongs to it can use LocalAllowed or LocalForbidden.
	Local bool
}

// Fetcher downloads from admin- and repository-supplied addresses.
type Fetcher struct {
	Timeout time.Duration
	// classify maps a resolved "ip:port" to its class; tests replace it.
	classify func(addr string) addrClass
	// tlsConfig is nil in production; tests trust their own server certificates.
	tlsConfig *tls.Config
}

func New() *Fetcher { return &Fetcher{Timeout: DefaultTimeout, classify: classifyAddr} }

func classifyAddr(addr string) addrClass {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return classBlocked
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return classBlocked
	}
	switch {
	case ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast(), ip.IsUnspecified(), ip.IsMulticast():
		return classBlocked
	case ip.IsLoopback(), ip.IsPrivate():
		return classLocal
	}
	return classPublic
}

// guard is the per-request state shared by every connection of one download.
type guard struct {
	mu         sync.Mutex
	classify   func(string) addrClass
	policy     LocalPolicy
	decided    bool
	allowLocal bool
	local      bool
	scheme     string
}

func (g *guard) setScheme(s string) {
	g.mu.Lock()
	g.scheme = s
	g.mu.Unlock()
}

func (g *guard) control(_, address string, _ syscall.RawConn) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	class := g.classify(address)
	if class == classBlocked {
		return errors.New("this address is not allowed")
	}
	if !g.decided {
		g.decided = true
		g.local = class == classLocal
		switch g.policy {
		case LocalAllowed:
			g.allowLocal = true
		case LocalForbidden:
			g.allowLocal = false
		default:
			g.allowLocal = g.local
		}
	}
	if class == classLocal && !g.allowLocal {
		return errors.New("a public address must not lead to a local address")
	}
	if g.scheme == "http" && class != classLocal {
		return errors.New("plain http is only allowed for local addresses")
	}
	return nil
}

// Get downloads rawURL, reading at most maxBytes. Errors never contain the
// response body.
func (f *Fetcher) Get(ctx context.Context, rawURL string, maxBytes int64, policy LocalPolicy) (*Result, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" {
		return nil, errors.New("the link is not a valid http(s) address")
	}
	if u.User != nil {
		return nil, errors.New("the link must not contain credentials")
	}
	g := &guard{classify: f.classify, policy: policy, scheme: u.Scheme}
	if g.classify == nil {
		g.classify = classifyAddr
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: g.control}
	client := &http.Client{
		Timeout: f.timeout(),
		Transport: &http.Transport{
			Proxy:               nil,
			DialContext:         dialer.DialContext,
			DisableKeepAlives:   true,
			TLSClientConfig:     f.tlsConfig,
			TLSHandshakeTimeout: 10 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > MaxRedirects {
				return fmt.Errorf("too many redirects (more than %d)", MaxRedirects)
			}
			if req.URL.User != nil {
				return errors.New("a redirect to a link with credentials is not followed")
			}
			g.setScheme(req.URL.Scheme)
			return nil
		},
	}
	defer client.CloseIdleConnections()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.New("the link is not a valid http(s) address")
	}
	req.Header.Set("User-Agent", "cinqo-tool-repository")
	resp, err := client.Do(req)
	if err != nil {
		return nil, describe(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the server answered with status %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return nil, fmt.Errorf("the file is larger than %d bytes", maxBytes)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, describe(err)
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("the file is larger than %d bytes", maxBytes)
	}
	g.mu.Lock()
	local := g.local
	g.mu.Unlock()
	return &Result{Body: body, Local: local}, nil
}

func (f *Fetcher) timeout() time.Duration {
	if f.Timeout > 0 {
		return f.Timeout
	}
	return DefaultTimeout
}

// describe turns a transport error into a short message without echoing
// anything the server sent.
func describe(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("the server took too long to answer")
	case errors.Is(err, context.Canceled):
		return errors.New("the request was cancelled")
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return errors.New("the server took too long to answer")
	}
	for _, known := range []string{
		"this address is not allowed",
		"a public address must not lead to a local address",
		"plain http is only allowed for local addresses",
		"too many redirects",
		"redirect to a link with credentials",
	} {
		if strings.Contains(err.Error(), known) {
			return errors.New(known)
		}
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return errors.New("the host could not be found")
	}
	return errors.New("the server could not be reached")
}
