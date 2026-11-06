package instruction

import "strings"

// ListingMethodFor is how a link's job list is crawled, decided by the Browser's selected engine (plan 190, now through the registry): the
// engine's own ListingMethod for a link that needs no login. A link that needs a login is always read with its AI instructions, and an
// engine that is not registered (an unknown name, an older Browser that sends none, or an engine that was removed) crawls with the AI
// instructions too. The engine name is trimmed but otherwise exact.
func ListingMethodFor(browserEngine string, requiresLogin bool) string {
	return ListingMethodForLink(browserEngine, requiresLogin, false)
}

// ListingMethodForLink is ListingMethodFor for a link that may have a login kept in the Browser tool (plan 195, step 4): such a link is read
// by the engine's own method like a link that needs no login, when the engine uses a kept login (UsesKeptLogin); every other login link keeps
// the AI instructions and the login window.
func ListingMethodForLink(browserEngine string, requiresLogin, keptLogin bool) string {
	if requiresLogin && !(keptLogin && KeptLoginUsedBy(browserEngine)) {
		return MethodInstructions
	}
	if e, ok := ForBrowserEngine(strings.TrimSpace(browserEngine)); ok {
		return e.ListingMethod()
	}
	return MethodInstructions
}

// KeptLoginUser is implemented by an engine that reads a link that needs a login with a login kept in the Browser tool (Katana). An engine
// without it (Rod) reads such a link with its AI instructions in the login window, as before.
type KeptLoginUser interface {
	UsesKeptLogin() bool
}

// KeptLoginUsedBy says whether the Browser's selected engine reads login links with a kept login. false for an engine that is not registered.
func KeptLoginUsedBy(browserEngine string) bool {
	e, ok := ForBrowserEngine(strings.TrimSpace(browserEngine))
	if !ok {
		return false
	}
	u, ok := e.(KeptLoginUser)
	return ok && u.UsesKeptLogin()
}
