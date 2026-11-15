package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/a-digi/cinqo/src/organisation"
	organisation_entity "github.com/a-digi/cinqo/src/organisation/entity"
	organisation_query "github.com/a-digi/cinqo/src/organisation/repository/query"
	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
	tool_persistent "github.com/a-digi/cinqo/src/tool/repository/persistent"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// Unattended calls (plan/ai/tools/namako/saas/s7-unattended-monitoring.md, step 3): one tool calling a route of another tool through the proxy with its OWN service
// token, while nobody is signed in. Everything that lets such a call through is checked on every call: the token, that the calling tool is running, that the route
// is one the target marks allow_background RIGHT NOW, that an administrator approved exactly that route for the caller, that the organisation exists, is active and
// has unattended use switched on, and that the caller is within its rate limit. The service token is removed before the request goes on: the target never sees it.

// serviceAuthScheme is the Authorization scheme of a tool's service token (never "Bearer", like the other routes that take it).
const serviceAuthScheme = "ToolService "

// killSwitchEnv, set to "off", stops every unattended call at once (the name lives in the organisation package, which the operator's page also reads).
const killSwitchEnv = organisation.BackgroundKillSwitchEnv

// The limits per (calling tool, organisation): requests in any 60 seconds and calls in flight at the same time. Variables so tests can lower them.
var (
	backgroundPerMinute  = 60
	backgroundConcurrent = 3
)

// backgroundOrgPattern is what an organisation id may look like (core makes UUIDs, and "default").
var backgroundOrgPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,63}$`)

// isServiceCall says whether a request announces itself as a tool's own call.
func isServiceCall(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Authorization"), serviceAuthScheme)
}

// scrubInternalHeaders removes from a request every header only core may set (in any spelling) and returns what the caller put in ActingOrgHeader. It runs on
// every proxied request first, so a person can never reach a tool with a header that says another organisation or another calling tool.
func scrubInternalHeaders(r *http.Request) (acting string) {
	for k, v := range r.Header {
		switch {
		case strings.EqualFold(k, organisation.ActingOrgHeader):
			if len(v) > 0 {
				acting = strings.TrimSpace(v[0])
			}
			delete(r.Header, k)
		case strings.EqualFold(k, organisation.Header), strings.EqualFold(k, organisation.CallerToolHeader):
			delete(r.Header, k)
		}
	}
	return acting
}

// limiter keeps, per key, the times of the calls of the last minute and how many are in flight.
type limiter struct {
	mu     sync.Mutex
	now    func() time.Time
	hits   map[string][]time.Time
	active map[string]int
}

func newLimiter() *limiter {
	return &limiter{now: time.Now, hits: map[string][]time.Time{}, active: map[string]int{}}
}

var backgroundLimiter = newLimiter()

// acquire takes one call for key. It refuses (with how long to wait) when perMinute calls were made in the last minute or concurrent calls are in flight; a
// refused call is not recorded. release must be called when the call is over.
func (l *limiter) acquire(key string, perMinute, concurrent int) (release func(), retryAfter time.Duration, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	hits := l.hits[key][:0:0]
	for _, t := range l.hits[key] {
		if now.Sub(t) < time.Minute {
			hits = append(hits, t)
		}
	}
	if len(hits) >= perMinute {
		l.hits[key] = hits
		return nil, hits[0].Add(time.Minute).Sub(now), false
	}
	if l.active[key] >= concurrent {
		l.hits[key] = hits
		return nil, time.Second, false
	}
	l.hits[key] = append(hits, now)
	l.active[key]++
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			if l.active[key]--; l.active[key] <= 0 {
				delete(l.active, key)
			}
			if len(l.hits[key]) == 0 {
				delete(l.hits, key)
			}
		})
	}, 0, true
}

// serviceResult is the verdict on an unattended call: a zero code means "forward it" (and release must be called when it is over).
type serviceResult struct {
	code       int
	message    string
	retryAfter time.Duration
	release    func()
}

func refuse(code int, message string) serviceResult {
	return serviceResult{code: code, message: message}
}

// serviceCall decides an unattended call to a route of the target tool and, when it is allowed, rewrites the request for the target: the organisation header set
// from the checked organisation, the calling tool named, and the caller's credentials removed.
func serviceCall(db *sql.DB, r *http.Request, acting string, target *tool_entity.Tool, route *tool_entity.ToolRoute, remainder string) serviceResult {
	if organisation.BackgroundKillSwitchOn() {
		return refuse(http.StatusServiceUnavailable, "unattended calls are switched off")
	}
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), serviceAuthScheme))
	if token == "" {
		return refuse(http.StatusUnauthorized, "missing or malformed service token")
	}
	caller, err := tool_query.NewToolQueryRepo(db).FindByServiceToken(token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return refuse(http.StatusUnauthorized, "invalid service token")
		}
		return refuse(http.StatusInternalServerError, "failed to authenticate")
	}
	counter := tool_persistent.NewToolBackgroundPersistentRepo(db)
	orgID := ""
	if backgroundOrgPattern.MatchString(acting) {
		orgID = acting
	}
	denied := func(code int, message string) serviceResult {
		_ = counter.CountCall(caller.ID, orgID, true) // best effort: a counter that cannot be written must not turn a refusal into an error
		return refuse(code, message)
	}
	if !caller.Enabled || caller.Status != "running" {
		return denied(http.StatusForbidden, "the calling tool is not running")
	}
	if !route.AllowBackground {
		return denied(http.StatusForbidden, "that route cannot be used without a signed-in person")
	}
	granted, err := tool_query.NewToolBackgroundQueryRepo(db).IsGranted(caller.ID, target.Slug, route.Method, remainder)
	if err != nil {
		return refuse(http.StatusInternalServerError, "failed to check the approval")
	}
	if !granted {
		return denied(http.StatusForbidden, "the calling tool has no approval to use that route")
	}
	if orgID == "" {
		return denied(http.StatusForbidden, "no organisation named")
	}
	oq := organisation_query.NewOrganisationQueryRepo(db)
	org, err := oq.FindByID(orgID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return denied(http.StatusForbidden, "unknown organisation")
		}
		return refuse(http.StatusInternalServerError, "failed to read the organisation")
	}
	if org.Status != organisation_entity.StatusActive {
		return denied(http.StatusForbidden, "the organisation is suspended")
	}
	on, err := oq.BackgroundEnabled(orgID)
	if err != nil {
		return refuse(http.StatusInternalServerError, "failed to read the organisation")
	}
	if !on {
		return denied(http.StatusForbidden, "unattended use is not switched on for that organisation")
	}
	release, wait, ok := backgroundLimiter.acquire(caller.ID+"\x00"+orgID, backgroundPerMinute, backgroundConcurrent)
	if !ok {
		res := denied(http.StatusTooManyRequests, "too many unattended calls: slow down")
		res.retryAfter = wait
		return res
	}
	_ = counter.CountCall(caller.ID, orgID, false)
	r.Header.Del("Authorization")
	r.Header.Del("Cookie")
	r.Header.Set(organisation.Header, orgID)
	r.Header.Set(organisation.CallerToolHeader, caller.Slug)
	return serviceResult{release: release}
}
