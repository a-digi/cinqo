package routes

import (
	"fmt"
	"os"

	lift_routes "github.com/a-digi/coco-lift/routes"
	lift_security "github.com/a-digi/coco-lift/security"
	serverdi "github.com/a-digi/coco-server/server/di"
	"github.com/a-digi/coco-server/server/routing"

	"github.com/a-digi/cinqo/config"
	local_routing "github.com/a-digi/cinqo/config/routing"
	auth_handler "github.com/a-digi/cinqo/src/auth/handler"
	auth_service "github.com/a-digi/cinqo/src/auth/service"
	"github.com/a-digi/cinqo/src/conversation"
	conversation_handler "github.com/a-digi/cinqo/src/conversation/handler"
	"github.com/a-digi/cinqo/src/health"
	"github.com/a-digi/cinqo/src/ping"
	"github.com/a-digi/cinqo/src/platform"
	platform_handler "github.com/a-digi/cinqo/src/platform/handler"
	"github.com/a-digi/cinqo/src/security/scopes"
	security_handler "github.com/a-digi/cinqo/src/security/scopes/handler"
	"github.com/a-digi/cinqo/src/tool"
	tool_handler "github.com/a-digi/cinqo/src/tool/handler"
)

// diStore mirrors src/auth/handler.diStore — Get isn't part of
// serverdi.Context, so the concrete ContextBag is re-asserted to this
// local interface to reach it.
type diStore interface {
	Get(key string) (any, bool)
}

// Init loads and merges every config/routes/route*.yaml file, registers
// this service's explicit per-verb handlers under their executor names,
// and wires the security layer.
//
// Security layer: prefers the JwksService main.go registers into DI
// ("jwks_service") — real coco-iam JWKS/RS256 validation with an HS256
// dev fallback. Falls back to building a validator straight from
// config.json's "auth" block only if no JwksService is in DI (matches
// coco-mda's own structure; in practice main.go always registers one, so
// this fallback exists for defense, not routine use).
//
// Wrapped in CookieSecurityLayer (auth/03), which promotes an httpOnly
// access_token cookie into an Authorization: Bearer header before this
// layer ever runs — so a browser session (cookie-based) and a direct API
// caller (real Authorization header) are both enforced identically.
func Init(ctx serverdi.Context) {
	routing.GlobalRouteBuilder.AddContext(ctx)

	updatedYamlBytes, _ := lift_routes.LoadRoutesYAML(config.ConfigFS)
	config.SetEnforcedScopes(updatedYamlBytes)

	// Security engine bridge — each domain that owns scopes registers
	// its own group here, explicitly, right next to where its handlers
	// get wired below. See plan/ai/security/security.md.
	scopes.Register(scopes.CoreScopeGroup)
	scopes.Register(ping.ScopeGroup)
	scopes.Register(tool.ScopeGroup)
	scopes.Register(platform.ScopeGroup)
	scopes.Register(conversation.ScopeGroup)

	handlerMap := map[string]routing.HandlerInterface{
		"HealthzGet": local_routing.HandlerFunc(health.GetHandler),

		"AuthConfig":   &auth_handler.AuthConfigHandler{},
		"AuthCallback": &auth_handler.OidcCallbackHandler{},
		"AuthRenew":    &auth_handler.TokenRenewHandler{},
		"AuthMe":       &auth_handler.MeHandler{},
		"AuthLogout":   &auth_handler.LogoutHandler{},

		"PingList":   local_routing.HandlerFunc(ping.ListHandler),
		"PingCreate": local_routing.HandlerFunc(ping.CreateHandler),

		"SecurityScopesList": &security_handler.ScopesListHandler{},

		"ToolList":           local_routing.HandlerFunc(tool_handler.ListHandler),
		"ToolInstall":        local_routing.HandlerFunc(tool_handler.InstallHandler),
		"ToolDelete":         local_routing.HandlerFunc(tool_handler.DeleteHandler),
		"ToolEnable":         local_routing.HandlerFunc(tool_handler.EnableHandler),
		"ToolDisable":        local_routing.HandlerFunc(tool_handler.DisableHandler),
		"ToolFrontendBundle": local_routing.HandlerFunc(tool_handler.FrontendBundleHandler),
		"ToolProxy":          local_routing.HandlerFunc(tool_handler.ProxyHandler),

		"PlatformList":      local_routing.HandlerFunc(platform_handler.ListPlatformsHandler),
		"PlatformKeyList":   local_routing.HandlerFunc(platform_handler.ListKeysHandler),
		"PlatformKeyCreate": local_routing.HandlerFunc(platform_handler.CreateKeyHandler),
		"PlatformKeyDelete": local_routing.HandlerFunc(platform_handler.DeleteKeyHandler),

		"ConversationCreate":         local_routing.HandlerFunc(conversation_handler.CreateHandler),
		"ConversationList":           local_routing.HandlerFunc(conversation_handler.ListHandler),
		"ConversationGet":            local_routing.HandlerFunc(conversation_handler.GetHandler),
		"ConversationUpdateTitle":    local_routing.HandlerFunc(conversation_handler.UpdateTitleHandler),
		"ConversationDelete":         local_routing.HandlerFunc(conversation_handler.DeleteHandler),
		"ConversationSendMessage":    local_routing.HandlerFunc(conversation_handler.SendMessageHandler),
		"ConversationGetActiveTurn":  local_routing.HandlerFunc(conversation_handler.GetActiveTurnHandler),
		"ConversationStopActiveTurn": local_routing.HandlerFunc(conversation_handler.StopActiveTurnHandler),
		"ConversationGetLogs":        local_routing.HandlerFunc(conversation_handler.GetLogsHandler),
		"ConversationGetAllLogs":     local_routing.HandlerFunc(conversation_handler.GetAllLogsHandler),
		"ConversationGetSettings":    local_routing.HandlerFunc(conversation_handler.GetSettingsHandler),
		"ConversationUpdateSettings": local_routing.HandlerFunc(conversation_handler.UpdateSettingsHandler),
	}

	var inner *lift_security.ScopeSecurityLayer
	if storeCtx, ok := ctx.(diStore); ok {
		if raw, ok := storeCtx.Get("jwks_service"); ok {
			if jwksSvc, ok := raw.(*auth_service.JwksService); ok {
				inner = &lift_security.ScopeSecurityLayer{Validator: jwksSvc}
			}
		}
	}
	if inner == nil {
		authCfgBytes, err := config.ReadConfigFile("config.json")
		if err != nil {
			fmt.Println("routes: failed to read auth config:", err)
			os.Exit(1)
		}
		inner = lift_security.NewScopeSecurityLayer(handlerMap, authCfgBytes, updatedYamlBytes)
	}

	routing.GlobalRouteBuilder.SetSecurityLayer(auth_handler.NewCookieSecurityLayer(inner))

	routing.GlobalRouteBuilder.AddRoute(routing.Routes{
		YamlContent: updatedYamlBytes,
		HandlerMap:  handlerMap,
	})
}
