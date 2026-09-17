package entity

// ToolRoute is one backend route a tool declared in its manifest —
// consumed entirely by the reverse-proxy handler (see
// plan/ai/tools/step-05-reverse-proxy-and-request-enforcement.md) to
// decide which paths under a tool's proxy prefix actually exist and
// what scope each requires. Unrelated to cinqo's own
// route-*.yaml/ScopeSecurityLayer — a tool's routes are never
// statically declared there.
type ToolRoute struct {
	_             struct{} `table:"tool_routes"`
	ToolID        string   `db:"tool_id" dbtype:"TEXT" nullable:"false" json:"tool_id"`
	Method        string   `db:"method" dbtype:"TEXT" nullable:"false" json:"method"`
	PathSuffix    string   `db:"path_suffix" dbtype:"TEXT" nullable:"false" json:"path_suffix"`
	RequiredScope string   `db:"required_scope" dbtype:"TEXT" nullable:"false" json:"required_scope"`
	// AllowCapabilityToken opts a single route out of ProxyHandler's own
	// bearer/cookie scope check entirely (see proxy_handler.go's
	// callerScopes) — for routes whose caller is a stateless tool
	// subprocess (e.g. pdf_tools fetching a career cv-import/file URL)
	// that structurally can never present a session token. Security for
	// such a route is delegated wholesale to the downstream tool's own
	// handler (e.g. a random capability token in the query string) —
	// RequiredScope above is then unused for that route, since
	// ProxyHandler never evaluates it when this is true.
	AllowCapabilityToken bool `db:"allow_capability_token" dbtype:"INTEGER" nullable:"false" json:"allow_capability_token"`
}
