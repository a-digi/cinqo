// Package main builds a Caddy binary as part of cinqo's own build
// pipeline — no xcaddy, no system-wide Caddy install required on any
// machine that builds this module. Verbatim copy of Caddy's own
// documented "build without xcaddy" entry point
// (github.com/caddyserver/caddy/v2/cmd/caddy/main.go): "There is no need
// to modify the Caddy source code to customize your builds... Copy this
// file into a new folder... go build - you now have a custom binary!"
// See plan/ai/deploy/local-caddy-plan.md.
package main

import (
	_ "time/tzdata"

	caddycmd "github.com/caddyserver/caddy/v2/cmd"

	// plug in Caddy modules here — modules/standard covers everything
	// this project's Caddyfile uses (reverse_proxy, file_server, handle,
	// try_files).
	_ "github.com/caddyserver/caddy/v2/modules/standard"
)

func main() {
	caddycmd.Main()
}
