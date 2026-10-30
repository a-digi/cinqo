package fetch

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testFetcher classifies servers by port, so servers that all listen on
// 127.0.0.1 can stand in for public, local and forbidden addresses.
func testFetcher(classes map[string]addrClass, certs ...*x509.Certificate) *Fetcher {
	pool := x509.NewCertPool()
	for _, c := range certs {
		pool.AddCert(c)
	}
	return &Fetcher{
		Timeout:   2 * time.Second,
		tlsConfig: &tls.Config{RootCAs: pool},
		classify: func(addr string) addrClass {
			_, port, _ := net.SplitHostPort(addr)
			return classes[port]
		},
	}
}

func port(s *httptest.Server) string {
	_, p, _ := net.SplitHostPort(s.Listener.Addr().String())
	return p
}

func get(f *Fetcher, url string, policy LocalPolicy) (*Result, error) {
	return f.Get(context.Background(), url, 1<<20, policy)
}

func ok(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }
}

func redirectTo(target string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target, http.StatusFound) }
}

func wantErr(t *testing.T, err error, contains string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), contains) {
		t.Fatalf("want error containing %q, got %v", contains, err)
	}
}

func TestLocalRepositoryOverHTTPIsAllowed(t *testing.T) {
	s := httptest.NewServer(ok("{}"))
	defer s.Close()
	// the real classifier: 127.0.0.1 is a local address
	res, err := get(New(), s.URL, LocalByFirstHop)
	if err != nil || string(res.Body) != "{}" || !res.Local {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestLocalRepositoryRedirectingToLocalIsAllowed(t *testing.T) {
	b := httptest.NewServer(ok("b"))
	defer b.Close()
	a := httptest.NewServer(redirectTo(b.URL))
	defer a.Close()
	res, err := get(New(), a.URL, LocalByFirstHop)
	if err != nil || string(res.Body) != "b" {
		t.Fatalf("%v", err)
	}
}

func TestPublicHTTPSRepositoryIsAllowed(t *testing.T) {
	s := httptest.NewTLSServer(ok("pub"))
	defer s.Close()
	f := testFetcher(map[string]addrClass{port(s): classPublic}, s.Certificate())
	res, err := get(f, s.URL, LocalByFirstHop)
	if err != nil || string(res.Body) != "pub" || res.Local {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestPublicRepositoryRedirectToLocalIsRefused(t *testing.T) {
	local := httptest.NewServer(ok("secret"))
	defer local.Close()
	pub := httptest.NewTLSServer(redirectTo(local.URL))
	defer pub.Close()
	f := testFetcher(map[string]addrClass{port(pub): classPublic, port(local): classLocal}, pub.Certificate())
	_, err := get(f, pub.URL, LocalByFirstHop)
	wantErr(t, err, "must not lead to a local address")
}

func TestPublicHTTPSRedirectToPlainHTTPPublicIsRefused(t *testing.T) {
	plain := httptest.NewServer(ok("x"))
	defer plain.Close()
	pub := httptest.NewTLSServer(redirectTo(plain.URL))
	defer pub.Close()
	f := testFetcher(map[string]addrClass{port(pub): classPublic, port(plain): classPublic}, pub.Certificate())
	_, err := get(f, pub.URL, LocalByFirstHop)
	wantErr(t, err, "plain http is only allowed for local addresses")
}

func TestPlainHTTPToPublicHostIsRefused(t *testing.T) {
	s := httptest.NewServer(ok("x"))
	defer s.Close()
	f := testFetcher(map[string]addrClass{port(s): classPublic})
	_, err := get(f, s.URL, LocalByFirstHop)
	wantErr(t, err, "plain http is only allowed for local addresses")
}

func TestMetadataAndUnspecifiedAddressesAreAlwaysRefused(t *testing.T) {
	s := httptest.NewServer(ok("x"))
	defer s.Close()
	f := testFetcher(map[string]addrClass{port(s): classBlocked})
	for _, p := range []LocalPolicy{LocalByFirstHop, LocalAllowed, LocalForbidden} {
		_, err := get(f, s.URL, p)
		wantErr(t, err, "this address is not allowed")
	}
	// the real classifier, by literal address (nothing is contacted)
	for _, u := range []string{"http://169.254.169.254/latest/meta-data", "http://0.0.0.0:1/", "http://[fe80::1]/", "http://[::]:1/"} {
		_, err := get(New(), u, LocalAllowed)
		wantErr(t, err, "this address is not allowed")
	}
}

func TestRedirectToMetadataAddressIsRefused(t *testing.T) {
	bad := httptest.NewServer(ok("x"))
	defer bad.Close()
	a := httptest.NewServer(redirectTo(bad.URL))
	defer a.Close()
	f := testFetcher(map[string]addrClass{port(a): classLocal, port(bad): classBlocked})
	_, err := get(f, a.URL, LocalByFirstHop)
	wantErr(t, err, "this address is not allowed")
}

func TestPoliciesForLaterDownloads(t *testing.T) {
	s := httptest.NewServer(ok("x"))
	defer s.Close()
	_, err := get(New(), s.URL, LocalForbidden)
	wantErr(t, err, "must not lead to a local address")
	if _, err := get(New(), s.URL, LocalAllowed); err != nil {
		t.Fatal(err)
	}
}

func TestOversizedBodyIsRefused(t *testing.T) {
	big := httptest.NewServer(ok(strings.Repeat("a", 2000)))
	defer big.Close()
	_, err := New().Get(context.Background(), big.URL, 1000, LocalByFirstHop)
	wantErr(t, err, "larger than")

	chunked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 4; i++ {
			w.Write([]byte(strings.Repeat("a", 500)))
			w.(http.Flusher).Flush()
		}
	}))
	defer chunked.Close()
	_, err = New().Get(context.Background(), chunked.URL, 1000, LocalByFirstHop)
	wantErr(t, err, "larger than")

	exact := httptest.NewServer(ok(strings.Repeat("a", 1000)))
	defer exact.Close()
	if _, err := New().Get(context.Background(), exact.URL, 1000, LocalByFirstHop); err != nil {
		t.Fatal(err)
	}
}

func TestSlowServerTimesOut(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(3 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer s.Close()
	f := New()
	f.Timeout = 200 * time.Millisecond
	_, err := get(f, s.URL, LocalByFirstHop)
	wantErr(t, err, "too long")
}

func TestTooManyRedirects(t *testing.T) {
	var s *httptest.Server
	s = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, s.URL+"/again", http.StatusFound)
	}))
	defer s.Close()
	_, err := get(New(), s.URL, LocalByFirstHop)
	wantErr(t, err, "too many redirects")
}

func TestErrorsNeverEchoTheBody(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("SECRET-INTERNAL-DETAIL"))
	}))
	defer s.Close()
	_, err := get(New(), s.URL, LocalByFirstHop)
	wantErr(t, err, "status 500")
	if strings.Contains(err.Error(), "SECRET") {
		t.Fatal("body echoed")
	}
}

func TestInvalidLinks(t *testing.T) {
	for _, u := range []string{"", "ftp://example.com/a", "file:///etc/passwd", "https://user:pw@example.com/a", "http:///a", "://x"} {
		if _, err := get(New(), u, LocalByFirstHop); err == nil {
			t.Errorf("%q should be refused", u)
		}
	}
}

func TestUnreachableHost(t *testing.T) {
	s := httptest.NewServer(ok("x"))
	url := s.URL
	s.Close()
	_, err := get(New(), url, LocalByFirstHop)
	wantErr(t, err, "could not be reached")
}
