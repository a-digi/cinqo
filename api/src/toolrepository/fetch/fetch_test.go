package fetch

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func stream(f *Fetcher, url string, opts StreamOptions) (*Stream, error) {
	return f.Stream(context.Background(), url, opts)
}

func readAll(t *testing.T, s *Stream) string {
	t.Helper()
	defer s.Close()
	b := make([]byte, 0, 64)
	buf := make([]byte, 16)
	for {
		n, err := s.Body.Read(buf)
		b = append(b, buf[:n]...)
		if err != nil {
			break
		}
	}
	return string(b)
}

func TestStreamReturnsTheBodyWithoutBufferingAndReportsTheStatus(t *testing.T) {
	s := httptest.NewServer(ok("hello stream"))
	defer s.Close()
	f := testFetcher(map[string]addrClass{port(s): classLocal})
	st, err := stream(f, s.URL, StreamOptions{Policy: LocalAllowed})
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != 200 || st.ContentLength != int64(len("hello stream")) || !st.Local {
		t.Errorf("status %d length %d local %v", st.Status, st.ContentLength, st.Local)
	}
	if got := readAll(t, st); got != "hello stream" {
		t.Errorf("body = %q", got)
	}
}

func TestStreamSendsTheCallersHeadersAndReturnsEveryStatus(t *testing.T) {
	var gotRange string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRange = r.Header.Get("Range")
		w.Header().Set("Content-Range", "bytes 5-9/10")
		w.WriteHeader(http.StatusPartialContent)
		w.Write([]byte("fghij"))
	}))
	defer s.Close()
	f := testFetcher(map[string]addrClass{port(s): classLocal})
	st, err := stream(f, s.URL, StreamOptions{Policy: LocalAllowed, Header: http.Header{"Range": {"bytes=5-"}}})
	if err != nil {
		t.Fatal(err)
	}
	if gotRange != "bytes=5-" || st.Status != http.StatusPartialContent || st.Header.Get("Content-Range") != "bytes 5-9/10" {
		t.Errorf("range %q status %d content-range %q", gotRange, st.Status, st.Header.Get("Content-Range"))
	}
	st.Close()

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "nope", http.StatusTeapot) }))
	defer bad.Close()
	f = testFetcher(map[string]addrClass{port(bad): classLocal})
	st, err = stream(f, bad.URL, StreamOptions{Policy: LocalAllowed})
	if err != nil || st.Status != http.StatusTeapot {
		t.Fatalf("an HTTP error status is the caller's to judge: %v %+v", err, st)
	}
	st.Close()
}

func TestStreamHasNoLimitOnTheWholeRequestUnlikeGet(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fl := w.(http.Flusher)
		for i := 0; i < 4; i++ {
			w.Write([]byte("abcd"))
			fl.Flush()
			time.Sleep(150 * time.Millisecond)
		}
	}))
	defer s.Close()
	f := testFetcher(map[string]addrClass{port(s): classLocal})
	f.Timeout = 200 * time.Millisecond // the whole transfer takes about 600 ms
	if _, err := get(f, s.URL, LocalAllowed); err == nil {
		t.Error("Get must still be limited by the timeout")
	}
	st, err := stream(f, s.URL, StreamOptions{Policy: LocalAllowed})
	if err != nil {
		t.Fatal(err)
	}
	if got := readAll(t, st); got != "abcdabcdabcdabcd" {
		t.Errorf("a slow stream must complete: %q", got)
	}
}

func TestStreamKeepsTheSameAddressRulesAsGet(t *testing.T) {
	local := httptest.NewServer(ok("secret"))
	defer local.Close()
	pub := httptest.NewTLSServer(redirectTo(local.URL))
	defer pub.Close()
	f := testFetcher(map[string]addrClass{port(local): classLocal, port(pub): classPublic}, pub.Certificate())
	_, err := stream(f, pub.URL, StreamOptions{Policy: LocalByFirstHop})
	wantErr(t, err, "a public address must not lead to a local address")

	plain := httptest.NewServer(ok("x"))
	defer plain.Close()
	f = testFetcher(map[string]addrClass{port(plain): classPublic})
	_, err = stream(f, plain.URL, StreamOptions{Policy: LocalForbidden})
	wantErr(t, err, "plain http is only allowed for local addresses")

	blocked := httptest.NewServer(ok("x"))
	defer blocked.Close()
	f = testFetcher(map[string]addrClass{port(blocked): classBlocked})
	_, err = stream(f, blocked.URL, StreamOptions{Policy: LocalAllowed})
	wantErr(t, err, "this address is not allowed")

	_, err = stream(f, "ftp://example.com/x", StreamOptions{})
	wantErr(t, err, "not a valid http(s) address")
	_, err = stream(f, "https://user:pw@example.com/x", StreamOptions{})
	wantErr(t, err, "credentials")
}

func TestStreamAsksTheCallerAboutTheFirstAddressAndEveryRedirect(t *testing.T) {
	target := httptest.NewServer(ok("payload"))
	defer target.Close()
	first := httptest.NewServer(redirectTo(target.URL))
	defer first.Close()
	f := testFetcher(map[string]addrClass{port(first): classLocal, port(target): classLocal})

	var asked []string
	allow := func(u *url.URL) error {
		asked = append(asked, u.Host)
		if u.Host == target.Listener.Addr().String() {
			return &RefusedError{Reason: "this host is not on the list"}
		}
		return nil
	}
	_, err := stream(f, first.URL, StreamOptions{Policy: LocalAllowed, AllowURL: allow})
	wantErr(t, err, "this host is not on the list")
	if len(asked) != 2 {
		t.Errorf("asked about %v, want the first address and the redirect target", asked)
	}

	_, err = stream(f, target.URL, StreamOptions{Policy: LocalAllowed, AllowURL: func(*url.URL) error { return &RefusedError{Reason: "no"} }})
	wantErr(t, err, "no")
}
