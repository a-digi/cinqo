// Package routing provides a thin adapter so a plain function can satisfy
// coco-server's routing.HandlerInterface without declaring a new named
// type per handler. Own copy per coco-aim app convention — no code
// shared between cinqo and coco-mda/coco-release, even though this file
// is byte-for-byte identical to coco-mda's own
// api/config/routing/handlerfunc.go.
//
// This package exists because coco-lift's generic ApiResourceHandler
// (executor: ApiResourceHandler bound to {res:name} routes) cannot
// dispatch real HTTP requests against this coco-lift version — every
// request through that executor 400s with "malformed resource in url"
// (confirmed against coco-mda's own build notes for the identical
// coco-lift dependency). Until that's fixed upstream, cinqo registers
// one small explicit handler per verb per resource under its own
// executor name instead of executor: ApiResourceHandler.
package routing

import "github.com/a-digi/coco-server/server/request"

type HandlerFunc func(reqCtx request.RequestContext)

func (f HandlerFunc) ServeHTTP(reqCtx request.RequestContext) { f(reqCtx) }
