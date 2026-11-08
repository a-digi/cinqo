package crawl

import (
	"context"
	"errors"

	"career-tool-backend/instruction"
	"career-tool-backend/portal"
)

// keptlogin.go (plan 195, step 4): a link that needs a login and has a login kept in the Browser tool is crawled without a login window, the
// Browser tool sending the kept session with every page it loads for the link. The session id travels in the context of the call, so the
// functions that load pages keep their signatures; the id is never logged and never shown (only the Browser tool can open it).

type keptLoginCtxKey struct{}

// withKeptLogin returns a context whose Browser calls name the kept login (an empty id changes nothing).
func withKeptLogin(ctx context.Context, sessionID string) context.Context {
	if sessionID == "" {
		return ctx
	}
	return context.WithValue(ctx, keptLoginCtxKey{}, keptLoginCtx{sessionID: sessionID})
}

// withKeptLoginFor is withKeptLogin that also remembers the link, so a login_expired answer deep in a crawl can mark that link.
func withKeptLoginFor(ctx context.Context, linkID, sessionID string) context.Context {
	if sessionID == "" {
		return ctx
	}
	return context.WithValue(ctx, keptLoginCtxKey{}, keptLoginCtx{sessionID: sessionID, linkID: linkID})
}

type keptLoginCtx struct{ sessionID, linkID string }

func keptLoginFrom(ctx context.Context) string {
	c, _ := ctx.Value(keptLoginCtxKey{}).(keptLoginCtx)
	return c.sessionID
}

func keptLoginLinkFrom(ctx context.Context) string {
	c, _ := ctx.Value(keptLoginCtxKey{}).(keptLoginCtx)
	return c.linkID
}

// keptLoginForRun returns the kept login a crawl of this link may use instead of a login window: the link needs a login, has a kept login
// that is not known to be expired, and the Browser's selected engine reads login links with it (Katana). "" for any other case, including an
// engine that cannot be read (the window path then stays as it was).
func keptLoginForRun(ctx context.Context, coreURL, accessToken, linkID string) string {
	id := portal.KeptLoginFor(linkID)
	if id == "" {
		return ""
	}
	engine, err := effectiveEngine(ctx, coreURL, accessToken, linkID) // plan 197: a link on Rod never uses a kept login
	if err != nil || !instruction.KeptLoginUsedBy(engine) {
		return ""
	}
	return id
}

// loginExpiredError is the Browser tool's answer when the site no longer accepts a link's kept login.
type loginExpiredError struct{ Message string }

func (e *loginExpiredError) Error() string {
	if e.Message == "" {
		return "The kept login no longer works: log in again and keep the login"
	}
	return e.Message
}

// noteLoginExpired marks the link of the context (when it has one) as having an expired login, if err is a login_expired answer.
func noteLoginExpired(ctx context.Context, err error) {
	var le *loginExpiredError
	if errors.As(err, &le) {
		if linkID := keptLoginLinkFrom(ctx); linkID != "" {
			_ = portal.MarkLoginSessionExpired(linkID)
		}
	}
}

// codeLoginExpired is the Browser tool's answer when the site no longer accepts the kept login.
const codeLoginExpired = "login_expired"

// noteFetchOutcome (plan 197, step 4) is called after every Katana-only read of a link's page (the link list, the job page analysis): a read
// that went through forgets a recorded block, a read the site stopped (Cloudflare, a rate limit, another bot wall) records it so the page can
// suggest Rod, and a login_expired answer is noted as before. linkID may be empty (nothing to mark).
func noteFetchOutcome(linkID string, err error) {
	if linkID == "" {
		return
	}
	noteLoginVerdict(linkID, err)
	if err == nil {
		_ = portal.ClearEngineBlock(linkID)
		return
	}
	var fe *linkFetchError
	if errors.As(err, &fe) && portal.IsEngineBlockCode(fe.Code) {
		_ = portal.NoteEngineBlock(linkID, fe.Code)
	}
}

// NoteFetchOutcome is noteFetchOutcome for the Katana AI service (its page reads go through LoadPageLinks and the rule test).
func NoteFetchOutcome(linkID string, err error) { noteFetchOutcome(linkID, err) }

// noteLoginVerdict records a login_expired answer of the Browser tool on the link, so the page offers the login again. Other errors are
// ignored. linkID may be empty (no link to mark).
func noteLoginVerdict(linkID string, err error) {
	var fe *linkFetchError
	if linkID != "" && errors.As(err, &fe) && fe.Code == codeLoginExpired {
		_ = portal.MarkLoginSessionExpired(linkID)
	}
}
