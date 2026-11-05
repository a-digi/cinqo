// Package portalhandlers holds the HTTP routes of the job portals and their links: the portals, the AI overrides, the links and the link pipeline.
// Every handler here calls the functions its domain package already has and is reached through core's reverse proxy, which enforces the
// scope that manifest.json declares for the route; nothing here re-checks it. The handlers were moved out of the root package main
// into their domain's folder; main.go only registers them.
package portalhandlers
