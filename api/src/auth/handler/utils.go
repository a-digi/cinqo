package auth_handler

import (
	"github.com/a-digi/coco-server/server/request"

	auth_config "github.com/a-digi/cinqo/src/auth/config"
)

// diStore is satisfied by this app's ContextBag. Get/Set aren't part of
// serverdi.Context itself, so every handler needing them re-asserts to
// this local interface off reqCtx.GetDI() — same pattern coco-mda uses.
type diStore interface {
	Get(key string) (any, bool)
}

func getAuthConfig(reqCtx request.RequestContext) (auth_config.AppAuthConfig, bool) {
	storeCtx, ok := reqCtx.GetDI().(diStore)
	if !ok {
		return auth_config.AppAuthConfig{}, false
	}
	raw, ok := storeCtx.Get("auth_config")
	if !ok {
		return auth_config.AppAuthConfig{}, false
	}
	cfg, ok := raw.(auth_config.AppAuthConfig)
	return cfg, ok
}
