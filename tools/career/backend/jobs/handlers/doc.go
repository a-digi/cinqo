// Package jobshandlers holds the HTTP routes of the jobs: the list and manual jobs, the match and the requirements, the locations, the recruiters of a job, the job board with its statuses, entries and usage, and the applications overview.
// Every handler here calls the functions its domain package already has and is reached through core's reverse proxy, which enforces the
// scope that manifest.json declares for the route; nothing here re-checks it. The handlers were moved out of the root package main
// into their domain's folder; main.go only registers them.
package jobshandlers
