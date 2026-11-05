// Package cvhandlers holds the HTTP routes of a job's CV: the PDF, the link to a CV document and the generations.
// Every handler here calls the functions its domain package already has and is reached through core's reverse proxy, which enforces the
// scope that manifest.json declares for the route; nothing here re-checks it. The handlers were moved out of the root package main
// into their domain's folder; main.go only registers them.
package cvhandlers
