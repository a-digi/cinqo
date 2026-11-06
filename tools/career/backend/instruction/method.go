package instruction

import "strings"

// ListingMethodFor is how a link's job list is crawled, decided by the Browser's selected engine (plan 190, now through the registry): the
// engine's own ListingMethod for a link that needs no login. A link that needs a login is always read with its AI instructions, and an
// engine that is not registered (an unknown name, an older Browser that sends none, or an engine that was removed) crawls with the AI
// instructions too. The engine name is trimmed but otherwise exact.
func ListingMethodFor(browserEngine string, requiresLogin bool) string {
	if requiresLogin {
		return MethodInstructions
	}
	if e, ok := ForBrowserEngine(strings.TrimSpace(browserEngine)); ok {
		return e.ListingMethod()
	}
	return MethodInstructions
}
