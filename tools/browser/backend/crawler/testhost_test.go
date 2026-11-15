package crawler

import (
	"net"
	"strings"
	"testing"
)

func withTestHosts(t *testing.T, hosts ...string) {
	t.Helper()
	old := testHostAllowedFn
	testHostAllowedFn = func(h string) bool {
		for _, a := range hosts {
			if strings.EqualFold(a, h) {
				return true
			}
		}
		return false
	}
	t.Cleanup(func() { testHostAllowedFn = old })
}

func TestALoopbackTargetIsRefusedByDefault(t *testing.T) {
	withTestHosts(t)
	for _, u := range []string{"http://127.0.0.1/", "http://localhost/", "http://[::1]/", "http://10.0.0.1/", "http://192.168.1.1/", "http://169.254.169.254/latest", "http://0.0.0.0/"} {
		if err := validateCrawlURL(u); err == nil {
			t.Errorf("%s must be refused", u)
		}
	}
}

func TestAListedTestHostMayResolveToLoopbackAndNothingElse(t *testing.T) {
	withTestHosts(t, "localhost")
	if err := validateCrawlURL("http://localhost:8080/page"); err != nil {
		t.Errorf("a listed test host that resolves to loopback is allowed: %v", err)
	}
	// Every other loopback name stays refused: the match is on the exact host name.
	for _, u := range []string{"http://127.0.0.1/", "http://[::1]/", "http://sub.localhost/", "http://localhost.evil.test/"} {
		if err := validateCrawlURL(u); err == nil {
			t.Errorf("%s must still be refused (only the exact listed name is allowed)", u)
		}
	}
}

func TestEvenAListedHostNeverReachesPrivateOrLinkLocalAddresses(t *testing.T) {
	withTestHosts(t, "10.0.0.1", "192.168.1.1", "169.254.169.254", "0.0.0.0", "172.16.0.1")
	for _, u := range []string{"http://10.0.0.1/", "http://192.168.1.1/", "http://169.254.169.254/", "http://0.0.0.0/", "http://172.16.0.1/"} {
		if err := validateCrawlURL(u); err == nil {
			t.Errorf("%s must be refused even when listed: only loopback may be let through", u)
		}
	}
}

func TestTestHostMayBeLoopbackNeedsBothLoopbackAndAListedName(t *testing.T) {
	withTestHosts(t, "mock.test")
	if !testHostMayBeLoopback("mock.test", net.ParseIP("127.0.0.1")) || !testHostMayBeLoopback("MOCK.test", net.ParseIP("::1")) {
		t.Error("a listed name with a loopback address is allowed")
	}
	if testHostMayBeLoopback("mock.test", net.ParseIP("10.1.2.3")) || testHostMayBeLoopback("other.test", net.ParseIP("127.0.0.1")) {
		t.Error("a private address, or an unlisted name, is not")
	}
}
