package crawl

import "context"

// attended.go (plan 198, step V2): work a person started and is watching may let the Browser tool open its Katana window when a site shows
// a check only a person can clear. The flag travels in the context of the call, like the kept login, and is sent to the Browser as
// `attended`. Auto-discovery, the recrawl workers and any other unattended run never set it, so they never open a window.

type attendedCtxKey struct{}

func withAttended(ctx context.Context, attended bool) context.Context {
	if !attended {
		return ctx
	}
	return context.WithValue(ctx, attendedCtxKey{}, true)
}

func attendedFrom(ctx context.Context) bool {
	v, _ := ctx.Value(attendedCtxKey{}).(bool)
	return v
}

// WithAttended is withAttended for the Katana AI service: a run of the AI that a person started (Generate) is attended.
func WithAttended(ctx context.Context, attended bool) context.Context {
	return withAttended(ctx, attended)
}

// plan 198, step V4: a crawl of job pages for a link whose effective engine is Katana says so (`engine: "katana"`), so the Browser may read
// it in the Katana window; a request without it (a Rod link) never goes there.
type katanaEngineCtxKey struct{}

// withKatanaIfSelected marks the context when Katana is the engine that reads the link (its own engine, else the Browser's).
func withKatanaIfSelected(ctx context.Context, coreURL, accessToken, linkID string) context.Context {
	if requireKatanaFor(ctx, coreURL, accessToken, linkID) != nil {
		return ctx
	}
	return context.WithValue(ctx, katanaEngineCtxKey{}, true)
}

func katanaEngineFrom(ctx context.Context) bool {
	v, _ := ctx.Value(katanaEngineCtxKey{}).(bool)
	return v
}

// engineField is what a Browser job-page request carries for the context: "katana" or nothing.
func engineField(ctx context.Context) string {
	if katanaEngineFrom(ctx) {
		return "katana"
	}
	return ""
}
