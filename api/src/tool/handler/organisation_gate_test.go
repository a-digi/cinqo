package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/organisation"
	"github.com/a-digi/cinqo/src/organisation/repository/persistent"
)

func TestTheGateAlwaysThrowsAwayAHeaderTheCallerSent(t *testing.T) {
	di, _ := newUploadDI(t)
	for _, sub := range []string{"", "u1"} {
		r := httptest.NewRequest("GET", "/x", nil)
		r.Header.Set(organisation.Header, "someone-elses-organisation")
		r.Header.Add(organisation.Header, "another")
		code, _ := organisationGate(di.db, r, sub)
		got := r.Header.Values(organisation.Header)
		if sub == "" && (code != 0 || len(got) != 0) {
			t.Errorf("no identity: forwarded with no header: %d %v", code, got)
		}
		if sub != "" && (code != 0 || len(got) != 1 || got[0] != "default") {
			t.Errorf("a user: the caller's own organisation replaces what was sent: %d %v", code, got)
		}
	}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header["x-cinqo-org"] = []string{"forged"} // a non-canonical spelling must not slip through either
	_, _ = organisationGate(di.db, r, "")
	for k := range r.Header {
		if strings.EqualFold(k, organisation.Header) {
			t.Errorf("%s survived: %v", k, r.Header[k])
		}
	}
}

func TestTheGateSetsTheCallersOwnOrganisationAndRefusesASuspendedOne(t *testing.T) {
	di, _ := newUploadDI(t)
	p := persistent.NewOrganisationPersistentRepo(di.db)
	acme, _ := p.Create("Acme")
	_ = p.Assign("ann", acme.ID)
	r := httptest.NewRequest("GET", "/x", nil)
	if code, _ := organisationGate(di.db, r, "ann"); code != 0 || r.Header.Get(organisation.Header) != acme.ID {
		t.Errorf("%d %q", code, r.Header.Get(organisation.Header))
	}
	_ = p.Update(acme.ID, "Acme", "suspended")
	r = httptest.NewRequest("GET", "/x", nil)
	r.Header.Set(organisation.Header, "forged")
	code, msg := organisationGate(di.db, r, "ann")
	if code != http.StatusForbidden || !strings.Contains(msg, "suspended") || r.Header.Get(organisation.Header) != "" {
		t.Errorf("a suspended organisation gets nothing through: %d %q header=%q", code, msg, r.Header.Get(organisation.Header))
	}
	r = httptest.NewRequest("GET", "/x", nil)
	if code, _ := organisationGate(di.db, r, "bob"); code != 0 || r.Header.Get(organisation.Header) != "default" {
		t.Errorf("a new user lands in the default organisation: %d %q", code, r.Header.Get(organisation.Header))
	}
}

func TestTheGateFailsClosedWhenTheOrganisationCannotBeRead(t *testing.T) {
	di, _ := newUploadDI(t)
	_ = di.db.Close()
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set(organisation.Header, "forged")
	code, _ := organisationGate(di.db, r, "ann")
	if code != http.StatusInternalServerError || r.Header.Get(organisation.Header) != "" {
		t.Errorf("%d %q", code, r.Header.Get(organisation.Header))
	}
}

// Every path that forwards to a tool goes through the gate, after the caller is identified and before the request is handed to the reverse proxy.
func TestTheProxyHandlerRunsTheGateAfterTheIdentityAndBeforeForwarding(t *testing.T) {
	raw, err := os.ReadFile("proxy_handler.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	id, gate, fwd := strings.Index(src, "callerIdentity(reqCtx)"), strings.Index(src, "organisationGate(db, r, sub)"), strings.Index(src, "proxy.ServeHTTP(w, r)")
	if id < 0 || gate < 0 || fwd < 0 || !(id < gate && gate < fwd) {
		t.Errorf("order: identity %d, gate %d, forward %d", id, gate, fwd)
	}
	if strings.Count(src, "proxy.ServeHTTP(") != 1 {
		t.Error("the handler must forward from exactly one place, behind the gate")
	}
}
