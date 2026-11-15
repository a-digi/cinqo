package handler

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/a-digi/cinqo/src/organisation"
	"github.com/a-digi/cinqo/src/organisation/entity"
)

// organisationGate is the last check before a request is forwarded to a tool. It always removes a header the caller sent. For a request with an identity
// (sub is the validated token's user id) it sets the header from the caller's own organisation, and it answers a status and message when the request must
// not go on: the user's organisation is suspended (403), or it could not be read (500, nothing is let through). A request without an identity (a route that
// opted into capability tokens) goes on with no organisation header at all. A zero status means "forward".
func organisationGate(db *sql.DB, r *http.Request, sub string) (int, string) {
	for k := range r.Header { // every spelling, not only the canonical one
		if strings.EqualFold(k, organisation.Header) {
			delete(r.Header, k)
		}
	}
	if sub == "" {
		return 0, ""
	}
	org, err := organisation.Of(db, sub)
	if err != nil {
		return http.StatusInternalServerError, "could not determine your organisation"
	}
	if org.Status == entity.StatusSuspended {
		return http.StatusForbidden, "your organisation is suspended"
	}
	r.Header.Set(organisation.Header, org.ID)
	return 0, ""
}
